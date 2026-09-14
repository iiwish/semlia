package httpapi_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	discoveryfiles "github.com/iiwish/semlia/internal/adapters/discovery/files"
	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	ingestionapp "github.com/iiwish/semlia/internal/application/ingestion"
	applicationdomain "github.com/iiwish/semlia/internal/domain"
	"github.com/iiwish/semlia/internal/domain/authorization"
	discoverydomain "github.com/iiwish/semlia/internal/domain/discovery"
	ingestiondomain "github.com/iiwish/semlia/internal/domain/ingestion"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

type ingestionHTTPRepository struct {
	ingestionapp.Repository
	created ingestionapp.CreateArtifactCommand
}

func (*ingestionHTTPRepository) ListArtifacts(_ context.Context, query ingestionapp.ListArtifactsQuery) (ingestiondomain.Page[ingestiondomain.Artifact], error) {
	if query.PrincipalID.IsZero() || query.AuthorizationVersion != 7 {
		return ingestiondomain.Page[ingestiondomain.Artifact]{}, ingestiondomain.ErrInvalid
	}
	return ingestiondomain.Page[ingestiondomain.Artifact]{}, nil
}

type localIngestionAuthorizationRepository struct {
	authorizationapp.Repository
	principal authorization.Principal
	grant     bool
}

type localIngestionDiscoveryRepository struct {
	discoveryapp.SourceRepository
	principals []identity.PrincipalID
}

func (repo *localIngestionDiscoveryRepository) ListDiscoverySources(_ context.Context, query discoveryapp.ListSourcesQuery) (discoverydomain.SourcePage, error) {
	repo.principals = append(repo.principals, query.PrincipalID)
	return discoverydomain.SourcePage{}, nil
}
func (repo *localIngestionDiscoveryRepository) ListDiscoveryRuns(_ context.Context, query discoveryapp.ListRunsQuery) (discoverydomain.RunPage, error) {
	repo.principals = append(repo.principals, query.PrincipalID)
	return discoverydomain.RunPage{}, nil
}
func (repo *localIngestionDiscoveryRepository) ListDiscoveryCandidates(_ context.Context, query discoveryapp.ListCandidatesQuery) (discoverydomain.CandidatePage, error) {
	repo.principals = append(repo.principals, query.PrincipalID)
	return discoverydomain.CandidatePage{}, nil
}

func (repo *localIngestionAuthorizationRepository) LoadLocalUATPrincipal(_ context.Context, workspace identity.WorkspaceID, seed, role string) (authorization.Principal, error) {
	if workspace != repo.principal.WorkspaceID || seed != "principal" || role != "workspace_admin" {
		return authorization.Principal{}, authorization.ErrNotFound
	}
	return repo.principal, nil
}
func (repo *localIngestionAuthorizationRepository) LoadPrincipal(_ context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID) (authorization.Principal, error) {
	if workspace != repo.principal.WorkspaceID || principal != repo.principal.ID {
		return authorization.Principal{}, authorization.ErrNotFound
	}
	return repo.principal, nil
}
func (repo *localIngestionAuthorizationRepository) LoadPrincipalBindings(context.Context, identity.PrincipalID) ([]authorization.RoleBinding, error) {
	if !repo.grant {
		return nil, nil
	}
	return []authorization.RoleBinding{{PrincipalID: repo.principal.ID, ScopeType: authorization.ScopeWorkspace,
		ScopeID: repo.principal.WorkspaceID.UUID(), Actions: []authorization.Action{authorization.ActionSourceManage, authorization.ActionSourceRead}}}, nil
}
func (*localIngestionAuthorizationRepository) AuthorizationVersion(context.Context, identity.WorkspaceID) (int64, error) {
	return 7, nil
}
func (*localIngestionAuthorizationRepository) RecordDecision(context.Context, authorization.DecisionEvent) error {
	return nil
}

func TestIngestionHTTPUsesAuthorizedLocalPrincipalWithoutEnablingAliasesInProduction(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	for _, scenario := range []struct {
		name         string
		local, grant bool
		want         int
	}{
		{"local-author", true, true, http.StatusCreated},
		{"local-denied", true, false, http.StatusForbidden},
		{"production-alias-denied", false, true, http.StatusForbidden},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			authRepo := &localIngestionAuthorizationRepository{principal: authorization.Principal{ID: principal, WorkspaceID: workspace, Kind: authorization.PrincipalHuman, Status: authorization.PrincipalActive}, grant: scenario.grant}
			var options []authorizationapp.Option
			if scenario.local {
				options = append(options, authorizationapp.WithLocalUATIdentities())
			}
			authorizer := authorizationapp.NewService(authRepo, authorizationapp.ClockFunc(time.Now), options...)
			repo := &ingestionHTTPRepository{}
			service := ingestionapp.NewService(repo, &ingestionHTTPStore{values: map[string][]byte{}}, authorizer, ingestionapp.ClockFunc(time.Now), map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{ingestiondomain.ArtifactCSV: discoveryfiles.CSV()})
			discoveryRepo := &localIngestionDiscoveryRepository{}
			loader, err := discoveryapp.NewArtifactLoader(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			control := discoveryapp.NewControlService(discoveryRepo, nil, nil, loader, nil, authorizer, discoveryapp.ClockFunc(time.Now))
			handler := ingestionHTTPHandler(t, service, httpapi.WithDiscovery(control))
			request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspace.String()+"/ingestion/artifacts", bytes.NewBufferString("id,name\n1,Alice\n"))
			request.Header.Set("X-Semlia-Principal", "local-author")
			request.Header.Set("X-Artifact-Kind", "csv")
			request.Header.Set("X-File-Name", "people.csv")
			request.Header.Set("Content-Type", "text/csv")
			request.Header.Set("Idempotency-Key", scenario.name)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != scenario.want {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if scenario.want == http.StatusCreated {
				if repo.created.Artifact.UploadedByPrincipalID == nil || *repo.created.Artifact.UploadedByPrincipalID != principal {
					t.Fatalf("actor=%+v", repo.created.Artifact)
				}
				request = httptest.NewRequest(http.MethodGet, request.URL.String(), nil)
				request.Header.Set("X-Semlia-Principal", "local-author")
				response = httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
				}
				source, _ := identity.NewSourceConnectionID()
				for _, path := range []string{"sources", "sources/" + source.String() + "/discovery-runs", "semantic-candidates"} {
					request = httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/"+path, nil)
					request.Header.Set("X-Semlia-Principal", "local-author")
					response = httptest.NewRecorder()
					handler.ServeHTTP(response, request)
					if response.Code != http.StatusOK {
						t.Fatalf("GET %s status=%d body=%s", path, response.Code, response.Body.String())
					}
				}
				if len(discoveryRepo.principals) != 3 {
					t.Fatalf("queries=%d", len(discoveryRepo.principals))
				}
				for _, actor := range discoveryRepo.principals {
					if actor != principal {
						t.Fatalf("query principal=%s", actor)
					}
				}
			} else if !repo.created.Artifact.ID.IsZero() {
				t.Fatal("denied request wrote artifact")
			}
		})
	}
}

func (*ingestionHTTPRepository) FindArtifactByIdempotency(context.Context, identity.WorkspaceID, string, string, int64) (*ingestiondomain.Artifact, error) {
	return nil, nil
}

func (*ingestionHTTPRepository) ReserveArtifactObject(context.Context, ingestionapp.CreateArtifactCommand) error {
	return nil
}

func (*ingestionHTTPRepository) ReleaseArtifactObjectReservation(context.Context, ingestionapp.CreateArtifactCommand, func(context.Context, ingestiondomain.CleanupObject) error) error {
	return nil
}

func (repository *ingestionHTTPRepository) CreateArtifact(_ context.Context, command ingestionapp.CreateArtifactCommand) (ingestiondomain.Artifact, error) {
	repository.created = command
	return command.Artifact, nil
}

type ingestionHTTPStore struct{ values map[string][]byte }

func (store *ingestionHTTPStore) Put(_ context.Context, _ identity.WorkspaceID, digest string, input io.Reader, _ int64) error {
	value, err := io.ReadAll(input)
	store.values[digest] = value
	return err
}

func (store *ingestionHTTPStore) Open(_ context.Context, _ identity.WorkspaceID, digest string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(store.values[digest])), nil
}

func (*ingestionHTTPStore) Delete(context.Context, identity.WorkspaceID, string) error { return nil }
func (*ingestionHTTPStore) Check(context.Context) error                                { return nil }
func (*ingestionHTTPStore) Configured() bool                                           { return true }

func TestUploadEndpointBindsExistingSourceHeadersAndRejectsPublicSQL(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	source, _ := identity.NewSourceConnectionID()
	repository := &ingestionHTTPRepository{}
	store := &ingestionHTTPStore{values: map[string][]byte{}}
	service := ingestionapp.NewService(repository, store, nil, ingestionapp.ClockFunc(time.Now),
		map[ingestiondomain.ArtifactKind]discoverydomain.Adapter{ingestiondomain.ArtifactCSV: discoveryfiles.CSV()})
	handler := ingestionHTTPHandler(t, service)

	content := []byte("id,name\n1,Alice\n")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspace.String()+"/ingestion/artifacts", bytes.NewReader(content))
	request.Header.Set("X-Semlia-Principal", principal.String())
	request.Header.Set("X-Artifact-Kind", "csv")
	request.Header.Set("X-File-Name", "people.csv")
	request.Header.Set("Content-Type", "text/csv")
	request.Header.Set("Idempotency-Key", "source-scoped-upload")
	request.Header.Set("X-Source-Id", source.String())
	request.Header.Set("X-Expected-Source-Version", "7")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if repository.created.Artifact.SourceConnectionID == nil || *repository.created.Artifact.SourceConnectionID != source ||
		repository.created.ExpectedSourceVersion == nil || *repository.created.ExpectedSourceVersion != 7 {
		t.Fatalf("created=%+v", repository.created)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspace.String()+"/ingestion/artifacts", bytes.NewReader([]byte("select 1")))
	request.Header.Set("X-Semlia-Principal", principal.String())
	request.Header.Set("X-Artifact-Kind", "sql")
	request.Header.Set("X-File-Name", "model.sql")
	request.Header.Set("Content-Type", "application/sql")
	request.Header.Set("Idempotency-Key", "public-sql")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("public SQL status=%d body=%s", response.Code, response.Body.String())
	}
}

func ingestionHTTPHandler(t *testing.T, ingestion *ingestionapp.Service, options ...httpapi.Option) http.Handler {
	t.Helper()
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	system := application.NewSystemService(application.ReadinessProbeFunc(func(context.Context) error { return nil }), applicationdomain.SystemInfo{
		APIVersion: "v1", SchemaVersion: "0.1.0", BuildVersion: "test",
	})
	schedules := ingestionapp.NewScheduleService(nil, nil, ingestionapp.ClockFunc(time.Now))
	options = append(options, httpapi.WithIngestion(ingestion, schedules))
	return httpapi.NewHandler(system, slog.New(slog.NewTextHandler(io.Discard, nil)), provider.Tracer("ingestion-http-test"),
		options...)
}

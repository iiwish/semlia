package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	"github.com/iiwish/semlia/internal/domain"
	catalogdomain "github.com/iiwish/semlia/internal/domain/catalog"
	"github.com/iiwish/semlia/internal/domain/semantic"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

func TestCatalogRoutesMapDynamicPathsAndSafeResponses(t *testing.T) {
	repository := newCatalogRepository(t)
	handler, logs := newCatalogHandler(t, repository)
	path := "/api/v1/workspaces/" + repository.workspace.String() + "/catalog/assets?limit=2"
	response := request(t, handler, http.MethodGet, path, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(response.Header().Get("X-Trace-ID")) != 32 {
		t.Fatalf("trace header = %q", response.Header().Get("X-Trace-ID"))
	}
	if !strings.Contains(response.Body.String(), repository.asset.String()) || !strings.Contains(response.Body.String(), "commerce.net_revenue") {
		t.Fatalf("catalog response = %s", response.Body.String())
	}
	if strings.Contains(logs.String(), repository.workspace.String()) || strings.Contains(logs.String(), repository.asset.String()) {
		t.Fatalf("dynamic IDs leaked into route log: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "/api/v1/workspaces/{workspaceId}/catalog/assets") {
		t.Fatalf("route template missing from log: %s", logs.String())
	}
}

func TestCatalogCreateUsesTraceAndReturnsCreatedDetail(t *testing.T) {
	repository := newCatalogRepository(t)
	handler, _ := newCatalogHandler(t, repository)
	body := `{"address":"commerce.net_revenue","assetType":"metric","schemaVersion":"1.0.0","content":{"name":"Net revenue"},"createdBy":"founder"}`
	path := "/api/v1/workspaces/" + repository.workspace.String() + "/catalog/assets"
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("traceparent", "00-"+incomingTraceID+"-00f067aa0ba902b7-01")
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.created.TraceID != incomingTraceID || repository.created.ContentDigest == "" {
		t.Fatalf("create command = %+v", repository.created)
	}
	assertJSONField(t, response.Body.Bytes(), "address", "commerce.net_revenue")
}

func TestWorkspaceBootstrapUsesPublicTypeID(t *testing.T) {
	repository := newCatalogRepository(t)
	handler, _ := newCatalogHandler(t, repository)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/workspaces", strings.NewReader(`{"slug":"semantic-core","displayName":"Semantic Core"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "slug", "semantic-core")
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if id, _ := payload["id"].(string); !strings.HasPrefix(id, "wsp_") {
		t.Fatalf("workspace id = %q", id)
	}
}

func TestCatalogValidationAndErrorsUseStableEnvelopes(t *testing.T) {
	repository := newCatalogRepository(t)
	handler, _ := newCatalogHandler(t, repository)
	base := "/api/v1/workspaces/" + repository.workspace.String() + "/catalog/assets"
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{"wrong prefix", http.MethodGet, "/api/v1/workspaces/" + repository.asset.String() + "/catalog/assets", "", 400, "INVALID_ARGUMENT"},
		{"invalid limit", http.MethodGet, base + "?limit=999", "", 400, "INVALID_ARGUMENT"},
		{"unknown body field", http.MethodPost, base, `{"unknown":true}`, 400, "INVALID_ARGUMENT"},
		{"method", http.MethodDelete, base, "", 405, "METHOD_NOT_ALLOWED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
			if response.Code != test.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			assertJSONField(t, response.Body.Bytes(), "code", test.code)
		})
	}

	repository.getErr = catalogdomain.ErrNotFound
	response := request(t, handler, http.MethodGet, base+"/"+repository.asset.String(), "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d", response.Code)
	}
	assertJSONField(t, response.Body.Bytes(), "code", "NOT_FOUND")
}

func TestCatalogRoutesRequireConfiguredRepository(t *testing.T) {
	handler, _ := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error { return nil }))
	workspace := mustCatalogID(t, identity.NewWorkspaceID)
	response := request(t, handler, http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/catalog/assets", "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "DEPENDENCY_UNAVAILABLE")
}

type catalogRepository struct {
	workspace identity.WorkspaceID
	asset     identity.AssetID
	revision  identity.RevisionID
	created   catalogdomain.CreateAssetCommand
	getErr    error
}

func (repository *catalogRepository) ListCatalogWorkspaces(context.Context) ([]catalogdomain.Workspace, error) {
	return nil, nil
}

func (repository *catalogRepository) CreateCatalogWorkspace(_ context.Context, command catalogdomain.CreateWorkspaceCommand) (catalogdomain.Workspace, error) {
	return catalogdomain.Workspace{ID: command.ID, Slug: command.Slug, DisplayName: command.DisplayName, CreatedAt: command.CreatedAt, UpdatedAt: command.CreatedAt}, nil
}

func newCatalogRepository(t *testing.T) *catalogRepository {
	t.Helper()
	return &catalogRepository{
		workspace: mustCatalogID(t, identity.NewWorkspaceID),
		asset:     mustCatalogID(t, identity.NewAssetID),
		revision:  mustCatalogID(t, identity.NewRevisionID),
	}
}

func (repository *catalogRepository) summary(t time.Time) catalogdomain.AssetSummary {
	address, _ := semantic.NewAddress("commerce", "net_revenue")
	return catalogdomain.AssetSummary{
		ID: repository.asset, Address: address, Type: semantic.Metric, LifecycleState: "active",
		CurrentRevisionID: &repository.revision, Title: "Net revenue", Summary: "Revenue after refunds", UpdatedAt: t,
	}
}

func (repository *catalogRepository) detail(t time.Time) catalogdomain.AssetDetail {
	summary := repository.summary(t)
	revision := catalogdomain.Revision{
		ID: repository.revision, AssetID: repository.asset, Sequence: 1, SchemaVersion: "1.0.0",
		ContentDigest: strings.Repeat("a", 64), Content: json.RawMessage(`{"name":"Net revenue"}`),
		CreatedBy: "founder", CreatedAt: t, Evidence: []catalogdomain.Evidence{},
	}
	return catalogdomain.AssetDetail{AssetSummary: summary, CreatedAt: t, CurrentRevision: &revision}
}

func (repository *catalogRepository) ListCatalogAssets(context.Context, catalogdomain.ListAssetsQuery) ([]catalogdomain.AssetSummary, error) {
	return []catalogdomain.AssetSummary{repository.summary(time.Now().UTC())}, nil
}

func (repository *catalogRepository) CountCatalogAssets(context.Context, catalogdomain.ListAssetsQuery) (int64, error) {
	return 1, repository.getErr
}

func (repository *catalogRepository) GetCatalogAsset(context.Context, identity.WorkspaceID, identity.AssetID) (catalogdomain.AssetDetail, error) {
	if repository.getErr != nil {
		return catalogdomain.AssetDetail{}, repository.getErr
	}
	return repository.detail(time.Now().UTC()), nil
}

func (repository *catalogRepository) ListCatalogAuthorityRecords(context.Context, catalogdomain.ListAuthorityRecordsQuery) (catalogdomain.AuthorityRecordResult, error) {
	return catalogdomain.AuthorityRecordResult{}, repository.getErr
}

func (repository *catalogRepository) CreateCatalogAsset(_ context.Context, command catalogdomain.CreateAssetCommand) (catalogdomain.AssetDetail, error) {
	repository.created = command
	repository.asset, repository.revision = command.AssetID, command.RevisionID
	return repository.detail(command.CreatedAt), nil
}

func (repository *catalogRepository) ListCatalogRevisions(context.Context, catalogdomain.ListRevisionsQuery) ([]catalogdomain.Revision, error) {
	return nil, nil
}

func (repository *catalogRepository) CountCatalogRevisions(context.Context, identity.WorkspaceID, identity.AssetID) (int64, error) {
	return 0, repository.getErr
}

func (repository *catalogRepository) GetCatalogRevision(context.Context, identity.WorkspaceID, identity.AssetID, identity.RevisionID) (catalogdomain.Revision, error) {
	return catalogdomain.Revision{}, repository.getErr
}

func (repository *catalogRepository) AppendCatalogRevision(context.Context, catalogdomain.AppendRevisionCommand) (catalogdomain.Revision, error) {
	return catalogdomain.Revision{}, nil
}

func (repository *catalogRepository) ListCatalogRelations(context.Context, catalogdomain.ListRelationsQuery) ([]catalogdomain.Relation, error) {
	return nil, nil
}

func (repository *catalogRepository) GetCatalogDiscoveryRun(context.Context, identity.WorkspaceID, identity.RunID) (catalogdomain.DiscoveryRun, error) {
	return catalogdomain.DiscoveryRun{}, repository.getErr
}

func newCatalogHandler(t *testing.T, repository *catalogRepository) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	system := application.NewSystemService(application.ReadinessProbeFunc(func(context.Context) error { return nil }), domain.SystemInfo{
		APIVersion: "v1", SchemaVersion: "0.3.0", BuildVersion: "test-build",
	})
	catalog := catalogapp.NewService(repository, catalogapp.ClockFunc(func() time.Time { return time.Now().UTC() }))
	return httpapi.NewHandler(system, logger, provider.Tracer("semlia-test"), httpapi.WithCatalog(catalog)), &logs
}

func mustCatalogID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

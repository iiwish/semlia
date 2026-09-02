package authorization_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	pgstore "github.com/iiwish/semlia/internal/adapters/postgres"
	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	"github.com/iiwish/semlia/internal/domain"
	"github.com/iiwish/semlia/internal/domain/authorization"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"go.opentelemetry.io/otel/sdk/trace"
)

const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

var databaseURL string

func TestMain(testingMain *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := tcpostgres.Run(
		ctx, "postgres:18-alpine",
		tcpostgres.WithDatabase("semlia_authorization_test"),
		tcpostgres.WithUsername("semlia"),
		tcpostgres.WithPassword("integration-test-only"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start authorization PostgreSQL container: failed")
		os.Exit(1)
	}
	databaseURL, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "resolve authorization PostgreSQL URL: failed")
		os.Exit(1)
	}
	migrator, err := pgstore.NewMigrator(databaseURL, filepath.Join(repositoryRoot(), "migrations"))
	if err != nil || migrator.Up() != nil {
		_ = testcontainers.TerminateContainer(container)
		fmt.Fprintln(os.Stderr, "migrate authorization PostgreSQL: failed")
		os.Exit(1)
	}
	_ = migrator.Close()
	code := testingMain.Run()
	if err := testcontainers.TerminateContainer(container); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

type fixture struct {
	pool    *pgstore.Pool
	store   *pgstore.Store
	handler http.Handler
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool, err := pgstore.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(context.Background(), "TRUNCATE workspaces CASCADE"); err != nil {
		t.Fatal(err)
	}
	store := pgstore.NewStore(pool)
	authorizer := authorizationapp.NewService(store, authorizationapp.ClockFunc(time.Now))
	catalog := catalogapp.NewService(
		store,
		catalogapp.ClockFunc(func() time.Time { return time.Now().UTC() }),
		catalogapp.WithAuthorizer(authorizer),
	)
	var logs bytes.Buffer
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	handler := httpapi.NewHandler(
		application.NewSystemService(
			application.ReadinessProbeFunc(func(context.Context) error { return nil }),
			domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.4.0", BuildVersion: "test-build"},
		),
		slog.New(slog.NewTextHandler(&logs, nil)),
		provider.Tracer("authorization-integration"),
		httpapi.WithCatalog(catalog),
	)
	return &fixture{pool: pool, store: store, handler: handler}
}

func (environment *fixture) createWorkspace(t *testing.T, slug string) identity.WorkspaceID {
	t.Helper()
	workspaceID := mustID(t, identity.NewWorkspaceID)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO workspaces (id, slug, display_name) VALUES ($1, $2, $2)`,
		workspaceID.UUID(), slug); err != nil {
		t.Fatal(err)
	}
	return workspaceID
}

func (environment *fixture) defaultPrincipal(t *testing.T, workspace identity.WorkspaceID) authorization.Principal {
	t.Helper()
	principal, err := environment.store.LoadDefaultPrincipal(context.Background(), workspace)
	if err != nil {
		t.Fatalf("seeded workspace principal: %v", err)
	}
	return principal
}

func (environment *fixture) createAgent(t *testing.T, workspace identity.WorkspaceID, owner identity.PrincipalID) identity.PrincipalID {
	t.Helper()
	agentID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: agentID, WorkspaceID: workspace, Kind: authorization.PrincipalAgent,
		DisplayName: "Governance Agent", OwnerPrincipalID: &owner,
		Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	return agentID
}

func (environment *fixture) bindRole(
	t *testing.T,
	principal identity.PrincipalID,
	roleID string,
	scope authorization.Resource,
) identity.BindingID {
	t.Helper()
	bindingID := mustID(t, identity.NewBindingID)
	if _, err := environment.store.CreateRoleBinding(context.Background(), authorization.RoleBinding{
		ID: bindingID, PrincipalID: principal, RoleID: roleID,
		ScopeType: scope.Type, ScopeID: scope.ID,
	}); err != nil {
		t.Fatal(err)
	}
	return bindingID
}

func (environment *fixture) post(t *testing.T, path, principalRef, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if principalRef != "" {
		request.Header.Set("X-Semlia-Principal", principalRef)
	}
	response := httptest.NewRecorder()
	environment.handler.ServeHTTP(response, request)
	return response
}

func (environment *fixture) createAssetBody() string {
	return `{"address":"commerce.net_revenue","assetType":"metric","schemaVersion":"1.0.0","content":{"name":"Net revenue","definition":"Revenue after refunds"},"createdBy":"founder"}`
}

func TestProtectedWriteWithoutMatchingGrantIsDeniedAndAudited(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-deny")
	unbound := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: unbound, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Unbound Member", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/workspaces/" + workspace.String() + "/catalog/assets"

	response := environment.post(t, path, unbound.String(), environment.createAssetBody())
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "NO_MATCHING_GRANT")

	var decision, reasonCode string
	var version int64
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT decision, reason_code, authorization_version FROM authorization_events
		WHERE workspace_id = $1 AND principal_id = $2 AND action = 'asset.propose'`,
		workspace.UUID(), unbound.UUID()).Scan(&decision, &reasonCode, &version); err != nil {
		t.Fatalf("denial decision event not recorded: %v", err)
	}
	if decision != "deny" || reasonCode != "NO_MATCHING_GRANT" || version != 1 {
		t.Fatalf("recorded denial = %s %s version %d", decision, reasonCode, version)
	}

	unknownResponse := environment.post(t, path, mustID(t, identity.NewPrincipalID).String(), environment.createAssetBody())
	if unknownResponse.Code != http.StatusForbidden {
		t.Fatalf("unknown principal status = %d", unknownResponse.Code)
	}
	assertJSONField(t, unknownResponse.Body.Bytes(), "code", "NO_MATCHING_GRANT")
}

func TestBindingInAnotherWorkspaceNeverGrants(t *testing.T) {
	environment := newFixture(t)
	homeWorkspace := environment.createWorkspace(t, "authz-home")
	otherWorkspace := environment.createWorkspace(t, "authz-other")
	admin := environment.defaultPrincipal(t, homeWorkspace)
	path := "/api/v1/workspaces/" + otherWorkspace.String() + "/catalog/assets"

	response := environment.post(t, path, admin.ID.String(), environment.createAssetBody())
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "NO_MATCHING_GRANT")
}

func TestAgentPrincipalActsWhereAllowedAndIsBlockedFromHumanDuties(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-agent")
	admin := environment.defaultPrincipal(t, workspace)
	agent := environment.createAgent(t, workspace, admin.ID)
	environment.bindRole(t, agent, "semantic_steward", authorization.Resource{
		Type: authorization.ScopeWorkspace, ID: workspace.UUID(),
	})
	path := "/api/v1/workspaces/" + workspace.String() + "/catalog/assets"

	response := environment.post(t, path, agent.String(), environment.createAssetBody())
	if response.Code != http.StatusCreated {
		t.Fatalf("agent asset create status = %d, body = %s", response.Code, response.Body.String())
	}

	var decision, reasonCode string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT decision, reason_code FROM authorization_events
		WHERE workspace_id = $1 AND principal_id = $2 AND action = 'asset.propose'`,
		workspace.UUID(), agent.UUID()).Scan(&decision, &reasonCode); err != nil {
		t.Fatalf("agent allow event not recorded: %v", err)
	}
	if decision != "allow" || reasonCode != "ROLE_GRANT" {
		t.Fatalf("agent decision = %s %s", decision, reasonCode)
	}

	authorizer := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(time.Now))
	environment.bindRole(t, agent, "publisher", authorization.Resource{
		Type: authorization.ScopeWorkspace, ID: workspace.UUID(),
	})
	humanDenied, err := authorizer.Evaluate(context.Background(), authorizationapp.EvaluationRequest{
		PrincipalRef: agent.String(), WorkspaceID: workspace,
		Action:   authorization.ActionReleasePublish,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()},
		TraceID:  traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if humanDenied.Allowed || humanDenied.ReasonCode != authorization.ReasonSeparationOfDuty {
		t.Fatalf("agent human-duty decision = %+v", humanDenied)
	}
}

func TestAuthorizedActorsKeepM1BehaviorAndRecordAllowEvents(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-m1")
	path := "/api/v1/workspaces/" + workspace.String() + "/catalog/assets"

	created := environment.post(t, path, "", environment.createAssetBody())
	if created.Code != http.StatusCreated {
		t.Fatalf("default actor asset create status = %d, body = %s", created.Code, created.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	assetID, _ := payload["id"].(string)
	if !strings.HasPrefix(assetID, "ast_") {
		t.Fatalf("asset id = %q", assetID)
	}

	revisionResponse := environment.post(
		t,
		path+"/"+assetID+"/revisions",
		"",
		`{"schemaVersion":"1.0.0","content":{"name":"Net revenue","definition":"Revenue after chargebacks"},"createdBy":"founder"}`,
	)
	if revisionResponse.Code != http.StatusCreated {
		t.Fatalf("default actor revision append status = %d, body = %s", revisionResponse.Code, revisionResponse.Body.String())
	}

	var allowCount int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM authorization_events
		WHERE workspace_id = $1 AND decision = 'allow' AND reason_code = 'ROLE_GRANT'`,
		workspace.UUID()).Scan(&allowCount); err != nil {
		t.Fatal(err)
	}
	if allowCount != 2 {
		t.Fatalf("allow events = %d, want one per protected command", allowCount)
	}
}

func assertJSONField(t *testing.T, body []byte, field, want string) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, body)
	}
	if payload[field] != want {
		t.Errorf("%s = %v, want %s", field, payload[field], want)
	}
}

func mustID[T any](t *testing.T, create func() (T, error)) T {
	t.Helper()
	value, err := create()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func repositoryRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("resolve repository root")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

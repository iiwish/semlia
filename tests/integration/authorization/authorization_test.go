package authorization_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
		httpapi.WithAuthorization(authorizer),
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

func (environment *fixture) request(t *testing.T, method, path, principalRef, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
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
	path := "/api/v1/workspaces/" + workspace.String() + "/catalog/assets"

	missingPrincipal := environment.post(t, path, "", environment.createAssetBody())
	if missingPrincipal.Code != http.StatusForbidden {
		t.Fatalf("missing principal status = %d, body = %s", missingPrincipal.Code, missingPrincipal.Body.String())
	}
	assertJSONField(t, missingPrincipal.Body.Bytes(), "code", "NO_MATCHING_GRANT")

	unbound := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: unbound, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Unbound Member", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
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

func TestLocalUATReviewerAliasNeverSelectsAnOrdinaryRoleHolder(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-uat-reviewer")
	decoyID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: decoyID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Ordinary Reviewer", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	environment.bindRole(t, decoyID, "reviewer", authorization.Resource{
		Type: authorization.ScopeWorkspace, ID: workspace.UUID(),
	})
	if _, err := environment.pool.Exec(context.Background(), `
		UPDATE principals SET created_at = '2000-01-01T00:00:00Z' WHERE id = $1`, decoyID.UUID()); err != nil {
		t.Fatal(err)
	}

	var seededReviewerID string
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT semlia_seed_uuidv7('independent_reviewer', id::text, created_at)::text
		FROM workspaces WHERE id = $1`, workspace.UUID()).Scan(&seededReviewerID); err != nil {
		t.Fatal(err)
	}
	authorizer := authorizationapp.NewService(
		environment.store, authorizationapp.ClockFunc(time.Now), authorizationapp.WithLocalUATIdentities(),
	)
	decision, err := authorizer.Evaluate(context.Background(), authorizationapp.EvaluationRequest{
		PrincipalRef: authorizationapp.LocalUATReviewerPrincipalRef,
		WorkspaceID:  workspace,
		Action:       authorization.ActionAssetRead,
		Resource: authorization.Resource{
			Type: authorization.ScopeWorkspace, ID: workspace.UUID(),
		},
		TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.PrincipalID.UUID() != seededReviewerID {
		t.Fatalf("local reviewer decision = %+v, want seeded principal %s", decision, seededReviewerID)
	}
	if decision.PrincipalID == decoyID {
		t.Fatalf("local reviewer alias resolved ordinary reviewer %s", decoyID)
	}
}

func TestAuthorizedActorsKeepM1BehaviorAndRecordAllowEvents(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-m1")
	principal := environment.defaultPrincipal(t, workspace)
	path := "/api/v1/workspaces/" + workspace.String() + "/catalog/assets"

	created := environment.post(t, path, principal.ID.String(), environment.createAssetBody())
	if created.Code != http.StatusCreated {
		t.Fatalf("workspace principal asset create status = %d, body = %s", created.Code, created.Body.String())
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
		principal.ID.String(),
		`{"schemaVersion":"1.0.0","content":{"name":"Net revenue","definition":"Revenue after chargebacks"},"createdBy":"founder"}`,
	)
	if revisionResponse.Code != http.StatusCreated {
		t.Fatalf("workspace principal revision append status = %d, body = %s", revisionResponse.Code, revisionResponse.Body.String())
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

func TestAuthorizationAdministrationPersistsVersionsBindingsAndRevocation(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-admin")
	admin := environment.defaultPrincipal(t, workspace)
	basePath := "/api/v1/workspaces/" + workspace.String() + "/authorization"

	roles := environment.request(t, http.MethodGet, basePath+"/roles", admin.ID.String(), "")
	if roles.Code != http.StatusOK {
		t.Fatalf("list roles status = %d, body = %s", roles.Code, roles.Body.String())
	}

	created := environment.request(t, http.MethodPost, basePath+"/roles", admin.ID.String(),
		`{"name":"Read only analyst","description":"Reads governed assets.","actions":["asset.read"]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create role status = %d, body = %s", created.Code, created.Body.String())
	}
	var createdPayload struct {
		AuthorizationVersion int64 `json:"authorizationVersion"`
		Role                 struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
		} `json:"role"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdPayload); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(createdPayload.Role.ID, "custom_") || createdPayload.Role.Version != 1 || createdPayload.AuthorizationVersion != 2 {
		t.Fatalf("created role = %+v", createdPayload)
	}

	updated := environment.request(t, http.MethodPatch, basePath+"/roles/"+createdPayload.Role.ID, admin.ID.String(),
		`{"name":"Governed analyst","description":"Reads governed assets and evidence.","actions":["asset.read","evidence.read"],"expectedVersion":1}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update role status = %d, body = %s", updated.Code, updated.Body.String())
	}
	var updatedPayload struct {
		AuthorizationVersion int64 `json:"authorizationVersion"`
		Role                 struct {
			Version int64 `json:"version"`
		} `json:"role"`
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &updatedPayload); err != nil {
		t.Fatal(err)
	}
	if updatedPayload.Role.Version != 2 || updatedPayload.AuthorizationVersion != 3 {
		t.Fatalf("updated role = %+v", updatedPayload)
	}

	immutable := environment.request(t, http.MethodPatch, basePath+"/roles/workspace_admin", admin.ID.String(),
		`{"name":"Workspace Admin","description":"Cannot mutate.","actions":["workspace.read"],"expectedVersion":1}`)
	if immutable.Code != http.StatusConflict {
		t.Fatalf("system role update status = %d, body = %s", immutable.Code, immutable.Body.String())
	}
	assertJSONField(t, immutable.Body.Bytes(), "code", "SYSTEM_ROLE_IMMUTABLE")

	targetID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: targetID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Governed Analyst", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	binding := environment.request(t, http.MethodPost, basePath+"/role-bindings", admin.ID.String(), fmt.Sprintf(
		`{"principalId":%q,"roleId":%q,"expectedRoleVersion":2,"scope":{"type":"workspace","id":%q}}`,
		targetID.String(), createdPayload.Role.ID, workspace.UUID()))
	if binding.Code != http.StatusCreated {
		t.Fatalf("create binding status = %d, body = %s", binding.Code, binding.Body.String())
	}
	var bindingPayload struct {
		AuthorizationVersion int64 `json:"authorizationVersion"`
		Binding              struct {
			ID      string `json:"id"`
			Version int64  `json:"version"`
			Status  string `json:"status"`
		} `json:"binding"`
	}
	if err := json.Unmarshal(binding.Body.Bytes(), &bindingPayload); err != nil {
		t.Fatal(err)
	}
	if bindingPayload.AuthorizationVersion != 4 || bindingPayload.Binding.Version != 1 || bindingPayload.Binding.Status != "active" {
		t.Fatalf("created binding = %+v", bindingPayload)
	}

	inspectBody := fmt.Sprintf(`{"principalId":%q,"action":"evidence.read","resource":{"type":"workspace","id":%q}}`, targetID.String(), workspace.UUID())
	allowed := environment.request(t, http.MethodPost, basePath+":inspect", admin.ID.String(), inspectBody)
	if allowed.Code != http.StatusOK {
		t.Fatalf("inspect allow status = %d, body = %s", allowed.Code, allowed.Body.String())
	}
	assertJSONField(t, allowed.Body.Bytes(), "reasonCode", "ROLE_GRANT")

	revoked := environment.request(t, http.MethodPost, basePath+"/role-bindings/"+bindingPayload.Binding.ID+":revoke", admin.ID.String(),
		`{"expectedVersion":1,"reason":"access no longer required"}`)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke binding status = %d, body = %s", revoked.Code, revoked.Body.String())
	}
	assertJSONField(t, revoked.Body.Bytes(), "authorizationVersion", float64(5))

	denied := environment.request(t, http.MethodPost, basePath+":inspect", admin.ID.String(), inspectBody)
	if denied.Code != http.StatusOK {
		t.Fatalf("inspect deny status = %d, body = %s", denied.Code, denied.Body.String())
	}
	assertJSONField(t, denied.Body.Bytes(), "reasonCode", "NO_MATCHING_GRANT")
}

func TestAuthorizationAdministrationDoesNotDiscloseCrossWorkspaceResources(t *testing.T) {
	environment := newFixture(t)
	first := environment.createWorkspace(t, "authz-first")
	second := environment.createWorkspace(t, "authz-second")
	firstAdmin := environment.defaultPrincipal(t, first)
	secondAdmin := environment.defaultPrincipal(t, second)
	firstBase := "/api/v1/workspaces/" + first.String() + "/authorization"
	secondBase := "/api/v1/workspaces/" + second.String() + "/authorization"

	created := environment.request(t, http.MethodPost, firstBase+"/roles", firstAdmin.ID.String(),
		`{"name":"First analyst","description":"First workspace only.","actions":["asset.read"]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create role status = %d, body = %s", created.Code, created.Body.String())
	}
	var payload struct {
		Role struct {
			ID string `json:"id"`
		} `json:"role"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}

	response := environment.request(t, http.MethodGet, secondBase+"/roles/"+payload.Role.ID, secondAdmin.ID.String(), "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace role status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "NOT_FOUND")

	forbidden := environment.request(t, http.MethodGet, secondBase+"/roles", firstAdmin.ID.String(), "")
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("cross-workspace caller status = %d, body = %s", forbidden.Code, forbidden.Body.String())
	}
	assertJSONField(t, forbidden.Body.Bytes(), "code", "NO_MATCHING_GRANT")

	targetID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: targetID, WorkspaceID: first, Kind: authorization.PrincipalHuman,
		DisplayName: "Scoped Analyst", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	otherAsset := mustID(t, identity.NewAssetID)
	if _, err := environment.pool.Exec(context.Background(), `
		INSERT INTO semantic_assets (id, workspace_id, namespace, key, asset_type, lifecycle_state)
		VALUES ($1, $2, 'other', 'private_asset', 'metric', 'draft')`, otherAsset.UUID(), second.UUID()); err != nil {
		t.Fatal(err)
	}
	service := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(time.Now))
	_, _, err := service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: first, PrincipalRef: firstAdmin.ID.String(), TraceID: traceID},
		PrincipalID:   targetID, RoleID: "asset_owner", ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeAsset, ScopeID: otherAsset.UUID(),
	})
	if !errors.Is(err, authorization.ErrNotFound) {
		t.Fatalf("cross-workspace scope error = %v", err)
	}
}

func TestBindingExpiryBumpsVersionAndDeniesTheNextEvaluation(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-expiry")
	admin := environment.defaultPrincipal(t, workspace)
	targetID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: targetID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Temporary Consumer", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	service := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(func() time.Time { return now }))
	created, version, err := service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID},
		PrincipalID:   targetID, RoleID: "consumer_developer", ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(), ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if version != 2 || created.StatusAt(now) != authorization.BindingActive {
		t.Fatalf("temporary binding = %+v version %d", created, version)
	}

	later := expiresAt.Add(time.Second)
	laterService := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(func() time.Time { return later }))
	decision, err := laterService.Evaluate(context.Background(), authorizationapp.EvaluationRequest{
		PrincipalRef: targetID.String(), WorkspaceID: workspace, Action: authorization.ActionSemanticExecute,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.AuthorizationVersion != 3 {
		t.Fatalf("post-expiry decision = %+v", decision)
	}
	var expired bool
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT expired_at IS NOT NULL FROM role_bindings WHERE id = $1`, created.ID.UUID()).Scan(&expired); err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Fatal("expired binding lifecycle was not persisted")
	}
	if _, _, err := laterService.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID},
		PrincipalID:   targetID, RoleID: "consumer_developer", ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
	}); err != nil {
		t.Fatalf("regrant after expiry: %v", err)
	}
}

func TestFinalWorkspaceAdministratorCannotBeRevoked(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-final-admin")
	admin := environment.defaultPrincipal(t, workspace)
	service := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(time.Now))
	bindings, err := environment.store.ListAuthorizationRoleBindings(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	var adminBinding authorization.RoleBinding
	for _, binding := range bindings {
		if binding.PrincipalID == admin.ID && binding.RoleID == "workspace_admin" {
			adminBinding = binding
			break
		}
	}
	if adminBinding.ID.IsZero() {
		t.Fatal("seeded workspace administrator binding is missing")
	}
	_, _, err = service.RevokeBinding(context.Background(), authorizationapp.RevokeRoleBindingRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID},
		BindingID:     adminBinding.ID, ExpectedVersion: adminBinding.Version, Reason: "must remain protected",
	})
	if !errors.Is(err, authorization.ErrFinalAdministrator) {
		t.Fatalf("final administrator revocation error = %v", err)
	}
	temporaryID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: temporaryID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Temporary Administrator", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	_, _, err = service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID},
		PrincipalID:   temporaryID, RoleID: "workspace_admin", ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(), ExpiresAt: &expiresAt,
	})
	if !errors.Is(err, authorization.ErrFinalAdministrator) {
		t.Fatalf("expiring administrator assignment error = %v", err)
	}
	var denials int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'authorization.policy_denied'`, workspace.UUID()).Scan(&denials); err != nil {
		t.Fatal(err)
	}
	if denials < 2 {
		t.Fatalf("final-admin policy denial audit facts = %d, want at least 2", denials)
	}
}

func TestAuthorizationCeilingHumanOnlyAndSeparationOfDutyFailClosed(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-policy")
	admin := environment.defaultPrincipal(t, workspace)
	service := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(time.Now))
	resource := authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}

	securityID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: securityID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Security Admin", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	environment.bindRole(t, securityID, "security_admin", resource)
	_, _, err := service.CreateCustomRole(context.Background(), authorizationapp.CreateCustomRoleRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: securityID.String(), TraceID: traceID},
		Name:          "Asset reader", Description: "Would exceed the grantor ceiling.", Actions: []authorization.Action{authorization.ActionAssetRead},
	})
	if !errors.Is(err, authorization.ErrAuthorizationCeiling) {
		t.Fatalf("authorization ceiling error = %v", err)
	}

	agentID := environment.createAgent(t, workspace, admin.ID)
	_, _, err = service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID},
		PrincipalID:   agentID, RoleID: "security_admin", ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
	})
	if !errors.Is(err, authorization.ErrSeparationOfDuties) {
		t.Fatalf("agent human-only assignment error = %v", err)
	}

	reviewerID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: reviewerID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Review Duty Tester", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	environment.bindRole(t, reviewerID, "reviewer", resource)
	_, _, err = service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID},
		PrincipalID:   reviewerID, RoleID: "publisher", ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
	})
	if !errors.Is(err, authorization.ErrSeparationOfDuties) {
		t.Fatalf("review/publish separation error = %v", err)
	}
	var denials int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'authorization.policy_denied'`, workspace.UUID()).Scan(&denials); err != nil {
		t.Fatal(err)
	}
	if denials < 3 {
		t.Fatalf("policy denial audit facts = %d, want at least 3", denials)
	}
}

func TestCustomRoleEditAdvancesActiveBindingsToTheNewImmutableVersion(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-pinned-version")
	admin := environment.defaultPrincipal(t, workspace)
	targetID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: targetID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Versioned Analyst", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	service := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(time.Now))
	access := authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID}
	role, _, err := service.CreateCustomRole(context.Background(), authorizationapp.CreateCustomRoleRequest{
		AccessRequest: access, Name: "Versioned reader", Description: "Tests immutable role grants.",
		Actions: []authorization.Action{authorization.ActionAssetRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, _, err := service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: access, PrincipalID: targetID, RoleID: role.ID, ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.UpdateCustomRole(context.Background(), authorizationapp.UpdateCustomRoleRequest{
		AccessRequest: access, RoleID: role.ID, ExpectedVersion: 1, Name: "Versioned reader",
		Description: "A new immutable version.", Actions: []authorization.Action{authorization.ActionEvidenceRead},
	}); err != nil {
		t.Fatal(err)
	}

	assetDecision, err := service.Evaluate(context.Background(), authorizationapp.EvaluationRequest{
		PrincipalRef: targetID.String(), WorkspaceID: workspace, Action: authorization.ActionAssetRead,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceDecision, err := service.Evaluate(context.Background(), authorizationapp.EvaluationRequest{
		PrincipalRef: targetID.String(), WorkspaceID: workspace, Action: authorization.ActionEvidenceRead,
		Resource: authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()}, TraceID: traceID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if assetDecision.Allowed || !evidenceDecision.Allowed || evidenceDecision.BindingID != binding.ID {
		t.Fatalf("advanced binding decisions: asset=%+v evidence=%+v", assetDecision, evidenceDecision)
	}
	bindings, err := environment.store.ListAuthorizationRoleBindings(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	var advanced authorization.RoleBinding
	for _, item := range bindings {
		if item.ID == binding.ID {
			advanced = item
		}
	}
	if advanced.RoleVersion != 2 || advanced.Version != 2 {
		t.Fatalf("active binding version = role %d lifecycle %d", advanced.RoleVersion, advanced.Version)
	}
	if _, _, err := service.RevokeBinding(context.Background(), authorizationapp.RevokeRoleBindingRequest{
		AccessRequest: access, BindingID: advanced.ID, ExpectedVersion: advanced.Version, Reason: "replace grant",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: access, PrincipalID: targetID, RoleID: role.ID, ExpectedRoleVersion: 2,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
	}); err != nil {
		t.Fatalf("regrant after revocation: %v", err)
	}
}

func TestCustomRoleEditCannotTurnAnExistingGrantIntoASeparationOfDutiesBypass(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-role-edit-sod")
	admin := environment.defaultPrincipal(t, workspace)
	targetID := mustID(t, identity.NewPrincipalID)
	if _, err := environment.store.CreatePrincipal(context.Background(), authorization.Principal{
		ID: targetID, WorkspaceID: workspace, Kind: authorization.PrincipalHuman,
		DisplayName: "Role Edit Reviewer", Status: authorization.PrincipalActive,
	}); err != nil {
		t.Fatal(err)
	}
	environment.bindRole(t, targetID, "reviewer", authorization.Resource{Type: authorization.ScopeWorkspace, ID: workspace.UUID()})
	service := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(time.Now))
	access := authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID}
	role, _, err := service.CreateCustomRole(context.Background(), authorizationapp.CreateCustomRoleRequest{
		AccessRequest: access, Name: "Harmless reader", Description: "Starts without a protected duty.",
		Actions: []authorization.Action{authorization.ActionAssetRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: access, PrincipalID: targetID, RoleID: role.ID, ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.UpdateCustomRole(context.Background(), authorizationapp.UpdateCustomRoleRequest{
		AccessRequest: access, RoleID: role.ID, ExpectedVersion: 1, Name: role.Name,
		Description: "Would silently add publisher duty.", Actions: []authorization.Action{authorization.ActionReleasePublish},
	}); !errors.Is(err, authorization.ErrSeparationOfDuties) {
		t.Fatalf("role-edit separation-of-duties error = %v", err)
	}
	current, err := environment.store.GetAuthorizationRole(context.Background(), workspace, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != 1 || len(current.Actions) != 1 || current.Actions[0] != authorization.ActionAssetRead {
		t.Fatalf("rejected role edit changed active role: %+v", current)
	}
	var denials int
	if err := environment.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE workspace_id = $1 AND event_type = 'authorization.policy_denied'
		  AND payload->>'reason' = 'AFFECTED_BINDING_POLICY'`, workspace.UUID()).Scan(&denials); err != nil {
		t.Fatal(err)
	}
	if denials != 1 {
		t.Fatalf("role-edit policy denial audit facts = %d", denials)
	}
	agentID := environment.createAgent(t, workspace, admin.ID)
	agentRole, _, err := service.CreateCustomRole(context.Background(), authorizationapp.CreateCustomRoleRequest{
		AccessRequest: access, Name: "Agent reader", Description: "Starts with an agent-safe action.",
		Actions: []authorization.Action{authorization.ActionAssetRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.CreateBinding(context.Background(), authorizationapp.CreateRoleBindingRequest{
		AccessRequest: access, PrincipalID: agentID, RoleID: agentRole.ID, ExpectedRoleVersion: 1,
		ScopeType: authorization.ScopeWorkspace, ScopeID: workspace.UUID(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.UpdateCustomRole(context.Background(), authorizationapp.UpdateCustomRoleRequest{
		AccessRequest: access, RoleID: agentRole.ID, ExpectedVersion: 1, Name: agentRole.Name,
		Description: "Would silently add a human-only duty.", Actions: []authorization.Action{authorization.ActionReleasePublish},
	}); !errors.Is(err, authorization.ErrSeparationOfDuties) {
		t.Fatalf("role-edit human-only error = %v", err)
	}
}

func TestRoleActionsRejectSystemRetargetAndCustomUnversionedWrites(t *testing.T) {
	environment := newFixture(t)
	workspace := environment.createWorkspace(t, "authz-role-action-trigger")
	admin := environment.defaultPrincipal(t, workspace)
	service := authorizationapp.NewService(environment.store, authorizationapp.ClockFunc(time.Now))
	role, _, err := service.CreateCustomRole(context.Background(), authorizationapp.CreateCustomRoleRequest{
		AccessRequest: authorizationapp.AccessRequest{WorkspaceID: workspace, PrincipalRef: admin.ID.String(), TraceID: traceID},
		Name:          "Trigger guard", Description: "Validates versioned action writes.", Actions: []authorization.Action{authorization.ActionAssetRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := environment.pool.Exec(context.Background(), `INSERT INTO role_actions (role_id, action) VALUES ($1, 'evidence.read')`, role.ID); err == nil {
		t.Fatal("custom role accepted an unversioned role_actions insert")
	}
	if _, err := environment.pool.Exec(context.Background(), `
		UPDATE role_actions SET role_id = $1
		WHERE role_id = 'workspace_admin' AND action = 'workspace.manage'`, role.ID); err == nil {
		t.Fatal("system role action could be retargeted to a custom role")
	}
}

func assertJSONField(t *testing.T, body []byte, field string, want any) {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, body)
	}
	if payload[field] != want {
		t.Errorf("%s = %v, want %v", field, payload[field], want)
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

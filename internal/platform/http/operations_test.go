package httpapi_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	operationsapp "github.com/iiwish/semlia/internal/application/operations"
	"github.com/iiwish/semlia/internal/domain"
	"github.com/iiwish/semlia/internal/domain/authorization"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

type operationsRepository struct {
	audit     []operationsapp.StoredAuditEvent
	run       operationsdomain.RuntimeRun
	events    []operationsdomain.RuntimeRunEvent
	settings  operationsdomain.RuntimeSettings
	download  operationsapp.AuditExportContent
	exportErr error
}

func (repo *operationsRepository) GetAuditExportContent(context.Context, identity.WorkspaceID, identity.EventID, time.Time) (operationsapp.AuditExportContent, error) {
	return repo.download, nil
}

func (repo *operationsRepository) ListAuditEvents(context.Context, operationsapp.AuditQuery) ([]operationsapp.StoredAuditEvent, error) {
	return append([]operationsapp.StoredAuditEvent(nil), repo.audit...), nil
}

func (*operationsRepository) FindAuditExportReplay(context.Context, operationsapp.AuditExportReplayRequest) (operationsapp.AuditExport, bool, error) {
	return operationsapp.AuditExport{}, false, nil
}

func (repo *operationsRepository) CreateAuditExport(context.Context, operationsapp.AuditExportRecord) (operationsapp.AuditExport, error) {
	return operationsapp.AuditExport{}, repo.exportErr
}

func (repo *operationsRepository) ListRuntimeRuns(context.Context, operationsapp.RuntimeQuery) ([]operationsdomain.RuntimeRun, error) {
	return []operationsdomain.RuntimeRun{repo.run}, nil
}

func (repo *operationsRepository) GetRuntimeRun(context.Context, identity.WorkspaceID, identity.RunID) (operationsdomain.RuntimeRun, error) {
	return repo.run, nil
}

func (repo *operationsRepository) ListRuntimeRunEvents(context.Context, identity.WorkspaceID, identity.RunID, int) ([]operationsdomain.RuntimeRunEvent, error) {
	return append([]operationsdomain.RuntimeRunEvent(nil), repo.events...), nil
}

func (repo *operationsRepository) GetRuntimeSettings(_ context.Context, _ identity.WorkspaceID, _ time.Time) (operationsdomain.RuntimeSettings, error) {
	return repo.settings, nil
}

func (repo *operationsRepository) UpdateRuntimeSettings(_ context.Context, value operationsdomain.RuntimeSettings, _ int64) (operationsdomain.RuntimeSettings, error) {
	return value, nil
}

func TestOperationsAuditHTTPReturnsOnlyExplicitRedactedFields(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	eventID, _ := identity.NewEventID()
	now := time.Now().UTC()
	repo := &operationsRepository{audit: []operationsapp.StoredAuditEvent{{ID: eventID, WorkspaceID: workspace,
		EventType: "source.tested", ActorID: "prn_actor", ObjectType: "source", ObjectID: "src_safe",
		TraceID: "4bf92f3577b34da6a3ce929d0e0e4736", CreatedAt: now,
		Payload: []byte(`{"objectType":"source","objectId":"src_safe","outcome":"failed","dsn":"postgres://user:secret@db/app","prompt":"private"}`)}}}
	evaluator := &stubEvaluator{decision: authorization.Decision{Allowed: true, Action: authorization.ActionAuditRead,
		ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 1}}
	handler := newOperationsHandler(t, repo, evaluator)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/operations/audit-events", nil)
	request.Header.Set("X-Semlia-Principal", "prn_actor")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if evaluator.captured.Action != authorization.ActionAuditRead {
		t.Fatalf("action=%q", evaluator.captured.Action)
	}
	for _, secret := range []string{"postgres://", "secret", "private", "payload", "dsn"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatalf("audit response leaked %q: %s", secret, response.Body.String())
		}
	}
	for _, field := range []string{"eventType", "actorId", "objectType", "objectId", "outcome", "traceId", "summary"} {
		if !strings.Contains(response.Body.String(), field) {
			t.Fatalf("audit response lacks %q: %s", field, response.Body.String())
		}
	}
}

func TestOperationsAuditHTTPTamperedCursorReturnsBadRequestBeforeRepository(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	now := time.Now().UTC()
	rows := make([]operationsapp.StoredAuditEvent, 2)
	for index := range rows {
		eventID, _ := identity.NewEventID()
		rows[index] = operationsapp.StoredAuditEvent{ID: eventID, WorkspaceID: workspace,
			EventType: "audit.cursor", TraceID: strings.Repeat("a", 32), CreatedAt: now.Add(-time.Duration(index) * time.Second), Payload: []byte(`{}`)}
	}
	repo := &operationsRepository{audit: rows}
	evaluator := &stubEvaluator{decision: authorization.Decision{Allowed: true, Action: authorization.ActionAuditRead,
		ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 1}}
	handler := newOperationsHandler(t, repo, evaluator)
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/"+workspace.String()+"/operations/audit-events?limit=1", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	var page struct {
		Page struct {
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	if json.Unmarshal(first.Body.Bytes(), &page) != nil || page.Page.NextCursor == "" {
		t.Fatalf("first page = %s", first.Body.String())
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(page.Page.NextCursor)
	var cursor map[string]any
	if json.Unmarshal(decoded, &cursor) != nil {
		t.Fatal("decode cursor")
	}
	cursor["id"] = "00000000-0000-0000-0000-000000000000"
	tampered, _ := json.Marshal(cursor)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet,
		"/api/v1/workspaces/"+workspace.String()+"/operations/audit-events?limit=1&cursor="+
			url.QueryEscape(base64.RawURLEncoding.EncodeToString(tampered)), nil))
	if second.Code != http.StatusBadRequest || !strings.Contains(second.Body.String(), "INVALID_ARGUMENT") {
		t.Fatalf("tampered cursor status=%d body=%s", second.Code, second.Body.String())
	}
}

func TestOperationsAuditExportContentReturnsExactPersistedBytesAndDigest(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	exportID, _ := identity.NewEventID()
	content := []byte(`[{"id":"evt_01m1pnr8k9f0r9h19rzat3hcvc","eventType":"source.tested","actorId":"","objectType":"source","objectId":"src_safe","channel":"api","outcome":"succeeded","reasonCode":"","traceId":"4bf92f3577b34da6a3ce929d0e0e4736","summary":"source.tested","createdAt":"2026-09-05T00:00:00Z"}]`)
	digest := sha256.Sum256(content)
	repo := &operationsRepository{download: operationsapp.AuditExportContent{ID: exportID, Format: "json",
		Content: content, ContentDigest: hex.EncodeToString(digest[:]), ExpiresAt: time.Now().Add(time.Hour)}}
	evaluator := &stubEvaluator{decision: authorization.Decision{Allowed: true, Action: authorization.ActionAuditRead,
		ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 1}}
	handler := newOperationsHandler(t, repo, evaluator)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/operations/audit-exports/"+exportID.String()+"/content", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != string(content) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("X-Content-SHA256") != hex.EncodeToString(digest[:]) || response.Header().Get("Content-Disposition") == "" {
		t.Fatalf("download headers = %v", response.Header())
	}
	if evaluator.captured.Action != authorization.ActionAuditRead {
		t.Fatalf("action=%q", evaluator.captured.Action)
	}
}

func TestOperationsAuditExportIdempotencyConflictIsStableHTTP409(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	principal, _ := identity.NewPrincipalID()
	repo := &operationsRepository{exportErr: operationsdomain.ErrConflict}
	evaluator := &stubEvaluator{decision: authorization.Decision{Allowed: true, Action: authorization.ActionAuditRead,
		PrincipalID: principal, ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 1}}
	handler := newOperationsHandler(t, repo, evaluator)
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/operations/audit-exports",
		strings.NewReader(`{"idempotencyKey":"reused-command","filter":{}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("idempotency conflict status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestOperationsRuntimeActionsFailClosedWhenOwningDomainIsUnsupported(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	runID, _ := identity.NewRunID()
	now := time.Now().UTC()
	repo := &operationsRepository{run: operationsdomain.RuntimeRun{ID: runID, WorkspaceID: workspace,
		Kind: operationsdomain.RunKindDiscovery, SourceType: "discovery_run", SourceID: runID.String(),
		SourceVersionDigest: strings.Repeat("a", 64), TraceID: "4bf92f3577b34da6a3ce929d0e0e4736",
		IdempotencyKey: "run", State: operationsdomain.RunSucceeded, MaxAttempts: 3, FinishedAt: &now,
		Version: 1, CreatedAt: now, UpdatedAt: now}}
	evaluator := &stubEvaluator{decision: authorization.Decision{Allowed: true, Action: authorization.ActionRuntimeManage,
		ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 1}}
	handler := newOperationsHandler(t, repo, evaluator)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspace.String()+"/operations/runs/"+runID.String()+":retry", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "RUN_ACTION_UNSUPPORTED") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if evaluator.captured.Action != authorization.ActionRuntimeManage {
		t.Fatalf("action=%q", evaluator.captured.Action)
	}
}

func TestRuntimePolicyPatchRejectsDeploymentOwnedFields(t *testing.T) {
	workspace, _ := identity.NewWorkspaceID()
	repo := &operationsRepository{}
	evaluator := &stubEvaluator{decision: authorization.Decision{Allowed: true, Action: authorization.ActionRuntimeManage,
		ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 1}}
	handler := newOperationsHandler(t, repo, evaluator)
	body := `{"expectedVersion":1,"retryCeiling":3,"statementTimeoutMs":30000,"webhookTimeoutMs":10000,"queryRowLimit":1000,"queryByteLimit":1048576,"runMetadataRetentionDays":30,"otelEndpoint":"https://attacker.example"}`
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/workspaces/"+workspace.String()+"/runtime-policy", strings.NewReader(body))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func newOperationsHandler(t *testing.T, repo *operationsRepository, evaluator *stubEvaluator) http.Handler {
	t.Helper()
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	system := application.NewSystemService(application.ReadinessProbeFunc(func(context.Context) error { return nil }),
		domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.9.0", BuildVersion: "test"})
	service := operationsapp.NewService(repo, evaluator, operationsapp.ClockFunc(func() time.Time { return time.Now().UTC() }),
		operationsdomain.DeploymentStatus{AuditRetention: "deployment_managed"})
	return httpapi.NewHandler(system, nil, provider.Tracer("operations-test"), httpapi.WithOperations(service))
}

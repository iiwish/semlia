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
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	"github.com/iiwish/semlia/internal/domain"
	"github.com/iiwish/semlia/internal/domain/authorization"
	distribution "github.com/iiwish/semlia/internal/domain/distribution"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/sdk/trace"
)

func TestDistributionResolveReturnsPersistedNoReleaseRefusal(t *testing.T) {
	repository := &distributionRepository{}
	handler := newDistributionHandler(t, repository)
	workspace := mustDistributionID(t, identity.NewWorkspaceID)
	body := `{"query":{"schemaVersion":"1.0.0","intent":"aggregate","measures":[{"address":"commerce.revenue"}],"context":{"mode":"current"}},"channel":"api","idempotencyKey":"request-1"}`
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/workspaces/"+workspace.String()+"/semantic-queries:resolve", strings.NewReader(body))
	request.Header.Set("X-Semlia-Principal", "local:test")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.record.Refusal == nil || repository.record.Refusal.Code != distribution.RefusalNoRelease {
		t.Fatalf("persisted resolution = %+v", repository.record)
	}
	assertNestedJSONField(t, response.Body.Bytes(), "refusal", "code", string(distribution.RefusalNoRelease))
	assertJSONField(t, response.Body.Bytes(), "outcome", "refused")
}

func TestDistributionResolveRejectsUnknownAndUnsafeInput(t *testing.T) {
	handler := newDistributionHandler(t, &distributionRepository{})
	workspace := mustDistributionID(t, identity.NewWorkspaceID)
	path := "/api/v1/workspaces/" + workspace.String() + "/semantic-queries:resolve"
	tests := []string{
		`{"query":{"schemaVersion":"1.0.0","intent":"aggregate","measures":[{"address":"commerce.revenue"}],"context":{"mode":"current"},"sql":"select * from secret"},"channel":"api","idempotencyKey":"unsafe"}`,
		`{"query":{"schemaVersion":"1.0.0","intent":"aggregate","measures":[{"address":"commerce.revenue","search":"revenue"}],"context":{"mode":"current"}},"channel":"api","idempotencyKey":"ambiguous-selector"}`,
	}
	for _, body := range tests {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("X-Semlia-Principal", "local:test")
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	}
}

func TestDistributionRoutesRequireServiceAndAdvertiseMethods(t *testing.T) {
	workspace := mustDistributionID(t, identity.NewWorkspaceID)
	withoutService, _ := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error { return nil }))
	response := request(t, withoutService, http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/consumers", "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing service status = %d, body = %s", response.Code, response.Body.String())
	}

	withService := newDistributionHandler(t, &distributionRepository{})
	response = request(t, withService, http.MethodDelete, "/api/v1/workspaces/"+workspace.String()+"/consumers", "")
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("method response = %d allow=%q", response.Code, response.Header().Get("Allow"))
	}
	response = request(t, withService, http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/ask", "")
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "POST" {
		t.Fatalf("Ask method response = %d allow=%q", response.Code, response.Header().Get("Allow"))
	}
	response = request(t, withService, http.MethodPost, "/api/v1/workspaces/"+workspace.String()+"/ask",
		`{"question":"net revenue","idempotencyKey":"ask-1"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing Ask service status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestDistributionRoutesRequireAuthenticationAndCapability(t *testing.T) {
	workspace := mustDistributionID(t, identity.NewWorkspaceID)
	unauthenticated := newDistributionSecurityHandler(t, &identityServiceStub{}, &distributionAuthorizerStub{})
	response := request(t, unauthenticated, http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/consumers", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, body = %s", response.Code, response.Body.String())
	}

	identityStub := authenticatedIdentityStub(t)
	workspace = identityStub.authenticated.Session.Memberships[0].WorkspaceID
	denied := newDistributionSecurityHandler(t, identityStub, &distributionAuthorizerStub{deny: true})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+workspace.String()+"/consumers", nil)
	request.AddCookie(&http.Cookie{Name: "semlia_session_dev", Value: "opaque-session-token"})
	denied.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("capability denial status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertJSONField(t, recorder.Body.Bytes(), "code", string(authorization.ReasonNoMatchingGrant))
}

type distributionRepository struct {
	record distributionapp.ResolutionRecord
}

func (repository *distributionRepository) CreateConsumer(_ context.Context, value distribution.Consumer) (distribution.Consumer, error) {
	return value, nil
}
func (repository *distributionRepository) GetConsumer(context.Context, identity.WorkspaceID, identity.ConsumerID) (distribution.Consumer, error) {
	return distribution.Consumer{}, distribution.ErrNotFound
}
func (repository *distributionRepository) ListConsumers(context.Context, identity.WorkspaceID) ([]distribution.Consumer, error) {
	return nil, nil
}
func (repository *distributionRepository) UpdateConsumer(_ context.Context, value distribution.Consumer) (distribution.Consumer, error) {
	return value, nil
}
func (repository *distributionRepository) CreateConsumerBinding(_ context.Context, value distribution.ConsumerBinding) (distribution.ConsumerBinding, error) {
	return value, nil
}
func (repository *distributionRepository) GetConsumerBinding(context.Context, identity.WorkspaceID, identity.ConsumerBindingID) (distribution.ConsumerBinding, error) {
	return distribution.ConsumerBinding{}, distribution.ErrNotFound
}
func (repository *distributionRepository) ListConsumerBindings(context.Context, identity.WorkspaceID) ([]distribution.ConsumerBinding, error) {
	return nil, nil
}
func (repository *distributionRepository) UpdateConsumerBinding(_ context.Context, value distribution.ConsumerBinding, _ int) (distribution.ConsumerBinding, error) {
	return value, nil
}
func (repository *distributionRepository) CurrentReleaseSnapshot(context.Context, identity.WorkspaceID) (distribution.ReleaseSnapshot, error) {
	return distribution.ReleaseSnapshot{}, distribution.ErrNotFound
}
func (repository *distributionRepository) ReleaseSnapshot(context.Context, identity.WorkspaceID, identity.ReleaseID) (distribution.ReleaseSnapshot, error) {
	return distribution.ReleaseSnapshot{}, distribution.ErrNotFound
}
func (repository *distributionRepository) RecordResolution(_ context.Context, value distributionapp.ResolutionRecord) error {
	repository.record = value
	return nil
}
func (repository *distributionRepository) GetSemanticQuery(context.Context, identity.WorkspaceID, identity.SemanticQueryID) (distributionapp.ResolutionResult, error) {
	return distributionapp.ResolutionResult{}, distribution.ErrNotFound
}
func (repository *distributionRepository) GetResolutionByIdempotency(context.Context, identity.WorkspaceID, string, string) (distributionapp.ResolutionResult, error) {
	return distributionapp.ResolutionResult{}, distribution.ErrNotFound
}
func (repository *distributionRepository) GetResolvedSemanticPlan(context.Context, identity.WorkspaceID, identity.ResolvedSemanticPlanID) (distribution.ResolvedSemanticPlan, error) {
	return distribution.ResolvedSemanticPlan{}, distribution.ErrNotFound
}

func newDistributionHandler(t *testing.T, repository distributionapp.Repository) http.Handler {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	system := application.NewSystemService(application.ReadinessProbeFunc(func(context.Context) error { return nil }), domain.SystemInfo{
		APIVersion: "v1", SchemaVersion: "0.7.0", BuildVersion: "test-build",
	})
	clock := distributionapp.ClockFunc(func() time.Time { return time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC) })
	return httpapi.NewHandler(system, logger, provider.Tracer("distribution-test"),
		httpapi.WithDistribution(distributionapp.NewService(repository, nil, clock)))
}

type distributionAuthorizerStub struct{ deny bool }

func (stub *distributionAuthorizerStub) Evaluate(_ context.Context, request authorizationapp.EvaluationRequest) (authorization.Decision, error) {
	return authorization.Decision{Allowed: !stub.deny, Action: request.Action,
		ReasonCode: authorization.ReasonNoMatchingGrant, RoleID: "workspace_admin"}, nil
}

func newDistributionSecurityHandler(t *testing.T, identityService httpapi.IdentityService, authorizer authorizationapp.Evaluator) http.Handler {
	t.Helper()
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	system := application.NewSystemService(application.ReadinessProbeFunc(func(context.Context) error { return nil }),
		domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.7.0", BuildVersion: "test-build"})
	clock := distributionapp.ClockFunc(func() time.Time { return time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC) })
	return httpapi.NewHandler(system, nil, provider.Tracer("distribution-security-test"),
		httpapi.WithIdentity(identityService, httpapi.IdentityHTTPConfig{AllowedOrigins: []string{"https://app.example.com"}}),
		httpapi.WithDistribution(distributionapp.NewService(&distributionRepository{}, authorizer, clock)))
}

func assertNestedJSONField(t *testing.T, body []byte, object, field, want string) {
	t.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	var nested map[string]any
	if err := json.Unmarshal(payload[object], &nested); err != nil {
		t.Fatalf("decode %s: %v", object, err)
	}
	if nested[field] != want {
		t.Fatalf("%s.%s = %v, want %s", object, field, nested[field], want)
	}
}

func mustDistributionID[T any](t *testing.T, factory func() (T, error)) T {
	t.Helper()
	value, err := factory()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

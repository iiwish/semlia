package httpapi_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	"github.com/iiwish/semlia/internal/domain"
	"github.com/iiwish/semlia/internal/domain/authorization"
	httpapi "github.com/iiwish/semlia/internal/platform/http"
	"go.opentelemetry.io/otel/sdk/trace"
)

type stubEvaluator struct {
	decision authorization.Decision
	err      error
	captured authorizationapp.EvaluationRequest
}

func (evaluator *stubEvaluator) Evaluate(
	_ context.Context, request authorizationapp.EvaluationRequest,
) (authorization.Decision, error) {
	evaluator.captured = request
	if evaluator.err != nil {
		return authorization.Decision{}, evaluator.err
	}
	return evaluator.decision, nil
}

func denialDecision() authorization.Decision {
	return authorization.Decision{
		Allowed: false, Action: authorization.ActionAssetPropose,
		ReasonCode: authorization.ReasonNoMatchingGrant, AuthorizationVersion: 4,
	}
}

func TestProtectedWriteDeniedByServerSideCapability(t *testing.T) {
	repository := newCatalogRepository(t)
	evaluator := &stubEvaluator{decision: denialDecision()}
	handler, _ := newAuthorizedCatalogHandler(t, repository, evaluator)
	body := `{"address":"commerce.net_revenue","assetType":"metric","schemaVersion":"1.0.0","content":{"name":"Net revenue"},"createdBy":"founder"}`
	path := "/api/v1/workspaces/" + repository.workspace.String() + "/catalog/assets"

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("X-Semlia-Principal", "prn_01k3exampleexampleexampleexampl")
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertJSONField(t, response.Body.Bytes(), "code", "NO_MATCHING_GRANT")
	if evaluator.captured.PrincipalRef != "prn_01k3exampleexampleexampleexampl" {
		t.Fatalf("principal header not plumbed to evaluator: %+v", evaluator.captured)
	}
	if evaluator.captured.Action != authorization.ActionAssetPropose {
		t.Fatalf("asset create must evaluate asset.propose, got %q", evaluator.captured.Action)
	}
}

func TestProtectedWritePlumbsPrincipalHeaderAndAllowsOnGrant(t *testing.T) {
	repository := newCatalogRepository(t)
	evaluator := &stubEvaluator{decision: authorization.Decision{
		Allowed: true, Action: authorization.ActionAssetPropose,
		ReasonCode: authorization.ReasonRoleGrant, AuthorizationVersion: 2,
	}}
	handler, _ := newAuthorizedCatalogHandler(t, repository, evaluator)
	body := `{"address":"commerce.net_revenue","assetType":"metric","schemaVersion":"1.0.0","content":{"name":"Net revenue"},"createdBy":"founder"}`
	path := "/api/v1/workspaces/" + repository.workspace.String() + "/catalog/assets"

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("X-Semlia-Principal", repository.workspace.String())
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if evaluator.captured.Resource.Type != authorization.ScopeWorkspace || evaluator.captured.Resource.ID != repository.workspace.UUID() {
		t.Fatalf("evaluation resource = %+v", evaluator.captured.Resource)
	}
}

func TestRevisionAppendEvaluatesAssetEdit(t *testing.T) {
	repository := newCatalogRepository(t)
	evaluator := &stubEvaluator{decision: denialDecision()}
	handler, _ := newAuthorizedCatalogHandler(t, repository, evaluator)
	body := `{"schemaVersion":"1.0.0","content":{"name":"Net revenue"},"createdBy":"founder"}`
	path := "/api/v1/workspaces/" + repository.workspace.String() + "/catalog/assets/" + repository.asset.String() + "/revisions"

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if evaluator.captured.Action != authorization.ActionAssetEdit {
		t.Fatalf("revision append must evaluate asset.edit, got %q", evaluator.captured.Action)
	}
}

func newAuthorizedCatalogHandler(
	t *testing.T,
	repository *catalogRepository,
	evaluator *stubEvaluator,
) (http.Handler, *stubEvaluator) {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	system := application.NewSystemService(
		application.ReadinessProbeFunc(func(context.Context) error { return nil }),
		domain.SystemInfo{APIVersion: "v1", SchemaVersion: "0.4.0", BuildVersion: "test-build"},
	)
	catalog := catalogapp.NewService(
		repository,
		catalogapp.ClockFunc(func() time.Time { return time.Now().UTC() }),
		catalogapp.WithAuthorizer(evaluator),
	)
	return httpapi.NewHandler(system, logger, provider.Tracer("semlia-test"), httpapi.WithCatalog(catalog)), evaluator
}

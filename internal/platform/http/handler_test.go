package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/semlia/semlia/internal/application"
	"github.com/semlia/semlia/internal/domain"
	httpapi "github.com/semlia/semlia/internal/platform/http"
	"go.opentelemetry.io/otel/sdk/trace"
)

const incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"

func TestLivenessDoesNotCallDependencyProbe(t *testing.T) {
	var calls atomic.Int32
	handler, _ := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error {
		calls.Add(1)
		return errors.New("database down")
	}))

	response := request(t, handler, http.MethodGet, "/health/live", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if calls.Load() != 0 {
		t.Fatalf("liveness called dependency probe %d times", calls.Load())
	}
	assertTraceCorrelation(t, response)
	assertJSONField(t, response.Body.Bytes(), "status", "live")
}

func TestReadinessReportsReadyOnlyAfterSuccessfulProbe(t *testing.T) {
	handler, _ := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error { return nil }))
	response := request(t, handler, http.MethodGet, "/health/ready", "")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertTraceCorrelation(t, response)
	assertJSONField(t, response.Body.Bytes(), "status", "ready")
}

func TestReadinessUsesStableSafeErrorEnvelope(t *testing.T) {
	handler, _ := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error {
		return errors.New("postgres://user:secret@database.internal/semlia refused connection")
	}))
	response := request(t, handler, http.MethodGet, "/health/ready", "")

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") != "5" {
		t.Errorf("Retry-After = %q", response.Header().Get("Retry-After"))
	}
	assertTraceCorrelation(t, response)
	assertJSONField(t, response.Body.Bytes(), "code", "DEPENDENCY_UNAVAILABLE")
	for _, leaked := range []string{"user:secret", "database.internal", "refused connection"} {
		if strings.Contains(response.Body.String(), leaked) {
			t.Errorf("response leaked %q: %s", leaked, response.Body.String())
		}
	}
}

func TestTraceParentIsPropagatedIntoHeaderAndBody(t *testing.T) {
	handler, _ := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error { return nil }))
	response := request(t, handler, http.MethodGet, "/api/v1/system/info", "00-"+incomingTraceID+"-00f067aa0ba902b7-01")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("X-Trace-ID"); got != incomingTraceID {
		t.Errorf("trace header = %q", got)
	}
	assertJSONField(t, response.Body.Bytes(), "traceId", incomingTraceID)
}

func TestSystemInfoContainsOnlyPublicBuildFields(t *testing.T) {
	handler, _ := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error { return nil }))
	response := request(t, handler, http.MethodGet, "/api/v1/system/info", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 5 {
		t.Fatalf("system info fields = %v", payload)
	}
	for key, want := range map[string]string{
		"service":       "semlia",
		"apiVersion":    "v1",
		"schemaVersion": "0.1.0",
		"buildVersion":  "test-build",
	} {
		if payload[key] != want {
			t.Errorf("%s = %v, want %s", key, payload[key], want)
		}
	}
}

func TestRoutingErrorsUseThePublicErrorEnvelope(t *testing.T) {
	handler, _ := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error { return nil }))
	tests := []struct {
		method string
		path   string
		status int
		code   string
	}{
		{http.MethodPost, "/health/live", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{http.MethodGet, "/missing", http.StatusNotFound, "NOT_FOUND"},
	}

	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			response := request(t, handler, test.method, test.path, "")
			if response.Code != test.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			assertTraceCorrelation(t, response)
			assertJSONField(t, response.Body.Bytes(), "code", test.code)
		})
	}
}

func TestUnexpectedFailuresUseASafeInternalError(t *testing.T) {
	handler, logs := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error {
		panic("database-secret")
	}))
	response := request(t, handler, http.MethodGet, "/health/ready", "")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	assertTraceCorrelation(t, response)
	assertJSONField(t, response.Body.Bytes(), "code", "INTERNAL_ERROR")
	if strings.Contains(response.Body.String(), "database-secret") || strings.Contains(logs.String(), "database-secret") {
		t.Fatal("panic detail leaked into response or log")
	}
}

func TestRequestLogsContainSafeOperationalFieldsOnly(t *testing.T) {
	handler, logs := newHandler(t, application.ReadinessProbeFunc(func(context.Context) error {
		return errors.New("postgres://user:database-secret@database.internal/semlia")
	}))
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready?token=query-secret", nil)
	req.Header.Set("Authorization", "Bearer authorization-secret")
	handler.ServeHTTP(recorder, req)

	output := logs.String()
	for _, required := range []string{"request completed", "DEPENDENCY_UNAVAILABLE", "/health/ready", "trace_id"} {
		if !strings.Contains(output, required) {
			t.Errorf("log missing %q: %s", required, output)
		}
	}
	for _, leaked := range []string{"query-secret", "authorization-secret", "database-secret", "database.internal"} {
		if strings.Contains(output, leaked) {
			t.Errorf("log leaked %q: %s", leaked, output)
		}
	}
}

func newHandler(t *testing.T, probe application.ReadinessProbe) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	provider := trace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	service := application.NewSystemService(probe, domain.SystemInfo{
		APIVersion:    "v1",
		SchemaVersion: "0.1.0",
		BuildVersion:  "test-build",
	})
	return httpapi.NewHandler(service, logger, provider.Tracer("semlia-test")), &logs
}

func request(t *testing.T, handler http.Handler, method, path, traceparent string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
	handler.ServeHTTP(recorder, req)
	return recorder
}

func assertTraceCorrelation(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	traceID := response.Header().Get("X-Trace-ID")
	if len(traceID) != 32 {
		t.Fatalf("trace header = %q", traceID)
	}
	assertJSONField(t, response.Body.Bytes(), "traceId", traceID)
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

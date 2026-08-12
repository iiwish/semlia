package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	contract "github.com/iiwish/semlia/api/gen/go"
	"github.com/iiwish/semlia/internal/application"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	headerTraceID = "X-Trace-ID"
	retryAfter    = "5"
)

type Handler struct {
	service *application.SystemService
	logger  *slog.Logger
	tracer  trace.Tracer
}

func NewHandler(service *application.SystemService, logger *slog.Logger, tracer trace.Tracer) http.Handler {
	if service == nil {
		panic("system service is required")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if tracer == nil {
		panic("tracer is required")
	}
	return &Handler{service: service, logger: logger, tracer: tracer}
}

func (handler *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	started := time.Now()
	route := routeLabel(request.URL.Path)
	ctx := propagation.TraceContext{}.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
	ctx, span := handler.tracer.Start(ctx, request.Method+" "+route, trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

	traceID := span.SpanContext().TraceID().String()
	response.Header().Set(headerTraceID, traceID)
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	recorder := &statusRecorder{ResponseWriter: response}
	errorCode := ""

	defer func() {
		if recovered := recover(); recovered != nil {
			errorCode = "INTERNAL_ERROR"
			if !recorder.wroteHeader {
				writeError(recorder, http.StatusInternalServerError, errorCode, "the request could not be completed", traceID, false)
			}
			handler.logger.ErrorContext(ctx, "request panic recovered", "trace_id", traceID, "route", route, "error_code", errorCode)
		}

		attributes := []any{
			"method", request.Method,
			"route", route,
			"status", recorder.statusCode(),
			"duration_ms", time.Since(started).Milliseconds(),
			"trace_id", traceID,
		}
		if errorCode != "" {
			attributes = append(attributes, "error_code", errorCode)
		}
		handler.logger.InfoContext(ctx, "request completed", attributes...)
	}()

	errorCode = handler.route(recorder, request.WithContext(ctx), traceID)
}

func (handler *Handler) route(response http.ResponseWriter, request *http.Request, traceID string) string {
	if !knownRoute(request.URL.Path) {
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "the request method is not allowed", traceID, false)
		return "METHOD_NOT_ALLOWED"
	}

	switch request.URL.Path {
	case "/health/live":
		writeJSON(response, http.StatusOK, contract.HealthResponse{Status: contract.Live, TraceId: traceID})
		return ""
	case "/health/ready":
		if err := handler.service.CheckReadiness(request.Context()); err != nil {
			response.Header().Set("Retry-After", retryAfter)
			writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "a required dependency is unavailable", traceID, true)
			return "DEPENDENCY_UNAVAILABLE"
		}
		writeJSON(response, http.StatusOK, contract.HealthResponse{Status: contract.Ready, TraceId: traceID})
		return ""
	case "/api/v1/system/info":
		info := handler.service.Info()
		writeJSON(response, http.StatusOK, contract.SystemInfo{
			Service:       contract.Semlia,
			ApiVersion:    info.APIVersion,
			SchemaVersion: info.SchemaVersion,
			BuildVersion:  info.BuildVersion,
			TraceId:       traceID,
		})
		return ""
	default:
		panic("known route is not handled")
	}
}

func writeError(response http.ResponseWriter, status int, code, message, traceID string, retryable bool) {
	payload := contract.ErrorResponse{
		Code:      code,
		Message:   message,
		TraceId:   traceID,
		Details:   map[string]interface{}{},
		Retryable: &retryable,
	}
	writeJSON(response, status, payload)
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_, _ = response.Write(append(encoded, '\n'))
}

func knownRoute(path string) bool {
	switch path {
	case "/health/live", "/health/ready", "/api/v1/system/info":
		return true
	default:
		return false
	}
}

func routeLabel(path string) string {
	if knownRoute(path) {
		return path
	}
	return "unmatched"
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.wroteHeader {
		return
	}
	recorder.status = status
	recorder.wroteHeader = true
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(body []byte) (int, error) {
	if !recorder.wroteHeader {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(body)
}

func (recorder *statusRecorder) statusCode() int {
	if recorder.status == 0 {
		return http.StatusOK
	}
	return recorder.status
}

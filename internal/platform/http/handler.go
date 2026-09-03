package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	contract "github.com/iiwish/semlia/api/gen/go"
	"github.com/iiwish/semlia/internal/application"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	headerTraceID   = "X-Trace-ID"
	headerPrincipal = "X-Semlia-Principal"
	retryAfter      = "5"
)

type Handler struct {
	service    *application.SystemService
	catalog    *catalogapp.Service
	governance *governanceapp.AuthoringService
	logger     *slog.Logger
	tracer     trace.Tracer
}

type Option func(*Handler)

func WithCatalog(service *catalogapp.Service) Option {
	return func(handler *Handler) { handler.catalog = service }
}

func WithGovernance(service *governanceapp.AuthoringService) Option {
	return func(handler *Handler) { handler.governance = service }
}

func NewHandler(service *application.SystemService, logger *slog.Logger, tracer trace.Tracer, options ...Option) http.Handler {
	if service == nil {
		panic("system service is required")
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if tracer == nil {
		panic("tracer is required")
	}
	handler := &Handler{service: service, logger: logger, tracer: tracer}
	for _, option := range options {
		if option != nil {
			option(handler)
		}
	}
	return handler
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
	route := matchRoute(request.URL.Path)
	if route.kind == routeUnknown {
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	}
	if !route.allows(request.Method) {
		response.Header().Set("Allow", strings.Join(route.methods(), ", "))
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
		if isGovernanceRoute(route.kind) {
			if handler.governance == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the governance dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeGovernance(response, request, traceID, route)
		}
		if handler.catalog == nil {
			response.Header().Set("Retry-After", retryAfter)
			writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the catalog dependency is unavailable", traceID, true)
			return "DEPENDENCY_UNAVAILABLE"
		}
		return handler.routeCatalog(response, request, traceID, route)
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
	return matchRoute(path).kind != routeUnknown
}

func routeLabel(path string) string {
	if route := matchRoute(path); route.kind != routeUnknown {
		return route.label
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

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	contract "github.com/iiwish/semlia/api/gen/go"
	"github.com/iiwish/semlia/internal/application"
	authorizationapp "github.com/iiwish/semlia/internal/application/authorization"
	catalogapp "github.com/iiwish/semlia/internal/application/catalog"
	discoveryapp "github.com/iiwish/semlia/internal/application/discovery"
	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	embeddingapp "github.com/iiwish/semlia/internal/application/embedding"
	executionapp "github.com/iiwish/semlia/internal/application/execution"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	ingestionapp "github.com/iiwish/semlia/internal/application/ingestion"
	operationsapp "github.com/iiwish/semlia/internal/application/operations"
	webhookapp "github.com/iiwish/semlia/internal/application/webhooks"
	workbenchapp "github.com/iiwish/semlia/internal/application/workbench"
	authorizationdomain "github.com/iiwish/semlia/internal/domain/authorization"
	"github.com/iiwish/semlia/pkg/identity"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	routeAuthorizationRoles routeKind = 1000 + iota
	routeAuthorizationRole
	routeAuthorizationBindings
	routeAuthorizationBindingRevoke
	routeAuthorizationInspect
)

const (
	headerTraceID   = "X-Trace-ID"
	headerPrincipal = "X-Semlia-Principal"
	retryAfter      = "5"
)

type Handler struct {
	service              *application.SystemService
	catalog              *catalogapp.Service
	discovery            *discoveryapp.ControlService
	distribution         *distributionapp.Service
	execution            *executionapp.Service
	authorization        *authorizationapp.Service
	ask                  *governanceapp.AskService
	governance           *governanceapp.AuthoringService
	operations           *operationsapp.Service
	workbench            *workbenchapp.Service
	ingestion            *ingestionapp.Service
	embedding            *embeddingapp.Service
	schedules            *ingestionapp.ScheduleService
	modelConfig          *governanceapp.ModelConfigService
	generation           *governanceapp.GenerationService
	production           *governanceapp.ProductionService
	productionGeneration *governanceapp.ProductionGenerationService
	identity             IdentityService
	machine              MachineIdentityService
	machineLimits        *machineRateLimiter
	mcp                  http.Handler
	webhooks             *webhookapp.Service
	identityConfig       IdentityHTTPConfig
	logger               *slog.Logger
	tracer               trace.Tracer
}

type Option func(*Handler)

func WithEmbedding(service *embeddingapp.Service) Option {
	return func(handler *Handler) { handler.embedding = service }
}

func WithCatalog(service *catalogapp.Service) Option {
	return func(handler *Handler) { handler.catalog = service }
}

func WithDiscovery(service *discoveryapp.ControlService) Option {
	return func(handler *Handler) { handler.discovery = service }
}

func WithDistribution(service *distributionapp.Service) Option {
	return func(handler *Handler) { handler.distribution = service }
}
func WithExecution(service *executionapp.Service) Option {
	return func(handler *Handler) { handler.execution = service }
}

func WithAuthorization(service *authorizationapp.Service) Option {
	return func(handler *Handler) { handler.authorization = service }
}

func WithAsk(service *governanceapp.AskService) Option {
	return func(handler *Handler) { handler.ask = service }
}

func WithGovernance(service *governanceapp.AuthoringService) Option {
	return func(handler *Handler) { handler.governance = service }
}

func WithModelConfig(service *governanceapp.ModelConfigService) Option {
	return func(handler *Handler) { handler.modelConfig = service }
}

func WithGeneration(service *governanceapp.GenerationService) Option {
	return func(handler *Handler) { handler.generation = service }
}

func WithProduction(service *governanceapp.ProductionService) Option {
	return func(handler *Handler) { handler.production = service }
}

func WithOperations(service *operationsapp.Service) Option {
	return func(handler *Handler) { handler.operations = service }
}

func WithWorkbench(service *workbenchapp.Service) Option {
	return func(handler *Handler) { handler.workbench = service }
}

func WithIngestion(service *ingestionapp.Service, schedules *ingestionapp.ScheduleService) Option {
	return func(handler *Handler) {
		handler.ingestion = service
		handler.schedules = schedules
	}
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
	// The T009 surfaces hang off the governance service when it composes them
	// itself (WithModelConfig/WithGeneration on the authoring service); the
	// explicit options only override that default.
	if handler.governance != nil {
		if handler.modelConfig == nil {
			handler.modelConfig = handler.governance.ModelConfig()
		}
		if handler.generation == nil {
			handler.generation = handler.governance.Generation()
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

	authenticatedRequest, authCode, ok := handler.authenticateRequest(recorder, request.WithContext(ctx), traceID, matchHTTPRoute(request.URL.Path))
	if !ok {
		errorCode = authCode
		return
	}
	errorCode = handler.route(recorder, authenticatedRequest, traceID)
}

func (handler *Handler) route(response http.ResponseWriter, request *http.Request, traceID string) string {
	route := matchHTTPRoute(request.URL.Path)
	if route.kind == routeUnknown {
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	}
	methods := route.methods()
	if isAuthorizationRoute(route.kind) {
		methods = authorizationRouteMethods(route.kind)
	}
	if isOperationsRoute(route.kind) {
		methods = operationsRouteMethods(route.kind)
	}
	if !allowsMethod(methods, request.Method) {
		response.Header().Set("Allow", strings.Join(methods, ", "))
		writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "the request method is not allowed", traceID, false)
		return "METHOD_NOT_ALLOWED"
	}

	switch request.URL.Path {
	case "/health/live":
		writeJSON(response, http.StatusOK, contract.HealthResponse{Status: contract.HealthResponseStatusLive, TraceId: traceID})
		return ""
	case "/health/ready":
		if err := handler.service.CheckReadiness(request.Context()); err != nil {
			response.Header().Set("Retry-After", retryAfter)
			writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "a required dependency is unavailable", traceID, true)
			return "DEPENDENCY_UNAVAILABLE"
		}
		writeJSON(response, http.StatusOK, contract.HealthResponse{Status: contract.HealthResponseStatusReady, TraceId: traceID})
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
		if isAuthorizationRoute(route.kind) {
			if handler.authorization == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the authorization dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeAuthorization(response, request, traceID, route)
		}
		if isIdentityRoute(route.kind) {
			if handler.identity == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the identity dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeIdentity(response, request, traceID, route)
		}
		if isProductionRoute(route.kind) {
			if handler.production == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the production dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeProduction(response, request, traceID, route)
		}
		if isGovernanceRoute(route.kind) {
			if handler.governance == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the governance dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeGovernance(response, request, traceID, route)
		}
		if isDiscoveryControlRoute(route.kind) {
			if handler.discovery == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the discovery dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeDiscoveryControl(response, request, traceID, route)
		}
		if isDistributionRoute(route.kind) {
			if handler.distribution == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the semantic distribution dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeDistribution(response, request, traceID, route)
		}
		if isOperationsRoute(route.kind) {
			if handler.operations == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the operations dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeOperations(response, request, traceID, route)
		}
		if isWorkbenchRoute(route.kind) {
			if handler.workbench == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the workbench dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeWorkbench(response, request, traceID, route)
		}
		if isIngestionRoute(route.kind) {
			if handler.ingestion == nil || handler.schedules == nil {
				response.Header().Set("Retry-After", retryAfter)
				writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the ingestion dependency is unavailable", traceID, true)
				return "DEPENDENCY_UNAVAILABLE"
			}
			return handler.routeIngestion(response, request, traceID, route)
		}
		if route.kind >= routeEmbeddingStatus && route.kind <= routeEmbeddingSearch {
			return handler.routeEmbedding(response, request, traceID, route)
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
	return matchHTTPRoute(path).kind != routeUnknown
}

func routeLabel(path string) string {
	if route := matchHTTPRoute(path); route.kind != routeUnknown {
		return route.label
	}
	return "unmatched"
}

func matchHTTPRoute(path string) matchedRoute {
	if route := matchRoute(path); route.kind != routeUnknown {
		return route
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "workspaces" {
		return matchedRoute{}
	}
	route := matchedRoute{workspace: parts[3]}
	switch {
	case len(parts) == 5 && parts[4] == "authorization:inspect":
		route.kind, route.label = routeAuthorizationInspect, "/api/v1/workspaces/{workspaceId}/authorization:inspect"
	case len(parts) == 6 && parts[4] == "authorization" && parts[5] == "roles":
		route.kind, route.label = routeAuthorizationRoles, "/api/v1/workspaces/{workspaceId}/authorization/roles"
	case len(parts) == 7 && parts[4] == "authorization" && parts[5] == "roles":
		route.kind, route.label, route.provider = routeAuthorizationRole, "/api/v1/workspaces/{workspaceId}/authorization/roles/{roleId}", parts[6]
	case len(parts) == 6 && parts[4] == "authorization" && parts[5] == "role-bindings":
		route.kind, route.label = routeAuthorizationBindings, "/api/v1/workspaces/{workspaceId}/authorization/role-bindings"
	case len(parts) == 7 && parts[4] == "authorization" && parts[5] == "role-bindings" && strings.HasSuffix(parts[6], ":revoke"):
		route.kind, route.label, route.binding = routeAuthorizationBindingRevoke, "/api/v1/workspaces/{workspaceId}/authorization/role-bindings/{bindingId}:revoke", strings.TrimSuffix(parts[6], ":revoke")
	case len(parts) == 6 && parts[4] == "operations" && parts[5] == "audit-events":
		route.kind, route.label = routeOperationsAuditEvents, "/api/v1/workspaces/{workspaceId}/operations/audit-events"
	case len(parts) == 6 && parts[4] == "operations" && parts[5] == "audit-exports":
		route.kind, route.label = routeOperationsAuditExports, "/api/v1/workspaces/{workspaceId}/operations/audit-exports"
	case len(parts) == 8 && parts[4] == "operations" && parts[5] == "audit-exports" && parts[7] == "content":
		route.kind, route.label, route.export = routeOperationsAuditExportContent, "/api/v1/workspaces/{workspaceId}/operations/audit-exports/{exportId}/content", parts[6]
	case len(parts) == 6 && parts[4] == "operations" && parts[5] == "runs":
		route.kind, route.label = routeOperationsRuns, "/api/v1/workspaces/{workspaceId}/operations/runs"
	case len(parts) == 7 && parts[4] == "operations" && parts[5] == "runs" && strings.HasSuffix(parts[6], ":retry"):
		route.kind, route.label, route.run = routeOperationsRunRetry, "/api/v1/workspaces/{workspaceId}/operations/runs/{runId}:retry", strings.TrimSuffix(parts[6], ":retry")
	case len(parts) == 7 && parts[4] == "operations" && parts[5] == "runs" && strings.HasSuffix(parts[6], ":cancel"):
		route.kind, route.label, route.run = routeOperationsRunCancel, "/api/v1/workspaces/{workspaceId}/operations/runs/{runId}:cancel", strings.TrimSuffix(parts[6], ":cancel")
	case len(parts) == 7 && parts[4] == "operations" && parts[5] == "runs":
		route.kind, route.label, route.run = routeOperationsRun, "/api/v1/workspaces/{workspaceId}/operations/runs/{runId}", parts[6]
	case len(parts) == 5 && parts[4] == "runtime-policy":
		route.kind, route.label = routeRuntimePolicy, "/api/v1/workspaces/{workspaceId}/runtime-policy"
	case len(parts) == 6 && parts[4] == "workbench" && parts[5] == "items":
		route.kind, route.label = routeWorkbenchItems, "/api/v1/workspaces/{workspaceId}/workbench/items"
	case len(parts) == 7 && parts[4] == "workbench" && parts[5] == "items":
		route.kind, route.label, route.attention = routeWorkbenchItem, "/api/v1/workspaces/{workspaceId}/workbench/items/{attentionItemId}", parts[6]
	}
	return route
}

func isAuthorizationRoute(kind routeKind) bool {
	return kind >= routeAuthorizationRoles && kind <= routeAuthorizationInspect
}

func authorizationRouteMethods(kind routeKind) []string {
	switch kind {
	case routeAuthorizationRoles, routeAuthorizationBindings:
		return []string{http.MethodGet, http.MethodPost}
	case routeAuthorizationRole:
		return []string{http.MethodGet, http.MethodPatch}
	case routeAuthorizationBindingRevoke, routeAuthorizationInspect:
		return []string{http.MethodPost}
	default:
		return nil
	}
}

func allowsMethod(methods []string, method string) bool {
	for _, candidate := range methods {
		if candidate == method {
			return true
		}
	}
	return false
}

func (handler *Handler) routeAuthorization(
	response http.ResponseWriter,
	request *http.Request,
	traceID string,
	route matchedRoute,
) string {
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeAuthorizationError(response, authorizationdomain.ErrInvalidArgument, traceID)
	}
	access := authorizationapp.AccessRequest{
		WorkspaceID: workspaceID, PrincipalRef: principalRef(request), TraceID: traceID,
	}
	switch route.kind {
	case routeAuthorizationRoles:
		if request.Method == http.MethodGet {
			roles, loadErr := handler.authorization.ListRoles(request.Context(), access)
			if loadErr != nil {
				return writeAuthorizationError(response, loadErr, traceID)
			}
			items := make([]contract.AuthorizationRole, 0, len(roles))
			for _, role := range roles {
				items = append(items, authorizationRoleResponse(role))
			}
			writeJSON(response, http.StatusOK, contract.AuthorizationRolePage{Items: items})
			return ""
		}
		var body contract.CreateAuthorizationRoleRequest
		if err := decodeRequest(request, &body); err != nil {
			return writeAuthorizationError(response, authorizationdomain.ErrInvalidArgument, traceID)
		}
		role, version, createErr := handler.authorization.CreateCustomRole(request.Context(), authorizationapp.CreateCustomRoleRequest{
			AccessRequest: access, Name: body.Name, Description: body.Description, Actions: authorizationActions(body.Actions),
		})
		if createErr != nil {
			return writeAuthorizationError(response, createErr, traceID)
		}
		writeJSON(response, http.StatusCreated, contract.AuthorizationRoleMutationResult{Role: authorizationRoleResponse(role), AuthorizationVersion: version})
		return ""
	case routeAuthorizationRole:
		if request.Method == http.MethodGet {
			role, loadErr := handler.authorization.GetRole(request.Context(), access, route.provider)
			if loadErr != nil {
				return writeAuthorizationError(response, loadErr, traceID)
			}
			writeJSON(response, http.StatusOK, authorizationRoleResponse(role))
			return ""
		}
		var body contract.UpdateAuthorizationRoleRequest
		if err := decodeRequest(request, &body); err != nil {
			return writeAuthorizationError(response, authorizationdomain.ErrInvalidArgument, traceID)
		}
		role, version, updateErr := handler.authorization.UpdateCustomRole(request.Context(), authorizationapp.UpdateCustomRoleRequest{
			AccessRequest: access, RoleID: route.provider, ExpectedVersion: body.ExpectedVersion,
			Name: body.Name, Description: body.Description, Actions: authorizationActions(body.Actions),
		})
		if updateErr != nil {
			return writeAuthorizationError(response, updateErr, traceID)
		}
		writeJSON(response, http.StatusOK, contract.AuthorizationRoleMutationResult{Role: authorizationRoleResponse(role), AuthorizationVersion: version})
		return ""
	case routeAuthorizationBindings:
		if request.Method == http.MethodGet {
			bindings, loadErr := handler.authorization.ListBindings(request.Context(), access)
			if loadErr != nil {
				return writeAuthorizationError(response, loadErr, traceID)
			}
			items := make([]contract.AuthorizationRoleBinding, 0, len(bindings))
			for _, binding := range bindings {
				items = append(items, authorizationBindingResponse(binding))
			}
			writeJSON(response, http.StatusOK, contract.AuthorizationRoleBindingPage{Items: items})
			return ""
		}
		var body contract.CreateAuthorizationRoleBindingRequest
		if err := decodeRequest(request, &body); err != nil {
			return writeAuthorizationError(response, authorizationdomain.ErrInvalidArgument, traceID)
		}
		if authorizationdomain.ScopeType(body.Scope.Type) == authorizationdomain.ScopeWorkspace {
			if publicScope, err := identity.ParseWorkspaceID(body.Scope.Id); err == nil {
				body.Scope.Id = publicScope.UUID()
			}
		}
		binding, version, createErr := handler.authorization.CreateBinding(request.Context(), authorizationapp.CreateRoleBindingRequest{
			AccessRequest: access, PrincipalID: body.PrincipalId, RoleID: body.RoleId,
			ExpectedRoleVersion: body.ExpectedRoleVersion, ScopeType: authorizationdomain.ScopeType(body.Scope.Type),
			ScopeID: body.Scope.Id, ExpiresAt: body.ExpiresAt,
		})
		if createErr != nil {
			return writeAuthorizationError(response, createErr, traceID)
		}
		writeJSON(response, http.StatusCreated, contract.AuthorizationRoleBindingMutationResult{Binding: authorizationBindingResponse(binding), AuthorizationVersion: version})
		return ""
	case routeAuthorizationBindingRevoke:
		bindingID, parseErr := identity.ParseBindingID(route.binding)
		if parseErr != nil {
			return writeAuthorizationError(response, authorizationdomain.ErrInvalidArgument, traceID)
		}
		var body contract.RevokeAuthorizationRoleBindingRequest
		if err := decodeRequest(request, &body); err != nil {
			return writeAuthorizationError(response, authorizationdomain.ErrInvalidArgument, traceID)
		}
		binding, version, revokeErr := handler.authorization.RevokeBinding(request.Context(), authorizationapp.RevokeRoleBindingRequest{
			AccessRequest: access, BindingID: bindingID, ExpectedVersion: body.ExpectedVersion, Reason: body.Reason,
		})
		if revokeErr != nil {
			return writeAuthorizationError(response, revokeErr, traceID)
		}
		writeJSON(response, http.StatusOK, contract.AuthorizationRoleBindingMutationResult{Binding: authorizationBindingResponse(binding), AuthorizationVersion: version})
		return ""
	case routeAuthorizationInspect:
		var body contract.InspectAuthorizationRequest
		if err := decodeRequest(request, &body); err != nil {
			return writeAuthorizationError(response, authorizationdomain.ErrInvalidArgument, traceID)
		}
		domainID := ""
		if body.Resource.DomainId != nil {
			domainID = *body.Resource.DomainId
		}
		decision, inspectErr := handler.authorization.Inspect(request.Context(), authorizationapp.InspectRequest{
			AccessRequest: access, TargetPrincipalRef: body.PrincipalId.String(),
			Action: authorizationdomain.Action(body.Action), Resource: authorizationdomain.Resource{
				Type: authorizationdomain.ScopeType(body.Resource.Type), ID: body.Resource.Id, DomainID: domainID,
			},
		})
		if inspectErr != nil {
			return writeAuthorizationError(response, inspectErr, traceID)
		}
		writeJSON(response, http.StatusOK, authorizationDecisionResponse(decision, body.PrincipalId))
		return ""
	default:
		panic("authorization route is not handled")
	}
}

func writeAuthorizationError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authorizationdomain.DenialError
	switch {
	case errors.As(err, &denial):
		code := string(denial.Decision.ReasonCode)
		writeError(response, http.StatusForbidden, code, "the acting principal lacks the required capability", traceID, false)
		return code
	case errors.Is(err, authorizationdomain.ErrInvalidArgument):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, authorizationdomain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested authorization resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, authorizationdomain.ErrAuthorizationCeiling):
		writeError(response, http.StatusForbidden, "AUTHORIZATION_CEILING", "the requested grant exceeds the acting principal's authorization", traceID, false)
		return "AUTHORIZATION_CEILING"
	case errors.Is(err, authorizationdomain.ErrRolesImmutable):
		writeError(response, http.StatusConflict, "SYSTEM_ROLE_IMMUTABLE", "system roles are immutable", traceID, false)
		return "SYSTEM_ROLE_IMMUTABLE"
	case errors.Is(err, authorizationdomain.ErrVersionConflict):
		writeError(response, http.StatusConflict, "VERSION_CONFLICT", "the authorization resource version has changed", traceID, false)
		return "VERSION_CONFLICT"
	case errors.Is(err, authorizationdomain.ErrSeparationOfDuties):
		writeError(response, http.StatusConflict, "SEPARATION_OF_DUTY", "the assignment conflicts with authorization policy", traceID, false)
		return "SEPARATION_OF_DUTY"
	case errors.Is(err, authorizationdomain.ErrFinalAdministrator):
		writeError(response, http.StatusConflict, "FINAL_ADMINISTRATOR", "the final workspace administrator cannot be revoked", traceID, false)
		return "FINAL_ADMINISTRATOR"
	case errors.Is(err, authorizationdomain.ErrConflict), errors.Is(err, authorizationdomain.ErrInvariant):
		writeError(response, http.StatusConflict, "CONFLICT", "the request conflicts with authorization policy", traceID, false)
		return "CONFLICT"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}

func authorizationRoleResponse(role authorizationdomain.Role) contract.AuthorizationRole {
	result := contract.AuthorizationRole{
		Id: role.ID, Name: role.Name, Description: role.Description,
		Category: contract.AuthorizationRoleCategory(role.Category), Version: role.Version,
		Actions: make([]contract.AuthorizationAction, 0, len(role.Actions)),
	}
	for _, action := range role.Actions {
		result.Actions = append(result.Actions, contract.AuthorizationAction(action))
	}
	if !role.WorkspaceID.IsZero() {
		workspaceID := role.WorkspaceID
		result.WorkspaceId = &workspaceID
	}
	if !role.CreatedAt.IsZero() {
		createdAt := role.CreatedAt.UTC()
		result.CreatedAt = &createdAt
	}
	return result
}

func authorizationBindingResponse(binding authorizationdomain.RoleBinding) contract.AuthorizationRoleBinding {
	result := contract.AuthorizationRoleBinding{
		Id: binding.ID, WorkspaceId: binding.WorkspaceID, PrincipalId: binding.PrincipalID,
		RoleId: binding.RoleID, RoleVersion: binding.RoleVersion,
		Scope:     contract.AuthorizationScope{Type: contract.AuthorizationScopeType(binding.ScopeType), Id: binding.ScopeID},
		Status:    contract.AuthorizationRoleBindingStatus(binding.StatusAt(time.Now().UTC())),
		GrantedAt: binding.GrantedAt.UTC(), GrantedBy: binding.GrantedBy, Version: binding.Version,
	}
	if binding.ExpiresAt != nil {
		value := binding.ExpiresAt.UTC()
		result.ExpiresAt = &value
	}
	if binding.ExpiredAt != nil {
		value := binding.ExpiredAt.UTC()
		result.ExpiredAt = &value
	}
	if binding.RevokedAt != nil {
		value := binding.RevokedAt.UTC()
		result.RevokedAt = &value
	}
	result.RevokedBy = binding.RevokedBy
	if binding.RevokeReason != "" {
		reason := binding.RevokeReason
		result.RevocationReason = &reason
	}
	return result
}

func authorizationDecisionResponse(decision authorizationdomain.Decision, fallback identity.PrincipalID) contract.AuthorizationDecision {
	principalID := decision.PrincipalID
	if principalID.IsZero() {
		principalID = fallback
	}
	result := contract.AuthorizationDecision{
		PrincipalId: principalID, Action: contract.AuthorizationAction(decision.Action), Allowed: decision.Allowed,
		ReasonCode: contract.AuthorizationDecisionReasonCode(decision.ReasonCode), AuthorizationVersion: decision.AuthorizationVersion,
	}
	if decision.RoleID != "" {
		roleID := decision.RoleID
		result.RoleId = &roleID
	}
	if !decision.BindingID.IsZero() {
		bindingID := decision.BindingID
		result.BindingId = &bindingID
	}
	return result
}

func authorizationActions(items []contract.AuthorizationAction) []authorizationdomain.Action {
	result := make([]authorizationdomain.Action, 0, len(items))
	for _, item := range items {
		result = append(result, authorizationdomain.Action(item))
	}
	return result
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

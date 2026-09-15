package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	contract "github.com/iiwish/semlia/api/gen/go"
	operationsapp "github.com/iiwish/semlia/internal/application/operations"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	routeOperationsAuditEvents routeKind = 2000 + iota
	routeOperationsAuditExports
	routeOperationsAuditExportContent
	routeOperationsRuns
	routeOperationsRun
	routeOperationsRunRetry
	routeOperationsRunCancel
	routeRuntimePolicy
)

func isOperationsRoute(kind routeKind) bool {
	return kind >= routeOperationsAuditEvents && kind <= routeRuntimePolicy
}

func operationsRouteMethods(kind routeKind) []string {
	switch kind {
	case routeOperationsAuditEvents, routeOperationsAuditExportContent, routeOperationsRuns, routeOperationsRun:
		return []string{http.MethodGet}
	case routeOperationsAuditExports, routeOperationsRunRetry, routeOperationsRunCancel:
		return []string{http.MethodPost}
	case routeRuntimePolicy:
		return []string{http.MethodGet, http.MethodPatch}
	default:
		return nil
	}
}

func (handler *Handler) routeOperations(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	if !allowsMethod(operationsRouteMethods(route.kind), request.Method) {
		response.Header().Set("Allow", strings.Join(operationsRouteMethods(route.kind), ", "))
		writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "the request method is not allowed", traceID, false)
		return "METHOD_NOT_ALLOWED"
	}
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeOperationsError(response, domain.ErrInvalidArgument, traceID)
	}
	access := operationsapp.AccessRequest{WorkspaceID: workspaceID, PrincipalRef: principalRef(request), TraceID: traceID}
	switch route.kind {
	case routeOperationsAuditEvents:
		input, parseErr := auditListRequest(request, access)
		if parseErr != nil {
			return writeOperationsError(response, parseErr, traceID)
		}
		page, listErr := handler.operations.ListAuditEvents(request.Context(), input)
		if listErr != nil {
			return writeOperationsError(response, listErr, traceID)
		}
		writeJSON(response, http.StatusOK, auditPageResponse(page, normalizedPageLimit(input.Limit)))
	case routeOperationsAuditExports:
		var body contract.CreateOperationsAuditExportRequest
		if decodeRequest(request, &body) != nil {
			return writeOperationsError(response, domain.ErrInvalidArgument, traceID)
		}
		input := operationsapp.CreateAuditExportRequest{AuditListRequest: auditListFromFilter(access, body.Filter), IdempotencyKey: body.IdempotencyKey}
		exported, createErr := handler.operations.CreateAuditExport(request.Context(), input)
		if createErr != nil {
			return writeOperationsError(response, createErr, traceID)
		}
		writeJSON(response, http.StatusCreated, auditExportResponse(exported))
	case routeOperationsAuditExportContent:
		exportID, parseErr := identity.ParseEventID(route.export)
		if parseErr != nil {
			return writeOperationsError(response, domain.ErrInvalidArgument, traceID)
		}
		content, getErr := handler.operations.GetAuditExportContent(request.Context(), access, exportID)
		if getErr != nil {
			return writeOperationsError(response, getErr, traceID)
		}
		response.Header().Set("Content-Type", "application/json")
		response.Header().Set("Content-Disposition", `attachment; filename="audit-export-`+content.ID.String()+`.json"`)
		response.Header().Set("Cache-Control", "private, no-store")
		response.Header().Set("ETag", `"sha256:`+content.ContentDigest+`"`)
		response.Header().Set("X-Content-SHA256", content.ContentDigest)
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(content.Content)
	case routeOperationsRuns:
		limit, parseErr := queryInteger(request, "limit")
		if parseErr != nil {
			return writeOperationsError(response, parseErr, traceID)
		}
		input := operationsapp.RuntimeListRequest{AccessRequest: access,
			Kind: domain.RunKind(request.URL.Query().Get("kind")), State: domain.RunState(request.URL.Query().Get("state")),
			SourceType: request.URL.Query().Get("sourceType"), SourceID: request.URL.Query().Get("sourceId"),
			TraceFilter: request.URL.Query().Get("traceId"), Cursor: request.URL.Query().Get("cursor"), Limit: limit}
		page, listErr := handler.operations.ListRuns(request.Context(), input)
		if listErr != nil {
			return writeOperationsError(response, listErr, traceID)
		}
		writeJSON(response, http.StatusOK, runtimePageResponse(page, normalizedPageLimit(limit)))
	case routeOperationsRun:
		runID, parseErr := identity.ParseRunID(route.run)
		if parseErr != nil {
			return writeOperationsError(response, domain.ErrInvalidArgument, traceID)
		}
		detail, getErr := handler.operations.GetRun(request.Context(), access, runID)
		if getErr != nil {
			return writeOperationsError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, runtimeDetailResponse(detail))
	case routeOperationsRunRetry, routeOperationsRunCancel:
		runID, parseErr := identity.ParseRunID(route.run)
		if parseErr != nil {
			return writeOperationsError(response, domain.ErrInvalidArgument, traceID)
		}
		if route.kind == routeOperationsRunRetry {
			err = handler.operations.RetryRun(request.Context(), access, runID)
		} else {
			err = handler.operations.CancelRun(request.Context(), access, runID)
		}
		if err != nil {
			return writeOperationsError(response, err, traceID)
		}
		response.WriteHeader(http.StatusAccepted)
	case routeRuntimePolicy:
		if request.Method == http.MethodGet {
			policy, getErr := handler.operations.GetRuntimePolicy(request.Context(), access)
			if getErr != nil {
				return writeOperationsError(response, getErr, traceID)
			}
			writeJSON(response, http.StatusOK, runtimePolicyResponse(policy))
			break
		}
		var body contract.UpdateOperationsRuntimeSettingsRequest
		if decodeRequest(request, &body) != nil {
			return writeOperationsError(response, domain.ErrInvalidArgument, traceID)
		}
		policy, updateErr := handler.operations.UpdateRuntimeSettings(request.Context(), operationsapp.UpdateRuntimeSettingsRequest{
			AccessRequest: access, ExpectedVersion: body.ExpectedVersion, RetryCeiling: int32(body.RetryCeiling),
			StatementTimeoutMS: int32(body.StatementTimeoutMs), WebhookTimeoutMS: int32(body.WebhookTimeoutMs),
			QueryRowLimit: int32(body.QueryRowLimit), QueryByteLimit: body.QueryByteLimit,
			RunMetadataRetentionDays: int32(body.RunMetadataRetentionDays)})
		if updateErr != nil {
			return writeOperationsError(response, updateErr, traceID)
		}
		writeJSON(response, http.StatusOK, runtimePolicyResponse(policy))
	default:
		panic("operations route is not handled")
	}
	return ""
}

func auditListRequest(request *http.Request, access operationsapp.AccessRequest) (operationsapp.AuditListRequest, error) {
	limit, err := queryInteger(request, "limit")
	if err != nil {
		return operationsapp.AuditListRequest{}, err
	}
	from, err := queryTimestamp(request.URL.Query().Get("from"))
	if err != nil {
		return operationsapp.AuditListRequest{}, err
	}
	to, err := queryTimestamp(request.URL.Query().Get("to"))
	if err != nil {
		return operationsapp.AuditListRequest{}, err
	}
	return operationsapp.AuditListRequest{AccessRequest: access, ActorID: request.URL.Query().Get("actorId"),
		EventType: request.URL.Query().Get("eventType"), ObjectType: request.URL.Query().Get("objectType"),
		ObjectID: request.URL.Query().Get("objectId"), TraceFilter: request.URL.Query().Get("traceId"),
		From: from, To: to, Cursor: request.URL.Query().Get("cursor"), Limit: limit}, nil
}

func auditListFromFilter(access operationsapp.AccessRequest, filter contract.OperationsAuditFilter) operationsapp.AuditListRequest {
	return operationsapp.AuditListRequest{AccessRequest: access, ActorID: optionalString(filter.ActorId),
		EventType: optionalString(filter.EventType), ObjectType: optionalString(filter.ObjectType),
		ObjectID: optionalString(filter.ObjectId), TraceFilter: optionalTrace(filter.TraceId),
		From: filter.From, To: filter.To, Limit: operationsapp.MaxExportRows}
}

func auditPageResponse(page operationsapp.AuditPage, limit int) contract.OperationsAuditEventPage {
	result := contract.OperationsAuditEventPage{Items: make([]contract.OperationsAuditEvent, 0, len(page.Items)),
		Page: contract.PageInfo{Limit: limit}}
	if page.NextCursor != "" {
		cursor := contract.Cursor(page.NextCursor)
		result.Page.NextCursor = &cursor
	}
	for _, item := range page.Items {
		result.Items = append(result.Items, contract.OperationsAuditEvent{Id: item.ID, EventType: item.EventType,
			ActorId: item.ActorID, ObjectType: item.ObjectType, ObjectId: item.ObjectID, Channel: item.Channel,
			Outcome: item.Outcome, ReasonCode: item.ReasonCode, TraceId: item.TraceID,
			Summary: item.Summary, CreatedAt: item.CreatedAt.UTC()})
	}
	return result
}

func auditExportResponse(value operationsapp.AuditExport) contract.OperationsAuditExport {
	return contract.OperationsAuditExport{Id: value.ID, RuntimeRunId: value.RuntimeRunID, ArtifactId: value.ArtifactID,
		Format: contract.OperationsAuditExportFormat(value.Format), RowCount: value.RowCount,
		ContentDigest: value.ContentDigest, CreatedAt: value.CreatedAt.UTC(), ExpiresAt: value.ExpiresAt.UTC()}
}

func runtimePageResponse(page operationsapp.RuntimePage, limit int) contract.OperationsRuntimeRunPage {
	result := contract.OperationsRuntimeRunPage{Items: make([]contract.OperationsRuntimeRun, 0, len(page.Items)),
		Page: contract.PageInfo{Limit: limit}}
	if page.NextCursor != "" {
		cursor := contract.Cursor(page.NextCursor)
		result.Page.NextCursor = &cursor
	}
	for _, item := range page.Items {
		result.Items = append(result.Items, runtimeRunResponse(item))
	}
	return result
}

func runtimeDetailResponse(value operationsapp.RuntimeDetail) contract.OperationsRuntimeRunDetail {
	result := contract.OperationsRuntimeRunDetail{Run: runtimeRunResponse(value.Run),
		Events: make([]contract.OperationsRuntimeRunEvent, 0, len(value.Events))}
	for _, event := range value.Events {
		item := contract.OperationsRuntimeRunEvent{Id: event.ID, Sequence: event.Sequence,
			EventType: contract.OperationsRuntimeRunEventEventType(event.Type), CreatedAt: event.CreatedAt.UTC(),
			Phase: optionalStringPointer(event.Phase), ProgressCurrent: event.ProgressCurrent, ProgressTotal: event.ProgressTotal,
			Summary: optionalStringPointer(event.Summary)}
		if event.State != "" {
			state := contract.OperationsRuntimeRunState(event.State)
			item.State = &state
		}
		if event.ErrorCode != "" {
			code := contract.ErrorCode(event.ErrorCode)
			item.ErrorCode = &code
		}
		result.Events = append(result.Events, item)
	}
	return result
}

func runtimeRunResponse(value domain.RuntimeRun) contract.OperationsRuntimeRun {
	result := contract.OperationsRuntimeRun{Id: value.ID, Kind: contract.OperationsRuntimeRunKind(value.Kind),
		SourceType: value.SourceType, SourceId: value.SourceID, SourceVersionDigest: value.SourceVersionDigest,
		TraceId: optionalTracePointer(value.TraceID), IdempotencyKey: value.IdempotencyKey, State: contract.OperationsRuntimeRunState(value.State),
		Phase: optionalStringPointer(value.Phase), ProgressCurrent: value.ProgressCurrent, ProgressTotal: value.ProgressTotal,
		Attempt: int(value.Attempt), MaxAttempts: int(value.MaxAttempts), Capabilities: contract.OperationsRuntimeRunCapabilities{
			Retry: value.Capabilities.Retry, Cancel: value.Capabilities.Cancel}, Version: value.Version,
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(), StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		ErrorSummary: optionalStringPointer(value.ErrorSummary)}
	if value.JobID != nil {
		result.JobId = value.JobID
	}
	if value.RequestedByPrincipalID != nil {
		result.RequestedByPrincipalId = value.RequestedByPrincipalID
	}
	if value.ErrorCode != "" {
		code := contract.ErrorCode(value.ErrorCode)
		result.ErrorCode = &code
	}
	return result
}

func runtimePolicyResponse(value operationsapp.RuntimePolicy) contract.OperationsRuntimePolicy {
	settings := value.Settings
	return contract.OperationsRuntimePolicy{Settings: contract.OperationsRuntimeSettings{
		RetryCeiling: int(settings.RetryCeiling), StatementTimeoutMs: int(settings.StatementTimeoutMS),
		WebhookTimeoutMs: int(settings.WebhookTimeoutMS), QueryRowLimit: int(settings.QueryRowLimit),
		QueryByteLimit: settings.QueryByteLimit, RunMetadataRetentionDays: int(settings.RunMetadataRetentionDays),
		Version: settings.Version, UpdatedAt: settings.UpdatedAt.UTC()}, Deployment: contract.OperationsDeploymentStatus{
		WorkerConfigured: value.Deployment.WorkerConfigured, TelemetryConfigured: value.Deployment.TelemetryConfigured,
		OidcConfigured: value.Deployment.OIDCConfigured, EncryptionConfigured: value.Deployment.EncryptionConfigured,
		AuditRetention: contract.OperationsDeploymentStatusAuditRetention(value.Deployment.AuditRetention)}}
}

func writeOperationsError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authorization.DenialError
	switch {
	case errors.As(err, &denial):
		code := string(denial.Decision.ReasonCode)
		if code == "" {
			code = "FORBIDDEN"
		}
		writeError(response, http.StatusForbidden, code, "the operation is not authorized", traceID, false)
		return code
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the operations request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the runtime resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrRunActionUnsupported):
		writeError(response, http.StatusConflict, "RUN_ACTION_UNSUPPORTED", "the owning runtime does not support this action", traceID, false)
		return "RUN_ACTION_UNSUPPORTED"
	case errors.Is(err, domain.ErrConflict):
		writeError(response, http.StatusConflict, "VERSION_CONFLICT", "the operations resource changed or exceeds a bounded limit", traceID, false)
		return "VERSION_CONFLICT"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the operations request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}

func queryTimestamp(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func normalizedPageLimit(value int) int {
	if value <= 0 {
		return operationsapp.DefaultPageSize
	}
	if value > operationsapp.MaxPageSize {
		return operationsapp.MaxPageSize
	}
	return value
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalTrace(value *contract.TraceId) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func optionalTracePointer(value string) *contract.TraceId {
	if value == "" {
		return nil
	}
	traceID := contract.TraceId(value)
	return &traceID
}

func optionalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

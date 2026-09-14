package httpapi

import (
	"errors"
	"net/http"
	"strings"

	contract "github.com/iiwish/semlia/api/gen/go"
	workbenchapp "github.com/iiwish/semlia/internal/application/workbench"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/workbench"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	routeWorkbenchItems routeKind = 3000 + iota
	routeWorkbenchItem
)

func isWorkbenchRoute(kind routeKind) bool {
	return kind >= routeWorkbenchItems && kind <= routeWorkbenchItem
}

func (handler *Handler) routeWorkbench(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeWorkbenchError(response, domain.ErrInvalidArgument, traceID)
	}
	if route.kind == routeWorkbenchItems {
		if request.Method != http.MethodGet {
			return workbenchMethodNotAllowed(response, traceID, http.MethodGet)
		}
		limit, parseErr := queryInteger(request, "limit")
		if parseErr != nil {
			return writeWorkbenchError(response, parseErr, traceID)
		}
		page, listErr := handler.workbench.List(request.Context(), workbenchapp.ListRequest{
			WorkspaceID: workspaceID, PrincipalRef: principalRef(request), TraceID: traceID,
			View: domain.View(request.URL.Query().Get("view")), Search: request.URL.Query().Get("search"),
			Kind: domain.Kind(request.URL.Query().Get("kind")), State: domain.State(request.URL.Query().Get("state")),
			Priority: domain.Priority(request.URL.Query().Get("priority")), Risk: domain.Priority(request.URL.Query().Get("risk")),
			Sort: domain.Sort(request.URL.Query().Get("sort")), Limit: limit, Cursor: request.URL.Query().Get("cursor"),
		})
		if listErr != nil {
			return writeWorkbenchError(response, listErr, traceID)
		}
		writeJSON(response, http.StatusOK, workbenchPageResponse(page))
		return ""
	}
	itemID, err := identity.ParseAttentionItemID(route.attention)
	if err != nil {
		return writeWorkbenchError(response, domain.ErrInvalidArgument, traceID)
	}
	switch request.Method {
	case http.MethodGet:
		item, getErr := handler.workbench.Get(request.Context(), workspaceID, itemID, principalRef(request), traceID)
		if getErr != nil {
			return writeWorkbenchError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, workbenchItemResponse(item))
		return ""
	case http.MethodPatch:
		var body contract.UpdateWorkbenchAttentionItemRequest
		if decodeRequest(request, &body) != nil {
			return writeWorkbenchError(response, domain.ErrInvalidArgument, traceID)
		}
		setAssignee := body.SetAssignee != nil && *body.SetAssignee
		var state domain.State
		if body.State != nil {
			state = domain.State(*body.State)
		}
		item, updateErr := handler.workbench.Update(request.Context(), workbenchapp.UpdateRequest{
			WorkspaceID: workspaceID, ItemID: itemID, PrincipalRef: principalRef(request), TraceID: traceID,
			IdempotencyKey: request.Header.Get("Idempotency-Key"), AssigneePrincipalID: body.AssigneePrincipalId,
			SetAssignee: setAssignee, State: state, ExpectedVersion: body.ExpectedVersion,
		})
		if updateErr != nil {
			return writeWorkbenchError(response, updateErr, traceID)
		}
		writeJSON(response, http.StatusOK, workbenchItemResponse(item))
		return ""
	default:
		return workbenchMethodNotAllowed(response, traceID, http.MethodGet, http.MethodPatch)
	}
}

func workbenchMethodNotAllowed(response http.ResponseWriter, traceID string, methods ...string) string {
	response.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "the request method is not allowed", traceID, false)
	return "METHOD_NOT_ALLOWED"
}

func workbenchPageResponse(page workbenchapp.Page) contract.WorkbenchAttentionPage {
	items := make([]contract.WorkbenchAttentionItem, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, workbenchItemResponse(item))
	}
	return contract.WorkbenchAttentionPage{Items: items,
		Counts: contract.WorkbenchAttentionCounts{Total: page.Counts.Total, Open: page.Counts.Open,
			InProgress: page.Counts.InProgress, Critical: page.Counts.Critical},
		Page: contract.PageInfo{Limit: page.Limit, NextCursor: workbenchCursor(page.NextCursor)},
	}
}

func workbenchItemResponse(item domain.Item) contract.WorkbenchAttentionItem {
	actions := make([]contract.WorkbenchAction, 0, len(item.NextActions))
	for _, action := range item.NextActions {
		actions = append(actions, contract.WorkbenchAction(action))
	}
	result := contract.WorkbenchAttentionItem{Id: item.ID, Kind: contract.WorkbenchAttentionKind(item.Kind),
		State: contract.WorkbenchAttentionState(item.State), Priority: contract.WorkbenchPriority(item.Priority),
		Risk: contract.WorkbenchPriority(item.Risk), TargetType: item.TargetType, TargetId: item.TargetID,
		TargetRoute: item.TargetRoute, AssigneePrincipalId: item.AssigneePrincipalID,
		InitiatorPrincipalId: item.InitiatorPrincipalID, RuleVersion: item.RuleVersion, Title: item.Title,
		Summary: item.Summary, ReasonCode: item.ReasonCode, TraceId: item.TraceID,
		OpenedAt: item.OpenedAt.UTC(), UpdatedAt: item.UpdatedAt.UTC(), Version: item.Version, NextActions: actions}
	if item.AudienceRoleID != "" {
		result.AudienceRoleId = &item.AudienceRoleID
	}
	if item.EvidenceRef != "" {
		result.EvidenceRef = &item.EvidenceRef
	}
	result.DueAt, result.ResolvedAt = item.DueAt, item.ResolvedAt
	return result
}

func writeWorkbenchError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authorization.DenialError
	switch {
	case errors.As(err, &denial):
		writeError(response, http.StatusForbidden, string(denial.Decision.ReasonCode),
			"the acting principal lacks the required capability", traceID, false)
		return string(denial.Decision.ReasonCode)
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict):
		writeError(response, http.StatusConflict, "CONFLICT", "the request conflicts with current state", traceID, false)
		return "CONFLICT"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}

func workbenchCursor(value string) *contract.Cursor {
	if value == "" {
		return nil
	}
	cursor := contract.Cursor(value)
	return &cursor
}

package httpapi

import (
	"errors"
	"net/http"

	app "github.com/iiwish/semlia/internal/application/execution"
	dist "github.com/iiwish/semlia/internal/domain/distribution"
	d "github.com/iiwish/semlia/internal/domain/execution"
	"github.com/iiwish/semlia/pkg/identity"
)

func (h *Handler) routeExecution(w http.ResponseWriter, r *http.Request, trace string, route matchedRoute, workspace identity.WorkspaceID) string {
	w.Header().Set("Cache-Control", "no-store")
	if h.execution == nil {
		writeError(w, 503, "EXECUTION_NOT_CONFIGURED", "query execution is not configured", trace, false)
		return "EXECUTION_NOT_CONFIGURED"
	}
	var result d.Result
	var err error
	if route.kind == routeQueryExecution {
		result, err = h.execution.Get(r.Context(), workspace, route.query, principalRef(r), trace)
	} else if route.kind == routeQueryExecutionCancel {
		result, err = h.execution.Cancel(r.Context(), workspace, route.query, principalRef(r), trace)
	} else {
		var input app.Request
		if decodeRequest(r, &input) != nil {
			return writeDistributionError(w, dist.ErrInvalidArgument, trace)
		}
		if route.plan != "" {
			plan, parseErr := identity.ParseResolvedSemanticPlanID(route.plan)
			if parseErr != nil || (!input.PlanID.IsZero() && input.PlanID != plan) {
				return writeDistributionError(w, dist.ErrInvalidArgument, trace)
			}
			input.PlanID = plan
		}
		input.WorkspaceID = workspace
		input.PrincipalRef = principalRef(r)
		input.TraceID = trace
		if input.Channel != "web" {
			input.Channel = canonicalHTTPChannel(input.Channel)
		}
		result, err = h.execution.Execute(r.Context(), input)
	}
	if err != nil {
		switch {
		case errors.Is(err, d.ErrInvalidPlan):
			writeError(w, 422, "EXECUTION_PLAN_UNSUPPORTED", "plan is stale, unsupported or lacks immutable execution provenance", trace, false)
			return "EXECUTION_PLAN_UNSUPPORTED"
		case errors.Is(err, d.ErrConflict), errors.Is(err, d.ErrBusy):
			writeError(w, 409, "EXECUTION_CONFLICT", "execution conflicts with the current request or capacity", trace, false)
			return "EXECUTION_CONFLICT"
		case errors.Is(err, d.ErrNotFound):
			return writeDistributionError(w, dist.ErrNotFound, trace)
		default:
			return writeDistributionError(w, err, trace)
		}
	}
	writeJSON(w, 200, result)
	return ""
}

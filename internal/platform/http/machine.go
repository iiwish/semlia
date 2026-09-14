package httpapi

import (
	"context"
	authapp "github.com/iiwish/semlia/internal/application/authorization"
	app "github.com/iiwish/semlia/internal/application/identity"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/identity"
	id "github.com/iiwish/semlia/pkg/identity"
	"net/http"
	"sync"
	"time"
)

type MachineIdentityService interface {
	Authenticate(context.Context, string) (authapp.CredentialLimit, error)
	List(context.Context, id.WorkspaceID, string, string) ([]domain.ClientCredential, error)
	Issue(context.Context, app.IssueMachineCredential) (domain.IssuedCredential, error)
	Rotate(context.Context, id.WorkspaceID, id.ClientCredentialID, string, string) (domain.IssuedCredential, error)
	Revoke(context.Context, id.WorkspaceID, id.ClientCredentialID, string, string) error
	CreatePrincipal(context.Context, id.WorkspaceID, string, string, string) (auth.Principal, error)
}

func WithMachineIdentity(service MachineIdentityService) Option {
	return func(h *Handler) {
		h.machine = service
		h.machineLimits = &machineRateLimiter{windows: map[string]rateWindow{}}
	}
}
func WithMCP(handler http.Handler) Option { return func(h *Handler) { h.mcp = handler } }
func machineRouteAllowed(kind routeKind) bool {
	switch kind {
	case routeSemanticResolve, routeSemanticQuery, routeResolvedSemanticPlan, routeSemanticSearch, routeSemanticDescribe, routeMCP, routeSemanticExecute, routeQueryExecution, routeQueryExecutionCancel:
		return true
	}
	return false
}
func (h *Handler) routeMachine(w http.ResponseWriter, r *http.Request, trace string, route matchedRoute, workspace id.WorkspaceID) string {
	w.Header().Set("Cache-Control", "no-store")
	if h.machine == nil {
		writeError(w, 503, "DEPENDENCY_UNAVAILABLE", "machine identity is unavailable", trace, true)
		return "DEPENDENCY_UNAVAILABLE"
	}
	var err error
	switch route.kind {
	case routeMachinePrincipals:
		var body struct {
			Name string `json:"name"`
		}
		if decodeRequest(r, &body) != nil {
			return writeIdentityError(w, domain.ErrInvalidArgument, trace)
		}
		p, e := h.machine.CreatePrincipal(r.Context(), workspace, principalRef(r), trace, body.Name)
		err = e
		if err == nil {
			writeJSON(w, 201, map[string]any{"id": p.ID, "name": p.DisplayName, "status": p.Status})
			return ""
		}
	case routeClientCredentials:
		if r.Method == http.MethodGet {
			items, e := h.machine.List(r.Context(), workspace, principalRef(r), trace)
			err = e
			if err == nil {
				writeJSON(w, 200, map[string]any{"items": items})
				return ""
			}
		} else {
			var body app.IssueMachineCredential
			if decodeRequest(r, &body) != nil {
				return writeIdentityError(w, domain.ErrInvalidArgument, trace)
			}
			body.WorkspaceID, body.Actor, body.TraceID = workspace, principalRef(r), trace
			issued, e := h.machine.Issue(r.Context(), body)
			err = e
			if err == nil {
				writeJSON(w, 201, issued)
				return ""
			}
		}
	default:
		if r.ContentLength != 0 {
			var body struct{}
			if decodeRequest(r, &body) != nil {
				return writeIdentityError(w, domain.ErrInvalidArgument, trace)
			}
		}
		credential, e := id.ParseClientCredentialID(route.query)
		if e != nil {
			return writeIdentityError(w, domain.ErrInvalidArgument, trace)
		}
		if route.kind == routeClientCredentialRotate {
			issued, e := h.machine.Rotate(r.Context(), workspace, credential, principalRef(r), trace)
			err = e
			if err == nil {
				writeJSON(w, 201, issued)
				return ""
			}
		} else {
			err = h.machine.Revoke(r.Context(), workspace, credential, principalRef(r), trace)
			if err == nil {
				w.WriteHeader(204)
				return ""
			}
		}
	}
	if _, ok := err.(*auth.DenialError); ok {
		return writeDistributionError(w, err, trace)
	}
	return writeIdentityError(w, err, trace)
}

type rateWindow struct {
	minute int64
	count  int
}
type machineRateLimiter struct {
	mu      sync.Mutex
	windows map[string]rateWindow
}

func (l *machineRateLimiter) allow(credential, workspace string, now time.Time) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	minute := now.Unix() / 60
	for key, window := range l.windows {
		if window.minute != minute {
			delete(l.windows, key)
		}
	}
	if len(l.windows) > 10000 {
		return false
	}
	keys := []string{"c:" + credential, "w:" + workspace}
	limits := []int{120, 1200}
	for i, key := range keys {
		if l.windows[key].count >= limits[i] {
			return false
		}
	}
	for _, key := range keys {
		v := l.windows[key]
		v.minute = minute
		v.count++
		l.windows[key] = v
	}
	return true
}

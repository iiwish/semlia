package httpapi

import (
	"errors"
	app "github.com/iiwish/semlia/internal/application/webhooks"
	auth "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/webhooks"
	"github.com/iiwish/semlia/pkg/identity"
	"net/http"
)

func WithWebhooks(service *app.Service) Option { return func(h *Handler) { h.webhooks = service } }
func (h *Handler) routeWebhooks(w http.ResponseWriter, r *http.Request, trace string, route matchedRoute, workspace identity.WorkspaceID) string {
	w.Header().Set("Cache-Control", "no-store")
	if h.webhooks == nil {
		writeError(w, 503, "DEPENDENCY_UNAVAILABLE", "webhook signing is not configured", trace, true)
		return "DEPENDENCY_UNAVAILABLE"
	}
	if route.kind == routeWebhookReplay {
		var body struct {
			ExpectedAttempt int `json:"expectedAttempt"`
		}
		if decodeRequest(r, &body) != nil {
			return writeWebhookError(w, domain.ErrInvalidArgument, trace)
		}
		id, err := identity.Parse(identity.WebhookDelivery, route.query)
		if err != nil {
			return writeWebhookError(w, domain.ErrInvalidArgument, trace)
		}
		delivery, err := h.webhooks.Replay(r.Context(), workspace, id, body.ExpectedAttempt, principalRef(r), trace)
		if err != nil {
			return writeWebhookError(w, err, trace)
		}
		writeJSON(w, 200, delivery)
		return ""
	}
	if route.kind == routeWebhookDeliveries {
		items, err := h.webhooks.Deliveries(r.Context(), workspace, principalRef(r), trace)
		if err != nil {
			return writeWebhookError(w, err, trace)
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return ""
	}
	if r.Method == http.MethodGet {
		items, err := h.webhooks.List(r.Context(), workspace, principalRef(r), trace)
		if err != nil {
			return writeWebhookError(w, err, trace)
		}
		writeJSON(w, 200, map[string]any{"items": items})
		return ""
	}
	var body struct {
		Name            string   `json:"name"`
		Endpoint        string   `json:"endpoint"`
		Enabled         bool     `json:"enabled"`
		EventTypes      []string `json:"eventTypes"`
		ExpectedVersion int      `json:"expectedVersion"`
	}
	if decodeRequest(r, &body) != nil {
		return writeWebhookError(w, domain.ErrInvalidArgument, trace)
	}
	input := app.SaveRequest{WorkspaceID: workspace, Actor: principalRef(r), TraceID: trace, Name: body.Name, Endpoint: body.Endpoint, Enabled: body.Enabled, EventTypes: body.EventTypes, ExpectedVersion: body.ExpectedVersion, Rotate: route.kind == routeWebhookRotate}
	if route.kind != routeWebhooks {
		id, err := identity.Parse(identity.WebhookSubscription, route.query)
		if err != nil {
			return writeWebhookError(w, domain.ErrInvalidArgument, trace)
		}
		input.ID = id
	}
	issued, err := h.webhooks.Save(r.Context(), input)
	if err != nil {
		return writeWebhookError(w, err, trace)
	}
	status := 200
	if route.kind == routeWebhooks {
		status = 201
	}
	writeJSON(w, status, issued)
	return ""
}
func writeWebhookError(w http.ResponseWriter, err error, trace string) string {
	var denial *auth.DenialError
	if errors.As(err, &denial) {
		return writeDistributionError(w, err, trace)
	}
	code, status := "INTERNAL_ERROR", 500
	switch {
	case errors.Is(err, domain.ErrInvalidArgument):
		code, status = "INVALID_ARGUMENT", 400
	case errors.Is(err, domain.ErrNotFound):
		code, status = "NOT_FOUND", 404
	case errors.Is(err, domain.ErrConflict), errors.Is(err, auth.ErrVersionConflict):
		code, status = "CONFLICT", 409
	}
	writeError(w, status, code, "webhook operation could not be completed", trace, false)
	return code
}

package httpapi

import (
	"errors"
	"net/http"

	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/embedding"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	routeEmbeddingStatus routeKind = 4000 + iota
	routeEmbeddingCancel
	routeEmbeddingSearch
)

func (h *Handler) routeEmbedding(w http.ResponseWriter, r *http.Request, trace string, route matchedRoute) string {
	if h.embedding == nil {
		return writeEmbeddingError(w, domain.ErrNotConfigured, trace)
	}
	workspace, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeEmbeddingError(w, domain.ErrInvalid, trace)
	}
	switch route.kind {
	case routeEmbeddingStatus:
		if r.Method == http.MethodGet {
			result, err := h.embedding.Status(r.Context(), workspace, principalRef(r), trace)
			if err != nil {
				return writeEmbeddingError(w, err, trace)
			}
			writeJSON(w, http.StatusOK, result)
			return ""
		}
		result, err := h.embedding.Start(r.Context(), workspace, principalRef(r), trace, r.Header.Get("Idempotency-Key"))
		if err != nil {
			return writeEmbeddingError(w, err, trace)
		}
		writeJSON(w, http.StatusAccepted, result)
	case routeEmbeddingCancel:
		id, err := identity.ParseRunID(route.run)
		if err != nil {
			return writeEmbeddingError(w, domain.ErrInvalid, trace)
		}
		if err := h.embedding.Cancel(r.Context(), workspace, id, principalRef(r), trace); err != nil {
			return writeEmbeddingError(w, err, trace)
		}
		w.WriteHeader(http.StatusNoContent)
	case routeEmbeddingSearch:
		result, err := h.embedding.Search(r.Context(), workspace, principalRef(r), trace, r.URL.Query().Get("q"))
		if err != nil {
			return writeEmbeddingError(w, err, trace)
		}
		writeJSON(w, http.StatusOK, result)
	}
	return ""
}

func writeEmbeddingError(w http.ResponseWriter, err error, trace string) string {
	code, status, message := "EMBEDDING_UNAVAILABLE", http.StatusServiceUnavailable, "the embedding operation is unavailable"
	var denial *authorization.DenialError
	switch {
	case errors.As(err, &denial):
		code, status, message = "FORBIDDEN", http.StatusForbidden, "the acting principal lacks the required capability"
	case errors.Is(err, domain.ErrNoPublishedRelease):
		code, status, message = "EMBEDDING_NO_PUBLISHED_RELEASE", http.StatusConflict, "no published release is available to index"
	case errors.Is(err, domain.ErrNotConfigured):
		code, message = "EMBEDDING_NOT_CONFIGURED", "the embedding provider or vector store is not configured"
	case errors.Is(err, domain.ErrInvalid):
		code, status = "INVALID_ARGUMENT", http.StatusBadRequest
	case errors.Is(err, domain.ErrConflict):
		code, status = "VERSION_CONFLICT", http.StatusConflict
	case errors.Is(err, domain.ErrNotFound):
		code, status = "NOT_FOUND", http.StatusNotFound
	}
	writeError(w, status, code, message, trace, status == http.StatusServiceUnavailable)
	return code
}

package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionValidationCursor struct {
	Workspace string `json:"w"`
	Principal string `json:"p"`
	Operation string `json:"o"`
	Version   int    `json:"v"`
	Upper     int    `json:"u"`
	After     int    `json:"a"`
	Limit     int    `json:"l"`
}

func (handler *Handler) productionValidationHistory(response http.ResponseWriter, request *http.Request, trace string, w identity.WorkspaceID, op identity.ProductionOperationID) string {
	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", trace, false)
		return "UNAUTHENTICATED"
	}
	principal, err := identity.ParsePrincipalID(actor)
	if err != nil {
		return writeProductionError(response, domain.ErrInvalidArgument, trace)
	}
	query := request.URL.Query()
	for key, values := range query {
		if len(values) != 1 || (key != "version" && key != "limit" && key != "cursor") {
			return writeProductionError(response, domain.ErrInvalidArgument, trace)
		}
	}
	version, err := strconv.Atoi(query.Get("version"))
	if err != nil || version < 1 {
		return writeProductionError(response, domain.ErrInvalidArgument, trace)
	}
	limit := 50
	if query.Has("limit") {
		limit, err = strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > 200 {
			return writeProductionError(response, domain.ErrInvalidArgument, trace)
		}
	}
	cursor := productionValidationCursor{Workspace: w.String(), Principal: actor, Operation: op.String(), Version: version, Limit: limit}
	if query.Has("cursor") {
		invalid := func() string {
			writeError(response, http.StatusBadRequest, "INVALID_CURSOR", "validation history cursor is invalid for this query", trace, false)
			return "INVALID_CURSOR"
		}
		token := query.Get("cursor")
		if len(token) > 2048 {
			return invalid()
		}
		sealed, err := base64.RawURLEncoding.DecodeString(token)
		n := productionCursorCipher.NonceSize()
		if err != nil || len(sealed) < n+productionCursorCipher.Overhead() {
			return invalid()
		}
		raw, err := productionCursorCipher.Open(nil, sealed[:n], sealed[n:], []byte("production-validation-history/v1"))
		if err != nil {
			return invalid()
		}
		var decoded productionValidationCursor
		if json.Unmarshal(raw, &decoded) != nil || decoded.Workspace != cursor.Workspace || decoded.Principal != actor || decoded.Operation != cursor.Operation || decoded.Version != version || decoded.Limit != limit || decoded.Upper < 1 || decoded.Upper > 256 || decoded.After < 1 || decoded.After > decoded.Upper {
			return invalid()
		}
		cursor = decoded
	}
	versions, upper, more, err := handler.production.GetProductionValidationHistory(request.Context(), w, principal, op, version, limit, cursor.Upper, cursor.After)
	if err != nil {
		return writeProductionError(response, err, trace)
	}
	items := []ValidationAttemptResultResponse{}
	for _, ver := range versions {
		value, _ := productionValidationRecovery(ver)
		item, ok := value.(ValidationAttemptResultResponse)
		if !ok {
			return writeProductionError(response, domain.ErrPriorStateUnknown, trace)
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return writeProductionError(response, err, trace)
		}
		if len(raw) > 1<<20 {
			return writeProductionError(response, domain.ErrLimitExceeded, trace)
		}
		items = append(items, item)
	}
	var next *string
	if more && len(items) > 0 {
		cursor.Upper, cursor.After = upper, items[len(items)-1].AttemptNo
		raw, err := json.Marshal(cursor)
		if err != nil {
			return writeProductionError(response, err, trace)
		}
		nonce := make([]byte, productionCursorCipher.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return writeProductionError(response, err, trace)
		}
		token := base64.RawURLEncoding.EncodeToString(productionCursorCipher.Seal(nonce, nonce, raw, []byte("production-validation-history/v1")))
		next = &token
	}
	return writeProductionRecovery(response, ValidationAttemptPageResponse{OperationID: op.String(), Version: version, Items: items, NextCursor: next}, trace)
}

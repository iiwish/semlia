package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type productionBusinessRulePayload domain.ProductionBusinessRuleCommand

func (p *productionBusinessRulePayload) UnmarshalJSON(raw []byte) error {
	value, err := domain.ParseProductionJSON(raw)
	if err != nil {
		return err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return domain.ErrInvalidArgument
	}
	for _, key := range []string{"expectedVersion", "setDigest", "targetKey", "action"} {
		if v, exists := object[key]; !exists || v == nil {
			return domain.ErrInvalidArgument
		}
	}
	if object["action"] == "confirm" {
		evidence, hasEvidence := object["evidenceId"]
		declaration, hasDeclaration := object["declaration"]
		if hasEvidence == hasDeclaration || (hasEvidence && evidence == nil) || (hasDeclaration && declaration == nil) {
			return domain.ErrInvalidArgument
		}
	} else {
		if _, exists := object["evidenceId"]; exists {
			return domain.ErrInvalidArgument
		}
		if _, exists := object["declaration"]; exists {
			return domain.ErrInvalidArgument
		}
	}
	type plain productionBusinessRulePayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*plain)(p))
}

func (handler *Handler) productionBusinessRuleRequest(response http.ResponseWriter, request *http.Request, traceID string, w identity.WorkspaceID, route matchedRoute) string {
	if handler.production == nil {
		writeError(response, http.StatusServiceUnavailable, "FEATURE_UNAVAILABLE", "production is not configured", traceID, false)
		return "FEATURE_UNAVAILABLE"
	}
	principal, err := identity.ParsePrincipalID(principalRef(request))
	if err != nil {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	op, err := identity.ParseProductionOperationID(route.operation)
	if err != nil {
		return writeProductionError(response, domain.ErrInvalidArgument, traceID)
	}
	if request.Method == http.MethodGet {
		version, err := strconv.Atoi(request.URL.Query().Get("version"))
		if err != nil || version < 1 {
			return writeProductionError(response, domain.ErrInvalidArgument, traceID)
		}
		records, err := handler.production.BusinessRules(request.Context(), w, principal, op, version)
		if err != nil {
			return writeProductionError(response, err, traceID)
		}
		writeJSON(response, http.StatusOK, map[string]any{"items": records})
		return ""
	}
	var payload productionBusinessRulePayload
	if err := decodeStrictJSON(response, request, &payload); err != nil {
		return writeProductionError(response, err, traceID)
	}
	cmd := domain.ProductionBusinessRuleCommand(payload)
	cmd.WorkspaceID, cmd.OperationID, cmd.PrincipalID, cmd.IdempotencyKey = w, op, principal, strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	result, err := handler.production.RecordBusinessRule(request.Context(), cmd)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	writeJSON(response, status, result)
	return ""
}

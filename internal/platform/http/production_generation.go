package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func WithProductionGeneration(service *app.ProductionGenerationService) Option {
	return func(handler *Handler) { handler.productionGeneration = service }
}

type productionGenerationPayload domain.ProductionGenerationRequest

func (p *productionGenerationPayload) UnmarshalJSON(raw []byte) error {
	value, err := domain.ParseProductionJSON(raw)
	if err != nil {
		return err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return domain.ErrInvalidArgument
	}
	for _, key := range []string{"expectedVersion", "inputDigest", "modelSettingId", "modelConfigRevision", "instruction", "maxOutputTokens", "maxCostMicros"} {
		if v, exists := object[key]; !exists || v == nil {
			return domain.ErrInvalidArgument
		}
	}
	type plain productionGenerationPayload
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode((*plain)(p))
}

func (handler *Handler) productionGenerationRequest(response http.ResponseWriter, request *http.Request, traceID string, w identity.WorkspaceID, route matchedRoute) string {
	if handler.productionGeneration == nil {
		writeError(response, http.StatusServiceUnavailable, "FEATURE_UNAVAILABLE", "production generation is not configured", traceID, false)
		return "FEATURE_UNAVAILABLE"
	}
	actor := principalRef(request)
	if actor == "" {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required", traceID, false)
		return "UNAUTHENTICATED"
	}
	principal, err := identity.ParsePrincipalID(actor)
	if err != nil {
		return writeProductionError(response, domain.ErrInvalidArgument, traceID)
	}
	op, err := identity.ParseProductionOperationID(route.operation)
	if err != nil {
		return writeProductionError(response, domain.ErrInvalidArgument, traceID)
	}
	if route.kind == routeProductionGenerationRun {
		run, err := identity.ParseAgentRunID(route.run)
		if err != nil {
			return writeProductionError(response, domain.ErrInvalidArgument, traceID)
		}
		result, err := handler.productionGeneration.Get(request.Context(), w, principal, op, run)
		if err != nil {
			return writeProductionGenerationError(response, err, traceID)
		}
		return writeProductionRecovery(response, result, traceID)
	}
	var body productionGenerationPayload
	if err := decodeStrictJSON(response, request, &body); err != nil {
		return writeProductionError(response, err, traceID)
	}
	command := domain.ProductionGenerationRequest(body)
	command.WorkspaceID = w
	command.PrincipalID = principal
	command.OperationID = op
	command.IdempotencyKey = strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	command.TraceID = traceID
	result, err := handler.productionGeneration.Generate(request.Context(), command)
	if err != nil {
		return writeProductionGenerationError(response, err, traceID)
	}
	status := http.StatusAccepted
	if result.Replayed {
		status = http.StatusOK
	}
	response.Header().Set("Location", "/api/v1/workspaces/"+w.String()+"/production-operations/"+op.String()+"/generation/"+result.RunID.String())
	writeJSON(response, status, result)
	return ""
}

func writeProductionGenerationError(response http.ResponseWriter, err error, traceID string) string {
	if errors.Is(err, domain.ErrGenerationNotAuthorized) {
		writeError(response, http.StatusForbidden, "GENERATION_NOT_AUTHORIZED", "generation requires a current server-side model and spending grant", traceID, false)
		return "GENERATION_NOT_AUTHORIZED"
	}
	if errors.Is(err, app.ErrAIOutputInvalid) {
		writeError(response, http.StatusUnprocessableEntity, "AI_OUTPUT_INVALID", "suggestions violate the production contract", traceID, false)
		return "AI_OUTPUT_INVALID"
	}
	return writeProductionError(response, err, traceID)
}

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	distributionapp "github.com/iiwish/semlia/internal/application/distribution"
	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/distribution"
	governancedomain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

const (
	routeConsumers routeKind = iota + 200
	routeAsk
	routeConsumer
	routeConsumerBindings
	routeConsumerBinding
	routeSemanticResolve
	routeSemanticQuery
	routeResolvedSemanticPlan
	routeSemanticExecute
	routeQueryExecution
	routeQueryExecutionCancel
	routeClientCredentials
	routeClientCredentialRevoke
	routeClientCredentialRotate
	routeMachinePrincipals
	routeSemanticSearch
	routeSemanticDescribe
	routeMCP
	routeWebhooks
	routeWebhook
	routeWebhookRotate
	routeWebhookDeliveries
	routeWebhookReplay
)

func isDistributionRoute(kind routeKind) bool {
	return kind >= routeConsumers && kind <= routeWebhookReplay
}

func matchDistributionRoute(parts []string, base matchedRoute) (matchedRoute, bool) {
	if len(parts) < 5 {
		return matchedRoute{}, false
	}
	switch parts[4] {
	case "query-executions":
		if len(parts) == 5 {
			base.kind, base.label = routeSemanticExecute, "/api/v1/workspaces/{workspaceId}/query-executions"
		} else if len(parts) == 6 {
			base.kind, base.label, base.query = routeQueryExecution, "/api/v1/workspaces/{workspaceId}/query-executions/{runId}", parts[5]
			if strings.HasSuffix(parts[5], ":cancel") {
				base.kind, base.label, base.query = routeQueryExecutionCancel, "/api/v1/workspaces/{workspaceId}/query-executions/{runId}:cancel", strings.TrimSuffix(parts[5], ":cancel")
			}
		} else {
			return matchedRoute{}, false
		}
	case "webhooks":
		if len(parts) == 5 {
			base.kind, base.label = routeWebhooks, "/api/v1/workspaces/{workspaceId}/webhooks"
		} else if len(parts) == 6 {
			base.kind, base.label, base.query = routeWebhook, "/api/v1/workspaces/{workspaceId}/webhooks/{subscriptionId}", parts[5]
		} else if len(parts) == 7 && parts[6] == "rotate" {
			base.kind, base.label, base.query = routeWebhookRotate, "/api/v1/workspaces/{workspaceId}/webhooks/{subscriptionId}/rotate", parts[5]
		} else {
			return matchedRoute{}, false
		}
	case "webhook-deliveries":
		if len(parts) == 5 {
			base.kind, base.label = routeWebhookDeliveries, "/api/v1/workspaces/{workspaceId}/webhook-deliveries"
		} else if len(parts) == 7 && parts[6] == "replay" {
			base.kind, base.label, base.query = routeWebhookReplay, "/api/v1/workspaces/{workspaceId}/webhook-deliveries/{deliveryId}/replay", parts[5]
		} else {
			return matchedRoute{}, false
		}
	case "client-credentials":
		if len(parts) == 5 {
			base.kind, base.label = routeClientCredentials, "/api/v1/workspaces/{workspaceId}/client-credentials"
		} else if len(parts) == 7 && (parts[6] == "revoke" || parts[6] == "rotate") {
			base.query = parts[5]
			base.kind = routeClientCredentialRevoke
			if parts[6] == "rotate" {
				base.kind = routeClientCredentialRotate
			}
			base.label = "/api/v1/workspaces/{workspaceId}/client-credentials/{credentialId}/" + parts[6]
		} else {
			return matchedRoute{}, false
		}
	case "machine-principals":
		if len(parts) != 5 {
			return matchedRoute{}, false
		}
		base.kind, base.label = routeMachinePrincipals, "/api/v1/workspaces/{workspaceId}/machine-principals"
	case "semantic-search":
		if len(parts) != 5 {
			return matchedRoute{}, false
		}
		base.kind, base.label = routeSemanticSearch, "/api/v1/workspaces/{workspaceId}/semantic-search"
	case "semantic-describe":
		if len(parts) != 5 {
			return matchedRoute{}, false
		}
		base.kind, base.label = routeSemanticDescribe, "/api/v1/workspaces/{workspaceId}/semantic-describe"
	case "mcp":
		if len(parts) != 5 {
			return matchedRoute{}, false
		}
		base.kind, base.label = routeMCP, "/api/v1/workspaces/{workspaceId}/mcp"
	case "ask":
		if len(parts) != 5 {
			return matchedRoute{}, false
		}
		base.kind, base.label = routeAsk, "/api/v1/workspaces/{workspaceId}/ask"
	case "consumers":
		if len(parts) == 5 {
			base.kind, base.label = routeConsumers, "/api/v1/workspaces/{workspaceId}/consumers"
		} else if len(parts) == 6 {
			base.kind, base.label, base.consumer = routeConsumer, "/api/v1/workspaces/{workspaceId}/consumers/{consumerId}", parts[5]
		} else {
			return matchedRoute{}, false
		}
	case "consumer-bindings":
		if len(parts) == 5 {
			base.kind, base.label = routeConsumerBindings, "/api/v1/workspaces/{workspaceId}/consumer-bindings"
		} else if len(parts) == 6 {
			base.kind, base.label, base.binding = routeConsumerBinding, "/api/v1/workspaces/{workspaceId}/consumer-bindings/{bindingId}", parts[5]
		} else {
			return matchedRoute{}, false
		}
	case "semantic-queries:resolve":
		if len(parts) != 5 {
			return matchedRoute{}, false
		}
		base.kind, base.label = routeSemanticResolve, "/api/v1/workspaces/{workspaceId}/semantic-queries:resolve"
	case "semantic-queries":
		if len(parts) != 6 {
			return matchedRoute{}, false
		}
		base.kind, base.label, base.query = routeSemanticQuery, "/api/v1/workspaces/{workspaceId}/semantic-queries/{queryId}", parts[5]
	case "resolved-semantic-plans":
		if len(parts) != 6 {
			return matchedRoute{}, false
		}
		base.kind, base.label, base.plan = routeResolvedSemanticPlan, "/api/v1/workspaces/{workspaceId}/resolved-semantic-plans/{planId}", parts[5]
		if strings.HasSuffix(parts[5], ":execute") {
			base.kind, base.label, base.plan = routeSemanticExecute, "/api/v1/workspaces/{workspaceId}/resolved-semantic-plans/{planId}:execute", strings.TrimSuffix(parts[5], ":execute")
		}
	default:
		return matchedRoute{}, false
	}
	return base, true
}

func distributionRouteMethods(kind routeKind) []string {
	switch kind {
	case routeConsumers, routeConsumerBindings, routeClientCredentials, routeWebhooks:
		return []string{http.MethodGet, http.MethodPost}
	case routeConsumer, routeConsumerBinding:
		return []string{http.MethodGet, http.MethodPatch}
	case routeWebhook:
		return []string{http.MethodPatch}
	case routeWebhookRotate, routeWebhookReplay:
		return []string{http.MethodPost}
	case routeSemanticResolve, routeSemanticExecute, routeQueryExecutionCancel, routeAsk, routeClientCredentialRevoke, routeClientCredentialRotate, routeMachinePrincipals, routeSemanticDescribe:
		return []string{http.MethodPost}
	case routeMCP:
		return []string{http.MethodPost, http.MethodGet, http.MethodDelete}
	default:
		return []string{http.MethodGet}
	}
}

func (handler *Handler) routeDistribution(response http.ResponseWriter, request *http.Request, traceID string, route matchedRoute) string {
	workspaceID, err := identity.ParseWorkspaceID(route.workspace)
	if err != nil {
		return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
	}
	switch route.kind {
	case routeSemanticExecute, routeQueryExecution, routeQueryExecutionCancel:
		return handler.routeExecution(response, request, traceID, route, workspaceID)
	case routeWebhooks, routeWebhook, routeWebhookRotate, routeWebhookDeliveries, routeWebhookReplay:
		return handler.routeWebhooks(response, request, traceID, route, workspaceID)
	case routeClientCredentials, routeClientCredentialRevoke, routeClientCredentialRotate, routeMachinePrincipals:
		return handler.routeMachine(response, request, traceID, route, workspaceID)
	case routeMCP:
		if handler.mcp == nil {
			writeError(response, 503, "DEPENDENCY_UNAVAILABLE", "MCP is unavailable", traceID, true)
			return "DEPENDENCY_UNAVAILABLE"
		}
		request.Header.Set(headerTraceID, traceID)
		handler.mcp.ServeHTTP(response, request)
		return ""
	case routeSemanticSearch:
		items, searchErr := handler.distribution.Search(request.Context(), distributionapp.SearchRequest{WorkspaceID: workspaceID, PrincipalRef: principalRef(request), TraceID: traceID, Text: request.URL.Query().Get("q")})
		if searchErr != nil {
			return writeDistributionError(response, searchErr, traceID)
		}
		writeJSON(response, 200, map[string]any{"items": items})
		return ""
	case routeSemanticDescribe:
		var body resolveBody
		if decodeRequest(request, &body) != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		body.Query.Intent = domain.IntentDescribe
		result, err := handler.distribution.Resolve(request.Context(), distributionapp.ResolveRequest{WorkspaceID: workspaceID, Input: body.Query, Channel: canonicalHTTPChannel(body.Channel), IdempotencyKey: body.IdempotencyKey, PrincipalRef: principalRef(request), TraceID: traceID})
		if err != nil {
			return writeDistributionError(response, err, traceID)
		}
		writeJSON(response, 200, resolutionResponse(result))
		return ""
	case routeAsk:
		if handler.ask == nil {
			response.Header().Set("Retry-After", retryAfter)
			writeError(response, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "the Ask dependency is unavailable", traceID, true)
			return "DEPENDENCY_UNAVAILABLE"
		}
		var body askBody
		if decodeRequest(request, &body) != nil {
			return writeAskError(response, governancedomain.ErrInvalidArgument, traceID)
		}
		resolutionContext := body.Context
		if resolutionContext.Mode == "" {
			resolutionContext.Mode = domain.ResolutionCurrent
		}
		result, askErr := handler.ask.Ask(request.Context(), governanceapp.AskRequest{
			WorkspaceID: workspaceID, Question: body.Question, Context: resolutionContext,
			IdempotencyKey: body.IdempotencyKey, PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if askErr != nil {
			return writeAskError(response, askErr, traceID)
		}
		writeJSON(response, http.StatusOK, askResponse(result))
		return ""
	case routeConsumers:
		if request.Method == http.MethodGet {
			items, listErr := handler.distribution.ListConsumers(request.Context(), workspaceID, principalRef(request), traceID)
			if listErr != nil {
				return writeDistributionError(response, listErr, traceID)
			}
			writeJSON(response, http.StatusOK, map[string]any{"items": consumerResponses(items)})
			return ""
		}
		var body createConsumerBody
		if decodeRequest(request, &body) != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		item, createErr := handler.distribution.CreateConsumer(request.Context(), distributionapp.CreateConsumerRequest{
			WorkspaceID: workspaceID, StableKey: body.StableKey, Name: body.Name, Kind: body.Kind,
			OwnerPrincipalRef: body.OwnerPrincipalRef, Metadata: body.Metadata, PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if createErr != nil {
			return writeDistributionError(response, createErr, traceID)
		}
		writeJSON(response, http.StatusCreated, consumerResponse(item))
		return ""
	case routeConsumer:
		consumerID, parseErr := identity.ParseConsumerID(route.consumer)
		if parseErr != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		if request.Method == http.MethodGet {
			item, getErr := handler.distribution.GetConsumer(request.Context(), workspaceID, consumerID, principalRef(request), traceID)
			if getErr != nil {
				return writeDistributionError(response, getErr, traceID)
			}
			writeJSON(response, http.StatusOK, consumerResponse(item))
			return ""
		}
		var body updateConsumerBody
		if decodeRequest(request, &body) != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		item, updateErr := handler.distribution.UpdateConsumer(request.Context(), distributionapp.UpdateConsumerRequest{
			WorkspaceID: workspaceID, ConsumerID: consumerID, Name: body.Name, Status: domain.ConsumerStatus(body.Status),
			Metadata: body.Metadata, PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if updateErr != nil {
			return writeDistributionError(response, updateErr, traceID)
		}
		writeJSON(response, http.StatusOK, consumerResponse(item))
		return ""
	case routeConsumerBindings:
		if request.Method == http.MethodGet {
			items, listErr := handler.distribution.ListBindings(request.Context(), workspaceID, principalRef(request), traceID)
			if listErr != nil {
				return writeDistributionError(response, listErr, traceID)
			}
			writeJSON(response, http.StatusOK, map[string]any{"items": bindingResponses(items)})
			return ""
		}
		var body bindingBody
		if decodeRequest(request, &body) != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		item, createErr := handler.distribution.CreateBinding(request.Context(), distributionapp.CreateBindingRequest{
			WorkspaceID: workspaceID, ConsumerID: body.ConsumerID, Environment: body.Environment, Purpose: body.Purpose,
			Mode: domain.BindingMode(body.Mode), ReleaseID: body.ReleaseID, CompatibilityConstraint: body.CompatibilityConstraint,
			ExpiresAt: body.ExpiresAt, PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if createErr != nil {
			return writeDistributionError(response, createErr, traceID)
		}
		writeJSON(response, http.StatusCreated, bindingResponse(item))
		return ""
	case routeConsumerBinding:
		bindingID, parseErr := identity.ParseConsumerBindingID(route.binding)
		if parseErr != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		if request.Method == http.MethodGet {
			item, getErr := handler.distribution.GetBinding(request.Context(), workspaceID, bindingID, principalRef(request), traceID)
			if getErr != nil {
				return writeDistributionError(response, getErr, traceID)
			}
			writeJSON(response, http.StatusOK, bindingResponse(item))
			return ""
		}
		var body updateBindingBody
		if decodeRequest(request, &body) != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		item, updateErr := handler.distribution.UpdateBinding(request.Context(), distributionapp.UpdateBindingRequest{
			WorkspaceID: workspaceID, BindingID: bindingID, ExpectedVersion: body.ExpectedVersion,
			Purpose: body.Purpose, Mode: domain.BindingMode(body.Mode), ReleaseID: body.ReleaseID,
			CompatibilityConstraint: body.CompatibilityConstraint, ExpiresAt: body.ExpiresAt,
			Status: domain.BindingStatus(body.Status), PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if updateErr != nil {
			return writeDistributionError(response, updateErr, traceID)
		}
		writeJSON(response, http.StatusOK, bindingResponse(item))
		return ""
	case routeSemanticResolve:
		var body resolveBody
		if decodeRequest(request, &body) != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		result, resolveErr := handler.distribution.Resolve(request.Context(), distributionapp.ResolveRequest{
			WorkspaceID: workspaceID, Input: body.Query, Channel: canonicalHTTPChannel(body.Channel), IdempotencyKey: body.IdempotencyKey,
			PrincipalRef: principalRef(request), TraceID: traceID,
		})
		if resolveErr != nil {
			return writeDistributionError(response, resolveErr, traceID)
		}
		writeJSON(response, http.StatusOK, resolutionResponse(result))
		return ""
	case routeSemanticQuery:
		queryID, parseErr := identity.ParseSemanticQueryID(route.query)
		if parseErr != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		result, getErr := handler.distribution.GetQuery(request.Context(), workspaceID, queryID, principalRef(request), traceID)
		if getErr != nil {
			return writeDistributionError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, resolutionResponse(result))
		return ""
	case routeResolvedSemanticPlan:
		planID, parseErr := identity.ParseResolvedSemanticPlanID(route.plan)
		if parseErr != nil {
			return writeDistributionError(response, domain.ErrInvalidArgument, traceID)
		}
		plan, getErr := handler.distribution.GetPlan(request.Context(), workspaceID, planID, principalRef(request), traceID)
		if getErr != nil {
			return writeDistributionError(response, getErr, traceID)
		}
		writeJSON(response, http.StatusOK, distributionapp.PublicPlan(plan))
		return ""
	}
	panic("distribution route is not handled")
}

type createConsumerBody struct {
	StableKey         string          `json:"stableKey"`
	Name              string          `json:"name"`
	Kind              string          `json:"kind"`
	OwnerPrincipalRef string          `json:"ownerPrincipalRef"`
	Metadata          json.RawMessage `json:"metadata"`
}

type askBody struct {
	Question       string                   `json:"question"`
	Context        domain.ResolutionContext `json:"context"`
	IdempotencyKey string                   `json:"idempotencyKey"`
}

type updateConsumerBody struct {
	Name     string          `json:"name"`
	Status   string          `json:"status"`
	Metadata json.RawMessage `json:"metadata"`
}

type bindingBody struct {
	ConsumerID              identity.ConsumerID `json:"consumerId"`
	Environment             string              `json:"environment"`
	Purpose                 string              `json:"purpose"`
	Mode                    string              `json:"mode"`
	ReleaseID               *identity.ReleaseID `json:"releaseId"`
	CompatibilityConstraint json.RawMessage     `json:"compatibilityConstraint"`
	ExpiresAt               *time.Time          `json:"expiresAt"`
}

type updateBindingBody struct {
	ExpectedVersion         int                 `json:"expectedVersion"`
	Purpose                 string              `json:"purpose"`
	Mode                    string              `json:"mode"`
	ReleaseID               *identity.ReleaseID `json:"releaseId"`
	CompatibilityConstraint json.RawMessage     `json:"compatibilityConstraint"`
	ExpiresAt               *time.Time          `json:"expiresAt"`
	Status                  string              `json:"status"`
}

type resolveBody struct {
	Query          domain.SemanticQueryInput `json:"query"`
	Channel        string                    `json:"channel"`
	IdempotencyKey string                    `json:"idempotencyKey"`
}

type consumerDTO struct {
	ID                string          `json:"id"`
	StableKey         string          `json:"stableKey"`
	Name              string          `json:"name"`
	Kind              string          `json:"kind"`
	Status            string          `json:"status"`
	OwnerPrincipalRef string          `json:"ownerPrincipalRef"`
	Metadata          json.RawMessage `json:"metadata"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

func consumerResponse(value domain.Consumer) consumerDTO {
	return consumerDTO{ID: value.ID.String(), StableKey: value.StableKey, Name: value.Name, Kind: value.Kind,
		Status: string(value.Status), OwnerPrincipalRef: value.OwnerPrincipalRef, Metadata: value.Metadata,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func consumerResponses(values []domain.Consumer) []consumerDTO {
	items := make([]consumerDTO, 0, len(values))
	for _, value := range values {
		items = append(items, consumerResponse(value))
	}
	return items
}

type bindingDTO struct {
	ID                      string              `json:"id"`
	ConsumerID              string              `json:"consumerId"`
	Environment             string              `json:"environment"`
	Purpose                 string              `json:"purpose"`
	Mode                    string              `json:"mode"`
	ReleaseID               *identity.ReleaseID `json:"releaseId,omitempty"`
	CompatibilityConstraint json.RawMessage     `json:"compatibilityConstraint"`
	ExpiresAt               *time.Time          `json:"expiresAt,omitempty"`
	Status                  string              `json:"status"`
	Version                 int                 `json:"version"`
	CreatedAt               time.Time           `json:"createdAt"`
	UpdatedAt               time.Time           `json:"updatedAt"`
}

func bindingResponse(value domain.ConsumerBinding) bindingDTO {
	return bindingDTO{ID: value.ID.String(), ConsumerID: value.ConsumerID.String(), Environment: value.Environment,
		Purpose: value.Purpose, Mode: string(value.Mode), ReleaseID: value.ReleaseID,
		CompatibilityConstraint: value.CompatibilityConstraint, ExpiresAt: value.ExpiresAt,
		Status: string(value.Status), Version: value.Version, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func bindingResponses(values []domain.ConsumerBinding) []bindingDTO {
	items := make([]bindingDTO, 0, len(values))
	for _, value := range values {
		items = append(items, bindingResponse(value))
	}
	return items
}

func resolutionResponse(result distributionapp.ResolutionResult) map[string]any {
	return distributionapp.Response(result)
}

func canonicalHTTPChannel(value string) string {
	switch value {
	case "cli", "sdk":
		return value
	default:
		return "api"
	}
}

func writeDistributionError(response http.ResponseWriter, err error, traceID string) string {
	var denial *authorization.DenialError
	var validation *domain.ValidationError
	switch {
	case errors.As(err, &denial):
		code := string(denial.Decision.ReasonCode)
		writeError(response, http.StatusForbidden, code, "the principal is not authorized for this operation", traceID, false)
		return code
	case errors.As(err, &validation):
		writeError(response, http.StatusBadRequest, string(validation.Code), validation.Message, traceID, false)
		return string(validation.Code)
	case errors.Is(err, domain.ErrInvalidArgument):
		writeError(response, http.StatusBadRequest, "INVALID_ARGUMENT", "the request is invalid", traceID, false)
		return "INVALID_ARGUMENT"
	case errors.Is(err, domain.ErrNotFound):
		writeError(response, http.StatusNotFound, "NOT_FOUND", "the requested resource was not found", traceID, false)
		return "NOT_FOUND"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrInvariant):
		writeError(response, http.StatusConflict, "CONFLICT", "the operation conflicts with current state", traceID, false)
		return "CONFLICT"
	default:
		writeError(response, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed", traceID, false)
		return "INTERNAL_ERROR"
	}
}

func writeAskError(response http.ResponseWriter, err error, traceID string) string {
	var validation *domain.ValidationError
	if errors.As(err, &validation) || errors.Is(err, domain.ErrInvalidArgument) ||
		errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrInvariant) {
		return writeDistributionError(response, err, traceID)
	}
	return writeGovernanceError(response, err, traceID)
}

func askResponse(result governanceapp.AskResult) map[string]any {
	response := map[string]any{
		"agentRun":       governanceAgentRunResponse(result.Run),
		"interpretation": result.Interpretation,
		"definitions":    []any{},
	}
	if result.Resolution == nil {
		return response
	}
	response["resolution"] = resolutionResponse(*result.Resolution)
	definitions := make([]map[string]any, 0, len(result.Resolution.Definitions))
	for _, definition := range result.Resolution.Definitions {
		definitions = append(definitions, map[string]any{
			"assetId": definition.AssetID, "revisionId": definition.RevisionID,
			"address": definition.Address, "assetType": definition.AssetType,
			"name": definition.Name, "definition": definition.Definition,
			"contentDigest": definition.ContentDigest,
		})
	}
	response["definitions"] = definitions
	return response
}

package distribution

import domain "github.com/iiwish/semlia/internal/domain/distribution"

// PublicPlan keeps server-owned physical locators outside resolve/read scope.
// Its digest is opaque: clients do not reconstruct the private compiler input.
func PublicPlan(plan domain.ResolvedSemanticPlan) domain.ResolvedSemanticPlan {
	plan.Execution = nil
	return plan
}

// Response is the canonical public projection used by REST and MCP. CLI and
// SDK preserve that REST response without transport-specific plan generation.
func Response(result ResolutionResult) map[string]any {
	definitions := result.Definitions
	if definitions == nil {
		definitions = []DefinitionSummary{}
	}
	response := map[string]any{"id": result.Query.ID.String(), "schemaVersion": result.Query.SchemaVersion, "resolverVersion": result.Query.ResolverVersion, "requestDigest": result.Query.RequestDigest, "outcome": result.Query.Outcome, "channel": result.Query.Channel, "createdAt": result.Query.CreatedAt, "validation": result.Validation, "definitions": definitions}
	if !result.Query.SelectedReleaseID.IsZero() {
		response["releaseId"] = result.Query.SelectedReleaseID.String()
	}
	if result.Query.ConsumerID != nil {
		response["consumerId"] = result.Query.ConsumerID.String()
	}
	if result.Query.BindingID != nil {
		response["bindingId"] = result.Query.BindingID.String()
	}
	if result.Plan != nil {
		response["plan"] = PublicPlan(*result.Plan)
	}
	if result.Refusal != nil {
		ids := []string{}
		for _, id := range result.Refusal.CandidateIDs {
			ids = append(ids, id.String())
		}
		response["refusal"] = map[string]any{"code": result.Refusal.Code, "candidateIds": ids, "clarification": result.Refusal.Clarification, "details": result.Refusal.Details}
	}
	return response
}

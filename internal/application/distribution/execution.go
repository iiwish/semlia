package distribution

import (
	"context"
	"encoding/json"

	"github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/distribution"
	"github.com/iiwish/semlia/pkg/identity"
)

// ExecutionAccess repeats current authorization without release freshness.
// Existing runs remain cancellable when a current binding advances releases.
func (s *Service) ExecutionAccess(ctx context.Context, w identity.WorkspaceID, id identity.ResolvedSemanticPlanID, principal, trace string) (ResolutionResult, error) {
	if _, err := s.authorize(ctx, w, authorization.ActionSemanticExecute, authorization.Resource{Type: authorization.ScopeWorkspace, ID: w.UUID()}, principal, trace); err != nil {
		return ResolutionResult{}, err
	}
	plan, err := s.repository.GetResolvedSemanticPlan(ctx, w, id)
	if err != nil {
		return ResolutionResult{}, err
	}
	result, err := s.repository.GetSemanticQuery(ctx, w, plan.QueryID)
	if err != nil {
		return ResolutionResult{}, err
	}
	if result.Plan == nil || result.Plan.ID != id || result.Plan.PlanDigest != plan.PlanDigest || result.Validation.Status != "passed" || result.Validation.InputDigest != plan.PlanDigest {
		return ResolutionResult{}, domain.ErrInvariant
	}
	if err := s.checkStoredAccessWithFreshness(ctx, w, result, principal, trace, false); err != nil {
		return ResolutionResult{}, err
	}
	for _, asset := range plan.Assets {
		if _, err := s.authorize(ctx, w, authorization.ActionSemanticExecute, authorization.Resource{Type: authorization.ScopeAsset, ID: asset.AssetID.UUID()}, principal, trace); err != nil {
			return ResolutionResult{}, err
		}
	}
	return result, nil
}

func (s *Service) ExecutionPlan(ctx context.Context, w identity.WorkspaceID, id identity.ResolvedSemanticPlanID, principal, trace string) (ResolutionResult, error) {
	result, err := s.ExecutionAccess(ctx, w, id, principal, trace)
	if err != nil {
		return result, err
	}
	plan := *result.Plan
	var query domain.SemanticQueryInput
	if json.Unmarshal(result.Query.CanonicalRequest, &query) != nil {
		return ResolutionResult{}, domain.ErrInvariant
	}
	decision, err := s.authorize(ctx, w, authorization.ActionSemanticExecute, authorization.Resource{Type: authorization.ScopeWorkspace, ID: w.UUID()}, principal, trace)
	if err != nil {
		return ResolutionResult{}, err
	}
	snapshot, _, _, refusal, err := s.selectSnapshot(ctx, ResolveRequest{WorkspaceID: w, PrincipalRef: principal, TraceID: trace, Input: query}, decision, s.clock.Now())
	if err != nil {
		return ResolutionResult{}, err
	}
	if refusal != nil || snapshot.ReleaseID != plan.ReleaseID {
		return ResolutionResult{}, domain.ErrConflict
	}
	for _, asset := range plan.Assets {
		if _, err := s.authorize(ctx, w, authorization.ActionSemanticExecute, authorization.Resource{Type: authorization.ScopeAsset, ID: asset.AssetID.UUID()}, principal, trace); err != nil {
			return ResolutionResult{}, err
		}
	}
	// Rebuild from the exact frozen release to reject a tampered canonical plan,
	// even if an attacker also replaced its stored digest.
	selected := []domain.ReleasedAsset{}
	for _, asset := range plan.Assets {
		for _, released := range snapshot.Assets {
			if released.AssetID == asset.AssetID && released.RevisionID == asset.RevisionID {
				selected = append(selected, released)
			}
		}
	}
	rebuilt, refusal, err := domain.BuildPlan(query, snapshot, plan.QueryID, plan.ID, selected, plan.CreatedAt)
	if err != nil {
		return ResolutionResult{}, err
	}
	if refusal != nil || plan.Execution == nil || domain.PlanDigest(plan) != plan.PlanDigest || rebuilt.PlanDigest != plan.PlanDigest {
		return ResolutionResult{}, domain.ErrInvariant
	}
	return result, nil
}

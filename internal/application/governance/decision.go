package governance

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// PolicyDecisionTrigger is the §8.2 decision step of the governance pipeline:
// when a proposal reaches in_review it computes the closed DecisionInputs
// from persisted state, evaluates the current rule version through the
// PolicyService and persists exactly one policy_decisions row for the
// (proposal, rule_version, inputs_digest) triple. The trigger is idempotent —
// re-executions over unchanged state return the stored decision instead of
// appending rows.
type PolicyDecisionTrigger struct {
	facts      DecisionFactsRepository
	collectors *InputsCollectorRegistry
	policy     *PolicyService
}

func NewPolicyDecisionTrigger(
	facts DecisionFactsRepository,
	policy *PolicyService,
) *PolicyDecisionTrigger {
	if facts == nil || policy == nil {
		panic("policy decision trigger requires the decision facts repository and policy service")
	}
	return &PolicyDecisionTrigger{
		facts: facts, collectors: NewInputsCollectorRegistry(facts), policy: policy,
	}
}

// EnsureDecisionForProposal computes and persists the decision for one
// in_review proposal. Proposals outside in_review are skipped (no decision,
// no error) so a concurrently rejected proposal never gains one.
func (trigger *PolicyDecisionTrigger) EnsureDecisionForProposal(
	ctx context.Context,
	workspace identity.WorkspaceID,
	proposalID identity.ProposalID,
) (governance.PolicyDecision, error) {
	proposal, err := trigger.facts.GetProposal(ctx, workspace, proposalID)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	if proposal.State != governance.ProposalInReview {
		return governance.PolicyDecision{}, nil
	}
	changes, err := trigger.facts.ListProposalChanges(ctx, workspace, proposalID)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	outcomes, err := trigger.facts.SummarizeProposalValidationOutcomes(ctx, workspace, proposalID)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	collector, err := trigger.collectors.For(proposal.TargetObjectType)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	target, err := collector.CollectTargetInputs(ctx, workspace, proposal)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	inputs, err := BuildDecisionInputs(proposal, changes, outcomes, target)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	encoded, err := json.Marshal(inputs)
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("encode decision inputs: %w", err)
	}
	decision, err := trigger.policy.EnsureDecision(ctx, DecideRequest{
		WorkspaceID: workspace, ProposalID: proposalID,
		RuleVersion: governance.RiskRuleVersion, Inputs: encoded,
	})
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	return decision, nil
}

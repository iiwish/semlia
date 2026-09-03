package governance_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestReviewCommandDecisionOutcomeSharesOneRule(t *testing.T) {
	approved, approvedTransition, err := governanceapp.ReviewCommandApprove.Outcome()
	if err != nil {
		t.Fatal(err)
	}
	if approved != governance.ReviewApproved || approvedTransition != nil {
		t.Fatalf("approve outcome = %s/%v, want an approved review with no transition", approved, approvedTransition)
	}
	rejected, rejectedTransition, err := governanceapp.ReviewCommandReject.Outcome()
	if err != nil {
		t.Fatal(err)
	}
	if rejected != governance.ReviewRejected || rejectedTransition == nil ||
		*rejectedTransition != governance.ProposalRejected {
		t.Fatalf("reject outcome = %s/%v, want a rejected review with the rejected transition", rejected, rejectedTransition)
	}
	if _, _, err := governanceapp.ReviewCommandDecision("defer").Outcome(); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("unknown decision error = %v, want ErrInvalidArgument", err)
	}
}

func TestDominantDiffCategoryPicksTheMostRiskBearingFlag(t *testing.T) {
	tests := []struct {
		name   string
		inputs governance.DecisionInputs
		want   governance.DiffCategory
	}{
		{"definition only", governance.DecisionInputs{AffectsDefinition: true}, governance.DiffCategoryDefinition},
		{"computation dominates definition", governance.DecisionInputs{
			AffectsComputation: true, AffectsDefinition: true,
		}, governance.DiffCategoryComputation},
		{"access dominates contract", governance.DecisionInputs{
			AffectsAccess: true, AffectsContract: true, AffectsDefinition: true,
		}, governance.DiffCategoryAccess},
		{"contract dominates relations", governance.DecisionInputs{
			AffectsContract: true, AffectsRelations: true, AffectsDefinition: true,
		}, governance.DiffCategoryContract},
		{"relations dominates definition", governance.DecisionInputs{
			AffectsRelations: true, AffectsDefinition: true,
		}, governance.DiffCategoryRelations},
		{"no flags falls back to definition", governance.DecisionInputs{}, governance.DiffCategoryDefinition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := governance.DominantDiffCategory(test.inputs); got != test.want {
				t.Fatalf("dominant category = %q, want %q", got, test.want)
			}
		})
	}
}

func eligibleProposal(t *testing.T, workspace identity.WorkspaceID, decision governance.PolicyDecision) governanceapp.BatchEligibleProposal {
	t.Helper()
	if decision.Inputs == nil {
		raw, err := json.Marshal(governance.DecisionInputs{
			AssetType: "metric", TargetObjectType: "semantic_asset", AffectsDefinition: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		decision.Inputs = raw
	}
	proposalID := mustProposalID(t)
	decision.ID = mustPolicyDecisionID(t)
	decision.ProposalID = proposalID
	decision.WorkspaceID = workspace
	return governanceapp.BatchEligibleProposal{
		Proposal: governance.Proposal{
			ID: proposalID, WorkspaceID: workspace, TargetObjectType: governance.TargetSemanticAsset,
			State: governance.ProposalInReview,
		},
		Decision: decision,
	}
}

func mustProposalID(t *testing.T) identity.ProposalID {
	t.Helper()
	id, err := identity.NewProposalID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustPolicyDecisionID(t *testing.T) identity.PolicyDecisionID {
	t.Helper()
	id, err := identity.NewPolicyDecisionID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAssembleReviewBatchesClustersDeterministically(t *testing.T) {
	workspace := mustWorkspaceID(t)
	lowRisk := governance.PolicyDecision{
		MatchedPolicy: "semlia.risk.v1/low-risk", RiskLevel: governance.RiskLow,
		Routing: governance.RoutingBatch, ReasonCode: governance.ReasonRiskLowBatch,
		RuleVersion: "1.0", InputsDigest: "sha256:" + repeat('a', 64),
	}
	expertRisk := governance.PolicyDecision{
		MatchedPolicy: "semlia.risk.v1/computation", RiskLevel: governance.RiskMedium,
		Routing: governance.RoutingExpert, ReasonCode: governance.ReasonRiskComputationChange,
		RuleVersion: "1.0", InputsDigest: "sha256:" + repeat('b', 64),
	}
	eligible := []governanceapp.BatchEligibleProposal{
		eligibleProposal(t, workspace, lowRisk),
		eligibleProposal(t, workspace, expertRisk),
	}

	assemblies, err := governanceapp.AssembleReviewBatches(eligible, "prn_reviewer", time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC), "test-trace")
	if err != nil {
		t.Fatal(err)
	}
	if len(assemblies) != 1 {
		t.Fatalf("assemblies = %d, want exactly the low-risk batch cluster", len(assemblies))
	}
	assembly := assemblies[0]
	if len(assembly.Members) != 1 {
		t.Fatalf("member count = %d, want the single batch-routed proposal", len(assembly.Members))
	}
	if assembly.Batch.Status != governance.ReviewBatchOpen ||
		assembly.Batch.PolicyVersion != "1.0" || assembly.Batch.CreatedBy != "prn_reviewer" {
		t.Fatalf("batch = %+v, want an open 1.0 batch created by the reviewer", assembly.Batch)
	}
	if assembly.Batch.GroupingRule.MatchedRuleID != "semlia.risk.v1/low-risk" ||
		assembly.Batch.GroupingRule.DiffCategory != governance.DiffCategoryDefinition ||
		assembly.Batch.GroupingRule.TargetObjectType != governance.TargetSemanticAsset {
		t.Fatalf("grouping rule = %+v", assembly.Batch.GroupingRule)
	}
	reason, err := governance.ParseReviewAddedReason(assembly.Members[0].AddedReason)
	if err != nil {
		t.Fatal(err)
	}
	if reason.MatchedRuleID != "semlia.risk.v1/low-risk" || reason.RiskLevel != governance.RiskLow ||
		reason.ReasonCode != governance.ReasonRiskLowBatch || reason.RuleVersion != "1.0" ||
		reason.InputsDigest != lowRisk.InputsDigest {
		t.Fatalf("added reason snapshot = %+v, want the frozen decision", reason)
	}
	if !assembly.Members[0].Sample {
		t.Fatal("the single member of a cluster must be its representative sample")
	}
	if _, err := identity.ParseReviewBatchID(assembly.Batch.ID.String()); err != nil {
		t.Fatalf("batch id is not a review batch TypeID: %v", err)
	}
}

func TestAssembleReviewBatchesSeparatesClustersAndCapsSamples(t *testing.T) {
	workspace := mustWorkspaceID(t)
	lowRisk := governance.PolicyDecision{
		MatchedPolicy: "semlia.risk.v1/low-risk", RiskLevel: governance.RiskLow,
		Routing: governance.RoutingBatch, ReasonCode: governance.ReasonRiskLowBatch,
		RuleVersion: "1.0", InputsDigest: "sha256:" + repeat('a', 64),
	}
	var eligible []governanceapp.BatchEligibleProposal
	for index := 0; index < 5; index++ {
		eligible = append(eligible, eligibleProposal(t, workspace, lowRisk))
	}
	grainCandidate := eligibleProposal(t, workspace, lowRisk)
	grainCandidate.Proposal.TargetObjectType = governance.TargetModelGrain
	eligible = append(eligible, grainCandidate)

	assemblies, err := governanceapp.AssembleReviewBatches(eligible, "prn_reviewer", time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC), "test-trace")
	if err != nil {
		t.Fatal(err)
	}
	if len(assemblies) != 2 {
		t.Fatalf("assemblies = %d, want one cluster per target object type", len(assemblies))
	}
	first, second := assemblies[0], assemblies[1]
	if !(first.Batch.GroupingRule.TargetObjectType < second.Batch.GroupingRule.TargetObjectType) {
		t.Fatalf("clusters are not ordered by grouping rule: %+v vs %+v",
			first.Batch.GroupingRule, second.Batch.GroupingRule)
	}
	if first.Batch.GroupingRule.TargetObjectType != governance.TargetModelGrain ||
		second.Batch.GroupingRule.TargetObjectType != governance.TargetSemanticAsset {
		t.Fatalf("cluster targets = %+v / %+v, want model_grain then semantic_asset",
			first.Batch.GroupingRule, second.Batch.GroupingRule)
	}
	var sampled int
	for _, member := range second.Members {
		if member.Sample {
			sampled++
		}
	}
	if len(second.Members) != 5 || sampled != governance.SampleCount {
		t.Fatalf("members=%d samples=%d, want 5 members with exactly %d samples",
			len(second.Members), sampled, governance.SampleCount)
	}
	if len(first.Members) != 1 || !first.Members[0].Sample {
		t.Fatalf("small cluster must still mark its single sample: %+v", first.Members)
	}
}

func TestDecideBatchMemberOutcomeSplitsEscalations(t *testing.T) {
	workspace := mustWorkspaceID(t)
	batchDecision := governance.PolicyDecision{
		MatchedPolicy: "semlia.risk.v1/low-risk", RiskLevel: governance.RiskLow,
		Routing: governance.RoutingBatch, ReasonCode: governance.ReasonRiskLowBatch,
	}
	inReview := governance.Proposal{State: governance.ProposalInReview}
	tests := []struct {
		name       string
		state      governance.ProposalState
		decision   *governance.PolicyDecision
		wantSplit  bool
		wantReason string
	}{
		{"stays batch eligible", governance.ProposalInReview, &batchDecision, false, ""},
		{"rejected proposal splits", governance.ProposalRejected, &batchDecision, true, "state_changed:rejected"},
		{"missing decision splits", governance.ProposalInReview, nil, true, "decision_missing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			proposal := inReview
			proposal.State = test.state
			outcome, err := governanceapp.DecideBatchMemberOutcome(governanceapp.ConfirmMemberState{
				Proposal: proposal, LatestDecision: test.decision,
			}, "prn_reviewer", workspace)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Split != test.wantSplit || outcome.SplitReason != test.wantReason {
				t.Fatalf("outcome = %+v, want split=%t reason=%q", outcome, test.wantSplit, test.wantReason)
			}
		})
	}
}

func TestDecideBatchMemberOutcomeSplitsHighRiskAndRoutingEscalations(t *testing.T) {
	workspace := mustWorkspaceID(t)
	highRisk := governance.PolicyDecision{
		MatchedPolicy: "semlia.risk.v1/blockers", RiskLevel: governance.RiskHigh,
		Routing: governance.RoutingExpert, ReasonCode: governance.ReasonRiskBlocker,
	}
	outcome, err := governanceapp.DecideBatchMemberOutcome(governanceapp.ConfirmMemberState{
		Proposal:       governance.Proposal{State: governance.ProposalInReview},
		LatestDecision: &highRisk,
	}, "prn_reviewer", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Split || outcome.SplitReason != "risk_escalated:high:RISK_BLOCKER" {
		t.Fatalf("high-risk outcome = %+v, want the escalation split reason", outcome)
	}

	routingEscalation := governance.PolicyDecision{
		MatchedPolicy: "semlia.risk.v1/computation", RiskLevel: governance.RiskMedium,
		Routing: governance.RoutingExpert, ReasonCode: governance.ReasonRiskComputationChange,
	}
	outcome, err = governanceapp.DecideBatchMemberOutcome(governanceapp.ConfirmMemberState{
		Proposal:       governance.Proposal{State: governance.ProposalInReview},
		LatestDecision: &routingEscalation,
	}, "prn_reviewer", workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Split || outcome.SplitReason != "routing_changed:expert" {
		t.Fatalf("routing outcome = %+v, want the routing split reason", outcome)
	}
}

func TestDecideBatchMemberOutcomeRefusesReviewerAuthoredMember(t *testing.T) {
	workspace := mustWorkspaceID(t)
	proposal := governance.Proposal{State: governance.ProposalInReview, CreatedBy: "prn_author"}
	batchDecision := governance.PolicyDecision{
		MatchedPolicy: "semlia.risk.v1/low-risk", RiskLevel: governance.RiskLow,
		Routing: governance.RoutingBatch, ReasonCode: governance.ReasonRiskLowBatch,
	}
	_, err := governanceapp.DecideBatchMemberOutcome(governanceapp.ConfirmMemberState{
		Proposal: proposal, LatestDecision: &batchDecision,
	}, "prn_author", workspace)
	var denial *governanceapp.SeparationOfDutyError
	if !errors.As(err, &denial) {
		t.Fatalf("author confirm error = %v, want SeparationOfDutyError", err)
	}
	if len(denial.Conflicted) != 1 || denial.Conflicted[0] != proposal.ID {
		t.Fatalf("conflicting members = %v, want the authored member named", denial.Conflicted)
	}
	if denial.Conflict == "" || denial.Scope != workspace.String() || denial.Source == "" || len(denial.Recovery) == 0 {
		t.Fatalf("FR-007 denial must name conflict, scope, policy source and recovery: %+v", denial)
	}
}

func TestSeparationOfDutyErrorCarriesStableExplainabilityFields(t *testing.T) {
	workspace := mustWorkspaceID(t)
	denial := governanceapp.NewAuthorSeparationOfDutyError(workspace)
	if denial.Conflict == "" || denial.Scope != workspace.String() {
		t.Fatalf("denial = %+v, want conflict and workspace scope", denial)
	}
	if denial.Source != governanceapp.SeparationOfDutyPolicySource || len(denial.Recovery) == 0 {
		t.Fatalf("denial = %+v, want the stable policy source and recovery actions", denial)
	}
}

func mustWorkspaceID(t *testing.T) identity.WorkspaceID {
	t.Helper()
	id, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func repeat(value byte, count int) string {
	out := make([]byte, count)
	for index := range out {
		out[index] = value
	}
	return string(out)
}

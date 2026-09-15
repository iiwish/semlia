package governance_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

func TestReviewGroupingRuleValidateAcceptsOnlyClosedVocabularies(t *testing.T) {
	valid := governance.ReviewGroupingRule{
		TargetObjectType: governance.TargetSemanticAsset,
		DiffCategory:     governance.DiffCategoryDefinition,
		MatchedRuleID:    "semlia.risk.v1/low-risk",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid grouping rule rejected: %v", err)
	}
	for targetType := range map[governance.TargetObjectType]bool{
		governance.TargetModelGrain: true, governance.TargetEntityKey: true, governance.TargetJoinContract: true,
	} {
		rule := valid
		rule.TargetObjectType = targetType
		if err := rule.Validate(); err != nil {
			t.Fatalf("target type %s rejected: %v", targetType, err)
		}
	}
	invalid := []governance.ReviewGroupingRule{
		{TargetObjectType: "dashboard", DiffCategory: governance.DiffCategoryDefinition, MatchedRuleID: "rule"},
		{TargetObjectType: governance.TargetSemanticAsset, DiffCategory: "vibes", MatchedRuleID: "rule"},
		{TargetObjectType: governance.TargetSemanticAsset, DiffCategory: governance.DiffCategoryDefinition},
		{TargetObjectType: governance.TargetSemanticAsset, DiffCategory: governance.DiffCategoryDefinition, MatchedRuleID: overload(129)},
	}
	for index, rule := range invalid {
		if err := rule.Validate(); !errors.Is(err, governance.ErrInvalidArgument) {
			t.Fatalf("invalid rule %d accepted: %+v (%v)", index, rule, err)
		}
	}
}

func TestReviewBatchMemberValidate(t *testing.T) {
	reason, err := json.Marshal(governance.ReviewAddedReason{
		MatchedRuleID: "semlia.risk.v1/low-risk", RiskLevel: governance.RiskLow,
		ReasonCode: governance.ReasonRiskLowBatch, RuleVersion: "1.0",
		InputsDigest: "sha256:" + repeat('a', 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	approved := governance.ReviewApproved
	splitReason := "risk_escalated:high:RISK_BLOCKER"
	valid := governance.ReviewBatchMember{
		BatchID: mustReviewBatchID(t), WorkspaceID: mustWorkspaceID(t), ProposalID: mustProposalIDFor(t),
		AddedReason: reason,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid member rejected: %v", err)
	}
	decided := valid
	decided.Decision = &approved
	if err := decided.Validate(); err != nil {
		t.Fatalf("decided member rejected: %v", err)
	}
	split := valid
	split.SplitOut = true
	split.SplitReason = &splitReason
	if err := split.Validate(); err != nil {
		t.Fatalf("split member rejected: %v", err)
	}
	invalid := []governance.ReviewBatchMember{
		{AddedReason: nil},
		{BatchID: valid.BatchID, WorkspaceID: valid.WorkspaceID, ProposalID: valid.ProposalID, AddedReason: json.RawMessage(`{}`)},
		{BatchID: valid.BatchID, WorkspaceID: valid.WorkspaceID, ProposalID: valid.ProposalID,
			AddedReason: reason, Decision: &approved, SplitOut: true, SplitReason: &splitReason},
		{BatchID: valid.BatchID, WorkspaceID: valid.WorkspaceID, ProposalID: valid.ProposalID, AddedReason: reason, SplitOut: true},
	}
	for index, member := range invalid {
		if err := member.Validate(); err == nil {
			t.Fatalf("invalid member %d accepted: %+v", index, member)
		}
	}
}

func TestParseReviewAddedReasonRejectsIncompleteSnapshots(t *testing.T) {
	complete, err := json.Marshal(governance.ReviewAddedReason{
		MatchedRuleID: "semlia.risk.v1/low-risk", RiskLevel: governance.RiskLow,
		ReasonCode: governance.ReasonRiskLowBatch, RuleVersion: "1.0",
		InputsDigest: "sha256:" + repeat('a', 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := governance.ParseReviewAddedReason(complete)
	if err != nil {
		t.Fatalf("complete snapshot rejected: %v", err)
	}
	if parsed.MatchedRuleID != "semlia.risk.v1/low-risk" || parsed.RiskLevel != governance.RiskLow {
		t.Fatalf("parsed snapshot = %+v", parsed)
	}
	if _, err := governance.ParseReviewAddedReason(json.RawMessage(`{"matchedRuleId":"x"}`)); err == nil {
		t.Fatal("incomplete snapshot accepted")
	}
}

func TestRiskRankOrdersExplainableLevels(t *testing.T) {
	if !(governance.RiskHigh.RiskRank() > governance.RiskMedium.RiskRank() &&
		governance.RiskMedium.RiskRank() > governance.RiskLow.RiskRank()) {
		t.Fatal("risk rank must order high > medium > low")
	}
}

func overload(length int) string {
	out := make([]byte, length)
	for index := range out {
		out[index] = 'x'
	}
	return string(out)
}

func repeat(value byte, count int) string {
	out := make([]byte, count)
	for index := range out {
		out[index] = value
	}
	return string(out)
}

func mustReviewBatchID(t *testing.T) identity.ReviewBatchID {
	t.Helper()
	id, err := identity.NewReviewBatchID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustWorkspaceID(t *testing.T) identity.WorkspaceID {
	t.Helper()
	id, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustProposalIDFor(t *testing.T) identity.ProposalID {
	t.Helper()
	id, err := identity.NewProposalID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

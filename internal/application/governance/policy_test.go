package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	governanceapp "github.com/iiwish/semlia/internal/application/governance"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type fakePolicyRepository struct {
	decisions    []governance.PolicyDecision
	linkedRisk   map[string]governance.RiskLevel
	linkedPolicy map[string]identity.PolicyDecisionID
}

func newFakePolicyRepository() *fakePolicyRepository {
	return &fakePolicyRepository{
		linkedRisk:   map[string]governance.RiskLevel{},
		linkedPolicy: map[string]identity.PolicyDecisionID{},
	}
}

func (repository *fakePolicyRepository) CreatePolicyDecision(
	_ context.Context, command governanceapp.PolicyDecisionCommand,
) (governance.PolicyDecision, error) {
	decision := governance.PolicyDecision{
		ID: command.ID, WorkspaceID: command.WorkspaceID, ProposalID: command.ProposalID,
		RuleVersion: command.RuleVersion, Inputs: command.Inputs, InputsDigest: command.InputsDigest,
		MatchedPolicy: command.MatchedPolicy, RiskLevel: command.RiskLevel,
		Routing: command.Routing, ReasonCode: command.ReasonCode, DecidedAt: command.DecidedAt,
	}
	repository.decisions = append(repository.decisions, decision)
	if command.LinkProposal {
		repository.linkedRisk[command.ProposalID.String()] = command.RiskLevel
		repository.linkedPolicy[command.ProposalID.String()] = command.ID
	}
	return decision, nil
}

func (repository *fakePolicyRepository) GetPolicyDecision(
	_ context.Context, _ identity.WorkspaceID, id identity.PolicyDecisionID,
) (governance.PolicyDecision, error) {
	for _, decision := range repository.decisions {
		if decision.ID == id {
			return decision, nil
		}
	}
	return governance.PolicyDecision{}, governance.ErrNotFound
}

func newGovernanceWorkspaceID(t *testing.T) identity.WorkspaceID {
	t.Helper()
	id, err := identity.NewWorkspaceID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newGovernanceProposalID(t *testing.T) identity.ProposalID {
	t.Helper()
	id, err := identity.NewProposalID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecomputabilityIdenticalInputsProduceIdenticalDecisions(t *testing.T) {
	// Packet red scenario: two policy decisions with identical inputs and the
	// same rule version must produce the identical risk level, routing and
	// reason plus the same inputs_digest, regardless of JSON key order.
	repository := newFakePolicyRepository()
	service := governanceapp.NewPolicyService(repository, governanceapp.ClockFunc(func() time.Time {
		return time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	}))
	workspace := newGovernanceWorkspaceID(t)
	firstProposal := newGovernanceProposalID(t)
	secondProposal := newGovernanceProposalID(t)

	first, err := service.Decide(context.Background(), governanceapp.DecideRequest{
		WorkspaceID: workspace, ProposalID: firstProposal, RuleVersion: governance.RiskRuleVersion,
		Inputs: json.RawMessage(`{"assetType":"metric","affectsComputation":true,"productionEnvironment":true,"blockerCount":0,"evidenceComplete":true}`),
	})
	if err != nil {
		t.Fatalf("first decision: %v", err)
	}
	second, err := service.Decide(context.Background(), governanceapp.DecideRequest{
		WorkspaceID: workspace, ProposalID: secondProposal, RuleVersion: governance.RiskRuleVersion,
		Inputs: json.RawMessage(`{"evidenceComplete":true, "blockerCount":0, "productionEnvironment":true, "affectsComputation":true, "assetType":"metric"}`),
	})
	if err != nil {
		t.Fatalf("second decision: %v", err)
	}
	if first.InputsDigest != second.InputsDigest {
		t.Fatalf("inputs digests differ: %s != %s", first.InputsDigest, second.InputsDigest)
	}
	if first.RiskLevel != second.RiskLevel || first.Routing != second.Routing || first.ReasonCode != second.ReasonCode {
		t.Fatalf("decisions differ: %+v vs %+v", first, second)
	}
	if first.RiskLevel != governance.RiskHigh || first.Routing != governance.RoutingExpert {
		t.Fatalf("production computation change routed as %s/%s, want high/expert", first.RiskLevel, first.Routing)
	}
	if first.MatchedPolicy == "" || first.ReasonCode == "" {
		t.Fatalf("decision must carry matched policy and reason: %+v", first)
	}
	if repository.linkedRisk[firstProposal.String()] != governance.RiskHigh {
		t.Fatalf("proposal risk level = %s, want high", repository.linkedRisk[firstProposal.String()])
	}
	if repository.linkedPolicy[firstProposal.String()] != first.ID {
		t.Fatal("proposal policy decision link missing")
	}
}

func TestRecomputabilityDifferentInputsProduceDifferentDigests(t *testing.T) {
	repository := newFakePolicyRepository()
	service := governanceapp.NewPolicyService(repository, governanceapp.ClockFunc(time.Now))
	workspace := newGovernanceWorkspaceID(t)
	proposal := newGovernanceProposalID(t)

	lowRisk, err := service.Decide(context.Background(), governanceapp.DecideRequest{
		WorkspaceID: workspace, ProposalID: proposal, RuleVersion: governance.RiskRuleVersion,
		Inputs: json.RawMessage(`{"assetType":"business_term","blockerCount":0,"evidenceComplete":true}`),
	})
	if err != nil {
		t.Fatalf("low-risk decision: %v", err)
	}
	highRisk, err := service.Decide(context.Background(), governanceapp.DecideRequest{
		WorkspaceID: workspace, ProposalID: proposal, RuleVersion: governance.RiskRuleVersion,
		Inputs: json.RawMessage(`{"assetType":"metric","blockerCount":2,"evidenceComplete":true}`),
	})
	if err != nil {
		t.Fatalf("high-risk decision: %v", err)
	}
	if lowRisk.InputsDigest == highRisk.InputsDigest {
		t.Fatal("different inputs must not share an inputs digest")
	}
	if lowRisk.RiskLevel != governance.RiskLow || lowRisk.Routing != governance.RoutingBatch {
		t.Fatalf("clean documentation change routed as %s/%s, want low/batch", lowRisk.RiskLevel, lowRisk.Routing)
	}
	if highRisk.RiskLevel != governance.RiskHigh {
		t.Fatalf("blocker-bearing change risk = %s, want high", highRisk.RiskLevel)
	}
}

func TestPolicyServiceRejectsUnknownRuleVersionAndMalformedInputs(t *testing.T) {
	repository := newFakePolicyRepository()
	service := governanceapp.NewPolicyService(repository, governanceapp.ClockFunc(time.Now))
	workspace := newGovernanceWorkspaceID(t)
	proposal := newGovernanceProposalID(t)

	if _, err := service.Decide(context.Background(), governanceapp.DecideRequest{
		WorkspaceID: workspace, ProposalID: proposal, RuleVersion: "99",
		Inputs: json.RawMessage(`{"assetType":"metric"}`),
	}); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("unknown rule version error = %v, want ErrInvalidArgument", err)
	}
	if _, err := service.Decide(context.Background(), governanceapp.DecideRequest{
		WorkspaceID: workspace, ProposalID: proposal, RuleVersion: governance.RiskRuleVersion,
		Inputs: json.RawMessage(`{"assetType":"metric","nonsense":true}`),
	}); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("unknown input field error = %v, want ErrInvalidArgument", err)
	}
	if _, err := service.Decide(context.Background(), governanceapp.DecideRequest{
		WorkspaceID: workspace, ProposalID: proposal, RuleVersion: governance.RiskRuleVersion,
		Inputs: json.RawMessage(`not json`),
	}); !errors.Is(err, governance.ErrInvalidArgument) {
		t.Fatalf("malformed inputs error = %v, want ErrInvalidArgument", err)
	}
	if len(repository.decisions) != 0 {
		t.Fatalf("rejected decisions must not persist, got %d", len(repository.decisions))
	}
}

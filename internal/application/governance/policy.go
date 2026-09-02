package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type PolicyRepository interface {
	CreatePolicyDecision(ctx context.Context, command PolicyDecisionCommand) (governance.PolicyDecision, error)
	GetPolicyDecision(ctx context.Context, workspace identity.WorkspaceID, decision identity.PolicyDecisionID) (governance.PolicyDecision, error)
}

// PolicyDecisionCommand persists the decision and links it to the proposal
// (policy_decision_id plus risk_level) in one transaction.
type PolicyDecisionCommand struct {
	ID            identity.PolicyDecisionID
	WorkspaceID   identity.WorkspaceID
	ProposalID    identity.ProposalID
	RuleVersion   string
	Inputs        json.RawMessage
	InputsDigest  string
	MatchedPolicy string
	RiskLevel     governance.RiskLevel
	Routing       governance.RoutingChannel
	ReasonCode    string
	DecidedAt     time.Time
	LinkProposal  bool
}

type PolicyService struct {
	repository PolicyRepository
	clock      Clock
}

func NewPolicyService(repository PolicyRepository, clock Clock) *PolicyService {
	return &PolicyService{repository: repository, clock: clock}
}

type DecideRequest struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	RuleVersion string
	Inputs      json.RawMessage
}

// Decide evaluates the versioned risk rule over strict canonical inputs and
// persists the immutable decision. Recomputability contract (SSOT §8.2): the
// same inputs under the same rule version produce the identical risk level,
// routing, reason and inputs_digest — the digest is the sha256 of the
// canonical (sorted-key) inputs encoding.
func (service *PolicyService) Decide(ctx context.Context, request DecideRequest) (governance.PolicyDecision, error) {
	if request.WorkspaceID.IsZero() || request.ProposalID.IsZero() || len(request.Inputs) == 0 {
		return governance.PolicyDecision{}, governance.ErrInvalidArgument
	}
	inputs, err := governance.ParseDecisionInputs(request.Inputs)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	decision, err := governance.EvaluateRiskRule(request.RuleVersion, inputs)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	canonical, err := governance.CanonicalJSON(mustMarshalInputs(inputs))
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	digest, err := governance.DigestJSON(canonical)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	decisionID, err := identity.NewPolicyDecisionID()
	if err != nil {
		return governance.PolicyDecision{}, fmt.Errorf("mint policy decision ID: %w", err)
	}
	return service.repository.CreatePolicyDecision(ctx, PolicyDecisionCommand{
		ID: decisionID, WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
		RuleVersion: request.RuleVersion, Inputs: canonical, InputsDigest: digest,
		MatchedPolicy: decision.MatchedPolicy, RiskLevel: decision.RiskLevel,
		Routing: decision.Routing, ReasonCode: decision.ReasonCode,
		DecidedAt: service.clock.Now().UTC(), LinkProposal: true,
	})
}

func (service *PolicyService) GetDecision(
	ctx context.Context, workspace identity.WorkspaceID, decision identity.PolicyDecisionID,
) (governance.PolicyDecision, error) {
	return service.repository.GetPolicyDecision(ctx, workspace, decision)
}

func mustMarshalInputs(inputs governance.DecisionInputs) json.RawMessage {
	encoded, err := json.Marshal(inputs)
	if err != nil {
		panic(fmt.Sprintf("marshal decision inputs: %v", err))
	}
	return encoded
}

package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type PolicyRepository interface {
	CreatePolicyDecision(ctx context.Context, command PolicyDecisionCommand) (governance.PolicyDecision, error)
	GetPolicyDecision(ctx context.Context, workspace identity.WorkspaceID, decision identity.PolicyDecisionID) (governance.PolicyDecision, error)
}

// PolicyRuleSource loads the versioned rule-table rows a decision evaluates.
// The store-backed implementation reads the rows migration 000008 seeds;
// decisions are data-driven and stay recomputable because rows are immutable
// per rule version.
type PolicyRuleSource interface {
	ListPolicyRules(ctx context.Context, ruleVersion string) ([]governance.PolicyRule, error)
}

type PolicyServiceOption func(*PolicyService)

// WithRuleSource attaches the rule-table loader. Without one the service
// falls back to the canonical built-in table behind EvaluateRiskRule; the
// integration suite locks both paths to identical outcomes.
func WithRuleSource(source PolicyRuleSource) PolicyServiceOption {
	return func(service *PolicyService) { service.ruleSource = source }
}

// StaticRuleSource pins one rule table, for tests and deployments that
// evaluate a fixed version.
func NewStaticRuleSource(ruleVersion string, rules []governance.PolicyRule) PolicyRuleSource {
	return staticRuleSource{ruleVersion: ruleVersion, rules: rules}
}

type staticRuleSource struct {
	ruleVersion string
	rules       []governance.PolicyRule
}

func (source staticRuleSource) ListPolicyRules(
	_ context.Context, ruleVersion string,
) ([]governance.PolicyRule, error) {
	if ruleVersion != source.ruleVersion {
		return nil, fmt.Errorf("%w: static rule source holds %s, not %s",
			governance.ErrInvalidArgument, source.ruleVersion, ruleVersion)
	}
	return append([]governance.PolicyRule(nil), source.rules...), nil
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
	ruleSource PolicyRuleSource
	clock      Clock
}

func NewPolicyService(repository PolicyRepository, clock Clock, options ...PolicyServiceOption) *PolicyService {
	service := &PolicyService{repository: repository, clock: clock}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
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
	return service.decide(ctx, request)
}

// EnsureDecisionRequest is the idempotent variant of DecideRequest.
type EnsureDecisionRequest = DecideRequest

// EnsureDecision returns the one decision a proposal owns for
// (rule_version, inputs_digest): an existing row is returned as-is, a new one
// is computed and persisted exactly once. Re-running the in_review decision
// over unchanged state therefore never duplicates rows; materially changed
// state (a different digest under the same rule version) legitimately
// produces a new append-only row for the new inputs.
func (service *PolicyService) EnsureDecision(ctx context.Context, request DecideRequest) (governance.PolicyDecision, error) {
	return service.decide(ctx, request)
}

func (service *PolicyService) decide(ctx context.Context, request DecideRequest) (governance.PolicyDecision, error) {
	if request.WorkspaceID.IsZero() || request.ProposalID.IsZero() || len(request.Inputs) == 0 {
		return governance.PolicyDecision{}, governance.ErrInvalidArgument
	}
	inputs, err := governance.ParseDecisionInputs(request.Inputs)
	if err != nil {
		return governance.PolicyDecision{}, err
	}
	decision, err := service.evaluate(ctx, request.RuleVersion, inputs)
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

// evaluate consults the wired rule table when present, degrading to the
// fail-safe expert decision when the table cannot be evaluated (SSOT §8.2:
// policy conflicts degrade to human handling, never silently pass), and to
// the canonical built-in table otherwise.
func (service *PolicyService) evaluate(ctx context.Context, ruleVersion string, inputs governance.DecisionInputs) (governance.RiskDecision, error) {
	if service.ruleSource == nil {
		return governance.EvaluateRiskRule(ruleVersion, inputs)
	}
	rules, err := service.ruleSource.ListPolicyRules(ctx, ruleVersion)
	if err != nil {
		return governance.RiskDecision{}, fmt.Errorf("load policy rules: %w", err)
	}
	decision, err := governance.EvaluatePolicyRules(rules, inputs)
	if err == nil {
		return decision, nil
	}
	if errors.Is(err, governance.ErrInvalidArgument) {
		return governance.RiskDecision{}, err
	}
	return governance.PolicyFallbackDecision(), nil
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

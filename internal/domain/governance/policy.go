package governance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// RiskRuleVersion is the current versioned risk rule (SSOT §8.2: every risk
// conclusion records its rule version and can be recomputed on identical
// inputs). Versions are "<major>.<minor>"; a new rule semantics means a new
// version, never a silent change.
const RiskRuleVersion = "1.0"

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type RoutingChannel string

const (
	RoutingExpert RoutingChannel = "expert"
	RoutingBatch  RoutingChannel = "batch"
)

// MatchedRiskPolicy is the identifier of the single policy row the current
// rule version consults.
const MatchedRiskPolicy = "semlia.risk.v1/default"

// DecisionInputs are the canonical risk-computation inputs (SSOT §8.3). The
// struct is closed: unknown fields are rejected so the digest always covers
// exactly the features the rule consumes.
type DecisionInputs struct {
	AssetType             string `json:"assetType"`
	AffectsComputation    bool   `json:"affectsComputation"`
	AffectsAccess         bool   `json:"affectsAccess"`
	AffectsContract       bool   `json:"affectsContract"`
	ProductionEnvironment bool   `json:"productionEnvironment"`
	BlockerCount          int    `json:"blockerCount"`
	EvidenceComplete      bool   `json:"evidenceComplete"`
}

func (inputs DecisionInputs) Validate() error {
	if inputs.AssetType == "" || inputs.BlockerCount < 0 {
		return ErrInvalidArgument
	}
	return nil
}

// RiskDecision is the deterministic rule output.
type RiskDecision struct {
	MatchedPolicy string
	RiskLevel     RiskLevel
	Routing       RoutingChannel
	ReasonCode    string
}

// Reason codes are stable identifiers, shared verbatim with the frontend
// contract when T003 exposes them.
const (
	ReasonRiskBlocker                = "RISK_BLOCKER"
	ReasonRiskProductionComputation  = "RISK_PRODUCTION_COMPUTATION"
	ReasonRiskAccessOrContractChange = "RISK_ACCESS_OR_CONTRACT_CHANGE"
	ReasonRiskComputationChange      = "RISK_COMPUTATION_CHANGE"
	ReasonRiskProductionChange       = "RISK_PRODUCTION_CHANGE"
	ReasonRiskLowBatch               = "RISK_LOW_BATCH"
)

// EvaluateRiskRule recomputes the risk decision from inputs alone. It is a
// pure function: no clock, no I/O, no environment. Identical inputs under one
// rule version always produce identical risk_level, routing and reason.
func EvaluateRiskRule(version string, inputs DecisionInputs) (RiskDecision, error) {
	if version != RiskRuleVersion {
		return RiskDecision{}, fmt.Errorf("%w: unknown risk rule version %q", ErrInvalidArgument, version)
	}
	if err := inputs.Validate(); err != nil {
		return RiskDecision{}, err
	}
	decision := RiskDecision{MatchedPolicy: MatchedRiskPolicy}
	switch {
	case inputs.BlockerCount > 0:
		decision.RiskLevel = RiskHigh
		decision.Routing = RoutingExpert
		decision.ReasonCode = ReasonRiskBlocker
	case inputs.AffectsComputation && inputs.ProductionEnvironment:
		decision.RiskLevel = RiskHigh
		decision.Routing = RoutingExpert
		decision.ReasonCode = ReasonRiskProductionComputation
	case inputs.AffectsAccess || inputs.AffectsContract:
		// Access-scope and consumption-contract changes always take the
		// expert channel (SSOT §8.4).
		decision.RiskLevel = RiskHigh
		decision.Routing = RoutingExpert
		decision.ReasonCode = ReasonRiskAccessOrContractChange
	case inputs.AffectsComputation:
		decision.RiskLevel = RiskMedium
		decision.Routing = RoutingExpert
		decision.ReasonCode = ReasonRiskComputationChange
	case inputs.ProductionEnvironment:
		decision.RiskLevel = RiskMedium
		decision.Routing = RoutingExpert
		decision.ReasonCode = ReasonRiskProductionChange
	default:
		decision.RiskLevel = RiskLow
		decision.Routing = RoutingBatch
		decision.ReasonCode = ReasonRiskLowBatch
	}
	return decision, nil
}

// ParseDecisionInputs decodes strict canonical inputs from raw JSON. Unknown
// fields are rejected so identical stored inputs always re-evaluate
// identically.
func ParseDecisionInputs(raw json.RawMessage) (DecisionInputs, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var inputs DecisionInputs
	if err := decoder.Decode(&inputs); err != nil {
		return DecisionInputs{}, fmt.Errorf("%w: decision inputs", ErrInvalidArgument)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return DecisionInputs{}, fmt.Errorf("%w: trailing decision inputs", ErrInvalidArgument)
	}
	if err := inputs.Validate(); err != nil {
		return DecisionInputs{}, err
	}
	return inputs, nil
}

// PolicyDecision is an immutable, recomputable risk fact for one proposal.
// Inputs are stored in canonical form and inputs_digest is their sha256, so
// two decisions with identical inputs share one digest (SSOT §8.2).
type PolicyDecision struct {
	ID            identity.PolicyDecisionID
	WorkspaceID   identity.WorkspaceID
	ProposalID    identity.ProposalID
	RuleVersion   string
	Inputs        json.RawMessage
	InputsDigest  string
	MatchedPolicy string
	RiskLevel     RiskLevel
	Routing       RoutingChannel
	ReasonCode    string
	DecidedAt     time.Time
}

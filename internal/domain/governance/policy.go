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

// MatchedRiskPolicy is the stable identifier recorded when no seeded rule
// row matches (or the rule table is unevaluable): the fail-safe default that
// routes the change to expert review (SSOT §8.2 degradation).
const MatchedRiskPolicy = "semlia.risk.v1/default"

// ReasonRiskPolicyUnmatched is the stable fail-safe reason code. It is the
// only reason that never appears on a seeded rule row.
const ReasonRiskPolicyUnmatched = "RISK_POLICY_UNMATCHED"

// Closed input-category vocabularies. The zero value ("") is legal and means
// "not recorded", so inputs persisted before a vocabulary shipped still parse
// and re-evaluate (recomputability, SSOT §8.2).
const (
	// DecisionInputNotApplicable marks inputs that carry no value for the
	// target (an asset type for join contracts) or are not yet computable.
	DecisionInputNotApplicable = "not_applicable"
	OwnerAssigned              = "assigned"
	OwnerUnassigned            = "unassigned"
	OwnerNotApplicable         = DecisionInputNotApplicable
	AuthorKindHuman            = "human"
	AuthorKindAgent            = "agent"
)

// DecisionInputs are the canonical risk-computation inputs (SSOT §8.3). The
// struct is closed: unknown fields are rejected so the digest always covers
// exactly the features the rule consumes.
//
// Categories that M2 cannot compute from persisted state are represented as
// explicit not-applicable placeholders (ConsumerCount,
// HistoricalAcceptRate: JSON null until M4 populates them). They are already
// part of the schema, so M4 fills values under a new rule version without a
// schema-shape change that would break stored digests.
type DecisionInputs struct {
	AssetType             string   `json:"assetType"`
	TargetObjectType      string   `json:"targetObjectType"`
	AffectsComputation    bool     `json:"affectsComputation"`
	AffectsDefinition     bool     `json:"affectsDefinition"`
	AffectsRelations      bool     `json:"affectsRelations"`
	AffectsAccess         bool     `json:"affectsAccess"`
	AffectsContract       bool     `json:"affectsContract"`
	ProductionEnvironment bool     `json:"productionEnvironment"`
	ValidationRunCount    int      `json:"validationRunCount"`
	ValidationFailedCount int      `json:"validationFailedCount"`
	BlockerCount          int      `json:"blockerCount"`
	WarningCount          int      `json:"warningCount"`
	InfoCount             int      `json:"infoCount"`
	EvidenceLinkCount     int      `json:"evidenceLinkCount"`
	EvidenceComplete      bool     `json:"evidenceComplete"`
	OwnerAssigned         string   `json:"ownerAssigned"`
	AuthorKind            string   `json:"authorKind"`
	ConsumerCount         *int     `json:"consumerCount"`
	HistoricalAcceptRate  *float64 `json:"historicalAcceptRate"`
}

func (inputs DecisionInputs) Validate() error {
	if inputs.AssetType == "" {
		return ErrInvalidArgument
	}
	if inputs.BlockerCount < 0 || inputs.WarningCount < 0 || inputs.InfoCount < 0 ||
		inputs.ValidationRunCount < 0 || inputs.ValidationFailedCount < 0 || inputs.EvidenceLinkCount < 0 {
		return ErrInvalidArgument
	}
	switch TargetObjectType(inputs.TargetObjectType) {
	case "", TargetSemanticAsset, TargetPhysicalBinding, TargetModelGrain, TargetEntityKey, TargetJoinContract:
	default:
		return ErrInvalidArgument
	}
	switch inputs.OwnerAssigned {
	case "", OwnerAssigned, OwnerUnassigned, OwnerNotApplicable:
	default:
		return ErrInvalidArgument
	}
	switch inputs.AuthorKind {
	case "", AuthorKindHuman, AuthorKindAgent:
	default:
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

// EvaluateRiskRule recomputes the risk decision from inputs alone by
// evaluating the canonical rule table for the version (migration 000008
// seeds the same rows). It is a pure function: no clock, no I/O, no
// environment. Identical inputs under one rule version always produce
// identical risk_level, routing and reason.
func EvaluateRiskRule(version string, inputs DecisionInputs) (RiskDecision, error) {
	rules, err := CanonicalPolicyRules(version)
	if err != nil {
		return RiskDecision{}, err
	}
	return EvaluatePolicyRules(rules, inputs)
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

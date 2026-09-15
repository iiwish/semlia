package governance

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// ProposalState is the SSOT §7.5 workflow state of a proposal aggregate.
// Asset lifecycle (draft/active/deprecated/retired), deployment state and
// health state are orthogonal dimensions stored elsewhere and are deliberately
// absent from this vocabulary.
type ProposalState string

const (
	ProposalDraft      ProposalState = "draft"
	ProposalProposed   ProposalState = "proposed"
	ProposalValidating ProposalState = "validating"
	ProposalInReview   ProposalState = "in_review"
	ProposalReleased   ProposalState = "released"
	ProposalRejected   ProposalState = "rejected"
)

// proposalTransitions is the exact transition table this package implements
// from SSOT §7.5 (the review-terminal rejection edge) plus the §8.2 pipeline
// rule that any governance stage may end in rejection:
//
//	draft      -> proposed | rejected
//	proposed   -> validating | rejected
//	validating -> in_review | rejected
//	in_review  -> released | rejected
//	released   -> (terminal)
//	rejected   -> (terminal)
//
// released and rejected are terminal for the proposal aggregate; released
// assets continue on the separate §7.5 asset lifecycle (deprecated -> retired)
// owned by semantic_assets, never by a proposal row.
var proposalTransitions = map[ProposalState][]ProposalState{
	ProposalDraft:      {ProposalProposed, ProposalRejected},
	ProposalProposed:   {ProposalValidating, ProposalRejected},
	ProposalValidating: {ProposalInReview, ProposalRejected},
	ProposalInReview:   {ProposalReleased, ProposalRejected},
	ProposalReleased:   {},
	ProposalRejected:   {},
}

func (state ProposalState) Valid() bool {
	_, ok := proposalTransitions[state]
	return ok
}

func (state ProposalState) Terminal() bool {
	return len(proposalTransitions[state]) == 0
}

// LegalProposalTransition reports whether from -> to is legal. A no-op
// (from == to) stays legal so idempotent writes never fail.
func LegalProposalTransition(from, to ProposalState) bool {
	if from == to {
		return true
	}
	for _, candidate := range proposalTransitions[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

// TransitionError carries the rejected from -> to pair.
type TransitionError struct {
	From ProposalState
	To   ProposalState
}

func (err *TransitionError) Error() string {
	return fmt.Sprintf("invalid proposal state transition %s -> %s", err.From, err.To)
}

func (err *TransitionError) Unwrap() error { return ErrInvalidTransition }

// CheckProposalTransition validates a transition and returns *TransitionError
// (unwrapping to ErrInvalidTransition) for every edge outside the table.
func CheckProposalTransition(from, to ProposalState) error {
	if !from.Valid() || !to.Valid() {
		return &TransitionError{From: from, To: to}
	}
	if LegalProposalTransition(from, to) {
		return nil
	}
	return &TransitionError{From: from, To: to}
}

// ChangeSetEditable reports whether change-set items may still be written for
// a proposal in the given state: they freeze the moment the proposal leaves
// draft (data-model.md: immutable once submitted).
func ChangeSetEditable(state ProposalState) bool {
	return state == ProposalDraft
}

type ChangeOp string

const (
	ChangeAdd    ChangeOp = "add"
	ChangeUpdate ChangeOp = "update"
	ChangeRemove ChangeOp = "remove"
)

var sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func isContentDigest(value string) bool {
	return sha256DigestPattern.MatchString(value)
}

// IsValidContentDigest reports whether value matches the repository-wide
// "sha256:<64 lowercase hex>" content digest shape.
func IsValidContentDigest(value string) bool {
	return isContentDigest(value)
}

func validJSONValue(value json.RawMessage) bool {
	return len(value) > 0 && json.Valid(value)
}

// ChangeSetItem is one structured patch entry against the released baseline
// revision. Digests make diffs verifiable without storing full baselines.
type ChangeSetItem struct {
	ID           identity.ProposalChangeID
	WorkspaceID  identity.WorkspaceID
	ProposalID   identity.ProposalID
	FieldPath    string
	Op           ChangeOp
	BeforeDigest string
	AfterDigest  string
	BeforeValue  json.RawMessage
	AfterValue   json.RawMessage
	CreatedAt    time.Time
}

// Validate enforces op/value/digest consistency: add carries only an after
// side, remove only a before side, update carries both, and every digest is
// paired with its jsonb value.
func (item ChangeSetItem) Validate() error {
	if item.FieldPath == "" || len(item.FieldPath) > 512 {
		return ErrInvalidArgument
	}
	switch item.Op {
	case ChangeAdd:
		if item.BeforeDigest != "" || item.BeforeValue != nil || item.AfterDigest == "" || item.AfterValue == nil {
			return ErrInvalidArgument
		}
	case ChangeUpdate:
		if item.BeforeDigest == "" || item.AfterDigest == "" || item.BeforeValue == nil || item.AfterValue == nil {
			return ErrInvalidArgument
		}
	case ChangeRemove:
		if item.AfterDigest != "" || item.AfterValue != nil || item.BeforeDigest == "" || item.BeforeValue == nil {
			return ErrInvalidArgument
		}
	default:
		return ErrInvalidArgument
	}
	if item.BeforeDigest != "" && !isContentDigest(item.BeforeDigest) {
		return ErrInvalidArgument
	}
	if item.AfterDigest != "" && !isContentDigest(item.AfterDigest) {
		return ErrInvalidArgument
	}
	if (item.BeforeValue != nil && !validJSONValue(item.BeforeValue)) ||
		(item.AfterValue != nil && !validJSONValue(item.AfterValue)) {
		return ErrInvalidArgument
	}
	return nil
}

type TargetObjectType string

const (
	TargetSemanticAsset TargetObjectType = "semantic_asset"
	TargetModelGrain    TargetObjectType = "model_grain"
	TargetEntityKey     TargetObjectType = "entity_key"
	TargetJoinContract  TargetObjectType = "join_contract"
)

// Proposal is the governed-authoring aggregate root. risk_level and
// policy_decision_id are filled by the recomputable policy decision;
// agent_run_id links the AI write to its §8.6 agent run.
type Proposal struct {
	ID                              identity.ProposalID
	WorkspaceID                     identity.WorkspaceID
	AssetID                         *identity.AssetID
	BaseRevisionID                  *identity.RevisionID
	TargetObjectType                TargetObjectType
	TargetObjectID                  string
	State                           ProposalState
	Title                           string
	Summary                         string
	Reason                          string
	RiskLevel                       RiskLevel
	PolicyDecisionID                *identity.PolicyDecisionID
	AgentRunID                      *identity.AgentRunID
	CreatedBy                       string
	Intent                          string
	CreationContent                 json.RawMessage
	BaseObjectVersion               *int
	ProductionOperationID           *identity.ProductionOperationID
	ProductionVersion               *int
	ReintroductionCreationReleaseID *string
	ReintroductionAbsenceReleaseID  *string
	SubmittedAt                     *time.Time
	DecidedAt                       *time.Time
	CreatedAt                       time.Time
	UpdatedAt                       time.Time
}

func (p Proposal) IsProductionMember() bool {
	return p.ProductionOperationID != nil
}

type ReviewChannel string

const (
	ReviewAutomatic ReviewChannel = "automatic"
	ReviewBatch     ReviewChannel = "batch"
	ReviewExpert    ReviewChannel = "expert"
)

func (channel ReviewChannel) Valid() bool {
	switch channel {
	case ReviewAutomatic, ReviewBatch, ReviewExpert:
		return true
	}
	return false
}

type ReviewDecision string

const (
	ReviewApproved         ReviewDecision = "approved"
	ReviewRejected         ReviewDecision = "rejected"
	ReviewChangesRequested ReviewDecision = "changes_requested"
)

func (decision ReviewDecision) Valid() bool {
	switch decision {
	case ReviewApproved, ReviewRejected, ReviewChangesRequested:
		return true
	}
	return false
}

// Review is an immutable §8.4 review fact. It records the human or AI review
// outcome; the workflow transition itself goes through the proposal state
// machine.
type Review struct {
	ID                  identity.ReviewID
	WorkspaceID         identity.WorkspaceID
	ProposalID          identity.ProposalID
	ReviewerPrincipalID identity.PrincipalID
	Channel             ReviewChannel
	Decision            ReviewDecision
	Note                string
	CreatedAt           time.Time
}

func (review Review) Validate() error {
	if review.ID.IsZero() || review.WorkspaceID.IsZero() || review.ProposalID.IsZero() ||
		review.ReviewerPrincipalID.IsZero() || !review.Channel.Valid() || !review.Decision.Valid() {
		return ErrInvalidArgument
	}
	return nil
}

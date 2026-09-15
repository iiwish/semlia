package governance

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// ReviewBatchStatus is the one-way §8.4 batch state: assembly writes an open
// batch and exactly one confirm command moves it to confirmed or rejected.
// The database trigger enforces the same discipline.
type ReviewBatchStatus string

const (
	ReviewBatchOpen      ReviewBatchStatus = "open"
	ReviewBatchConfirmed ReviewBatchStatus = "confirmed"
	ReviewBatchRejected  ReviewBatchStatus = "rejected"
)

func (status ReviewBatchStatus) Valid() bool {
	switch status {
	case ReviewBatchOpen, ReviewBatchConfirmed, ReviewBatchRejected:
		return true
	}
	return false
}

// DiffCategory is the closed §8.3 structured-diff category vocabulary used as
// the dominant-diff component of the persisted grouping rule. The zero value
// is legal for inputs persisted before the vocabulary shipped and is treated
// as the definition/docs category, exactly like ClassifyDiffCategories.
type DiffCategory string

const (
	DiffCategoryComputation DiffCategory = "computation"
	DiffCategoryAccess      DiffCategory = "access"
	DiffCategoryContract    DiffCategory = "contract"
	DiffCategoryRelations   DiffCategory = "relations"
	DiffCategoryDefinition  DiffCategory = "definition"
)

// dominantDiffCategoryPriority is the fixed precedence used to pick the
// dominant category of one proposal from its decision inputs: the most
// risk-bearing category wins, ties resolve by this order alone.
var dominantDiffCategoryPriority = []DiffCategory{
	DiffCategoryComputation, DiffCategoryAccess, DiffCategoryContract,
	DiffCategoryRelations, DiffCategoryDefinition,
}

// DominantDiffCategory reduces the closed decision-input diff flags to the
// single category that dominates the change (deterministic; no-tie by
// construction). A change with no recorded flag is a definition change.
func DominantDiffCategory(inputs DecisionInputs) DiffCategory {
	flags := map[DiffCategory]bool{
		DiffCategoryComputation: inputs.AffectsComputation,
		DiffCategoryAccess:      inputs.AffectsAccess,
		DiffCategoryContract:    inputs.AffectsContract,
		DiffCategoryRelations:   inputs.AffectsRelations,
		DiffCategoryDefinition:  inputs.AffectsDefinition,
	}
	for _, category := range dominantDiffCategoryPriority {
		if flags[category] {
			return category
		}
	}
	return DiffCategoryDefinition
}

// ReviewGroupingRule is the persisted §8.4 grouping criteria of one batch:
// every member shares the target object type, the dominant structured-diff
// category and the matched policy rule of its creation-time decision.
type ReviewGroupingRule struct {
	TargetObjectType TargetObjectType `json:"targetObjectType"`
	DiffCategory     DiffCategory     `json:"diffCategory"`
	MatchedRuleID    string           `json:"matchedRuleId"`
}

func (rule ReviewGroupingRule) Validate() error {
	switch rule.TargetObjectType {
	case TargetSemanticAsset, TargetModelGrain, TargetEntityKey, TargetJoinContract:
	default:
		return fmt.Errorf("%w: review batch grouping rule target object type", ErrInvalidArgument)
	}
	switch rule.DiffCategory {
	case DiffCategoryComputation, DiffCategoryAccess, DiffCategoryContract,
		DiffCategoryRelations, DiffCategoryDefinition:
	default:
		return fmt.Errorf("%w: review batch grouping rule diff category %q", ErrInvalidArgument, rule.DiffCategory)
	}
	if rule.MatchedRuleID == "" || len(rule.MatchedRuleID) > 128 {
		return fmt.Errorf("%w: review batch grouping rule matched rule id", ErrInvalidArgument)
	}
	return nil
}

// ReviewAddedReason is the frozen decision snapshot recorded on every member
// at assembly time: the exact policy decision that made the proposal
// batch-eligible. It is the per-member half of the §8.4 audit record.
type ReviewAddedReason struct {
	MatchedRuleID string          `json:"matchedRuleId"`
	RiskLevel     RiskLevel       `json:"riskLevel"`
	ReasonCode    string          `json:"reasonCode"`
	RuleVersion   string          `json:"ruleVersion"`
	InputsDigest  string          `json:"inputsDigest"`
	Extra         json.RawMessage `json:"-"`
}

func (reason ReviewAddedReason) validate() error {
	if reason.MatchedRuleID == "" || len(reason.MatchedRuleID) > 128 || !RiskLevelValid(reason.RiskLevel) ||
		reason.ReasonCode == "" || reason.RuleVersion == "" ||
		!isContentDigest(reason.InputsDigest) {
		return fmt.Errorf("%w: incomplete review batch added reason", ErrInvalidArgument)
	}
	return nil
}

func (reason ReviewAddedReason) Marshal() (json.RawMessage, error) {
	if err := reason.validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(struct {
		MatchedRuleID string    `json:"matchedRuleId"`
		RiskLevel     RiskLevel `json:"riskLevel"`
		ReasonCode    string    `json:"reasonCode"`
		RuleVersion   string    `json:"ruleVersion"`
		InputsDigest  string    `json:"inputsDigest"`
	}{
		MatchedRuleID: reason.MatchedRuleID, RiskLevel: reason.RiskLevel,
		ReasonCode: reason.ReasonCode, RuleVersion: reason.RuleVersion,
		InputsDigest: reason.InputsDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: encode review batch added reason", ErrInvalidArgument)
	}
	return encoded, nil
}

func ParseReviewAddedReason(raw json.RawMessage) (ReviewAddedReason, error) {
	var reason ReviewAddedReason
	if len(raw) == 0 || !json.Valid(raw) {
		return reason, fmt.Errorf("%w: review batch added reason", ErrInvalidArgument)
	}
	if err := json.Unmarshal(raw, &reason); err != nil {
		return reason, fmt.Errorf("%w: review batch added reason", ErrInvalidArgument)
	}
	if err := reason.validate(); err != nil {
		return ReviewAddedReason{}, err
	}
	return reason, nil
}

func RiskLevelValid(level RiskLevel) bool {
	switch level {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	}
	return false
}

// RiskRank orders risk levels for the max-risk member pick (higher wins).
func (level RiskLevel) RiskRank() int {
	switch level {
	case RiskHigh:
		return 3
	case RiskMedium:
		return 2
	case RiskLow:
		return 1
	}
	return 0
}

// ReviewBatch is the persisted §8.4 audit record of one batch confirmation
// over one proposal cluster. The workflow decision itself stays on the T002
// review rows; this aggregate only freezes what the operator confirmed and
// what was excluded from the confirmation.
type ReviewBatchRecord struct {
	ID            identity.ReviewBatchID
	WorkspaceID   identity.WorkspaceID
	GroupingRule  ReviewGroupingRule
	PolicyVersion string
	Status        ReviewBatchStatus
	CreatedBy     string
	DecidedBy     *string
	DecidedAt     *time.Time
	CreatedAt     time.Time
}

// ReviewBatchMember is one frozen snapshot of a proposal inside a batch.
// Decision, sample and the split columns fill at confirm time; proposal and
// added reason are immutable.
type ReviewBatchMember struct {
	BatchID     identity.ReviewBatchID
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	AddedReason json.RawMessage
	Decision    *ReviewDecision
	Sample      bool
	SplitOut    bool
	SplitReason *string
	CreatedAt   time.Time
}

func (member ReviewBatchMember) Validate() error {
	if member.BatchID.IsZero() || member.WorkspaceID.IsZero() || member.ProposalID.IsZero() {
		return ErrInvalidArgument
	}
	if _, err := ParseReviewAddedReason(member.AddedReason); err != nil {
		return err
	}
	if member.Decision != nil {
		if member.SplitOut || (*member.Decision != ReviewApproved && *member.Decision != ReviewRejected) {
			return ErrInvalidArgument
		}
	}
	if member.SplitOut {
		if member.SplitReason == nil || *member.SplitReason == "" || len(*member.SplitReason) > 512 {
			return ErrInvalidArgument
		}
	}
	return nil
}

// ReviewBatchDetail is the read model of one batch: the batch with its full
// membership plus the derived §8.4 surfaces (samples, exclusions, max-risk
// member). Samples and the max-risk member are deterministic projections of
// the persisted member rows, so the audit record reproduces exactly.
type ReviewBatchDetail struct {
	Batch         ReviewBatchRecord
	Members       []ReviewBatchMember
	Samples       []ReviewBatchMember
	Exclusions    []ReviewBatchMember
	MaxRiskMember *ReviewBatchMember
}

// SampleCount is the deterministic representative-sample size: the first N
// members by assembly sequence. Three keeps the sample meaningful for the
// small clusters the first batch workspaces produce while staying bounded.
const SampleCount = 3

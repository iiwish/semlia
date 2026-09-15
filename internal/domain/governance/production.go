package governance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

const (
	TargetKindSemanticAsset   = "semantic_asset"
	TargetKindPhysicalBinding = "physical_binding"
	TargetKindModelGrain      = "model_grain"
	TargetKindEntityKey       = "entity_key"
	TargetKindJoinContract    = "join_contract"

	ProductionIntentCreate = "create"
	ProductionIntentUpdate = "update"

	ProductionOutcomeProposal = "proposal"
	ProductionOutcomeNoChange = "no_change"

	CommandCreate       = "create"
	CommandReplaceDraft = "replace_draft"
	CommandSubmit       = "submit"
	CommandValidate     = "validate"
	CommandReview       = "review"
	CommandPublish      = "publish"
	CommandGenerate     = "generate"
	CommandRollback     = "rollback"

	ContributorAuthor    = "author"
	ContributorEditor    = "editor"
	ContributorInitiator = "initiator"
	ContributorAgent     = "agent"

	MaxTargetsPerOperation = 32
	MaxChangesPerTarget    = 100
	MaxTotalChanges        = 256
	MaxCanonicalInputBytes = 768 * 1024 // 768 KiB
)

var localKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func IsValidLocalKey(key string) bool {
	return localKeyPattern.MatchString(key)
}

func IsValidTargetKind(kind string) bool {
	switch kind {
	case TargetKindSemanticAsset, TargetKindPhysicalBinding, TargetKindModelGrain, TargetKindEntityKey, TargetKindJoinContract:
		return true
	default:
		return false
	}
}

type ProductionOperation struct {
	ID                    identity.ProductionOperationID
	WorkspaceID           identity.WorkspaceID
	CreatedBy             identity.PrincipalID
	CurrentVersion        int
	SupersedesOperationID *identity.ProductionOperationID
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type ProductionListQuery struct {
	Limit         int
	CreatedBy     string
	SourceID      string
	CandidateID   string
	WatermarkTime time.Time
	WatermarkID   string
	AfterTime     time.Time
	AfterID       string
}

type AlreadyProducedError struct {
	OperationID identity.ProductionOperationID
}

func (err *AlreadyProducedError) Error() string { return ErrAlreadyProduced.Error() }
func (err *AlreadyProducedError) Unwrap() error { return ErrAlreadyProduced }

type ProductionVersion struct {
	SupersedesOperationID *identity.ProductionOperationID
	SupersedesVersion     int
	ActiveValidation      *ValidationAttempt
	ValidationRuns        []ValidationRun
	ValidationResults     map[string][]ValidationResult
	ReleaseID             *identity.ReleaseID
	TraceID               string
	DeclarationsJSON      json.RawMessage
	BaselineJSON          json.RawMessage
	BaselineHead          HeadReference
	HistoryQuality        string
	WorkspaceID           identity.WorkspaceID
	OperationID           identity.ProductionOperationID
	Version               int
	InputJSON             json.RawMessage
	InputDigest           string
	RequestDigest         string
	SetDigest             string
	FrozenAt              *time.Time
	CreatedBy             identity.PrincipalID
	CreatedAt             time.Time
}

type ProductionTarget struct {
	ValidationRunIDs     []string
	ReviewIDs            []string
	ReviewApproved       bool
	ProposalState        *string
	Declaration          *TargetDeclaration
	Changes              []ChangeSetItem
	WorkspaceID          identity.WorkspaceID
	OperationID          identity.ProductionOperationID
	Version              int
	LocalKey             string
	Kind                 string
	Intent               string
	TargetID             string
	IdentityKey          *string
	BaseRevisionID       *string
	BaseObjectVersion    *int
	RegistryWriteVersion *int
	ContentJSON          json.RawMessage
	ContentDigest        string
	ProposalID           *identity.ProposalID
	Outcome              string
}

type ProductionCandidateLink struct {
	WorkspaceID     identity.WorkspaceID
	CandidateID     string
	CandidateDigest string
	OperationID     identity.ProductionOperationID
	Version         int
	LocalKey        string
	DecisionID      *string
	IsPrimary       bool
}

type ProductionContributor struct {
	WorkspaceID identity.WorkspaceID
	OperationID identity.ProductionOperationID
	PrincipalID identity.PrincipalID
	Role        string
	CreatedAt   time.Time
}

type ProductionIdentityReservation struct {
	WorkspaceID         identity.WorkspaceID
	Kind                string
	IdentityKey         string
	TargetID            string
	CreationOperationID identity.ProductionOperationID
	OwnerOperationID    identity.ProductionOperationID
	CreatedAt           time.Time
}

type ProductionCommand struct {
	ResponseJSON     json.RawMessage
	WorkspaceID      identity.WorkspaceID
	PrincipalID      identity.PrincipalID
	CommandKind      string
	IdempotencyKey   string
	RequestDigest    string
	OperationID      identity.ProductionOperationID
	OperationVersion int
	ResultKind       string
	ResultID         string
	CommittedAt      time.Time
}

type ProductionRequestClaim struct {
	WorkspaceID    identity.WorkspaceID
	BusinessDigest string
	OperationID    identity.ProductionOperationID
	CreatedAt      time.Time
}

// TargetChangeInput specifies changes for an update target.
type TargetChangeInput struct {
	FieldPath   string          `json:"fieldPath"`
	Op          string          `json:"op"`
	BeforeValue json.RawMessage `json:"beforeValue,omitempty"`
	AfterValue  json.RawMessage `json:"afterValue,omitempty"`
}

// TargetDeclaration specifies an intent to create or update a target.
type TargetDeclaration struct {
	ReuseIdentity        *ProductionReintroductionIdentity `json:"reuseIdentity,omitempty"`
	LocalKey             string                            `json:"localKey"`
	Title                string                            `json:"title"`
	TargetID             *string                           `json:"targetId,omitempty"`
	EvidenceIDs          []string                          `json:"evidenceIds"`
	Kind                 string                            `json:"kind"`
	Intent               string                            `json:"intent"`
	IdentityKey          *string                           `json:"identityKey,omitempty"`
	BaseRevisionID       *string                           `json:"baseRevisionId,omitempty"`
	BaseObjectVersion    *int                              `json:"baseObjectVersion,omitempty"`
	RegistryWriteVersion *int                              `json:"registryWriteVersion,omitempty"`
	Content              json.RawMessage                   `json:"content,omitempty"`
	Changes              []TargetChangeInput               `json:"changes"`
}

type ProductionReintroductionIdentity struct {
	TargetID            string        `json:"targetId"`
	CreationOperationID string        `json:"creationOperationId"`
	CreationReleaseID   string        `json:"creationReleaseId"`
	AbsenceReleaseID    string        `json:"absenceReleaseId"`
	ExpectedHead        HeadReference `json:"expectedHead"`
}

// CandidateDeclaration links a candidate to targets.
type CandidateDeclaration struct {
	CandidateID      string   `json:"candidateId"`
	CandidateDigest  string   `json:"candidateDigest"`
	TargetKeys       []string `json:"targetKeys"`
	PrimaryTargetKey string   `json:"primaryTargetKey"`
}

// ValidateTargetDeclarations validates limits, keys, kinds, and change counts.
func ValidateTargetDeclarations(targets []TargetDeclaration) error {
	if len(targets) == 0 {
		return fmt.Errorf("%w: at least one target required", ErrInvalidArgument)
	}
	if len(targets) > MaxTargetsPerOperation {
		return fmt.Errorf("%w: target count %d exceeds maximum %d", ErrLimitExceeded, len(targets), MaxTargetsPerOperation)
	}

	seenKeys := make(map[string]bool, len(targets))
	totalChanges := 0

	for _, target := range targets {
		if !IsValidLocalKey(target.LocalKey) {
			return fmt.Errorf("%w: invalid localKey %q", ErrInvalidArgument, target.LocalKey)
		}
		if seenKeys[target.LocalKey] {
			return fmt.Errorf("%w: duplicate localKey %q", ErrInvalidArgument, target.LocalKey)
		}
		seenKeys[target.LocalKey] = true

		if !IsValidTargetKind(target.Kind) {
			return fmt.Errorf("%w: invalid target kind %q", ErrInvalidArgument, target.Kind)
		}
		if target.Intent != ProductionIntentCreate && target.Intent != ProductionIntentUpdate {
			return fmt.Errorf("%w: invalid intent %q", ErrInvalidArgument, target.Intent)
		}

		if target.Intent == ProductionIntentCreate {
			if len(target.Changes) > 0 {
				return fmt.Errorf("%w: create target %q cannot specify changes", ErrInvalidArgument, target.LocalKey)
			}
			if len(target.Content) == 0 {
				return fmt.Errorf("%w: create target %q requires content", ErrInvalidArgument, target.LocalKey)
			}
		}

		if target.Intent == ProductionIntentUpdate {
			if len(target.Changes) > MaxChangesPerTarget {
				return fmt.Errorf("%w: target %q has %d changes, exceeding max %d", ErrLimitExceeded, target.LocalKey, len(target.Changes), MaxChangesPerTarget)
			}
			totalChanges += len(target.Changes)
		}
	}

	if totalChanges > MaxTotalChanges {
		return fmt.Errorf("%w: total changes %d exceeds maximum %d", ErrLimitExceeded, totalChanges, MaxTotalChanges)
	}

	return nil
}

// ValidateCandidateDeclarations ensures candidate references match declared targets and primary rules.
func ValidateCandidateDeclarations(candidates []CandidateDeclaration, targets []TargetDeclaration) error {
	targetMap := make(map[string]TargetDeclaration, len(targets))
	for _, t := range targets {
		targetMap[t.LocalKey] = t
	}

	seenCandidates := make(map[string]bool, len(candidates))
	for _, cand := range candidates {
		if cand.CandidateID == "" || !isContentDigest(cand.CandidateDigest) {
			return fmt.Errorf("%w: invalid candidate ID or digest", ErrInvalidArgument)
		}
		if seenCandidates[cand.CandidateID] {
			return fmt.Errorf("%w: duplicate candidate %q", ErrInvalidArgument, cand.CandidateID)
		}
		seenCandidates[cand.CandidateID] = true

		if cand.PrimaryTargetKey == "" {
			return fmt.Errorf("%w: primaryTargetKey is required for candidate %q", ErrInvalidArgument, cand.CandidateID)
		}

		primaryFound := false
		seenTargetKeys := make(map[string]bool, len(cand.TargetKeys))
		for _, key := range cand.TargetKeys {
			if seenTargetKeys[key] {
				return fmt.Errorf("%w: duplicate candidate targetKey %q", ErrInvalidArgument, key)
			}
			seenTargetKeys[key] = true
			if _, exists := targetMap[key]; !exists {
				return fmt.Errorf("%w: candidate %q references unknown targetKey %q", ErrInvalidArgument, cand.CandidateID, key)
			}
			if key == cand.PrimaryTargetKey {
				primaryFound = true
			}
		}

		if !primaryFound {
			return fmt.Errorf("%w: primaryTargetKey %q not in targetKeys for candidate %q", ErrInvalidArgument, cand.PrimaryTargetKey, cand.CandidateID)
		}
	}

	return nil
}

// ResolveLocalReferences resolves structured local references, leaving business strings intact.
func ResolveLocalReferences(content json.RawMessage, localKeyToTargetID map[string]string) (json.RawMessage, error) {
	if len(content) == 0 {
		return content, nil
	}

	var parsed any
	canonical, err := CanonicalJSON(content)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("%w: failed to unmarshal content for reference resolution", ErrInvalidArgument)
	}

	var resolveVal func(v any) (any, error)
	resolveVal = func(v any) (any, error) {
		switch val := v.(type) {
		case string:
			return val, nil
		case []any:
			res := make([]any, len(val))
			for i, item := range val {
				resolved, err := resolveVal(item)
				if err != nil {
					return nil, err
				}
				res[i] = resolved
			}
			return res, nil
		case map[string]any:
			if _, exists := val["snapshotId"]; exists {
				ref, err := parseProductionPhysicalReference(val)
				if err != nil {
					return nil, err
				}
				return ref.ObjectID, nil
			}
			if _, exists := val["releaseId"]; exists {
				ref, err := parseProductionPublishedReference(val)
				if err != nil {
					return nil, err
				}
				return ref.TargetID, nil
			}
			if key, exists := val["localKey"]; exists {
				name, valid := key.(string)
				targetID, found := localKeyToTargetID[name]
				if !valid || !found || len(val) != 1 {
					return nil, fmt.Errorf("%w: invalid local reference", ErrInvalidArgument)
				}
				return targetID, nil
			}
			res := make(map[string]any, len(val))
			for k, item := range val {
				resolved, err := resolveVal(item)
				if err != nil {
					return nil, err
				}
				res[k] = resolved
			}
			return res, nil
		default:
			return val, nil
		}
	}

	resolved, err := resolveVal(parsed)
	if err != nil {
		return nil, err
	}

	return json.Marshal(resolved)
}

// ComputeBusinessDigest creates a stable fingerprint for a set of desired outputs, excluding caller/trace/idempotency keys.
func ComputeBusinessDigest(workspaceID string, inputDigest string, sortedTargetDigests []string) string {
	sort.Strings(sortedTargetDigests)
	h := sha256.New()
	h.Write([]byte("semlia.production.claim/v1\n"))
	h.Write([]byte(workspaceID + "\n"))
	h.Write([]byte(inputDigest + "\n"))
	for _, td := range sortedTargetDigests {
		h.Write([]byte(td + "\n"))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// RequestDigestInput holds canonical attributes that define a distinct production request.
type RequestDigestInput struct {
	WorkspaceID           string                 `json:"workspaceId"`
	ResourceID            string                 `json:"resourceId,omitempty"`
	CommandKind           string                 `json:"commandKind"`
	ExpectedVersion       *int                   `json:"expectedVersion,omitempty"`
	SupersedesOperationID *string                `json:"supersedesOperationId,omitempty"`
	InputDigest           string                 `json:"inputDigest"`
	Targets               []TargetDeclaration    `json:"targets"`
	Candidates            []CandidateDeclaration `json:"candidates"`
}

// ComputeRequestDigest preserves numeric lexemes using the production canonical contract.
func ComputeRequestDigest(in RequestDigestInput) (string, error) {
	var err error
	in.Targets, in.Candidates, err = NormalizeProductionDeclarations(in.Targets, in.Candidates)
	if err != nil {
		return "", err
	}
	if err := ValidateProductionCanonicalBudget(json.RawMessage(`null`), in.Targets); err != nil {
		return "", err
	}
	bytes, err := json.Marshal(in)
	if err != nil {
		return "", err
	}
	return DigestJSON(bytes)
}

var (
	ErrVersionConflict    = errors.New("production version conflict")
	ErrBaselineConflict   = errors.New("production baseline conflict")
	ErrHeadConflict       = errors.New("production head conflict")
	ErrInputStale         = errors.New("production input stale")
	ErrValidationRequired = errors.New("production validation required")
	ErrReviewRequired     = errors.New("production review required")
	ErrSodConflict        = errors.New("production separation of duties conflict")
	ErrPriorStateUnknown  = errors.New("production prior state unknown")
)

const (
	ValidationStatusNotRequested = "not_requested"
	ValidationStatusQueued       = "queued"
	ValidationStatusRunning      = "running"
	ValidationStatusSucceeded    = "succeeded"
	ValidationStatusFailed       = "failed"

	RoleApplied  = "applied"
	RoleReverted = "reverted"

	PresencePresent = "present"
	PresenceAbsent  = "absent"
)

type ValidationAttempt struct {
	WorkspaceID          identity.WorkspaceID
	OperationID          identity.ProductionOperationID
	ProductionVersion    int
	AttemptNo            int
	SetDigest            string
	InputDigest          string
	FreshnessWitnessJSON json.RawMessage
	FreshnessDigest      string
	RequiredChecksJSON   json.RawMessage
	RequiredChecksDigest string
	Status               string
	ValidationDigest     *string
	CreatedBy            identity.PrincipalID
	CreatedAt            time.Time
	CompletedAt          *time.Time
}

type ValidationBinding struct {
	WorkspaceID           identity.WorkspaceID
	ValidationRunID       identity.ValidationRunID
	OperationID           identity.ProductionOperationID
	ProductionVersion     int
	AttemptNo             int
	SetDigest             string
	ProposalID            identity.ProposalID
	ProposalContentDigest string
	InputDigest           string
	CreatedAt             time.Time
}

type ProductionReviewBinding struct {
	WorkspaceID       identity.WorkspaceID
	ReviewID          identity.ReviewID
	OperationID       identity.ProductionOperationID
	ProductionVersion int
	AttemptNo         int
	SetDigest         string
	ValidationDigest  string
	CreatedAt         time.Time
}

type ReleaseProposal struct {
	WorkspaceID           identity.WorkspaceID
	ReleaseID             identity.ReleaseID
	ProposalID            identity.ProposalID
	OperationID           identity.ProductionOperationID
	ProductionVersion     int
	SetDigest             string
	ProposalContentDigest string
	AuthorPrincipalID     identity.PrincipalID
	AttemptNo             int
	ValidationDigest      string
	ReviewIDs             json.RawMessage
	Role                  string
	CreatedAt             time.Time
}

type ProductionReleaseManifest struct {
	WorkspaceID          identity.WorkspaceID
	ReleaseID            identity.ReleaseID
	BeforeReleaseID      *identity.ReleaseID
	BeforeManifestJSON   json.RawMessage
	BeforeManifestDigest string
	AfterManifestDigest  string
	AttributionDigest    string
	CreatedAt            time.Time
}

type ProductionReleaseBeforePin struct {
	WorkspaceID   identity.WorkspaceID
	ReleaseID     identity.ReleaseID
	TargetKind    string
	TargetID      string
	Presence      string
	RevisionID    *identity.RevisionID
	ObjectVersion *int
	ContentDigest *string
	CreatedAt     time.Time
}

type ProductionReleaseBindingInput struct {
	WorkspaceID       identity.WorkspaceID
	ReleaseID         identity.ReleaseID
	BindingID         string
	BindingVersion    int
	SnapshotID        identity.SourceSnapshotID
	DatasetRevisionID string
	FieldRevisionIDs  json.RawMessage
	CreatedAt         time.Time
}

type ProductionReleaseProtection struct {
	RootReleaseID           identity.ReleaseID
	RollbackDepth           int
	RollbackParentReleaseID *identity.ReleaseID
}

type HeadReference struct {
	Presence       string              `json:"presence"`
	ReleaseID      *identity.ReleaseID `json:"releaseId,omitempty"`
	ManifestDigest *string             `json:"manifestDigest,omitempty"`
}

type ValidationReference struct {
	AttemptNo        int    `json:"attemptNo"`
	ValidationDigest string `json:"validationDigest"`
}

// ComputeValidationDigest calculates the deterministic SHA256 digest of a validation attempt.
func ComputeValidationDigest(requiredChecksDigest string, freshnessDigest string, sortedRunDigests []string) string {
	sort.Strings(sortedRunDigests)
	h := sha256.New()
	h.Write([]byte("semlia.validation/v1\n"))
	h.Write([]byte(requiredChecksDigest + "\n"))
	h.Write([]byte(freshnessDigest + "\n"))
	for _, rd := range sortedRunDigests {
		h.Write([]byte(rd + "\n"))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// ComputeAttributionDigest calculates the deterministic SHA256 digest of a release attribution.
func ComputeAttributionDigest(operationID string, version int, setDigest string, validationDigest string, sortedReviewIDs []string) string {
	sort.Strings(sortedReviewIDs)
	h := sha256.New()
	h.Write([]byte("semlia.attribution/v1\n"))
	h.Write([]byte(operationID + "\n"))
	h.Write([]byte(fmt.Sprintf("%d\n", version)))
	h.Write([]byte(setDigest + "\n"))
	h.Write([]byte(validationDigest + "\n"))
	for _, rid := range sortedReviewIDs {
		h.Write([]byte(rid + "\n"))
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

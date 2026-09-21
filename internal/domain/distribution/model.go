package distribution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrInvalidArgument = errors.New("invalid semantic distribution argument")
	ErrNotFound        = errors.New("semantic distribution resource not found")
	ErrConflict        = errors.New("semantic distribution state conflict")
	ErrInvariant       = errors.New("semantic distribution invariant violated")
)

const (
	QuerySchemaVersion = "1.0.0"
	ResolverVersion    = "alpha-1"
)

type RefusalCode string

const (
	RefusalNoRelease              RefusalCode = "NO_RELEASE"
	RefusalBindingInactive        RefusalCode = "BINDING_INACTIVE"
	RefusalBindingExpired         RefusalCode = "BINDING_EXPIRED"
	RefusalConsumerInactive       RefusalCode = "CONSUMER_INACTIVE"
	RefusalUnauthorizedConsumer   RefusalCode = "UNAUTHORIZED_CONSUMER"
	RefusalStaleReleaseBinding    RefusalCode = "STALE_RELEASE_BINDING"
	RefusalNoMatch                RefusalCode = "NO_MATCH"
	RefusalAmbiguousMatch         RefusalCode = "AMBIGUOUS_MATCH"
	RefusalUnauthorizedAsset      RefusalCode = "UNAUTHORIZED_ASSET"
	RefusalMissingPhysicalBinding RefusalCode = "MISSING_PHYSICAL_BINDING"
	RefusalMissingJoinPath        RefusalCode = "MISSING_JOIN_PATH"
	RefusalIncompatibleGrain      RefusalCode = "INCOMPATIBLE_GRAIN"
	RefusalInvalidQuery           RefusalCode = "INVALID_QUERY"
	RefusalInvalidFilter          RefusalCode = "INVALID_FILTER"
	RefusalInvalidTimeRange       RefusalCode = "INVALID_TIME_RANGE"
	RefusalInvalidOrdering        RefusalCode = "INVALID_ORDERING"
	RefusalInvalidLimit           RefusalCode = "INVALID_LIMIT"
	RefusalInvalidPlan            RefusalCode = "INVALID_PLAN"
)

type ValidationError struct {
	Code    RefusalCode
	Message string
}

func (err *ValidationError) Error() string { return err.Message }

type ConsumerStatus string

const (
	ConsumerActive    ConsumerStatus = "active"
	ConsumerSuspended ConsumerStatus = "suspended"
	ConsumerRevoked   ConsumerStatus = "revoked"
)

type Consumer struct {
	ID                identity.ConsumerID
	WorkspaceID       identity.WorkspaceID
	StableKey         string
	Name              string
	Kind              string
	Status            ConsumerStatus
	OwnerPrincipalRef string
	Metadata          json.RawMessage
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (consumer Consumer) Validate() error {
	if consumer.ID.IsZero() || consumer.WorkspaceID.IsZero() || !validToken(consumer.StableKey, 120) ||
		strings.TrimSpace(consumer.Name) == "" || len(consumer.Name) > 160 ||
		strings.TrimSpace(consumer.OwnerPrincipalRef) == "" || len(consumer.OwnerPrincipalRef) > 256 {
		return ErrInvalidArgument
	}
	if consumer.Kind != "application" && consumer.Kind != "agent" && consumer.Kind != "human" {
		return ErrInvalidArgument
	}
	if consumer.Status != ConsumerActive && consumer.Status != ConsumerSuspended && consumer.Status != ConsumerRevoked {
		return ErrInvalidArgument
	}
	if !validJSONObject(consumer.Metadata) {
		return ErrInvalidArgument
	}
	return nil
}

type BindingMode string

const (
	BindingCurrent BindingMode = "current"
	BindingPinned  BindingMode = "pinned"
)

type BindingStatus string

const (
	BindingActive    BindingStatus = "active"
	BindingSuspended BindingStatus = "suspended"
	BindingRevoked   BindingStatus = "revoked"
)

type ConsumerBinding struct {
	ID                      identity.ConsumerBindingID
	WorkspaceID             identity.WorkspaceID
	ConsumerID              identity.ConsumerID
	Environment             string
	Purpose                 string
	Mode                    BindingMode
	ReleaseID               *identity.ReleaseID
	CompatibilityConstraint json.RawMessage
	ExpiresAt               *time.Time
	Status                  BindingStatus
	Version                 int
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (binding ConsumerBinding) Validate() error {
	if binding.ID.IsZero() || binding.WorkspaceID.IsZero() || binding.ConsumerID.IsZero() ||
		!validToken(binding.Environment, 80) || strings.TrimSpace(binding.Purpose) == "" ||
		len(binding.Purpose) > 500 || binding.Version < 1 || !validJSONObject(binding.CompatibilityConstraint) {
		return ErrInvalidArgument
	}
	if binding.Status != BindingActive && binding.Status != BindingSuspended && binding.Status != BindingRevoked {
		return ErrInvalidArgument
	}
	if binding.Mode == BindingCurrent && binding.ReleaseID != nil {
		return ErrInvalidArgument
	}
	if binding.Mode == BindingPinned && (binding.ReleaseID == nil || binding.ReleaseID.IsZero()) {
		return ErrInvalidArgument
	}
	if binding.Mode != BindingCurrent && binding.Mode != BindingPinned {
		return ErrInvalidArgument
	}
	return nil
}

type Intent string

const (
	IntentDescribe  Intent = "describe"
	IntentAggregate Intent = "aggregate"
	IntentBreakdown Intent = "breakdown"
	IntentCompare   Intent = "compare"
)

type Selector struct {
	MemberID string            `json:"memberId,omitempty"`
	AssetID  *identity.AssetID `json:"assetId,omitempty"`
	Address  string            `json:"address,omitempty"`
	Search   string            `json:"search,omitempty"`
}

func (selector Selector) validate() bool {
	if selector.MemberID != "" && !regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`).MatchString(selector.MemberID) {
		return false
	}
	count := 0
	if selector.AssetID != nil && !selector.AssetID.IsZero() {
		count++
	}
	if strings.TrimSpace(selector.Address) != "" {
		count++
	}
	if strings.TrimSpace(selector.Search) != "" {
		count++
	}
	return count == 1 && len(selector.Address) <= 240 && len(selector.Search) <= 240
}

type Filter struct {
	Selector Selector        `json:"selector"`
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value"`
}

type TimeRange struct {
	Selector    Selector  `json:"selector"`
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	Granularity string    `json:"granularity,omitempty"`
}

type Order struct {
	Selector  Selector `json:"selector"`
	Direction string   `json:"direction"`
}

type ResolutionMode string

const (
	ResolutionCurrent  ResolutionMode = "current"
	ResolutionExplicit ResolutionMode = "explicit"
	ResolutionBinding  ResolutionMode = "binding"
)

type ResolutionContext struct {
	Mode      ResolutionMode              `json:"mode"`
	ReleaseID *identity.ReleaseID         `json:"releaseId,omitempty"`
	BindingID *identity.ConsumerBindingID `json:"bindingId,omitempty"`
}

type SemanticQueryInput struct {
	ModelID       *identity.AssetID `json:"modelId,omitempty"`
	SchemaVersion string            `json:"schemaVersion"`
	Intent        Intent            `json:"intent"`
	Measures      []Selector        `json:"measures,omitempty"`
	Dimensions    []Selector        `json:"dimensions,omitempty"`
	Filters       []Filter          `json:"filters,omitempty"`
	TimeRange     *TimeRange        `json:"timeRange,omitempty"`
	Order         []Order           `json:"order,omitempty"`
	Limit         int               `json:"limit,omitempty"`
	Context       ResolutionContext `json:"context"`
}

func (query SemanticQueryInput) Validate() error {
	if query.SchemaVersion != QuerySchemaVersion {
		return &ValidationError{Code: RefusalInvalidQuery, Message: "unsupported SemanticQuery schema version"}
	}
	switch query.Intent {
	case IntentDescribe:
		if len(query.Measures)+len(query.Dimensions) == 0 {
			return &ValidationError{Code: RefusalInvalidQuery, Message: "describe requires at least one selector"}
		}
	case IntentAggregate:
		if len(query.Measures) == 0 {
			return &ValidationError{Code: RefusalInvalidQuery, Message: "aggregate requires a measure"}
		}
	case IntentBreakdown:
		if len(query.Measures) == 0 || len(query.Dimensions) == 0 {
			return &ValidationError{Code: RefusalInvalidQuery, Message: "breakdown requires a measure and dimension"}
		}
	case IntentCompare:
		if len(query.Measures) == 0 || len(query.Dimensions)+len(query.Filters) == 0 {
			return &ValidationError{Code: RefusalInvalidQuery, Message: "compare requires a measure and comparison selector"}
		}
	default:
		return &ValidationError{Code: RefusalInvalidQuery, Message: "unknown semantic intent"}
	}
	if len(query.Measures) > 32 || len(query.Dimensions) > 32 || len(query.Filters) > 32 || len(query.Order) > 8 {
		return &ValidationError{Code: RefusalInvalidQuery, Message: "SemanticQuery exceeds selector limits"}
	}
	for _, selector := range append(append([]Selector{}, query.Measures...), query.Dimensions...) {
		if !selector.validate() {
			return &ValidationError{Code: RefusalInvalidQuery, Message: "each selector requires exactly one reference"}
		}
	}
	validOperators := map[string]bool{"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true, "in": true, "not_in": true, "contains": true}
	for _, filter := range query.Filters {
		if !filter.Selector.validate() || !validOperators[filter.Operator] || len(filter.Value) == 0 || !json.Valid(filter.Value) {
			return &ValidationError{Code: RefusalInvalidFilter, Message: "filter semantics are invalid"}
		}
	}
	if query.TimeRange != nil {
		validGranularity := map[string]bool{"": true, "day": true, "week": true, "month": true, "quarter": true, "year": true}
		if !query.TimeRange.Selector.validate() || query.TimeRange.From.IsZero() || query.TimeRange.To.IsZero() ||
			!query.TimeRange.From.Before(query.TimeRange.To) || !validGranularity[query.TimeRange.Granularity] {
			return &ValidationError{Code: RefusalInvalidTimeRange, Message: "time range semantics are invalid"}
		}
	}
	for _, order := range query.Order {
		if !order.Selector.validate() || (order.Direction != "asc" && order.Direction != "desc") {
			return &ValidationError{Code: RefusalInvalidOrdering, Message: "ordering semantics are invalid"}
		}
	}
	if query.Limit < 0 || query.Limit > 10000 {
		return &ValidationError{Code: RefusalInvalidLimit, Message: "limit must be between 0 and 10000"}
	}
	switch query.Context.Mode {
	case ResolutionCurrent:
		if query.Context.ReleaseID != nil || query.Context.BindingID != nil {
			return &ValidationError{Code: RefusalInvalidQuery, Message: "current context cannot name a release or binding"}
		}
	case ResolutionExplicit:
		if query.Context.ReleaseID == nil || query.Context.ReleaseID.IsZero() || query.Context.BindingID != nil {
			return &ValidationError{Code: RefusalInvalidQuery, Message: "explicit context requires one release"}
		}
	case ResolutionBinding:
		if query.Context.BindingID == nil || query.Context.BindingID.IsZero() || query.Context.ReleaseID != nil {
			return &ValidationError{Code: RefusalInvalidQuery, Message: "binding context requires one binding"}
		}
	default:
		return &ValidationError{Code: RefusalInvalidQuery, Message: "unknown resolution context"}
	}
	return nil
}

func (query SemanticQueryInput) Canonical() (json.RawMessage, string, error) {
	if err := query.Validate(); err != nil {
		return nil, "", err
	}
	payload, err := json.Marshal(query)
	if err != nil {
		return nil, "", err
	}
	payload, err = canonicalJSON(payload)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(payload)
	return payload, "sha256:" + hex.EncodeToString(digest[:]), nil
}

type ReleasedAsset struct {
	AssetID       identity.AssetID
	RevisionID    identity.RevisionID
	Address       string
	AssetType     semantic.AssetType
	Name          string
	Aliases       []string
	Content       json.RawMessage
	ContentDigest string
	Position      int
}

type ReleasedPhysicalBinding struct {
	Transform string
	ID        identity.PhysicalBindingID
	Version   int
	AssetID   identity.AssetID
	DatasetID identity.PhysicalDatasetID
	FieldID   *identity.PhysicalFieldID
	Retired   bool
}

type ReleasedModelGrain struct {
	ID              identity.ModelGrainID
	Version         int
	AssetID         identity.AssetID
	GrainExpression string
}

type ReleasedEntityKey struct {
	ID      identity.EntityKeyID
	Version int
	AssetID identity.AssetID
}

type ReleasedJoinContract struct {
	ID             identity.JoinContractID
	Version        int
	LeftDatasetID  identity.PhysicalDatasetID
	RightDatasetID identity.PhysicalDatasetID
	Cardinality    governance.JoinCardinality
	JoinType       governance.JoinType
	JoinExpression string
}

type ReleaseSnapshot struct {
	Execution      *ExecutionProvenance
	ReleaseID      identity.ReleaseID
	WorkspaceID    identity.WorkspaceID
	Sequence       int64
	ManifestDigest string
	PublishedAt    time.Time
	Assets         []ReleasedAsset
	Bindings       []ReleasedPhysicalBinding
	Grains         []ReleasedModelGrain
	Keys           []ReleasedEntityKey
	Joins          []ReleasedJoinContract
}

type SemanticQuery struct {
	ID                identity.SemanticQueryID
	WorkspaceID       identity.WorkspaceID
	PrincipalRef      string
	ConsumerID        *identity.ConsumerID
	BindingID         *identity.ConsumerBindingID
	SelectedReleaseID identity.ReleaseID
	SchemaVersion     string
	ResolverVersion   string
	CanonicalRequest  json.RawMessage
	RequestDigest     string
	Channel           string
	TraceID           string
	IdempotencyKey    string
	Outcome           string
	CreatedAt         time.Time
	FinalizedAt       time.Time
}

type ResolvedAsset struct {
	AssetID    identity.AssetID    `json:"assetId"`
	RevisionID identity.RevisionID `json:"revisionId"`
	Address    string              `json:"address"`
	AssetType  semantic.AssetType  `json:"assetType"`
}

type ResolvedObject struct {
	ObjectType string `json:"objectType"`
	ObjectID   string `json:"objectId"`
	Version    int    `json:"version"`
}

type ResolvedSemanticPlan struct {
	Model           *ResolvedAsset                  `json:"model,omitempty"`
	Execution       *ExecutionProvenance            `json:"execution,omitempty"`
	ID              identity.ResolvedSemanticPlanID `json:"id"`
	QueryID         identity.SemanticQueryID        `json:"queryId"`
	ReleaseID       identity.ReleaseID              `json:"releaseId"`
	ResolverVersion string                          `json:"resolverVersion"`
	Intent          Intent                          `json:"intent"`
	Assets          []ResolvedAsset                 `json:"assets"`
	Objects         []ResolvedObject                `json:"objects"`
	Measures        []Selector                      `json:"measures,omitempty"`
	Filters         []Filter                        `json:"filters,omitempty"`
	Grouping        []Selector                      `json:"grouping,omitempty"`
	TimeRange       *TimeRange                      `json:"timeRange,omitempty"`
	Order           []Order                         `json:"order,omitempty"`
	Limit           int                             `json:"limit,omitempty"`
	ExecutionStatus string                          `json:"executionStatus"`
	PlanDigest      string                          `json:"planDigest"`
	CreatedAt       time.Time                       `json:"createdAt"`
}

type Refusal struct {
	QueryID       identity.SemanticQueryID
	Code          RefusalCode
	CandidateIDs  []identity.AssetID
	Clarification string
	Details       json.RawMessage
	CreatedAt     time.Time
}

type ValidationRun struct {
	ID               identity.QueryValidationRunID    `json:"id"`
	QueryID          identity.SemanticQueryID         `json:"queryId"`
	PlanID           *identity.ResolvedSemanticPlanID `json:"planId,omitempty"`
	Validator        string                           `json:"validator"`
	ValidatorVersion string                           `json:"validatorVersion"`
	InputDigest      string                           `json:"inputDigest"`
	Status           string                           `json:"status"`
	Results          []ValidationResult               `json:"results"`
	CreatedAt        time.Time                        `json:"createdAt"`
	CompletedAt      time.Time                        `json:"completedAt"`
}

type ValidationResult struct {
	Severity string          `json:"severity"`
	Code     string          `json:"code"`
	Message  string          `json:"message"`
	Details  json.RawMessage `json:"details"`
}

func canonicalJSON(input []byte) ([]byte, error) {
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return json.Marshal(decoded)
}

func stableDigest(value any) (json.RawMessage, string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	payload, err = canonicalJSON(payload)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(payload)
	return payload, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func validToken(value string, max int) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' && char != '.' {
			return false
		}
	}
	return true
}

func validJSONObject(value json.RawMessage) bool {
	if len(value) == 0 {
		return true
	}
	var object map[string]any
	return json.Unmarshal(value, &object) == nil
}

func sortedResolvedObjects(objects []ResolvedObject) []ResolvedObject {
	result := append([]ResolvedObject{}, objects...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].ObjectType != result[j].ObjectType {
			return result[i].ObjectType < result[j].ObjectType
		}
		return result[i].ObjectID < result[j].ObjectID
	})
	return result
}

func invalidPlan(message string) error {
	return &ValidationError{Code: RefusalInvalidPlan, Message: fmt.Sprintf("invalid resolved plan: %s", message)}
}

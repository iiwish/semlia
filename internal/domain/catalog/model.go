package catalog

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrInvalidArgument = errors.New("invalid catalog argument")
	ErrNotFound        = semantic.ErrNotFound
	ErrConflict        = semantic.ErrConflict
	ErrInvariant       = semantic.ErrInvariant
)

type Workspace struct {
	ID          identity.WorkspaceID
	Slug        string
	DisplayName string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateWorkspaceCommand struct {
	ID           identity.WorkspaceID
	AuditEventID identity.EventID
	Slug         string
	DisplayName  string
	TraceID      string
	CreatedAt    time.Time
}

type AssetSummary struct {
	ID                identity.AssetID
	Address           semantic.Address
	Type              semantic.AssetType
	LifecycleState    string
	CurrentRevisionID *identity.RevisionID
	Title             string
	Summary           string
	UpdatedAt         time.Time
	Rank              float64
}

type AssetDetail struct {
	AssetSummary
	CreatedAt         time.Time
	CurrentRevision   *Revision
	RelationCount     int
	AuthoritySections []AuthoritySection
}

type Availability string

const (
	AvailabilityAvailable     Availability = "available"
	AvailabilityNotConfigured Availability = "not_configured"
	AvailabilityNotReleased   Availability = "not_released"
	AvailabilityForbidden     Availability = "forbidden"
	AvailabilityFailed        Availability = "failed"
)

type AuthoritySection struct {
	Kind              string
	Authority         string
	Availability      Availability
	RevisionID        *identity.RevisionID
	ReleaseID         *identity.ReleaseID
	ReleaseSequence   *int64
	Relation          *RelationAuthority
	Values            map[string]int64
	Records           []AuthorityRecord
	RecordsLimit      int
	RecordsTotal      int64
	RecordsNextCursor string
}

type RelationAuthority struct {
	Direction      string
	Predicate      string
	Plane          string
	AssertionState string
	SubjectAssetID identity.AssetID
	ObjectAssetID  identity.AssetID
}

type AuthorityRecord struct {
	Kind            string
	ID              string
	Authority       string
	Status          string
	Label           string
	RelatedID       string
	Version         int
	ReleaseID       *identity.ReleaseID
	ReleaseSequence *int64
	Relation        *RelationAuthority
	PhysicalBinding *PhysicalBindingAuthority
	ModelGrain      *ModelGrainAuthority
	EntityKey       *EntityKeyAuthority
	JoinContract    *JoinContractAuthority
	Lineage         *LineageAuthority
	ConsumerBinding *ConsumerBindingAuthority
}

type PhysicalBindingAuthority struct {
	AssetID   identity.AssetID
	DatasetID identity.PhysicalDatasetID
	FieldID   *identity.PhysicalFieldID
	Transform string
}

type ModelGrainAuthority struct {
	AssetID         identity.AssetID
	GrainExpression string
	GrainFieldRefs  []identity.PhysicalFieldID
	DocumentedBy    *identity.EvidenceID
}

type EntityKeyAuthority struct {
	AssetID             identity.AssetID
	KeyFieldRefs        []identity.PhysicalFieldID
	UniquenessSemantics string
}

type JoinContractAuthority struct {
	Direction      string
	LeftDatasetID  identity.PhysicalDatasetID
	RightDatasetID identity.PhysicalDatasetID
	LeftFieldRefs  []identity.PhysicalFieldID
	RightFieldRefs []identity.PhysicalFieldID
	JoinType       string
	Cardinality    string
	JoinExpression string
}

type LineageAuthority struct {
	Direction           string
	UpstreamDatasetID   identity.PhysicalDatasetID
	DownstreamDatasetID identity.PhysicalDatasetID
	EdgeKind            string
	SourceRevisionID    identity.SourceRevisionID
	CodeArtifactID      *identity.CodeArtifactID
	Confidence          float64
}

type ConsumerBindingAuthority struct {
	ConsumerID              identity.ConsumerID
	EffectiveReleaseID      identity.ReleaseID
	Environment             string
	Purpose                 string
	Mode                    string
	Status                  string
	CompatibilityConstraint json.RawMessage
	ExpiresAt               *time.Time
}

type AuthorityRecordCursor struct {
	Kind string
	ID   string
}

type ListAuthorityRecordsQuery struct {
	WorkspaceID identity.WorkspaceID
	AssetID     identity.AssetID
	Section     string
	Limit       int
	Cursor      *AuthorityRecordCursor
}

type AuthorityRecordResult struct {
	Items []AuthorityRecord
	Total int64
}

type Revision struct {
	ID            identity.RevisionID
	AssetID       identity.AssetID
	Sequence      int64
	SchemaVersion string
	ContentDigest string
	Content       json.RawMessage
	CreatedBy     string
	CreatedAt     time.Time
	Evidence      []Evidence
}

type Evidence struct {
	ID               identity.EvidenceID
	EvidenceType     string
	SourceRevisionID *identity.SourceRevisionID
	Locator          string
	ContentDigest    string
	Metadata         json.RawMessage
	Role             string
	FieldPath        string
	Note             string
	CreatedAt        time.Time
}

type Relation struct {
	ID                 identity.RelationID
	Depth              int
	Direction          string
	Predicate          semantic.RelationPredicate
	Plane              semantic.RelationPlane
	AssertionState     semantic.AssertionState
	SubjectAssetID     identity.AssetID
	ObjectAssetID      identity.AssetID
	Counterpart        AssetSummary
	EvidenceArtifactID *identity.EvidenceID
	SourceRevisionID   *identity.SourceRevisionID
	CreatedAt          time.Time
}

type DiscoveryFinding struct {
	Sequence int
	Code     string
	Severity string
	Locator  string
	Details  json.RawMessage
}

type DiscoveryRun struct {
	ID                 identity.RunID
	SourceConnectionID identity.SourceConnectionID
	SourceRevisionID   *identity.SourceRevisionID
	AdapterVersion     string
	Status             string
	ErrorCode          string
	Stats              json.RawMessage
	Findings           []DiscoveryFinding
	StartedAt          *time.Time
	CompletedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type AssetCursor struct {
	Rank      float64
	UpdatedAt time.Time
	ID        identity.AssetID
}

type RevisionCursor struct {
	Sequence int64
	ID       identity.RevisionID
}

type ListAssetsQuery struct {
	WorkspaceID identity.WorkspaceID
	Search      string
	AssetType   semantic.AssetType
	Lifecycle   string
	Limit       int
	Cursor      *AssetCursor
}

type ListRevisionsQuery struct {
	WorkspaceID identity.WorkspaceID
	AssetID     identity.AssetID
	Limit       int
	Cursor      *RevisionCursor
}

type ListRelationsQuery struct {
	WorkspaceID identity.WorkspaceID
	AssetID     identity.AssetID
	Direction   string
	Plane       semantic.RelationPlane
	Depth       int
	Limit       int
}

type CreateAssetCommand struct {
	WorkspaceID   identity.WorkspaceID
	AssetID       identity.AssetID
	RevisionID    identity.RevisionID
	AuditEventID  identity.EventID
	OutboxEventID identity.EventID
	Address       semantic.Address
	AssetType     semantic.AssetType
	Lifecycle     string
	SchemaVersion string
	ContentDigest string
	Content       json.RawMessage
	CreatedBy     string
	EvidenceIDs   []identity.EvidenceID
	TraceID       string
	CreatedAt     time.Time
}

type AppendRevisionCommand struct {
	WorkspaceID   identity.WorkspaceID
	AssetID       identity.AssetID
	RevisionID    identity.RevisionID
	AuditEventID  identity.EventID
	OutboxEventID identity.EventID
	SchemaVersion string
	ContentDigest string
	Content       json.RawMessage
	CreatedBy     string
	EvidenceIDs   []identity.EvidenceID
	TraceID       string
	CreatedAt     time.Time
}

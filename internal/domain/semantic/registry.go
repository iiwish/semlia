package semantic

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrNotFound  = errors.New("semantic registry resource not found")
	ErrConflict  = errors.New("semantic registry resource conflicts with existing state")
	ErrInvariant = errors.New("semantic registry invariant violated")
)

type SourceConnection struct {
	ID                identity.SourceConnectionID
	WorkspaceID       identity.WorkspaceID
	AdapterKind       string
	Name              string
	NormalizedLocator string
	CredentialRef     string
	Status            string
	Metadata          json.RawMessage
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type SourceRevision struct {
	ID                 identity.SourceRevisionID
	WorkspaceID        identity.WorkspaceID
	SourceConnectionID identity.SourceConnectionID
	ExternalRevision   string
	ContentDigest      string
	AdapterVersion     string
	ObservedAt         time.Time
	Metadata           json.RawMessage
	CreatedAt          time.Time
}

type Asset struct {
	ID                identity.AssetID
	WorkspaceID       identity.WorkspaceID
	Address           Address
	Type              AssetType
	LifecycleState    string
	CurrentRevisionID *identity.RevisionID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RevisionRecord struct {
	ID            identity.RevisionID
	WorkspaceID   identity.WorkspaceID
	AssetID       identity.AssetID
	Sequence      int64
	SchemaVersion string
	ContentDigest string
	Content       json.RawMessage
	CreatedBy     string
	CreatedAt     time.Time
}

type EvidenceArtifact struct {
	ID               identity.EvidenceID
	WorkspaceID      identity.WorkspaceID
	EvidenceType     string
	SourceRevisionID *identity.SourceRevisionID
	Locator          string
	ContentDigest    string
	Metadata         json.RawMessage
	CreatedAt        time.Time
}

type RelationRecord struct {
	ID                 identity.RelationID
	WorkspaceID        identity.WorkspaceID
	SubjectAssetID     identity.AssetID
	Predicate          RelationPredicate
	ObjectAssetID      identity.AssetID
	Plane              RelationPlane
	AssertionState     AssertionState
	SourceRevisionID   *identity.SourceRevisionID
	EvidenceArtifactID *identity.EvidenceID
	InferenceRule      string
	CreatedBy          string
	CreatedAt          time.Time
}

type OntologyRevision struct {
	ID            identity.OntologyID
	WorkspaceID   identity.WorkspaceID
	Sequence      int64
	Status        string
	ContentDigest string
	CreatedBy     string
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

type RegistryRepository interface {
	CreateSourceConnection(context.Context, SourceConnection) (SourceConnection, error)
	CreateSourceRevision(context.Context, SourceRevision) (SourceRevision, error)
	CreateAsset(context.Context, Asset) (Asset, error)
	CreateAssetRevision(context.Context, RevisionRecord) (RevisionRecord, error)
	CreateEvidence(context.Context, EvidenceArtifact) (EvidenceArtifact, error)
	CreateRelation(context.Context, RelationRecord) (RelationRecord, error)
	CreateOntologyRevision(context.Context, OntologyRevision) (OntologyRevision, error)
	AddOntologyRelation(context.Context, identity.WorkspaceID, identity.OntologyID, identity.RelationID) error
	PublishOntologyRevision(context.Context, identity.WorkspaceID, identity.OntologyID, time.Time) error
}

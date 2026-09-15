package projection

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
)

var (
	ErrInvalid     = errors.New("invalid projection")
	ErrConflict    = errors.New("projection base conflict")
	ErrDirty       = errors.New("projection repository is dirty")
	ErrUnsupported = errors.New("unsupported projection event")
)

type Asset struct {
	WorkspaceID            identity.WorkspaceID
	AssetID                identity.AssetID
	RevisionID             identity.RevisionID
	ExpectedBaseRevisionID *identity.RevisionID
	Address                semantic.Address
	AssetType              semantic.AssetType
	LifecycleState         string
	Sequence               int64
	SchemaVersion          string
	ContentDigest          string
	Content                json.RawMessage
	CreatedBy              string
	CreatedAt              time.Time
}

type Result struct {
	Path       string
	CommitHash string
	Changed    bool
}

// ManifestAssetProjection is one asset pin of a release manifest, expressed
// entirely in TypeIDs and readable addresses.
type ManifestAssetProjection struct {
	AssetID       identity.AssetID
	RevisionID    identity.RevisionID
	Address       semantic.Address
	Compatibility json.RawMessage
	Position      int
}

// ManifestObjectProjection is one governance-object pin of a release manifest.
type ManifestObjectProjection struct {
	ObjectType string
	ObjectID   string
	Version    int
	Position   int
}

// ReleaseProposalMeta describes the originating proposal of a release.
type ReleaseProposalMeta struct {
	ProposalID       identity.ProposalID
	Title            string
	TargetObjectType string
	TargetObjectID   string
	CreatedBy        string
}

// Release is the read model the Git release projection renders: the immutable
// release aggregate with its manifest and originating proposal metadata.
type Release struct {
	WorkspaceID           identity.WorkspaceID
	ReleaseID             identity.ReleaseID
	Sequence              int64
	ManifestDigest        string
	State                 string
	RolledBackToReleaseID *identity.ReleaseID
	PublishedBy           string
	PublishedAt           time.Time
	Assets                []ManifestAssetProjection
	Objects               []ManifestObjectProjection
	Proposal              *ReleaseProposalMeta
	Production            *ProductionRelease
}

type ProductionRelease struct {
	RootReleaseID        string               `json:"rootReleaseId"`
	RollbackParentID     *string              `json:"rollbackParentId"`
	RollbackDepth        int                  `json:"rollbackDepth"`
	BeforeReleaseID      *string              `json:"beforeReleaseId"`
	BeforeManifest       json.RawMessage      `json:"beforeManifest"`
	BeforeManifestDigest string               `json:"beforeManifestDigest"`
	AttributionDigest    string               `json:"attributionDigest"`
	Proposals            []ProductionProposal `json:"proposals"`
}

type ProductionProposal struct {
	ProposalID        string          `json:"proposalId"`
	OperationID       string          `json:"operationId"`
	ProductionVersion int             `json:"productionVersion"`
	SetDigest         string          `json:"setDigest"`
	ContentDigest     string          `json:"contentDigest"`
	AuthorPrincipalID string          `json:"authorPrincipalId"`
	AttemptNo         int             `json:"attemptNo"`
	ValidationDigest  string          `json:"validationDigest"`
	ReviewIDs         json.RawMessage `json:"reviewIds"`
	Role              string          `json:"role"`
}

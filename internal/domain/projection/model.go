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

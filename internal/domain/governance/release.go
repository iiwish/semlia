package governance

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// ReleaseState is the release's own lifecycle vocabulary. Rows are immutable:
// every cut or rollback inserts a new row, so T002 persists only "published"
// and reserves "rolled_back" in the vocabulary for downstream surfacing.
type ReleaseState string

const (
	ReleasePublished  ReleaseState = "published"
	ReleaseRolledBack ReleaseState = "rolled_back"
)

// ManifestEntry pins one asset into an immutable release manifest with its
// exact revision and the compatibility conclusion.
type ManifestEntry struct {
	AssetID       identity.AssetID
	RevisionID    identity.RevisionID
	Compatibility json.RawMessage
	Position      int
}

func (entry ManifestEntry) Validate() error {
	if entry.AssetID.IsZero() || entry.RevisionID.IsZero() || entry.Position < 1 {
		return ErrInvalidArgument
	}
	if len(entry.Compatibility) == 0 {
		return nil
	}
	if !json.Valid(entry.Compatibility) {
		return ErrInvalidArgument
	}
	return nil
}

type manifestDigestEntry struct {
	AssetID       string          `json:"assetId"`
	RevisionID    string          `json:"revisionId"`
	Compatibility json.RawMessage `json:"compatibility"`
	Position      int             `json:"position"`
}

// ManifestDigestPayload builds the canonical manifest payload whose sha256 is
// the release manifest_digest: entries sorted by position, canonical JSON, so
// the same manifest content always yields the same digest.
func ManifestDigestPayload(entries []ManifestEntry) (json.RawMessage, error) {
	normalized := make([]ManifestEntry, len(entries))
	copy(normalized, entries)
	for _, entry := range normalized {
		if err := entry.Validate(); err != nil {
			return nil, err
		}
	}
	sort.SliceStable(normalized, func(first, second int) bool {
		return normalized[first].Position < normalized[second].Position
	})
	payload := make([]manifestDigestEntry, 0, len(normalized))
	for _, entry := range normalized {
		compatibility := entry.Compatibility
		if len(compatibility) == 0 {
			compatibility = json.RawMessage(`{}`)
		}
		payload = append(payload, manifestDigestEntry{
			AssetID:       entry.AssetID.String(),
			RevisionID:    entry.RevisionID.String(),
			Compatibility: compatibility,
			Position:      entry.Position,
		})
	}
	return CanonicalJSON(mustMarshal(payload))
}

func mustMarshal(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("canonical manifest encoding: %v", err))
	}
	return encoded
}

// ObjectManifestEntry pins one governance object into an immutable release
// manifest with its exact content version. The object identity is the storage
// UUID string; the wire and Git projections convert it to the TypeID form
// (ADR-0003: storage UUIDs never leave the service boundary).
type ObjectManifestEntry struct {
	ObjectType TargetObjectType
	ObjectID   string
	Version    int
	Position   int
}

func (entry ObjectManifestEntry) Validate() error {
	if !entry.ObjectType.IsGovernedObject() || entry.Version < 1 || entry.Position < 1 {
		return ErrInvalidArgument
	}
	if _, err := entry.TypedID(); err != nil {
		return err
	}
	return nil
}

// TypedID converts the pinned storage UUID into the wire TypeID form.
func (entry ObjectManifestEntry) TypedID() (string, error) {
	prefix, err := entry.ObjectType.Prefix()
	if err != nil {
		return "", err
	}
	typed, err := identity.FromUUID(prefix, entry.ObjectID)
	if err != nil {
		return "", fmt.Errorf("%w: object manifest entry %q: %v", ErrInvalidArgument, entry.ObjectID, err)
	}
	return typed.String(), nil
}

type objectManifestDigestEntry struct {
	ObjectType string `json:"objectType"`
	ObjectID   string `json:"objectId"`
	Version    int    `json:"version"`
	Position   int    `json:"position"`
}

// ManifestDigestPayloadWithObjects builds the canonical manifest payload whose
// sha256 is the release manifest_digest for manifests that pin governance
// objects. Asset-only manifests keep the exact T002 payload shape (a bare
// sorted entry array), so digests of asset-only releases are byte-identical
// with the pre-T007 behavior; object pins add the object array wrapper.
func ManifestDigestPayloadWithObjects(entries []ManifestEntry, objects []ObjectManifestEntry) (json.RawMessage, error) {
	for _, object := range objects {
		if err := object.Validate(); err != nil {
			return nil, err
		}
	}
	if positionsInvalidObjects(objects) {
		return nil, ErrInvalidArgument
	}
	if len(objects) == 0 {
		return ManifestDigestPayload(entries)
	}
	assetPayload, err := ManifestDigestPayload(entries)
	if err != nil {
		return nil, err
	}
	sortedObjects := make([]ObjectManifestEntry, len(objects))
	copy(sortedObjects, objects)
	sort.SliceStable(sortedObjects, func(first, second int) bool {
		return sortedObjects[first].Position < sortedObjects[second].Position
	})
	digestObjects := make([]objectManifestDigestEntry, 0, len(sortedObjects))
	for _, object := range sortedObjects {
		digestObjects = append(digestObjects, objectManifestDigestEntry{
			ObjectType: string(object.ObjectType), ObjectID: object.ObjectID,
			Version: object.Version, Position: object.Position,
		})
	}
	payload := struct {
		Assets  json.RawMessage             `json:"assets"`
		Objects []objectManifestDigestEntry `json:"objects"`
	}{Assets: assetPayload, Objects: digestObjects}
	return CanonicalJSON(mustMarshal(payload))
}

func positionsInvalidObjects(entries []ObjectManifestEntry) bool {
	seen := make(map[int]struct{}, len(entries))
	for _, entry := range entries {
		if _, exists := seen[entry.Position]; exists {
			return true
		}
		seen[entry.Position] = struct{}{}
	}
	return false
}

// Release is an immutable manifest snapshot. A rollback is a NEW release row
// whose RolledBackToReleaseID references the release being rolled back; no
// release row is ever updated or deleted (P-003, SSOT §11.4).
type Release struct {
	ID                    identity.ReleaseID
	WorkspaceID           identity.WorkspaceID
	Sequence              int64
	ManifestDigest        string
	State                 ReleaseState
	RolledBackToReleaseID *identity.ReleaseID
	OriginProposalID      *identity.ProposalID
	PublishedBy           string
	PublishedAt           time.Time
	CreatedAt             time.Time
	Entries               []ManifestEntry
	Objects               []ObjectManifestEntry

	// Production protection extensions (SP-T004)
	ProductionRootReleaseID    *identity.ReleaseID
	ProductionRollbackParentID *identity.ReleaseID
	ProductionRollbackDepth    *int
}

func (r Release) IsProductionProtected() bool {
	return r.ProductionRootReleaseID != nil
}

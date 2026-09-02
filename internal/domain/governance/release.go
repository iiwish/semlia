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
	PublishedBy           string
	PublishedAt           time.Time
	CreatedAt             time.Time
	Entries               []ManifestEntry
}

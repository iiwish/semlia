package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) productionManifestPinTx(ctx context.Context, tx pgx.Tx, release domain.Release, kind, target string) (domain.ProductionReleaseBeforePin, error) {
	pin := domain.ProductionReleaseBeforePin{WorkspaceID: release.WorkspaceID, TargetKind: kind, TargetID: target, Presence: domain.PresenceAbsent}
	for _, asset := range release.Entries {
		if kind != domain.TargetKindSemanticAsset || asset.AssetID.String() != target {
			continue
		}
		var content []byte
		var stored string
		if err := tx.QueryRow(ctx, `SELECT content,content_digest FROM asset_revisions WHERE workspace_id=$1 AND asset_id=$2 AND id=$3`, release.WorkspaceID.UUID(), asset.AssetID.UUID(), asset.RevisionID.UUID()).Scan(&content, &stored); err != nil {
			return pin, err
		}
		digest, err := domain.DigestJSON(content)
		if err != nil || digest != stored {
			return pin, domain.ErrPriorStateUnknown
		}
		revision := asset.RevisionID
		pin.Presence, pin.RevisionID, pin.ContentDigest = domain.PresencePresent, &revision, &digest
	}
	for _, object := range release.Objects {
		id, err := object.TypedID()
		if err != nil {
			return pin, err
		}
		if string(object.ObjectType) != kind || id != target {
			continue
		}
		var content []byte
		if err := tx.QueryRow(ctx, `SELECT payload FROM release_object_snapshots WHERE workspace_id=$1 AND release_id=$2 AND object_type=$3 AND object_id=$4 AND version=$5`, release.WorkspaceID.UUID(), release.ID.UUID(), kind, object.ObjectID, object.Version).Scan(&content); err != nil {
			return pin, domain.ErrPriorStateUnknown
		}
		digest, err := domain.DigestJSON(content)
		if err != nil {
			return pin, err
		}
		version := object.Version
		pin.Presence, pin.ObjectVersion, pin.ContentDigest = domain.PresencePresent, &version, &digest
	}
	return pin, nil
}

func (s *Store) productionRollbackPinsTx(ctx context.Context, tx pgx.Tx, before, after domain.Release, targets []domain.ProductionTarget, oldPins []domain.ProductionReleaseBeforePin) ([]domain.ProductionReleaseBeforePin, error) {
	if len(targets) != len(oldPins) {
		return nil, domain.ErrPriorStateUnknown
	}
	pins := []domain.ProductionReleaseBeforePin{}
	for _, target := range targets {
		pin, err := s.productionManifestPinTx(ctx, tx, before, target.Kind, target.TargetID)
		if err != nil {
			return nil, err
		}
		pin.ReleaseID, pin.CreatedAt = after.ID, after.CreatedAt
		pins = append(pins, pin)
		// Every original before pin must agree with the complete restored view.
		found := false
		for _, old := range oldPins {
			id, err := parseUUIDOrTypeID(old.TargetID)
			if err != nil {
				return nil, domain.ErrPriorStateUnknown
			}
			typed, err := productionTargetID(old.TargetKind, id)
			if err != nil {
				return nil, err
			}
			if old.TargetKind != target.Kind || typed != target.TargetID {
				continue
			}
			if found {
				return nil, domain.ErrPriorStateUnknown
			}
			found = true
			present := false
			for _, asset := range after.Entries {
				if target.Kind == domain.TargetKindSemanticAsset && asset.AssetID.String() == typed {
					present = true
					if old.RevisionID == nil || *old.RevisionID != asset.RevisionID {
						return nil, domain.ErrPriorStateUnknown
					}
				}
			}
			for _, object := range after.Objects {
				objectID, err := object.TypedID()
				if err != nil {
					return nil, err
				}
				if string(object.ObjectType) == target.Kind && objectID == typed {
					present = true
					if old.ObjectVersion == nil || *old.ObjectVersion != object.Version {
						return nil, domain.ErrPriorStateUnknown
					}
				}
			}
			if present != (old.Presence == domain.PresencePresent) {
				return nil, domain.ErrPriorStateUnknown
			}
		}
		if !found {
			return nil, domain.ErrPriorStateUnknown
		}
	}
	return pins, nil
}

func (s *Store) productionRestoreManifestTx(ctx context.Context, tx pgx.Tx, before, after domain.Release, origin *identity.ReleaseID, targets []domain.ProductionTarget, trace string) error {
	assetPins := map[string]domain.ManifestEntry{}
	for _, asset := range after.Entries {
		assetPins[asset.AssetID.UUID()] = asset
	}
	// Verify all restored revisions, including unchanged entries. Their immutable
	// content and historical snapshots are the authority, never current registry data.
	for _, asset := range after.Entries {
		if _, err := s.productionManifestPinTx(ctx, tx, after, domain.TargetKindSemanticAsset, asset.AssetID.String()); err != nil {
			return err
		}
	}
	objectContent := map[string]json.RawMessage{}
	for _, object := range after.Objects {
		if origin == nil {
			return domain.ErrPriorStateUnknown
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT payload FROM release_object_snapshots WHERE workspace_id=$1 AND release_id=$2 AND object_type=$3 AND object_id=$4 AND version=$5`, after.WorkspaceID.UUID(), origin.UUID(), string(object.ObjectType), object.ObjectID, object.Version).Scan(&raw); err != nil {
			return domain.ErrPriorStateUnknown
		}
		var snapshot map[string]json.RawMessage
		if json.Unmarshal(raw, &snapshot) != nil {
			return domain.ErrPriorStateUnknown
		}
		content, err := productionObjectBusinessBaseline(string(object.ObjectType), snapshot)
		if err != nil {
			return err
		}
		var owner string
		if value, ok := snapshot["asset_id"]; ok {
			if json.Unmarshal(value, &owner) != nil {
				return domain.ErrPriorStateUnknown
			}
			if _, ok := assetPins[owner]; !ok {
				return domain.ErrDependencyInvalid
			}
		}
		objectContent[string(object.ObjectType)+":"+object.ObjectID] = content
	}
	for _, target := range targets {
		id, err := parseUUIDOrTypeID(target.TargetID)
		if err != nil {
			return err
		}
		if target.Kind == domain.TargetKindSemanticAsset {
			var prior pgtype.UUID
			if err := tx.QueryRow(ctx, `SELECT current_revision_id FROM semantic_assets WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, after.WorkspaceID.UUID(), id).Scan(&prior); err != nil {
				return err
			}
			var expected pgtype.UUID
			for _, asset := range before.Entries {
				if asset.AssetID.UUID() == formatUUID(id) {
					expected, _ = uuidFromString(asset.RevisionID.UUID())
				}
			}
			if prior != expected {
				return domain.ErrBaselineConflict
			}
			var restored pgtype.UUID
			state := "draft"
			if asset, ok := assetPins[formatUUID(id)]; ok {
				restored, _ = uuidFromString(asset.RevisionID.UUID())
				state = "active"
			}
			if _, err := tx.Exec(ctx, `UPDATE semantic_assets SET current_revision_id=$3,lifecycle_state=$4,updated_at=$5 WHERE workspace_id=$1 AND id=$2`, after.WorkspaceID.UUID(), id, restored, state, after.CreatedAt); err != nil {
				return err
			}
			continue
		}
		content, exists := objectContent[target.Kind+":"+formatUUID(id)]
		if !exists {
			continue
		}
		table, columns, values, err := productionObjectColumns(target.Kind, content)
		if err != nil {
			return err
		}
		var current int
		if err := tx.QueryRow(ctx, `SELECT version FROM `+table+` WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, after.WorkspaceID.UUID(), id).Scan(&current); err != nil {
			return err
		}
		if current >= 2147483647 {
			return domain.ErrLimitExceeded
		}
		args := []any{after.WorkspaceID.UUID(), id}
		args = append(args, values...)
		columns = append(columns, "content", "updated_at")
		args = append(args, content, after.CreatedAt)
		sets := []string{}
		for i, column := range columns {
			sets = append(sets, fmt.Sprintf("%s=$%d", column, i+3))
		}
		if _, err := tx.Exec(ctx, `UPDATE `+table+` SET `+strings.Join(sets, ",")+`,version=version+1 WHERE workspace_id=$1 AND id=$2`, args...); err != nil {
			return err
		}
		if err := s.productionObjectAuditTx(ctx, tx, after.WorkspaceID, target.Kind, target.TargetID, "restored", current+1, after.PublishedBy, trace, after.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

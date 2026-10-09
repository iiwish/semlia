package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
)

func validateKnowledgePublication(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, targetID string, kind semantic.AssetType, raw json.RawMessage) error {
	var content struct {
		AssetType  semantic.AssetType `json:"assetType"`
		Definition string             `json:"definition"`
		Scope      string             `json:"scope"`
		Spec       json.RawMessage    `json:"spec"`
	}
	if json.Unmarshal(raw, &content) != nil || content.AssetType != kind || strings.TrimSpace(content.Definition) == "" || strings.TrimSpace(content.Scope) == "" {
		return governance.ErrInvalidArgument
	}
	spec, err := semantic.ParseKnowledgeSpec(kind, content.Spec, true)
	if err != nil {
		return fmt.Errorf("%w: %s", governance.ErrInvalidArgument, err)
	}
	vocabulary, err := semantic.ParseVocabulary(raw)
	if err != nil {
		return fmt.Errorf("%w: %s", governance.ErrInvalidArgument, err)
	}
	for _, ref := range vocabulary.References() {
		if ref.AssetID == targetID && ref.MemberID == "" {
			return governance.ErrDependencyInvalid
		}
		if err := requirePublishedKnowledgeRef(ctx, tx, workspace, ref); err != nil {
			return err
		}
	}
	if kind == semantic.DataAsset {
		refs := []semantic.SourceReference{*spec.DatasetRef}
		for _, member := range spec.Members {
			refs = append(refs, *member.SourceFieldRef)
		}
		dataset, _ := identity.ParsePhysicalDatasetID(spec.DatasetRef.ObjectID)
		datasetRevision, _ := identity.ParsePhysicalDatasetRevisionID(spec.DatasetRef.RevisionID)
		for _, ref := range refs {
			snapshot, _ := identity.ParseSourceSnapshotID(ref.SnapshotID)
			object, _ := parseUUIDOrTypeID(ref.ObjectID)
			revision, _ := parseUUIDOrTypeID(ref.RevisionID)
			var found bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_snapshot_members m JOIN source_snapshots s ON s.workspace_id=m.workspace_id AND s.id=m.snapshot_id JOIN source_snapshot_scope c ON c.workspace_id=m.workspace_id AND c.snapshot_id=m.snapshot_id AND c.coverage_key=m.coverage_key WHERE m.workspace_id=$1 AND m.snapshot_id=$2 AND m.kind=$3 AND m.object_id=$4 AND m.revision_id=$5 AND s.history_quality='verified' AND c.status='complete' AND c.enumeration_complete AND (m.kind='dataset' OR (m.parent_object_id=$6 AND m.parent_revision_id=$7)))`, workspace.UUID(), snapshot.UUID(), ref.Kind, object, revision, dataset.UUID(), datasetRevision.UUID()).Scan(&found); err != nil {
				return err
			}
			if !found {
				return governance.ErrDependencyInvalid
			}
		}
	}
	return spec.ValidateReferences(func(ref semantic.KnowledgeReference) (semantic.AssetType, semantic.KnowledgeSpec, error) {
		if ref.AssetID == targetID {
			return "", semantic.KnowledgeSpec{}, governance.ErrDependencyInvalid
		}
		asset, _ := identity.ParseAssetID(ref.AssetID)
		revision, _ := identity.ParseRevisionID(ref.RevisionID)
		release, _ := identity.ParseReleaseID(ref.ReleaseID)
		var targetKind semantic.AssetType
		var body []byte
		err := tx.QueryRow(ctx, `SELECT a.asset_type,r.content->'spec' FROM release_assets p JOIN semantic_assets a ON a.workspace_id=p.workspace_id AND a.id=p.asset_id JOIN asset_revisions r ON r.workspace_id=p.workspace_id AND r.id=p.revision_id WHERE p.workspace_id=$1 AND p.release_id=$2 AND p.asset_id=$3 AND p.revision_id=$4`, workspace.UUID(), release.UUID(), asset.UUID(), revision.UUID()).Scan(&targetKind, &body)
		if err != nil {
			return "", semantic.KnowledgeSpec{}, governance.ErrDependencyInvalid
		}
		targetSpec, err := semantic.ParseKnowledgeSpec(targetKind, body, true)
		return targetKind, targetSpec, err
	})
}

func requirePublishedKnowledgeRef(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, ref semantic.KnowledgeReference) error {
	asset, _ := identity.ParseAssetID(ref.AssetID)
	revision, _ := identity.ParseRevisionID(ref.RevisionID)
	release, _ := identity.ParseReleaseID(ref.ReleaseID)
	var found bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM release_assets WHERE workspace_id=$1 AND release_id=$2 AND asset_id=$3 AND revision_id=$4)`, workspace.UUID(), release.UUID(), asset.UUID(), revision.UUID()).Scan(&found); err != nil {
		return err
	}
	if !found {
		return governance.ErrDependencyInvalid
	}
	return nil
}

// releaseVocabularyEntries loads the conflict-check view of every asset pinned by one release.
func releaseVocabularyEntries(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, release identity.ReleaseID) ([]semantic.VocabularyEntry, error) {
	rows, err := tx.Query(ctx, `SELECT a.id,a.asset_type,r.content FROM release_assets e JOIN semantic_assets a ON a.workspace_id=e.workspace_id AND a.id=e.asset_id JOIN asset_revisions r ON r.workspace_id=e.workspace_id AND r.id=e.revision_id AND r.asset_id=e.asset_id WHERE e.workspace_id=$1 AND e.release_id=$2`, workspace.UUID(), release.UUID())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []semantic.VocabularyEntry{}
	for rows.Next() {
		var id pgtype.UUID
		var kind string
		var content []byte
		if err := rows.Scan(&id, &kind, &content); err != nil {
			return nil, err
		}
		asset, err := identity.AssetIDFromUUIDBytes(id.Bytes)
		if err != nil {
			return nil, err
		}
		entry, err := semantic.VocabularyEntryFromContent(asset.String(), semantic.AssetType(kind), content)
		if err != nil {
			// Content published before vocabulary existed is not re-judged here; its own publication gate applied.
			continue
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// checkReleaseVocabularyTx is the authoritative publication gate: within one release a term may name only one asset
// of a type unless one side declares the other confusable. Only conflicts involving an asset this release changed
// block it, so an overlap that predates the gate never stops unrelated publications.
func checkReleaseVocabularyTx(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, release identity.ReleaseID, changed map[string]bool) error {
	if len(changed) == 0 {
		return nil
	}
	entries, err := releaseVocabularyEntries(ctx, tx, workspace, release)
	if err != nil {
		return governanceRepositoryError("load release vocabulary", err)
	}
	blocking := []semantic.VocabularyConflict{}
	for _, conflict := range semantic.VocabularyConflicts(entries) {
		if changed[conflict.Assets[0].AssetID] || changed[conflict.Assets[1].AssetID] {
			blocking = append(blocking, conflict)
		}
	}
	if len(blocking) > 0 {
		return &governance.VocabularyConflictError{Conflicts: blocking}
	}
	return nil
}

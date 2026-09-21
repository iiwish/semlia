package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
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

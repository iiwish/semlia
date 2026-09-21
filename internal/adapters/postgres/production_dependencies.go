package postgres

import (
	"context"
	"encoding/json"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/internal/domain/semantic"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func validateProductionDependencies(ctx context.Context, tx pgx.Tx, access authapp.AccessSnapshot, version domain.ProductionVersion, targets []domain.ProductionTarget, dependencies []json.RawMessage, fresh bool) error {
	w := version.WorkspaceID
	selected := map[string]domain.ProductionPublishedReference{}
	for _, raw := range dependencies {
		ref, err := domain.ParseProductionPublishedReference(raw)
		if err != nil {
			return err
		}
		key := ref.Kind + "/" + ref.TargetID
		if _, exists := selected[key]; exists {
			return domain.ErrInvalidArgument
		}
		selected[key] = ref
		release, _ := identity.ParseReleaseID(ref.ReleaseID)
		var present pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM releases WHERE workspace_id=$1 AND id=$2`, w.UUID(), release.UUID()).Scan(&present); err != nil {
			return governanceRepositoryError("read dependency release", err)
		}
		checkHead := fresh || version.BaselineHead.Presence != ""
		if checkHead {
			if version.BaselineHead.ReleaseID == nil || (ref.Kind != domain.TargetKindSemanticAsset && *version.BaselineHead.ReleaseID != release) {
				return domain.ErrDependencyInvalid
			}
		}
		id, err := parseUUIDOrTypeID(ref.TargetID)
		if err != nil {
			return domain.ErrInvalidArgument
		}
		if ref.Kind == domain.TargetKindSemanticAsset {
			revision, _ := identity.ParseRevisionID(ref.RevisionID)
			var found bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM release_assets WHERE workspace_id=$1 AND release_id=$2 AND asset_id=$3 AND revision_id=$4)`, w.UUID(), release.UUID(), id, revision.UUID()).Scan(&found); err != nil {
				return err
			}
			if !found {
				return domain.ErrDependencyInvalid
			}
			// A reference keeps its original immutable release. It remains usable
			// only while that exact revision is also present in the authoring head.
			if checkHead {
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM release_assets WHERE workspace_id=$1 AND release_id=$2 AND asset_id=$3 AND revision_id=$4)`, w.UUID(), version.BaselineHead.ReleaseID.UUID(), id, revision.UUID()).Scan(&found); err != nil {
					return err
				}
				if !found {
					return domain.ErrDependencyInvalid
				}
			}
			resource, err := productionAssetResource(ctx, tx, w, ref.TargetID)
			if err != nil {
				return err
			}
			if err := productionRequire(access, authz.ActionAssetRead, resource); err != nil {
				return err
			}
			rows, err := tx.Query(ctx, `SELECT r.source_connection_id FROM revision_evidence_links l JOIN evidence_artifacts e ON e.workspace_id=l.workspace_id AND e.id=l.evidence_artifact_id LEFT JOIN source_revisions r ON r.workspace_id=e.workspace_id AND r.id=e.source_revision_id WHERE l.workspace_id=$1 AND l.asset_revision_id=$2`, w.UUID(), revision.UUID())
			if err != nil {
				return err
			}
			for rows.Next() {
				var source pgtype.UUID
				if err := rows.Scan(&source); err != nil {
					rows.Close()
					return err
				}
				resource := authz.Resource{Type: authz.ScopeWorkspace, ID: w.UUID()}
				if source.Valid {
					resource = authz.Resource{Type: authz.ScopeSource, ID: formatUUID(source)}
					if err := productionRequire(access, authz.ActionSourceRead, resource); err != nil {
						rows.Close()
						return err
					}
				}
				if err := productionRequire(access, authz.ActionEvidenceRead, resource); err != nil {
					rows.Close()
					return err
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		} else {
			var payload []byte
			if err := tx.QueryRow(ctx, `SELECT payload FROM release_object_snapshots WHERE workspace_id=$1 AND release_id=$2 AND object_type=$3 AND object_id=$4 AND version=$5`, w.UUID(), release.UUID(), ref.Kind, id, ref.ObjectVersion).Scan(&payload); err != nil {
				return governanceRepositoryError("read dependency object snapshot", err)
			}
			var object map[string]json.RawMessage
			if json.Unmarshal(payload, &object) != nil {
				return domain.ErrPriorStateUnknown
			}
			business, err := productionObjectBusinessBaseline(ref.Kind, object)
			if err != nil {
				return err
			}
			digest, err := domain.DigestJSON(business)
			if err != nil || digest != ref.ContentDigest {
				return domain.ErrDependencyInvalid
			}
			resources, err := productionBusinessResources(ctx, tx, w, ref.Kind, business)
			if err != nil {
				return err
			}
			for _, resource := range resources {
				readAction := authz.ActionAssetRead
				if ref.Kind == domain.TargetKindJoinContract {
					readAction = authz.ActionSourceRead
				}
				if err := productionRequire(access, readAction, resource); err != nil {
					return err
				}
				if err := productionRequire(access, authz.ActionBindingRead, resource); err != nil {
					return err
				}
			}
		}
	}
	for _, target := range targets {
		if target.Declaration == nil {
			return domain.ErrPriorStateUnknown
		}
		refs, err := domain.InspectProductionContent(target.Kind, target.Declaration.Content)
		if err != nil {
			return err
		}
		for _, ref := range refs.Published {
			if selected[ref.Kind+"/"+ref.TargetID] != ref {
				return domain.ErrDependencyInvalid
			}
		}
		if target.Kind == domain.TargetKindSemanticAsset {
			var content struct {
				AssetType semantic.AssetType     `json:"assetType"`
				Spec      semantic.KnowledgeSpec `json:"spec"`
			}
			if err := json.Unmarshal(target.Declaration.Content, &content); err != nil {
				return err
			}
			resolvedTypes := map[string]semantic.AssetType{}
			resolvedSpecs := map[string]semantic.KnowledgeSpec{}
			for _, ref := range content.Spec.References() {
				if ref.AssetID == target.TargetID {
					return domain.ErrDependencyInvalid
				}
				revision, _ := identity.ParseRevisionID(ref.RevisionID)
				var raw []byte
				var assetType string
				if err := tx.QueryRow(ctx, `SELECT a.asset_type,r.content FROM asset_revisions r JOIN semantic_assets a ON a.workspace_id=r.workspace_id AND a.id=r.asset_id WHERE r.workspace_id=$1 AND r.id=$2`, w.UUID(), revision.UUID()).Scan(&assetType, &raw); err != nil {
					return governanceRepositoryError("read knowledge member", err)
				}
				var dependency struct {
					Spec semantic.KnowledgeSpec `json:"spec"`
				}
				if json.Unmarshal(raw, &dependency) != nil {
					return domain.ErrDependencyInvalid
				}
				resolvedTypes[ref.AssetID] = semantic.AssetType(assetType)
				resolvedSpecs[ref.AssetID] = dependency.Spec
				if ref.MemberID != "" {
					if assetType != "business_object" && assetType != "data_asset" {
						return domain.ErrDependencyInvalid
					}
					found := false
					for _, member := range dependency.Spec.Members {
						if member.ID == ref.MemberID {
							found = true
						}
					}
					if !found {
						return domain.ErrDependencyInvalid
					}
				}
			}
			if err := content.Spec.ValidateReferences(func(ref semantic.KnowledgeReference) (semantic.AssetType, semantic.KnowledgeSpec, error) {
				return resolvedTypes[ref.AssetID], resolvedSpecs[ref.AssetID], nil
			}); err != nil {
				return domain.ErrDependencyInvalid
			}
		}
	}
	return nil
}

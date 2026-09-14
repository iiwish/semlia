package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type productionSnapshotPin struct {
	source, revision pgtype.UUID
	coverage         map[string]bool
}

func productionAssetResource(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, id string) (authz.Resource, error) {
	asset, err := identity.ParseAssetID(id)
	if err != nil {
		return authz.Resource{}, domain.ErrInvalidArgument
	}
	var namespace string
	if err := tx.QueryRow(ctx, `SELECT namespace FROM semantic_assets WHERE workspace_id=$1 AND id=$2`, w.UUID(), asset.UUID()).Scan(&namespace); err != nil {
		return authz.Resource{}, governanceRepositoryError("read target asset", err)
	}
	return authz.Resource{Type: authz.ScopeAsset, ID: asset.UUID(), DomainID: namespace}, nil
}

func authorizeProductionTargets(ctx context.Context, tx pgx.Tx, access authapp.AccessSnapshot, w identity.WorkspaceID, targets []domain.ProductionTarget, fresh bool, baselineJSON ...json.RawMessage) error {
	var baseline struct {
		Targets map[string]domain.ProductionTargetBaseline `json:"targets"`
	}
	if len(baselineJSON) > 0 && len(baselineJSON[0]) > 0 {
		if err := json.Unmarshal(baselineJSON[0], &baseline); err != nil {
			return domain.ErrPriorStateUnknown
		}
	}
	local := map[string]domain.ProductionTarget{}
	for _, target := range targets {
		local[target.LocalKey] = target
	}
	createResource := func(target domain.ProductionTarget) (authz.Resource, error) {
		if target.IdentityKey == nil {
			return authz.Resource{}, domain.ErrInvalidArgument
		}
		at := strings.LastIndex(*target.IdentityKey, ".")
		if at < 1 {
			return authz.Resource{}, domain.ErrInvalidArgument
		}
		id := target.TargetID
		if id != "" {
			asset, err := identity.ParseAssetID(id)
			if err != nil {
				return authz.Resource{}, domain.ErrInvalidArgument
			}
			id = asset.UUID()
		}
		return authz.Resource{Type: authz.ScopeAsset, ID: id, DomainID: (*target.IdentityKey)[:at]}, nil
	}
	for _, target := range targets {
		var resources []authz.Resource
		if target.Intent == domain.ProductionIntentUpdate && target.Kind != domain.TargetKindSemanticAsset {
			if previous, ok := baseline.Targets[target.LocalKey]; ok {
				oldResources, err := productionBusinessResources(ctx, tx, w, target.Kind, previous.Content)
				if err != nil {
					return err
				}
				resources = append(resources, oldResources...)
			}
		}
		if target.Kind == domain.TargetKindSemanticAsset {
			var resource authz.Resource
			var err error
			if target.Intent == domain.ProductionIntentCreate {
				resource, err = createResource(target)
			} else {
				resource, err = productionAssetResource(ctx, tx, w, target.TargetID)
			}
			if err != nil {
				return err
			}
			resources = append(resources, resource)
		}
		if target.Declaration == nil {
			return domain.ErrPriorStateUnknown
		}
		refs, err := domain.InspectProductionContent(target.Kind, target.Declaration.Content)
		if err != nil {
			return err
		}
		if refs.OwnerPrincipalID != "" {
			owner, _ := identity.ParsePrincipalID(refs.OwnerPrincipalID)
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM principals WHERE workspace_id=$1 AND id=$2)`, w.UUID(), owner.UUID()).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return domain.ErrNotFound
			}
		}
		for _, key := range refs.LocalKeys {
			ref, ok := local[key]
			if !ok || ref.Kind != domain.TargetKindSemanticAsset {
				return domain.ErrDependencyInvalid
			}
			resource, err := createResource(ref)
			if err != nil {
				return err
			}
			resources = append(resources, resource)
		}
		for _, ref := range refs.Published {
			resource, err := productionAssetResource(ctx, tx, w, ref.TargetID)
			if err != nil {
				return err
			}
			resources = append(resources, resource)
		}
		if target.Kind == domain.TargetKindJoinContract {
			for _, ref := range refs.Physical {
				var source pgtype.UUID
				if err := tx.QueryRow(ctx, `SELECT source_connection_id FROM source_snapshots WHERE workspace_id=$1 AND id=$2`, w.UUID(), mustSnapshotUUID(ref.SnapshotID)).Scan(&source); err != nil {
					return governanceRepositoryError("read join source", err)
				}
				resources = append(resources, authz.Resource{Type: authz.ScopeSource, ID: formatUUID(source)})
			}
		}
		if len(resources) == 0 {
			return domain.ErrDependencyInvalid
		}
		for _, resource := range resources {
			readAction := authz.ActionAssetRead
			if target.Kind == domain.TargetKindJoinContract {
				readAction = authz.ActionSourceRead
			}
			if err := productionRequire(access, readAction, resource); err != nil {
				return err
			}
			if fresh && target.Kind != domain.TargetKindJoinContract {
				if err := productionRequire(access, authz.ActionAssetPropose, resource); err != nil {
					return err
				}
			}
			if target.Kind != domain.TargetKindSemanticAsset {
				if err := productionRequire(access, authz.ActionBindingRead, resource); err != nil {
					return err
				}
				if fresh {
					if err := productionRequire(access, authz.ActionBindingManage, resource); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validateProductionPhysicalReferences(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, targets []domain.ProductionTarget, snapshots map[string]productionSnapshotPin, evidence map[string]bool) error {
	for _, target := range targets {
		if target.Declaration == nil {
			return domain.ErrPriorStateUnknown
		}
		for _, id := range target.Declaration.EvidenceIDs {
			if !evidence[id] {
				return domain.ErrEvidenceMissing
			}
		}
		refs, err := domain.InspectProductionContent(target.Kind, target.Declaration.Content)
		if err != nil {
			return err
		}
		parents := map[string]string{}
		for _, ref := range refs.Physical {
			pin, ok := snapshots[ref.SnapshotID]
			if !ok {
				return domain.ErrInputIncomplete
			}
			object, err := parseUUIDOrTypeID(ref.ObjectID)
			if err != nil {
				return domain.ErrInvalidArgument
			}
			revision, err := parseUUIDOrTypeID(ref.RevisionID)
			if err != nil {
				return domain.ErrInvalidArgument
			}
			var key string
			var parent, parentRevision pgtype.UUID
			if err := tx.QueryRow(ctx, `SELECT coverage_key,parent_object_id,parent_revision_id FROM source_snapshot_members WHERE workspace_id=$1 AND snapshot_id=$2 AND kind=$3 AND object_id=$4 AND revision_id=$5`, w.UUID(), mustSnapshotUUID(ref.SnapshotID), ref.Kind, object, revision).Scan(&key, &parent, &parentRevision); err != nil {
				return governanceRepositoryError("read physical member pin", err)
			}
			if !pin.coverage[key] {
				return domain.ErrInputIncomplete
			}
			if ref.Kind == "field" {
				var parentKey string
				if !parent.Valid || !parentRevision.Valid {
					return domain.ErrDependencyInvalid
				}
				if err := tx.QueryRow(ctx, `SELECT coverage_key FROM source_snapshot_members WHERE workspace_id=$1 AND snapshot_id=$2 AND kind='dataset' AND object_id=$3 AND revision_id=$4`, w.UUID(), mustSnapshotUUID(ref.SnapshotID), parent, parentRevision).Scan(&parentKey); err != nil {
					return domain.ErrDependencyInvalid
				}
				if !pin.coverage[parentKey] {
					return domain.ErrInputIncomplete
				}
				parents[ref.SnapshotID+"/"+ref.ObjectID] = formatUUID(parent) + "/" + formatUUID(parentRevision)
			}
		}
		var content struct {
			Dataset domain.ProductionPhysicalReference  `json:"dataset"`
			Field   *domain.ProductionPhysicalReference `json:"field"`
			Left    domain.ProductionPhysicalReference  `json:"leftDataset"`
			Right   domain.ProductionPhysicalReference  `json:"rightDataset"`
			Pairs   []struct {
				Left  domain.ProductionPhysicalReference `json:"left"`
				Right domain.ProductionPhysicalReference `json:"right"`
			} `json:"pairs"`
		}
		if err := json.Unmarshal(target.Declaration.Content, &content); err != nil {
			return err
		}
		belongs := func(field, dataset domain.ProductionPhysicalReference) bool {
			object, e1 := parseUUIDOrTypeID(dataset.ObjectID)
			revision, e2 := parseUUIDOrTypeID(dataset.RevisionID)
			return e1 == nil && e2 == nil && field.SnapshotID == dataset.SnapshotID && parents[field.SnapshotID+"/"+field.ObjectID] == formatUUID(object)+"/"+formatUUID(revision)
		}
		if content.Field != nil && !belongs(*content.Field, content.Dataset) {
			return fmt.Errorf("%w: field does not belong to selected dataset revision", domain.ErrDependencyInvalid)
		}
		for _, pair := range content.Pairs {
			if !belongs(pair.Left, content.Left) || !belongs(pair.Right, content.Right) {
				return domain.ErrDependencyInvalid
			}
		}
	}
	return nil
}

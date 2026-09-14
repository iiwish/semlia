package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type productionBaselinePin struct {
	Kind          string `json:"kind"`
	TargetID      string `json:"targetId"`
	RevisionID    string `json:"revisionId,omitempty"`
	Version       int    `json:"version,omitempty"`
	ContentDigest string `json:"contentDigest"`
}

func (s *Store) PrepareProductionBaseline(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, declarations []domain.TargetDeclaration) (domain.ProductionBaseline, error) {
	return s.PrepareProductionBaselineForOperation(ctx, workspace, principal, identity.ProductionOperationID{}, declarations)
}

func (s *Store) PrepareProductionBaselineForOperation(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, operation identity.ProductionOperationID, declarations []domain.TargetDeclaration) (domain.ProductionBaseline, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.ProductionBaseline{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM workspaces WHERE id=$1 FOR UPDATE`, workspace.UUID()).Scan(&locked); err != nil {
		return domain.ProductionBaseline{}, governanceRepositoryError("lock baseline workspace", err)
	}
	result, err := loadProductionBaseline(ctx, tx, workspace, declarations, operation)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func loadProductionBaseline(ctx context.Context, tx pgx.Tx, workspace identity.WorkspaceID, declarations []domain.TargetDeclaration, owners ...identity.ProductionOperationID) (domain.ProductionBaseline, error) {
	result := domain.ProductionBaseline{Head: domain.HeadReference{Presence: domain.PresenceAbsent}, Targets: map[string]domain.ProductionTargetBaseline{}}
	var release pgtype.UUID
	var manifestDigest string
	err := tx.QueryRow(ctx, `SELECT id,manifest_digest FROM releases WHERE workspace_id=$1 ORDER BY sequence DESC LIMIT 1`, workspace.UUID()).Scan(&release, &manifestDigest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	pins := []productionBaselinePin{}
	if err == nil {
		id, err := identity.ReleaseIDFromUUIDBytes(release.Bytes)
		if err != nil {
			return result, err
		}
		result.Head = domain.HeadReference{Presence: domain.PresencePresent, ReleaseID: &id, ManifestDigest: &manifestDigest}
		rows, err := tx.Query(ctx, `SELECT 'semantic_asset',a.asset_id,a.revision_id,0,r.content FROM release_assets a
		JOIN asset_revisions r ON r.workspace_id=a.workspace_id AND r.id=a.revision_id AND r.asset_id=a.asset_id
		WHERE a.workspace_id=$1 AND a.release_id=$2
		UNION ALL SELECT o.object_type,o.object_id,NULL,o.version,s.payload
		FROM release_objects o JOIN release_object_snapshots s ON s.workspace_id=o.workspace_id AND s.release_id=o.release_id AND s.object_type=o.object_type AND s.object_id=o.object_id AND s.version=o.version
		WHERE o.workspace_id=$1 AND o.release_id=$2 ORDER BY 1,2 LIMIT 2001`, workspace.UUID(), release)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var pin productionBaselinePin
			var target, revision pgtype.UUID
			var content []byte
			if err := rows.Scan(&pin.Kind, &target, &revision, &pin.Version, &content); err != nil {
				rows.Close()
				return result, err
			}
			pin.ContentDigest, err = domain.DigestJSON(content)
			if err != nil {
				rows.Close()
				return result, err
			}
			pin.TargetID, err = productionTargetID(pin.Kind, target)
			if err != nil {
				rows.Close()
				return result, err
			}
			if revision.Valid {
				id, err := identity.RevisionIDFromUUIDBytes(revision.Bytes)
				if err != nil {
					rows.Close()
					return result, err
				}
				pin.RevisionID = id.String()
			}
			pins = append(pins, pin)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return result, err
		}
		if len(pins) > 2000 {
			return result, domain.ErrLimitExceeded
		}
		var expected int
		if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM release_assets WHERE workspace_id=$1 AND release_id=$2)+(SELECT count(*) FROM release_objects WHERE workspace_id=$1 AND release_id=$2)`, workspace.UUID(), release).Scan(&expected); err != nil {
			return result, err
		}
		if expected != len(pins) {
			return result, domain.ErrPriorStateUnknown
		}
	}
	for _, decl := range declarations {
		if decl.ReuseIdentity != nil {
			owner := identity.ProductionOperationID{}
			if len(owners) > 0 {
				owner = owners[0]
			}
			baseline, err := productionReintroductionBaselineTx(ctx, tx, workspace, owner, decl, result.Head)
			if err != nil {
				return result, err
			}
			result.Targets[decl.LocalKey] = baseline
			continue
		}
		if decl.Intent != domain.ProductionIntentUpdate {
			continue
		}
		if decl.TargetID == nil {
			return result, domain.ErrInvalidArgument
		}
		var pin *productionBaselinePin
		for i := range pins {
			if pins[i].Kind == decl.Kind && pins[i].TargetID == *decl.TargetID {
				pin = &pins[i]
				break
			}
		}
		if pin == nil {
			return result, domain.ErrBaselineConflict
		}
		id, err := parseUUIDOrTypeID(*decl.TargetID)
		if err != nil {
			return result, domain.ErrInvalidArgument
		}
		baseline := domain.ProductionTargetBaseline{}
		if decl.Kind == domain.TargetKindSemanticAsset {
			if decl.BaseRevisionID == nil || *decl.BaseRevisionID != pin.RevisionID {
				return result, domain.ErrBaselineConflict
			}
			revision, _ := identity.ParseRevisionID(pin.RevisionID)
			if err := tx.QueryRow(ctx, `SELECT content FROM asset_revisions WHERE workspace_id=$1 AND id=$2 AND asset_id=$3`, workspace.UUID(), revision.UUID(), id).Scan(&baseline.Content); err != nil {
				return result, err
			}
			var contentIdentity struct {
				Address   string `json:"address"`
				AssetType string `json:"assetType"`
			}
			var address, assetType string
			if err := tx.QueryRow(ctx, `SELECT namespace||'.'||key,asset_type FROM semantic_assets WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), id).Scan(&address, &assetType); err != nil {
				return result, err
			}
			if json.Unmarshal(baseline.Content, &contentIdentity) != nil || contentIdentity.Address != address || contentIdentity.AssetType != assetType {
				return result, domain.ErrPriorStateUnknown
			}
		} else {
			if decl.BaseObjectVersion == nil || *decl.BaseObjectVersion != pin.Version {
				return result, domain.ErrBaselineConflict
			}
			var payload, current []byte
			if err := tx.QueryRow(ctx, `SELECT payload FROM release_object_snapshots WHERE workspace_id=$1 AND release_id=$2 AND object_type=$3 AND object_id=$4 AND version=$5`, workspace.UUID(), release, decl.Kind, id, pin.Version).Scan(&payload); err != nil {
				return result, err
			}
			table := map[string]string{domain.TargetKindPhysicalBinding: "physical_bindings", domain.TargetKindModelGrain: "model_grains", domain.TargetKindEntityKey: "entity_keys", domain.TargetKindJoinContract: "join_contracts"}[decl.Kind]
			if table == "" {
				return result, domain.ErrInvalidArgument
			}
			var version int
			if err := tx.QueryRow(ctx, `SELECT to_jsonb(value),version FROM `+table+` value WHERE workspace_id=$1 AND id=$2`, workspace.UUID(), id).Scan(&current, &version); err != nil {
				return result, governanceRepositoryError("read registry baseline", err)
			}
			var oldObject, newObject map[string]json.RawMessage
			if json.Unmarshal(payload, &oldObject) != nil || json.Unmarshal(current, &newObject) != nil {
				return result, domain.ErrPriorStateUnknown
			}
			baseline.Content, err = productionObjectBusinessBaseline(decl.Kind, oldObject)
			if err != nil {
				return result, err
			}
			for _, key := range []string{"version", "updated_at", "created_at", "created_by"} {
				delete(oldObject, key)
				delete(newObject, key)
			}
			a, _ := json.Marshal(oldObject)
			b, _ := json.Marshal(newObject)
			ca, _ := domain.CanonicalJSON(a)
			cb, _ := domain.CanonicalJSON(b)
			if !bytes.Equal(ca, cb) {
				return result, domain.ErrBaselineConflict
			}
			baseline.RegistryWriteVersion = &version
		}
		if decl.Kind == domain.TargetKindSemanticAsset {
			if _, err := domain.InspectProductionContent(decl.Kind, baseline.Content); err != nil {
				return result, fmt.Errorf("%w: published business schema unavailable", domain.ErrPriorStateUnknown)
			}
		}
		result.Targets[decl.LocalKey] = baseline
	}
	result.CanonicalJSON, err = json.Marshal(struct {
		Head    domain.HeadReference                       `json:"head"`
		Pins    []productionBaselinePin                    `json:"pins"`
		Targets map[string]domain.ProductionTargetBaseline `json:"targets"`
	}{result.Head, pins, result.Targets})
	if err != nil {
		return result, err
	}
	result.CanonicalJSON, err = domain.CanonicalJSON(result.CanonicalJSON)
	if err != nil {
		return result, err
	}
	return result, nil
}

func checkProductionBaselineTx(ctx context.Context, tx pgx.Tx, version domain.ProductionVersion, targets []domain.ProductionTarget) error {
	localIDs := map[string]string{}
	declarations := make([]domain.TargetDeclaration, 0, len(targets))
	for _, target := range targets {
		localIDs[target.LocalKey] = target.TargetID
		if target.Declaration == nil {
			return domain.ErrPriorStateUnknown
		}
		declarations = append(declarations, *target.Declaration)
	}
	current, err := loadProductionBaseline(ctx, tx, version.WorkspaceID, declarations, version.OperationID)
	if err != nil {
		return err
	}
	if !bytes.Equal(current.CanonicalJSON, version.BaselineJSON) {
		return domain.ErrHeadConflict
	}
	for _, target := range targets {
		if target.Intent != domain.ProductionIntentUpdate && (target.Declaration == nil || target.Declaration.ReuseIdentity == nil) {
			continue
		}
		baseline := current.Targets[target.LocalKey]
		if (baseline.RegistryWriteVersion == nil) != (target.RegistryWriteVersion == nil) || baseline.RegistryWriteVersion != nil && *baseline.RegistryWriteVersion != *target.RegistryWriteVersion {
			return domain.ErrBaselineConflict
		}
		if target.Intent == domain.ProductionIntentCreate {
			continue
		}
		_, noChange, err := domain.ReplayProductionChanges(*target.Declaration, baseline.Content, localIDs)
		if err != nil {
			return err
		}
		if noChange != (target.Outcome == domain.ProductionOutcomeNoChange) {
			return domain.ErrContentMismatch
		}
	}
	return nil
}

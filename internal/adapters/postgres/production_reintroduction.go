package postgres

import (
	"context"
	"encoding/json"
	"errors"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func productionReintroductionBaselineTx(ctx context.Context, tx pgx.Tx, w identity.WorkspaceID, operation identity.ProductionOperationID, decl domain.TargetDeclaration, head domain.HeadReference) (domain.ProductionTargetBaseline, error) {
	result := domain.ProductionTargetBaseline{Content: json.RawMessage(`null`)}
	proof := decl.ReuseIdentity
	if proof == nil || decl.Intent != domain.ProductionIntentCreate || decl.IdentityKey == nil {
		return result, domain.ErrInvalidArgument
	}
	if proof.ExpectedHead.Validate() != nil || head.Presence != domain.PresencePresent || proof.ExpectedHead.Presence != domain.PresencePresent || *head.ReleaseID != *proof.ExpectedHead.ReleaseID || *head.ManifestDigest != *proof.ExpectedHead.ManifestDigest {
		return result, domain.ErrHeadConflict
	}
	target, err := parseUUIDOrTypeID(proof.TargetID)
	if err != nil {
		return result, domain.ErrInvalidArgument
	}
	creationOp, err := identity.ParseProductionOperationID(proof.CreationOperationID)
	if err != nil {
		return result, err
	}
	creation, err := identity.ParseReleaseID(proof.CreationReleaseID)
	if err != nil {
		return result, err
	}
	absence, err := identity.ParseReleaseID(proof.AbsenceReleaseID)
	if err != nil {
		return result, err
	}
	var owner pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_operation_id FROM production_identity_reservations WHERE workspace_id=$1 AND kind=$2 AND identity_key=$3 AND target_id=$4 AND creation_operation_id=$5 FOR UPDATE`, w.UUID(), decl.Kind, *decl.IdentityKey, target, creationOp.UUID()).Scan(&owner); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return result, domain.ErrPriorStateUnknown
		}
		return result, err
	}
	if operation.IsZero() || operation.UUID() != formatUUID(owner) {
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM proposals WHERE workspace_id=$1 AND production_operation_id=$2 AND state NOT IN ('released','rejected'))`, w.UUID(), owner).Scan(&active); err != nil {
			return result, err
		}
		if active {
			return result, domain.ErrIdentityConflict
		}
	}
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM releases c JOIN production_release_integrity ci ON ci.workspace_id=c.workspace_id AND ci.release_id=c.id
 JOIN release_proposals rp ON rp.workspace_id=c.workspace_id AND rp.release_id=c.id AND rp.role='applied'
 JOIN production_targets t ON t.workspace_id=rp.workspace_id AND t.operation_id=rp.operation_id AND t.version=rp.production_version AND t.proposal_id=rp.proposal_id
 JOIN production_release_before_pins cp ON cp.workspace_id=c.workspace_id AND cp.release_id=c.id AND cp.target_kind=t.kind AND cp.target_id=t.target_id AND cp.presence='absent'
 JOIN releases a ON a.workspace_id=c.workspace_id AND a.id=$6 AND a.production_rollback_depth>0 AND a.sequence>c.sequence
 JOIN production_release_integrity ai ON ai.workspace_id=a.workspace_id AND ai.release_id=a.id
 JOIN production_release_before_pins ap ON ap.workspace_id=a.workspace_id AND ap.release_id=a.id AND ap.target_kind=t.kind AND ap.target_id=t.target_id AND ap.presence='present'
 JOIN releases h ON h.workspace_id=a.workspace_id AND h.id=$7 AND h.sequence>=a.sequence
 WHERE c.workspace_id=$1 AND c.id=$5 AND c.production_rollback_depth=0 AND t.kind=$2 AND t.target_id=$3 AND t.identity_key=$4 AND t.intent='create'
 AND a.production_root_release_id=c.id
 AND NOT EXISTS(SELECT 1 FROM releases later WHERE later.workspace_id=$1 AND later.sequence>=a.sequence AND later.sequence<=h.sequence
   AND (EXISTS(SELECT 1 FROM release_assets x WHERE x.workspace_id=later.workspace_id AND x.release_id=later.id AND $2='semantic_asset' AND x.asset_id=$3)
     OR EXISTS(SELECT 1 FROM release_objects x WHERE x.workspace_id=later.workspace_id AND x.release_id=later.id AND x.object_type=$2 AND x.object_id=$3)))
 AND EXISTS(SELECT 1 FROM production_targets original WHERE original.workspace_id=$1 AND original.operation_id=$8 AND original.target_id=$3 AND original.kind=$2 AND original.identity_key=$4 AND original.intent='create')
)`, w.UUID(), decl.Kind, target, *decl.IdentityKey, creation.UUID(), absence.UUID(), head.ReleaseID.UUID(), creationOp.UUID()).Scan(&valid); err != nil {
		return result, err
	}
	if !valid {
		return result, domain.ErrPriorStateUnknown
	}
	if decl.Kind == domain.TargetKindSemanticAsset {
		var address, assetType string
		var current pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT namespace||'.'||key,asset_type,current_revision_id FROM semantic_assets WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, w.UUID(), target).Scan(&address, &assetType, &current); err != nil {
			return result, err
		}
		var content struct {
			AssetType string `json:"assetType"`
		}
		if json.Unmarshal(decl.Content, &content) != nil || current.Valid || address != *decl.IdentityKey || assetType != content.AssetType {
			return result, domain.ErrIdentityConflict
		}
	} else {
		table := map[string]string{domain.TargetKindPhysicalBinding: "physical_bindings", domain.TargetKindModelGrain: "model_grains", domain.TargetKindEntityKey: "entity_keys", domain.TargetKindJoinContract: "join_contracts"}[decl.Kind]
		if table == "" {
			return result, domain.ErrInvalidArgument
		}
		var counter int
		if err := tx.QueryRow(ctx, `SELECT version FROM `+table+` WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, w.UUID(), target).Scan(&counter); err != nil {
			return result, domain.ErrPriorStateUnknown
		}
		result.RegistryWriteVersion = &counter
	}
	return result, nil
}

func transferProductionReintroductionsTx(ctx context.Context, tx pgx.Tx, version domain.ProductionVersion, targets []domain.ProductionTarget) error {
	for _, target := range targets {
		if target.Declaration == nil || target.Declaration.ReuseIdentity == nil {
			continue
		}
		proof := target.Declaration.ReuseIdentity
		id, err := parseUUIDOrTypeID(target.TargetID)
		if err != nil {
			return err
		}
		if proof.TargetID != target.TargetID || target.IdentityKey == nil {
			return domain.ErrPriorStateUnknown
		}
		var owner pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT owner_operation_id FROM production_identity_reservations WHERE workspace_id=$1 AND kind=$2 AND identity_key=$3 AND target_id=$4 FOR UPDATE`, version.WorkspaceID.UUID(), target.Kind, *target.IdentityKey, id).Scan(&owner); err != nil {
			return err
		}
		if formatUUID(owner) == version.OperationID.UUID() {
			continue
		}
		updated, err := tx.Exec(ctx, `UPDATE production_identity_reservations SET owner_operation_id=$5 WHERE workspace_id=$1 AND kind=$2 AND identity_key=$3 AND target_id=$4 AND owner_operation_id=$6`, version.WorkspaceID.UUID(), target.Kind, *target.IdentityKey, id, version.OperationID.UUID(), owner)
		if err != nil {
			return err
		}
		if updated.RowsAffected() != 1 {
			return domain.ErrIdentityConflict
		}
	}
	return nil
}

func inheritProductionCreationContributorsTx(ctx context.Context, tx pgx.Tx, version domain.ProductionVersion, targets []domain.ProductionTarget) error {
	for _, target := range targets {
		if target.Declaration == nil || target.Declaration.ReuseIdentity == nil {
			continue
		}
		proof := target.Declaration.ReuseIdentity
		creation, err := identity.ParseReleaseID(proof.CreationReleaseID)
		if err != nil {
			return err
		}
		original, err := identity.ParseProductionOperationID(proof.CreationOperationID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO production_contributors(workspace_id,operation_id,principal_id,role,created_at)
 SELECT DISTINCT workspace_id,$2::uuid,principal_id,role,$3::timestamptz FROM production_contributors WHERE workspace_id=$1 AND
 (operation_id=$4 OR operation_id IN (SELECT operation_id FROM release_proposals WHERE workspace_id=$1 AND release_id=$5)) ON CONFLICT DO NOTHING`, version.WorkspaceID.UUID(), version.OperationID.UUID(), version.CreatedAt, original.UUID(), creation.UUID()); err != nil {
			return err
		}
	}
	return nil
}

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func productionIdentityRepositoryError(context string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: natural identity is already reserved", domain.ErrIdentityConflict)
	}
	return governanceRepositoryError(context, err)
}

func insertProductionVersion(ctx context.Context, q *dbgen.Queries, version domain.ProductionVersion) error {
	input, err := domain.CanonicalJSON(version.InputJSON)
	if err != nil {
		return err
	}
	declarations, err := domain.CanonicalJSON(version.DeclarationsJSON)
	if err != nil {
		return err
	}
	baseline, err := domain.CanonicalJSON(version.BaselineJSON)
	if err != nil {
		return err
	}
	head, err := json.Marshal(version.BaselineHead)
	if err != nil {
		return err
	}
	digest, err := domain.DigestJSON(baseline)
	if err != nil {
		return err
	}
	w, _ := uuidFromString(version.WorkspaceID.UUID())
	op, _ := uuidFromString(version.OperationID.UUID())
	actor, _ := uuidFromString(version.CreatedBy.UUID())
	_, err = q.InsertProductionVersion(ctx, dbgen.InsertProductionVersionParams{WorkspaceID: w, OperationID: op, Version: int32(version.Version), InputJson: input, InputDigest: version.InputDigest, RequestDigest: version.RequestDigest, SetDigest: version.SetDigest, CreatedBy: actor, CreatedAt: timestamp(version.CreatedAt), CanonicalInput: input, CanonicalDeclarations: declarations, CanonicalBaseline: baseline, BaselineHead: head, BaselineDigest: pgtype.Text{String: digest, Valid: true}})
	if err != nil {
		return governanceRepositoryError("insert canonical production version", err)
	}
	return nil
}

func insertProductionChanges(ctx context.Context, q *dbgen.Queries, targets []domain.ProductionTarget) error {
	for _, target := range targets {
		for _, item := range target.Changes {
			id, err := uuidValue(item.ID)
			if err != nil {
				return err
			}
			w, err := uuidValue(item.WorkspaceID)
			if err != nil {
				return err
			}
			p, err := uuidValue(item.ProposalID)
			if err != nil {
				return err
			}
			_, err = q.CreateProposalChange(ctx, dbgen.CreateProposalChangeParams{ID: id, WorkspaceID: w, ProposalID: p, FieldPath: item.FieldPath, Op: string(item.Op), BeforeDigest: optionalTextContent(item.BeforeDigest), AfterDigest: optionalTextContent(item.AfterDigest), BeforeValue: optionalJSON(item.BeforeValue), AfterValue: optionalJSON(item.AfterValue), CreatedAt: timestamp(item.CreatedAt)})
			if err != nil {
				return governanceRepositoryError("insert production change", err)
			}
		}
	}
	return nil
}

func insertProductionDraftProposals(ctx context.Context, tx pgx.Tx, wspUUID pgtype.UUID, proposals []domain.Proposal) error {
	// 5. Insert draft Proposals
	for _, p := range proposals {
		pUUID, err := uuidFromString(p.ID.UUID())
		if err != nil {
			return err
		}
		var targetObjUUID pgtype.UUID
		parsedT, err := identity.ParseAny(p.TargetObjectID)
		if err == nil {
			targetObjUUID, err = uuidFromString(parsedT.UUID())
		} else {
			targetObjUUID, err = uuidFromString(p.TargetObjectID)
		}
		if err != nil {
			return err
		}

		var assetUUID pgtype.UUID
		if p.AssetID != nil {
			assetUUID, err = uuidFromString(p.AssetID.UUID())
			if err != nil {
				return err
			}
		}

		var prodOpUUID pgtype.UUID
		if p.ProductionOperationID != nil {
			prodOpUUID, err = uuidFromString(p.ProductionOperationID.UUID())
			if err != nil {
				return err
			}
		}
		var prodVer int32
		if p.ProductionVersion != nil {
			prodVer = int32(*p.ProductionVersion)
		}
		var baseRevision pgtype.UUID
		if p.BaseRevisionID != nil {
			baseRevision, err = uuidFromString(p.BaseRevisionID.UUID())
			if err != nil {
				return err
			}
		}
		var baseVersion pgtype.Int4
		if p.BaseObjectVersion != nil {
			baseVersion = pgtype.Int4{Int32: int32(*p.BaseObjectVersion), Valid: true}
		}
		var creationRelease, absenceRelease pgtype.UUID
		if p.ReintroductionCreationReleaseID != nil {
			creationRelease, err = parseUUIDOrTypeID(*p.ReintroductionCreationReleaseID)
			if err != nil {
				return err
			}
		}
		if p.ReintroductionAbsenceReleaseID != nil {
			absenceRelease, err = parseUUIDOrTypeID(*p.ReintroductionAbsenceReleaseID)
			if err != nil {
				return err
			}
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO proposals (
				id, workspace_id, asset_id, target_object_type, target_object_id,
				state, title, created_by, intent, creation_content,
				production_operation_id, production_version, base_revision_id, base_object_version, reintroduction_creation_release_id,reintroduction_absence_release_id,created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,$15,$16, clock_timestamp(), clock_timestamp())
		`, pUUID, wspUUID, assetUUID, string(p.TargetObjectType), targetObjUUID,
			string(p.State), p.Title, p.CreatedBy, p.Intent, p.CreationContent,
			prodOpUUID, prodVer, baseRevision, baseVersion, creationRelease, absenceRelease)
		if err != nil {
			return governanceRepositoryError("insert draft proposal", err)
		}
	}

	return nil
}

// Lock the version before checking member state. Submit locks the same version
// when freezing it, so a replacement cannot race past the frozen boundary.
func lockProductionDraft(ctx context.Context, tx pgx.Tx, workspace, operation pgtype.UUID, version int32) error {
	var frozen pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `SELECT frozen_at FROM production_versions WHERE workspace_id=$1 AND operation_id=$2 AND version=$3 FOR UPDATE`, workspace, operation, version).Scan(&frozen); err != nil {
		return governanceRepositoryError("lock draft version", err)
	}
	if frozen.Valid {
		return fmt.Errorf("%w: operation is frozen", domain.ErrConflict)
	}
	rows, err := tx.Query(ctx, `SELECT state FROM proposals WHERE workspace_id=$1 AND production_operation_id=$2 AND production_version=$3 ORDER BY id FOR UPDATE`, workspace, operation, version)
	if err != nil {
		return governanceRepositoryError("lock draft proposals", err)
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		if err := rows.Scan(&state); err != nil {
			return err
		}
		if state != "draft" {
			return fmt.Errorf("%w: all members must be draft", domain.ErrConflict)
		}
	}
	return rows.Err()
}

func productionTargetID(kind string, id pgtype.UUID) (string, error) {
	switch kind {
	case domain.TargetKindSemanticAsset:
		v, err := identity.AssetIDFromUUIDBytes(id.Bytes)
		return v.String(), err
	case domain.TargetKindPhysicalBinding:
		v, err := identity.PhysicalBindingIDFromUUIDBytes(id.Bytes)
		return v.String(), err
	case domain.TargetKindModelGrain:
		v, err := identity.ModelGrainIDFromUUIDBytes(id.Bytes)
		return v.String(), err
	case domain.TargetKindEntityKey:
		v, err := identity.EntityKeyIDFromUUIDBytes(id.Bytes)
		return v.String(), err
	case domain.TargetKindJoinContract:
		v, err := identity.JoinContractIDFromUUIDBytes(id.Bytes)
		return v.String(), err
	default:
		return "", fmt.Errorf("%w: invalid target kind", domain.ErrInvalidArgument)
	}
}

func reserveReplacementTargets(ctx context.Context, tx pgx.Tx, workspace, operation pgtype.UUID, targets []domain.ProductionTarget) error {
	for _, target := range targets {
		if target.Intent != domain.ProductionIntentCreate {
			continue
		}
		if target.IdentityKey == nil || *target.IdentityKey == "" {
			return fmt.Errorf("%w: create identity required", domain.ErrInvalidArgument)
		}
		id, err := parseUUIDOrTypeID(target.TargetID)
		if err != nil {
			return fmt.Errorf("%w: invalid target", domain.ErrInvalidArgument)
		}
		_, err = tx.Exec(ctx, `INSERT INTO production_identity_reservations
			(workspace_id,kind,identity_key,target_id,creation_operation_id,owner_operation_id,created_at)
			VALUES ($1,$2,$3,$4,$5,$5,clock_timestamp()) ON CONFLICT (workspace_id,kind,identity_key) DO NOTHING`, workspace, target.Kind, *target.IdentityKey, id, operation)
		if err != nil {
			return governanceRepositoryError("reserve replacement identity", err)
		}
		var reserved, owner pgtype.UUID
		if err := tx.QueryRow(ctx, `SELECT target_id,owner_operation_id FROM production_identity_reservations WHERE workspace_id=$1 AND kind=$2 AND identity_key=$3 FOR UPDATE`, workspace, target.Kind, *target.IdentityKey).Scan(&reserved, &owner); err != nil {
			return err
		}
		if reserved != id || owner != operation {
			return fmt.Errorf("%w: identity is already reserved", domain.ErrIdentityConflict)
		}
		if target.Kind == domain.TargetKindSemanticAsset {
			namespace, key := "default", *target.IdentityKey
			if separator := strings.LastIndexByte(key, '.'); separator >= 0 {
				namespace, key = key[:separator], key[separator+1:]
			}
			_, err = tx.Exec(ctx, `INSERT INTO semantic_assets(id,workspace_id,namespace,key,asset_type,lifecycle_state,current_revision_id,created_at,updated_at)
			VALUES($1,$2,$3,$4,COALESCE($5::jsonb->>'assetType','concept'),'draft',NULL,clock_timestamp(),clock_timestamp()) ON CONFLICT (id) DO NOTHING`, id, workspace, namespace, key, target.ContentJSON)
			if err != nil {
				return governanceRepositoryError("insert replacement asset identity", err)
			}
		}
	}
	return nil
}

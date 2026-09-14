package postgres

import (
	"context"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) checkProductionSupersedeTx(ctx context.Context, tx pgx.Tx, version domain.ProductionVersion) error {
	if version.SupersedesOperationID == nil {
		return nil
	}
	var current int
	if err := tx.QueryRow(ctx, `SELECT current_version FROM production_operations WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, version.WorkspaceID.UUID(), version.SupersedesOperationID.UUID()).Scan(&current); err != nil {
		return governanceRepositoryError("lock predecessor", err)
	}
	if current != version.SupersedesVersion {
		return domain.ErrVersionConflict
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM production_operations WHERE workspace_id=$1 AND supersedes_operation_id=$2)`, version.WorkspaceID.UUID(), version.SupersedesOperationID.UUID()).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return domain.ErrConflict
	}
	var frozen pgtype.Timestamptz
	if err := tx.QueryRow(ctx, `SELECT frozen_at FROM production_versions WHERE workspace_id=$1 AND operation_id=$2 AND version=$3 FOR UPDATE`, version.WorkspaceID.UUID(), version.SupersedesOperationID.UUID(), current).Scan(&frozen); err != nil {
		return err
	}
	_, previous, targets, _, _, err := s.GetProductionOperationVersion(ctx, version.WorkspaceID, *version.SupersedesOperationID, current)
	if err != nil {
		return err
	}
	previous.CreatedBy = version.CreatedBy
	if err := s.validateProductionInputTx(ctx, tx, previous, targets, false); err != nil {
		return err
	}
	access, err := authapp.NewService(s, authapp.ClockFunc(time.Now)).Snapshot(ctx, version.WorkspaceID, version.CreatedBy)
	if err != nil {
		return err
	}
	return authorizeProductionTargets(ctx, tx, access, version.WorkspaceID, targets, true, previous.BaselineJSON)
}

func transferProductionSupersedeTx(ctx context.Context, tx pgx.Tx, version domain.ProductionVersion, targets []domain.ProductionTarget) error {
	if version.SupersedesOperationID == nil {
		return nil
	}
	for _, target := range targets {
		id, err := parseUUIDOrTypeID(target.TargetID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE production_identity_reservations SET owner_operation_id=$3 WHERE workspace_id=$1 AND owner_operation_id=$2 AND target_id=$4`, version.WorkspaceID.UUID(), version.SupersedesOperationID.UUID(), version.OperationID.UUID(), id); err != nil {
			return governanceRepositoryError("transfer production reservation", err)
		}
	}
	_, err := tx.Exec(ctx, `UPDATE proposals SET state='rejected',submitted_at=COALESCE(submitted_at,$4),decided_at=$4,updated_at=$4 WHERE workspace_id=$1 AND production_operation_id=$2 AND production_version=$3 AND state NOT IN ('released','rejected')`, version.WorkspaceID.UUID(), version.SupersedesOperationID.UUID(), version.SupersedesVersion, version.CreatedAt)
	if err != nil {
		return governanceRepositoryError("terminate superseded proposals", err)
	}
	return nil
}

package postgres

import (
	"context"
	"time"

	authapp "github.com/iiwish/semlia/internal/application/authorization"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CheckProductionAction(ctx context.Context, ver domain.ProductionVersion, targets []domain.ProductionTarget, command string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := s.validateProductionInputTx(ctx, tx, ver, targets, false); err != nil {
		return err
	}
	return s.productionActionTx(ctx, tx, ver, command)
}

func (s *Store) productionActionTx(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, command string) error {
	var version int64
	if err := tx.QueryRow(ctx, `SELECT authorization_version FROM workspaces WHERE id=$1 FOR UPDATE`, ver.WorkspaceID.UUID()).Scan(&version); err != nil {
		return err
	}
	access, err := authapp.NewService(s, authapp.ClockFunc(time.Now)).Snapshot(ctx, ver.WorkspaceID, ver.CreatedBy)
	if err != nil {
		return err
	}
	if access.AuthorizationVersion != version {
		return domain.ErrVersionConflict
	}
	actions := map[string][]authz.Action{
		domain.CommandSubmit:   {authz.ActionAssetPropose, authz.ActionValidationRun},
		domain.CommandValidate: {authz.ActionValidationRun},
		domain.CommandReview:   {authz.ActionProposalReview},
		domain.CommandPublish:  {authz.ActionReleasePublish},
		domain.CommandRollback: {authz.ActionReleaseRollback},
	}[command]
	if len(actions) == 0 {
		return domain.ErrInvalidArgument
	}
	for _, action := range actions {
		if err := productionRequire(access, action, authz.Resource{Type: authz.ScopeWorkspace, ID: ver.WorkspaceID.UUID()}); err != nil {
			return err
		}
	}
	return nil
}

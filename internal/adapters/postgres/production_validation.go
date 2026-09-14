package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) queueProductionAttemptTx(ctx context.Context, w identity.WorkspaceID, op identity.ProductionOperationID, version int, attempt domain.ValidationAttempt, cmd domain.ProductionCommand, submit bool) error {
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, w, op, version)
	if err != nil {
		return err
	}
	if cmd.WorkspaceID != w || cmd.OperationID != op || cmd.OperationVersion != version || cmd.PrincipalID != attempt.CreatedBy || attempt.WorkspaceID != w || attempt.OperationID != op || attempt.ProductionVersion != version || attempt.Status != domain.ValidationStatusQueued || attempt.CompletedAt != nil || attempt.ValidationDigest != nil || attempt.SetDigest != ver.SetDigest || attempt.InputDigest != ver.InputDigest {
		return domain.ErrInvalidArgument
	}
	checks, err := app.ProductionRequiredChecks(targets)
	if err != nil {
		return err
	}
	checksJSON, err := json.Marshal(checks)
	if err != nil {
		return err
	}
	digest, err := domain.DigestJSON(checksJSON)
	if err != nil {
		return err
	}
	actual, err := domain.DigestJSON(attempt.RequiredChecksJSON)
	if err != nil {
		return err
	}
	if digest != actual || digest != attempt.RequiredChecksDigest {
		return domain.ErrInvalidArgument
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ver.CreatedBy = cmd.PrincipalID
	if err := s.productionActionTx(ctx, tx, ver, cmd.CommandKind); err != nil {
		return err
	}
	if err := s.validateProductionInputTx(ctx, tx, ver, targets, true, true); err != nil {
		return err
	}
	if err := checkProductionBaselineTx(ctx, tx, ver, targets); err != nil {
		return err
	}
	var current int
	if err := tx.QueryRow(ctx, `SELECT current_version FROM production_operations WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, w.UUID(), op.UUID()).Scan(&current); err != nil {
		return err
	}
	if current != version {
		return domain.ErrVersionConflict
	}
	var frozen pgtype.Timestamptz
	var active pgtype.Int4
	if err := tx.QueryRow(ctx, `SELECT frozen_at,active_validation_attempt_no FROM production_versions WHERE workspace_id=$1 AND operation_id=$2 AND version=$3 FOR UPDATE`, w.UUID(), op.UUID(), version).Scan(&frozen, &active); err != nil {
		return err
	}
	if submit {
		if cmd.CommandKind != domain.CommandSubmit || frozen.Valid || active.Valid || attempt.AttemptNo != 1 {
			return domain.ErrVersionConflict
		}
	} else {
		if cmd.CommandKind != domain.CommandValidate || !frozen.Valid || !active.Valid || int(active.Int32)+1 != attempt.AttemptNo {
			return domain.ErrVersionConflict
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM production_validation_attempts WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=$4`, w.UUID(), op.UUID(), version, active.Int32).Scan(&status); err != nil {
			return err
		}
		if status != "succeeded" && status != "failed" {
			return domain.ErrVersionConflict
		}
	}
	for _, target := range targets {
		if target.ProposalID == nil {
			continue
		}
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM proposals WHERE workspace_id=$1 AND id=$2 AND production_operation_id=$3 AND production_version=$4 FOR UPDATE`, w.UUID(), target.ProposalID.UUID(), op.UUID(), version).Scan(&state); err != nil {
			return err
		}
		if (submit && state != "draft") || (!submit && state != "in_review") {
			return domain.ErrVersionConflict
		}
		if submit {
			for _, next := range []string{"proposed", "validating"} {
				if _, err := tx.Exec(ctx, `UPDATE proposals SET state=$3,submitted_at=$4,updated_at=$4 WHERE workspace_id=$1 AND id=$2`, w.UUID(), target.ProposalID.UUID(), next, attempt.CreatedAt); err != nil {
					return err
				}
			}
		}
	}
	q := s.queries.WithTx(tx)
	rules, err := s.productionRulesTx(ctx, tx)
	if err != nil {
		return err
	}
	attempt.FreshnessWitnessJSON, _, err = s.productionFreshnessTx(ctx, tx, ver, targets, rules)
	if err != nil {
		return err
	}
	attempt.FreshnessDigest, err = domain.DigestJSON(attempt.FreshnessWitnessJSON)
	if err != nil {
		return err
	}
	wUUID, _ := uuidFromString(w.UUID())
	opUUID, _ := uuidFromString(op.UUID())
	principal, _ := uuidFromString(cmd.PrincipalID.UUID())
	if _, err := q.InsertProductionValidationAttempt(ctx, dbgen.InsertProductionValidationAttemptParams{WorkspaceID: wUUID, OperationID: opUUID, ProductionVersion: int32(version), AttemptNo: int32(attempt.AttemptNo), SetDigest: ver.SetDigest, InputDigest: ver.InputDigest, FreshnessWitnessJson: attempt.FreshnessWitnessJSON, FreshnessDigest: attempt.FreshnessDigest, RequiredChecksJson: attempt.RequiredChecksJSON, RequiredChecksDigest: attempt.RequiredChecksDigest, Status: domain.ValidationStatusQueued, CreatedBy: principal, CreatedAt: pgtype.Timestamptz{Time: attempt.CreatedAt, Valid: true}}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE production_versions SET frozen_at=COALESCE(frozen_at,$4),active_validation_attempt_no=$5 WHERE workspace_id=$1 AND operation_id=$2 AND version=$3`, w.UUID(), op.UUID(), version, attempt.CreatedAt, attempt.AttemptNo); err != nil {
		return err
	}
	job, err := identity.NewRunID()
	if err != nil {
		return err
	}
	jobUUID, _ := uuidFromString(job.UUID())
	if ver.TraceID == "" {
		ver.TraceID = strings.ReplaceAll(job.UUID(), "-", "")
	}
	payload, err := json.Marshal(app.ValidationJobPayload{OperationID: op.String(), ProductionVersion: version, Attempt: attempt.AttemptNo})
	if err != nil {
		return err
	}
	if _, err := q.EnqueueJob(ctx, dbgen.EnqueueJobParams{ID: jobUUID, WorkspaceID: wUUID, JobType: app.ValidationJobType, Payload: payload, MaxAttempts: app.ValidationJobMaxAttempts, AvailableAt: pgtype.Timestamptz{Time: attempt.CreatedAt, Valid: true}, IdempotencyKey: fmt.Sprintf("production-validation:%s:%d:%d", op.String(), version, attempt.AttemptNo), TraceID: ver.TraceID}); err != nil {
		return err
	}
	if err := q.InsertProductionCommand(ctx, dbgen.InsertProductionCommandParams{ResponseJson: cmd.ResponseJSON, WorkspaceID: wUUID, PrincipalID: principal, CommandKind: cmd.CommandKind, IdempotencyKey: cmd.IdempotencyKey, RequestDigest: cmd.RequestDigest, OperationID: opUUID, OperationVersion: int32(version), ResultKind: "operation", ResultID: opUUID, CommittedAt: pgtype.Timestamptz{Time: cmd.CommittedAt, Valid: true}}); err != nil {
		return governanceRepositoryError("record validation command", err)
	}
	ver.CreatedAt = time.Now().UTC()
	if err := insertProductionMutationEvents(ctx, q, ver, cmd.CommandKind); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

package postgres

import (
	"context"
	"errors"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func loadProductionRecovery(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, version *domain.ProductionVersion, targets []domain.ProductionTarget, active pgtype.Int4) error {
	w, _ := uuidFromString(version.WorkspaceID.UUID())
	op, _ := uuidFromString(version.OperationID.UUID())
	byProposal := map[string]*domain.ProductionTarget{}
	for i := range targets {
		targets[i].ValidationRunIDs = []string{}
		targets[i].ReviewIDs = []string{}
		if targets[i].ProposalID != nil {
			byProposal[targets[i].ProposalID.String()] = &targets[i]
		}
	}
	var released pgtype.UUID
	err := tx.QueryRow(ctx, `SELECT rp.release_id FROM release_proposals rp JOIN releases r ON r.workspace_id=rp.workspace_id AND r.id=rp.release_id
	WHERE rp.workspace_id=$1 AND rp.operation_id=$2 AND rp.production_version=$3 AND rp.role='applied' AND rp.set_digest=$4 ORDER BY r.sequence LIMIT 1`, w, op, version.Version, version.SetDigest).Scan(&released)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if released.Valid {
		id, err := identity.ReleaseIDFromUUIDBytes(released.Bytes)
		if err != nil {
			return err
		}
		version.ReleaseID = &id
	}
	if !active.Valid {
		return nil
	}
	row, err := q.GetProductionValidationAttempt(ctx, dbgen.GetProductionValidationAttemptParams{WorkspaceID: w, OperationID: op, ProductionVersion: int32(version.Version), AttemptNo: active.Int32})
	if err != nil {
		return err
	}
	version.ActiveValidation = validationAttemptFromRow(row)
	var count, size int64
	if err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(octet_length(to_jsonb(v)::text)),0) FROM validation_runs v WHERE workspace_id=$1 AND production_operation_id=$2 AND production_version=$3 AND production_attempt_no=$4`, w, op, version.Version, active.Int32).Scan(&count, &size); err != nil {
		return err
	}
	if count > 256 || size > 4*1024*1024 {
		return domain.ErrLimitExceeded
	}
	runs, err := q.ListValidationRunsForProductionAttempt(ctx, dbgen.ListValidationRunsForProductionAttemptParams{WorkspaceID: w, ProductionOperationID: op, ProductionVersion: pgtype.Int4{Int32: int32(version.Version), Valid: true}, ProductionAttemptNo: active})
	if err != nil {
		return err
	}
	version.ValidationResults = map[string][]domain.ValidationResult{}
	for _, row := range runs {
		run, err := validationRunFromRow(row)
		if err != nil {
			return err
		}
		target := byProposal[run.ProposalID.String()]
		if target == nil {
			return domain.ErrPriorStateUnknown
		}
		target.ValidationRunIDs = append(target.ValidationRunIDs, run.ID.String())
		version.ValidationRuns = append(version.ValidationRuns, run)
		if err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(octet_length(to_jsonb(v)::text)),0) FROM validation_results v WHERE workspace_id=$1 AND validation_run_id=$2`, w, row.ID).Scan(&count, &size); err != nil {
			return err
		}
		if count > 256 || size > 4*1024*1024 {
			return domain.ErrLimitExceeded
		}
		results, err := q.ListValidationRunResults(ctx, dbgen.ListValidationRunResultsParams{WorkspaceID: w, ValidationRunID: row.ID})
		if err != nil {
			return err
		}
		for _, row := range results {
			result, err := validationResultFromRow(row)
			if err != nil {
				return err
			}
			version.ValidationResults[run.ID.String()] = append(version.ValidationResults[run.ID.String()], result)
		}
	}
	rows, err := tx.Query(ctx, `SELECT r.id,r.proposal_id,r.decision FROM production_review_bindings b JOIN reviews r ON r.workspace_id=b.workspace_id AND r.id=b.review_id
	WHERE b.workspace_id=$1 AND b.operation_id=$2 AND b.production_version=$3 AND b.attempt_no=$4 AND b.set_digest=$5
	ORDER BY r.created_at,r.id LIMIT 257`, w, op, version.Version, active.Int32, version.SetDigest)
	if err != nil {
		return err
	}
	defer rows.Close()
	count = 0
	for rows.Next() {
		count++
		if count > 256 {
			return domain.ErrLimitExceeded
		}
		var review, proposal pgtype.UUID
		var decision string
		if err := rows.Scan(&review, &proposal, &decision); err != nil {
			return err
		}
		reviewID, err := identity.ReviewIDFromUUIDBytes(review.Bytes)
		if err != nil {
			return err
		}
		proposalID, err := identity.ProposalIDFromUUIDBytes(proposal.Bytes)
		if err != nil {
			return err
		}
		target := byProposal[proposalID.String()]
		if target == nil {
			return domain.ErrPriorStateUnknown
		}
		target.ReviewIDs = append(target.ReviewIDs, reviewID.String())
		target.ReviewApproved = decision == "approved"
	}
	return rows.Err()
}

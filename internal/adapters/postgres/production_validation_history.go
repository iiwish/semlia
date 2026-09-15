package postgres

import (
	"context"
	"encoding/json"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) ProductionValidationHistory(ctx context.Context, w identity.WorkspaceID, p identity.PrincipalID, op identity.ProductionOperationID, version, limit, upper, after int) ([]domain.ProductionVersion, int, bool, error) {
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, w, op, version)
	if err != nil {
		return nil, 0, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ver.CreatedBy = p
	if err := s.validateProductionInputTx(ctx, tx, ver, targets, false); err != nil {
		return nil, 0, false, err
	}
	if upper == 0 {
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(attempt_no),0) FROM production_validation_attempts WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3`, w.UUID(), op.UUID(), version).Scan(&upper); err != nil {
			return nil, 0, false, err
		}
	}
	rows, err := tx.Query(ctx, `SELECT attempt_no FROM production_validation_attempts WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no<=$4 AND ($5=0 OR attempt_no<$5) ORDER BY attempt_no DESC LIMIT $6`, w.UUID(), op.UUID(), version, upper, after, limit+1)
	if err != nil {
		return nil, 0, false, err
	}
	numbers := []int{}
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, 0, false, err
		}
		numbers = append(numbers, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, false, err
	}
	items := []domain.ProductionVersion{}
	more, size := false, 0
	for _, n := range numbers {
		if len(items) == limit {
			more = true
			break
		}
		item := domain.ProductionVersion{WorkspaceID: w, OperationID: op, Version: version, SetDigest: ver.SetDigest}
		if err := loadProductionRecovery(ctx, tx, s.queries.WithTx(tx), &item, targets, pgtype.Int4{Int32: int32(n), Valid: true}); err != nil {
			return nil, 0, false, err
		}
		raw, err := json.Marshal(struct {
			Attempt *domain.ValidationAttempt
			Runs    []domain.ValidationRun
			Results map[string][]domain.ValidationResult
		}{item.ActiveValidation, item.ValidationRuns, item.ValidationResults})
		if err != nil {
			return nil, 0, false, err
		}
		if len(raw) > 2<<20 {
			return nil, 0, false, domain.ErrLimitExceeded
		}
		if size+len(raw) > 3<<20 && len(items) > 0 {
			more = true
			break
		}
		size += len(raw)
		items = append(items, item)
	}
	return items, upper, more, nil
}

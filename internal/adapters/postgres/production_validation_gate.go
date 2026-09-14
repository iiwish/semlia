package postgres

import (
	"context"
	"encoding/json"
	"errors"

	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/jackc/pgx/v5"
)

func (s *Store) productionAttemptGate(ctx context.Context, tx pgx.Tx, ver domain.ProductionVersion, targets []domain.ProductionTarget, attempt int, digest string, allowFailed bool) error {
	var current, active int
	if err := tx.QueryRow(ctx, `SELECT current_version FROM production_operations WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, ver.WorkspaceID.UUID(), ver.OperationID.UUID()).Scan(&current); err != nil {
		return err
	}
	if current != ver.Version {
		return domain.ErrVersionConflict
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(active_validation_attempt_no,0) FROM production_versions WHERE workspace_id=$1 AND operation_id=$2 AND version=$3 FOR UPDATE`, ver.WorkspaceID.UUID(), ver.OperationID.UUID(), ver.Version).Scan(&active); err != nil {
		return err
	}
	if active != attempt {
		return domain.ErrValidationRequired
	}
	a := domain.ValidationAttempt{WorkspaceID: ver.WorkspaceID, OperationID: ver.OperationID, ProductionVersion: ver.Version, AttemptNo: attempt}
	var results, policies []byte
	var validationDigest string
	if err := tx.QueryRow(ctx, `SELECT a.status,a.set_digest,a.input_digest,a.required_checks_digest,a.freshness_digest,a.validation_digest,a.completed_at,s.results_json,s.policy_ids FROM production_validation_attempts a JOIN production_validation_seals s USING(workspace_id,operation_id,production_version,attempt_no) WHERE a.workspace_id=$1 AND a.operation_id=$2 AND a.production_version=$3 AND a.attempt_no=$4 AND a.validation_digest=s.validation_digest`, ver.WorkspaceID.UUID(), ver.OperationID.UUID(), ver.Version, attempt).Scan(&a.Status, &a.SetDigest, &a.InputDigest, &a.RequiredChecksDigest, &a.FreshnessDigest, &validationDigest, &a.CompletedAt, &results, &policies); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrValidationRequired
		}
		return err
	}
	if validationDigest != digest || a.CompletedAt == nil || a.SetDigest != ver.SetDigest || a.InputDigest != ver.InputDigest || (a.Status != "succeeded" && (!allowFailed || a.Status != "failed")) {
		return domain.ErrValidationRequired
	}
	a.ValidationDigest = &validationDigest
	checks, err := app.ProductionRequiredChecks(targets)
	if err != nil {
		return err
	}
	checkJSON, err := json.Marshal(checks)
	if err != nil {
		return err
	}
	checkDigest, err := domain.DigestJSON(checkJSON)
	if err != nil {
		return err
	}
	if checkDigest != a.RequiredChecksDigest {
		return domain.ErrValidationRequired
	}
	var outputs []app.ProductionCheckResult
	if err := json.Unmarshal(results, &outputs); err != nil {
		return err
	}
	if len(outputs) != len(checks) {
		return domain.ErrValidationRequired
	}
	for i, check := range checks {
		if check != outputs[i].Check {
			return domain.ErrValidationRequired
		}
	}
	sealed, err := app.SealProductionValidation(app.ProductionValidationCompletion{Attempt: a, Checks: outputs}, *a.CompletedAt)
	if err != nil {
		return err
	}
	if *sealed.Attempt.ValidationDigest != digest || sealed.Attempt.Status != a.Status {
		return domain.ErrValidationRequired
	}
	var complete bool
	if err := tx.QueryRow(ctx, `SELECT count(*)=$5 AND bool_and(r.status IN ('succeeded','failed')) AND bool_and(b.set_digest=$6 AND b.input_digest=$7 AND b.proposal_content_digest=t.content_digest AND r.proposal_id=t.proposal_id)
 FROM validation_runs r JOIN production_validation_bindings b ON b.workspace_id=r.workspace_id AND b.validation_run_id=r.id
 JOIN production_targets t ON t.workspace_id=b.workspace_id AND t.operation_id=b.operation_id AND t.version=b.production_version AND t.proposal_id=b.proposal_id
 WHERE r.workspace_id=$1 AND r.production_operation_id=$2 AND r.production_version=$3 AND r.production_attempt_no=$4`, ver.WorkspaceID.UUID(), ver.OperationID.UUID(), ver.Version, attempt, len(checks), ver.SetDigest, ver.InputDigest).Scan(&complete); err != nil {
		return err
	}
	if !complete {
		return domain.ErrValidationRequired
	}
	if !allowFailed {
		if err := s.validateProductionInputTx(ctx, tx, ver, targets, true, true, false); err != nil {
			return err
		}
		if err := checkProductionBaselineTx(ctx, tx, ver, targets); err != nil {
			return err
		}
		rules, err := s.productionRulesTx(ctx, tx)
		if err != nil {
			return err
		}
		witness, confirmed, err := s.productionFreshnessTx(ctx, tx, ver, targets, rules)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if domain.RequiresProductionBusinessRule(target) && !confirmed[target.LocalKey] {
				return domain.ErrValidationRequired
			}
		}
		fresh, err := domain.DigestJSON(witness)
		if err != nil {
			return err
		}
		if fresh != a.FreshnessDigest {
			return domain.ErrValidationRequired
		}
		var ids []string
		if json.Unmarshal(policies, &ids) != nil || len(ids)*4 != len(checks) {
			return domain.ErrValidationRequired
		}
	}
	return nil
}

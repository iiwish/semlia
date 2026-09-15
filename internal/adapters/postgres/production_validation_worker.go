package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	app "github.com/iiwish/semlia/internal/application/governance"
	authz "github.com/iiwish/semlia/internal/domain/authorization"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"github.com/jackc/pgx/v5"
)

func productionValidationFailure(err error) string {
	if err == nil {
		return ""
	}
	var denial *authz.DenialError
	if errors.As(err, &denial) {
		return "PRODUCTION_AUTHORIZATION_REVOKED"
	}
	for _, known := range []error{domain.ErrInputStale, domain.ErrInputIncomplete, domain.ErrEvidenceMissing, domain.ErrNotFound, domain.ErrAlreadyProduced, domain.ErrDependencyInvalid, domain.ErrHeadConflict, domain.ErrBaselineConflict, domain.ErrPriorStateUnknown, domain.ErrContentMismatch} {
		if errors.Is(err, known) {
			return "PRODUCTION_INPUT_STALE"
		}
	}
	return ""
}

func (s *Store) LoadProductionValidationWork(ctx context.Context, w identity.WorkspaceID, op identity.ProductionOperationID, version, number int) (app.ProductionValidationWork, error) {
	work := app.ProductionValidationWork{Proposals: map[string]domain.Proposal{}}
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, w, op, version)
	if err != nil {
		return work, err
	}
	attempt, err := s.GetValidationAttempt(ctx, w, op, version, number)
	if err != nil {
		return work, err
	}
	if attempt == nil {
		return work, domain.ErrNotFound
	}
	work.Version, work.Targets, work.Attempt = ver, targets, *attempt
	work.Rules, err = s.ListPolicyRules(ctx, domain.RiskRuleVersion)
	if err != nil {
		return work, err
	}
	for _, target := range targets {
		if target.ProposalID == nil {
			continue
		}
		proposal, err := s.GetProposal(ctx, w, *target.ProposalID)
		if err != nil {
			return work, err
		}
		work.Proposals[proposal.ID.String()] = proposal
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return work, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ver.CreatedBy = attempt.CreatedBy
	err = s.validateProductionInputTx(ctx, tx, ver, targets, true, true)
	if err == nil {
		err = checkProductionBaselineTx(ctx, tx, ver, targets)
	}
	if err != nil {
		work.ReferenceFailure = productionValidationFailure(err)
		if work.ReferenceFailure == "" {
			return work, err
		}
	}
	witness, confirmed, witnessErr := s.productionFreshnessTx(ctx, tx, ver, targets, work.Rules)
	work.BusinessRules = confirmed
	if witnessErr != nil {
		return work, witnessErr
	}
	digest, digestErr := domain.DigestJSON(witness)
	if digestErr != nil {
		return work, digestErr
	}
	if digest != attempt.FreshnessDigest {
		work.ReferenceFailure = "PRODUCTION_VALIDATION_CONTEXT_CHANGED"
	}
	return work, nil
}

func (s *Store) CompleteProductionValidation(ctx context.Context, completion app.ProductionValidationCompletion) error {
	a := completion.Attempt
	if a.CompletedAt == nil || a.ValidationDigest == nil {
		return domain.ErrInvalidArgument
	}
	_, ver, targets, _, _, err := s.GetProductionOperationVersion(ctx, a.WorkspaceID, a.OperationID, a.ProductionVersion)
	if err != nil {
		return err
	}
	checks, err := app.ProductionRequiredChecks(targets)
	if err != nil {
		return err
	}
	if len(checks) != len(completion.Checks) {
		return domain.ErrValidationRequired
	}
	for i, check := range checks {
		if check != completion.Checks[i].Check {
			return domain.ErrValidationRequired
		}
		for _, target := range targets {
			if target.ProposalID != nil && target.ProposalID.String() == check.ProposalID {
				digest, err := app.ProductionCheckInputDigest(target, a, check)
				if err != nil {
					return err
				}
				if digest != completion.Checks[i].InputDigest {
					return domain.ErrValidationRequired
				}
			}
		}
	}
	sealed, err := app.SealProductionValidation(completion, *a.CompletedAt)
	if err != nil {
		return err
	}
	if sealed.Attempt.Status != a.Status || *sealed.Attempt.ValidationDigest != *a.ValidationDigest {
		return domain.ErrValidationRequired
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM workspaces WHERE id=$1 FOR UPDATE`, a.WorkspaceID.UUID()).Scan(&current); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT current_version FROM production_operations WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, a.WorkspaceID.UUID(), a.OperationID.UUID()).Scan(&current); err != nil {
		return err
	}
	if current != a.ProductionVersion {
		return domain.ErrVersionConflict
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT active_validation_attempt_no FROM production_versions WHERE workspace_id=$1 AND operation_id=$2 AND version=$3 FOR UPDATE`, a.WorkspaceID.UUID(), a.OperationID.UUID(), a.ProductionVersion).Scan(&active); err != nil {
		return err
	}
	if active != a.AttemptNo {
		return domain.ErrVersionConflict
	}
	var status, setDigest, inputDigest, requiredDigest, freshnessDigest string
	if err := tx.QueryRow(ctx, `SELECT status,set_digest,input_digest,required_checks_digest,freshness_digest FROM production_validation_attempts WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=$4 FOR UPDATE`, a.WorkspaceID.UUID(), a.OperationID.UUID(), a.ProductionVersion, a.AttemptNo).Scan(&status, &setDigest, &inputDigest, &requiredDigest, &freshnessDigest); err != nil {
		return err
	}
	if status == "succeeded" || status == "failed" {
		return nil
	}
	if setDigest != a.SetDigest || inputDigest != a.InputDigest || requiredDigest != a.RequiredChecksDigest || freshnessDigest != a.FreshnessDigest {
		return domain.ErrValidationRequired
	}
	ver.CreatedBy = a.CreatedBy
	freshErr := s.validateProductionInputTx(ctx, tx, ver, targets, true, true)
	if freshErr == nil {
		freshErr = checkProductionBaselineTx(ctx, tx, ver, targets)
	}
	contextChanged := false
	if freshErr == nil {
		rules, err := s.productionRulesTx(ctx, tx)
		if err != nil {
			return err
		}
		witness, _, err := s.productionFreshnessTx(ctx, tx, ver, targets, rules)
		if err != nil {
			return err
		}
		digest, err := domain.DigestJSON(witness)
		if err != nil {
			return err
		}
		contextChanged = digest != a.FreshnessDigest
	}
	if freshErr != nil || contextChanged {
		code := productionValidationFailure(freshErr)
		if contextChanged {
			code = "PRODUCTION_VALIDATION_CONTEXT_CHANGED"
		}
		if code == "" {
			return freshErr
		}
		for i := range completion.Checks {
			completion.Checks[i].Status = domain.ValidationFailed
			completion.Checks[i].Findings = []app.Finding{{Severity: domain.SeverityBlocker, Code: code, Message: "production input or authorization changed before validation committed", InputDigest: completion.Checks[i].InputDigest, Details: json.RawMessage(`{}`)}}
		}
		completion.Policies = nil
		completion, err = app.SealProductionValidation(completion, *a.CompletedAt)
		if err != nil {
			return err
		}
		a = completion.Attempt
	}
	byProposal := map[string]domain.ProductionTarget{}
	for _, target := range targets {
		if target.ProposalID != nil {
			byProposal[target.ProposalID.String()] = target
		}
	}
	for _, check := range completion.Checks {
		target := byProposal[check.Check.ProposalID]
		run, err := identity.NewValidationRunID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO validation_runs(id,workspace_id,proposal_id,validator_id,validator_version,status,started_at,finished_at,production_operation_id,production_version,production_attempt_no) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, run.UUID(), a.WorkspaceID.UUID(), target.ProposalID.UUID(), check.Check.ValidatorID, check.Check.ValidatorVersion, string(check.Status), a.CreatedAt, a.CompletedAt, a.OperationID.UUID(), a.ProductionVersion, a.AttemptNo); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO production_validation_bindings(workspace_id,validation_run_id,operation_id,production_version,attempt_no,set_digest,proposal_id,proposal_content_digest,input_digest,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, a.WorkspaceID.UUID(), run.UUID(), a.OperationID.UUID(), a.ProductionVersion, a.AttemptNo, a.SetDigest, target.ProposalID.UUID(), target.ContentDigest, a.InputDigest, a.CompletedAt); err != nil {
			return err
		}
		for _, finding := range check.Findings {
			id, err := identity.NewValidationResultID()
			if err != nil {
				return err
			}
			details := finding.Details
			if len(details) == 0 {
				details = json.RawMessage(`{}`)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO validation_results(id,workspace_id,validation_run_id,severity,code,message,input_digest,details,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id.UUID(), a.WorkspaceID.UUID(), run.UUID(), string(finding.Severity), finding.Code, finding.Message, finding.InputDigest, details, a.CompletedAt); err != nil {
				return err
			}
		}
	}
	policyIDs := []string{}
	policyByProposal := map[string]string{}
	for _, policy := range completion.Policies {
		if _, ok := byProposal[policy.ProposalID.String()]; !ok {
			return domain.ErrInvalidArgument
		}
		if _, err := tx.Exec(ctx, `INSERT INTO policy_decisions(id,workspace_id,proposal_id,rule_version,inputs,inputs_digest,matched_policy,risk_level,routing,reason_code,decided_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11) ON CONFLICT(proposal_id,rule_version,inputs_digest) DO NOTHING`, policy.ID.UUID(), a.WorkspaceID.UUID(), policy.ProposalID.UUID(), policy.RuleVersion, policy.Inputs, policy.InputsDigest, policy.MatchedPolicy, string(policy.RiskLevel), string(policy.Routing), policy.ReasonCode, policy.DecidedAt); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM policy_decisions WHERE workspace_id=$1 AND proposal_id=$2 AND rule_version=$3 AND inputs_digest=$4`, a.WorkspaceID.UUID(), policy.ProposalID.UUID(), policy.RuleVersion, policy.InputsDigest).Scan(&id); err != nil {
			return err
		}
		policyIDs = append(policyIDs, id)
		policyByProposal[policy.ProposalID.String()] = id
	}
	resultJSON, err := json.Marshal(completion.Checks)
	if err != nil {
		return err
	}
	policyJSON, err := json.Marshal(policyIDs)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE production_validation_attempts SET status=$5,validation_digest=$6,completed_at=$7 WHERE workspace_id=$1 AND operation_id=$2 AND production_version=$3 AND attempt_no=$4`, a.WorkspaceID.UUID(), a.OperationID.UUID(), a.ProductionVersion, a.AttemptNo, a.Status, *a.ValidationDigest, a.CompletedAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO production_validation_seals(workspace_id,operation_id,production_version,attempt_no,results_json,validation_digest,policy_ids) VALUES($1,$2,$3,$4,$5,$6,$7)`, a.WorkspaceID.UUID(), a.OperationID.UUID(), a.ProductionVersion, a.AttemptNo, resultJSON, *a.ValidationDigest, policyJSON); err != nil {
		return err
	}
	for _, target := range targets {
		if target.ProposalID == nil {
			continue
		}
		if id, ok := policyByProposal[target.ProposalID.String()]; ok {
			if _, err := tx.Exec(ctx, `UPDATE proposals SET policy_decision_id=$3,updated_at=$4 WHERE workspace_id=$1 AND id=$2`, a.WorkspaceID.UUID(), target.ProposalID.UUID(), id, a.CompletedAt); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE proposals SET state='in_review',updated_at=$3 WHERE workspace_id=$1 AND id=$2 AND state='validating'`, a.WorkspaceID.UUID(), target.ProposalID.UUID(), a.CompletedAt); err != nil {
			return err
		}
	}
	ver.CreatedAt = time.Now().UTC()
	if err := insertProductionMutationEvents(ctx, s.queries.WithTx(tx), ver, "validated"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

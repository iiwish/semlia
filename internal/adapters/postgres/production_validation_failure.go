package postgres

import (
	"context"
	"encoding/json"
	app "github.com/iiwish/semlia/internal/application/governance"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
	"time"
)

func (s *Store) FailProductionValidation(ctx context.Context, w identity.WorkspaceID, op identity.ProductionOperationID, version, number int) error {
	_, _, targets, _, _, err := s.GetProductionOperationVersion(ctx, w, op, version)
	if err != nil {
		return err
	}
	attempt, err := s.GetValidationAttempt(ctx, w, op, version, number)
	if err != nil {
		return err
	}
	if attempt == nil {
		return domain.ErrNotFound
	}
	if attempt.Status == domain.ValidationStatusFailed || attempt.Status == domain.ValidationStatusSucceeded {
		return nil
	}
	checks, err := app.ProductionRequiredChecks(targets)
	if err != nil {
		return err
	}
	byProposal := map[string]domain.ProductionTarget{}
	for _, target := range targets {
		if target.ProposalID != nil {
			byProposal[target.ProposalID.String()] = target
		}
	}
	completion := app.ProductionValidationCompletion{Attempt: *attempt, Checks: []app.ProductionCheckResult{}}
	for _, check := range checks {
		digest, err := app.ProductionCheckInputDigest(byProposal[check.ProposalID], *attempt, check)
		if err != nil {
			return err
		}
		completion.Checks = append(completion.Checks, app.ProductionCheckResult{Check: check, InputDigest: digest, Status: domain.ValidationFailed, Findings: []app.Finding{{Severity: domain.SeverityBlocker, Code: "VALIDATION_EXECUTION_FAILED", Message: "validation exhausted its execution attempts; a new complete validation is required", InputDigest: digest, Details: json.RawMessage(`{}`)}}})
	}
	completion, err = app.SealProductionValidation(completion, time.Now().UTC())
	if err != nil {
		return err
	}
	return s.CompleteProductionValidation(ctx, completion)
}

package governance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// ProductionCheck pins a required executable, not a successful outcome.
type ProductionCheck struct {
	ProposalID       string `json:"proposalId"`
	ValidatorID      string `json:"validatorId"`
	ValidatorVersion string `json:"validatorVersion"`
}

const ProductionValidatorVersion = "1.0.0-production.2"

func ProductionRequiredChecks(targets []domain.ProductionTarget) ([]ProductionCheck, error) {
	checks := []ProductionCheck{}
	for _, target := range targets {
		if target.ProposalID == nil {
			continue
		}
		for _, id := range []string{ValidatorIDSchema, ValidatorIDReference, ValidatorIDStructural, "policy"} {
			checks = append(checks, ProductionCheck{target.ProposalID.String(), id, ProductionValidatorVersion})
		}
	}
	if len(checks) == 0 {
		return nil, domain.ErrNoSubstantiveChange
	}
	if len(checks) > 256 {
		return nil, domain.ErrLimitExceeded
	}
	return checks, nil
}

func (s *ProductionService) queueProductionValidation(ctx context.Context, cmd SubmitOperationCommand, previous int, reason string) (*ProductionCommandResult, error) {
	if cmd.WorkspaceID.IsZero() || cmd.PrincipalID.IsZero() || cmd.OperationID.IsZero() || strings.TrimSpace(cmd.IdempotencyKey) == "" || cmd.ExpectedVersion < 1 || previous < 0 || !domain.IsValidContentDigest(cmd.SetDigest) {
		return nil, domain.ErrInvalidArgument
	}
	kind, outcome := domain.CommandSubmit, "submitted"
	if previous > 0 {
		kind, outcome = domain.CommandValidate, "validation_queued"
	}
	body, err := json.Marshal(map[string]any{"schema": "production.validation-command.v1", "workspaceId": cmd.WorkspaceID.String(), "operationId": cmd.OperationID.String(), "command": kind, "expectedVersion": cmd.ExpectedVersion, "setDigest": cmd.SetDigest, "previousAttemptNo": previous, "reason": reason})
	if err != nil {
		return nil, err
	}
	digest, err := domain.DigestJSON(body)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, kind, cmd.IdempotencyKey)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	version := cmd.ExpectedVersion
	if existing != nil {
		if existing.RequestDigest != digest {
			return nil, domain.ErrIdempotencyConflict
		}
		version = existing.OperationVersion
	}
	_, ver, targets, _, _, err := s.GetOperationVersion(ctx, cmd.WorkspaceID, cmd.OperationID, version)
	if errors.Is(err, domain.ErrNotFound) {
		if _, _, _, _, _, lookupErr := s.repo.GetProductionOperation(ctx, cmd.WorkspaceID, cmd.OperationID); lookupErr == nil {
			return nil, domain.ErrVersionConflict
		}
	}
	if err != nil {
		return nil, err
	}
	if err := s.AuthorizeOperationRead(ctx, cmd.PrincipalID, *ver, targets); err != nil {
		return nil, err
	}
	if err := s.checkProductionAction(ctx, cmd.PrincipalID, *ver, targets, kind); err != nil {
		return nil, err
	}
	proposalIDs := []identity.ProposalID{}
	for _, target := range targets {
		if target.ProposalID != nil {
			proposalIDs = append(proposalIDs, *target.ProposalID)
		}
	}
	next := previous + 1
	result := &ProductionCommandResult{OperationID: cmd.OperationID, Version: version, SetDigest: ver.SetDigest, Outcome: outcome, ProposalIDs: proposalIDs, ValidationAttemptNo: &next, ValidationRunIDs: []identity.ValidationRunID{}, ReviewIDs: []identity.ReviewID{}}
	if existing != nil {
		attempt, err := s.repo.GetValidationAttempt(ctx, cmd.WorkspaceID, cmd.OperationID, version, next)
		if err != nil {
			return nil, err
		}
		if attempt == nil {
			return nil, domain.ErrInvariant
		}
		result.Replayed = true
		if len(existing.ResponseJSON) > 0 {
			if err := json.Unmarshal(existing.ResponseJSON, result); err != nil {
				return nil, err
			}
			result.Replayed = true
		}
		return result, nil
	}
	if ver.SetDigest != cmd.SetDigest {
		return nil, domain.ErrBaselineConflict
	}
	if previous == 0 && ver.FrozenAt != nil {
		return nil, domain.ErrVersionConflict
	}
	if previous > 0 {
		if ver.FrozenAt == nil {
			return nil, domain.ErrVersionConflict
		}
		latest, err := s.repo.GetLatestValidationAttempt(ctx, cmd.WorkspaceID, cmd.OperationID, version)
		if err != nil {
			return nil, err
		}
		if latest == nil || latest.AttemptNo != previous || latest.Status == domain.ValidationStatusQueued || latest.Status == domain.ValidationStatusRunning {
			return nil, domain.ErrVersionConflict
		}
	}
	if next > 256 {
		return nil, domain.ErrLimitExceeded
	}
	checks, err := ProductionRequiredChecks(targets)
	if err != nil {
		return nil, err
	}
	checksJSON, err := json.Marshal(checks)
	if err != nil {
		return nil, err
	}
	checksDigest, err := domain.DigestJSON(checksJSON)
	if err != nil {
		return nil, err
	}
	rules, err := domain.CanonicalPolicyRules(domain.RiskRuleVersion)
	if source, ok := s.repo.(PolicyRuleSource); ok {
		rules, err = source.ListPolicyRules(ctx, domain.RiskRuleVersion)
	}
	if err != nil {
		return nil, err
	}
	witness, err := ProductionFreshnessWitness(*ver, rules)
	if err != nil {
		return nil, err
	}
	freshnessDigest, err := domain.DigestJSON(witness)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	attempt := domain.ValidationAttempt{WorkspaceID: cmd.WorkspaceID, OperationID: cmd.OperationID, ProductionVersion: version, AttemptNo: next, SetDigest: ver.SetDigest, InputDigest: ver.InputDigest, FreshnessWitnessJSON: witness, FreshnessDigest: freshnessDigest, RequiredChecksJSON: checksJSON, RequiredChecksDigest: checksDigest, Status: domain.ValidationStatusQueued, CreatedBy: cmd.PrincipalID, CreatedAt: now}
	command := domain.ProductionCommand{WorkspaceID: cmd.WorkspaceID, PrincipalID: cmd.PrincipalID, CommandKind: kind, IdempotencyKey: cmd.IdempotencyKey, RequestDigest: digest, OperationID: cmd.OperationID, OperationVersion: version, ResultKind: "operation", ResultID: cmd.OperationID.String(), CommittedAt: now}
	command.ResponseJSON, err = json.Marshal(result)
	if err != nil {
		return nil, err
	}
	if previous == 0 {
		proposals := []domain.Proposal{}
		for _, id := range proposalIDs {
			proposals = append(proposals, domain.Proposal{ID: id, WorkspaceID: cmd.WorkspaceID, State: domain.ProposalValidating, SubmittedAt: &now})
		}
		err = s.repo.SubmitOperationTx(ctx, cmd.WorkspaceID, cmd.OperationID, version, now, attempt, nil, nil, proposals, command)
	} else {
		err = s.repo.CreateValidationAttemptTx(ctx, cmd.WorkspaceID, cmd.OperationID, version, attempt, nil, nil, command)
	}
	if err != nil {
		if committed, lookupErr := s.repo.GetProductionCommand(ctx, cmd.WorkspaceID, cmd.PrincipalID, kind, cmd.IdempotencyKey); lookupErr == nil && committed != nil {
			return s.queueProductionValidation(ctx, cmd, previous, reason)
		}
		return nil, err
	}
	return result, nil
}

func ProductionFreshnessWitness(ver domain.ProductionVersion, rules []domain.PolicyRule, businessRules ...domain.ProductionBusinessRuleWitness) (json.RawMessage, error) {
	if businessRules == nil {
		businessRules = []domain.ProductionBusinessRuleWitness{}
	}
	return json.Marshal(map[string]any{"input": ver.InputJSON, "baseline": ver.BaselineJSON, "policyVersion": domain.RiskRuleVersion, "rules": rules, "businessRules": businessRules})
}

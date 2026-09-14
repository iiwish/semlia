package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/iiwish/semlia/internal/application/jobs"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type ProductionValidationWork struct {
	Version          domain.ProductionVersion
	Attempt          domain.ValidationAttempt
	Targets          []domain.ProductionTarget
	Proposals        map[string]domain.Proposal
	Rules            []domain.PolicyRule
	ReferenceFailure string
	BusinessRules    map[string]bool
}

type ProductionCheckResult struct {
	Check       ProductionCheck         `json:"check"`
	InputDigest string                  `json:"inputDigest"`
	Status      domain.ValidationStatus `json:"status"`
	Findings    []Finding               `json:"findings"`
}

type ProductionValidationCompletion struct {
	Attempt  domain.ValidationAttempt
	Checks   []ProductionCheckResult
	Policies []PolicyDecisionCommand
}

type ProductionValidationRepository interface {
	LoadProductionValidationWork(context.Context, identity.WorkspaceID, identity.ProductionOperationID, int, int) (ProductionValidationWork, error)
	CompleteProductionValidation(context.Context, ProductionValidationCompletion) error
}

type productionValidationFailureRepository interface {
	FailProductionValidation(context.Context, identity.WorkspaceID, identity.ProductionOperationID, int, int) error
}

func (handler *ValidationJobHandler) handleProduction(ctx context.Context, job jobs.Job, payload ValidationJobPayload) error {
	repo, ok := handler.repository.(ProductionValidationRepository)
	if !ok {
		return domain.ErrInvariant
	}
	op, err := identity.ParseProductionOperationID(payload.OperationID)
	if err != nil || payload.ProductionVersion < 1 || payload.Attempt < 1 || payload.Attempt > 256 || payload.ProposalID != "" {
		return domain.ErrInvalidArgument
	}
	work, err := repo.LoadProductionValidationWork(ctx, job.WorkspaceID, op, payload.ProductionVersion, payload.Attempt)
	if err != nil {
		return handler.finishProductionValidationFailure(ctx, job, payload, op, err)
	}
	if work.Attempt.Status == domain.ValidationStatusSucceeded || work.Attempt.Status == domain.ValidationStatusFailed {
		return nil
	}
	completion, err := EvaluateProductionValidation(work, handler.clock.Now().UTC())
	if err != nil {
		return handler.finishProductionValidationFailure(ctx, job, payload, op, err)
	}
	err = repo.CompleteProductionValidation(ctx, completion)
	return handler.finishProductionValidationFailure(ctx, job, payload, op, err)
}

func (handler *ValidationJobHandler) finishProductionValidationFailure(ctx context.Context, job jobs.Job, payload ValidationJobPayload, op identity.ProductionOperationID, cause error) error {
	if cause == nil || job.MaxAttempts < 1 || job.Attempt < job.MaxAttempts {
		return cause
	}
	repo, ok := handler.repository.(productionValidationFailureRepository)
	if !ok {
		return cause
	}
	if err := repo.FailProductionValidation(ctx, job.WorkspaceID, op, payload.ProductionVersion, payload.Attempt); err != nil {
		return err
	}
	return cause
}

func ProductionCheckInputDigest(target domain.ProductionTarget, attempt domain.ValidationAttempt, check ProductionCheck) (string, error) {
	input, err := json.Marshal(map[string]any{"check": check, "setDigest": attempt.SetDigest, "inputDigest": attempt.InputDigest, "freshnessDigest": attempt.FreshnessDigest, "contentDigest": target.ContentDigest, "content": target.ContentJSON, "changes": target.Changes})
	if err != nil {
		return "", err
	}
	return domain.DigestJSON(input)
}

// Validators execute over the immutable production business schema. Reference
// facts are resolved by the repository; structural and policy checks reuse the
// same deterministic engines as legacy governance, without inventing a base
// revision for a newly created target.
func EvaluateProductionValidation(work ProductionValidationWork, now time.Time) (ProductionValidationCompletion, error) {
	result := ProductionValidationCompletion{Attempt: work.Attempt, Checks: []ProductionCheckResult{}, Policies: []PolicyDecisionCommand{}}
	required, err := ProductionRequiredChecks(work.Targets)
	if err != nil {
		return result, err
	}
	raw, err := json.Marshal(required)
	if err != nil {
		return result, err
	}
	digest, err := domain.DigestJSON(raw)
	if err != nil {
		return result, err
	}
	if digest != work.Attempt.RequiredChecksDigest {
		return result, domain.ErrValidationRequired
	}
	local := map[string]string{}
	for _, target := range work.Targets {
		local[target.LocalKey] = target.TargetID
	}
	var baseline struct {
		Targets map[string]domain.ProductionTargetBaseline `json:"targets"`
	}
	if err := json.Unmarshal(work.Version.BaselineJSON, &baseline); err != nil {
		return result, err
	}
	for _, target := range work.Targets {
		if target.ProposalID == nil {
			continue
		}
		proposal, ok := work.Proposals[target.ProposalID.String()]
		if !ok {
			return result, domain.ErrInvariant
		}
		outcomes := ProposalValidationOutcomes{}
		for _, id := range []string{ValidatorIDSchema, ValidatorIDReference, ValidatorIDStructural, "policy"} {
			check := ProductionCheck{target.ProposalID.String(), id, ProductionValidatorVersion}
			digest, err := ProductionCheckInputDigest(target, work.Attempt, check)
			if err != nil {
				return result, err
			}
			findings := []Finding{}
			block := func(code string) {
				findings = append(findings, blockerFinding(digest, code, "production validation did not satisfy the required check"))
			}
			switch id {
			case ValidatorIDSchema:
				if domain.RequiresProductionBusinessRule(target) && !work.BusinessRules[target.LocalKey] {
					block("PRODUCTION_BUSINESS_RULE_UNCONFIRMED")
				}
				if target.Declaration == nil {
					block("PRODUCTION_SCHEMA_MISSING")
				} else if _, err := domain.InspectProductionContent(target.Kind, target.Declaration.Content); err != nil {
					block("PRODUCTION_SCHEMA_INVALID")
				} else if target.Kind == domain.TargetKindSemanticAsset {
					// Null is valid draft content, but cannot satisfy release readiness.
					var content struct {
						Definition *string `json:"definition"`
						Scope      *string `json:"scope"`
					}
					if err := json.Unmarshal(target.Declaration.Content, &content); err != nil {
						block("PRODUCTION_SCHEMA_INVALID")
					} else {
						if content.Definition == nil {
							block("PRODUCTION_DEFINITION_UNRESOLVED")
						}
						if content.Scope == nil {
							block("PRODUCTION_SCOPE_UNRESOLVED")
						}
					}
				}
			case ValidatorIDReference:
				if work.ReferenceFailure != "" {
					block(work.ReferenceFailure)
				}
			case ValidatorIDStructural:
				actual, err := domain.DigestJSON(target.ContentJSON)
				if err != nil || actual != target.ContentDigest {
					block("PRODUCTION_CONTENT_DIGEST_MISMATCH")
				}
				if target.Intent == domain.ProductionIntentUpdate && target.Declaration != nil {
					_, _, err := domain.ReplayProductionChanges(*target.Declaration, baseline.Targets[target.LocalKey].Content, local)
					if err != nil {
						block("PRODUCTION_CHANGE_REPLAY_FAILED")
					}
				}
				legacyInput := ValidationInput{Proposal: ValidationProposalSnapshot{ProposalID: proposal.ID.UUID(), TargetType: proposal.TargetObjectType, TargetObjectID: proposal.TargetObjectID}, Changes: target.Changes}
				checks, err := (&structuralValidator{}).Validate(legacyInput)
				if err != nil {
					return result, err
				}
				for _, finding := range checks {
					finding.InputDigest = digest
					findings = append(findings, finding)
				}
			case "policy":
				var content struct {
					AssetType string `json:"assetType"`
				}
				if err := json.Unmarshal(target.ContentJSON, &content); err != nil {
					return result, err
				}
				inputs, err := BuildDecisionInputs(proposal, target.Changes, outcomes, TargetInputs{AssetType: content.AssetType, OwnerContent: target.ContentJSON, EvidenceLinkCount: len(target.Declaration.EvidenceIDs)})
				if err != nil {
					return result, err
				}
				// Creating an object changes its complete contract, not an empty diff.
				if target.Intent == domain.ProductionIntentCreate {
					inputs.AffectsDefinition = true
					inputs.AffectsContract = target.Kind != domain.TargetKindSemanticAsset
				}
				decision, err := domain.EvaluatePolicyRules(work.Rules, inputs)
				if err != nil {
					return result, err
				}
				encoded, err := json.Marshal(inputs)
				if err != nil {
					return result, err
				}
				inputsDigest, err := domain.DigestJSON(encoded)
				if err != nil {
					return result, err
				}
				pid, err := identity.NewPolicyDecisionID()
				if err != nil {
					return result, err
				}
				result.Policies = append(result.Policies, PolicyDecisionCommand{ID: pid, WorkspaceID: proposal.WorkspaceID, ProposalID: proposal.ID, RuleVersion: domain.RiskRuleVersion, Inputs: encoded, InputsDigest: inputsDigest, MatchedPolicy: decision.MatchedPolicy, RiskLevel: decision.RiskLevel, Routing: decision.Routing, ReasonCode: decision.ReasonCode, DecidedAt: now, LinkProposal: true})
				findings = append(findings, Finding{Severity: domain.SeverityInfo, Code: decision.ReasonCode, Message: "production risk policy evaluated", InputDigest: digest, Details: encoded})
			}
			findings = append(findings, Finding{Severity: domain.SeverityInfo, Code: validatorCompletedCode(id), Message: fmt.Sprintf("%s@%s completed", id, ProductionValidatorVersion), InputDigest: digest, Details: json.RawMessage(`{}`)})
			status := domain.ValidationSucceeded
			for _, finding := range findings {
				switch finding.Severity {
				case domain.SeverityBlocker:
					status = domain.ValidationFailed
					outcomes.BlockerCount++
				case domain.SeverityWarning:
					outcomes.WarningCount++
				default:
					outcomes.InfoCount++
				}
			}
			if status == domain.ValidationFailed {
				outcomes.FailedCount++
			}
			outcomes.RunCount++
			for i := range findings {
				if len(findings[i].Details) == 0 {
					findings[i].Details = json.RawMessage(`{}`)
				}
			}
			result.Checks = append(result.Checks, ProductionCheckResult{check, digest, status, findings})
		}
	}
	return SealProductionValidation(result, now)
}

func SealProductionValidation(result ProductionValidationCompletion, now time.Time) (ProductionValidationCompletion, error) {
	if result.Attempt.WorkspaceID.IsZero() || result.Attempt.OperationID.IsZero() || result.Attempt.ProductionVersion < 1 || result.Attempt.AttemptNo < 1 || result.Attempt.AttemptNo > 256 || len(result.Checks) == 0 || len(result.Checks) > 256 {
		return result, domain.ErrInvalidArgument
	}
	seen := map[ProductionCheck]bool{}
	for _, check := range result.Checks {
		if seen[check.Check] || !domain.IsValidContentDigest(check.InputDigest) || len(check.Findings) == 0 {
			return result, domain.ErrValidationRequired
		}
		seen[check.Check] = true
		blocked := false
		for _, finding := range check.Findings {
			if finding.InputDigest != check.InputDigest {
				return result, domain.ErrValidationRequired
			}
			if finding.Severity == domain.SeverityBlocker {
				blocked = true
			}
		}
		if (check.Status != domain.ValidationSucceeded && check.Status != domain.ValidationFailed) || (check.Status == domain.ValidationFailed) != blocked {
			return result, domain.ErrValidationRequired
		}
	}
	raw, err := json.Marshal(result.Checks)
	if err != nil {
		return result, err
	}
	over := len(raw) > (1<<20)-(256*256)
	for _, check := range result.Checks {
		if len(check.Findings) > 256 {
			over = true
		}
	}
	if over {
		// Preserve every required check with an explicit failure rather than
		// exposing a truncated collection as a successful complete result.
		for i := range result.Checks {
			result.Checks[i].Status = domain.ValidationFailed
			result.Checks[i].Findings = []Finding{blockerFinding(result.Checks[i].InputDigest, "VALIDATION_RESULT_LIMIT_EXCEEDED", "validation result exceeded the complete-attempt budget")}
		}
		result.Policies = nil
	}
	status := domain.ValidationStatusSucceeded
	digests := []string{}
	for _, check := range result.Checks {
		if check.Status != domain.ValidationSucceeded {
			status = domain.ValidationStatusFailed
		}
		raw, err := json.Marshal(check)
		if err != nil {
			return result, err
		}
		digest, err := domain.DigestJSON(raw)
		if err != nil {
			return result, err
		}
		digests = append(digests, digest)
	}
	payload, err := json.Marshal(map[string]any{"schema": "semlia.production-validation/v1", "workspaceId": result.Attempt.WorkspaceID.String(), "operationId": result.Attempt.OperationID.String(), "productionVersion": result.Attempt.ProductionVersion, "attemptNo": result.Attempt.AttemptNo, "setDigest": result.Attempt.SetDigest, "inputDigest": result.Attempt.InputDigest, "requiredChecksDigest": result.Attempt.RequiredChecksDigest, "freshnessDigest": result.Attempt.FreshnessDigest, "checkDigests": digests})
	if err != nil {
		return result, err
	}
	digest, err := domain.DigestJSON(payload)
	if err != nil {
		return result, err
	}
	result.Attempt.Status = status
	result.Attempt.ValidationDigest = &digest
	result.Attempt.CompletedAt = &now
	return result, nil
}

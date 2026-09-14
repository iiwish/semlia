package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// ValidationJobRepository is everything the validator-execution loop needs:
// the T002 validation-run persistence, the frozen change-set read, the
// idempotent per-validator run lookup and the workspace-scoped reference
// resolution backed by the M1 physical graph.
type ValidationJobRepository interface {
	GetProposal(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (governance.Proposal, error)
	ListProposalChanges(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) ([]governance.ChangeSetItem, error)
	CreateValidationRun(ctx context.Context, run governance.ValidationRun) (governance.ValidationRun, error)
	GetValidationRun(ctx context.Context, workspace identity.WorkspaceID, run identity.ValidationRunID) (governance.ValidationRun, error)
	FinishValidationRun(ctx context.Context, command ValidationRunFinishCommand) (governance.ValidationRun, error)
	CreateValidationResult(ctx context.Context, result governance.ValidationResult) (governance.ValidationResult, error)
	ListRunResults(ctx context.Context, workspace identity.WorkspaceID, run identity.ValidationRunID) ([]governance.ValidationResult, error)
	GetProposalValidationRun(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID, validatorID, validatorVersion string) (governance.ValidationRun, error)
	ResolveValidationReference(ctx context.Context, workspace identity.WorkspaceID, token string) (ReferenceResolution, error)
	ValidationAssetExists(ctx context.Context, workspace identity.WorkspaceID, assetUUID string) (bool, error)
	ValidationAssetRevisionExists(ctx context.Context, workspace identity.WorkspaceID, assetUUID, revisionUUID string) (bool, error)
	ValidationGovernedObjectExists(ctx context.Context, workspace identity.WorkspaceID, objectType governance.TargetObjectType, objectUUID string) (bool, error)
}

// ValidationExecutor is the extracted validator-execution loop (packet
// refactor step): adding a validator is a registry entry, never an
// orchestration change. The loop runs every registry validator in stable
// order, records each run with its input digest and finishes it — succeeded
// for a clean run, failed when any blocker finding blocks release candidacy
// (§3.3). Infra errors abort the loop so the job framework can retry; they
// never become silent passes.
type ValidationExecutor struct {
	repository  ValidationJobRepository
	proposals   *ProposalService
	validations *ValidationService
	registry    *Registry
	decisions   *PolicyDecisionTrigger
}

func NewValidationExecutor(
	repository ValidationJobRepository,
	proposals *ProposalService,
	validations *ValidationService,
	registry *Registry,
	decisions *PolicyDecisionTrigger,
) *ValidationExecutor {
	if repository == nil || proposals == nil || validations == nil || registry == nil || decisions == nil {
		panic("validation executor requires repository, proposal service, validation service, registry and decision trigger")
	}
	return &ValidationExecutor{
		repository: repository, proposals: proposals, validations: validations,
		registry: registry, decisions: decisions,
	}
}

// Execute runs the deterministic validation pipeline for one proposal. A
// proposal that vanished completes without transitioning (no error loop);
// every other outcome — clean, blocker or infra failure — leaves the proposal
// in in_review with the outcome persisted and the policy decision ensured.
// A re-execution over a proposal that already sits in in_review (a crashed
// attempt after the transition) only re-ensures the decision, which is
// idempotent for unchanged state.
func (executor *ValidationExecutor) Execute(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID, traceID string,
) error {
	current, err := executor.repository.GetProposal(ctx, workspace, proposal)
	if err != nil {
		if errors.Is(err, governance.ErrNotFound) {
			return nil
		}
		return err
	}
	switch current.State {
	case governance.ProposalInReview:
		_, err = executor.decisions.EnsureDecisionForProposal(ctx, workspace, proposal)
		return err
	case governance.ProposalValidating:
	default:
		return nil
	}
	changes, err := executor.repository.ListProposalChanges(ctx, workspace, proposal)
	if err != nil {
		return err
	}
	input, err := executor.buildInput(ctx, workspace, current, changes)
	if err != nil {
		return err
	}
	for _, validator := range executor.registry.All() {
		if err := executor.runValidator(ctx, workspace, proposal, validator, input, traceID); err != nil {
			return err
		}
	}
	return executor.transitionToReview(ctx, workspace, proposal, traceID)
}

// runValidator persists one validator execution: reuse the run row from a
// crashed attempt when present (the unique index allows exactly one run per
// proposal per validator version), record only the findings not yet recorded,
// and finish with the §3.3 outcome.
func (executor *ValidationExecutor) runValidator(
	ctx context.Context,
	workspace identity.WorkspaceID,
	proposalID identity.ProposalID,
	validator Validator,
	input ValidationInput,
	traceID string,
) error {
	digest, err := ValidationInputDigest(validator.ID(), validator.Version(), input)
	if err != nil {
		return err
	}
	findings, err := validator.Validate(input)
	if err != nil {
		return fmt.Errorf("validator %s failed: %w", validator.ID(), err)
	}
	for index := range findings {
		if findings[index].InputDigest == "" {
			findings[index].InputDigest = digest
		}
	}
	findings = append(findings, completedFinding(validator, digest, len(findings)))
	run, err := executor.ensureRun(ctx, workspace, proposalID, validator, traceID)
	if err != nil {
		return err
	}
	if run.Status != governance.ValidationRunning {
		return nil
	}
	existing, err := executor.repository.ListRunResults(ctx, workspace, run.ID)
	if err != nil {
		return err
	}
	for _, finding := range missingFindings(existing, findings) {
		if _, err := executor.validations.RecordResult(ctx, RecordResultRequest{
			WorkspaceID: workspace, ValidationRunID: run.ID,
			Severity: finding.Severity, Code: finding.Code, Message: finding.Message,
			InputDigest: finding.InputDigest, Details: finding.Details,
		}); err != nil {
			return err
		}
	}
	outcome := governance.ValidationSucceeded
	for _, finding := range findings {
		if finding.Severity.BlocksRelease() {
			outcome = governance.ValidationFailed
		}
	}
	if _, err := executor.validations.FinishRun(ctx, FinishRunRequest{
		WorkspaceID: workspace, RunID: run.ID, Status: outcome,
	}); err != nil {
		return err
	}
	return nil
}

// ensureRun starts the run or reuses the row an earlier attempt already
// created. A terminal run means the outcome is already recorded.
func (executor *ValidationExecutor) ensureRun(
	ctx context.Context,
	workspace identity.WorkspaceID,
	proposalID identity.ProposalID,
	validator Validator,
	traceID string,
) (governance.ValidationRun, error) {
	existing, err := executor.repository.GetProposalValidationRun(
		ctx, workspace, proposalID, validator.ID(), validator.Version())
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, governance.ErrNotFound) {
		return governance.ValidationRun{}, err
	}
	return executor.validations.StartRun(ctx, StartRunRequest{
		WorkspaceID: workspace, ProposalID: proposalID,
		ValidatorID: validator.ID(), ValidatorVersion: validator.Version(),
	})
}

// buildInput assembles the deterministic snapshot: proposal coordinates,
// target existence and one resolution per TypeID token found in the
// change-set values. All I/O happens here; the validators stay pure.
func (executor *ValidationExecutor) buildInput(
	ctx context.Context,
	workspace identity.WorkspaceID,
	proposal governance.Proposal,
	changes []governance.ChangeSetItem,
) (ValidationInput, error) {
	input := ValidationInput{
		Proposal: ValidationProposalSnapshot{
			ProposalID:     proposal.ID.UUID(),
			TargetType:     proposal.TargetObjectType,
			TargetObjectID: proposal.TargetObjectID,
		},
		Target:  ValidationTargetSnapshot{Resolutions: map[string]ReferenceResolution{}},
		Changes: append([]governance.ChangeSetItem(nil), changes...),
	}
	switch proposal.TargetObjectType {
	case governance.TargetSemanticAsset:
		if proposal.AssetID == nil || proposal.BaseRevisionID == nil {
			return ValidationInput{}, fmt.Errorf(
				"%w: validating semantic-asset proposal %s without asset references",
				governance.ErrInvariant, proposal.ID)
		}
		input.Proposal.AssetID = proposal.AssetID.UUID()
		input.Proposal.BaseRevisionID = proposal.BaseRevisionID.UUID()
		assetExists, err := executor.repository.ValidationAssetExists(ctx, workspace, input.Proposal.AssetID)
		if err != nil {
			return ValidationInput{}, err
		}
		revisionExists, err := executor.repository.ValidationAssetRevisionExists(
			ctx, workspace, input.Proposal.AssetID, input.Proposal.BaseRevisionID)
		if err != nil {
			return ValidationInput{}, err
		}
		input.Target.AssetExists = assetExists
		input.Target.BaseRevisionExists = revisionExists
	default:
		exists, err := executor.repository.ValidationGovernedObjectExists(
			ctx, workspace, proposal.TargetObjectType, proposal.TargetObjectID)
		if err != nil {
			return ValidationInput{}, err
		}
		input.Target.TargetObjectExists = exists
	}
	for _, item := range changes {
		for _, token := range append(referenceTokens(item.BeforeValue), referenceTokens(item.AfterValue)...) {
			if _, resolved := input.Target.Resolutions[token]; resolved {
				continue
			}
			resolution, err := executor.repository.ResolveValidationReference(ctx, workspace, token)
			if err != nil {
				return ValidationInput{}, err
			}
			input.Target.Resolutions[token] = resolution
		}
	}
	return input, nil
}

// transitionToReview moves validating -> in_review through the T002 state
// machine and then ensures the §8.2 policy decision for the proposal. A
// proposal rejected concurrently is already outside validating; the job
// completes without transitioning instead of error-looping.
func (executor *ValidationExecutor) transitionToReview(
	ctx context.Context,
	workspace identity.WorkspaceID,
	proposalID identity.ProposalID,
	traceID string,
) error {
	proposal, err := executor.repository.GetProposal(ctx, workspace, proposalID)
	if err != nil {
		return err
	}
	if proposal.State != governance.ProposalValidating {
		return nil
	}
	if _, err := executor.proposals.Transition(ctx, TransitionRequest{
		WorkspaceID: workspace, ProposalID: proposalID,
		To: governance.ProposalInReview, Actor: "semlia-validation", TraceID: traceID,
	}); err != nil {
		if errors.Is(err, governance.ErrInvalidTransition) {
			return nil
		}
		return err
	}
	if _, err := executor.decisions.EnsureDecisionForProposal(ctx, workspace, proposalID); err != nil {
		return fmt.Errorf("ensure policy decision: %w", err)
	}
	return nil
}

// completedFinding is the run-level summary result: it carries the run input
// digest even for clean runs, so every recorded run is reproducible
// (SSOT NFR-003) without a schema migration.
func completedFinding(validator Validator, digest string, findingCount int) Finding {
	details, err := json.Marshal(map[string]any{"findingCount": findingCount})
	if err != nil {
		details = json.RawMessage(`{}`)
	}
	return Finding{
		Severity:    governance.SeverityInfo,
		Code:        validatorCompletedCode(validator.ID()),
		Message:     fmt.Sprintf("%s@%s completed with %d finding(s)", validator.ID(), validator.Version(), findingCount),
		Details:     details,
		InputDigest: digest,
	}
}

func validatorCompletedCode(validatorID string) string {
	code := ""
	for _, character := range validatorID {
		if character >= 'a' && character <= 'z' {
			code += string(character - 'a' + 'A')
			continue
		}
		code += string(character)
	}
	return code + "_COMPLETED"
}

// findingKey identifies one finding for idempotent recording: deterministic
// validators over identical inputs produce identical keys, so a retried job
// never duplicates results.
func findingKey(severity governance.ValidationSeverity, code, message, inputDigest string, details json.RawMessage) string {
	return string(severity) + "|" + code + "|" + message + "|" + inputDigest + "|" + string(details)
}

func missingFindings(existing []governance.ValidationResult, findings []Finding) []Finding {
	recorded := map[string]int{}
	for _, result := range existing {
		key := findingKey(result.Severity, result.Code, result.Message, result.InputDigest, result.Details)
		recorded[key]++
	}
	pending := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		key := findingKey(finding.Severity, finding.Code, finding.Message, finding.InputDigest, finding.Details)
		if recorded[key] > 0 {
			recorded[key]--
			continue
		}
		pending = append(pending, finding)
	}
	return pending
}

// ValidationJobHandler adapts the executor to the jobs framework. Transient
// infra errors return as handler errors so the framework retries with its
// lease/backoff semantics; the final attempt degrades to human handling
// (a failed orchestration run plus the validating -> in_review transition)
// instead of leaving the proposal stuck or looping silently.
type ValidationJobHandler struct {
	executor   *ValidationExecutor
	repository ValidationJobRepository
	clock      Clock
}

func NewValidationJobHandler(
	repository ValidationJobRepository,
	proposals *ProposalService,
	validations *ValidationService,
	registry *Registry,
	policy *PolicyService,
	clock Clock,
) *ValidationJobHandler {
	if repository == nil || proposals == nil || validations == nil || registry == nil || policy == nil || clock == nil {
		panic("validation job handler requires repository, services, registry, policy service and clock")
	}
	facts, ok := repository.(DecisionFactsRepository)
	if !ok {
		panic("validation job handler repository must expose the decision input facts")
	}
	return &ValidationJobHandler{
		executor:   NewValidationExecutor(repository, proposals, validations, registry, NewPolicyDecisionTrigger(facts, policy)),
		repository: repository,
		clock:      clock,
	}
}

func (handler *ValidationJobHandler) Handle(ctx context.Context, job jobs.Job) error {
	var payload ValidationJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("decode validation job payload: %w", err)
	}
	if payload.OperationID != "" {
		return handler.handleProduction(ctx, job, payload)
	}
	proposalID, err := identity.ParseProposalID(payload.ProposalID)
	if err != nil {
		return fmt.Errorf("parse validation job proposal: %w", err)
	}
	executeErr := handler.executor.Execute(ctx, job.WorkspaceID, proposalID, job.TraceID)
	if executeErr == nil {
		return nil
	}
	if job.Attempt < job.MaxAttempts {
		return executeErr
	}
	if degradeErr := handler.degrade(ctx, job, proposalID); degradeErr != nil {
		return degradeErr
	}
	return executeErr
}

// degrade records the human-handling failure outcome for an execution that
// exhausted its attempts: one failed run under the stable `orchestration`
// validator id with a single bounded finding (the raw infra error never
// reaches persistence), then the validating -> in_review transition when the
// proposal still sits in validating.
func (handler *ValidationJobHandler) degrade(ctx context.Context, job jobs.Job, proposalID identity.ProposalID) error {
	digest, err := governance.DigestJSON(json.RawMessage(fmt.Sprintf(
		`{"proposalId":%q,"reason":%q}`, proposalID.String(), "VALIDATION_INFRA_FAILURE")))
	if err != nil {
		return err
	}
	run, err := handler.repository.GetProposalValidationRun(ctx, job.WorkspaceID, proposalID, "orchestration", ValidatorVersion1)
	if errors.Is(err, governance.ErrNotFound) {
		runID, mintErr := identity.NewValidationRunID()
		if mintErr != nil {
			return mintErr
		}
		run, err = handler.repository.CreateValidationRun(ctx, governance.ValidationRun{
			ID: runID, WorkspaceID: job.WorkspaceID, ProposalID: proposalID,
			ValidatorID: "orchestration", ValidatorVersion: ValidatorVersion1,
			Status:    governance.ValidationRunning,
			StartedAt: handler.clock.Now().UTC(),
		})
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if run.Status == governance.ValidationRunning {
		now := handler.clock.Now().UTC()
		resultID, mintErr := identity.NewValidationResultID()
		if mintErr != nil {
			return mintErr
		}
		if _, err := handler.repository.CreateValidationResult(ctx, governance.ValidationResult{
			ID: resultID, WorkspaceID: job.WorkspaceID, ValidationRunID: run.ID,
			Severity:    governance.SeverityInfo,
			Code:        "VALIDATION_INFRA_FAILURE",
			Message:     "validation could not complete after its attempts; the proposal degrades to human handling",
			InputDigest: digest,
			Details:     json.RawMessage(`{}`),
			CreatedAt:   now,
		}); err != nil {
			return err
		}
		if _, err := handler.repository.FinishValidationRun(ctx, ValidationRunFinishCommand{
			WorkspaceID: job.WorkspaceID, RunID: run.ID, Status: governance.ValidationFailed,
			FinishedAt: now,
		}); err != nil {
			return err
		}
	}
	proposal, err := handler.repository.GetProposal(ctx, job.WorkspaceID, proposalID)
	if err != nil {
		return err
	}
	if proposal.State != governance.ProposalValidating {
		return nil
	}
	return handler.executor.transitionToReview(ctx, job.WorkspaceID, proposalID, job.TraceID)
}

package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/iiwish/semlia/internal/application/jobs"
	domain "github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

// ProductionAutoFinalizeJobType carries an AI-drafted production operation
// from a succeeded generation to a submitted, validated, review-pending
// state without requiring an operator to click through apply, submit, and
// re-validation. It only advances machine-driven steps: applying the
// suggestion, submitting for validation, and re-validating after a human
// records business-rule confirmations. Human review and publishing stay
// manual; failed validations with non-rule blockers stop the chain.
const ProductionAutoFinalizeJobType = "production.autofinalize"

const (
	autoFinalizePhaseFinalize = "finalize"
	autoFinalizePhaseRecheck  = "recheck"

	autoFinalizeInterval  = 2 * time.Minute
	autoFinalizeMaxRounds = 30
	autoRecheckInterval   = 5 * time.Minute
	autoRecheckMaxRounds  = 12
)

// AutoFinalizePayload pins one generation run to one operation version. The
// finalize phase applies the suggestion and submits; the recheck phase
// watches the validation attempt and re-validates once, after a human
// confirms the business rules that AI drafts may never self-confirm.
type AutoFinalizePayload struct {
	WorkspaceID      string `json:"workspaceId"`
	OperationID      string `json:"operationId"`
	Version          int    `json:"version"`
	RunID            string `json:"runId"`
	PrincipalID      string `json:"principalId"`
	Phase            string `json:"phase"`
	Round            int    `json:"round"`
	AutoValidatedFor int    `json:"autoValidatedFor"`
}

// AutoFinalizeRepository is the persistence the finalizer needs. The
// postgres Store implements it; the production service carries the rest.
type AutoFinalizeRepository interface {
	ReadProductionGeneration(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, operation identity.ProductionOperationID, run identity.AgentRunID) (domain.ProductionGenerationResult, error)
	EnqueueJob(ctx context.Context, params jobs.EnqueueJobParams) (jobs.Job, error)
}

// ProductionAutoFinalizeService advances auto-drafted operations without
// operator clicks. Every step re-reads authoritative state and stops when a
// human moved, edited, released, or must model the content.
type ProductionAutoFinalizeService struct {
	repo       AutoFinalizeRepository
	production *ProductionService
	clock      Clock
	logger     *slog.Logger
}

func NewProductionAutoFinalizeService(repo AutoFinalizeRepository, production *ProductionService, clock Clock, logger *slog.Logger) *ProductionAutoFinalizeService {
	if repo == nil || production == nil || clock == nil {
		panic("auto-finalize service requires repository, production, and clock")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ProductionAutoFinalizeService{repo: repo, production: production, clock: clock, logger: logger}
}

// EnqueueForAutodraft starts the finalize watch after an auto-draft
// generation was queued. It is called synchronously from the auto-draft
// batch so every AI draft has a machine-driven path to review-pending.
func (service *ProductionAutoFinalizeService) EnqueueForAutodraft(ctx context.Context, workspace identity.WorkspaceID, principal identity.PrincipalID, operation identity.ProductionOperationID, version int, run identity.AgentRunID) error {
	payload, err := json.Marshal(AutoFinalizePayload{
		WorkspaceID: workspace.String(), OperationID: operation.String(), Version: version,
		RunID: run.String(), PrincipalID: principal.String(), Phase: autoFinalizePhaseFinalize,
	})
	if err != nil {
		return err
	}
	jobID, err := identity.NewRunID()
	if err != nil {
		return err
	}
	_, err = service.repo.EnqueueJob(ctx, jobs.EnqueueJobParams{
		ID: jobID, WorkspaceID: workspace, Type: ProductionAutoFinalizeJobType,
		Payload: payload, MaxAttempts: 5, AvailableAt: service.clock.Now().UTC().Add(autoFinalizeInterval),
		IdempotencyKey: "production-autofinalize/" + run.String(), TraceID: newAutoDraftTraceID(),
	})
	return err
}

func (service *ProductionAutoFinalizeService) reschedule(ctx context.Context, payload AutoFinalizePayload, workspace identity.WorkspaceID, run identity.AgentRunID, phase string, round int) error {
	payload.Phase = phase
	payload.Round = round
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	interval := autoFinalizeInterval
	cap := autoFinalizeMaxRounds
	if phase == autoFinalizePhaseRecheck {
		interval = autoRecheckInterval
		cap = autoRecheckMaxRounds
	}
	if round > cap {
		service.logger.Info("auto-finalize watch exhausted", "operation", payload.OperationID, "phase", phase)
		return nil
	}
	jobID, err := identity.NewRunID()
	if err != nil {
		return err
	}
	_, err = service.repo.EnqueueJob(ctx, jobs.EnqueueJobParams{
		ID: jobID, WorkspaceID: workspace, Type: ProductionAutoFinalizeJobType,
		Payload: raw, MaxAttempts: 5, AvailableAt: service.clock.Now().UTC().Add(interval),
		IdempotencyKey: fmt.Sprintf("production-autofinalize/%s/%s/%d", run.String(), phase, round),
		TraceID:        newAutoDraftTraceID(),
	})
	return err
}

// JobHandler applies a succeeded suggestion, submits for validation, then
// watches the attempt until it succeeds or needs human modeling.
func (service *ProductionAutoFinalizeService) JobHandler() jobs.Handler {
	return func(ctx context.Context, job jobs.Job) error {
		var payload AutoFinalizePayload
		if json.Unmarshal(job.Payload, &payload) != nil {
			return domain.ErrInvalidArgument
		}
		workspace, err := identity.ParseWorkspaceID(payload.WorkspaceID)
		if err != nil {
			return domain.ErrInvalidArgument
		}
		operation, err := identity.ParseProductionOperationID(payload.OperationID)
		if err != nil {
			return domain.ErrInvalidArgument
		}
		run, err := identity.ParseAgentRunID(payload.RunID)
		if err != nil {
			return domain.ErrInvalidArgument
		}
		principal, err := identity.ParsePrincipalID(payload.PrincipalID)
		if err != nil {
			return domain.ErrInvalidArgument
		}
		switch payload.Phase {
		case autoFinalizePhaseRecheck:
			return service.recheck(ctx, job, payload, workspace, operation, run, principal)
		case autoFinalizePhaseFinalize, "":
			return service.finalize(ctx, job, payload, workspace, operation, run, principal)
		default:
			return domain.ErrInvalidArgument
		}
	}
}

func (service *ProductionAutoFinalizeService) finalize(ctx context.Context, job jobs.Job, payload AutoFinalizePayload, workspace identity.WorkspaceID, operation identity.ProductionOperationID, run identity.AgentRunID, principal identity.PrincipalID) error {
	result, err := service.repo.ReadProductionGeneration(ctx, workspace, principal, operation, run)
	if err != nil {
		return err
	}
	switch result.Status {
	case "queued", "running":
		return service.reschedule(ctx, payload, workspace, run, autoFinalizePhaseFinalize, payload.Round+1)
	case "succeeded":
	default:
		service.logger.Info("auto-finalize skipped: generation did not succeed", "operation", operation.String(), "status", result.Status)
		return nil
	}
	if len(result.Output) == 0 {
		return nil
	}
	_, version, _, _, _, err := service.production.GetOperation(ctx, workspace, operation)
	if err != nil {
		return err
	}
	if version.ReleaseID != nil {
		return nil
	}
	if version.Version != payload.Version {
		// Resume only our own committed replacement, never a later human edit.
		applied, err := service.production.repo.GetProductionCommand(ctx, workspace, principal, domain.CommandReplaceDraft, "autofinalize/"+run.String())
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if applied == nil || applied.OperationID != operation || applied.OperationVersion != version.Version {
			return nil
		}
		_, version, _, _, _, err = service.production.GetOperationVersion(ctx, workspace, operation, payload.Version)
		if err != nil {
			return err
		}
	}
	if result.InputDigest != version.InputDigest {
		service.logger.Info("auto-finalize skipped: draft input changed since generation", "operation", operation.String())
		return nil
	}
	if version.FrozenAt != nil {
		return service.reschedule(ctx, payload, workspace, run, autoFinalizePhaseRecheck, 0)
	}
	var output struct {
		Targets []domain.TargetDeclaration `json:"targets"`
	}
	if json.Unmarshal(result.Output, &output) != nil || len(output.Targets) == 0 {
		return domain.ErrInvalidArgument
	}
	candidates, err := autoFinalizeCandidates(version.InputJSON)
	if err != nil {
		return err
	}
	// version.InputJSON is the stored {"snapshotId": ..., "scope": ...}
	// envelope; ReplaceDraft takes the inner scope plus the snapshot pin and
	// re-envelopes them. Unwrap first: passing the envelope as the scope
	// double-wraps the input (validates as snapshot-less), and dropping the
	// snapshot pin changes the input digest the generation link verifies.
	scope, snapshotID, err := autoFinalizeInputScope(version.InputJSON)
	if err != nil {
		return err
	}
	replaced, err := service.production.ReplaceDraft(ctx, ReplaceDraftCommand{
		WorkspaceID: workspace, PrincipalID: principal, OperationID: operation,
		ExpectedVersion: version.Version, IdempotencyKey: "autofinalize/" + run.String(),
		InputSnapshotID: snapshotID, InputScope: scope, Targets: output.Targets, Candidates: candidates,
		SuggestionRunID: &run, TraceID: newAutoDraftTraceID(),
	})
	if err != nil {
		return fmt.Errorf("auto-finalize apply suggestion: %w", err)
	}
	submitted, err := service.production.SubmitOperation(ctx, SubmitOperationCommand{
		WorkspaceID: workspace, PrincipalID: principal, OperationID: operation,
		IdempotencyKey:  "autofinalize-submit/" + run.String(),
		ExpectedVersion: replaced.Version, SetDigest: replaced.SetDigest, TraceID: newAutoDraftTraceID(),
	})
	if err != nil {
		return fmt.Errorf("auto-finalize submit: %w", err)
	}
	service.logger.Info("auto-finalize submitted", "operation", operation.String(),
		"version", submitted.Version, "outcome", submitted.Outcome)
	payload.Version = submitted.Version
	return service.reschedule(ctx, payload, workspace, run, autoFinalizePhaseRecheck, 0)
}

func (service *ProductionAutoFinalizeService) recheck(ctx context.Context, job jobs.Job, payload AutoFinalizePayload, workspace identity.WorkspaceID, operation identity.ProductionOperationID, run identity.AgentRunID, principal identity.PrincipalID) error {
	_, version, targets, _, _, err := service.production.GetOperation(ctx, workspace, operation)
	if err != nil {
		return err
	}
	if version.Version != payload.Version || version.ReleaseID != nil {
		return nil
	}
	attempts, err := service.production.GetValidationAttempts(ctx, workspace, operation, version.Version, 64, 0)
	if err != nil {
		return err
	}
	var latest *domain.ValidationAttempt
	for i := range attempts {
		if latest == nil || attempts[i].AttemptNo > latest.AttemptNo {
			latest = &attempts[i]
		}
	}
	if latest == nil {
		return service.reschedule(ctx, payload, workspace, run, autoFinalizePhaseRecheck, payload.Round+1)
	}
	switch latest.Status {
	case domain.ValidationStatusSucceeded:
		service.logger.Info("auto-finalize complete: review-pending", "operation", operation.String())
		return nil
	case domain.ValidationStatusQueued, domain.ValidationStatusRunning:
		return service.reschedule(ctx, payload, workspace, run, autoFinalizePhaseRecheck, payload.Round+1)
	}
	if payload.AutoValidatedFor == latest.AttemptNo {
		return service.reschedule(ctx, payload, workspace, run, autoFinalizePhaseRecheck, payload.Round+1)
	}
	witnesses, err := service.production.BusinessRules(ctx, workspace, principal, operation, version.Version)
	if err != nil {
		return err
	}
	confirmed := map[string]bool{}
	for _, witness := range witnesses {
		if witness.Valid {
			confirmed[witness.Event.TargetKey] = true
		}
	}
	for _, target := range targets {
		if domain.RequiresProductionBusinessRule(target) && !confirmed[target.LocalKey] {
			return service.reschedule(ctx, payload, workspace, run, autoFinalizePhaseRecheck, payload.Round+1)
		}
	}
	payload.AutoValidatedFor = latest.AttemptNo
	if _, err := service.production.ValidateOperation(ctx, ValidateOperationCommand{
		WorkspaceID: workspace, PrincipalID: principal, OperationID: operation,
		IdempotencyKey:  fmt.Sprintf("autofinalize-revalidate/%s/%d", run.String(), latest.AttemptNo),
		ExpectedVersion: version.Version, SetDigest: version.SetDigest,
		PreviousAttemptNo: latest.AttemptNo,
		Reason:            "business-rule confirmations completed; automatic re-validation",
		TraceID:           newAutoDraftTraceID(),
	}); err != nil {
		return fmt.Errorf("auto-finalize re-validate: %w", err)
	}
	return service.reschedule(ctx, payload, workspace, run, autoFinalizePhaseRecheck, payload.Round+1)
}

// autoFinalizeInputScope unwraps the stored {"snapshotId": ..., "scope":
// ...} input envelope back to the scope plus snapshot pin ReplaceDraft
// expects. Inputs without an envelope pass through with no pin.
func autoFinalizeInputScope(inputJSON json.RawMessage) (json.RawMessage, *identity.SourceSnapshotID, error) {
	var envelope struct {
		SnapshotID *string         `json:"snapshotId"`
		Scope      json.RawMessage `json:"scope"`
	}
	if json.Unmarshal(inputJSON, &envelope) != nil || len(envelope.Scope) == 0 {
		return nil, nil, domain.ErrInvalidArgument
	}
	var snapshotID *identity.SourceSnapshotID
	if envelope.SnapshotID != nil {
		parsed, err := identity.ParseSourceSnapshotID(*envelope.SnapshotID)
		if err != nil {
			return nil, nil, domain.ErrInvalidArgument
		}
		snapshotID = &parsed
	}
	return envelope.Scope, snapshotID, nil
}

// autoFinalizeCandidates rebuilds the candidate declarations pinned in the
// frozen input scope so the suggestion application carries the same
// provenance the draft was created with.
func autoFinalizeCandidates(inputJSON json.RawMessage) ([]domain.CandidateDeclaration, error) {
	var input struct {
		Scope *json.RawMessage `json:"scope"`
	}
	if json.Unmarshal(inputJSON, &input) != nil || input.Scope == nil {
		return nil, domain.ErrInvalidArgument
	}
	var scope struct {
		Candidates []struct {
			CandidateID      string   `json:"candidateId"`
			CandidateDigest  string   `json:"candidateDigest"`
			Digest           string   `json:"digest"`
			TargetKeys       []string `json:"targetKeys"`
			PrimaryTargetKey string   `json:"primaryTargetKey"`
		} `json:"candidates"`
	}
	if json.Unmarshal(*input.Scope, &scope) != nil {
		return nil, domain.ErrInvalidArgument
	}
	declarations := make([]domain.CandidateDeclaration, 0, len(scope.Candidates))
	for _, candidate := range scope.Candidates {
		digest := candidate.CandidateDigest
		if strings.TrimSpace(digest) == "" {
			digest = candidate.Digest
		}
		declarations = append(declarations, domain.CandidateDeclaration{
			CandidateID: candidate.CandidateID, CandidateDigest: digest,
			TargetKeys: candidate.TargetKeys, PrimaryTargetKey: candidate.PrimaryTargetKey,
		})
	}
	return declarations, nil
}

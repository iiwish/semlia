package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type ValidationRepository interface {
	GetProposal(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (governance.Proposal, error)
	CreateValidationRun(ctx context.Context, run governance.ValidationRun) (governance.ValidationRun, error)
	GetValidationRun(ctx context.Context, workspace identity.WorkspaceID, run identity.ValidationRunID) (governance.ValidationRun, error)
	FinishValidationRun(ctx context.Context, command ValidationRunFinishCommand) (governance.ValidationRun, error)
	CreateValidationResult(ctx context.Context, result governance.ValidationResult) (governance.ValidationResult, error)
	ListRunResults(ctx context.Context, workspace identity.WorkspaceID, run identity.ValidationRunID) ([]governance.ValidationResult, error)
	CountBlockingResults(ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID) (int, error)
}

type ValidationRunFinishCommand struct {
	WorkspaceID identity.WorkspaceID
	RunID       identity.ValidationRunID
	Status      governance.ValidationStatus
	FinishedAt  time.Time
}

type ValidationService struct {
	repository ValidationRepository
	clock      Clock
}

func NewValidationService(repository ValidationRepository, clock Clock) *ValidationService {
	return &ValidationService{repository: repository, clock: clock}
}

type StartRunRequest struct {
	WorkspaceID      identity.WorkspaceID
	ProposalID       identity.ProposalID
	ValidatorID      string
	ValidatorVersion string
}

// StartRun records one deterministic validator execution. The proposal must
// exist in a pre-terminal governance state; one run per proposal per validator
// version is enforced by the repository (unique index).
func (service *ValidationService) StartRun(ctx context.Context, request StartRunRequest) (governance.ValidationRun, error) {
	if _, err := service.repository.GetProposal(ctx, request.WorkspaceID, request.ProposalID); err != nil {
		return governance.ValidationRun{}, err
	}
	if request.ValidatorID == "" || len(request.ValidatorID) > 64 || request.ValidatorVersion == "" {
		return governance.ValidationRun{}, governance.ErrInvalidArgument
	}
	run := governance.ValidationRun{
		WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
		ValidatorID: request.ValidatorID, ValidatorVersion: request.ValidatorVersion,
		Status: governance.ValidationRunning, StartedAt: service.clock.Now().UTC(),
	}
	runID, err := identity.NewValidationRunID()
	if err != nil {
		return governance.ValidationRun{}, fmt.Errorf("mint validation run ID: %w", err)
	}
	run.ID = runID
	return service.repository.CreateValidationRun(ctx, run)
}

type RecordResultRequest struct {
	WorkspaceID     identity.WorkspaceID
	ValidationRunID identity.ValidationRunID
	Severity        governance.ValidationSeverity
	Code            string
	Message         string
	InputDigest     string
	Details         json.RawMessage
}

// RecordResult appends one immutable finding to a running validation run.
func (service *ValidationService) RecordResult(ctx context.Context, request RecordResultRequest) (governance.ValidationResult, error) {
	run, err := service.repository.GetValidationRun(ctx, request.WorkspaceID, request.ValidationRunID)
	if err != nil {
		return governance.ValidationResult{}, err
	}
	if run.Status != governance.ValidationRunning {
		return governance.ValidationResult{}, governance.ErrConflict
	}
	result := governance.ValidationResult{
		WorkspaceID: request.WorkspaceID, ValidationRunID: request.ValidationRunID,
		Severity: request.Severity, Code: request.Code, Message: request.Message,
		InputDigest: request.InputDigest, Details: request.Details,
	}
	resultID, err := identity.NewValidationResultID()
	if err != nil {
		return governance.ValidationResult{}, fmt.Errorf("mint validation result ID: %w", err)
	}
	result.ID = resultID
	if err := result.Validate(); err != nil {
		return governance.ValidationResult{}, err
	}
	result.CreatedAt = service.clock.Now().UTC()
	return service.repository.CreateValidationResult(ctx, result)
}

type FinishRunRequest struct {
	WorkspaceID identity.WorkspaceID
	RunID       identity.ValidationRunID
	Status      governance.ValidationStatus
}

// FinishRun closes a running run. A run cannot be declared successful while
// its own findings contain a Blocker: Blocker severity blocks release
// candidacy (§3.3), so the caller must mark the run failed or cancelled.
func (service *ValidationService) FinishRun(ctx context.Context, request FinishRunRequest) (governance.ValidationRun, error) {
	run, err := service.repository.GetValidationRun(ctx, request.WorkspaceID, request.RunID)
	if err != nil {
		return governance.ValidationRun{}, err
	}
	if run.Status != governance.ValidationRunning {
		return governance.ValidationRun{}, governance.ErrConflict
	}
	if !request.Status.Terminal() {
		return governance.ValidationRun{}, governance.ErrInvalidArgument
	}
	if request.Status == governance.ValidationSucceeded {
		results, err := service.repository.ListRunResults(ctx, request.WorkspaceID, request.RunID)
		if err != nil {
			return governance.ValidationRun{}, err
		}
		for _, result := range results {
			if result.Severity.BlocksRelease() {
				return governance.ValidationRun{}, fmt.Errorf(
					"%w: blocker findings block release candidacy", governance.ErrInvariant)
			}
		}
	}
	return service.repository.FinishValidationRun(ctx, ValidationRunFinishCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, Status: request.Status,
		FinishedAt: service.clock.Now().UTC(),
	})
}

// HasBlockers reports whether reliable (running or succeeded) runs carry
// Blocker findings for the proposal. Findings from failed or cancelled runs
// are unusable — the harness failure invalidated them.
func (service *ValidationService) HasBlockers(
	ctx context.Context, workspace identity.WorkspaceID, proposal identity.ProposalID,
) (bool, error) {
	count, err := service.repository.CountBlockingResults(ctx, workspace, proposal)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

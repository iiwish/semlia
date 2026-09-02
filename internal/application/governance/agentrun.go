package governance

import (
	"context"
	"fmt"
	"time"

	"github.com/iiwish/semlia/internal/domain/governance"
	"github.com/iiwish/semlia/pkg/identity"
)

type AgentRunRepository interface {
	CreateAgentRun(ctx context.Context, run governance.AgentRun) (governance.AgentRun, error)
	GetAgentRun(ctx context.Context, workspace identity.WorkspaceID, run identity.AgentRunID) (governance.AgentRun, error)
	FinishAgentRun(ctx context.Context, command AgentRunFinishCommand) (governance.AgentRun, error)
	CreateAgentStep(ctx context.Context, step governance.AgentStep) (governance.AgentStep, error)
}

type AgentRunFinishCommand struct {
	WorkspaceID  identity.WorkspaceID
	RunID        identity.AgentRunID
	FinalState   governance.AgentRunState
	OutputDigest *string
	CostMicros   int64
	FinishedAt   time.Time
	DurationMS   int64
}

type AgentRunService struct {
	repository AgentRunRepository
	clock      Clock
}

func NewAgentRunService(repository AgentRunRepository, clock Clock) *AgentRunService {
	return &AgentRunService{repository: repository, clock: clock}
}

type StartAgentRunRequest struct {
	WorkspaceID    identity.WorkspaceID
	PrincipalID    *identity.PrincipalID
	Model          string
	ConfigRevision string
	InputHash      string
}

// Start opens an agent run in the running state. InputHash must be the sha256
// content digest of the canonical input; the request shape carries no prompt
// or credential field at all, so raw text is unrepresentable (SSOT §8.6).
func (service *AgentRunService) Start(ctx context.Context, request StartAgentRunRequest) (governance.AgentRun, error) {
	if request.WorkspaceID.IsZero() || request.Model == "" || len(request.Model) > 128 ||
		request.ConfigRevision == "" || len(request.ConfigRevision) > 128 ||
		!governance.IsValidContentDigest(request.InputHash) {
		return governance.AgentRun{}, governance.ErrInvalidArgument
	}
	runID, err := identity.NewAgentRunID()
	if err != nil {
		return governance.AgentRun{}, fmt.Errorf("mint agent run ID: %w", err)
	}
	run := governance.AgentRun{
		ID: runID, WorkspaceID: request.WorkspaceID, PrincipalID: request.PrincipalID,
		Model: request.Model, ConfigRevision: request.ConfigRevision,
		InputHash: request.InputHash, Status: governance.AgentRunRunning,
		CostMicros: 0, StartedAt: service.clock.Now().UTC(), CreatedAt: service.clock.Now().UTC(),
	}
	return service.repository.CreateAgentRun(ctx, run)
}

type RecordStepRequest struct {
	WorkspaceID identity.WorkspaceID
	AgentRunID  identity.AgentRunID
	Sequence    int
	Kind        governance.AgentStepKind
	ToolName    string
	InputHash   string
	OutputHash  string
	ErrorCode   string
}

// RecordStep appends one immutable model or tool call to a running agent run.
func (service *AgentRunService) RecordStep(ctx context.Context, request RecordStepRequest) (governance.AgentStep, error) {
	run, err := service.repository.GetAgentRun(ctx, request.WorkspaceID, request.AgentRunID)
	if err != nil {
		return governance.AgentStep{}, err
	}
	if run.Status != governance.AgentRunRunning {
		return governance.AgentStep{}, governance.ErrConflict
	}
	step := governance.AgentStep{
		WorkspaceID: request.WorkspaceID, AgentRunID: request.AgentRunID,
		Sequence: request.Sequence, Kind: request.Kind, InputHash: request.InputHash,
		OutputHash: request.OutputHash,
	}
	if request.ToolName != "" {
		toolName := request.ToolName
		step.ToolName = &toolName
	}
	if request.ErrorCode != "" {
		errorCode := request.ErrorCode
		step.ErrorCode = &errorCode
	}
	stepID, err := identity.NewAgentStepID()
	if err != nil {
		return governance.AgentStep{}, fmt.Errorf("mint agent step ID: %w", err)
	}
	step.ID = stepID
	if err := step.Validate(); err != nil {
		return governance.AgentStep{}, err
	}
	step.CreatedAt = service.clock.Now().UTC()
	return service.repository.CreateAgentStep(ctx, step)
}

type FinishAgentRunRequest struct {
	WorkspaceID  identity.WorkspaceID
	RunID        identity.AgentRunID
	FinalState   governance.AgentRunState
	OutputDigest string
	CostMicros   int64
}

// Finish moves a running run to its terminal final state, recording output
// digest, cost and duration. The final state is terminal: finishing twice is
// a conflict.
func (service *AgentRunService) Finish(ctx context.Context, request FinishAgentRunRequest) (governance.AgentRun, error) {
	run, err := service.repository.GetAgentRun(ctx, request.WorkspaceID, request.RunID)
	if err != nil {
		return governance.AgentRun{}, err
	}
	if run.Status != governance.AgentRunRunning || !request.FinalState.Valid() ||
		request.FinalState == governance.AgentRunRunning {
		return governance.AgentRun{}, governance.ErrConflict
	}
	var outputDigest *string
	if request.OutputDigest != "" {
		if !governance.IsValidContentDigest(request.OutputDigest) {
			return governance.AgentRun{}, governance.ErrInvalidArgument
		}
		outputDigest = &request.OutputDigest
	}
	if request.FinalState == governance.AgentRunSucceeded && outputDigest == nil {
		return governance.AgentRun{}, governance.ErrInvalidArgument
	}
	now := service.clock.Now().UTC()
	duration := now.Sub(run.StartedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	return service.repository.FinishAgentRun(ctx, AgentRunFinishCommand{
		WorkspaceID: request.WorkspaceID, RunID: request.RunID, FinalState: request.FinalState,
		OutputDigest: outputDigest, CostMicros: request.CostMicros, FinishedAt: now, DurationMS: duration,
	})
}

func (service *AgentRunService) GetRun(
	ctx context.Context, workspace identity.WorkspaceID, run identity.AgentRunID,
) (governance.AgentRun, error) {
	return service.repository.GetAgentRun(ctx, workspace, run)
}

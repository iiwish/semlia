package governance

import (
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

// AgentRunState is the §8.6 final-state contract for one agent execution.
type AgentRunState string

const (
	AgentRunRunning   AgentRunState = "running"
	AgentRunSucceeded AgentRunState = "succeeded"
	AgentRunFailed    AgentRunState = "failed"
	AgentRunCancelled AgentRunState = "cancelled"
)

func (state AgentRunState) Valid() bool {
	switch state {
	case AgentRunRunning, AgentRunSucceeded, AgentRunFailed, AgentRunCancelled:
		return true
	}
	return false
}

func (state AgentRunState) Terminal() bool {
	return state == AgentRunSucceeded || state == AgentRunFailed || state == AgentRunCancelled
}

type AgentStepKind string

const (
	AgentStepModel AgentStepKind = "model"
	AgentStepTool  AgentStepKind = "tool"
)

func (kind AgentStepKind) Valid() bool {
	return kind == AgentStepModel || kind == AgentStepTool
}

// AgentRun records the §8.6 AI behavior contract: model, configuration
// revision, input hash, output digest, cost, duration and final state. The
// input is stored as a hash only — raw prompts and provider credentials are
// structurally unrepresentable in this schema.
type AgentRun struct {
	ID             identity.AgentRunID
	WorkspaceID    identity.WorkspaceID
	PrincipalID    *identity.PrincipalID
	Model          string
	ConfigRevision string
	InputHash      string
	Status         AgentRunState
	OutputDigest   *string
	CostMicros     int64
	StartedAt      time.Time
	FinishedAt     *time.Time
	DurationMS     *int64
	CreatedAt      time.Time
}

// AgentStep is one immutable model or tool call inside an agent run, carrying
// hashes instead of payloads.
type AgentStep struct {
	ID          identity.AgentStepID
	WorkspaceID identity.WorkspaceID
	AgentRunID  identity.AgentRunID
	Sequence    int
	Kind        AgentStepKind
	ToolName    *string
	InputHash   string
	OutputHash  string
	ErrorCode   *string
	CreatedAt   time.Time
}

func (step AgentStep) Validate() error {
	if step.ID.IsZero() || step.WorkspaceID.IsZero() || step.AgentRunID.IsZero() ||
		step.Sequence < 1 || !step.Kind.Valid() ||
		!isContentDigest(step.InputHash) || !isContentDigest(step.OutputHash) {
		return ErrInvalidArgument
	}
	if step.Kind == AgentStepTool && (step.ToolName == nil || *step.ToolName == "") {
		return ErrInvalidArgument
	}
	if step.Kind == AgentStepModel && step.ToolName != nil {
		return ErrInvalidArgument
	}
	return nil
}

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

// ValidationJobType is the only job kind the T004 orchestration enqueues. The
// payload carries the proposal TypeID (never the storage UUID) and the
// orchestration attempt.
const ValidationJobType = "governance.proposal.validate"

// ValidationJobMaxAttempts bounds the lease/retry cycle of one validation
// job; the final attempt degrades to human handling instead of looping.
const ValidationJobMaxAttempts int32 = 3

// ValidationJobPayload is the decoded job body.
type ValidationJobPayload struct {
	ProposalID string `json:"proposalId"`
	Attempt    int    `json:"attempt"`
}

// ValidationJobIdempotencyKey pins one validation job per proposal and
// orchestration attempt: the jobs table unique index collapses concurrent
// enqueues into exactly one row.
func ValidationJobIdempotencyKey(proposal identity.ProposalID, attempt int) string {
	return fmt.Sprintf("%s:%s:%d", ValidationJobType, proposal.UUID(), attempt)
}

// JobEnqueuer is the jobs-framework write port of the orchestrator.
type JobEnqueuer interface {
	EnqueueJob(ctx context.Context, input jobs.EnqueueJobParams) (jobs.Job, error)
}

// ValidationOrchestrator implements the §8.2 hand-off between submission and
// deterministic validation: after the T003 submit transition to proposed, it
// moves the proposal to validating and enqueues exactly one validation job.
type ValidationOrchestrator struct {
	proposals *ProposalService
	enqueuer  JobEnqueuer
	clock     Clock
}

func NewValidationOrchestrator(proposals *ProposalService, enqueuer JobEnqueuer, clock Clock) *ValidationOrchestrator {
	if proposals == nil || enqueuer == nil || clock == nil {
		panic("validation orchestrator requires the proposal service, job enqueuer and clock")
	}
	return &ValidationOrchestrator{proposals: proposals, enqueuer: enqueuer, clock: clock}
}

type BeginValidationRequest struct {
	WorkspaceID identity.WorkspaceID
	ProposalID  identity.ProposalID
	Actor       string
	Attempt     int
	TraceID     string
}

// BeginValidation is idempotent: the validating transition is a no-op when a
// crashed submit already applied it, and the idempotency key collapses
// concurrent enqueues into exactly one job row. The proposal must be in the
// proposed or validating state; anything else is refused.
func (orchestrator *ValidationOrchestrator) BeginValidation(ctx context.Context, request BeginValidationRequest) (governance.Proposal, error) {
	if request.WorkspaceID.IsZero() || request.ProposalID.IsZero() ||
		(request.Attempt != 0 && request.Attempt < 1) {
		return governance.Proposal{}, governance.ErrInvalidArgument
	}
	attempt := request.Attempt
	if attempt < 1 {
		attempt = 1
	}
	proposal, err := orchestrator.proposals.GetProposal(ctx, request.WorkspaceID, request.ProposalID)
	if err != nil {
		return governance.Proposal{}, err
	}
	if proposal.State != governance.ProposalProposed && proposal.State != governance.ProposalValidating {
		return governance.Proposal{}, fmt.Errorf(
			"%w: validation begins from proposed, proposal is %s", governance.ErrInvalidTransition, proposal.State)
	}
	proposal, err = orchestrator.proposals.Transition(ctx, TransitionRequest{
		WorkspaceID: request.WorkspaceID, ProposalID: request.ProposalID,
		To: governance.ProposalValidating, Actor: request.Actor, TraceID: request.TraceID,
	})
	if err != nil {
		return governance.Proposal{}, err
	}
	payload, err := json.Marshal(ValidationJobPayload{ProposalID: request.ProposalID.String(), Attempt: attempt})
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("encode validation job payload: %w", err)
	}
	jobID, err := identity.NewRunID()
	if err != nil {
		return governance.Proposal{}, fmt.Errorf("mint validation job ID: %w", err)
	}
	if _, err := orchestrator.enqueuer.EnqueueJob(ctx, jobs.EnqueueJobParams{
		ID: jobID, WorkspaceID: request.WorkspaceID, Type: ValidationJobType,
		Payload: payload, MaxAttempts: ValidationJobMaxAttempts,
		AvailableAt:    orchestrator.clock.Now().UTC(),
		IdempotencyKey: ValidationJobIdempotencyKey(request.ProposalID, attempt),
		TraceID:        request.TraceID,
	}); err != nil {
		return governance.Proposal{}, fmt.Errorf("enqueue validation job: %w", err)
	}
	return proposal, nil
}

var errOrchestrationNotConfigured = errors.New("validation orchestration is not configured")

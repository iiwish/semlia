package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/iiwish/semlia/pkg/identity"
)

type Job struct {
	ID             identity.RunID
	WorkspaceID    identity.WorkspaceID
	Type           string
	Payload        json.RawMessage
	Status         string
	Attempt        int32
	MaxAttempts    int32
	AvailableAt    time.Time
	LeasedUntil    *time.Time
	LeaseOwner     string
	IdempotencyKey string
	LastErrorCode  string
	TraceID        string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}

type EnqueueJobParams struct {
	ID             identity.RunID
	WorkspaceID    identity.WorkspaceID
	Type           string
	Payload        json.RawMessage
	MaxAttempts    int32
	AvailableAt    time.Time
	IdempotencyKey string
	TraceID        string
}

type AuditEvent struct {
	ID          identity.EventID
	WorkspaceID identity.WorkspaceID
	Type        string
	ActorID     string
	Payload     json.RawMessage
	TraceID     string
	CreatedAt   time.Time
}

type OutboxEvent struct {
	ID            identity.EventID
	WorkspaceID   identity.WorkspaceID
	Type          string
	Payload       json.RawMessage
	Status        string
	Attempt       int32
	MaxAttempts   int32
	AvailableAt   time.Time
	LeasedUntil   *time.Time
	LeaseOwner    string
	LastErrorCode string
	TraceID       string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	PublishedAt   *time.Time
}

type Clock interface {
	Now() time.Time
}

type ClockFunc func() time.Time

func (fn ClockFunc) Now() time.Time { return fn() }

type Backoff interface {
	Delay(attempt int32) time.Duration
}

type BackoffFunc func(attempt int32) time.Duration

func (fn BackoffFunc) Delay(attempt int32) time.Duration { return fn(attempt) }

type Handler func(context.Context, Job) error

type HandlerError struct {
	Err       error
	Code      string
	Permanent bool
}

func (failure *HandlerError) Error() string { return failure.Err.Error() }
func (failure *HandlerError) Unwrap() error { return failure.Err }

func Permanent(err error, code string) error {
	if err == nil {
		err = errors.New("permanent job failure")
	}
	return &HandlerError{Err: err, Code: code, Permanent: true}
}

type Publisher interface {
	Publish(context.Context, OutboxEvent) error
}

type Repository interface {
	ReapExpiredJobs(context.Context, time.Time) error
	ClaimJob(context.Context, string, time.Time, time.Duration) (*Job, error)
	ExtendJobLease(context.Context, identity.RunID, string, time.Time, time.Duration) error
	MarkJobSucceeded(context.Context, identity.RunID, string, time.Time) error
	MarkJobFailedWithDisposition(context.Context, identity.RunID, string, string, bool, time.Time, time.Time) error
}

type OutboxRepository interface {
	ReapExpiredOutboxEvents(context.Context, time.Time) error
	ClaimOutboxEvent(context.Context, string, time.Time, time.Duration) (*OutboxEvent, error)
	MarkOutboxEventPublished(context.Context, identity.EventID, string, time.Time) error
	MarkOutboxEventFailed(context.Context, identity.EventID, string, string, time.Time, time.Time) error
}

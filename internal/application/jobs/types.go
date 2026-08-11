package jobs

import (
	"context"
	"encoding/json"
	"time"
)

type Job struct {
	ID             string
	WorkspaceID    string
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
	ID             string
	WorkspaceID    string
	Type           string
	Payload        json.RawMessage
	MaxAttempts    int32
	AvailableAt    time.Time
	IdempotencyKey string
	TraceID        string
}

type AuditEvent struct {
	ID          string
	WorkspaceID string
	Type        string
	ActorID     string
	Payload     json.RawMessage
	TraceID     string
	CreatedAt   time.Time
}

type OutboxEvent struct {
	ID            string
	WorkspaceID   string
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

type Publisher interface {
	Publish(context.Context, OutboxEvent) error
}

type Repository interface {
	ReapExpiredJobs(context.Context, time.Time) error
	ClaimJob(context.Context, string, time.Time, time.Duration) (*Job, error)
	MarkJobSucceeded(context.Context, string, string, time.Time) error
	MarkJobFailed(context.Context, string, string, string, time.Time, time.Time) error
}

type OutboxRepository interface {
	ReapExpiredOutboxEvents(context.Context, time.Time) error
	ClaimOutboxEvent(context.Context, string, time.Time, time.Duration) (*OutboxEvent, error)
	MarkOutboxEventPublished(context.Context, string, string, time.Time) error
	MarkOutboxEventFailed(context.Context, string, string, string, time.Time, time.Time) error
}

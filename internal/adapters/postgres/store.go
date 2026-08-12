package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	"github.com/iiwish/semlia/internal/application/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrLeaseLost = errors.New("lease ownership was lost")

type Store struct {
	pool    *Pool
	queries *dbgen.Queries
}

type TxStore struct {
	queries *dbgen.Queries
}

func NewStore(pool *Pool) *Store {
	return &Store{pool: pool, queries: dbgen.New(pool)}
}

func (store *Store) EnqueueJob(ctx context.Context, input jobs.EnqueueJobParams) (jobs.Job, error) {
	row, err := store.queries.EnqueueJob(ctx, dbgen.EnqueueJobParams{
		ID:             input.ID,
		WorkspaceID:    input.WorkspaceID,
		JobType:        input.Type,
		Payload:        input.Payload,
		MaxAttempts:    input.MaxAttempts,
		AvailableAt:    timestamp(input.AvailableAt),
		IdempotencyKey: input.IdempotencyKey,
		TraceID:        input.TraceID,
	})
	if err != nil {
		return jobs.Job{}, fmt.Errorf("enqueue job: %w", err)
	}
	return jobFromRow(row), nil
}

func (store *Store) ReapExpiredJobs(ctx context.Context, now time.Time) error {
	if _, err := store.queries.ReapExpiredJobs(ctx, timestamp(now)); err != nil {
		return fmt.Errorf("reap expired job leases: %w", err)
	}
	return nil
}

func (store *Store) ClaimJob(ctx context.Context, owner string, now time.Time, leaseDuration time.Duration) (*jobs.Job, error) {
	row, err := store.queries.ClaimJob(ctx, dbgen.ClaimJobParams{
		LeasedUntil: timestamp(now.Add(leaseDuration)),
		LeaseOwner:  textValue(owner),
		ClaimedAt:   timestamp(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim job lease: %w", err)
	}
	job := jobFromRow(row)
	return &job, nil
}

func (store *Store) MarkJobSucceeded(ctx context.Context, id, owner string, completedAt time.Time) error {
	rows, err := store.queries.MarkJobSucceeded(ctx, dbgen.MarkJobSucceededParams{
		CompletedAt: timestamp(completedAt),
		ID:          id,
		LeaseOwner:  textValue(owner),
	})
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return requireLease(rows)
}

func (store *Store) MarkJobFailed(
	ctx context.Context,
	id, owner, errorCode string,
	availableAt, failedAt time.Time,
) error {
	rows, err := store.queries.MarkJobFailed(ctx, dbgen.MarkJobFailedParams{
		AvailableAt: timestamp(availableAt),
		ErrorCode:   textValue(errorCode),
		FailedAt:    timestamp(failedAt),
		ID:          id,
		LeaseOwner:  textValue(owner),
	})
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	return requireLease(rows)
}

func (store *Store) ReapExpiredOutboxEvents(ctx context.Context, now time.Time) error {
	if _, err := store.queries.ReapExpiredOutboxEvents(ctx, timestamp(now)); err != nil {
		return fmt.Errorf("reap expired outbox leases: %w", err)
	}
	return nil
}

func (store *Store) ClaimOutboxEvent(
	ctx context.Context,
	owner string,
	now time.Time,
	leaseDuration time.Duration,
) (*jobs.OutboxEvent, error) {
	row, err := store.queries.ClaimOutboxEvent(ctx, dbgen.ClaimOutboxEventParams{
		LeasedUntil: timestamp(now.Add(leaseDuration)),
		LeaseOwner:  textValue(owner),
		ClaimedAt:   timestamp(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claim outbox lease: %w", err)
	}
	event := outboxFromRow(row)
	return &event, nil
}

func (store *Store) MarkOutboxEventPublished(ctx context.Context, id, owner string, publishedAt time.Time) error {
	rows, err := store.queries.MarkOutboxEventPublished(ctx, dbgen.MarkOutboxEventPublishedParams{
		PublishedAt: timestamp(publishedAt),
		ID:          id,
		LeaseOwner:  textValue(owner),
	})
	if err != nil {
		return fmt.Errorf("publish outbox event: %w", err)
	}
	return requireLease(rows)
}

func (store *Store) MarkOutboxEventFailed(
	ctx context.Context,
	id, owner, errorCode string,
	availableAt, failedAt time.Time,
) error {
	rows, err := store.queries.MarkOutboxEventFailed(ctx, dbgen.MarkOutboxEventFailedParams{
		AvailableAt: timestamp(availableAt),
		ErrorCode:   textValue(errorCode),
		FailedAt:    timestamp(failedAt),
		ID:          id,
		LeaseOwner:  textValue(owner),
	})
	if err != nil {
		return fmt.Errorf("fail outbox event: %w", err)
	}
	return requireLease(rows)
}

func (store *Store) WithTx(ctx context.Context, operation func(*TxStore) error) error {
	if operation == nil {
		return errors.New("transaction operation is required")
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txStore := &TxStore{queries: dbgen.New(tx)}
	if err := operation(txStore); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func (store *TxStore) CreateAuditEvent(ctx context.Context, event jobs.AuditEvent) error {
	actorID := pgtype.Text{}
	if event.ActorID != "" {
		actorID = textValue(event.ActorID)
	}
	if err := store.queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID:          event.ID,
		WorkspaceID: event.WorkspaceID,
		EventType:   event.Type,
		ActorID:     actorID,
		Payload:     event.Payload,
		TraceID:     event.TraceID,
		CreatedAt:   timestamp(event.CreatedAt),
	}); err != nil {
		return fmt.Errorf("create audit event: %w", err)
	}
	return nil
}

func (store *TxStore) EnqueueOutbox(ctx context.Context, event jobs.OutboxEvent) error {
	if err := store.queries.EnqueueOutboxEvent(ctx, dbgen.EnqueueOutboxEventParams{
		ID:          event.ID,
		WorkspaceID: event.WorkspaceID,
		EventType:   event.Type,
		Payload:     event.Payload,
		MaxAttempts: event.MaxAttempts,
		AvailableAt: timestamp(event.AvailableAt),
		TraceID:     event.TraceID,
	}); err != nil {
		return fmt.Errorf("enqueue outbox event: %w", err)
	}
	return nil
}

func requireLease(rows int64) error {
	if rows != 1 {
		return ErrLeaseLost
	}
	return nil
}

func timestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func textValue(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}

func jobFromRow(row dbgen.Job) jobs.Job {
	return jobs.Job{
		ID:             row.ID,
		WorkspaceID:    row.WorkspaceID,
		Type:           row.JobType,
		Payload:        cloneJSON(row.Payload),
		Status:         row.Status,
		Attempt:        row.Attempt,
		MaxAttempts:    row.MaxAttempts,
		AvailableAt:    row.AvailableAt.Time,
		LeasedUntil:    optionalTime(row.LeasedUntil),
		LeaseOwner:     optionalText(row.LeaseOwner),
		IdempotencyKey: row.IdempotencyKey,
		LastErrorCode:  optionalText(row.LastErrorCode),
		TraceID:        row.TraceID,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
		CompletedAt:    optionalTime(row.CompletedAt),
	}
}

func outboxFromRow(row dbgen.OutboxEvent) jobs.OutboxEvent {
	return jobs.OutboxEvent{
		ID:            row.ID,
		WorkspaceID:   row.WorkspaceID,
		Type:          row.EventType,
		Payload:       cloneJSON(row.Payload),
		Status:        row.Status,
		Attempt:       row.Attempt,
		MaxAttempts:   row.MaxAttempts,
		AvailableAt:   row.AvailableAt.Time,
		LeasedUntil:   optionalTime(row.LeasedUntil),
		LeaseOwner:    optionalText(row.LeaseOwner),
		LastErrorCode: optionalText(row.LastErrorCode),
		TraceID:       row.TraceID,
		CreatedAt:     row.CreatedAt.Time,
		UpdatedAt:     row.UpdatedAt.Time,
		PublishedAt:   optionalTime(row.PublishedAt),
	}
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time.UTC()
	return &result
}

func optionalText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func cloneJSON(value []byte) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

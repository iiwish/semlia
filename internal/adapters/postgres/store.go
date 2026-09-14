package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbgen "github.com/iiwish/semlia/internal/adapters/postgres/sqlc"
	"github.com/iiwish/semlia/internal/application/jobs"
	operationsdomain "github.com/iiwish/semlia/internal/domain/operations"
	"github.com/iiwish/semlia/pkg/identity"
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
	id, err := uuidValue(input.ID)
	if err != nil {
		return jobs.Job{}, fmt.Errorf("encode job ID: %w", err)
	}
	workspaceID, err := uuidValue(input.WorkspaceID)
	if err != nil {
		return jobs.Job{}, fmt.Errorf("encode workspace ID: %w", err)
	}
	row, err := store.queries.EnqueueJob(ctx, dbgen.EnqueueJobParams{
		ID:             id,
		WorkspaceID:    workspaceID,
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
	job, err := jobFromRow(row)
	if err != nil {
		return jobs.Job{}, fmt.Errorf("decode enqueued job: %w", err)
	}
	return job, nil
}

func (store *Store) ReapExpiredJobs(ctx context.Context, now time.Time) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin reap expired job leases: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	reaped, err := queries.ReapExpiredJobs(ctx, timestamp(now))
	if err != nil {
		return fmt.Errorf("reap expired job leases: %w", err)
	}
	for _, job := range reaped {
		state := operationsdomain.RunQueued
		if job.Status == "dead_letter" {
			state = operationsdomain.RunDeadLetter
		}
		if err := projectJobTerminalState(ctx, queries, job.WorkspaceID, job.ID, state, "LEASE_EXPIRED", now); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit expired job leases: %w", err)
	}
	return nil
}

func (store *Store) ClaimJob(ctx context.Context, owner string, now time.Time, leaseDuration time.Duration) (*jobs.Job, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin claim job lease: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	row, err := queries.ClaimJob(ctx, dbgen.ClaimJobParams{
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
	job, err := jobFromRow(row)
	if err != nil {
		return nil, fmt.Errorf("decode claimed job: %w", err)
	}
	if err := projectJobTerminalState(ctx, queries, row.WorkspaceID, row.ID, operationsdomain.RunRunning, "", now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claimed job lease: %w", err)
	}
	return &job, nil
}

func (store *Store) ExtendJobLease(
	ctx context.Context,
	id identity.RunID,
	owner string,
	renewedAt time.Time,
	leaseDuration time.Duration,
) error {
	if strings.TrimSpace(owner) == "" || leaseDuration <= 0 {
		return ErrLeaseLost
	}
	databaseID, err := uuidValue(id)
	if err != nil {
		return fmt.Errorf("encode job ID: %w", err)
	}
	rows, err := store.queries.ExtendJobLease(ctx, dbgen.ExtendJobLeaseParams{
		LeasedUntil: timestamp(renewedAt.Add(leaseDuration)),
		RenewedAt:   timestamp(renewedAt),
		ID:          databaseID,
		LeaseOwner:  textValue(owner),
	})
	if err != nil {
		return fmt.Errorf("extend job lease: %w", err)
	}
	return requireLease(rows)
}

func (store *Store) MarkJobSucceeded(ctx context.Context, id identity.RunID, owner string, completedAt time.Time) error {
	databaseID, err := uuidValue(id)
	if err != nil {
		return fmt.Errorf("encode job ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin job success: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	var workspaceID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM jobs WHERE id=$1 FOR UPDATE`, databaseID).Scan(&workspaceID); err != nil {
		return fmt.Errorf("load job workspace: %w", err)
	}
	rows, err := queries.MarkJobSucceeded(ctx, dbgen.MarkJobSucceededParams{
		CompletedAt: timestamp(completedAt),
		ID:          databaseID,
		LeaseOwner:  textValue(owner),
	})
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	if err := requireLease(rows); err != nil {
		return err
	}
	if err := projectJobTerminalState(ctx, queries, workspaceID, databaseID, operationsdomain.RunSucceeded, "", completedAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit job success: %w", err)
	}
	return nil
}

func (store *Store) MarkJobFailed(
	ctx context.Context,
	id identity.RunID, owner, errorCode string,
	availableAt, failedAt time.Time,
) error {
	return store.MarkJobFailedWithDisposition(ctx, id, owner, errorCode, false, availableAt, failedAt)
}

func (store *Store) MarkJobFailedWithDisposition(
	ctx context.Context,
	id identity.RunID, owner, errorCode string, permanent bool,
	availableAt, failedAt time.Time,
) error {
	databaseID, err := uuidValue(id)
	if err != nil {
		return fmt.Errorf("encode job ID: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin job failure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := dbgen.New(tx)
	var workspaceID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM jobs WHERE id=$1 FOR UPDATE`, databaseID).Scan(&workspaceID); err != nil {
		return fmt.Errorf("load job workspace: %w", err)
	}
	rows, err := queries.MarkJobFailed(ctx, dbgen.MarkJobFailedParams{
		AvailableAt: timestamp(availableAt),
		ErrorCode:   textValue(errorCode),
		FailedAt:    timestamp(failedAt),
		Permanent:   permanent,
		ID:          databaseID,
		LeaseOwner:  textValue(owner),
	})
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	if err := requireLease(rows); err != nil {
		return err
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM jobs WHERE id=$1`, databaseID).Scan(&status); err != nil {
		return fmt.Errorf("load failed job status: %w", err)
	}
	state := operationsdomain.RunQueued
	if status == "dead_letter" {
		state = operationsdomain.RunDeadLetter
	}
	if err := projectJobTerminalState(ctx, queries, workspaceID, databaseID, state, errorCode, failedAt); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit job failure: %w", err)
	}
	return nil
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
	event, err := outboxFromRow(row)
	if err != nil {
		return nil, fmt.Errorf("decode claimed outbox event: %w", err)
	}
	return &event, nil
}

func (store *Store) MarkOutboxEventPublished(ctx context.Context, id identity.EventID, owner string, publishedAt time.Time) error {
	databaseID, err := uuidValue(id)
	if err != nil {
		return fmt.Errorf("encode outbox event ID: %w", err)
	}
	rows, err := store.queries.MarkOutboxEventPublished(ctx, dbgen.MarkOutboxEventPublishedParams{
		PublishedAt: timestamp(publishedAt),
		ID:          databaseID,
		LeaseOwner:  textValue(owner),
	})
	if err != nil {
		return fmt.Errorf("publish outbox event: %w", err)
	}
	return requireLease(rows)
}

func (store *Store) MarkOutboxEventFailed(
	ctx context.Context,
	id identity.EventID, owner, errorCode string,
	availableAt, failedAt time.Time,
) error {
	databaseID, err := uuidValue(id)
	if err != nil {
		return fmt.Errorf("encode outbox event ID: %w", err)
	}
	rows, err := store.queries.MarkOutboxEventFailed(ctx, dbgen.MarkOutboxEventFailedParams{
		AvailableAt: timestamp(availableAt),
		ErrorCode:   textValue(errorCode),
		FailedAt:    timestamp(failedAt),
		ID:          databaseID,
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
	id, err := uuidValue(event.ID)
	if err != nil {
		return fmt.Errorf("encode audit event ID: %w", err)
	}
	workspaceID, err := uuidValue(event.WorkspaceID)
	if err != nil {
		return fmt.Errorf("encode workspace ID: %w", err)
	}
	actorID := pgtype.Text{}
	if event.ActorID != "" {
		actorID = textValue(event.ActorID)
	}
	if err := store.queries.CreateAuditEvent(ctx, dbgen.CreateAuditEventParams{
		ID:          id,
		WorkspaceID: workspaceID,
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
	id, err := uuidValue(event.ID)
	if err != nil {
		return fmt.Errorf("encode outbox event ID: %w", err)
	}
	workspaceID, err := uuidValue(event.WorkspaceID)
	if err != nil {
		return fmt.Errorf("encode workspace ID: %w", err)
	}
	if err := store.queries.EnqueueOutboxEvent(ctx, dbgen.EnqueueOutboxEventParams{
		ID:          id,
		WorkspaceID: workspaceID,
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

func jobFromRow(row dbgen.Job) (jobs.Job, error) {
	id, err := identity.RunIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return jobs.Job{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return jobs.Job{}, err
	}
	return jobs.Job{
		ID:             id,
		WorkspaceID:    workspaceID,
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
	}, nil
}

func outboxFromRow(row dbgen.OutboxEvent) (jobs.OutboxEvent, error) {
	id, err := identity.EventIDFromUUIDBytes(row.ID.Bytes)
	if err != nil {
		return jobs.OutboxEvent{}, err
	}
	workspaceID, err := identity.WorkspaceIDFromUUIDBytes(row.WorkspaceID.Bytes)
	if err != nil {
		return jobs.OutboxEvent{}, err
	}
	return jobs.OutboxEvent{
		ID:            id,
		WorkspaceID:   workspaceID,
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
	}, nil
}

type uuidIdentity interface {
	UUID() string
}

func uuidValue(value uuidIdentity) (pgtype.UUID, error) {
	var result pgtype.UUID
	if err := result.Scan(value.UUID()); err != nil {
		return pgtype.UUID{}, err
	}
	return result, nil
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

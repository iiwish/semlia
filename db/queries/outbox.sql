-- name: EnqueueOutboxEvent :exec
INSERT INTO outbox_events (
    id, workspace_id, event_type, payload, max_attempts, available_at, trace_id
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(event_type), sqlc.arg(payload),
    sqlc.arg(max_attempts), sqlc.arg(available_at), sqlc.arg(trace_id)
);

-- name: ReapExpiredOutboxEvents :execrows
UPDATE outbox_events
SET status = CASE WHEN attempt >= max_attempts THEN 'dead_letter' ELSE 'retryable' END,
    available_at = sqlc.arg(expired_at),
    leased_until = NULL,
    lease_owner = NULL,
    last_error_code = 'LEASE_EXPIRED',
    updated_at = sqlc.arg(expired_at)
WHERE status = 'publishing'
  AND leased_until <= sqlc.arg(expired_at);

-- name: ClaimOutboxEvent :one
WITH candidate AS (
    SELECT id
    FROM outbox_events
    WHERE status IN ('queued', 'retryable')
      AND available_at <= sqlc.arg(claimed_at)
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE outbox_events
SET status = 'publishing',
    attempt = attempt + 1,
    leased_until = sqlc.arg(leased_until),
    lease_owner = sqlc.arg(lease_owner),
    last_error_code = NULL,
    updated_at = sqlc.arg(claimed_at)
FROM candidate
WHERE outbox_events.id = candidate.id
RETURNING outbox_events.*;

-- name: MarkOutboxEventPublished :execrows
UPDATE outbox_events
SET status = 'published',
    leased_until = NULL,
    lease_owner = NULL,
    last_error_code = NULL,
    updated_at = sqlc.arg(published_at),
    published_at = sqlc.arg(published_at)
WHERE id = sqlc.arg(id)
  AND status = 'publishing'
  AND lease_owner = sqlc.arg(lease_owner);

-- name: MarkOutboxEventFailed :execrows
UPDATE outbox_events
SET status = CASE WHEN attempt >= max_attempts THEN 'dead_letter' ELSE 'retryable' END,
    available_at = sqlc.arg(available_at),
    leased_until = NULL,
    lease_owner = NULL,
    last_error_code = sqlc.arg(error_code),
    updated_at = sqlc.arg(failed_at)
WHERE id = sqlc.arg(id)
  AND status = 'publishing'
  AND lease_owner = sqlc.arg(lease_owner);

-- name: EnqueueJob :one
INSERT INTO jobs (
    id, workspace_id, job_type, payload, max_attempts,
    available_at, idempotency_key, trace_id
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(job_type), sqlc.arg(payload),
    sqlc.arg(max_attempts), sqlc.arg(available_at), sqlc.arg(idempotency_key), sqlc.arg(trace_id)
)
ON CONFLICT (workspace_id, idempotency_key) DO UPDATE
SET idempotency_key = EXCLUDED.idempotency_key
RETURNING *;

-- name: GetJobByID :one
SELECT * FROM jobs WHERE id = sqlc.arg(id);

-- name: ReapExpiredJobs :many
UPDATE jobs
SET status = CASE WHEN attempt >= max_attempts THEN 'dead_letter' ELSE 'retryable' END,
    available_at = sqlc.arg(expired_at),
    leased_until = NULL,
    lease_owner = NULL,
    last_error_code = 'LEASE_EXPIRED',
    updated_at = sqlc.arg(expired_at),
    completed_at = CASE WHEN attempt >= max_attempts THEN sqlc.arg(expired_at) ELSE NULL END
WHERE status = 'running'
  AND leased_until <= sqlc.arg(expired_at)
RETURNING *;

-- name: ClaimJob :one
WITH candidate AS (
    SELECT id
    FROM jobs
    WHERE status IN ('queued', 'retryable')
      AND available_at <= sqlc.arg(claimed_at)
    ORDER BY available_at, created_at, id
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
UPDATE jobs
SET status = 'running',
    attempt = attempt + 1,
    leased_until = sqlc.arg(leased_until),
    lease_owner = sqlc.arg(lease_owner),
    last_error_code = NULL,
    updated_at = sqlc.arg(claimed_at)
FROM candidate
WHERE jobs.id = candidate.id
RETURNING jobs.*;

-- name: ExtendJobLease :execrows
UPDATE jobs
SET leased_until = sqlc.arg(leased_until),
    updated_at = sqlc.arg(renewed_at)
WHERE id = sqlc.arg(id)
  AND status = 'running'
  AND lease_owner = sqlc.arg(lease_owner)
  AND leased_until > sqlc.arg(renewed_at);

-- name: MarkJobSucceeded :execrows
UPDATE jobs
SET status = 'succeeded',
    leased_until = NULL,
    lease_owner = NULL,
    last_error_code = NULL,
    updated_at = sqlc.arg(completed_at),
    completed_at = sqlc.arg(completed_at)
WHERE id = sqlc.arg(id)
  AND status = 'running'
  AND lease_owner = sqlc.arg(lease_owner)
  AND leased_until > sqlc.arg(completed_at);

-- name: MarkJobFailed :execrows
UPDATE jobs
SET status = CASE WHEN sqlc.arg(permanent)::boolean OR attempt >= max_attempts THEN 'dead_letter' ELSE 'retryable' END,
    available_at = sqlc.arg(available_at),
    leased_until = NULL,
    lease_owner = NULL,
    last_error_code = sqlc.arg(error_code),
    updated_at = sqlc.arg(failed_at),
    completed_at = CASE
        WHEN sqlc.arg(permanent)::boolean OR attempt >= max_attempts THEN sqlc.arg(failed_at)::timestamptz
        ELSE NULL::timestamptz
    END
WHERE id = sqlc.arg(id)
  AND status = 'running'
  AND lease_owner = sqlc.arg(lease_owner)
  AND leased_until > sqlc.arg(failed_at);

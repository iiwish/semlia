-- name: UpsertRuntimeRun :one
INSERT INTO runtime_runs (
    id, workspace_id, kind, source_type, source_id, source_version_digest, job_id,
    trace_id, idempotency_key, requested_by_principal_id, state, phase,
    progress_current, progress_total, attempt, max_attempts, started_at, finished_at,
    error_code, error_summary, version, created_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(kind), sqlc.arg(source_type),
    sqlc.arg(source_id), sqlc.arg(source_version_digest), sqlc.narg(job_id),
    sqlc.narg(trace_id), sqlc.arg(idempotency_key), sqlc.narg(requested_by_principal_id),
    sqlc.arg(state), sqlc.narg(phase), sqlc.narg(progress_current), sqlc.narg(progress_total),
    sqlc.arg(attempt), sqlc.arg(max_attempts), sqlc.narg(started_at), sqlc.narg(finished_at),
    sqlc.narg(error_code), sqlc.narg(error_summary), sqlc.arg(version),
    sqlc.arg(created_at), sqlc.arg(updated_at)
)
ON CONFLICT (workspace_id, kind, source_type, source_id) DO UPDATE
SET source_version_digest = EXCLUDED.source_version_digest,
    job_id = COALESCE(EXCLUDED.job_id, runtime_runs.job_id),
    trace_id = COALESCE(EXCLUDED.trace_id, runtime_runs.trace_id),
    requested_by_principal_id = COALESCE(EXCLUDED.requested_by_principal_id, runtime_runs.requested_by_principal_id),
    state = EXCLUDED.state,
    phase = EXCLUDED.phase,
    progress_current = EXCLUDED.progress_current,
    progress_total = EXCLUDED.progress_total,
    attempt = GREATEST(runtime_runs.attempt, EXCLUDED.attempt),
    max_attempts = GREATEST(runtime_runs.max_attempts, EXCLUDED.max_attempts),
    started_at = COALESCE(runtime_runs.started_at, EXCLUDED.started_at),
    finished_at = EXCLUDED.finished_at,
    error_code = EXCLUDED.error_code,
    error_summary = EXCLUDED.error_summary,
    version = CASE WHEN ROW(
        runtime_runs.source_version_digest, runtime_runs.job_id, runtime_runs.trace_id,
        runtime_runs.requested_by_principal_id, runtime_runs.state, runtime_runs.phase,
        runtime_runs.progress_current, runtime_runs.progress_total, runtime_runs.attempt,
        runtime_runs.max_attempts, runtime_runs.started_at, runtime_runs.finished_at,
        runtime_runs.error_code, runtime_runs.error_summary
    ) IS NOT DISTINCT FROM ROW(
        EXCLUDED.source_version_digest, EXCLUDED.job_id, EXCLUDED.trace_id,
        EXCLUDED.requested_by_principal_id, EXCLUDED.state, EXCLUDED.phase,
        EXCLUDED.progress_current, EXCLUDED.progress_total, EXCLUDED.attempt,
        EXCLUDED.max_attempts, EXCLUDED.started_at, EXCLUDED.finished_at,
        EXCLUDED.error_code, EXCLUDED.error_summary
    ) THEN runtime_runs.version ELSE runtime_runs.version + 1 END,
    updated_at = CASE WHEN ROW(
        runtime_runs.source_version_digest, runtime_runs.job_id, runtime_runs.trace_id,
        runtime_runs.requested_by_principal_id, runtime_runs.state, runtime_runs.phase,
        runtime_runs.progress_current, runtime_runs.progress_total, runtime_runs.attempt,
        runtime_runs.max_attempts, runtime_runs.started_at, runtime_runs.finished_at,
        runtime_runs.error_code, runtime_runs.error_summary
    ) IS NOT DISTINCT FROM ROW(
        EXCLUDED.source_version_digest, EXCLUDED.job_id, EXCLUDED.trace_id,
        EXCLUDED.requested_by_principal_id, EXCLUDED.state, EXCLUDED.phase,
        EXCLUDED.progress_current, EXCLUDED.progress_total, EXCLUDED.attempt,
        EXCLUDED.max_attempts, EXCLUDED.started_at, EXCLUDED.finished_at,
        EXCLUDED.error_code, EXCLUDED.error_summary
    ) THEN runtime_runs.updated_at ELSE EXCLUDED.updated_at END
RETURNING *;

-- name: AppendRuntimeRunEvent :exec
WITH locked_run AS (
    SELECT id FROM runtime_runs
    WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(runtime_run_id)
    FOR UPDATE
), next_sequence AS (
    SELECT COALESCE(MAX(sequence), 0) + 1 AS value
    FROM runtime_run_events
    WHERE workspace_id = sqlc.arg(workspace_id) AND runtime_run_id = sqlc.arg(runtime_run_id)
)
INSERT INTO runtime_run_events (
    id, workspace_id, runtime_run_id, event_key, sequence, event_type, phase,
    progress_current, progress_total, state, error_code, summary, metadata, created_at
)
SELECT sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(runtime_run_id), sqlc.arg(event_key),
    next_sequence.value, sqlc.arg(event_type), sqlc.narg(phase), sqlc.narg(progress_current),
    sqlc.narg(progress_total), sqlc.narg(state), sqlc.narg(error_code), sqlc.narg(summary),
    sqlc.arg(metadata), sqlc.arg(created_at)
FROM locked_run, next_sequence
ON CONFLICT (workspace_id, runtime_run_id, event_key) DO NOTHING;

-- name: ListOperationsRuntimeRuns :many
SELECT * FROM runtime_runs
WHERE workspace_id = sqlc.arg(workspace_id)
  AND (NOT sqlc.arg(has_kind)::boolean OR kind = sqlc.arg(kind))
  AND (NOT sqlc.arg(has_state)::boolean OR state = sqlc.arg(state))
  AND (NOT sqlc.arg(has_source_type)::boolean OR source_type = sqlc.arg(source_type))
  AND (NOT sqlc.arg(has_source_id)::boolean OR source_id = sqlc.arg(source_id))
  AND (NOT sqlc.arg(has_trace)::boolean OR trace_id = sqlc.arg(trace_id))
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR updated_at < sqlc.arg(cursor_time)
      OR (updated_at = sqlc.arg(cursor_time) AND id < sqlc.arg(cursor_id)::uuid)
  )
ORDER BY updated_at DESC, id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetOperationsRuntimeRun :one
SELECT * FROM runtime_runs
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(id);

-- name: GetOperationsRuntimeRunBySource :one
SELECT * FROM runtime_runs
WHERE workspace_id = sqlc.arg(workspace_id) AND kind = sqlc.arg(kind)
  AND source_type = sqlc.arg(source_type) AND source_id = sqlc.arg(source_id)
FOR UPDATE;

-- name: GetOperationsRuntimeRunByJob :one
SELECT * FROM runtime_runs
WHERE workspace_id = sqlc.arg(workspace_id) AND job_id = sqlc.arg(job_id)
FOR UPDATE;

-- name: ListOperationsRuntimeRunEvents :many
SELECT * FROM runtime_run_events
WHERE workspace_id = sqlc.arg(workspace_id) AND runtime_run_id = sqlc.arg(runtime_run_id)
ORDER BY sequence
LIMIT sqlc.arg(page_limit);

-- name: EnsureRuntimeSettings :one
INSERT INTO runtime_settings (workspace_id, created_at, updated_at)
VALUES (sqlc.arg(workspace_id), sqlc.arg(now), sqlc.arg(now))
ON CONFLICT (workspace_id) DO UPDATE SET workspace_id = EXCLUDED.workspace_id
RETURNING *;

-- name: UpdateRuntimeSettings :one
UPDATE runtime_settings
SET retry_ceiling = sqlc.arg(retry_ceiling),
    statement_timeout_ms = sqlc.arg(statement_timeout_ms),
    webhook_timeout_ms = sqlc.arg(webhook_timeout_ms),
    query_row_limit = sqlc.arg(query_row_limit),
    query_byte_limit = sqlc.arg(query_byte_limit),
    run_metadata_retention_days = sqlc.arg(run_metadata_retention_days),
    version = version + 1,
    updated_at = sqlc.arg(updated_at)
WHERE workspace_id = sqlc.arg(workspace_id) AND version = sqlc.arg(expected_version)
RETURNING *;

-- name: GetAuditExportByIdempotency :one
SELECT * FROM audit_exports
WHERE workspace_id = sqlc.arg(workspace_id) AND idempotency_key = sqlc.arg(idempotency_key);

-- name: GetAuditExportContent :one
SELECT id, content, content_digest, format, expires_at
FROM audit_exports
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(id)
  AND expires_at > sqlc.arg(now);

-- name: CreateAuditExport :one
INSERT INTO audit_exports (
    id, workspace_id, runtime_run_id, artifact_id, format, filters, row_count,
    content_digest, request_fingerprint, idempotency_key, created_by_principal_id, content, created_at, expires_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(runtime_run_id), sqlc.arg(artifact_id),
    sqlc.arg(format), sqlc.arg(filters), sqlc.arg(row_count), sqlc.arg(content_digest),
    sqlc.arg(request_fingerprint), sqlc.arg(idempotency_key), sqlc.arg(created_by_principal_id),
    sqlc.arg(content), sqlc.arg(created_at), sqlc.arg(expires_at)
)
RETURNING *;

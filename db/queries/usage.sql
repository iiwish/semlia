-- name: CreateUsageEvent :execrows
INSERT INTO usage_events (
    id, workspace_id, event_type, idempotency_key, data_version, actor_id,
    asset_id, revision_id, channel, outcome, reason_code, trace_id,
    search_fingerprint, search_language, token_bucket, result_bucket,
    asset_type_filter, lifecycle_filter, occurred_at, received_at, expires_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(event_type), sqlc.arg(idempotency_key),
    sqlc.arg(data_version), sqlc.narg(actor_id), sqlc.narg(asset_id), sqlc.narg(revision_id),
    sqlc.arg(channel), sqlc.arg(outcome), sqlc.narg(reason_code), sqlc.arg(trace_id),
    sqlc.narg(search_fingerprint), sqlc.narg(search_language), sqlc.narg(token_bucket),
    sqlc.narg(result_bucket), sqlc.narg(asset_type_filter), sqlc.narg(lifecycle_filter),
    sqlc.arg(occurred_at), sqlc.arg(received_at), sqlc.arg(expires_at)
)
ON CONFLICT (workspace_id, event_type, idempotency_key) DO NOTHING;

-- name: DeleteExpiredUsageEvents :execrows
WITH expired AS (
    SELECT id
    FROM usage_events
    WHERE usage_events.expires_at <= sqlc.arg(expired_at)
    ORDER BY usage_events.expires_at, usage_events.id
    LIMIT sqlc.arg(delete_limit)
    FOR UPDATE SKIP LOCKED
)
DELETE FROM usage_events
USING expired
WHERE usage_events.id = expired.id;

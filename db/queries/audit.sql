-- name: CreateAuditEvent :exec
INSERT INTO audit_events (
    id, workspace_id, event_type, actor_id, payload, trace_id, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(event_type),
    sqlc.narg(actor_id), sqlc.arg(payload), sqlc.arg(trace_id), sqlc.arg(created_at)
);

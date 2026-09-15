-- name: CreateAuditEvent :exec
INSERT INTO audit_events (
    id, workspace_id, event_type, actor_id, payload, trace_id, created_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(event_type),
    sqlc.narg(actor_id), sqlc.arg(payload), sqlc.arg(trace_id), sqlc.arg(created_at)
);

-- name: ListOperationsAuditEvents :many
SELECT event.id, event.workspace_id, event.event_type, event.actor_id, event.payload,
    event.trace_id, event.created_at, COALESCE(primary_target.object_type, '') AS object_type,
    COALESCE(primary_target.object_id, '') AS object_id
FROM audit_events AS event
LEFT JOIN LATERAL (
    SELECT target.object_type, target.object_id
    FROM audit_event_targets AS target
    WHERE target.workspace_id = event.workspace_id AND target.audit_event_id = event.id
      AND (NOT sqlc.arg(has_object_type)::boolean OR target.object_type = sqlc.arg(object_type)::text)
      AND (NOT sqlc.arg(has_object_id)::boolean OR target.object_id = sqlc.arg(object_id)::text)
    ORDER BY target.ordinal, target.object_type, target.object_id
    LIMIT 1
) AS primary_target ON true
WHERE event.workspace_id = sqlc.arg(workspace_id)
  AND (NOT sqlc.arg(has_actor)::boolean OR event.actor_id = sqlc.arg(actor_id))
  AND (NOT sqlc.arg(has_event_type)::boolean OR event.event_type = sqlc.arg(event_type))
  AND (
      NOT (sqlc.arg(has_object_type)::boolean OR sqlc.arg(has_object_id)::boolean)
      OR EXISTS (
          SELECT 1 FROM audit_event_targets AS target
          WHERE target.workspace_id = event.workspace_id AND target.audit_event_id = event.id
            AND (NOT sqlc.arg(has_object_type)::boolean OR target.object_type = sqlc.arg(object_type)::text)
            AND (NOT sqlc.arg(has_object_id)::boolean OR target.object_id = sqlc.arg(object_id)::text)
      )
  )
  AND (NOT sqlc.arg(has_trace)::boolean OR event.trace_id = sqlc.arg(trace_id))
  AND (NOT sqlc.arg(has_from)::boolean OR event.created_at >= sqlc.arg(from_time))
  AND (NOT sqlc.arg(has_to)::boolean OR event.created_at <= sqlc.arg(to_time))
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR event.created_at < sqlc.arg(cursor_time)
      OR (event.created_at = sqlc.arg(cursor_time) AND event.id < sqlc.arg(cursor_id)::uuid)
  )
ORDER BY event.created_at DESC, event.id DESC
LIMIT sqlc.arg(page_limit);

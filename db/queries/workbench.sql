-- name: ListAttentionItems :many
SELECT item.id, item.workspace_id, item.kind, item.dedupe_key, item.state, item.priority,
       item.risk, item.target_type, item.target_id, item.target_route, item.assignee_principal_id,
       item.audience_role_id, item.initiator_principal_id, item.rule_version, item.title,
       item.summary, item.reason_code, item.evidence_ref, item.trace_id, item.opened_at,
       item.due_at, item.resolved_at, item.updated_at, item.version
FROM attention_items AS item
WHERE item.workspace_id = sqlc.arg(workspace_id)
  AND item.kind = ANY(sqlc.arg(allowed_kinds)::text[])
  AND (sqlc.arg(kind)::text = '' OR item.kind = sqlc.arg(kind)::text)
  AND (sqlc.arg(state)::text = '' OR item.state = sqlc.arg(state)::text)
  AND (sqlc.arg(priority)::text = '' OR item.priority = sqlc.arg(priority)::text)
  AND (sqlc.arg(risk)::text = '' OR item.risk = sqlc.arg(risk)::text)
  AND (sqlc.arg(search)::text = '' OR item.title ILIKE '%' || sqlc.arg(search)::text || '%'
       OR item.summary ILIKE '%' || sqlc.arg(search)::text || '%'
       OR item.target_id ILIKE '%' || sqlc.arg(search)::text || '%')
  AND EXISTS (
      SELECT 1
      FROM jsonb_to_recordset(sqlc.arg(access_grants)::jsonb)
          AS access("action" text, "scopeType" text, "scopeId" text)
      WHERE access."action" = CASE item.kind
              WHEN 'review' THEN 'asset.read'
              WHEN 'validation' THEN 'asset.read'
              WHEN 'source' THEN 'source.read'
              WHEN 'runtime' THEN 'runtime.read'
              WHEN 'compatibility' THEN 'binding.read'
          END
		AND (access."scopeType" = '*'
		     OR (access."scopeType" = item.target_type AND access."scopeId" = item.target_id))
  )
  AND (
      (sqlc.arg(view)::text = 'initiated' AND item.initiator_principal_id = sqlc.arg(principal_id))
      OR (sqlc.arg(view)::text = 'mine' AND (
          item.assignee_principal_id = sqlc.arg(principal_id)
          OR (item.assignee_principal_id IS NULL AND item.audience_role_id IS NOT NULL AND EXISTS (
              SELECT 1
              FROM jsonb_to_recordset(sqlc.arg(access_grants)::jsonb)
                  AS audience("roleId" text, "scopeType" text, "scopeId" text)
              WHERE audience."roleId" = item.audience_role_id
				AND (audience."scopeType" = '*'
                     OR (audience."scopeType" = item.target_type AND audience."scopeId" = item.target_id))
          ))
      ))
      OR (sqlc.arg(view)::text = 'team' AND item.assignee_principal_id IS NULL AND (
          item.audience_role_id IS NULL OR EXISTS (
              SELECT 1
              FROM jsonb_to_recordset(sqlc.arg(access_grants)::jsonb)
                  AS audience("roleId" text, "scopeType" text, "scopeId" text)
              WHERE audience."roleId" = item.audience_role_id
				AND (audience."scopeType" = '*'
                     OR (audience."scopeType" = item.target_type AND audience."scopeId" = item.target_id))
          )
      ))
  )
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR (sqlc.arg(sort)::text = 'updated_desc' AND (item.updated_at, item.id) < (sqlc.arg(cursor_updated_at), sqlc.arg(cursor_id)::uuid))
      OR (sqlc.arg(sort)::text = 'priority_desc' AND (
          CASE item.priority WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 ELSE 1 END < sqlc.arg(cursor_priority_rank)::integer
          OR (CASE item.priority WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 ELSE 1 END = sqlc.arg(cursor_priority_rank)::integer
              AND (item.updated_at, item.id) < (sqlc.arg(cursor_updated_at), sqlc.arg(cursor_id)::uuid))
      ))
      OR (sqlc.arg(sort)::text = 'due_asc' AND (
          COALESCE(item.due_at, 'infinity'::timestamptz) > COALESCE(sqlc.narg(cursor_due_at)::timestamptz, 'infinity'::timestamptz)
          OR (COALESCE(item.due_at, 'infinity'::timestamptz) = COALESCE(sqlc.narg(cursor_due_at)::timestamptz, 'infinity'::timestamptz)
              AND (item.updated_at, item.id) < (sqlc.arg(cursor_updated_at), sqlc.arg(cursor_id)::uuid))
      ))
  )
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'priority_desc' THEN
      CASE item.priority WHEN 'critical' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 ELSE 1 END
  END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'due_asc' THEN COALESCE(item.due_at, 'infinity'::timestamptz) END ASC,
  item.updated_at DESC,
  item.id DESC
LIMIT sqlc.arg(page_limit);

-- name: CountAttentionItems :one
SELECT count(*)::integer AS total,
       count(*) FILTER (WHERE item.state = 'open')::integer AS open,
       count(*) FILTER (WHERE item.state = 'in_progress')::integer AS in_progress,
       count(*) FILTER (WHERE item.priority = 'critical' AND item.state IN ('open', 'in_progress'))::integer AS critical
FROM attention_items AS item
WHERE item.workspace_id = sqlc.arg(workspace_id)
  AND item.kind = ANY(sqlc.arg(allowed_kinds)::text[])
  AND (sqlc.arg(kind)::text = '' OR item.kind = sqlc.arg(kind)::text)
  AND (sqlc.arg(state)::text = '' OR item.state = sqlc.arg(state)::text)
  AND (sqlc.arg(priority)::text = '' OR item.priority = sqlc.arg(priority)::text)
  AND (sqlc.arg(risk)::text = '' OR item.risk = sqlc.arg(risk)::text)
  AND (sqlc.arg(search)::text = '' OR item.title ILIKE '%' || sqlc.arg(search)::text || '%'
       OR item.summary ILIKE '%' || sqlc.arg(search)::text || '%'
       OR item.target_id ILIKE '%' || sqlc.arg(search)::text || '%')
  AND EXISTS (
      SELECT 1
      FROM jsonb_to_recordset(sqlc.arg(access_grants)::jsonb)
          AS access("action" text, "scopeType" text, "scopeId" text)
      WHERE access."action" = CASE item.kind
              WHEN 'review' THEN 'asset.read'
              WHEN 'validation' THEN 'asset.read'
              WHEN 'source' THEN 'source.read'
              WHEN 'runtime' THEN 'runtime.read'
              WHEN 'compatibility' THEN 'binding.read'
          END
		AND (access."scopeType" = '*'
             OR (access."scopeType" = item.target_type AND access."scopeId" = item.target_id))
  )
  AND (
      (sqlc.arg(view)::text = 'initiated' AND item.initiator_principal_id = sqlc.arg(principal_id))
      OR (sqlc.arg(view)::text = 'mine' AND (
          item.assignee_principal_id = sqlc.arg(principal_id)
          OR (item.assignee_principal_id IS NULL AND item.audience_role_id IS NOT NULL AND EXISTS (
              SELECT 1
              FROM jsonb_to_recordset(sqlc.arg(access_grants)::jsonb)
                  AS audience("roleId" text, "scopeType" text, "scopeId" text)
              WHERE audience."roleId" = item.audience_role_id
				AND (audience."scopeType" = '*'
                     OR (audience."scopeType" = item.target_type AND audience."scopeId" = item.target_id))
          ))
      ))
      OR (sqlc.arg(view)::text = 'team' AND item.assignee_principal_id IS NULL AND (
          item.audience_role_id IS NULL OR EXISTS (
              SELECT 1
              FROM jsonb_to_recordset(sqlc.arg(access_grants)::jsonb)
                  AS audience("roleId" text, "scopeType" text, "scopeId" text)
              WHERE audience."roleId" = item.audience_role_id
				AND (audience."scopeType" = '*'
                     OR (audience."scopeType" = item.target_type AND audience."scopeId" = item.target_id))
          )
      ))
  );

-- name: GetWorkbenchMutationAudit :one
SELECT payload
FROM audit_events
WHERE workspace_id = sqlc.arg(workspace_id)
  AND event_type = 'workbench.attention_item.updated'
  AND payload->>'idempotencyKey' = CAST(sqlc.arg(idempotency_key) AS text)
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetLatestAuditTraceForObject :one
SELECT event.trace_id
FROM audit_event_targets AS target
JOIN audit_events AS event
  ON event.workspace_id = target.workspace_id AND event.id = target.audit_event_id
WHERE target.workspace_id = sqlc.arg(workspace_id)
  AND target.object_id = CAST(sqlc.arg(object_id) AS text)
ORDER BY event.created_at DESC, event.id DESC
LIMIT 1;

-- name: GetAttentionItem :one
SELECT item.*
FROM attention_items AS item
WHERE item.workspace_id = sqlc.arg(workspace_id) AND item.id = sqlc.arg(item_id);

-- name: ListReviewedAttentionItemIDs :many
SELECT DISTINCT item.id
FROM attention_items AS item
JOIN proposals AS proposal
  ON proposal.workspace_id = item.workspace_id
 AND item.dedupe_key = 'review:' || proposal.id::text
JOIN reviews AS review
  ON review.workspace_id = proposal.workspace_id
 AND review.proposal_id = proposal.id
WHERE item.workspace_id = sqlc.arg(workspace_id)
  AND item.id = ANY(sqlc.arg(item_ids)::uuid[])
  AND review.reviewer_principal_id = sqlc.arg(principal_id);

-- name: LockAttentionProjection :exec
SELECT pg_advisory_xact_lock(hashtext('semlia.attention-projection')::bigint);

-- name: GetPhysicalBindingAttentionAsset :one
SELECT asset_id FROM physical_bindings
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(object_id);

-- name: GetModelGrainAttentionAsset :one
SELECT asset_id FROM model_grains
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(object_id);

-- name: GetEntityKeyAttentionAsset :one
SELECT asset_id FROM entity_keys
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(object_id);

-- name: LockAttentionItem :one
SELECT * FROM attention_items
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(item_id)
FOR UPDATE;

-- name: UpdateAttentionItem :one
UPDATE attention_items
SET assignee_principal_id = CASE WHEN sqlc.arg(set_assignee)::boolean THEN sqlc.narg(assignee_principal_id) ELSE assignee_principal_id END,
    state = CASE WHEN sqlc.arg(state)::text = '' THEN state ELSE sqlc.arg(state)::text END,
    updated_at = sqlc.arg(updated_at),
    version = version + 1
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(item_id)
  AND version = sqlc.arg(expected_version)
  AND state <> 'resolved'
RETURNING *;

-- name: UpsertAttentionItem :one
INSERT INTO attention_items (
    id, workspace_id, kind, dedupe_key, state, priority, risk, target_type, target_id,
    target_route, assignee_principal_id, audience_role_id, initiator_principal_id,
    rule_version, title, summary, reason_code, evidence_ref, trace_id, opened_at,
    due_at, resolved_at, updated_at, version
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(kind), sqlc.arg(dedupe_key), 'open',
    sqlc.arg(priority), sqlc.arg(risk), sqlc.arg(target_type), sqlc.arg(target_id),
    sqlc.arg(target_route), sqlc.narg(assignee_principal_id), sqlc.narg(audience_role_id),
    sqlc.narg(initiator_principal_id), sqlc.arg(rule_version), sqlc.arg(title),
    sqlc.arg(summary), sqlc.arg(reason_code), sqlc.narg(evidence_ref), sqlc.arg(trace_id),
    sqlc.arg(opened_at), sqlc.narg(due_at), NULL, sqlc.arg(updated_at), 1
)
ON CONFLICT (workspace_id, dedupe_key) DO UPDATE
SET priority = EXCLUDED.priority,
    risk = EXCLUDED.risk,
    target_type = EXCLUDED.target_type,
    target_id = EXCLUDED.target_id,
    target_route = EXCLUDED.target_route,
    audience_role_id = EXCLUDED.audience_role_id,
    initiator_principal_id = EXCLUDED.initiator_principal_id,
    rule_version = EXCLUDED.rule_version,
    title = EXCLUDED.title,
    summary = EXCLUDED.summary,
    reason_code = EXCLUDED.reason_code,
    evidence_ref = EXCLUDED.evidence_ref,
    trace_id = EXCLUDED.trace_id,
    opened_at = CASE WHEN attention_items.state = 'resolved' THEN EXCLUDED.opened_at ELSE attention_items.opened_at END,
    due_at = EXCLUDED.due_at,
    state = CASE WHEN attention_items.state = 'resolved' THEN 'open' ELSE attention_items.state END,
    resolved_at = NULL,
    updated_at = EXCLUDED.updated_at,
    version = CASE WHEN attention_items.state = 'resolved'
                   OR attention_items.priority IS DISTINCT FROM EXCLUDED.priority
                   OR attention_items.risk IS DISTINCT FROM EXCLUDED.risk
                   OR attention_items.target_type IS DISTINCT FROM EXCLUDED.target_type
                   OR attention_items.target_id IS DISTINCT FROM EXCLUDED.target_id
                   OR attention_items.target_route IS DISTINCT FROM EXCLUDED.target_route
                   OR attention_items.audience_role_id IS DISTINCT FROM EXCLUDED.audience_role_id
                   OR attention_items.initiator_principal_id IS DISTINCT FROM EXCLUDED.initiator_principal_id
                   OR attention_items.rule_version IS DISTINCT FROM EXCLUDED.rule_version
                   OR attention_items.title IS DISTINCT FROM EXCLUDED.title
                   OR attention_items.summary IS DISTINCT FROM EXCLUDED.summary
                   OR attention_items.reason_code IS DISTINCT FROM EXCLUDED.reason_code
                   OR attention_items.evidence_ref IS DISTINCT FROM EXCLUDED.evidence_ref
                   OR attention_items.trace_id IS DISTINCT FROM EXCLUDED.trace_id
                   OR attention_items.due_at IS DISTINCT FROM EXCLUDED.due_at
                   OR attention_items.updated_at IS DISTINCT FROM EXCLUDED.updated_at
                   THEN attention_items.version + 1 ELSE attention_items.version END
RETURNING *;

-- name: ResolveAttentionItemByDedupe :execrows
UPDATE attention_items
SET state = 'resolved', resolved_at = sqlc.arg(resolved_at), updated_at = sqlc.arg(resolved_at), version = version + 1
WHERE workspace_id = sqlc.arg(workspace_id) AND dedupe_key = sqlc.arg(dedupe_key)
  AND state IN ('open', 'in_progress');

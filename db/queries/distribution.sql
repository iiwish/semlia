-- name: CreateDistributionConsumer :one
INSERT INTO consumers (
    id, workspace_id, stable_key, name, kind, status, owner_principal_ref, metadata, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
RETURNING *;

-- name: GetDistributionConsumer :one
SELECT * FROM consumers WHERE workspace_id = $1 AND id = $2;

-- name: ListDistributionConsumers :many
SELECT * FROM consumers WHERE workspace_id = $1 ORDER BY updated_at DESC, id;

-- name: UpdateDistributionConsumer :one
UPDATE consumers
SET name = $3, status = $4, metadata = $5, updated_at = $6
WHERE workspace_id = $1 AND id = $2
RETURNING *;

-- name: CreateDistributionBinding :one
INSERT INTO consumer_bindings (
    id, workspace_id, consumer_id, environment, purpose, mode, release_id,
    compatibility_constraint, expires_at, status, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12)
RETURNING *;

-- name: GetDistributionBinding :one
SELECT * FROM consumer_bindings WHERE workspace_id = $1 AND id = $2;

-- name: ListDistributionBindings :many
SELECT * FROM consumer_bindings WHERE workspace_id = $1 ORDER BY updated_at DESC, id;

-- name: UpdateDistributionBinding :one
UPDATE consumer_bindings
SET purpose = $3, mode = $4, release_id = $5, compatibility_constraint = $6,
    expires_at = $7, status = $8, version = version + 1, updated_at = $9
WHERE workspace_id = $1 AND id = $2 AND version = $10
RETURNING *;

-- name: GetDistributionRelease :one
SELECT id, workspace_id, sequence, manifest_digest, published_at
FROM releases WHERE workspace_id = $1 AND id = $2;

-- name: GetCurrentDistributionRelease :one
SELECT id, workspace_id, sequence, manifest_digest, published_at
FROM releases WHERE workspace_id = $1
  AND state = 'published'
ORDER BY sequence DESC LIMIT 1;

-- name: ListDistributionReleaseAssets :many
SELECT asset.id AS asset_id, revision.id AS revision_id, asset.namespace, asset.key,
       asset.asset_type, revision.content, revision.content_digest, entry.position
FROM release_assets entry
JOIN semantic_assets asset
  ON asset.workspace_id = entry.workspace_id AND asset.id = entry.asset_id
JOIN asset_revisions revision
  ON revision.workspace_id = entry.workspace_id AND revision.id = entry.revision_id
 AND revision.asset_id = entry.asset_id
WHERE entry.workspace_id = $1 AND entry.release_id = $2
ORDER BY entry.position;

-- name: ListDistributionReleaseObjectSnapshots :many
SELECT object_type, object_id, version, payload, position
FROM release_object_snapshots
WHERE workspace_id = $1 AND release_id = $2
ORDER BY position;

-- name: CreateSemanticQuery :exec
INSERT INTO semantic_queries (
    id, workspace_id, principal_ref, consumer_id, binding_id, selected_release_id,
    schema_version, resolver_version, canonical_request, request_digest, channel,
    trace_id, idempotency_key, outcome, created_at, finalized_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
);

-- name: CreateResolvedSemanticPlan :exec
INSERT INTO resolved_semantic_plans (
    id, workspace_id, query_id, release_id, resolver_version, canonical_plan,
    plan_digest, execution_status, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: CreateSemanticRefusal :exec
INSERT INTO semantic_refusals (
    workspace_id, query_id, reason_code, authorized_candidate_ids, clarification, details, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: CreateQueryValidationRun :exec
INSERT INTO query_validation_runs (
    id, workspace_id, query_id, plan_id, validator, validator_version,
    input_digest, status, created_at, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: CreateQueryValidationResult :exec
INSERT INTO query_validation_results (
    validation_run_id, position, severity, code, message, details
) VALUES ($1, $2, $3, $4, $5, $6);

-- name: CreateSemanticResolutionEvent :exec
INSERT INTO semantic_resolution_events (
    id, workspace_id, query_id, principal_ref, consumer_id, binding_id, release_id,
    channel, outcome, reason_code, trace_id, occurred_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: GetSemanticQueryRecord :one
SELECT * FROM semantic_queries WHERE workspace_id = $1 AND id = $2;

-- name: GetSemanticQueryByIdempotency :one
SELECT * FROM semantic_queries
WHERE workspace_id = $1 AND principal_ref = $2 AND idempotency_key = $3;

-- name: GetResolvedSemanticPlanRecord :one
SELECT * FROM resolved_semantic_plans WHERE workspace_id = $1 AND id = $2;

-- name: GetResolvedSemanticPlanByQuery :one
SELECT * FROM resolved_semantic_plans WHERE workspace_id = $1 AND query_id = $2;

-- name: GetSemanticRefusalByQuery :one
SELECT * FROM semantic_refusals WHERE workspace_id = $1 AND query_id = $2;

-- name: GetQueryValidationRunByQuery :one
SELECT * FROM query_validation_runs WHERE workspace_id = $1 AND query_id = $2;

-- name: ListQueryValidationResults :many
SELECT * FROM query_validation_results WHERE validation_run_id = $1 ORDER BY position;

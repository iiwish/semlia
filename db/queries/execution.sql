-- name: ListReleaseExecutionRelations :many
SELECT payload FROM release_execution_relations WHERE workspace_id=$1 AND release_id=$2 ORDER BY dataset_id;

-- name: ListReleaseExecutionBindingPins :many
SELECT binding_id,version,dataset_id,COALESCE(payload->>'datasetRevisionId','')::text AS dataset_revision_id
FROM release_execution_binding_pins WHERE workspace_id=$1 AND release_id=$2 ORDER BY binding_id;

-- name: GetExecutionRun :one
SELECT * FROM query_execution_runs WHERE workspace_id=$1 AND id=$2;

-- name: GetExecutionRunByKey :one
SELECT * FROM query_execution_runs WHERE workspace_id=$1 AND principal_id=$2 AND idempotency_key=$3;

-- name: LatestPublishedAssetPins :many
SELECT DISTINCT ON (a.asset_id) a.* FROM release_assets a
JOIN releases r ON r.workspace_id=a.workspace_id AND r.id=a.release_id
WHERE a.workspace_id=$1 AND r.state='published'
ORDER BY a.asset_id,r.sequence DESC;

-- name: LatestPublishedObjectPins :many
SELECT DISTINCT ON (a.object_type,a.object_id) a.* FROM release_objects a
JOIN releases r ON r.workspace_id=a.workspace_id AND r.id=a.release_id
WHERE a.workspace_id=$1 AND r.state='published'
ORDER BY a.object_type,a.object_id,r.sequence DESC;

-- name: GetAssetProjection :one
SELECT asset.namespace,
       asset.key,
       asset.asset_type,
       asset.lifecycle_state,
       revision.id AS revision_id,
       revision.asset_id,
       revision.sequence,
       revision.schema_version,
       revision.content_digest,
       revision.content,
       revision.created_by,
       revision.created_at
FROM semantic_assets AS asset
JOIN asset_revisions AS revision
  ON revision.workspace_id = asset.workspace_id
 AND revision.asset_id = asset.id
WHERE asset.workspace_id = sqlc.arg(workspace_id)
  AND asset.id = sqlc.arg(asset_id)
  AND revision.id = sqlc.arg(revision_id);

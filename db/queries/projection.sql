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

-- name: GetReleaseProjection :one
SELECT release.id,
       release.workspace_id,
       release.sequence,
       release.manifest_digest,
       release.state,
       release.rolled_back_to_release_id,
       release.origin_proposal_id,
       release.published_by,
       release.published_at
FROM releases AS release
WHERE release.workspace_id = sqlc.arg(workspace_id)
  AND release.id = sqlc.arg(release_id);

-- name: ListReleaseAssetProjection :many
SELECT entry.asset_id,
       entry.revision_id,
       entry.compatibility,
       entry.position,
       asset.namespace,
       asset.key
FROM release_assets AS entry
JOIN semantic_assets AS asset
  ON asset.workspace_id = entry.workspace_id
 AND asset.id = entry.asset_id
WHERE entry.workspace_id = sqlc.arg(workspace_id)
  AND entry.release_id = sqlc.arg(release_id)
ORDER BY entry.position;

-- name: ListReleaseObjectProjection :many
SELECT object_type, object_id, version, position
FROM release_objects
WHERE workspace_id = sqlc.arg(workspace_id)
  AND release_id = sqlc.arg(release_id)
ORDER BY position;

-- name: GetReleaseOriginProposalProjection :one
SELECT proposal.id,
       proposal.title,
       proposal.target_object_type,
       proposal.target_object_id,
       proposal.asset_id,
       proposal.created_by
FROM proposals AS proposal
WHERE proposal.workspace_id = sqlc.arg(workspace_id)
  AND proposal.id = sqlc.arg(proposal_id);

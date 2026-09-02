-- name: ListCatalogAssets :many
SELECT asset.id,
       asset.workspace_id,
       asset.namespace,
       asset.key,
       asset.asset_type,
       asset.lifecycle_state,
       asset.current_revision_id,
       COALESCE(revision.content->>'name', revision.content->>'title', asset.key) AS title,
       COALESCE(revision.content->>'summary', revision.content->>'definition', '')::text AS summary,
       asset.updated_at,
       CASE
           WHEN sqlc.arg(search)::text = '' THEN 0::double precision
           WHEN asset.namespace || '.' || asset.key = lower(sqlc.arg(search)::text) THEN 3::double precision
           WHEN asset.namespace || '.' || asset.key LIKE lower(sqlc.arg(search)::text) || '%' THEN 2::double precision
           ELSE ts_rank(
               to_tsvector('simple', COALESCE(revision.content::text, '')),
               plainto_tsquery('simple', sqlc.arg(search)::text)
           )::double precision
       END AS search_rank
FROM semantic_assets AS asset
LEFT JOIN asset_revisions AS revision ON revision.id = asset.current_revision_id
WHERE asset.workspace_id = sqlc.arg(workspace_id)
  AND (sqlc.arg(asset_type)::text = '' OR asset.asset_type = sqlc.arg(asset_type)::text)
  AND (sqlc.arg(lifecycle_state)::text = '' OR asset.lifecycle_state = sqlc.arg(lifecycle_state)::text)
  AND (
      sqlc.arg(search)::text = ''
      OR asset.namespace || '.' || asset.key ILIKE '%' || sqlc.arg(search)::text || '%'
      OR to_tsvector('simple', COALESCE(revision.content::text, '')) @@
         plainto_tsquery('simple', sqlc.arg(search)::text)
  )
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR (CASE
              WHEN sqlc.arg(search)::text = '' THEN 0::double precision
              WHEN asset.namespace || '.' || asset.key = lower(sqlc.arg(search)::text) THEN 3::double precision
              WHEN asset.namespace || '.' || asset.key LIKE lower(sqlc.arg(search)::text) || '%' THEN 2::double precision
              ELSE ts_rank(
                  to_tsvector('simple', COALESCE(revision.content::text, '')),
                  plainto_tsquery('simple', sqlc.arg(search)::text)
              )::double precision
          END) < sqlc.arg(cursor_rank)::double precision
      OR ((CASE
               WHEN sqlc.arg(search)::text = '' THEN 0::double precision
               WHEN asset.namespace || '.' || asset.key = lower(sqlc.arg(search)::text) THEN 3::double precision
               WHEN asset.namespace || '.' || asset.key LIKE lower(sqlc.arg(search)::text) || '%' THEN 2::double precision
               ELSE ts_rank(
                   to_tsvector('simple', COALESCE(revision.content::text, '')),
                   plainto_tsquery('simple', sqlc.arg(search)::text)
               )::double precision
           END) = sqlc.arg(cursor_rank)::double precision
          AND (asset.updated_at < sqlc.arg(cursor_updated_at)
               OR (asset.updated_at = sqlc.arg(cursor_updated_at) AND asset.id < sqlc.arg(cursor_id)::uuid)))
  )
ORDER BY search_rank DESC, asset.updated_at DESC, asset.id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetCatalogAsset :one
SELECT asset.id,
       asset.workspace_id,
       asset.namespace,
       asset.key,
       asset.asset_type,
       asset.lifecycle_state,
       asset.current_revision_id,
       asset.created_at,
       asset.updated_at,
       revision.id AS revision_id,
       revision.sequence AS revision_sequence,
       revision.schema_version,
       revision.content_digest,
       revision.content,
       revision.created_by,
       revision.created_at AS revision_created_at,
       COALESCE(revision.content->>'name', revision.content->>'title', asset.key) AS title,
       COALESCE(revision.content->>'summary', revision.content->>'definition', '')::text AS summary,
       (SELECT count(*)::integer
        FROM semantic_relations relation
        WHERE relation.workspace_id = asset.workspace_id
          AND (relation.subject_asset_id = asset.id OR relation.object_asset_id = asset.id)) AS relation_count
FROM semantic_assets AS asset
LEFT JOIN asset_revisions AS revision ON revision.id = asset.current_revision_id
WHERE asset.workspace_id = sqlc.arg(workspace_id)
  AND asset.id = sqlc.arg(asset_id);

-- name: ListCatalogAssetRevisions :many
SELECT revision.*
FROM asset_revisions AS revision
WHERE revision.workspace_id = sqlc.arg(workspace_id)
  AND revision.asset_id = sqlc.arg(asset_id)
  AND (
      NOT sqlc.arg(has_cursor)::boolean
      OR revision.sequence < sqlc.arg(cursor_sequence)
      OR (revision.sequence = sqlc.arg(cursor_sequence) AND revision.id < sqlc.arg(cursor_id))
  )
ORDER BY revision.sequence DESC, revision.id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetCatalogAssetRevision :one
SELECT *
FROM asset_revisions
WHERE workspace_id = sqlc.arg(workspace_id)
  AND asset_id = sqlc.arg(asset_id)
  AND id = sqlc.arg(revision_id);

-- name: GetRevisionEvidence :many
SELECT evidence.*, link.role, link.field_path, link.note
FROM revision_evidence_links AS link
JOIN evidence_artifacts AS evidence ON evidence.id = link.evidence_artifact_id
WHERE link.workspace_id = sqlc.arg(workspace_id)
  AND link.asset_revision_id = sqlc.arg(revision_id)
ORDER BY link.role, evidence.id;

-- name: LinkRevisionEvidence :execrows
INSERT INTO revision_evidence_links (
    workspace_id, asset_revision_id, evidence_artifact_id, role
)
SELECT sqlc.arg(workspace_id), sqlc.arg(revision_id), evidence.id, 'supports'
FROM evidence_artifacts AS evidence
WHERE evidence.workspace_id = sqlc.arg(workspace_id)
  AND evidence.id = sqlc.arg(evidence_id);

-- name: GetCatalogAssetForUpdate :one
SELECT *
FROM semantic_assets
WHERE workspace_id = sqlc.arg(workspace_id)
  AND id = sqlc.arg(asset_id)
FOR UPDATE;

-- name: NextAssetRevisionSequence :one
SELECT CAST(COALESCE(max(sequence), 0) + 1 AS bigint) AS next_sequence
FROM asset_revisions
WHERE workspace_id = sqlc.arg(workspace_id)
  AND asset_id = sqlc.arg(asset_id);

-- name: ListDirectCatalogAssetRelations :many
SELECT relation.id,
       relation.subject_asset_id,
       relation.predicate,
       relation.object_asset_id,
       relation.plane,
       relation.assertion_state,
       relation.source_revision_id,
       relation.evidence_artifact_id,
       relation.created_at,
       CASE WHEN relation.subject_asset_id = sqlc.arg(asset_id) THEN 'outgoing' ELSE 'incoming' END AS direction,
       (CASE WHEN relation.subject_asset_id = sqlc.arg(asset_id)
             THEN relation.object_asset_id ELSE relation.subject_asset_id END)::uuid AS counterpart_id,
       counterpart.namespace,
       counterpart.key,
       counterpart.asset_type,
       counterpart.lifecycle_state,
       counterpart.current_revision_id,
       counterpart.updated_at,
       COALESCE(revision.content->>'name', revision.content->>'title', counterpart.key) AS counterpart_title,
       COALESCE(revision.content->>'summary', revision.content->>'definition', '')::text AS counterpart_summary
FROM semantic_relations AS relation
JOIN semantic_assets AS counterpart
  ON counterpart.workspace_id = sqlc.arg(workspace_id)
 AND counterpart.id = CASE WHEN relation.subject_asset_id = sqlc.arg(asset_id)
                           THEN relation.object_asset_id ELSE relation.subject_asset_id END
LEFT JOIN asset_revisions AS revision ON revision.id = counterpart.current_revision_id
WHERE relation.workspace_id = sqlc.arg(workspace_id)
  AND (relation.subject_asset_id = sqlc.arg(asset_id) OR relation.object_asset_id = sqlc.arg(asset_id))
  AND (sqlc.arg(direction)::text = 'both'
       OR (sqlc.arg(direction)::text = 'outgoing' AND relation.subject_asset_id = sqlc.arg(asset_id))
       OR (sqlc.arg(direction)::text = 'incoming' AND relation.object_asset_id = sqlc.arg(asset_id)))
  AND (sqlc.arg(plane)::text = '' OR relation.plane = sqlc.arg(plane)::text)
ORDER BY relation.id;

-- name: GetCatalogDiscoveryRun :one
SELECT *
FROM discovery_runs
WHERE workspace_id = sqlc.arg(workspace_id)
  AND id = sqlc.arg(run_id);

-- name: GetCatalogDiscoveryFindings :many
SELECT *
FROM discovery_findings
WHERE discovery_run_id = sqlc.arg(run_id)
ORDER BY sequence;

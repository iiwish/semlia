-- name: ListCatalogAssets :many
SELECT asset.id,
       asset.workspace_id,
       asset.namespace,
       asset.key,
       asset.asset_type,
       asset.lifecycle_state,
       asset.current_revision_id,
       COALESCE(NULLIF(revision.content->>'displayName', ''), revision.content->>'name', revision.content->>'title', asset.key) AS title,
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

-- name: CountCatalogAssets :one
SELECT count(*)::bigint
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
  );

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
       COALESCE(NULLIF(revision.content->>'displayName', ''), revision.content->>'name', revision.content->>'title', asset.key) AS title,
       COALESCE(revision.content->>'summary', revision.content->>'definition', '')::text AS summary,
       (SELECT count(*)::integer
        FROM semantic_relations relation
        WHERE relation.workspace_id = asset.workspace_id
          AND (relation.subject_asset_id = asset.id OR relation.object_asset_id = asset.id)) AS relation_count
FROM semantic_assets AS asset
LEFT JOIN asset_revisions AS revision ON revision.id = asset.current_revision_id
WHERE asset.workspace_id = sqlc.arg(workspace_id)
  AND asset.id = sqlc.arg(asset_id);

-- name: GetCatalogAssetAuthorityFacts :one
WITH latest_pin AS (
    SELECT release_asset.release_id,
           release_asset.revision_id,
           release.sequence AS release_sequence,
           release.origin_proposal_id
    FROM release_assets AS release_asset
    JOIN releases AS release
      ON release.workspace_id = release_asset.workspace_id
     AND release.id = release_asset.release_id
    WHERE release_asset.workspace_id = sqlc.arg(workspace_id)
      AND release_asset.asset_id = sqlc.arg(asset_id)
    ORDER BY release.sequence DESC, release.id DESC
    LIMIT 1
), effective_objects AS (
    SELECT DISTINCT ON (snapshot.object_type, snapshot.object_id)
           snapshot.object_type, snapshot.object_id, snapshot.payload,
           snapshot.release_id, release.sequence AS release_sequence
    FROM release_object_snapshots AS snapshot
    JOIN releases AS release
      ON release.workspace_id = snapshot.workspace_id
     AND release.id = snapshot.release_id
    WHERE snapshot.workspace_id = sqlc.arg(workspace_id)
    ORDER BY snapshot.object_type, snapshot.object_id, release.sequence DESC, release.id DESC
), asset_objects AS (
    SELECT *
    FROM effective_objects
    WHERE object_type IN ('physical_binding', 'model_grain', 'entity_key')
      AND payload->>'asset_id' = sqlc.arg(asset_id)::uuid::text
), released_datasets AS (
    SELECT DISTINCT (payload->>'dataset_id')::uuid AS dataset_id
    FROM asset_objects
    WHERE object_type = 'physical_binding'
      AND payload ? 'dataset_id'
), join_objects AS (
    SELECT *
    FROM effective_objects
    WHERE object_type = 'join_contract'
      AND (
          (payload->>'left_dataset_id')::uuid IN (SELECT dataset_id FROM released_datasets)
          OR (payload->>'right_dataset_id')::uuid IN (SELECT dataset_id FROM released_datasets)
      )
), object_facts AS (
    SELECT count(*) FILTER (
               WHERE object_type = 'physical_binding'
           )::bigint AS physical_binding_count,
           count(*) FILTER (
               WHERE object_type = 'model_grain'
           )::bigint AS model_grain_count,
           count(*) FILTER (
               WHERE object_type = 'entity_key'
           )::bigint AS entity_key_count
    FROM asset_objects
), object_basis AS (
    SELECT release_id, release_sequence
    FROM asset_objects
    ORDER BY release_sequence DESC, release_id DESC
    LIMIT 1
), join_facts AS (
    SELECT count(*)::bigint AS join_contract_count
    FROM join_objects
), join_basis AS (
    SELECT release_id, release_sequence
    FROM join_objects
    ORDER BY release_sequence DESC, release_id DESC
    LIMIT 1
), validation_counts AS (
    SELECT count(DISTINCT run.id)::bigint AS run_count,
           count(result.id) FILTER (WHERE result.severity = 'blocker')::bigint AS blocker_count,
           count(result.id) FILTER (WHERE result.severity = 'warning')::bigint AS warning_count
    FROM latest_pin
    JOIN validation_runs AS run
      ON run.workspace_id = sqlc.arg(workspace_id)
     AND run.proposal_id = latest_pin.origin_proposal_id
    LEFT JOIN validation_results AS result
      ON result.workspace_id = run.workspace_id
     AND result.validation_run_id = run.id
), current_release AS (
    SELECT release.id
    FROM releases AS release
    WHERE release.workspace_id = sqlc.arg(workspace_id)
    ORDER BY release.sequence DESC, release.id DESC
    LIMIT 1
), effective_bindings AS (
    SELECT binding.mode,
           CASE WHEN binding.mode = 'current' THEN current_release.id ELSE binding.release_id END AS effective_release_id
    FROM consumer_bindings AS binding
    LEFT JOIN current_release ON true
    WHERE binding.workspace_id = sqlc.arg(workspace_id)
      AND binding.status = 'active'
      AND (binding.expires_at IS NULL OR binding.expires_at > CURRENT_TIMESTAMP)
), consumer_counts AS (
    SELECT count(*) FILTER (WHERE binding.mode = 'current')::bigint AS current_count,
           count(*) FILTER (WHERE binding.mode = 'pinned')::bigint AS pinned_count
    FROM effective_bindings AS binding
    JOIN latest_pin ON true
    JOIN release_assets AS manifest
      ON manifest.workspace_id = sqlc.arg(workspace_id)
     AND manifest.release_id = binding.effective_release_id
     AND manifest.asset_id = sqlc.arg(asset_id)
     AND manifest.revision_id = latest_pin.revision_id
), basis AS (
    SELECT asset.current_revision_id,
           latest_pin.release_id,
           latest_pin.revision_id AS released_revision_id,
           latest_pin.release_sequence,
		   asset.current_revision_id AS evidence_revision_id
    FROM semantic_assets AS asset
    LEFT JOIN latest_pin ON true
    WHERE asset.workspace_id = sqlc.arg(workspace_id)
      AND asset.id = sqlc.arg(asset_id)
)
SELECT basis.current_revision_id,
       basis.release_id,
       basis.released_revision_id,
       basis.release_sequence,
       basis.evidence_revision_id,
       (SELECT release_id FROM object_basis) AS physical_release_id,
       COALESCE((SELECT release_sequence FROM object_basis), 0)::bigint AS physical_release_sequence,
       (SELECT release_id FROM join_basis) AS join_release_id,
       COALESCE((SELECT release_sequence FROM join_basis), 0)::bigint AS join_release_sequence,
       (SELECT count(*)::bigint FROM semantic_relations AS relation
        WHERE relation.workspace_id = sqlc.arg(workspace_id)
          AND (relation.subject_asset_id = sqlc.arg(asset_id) OR relation.object_asset_id = sqlc.arg(asset_id))) AS relation_count,
       (SELECT count(*)::bigint
        FROM lineage_edges AS edge
        WHERE edge.workspace_id = sqlc.arg(workspace_id)
          AND (edge.upstream_dataset_id IN (SELECT dataset_id FROM released_datasets)
            OR edge.downstream_dataset_id IN (SELECT dataset_id FROM released_datasets))) AS lineage_count,
       COALESCE((SELECT physical_binding_count FROM object_facts), 0)::bigint AS physical_binding_count,
       COALESCE((SELECT model_grain_count FROM object_facts), 0)::bigint AS model_grain_count,
       COALESCE((SELECT entity_key_count FROM object_facts), 0)::bigint AS entity_key_count,
       COALESCE((SELECT join_contract_count FROM join_facts), 0)::bigint AS join_contract_count,
       COALESCE((SELECT run_count FROM validation_counts), 0)::bigint AS validation_run_count,
       COALESCE((SELECT blocker_count FROM validation_counts), 0)::bigint AS validation_blocker_count,
       COALESCE((SELECT warning_count FROM validation_counts), 0)::bigint AS validation_warning_count,
       (SELECT count(*)::bigint FROM revision_evidence_links AS link
        WHERE link.workspace_id = sqlc.arg(workspace_id)
          AND link.asset_revision_id = basis.evidence_revision_id) AS evidence_count,
       COALESCE((SELECT current_count FROM consumer_counts), 0)::bigint AS current_consumer_count,
       COALESCE((SELECT pinned_count FROM consumer_counts), 0)::bigint AS pinned_consumer_count
FROM basis;

-- name: ListCatalogAssetAuthorityRecords :many
WITH latest_pin AS (
    SELECT release_asset.release_id, release_asset.revision_id,
           release.sequence AS release_sequence, release.origin_proposal_id
    FROM release_assets AS release_asset
    JOIN releases AS release
      ON release.workspace_id = release_asset.workspace_id AND release.id = release_asset.release_id
    WHERE release_asset.workspace_id = sqlc.arg(workspace_id)
      AND release_asset.asset_id = sqlc.arg(asset_id)
    ORDER BY release.sequence DESC, release.id DESC
    LIMIT 1
), effective_objects AS (
    SELECT DISTINCT ON (snapshot.object_type, snapshot.object_id)
           snapshot.object_type, snapshot.object_id, snapshot.version, snapshot.payload,
           snapshot.release_id, release.sequence AS release_sequence
    FROM release_object_snapshots AS snapshot
    JOIN releases AS release
      ON release.workspace_id = snapshot.workspace_id AND release.id = snapshot.release_id
    WHERE snapshot.workspace_id = sqlc.arg(workspace_id)
    ORDER BY snapshot.object_type, snapshot.object_id, release.sequence DESC, release.id DESC
), asset_objects AS (
    SELECT * FROM effective_objects
    WHERE object_type IN ('physical_binding', 'model_grain', 'entity_key')
      AND payload->>'asset_id' = sqlc.arg(asset_id)::uuid::text
), asset_datasets AS (
    SELECT DISTINCT (payload->>'dataset_id')::uuid AS dataset_id
    FROM asset_objects
    WHERE object_type = 'physical_binding' AND payload ? 'dataset_id'
), join_objects AS (
    SELECT * FROM effective_objects
    WHERE object_type = 'join_contract'
      AND ((payload->>'left_dataset_id')::uuid IN (SELECT dataset_id FROM asset_datasets)
        OR (payload->>'right_dataset_id')::uuid IN (SELECT dataset_id FROM asset_datasets))
), current_release AS (
    SELECT release.id
    FROM releases AS release
    WHERE release.workspace_id = sqlc.arg(workspace_id)
    ORDER BY release.sequence DESC, release.id DESC
    LIMIT 1
), effective_bindings AS (
    SELECT binding.*,
           CASE WHEN binding.mode = 'current' THEN current_release.id ELSE binding.release_id END AS effective_release_id
    FROM consumer_bindings AS binding
    LEFT JOIN current_release ON true
    WHERE binding.workspace_id = sqlc.arg(workspace_id)
      AND binding.status = 'active'
      AND (binding.expires_at IS NULL OR binding.expires_at > CURRENT_TIMESTAMP)
), records AS (
    SELECT 'relations'::text AS section_kind, 'relation'::text AS record_kind,
           relation.id AS record_id, 'semantic_relations'::text AS authority,
           NULL::uuid AS release_id, 0::bigint AS release_sequence, 0::integer AS version,
           relation.assertion_state AS status, relation.predicate AS label,
           CASE WHEN relation.subject_asset_id = sqlc.arg(asset_id)
                THEN relation.object_asset_id ELSE relation.subject_asset_id END::text AS related_id,
           jsonb_build_object(
               'direction', CASE WHEN relation.subject_asset_id = sqlc.arg(asset_id) THEN 'outgoing' ELSE 'incoming' END,
               'predicate', relation.predicate, 'plane', relation.plane,
               'assertionState', relation.assertion_state,
               'subjectAssetId', relation.subject_asset_id,
               'objectAssetId', relation.object_asset_id
           ) AS details
    FROM semantic_relations AS relation
    WHERE relation.workspace_id = sqlc.arg(workspace_id)
      AND (relation.subject_asset_id = sqlc.arg(asset_id) OR relation.object_asset_id = sqlc.arg(asset_id))
    UNION ALL
    SELECT 'physical_bindings', object_type, object_id, 'release_object_snapshots',
           release_id, release_sequence, version,
           CASE WHEN object_type = 'physical_binding' AND payload->>'retired_at' IS NOT NULL THEN 'retired' ELSE 'available' END,
           CASE object_type
               WHEN 'physical_binding' THEN COALESCE(payload->>'transform', payload->>'dataset_id', object_type)
               WHEN 'model_grain' THEN COALESCE(payload->>'grain_expression', object_type)
               WHEN 'entity_key' THEN COALESCE(payload->>'uniqueness_semantics', object_type)
           END,
           CASE WHEN object_type = 'physical_binding' THEN COALESCE(payload->>'dataset_id', '') ELSE '' END,
           CASE object_type
               WHEN 'physical_binding' THEN jsonb_build_object(
                   'assetId', payload->>'asset_id', 'datasetId', payload->>'dataset_id',
                   'fieldId', payload->>'field_id', 'transform', payload->>'transform'
               )
               WHEN 'model_grain' THEN jsonb_build_object(
                   'assetId', payload->>'asset_id', 'grainExpression', payload->>'grain_expression',
                   'grainFieldRefs', COALESCE(payload->'grain_field_refs', '[]'::jsonb),
                   'documentedBy', payload->>'documented_by'
               )
               WHEN 'entity_key' THEN jsonb_build_object(
                   'assetId', payload->>'asset_id',
                   'keyFieldRefs', COALESCE(payload->'key_field_refs', '[]'::jsonb),
                   'uniquenessSemantics', payload->>'uniqueness_semantics'
               )
           END
    FROM asset_objects
    UNION ALL
    SELECT 'join_contracts', object_type, object_id, 'release_object_snapshots',
           release_id, release_sequence, version, COALESCE(payload->>'status', 'available'),
           COALESCE(payload->>'join_expression', payload->>'cardinality', object_type),
           CASE
               WHEN (payload->>'left_dataset_id')::uuid IN (SELECT dataset_id FROM asset_datasets)
                AND (payload->>'right_dataset_id')::uuid NOT IN (SELECT dataset_id FROM asset_datasets)
                 THEN COALESCE(payload->>'right_dataset_id', '')
               WHEN (payload->>'right_dataset_id')::uuid IN (SELECT dataset_id FROM asset_datasets)
                 THEN COALESCE(payload->>'left_dataset_id', '')
               ELSE ''
           END,
           jsonb_build_object(
               'direction', CASE
                   WHEN (payload->>'left_dataset_id')::uuid IN (SELECT dataset_id FROM asset_datasets)
                    AND (payload->>'right_dataset_id')::uuid IN (SELECT dataset_id FROM asset_datasets) THEN 'both'
                   WHEN (payload->>'left_dataset_id')::uuid IN (SELECT dataset_id FROM asset_datasets) THEN 'outgoing'
                   ELSE 'incoming'
               END,
               'leftDatasetId', payload->>'left_dataset_id',
               'rightDatasetId', payload->>'right_dataset_id',
               'leftFieldRefs', COALESCE(payload->'left_field_refs', '[]'::jsonb),
               'rightFieldRefs', COALESCE(payload->'right_field_refs', '[]'::jsonb),
               'joinType', payload->>'join_type', 'cardinality', payload->>'cardinality',
               'joinExpression', payload->>'join_expression'
           )
    FROM join_objects
    UNION ALL
    SELECT 'validation', 'validation_run', run.id, 'validation_runs',
           latest_pin.release_id, latest_pin.release_sequence, 0, run.status, run.validator_id,
           run.proposal_id::text, '{}'::jsonb
    FROM latest_pin
    JOIN validation_runs AS run
      ON run.workspace_id = sqlc.arg(workspace_id) AND run.proposal_id = latest_pin.origin_proposal_id
    UNION ALL
    SELECT 'lineage', 'lineage', edge.id, 'lineage_edges', NULL, 0, 0, edge.edge_kind,
           edge.edge_kind, CASE WHEN edge.upstream_dataset_id IN (SELECT dataset_id FROM asset_datasets)
                                THEN edge.downstream_dataset_id ELSE edge.upstream_dataset_id END::text,
           jsonb_build_object(
               'direction', CASE WHEN edge.upstream_dataset_id IN (SELECT dataset_id FROM asset_datasets)
                                 THEN 'outgoing' ELSE 'incoming' END,
               'upstreamDatasetId', edge.upstream_dataset_id,
               'downstreamDatasetId', edge.downstream_dataset_id,
               'edgeKind', edge.edge_kind, 'sourceRevisionId', edge.source_revision_id,
               'codeArtifactId', edge.code_artifact_id, 'confidence', edge.confidence
           )
    FROM lineage_edges AS edge
    WHERE edge.workspace_id = sqlc.arg(workspace_id)
      AND (edge.upstream_dataset_id IN (SELECT dataset_id FROM asset_datasets)
        OR edge.downstream_dataset_id IN (SELECT dataset_id FROM asset_datasets))
    UNION ALL
    SELECT 'consumer_impact', 'consumer_binding', binding.id, 'consumer_bindings',
           binding.effective_release_id, release.sequence, binding.version, binding.status,
           binding.environment, binding.consumer_id::text,
           jsonb_build_object('consumerId', binding.consumer_id, 'environment', binding.environment,
                              'purpose', binding.purpose, 'mode', binding.mode, 'status', binding.status,
                              'effectiveReleaseId', binding.effective_release_id,
                              'compatibilityConstraint', binding.compatibility_constraint,
                              'expiresAt', binding.expires_at)
    FROM effective_bindings AS binding
    JOIN latest_pin ON true
    JOIN release_assets AS manifest
      ON manifest.workspace_id = sqlc.arg(workspace_id)
     AND manifest.release_id = binding.effective_release_id
     AND manifest.asset_id = sqlc.arg(asset_id)
     AND manifest.revision_id = latest_pin.revision_id
    JOIN releases AS release
      ON release.workspace_id = binding.workspace_id AND release.id = binding.effective_release_id
), section_records AS (
    SELECT records.*, count(*) OVER()::bigint AS total_count
    FROM records
    WHERE section_kind = sqlc.arg(section_kind)::text
)
SELECT section_kind, record_kind, record_id, authority, release_id,
       release_sequence, version, status, label, related_id, details, total_count
FROM section_records
WHERE (
      NOT sqlc.arg(has_cursor)::boolean
      OR record_kind > sqlc.arg(cursor_record_kind)::text
      OR (record_kind = sqlc.arg(cursor_record_kind)::text AND record_id > sqlc.arg(cursor_record_id)::uuid)
  )
ORDER BY section_kind, record_kind, record_id
LIMIT sqlc.arg(page_limit);

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

-- name: CountCatalogAssetRevisions :one
SELECT count(*)::bigint
FROM asset_revisions
WHERE workspace_id = sqlc.arg(workspace_id)
  AND asset_id = sqlc.arg(asset_id);

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
       COALESCE(NULLIF(revision.content->>'displayName', ''), revision.content->>'name', revision.content->>'title', counterpart.key) AS counterpart_title,
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
ORDER BY relation.id
LIMIT sqlc.arg(page_limit);

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

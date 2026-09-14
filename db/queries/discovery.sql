-- name: CreateDiscoverySourceRevision :one
WITH inserted AS (
    INSERT INTO source_revisions (
        id, workspace_id, source_connection_id, external_revision, content_digest,
        adapter_version, observed_at, metadata
    ) VALUES (
        sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_connection_id), sqlc.narg(external_revision),
        sqlc.arg(content_digest), sqlc.arg(adapter_version), sqlc.arg(observed_at), sqlc.arg(metadata)
    )
    ON CONFLICT (source_connection_id, content_digest) DO NOTHING
    RETURNING source_revisions.*, true AS created
)
SELECT * FROM inserted
UNION ALL
SELECT source_revisions.*, false AS created
FROM source_revisions
WHERE workspace_id = sqlc.arg(workspace_id)
  AND source_connection_id = sqlc.arg(source_connection_id)
  AND content_digest = sqlc.arg(content_digest)
LIMIT 1;

-- name: GetSuccessfulDiscoveryRun :one
SELECT * FROM discovery_runs
WHERE workspace_id = sqlc.arg(workspace_id)
  AND source_revision_id = sqlc.arg(source_revision_id)
  AND adapter_version = sqlc.arg(adapter_version)
  AND status IN ('succeeded', 'degraded') AND NOT projection_reused;

-- name: GetSourceSnapshotForRun :one
SELECT snapshot_id FROM source_snapshot_runs
WHERE workspace_id=sqlc.arg(workspace_id) AND run_id=sqlc.arg(run_id);

-- name: CreateCompletedDiscoveryRun :one
INSERT INTO discovery_runs (
    id, workspace_id, source_connection_id, source_revision_id, adapter_version,
    status, error_code, stats, started_at, completed_at, updated_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_connection_id), sqlc.arg(source_revision_id),
    sqlc.arg(adapter_version), sqlc.arg(status), sqlc.narg(error_code), sqlc.arg(stats),
    sqlc.arg(completed_at), sqlc.arg(completed_at), sqlc.arg(completed_at)
)
RETURNING *;

-- name: FindPhysicalDatasetByQualifiedName :one
SELECT * FROM physical_datasets
WHERE workspace_id = sqlc.arg(workspace_id)
  AND source_connection_id = sqlc.arg(source_connection_id)
  AND qualified_name = sqlc.arg(qualified_name)
  AND external_key <> sqlc.arg(external_key)
LIMIT 1;

-- name: UpsertPhysicalDataset :one
INSERT INTO physical_datasets (
    id, workspace_id, source_connection_id, external_key, qualified_name
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_connection_id),
    sqlc.arg(external_key), sqlc.arg(qualified_name)
)
ON CONFLICT (source_connection_id, external_key) DO UPDATE
SET qualified_name = CASE WHEN sqlc.arg(publish_current)::boolean THEN EXCLUDED.qualified_name ELSE physical_datasets.qualified_name END,
    updated_at = CASE WHEN sqlc.arg(publish_current)::boolean THEN CURRENT_TIMESTAMP ELSE physical_datasets.updated_at END
RETURNING *;

-- name: GetCurrentPhysicalDatasetRevision :one
SELECT revision.*
FROM physical_datasets dataset
JOIN physical_dataset_revisions revision ON revision.id = dataset.current_revision_id
WHERE dataset.workspace_id = sqlc.arg(workspace_id)
  AND dataset.id = sqlc.arg(physical_dataset_id);

-- name: GetPhysicalDatasetRevisionByDigest :one
SELECT * FROM physical_dataset_revisions
WHERE workspace_id=sqlc.arg(workspace_id) AND physical_dataset_id=sqlc.arg(physical_dataset_id)
  AND content_digest=sqlc.arg(content_digest)
ORDER BY created_at,id LIMIT 1;

-- name: CreatePhysicalDatasetRevision :one
INSERT INTO physical_dataset_revisions (
    id, workspace_id, physical_dataset_id, source_revision_id,
    dataset_kind, locator, content_digest, metadata
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(physical_dataset_id), sqlc.arg(source_revision_id),
    sqlc.arg(dataset_kind), sqlc.arg(locator), sqlc.arg(content_digest), sqlc.arg(metadata)
)
RETURNING *;

-- name: SetCurrentPhysicalDatasetRevision :execrows
UPDATE physical_datasets
SET current_revision_id = sqlc.arg(revision_id), updated_at = CURRENT_TIMESTAMP
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(physical_dataset_id);

-- name: UpsertPhysicalField :one
INSERT INTO physical_fields (
    id, workspace_id, physical_dataset_id, external_key, name
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(physical_dataset_id),
    sqlc.arg(external_key), sqlc.arg(name)
)
ON CONFLICT (physical_dataset_id, external_key) DO UPDATE
SET name = CASE WHEN sqlc.arg(publish_current)::boolean THEN EXCLUDED.name ELSE physical_fields.name END,
    updated_at = CASE WHEN sqlc.arg(publish_current)::boolean THEN CURRENT_TIMESTAMP ELSE physical_fields.updated_at END
RETURNING *;

-- name: GetCurrentPhysicalFieldRevision :one
SELECT revision.*
FROM physical_fields field
JOIN physical_field_revisions revision ON revision.id = field.current_revision_id
WHERE field.workspace_id = sqlc.arg(workspace_id)
  AND field.id = sqlc.arg(physical_field_id);

-- name: GetPhysicalFieldRevisionByDigest :one
SELECT * FROM physical_field_revisions
WHERE workspace_id=sqlc.arg(workspace_id) AND physical_field_id=sqlc.arg(physical_field_id)
  AND dataset_revision_id=sqlc.arg(dataset_revision_id) AND metadata->>'fingerprint'=sqlc.arg(content_digest)::text
ORDER BY created_at,id LIMIT 1;

-- name: CreatePhysicalFieldRevision :one
INSERT INTO physical_field_revisions (
    id, workspace_id, physical_field_id, dataset_revision_id,
    ordinal, data_type, nullable, metadata
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(physical_field_id), sqlc.arg(dataset_revision_id),
    sqlc.arg(ordinal), sqlc.arg(data_type), sqlc.arg(nullable), sqlc.arg(metadata)
)
RETURNING *;

-- name: SetCurrentPhysicalFieldRevision :execrows
UPDATE physical_fields
SET current_revision_id = sqlc.arg(revision_id), updated_at = CURRENT_TIMESTAMP
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(physical_field_id);

-- name: UpsertCodeArtifact :one
INSERT INTO code_artifacts (
    id, workspace_id, source_revision_id, path, blob_oid, language, content_digest
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_revision_id), sqlc.arg(path),
    sqlc.narg(blob_oid), sqlc.arg(language), sqlc.arg(content_digest)
)
ON CONFLICT (source_revision_id, path) DO UPDATE
SET blob_oid = EXCLUDED.blob_oid, language = EXCLUDED.language, content_digest = EXCLUDED.content_digest
RETURNING *;

-- name: CreateLineageEdge :execrows
INSERT INTO lineage_edges (
    id, workspace_id, source_revision_id, upstream_dataset_id, downstream_dataset_id,
    edge_kind, code_artifact_id, confidence
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_revision_id), sqlc.arg(upstream_dataset_id),
    sqlc.arg(downstream_dataset_id), sqlc.arg(edge_kind), sqlc.narg(code_artifact_id), sqlc.arg(confidence)
)
ON CONFLICT (source_revision_id, upstream_dataset_id, downstream_dataset_id, edge_kind) DO NOTHING;

-- name: CreateDiscoveryFinding :exec
INSERT INTO discovery_findings (
    discovery_run_id, sequence, code, severity, locator, details
) VALUES (
    sqlc.arg(discovery_run_id), sqlc.arg(sequence), sqlc.arg(code), sqlc.arg(severity),
    sqlc.narg(locator), sqlc.arg(details)
);

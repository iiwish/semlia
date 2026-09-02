-- name: CreateSourceConnection :one
INSERT INTO source_connections (
    id, workspace_id, adapter_kind, name, normalized_locator, credential_ref, status, metadata
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(adapter_kind), sqlc.arg(name),
    sqlc.arg(normalized_locator), sqlc.narg(credential_ref), sqlc.arg(status), sqlc.arg(metadata)
)
RETURNING *;

-- name: CreateSourceRevision :one
WITH inserted AS (
INSERT INTO source_revisions (
    id, workspace_id, source_connection_id, external_revision, content_digest,
    adapter_version, observed_at, metadata
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(source_connection_id), sqlc.narg(external_revision),
    sqlc.arg(content_digest), sqlc.arg(adapter_version), sqlc.arg(observed_at), sqlc.arg(metadata)
)
ON CONFLICT (source_connection_id, content_digest) DO NOTHING
RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT * FROM source_revisions
WHERE source_connection_id = sqlc.arg(source_connection_id)
  AND content_digest = sqlc.arg(content_digest)
LIMIT 1;

-- name: GetSourceRevisionByDigest :one
SELECT * FROM source_revisions
WHERE workspace_id = sqlc.arg(workspace_id)
  AND source_connection_id = sqlc.arg(source_connection_id)
  AND content_digest = sqlc.arg(content_digest);

-- name: CreateSemanticAsset :one
INSERT INTO semantic_assets (
    id, workspace_id, namespace, key, asset_type, lifecycle_state
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(namespace), sqlc.arg(key),
    sqlc.arg(asset_type), sqlc.arg(lifecycle_state)
)
RETURNING *;

-- name: CreateAssetRevision :one
INSERT INTO asset_revisions (
    id, workspace_id, asset_id, sequence, schema_version, content_digest, content, created_by
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(asset_id), sqlc.arg(sequence),
    sqlc.arg(schema_version), sqlc.arg(content_digest), sqlc.arg(content), sqlc.arg(created_by)
)
RETURNING *;

-- name: SetCurrentAssetRevision :execrows
UPDATE semantic_assets
SET current_revision_id = sqlc.arg(revision_id), updated_at = CURRENT_TIMESTAMP
WHERE workspace_id = sqlc.arg(workspace_id) AND id = sqlc.arg(asset_id);

-- name: CreateEvidenceArtifact :one
WITH inserted AS (
INSERT INTO evidence_artifacts (
    id, workspace_id, evidence_type, source_revision_id, locator, content_digest, metadata
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(evidence_type), sqlc.narg(source_revision_id),
    sqlc.arg(locator), sqlc.arg(content_digest), sqlc.arg(metadata)
)
ON CONFLICT (workspace_id, evidence_type, locator, content_digest) DO NOTHING
RETURNING *
)
SELECT * FROM inserted
UNION ALL
SELECT * FROM evidence_artifacts
WHERE workspace_id = sqlc.arg(workspace_id)
  AND evidence_type = sqlc.arg(evidence_type)
  AND locator = sqlc.arg(locator)
  AND content_digest = sqlc.arg(content_digest)
LIMIT 1;

-- name: GetEvidenceArtifactByIdentity :one
SELECT * FROM evidence_artifacts
WHERE workspace_id = sqlc.arg(workspace_id)
  AND evidence_type = sqlc.arg(evidence_type)
  AND locator = sqlc.arg(locator)
  AND content_digest = sqlc.arg(content_digest);

-- name: CreateSemanticRelation :one
INSERT INTO semantic_relations (
    id, workspace_id, subject_asset_id, predicate, object_asset_id, plane, assertion_state,
    source_revision_id, evidence_artifact_id, inference_rule, created_by
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(subject_asset_id), sqlc.arg(predicate),
    sqlc.arg(object_asset_id), sqlc.arg(plane), sqlc.arg(assertion_state),
    sqlc.narg(source_revision_id), sqlc.narg(evidence_artifact_id), sqlc.narg(inference_rule),
    sqlc.arg(created_by)
)
RETURNING *;

-- name: CreateOntologyRevision :one
INSERT INTO ontology_revisions (
    id, workspace_id, sequence, status, content_digest, created_by, published_at
) VALUES (
    sqlc.arg(id), sqlc.arg(workspace_id), sqlc.arg(sequence), sqlc.arg(status),
    sqlc.arg(content_digest), sqlc.arg(created_by), sqlc.narg(published_at)
)
RETURNING *;

-- name: AddOntologyRelation :exec
INSERT INTO ontology_revision_relations (
    workspace_id, ontology_revision_id, semantic_relation_id
) VALUES (
    sqlc.arg(workspace_id), sqlc.arg(ontology_revision_id), sqlc.arg(semantic_relation_id)
);

-- name: PublishOntologyRevision :execrows
UPDATE ontology_revisions
SET status = 'published', published_at = sqlc.arg(published_at)
WHERE workspace_id = sqlc.arg(workspace_id)
  AND id = sqlc.arg(ontology_revision_id)
  AND status = 'draft';

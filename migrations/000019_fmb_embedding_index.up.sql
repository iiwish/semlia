BEGIN;

CREATE TABLE embedding_index_versions (
    id uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    release_id uuid NOT NULL,
    config jsonb NOT NULL CHECK (jsonb_typeof(config)='object'),
    config_digest text NOT NULL,
    corpus_digest text NOT NULL,
    chunk_count integer NOT NULL CHECK(chunk_count BETWEEN 1 AND 5000),
    vector_count integer NOT NULL DEFAULT 0 CHECK(vector_count>=0 AND vector_count<=chunk_count),
    state text NOT NULL CHECK(state IN ('building','active','retired','failed','cancelled')),
    error_code text NOT NULL DEFAULT '',
    job_id uuid NOT NULL UNIQUE REFERENCES jobs(id),
    runtime_run_id uuid NOT NULL UNIQUE REFERENCES runtime_runs(id),
    created_by_principal_id uuid NOT NULL REFERENCES principals(id),
    idempotency_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(workspace_id,id),
    UNIQUE(workspace_id,idempotency_key),
    FOREIGN KEY(workspace_id,release_id) REFERENCES releases(workspace_id,id)
);
CREATE UNIQUE INDEX embedding_one_building ON embedding_index_versions(workspace_id) WHERE state='building';
CREATE UNIQUE INDEX embedding_one_active ON embedding_index_versions(workspace_id) WHERE state='active';
CREATE TABLE knowledge_chunks (
    workspace_id uuid NOT NULL,
    index_version_id uuid NOT NULL,
    ordinal integer NOT NULL,
    asset_id uuid NOT NULL,
    revision_id uuid NOT NULL,
    address text NOT NULL,
    safe_text text NOT NULL CHECK(octet_length(safe_text) BETWEEN 1 AND 8192),
    content_digest text NOT NULL,
    PRIMARY KEY(index_version_id,ordinal),
    FOREIGN KEY(workspace_id,index_version_id) REFERENCES embedding_index_versions(workspace_id,id),
    FOREIGN KEY(workspace_id,asset_id) REFERENCES semantic_assets(workspace_id,id),
    FOREIGN KEY(workspace_id,revision_id) REFERENCES asset_revisions(workspace_id,id)
);
CREATE TABLE embedding_items (
    workspace_id uuid NOT NULL,
    index_version_id uuid NOT NULL,
    ordinal integer NOT NULL,
    dimension integer NOT NULL CHECK(dimension BETWEEN 1 AND 4096),
    PRIMARY KEY(index_version_id,ordinal),
    FOREIGN KEY(index_version_id,ordinal) REFERENCES knowledge_chunks(index_version_id,ordinal),
    FOREIGN KEY(workspace_id,index_version_id) REFERENCES embedding_index_versions(workspace_id,id)
);
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname='vector') THEN
        ALTER TABLE embedding_items ADD COLUMN vector_value vector NOT NULL;
    END IF;
END;
$$;

COMMIT;

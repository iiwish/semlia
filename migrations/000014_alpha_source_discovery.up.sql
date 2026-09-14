BEGIN;

ALTER TABLE source_connections
    ADD COLUMN active_credential_version bigint,
    ADD COLUMN artifact_paths jsonb NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(artifact_paths) = 'array'),
    ADD COLUMN disabled_at timestamptz;

CREATE TABLE source_credentials (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    version bigint NOT NULL CHECK (version > 0),
    key_version integer NOT NULL CHECK (key_version > 0),
    algorithm text NOT NULL CHECK (algorithm = 'AES-256-GCM'),
    nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
    ciphertext bytea NOT NULL CHECK (octet_length(ciphertext) >= 17),
    created_by text NOT NULL CHECK (created_by <> '' AND length(created_by) <= 256),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    retired_at timestamptz,
    UNIQUE (workspace_id, id),
    UNIQUE (source_connection_id, version),
    FOREIGN KEY (workspace_id, source_connection_id)
        REFERENCES source_connections (workspace_id, id) ON DELETE RESTRICT
);

ALTER TABLE source_connections
    ADD CONSTRAINT source_connections_active_credential_fkey
    FOREIGN KEY (id, active_credential_version)
    REFERENCES source_credentials (source_connection_id, version)
    DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE discovery_runs DROP CONSTRAINT discovery_runs_status_check;
ALTER TABLE discovery_runs DROP CONSTRAINT discovery_runs_time_state;
ALTER TABLE discovery_runs DROP CONSTRAINT discovery_runs_error_state;
ALTER TABLE discovery_runs
    ADD COLUMN credential_version bigint,
    ADD COLUMN job_id uuid,
    ADD COLUMN requested_by text,
    ADD COLUMN trace_id text CHECK (trace_id IS NULL OR trace_id ~ '^[0-9a-f]{32}$'),
    ADD CONSTRAINT discovery_runs_status_check
        CHECK (status IN ('queued', 'running', 'succeeded', 'degraded', 'failed', 'cancelled')),
    ADD CONSTRAINT discovery_runs_time_state CHECK (
        (status = 'queued' AND started_at IS NULL AND completed_at IS NULL)
        OR (status = 'running' AND started_at IS NOT NULL AND completed_at IS NULL)
        OR (status IN ('succeeded', 'degraded', 'failed', 'cancelled') AND started_at IS NOT NULL AND completed_at IS NOT NULL)
    ),
    ADD CONSTRAINT discovery_runs_error_state CHECK (
        (status = 'failed' AND error_code IS NOT NULL) OR (status <> 'failed' AND error_code IS NULL)
    ),
    ADD CONSTRAINT discovery_runs_credential_fkey
        FOREIGN KEY (source_connection_id, credential_version)
        REFERENCES source_credentials (source_connection_id, version) DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT discovery_runs_job_fkey FOREIGN KEY (job_id) REFERENCES jobs (id) ON DELETE RESTRICT,
    ADD CONSTRAINT discovery_runs_job_unique UNIQUE (job_id);

DROP INDEX discovery_runs_success_projection_key;
CREATE UNIQUE INDEX discovery_runs_success_projection_key
    ON discovery_runs (source_revision_id, adapter_version)
    WHERE status IN ('succeeded', 'degraded');
CREATE UNIQUE INDEX discovery_runs_active_source_idx
    ON discovery_runs (source_connection_id)
    WHERE status IN ('queued', 'running');

CREATE TABLE physical_key_observations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL,
    discovery_run_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    dataset_external_key text NOT NULL CHECK (dataset_external_key <> ''),
    constraint_name text NOT NULL CHECK (constraint_name <> ''),
    field_external_keys jsonb NOT NULL CHECK (jsonb_typeof(field_external_keys) = 'array'),
    key_kind text NOT NULL CHECK (key_kind IN ('primary', 'unique')),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (discovery_run_id, dataset_external_key, constraint_name),
    FOREIGN KEY (workspace_id, discovery_run_id) REFERENCES discovery_runs (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, source_revision_id) REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE join_observations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL,
    discovery_run_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    constraint_name text NOT NULL CHECK (constraint_name <> ''),
    from_dataset_external_key text NOT NULL CHECK (from_dataset_external_key <> ''),
    from_field_external_keys jsonb NOT NULL CHECK (jsonb_typeof(from_field_external_keys) = 'array'),
    to_dataset_external_key text NOT NULL CHECK (to_dataset_external_key <> ''),
    to_field_external_keys jsonb NOT NULL CHECK (jsonb_typeof(to_field_external_keys) = 'array'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (discovery_run_id, from_dataset_external_key, constraint_name),
    FOREIGN KEY (workspace_id, discovery_run_id) REFERENCES discovery_runs (workspace_id, id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, source_revision_id) REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE semantic_candidates (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    discovery_run_id uuid NOT NULL,
    candidate_key text NOT NULL CHECK (candidate_key <> ''),
    candidate_kind text NOT NULL CHECK (candidate_kind IN ('entity', 'dimension', 'metric', 'join')),
    title text NOT NULL CHECK (title <> '' AND length(title) <= 256),
    proposal_input jsonb NOT NULL CHECK (jsonb_typeof(proposal_input) = 'object'),
    evidence jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'array'),
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'dismissed', 'converted')),
    proposal_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (discovery_run_id, candidate_key),
    FOREIGN KEY (workspace_id, source_connection_id) REFERENCES source_connections (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_revision_id) REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, discovery_run_id) REFERENCES discovery_runs (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, proposal_id) REFERENCES proposals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT semantic_candidates_decision_state CHECK (
        (status = 'converted' AND proposal_id IS NOT NULL) OR
        (status <> 'converted' AND proposal_id IS NULL)
    )
);

CREATE INDEX semantic_candidates_workspace_status_idx
    ON semantic_candidates (workspace_id, status, created_at DESC, id);

CREATE TABLE semantic_candidate_decisions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    candidate_id uuid NOT NULL,
    action text NOT NULL CHECK (action IN ('dismiss', 'convert')),
    proposal_id uuid,
    actor text NOT NULL CHECK (actor <> '' AND length(actor) <= 256),
    reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 4096),
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 256),
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, idempotency_key),
    FOREIGN KEY (workspace_id, candidate_id) REFERENCES semantic_candidates (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, proposal_id) REFERENCES proposals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT semantic_candidate_decisions_proposal_shape CHECK (
        (action = 'convert' AND proposal_id IS NOT NULL) OR (action = 'dismiss' AND proposal_id IS NULL)
    )
);

CREATE TRIGGER semantic_candidate_decisions_immutable
    BEFORE UPDATE OR DELETE ON semantic_candidate_decisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

COMMIT;

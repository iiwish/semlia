BEGIN;

ALTER TABLE source_connections
    ADD COLUMN source_kind text NOT NULL DEFAULT 'postgresql'
        CHECK (source_kind IN ('postgresql', 'sql_bundle', 'dbt_bundle', 'file')),
    ADD COLUMN active_artifact_set_id uuid,
    ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version > 0);

ALTER TABLE source_connections
    ADD CONSTRAINT source_connections_kind_credential_shape CHECK (
        (source_kind = 'postgresql' AND active_artifact_set_id IS NULL)
        OR (source_kind <> 'postgresql' AND active_credential_version IS NULL
            AND (status <> 'active' OR active_artifact_set_id IS NOT NULL))
    ),
    ADD CONSTRAINT source_connections_workspace_kind_identity
        UNIQUE (workspace_id, id, source_kind);

ALTER TABLE source_revisions
    ADD CONSTRAINT source_revisions_workspace_source_identity
        UNIQUE (workspace_id, source_connection_id, id);

CREATE TABLE artifact_objects (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^sha256:[0-9a-f]{64}$'),
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 52428800),
    storage_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, content_sha256),
    UNIQUE (workspace_id, content_sha256, byte_size),
    UNIQUE (storage_key),
    CHECK (storage_key = 'workspaces/' || workspace_id::text || '/sha256/' ||
        substring(content_sha256 from 8))
);

CREATE TABLE workspace_artifact_usage (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE RESTRICT,
    active_object_count integer NOT NULL DEFAULT 0 CHECK (active_object_count BETWEEN 0 AND 10000),
    active_bytes bigint NOT NULL DEFAULT 0 CHECK (active_bytes BETWEEN 0 AND 5368709120),
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE artifact_object_retention (
    workspace_id uuid NOT NULL,
    content_sha256 text NOT NULL,
    state text NOT NULL CHECK (state IN ('reserved', 'retained', 'deleting', 'deleted')),
    reservation_count integer NOT NULL DEFAULT 0 CHECK (reservation_count BETWEEN 0 AND 10000),
    expires_at timestamptz,
    cleanup_attempts integer NOT NULL DEFAULT 0 CHECK (cleanup_attempts BETWEEN 0 AND 1000),
    cleanup_retry_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, content_sha256),
    FOREIGN KEY (workspace_id, content_sha256)
        REFERENCES artifact_objects (workspace_id, content_sha256) ON DELETE RESTRICT,
    CHECK (
        (state = 'reserved' AND reservation_count > 0 AND expires_at IS NOT NULL)
        OR (state = 'retained' AND reservation_count = 0)
        OR (state IN ('deleting', 'deleted') AND reservation_count = 0 AND expires_at IS NULL)
    )
);

CREATE INDEX artifact_object_retention_expiry_idx
    ON artifact_object_retention (expires_at, workspace_id, content_sha256)
    WHERE state = 'retained' AND expires_at IS NOT NULL;

CREATE TRIGGER artifact_objects_immutable
    BEFORE UPDATE OR DELETE ON artifact_objects
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE source_artifacts (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    source_connection_id uuid,
    artifact_kind text NOT NULL CHECK (
        artifact_kind IN ('csv', 'xlsx', 'markdown', 'sql', 'dbt_manifest', 'dbt_catalog')
    ),
    schema_version text NOT NULL CHECK (schema_version <> '' AND length(schema_version) <= 128),
    content_sha256 text,
    byte_size bigint NOT NULL CHECK (byte_size > 0 AND byte_size <= 52428800),
    media_type text NOT NULL CHECK (media_type IN (
        'text/csv',
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        'text/markdown', 'text/plain', 'application/sql', 'application/json'
    )),
    original_name text NOT NULL CHECK (
        original_name <> '' AND octet_length(original_name) <= 255
        AND original_name = regexp_replace(original_name, '[[:cntrl:]/\\]', '', 'g')
    ),
    registered_logical_path text CHECK (
        registered_logical_path IS NULL OR (
            artifact_kind = 'sql' AND registered_logical_path <> ''
            AND octet_length(registered_logical_path) <= 1024
            AND registered_logical_path !~ '(^|/)\.\.(/|$)'
            AND registered_logical_path !~ '^/' AND registered_logical_path !~ '\\'
        )
    ),
    validation_summary jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(validation_summary) = 'object'),
    status text NOT NULL CHECK (status IN ('uploaded', 'validated', 'rejected', 'consumed')),
    failure_code text CHECK (failure_code IS NULL OR failure_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    uploaded_by_principal_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 256),
    request_fingerprint text NOT NULL CHECK (request_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finalized_at timestamptz,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, source_connection_id, id, content_sha256),
    UNIQUE (workspace_id, idempotency_key),
    FOREIGN KEY (workspace_id, source_connection_id)
        REFERENCES source_connections (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, content_sha256, byte_size)
        REFERENCES artifact_objects (workspace_id, content_sha256, byte_size) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, uploaded_by_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT source_artifacts_state_shape CHECK (
        (status = 'rejected' AND content_sha256 IS NULL AND failure_code IS NOT NULL AND finalized_at IS NOT NULL AND expires_at IS NULL)
        OR (status = 'uploaded' AND content_sha256 IS NOT NULL AND failure_code IS NULL AND finalized_at IS NULL AND expires_at IS NOT NULL)
        OR (status IN ('validated', 'consumed') AND content_sha256 IS NOT NULL AND failure_code IS NULL AND finalized_at IS NOT NULL AND expires_at IS NULL)
    )
);

CREATE INDEX source_artifacts_workspace_created_idx
    ON source_artifacts (workspace_id, created_at DESC, id DESC);
CREATE INDEX source_artifacts_source_idx
    ON source_artifacts (source_connection_id, created_at DESC, id DESC)
    WHERE source_connection_id IS NOT NULL;
CREATE INDEX source_artifacts_expiry_idx
    ON source_artifacts (expires_at, workspace_id, id)
    WHERE status = 'uploaded';

CREATE INDEX discovery_runs_source_page_idx
    ON discovery_runs (workspace_id, source_connection_id, created_at DESC, id DESC);
CREATE INDEX source_connections_workspace_page_idx
    ON source_connections (workspace_id, updated_at DESC, id DESC);
CREATE INDEX semantic_candidates_workspace_status_page_idx
    ON semantic_candidates (workspace_id, status, created_at DESC, id DESC);

ALTER TABLE semantic_candidate_decisions
    ADD COLUMN request_fingerprint text CHECK (
        request_fingerprint IS NULL OR request_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    );

CREATE FUNCTION enforce_source_artifact_transition()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'source_artifacts rows are retained evidence' USING ERRCODE = '55000';
    END IF;
    IF NEW.id <> OLD.id OR NEW.workspace_id <> OLD.workspace_id
       OR NEW.artifact_kind <> OLD.artifact_kind OR NEW.schema_version <> OLD.schema_version
       OR NEW.byte_size <> OLD.byte_size OR NEW.media_type <> OLD.media_type
       OR NEW.original_name <> OLD.original_name
       OR NEW.registered_logical_path IS DISTINCT FROM OLD.registered_logical_path
       OR NEW.validation_summary <> OLD.validation_summary
       OR NEW.uploaded_by_principal_id <> OLD.uploaded_by_principal_id
       OR NEW.idempotency_key <> OLD.idempotency_key
       OR NEW.request_fingerprint <> OLD.request_fingerprint OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'source_artifact identity is immutable' USING ERRCODE = '55000';
    END IF;
    IF OLD.status = 'uploaded' AND NEW.status = 'validated' THEN
        IF NEW.source_connection_id IS NULL
           OR (OLD.source_connection_id IS NOT NULL AND NEW.source_connection_id IS DISTINCT FROM OLD.source_connection_id)
           OR NEW.content_sha256 IS DISTINCT FROM OLD.content_sha256 OR NEW.finalized_at IS NULL OR NEW.expires_at IS NOT NULL THEN
            RAISE EXCEPTION 'invalid artifact finalization' USING ERRCODE = '23514';
        END IF;
    ELSIF OLD.status = 'uploaded' AND NEW.status = 'rejected' THEN
        IF NEW.source_connection_id IS DISTINCT FROM OLD.source_connection_id OR NEW.content_sha256 IS NOT NULL
           OR NEW.failure_code IS NULL OR NEW.finalized_at IS NULL OR NEW.expires_at IS NOT NULL THEN
            RAISE EXCEPTION 'invalid artifact rejection' USING ERRCODE = '23514';
        END IF;
    ELSIF OLD.status = 'validated' AND NEW.status = 'consumed' THEN
        IF NEW.source_connection_id IS DISTINCT FROM OLD.source_connection_id
           OR NEW.content_sha256 IS DISTINCT FROM OLD.content_sha256
           OR NEW.finalized_at IS DISTINCT FROM OLD.finalized_at OR NEW.expires_at IS NOT NULL THEN
            RAISE EXCEPTION 'invalid artifact consumption' USING ERRCODE = '23514';
        END IF;
    ELSIF NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'invalid source_artifact transition' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER source_artifacts_transition_guard
    BEFORE UPDATE OR DELETE ON source_artifacts
    FOR EACH ROW EXECUTE FUNCTION enforce_source_artifact_transition();

CREATE TABLE source_artifact_sets (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    source_kind text NOT NULL CHECK (source_kind IN ('sql_bundle', 'dbt_bundle', 'file')),
    set_digest text NOT NULL CHECK (set_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_by_principal_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 256),
    request_fingerprint text NOT NULL CHECK (request_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, source_connection_id, id),
    UNIQUE (source_connection_id, set_digest),
    UNIQUE (source_connection_id, id),
    UNIQUE (workspace_id, idempotency_key),
    FOREIGN KEY (workspace_id, source_connection_id, source_kind)
        REFERENCES source_connections (workspace_id, id, source_kind) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, created_by_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT
);

CREATE TRIGGER source_artifact_sets_immutable
    BEFORE UPDATE OR DELETE ON source_artifact_sets
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE source_artifact_set_members (
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    artifact_set_id uuid NOT NULL,
    source_artifact_id uuid NOT NULL,
    logical_path text NOT NULL CHECK (
        logical_path <> '' AND length(logical_path) <= 1024
        AND logical_path !~ '(^|/)\.\.(/|$)' AND logical_path !~ '^/' AND logical_path !~ '\\'
    ),
    ordinal integer NOT NULL CHECK (ordinal > 0 AND ordinal <= 4096),
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (artifact_set_id, source_artifact_id),
    UNIQUE (artifact_set_id, logical_path),
    UNIQUE (artifact_set_id, ordinal),
    UNIQUE (workspace_id, source_connection_id, artifact_set_id, source_artifact_id,
        logical_path, ordinal, content_sha256),
    FOREIGN KEY (workspace_id, source_connection_id, artifact_set_id)
        REFERENCES source_artifact_sets (workspace_id, source_connection_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_connection_id, source_artifact_id, content_sha256)
        REFERENCES source_artifacts (workspace_id, source_connection_id, id, content_sha256) ON DELETE RESTRICT
);

CREATE TRIGGER source_artifact_set_members_immutable
    BEFORE UPDATE OR DELETE ON source_artifact_set_members
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

ALTER TABLE source_connections
    ADD CONSTRAINT source_connections_active_artifact_set_fkey
    FOREIGN KEY (id, active_artifact_set_id)
    REFERENCES source_artifact_sets (source_connection_id, id)
    DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE discovery_runs
    ADD COLUMN projection_reused boolean NOT NULL DEFAULT false,
    ADD COLUMN artifact_set_id uuid,
    ADD COLUMN request_fingerprint text CHECK (
        request_fingerprint IS NULL OR request_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    ),
    ADD COLUMN source_input_fingerprint text CHECK (
        source_input_fingerprint IS NULL OR source_input_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    ),
    ADD COLUMN source_config jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(source_config) = 'object'),
    ADD CONSTRAINT discovery_runs_artifact_set_fkey
        FOREIGN KEY (source_connection_id, artifact_set_id)
        REFERENCES source_artifact_sets (source_connection_id, id) DEFERRABLE INITIALLY DEFERRED,
    ADD CONSTRAINT discovery_runs_artifact_identity
        UNIQUE (workspace_id, source_connection_id, id, artifact_set_id),
    ADD CONSTRAINT discovery_runs_workspace_source_identity
        UNIQUE (workspace_id, source_connection_id, id),
    ADD CONSTRAINT discovery_runs_workspace_job_identity
        UNIQUE (workspace_id, id, job_id);

DROP INDEX discovery_runs_success_projection_key;
CREATE UNIQUE INDEX discovery_runs_success_projection_key
    ON discovery_runs (source_revision_id, adapter_version)
    WHERE status IN ('succeeded', 'degraded') AND NOT projection_reused;

UPDATE discovery_runs run
SET source_config = jsonb_build_object(
    'metadata', source.metadata,
    'artifactPaths', source.artifact_paths,
    'sourceVersion', source.version
)
FROM source_connections source
WHERE source.id = run.source_connection_id;

CREATE FUNCTION reject_discovery_run_input_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.credential_version IS DISTINCT FROM OLD.credential_version
       OR NEW.artifact_set_id IS DISTINCT FROM OLD.artifact_set_id
       OR NEW.request_fingerprint IS DISTINCT FROM OLD.request_fingerprint
       OR NEW.source_input_fingerprint IS DISTINCT FROM OLD.source_input_fingerprint
       OR NEW.source_config IS DISTINCT FROM OLD.source_config THEN
        RAISE EXCEPTION 'discovery run input is immutable' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER discovery_runs_input_immutable
    BEFORE UPDATE OF credential_version, artifact_set_id, request_fingerprint,
        source_input_fingerprint, source_config ON discovery_runs
    FOR EACH ROW EXECUTE FUNCTION reject_discovery_run_input_mutation();

CREATE TABLE discovery_run_artifacts (
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    discovery_run_id uuid NOT NULL,
    artifact_set_id uuid NOT NULL,
    source_artifact_id uuid NOT NULL,
    logical_path text NOT NULL CHECK (logical_path <> '' AND length(logical_path) <= 1024),
    ordinal integer NOT NULL CHECK (ordinal > 0 AND ordinal <= 4096),
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (discovery_run_id, source_artifact_id),
    UNIQUE (discovery_run_id, logical_path),
    UNIQUE (discovery_run_id, ordinal),
    FOREIGN KEY (workspace_id, source_connection_id, discovery_run_id, artifact_set_id)
        REFERENCES discovery_runs (workspace_id, source_connection_id, id, artifact_set_id) ON DELETE CASCADE,
    FOREIGN KEY (workspace_id, source_connection_id, artifact_set_id, source_artifact_id,
        logical_path, ordinal, content_sha256)
        REFERENCES source_artifact_set_members (
            workspace_id, source_connection_id, artifact_set_id, source_artifact_id,
            logical_path, ordinal, content_sha256
        ) ON DELETE RESTRICT
);

CREATE TRIGGER discovery_run_artifacts_immutable
    BEFORE UPDATE OR DELETE ON discovery_run_artifacts
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE source_revision_artifact_sets (
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    source_revision_id uuid PRIMARY KEY,
    artifact_set_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, source_connection_id, source_revision_id, artifact_set_id),
    FOREIGN KEY (workspace_id, source_connection_id, source_revision_id)
        REFERENCES source_revisions (workspace_id, source_connection_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_connection_id, artifact_set_id)
        REFERENCES source_artifact_sets (workspace_id, source_connection_id, id) ON DELETE RESTRICT
);

CREATE TRIGGER source_revision_artifact_sets_immutable
    BEFORE UPDATE OR DELETE ON source_revision_artifact_sets
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE source_revision_artifacts (
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    source_artifact_id uuid NOT NULL,
    artifact_set_id uuid NOT NULL,
    logical_path text NOT NULL CHECK (logical_path <> '' AND length(logical_path) <= 1024),
    ordinal integer NOT NULL CHECK (ordinal > 0 AND ordinal <= 4096),
    content_sha256 text NOT NULL CHECK (content_sha256 ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (source_revision_id, source_artifact_id),
    UNIQUE (source_revision_id, ordinal),
    FOREIGN KEY (workspace_id, source_connection_id, source_revision_id)
        REFERENCES source_revisions (workspace_id, source_connection_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_connection_id, source_revision_id, artifact_set_id)
        REFERENCES source_revision_artifact_sets (
            workspace_id, source_connection_id, source_revision_id, artifact_set_id
        ) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_connection_id, artifact_set_id, source_artifact_id,
        logical_path, ordinal, content_sha256)
        REFERENCES source_artifact_set_members (
            workspace_id, source_connection_id, artifact_set_id, source_artifact_id,
            logical_path, ordinal, content_sha256
        ) ON DELETE RESTRICT
);

CREATE TRIGGER source_revision_artifacts_immutable
    BEFORE UPDATE OR DELETE ON source_revision_artifacts
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE source_schedules (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    expression text NOT NULL CHECK (expression <> '' AND length(expression) <= 128),
    timezone text NOT NULL CHECK (timezone <> '' AND length(timezone) <= 128),
    misfire_policy text NOT NULL CHECK (misfire_policy IN ('skip', 'run_once')),
    enabled boolean NOT NULL DEFAULT true,
    next_run_at timestamptz,
    next_wall_clock_key text CHECK (
        next_wall_clock_key IS NULL OR next_wall_clock_key ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}$'
    ),
    last_run_at timestamptz,
    credential_version bigint,
    created_by_principal_id uuid NOT NULL,
    create_idempotency_key text NOT NULL CHECK (
        create_idempotency_key <> '' AND length(create_idempotency_key) <= 256
    ),
    create_request_fingerprint text NOT NULL CHECK (
        create_request_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    ),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    lease_owner text,
    leased_until timestamptz,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, source_connection_id, id),
    UNIQUE (workspace_id, create_idempotency_key),
    FOREIGN KEY (workspace_id, source_connection_id)
        REFERENCES source_connections (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, created_by_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (source_connection_id, credential_version)
        REFERENCES source_credentials (source_connection_id, version) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT source_schedules_enabled_shape CHECK (
        (deleted_at IS NULL AND ((enabled AND next_run_at IS NOT NULL AND next_wall_clock_key IS NOT NULL)
            OR (NOT enabled AND next_run_at IS NULL AND next_wall_clock_key IS NULL
                AND lease_owner IS NULL AND leased_until IS NULL)))
        OR (deleted_at IS NOT NULL AND NOT enabled AND next_run_at IS NULL
            AND next_wall_clock_key IS NULL AND lease_owner IS NULL AND leased_until IS NULL)
    ),
    CONSTRAINT source_schedules_lease_shape CHECK (
        (lease_owner IS NULL AND leased_until IS NULL)
        OR (lease_owner IS NOT NULL AND lease_owner <> '' AND leased_until IS NOT NULL)
    )
);

CREATE INDEX source_schedules_due_idx
    ON source_schedules (next_run_at, id)
    WHERE enabled AND deleted_at IS NULL;
CREATE INDEX source_schedules_workspace_source_idx
    ON source_schedules (workspace_id, source_connection_id, updated_at DESC, id DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE source_schedule_command_receipts (
    workspace_id uuid NOT NULL,
    schedule_id uuid NOT NULL,
    command text NOT NULL CHECK (command IN ('update', 'pause', 'resume', 'delete', 'run_now')),
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 256),
    request_fingerprint text NOT NULL CHECK (request_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
    result_schedule_version bigint NOT NULL CHECK (result_schedule_version > 0),
    result_schedule jsonb NOT NULL CHECK (jsonb_typeof(result_schedule) = 'object'),
    occurrence_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, idempotency_key),
    FOREIGN KEY (workspace_id, schedule_id)
        REFERENCES source_schedules (workspace_id, id) ON DELETE RESTRICT
);

CREATE TRIGGER source_schedule_command_receipts_immutable
    BEFORE UPDATE OR DELETE ON source_schedule_command_receipts
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE source_schedule_occurrences (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    schedule_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    trigger_kind text NOT NULL CHECK (trigger_kind IN ('scheduled', 'run_now')),
    schedule_version bigint NOT NULL CHECK (schedule_version > 0),
    scheduled_for timestamptz,
    eligible_at timestamptz NOT NULL,
    wall_clock_key text NOT NULL CHECK (
        wall_clock_key ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}$'
    ),
    state text NOT NULL CHECK (state IN ('enqueued', 'skipped')),
    reason_code text CHECK (reason_code IS NULL OR reason_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    misfire_disposition text CHECK (
        misfire_disposition IS NULL OR misfire_disposition IN ('on_time', 'coalesced')
    ),
    discovery_run_id uuid,
    job_id uuid,
    runtime_run_id uuid,
    artifact_set_id uuid,
    credential_version bigint,
    source_fingerprint text CHECK (
        source_fingerprint IS NULL OR source_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    ),
    requested_by_principal_id uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 256),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, idempotency_key),
    FOREIGN KEY (workspace_id, source_connection_id, schedule_id)
        REFERENCES source_schedules (workspace_id, source_connection_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_connection_id)
        REFERENCES source_connections (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, discovery_run_id)
        REFERENCES discovery_runs (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, job_id)
        REFERENCES jobs (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, runtime_run_id)
        REFERENCES runtime_runs (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (source_connection_id, artifact_set_id)
        REFERENCES source_artifact_sets (source_connection_id, id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (source_connection_id, credential_version)
        REFERENCES source_credentials (source_connection_id, version) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (workspace_id, requested_by_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT source_schedule_occurrences_state_shape CHECK (
        (state = 'enqueued' AND scheduled_for IS NOT NULL AND discovery_run_id IS NOT NULL
            AND job_id IS NOT NULL AND runtime_run_id = discovery_run_id AND reason_code IS NULL
            AND source_fingerprint IS NOT NULL
            AND num_nonnulls(artifact_set_id, credential_version) = 1)
        OR (state = 'skipped' AND discovery_run_id IS NULL AND job_id IS NULL
            AND runtime_run_id IS NULL AND reason_code IS NOT NULL
            AND artifact_set_id IS NULL AND credential_version IS NULL AND source_fingerprint IS NULL
            AND (scheduled_for IS NOT NULL OR reason_code = 'DST_GAP'))
    )
);

CREATE FUNCTION validate_source_schedule_occurrence_pins()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    pinned_source uuid;
    pinned_job uuid;
    pinned_artifact_set uuid;
    pinned_credential bigint;
    pinned_fingerprint text;
    current_schedule_version bigint;
BEGIN
    SELECT version INTO current_schedule_version
    FROM source_schedules
    WHERE workspace_id = NEW.workspace_id AND id = NEW.schedule_id;
    IF current_schedule_version IS DISTINCT FROM NEW.schedule_version THEN
        RAISE EXCEPTION 'schedule occurrence version does not match owning schedule' USING ERRCODE = '23514';
    END IF;
    IF NEW.state = 'enqueued' THEN
        SELECT source_connection_id, job_id, artifact_set_id, credential_version, source_input_fingerprint
        INTO pinned_source, pinned_job, pinned_artifact_set, pinned_credential, pinned_fingerprint
        FROM discovery_runs
        WHERE workspace_id = NEW.workspace_id AND id = NEW.discovery_run_id;
        IF NOT FOUND
           OR pinned_source IS DISTINCT FROM NEW.source_connection_id
           OR pinned_job IS DISTINCT FROM NEW.job_id
           OR pinned_artifact_set IS DISTINCT FROM NEW.artifact_set_id
           OR pinned_credential IS DISTINCT FROM NEW.credential_version
           OR pinned_fingerprint IS DISTINCT FROM NEW.source_fingerprint THEN
            RAISE EXCEPTION 'schedule occurrence pins do not match owning discovery run' USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER source_schedule_occurrences_pin_guard
    BEFORE INSERT ON source_schedule_occurrences
    FOR EACH ROW EXECUTE FUNCTION validate_source_schedule_occurrence_pins();

CREATE TRIGGER source_schedule_occurrences_immutable
    BEFORE UPDATE OR DELETE ON source_schedule_occurrences
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

ALTER TABLE source_schedule_command_receipts
    ADD CONSTRAINT source_schedule_command_receipts_occurrence_fkey
    FOREIGN KEY (workspace_id, occurrence_id)
    REFERENCES source_schedule_occurrences (workspace_id, id) DEFERRABLE INITIALLY DEFERRED;

CREATE UNIQUE INDEX source_schedule_occurrences_scheduled_instant_idx
    ON source_schedule_occurrences (schedule_id, schedule_version, scheduled_for)
    WHERE trigger_kind = 'scheduled' AND scheduled_for IS NOT NULL;
CREATE UNIQUE INDEX source_schedule_occurrences_scheduled_wall_idx
    ON source_schedule_occurrences (schedule_id, schedule_version, wall_clock_key)
    WHERE trigger_kind = 'scheduled';

ALTER TABLE source_schedule_occurrences
    ADD CONSTRAINT source_schedule_occurrences_run_source_fkey
        FOREIGN KEY (workspace_id, source_connection_id, discovery_run_id)
        REFERENCES discovery_runs (workspace_id, source_connection_id, id) ON DELETE RESTRICT,
    ADD CONSTRAINT source_schedule_occurrences_run_job_fkey
        FOREIGN KEY (workspace_id, discovery_run_id, job_id)
        REFERENCES discovery_runs (workspace_id, id, job_id) ON DELETE RESTRICT;

CREATE INDEX source_schedule_occurrences_schedule_idx
    ON source_schedule_occurrences (schedule_id, scheduled_for DESC, id);
CREATE INDEX source_schedule_occurrences_page_idx
    ON source_schedule_occurrences (workspace_id, schedule_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION project_audit_event_targets(payload jsonb)
RETURNS TABLE (object_type text, object_id text, ordinal smallint)
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT mapped.object_type, payload->>mapped.payload_key, mapped.ordinal
    FROM (VALUES
        ('workspace', 'workspaceId', 10::smallint),
        ('account', 'accountId', 20::smallint),
        ('principal', 'principalId', 30::smallint),
        ('workspace_membership', 'membershipId', 40::smallint),
        ('workspace_invitation', 'invitationId', 50::smallint),
        ('role', 'roleId', 60::smallint),
        ('binding', 'bindingId', 70::smallint),
        ('asset', 'assetId', 80::smallint),
        ('revision', 'revisionId', 81::smallint),
        ('revision', 'baseRevisionId', 82::smallint),
        ('proposal', 'proposalId', 90::smallint),
        ('review_batch', 'reviewBatchId', 91::smallint),
        ('release', 'releaseId', 92::smallint),
        ('release', 'rolledBackToReleaseId', 93::smallint),
        ('source', 'sourceId', 94::smallint),
        ('source_revision', 'sourceRevisionId', 95::smallint),
        ('discovery_run', 'discoveryRunId', 96::smallint),
        ('runtime_run', 'runId', 97::smallint),
        ('semantic_candidate', 'candidateId', 98::smallint),
        ('consumer', 'consumerId', 99::smallint),
        ('semantic_query', 'queryId', 100::smallint),
        ('artifact', 'artifactId', 71::smallint),
        ('artifact_set', 'artifactSetId', 72::smallint),
        ('source_schedule', 'scheduleId', 73::smallint),
        ('schedule_occurrence', 'occurrenceId', 74::smallint)
    ) AS mapped(object_type, payload_key, ordinal)
    WHERE jsonb_typeof(payload->mapped.payload_key) = 'string'
      AND payload->>mapped.payload_key <> ''
      AND length(payload->>mapped.payload_key) <= 128
    UNION ALL
    SELECT 'proposal', proposal_id, ordinal::smallint
    FROM jsonb_array_elements_text(
        CASE WHEN jsonb_typeof(payload->'proposals') = 'array' THEN payload->'proposals' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS proposal(proposal_id, ordinal)
    WHERE ordinal <= 100 AND proposal_id <> '' AND length(proposal_id) <= 128
    UNION ALL
    SELECT payload->>'objectType', payload->>'objectId', 0::smallint
    WHERE jsonb_typeof(payload->'objectType') = 'string'
      AND jsonb_typeof(payload->'objectId') = 'string'
      AND payload->>'objectType' IN (
          'workspace', 'account', 'principal', 'workspace_membership', 'workspace_invitation',
          'role', 'binding', 'asset', 'revision', 'proposal', 'review_batch', 'release',
          'source', 'source_revision', 'discovery_run', 'runtime_run', 'semantic_candidate',
          'consumer', 'semantic_query', 'audit_export', 'semantic_asset', 'model_grain',
          'entity_key', 'join_contract', 'artifact', 'artifact_set', 'source_schedule',
          'schedule_occurrence'
      )
      AND payload->>'objectId' <> ''
      AND length(payload->>'objectId') <= 128;
$$;

INSERT INTO audit_event_targets (workspace_id, audit_event_id, object_type, object_id, ordinal)
SELECT event.workspace_id, event.id, target.object_type, target.object_id, target.ordinal
FROM audit_events AS event
CROSS JOIN LATERAL project_audit_event_targets(event.payload) AS target
WHERE target.object_type IN ('artifact', 'artifact_set', 'source_schedule', 'schedule_occurrence')
ON CONFLICT DO NOTHING;

COMMIT;

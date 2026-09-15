BEGIN;

ALTER TABLE audit_events
    ADD CONSTRAINT audit_events_workspace_identity UNIQUE (workspace_id, id);
ALTER TABLE jobs
    ADD CONSTRAINT jobs_workspace_identity UNIQUE (workspace_id, id);

CREATE TABLE runtime_settings (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE RESTRICT,
    retry_ceiling integer NOT NULL DEFAULT 3 CHECK (retry_ceiling BETWEEN 1 AND 10),
    statement_timeout_ms integer NOT NULL DEFAULT 30000 CHECK (statement_timeout_ms BETWEEN 100 AND 300000),
    webhook_timeout_ms integer NOT NULL DEFAULT 10000 CHECK (webhook_timeout_ms BETWEEN 100 AND 120000),
    query_row_limit integer NOT NULL DEFAULT 10000 CHECK (query_row_limit BETWEEN 1 AND 100000),
    query_byte_limit bigint NOT NULL DEFAULT 10485760 CHECK (query_byte_limit BETWEEN 1024 AND 104857600),
    run_metadata_retention_days integer NOT NULL DEFAULT 30 CHECK (run_metadata_retention_days BETWEEN 1 AND 365),
    version bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE runtime_runs (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN (
        'discovery', 'validation', 'agent', 'semantic_resolution', 'embedding_rebuild',
        'webhook_delivery', 'query_execution', 'audit_export'
    )),
    source_type text NOT NULL CHECK (source_type ~ '^[a-z][a-z0-9_.-]{1,63}$'),
    source_id text NOT NULL CHECK (source_id <> '' AND length(source_id) <= 128),
    source_version_digest text NOT NULL CHECK (source_version_digest ~ '^[0-9a-f]{64}$'),
    job_id uuid,
    trace_id text CHECK (trace_id IS NULL OR trace_id ~ '^[0-9a-f]{32}$'),
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 256),
    requested_by_principal_id uuid,
    state text NOT NULL CHECK (state IN (
        'queued', 'running', 'succeeded', 'degraded', 'failed', 'cancelled', 'dead_letter'
    )),
    phase text CHECK (phase IS NULL OR (phase ~ '^[a-z][a-z0-9_.-]{0,63}$')),
    progress_current bigint CHECK (progress_current IS NULL OR progress_current >= 0),
    progress_total bigint CHECK (progress_total IS NULL OR progress_total > 0),
    attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts integer NOT NULL DEFAULT 1 CHECK (max_attempts >= 1 AND attempt <= max_attempts),
    started_at timestamptz,
    finished_at timestamptz,
    error_code text CHECK (error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    error_summary text CHECK (error_summary IS NULL OR length(error_summary) <= 512),
    version bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT runtime_runs_progress_pair CHECK (
        (progress_current IS NULL AND progress_total IS NULL)
        OR (progress_current IS NOT NULL AND progress_total IS NOT NULL AND progress_current <= progress_total)
    ),
    CONSTRAINT runtime_runs_finish_state CHECK (
        (state IN ('succeeded', 'degraded', 'failed', 'cancelled', 'dead_letter') AND finished_at IS NOT NULL)
        OR (state IN ('queued', 'running') AND finished_at IS NULL)
    ),
    CONSTRAINT runtime_runs_error_state CHECK (
        (state IN ('failed', 'dead_letter') AND error_code IS NOT NULL)
        OR (state NOT IN ('failed', 'dead_letter') AND error_code IS NULL)
    ),
    FOREIGN KEY (workspace_id, job_id)
        REFERENCES jobs (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, requested_by_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, kind, source_type, source_id),
    UNIQUE (workspace_id, idempotency_key)
);

CREATE UNIQUE INDEX runtime_runs_workspace_job_idx
    ON runtime_runs (workspace_id, job_id) WHERE job_id IS NOT NULL;
CREATE INDEX runtime_runs_workspace_updated_idx
    ON runtime_runs (workspace_id, updated_at DESC, id DESC);
CREATE INDEX runtime_runs_workspace_state_idx
    ON runtime_runs (workspace_id, state, updated_at DESC, id DESC);
CREATE INDEX runtime_runs_workspace_kind_idx
    ON runtime_runs (workspace_id, kind, updated_at DESC, id DESC);
CREATE INDEX runtime_runs_workspace_trace_idx
    ON runtime_runs (workspace_id, trace_id, updated_at DESC, id DESC);

CREATE TABLE runtime_run_events (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    runtime_run_id uuid NOT NULL,
    event_key text NOT NULL CHECK (event_key <> '' AND length(event_key) <= 256),
    sequence bigint NOT NULL CHECK (sequence >= 1),
    event_type text NOT NULL CHECK (event_type IN ('state', 'phase', 'progress', 'diagnostic')),
    phase text CHECK (phase IS NULL OR phase ~ '^[a-z][a-z0-9_.-]{0,63}$'),
    progress_current bigint CHECK (progress_current IS NULL OR progress_current >= 0),
    progress_total bigint CHECK (progress_total IS NULL OR progress_total > 0),
    state text CHECK (state IS NULL OR state IN (
        'queued', 'running', 'succeeded', 'degraded', 'failed', 'cancelled', 'dead_letter'
    )),
    error_code text CHECK (error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    summary text CHECK (summary IS NULL OR length(summary) <= 512),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (
        jsonb_typeof(metadata) = 'object' AND octet_length(metadata::text) <= 8192
    ),
    created_at timestamptz NOT NULL,
    FOREIGN KEY (workspace_id, runtime_run_id)
        REFERENCES runtime_runs (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT runtime_run_events_progress_pair CHECK (
        (progress_current IS NULL AND progress_total IS NULL)
        OR (progress_current IS NOT NULL AND progress_total IS NOT NULL AND progress_current <= progress_total)
    ),
    UNIQUE (workspace_id, runtime_run_id, event_key),
    UNIQUE (workspace_id, runtime_run_id, sequence)
);

CREATE INDEX runtime_run_events_run_sequence_idx
    ON runtime_run_events (workspace_id, runtime_run_id, sequence);

CREATE FUNCTION reject_runtime_run_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'runtime run events are immutable' USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER runtime_run_events_immutable
    BEFORE UPDATE OR DELETE ON runtime_run_events
    FOR EACH ROW EXECUTE FUNCTION reject_runtime_run_event_mutation();

CREATE TABLE audit_event_targets (
    workspace_id uuid NOT NULL,
    audit_event_id uuid NOT NULL,
    object_type text NOT NULL CHECK (object_type ~ '^[a-z][a-z0-9_.-]{1,63}$'),
    object_id text NOT NULL CHECK (object_id <> '' AND length(object_id) <= 128),
    ordinal smallint NOT NULL CHECK (ordinal BETWEEN 0 AND 100),
    PRIMARY KEY (workspace_id, audit_event_id, object_type, object_id),
    FOREIGN KEY (workspace_id, audit_event_id)
        REFERENCES audit_events (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX audit_event_targets_lookup_idx
    ON audit_event_targets (workspace_id, object_type, object_id, audit_event_id);

CREATE TRIGGER audit_event_targets_immutable
    BEFORE UPDATE OR DELETE ON audit_event_targets
    FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();

CREATE FUNCTION project_audit_event_targets(payload jsonb)
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
        ('semantic_query', 'queryId', 100::smallint)
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
          'entity_key', 'join_contract'
      )
      AND payload->>'objectId' <> ''
      AND length(payload->>'objectId') <= 128;
$$;

CREATE FUNCTION project_inserted_audit_event_targets()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO audit_event_targets (workspace_id, audit_event_id, object_type, object_id, ordinal)
    SELECT NEW.workspace_id, NEW.id, target.object_type, target.object_id, target.ordinal
    FROM project_audit_event_targets(NEW.payload) AS target
    ON CONFLICT DO NOTHING;
    RETURN NEW;
END;
$$;

CREATE TRIGGER audit_events_project_targets
    AFTER INSERT ON audit_events
    FOR EACH ROW EXECUTE FUNCTION project_inserted_audit_event_targets();

CREATE TABLE audit_exports (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    runtime_run_id uuid NOT NULL,
    artifact_id text NOT NULL CHECK (artifact_id <> '' AND length(artifact_id) <= 256),
    format text NOT NULL CHECK (format IN ('json')),
    filters jsonb NOT NULL CHECK (jsonb_typeof(filters) = 'object' AND octet_length(filters::text) <= 8192),
    row_count integer NOT NULL CHECK (row_count BETWEEN 0 AND 1000),
    content_digest text NOT NULL CHECK (content_digest ~ '^[0-9a-f]{64}$'),
    request_fingerprint text NOT NULL CHECK (request_fingerprint ~ '^[0-9a-f]{64}$'),
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 256),
    created_by_principal_id uuid NOT NULL,
    content bytea NOT NULL CHECK (octet_length(content) <= 2097152),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    FOREIGN KEY (workspace_id, runtime_run_id)
        REFERENCES runtime_runs (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, created_by_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT audit_exports_expiry CHECK (expires_at > created_at),
    UNIQUE (workspace_id, idempotency_key),
    UNIQUE (workspace_id, artifact_id)
);

CREATE TABLE attention_items (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (kind IN ('review', 'validation', 'source', 'runtime', 'compatibility')),
    dedupe_key text NOT NULL CHECK (dedupe_key <> '' AND length(dedupe_key) <= 256),
    state text NOT NULL CHECK (state IN ('open', 'in_progress', 'resolved', 'dismissed')),
    priority text NOT NULL CHECK (priority IN ('low', 'medium', 'high', 'critical')),
    risk text NOT NULL CHECK (risk IN ('low', 'medium', 'high', 'critical')),
    target_type text NOT NULL CHECK (target_type ~ '^[a-z][a-z0-9_.-]{1,63}$'),
    target_id text NOT NULL CHECK (target_id <> '' AND length(target_id) <= 128),
    target_route text NOT NULL CHECK (target_route LIKE '/%' AND length(target_route) <= 512),
    assignee_principal_id uuid,
    audience_role_id text REFERENCES roles (id) ON DELETE RESTRICT,
    initiator_principal_id uuid,
    rule_version text NOT NULL CHECK (rule_version <> '' AND length(rule_version) <= 64),
    title text NOT NULL CHECK (title <> '' AND length(title) <= 160),
    summary text NOT NULL CHECK (summary <> '' AND length(summary) <= 512),
    reason_code text NOT NULL CHECK (reason_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    evidence_ref text CHECK (evidence_ref IS NULL OR length(evidence_ref) <= 512),
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    opened_at timestamptz NOT NULL,
    due_at timestamptz,
    resolved_at timestamptz,
    updated_at timestamptz NOT NULL,
    version bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    FOREIGN KEY (workspace_id, assignee_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, initiator_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    UNIQUE (workspace_id, dedupe_key)
);

CREATE INDEX attention_items_workspace_state_idx
    ON attention_items (workspace_id, state, priority, updated_at DESC, id DESC);

CREATE FUNCTION operations_backfill_uuid(seed text)
RETURNS uuid
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT (
        substr(md5(seed), 1, 8) || '-' || substr(md5(seed), 9, 4) || '-7' ||
        substr(md5(seed), 14, 3) || '-a' || substr(md5(seed), 18, 3) || '-' ||
        substr(md5(seed), 21, 12)
    )::uuid;
$$;

INSERT INTO runtime_runs (
    id, workspace_id, kind, source_type, source_id, source_version_digest, job_id,
    trace_id, idempotency_key, state, phase, attempt, max_attempts, started_at, finished_at,
    error_code, error_summary, version, created_at, updated_at
)
SELECT operations_backfill_uuid('runtime:discovery:' || run.id::text), run.workspace_id,
    'discovery', 'discovery_run', run.id::text,
    encode(sha256(convert_to(concat_ws('|', run.source_connection_id::text,
        COALESCE(run.source_revision_id::text, ''), COALESCE(run.credential_version::text, ''), run.adapter_version), 'UTF8')), 'hex'),
    run.job_id, NULLIF(run.trace_id, ''), 'runtime:discovery:' || run.id::text,
    run.status, run.status, COALESCE(job.attempt, 0), COALESCE(job.max_attempts, 1), run.started_at,
    CASE WHEN run.status IN ('succeeded', 'degraded', 'failed', 'cancelled') THEN run.completed_at END,
    CASE WHEN run.status = 'failed' THEN run.error_code END,
    CASE WHEN run.status = 'failed' THEN 'discovery run failed' END,
    1, run.created_at, run.updated_at
FROM discovery_runs AS run
LEFT JOIN jobs AS job ON job.workspace_id = run.workspace_id AND job.id = run.job_id;

INSERT INTO runtime_runs (
    id, workspace_id, kind, source_type, source_id, source_version_digest, idempotency_key,
    state, phase, attempt, max_attempts, started_at, finished_at, error_code, error_summary,
    version, created_at, updated_at
)
SELECT operations_backfill_uuid('runtime:validation:' || run.id::text), run.workspace_id,
    'validation', 'validation_run', run.id::text,
    encode(sha256(convert_to(concat_ws('|', run.proposal_id::text, run.validator_id, run.validator_version), 'UTF8')), 'hex'),
    'runtime:validation:' || run.id::text, run.status, run.status, 0, 1, run.started_at, run.finished_at,
    CASE WHEN run.status = 'failed' THEN 'VALIDATION_FAILED' END,
    CASE WHEN run.status = 'failed' THEN 'validation run failed' END,
    1, run.started_at, COALESCE(run.finished_at, run.started_at)
FROM validation_runs AS run;

INSERT INTO runtime_runs (
    id, workspace_id, kind, source_type, source_id, source_version_digest,
    idempotency_key, requested_by_principal_id, state, phase, attempt, max_attempts,
    started_at, finished_at, error_code, error_summary, version, created_at, updated_at
)
SELECT operations_backfill_uuid('runtime:agent:' || run.id::text), run.workspace_id,
    'agent', 'agent_run', run.id::text, replace(run.input_hash, 'sha256:', ''),
    'runtime:agent:' || run.id::text, run.principal_id, run.status, run.status, 0, 1,
    run.started_at, run.finished_at,
    CASE WHEN run.status = 'failed' THEN 'AGENT_FAILED' END,
    CASE WHEN run.status = 'failed' THEN 'agent run failed' END,
    1, run.created_at, COALESCE(run.finished_at, run.created_at)
FROM agent_runs AS run;

INSERT INTO runtime_runs (
    id, workspace_id, kind, source_type, source_id, source_version_digest,
    trace_id, idempotency_key, state, phase, attempt, max_attempts, started_at,
    finished_at, version, created_at, updated_at
)
SELECT operations_backfill_uuid('runtime:semantic-resolution:' || query.id::text), query.workspace_id,
    'semantic_resolution', 'semantic_query', query.id::text, replace(query.request_digest, 'sha256:', ''),
    query.trace_id, 'runtime:semantic-resolution:' || query.id::text,
    'succeeded', 'finalized', 0, 1, query.created_at, query.finalized_at,
    1, query.created_at, query.finalized_at
FROM semantic_queries AS query;

INSERT INTO runtime_run_events (
    id, workspace_id, runtime_run_id, event_key, sequence, event_type, phase,
    state, error_code, summary, metadata, created_at
)
SELECT operations_backfill_uuid('runtime-event:' || run.kind || ':' || run.source_type || ':' || run.source_id),
    run.workspace_id, run.id, 'backfill:' || run.state, 1, 'state', run.phase,
    run.state, run.error_code,
    CASE WHEN run.state IN ('failed', 'dead_letter') THEN run.error_summary END,
    '{}'::jsonb, run.updated_at
FROM runtime_runs AS run
WHERE run.kind IN ('discovery', 'validation', 'agent', 'semantic_resolution');

INSERT INTO audit_event_targets (workspace_id, audit_event_id, object_type, object_id, ordinal)
SELECT event.workspace_id, event.id, target.object_type, target.object_id, target.ordinal
FROM audit_events AS event
CROSS JOIN LATERAL project_audit_event_targets(event.payload) AS target
ON CONFLICT DO NOTHING;

CREATE INDEX audit_events_workspace_actor_created_idx
    ON audit_events (workspace_id, actor_id, created_at DESC, id DESC);
CREATE INDEX audit_events_workspace_type_created_idx
    ON audit_events (workspace_id, event_type, created_at DESC, id DESC);
CREATE INDEX audit_events_workspace_trace_created_idx
    ON audit_events (workspace_id, trace_id, created_at DESC, id DESC);
DROP FUNCTION operations_backfill_uuid(text);

COMMIT;

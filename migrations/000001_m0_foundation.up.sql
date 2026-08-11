CREATE TABLE workspaces (
    id text PRIMARY KEY CHECK (id <> ''),
    slug text NOT NULL UNIQUE CHECK (slug <> ''),
    display_name text NOT NULL CHECK (display_name <> ''),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE audit_events (
    id text PRIMARY KEY CHECK (id <> ''),
    workspace_id text NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    event_type text NOT NULL CHECK (event_type <> ''),
    actor_id text,
    payload jsonb NOT NULL,
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX audit_events_workspace_created_idx
    ON audit_events (workspace_id, created_at DESC, id);

CREATE FUNCTION reject_audit_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit events are immutable' USING ERRCODE = '55000';
END;
$$;

CREATE TRIGGER audit_events_immutable
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();

CREATE TABLE jobs (
    id text PRIMARY KEY CHECK (id <> ''),
    workspace_id text NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    job_type text NOT NULL CHECK (job_type <> ''),
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'retryable', 'succeeded', 'dead_letter')),
    attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts integer NOT NULL CHECK (max_attempts >= 1),
    available_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    leased_until timestamptz,
    lease_owner text,
    idempotency_key text NOT NULL CHECK (idempotency_key <> ''),
    last_error_code text CHECK (last_error_code IS NULL OR last_error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at timestamptz,
    CONSTRAINT jobs_attempt_bound CHECK (attempt <= max_attempts),
    CONSTRAINT jobs_lease_state CHECK (
        (status = 'running' AND leased_until IS NOT NULL AND lease_owner IS NOT NULL)
        OR (status <> 'running' AND leased_until IS NULL AND lease_owner IS NULL)
    ),
    CONSTRAINT jobs_completion_state CHECK (
        (status IN ('succeeded', 'dead_letter') AND completed_at IS NOT NULL)
        OR (status NOT IN ('succeeded', 'dead_letter') AND completed_at IS NULL)
    ),
    UNIQUE (workspace_id, idempotency_key)
);

CREATE INDEX jobs_claimable_idx
    ON jobs (available_at, created_at, id)
    WHERE status IN ('queued', 'retryable');

CREATE INDEX jobs_expired_lease_idx
    ON jobs (leased_until, id)
    WHERE status = 'running';

CREATE INDEX jobs_workspace_status_updated_idx
    ON jobs (workspace_id, status, updated_at DESC, id);

CREATE TABLE outbox_events (
    id text PRIMARY KEY CHECK (id <> ''),
    workspace_id text NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    event_type text NOT NULL CHECK (event_type <> ''),
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'publishing', 'retryable', 'published', 'dead_letter')),
    attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts integer NOT NULL CHECK (max_attempts >= 1),
    available_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    leased_until timestamptz,
    lease_owner text,
    last_error_code text CHECK (last_error_code IS NULL OR last_error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at timestamptz,
    CONSTRAINT outbox_events_attempt_bound CHECK (attempt <= max_attempts),
    CONSTRAINT outbox_events_lease_state CHECK (
        (status = 'publishing' AND leased_until IS NOT NULL AND lease_owner IS NOT NULL)
        OR (status <> 'publishing' AND leased_until IS NULL AND lease_owner IS NULL)
    ),
    CONSTRAINT outbox_events_publication_state CHECK (
        (status = 'published' AND published_at IS NOT NULL)
        OR (status <> 'published' AND published_at IS NULL)
    )
);

CREATE INDEX outbox_events_claimable_idx
    ON outbox_events (available_at, created_at, id)
    WHERE status IN ('queued', 'retryable');

CREATE INDEX outbox_events_expired_lease_idx
    ON outbox_events (leased_until, id)
    WHERE status = 'publishing';

CREATE INDEX outbox_events_workspace_status_updated_idx
    ON outbox_events (workspace_id, status, updated_at DESC, id);

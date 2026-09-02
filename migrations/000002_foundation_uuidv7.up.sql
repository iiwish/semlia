BEGIN;

CREATE FUNCTION semlia_legacy_uuidv7(resource_kind text, legacy_value text, created_value timestamptz)
RETURNS uuid
LANGUAGE plpgsql
IMMUTABLE
STRICT
AS $$
DECLARE
    millis bigint;
    timestamp_hex text;
    entropy text;
BEGIN
    millis := floor(extract(epoch FROM created_value) * 1000)::bigint;
    IF millis < 0 OR millis > 281474976710655 THEN
        RAISE EXCEPTION 'legacy timestamp is outside UUIDv7 range' USING ERRCODE = '22008';
    END IF;
    timestamp_hex := lpad(to_hex(millis), 12, '0');
    entropy := md5(resource_kind || chr(31) || legacy_value);
    RETURN (
        substr(timestamp_hex, 1, 8) || '-' ||
        substr(timestamp_hex, 9, 4) || '-7' ||
        substr(entropy, 1, 3) || '-8' ||
        substr(entropy, 4, 3) || '-' ||
        substr(entropy, 7, 12)
    )::uuid;
END;
$$;

DROP TRIGGER audit_events_immutable ON audit_events;

ALTER TABLE audit_events DROP CONSTRAINT audit_events_workspace_id_fkey;
ALTER TABLE jobs DROP CONSTRAINT jobs_workspace_id_fkey;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_workspace_id_fkey;

ALTER TABLE workspaces DROP CONSTRAINT workspaces_pkey;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_pkey;
ALTER TABLE jobs DROP CONSTRAINT jobs_pkey;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_pkey;
ALTER TABLE jobs DROP CONSTRAINT jobs_workspace_id_idempotency_key_key;

DROP INDEX audit_events_workspace_created_idx;
DROP INDEX jobs_claimable_idx;
DROP INDEX jobs_expired_lease_idx;
DROP INDEX jobs_workspace_status_updated_idx;
DROP INDEX outbox_events_claimable_idx;
DROP INDEX outbox_events_expired_lease_idx;
DROP INDEX outbox_events_workspace_status_updated_idx;

ALTER TABLE workspaces RENAME COLUMN id TO legacy_id;
ALTER TABLE audit_events RENAME COLUMN id TO legacy_id;
ALTER TABLE audit_events RENAME COLUMN workspace_id TO legacy_workspace_id;
ALTER TABLE jobs RENAME COLUMN id TO legacy_id;
ALTER TABLE jobs RENAME COLUMN workspace_id TO legacy_workspace_id;
ALTER TABLE outbox_events RENAME COLUMN id TO legacy_id;
ALTER TABLE outbox_events RENAME COLUMN workspace_id TO legacy_workspace_id;

ALTER TABLE workspaces ADD COLUMN id uuid;
ALTER TABLE audit_events ADD COLUMN id uuid;
ALTER TABLE audit_events ADD COLUMN workspace_id uuid;
ALTER TABLE jobs ADD COLUMN id uuid;
ALTER TABLE jobs ADD COLUMN workspace_id uuid;
ALTER TABLE outbox_events ADD COLUMN id uuid;
ALTER TABLE outbox_events ADD COLUMN workspace_id uuid;

UPDATE workspaces
SET id = semlia_legacy_uuidv7('workspace', legacy_id, created_at);

UPDATE audit_events event
SET id = semlia_legacy_uuidv7('event', event.legacy_id, event.created_at),
    workspace_id = workspace.id
FROM workspaces workspace
WHERE workspace.legacy_id = event.legacy_workspace_id;

UPDATE jobs job
SET id = semlia_legacy_uuidv7('run', job.legacy_id, job.created_at),
    workspace_id = workspace.id
FROM workspaces workspace
WHERE workspace.legacy_id = job.legacy_workspace_id;

UPDATE outbox_events event
SET id = semlia_legacy_uuidv7('event', event.legacy_id, event.created_at),
    workspace_id = workspace.id
FROM workspaces workspace
WHERE workspace.legacy_id = event.legacy_workspace_id;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM workspaces WHERE id IS NULL)
       OR EXISTS (SELECT 1 FROM audit_events WHERE id IS NULL OR workspace_id IS NULL)
       OR EXISTS (SELECT 1 FROM jobs WHERE id IS NULL OR workspace_id IS NULL)
       OR EXISTS (SELECT 1 FROM outbox_events WHERE id IS NULL OR workspace_id IS NULL) THEN
        RAISE EXCEPTION 'foundation UUIDv7 backfill left null identities' USING ERRCODE = '23502';
    END IF;
END;
$$;

ALTER TABLE workspaces ALTER COLUMN legacy_id DROP NOT NULL;
ALTER TABLE audit_events ALTER COLUMN legacy_id DROP NOT NULL;
ALTER TABLE audit_events ALTER COLUMN legacy_workspace_id DROP NOT NULL;
ALTER TABLE jobs ALTER COLUMN legacy_id DROP NOT NULL;
ALTER TABLE jobs ALTER COLUMN legacy_workspace_id DROP NOT NULL;
ALTER TABLE outbox_events ALTER COLUMN legacy_id DROP NOT NULL;
ALTER TABLE outbox_events ALTER COLUMN legacy_workspace_id DROP NOT NULL;

ALTER TABLE workspaces ALTER COLUMN id SET NOT NULL;
ALTER TABLE audit_events ALTER COLUMN id SET NOT NULL;
ALTER TABLE audit_events ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE jobs ALTER COLUMN id SET NOT NULL;
ALTER TABLE jobs ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE outbox_events ALTER COLUMN id SET NOT NULL;
ALTER TABLE outbox_events ALTER COLUMN workspace_id SET NOT NULL;

ALTER TABLE workspaces
    ADD CONSTRAINT workspaces_pkey PRIMARY KEY (id),
    ADD CONSTRAINT workspaces_legacy_id_key UNIQUE (legacy_id),
    ADD CONSTRAINT workspaces_id_uuidv7 CHECK (substring(id::text, 15, 1) = '7');

ALTER TABLE audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id),
    ADD CONSTRAINT audit_events_legacy_id_key UNIQUE (legacy_id),
    ADD CONSTRAINT audit_events_id_uuidv7 CHECK (substring(id::text, 15, 1) = '7'),
    ADD CONSTRAINT audit_events_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE RESTRICT;

ALTER TABLE jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (id),
    ADD CONSTRAINT jobs_legacy_id_key UNIQUE (legacy_id),
    ADD CONSTRAINT jobs_id_uuidv7 CHECK (substring(id::text, 15, 1) = '7'),
    ADD CONSTRAINT jobs_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE RESTRICT,
    ADD CONSTRAINT jobs_workspace_id_idempotency_key_key UNIQUE (workspace_id, idempotency_key);

ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_pkey PRIMARY KEY (id),
    ADD CONSTRAINT outbox_events_legacy_id_key UNIQUE (legacy_id),
    ADD CONSTRAINT outbox_events_id_uuidv7 CHECK (substring(id::text, 15, 1) = '7'),
    ADD CONSTRAINT outbox_events_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE RESTRICT;

CREATE INDEX audit_events_workspace_created_idx
    ON audit_events (workspace_id, created_at DESC, id);

CREATE INDEX jobs_claimable_idx
    ON jobs (available_at, created_at, id)
    WHERE status IN ('queued', 'retryable');

CREATE INDEX jobs_expired_lease_idx
    ON jobs (leased_until, id)
    WHERE status = 'running';

CREATE INDEX jobs_workspace_status_updated_idx
    ON jobs (workspace_id, status, updated_at DESC, id);

CREATE INDEX outbox_events_claimable_idx
    ON outbox_events (available_at, created_at, id)
    WHERE status IN ('queued', 'retryable');

CREATE INDEX outbox_events_expired_lease_idx
    ON outbox_events (leased_until, id)
    WHERE status = 'publishing';

CREATE INDEX outbox_events_workspace_status_updated_idx
    ON outbox_events (workspace_id, status, updated_at DESC, id);

CREATE TRIGGER audit_events_immutable
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();

DROP FUNCTION semlia_legacy_uuidv7(text, text, timestamptz);

COMMIT;

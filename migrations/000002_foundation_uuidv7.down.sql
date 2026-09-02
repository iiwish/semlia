BEGIN;

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

UPDATE workspaces SET legacy_id = COALESCE(legacy_id, id::text);

UPDATE audit_events event
SET legacy_id = COALESCE(event.legacy_id, event.id::text),
    legacy_workspace_id = COALESCE(event.legacy_workspace_id, workspace.legacy_id)
FROM workspaces workspace
WHERE workspace.id = event.workspace_id;

UPDATE jobs job
SET legacy_id = COALESCE(job.legacy_id, job.id::text),
    legacy_workspace_id = COALESCE(job.legacy_workspace_id, workspace.legacy_id)
FROM workspaces workspace
WHERE workspace.id = job.workspace_id;

UPDATE outbox_events event
SET legacy_id = COALESCE(event.legacy_id, event.id::text),
    legacy_workspace_id = COALESCE(event.legacy_workspace_id, workspace.legacy_id)
FROM workspaces workspace
WHERE workspace.id = event.workspace_id;

ALTER TABLE workspaces DROP CONSTRAINT workspaces_legacy_id_key;
ALTER TABLE workspaces DROP CONSTRAINT workspaces_id_uuidv7;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_legacy_id_key;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_id_uuidv7;
ALTER TABLE jobs DROP CONSTRAINT jobs_legacy_id_key;
ALTER TABLE jobs DROP CONSTRAINT jobs_id_uuidv7;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_legacy_id_key;
ALTER TABLE outbox_events DROP CONSTRAINT outbox_events_id_uuidv7;

ALTER TABLE workspaces DROP COLUMN id;
ALTER TABLE audit_events DROP COLUMN id;
ALTER TABLE audit_events DROP COLUMN workspace_id;
ALTER TABLE jobs DROP COLUMN id;
ALTER TABLE jobs DROP COLUMN workspace_id;
ALTER TABLE outbox_events DROP COLUMN id;
ALTER TABLE outbox_events DROP COLUMN workspace_id;

ALTER TABLE workspaces RENAME COLUMN legacy_id TO id;
ALTER TABLE audit_events RENAME COLUMN legacy_id TO id;
ALTER TABLE audit_events RENAME COLUMN legacy_workspace_id TO workspace_id;
ALTER TABLE jobs RENAME COLUMN legacy_id TO id;
ALTER TABLE jobs RENAME COLUMN legacy_workspace_id TO workspace_id;
ALTER TABLE outbox_events RENAME COLUMN legacy_id TO id;
ALTER TABLE outbox_events RENAME COLUMN legacy_workspace_id TO workspace_id;

ALTER TABLE workspaces ALTER COLUMN id SET NOT NULL;
ALTER TABLE audit_events ALTER COLUMN id SET NOT NULL;
ALTER TABLE audit_events ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE jobs ALTER COLUMN id SET NOT NULL;
ALTER TABLE jobs ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE outbox_events ALTER COLUMN id SET NOT NULL;
ALTER TABLE outbox_events ALTER COLUMN workspace_id SET NOT NULL;

ALTER TABLE workspaces ADD CONSTRAINT workspaces_pkey PRIMARY KEY (id);
ALTER TABLE audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id),
    ADD CONSTRAINT audit_events_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE RESTRICT;
ALTER TABLE jobs
    ADD CONSTRAINT jobs_pkey PRIMARY KEY (id),
    ADD CONSTRAINT jobs_workspace_id_fkey FOREIGN KEY (workspace_id) REFERENCES workspaces (id) ON DELETE RESTRICT,
    ADD CONSTRAINT jobs_workspace_id_idempotency_key_key UNIQUE (workspace_id, idempotency_key);
ALTER TABLE outbox_events
    ADD CONSTRAINT outbox_events_pkey PRIMARY KEY (id),
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

COMMIT;

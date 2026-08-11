DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS jobs;
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS workspaces;
DROP FUNCTION IF EXISTS reject_audit_event_mutation();

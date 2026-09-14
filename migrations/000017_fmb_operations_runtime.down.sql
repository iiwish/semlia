BEGIN;

DROP INDEX IF EXISTS audit_events_workspace_trace_created_idx;
DROP INDEX IF EXISTS audit_events_workspace_type_created_idx;
DROP INDEX IF EXISTS audit_events_workspace_actor_created_idx;
DROP INDEX IF EXISTS attention_items_workspace_state_idx;
DROP TABLE IF EXISTS attention_items;
DROP TABLE IF EXISTS audit_exports;
DROP TRIGGER IF EXISTS audit_events_project_targets ON audit_events;
DROP FUNCTION IF EXISTS project_inserted_audit_event_targets();
DROP FUNCTION IF EXISTS project_audit_event_targets(jsonb);
DROP TRIGGER IF EXISTS audit_event_targets_immutable ON audit_event_targets;
DROP INDEX IF EXISTS audit_event_targets_lookup_idx;
DROP TABLE IF EXISTS audit_event_targets;
DROP TRIGGER IF EXISTS runtime_run_events_immutable ON runtime_run_events;
DROP FUNCTION IF EXISTS reject_runtime_run_event_mutation();
DROP INDEX IF EXISTS runtime_run_events_run_sequence_idx;
DROP TABLE IF EXISTS runtime_run_events;
DROP INDEX IF EXISTS runtime_runs_workspace_trace_idx;
DROP INDEX IF EXISTS runtime_runs_workspace_kind_idx;
DROP INDEX IF EXISTS runtime_runs_workspace_state_idx;
DROP INDEX IF EXISTS runtime_runs_workspace_updated_idx;
DROP INDEX IF EXISTS runtime_runs_workspace_job_idx;
DROP TABLE IF EXISTS runtime_runs;
DROP TABLE IF EXISTS runtime_settings;
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_workspace_identity;
ALTER TABLE audit_events DROP CONSTRAINT IF EXISTS audit_events_workspace_identity;

COMMIT;

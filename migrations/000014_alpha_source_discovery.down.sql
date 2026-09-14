BEGIN;

DROP TRIGGER IF EXISTS semantic_candidate_decisions_immutable ON semantic_candidate_decisions;
DROP TABLE IF EXISTS semantic_candidate_decisions;
DROP TABLE IF EXISTS semantic_candidates;
DROP TABLE IF EXISTS join_observations;
DROP TABLE IF EXISTS physical_key_observations;

DROP INDEX IF EXISTS discovery_runs_active_source_idx;
DROP INDEX IF EXISTS discovery_runs_success_projection_key;
ALTER TABLE discovery_runs
    DROP CONSTRAINT IF EXISTS discovery_runs_job_unique,
    DROP CONSTRAINT IF EXISTS discovery_runs_job_fkey,
    DROP CONSTRAINT IF EXISTS discovery_runs_credential_fkey,
    DROP CONSTRAINT IF EXISTS discovery_runs_error_state,
    DROP CONSTRAINT IF EXISTS discovery_runs_time_state,
    DROP CONSTRAINT IF EXISTS discovery_runs_status_check,
    DROP COLUMN IF EXISTS trace_id,
    DROP COLUMN IF EXISTS requested_by,
    DROP COLUMN IF EXISTS job_id,
    DROP COLUMN IF EXISTS credential_version,
    ADD CONSTRAINT discovery_runs_status_check
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    ADD CONSTRAINT discovery_runs_time_state CHECK (
        (status = 'queued' AND started_at IS NULL AND completed_at IS NULL)
        OR (status = 'running' AND started_at IS NOT NULL AND completed_at IS NULL)
        OR (status IN ('succeeded', 'failed', 'cancelled') AND started_at IS NOT NULL AND completed_at IS NOT NULL)
    ),
    ADD CONSTRAINT discovery_runs_error_state CHECK (
        (status = 'failed' AND error_code IS NOT NULL) OR (status <> 'failed' AND error_code IS NULL)
    );
CREATE UNIQUE INDEX discovery_runs_success_projection_key
    ON discovery_runs (source_revision_id, adapter_version) WHERE status = 'succeeded';

ALTER TABLE source_connections DROP CONSTRAINT IF EXISTS source_connections_active_credential_fkey;
DROP TABLE IF EXISTS source_credentials;
ALTER TABLE source_connections
    DROP COLUMN IF EXISTS disabled_at,
    DROP COLUMN IF EXISTS artifact_paths,
    DROP COLUMN IF EXISTS active_credential_version;

COMMIT;

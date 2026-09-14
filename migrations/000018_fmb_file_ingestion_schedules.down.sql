BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM discovery_runs WHERE projection_reused) THEN
        RAISE EXCEPTION 'cannot roll back ingestion migration while reused discovery projection history exists'
            USING ERRCODE = '55000';
    END IF;
END;
$$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM source_connections WHERE source_kind <> 'postgresql')
       OR EXISTS (SELECT 1 FROM source_artifacts)
       OR EXISTS (SELECT 1 FROM source_schedules) THEN
        RAISE EXCEPTION 'cannot roll back ingestion migration while version 18 artifact or schedule data exists'
            USING ERRCODE = '55000';
    END IF;
END;
$$;

DROP INDEX discovery_runs_success_projection_key;
CREATE UNIQUE INDEX discovery_runs_success_projection_key
    ON discovery_runs (source_revision_id, adapter_version)
    WHERE status IN ('succeeded', 'degraded');

DROP TRIGGER IF EXISTS audit_event_targets_immutable ON audit_event_targets;
DELETE FROM audit_event_targets
WHERE object_type IN ('artifact', 'artifact_set', 'source_schedule', 'schedule_occurrence');

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

CREATE TRIGGER audit_event_targets_immutable
    BEFORE UPDATE OR DELETE ON audit_event_targets
    FOR EACH ROW EXECUTE FUNCTION reject_audit_event_mutation();

ALTER TABLE source_schedule_command_receipts
    DROP CONSTRAINT IF EXISTS source_schedule_command_receipts_occurrence_fkey;
DROP TRIGGER IF EXISTS source_schedule_occurrences_pin_guard ON source_schedule_occurrences;
DROP FUNCTION IF EXISTS validate_source_schedule_occurrence_pins();
DROP TRIGGER IF EXISTS source_schedule_occurrences_immutable ON source_schedule_occurrences;
DROP TABLE IF EXISTS source_schedule_occurrences;
DROP TRIGGER IF EXISTS source_schedule_command_receipts_immutable ON source_schedule_command_receipts;
DROP TABLE IF EXISTS source_schedule_command_receipts;
DROP TABLE IF EXISTS source_schedules;
DROP TRIGGER IF EXISTS source_revision_artifacts_immutable ON source_revision_artifacts;
DROP TABLE IF EXISTS source_revision_artifacts;
DROP TRIGGER IF EXISTS source_revision_artifact_sets_immutable ON source_revision_artifact_sets;
DROP TABLE IF EXISTS source_revision_artifact_sets;
DROP TRIGGER IF EXISTS discovery_run_artifacts_immutable ON discovery_run_artifacts;
DROP TABLE IF EXISTS discovery_run_artifacts;
DROP TRIGGER IF EXISTS discovery_runs_input_immutable ON discovery_runs;
DROP FUNCTION IF EXISTS reject_discovery_run_input_mutation();
ALTER TABLE discovery_runs
    DROP COLUMN projection_reused,
    DROP CONSTRAINT IF EXISTS discovery_runs_artifact_identity,
    DROP CONSTRAINT IF EXISTS discovery_runs_workspace_source_identity,
    DROP CONSTRAINT IF EXISTS discovery_runs_workspace_job_identity,
    DROP CONSTRAINT IF EXISTS discovery_runs_artifact_set_fkey,
    DROP COLUMN IF EXISTS request_fingerprint,
    DROP COLUMN IF EXISTS source_input_fingerprint,
    DROP COLUMN IF EXISTS source_config,
    DROP COLUMN IF EXISTS artifact_set_id;
ALTER TABLE source_connections DROP CONSTRAINT IF EXISTS source_connections_active_artifact_set_fkey;
DROP TRIGGER IF EXISTS source_artifact_set_members_immutable ON source_artifact_set_members;
DROP TABLE IF EXISTS source_artifact_set_members;
DROP TRIGGER IF EXISTS source_artifact_sets_immutable ON source_artifact_sets;
DROP TABLE IF EXISTS source_artifact_sets;
DROP TRIGGER IF EXISTS source_artifacts_transition_guard ON source_artifacts;
ALTER TABLE semantic_candidate_decisions DROP COLUMN IF EXISTS request_fingerprint;
DROP INDEX IF EXISTS semantic_candidates_workspace_status_page_idx;
DROP INDEX IF EXISTS source_connections_workspace_page_idx;
DROP INDEX IF EXISTS discovery_runs_source_page_idx;
DROP TABLE IF EXISTS source_artifacts;
DROP FUNCTION IF EXISTS enforce_source_artifact_transition();
DROP TABLE IF EXISTS artifact_object_retention;
DROP TRIGGER IF EXISTS artifact_objects_immutable ON artifact_objects;
DROP TABLE IF EXISTS artifact_objects;
DROP TABLE IF EXISTS workspace_artifact_usage;

ALTER TABLE source_connections
    DROP CONSTRAINT IF EXISTS source_connections_kind_credential_shape,
    DROP CONSTRAINT IF EXISTS source_connections_workspace_kind_identity,
    DROP COLUMN IF EXISTS active_artifact_set_id,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS source_kind;

ALTER TABLE source_revisions
    DROP CONSTRAINT IF EXISTS source_revisions_workspace_source_identity;

COMMIT;

BEGIN;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM source_snapshots)
        OR EXISTS(SELECT 1 FROM source_snapshot_runs)
        OR EXISTS(SELECT 1 FROM source_snapshot_scope)
        OR EXISTS(SELECT 1 FROM source_snapshot_members)
        OR EXISTS(SELECT 1 FROM source_snapshot_diagnostics)
        OR EXISTS(SELECT 1 FROM source_code_revisions)
        OR EXISTS(SELECT 1 FROM source_lineage_revisions)
        OR EXISTS(SELECT 1 FROM source_coverage_heads)
        OR EXISTS(SELECT 1 FROM source_effective_snapshots) THEN
        RAISE EXCEPTION 'DOWN_MIGRATION_UNSAFE: source history cannot be represented by schema 21; downgrade refused' USING ERRCODE='55000';
    END IF;
END; $$;
DROP TRIGGER artifact_object_retention_source_history ON artifact_object_retention;
DROP TABLE source_coverage_heads,source_effective_snapshots,source_snapshot_diagnostics,source_snapshot_members,source_lineage_revisions,source_code_revisions,source_snapshot_scope,source_snapshot_runs,source_snapshots;
DROP FUNCTION reject_sealed_snapshot_insert(),validate_source_snapshot_head(),validate_source_snapshot_member(),validate_source_snapshot_seal(),protect_source_snapshot_artifact_bytes();
ALTER TABLE lineage_edges DROP CONSTRAINT lineage_edges_snapshot_identity_key;
ALTER TABLE code_artifacts DROP CONSTRAINT code_artifacts_snapshot_identity_key;
ALTER TABLE physical_field_revisions DROP CONSTRAINT physical_field_revisions_snapshot_identity_key;
ALTER TABLE physical_dataset_revisions DROP CONSTRAINT physical_dataset_revisions_snapshot_identity_key;
ALTER TABLE discovery_runs DROP CONSTRAINT discovery_runs_source_identity_key;
ALTER TABLE source_revisions DROP CONSTRAINT source_revisions_source_identity_key;
COMMIT;

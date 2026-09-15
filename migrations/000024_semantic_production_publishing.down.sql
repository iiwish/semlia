BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM production_validation_attempts LIMIT 1)
        OR EXISTS (SELECT 1 FROM production_validation_bindings LIMIT 1)
        OR EXISTS (SELECT 1 FROM production_review_bindings LIMIT 1)
        OR EXISTS (SELECT 1 FROM release_proposals LIMIT 1)
        OR EXISTS (SELECT 1 FROM production_release_manifests LIMIT 1)
        OR EXISTS (SELECT 1 FROM production_release_before_pins LIMIT 1)
        OR EXISTS (SELECT 1 FROM production_release_binding_inputs LIMIT 1)
        OR EXISTS (SELECT 1 FROM releases WHERE production_root_release_id IS NOT NULL LIMIT 1)
        OR EXISTS (SELECT 1 FROM validation_runs WHERE production_operation_id IS NOT NULL LIMIT 1)
    THEN
        RAISE EXCEPTION 'DOWN_MIGRATION_UNSAFE: semantic production publishing data exists';
    END IF;
END $$;

DROP TABLE IF EXISTS production_release_binding_inputs;
DROP TABLE IF EXISTS production_release_before_pins;
DROP TABLE IF EXISTS production_release_manifests;
DROP TABLE IF EXISTS release_proposals;

ALTER TABLE releases
    DROP CONSTRAINT IF EXISTS releases_production_protection_shape,
    DROP COLUMN IF EXISTS production_rollback_depth,
    DROP COLUMN IF EXISTS production_rollback_parent_id,
    DROP COLUMN IF EXISTS production_root_release_id;

DROP TABLE IF EXISTS production_review_bindings;
DROP TABLE IF EXISTS production_validation_bindings;

DROP INDEX IF EXISTS validation_runs_production_unique_idx;
DROP INDEX IF EXISTS validation_runs_legacy_unique_idx;

ALTER TABLE validation_runs
    DROP CONSTRAINT IF EXISTS validation_runs_production_shape,
    DROP CONSTRAINT IF EXISTS validation_runs_production_attempt_fkey,
    DROP COLUMN IF EXISTS production_attempt_no,
    DROP COLUMN IF EXISTS production_version,
    DROP COLUMN IF EXISTS production_operation_id,
    ADD CONSTRAINT validation_runs_proposal_id_validator_id_validator_version_key
        UNIQUE (proposal_id, validator_id, validator_version);

ALTER TABLE production_versions
    DROP CONSTRAINT IF EXISTS production_versions_active_attempt_fkey,
    DROP COLUMN IF EXISTS active_validation_attempt_no;

DROP TABLE IF EXISTS production_validation_attempts;

COMMIT;

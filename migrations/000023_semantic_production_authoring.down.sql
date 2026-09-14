BEGIN;

DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM production_operations)
        OR EXISTS(SELECT 1 FROM production_commands)
        OR EXISTS(SELECT 1 FROM production_request_claims)
        OR EXISTS(SELECT 1 FROM production_identity_reservations)
        OR EXISTS(SELECT 1 FROM proposals WHERE intent = 'create' OR production_operation_id IS NOT NULL) THEN
        RAISE EXCEPTION 'DOWN_MIGRATION_UNSAFE: production authoring history cannot be represented by schema 22; downgrade refused' USING ERRCODE='55000';
    END IF;
END; $$;

ALTER TABLE proposals DROP CONSTRAINT IF EXISTS proposals_production_operation_fkey;
ALTER TABLE proposals DROP CONSTRAINT IF EXISTS proposals_production_pair_check;
ALTER TABLE proposals DROP CONSTRAINT IF EXISTS proposals_reintroduction_check;
ALTER TABLE proposals DROP CONSTRAINT IF EXISTS proposals_target_shape;
ALTER TABLE proposals ADD CONSTRAINT proposals_target_shape CHECK (
    (
        target_object_type = 'semantic_asset'
        AND asset_id IS NOT NULL AND asset_id = target_object_id AND base_revision_id IS NOT NULL
    ) OR (
        target_object_type <> 'semantic_asset' AND asset_id IS NULL AND base_revision_id IS NULL
    )
);

ALTER TABLE proposals
    DROP COLUMN IF EXISTS reintroduction_absence_release_id,
    DROP COLUMN IF EXISTS reintroduction_creation_release_id,
    DROP COLUMN IF EXISTS production_version,
    DROP COLUMN IF EXISTS production_operation_id,
    DROP COLUMN IF EXISTS base_object_version,
    DROP COLUMN IF EXISTS creation_content,
    DROP COLUMN IF EXISTS intent;

DROP TABLE IF EXISTS production_generation_applications;
DROP TABLE IF EXISTS production_generation_outputs;
DROP TABLE IF EXISTS production_generation_links;
DROP TABLE IF EXISTS production_request_claims;
DROP TABLE IF EXISTS production_commands;
DROP TABLE IF EXISTS production_identity_reservations;
DROP TABLE IF EXISTS production_contributors;
DROP TABLE IF EXISTS production_candidate_links;
DROP TABLE IF EXISTS production_targets;
DROP TABLE IF EXISTS production_versions;
DROP TABLE IF EXISTS production_operations;

COMMIT;

BEGIN;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM production_operations)
       OR EXISTS(SELECT 1 FROM semantic_candidate_decisions WHERE previous_decision_id IS NOT NULL) THEN
        RAISE EXCEPTION 'DOWN_MIGRATION_UNSAFE: production authority exists' USING ERRCODE='23514';
    END IF;
END $$;

DROP TRIGGER production_asset_write_guard ON semantic_assets;
DROP TRIGGER production_revision_write_guard ON asset_revisions;
DROP FUNCTION guard_production_catalog_write();
DROP TRIGGER production_proposal_transition_guard ON proposals;
DROP FUNCTION guard_production_member_transition();
DROP TRIGGER production_proposal_reservation_guard ON proposals;
DROP FUNCTION guard_production_reserved_proposal();

DROP TRIGGER production_proposal_changes_guard ON proposal_changes;
DROP FUNCTION guard_production_change_append();
DROP TRIGGER production_proposal_content_guard ON proposals;
DROP FUNCTION guard_production_member_content();
DROP TRIGGER production_versions_complete ON production_versions;
DROP FUNCTION validate_production_version_commit();
DROP TRIGGER production_versions_verified_insert ON production_versions;
DROP FUNCTION guard_production_verified_insert();
ALTER TABLE production_generation_applications
    DROP CONSTRAINT production_generation_applications_output_fkey,
    DROP CONSTRAINT production_generation_applications_source_fkey,
    DROP CONSTRAINT production_generation_applications_actor_fkey,
    DROP CONSTRAINT production_generation_applications_delta_check;
ALTER TABLE production_generation_outputs
    DROP CONSTRAINT production_generation_outputs_operation_run_key,
    DROP CONSTRAINT production_generation_outputs_version_fkey,
    DROP CONSTRAINT production_generation_outputs_digest_check;
ALTER TABLE production_generation_links
    DROP CONSTRAINT production_generation_links_version_fkey,
    DROP CONSTRAINT production_generation_links_run_fkey;
ALTER TABLE production_targets
    DROP CONSTRAINT production_targets_revision_workspace_fkey,
    DROP CONSTRAINT production_targets_revision_asset_fkey;
ALTER TABLE production_contributors DROP CONSTRAINT production_contributors_principal_fkey;
ALTER TABLE production_commands DROP CONSTRAINT production_commands_principal_fkey;
ALTER TABLE production_versions DROP CONSTRAINT production_versions_creator_fkey;
ALTER TABLE production_operations DROP CONSTRAINT production_operations_creator_fkey;

DROP TRIGGER production_links_sealed ON production_candidate_links;
DROP TRIGGER production_targets_sealed ON production_targets;
DROP FUNCTION guard_production_history_append();
DROP TRIGGER production_generation_applications_immutable ON production_generation_applications;
DROP TRIGGER production_generation_outputs_immutable ON production_generation_outputs;
DROP TRIGGER production_generation_links_immutable ON production_generation_links;
DROP TRIGGER production_contributors_immutable ON production_contributors;
DROP TRIGGER production_claims_immutable ON production_request_claims;
DROP TRIGGER production_commands_immutable ON production_commands;
DROP TRIGGER production_candidate_links_immutable ON production_candidate_links;
DROP TRIGGER production_targets_immutable ON production_targets;
DROP TRIGGER production_reservations_identity_guard ON production_identity_reservations;
DROP FUNCTION guard_production_reservation_identity();
DROP TRIGGER production_operations_version_guard ON production_operations;
DROP FUNCTION guard_production_operation_version();
DROP TRIGGER production_versions_immutable ON production_versions;
DROP FUNCTION guard_production_version_history();

ALTER TABLE production_targets DROP CONSTRAINT production_targets_identity_key_check;
ALTER TABLE production_targets ADD CONSTRAINT production_targets_identity_key_check CHECK(identity_key IS NULL OR length(identity_key) BETWEEN 1 AND 256);
ALTER TABLE production_identity_reservations DROP CONSTRAINT production_identity_reservations_identity_key_check;
ALTER TABLE production_identity_reservations ADD CONSTRAINT production_identity_reservations_identity_key_check CHECK(length(identity_key) BETWEEN 1 AND 256);
ALTER TABLE semantic_candidate_decisions DROP COLUMN previous_decision_id;
ALTER TABLE production_candidate_links DROP CONSTRAINT production_candidate_links_decision_fkey, DROP CONSTRAINT production_candidate_links_candidate_fkey;
ALTER TABLE production_commands DROP CONSTRAINT production_commands_version_fkey;
ALTER TABLE production_targets DROP CONSTRAINT production_targets_outcome_shape, DROP CONSTRAINT production_targets_proposal_fkey, DROP COLUMN canonical_content;
ALTER TABLE production_versions DROP CONSTRAINT production_versions_canonical_shape,
    DROP COLUMN history_quality, DROP COLUMN canonical_input, DROP COLUMN canonical_declarations,
    DROP COLUMN canonical_baseline, DROP COLUMN baseline_head, DROP COLUMN baseline_digest;
COMMIT;

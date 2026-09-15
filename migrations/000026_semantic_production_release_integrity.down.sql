BEGIN;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM production_validation_seals) OR EXISTS(SELECT 1 FROM production_release_integrity) THEN
  RAISE EXCEPTION 'cannot downgrade sealed production validation history';
 END IF;
END $$;
DROP TRIGGER production_before_pin_guard ON production_release_before_pins;
DROP TRIGGER production_binding_input_guard ON production_release_binding_inputs;
DROP FUNCTION guard_production_release_pin();
DROP TRIGGER production_reintroduction_member_guard ON proposals;
DROP FUNCTION guard_production_reintroduction_member();
CREATE OR REPLACE FUNCTION guard_production_reservation_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'production reservation is immutable' USING ERRCODE='23514'; END IF;
    IF (to_jsonb(NEW)-'owner_operation_id') IS DISTINCT FROM (to_jsonb(OLD)-'owner_operation_id') THEN
        RAISE EXCEPTION 'production reservation identity is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.owner_operation_id IS DISTINCT FROM OLD.owner_operation_id AND NOT EXISTS(
       SELECT 1 FROM production_operations WHERE workspace_id=NEW.workspace_id AND id=NEW.owner_operation_id AND supersedes_operation_id=OLD.owner_operation_id) THEN
        RAISE EXCEPTION 'production reservation requires explicit successor' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
DROP FUNCTION production_reintroduction_matches(uuid,text,uuid,text,uuid,jsonb);
DROP TRIGGER production_attribution_guard ON release_proposals;
DROP FUNCTION guard_production_attribution();
DROP TRIGGER production_finding_append_guard ON validation_results;
DROP FUNCTION guard_production_finding_append();
DROP TRIGGER production_seal_contents_guard ON production_validation_seals;
DROP FUNCTION guard_production_seal_contents();
DROP FUNCTION production_sorted_json_array(jsonb);
DROP TRIGGER production_release_proposals_append ON release_proposals;
DROP TRIGGER production_release_manifests_append ON production_release_manifests;
DROP TRIGGER production_release_before_pins_append ON production_release_before_pins;
DROP TRIGGER production_release_binding_inputs_append ON production_release_binding_inputs;
DROP FUNCTION guard_production_release_append();
DROP TRIGGER production_release_commit_guard ON releases;
DROP FUNCTION guard_production_release_commit();
DROP TRIGGER production_validation_run_guard ON validation_runs;
DROP FUNCTION guard_production_validation_run();
DROP TRIGGER production_review_insert_guard ON reviews;
DROP FUNCTION guard_production_review_insert();
DROP TRIGGER production_validation_binding_guard ON production_validation_bindings;
DROP TRIGGER production_review_binding_guard ON production_review_bindings;
DROP FUNCTION guard_production_binding_integrity();
DROP FUNCTION production_manifest_document(uuid,uuid,boolean);
DROP FUNCTION production_typeid(text,uuid);
DROP TRIGGER production_validation_bindings_immutable ON production_validation_bindings;
DROP TRIGGER production_review_bindings_immutable ON production_review_bindings;
DROP TRIGGER release_proposals_immutable ON release_proposals;
DROP TRIGGER production_release_manifests_immutable ON production_release_manifests;
DROP TRIGGER production_release_before_pins_immutable ON production_release_before_pins;
DROP TRIGGER production_release_binding_inputs_immutable ON production_release_binding_inputs;
ALTER TABLE join_contracts DROP CONSTRAINT join_contracts_contract_notes_check;
ALTER TABLE join_contracts ADD CONSTRAINT join_contracts_contract_notes_check CHECK (contract_notes IS NULL OR (contract_notes<>'' AND length(contract_notes)<=4096));
DROP TABLE production_release_integrity;
DROP FUNCTION guard_production_release_integrity();
DROP TRIGGER production_attempt_completion_guard ON production_validation_attempts;
DROP FUNCTION guard_production_attempt_completion();
DROP TABLE production_validation_seals;
DROP FUNCTION guard_production_validation_seal();
ALTER TABLE production_commands DROP COLUMN response_json;
CREATE OR REPLACE FUNCTION guard_production_member_transition() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.production_operation_id IS NULL OR OLD.state=NEW.state THEN RETURN NULL; END IF;
    IF NOT EXISTS(SELECT 1 FROM production_commands c
       WHERE c.workspace_id=NEW.workspace_id AND c.operation_id=NEW.production_operation_id
         AND c.xmin=pg_current_xact_id()::xid
         AND ((c.operation_version=NEW.production_version AND c.command_kind IN ('submit','validate','review','publish'))
           OR (OLD.state='draft' AND NEW.state='rejected' AND c.command_kind='replace_draft' AND c.operation_version=NEW.production_version+1)))
       AND NOT EXISTS(SELECT 1 FROM production_commands c JOIN production_operations o ON o.workspace_id=c.workspace_id AND o.id=c.operation_id
          WHERE c.workspace_id=NEW.workspace_id AND o.supersedes_operation_id=NEW.production_operation_id
            AND c.command_kind='create' AND c.operation_version=1 AND c.xmin=pg_current_xact_id()::xid AND NEW.state='rejected') THEN
        RAISE EXCEPTION 'production member transition requires atomic production command' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;

CREATE OR REPLACE FUNCTION guard_production_reserved_proposal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM production_identity_reservations r
       WHERE r.workspace_id=NEW.workspace_id AND r.target_id=NEW.target_object_id
         AND r.owner_operation_id IS DISTINCT FROM NEW.production_operation_id) THEN
        RAISE EXCEPTION 'target is owned by production authoring' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;

COMMIT;

BEGIN;

-- Existing JSONB history cannot prove the original number lexemes. It remains
-- explicitly unverifiable; only the new writer supplies canonical authority.
ALTER TABLE production_versions
    ADD COLUMN history_quality text NOT NULL DEFAULT 'unverifiable'
        CHECK (history_quality IN ('verified','unverifiable')),
    ADD COLUMN canonical_input bytea,
    ADD COLUMN canonical_declarations bytea,
    ADD COLUMN canonical_baseline bytea,
    ADD COLUMN baseline_head jsonb,
    ADD COLUMN baseline_digest text,
    ADD CONSTRAINT production_versions_canonical_shape CHECK (
        (history_quality='unverifiable' AND canonical_input IS NULL AND canonical_declarations IS NULL
         AND canonical_baseline IS NULL AND baseline_head IS NULL AND baseline_digest IS NULL)
        OR
        (history_quality='verified' AND canonical_input IS NOT NULL AND canonical_declarations IS NOT NULL
         AND canonical_baseline IS NOT NULL AND baseline_head IS NOT NULL AND baseline_digest IS NOT NULL
         AND octet_length(canonical_input)+octet_length(canonical_declarations) BETWEEN 2 AND 786432
         AND octet_length(canonical_baseline) BETWEEN 2 AND 1048576
         AND convert_from(canonical_input,'UTF8')::jsonb=input_json
         AND jsonb_typeof(convert_from(canonical_declarations,'UTF8')::jsonb)='array'
         AND jsonb_typeof(convert_from(canonical_baseline,'UTF8')::jsonb)='object'
         AND input_digest='sha256:'||encode(sha256(canonical_input),'hex')
         AND baseline_digest='sha256:'||encode(sha256(canonical_baseline),'hex')
         AND COALESCE(((baseline_head->>'presence'='absent' AND baseline_head='{"presence":"absent"}'::jsonb)
              OR (baseline_head->>'presence'='present' AND baseline_head ?& ARRAY['releaseId','manifestDigest'])),false))
    );

ALTER TABLE production_targets
    ADD COLUMN canonical_content bytea,
    ADD CONSTRAINT production_targets_canonical_content CHECK (
        canonical_content IS NULL OR
        (octet_length(canonical_content) BETWEEN 2 AND 786432
         AND convert_from(canonical_content,'UTF8')::jsonb=content_json
         AND content_digest='sha256:'||encode(sha256(canonical_content),'hex'))
    ),
    ADD CONSTRAINT production_targets_proposal_fkey FOREIGN KEY (workspace_id,proposal_id)
        REFERENCES proposals(workspace_id,id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT production_targets_outcome_shape CHECK (
        (outcome='proposal' AND proposal_id IS NOT NULL) OR (outcome='no_change' AND proposal_id IS NULL AND intent='update')
    ) NOT VALID;

ALTER TABLE production_commands ADD CONSTRAINT production_commands_version_fkey
    FOREIGN KEY (workspace_id,operation_id,operation_version)
    REFERENCES production_versions(workspace_id,operation_id,version) ON DELETE RESTRICT NOT VALID;
ALTER TABLE production_candidate_links
    ADD CONSTRAINT production_candidate_links_candidate_fkey FOREIGN KEY (workspace_id,candidate_id)
        REFERENCES semantic_candidates(workspace_id,id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT production_candidate_links_decision_fkey FOREIGN KEY (workspace_id,decision_id)
        REFERENCES semantic_candidate_decisions(workspace_id,id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE semantic_candidate_decisions ADD COLUMN previous_decision_id uuid,
    ADD CONSTRAINT semantic_candidate_decisions_previous_fkey FOREIGN KEY (workspace_id,previous_decision_id)
        REFERENCES semantic_candidate_decisions(workspace_id,id) ON DELETE RESTRICT;

ALTER TABLE production_operations ADD CONSTRAINT production_operations_creator_fkey
    FOREIGN KEY(workspace_id,created_by) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE production_versions ADD CONSTRAINT production_versions_creator_fkey
    FOREIGN KEY(workspace_id,created_by) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE production_commands ADD CONSTRAINT production_commands_principal_fkey
    FOREIGN KEY(workspace_id,principal_id) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE production_contributors ADD CONSTRAINT production_contributors_principal_fkey
    FOREIGN KEY(workspace_id,principal_id) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE production_targets
    ADD CONSTRAINT production_targets_revision_workspace_fkey FOREIGN KEY(workspace_id,base_revision_id) REFERENCES asset_revisions(workspace_id,id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT production_targets_revision_asset_fkey FOREIGN KEY(target_id,base_revision_id) REFERENCES asset_revisions(asset_id,id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE production_generation_links
    ADD CONSTRAINT production_generation_links_version_fkey FOREIGN KEY(workspace_id,operation_id,input_version) REFERENCES production_versions(workspace_id,operation_id,version) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT production_generation_links_run_fkey FOREIGN KEY(workspace_id,agent_run_id) REFERENCES agent_runs(workspace_id,id) ON DELETE RESTRICT NOT VALID;
ALTER TABLE production_generation_outputs
    ADD CONSTRAINT production_generation_outputs_operation_run_key UNIQUE(workspace_id,operation_id,agent_run_id),
    ADD CONSTRAINT production_generation_outputs_version_fkey FOREIGN KEY(workspace_id,operation_id,input_version) REFERENCES production_versions(workspace_id,operation_id,version) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT production_generation_outputs_digest_check CHECK(output_digest='sha256:'||encode(sha256(canonical_output),'hex')) NOT VALID;
ALTER TABLE production_generation_applications
    ADD CONSTRAINT production_generation_applications_output_fkey FOREIGN KEY(workspace_id,operation_id,agent_run_id) REFERENCES production_generation_outputs(workspace_id,operation_id,agent_run_id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT production_generation_applications_source_fkey FOREIGN KEY(workspace_id,operation_id,source_version) REFERENCES production_versions(workspace_id,operation_id,version) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT production_generation_applications_actor_fkey FOREIGN KEY(workspace_id,actor_principal_id) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT production_generation_applications_delta_check CHECK(octet_length(canonical_delta) BETWEEN 1 AND 1048576 AND delta_digest='sha256:'||encode(sha256(canonical_delta),'hex')) NOT VALID;

CREATE FUNCTION guard_production_verified_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.history_quality<>'verified' THEN
        RAISE EXCEPTION 'production writer must supply canonical history' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER production_versions_verified_insert BEFORE INSERT ON production_versions
    FOR EACH ROW EXECUTE FUNCTION guard_production_verified_insert();

ALTER TABLE production_identity_reservations DROP CONSTRAINT production_identity_reservations_identity_key_check;
ALTER TABLE production_identity_reservations ADD CONSTRAINT production_identity_reservations_identity_key_check CHECK (length(identity_key) BETWEEN 1 AND 512);
ALTER TABLE production_targets DROP CONSTRAINT production_targets_identity_key_check;
ALTER TABLE production_targets ADD CONSTRAINT production_targets_identity_key_check CHECK (identity_key IS NULL OR length(identity_key) BETWEEN 1 AND 512);

CREATE FUNCTION guard_production_version_history() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'production history is immutable' USING ERRCODE='23514'; END IF;
    IF (to_jsonb(NEW)-'frozen_at'-'active_validation_attempt_no') IS DISTINCT FROM
       (to_jsonb(OLD)-'frozen_at'-'active_validation_attempt_no')
       OR (OLD.frozen_at IS NOT NULL AND NEW.frozen_at IS DISTINCT FROM OLD.frozen_at) THEN
        RAISE EXCEPTION 'production history is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER production_versions_immutable BEFORE UPDATE OR DELETE ON production_versions
    FOR EACH ROW EXECUTE FUNCTION guard_production_version_history();

CREATE FUNCTION guard_production_operation_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'production operation is immutable' USING ERRCODE='23514'; END IF;
    IF (to_jsonb(NEW)-'current_version'-'updated_at') IS DISTINCT FROM (to_jsonb(OLD)-'current_version'-'updated_at')
       OR NEW.current_version<>OLD.current_version+1
       OR EXISTS(SELECT 1 FROM production_operations WHERE workspace_id=OLD.workspace_id AND supersedes_operation_id=OLD.id)
       OR EXISTS(SELECT 1 FROM production_versions WHERE workspace_id=OLD.workspace_id AND operation_id=OLD.id
                 AND version=OLD.current_version AND frozen_at IS NOT NULL)
       OR EXISTS(SELECT 1 FROM proposals WHERE workspace_id=OLD.workspace_id AND production_operation_id=OLD.id
                 AND production_version=OLD.current_version AND state<>'draft') THEN
        RAISE EXCEPTION 'production draft version conflict' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER production_operations_version_guard BEFORE UPDATE OR DELETE ON production_operations
    FOR EACH ROW EXECUTE FUNCTION guard_production_operation_version();

CREATE FUNCTION guard_production_reservation_identity() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE TRIGGER production_reservations_identity_guard BEFORE UPDATE OR DELETE ON production_identity_reservations
    FOR EACH ROW EXECUTE FUNCTION guard_production_reservation_identity();

CREATE TRIGGER production_targets_immutable BEFORE UPDATE OR DELETE ON production_targets FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_candidate_links_immutable BEFORE UPDATE OR DELETE ON production_candidate_links FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_commands_immutable BEFORE UPDATE OR DELETE ON production_commands FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_claims_immutable BEFORE UPDATE OR DELETE ON production_request_claims FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_contributors_immutable BEFORE UPDATE OR DELETE ON production_contributors FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_generation_links_immutable BEFORE UPDATE OR DELETE ON production_generation_links FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_generation_outputs_immutable BEFORE UPDATE OR DELETE ON production_generation_outputs FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_generation_applications_immutable BEFORE UPDATE OR DELETE ON production_generation_applications FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE FUNCTION guard_production_history_append() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM production_commands WHERE workspace_id=NEW.workspace_id
              AND operation_id=NEW.operation_id AND operation_version=NEW.version AND command_kind IN ('create','replace_draft')) THEN
        RAISE EXCEPTION 'production history is sealed' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER production_targets_sealed BEFORE INSERT ON production_targets FOR EACH ROW EXECUTE FUNCTION guard_production_history_append();
CREATE TRIGGER production_links_sealed BEFORE INSERT ON production_candidate_links FOR EACH ROW EXECUTE FUNCTION guard_production_history_append();

CREATE FUNCTION validate_production_version_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target_count integer;
BEGIN
    SELECT count(*) INTO target_count FROM production_targets
      WHERE workspace_id=NEW.workspace_id AND operation_id=NEW.operation_id AND version=NEW.version;
    IF NEW.history_quality<>'verified' OR target_count NOT BETWEEN 1 AND 32
       OR target_count<>jsonb_array_length(convert_from(NEW.canonical_declarations,'UTF8')::jsonb)
       OR NOT EXISTS(SELECT 1 FROM production_commands c WHERE c.workspace_id=NEW.workspace_id
          AND c.operation_id=NEW.operation_id AND c.operation_version=NEW.version
          AND c.command_kind=CASE WHEN NEW.version=1 THEN 'create' ELSE 'replace_draft' END
          AND c.request_digest=NEW.request_digest AND c.principal_id=NEW.created_by)
       OR EXISTS(SELECT 1 FROM production_targets t LEFT JOIN proposals p ON p.workspace_id=t.workspace_id AND p.id=t.proposal_id
          WHERE t.workspace_id=NEW.workspace_id AND t.operation_id=NEW.operation_id AND t.version=NEW.version
          AND (t.canonical_content IS NULL OR (t.proposal_id IS NOT NULL AND
             (p.production_operation_id IS DISTINCT FROM t.operation_id OR p.production_version IS DISTINCT FROM t.version
              OR p.target_object_id IS DISTINCT FROM t.target_id OR p.target_object_type IS DISTINCT FROM t.kind
              OR p.intent IS DISTINCT FROM t.intent OR p.base_revision_id IS DISTINCT FROM t.base_revision_id
              OR p.base_object_version IS DISTINCT FROM t.base_object_version
              OR (t.intent='create' AND p.creation_content IS DISTINCT FROM t.content_json)))))
       OR EXISTS(SELECT 1 FROM production_candidate_links l
          WHERE l.workspace_id=NEW.workspace_id AND l.operation_id=NEW.operation_id AND l.version=NEW.version
          GROUP BY l.candidate_id HAVING count(*) FILTER(WHERE l.is_primary)<>1 OR count(DISTINCT l.decision_id)>1)
       OR EXISTS(SELECT 1 FROM production_candidate_links l
          JOIN production_targets t ON t.workspace_id=l.workspace_id AND t.operation_id=l.operation_id AND t.version=l.version AND t.local_key=l.local_key
          LEFT JOIN semantic_candidate_decisions d ON d.workspace_id=l.workspace_id AND d.id=l.decision_id
          WHERE l.workspace_id=NEW.workspace_id AND l.operation_id=NEW.operation_id AND l.version=NEW.version
          AND ((l.is_primary AND t.proposal_id IS NOT NULL AND (d.id IS NULL OR d.candidate_id<>l.candidate_id OR d.proposal_id IS DISTINCT FROM t.proposal_id))
               OR (l.decision_id IS NOT NULL AND d.candidate_id IS DISTINCT FROM l.candidate_id))) THEN
        RAISE EXCEPTION 'incomplete production authoring transaction' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_versions_complete AFTER INSERT ON production_versions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_production_version_commit();

CREATE FUNCTION guard_production_member_content() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.production_operation_id IS NOT NULL AND
       ((to_jsonb(NEW)-'state'-'submitted_at'-'decided_at'-'updated_at'-'policy_decision_id') IS DISTINCT FROM
        (to_jsonb(OLD)-'state'-'submitted_at'-'decided_at'-'updated_at'-'policy_decision_id')) THEN
        RAISE EXCEPTION 'production member content is immutable' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER production_proposal_content_guard BEFORE UPDATE ON proposals
    FOR EACH ROW EXECUTE FUNCTION guard_production_member_content();

CREATE FUNCTION guard_production_change_append() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM proposals p JOIN production_commands c ON c.workspace_id=p.workspace_id
        AND c.operation_id=p.production_operation_id AND c.operation_version=p.production_version
        WHERE p.workspace_id=NEW.workspace_id AND p.id=NEW.proposal_id AND c.command_kind IN ('create','replace_draft')) THEN
        RAISE EXCEPTION 'production member changes are sealed' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER production_proposal_changes_guard BEFORE INSERT ON proposal_changes
    FOR EACH ROW EXECUTE FUNCTION guard_production_change_append();

CREATE FUNCTION guard_production_reserved_proposal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM production_identity_reservations r
       WHERE r.workspace_id=NEW.workspace_id AND r.target_id=NEW.target_object_id
         AND r.owner_operation_id IS DISTINCT FROM NEW.production_operation_id) THEN
        RAISE EXCEPTION 'target is owned by production authoring' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER production_proposal_reservation_guard BEFORE INSERT ON proposals
    FOR EACH ROW EXECUTE FUNCTION guard_production_reserved_proposal();

CREATE FUNCTION guard_production_member_transition() RETURNS trigger LANGUAGE plpgsql AS $$
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
CREATE CONSTRAINT TRIGGER production_proposal_transition_guard AFTER UPDATE ON proposals
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_member_transition();

CREATE FUNCTION guard_production_catalog_write() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target uuid;
BEGIN
    IF TG_TABLE_NAME='semantic_assets' THEN
        IF (NEW.namespace,NEW.key,NEW.asset_type,NEW.current_revision_id,NEW.lifecycle_state)
            IS NOT DISTINCT FROM (OLD.namespace,OLD.key,OLD.asset_type,OLD.current_revision_id,OLD.lifecycle_state) THEN RETURN NULL; END IF;
        target:=NEW.id;
    ELSE
        target:=NEW.asset_id;
    END IF;
    IF (EXISTS(SELECT 1 FROM production_identity_reservations r WHERE r.workspace_id=NEW.workspace_id AND r.target_id=target)
        OR EXISTS(SELECT 1 FROM production_targets t JOIN proposals p ON p.workspace_id=t.workspace_id AND p.id=t.proposal_id
           WHERE t.workspace_id=NEW.workspace_id AND t.target_id=target AND p.state NOT IN ('released','rejected')))
       AND NOT EXISTS(SELECT 1 FROM production_targets t JOIN production_commands c
           ON c.workspace_id=t.workspace_id AND c.operation_id=t.operation_id AND c.operation_version=t.version
           WHERE t.workspace_id=NEW.workspace_id AND t.target_id=target AND c.command_kind IN ('publish','rollback')
             AND c.xmin=pg_current_xact_id()::xid) THEN
        RAISE EXCEPTION 'production asset requires atomic publication command' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_asset_write_guard AFTER UPDATE ON semantic_assets
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_catalog_write();
CREATE CONSTRAINT TRIGGER production_revision_write_guard AFTER INSERT ON asset_revisions
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_catalog_write();

COMMIT;

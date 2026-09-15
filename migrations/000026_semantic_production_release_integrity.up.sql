BEGIN;

ALTER TABLE join_contracts DROP CONSTRAINT join_contracts_contract_notes_check;
ALTER TABLE join_contracts ADD CONSTRAINT join_contracts_contract_notes_check CHECK (contract_notes IS NULL OR length(contract_notes)<=4096);

CREATE TABLE production_release_integrity (
    workspace_id uuid NOT NULL,
    release_id uuid NOT NULL,
    before_payload bytea NOT NULL,
    after_payload bytea NOT NULL,
    PRIMARY KEY (workspace_id,release_id),
    FOREIGN KEY (workspace_id,release_id) REFERENCES releases(workspace_id,id) ON DELETE RESTRICT,
    CHECK (octet_length(before_payload)<=1048576 AND octet_length(after_payload)<=1048576)
);

CREATE FUNCTION guard_production_release_integrity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP<>'INSERT' THEN
        RAISE EXCEPTION 'production release integrity is immutable' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM production_release_manifests m JOIN releases r ON r.workspace_id=m.workspace_id AND r.id=m.release_id
        WHERE m.workspace_id=NEW.workspace_id AND m.release_id=NEW.release_id
        AND r.production_root_release_id IS NOT NULL
        AND m.before_manifest_digest='sha256:'||encode(sha256(NEW.before_payload),'hex')
        AND m.after_manifest_digest='sha256:'||encode(sha256(NEW.after_payload),'hex')
        AND r.manifest_digest=m.after_manifest_digest)
        OR NOT EXISTS(SELECT 1 FROM production_commands c WHERE c.workspace_id=NEW.workspace_id AND c.result_id=NEW.release_id
            AND c.command_kind IN ('publish','rollback') AND c.xmin=pg_current_xact_id()::xid) THEN
        RAISE EXCEPTION 'production release integrity requires atomic release command and matching manifests' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_release_integrity_guard AFTER INSERT OR UPDATE OR DELETE ON production_release_integrity
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_release_integrity();

ALTER TABLE production_commands ADD COLUMN response_json jsonb NULL CHECK (response_json IS NULL OR jsonb_typeof(response_json)='object');

CREATE TABLE production_validation_seals (
    workspace_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    production_version integer NOT NULL,
    attempt_no integer NOT NULL,
    results_json jsonb NOT NULL CHECK (jsonb_typeof(results_json)='array' AND jsonb_array_length(results_json) BETWEEN 1 AND 256),
    validation_digest text NOT NULL CHECK (validation_digest ~ '^sha256:[0-9a-f]{64}$'),
    policy_ids jsonb NOT NULL CHECK (jsonb_typeof(policy_ids)='array'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id,operation_id,production_version,attempt_no),
    FOREIGN KEY (workspace_id,operation_id,production_version,attempt_no)
      REFERENCES production_validation_attempts(workspace_id,operation_id,production_version,attempt_no) ON DELETE RESTRICT
);

CREATE FUNCTION guard_production_validation_seal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP<>'INSERT' THEN
        RAISE EXCEPTION 'production validation seals are immutable' USING ERRCODE='23514';
    END IF;
    IF NOT EXISTS(SELECT 1 FROM production_validation_attempts a
        WHERE a.workspace_id=NEW.workspace_id AND a.operation_id=NEW.operation_id
        AND a.production_version=NEW.production_version AND a.attempt_no=NEW.attempt_no
        AND a.status IN ('succeeded','failed') AND a.validation_digest=NEW.validation_digest
        AND a.completed_at IS NOT NULL AND jsonb_array_length(a.required_checks_json)=jsonb_array_length(NEW.results_json))
       OR (SELECT count(*) FROM validation_runs r WHERE r.workspace_id=NEW.workspace_id AND r.production_operation_id=NEW.operation_id
        AND r.production_version=NEW.production_version AND r.production_attempt_no=NEW.attempt_no AND r.status IN ('succeeded','failed'))<>jsonb_array_length(NEW.results_json)
       OR (SELECT count(*) FROM production_validation_bindings b WHERE b.workspace_id=NEW.workspace_id AND b.operation_id=NEW.operation_id
        AND b.production_version=NEW.production_version AND b.attempt_no=NEW.attempt_no)<>jsonb_array_length(NEW.results_json) THEN
        RAISE EXCEPTION 'production validation seal requires complete bound runs' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_validation_seal_guard AFTER INSERT OR UPDATE OR DELETE ON production_validation_seals
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_validation_seal();

CREATE FUNCTION guard_production_attempt_completion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' AND OLD.status IN ('succeeded','failed') THEN
        RAISE EXCEPTION 'completed production validation is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.status IN ('succeeded','failed') AND NOT EXISTS(SELECT 1 FROM production_validation_seals s
       WHERE s.workspace_id=NEW.workspace_id AND s.operation_id=NEW.operation_id AND s.production_version=NEW.production_version
       AND s.attempt_no=NEW.attempt_no AND s.validation_digest=NEW.validation_digest) THEN
        RAISE EXCEPTION 'production validation completion requires seal' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_attempt_completion_guard AFTER INSERT OR UPDATE ON production_validation_attempts
 DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_attempt_completion();

CREATE OR REPLACE FUNCTION guard_production_member_transition() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.production_operation_id IS NULL OR OLD.state=NEW.state THEN RETURN NULL; END IF;
    IF OLD.state='validating' AND NEW.state='in_review' AND EXISTS(
      SELECT 1 FROM production_validation_seals s JOIN production_versions v ON v.workspace_id=s.workspace_id
       AND v.operation_id=s.operation_id AND v.version=s.production_version AND v.active_validation_attempt_no=s.attempt_no
      WHERE s.workspace_id=NEW.workspace_id AND s.operation_id=NEW.production_operation_id AND s.production_version=NEW.production_version
        AND s.xmin=pg_current_xact_id()::xid) THEN RETURN NULL; END IF;
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

CREATE TRIGGER production_validation_bindings_immutable BEFORE UPDATE OR DELETE ON production_validation_bindings FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_review_bindings_immutable BEFORE UPDATE OR DELETE ON production_review_bindings FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER release_proposals_immutable BEFORE UPDATE OR DELETE ON release_proposals FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_release_manifests_immutable BEFORE UPDATE OR DELETE ON production_release_manifests FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_release_before_pins_immutable BEFORE UPDATE OR DELETE ON production_release_before_pins FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER production_release_binding_inputs_immutable BEFORE UPDATE OR DELETE ON production_release_binding_inputs FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE FUNCTION production_typeid(prefix text, id uuid) RETURNS text LANGUAGE plpgsql IMMUTABLE STRICT AS $$
DECLARE
    bytes bytea := uuid_send(id);
    alphabet text := '0123456789abcdefghjkmnpqrstvwxyz';
    encoded text := '';
    value integer := 0;
    bit_index integer;
BEGIN
    FOR i IN 0..129 LOOP
        value := value*2;
        bit_index := i-2;
        IF bit_index>=0 THEN value := value + ((get_byte(bytes,bit_index/8) >> (7-bit_index%8)) & 1); END IF;
        IF i%5=4 THEN encoded:=encoded||substr(alphabet,value+1,1); value:=0; END IF;
    END LOOP;
    RETURN prefix||'_'||encoded;
END $$;

CREATE FUNCTION production_manifest_document(w uuid, r uuid, wire boolean) RETURNS jsonb LANGUAGE plpgsql STABLE AS $$
DECLARE assets jsonb; objects jsonb;
BEGIN
    SELECT COALESCE(jsonb_agg(jsonb_build_object('assetId',production_typeid('ast',asset_id),'revisionId',production_typeid('rev',revision_id),'compatibility',compatibility,'position',position) ORDER BY position),'[]'::jsonb)
        INTO assets FROM release_assets WHERE workspace_id=w AND release_id=r;
    SELECT COALESCE(jsonb_agg(CASE WHEN wire THEN jsonb_build_object('kind',object_type,'targetId',production_typeid(CASE object_type WHEN 'physical_binding' THEN 'phb' WHEN 'model_grain' THEN 'mgn' WHEN 'entity_key' THEN 'eky' WHEN 'join_contract' THEN 'jct' END,object_id),'objectVersion',version,'position',position)
        ELSE jsonb_build_object('objectType',object_type,'objectId',object_id::text,'version',version,'position',position) END ORDER BY position),'[]'::jsonb)
        INTO objects FROM release_objects WHERE workspace_id=w AND release_id=r;
    IF NOT wire AND objects='[]'::jsonb THEN RETURN assets; END IF;
    RETURN jsonb_build_object('assets',assets,'objects',objects);
END $$;

CREATE FUNCTION guard_production_binding_integrity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE review_row reviews%ROWTYPE;
BEGIN
    IF TG_TABLE_NAME='production_validation_bindings' THEN
        IF NOT EXISTS(SELECT 1 FROM validation_runs r JOIN production_targets t ON t.workspace_id=r.workspace_id AND t.proposal_id=r.proposal_id
            JOIN production_validation_attempts a ON a.workspace_id=t.workspace_id AND a.operation_id=t.operation_id AND a.production_version=t.version AND a.attempt_no=r.production_attempt_no
            WHERE r.id=NEW.validation_run_id AND r.workspace_id=NEW.workspace_id AND r.proposal_id=NEW.proposal_id
            AND r.production_operation_id=NEW.operation_id AND r.production_version=NEW.production_version AND r.production_attempt_no=NEW.attempt_no
            AND t.operation_id=NEW.operation_id AND t.version=NEW.production_version AND t.content_digest=NEW.proposal_content_digest
            AND a.set_digest=NEW.set_digest AND a.input_digest=NEW.input_digest
            AND a.required_checks_json @> jsonb_build_array(jsonb_build_object('proposalId',production_typeid('prp',r.proposal_id),'validatorId',r.validator_id,'validatorVersion',r.validator_version))
            AND EXISTS(SELECT 1 FROM production_validation_seals s WHERE s.workspace_id=a.workspace_id AND s.operation_id=a.operation_id AND s.production_version=a.production_version AND s.attempt_no=a.attempt_no AND s.xmin=pg_current_xact_id()::xid)) THEN
            RAISE EXCEPTION 'production validation binding does not match sealed member and check' USING ERRCODE='23514';
        END IF;
    ELSE
        SELECT * INTO review_row FROM reviews WHERE workspace_id=NEW.workspace_id AND id=NEW.review_id;
        IF NOT FOUND OR review_row.channel<>'expert' OR NOT EXISTS(SELECT 1 FROM principals WHERE workspace_id=NEW.workspace_id AND id=review_row.reviewer_principal_id AND kind='human' AND status='active')
            OR EXISTS(SELECT 1 FROM production_contributors WHERE workspace_id=NEW.workspace_id AND operation_id=NEW.operation_id AND principal_id=review_row.reviewer_principal_id)
            OR NOT EXISTS(SELECT 1 FROM production_targets t JOIN production_versions v ON v.workspace_id=t.workspace_id AND v.operation_id=t.operation_id AND v.version=t.version
                JOIN production_validation_attempts a ON a.workspace_id=v.workspace_id AND a.operation_id=v.operation_id AND a.production_version=v.version AND a.attempt_no=v.active_validation_attempt_no
                JOIN production_validation_seals s ON s.workspace_id=a.workspace_id AND s.operation_id=a.operation_id AND s.production_version=a.production_version AND s.attempt_no=a.attempt_no
                WHERE t.workspace_id=NEW.workspace_id AND t.operation_id=NEW.operation_id AND t.version=NEW.production_version AND t.proposal_id=review_row.proposal_id
                AND a.attempt_no=NEW.attempt_no AND a.set_digest=NEW.set_digest AND a.validation_digest=NEW.validation_digest AND s.validation_digest=NEW.validation_digest
                AND (a.status='succeeded' OR review_row.decision='rejected') AND a.completed_at IS NOT NULL)
            OR NOT EXISTS(SELECT 1 FROM production_commands c WHERE c.workspace_id=NEW.workspace_id AND c.operation_id=NEW.operation_id AND c.operation_version=NEW.production_version
                AND c.principal_id=review_row.reviewer_principal_id AND c.command_kind='review' AND c.xmin=pg_current_xact_id()::xid)
            OR EXISTS(SELECT 1 FROM production_review_bindings b JOIN reviews r ON r.workspace_id=b.workspace_id AND r.id=b.review_id WHERE b.workspace_id=NEW.workspace_id
                AND b.operation_id=NEW.operation_id AND b.production_version=NEW.production_version AND b.attempt_no=NEW.attempt_no AND b.review_id<>NEW.review_id
                AND r.proposal_id=review_row.proposal_id AND r.reviewer_principal_id=review_row.reviewer_principal_id AND r.channel=review_row.channel) THEN
            RAISE EXCEPTION 'production review requires bound active validation and independent human command' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_validation_binding_guard AFTER INSERT ON production_validation_bindings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_binding_integrity();
CREATE CONSTRAINT TRIGGER production_review_binding_guard AFTER INSERT ON production_review_bindings DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_binding_integrity();

CREATE FUNCTION guard_production_review_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM proposals p WHERE p.workspace_id=NEW.workspace_id AND p.id=NEW.proposal_id AND p.production_operation_id IS NOT NULL)
        AND NOT EXISTS(SELECT 1 FROM production_review_bindings b WHERE b.workspace_id=NEW.workspace_id AND b.review_id=NEW.id) THEN
        RAISE EXCEPTION 'production review requires production binding' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_review_insert_guard AFTER INSERT ON reviews DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_review_insert();

CREATE FUNCTION guard_production_validation_run() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP<>'INSERT' THEN
        IF OLD.production_operation_id IS NOT NULL THEN RAISE EXCEPTION 'production runs are immutable' USING ERRCODE='23514'; END IF;
    ELSIF EXISTS(SELECT 1 FROM proposals p WHERE p.workspace_id=NEW.workspace_id AND p.id=NEW.proposal_id AND p.production_operation_id IS NOT NULL)
        AND NOT EXISTS(SELECT 1 FROM production_validation_bindings b WHERE b.workspace_id=NEW.workspace_id AND b.validation_run_id=NEW.id) THEN
        RAISE EXCEPTION 'production run requires complete production binding' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_validation_run_guard AFTER INSERT OR UPDATE OR DELETE ON validation_runs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_validation_run();

CREATE FUNCTION guard_production_release_commit() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE m production_release_manifests%ROWTYPE; parent releases%ROWTYPE; proof production_release_integrity%ROWTYPE; c production_commands%ROWTYPE; expected_before uuid; member_count integer;
BEGIN
    IF NEW.rolled_back_to_release_id IS NOT NULL THEN
        SELECT * INTO parent FROM releases WHERE workspace_id=NEW.workspace_id AND id=NEW.rolled_back_to_release_id;
        IF parent.production_root_release_id IS NOT NULL AND (NEW.production_root_release_id IS DISTINCT FROM parent.production_root_release_id
            OR NEW.production_rollback_parent_id IS DISTINCT FROM parent.id OR NEW.production_rollback_depth IS DISTINCT FROM parent.production_rollback_depth+1) THEN
            RAISE EXCEPTION 'protected rollback requires inherited production protection' USING ERRCODE='23514';
        END IF;
    END IF;
    IF NEW.production_root_release_id IS NULL THEN RETURN NULL; END IF;
    SELECT * INTO proof FROM production_release_integrity WHERE workspace_id=NEW.workspace_id AND release_id=NEW.id AND xmin=pg_current_xact_id()::xid;
    IF NOT FOUND THEN RAISE EXCEPTION 'production release requires integrity proof' USING ERRCODE='23514'; END IF;
    SELECT * INTO m FROM production_release_manifests WHERE workspace_id=NEW.workspace_id AND release_id=NEW.id;
    IF NOT FOUND THEN RAISE EXCEPTION 'production release requires manifest' USING ERRCODE='23514'; END IF;
    SELECT * INTO c FROM production_commands WHERE workspace_id=NEW.workspace_id AND result_id=NEW.id AND result_kind='release'
        AND command_kind=CASE WHEN NEW.production_rollback_depth=0 THEN 'publish' ELSE 'rollback' END AND xmin=pg_current_xact_id()::xid;
    IF NOT FOUND OR c.response_json IS NULL OR c.response_json->>'releaseId' IS DISTINCT FROM production_typeid('rls',NEW.id)
        OR c.response_json->>'operationId' IS DISTINCT FROM production_typeid('prodop',c.operation_id)
        OR c.response_json->>'manifestDigest' IS DISTINCT FROM NEW.manifest_digest THEN
        RAISE EXCEPTION 'production release requires exact durable command receipt' USING ERRCODE='23514';
    END IF;
    SELECT id INTO expected_before FROM releases WHERE workspace_id=NEW.workspace_id AND sequence<NEW.sequence ORDER BY sequence DESC LIMIT 1;
    IF m.before_release_id IS DISTINCT FROM expected_before OR m.before_manifest_json IS DISTINCT FROM production_manifest_document(NEW.workspace_id,expected_before,true)
        OR convert_from(proof.before_payload,'UTF8')::jsonb IS DISTINCT FROM production_manifest_document(NEW.workspace_id,expected_before,false)
        OR convert_from(proof.after_payload,'UTF8')::jsonb IS DISTINCT FROM production_manifest_document(NEW.workspace_id,NEW.id,false) THEN
        RAISE EXCEPTION 'production release requires exact complete before and after manifests' USING ERRCODE='23514';
    END IF;
    IF NEW.production_rollback_depth>0 AND (NEW.production_rollback_parent_id IS DISTINCT FROM expected_before
        OR NEW.rolled_back_to_release_id IS DISTINCT FROM expected_before
        OR NOT EXISTS(SELECT 1 FROM production_release_integrity WHERE workspace_id=NEW.workspace_id AND release_id=expected_before)
        OR production_manifest_document(NEW.workspace_id,NEW.id,true) IS DISTINCT FROM (SELECT before_manifest_json FROM production_release_manifests WHERE workspace_id=NEW.workspace_id AND release_id=expected_before)) THEN
        RAISE EXCEPTION 'production rollback must restore protected parents exact before manifest' USING ERRCODE='23514';
    END IF;
    SELECT count(*) INTO member_count FROM production_targets WHERE workspace_id=c.workspace_id AND operation_id=c.operation_id AND version=c.operation_version AND proposal_id IS NOT NULL;
    IF member_count=0 OR member_count<>(SELECT count(*) FROM release_proposals WHERE workspace_id=NEW.workspace_id AND release_id=NEW.id)
        OR (SELECT count(*) FROM production_targets WHERE workspace_id=c.workspace_id AND operation_id=c.operation_id AND version=c.operation_version)
          <>(SELECT count(*) FROM production_release_before_pins WHERE workspace_id=NEW.workspace_id AND release_id=NEW.id)
        OR EXISTS(SELECT 1 FROM release_proposals rp LEFT JOIN production_targets t ON t.workspace_id=rp.workspace_id AND t.operation_id=rp.operation_id AND t.version=rp.production_version AND t.proposal_id=rp.proposal_id
            WHERE rp.workspace_id=NEW.workspace_id AND rp.release_id=NEW.id AND (t.proposal_id IS NULL OR rp.operation_id<>c.operation_id OR rp.production_version<>c.operation_version
                OR rp.proposal_content_digest<>t.content_digest OR rp.role<>CASE WHEN NEW.production_rollback_depth=0 THEN 'applied' ELSE 'reverted' END)) THEN
        RAISE EXCEPTION 'production release requires complete member and before pin attribution' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_release_commit_guard AFTER INSERT ON releases DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_release_commit();

CREATE FUNCTION guard_production_release_append() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS(SELECT 1 FROM releases WHERE workspace_id=NEW.workspace_id AND id=NEW.release_id AND production_root_release_id IS NOT NULL AND xmin=pg_current_xact_id()::xid) THEN
        RAISE EXCEPTION 'production release history must be written with its protected release' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER production_release_proposals_append BEFORE INSERT ON release_proposals FOR EACH ROW EXECUTE FUNCTION guard_production_release_append();
CREATE TRIGGER production_release_manifests_append BEFORE INSERT ON production_release_manifests FOR EACH ROW EXECUTE FUNCTION guard_production_release_append();
CREATE TRIGGER production_release_before_pins_append BEFORE INSERT ON production_release_before_pins FOR EACH ROW EXECUTE FUNCTION guard_production_release_append();
CREATE TRIGGER production_release_binding_inputs_append BEFORE INSERT ON production_release_binding_inputs FOR EACH ROW EXECUTE FUNCTION guard_production_release_append();

CREATE FUNCTION production_sorted_json_array(value jsonb) RETURNS jsonb LANGUAGE sql IMMUTABLE STRICT AS $$
    SELECT COALESCE(jsonb_agg(item ORDER BY item::text),'[]'::jsonb) FROM jsonb_array_elements(value) item;
$$;

CREATE FUNCTION guard_production_seal_contents() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE output jsonb; run_row validation_runs%ROWTYPE; actual jsonb; policy_count integer;
BEGIN
    FOR output IN SELECT value FROM jsonb_array_elements(NEW.results_json) LOOP
        SELECT * INTO run_row FROM validation_runs r WHERE r.workspace_id=NEW.workspace_id AND r.production_operation_id=NEW.operation_id
            AND r.production_version=NEW.production_version AND r.production_attempt_no=NEW.attempt_no
            AND production_typeid('prp',r.proposal_id)=output->'check'->>'proposalId' AND r.validator_id=output->'check'->>'validatorId'
            AND r.validator_version=output->'check'->>'validatorVersion';
        IF NOT FOUND OR run_row.status IS DISTINCT FROM output->>'status' THEN
            RAISE EXCEPTION 'sealed output must match its real validation run' USING ERRCODE='23514';
        END IF;
        SELECT COALESCE(jsonb_agg(jsonb_build_object('Severity',severity,'Code',code,'Message',message,'Details',details,'InputDigest',input_digest)),'[]'::jsonb)
            INTO actual FROM validation_results WHERE workspace_id=NEW.workspace_id AND validation_run_id=run_row.id;
        IF jsonb_array_length(actual)=0 OR jsonb_array_length(actual)>256 OR production_sorted_json_array(actual) IS DISTINCT FROM production_sorted_json_array(output->'findings')
            OR EXISTS(SELECT 1 FROM validation_results WHERE workspace_id=NEW.workspace_id AND validation_run_id=run_row.id AND input_digest IS DISTINCT FROM output->>'inputDigest')
            OR (run_row.status='succeeded') IS DISTINCT FROM NOT EXISTS(SELECT 1 FROM validation_results WHERE workspace_id=NEW.workspace_id AND validation_run_id=run_row.id AND severity='blocker') THEN
            RAISE EXCEPTION 'sealed output must match complete findings and their status' USING ERRCODE='23514';
        END IF;
    END LOOP;
    SELECT count(*) INTO policy_count FROM production_targets t JOIN proposals p ON p.workspace_id=t.workspace_id AND p.id=t.proposal_id
        JOIN policy_decisions d ON d.workspace_id=p.workspace_id AND d.id=p.policy_decision_id
        WHERE t.workspace_id=NEW.workspace_id AND t.operation_id=NEW.operation_id AND t.version=NEW.production_version
        AND NEW.policy_ids @> jsonb_build_array(d.id::text);
    IF jsonb_array_length(NEW.policy_ids)<>policy_count OR (EXISTS(SELECT 1 FROM production_validation_attempts a WHERE a.workspace_id=NEW.workspace_id AND a.operation_id=NEW.operation_id AND a.production_version=NEW.production_version AND a.attempt_no=NEW.attempt_no AND a.status='succeeded')
        AND policy_count<>(SELECT count(*) FROM production_targets WHERE workspace_id=NEW.workspace_id AND operation_id=NEW.operation_id AND version=NEW.production_version AND proposal_id IS NOT NULL)) THEN
        RAISE EXCEPTION 'sealed policy IDs must be actual member policy decisions' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_seal_contents_guard AFTER INSERT ON production_validation_seals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_seal_contents();

CREATE FUNCTION guard_production_finding_append() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM validation_runs r WHERE r.workspace_id=NEW.workspace_id AND r.id=NEW.validation_run_id AND r.production_operation_id IS NOT NULL
        AND NOT EXISTS(SELECT 1 FROM production_validation_seals s WHERE s.workspace_id=r.workspace_id AND s.operation_id=r.production_operation_id
            AND s.production_version=r.production_version AND s.attempt_no=r.production_attempt_no AND s.xmin=pg_current_xact_id()::xid)) THEN
        RAISE EXCEPTION 'production findings must be sealed with their complete attempt' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_finding_append_guard AFTER INSERT ON validation_results DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_finding_append();

CREATE FUNCTION guard_production_attribution() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE rel releases%ROWTYPE; member proposals%ROWTYPE; adopted_review jsonb; decision reviews%ROWTYPE; command_actor uuid;
BEGIN
    SELECT * INTO rel FROM releases WHERE workspace_id=NEW.workspace_id AND id=NEW.release_id;
    SELECT * INTO member FROM proposals WHERE workspace_id=NEW.workspace_id AND id=NEW.proposal_id;
    SELECT principal_id INTO command_actor FROM production_commands WHERE workspace_id=NEW.workspace_id AND result_id=NEW.release_id AND command_kind IN ('publish','rollback');
    IF command_actor IS NULL OR rel.published_by IS DISTINCT FROM production_typeid('prn',command_actor)
        OR member.created_by IS DISTINCT FROM production_typeid('prn',NEW.author_principal_id)
        OR member.production_operation_id IS DISTINCT FROM NEW.operation_id OR member.production_version IS DISTINCT FROM NEW.production_version
        OR jsonb_typeof(NEW.review_ids)<>'array' OR jsonb_array_length(NEW.review_ids) NOT BETWEEN 1 AND 256
        OR EXISTS(SELECT 1 FROM production_contributors WHERE workspace_id=NEW.workspace_id AND operation_id=NEW.operation_id AND principal_id=command_actor)
        OR NOT EXISTS(SELECT 1 FROM production_versions v JOIN production_validation_seals s ON s.workspace_id=v.workspace_id AND s.operation_id=v.operation_id AND s.production_version=v.version
            WHERE v.workspace_id=NEW.workspace_id AND v.operation_id=NEW.operation_id AND v.version=NEW.production_version AND v.set_digest=NEW.set_digest
            AND s.attempt_no=NEW.attempt_no AND s.validation_digest=NEW.validation_digest AND (rel.production_rollback_depth>0 OR v.active_validation_attempt_no=NEW.attempt_no)) THEN
        RAISE EXCEPTION 'release attribution must bind actual validated members and independent publisher' USING ERRCODE='23514';
    END IF;
    FOR adopted_review IN SELECT value FROM jsonb_array_elements(NEW.review_ids) LOOP
        SELECT r.* INTO decision FROM reviews r JOIN production_review_bindings b ON b.workspace_id=r.workspace_id AND b.review_id=r.id
            WHERE r.workspace_id=NEW.workspace_id AND production_typeid('rvw',r.id)=(adopted_review#>>'{}') AND r.proposal_id=NEW.proposal_id
            AND r.channel='expert' AND r.decision='approved' AND b.operation_id=NEW.operation_id AND b.production_version=NEW.production_version
            AND b.attempt_no=NEW.attempt_no AND b.set_digest=NEW.set_digest AND b.validation_digest=NEW.validation_digest;
        IF NOT FOUND OR decision.reviewer_principal_id=command_actor THEN
            RAISE EXCEPTION 'release attribution requires exact adopted independent reviews' USING ERRCODE='23514';
        END IF;
    END LOOP;
    IF rel.production_rollback_depth>0 AND NOT EXISTS(SELECT 1 FROM release_proposals p WHERE p.workspace_id=NEW.workspace_id AND p.release_id=rel.production_rollback_parent_id
        AND p.proposal_id=NEW.proposal_id AND p.operation_id=NEW.operation_id AND p.production_version=NEW.production_version AND p.set_digest=NEW.set_digest
        AND p.proposal_content_digest=NEW.proposal_content_digest AND p.author_principal_id=NEW.author_principal_id AND p.attempt_no=NEW.attempt_no AND p.validation_digest=NEW.validation_digest AND p.review_ids=NEW.review_ids) THEN
        RAISE EXCEPTION 'rollback attribution must preserve its parents complete member proof' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_attribution_guard AFTER INSERT ON release_proposals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_attribution();

CREATE FUNCTION production_reintroduction_matches(p_w uuid, p_kind text, p_target uuid, p_identity_key text, p_original_operation uuid, p_declaration jsonb) RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT EXISTS(
    SELECT 1 FROM releases c JOIN production_release_integrity ci ON ci.workspace_id=c.workspace_id AND ci.release_id=c.id
    JOIN release_proposals rp ON rp.workspace_id=c.workspace_id AND rp.release_id=c.id AND rp.role='applied'
    JOIN production_targets t ON t.workspace_id=rp.workspace_id AND t.operation_id=rp.operation_id AND t.version=rp.production_version AND t.proposal_id=rp.proposal_id
    JOIN production_release_before_pins cp ON cp.workspace_id=c.workspace_id AND cp.release_id=c.id AND cp.target_kind=t.kind AND cp.target_id=t.target_id AND cp.presence='absent'
    JOIN releases a ON a.workspace_id=c.workspace_id AND a.production_root_release_id=c.id AND a.production_rollback_depth>0 AND a.sequence>c.sequence
    JOIN production_release_integrity ai ON ai.workspace_id=a.workspace_id AND ai.release_id=a.id
    JOIN production_release_before_pins ap ON ap.workspace_id=a.workspace_id AND ap.release_id=a.id AND ap.target_kind=t.kind AND ap.target_id=t.target_id AND ap.presence='present'
    JOIN LATERAL(SELECT * FROM releases WHERE workspace_id=p_w ORDER BY sequence DESC LIMIT 1) h ON h.sequence>=a.sequence
    WHERE c.workspace_id=p_w AND c.production_rollback_depth=0 AND t.kind=p_kind AND t.target_id=p_target AND t.identity_key=p_identity_key AND t.intent='create'
    AND p_declaration->>'intent'='create' AND p_declaration->>'identityKey'=p_identity_key AND p_declaration->>'kind'=p_kind
    AND p_declaration->'reuseIdentity'->>'targetId'=production_typeid(CASE p_kind WHEN 'semantic_asset' THEN 'ast' WHEN 'physical_binding' THEN 'phb' WHEN 'model_grain' THEN 'mgn' WHEN 'entity_key' THEN 'eky' WHEN 'join_contract' THEN 'jct' END,p_target)
    AND p_declaration->'reuseIdentity'->>'creationOperationId'=production_typeid('prodop',p_original_operation)
    AND p_declaration->'reuseIdentity'->>'creationReleaseId'=production_typeid('rls',c.id)
    AND p_declaration->'reuseIdentity'->>'absenceReleaseId'=production_typeid('rls',a.id)
    AND p_declaration->'reuseIdentity'->'expectedHead'=jsonb_build_object('presence','present','releaseId',production_typeid('rls',h.id),'manifestDigest',h.manifest_digest)
    AND EXISTS(SELECT 1 FROM production_targets original WHERE original.workspace_id=p_w AND original.operation_id=p_original_operation AND original.target_id=p_target AND original.kind=p_kind AND original.identity_key=p_identity_key AND original.intent='create')
    AND NOT EXISTS(SELECT 1 FROM releases later WHERE later.workspace_id=p_w AND later.sequence>=a.sequence AND later.sequence<=h.sequence
      AND (EXISTS(SELECT 1 FROM release_assets x WHERE x.workspace_id=p_w AND x.release_id=later.id AND p_kind='semantic_asset' AND x.asset_id=p_target)
        OR EXISTS(SELECT 1 FROM release_objects x WHERE x.workspace_id=p_w AND x.release_id=later.id AND x.object_type=p_kind AND x.object_id=p_target)))
 );
$$;

CREATE OR REPLACE FUNCTION guard_production_reservation_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'production reservation is immutable' USING ERRCODE='23514'; END IF;
    IF (to_jsonb(NEW)-'owner_operation_id') IS DISTINCT FROM (to_jsonb(OLD)-'owner_operation_id') THEN
        RAISE EXCEPTION 'production reservation identity is immutable' USING ERRCODE='23514';
    END IF;
    IF NEW.owner_operation_id IS DISTINCT FROM OLD.owner_operation_id
        AND NOT EXISTS(SELECT 1 FROM production_operations WHERE workspace_id=NEW.workspace_id AND id=NEW.owner_operation_id AND supersedes_operation_id=OLD.owner_operation_id)
        AND (EXISTS(SELECT 1 FROM proposals WHERE workspace_id=OLD.workspace_id AND production_operation_id=OLD.owner_operation_id AND state NOT IN ('released','rejected'))
          OR NOT EXISTS(SELECT 1 FROM production_versions v CROSS JOIN LATERAL jsonb_array_elements(convert_from(v.canonical_declarations,'UTF8')::jsonb) d
            WHERE v.workspace_id=NEW.workspace_id AND v.operation_id=NEW.owner_operation_id AND v.history_quality='verified' AND v.xmin=pg_current_xact_id()::xid
            AND production_reintroduction_matches(NEW.workspace_id,NEW.kind,NEW.target_id,NEW.identity_key,NEW.creation_operation_id,d))) THEN
        RAISE EXCEPTION 'production reservation requires explicit successor or trusted absence reintroduction' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;

CREATE FUNCTION guard_production_reintroduction_member() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE declaration jsonb; reservation production_identity_reservations%ROWTYPE;
BEGIN
    IF NEW.production_operation_id IS NULL OR NEW.intent<>'create' THEN RETURN NULL; END IF;
    IF NEW.reintroduction_creation_release_id IS NULL THEN
        IF EXISTS(SELECT 1 FROM release_assets WHERE workspace_id=NEW.workspace_id AND asset_id=NEW.target_object_id)
            OR EXISTS(SELECT 1 FROM release_objects WHERE workspace_id=NEW.workspace_id AND object_type=NEW.target_object_type AND object_id=NEW.target_object_id) THEN
            RAISE EXCEPTION 'previously published identity requires trusted reintroduction proof' USING ERRCODE='23514';
        END IF;
        RETURN NULL;
    END IF;
    SELECT d INTO declaration FROM production_versions v JOIN production_targets t ON t.workspace_id=v.workspace_id AND t.operation_id=v.operation_id AND t.version=v.version
        CROSS JOIN LATERAL jsonb_array_elements(convert_from(v.canonical_declarations,'UTF8')::jsonb) d
        WHERE v.workspace_id=NEW.workspace_id AND v.operation_id=NEW.production_operation_id AND v.version=NEW.production_version AND t.proposal_id=NEW.id AND d->>'localKey'=t.local_key;
    SELECT * INTO reservation FROM production_identity_reservations WHERE workspace_id=NEW.workspace_id AND kind=NEW.target_object_type AND target_id=NEW.target_object_id;
    IF NOT FOUND OR declaration IS NULL OR reservation.owner_operation_id<>NEW.production_operation_id
        OR declaration->'reuseIdentity'->>'creationReleaseId' IS DISTINCT FROM production_typeid('rls',NEW.reintroduction_creation_release_id)
        OR declaration->'reuseIdentity'->>'absenceReleaseId' IS DISTINCT FROM production_typeid('rls',NEW.reintroduction_absence_release_id)
        OR NOT production_reintroduction_matches(NEW.workspace_id,NEW.target_object_type,NEW.target_object_id,reservation.identity_key,reservation.creation_operation_id,declaration) THEN
        RAISE EXCEPTION 'production create requires exact trusted reintroduction proof' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_reintroduction_member_guard AFTER INSERT ON proposals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_reintroduction_member();

CREATE FUNCTION guard_production_release_pin() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE prior uuid; asset_pin release_assets%ROWTYPE; object_pin release_objects%ROWTYPE; snapshot jsonb; dataset source_snapshot_members%ROWTYPE;
BEGIN
    IF TG_TABLE_NAME='production_release_before_pins' THEN
        SELECT before_release_id INTO prior FROM production_release_manifests WHERE workspace_id=NEW.workspace_id AND release_id=NEW.release_id;
        IF NOT EXISTS(SELECT 1 FROM production_commands c JOIN production_targets t ON t.workspace_id=c.workspace_id AND t.operation_id=c.operation_id AND t.version=c.operation_version
            WHERE c.workspace_id=NEW.workspace_id AND c.result_id=NEW.release_id AND c.command_kind IN ('publish','rollback') AND t.kind=NEW.target_kind AND t.target_id=NEW.target_id) THEN
            RAISE EXCEPTION 'production before pin must identify an exact operation target' USING ERRCODE='23514';
        END IF;
        IF NEW.target_kind='semantic_asset' THEN
            SELECT * INTO asset_pin FROM release_assets WHERE workspace_id=NEW.workspace_id AND release_id=prior AND asset_id=NEW.target_id;
            IF (NEW.presence='present') IS DISTINCT FROM FOUND OR NEW.object_version IS NOT NULL
                OR (NEW.presence='present' AND (NEW.asset_revision_id IS DISTINCT FROM asset_pin.revision_id
                  OR NEW.content_digest IS DISTINCT FROM (SELECT content_digest FROM asset_revisions WHERE workspace_id=NEW.workspace_id AND asset_id=NEW.target_id AND id=asset_pin.revision_id))) THEN
                RAISE EXCEPTION 'production asset before pin differs from actual prior manifest' USING ERRCODE='23514';
            END IF;
        ELSE
            SELECT * INTO object_pin FROM release_objects WHERE workspace_id=NEW.workspace_id AND release_id=prior AND object_type=NEW.target_kind AND object_id=NEW.target_id;
            IF (NEW.presence='present') IS DISTINCT FROM FOUND OR NEW.asset_revision_id IS NOT NULL
                OR (NEW.presence='present' AND NEW.object_version IS DISTINCT FROM object_pin.version) THEN
                RAISE EXCEPTION 'production object before pin differs from actual prior manifest' USING ERRCODE='23514';
            END IF;
        END IF;
    ELSE
        SELECT payload INTO snapshot FROM release_object_snapshots WHERE workspace_id=NEW.workspace_id AND release_id=NEW.release_id AND object_type='physical_binding' AND object_id=NEW.binding_id AND version=NEW.binding_version;
        IF NOT FOUND THEN RAISE EXCEPTION 'production binding inputs require exact published snapshot' USING ERRCODE='23514'; END IF;
        SELECT * INTO dataset FROM source_snapshot_members WHERE workspace_id=NEW.workspace_id AND snapshot_id=NEW.snapshot_id AND kind='dataset' AND revision_id=NEW.dataset_revision_id;
        IF NOT FOUND OR dataset.object_id::text IS DISTINCT FROM snapshot->>'dataset_id' OR jsonb_typeof(NEW.field_revision_ids)<>'array'
            OR jsonb_array_length(NEW.field_revision_ids)<>(CASE WHEN snapshot->>'field_id' IS NULL THEN 0 ELSE 1 END)
            OR EXISTS(SELECT 1 FROM jsonb_array_elements_text(NEW.field_revision_ids) f WHERE NOT EXISTS(
                SELECT 1 FROM source_snapshot_members m WHERE m.workspace_id=NEW.workspace_id AND m.snapshot_id=NEW.snapshot_id AND m.kind='field'
                AND production_typeid('pfr',m.revision_id)=f AND m.parent_object_id=dataset.object_id AND m.parent_revision_id=dataset.revision_id AND m.object_id::text=snapshot->>'field_id')) THEN
            RAISE EXCEPTION 'production binding inputs differ from exact physical parent pins' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER production_before_pin_guard AFTER INSERT ON production_release_before_pins DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_release_pin();
CREATE CONSTRAINT TRIGGER production_binding_input_guard AFTER INSERT ON production_release_binding_inputs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_release_pin();

CREATE OR REPLACE FUNCTION guard_production_reserved_proposal() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM production_identity_reservations r
       WHERE r.workspace_id=NEW.workspace_id AND r.target_id=NEW.target_object_id
         AND r.owner_operation_id IS DISTINCT FROM NEW.production_operation_id
         AND NOT (NEW.intent='update' AND NEW.production_operation_id IS NOT NULL
           AND NOT EXISTS(SELECT 1 FROM proposals p WHERE p.workspace_id=r.workspace_id AND p.production_operation_id=r.owner_operation_id AND p.state NOT IN ('released','rejected','superseded','withdrawn'))
           AND EXISTS(SELECT 1 FROM releases h WHERE h.workspace_id=NEW.workspace_id
             AND h.sequence=(SELECT max(sequence) FROM releases WHERE workspace_id=NEW.workspace_id)
             AND ((NEW.target_object_type='semantic_asset' AND EXISTS(SELECT 1 FROM release_assets a WHERE a.workspace_id=h.workspace_id AND a.release_id=h.id AND a.asset_id=NEW.target_object_id AND a.revision_id=NEW.base_revision_id))
               OR (NEW.target_object_type<>'semantic_asset' AND EXISTS(SELECT 1 FROM release_objects o WHERE o.workspace_id=h.workspace_id AND o.release_id=h.id AND o.object_type=NEW.target_object_type AND o.object_id=NEW.target_object_id AND o.version=NEW.base_object_version)))))) THEN
        RAISE EXCEPTION 'target is owned by production authoring' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;

COMMIT;

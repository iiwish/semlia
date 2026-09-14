BEGIN;

ALTER TABLE production_generation_links ALTER COLUMN model_config_revision TYPE text USING model_config_revision::text;

CREATE TABLE production_generation_requests (
    workspace_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    agent_run_id uuid NOT NULL,
    job_id uuid NOT NULL,
    input_version integer NOT NULL CHECK(input_version > 0),
    requested_by uuid NOT NULL,
    agent_principal_id uuid NOT NULL,
    model_setting_id uuid NOT NULL,
    model_config_revision text NOT NULL CHECK(length(model_config_revision) BETWEEN 1 AND 128),
    input_digest text NOT NULL CHECK(input_digest ~ '^sha256:[0-9a-f]{64}$'),
    set_digest text NOT NULL CHECK(set_digest ~ '^sha256:[0-9a-f]{64}$'),
    request_digest text NOT NULL CHECK(request_digest ~ '^sha256:[0-9a-f]{64}$'),
    prompt_digest text NOT NULL CHECK(prompt_digest ~ '^sha256:[0-9a-f]{64}$'),
    grant_digest text NOT NULL CHECK(grant_digest ~ '^sha256:[0-9a-f]{64}$'),
    canonical_request bytea NOT NULL CHECK(octet_length(canonical_request) BETWEEN 1 AND 65536),
    provider_mode text NOT NULL CHECK(provider_mode IN ('protocol_stub','actual_model')),
    status text NOT NULL CHECK(status IN ('queued','running','succeeded','failed','cancelled','outcome_unknown')),
    claim_token uuid,
    call_deadline timestamptz,
    error_code text CHECK(error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    cost_micros bigint CHECK(cost_micros >= 0),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at timestamptz,
    PRIMARY KEY(workspace_id,agent_run_id),
    UNIQUE(workspace_id,operation_id,agent_run_id),
    UNIQUE(workspace_id,job_id),
    FOREIGN KEY(workspace_id,operation_id,input_version) REFERENCES production_versions(workspace_id,operation_id,version) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,agent_run_id) REFERENCES agent_runs(workspace_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(job_id) REFERENCES jobs(id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,requested_by) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,agent_principal_id) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,model_setting_id) REFERENCES model_settings(workspace_id,id) ON DELETE RESTRICT,
    CHECK ((claim_token IS NULL) = (call_deadline IS NULL)),
    CHECK (status NOT IN ('running','succeeded','outcome_unknown') OR claim_token IS NOT NULL),
    CHECK ((status IN ('queued','running')) = (completed_at IS NULL)),
    CHECK ((status IN ('failed','cancelled','outcome_unknown')) = (error_code IS NOT NULL)),
    CHECK (completed_at IS NULL OR completed_at >= created_at),
    CHECK (jsonb_typeof(convert_from(canonical_request,'UTF8')::jsonb)='object')
);

CREATE FUNCTION protect_production_generation_request() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'production generation history is immutable' USING ERRCODE='23514'; END IF;
    IF (to_jsonb(NEW)-ARRAY['status','claim_token','call_deadline','error_code','cost_micros','completed_at']) IS DISTINCT FROM
       (to_jsonb(OLD)-ARRAY['status','claim_token','call_deadline','error_code','cost_micros','completed_at']) THEN
        RAISE EXCEPTION 'production generation input is immutable' USING ERRCODE='23514';
    END IF;
    IF NOT ((OLD.status='queued' AND NEW.status IN ('running','failed','cancelled')) OR
            (OLD.status='running' AND NEW.status IN ('succeeded','failed','outcome_unknown','cancelled'))) THEN
        RAISE EXCEPTION 'invalid production generation transition' USING ERRCODE='23514';
    END IF;
    IF OLD.claim_token IS NOT NULL AND (NEW.claim_token,NEW.call_deadline) IS DISTINCT FROM (OLD.claim_token,OLD.call_deadline) THEN
        RAISE EXCEPTION 'production generation invocation cannot be replayed' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER production_generation_request_guard BEFORE UPDATE OR DELETE ON production_generation_requests FOR EACH ROW EXECUTE FUNCTION protect_production_generation_request();

CREATE FUNCTION validate_production_generation_result() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r production_generation_requests%ROWTYPE; a agent_runs%ROWTYPE;
BEGIN
    SELECT * INTO r FROM production_generation_requests WHERE workspace_id=NEW.workspace_id AND agent_run_id=NEW.agent_run_id;
    SELECT * INTO a FROM agent_runs WHERE workspace_id=r.workspace_id AND id=r.agent_run_id;
    IF NOT EXISTS(SELECT 1 FROM jobs WHERE id=r.job_id AND workspace_id=r.workspace_id AND job_type='production.generate') THEN
        RAISE EXCEPTION 'production generation job attribution mismatch' USING ERRCODE='23514';
    END IF;
    IF a.principal_id IS DISTINCT FROM r.agent_principal_id OR a.input_hash<>r.prompt_digest OR a.config_revision<>r.model_config_revision THEN
        RAISE EXCEPTION 'production generation run attribution mismatch' USING ERRCODE='23514';
    END IF;
    IF r.status='succeeded' THEN
        IF a.status<>'succeeded' OR NOT EXISTS (
            SELECT 1 FROM production_generation_outputs o
            JOIN production_generation_links l USING(workspace_id,operation_id,agent_run_id)
            JOIN agent_steps s ON s.workspace_id=o.workspace_id AND s.agent_run_id=o.agent_run_id AND s.sequence=1 AND s.kind='model'
            WHERE o.workspace_id=r.workspace_id AND o.agent_run_id=r.agent_run_id AND o.operation_id=r.operation_id
            AND o.input_version=r.input_version AND l.input_version=r.input_version AND l.input_digest=r.input_digest
            AND l.model_config_revision=r.model_config_revision AND o.schema_version='semlia.production-suggestions/v1'
            AND l.provider_mode=CASE r.provider_mode WHEN 'actual_model' THEN 'live' ELSE 'stub' END
            AND o.output_digest=a.output_digest AND l.output_digest=o.output_digest AND s.output_hash=o.output_digest
            AND s.input_hash=r.prompt_digest AND s.error_code IS NULL
        ) THEN RAISE EXCEPTION 'production generation success requires complete output and run' USING ERRCODE='23514'; END IF;
    ELSE
        IF (r.status IN ('queued','running') AND a.status<>'running') OR
           (r.status IN ('failed','outcome_unknown') AND a.status<>'failed') OR
           (r.status='cancelled' AND a.status<>'cancelled') OR
           EXISTS(SELECT 1 FROM production_generation_outputs WHERE workspace_id=r.workspace_id AND agent_run_id=r.agent_run_id) THEN
            RAISE EXCEPTION 'production generation state mismatch' USING ERRCODE='23514';
        END IF;
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER production_generation_result_guard AFTER INSERT OR UPDATE ON production_generation_requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_production_generation_result();

COMMIT;

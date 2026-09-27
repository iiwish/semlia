BEGIN;

CREATE TABLE ask_requests (
    workspace_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 160),
    input_digest text NOT NULL CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$'),
    knowledge_digest text NOT NULL CHECK (knowledge_digest ~ '^sha256:[0-9a-f]{64}$'),
    agent_run_id uuid NOT NULL,
    claim_token uuid NOT NULL,
    call_deadline timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('running','succeeded','clarification','failed','cancelled','outcome_unknown')),
    semantic_query_id uuid,
    error_code text CHECK (error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    created_at timestamptz NOT NULL,
    completed_at timestamptz,
    PRIMARY KEY (workspace_id, requested_by, idempotency_key),
    UNIQUE (workspace_id, agent_run_id),
    FOREIGN KEY (workspace_id, requested_by) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, agent_run_id) REFERENCES agent_runs(workspace_id,id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (workspace_id, semantic_query_id) REFERENCES semantic_queries(workspace_id,id) ON DELETE RESTRICT,
    CHECK (call_deadline > created_at),
    CHECK ((status = 'running') = (completed_at IS NULL)),
    CHECK ((status = 'succeeded') = (semantic_query_id IS NOT NULL)),
    CHECK ((status IN ('failed','cancelled','outcome_unknown')) = (error_code IS NOT NULL)),
    CHECK (completed_at IS NULL OR completed_at >= created_at)
);
CREATE INDEX ask_requests_expired ON ask_requests(call_deadline) WHERE status='running';

CREATE FUNCTION protect_ask_request() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' THEN RAISE EXCEPTION 'Ask request history is immutable' USING ERRCODE='23514'; END IF;
    IF OLD.status<>'running' OR NEW.status='running' OR
       (to_jsonb(NEW)-ARRAY['status','semantic_query_id','error_code','completed_at']) IS DISTINCT FROM
       (to_jsonb(OLD)-ARRAY['status','semantic_query_id','error_code','completed_at']) THEN
        RAISE EXCEPTION 'invalid Ask request transition' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER ask_request_guard BEFORE UPDATE OR DELETE ON ask_requests FOR EACH ROW EXECUTE FUNCTION protect_ask_request();

CREATE FUNCTION validate_ask_attribution() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE r ask_requests%ROWTYPE; a agent_runs%ROWTYPE;
BEGIN
    SELECT * INTO r FROM ask_requests WHERE workspace_id=NEW.workspace_id AND agent_run_id=NEW.agent_run_id;
    SELECT * INTO a FROM agent_runs WHERE workspace_id=r.workspace_id AND id=r.agent_run_id;
    IF a.input_hash IS DISTINCT FROM r.input_digest OR
       a.status IS DISTINCT FROM (CASE WHEN r.status IN ('succeeded','clarification') THEN 'succeeded' WHEN r.status='cancelled' THEN 'cancelled' WHEN r.status='running' THEN 'running' ELSE 'failed' END) THEN
        RAISE EXCEPTION 'Ask attribution mismatch' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER ask_attribution_guard AFTER INSERT OR UPDATE ON ask_requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_ask_attribution();
COMMIT;

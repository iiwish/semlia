CREATE TABLE production_business_rule_events (
    sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    production_version integer NOT NULL,
    target_key text NOT NULL,
    action text NOT NULL CHECK (action IN ('confirm','revoke')),
    set_digest text NOT NULL CHECK (set_digest ~ '^sha256:[0-9a-f]{64}$'),
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    evidence_id uuid,
    evidence_digest text CHECK (evidence_digest ~ '^sha256:[0-9a-f]{64}$'),
    evidence_origin text NOT NULL CHECK (evidence_origin IN ('selected_evidence','human_declaration','none')),
    principal_id uuid NOT NULL,
    authorization_version bigint NOT NULL,
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 256),
    request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((action='confirm' AND evidence_id IS NOT NULL AND evidence_digest IS NOT NULL AND evidence_origin<>'none') OR
           (action='revoke' AND evidence_id IS NULL AND evidence_digest IS NULL AND evidence_origin='none')),
    UNIQUE (workspace_id,principal_id,idempotency_key),
    FOREIGN KEY (workspace_id,operation_id,production_version,target_key)
        REFERENCES production_targets(workspace_id,operation_id,version,local_key) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id,principal_id) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id,evidence_id) REFERENCES evidence_artifacts(workspace_id,id) ON DELETE RESTRICT
);
CREATE INDEX production_business_rule_latest ON production_business_rule_events
    (workspace_id,operation_id,production_version,target_key,sequence DESC);
CREATE TRIGGER production_business_rule_events_immutable BEFORE UPDATE OR DELETE ON production_business_rule_events
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE FUNCTION guard_production_business_rule_event() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM production_targets t JOIN production_versions v
        ON v.workspace_id=t.workspace_id AND v.operation_id=t.operation_id AND v.version=t.version
        JOIN principals p ON p.workspace_id=t.workspace_id AND p.id=NEW.principal_id
        JOIN production_contributors c ON c.workspace_id=t.workspace_id AND c.operation_id=t.operation_id AND c.principal_id=p.id
        WHERE t.workspace_id=NEW.workspace_id AND t.operation_id=NEW.operation_id AND t.version=NEW.production_version
        AND t.local_key=NEW.target_key AND t.kind='semantic_asset' AND t.proposal_id IS NOT NULL
        AND t.content_digest=NEW.content_digest AND v.set_digest=NEW.set_digest AND p.kind='human' AND p.status='active') THEN
        RAISE EXCEPTION 'business rule requires a bound human contributor' USING ERRCODE='23514';
    END IF;
    IF NEW.action='confirm' AND NOT EXISTS (SELECT 1 FROM evidence_artifacts e WHERE e.workspace_id=NEW.workspace_id
        AND e.id=NEW.evidence_id AND e.content_digest=NEW.evidence_digest AND e.evidence_type='declared') THEN
        RAISE EXCEPTION 'business rule requires declared evidence' USING ERRCODE='23514';
    END IF;
    RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER production_business_rule_event_guard AFTER INSERT ON production_business_rule_events
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION guard_production_business_rule_event();

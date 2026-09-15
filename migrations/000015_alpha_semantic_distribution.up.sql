BEGIN;

-- Immutable snapshots close the gap between release_objects version pins and
-- mutable governed-object rows. Future release-object inserts capture the
-- exact row in the same publishing transaction; compatible historic pins are
-- backfilled only when the current row still has the pinned version.
CREATE TABLE release_object_snapshots (
    workspace_id uuid NOT NULL,
    release_id uuid NOT NULL,
    object_type text NOT NULL CHECK (object_type IN ('physical_binding', 'model_grain', 'entity_key', 'join_contract')),
    object_id uuid NOT NULL,
    version integer NOT NULL CHECK (version > 0),
    payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    position integer NOT NULL CHECK (position > 0),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (release_id, object_type, object_id),
    UNIQUE (release_id, position),
    FOREIGN KEY (workspace_id, release_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX release_object_snapshots_object_idx
    ON release_object_snapshots (workspace_id, object_type, object_id, version);

CREATE TRIGGER release_object_snapshots_immutable
    BEFORE UPDATE OR DELETE ON release_object_snapshots
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE FUNCTION capture_release_object_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    snapshot jsonb;
BEGIN
    CASE NEW.object_type
        WHEN 'physical_binding' THEN
            SELECT to_jsonb(value) INTO snapshot FROM physical_bindings value
            WHERE value.workspace_id = NEW.workspace_id AND value.id = NEW.object_id AND value.version = NEW.version;
        WHEN 'model_grain' THEN
            SELECT to_jsonb(value) INTO snapshot FROM model_grains value
            WHERE value.workspace_id = NEW.workspace_id AND value.id = NEW.object_id AND value.version = NEW.version;
        WHEN 'entity_key' THEN
            SELECT to_jsonb(value) INTO snapshot FROM entity_keys value
            WHERE value.workspace_id = NEW.workspace_id AND value.id = NEW.object_id AND value.version = NEW.version;
        WHEN 'join_contract' THEN
            SELECT to_jsonb(value) INTO snapshot FROM join_contracts value
            WHERE value.workspace_id = NEW.workspace_id AND value.id = NEW.object_id AND value.version = NEW.version;
        ELSE
            RAISE EXCEPTION 'unsupported release object type %', NEW.object_type USING ERRCODE = '23514';
    END CASE;
    IF snapshot IS NULL THEN
        RAISE EXCEPTION 'release object version is unavailable for immutable snapshot'
        USING ERRCODE = '23514';
    END IF;
    INSERT INTO release_object_snapshots (
        workspace_id, release_id, object_type, object_id, version, payload, position, created_at
    ) VALUES (
        NEW.workspace_id, NEW.release_id, NEW.object_type, NEW.object_id, NEW.version,
        snapshot, NEW.position, NEW.created_at
    );
    RETURN NEW;
END;
$$;

INSERT INTO release_object_snapshots (
    workspace_id, release_id, object_type, object_id, version, payload, position, created_at
)
SELECT pinned.workspace_id, pinned.release_id, pinned.object_type, pinned.object_id,
       pinned.version, pinned.payload, pinned.position, pinned.created_at
FROM (
    SELECT entry.*, to_jsonb(value) AS payload
    FROM release_objects entry JOIN physical_bindings value
      ON entry.object_type = 'physical_binding' AND value.workspace_id = entry.workspace_id
     AND value.id = entry.object_id AND value.version = entry.version
    UNION ALL
    SELECT entry.*, to_jsonb(value) AS payload
    FROM release_objects entry JOIN model_grains value
      ON entry.object_type = 'model_grain' AND value.workspace_id = entry.workspace_id
     AND value.id = entry.object_id AND value.version = entry.version
    UNION ALL
    SELECT entry.*, to_jsonb(value) AS payload
    FROM release_objects entry JOIN entity_keys value
      ON entry.object_type = 'entity_key' AND value.workspace_id = entry.workspace_id
     AND value.id = entry.object_id AND value.version = entry.version
    UNION ALL
    SELECT entry.*, to_jsonb(value) AS payload
    FROM release_objects entry JOIN join_contracts value
      ON entry.object_type = 'join_contract' AND value.workspace_id = entry.workspace_id
     AND value.id = entry.object_id AND value.version = entry.version
) pinned;

CREATE TRIGGER release_objects_capture_snapshot
    AFTER INSERT ON release_objects
    FOR EACH ROW EXECUTE FUNCTION capture_release_object_snapshot();

CREATE TABLE consumers (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    stable_key text NOT NULL CHECK (stable_key ~ '^[a-z0-9][a-z0-9_.-]{0,119}$'),
    name text NOT NULL CHECK (name <> '' AND length(name) <= 160),
    kind text NOT NULL CHECK (kind IN ('application', 'agent', 'human')),
    status text NOT NULL CHECK (status IN ('active', 'suspended', 'revoked')),
    owner_principal_ref text NOT NULL CHECK (owner_principal_ref <> '' AND length(owner_principal_ref) <= 256),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, stable_key)
);

CREATE INDEX consumers_workspace_updated_idx ON consumers (workspace_id, updated_at DESC, id);

CREATE TABLE consumer_bindings (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    consumer_id uuid NOT NULL,
    environment text NOT NULL CHECK (environment ~ '^[a-z0-9][a-z0-9_.-]{0,79}$'),
    purpose text NOT NULL CHECK (purpose <> '' AND length(purpose) <= 500),
    mode text NOT NULL CHECK (mode IN ('current', 'pinned')),
    release_id uuid,
    compatibility_constraint jsonb NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(compatibility_constraint) = 'object'),
    expires_at timestamptz,
    status text NOT NULL CHECK (status IN ('active', 'suspended', 'revoked')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, consumer_id, environment),
    FOREIGN KEY (workspace_id, consumer_id)
        REFERENCES consumers (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, release_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT consumer_bindings_mode_release CHECK (
        (mode = 'current' AND release_id IS NULL) OR (mode = 'pinned' AND release_id IS NOT NULL)
    )
);

CREATE INDEX consumer_bindings_workspace_updated_idx
    ON consumer_bindings (workspace_id, updated_at DESC, id);

CREATE FUNCTION enforce_consumer_binding_version_bump()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.consumer_id IS DISTINCT FROM OLD.consumer_id OR NEW.environment IS DISTINCT FROM OLD.environment
       OR NEW.created_at IS DISTINCT FROM OLD.created_at OR NEW.version <> OLD.version + 1
       OR NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'consumer binding identity is frozen and updates must bump version once'
        USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER consumer_bindings_version_bump
    BEFORE UPDATE ON consumer_bindings
    FOR EACH ROW EXECUTE FUNCTION enforce_consumer_binding_version_bump();

CREATE TABLE semantic_queries (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    principal_ref text NOT NULL CHECK (principal_ref <> '' AND length(principal_ref) <= 256),
    consumer_id uuid,
    binding_id uuid,
    selected_release_id uuid,
    schema_version text NOT NULL CHECK (schema_version = '1.0.0'),
    resolver_version text NOT NULL CHECK (resolver_version <> '' AND length(resolver_version) <= 80),
    canonical_request jsonb NOT NULL CHECK (jsonb_typeof(canonical_request) = 'object'),
    request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
    channel text NOT NULL CHECK (channel IN ('api', 'ask', 'agent', 'system')),
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    idempotency_key text NOT NULL CHECK (idempotency_key <> '' AND length(idempotency_key) <= 160),
    outcome text NOT NULL CHECK (outcome IN ('resolved', 'refused')),
    created_at timestamptz NOT NULL,
    finalized_at timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, principal_ref, idempotency_key),
    FOREIGN KEY (workspace_id, consumer_id)
        REFERENCES consumers (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, binding_id)
        REFERENCES consumer_bindings (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, selected_release_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX semantic_queries_workspace_created_idx
    ON semantic_queries (workspace_id, created_at DESC, id);

CREATE TRIGGER semantic_queries_immutable
    BEFORE UPDATE OR DELETE ON semantic_queries
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE resolved_semantic_plans (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    query_id uuid NOT NULL,
    release_id uuid NOT NULL,
    resolver_version text NOT NULL CHECK (resolver_version <> '' AND length(resolver_version) <= 80),
    canonical_plan jsonb NOT NULL CHECK (jsonb_typeof(canonical_plan) = 'object'),
    plan_digest text NOT NULL CHECK (plan_digest ~ '^sha256:[0-9a-f]{64}$'),
    execution_status text NOT NULL CHECK (execution_status IN ('not_configured', 'ready')),
    created_at timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, query_id, resolver_version),
    FOREIGN KEY (workspace_id, query_id)
        REFERENCES semantic_queries (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, release_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX resolved_semantic_plans_workspace_created_idx
    ON resolved_semantic_plans (workspace_id, created_at DESC, id);

CREATE TRIGGER resolved_semantic_plans_immutable
    BEFORE UPDATE OR DELETE ON resolved_semantic_plans
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE semantic_refusals (
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    query_id uuid PRIMARY KEY,
    reason_code text NOT NULL CHECK (reason_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    authorized_candidate_ids jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(authorized_candidate_ids) = 'array'),
    clarification text NOT NULL CHECK (clarification <> '' AND length(clarification) <= 1000),
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    created_at timestamptz NOT NULL,
    UNIQUE (workspace_id, query_id),
    FOREIGN KEY (workspace_id, query_id)
        REFERENCES semantic_queries (workspace_id, id) ON DELETE RESTRICT
);

CREATE TRIGGER semantic_refusals_immutable
    BEFORE UPDATE OR DELETE ON semantic_refusals
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE query_validation_runs (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    query_id uuid NOT NULL,
    plan_id uuid,
    validator text NOT NULL CHECK (validator <> '' AND length(validator) <= 120),
    validator_version text NOT NULL CHECK (validator_version <> '' AND length(validator_version) <= 80),
    input_digest text NOT NULL CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$'),
    status text NOT NULL CHECK (status IN ('passed', 'failed')),
    created_at timestamptz NOT NULL,
    completed_at timestamptz NOT NULL,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, query_id, validator, validator_version),
    FOREIGN KEY (workspace_id, query_id)
        REFERENCES semantic_queries (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, plan_id)
        REFERENCES resolved_semantic_plans (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE query_validation_results (
    validation_run_id uuid NOT NULL,
    position integer NOT NULL CHECK (position > 0),
    severity text NOT NULL CHECK (severity IN ('info', 'warning', 'blocker')),
    code text NOT NULL CHECK (code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    message text NOT NULL CHECK (message <> '' AND length(message) <= 1000),
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    PRIMARY KEY (validation_run_id, position),
    FOREIGN KEY (validation_run_id) REFERENCES query_validation_runs (id) ON DELETE RESTRICT
);

CREATE TRIGGER query_validation_runs_immutable
    BEFORE UPDATE OR DELETE ON query_validation_runs
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER query_validation_results_immutable
    BEFORE UPDATE OR DELETE ON query_validation_results
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE semantic_resolution_events (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    query_id uuid NOT NULL,
    principal_ref text NOT NULL CHECK (principal_ref <> '' AND length(principal_ref) <= 256),
    consumer_id uuid,
    binding_id uuid,
    release_id uuid,
    channel text NOT NULL CHECK (channel IN ('api', 'ask', 'agent', 'system')),
    outcome text NOT NULL CHECK (outcome IN ('resolved', 'refused')),
    reason_code text CHECK (reason_code IS NULL OR reason_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
    occurred_at timestamptz NOT NULL,
    UNIQUE (workspace_id, query_id),
    FOREIGN KEY (workspace_id, query_id)
        REFERENCES semantic_queries (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX semantic_resolution_events_workspace_occurred_idx
    ON semantic_resolution_events (workspace_id, occurred_at DESC, id);

CREATE TRIGGER semantic_resolution_events_immutable
    BEFORE UPDATE OR DELETE ON semantic_resolution_events
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

COMMIT;

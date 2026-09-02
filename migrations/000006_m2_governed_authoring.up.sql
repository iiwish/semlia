BEGIN;

-- M2 governed authoring (docs/specs/m2-governed-authoring/data-model.md;
-- SSOT §7.5, §8.2, §8.6, §11.2): proposals, structured change-sets, reviews,
-- validation runs/results, recomputable policy decisions, immutable releases
-- and the §8.6 agent-behavior contract. Workflow state lives on proposals;
-- asset lifecycle stays on semantic_assets — never one enum (§4.8).

-- Every AI write is attributable to one agent run recording model, config
-- revision and hashes only (SSOT §8.6: no raw prompts, no credentials).
CREATE TABLE agent_runs (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    principal_id uuid,
    model text NOT NULL CHECK (model <> '' AND length(model) <= 128),
    config_revision text NOT NULL CHECK (config_revision <> '' AND length(config_revision) <= 128),
    input_hash text NOT NULL CHECK (input_hash ~ '^sha256:[0-9a-f]{64}$'),
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled')),
    output_digest text CHECK (output_digest IS NULL OR output_digest ~ '^sha256:[0-9a-f]{64}$'),
    cost_micros bigint NOT NULL DEFAULT 0 CHECK (cost_micros >= 0),
    started_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at timestamptz,
    duration_ms bigint CHECK (duration_ms IS NULL OR duration_ms >= 0),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_runs_finish_state CHECK ((status = 'running') = (finished_at IS NULL)),
    CONSTRAINT agent_runs_output_state CHECK ((status = 'succeeded') = (output_digest IS NOT NULL)),
    CONSTRAINT agent_runs_duration CHECK ((finished_at IS NULL) = (duration_ms IS NULL)),
    CONSTRAINT agent_runs_finish_time CHECK (finished_at IS NULL OR finished_at >= started_at)
);

CREATE INDEX agent_runs_workspace_created_idx
    ON agent_runs (workspace_id, created_at DESC, id);

CREATE TABLE agent_steps (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    agent_run_id uuid NOT NULL,
    sequence integer NOT NULL CHECK (sequence > 0),
    kind text NOT NULL CHECK (kind IN ('model', 'tool')),
    tool_name text CHECK (tool_name IS NULL OR (tool_name <> '' AND length(tool_name) <= 128)),
    input_hash text NOT NULL CHECK (input_hash ~ '^sha256:[0-9a-f]{64}$'),
    output_hash text NOT NULL CHECK (output_hash ~ '^sha256:[0-9a-f]{64}$'),
    error_code text CHECK (error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (agent_run_id, sequence),
    FOREIGN KEY (workspace_id, agent_run_id)
        REFERENCES agent_runs (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT agent_steps_tool_name CHECK ((kind = 'tool') = (tool_name IS NOT NULL))
);

CREATE INDEX agent_steps_run_idx ON agent_steps (agent_run_id, sequence);

CREATE TRIGGER agent_steps_immutable
    BEFORE UPDATE OR DELETE ON agent_steps
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

-- Proposals walk draft -> proposed -> validating -> in_review -> released |
-- rejected (SSOT §7.5). The transition table is enforced in the domain layer
-- and mirrored by the trigger below as defense in depth. released and rejected
-- are terminal for the proposal aggregate.
CREATE TABLE proposals (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    asset_id uuid,
    base_revision_id uuid,
    target_object_type text NOT NULL DEFAULT 'semantic_asset' CHECK (target_object_type IN (
        'semantic_asset', 'model_grain', 'entity_key', 'join_contract'
    )),
    target_object_id uuid NOT NULL,
    state text NOT NULL DEFAULT 'draft' CHECK (state IN (
        'draft', 'proposed', 'validating', 'in_review', 'released', 'rejected'
    )),
    title text NOT NULL CHECK (title <> '' AND length(title) <= 256),
    summary text NOT NULL DEFAULT '' CHECK (length(summary) <= 4096),
    reason text NOT NULL DEFAULT '' CHECK (length(reason) <= 4096),
    risk_level text CHECK (risk_level IS NULL OR risk_level IN ('low', 'medium', 'high')),
    policy_decision_id uuid,
    agent_run_id uuid,
    created_by text NOT NULL CHECK (created_by <> '' AND length(created_by) <= 256),
    submitted_at timestamptz,
    decided_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, base_revision_id)
        REFERENCES asset_revisions (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, agent_run_id)
        REFERENCES agent_runs (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT proposals_target_shape CHECK (
        (
            target_object_type = 'semantic_asset'
            AND asset_id IS NOT NULL AND asset_id = target_object_id AND base_revision_id IS NOT NULL
        ) OR (
            target_object_type <> 'semantic_asset' AND asset_id IS NULL AND base_revision_id IS NULL
        )
    ),
    CONSTRAINT proposals_submission_time CHECK ((state = 'draft') = (submitted_at IS NULL)),
    CONSTRAINT proposals_decision_time CHECK (
        (state IN ('released', 'rejected')) = (decided_at IS NOT NULL)
    )
);

CREATE INDEX proposals_workspace_state_idx
    ON proposals (workspace_id, state, created_at DESC, id);
CREATE INDEX proposals_asset_idx ON proposals (workspace_id, asset_id, id);

CREATE FUNCTION validate_proposal_state_transition()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.state IN ('released', 'rejected') THEN
        RAISE EXCEPTION 'terminal proposals are frozen' USING ERRCODE = '23514';
    END IF;
    IF NEW.state <> OLD.state AND (OLD.state, NEW.state) NOT IN (
        ('draft', 'proposed'), ('draft', 'rejected'),
        ('proposed', 'validating'), ('proposed', 'rejected'),
        ('validating', 'in_review'), ('validating', 'rejected'),
        ('in_review', 'released'), ('in_review', 'rejected')
    ) THEN
        RAISE EXCEPTION 'illegal proposal state transition % -> %', OLD.state, NEW.state USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER proposals_validate_state_transition
    BEFORE UPDATE ON proposals
    FOR EACH ROW EXECUTE FUNCTION validate_proposal_state_transition();

-- The structured patch against the released baseline revision. Rows are
-- freely editable while the proposal is a draft and frozen the moment the
-- proposal leaves draft (data-model.md: immutable once submitted).
CREATE TABLE proposal_changes (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    proposal_id uuid NOT NULL,
    field_path text NOT NULL CHECK (field_path <> '' AND length(field_path) <= 512),
    op text NOT NULL CHECK (op IN ('add', 'update', 'remove')),
    before_digest text CHECK (before_digest IS NULL OR before_digest ~ '^sha256:[0-9a-f]{64}$'),
    after_digest text CHECK (after_digest IS NULL OR after_digest ~ '^sha256:[0-9a-f]{64}$'),
    before_value jsonb,
    after_value jsonb,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (proposal_id, field_path),
    FOREIGN KEY (workspace_id, proposal_id)
        REFERENCES proposals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT proposal_changes_update_shape CHECK (
        op <> 'update' OR (before_digest IS NOT NULL AND after_digest IS NOT NULL)
    ),
    CONSTRAINT proposal_changes_add_shape CHECK (
        op <> 'add' OR (before_digest IS NULL AND before_value IS NULL AND after_digest IS NOT NULL)
    ),
    CONSTRAINT proposal_changes_remove_shape CHECK (
        op <> 'remove' OR (after_digest IS NULL AND after_value IS NULL AND before_digest IS NOT NULL)
    ),
    CONSTRAINT proposal_changes_digest_value CHECK (
        (before_digest IS NULL) = (before_value IS NULL)
        AND (after_digest IS NULL) = (after_value IS NULL)
    )
);

CREATE INDEX proposal_changes_proposal_idx ON proposal_changes (proposal_id, field_path);

CREATE FUNCTION guard_proposal_change_set()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    parent_state text;
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT state INTO parent_state FROM proposals WHERE id = OLD.proposal_id;
    ELSE
        SELECT state INTO parent_state FROM proposals WHERE id = NEW.proposal_id;
    END IF;
    IF parent_state IS NULL OR parent_state <> 'draft' THEN
        RAISE EXCEPTION 'proposal change-set is frozen once submitted' USING ERRCODE = '55000';
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE TRIGGER proposal_changes_guard
    BEFORE INSERT OR UPDATE OR DELETE ON proposal_changes
    FOR EACH ROW EXECUTE FUNCTION guard_proposal_change_set();

-- Review facts (SSOT §8.4 three-channel model). Immutable audit records.
CREATE TABLE reviews (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    proposal_id uuid NOT NULL,
    reviewer_principal_id uuid NOT NULL,
    channel text NOT NULL CHECK (channel IN ('automatic', 'batch', 'expert')),
    decision text NOT NULL CHECK (decision IN ('approved', 'rejected', 'changes_requested')),
    note text NOT NULL DEFAULT '' CHECK (length(note) <= 4096),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, proposal_id)
        REFERENCES proposals (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, reviewer_principal_id)
        REFERENCES principals (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX reviews_proposal_idx ON reviews (proposal_id, created_at DESC, id);

CREATE TRIGGER reviews_immutable
    BEFORE UPDATE OR DELETE ON reviews
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE validation_runs (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    proposal_id uuid NOT NULL,
    validator_id text NOT NULL CHECK (validator_id ~ '^[a-z][a-z0-9_.]{1,63}$'),
    validator_version text NOT NULL CHECK (validator_version ~ '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'),
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed', 'cancelled')),
    started_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at timestamptz,
    UNIQUE (workspace_id, id),
    UNIQUE (proposal_id, validator_id, validator_version),
    FOREIGN KEY (workspace_id, proposal_id)
        REFERENCES proposals (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT validation_runs_finish_state CHECK ((status = 'running') = (finished_at IS NULL)),
    CONSTRAINT validation_runs_finish_time CHECK (finished_at IS NULL OR finished_at >= started_at)
);

CREATE INDEX validation_runs_proposal_validator_idx
    ON validation_runs (proposal_id, validator_id);

CREATE TABLE validation_results (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    validation_run_id uuid NOT NULL,
    severity text NOT NULL CHECK (severity IN ('blocker', 'warning', 'info', 'not_applicable')),
    code text NOT NULL CHECK (code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    message text NOT NULL CHECK (length(message) <= 2048),
    input_digest text NOT NULL CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$'),
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, validation_run_id)
        REFERENCES validation_runs (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX validation_results_run_idx ON validation_results (validation_run_id, severity);
CREATE INDEX validation_results_workspace_code_idx
    ON validation_results (workspace_id, code);

CREATE TRIGGER validation_results_immutable
    BEFORE UPDATE OR DELETE ON validation_results
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

-- Versioned risk decisions over canonical inputs (SSOT §8.2): identical
-- inputs under one rule version must reproduce the identical decision, so the
-- digest and the rule output are deterministic and the row is immutable.
CREATE TABLE policy_decisions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    proposal_id uuid NOT NULL,
    rule_version text NOT NULL CHECK (rule_version ~ '^[0-9]+\.[0-9]+$'),
    inputs jsonb NOT NULL CHECK (jsonb_typeof(inputs) = 'object'),
    inputs_digest text NOT NULL CHECK (inputs_digest ~ '^sha256:[0-9a-f]{64}$'),
    matched_policy text NOT NULL CHECK (matched_policy <> '' AND length(matched_policy) <= 128),
    risk_level text NOT NULL CHECK (risk_level IN ('low', 'medium', 'high')),
    routing text NOT NULL CHECK (routing IN ('expert', 'batch')),
    reason_code text NOT NULL CHECK (reason_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    decided_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (proposal_id, rule_version, inputs_digest),
    FOREIGN KEY (workspace_id, proposal_id)
        REFERENCES proposals (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX policy_decisions_inputs_digest_idx ON policy_decisions (inputs_digest);
CREATE INDEX policy_decisions_proposal_idx
    ON policy_decisions (proposal_id, decided_at DESC, id);

CREATE TRIGGER policy_decisions_immutable
    BEFORE UPDATE OR DELETE ON policy_decisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

ALTER TABLE proposals
    ADD CONSTRAINT proposals_policy_decision_fkey
    FOREIGN KEY (workspace_id, policy_decision_id)
    REFERENCES policy_decisions (workspace_id, id) ON DELETE RESTRICT;

-- Immutable release manifest (P-003, SSOT §11.4). A rollback never mutates a
-- release: it publishes a NEW release row whose rolled_back_to_release_id
-- references the release being rolled back.
CREATE TABLE releases (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    sequence bigint NOT NULL CHECK (sequence > 0),
    manifest_digest text NOT NULL CHECK (manifest_digest ~ '^sha256:[0-9a-f]{64}$'),
    state text NOT NULL DEFAULT 'published' CHECK (state IN ('published', 'rolled_back')),
    rolled_back_to_release_id uuid,
    published_by text NOT NULL CHECK (published_by <> '' AND length(published_by) <= 256),
    published_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, sequence),
    FOREIGN KEY (workspace_id, rolled_back_to_release_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT releases_rollback_target CHECK (
        rolled_back_to_release_id IS NULL OR rolled_back_to_release_id <> id
    )
);

CREATE INDEX releases_workspace_published_idx
    ON releases (workspace_id, published_at DESC, id);

CREATE TRIGGER releases_immutable
    BEFORE UPDATE OR DELETE ON releases
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE release_assets (
    workspace_id uuid NOT NULL,
    release_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    revision_id uuid NOT NULL,
    compatibility jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(compatibility) = 'object'),
    position integer NOT NULL CHECK (position > 0),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (release_id, asset_id),
    UNIQUE (release_id, position),
    FOREIGN KEY (workspace_id, release_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, revision_id)
        REFERENCES asset_revisions (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX release_assets_asset_idx
    ON release_assets (workspace_id, asset_id, revision_id);

CREATE TRIGGER release_assets_immutable
    BEFORE UPDATE OR DELETE ON release_assets
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

COMMIT;

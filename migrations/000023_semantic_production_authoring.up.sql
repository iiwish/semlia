BEGIN;

CREATE TABLE production_operations (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    created_by uuid NOT NULL,
    current_version integer NOT NULL DEFAULT 1 CHECK (current_version >= 1),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    supersedes_operation_id uuid NULL,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, supersedes_operation_id) REFERENCES production_operations (workspace_id, id) ON DELETE RESTRICT
);
CREATE INDEX production_operations_workspace_idx ON production_operations(workspace_id, created_at DESC, id DESC);

CREATE TABLE production_versions (
    workspace_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    version integer NOT NULL CHECK (version >= 1),
    input_json jsonb NOT NULL,
    input_digest text NOT NULL CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$'),
    request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
    set_digest text NOT NULL CHECK (set_digest ~ '^sha256:[0-9a-f]{64}$'),
    frozen_at timestamptz NULL,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, operation_id, version),
    FOREIGN KEY (workspace_id, operation_id) REFERENCES production_operations (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE production_targets (
    workspace_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    version integer NOT NULL CHECK (version >= 1),
    local_key text NOT NULL CHECK (length(local_key) BETWEEN 1 AND 128),
    kind text NOT NULL CHECK (kind IN ('semantic_asset', 'physical_binding', 'model_grain', 'entity_key', 'join_contract')),
    intent text NOT NULL CHECK (intent IN ('create', 'update')),
    target_id uuid NOT NULL CHECK (substring(target_id::text, 15, 1) = '7'),
    identity_key text NULL CHECK (identity_key IS NULL OR length(identity_key) BETWEEN 1 AND 256),
    base_revision_id uuid NULL,
    base_object_version integer NULL,
    registry_write_version integer NULL,
    content_json jsonb NOT NULL,
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    proposal_id uuid NULL,
    outcome text NOT NULL CHECK (outcome IN ('proposal', 'no_change')),
    PRIMARY KEY (workspace_id, operation_id, version, local_key),
    UNIQUE (workspace_id, operation_id, version, target_id),
    FOREIGN KEY (workspace_id, operation_id, version) REFERENCES production_versions (workspace_id, operation_id, version) ON DELETE RESTRICT
);
CREATE INDEX production_targets_proposal_idx ON production_targets(workspace_id, proposal_id);

CREATE TABLE production_candidate_links (
    workspace_id uuid NOT NULL,
    candidate_id uuid NOT NULL,
    candidate_digest text NOT NULL CHECK (candidate_digest ~ '^sha256:[0-9a-f]{64}$'),
    operation_id uuid NOT NULL,
    version integer NOT NULL,
    local_key text NOT NULL,
    decision_id uuid NULL,
    is_primary boolean NOT NULL DEFAULT false,
    PRIMARY KEY (workspace_id, candidate_id, candidate_digest, operation_id, version, local_key),
    FOREIGN KEY (workspace_id, operation_id, version, local_key) REFERENCES production_targets (workspace_id, operation_id, version, local_key) ON DELETE RESTRICT
);

CREATE TABLE production_contributors (
    workspace_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('author', 'editor', 'initiator', 'agent')),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, operation_id, principal_id),
    FOREIGN KEY (workspace_id, operation_id) REFERENCES production_operations (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE production_identity_reservations (
    workspace_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('semantic_asset', 'physical_binding', 'model_grain', 'entity_key', 'join_contract')),
    identity_key text NOT NULL CHECK (length(identity_key) BETWEEN 1 AND 256),
    target_id uuid NOT NULL CHECK (substring(target_id::text, 15, 1) = '7'),
    creation_operation_id uuid NOT NULL,
    owner_operation_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, kind, identity_key),
    UNIQUE (workspace_id, target_id),
    FOREIGN KEY (workspace_id, creation_operation_id) REFERENCES production_operations (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, owner_operation_id) REFERENCES production_operations (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE production_commands (
    workspace_id uuid NOT NULL,
    principal_id uuid NOT NULL,
    command_kind text NOT NULL CHECK (command_kind IN ('create', 'replace_draft', 'submit', 'validate', 'review', 'publish', 'generate', 'rollback')),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 256),
    request_digest text NOT NULL CHECK (request_digest ~ '^sha256:[0-9a-f]{64}$'),
    operation_id uuid NOT NULL,
    operation_version integer NOT NULL,
    result_kind text NOT NULL CHECK (result_kind IN ('operation', 'proposal', 'release')),
    result_id uuid NOT NULL,
    committed_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, principal_id, command_kind, idempotency_key),
    FOREIGN KEY (workspace_id, operation_id) REFERENCES production_operations (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE production_request_claims (
    workspace_id uuid NOT NULL,
    business_digest text NOT NULL CHECK (business_digest ~ '^sha256:[0-9a-f]{64}$'),
    operation_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, business_digest),
    FOREIGN KEY (workspace_id, operation_id) REFERENCES production_operations (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE production_generation_links (
    workspace_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    agent_run_id uuid NOT NULL,
    input_version integer NOT NULL,
    input_digest text NOT NULL CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$'),
    model_config_revision integer NOT NULL,
    output_digest text NOT NULL CHECK (output_digest ~ '^sha256:[0-9a-f]{64}$'),
    provider_mode text NOT NULL CHECK (provider_mode IN ('live', 'stub')),
    PRIMARY KEY (workspace_id, operation_id, agent_run_id),
    FOREIGN KEY (workspace_id, operation_id) REFERENCES production_operations (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE production_generation_outputs (
    workspace_id uuid NOT NULL,
    agent_run_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    input_version integer NOT NULL,
    schema_version text NOT NULL CHECK (length(schema_version) BETWEEN 1 AND 64),
    canonical_output bytea NOT NULL,
    output_digest text NOT NULL CHECK (output_digest ~ '^sha256:[0-9a-f]{64}$'),
    canonical_bytes integer NOT NULL CHECK (canonical_bytes BETWEEN 1 AND 1048576),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, agent_run_id),
    CHECK (octet_length(canonical_output) = canonical_bytes),
    FOREIGN KEY (workspace_id, operation_id, agent_run_id) REFERENCES production_generation_links (workspace_id, operation_id, agent_run_id) ON DELETE RESTRICT
);

CREATE TABLE production_generation_applications (
    workspace_id uuid NOT NULL,
    operation_id uuid NOT NULL,
    applied_version integer NOT NULL,
    agent_run_id uuid NOT NULL,
    source_version integer NOT NULL,
    source_output_digest text NOT NULL CHECK (source_output_digest ~ '^sha256:[0-9a-f]{64}$'),
    applied_content_digest text NOT NULL CHECK (applied_content_digest ~ '^sha256:[0-9a-f]{64}$'),
    canonical_delta bytea NOT NULL,
    delta_digest text NOT NULL CHECK (delta_digest ~ '^sha256:[0-9a-f]{64}$'),
    actor_principal_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, operation_id, applied_version),
    FOREIGN KEY (workspace_id, agent_run_id) REFERENCES production_generation_outputs (workspace_id, agent_run_id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, operation_id, applied_version) REFERENCES production_versions (workspace_id, operation_id, version) ON DELETE RESTRICT
);

-- Expand-first updates to proposals table
ALTER TABLE proposals
    ADD COLUMN intent text NOT NULL DEFAULT 'update' CHECK (intent IN ('create', 'update')),
    ADD COLUMN creation_content jsonb NULL,
    ADD COLUMN base_object_version integer NULL,
    ADD COLUMN production_operation_id uuid NULL,
    ADD COLUMN production_version integer NULL,
    ADD COLUMN reintroduction_creation_release_id uuid NULL,
    ADD COLUMN reintroduction_absence_release_id uuid NULL;

ALTER TABLE proposals
    ADD CONSTRAINT proposals_reintroduction_check CHECK (
        (reintroduction_creation_release_id IS NULL AND reintroduction_absence_release_id IS NULL)
        OR (reintroduction_creation_release_id IS NOT NULL AND reintroduction_absence_release_id IS NOT NULL AND intent = 'create')
    );

ALTER TABLE proposals
    ADD CONSTRAINT proposals_production_pair_check CHECK (
        (production_operation_id IS NULL AND production_version IS NULL)
        OR (production_operation_id IS NOT NULL AND production_version IS NOT NULL)
    );

ALTER TABLE proposals
    ADD CONSTRAINT proposals_production_operation_fkey
    FOREIGN KEY (workspace_id, production_operation_id, production_version)
    REFERENCES production_versions (workspace_id, operation_id, version) ON DELETE RESTRICT;

ALTER TABLE proposals DROP CONSTRAINT proposals_target_shape;
ALTER TABLE proposals ADD CONSTRAINT proposals_target_shape CHECK (
    (
        intent = 'create' AND target_object_type = 'semantic_asset'
        AND asset_id IS NOT NULL AND asset_id = target_object_id
        AND base_revision_id IS NULL AND base_object_version IS NULL
        AND creation_content IS NOT NULL
    ) OR (
        intent = 'create' AND target_object_type <> 'semantic_asset'
        AND asset_id IS NULL AND base_revision_id IS NULL
        AND base_object_version IS NULL AND creation_content IS NOT NULL
    ) OR (
        intent = 'update' AND target_object_type = 'semantic_asset'
        AND asset_id IS NOT NULL AND asset_id = target_object_id
        AND base_revision_id IS NOT NULL AND creation_content IS NULL
        AND base_object_version IS NULL
    ) OR (
        intent = 'update' AND target_object_type <> 'semantic_asset'
        AND asset_id IS NULL AND base_revision_id IS NULL
        AND creation_content IS NULL
    )
);

COMMIT;

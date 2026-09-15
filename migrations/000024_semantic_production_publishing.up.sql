BEGIN;

CREATE TABLE production_validation_attempts (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    operation_id uuid NOT NULL,
    production_version integer NOT NULL CHECK (production_version >= 1),
    attempt_no integer NOT NULL CHECK (attempt_no >= 1 AND attempt_no <= 256),
    set_digest text NOT NULL CHECK (set_digest ~ '^sha256:[0-9a-f]{64}$'),
    input_digest text NOT NULL CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$'),
    freshness_witness_json jsonb NOT NULL,
    freshness_digest text NOT NULL CHECK (freshness_digest ~ '^sha256:[0-9a-f]{64}$'),
    required_checks_json jsonb NOT NULL,
    required_checks_digest text NOT NULL CHECK (required_checks_digest ~ '^sha256:[0-9a-f]{64}$'),
    status text NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    validation_digest text NULL CHECK (validation_digest IS NULL OR validation_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at timestamptz NULL,
    PRIMARY KEY (workspace_id, operation_id, production_version, attempt_no),
    FOREIGN KEY (workspace_id, operation_id, production_version)
        REFERENCES production_versions (workspace_id, operation_id, version) ON DELETE RESTRICT
);

ALTER TABLE production_versions
    ADD COLUMN active_validation_attempt_no integer NULL,
    ADD CONSTRAINT production_versions_active_attempt_fkey
        FOREIGN KEY (workspace_id, operation_id, version, active_validation_attempt_no)
        REFERENCES production_validation_attempts (workspace_id, operation_id, production_version, attempt_no) ON DELETE RESTRICT;

ALTER TABLE validation_runs
    ADD COLUMN production_operation_id uuid NULL,
    ADD COLUMN production_version integer NULL,
    ADD COLUMN production_attempt_no integer NULL,
    ADD CONSTRAINT validation_runs_production_attempt_fkey
        FOREIGN KEY (workspace_id, production_operation_id, production_version, production_attempt_no)
        REFERENCES production_validation_attempts (workspace_id, operation_id, production_version, attempt_no) ON DELETE RESTRICT,
    ADD CONSTRAINT validation_runs_production_shape CHECK (
        (production_operation_id IS NULL AND production_version IS NULL AND production_attempt_no IS NULL)
        OR (production_operation_id IS NOT NULL AND production_version IS NOT NULL AND production_attempt_no IS NOT NULL)
    );

ALTER TABLE validation_runs DROP CONSTRAINT validation_runs_proposal_id_validator_id_validator_version_key;

CREATE UNIQUE INDEX validation_runs_legacy_unique_idx
    ON validation_runs (proposal_id, validator_id, validator_version)
    WHERE production_operation_id IS NULL;

CREATE UNIQUE INDEX validation_runs_production_unique_idx
    ON validation_runs (workspace_id, production_operation_id, production_version, production_attempt_no, proposal_id, validator_id, validator_version)
    WHERE production_operation_id IS NOT NULL;

CREATE TABLE production_validation_bindings (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    validation_run_id uuid PRIMARY KEY REFERENCES validation_runs(id) ON DELETE RESTRICT,
    operation_id uuid NOT NULL,
    production_version integer NOT NULL CHECK (production_version >= 1),
    attempt_no integer NOT NULL CHECK (attempt_no >= 1 AND attempt_no <= 256),
    set_digest text NOT NULL CHECK (set_digest ~ '^sha256:[0-9a-f]{64}$'),
    proposal_id uuid NOT NULL REFERENCES proposals(id) ON DELETE RESTRICT,
    proposal_content_digest text NOT NULL CHECK (proposal_content_digest ~ '^sha256:[0-9a-f]{64}$'),
    input_digest text NOT NULL CHECK (input_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id, operation_id, production_version, attempt_no)
        REFERENCES production_validation_attempts (workspace_id, operation_id, production_version, attempt_no) ON DELETE RESTRICT
);

CREATE TABLE production_review_bindings (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    review_id uuid PRIMARY KEY REFERENCES reviews(id) ON DELETE RESTRICT,
    operation_id uuid NOT NULL,
    production_version integer NOT NULL CHECK (production_version >= 1),
    attempt_no integer NOT NULL CHECK (attempt_no >= 1 AND attempt_no <= 256),
    set_digest text NOT NULL CHECK (set_digest ~ '^sha256:[0-9a-f]{64}$'),
    validation_digest text NOT NULL CHECK (validation_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (workspace_id, operation_id, production_version, attempt_no)
        REFERENCES production_validation_attempts (workspace_id, operation_id, production_version, attempt_no) ON DELETE RESTRICT
);

ALTER TABLE releases
    ADD COLUMN production_root_release_id uuid NULL,
    ADD COLUMN production_rollback_parent_id uuid NULL,
    ADD COLUMN production_rollback_depth integer NULL,
    ADD CONSTRAINT releases_production_protection_shape CHECK (
        (production_root_release_id IS NULL AND production_rollback_parent_id IS NULL AND production_rollback_depth IS NULL)
        OR (production_root_release_id = id AND production_rollback_parent_id IS NULL AND production_rollback_depth = 0)
        OR (production_root_release_id IS NOT NULL AND production_rollback_parent_id IS NOT NULL AND production_rollback_depth > 0)
    ),
    ADD FOREIGN KEY (workspace_id, production_root_release_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT,
    ADD FOREIGN KEY (workspace_id, production_rollback_parent_id)
        REFERENCES releases (workspace_id, id) ON DELETE RESTRICT;

CREATE TABLE release_proposals (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    release_id uuid NOT NULL REFERENCES releases(id) ON DELETE RESTRICT,
    proposal_id uuid NOT NULL REFERENCES proposals(id) ON DELETE RESTRICT,
    operation_id uuid NOT NULL,
    production_version integer NOT NULL CHECK (production_version >= 1),
    set_digest text NOT NULL CHECK (set_digest ~ '^sha256:[0-9a-f]{64}$'),
    proposal_content_digest text NOT NULL CHECK (proposal_content_digest ~ '^sha256:[0-9a-f]{64}$'),
    author_principal_id uuid NOT NULL,
    attempt_no integer NOT NULL CHECK (attempt_no >= 1 AND attempt_no <= 256),
    validation_digest text NOT NULL CHECK (validation_digest ~ '^sha256:[0-9a-f]{64}$'),
    review_ids jsonb NOT NULL,
    role text NOT NULL CHECK (role IN ('applied', 'reverted')),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, release_id, proposal_id),
    FOREIGN KEY (workspace_id, operation_id, production_version, attempt_no)
        REFERENCES production_validation_attempts (workspace_id, operation_id, production_version, attempt_no) ON DELETE RESTRICT
);

CREATE TABLE production_release_manifests (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    release_id uuid PRIMARY KEY REFERENCES releases(id) ON DELETE RESTRICT,
    before_release_id uuid NULL REFERENCES releases(id) ON DELETE RESTRICT,
    before_manifest_json jsonb NOT NULL,
    before_manifest_digest text NOT NULL CHECK (before_manifest_digest ~ '^sha256:[0-9a-f]{64}$'),
    after_manifest_digest text NOT NULL CHECK (after_manifest_digest ~ '^sha256:[0-9a-f]{64}$'),
    attribution_digest text NOT NULL CHECK (attribution_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE production_release_before_pins (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    release_id uuid NOT NULL REFERENCES releases(id) ON DELETE RESTRICT,
    target_kind text NOT NULL CHECK (target_kind IN ('semantic_asset', 'physical_binding', 'model_grain', 'entity_key', 'join_contract')),
    target_id uuid NOT NULL CHECK (substring(target_id::text, 15, 1) = '7'),
    presence text NOT NULL CHECK (presence IN ('present', 'absent')),
    asset_revision_id uuid NULL,
    object_version integer NULL,
    content_digest text NULL CHECK (content_digest IS NULL OR content_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, release_id, target_kind, target_id),
    CONSTRAINT production_release_before_pins_shape CHECK (
        (presence = 'present' AND (asset_revision_id IS NOT NULL OR (object_version IS NOT NULL AND content_digest IS NOT NULL)))
        OR (presence = 'absent' AND asset_revision_id IS NULL AND object_version IS NULL AND content_digest IS NULL)
    )
);

CREATE TABLE production_release_binding_inputs (
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    release_id uuid NOT NULL REFERENCES releases(id) ON DELETE RESTRICT,
    binding_id uuid NOT NULL CHECK (substring(binding_id::text, 15, 1) = '7'),
    binding_version integer NOT NULL CHECK (binding_version >= 1),
    snapshot_id uuid NOT NULL CHECK (substring(snapshot_id::text, 15, 1) = '7'),
    dataset_revision_id uuid NOT NULL CHECK (substring(dataset_revision_id::text, 15, 1) = '7'),
    field_revision_ids jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (workspace_id, release_id, binding_id, binding_version)
);

COMMIT;

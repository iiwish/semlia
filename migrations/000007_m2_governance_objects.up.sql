BEGIN;

-- M2 governance objects (docs/specs/m2-governed-authoring/data-model.md step 3;
-- SSOT §17 M2 and D-014): PhysicalBinding, ModelGrain, EntityKey and
-- JoinContract as first-class, workspace-scoped governance objects. The four
-- tables carry no workflow/lifecycle enum — status dimensions stay with
-- assets and proposals (semantic-asset design §4.8); the only mutable
-- dimension is the row version, which is bumped exactly once per governed
-- change application (enforced by the trigger below as defense in depth).

CREATE FUNCTION enforce_governed_object_version_bump()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.created_by IS DISTINCT FROM OLD.created_by THEN
        RAISE EXCEPTION 'governed object identity columns are frozen' USING ERRCODE = '23514';
    END IF;
    IF NEW.version <> OLD.version + 1 OR NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'governed object updates must bump the version by exactly one'
        USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

-- PhysicalBinding (D-014): ties a semantic asset to one M1 physical target —
-- a dataset plus an optional field refinement. One ACTIVE binding per
-- (asset, physical target) is enforced with the two partial unique indexes
-- below; retired bindings free their target without deleting history.
CREATE TABLE physical_bindings (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    asset_id uuid NOT NULL,
    dataset_id uuid NOT NULL,
    field_id uuid,
    transform text CHECK (transform IS NULL OR (transform <> '' AND length(transform) <= 4096)),
    retired_at timestamptz,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    content jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(content) = 'object'),
    created_by text NOT NULL CHECK (created_by <> '' AND length(created_by) <= 256),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, dataset_id)
        REFERENCES physical_datasets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, field_id)
        REFERENCES physical_fields (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX physical_bindings_workspace_updated_idx
    ON physical_bindings (workspace_id, updated_at DESC, id);
CREATE INDEX physical_bindings_asset_idx
    ON physical_bindings (workspace_id, asset_id, id);

CREATE UNIQUE INDEX physical_bindings_active_dataset_target_idx
    ON physical_bindings (workspace_id, asset_id, dataset_id)
    WHERE retired_at IS NULL AND field_id IS NULL;
CREATE UNIQUE INDEX physical_bindings_active_field_target_idx
    ON physical_bindings (workspace_id, asset_id, dataset_id, field_id)
    WHERE retired_at IS NULL AND field_id IS NOT NULL;

CREATE TRIGGER physical_bindings_version_bump
    BEFORE UPDATE ON physical_bindings
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_object_version_bump();

-- ModelGrain: the documented grain of a semantic asset, expressed as a
-- statement plus the physical field references the grain is measurable on.
CREATE TABLE model_grains (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    asset_id uuid NOT NULL,
    grain_expression text NOT NULL CHECK (grain_expression <> '' AND length(grain_expression) <= 4096),
    grain_field_refs jsonb NOT NULL CHECK (jsonb_typeof(grain_field_refs) = 'array'),
    documented_by uuid,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    content jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(content) = 'object'),
    created_by text NOT NULL CHECK (created_by <> '' AND length(created_by) <= 256),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, documented_by)
        REFERENCES evidence_artifacts (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX model_grains_workspace_updated_idx
    ON model_grains (workspace_id, updated_at DESC, id);
CREATE INDEX model_grains_asset_idx ON model_grains (workspace_id, asset_id, id);

CREATE TRIGGER model_grains_version_bump
    BEFORE UPDATE ON model_grains
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_object_version_bump();

-- EntityKey: the uniqueness contract of an entity asset — the physical key
-- fields and whether the source yields exact or deduplicated uniqueness.
CREATE TABLE entity_keys (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    asset_id uuid NOT NULL,
    key_field_refs jsonb NOT NULL CHECK (jsonb_typeof(key_field_refs) = 'array' AND jsonb_array_length(key_field_refs) > 0),
    uniqueness_semantics text NOT NULL CHECK (uniqueness_semantics IN ('exact', 'deduplicated')),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    content jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(content) = 'object'),
    created_by text NOT NULL CHECK (created_by <> '' AND length(created_by) <= 256),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX entity_keys_workspace_updated_idx
    ON entity_keys (workspace_id, updated_at DESC, id);
CREATE INDEX entity_keys_asset_idx ON entity_keys (workspace_id, asset_id, id);

CREATE TRIGGER entity_keys_version_bump
    BEFORE UPDATE ON entity_keys
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_object_version_bump();

-- JoinContract (D-014): a governed join between two physical datasets with
-- explicit field pairs, join type and cardinality, so consumers and agents
-- resolve joins deterministically instead of guessing.
CREATE TABLE join_contracts (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    left_dataset_id uuid NOT NULL,
    right_dataset_id uuid NOT NULL,
    left_field_refs jsonb NOT NULL CHECK (jsonb_typeof(left_field_refs) = 'array'),
    right_field_refs jsonb NOT NULL CHECK (jsonb_typeof(right_field_refs) = 'array'),
    join_type text NOT NULL CHECK (join_type IN ('inner', 'left', 'right', 'full')),
    cardinality text NOT NULL CHECK (cardinality IN ('one_to_one', 'one_to_many', 'many_to_one', 'many_to_many')),
    join_expression text NOT NULL CHECK (join_expression <> '' AND length(join_expression) <= 4096),
    contract_notes text CHECK (contract_notes IS NULL OR (contract_notes <> '' AND length(contract_notes) <= 4096)),
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    content jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(content) = 'object'),
    created_by text NOT NULL CHECK (created_by <> '' AND length(created_by) <= 256),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, left_dataset_id)
        REFERENCES physical_datasets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, right_dataset_id)
        REFERENCES physical_datasets (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX join_contracts_workspace_updated_idx
    ON join_contracts (workspace_id, updated_at DESC, id);
CREATE INDEX join_contracts_left_dataset_idx
    ON join_contracts (workspace_id, left_dataset_id, id);
CREATE INDEX join_contracts_right_dataset_idx
    ON join_contracts (workspace_id, right_dataset_id, id);

CREATE TRIGGER join_contracts_version_bump
    BEFORE UPDATE ON join_contracts
    FOR EACH ROW EXECUTE FUNCTION enforce_governed_object_version_bump();

-- Proposal references to the governance objects: the T002 target vocabulary
-- gains physical_binding (model_grain, entity_key and join_contract were
-- already legal); proposals keep targeting the objects by
-- target_object_type/target_object_id with no FK, mirroring the T002 shape.
ALTER TABLE proposals DROP CONSTRAINT proposals_target_object_type_check;
ALTER TABLE proposals
    ADD CONSTRAINT proposals_target_object_type_check CHECK (target_object_type IN (
        'semantic_asset', 'physical_binding', 'model_grain', 'entity_key', 'join_contract'
    ));

COMMIT;

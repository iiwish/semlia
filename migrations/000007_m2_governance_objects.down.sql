-- Drop only the M2-T008 governance objects; M0/M1/M2-T001/M2-T002 schema and
-- rows stay untouched. Proposals whose target_object_type became possible only
-- with this migration are removed so the T002 target vocabulary CHECK can be
-- restored — governed objects themselves are dropped entirely.
DELETE FROM proposals WHERE target_object_type = 'physical_binding';

ALTER TABLE proposals DROP CONSTRAINT IF EXISTS proposals_target_object_type_check;
ALTER TABLE proposals
    ADD CONSTRAINT proposals_target_object_type_check CHECK (target_object_type IN (
        'semantic_asset', 'model_grain', 'entity_key', 'join_contract'
    ));

DROP TABLE IF EXISTS join_contracts;
DROP TABLE IF EXISTS entity_keys;
DROP TABLE IF EXISTS model_grains;
DROP TABLE IF EXISTS physical_bindings;
DROP FUNCTION IF EXISTS enforce_governed_object_version_bump;

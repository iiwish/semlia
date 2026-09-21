BEGIN;
CREATE TRIGGER principals_seeding_agent_authorization AFTER INSERT ON principals FOR EACH ROW EXECUTE FUNCTION seed_agent_authorization_on_principal();
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM semantic_assets) OR EXISTS(SELECT 1 FROM semantic_candidates) OR EXISTS(SELECT 1 FROM usage_events) THEN
        RAISE EXCEPTION 'Populated prototype knowledge cannot be downgraded. Restore the pre-cleanup database snapshot.';
    END IF;
END $$;
ALTER TABLE semantic_assets DROP CONSTRAINT semantic_assets_asset_type_check;
ALTER TABLE semantic_assets ADD CONSTRAINT semantic_assets_asset_type_check CHECK (asset_type IN ('concept','entity','semantic_model','dimension','measure','metric','segment'));
ALTER TABLE semantic_candidates DROP CONSTRAINT semantic_candidates_candidate_kind_check;
ALTER TABLE semantic_candidates ADD CONSTRAINT semantic_candidates_candidate_kind_check CHECK (candidate_kind IN ('entity','dimension','metric','join'));
ALTER TABLE usage_events DROP CONSTRAINT usage_events_asset_type_filter_check;
ALTER TABLE usage_events ADD CONSTRAINT usage_events_asset_type_filter_check CHECK (asset_type_filter IS NULL OR asset_type_filter IN ('concept','entity','semantic_model','dimension','measure','metric','segment'));
INSERT INTO relation_type_policies (
    predicate, plane, source_types, target_types, matching_types, cardinality, reasoning, schema_version
) VALUES
    ('measures', 'semantic', ARRAY['metric', 'measure'], ARRAY['entity', 'semantic_model'], false, 'many_to_many', 'directed', '1.0.0'),
    ('describes', 'semantic', ARRAY['dimension', 'concept'], ARRAY['entity', 'semantic_model', 'concept'], false, 'many_to_many', 'directed', '1.0.0'),
    ('depends_on', 'dependency', ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], ARRAY['metric', 'measure', 'dimension', 'semantic_model'], false, 'many_to_many', 'directed', '1.0.0'),
    ('derived_from', 'dependency', ARRAY['metric', 'measure', 'semantic_model'], ARRAY['metric', 'measure', 'semantic_model'], false, 'many_to_many', 'directed', '1.0.0'),
    ('filters_by', 'semantic', ARRAY['metric', 'measure', 'segment', 'semantic_model'], ARRAY['dimension', 'entity'], false, 'many_to_many', 'directed', '1.0.0'),
    ('synonym_of', 'taxonomy', ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], true, 'one_to_one', 'symmetric', '1.0.0'),
    ('contains', 'semantic', ARRAY['concept', 'entity', 'semantic_model'], ARRAY['metric', 'measure', 'dimension', 'concept'], false, 'one_to_many', 'directed', '1.0.0'),
    ('broader_than', 'taxonomy', ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], true, 'one_to_many', 'directed', '1.0.0'),
    ('narrower_than', 'taxonomy', ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], true, 'many_to_one', 'directed', '1.0.0'),
    ('equivalent_to', 'taxonomy', ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], true, 'one_to_one', 'symmetric', '1.0.0'),
    ('disjoint_with', 'taxonomy', ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], true, 'many_to_many', 'symmetric', '1.0.0')
ON CONFLICT (predicate) DO UPDATE SET source_types=EXCLUDED.source_types,target_types=EXCLUDED.target_types,schema_version=EXCLUDED.schema_version;
COMMIT;

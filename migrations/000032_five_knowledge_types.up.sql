BEGIN;

-- Only the dedicated workspace authoring agent receives an automatic grant.
-- Consumer-created machine principals require explicit role bindings.
DROP TRIGGER IF EXISTS principals_seeding_agent_authorization ON principals;

-- Incompatible prototype records must be explicitly backed up and cleared first.
ALTER TABLE semantic_assets DROP CONSTRAINT semantic_assets_asset_type_check;
ALTER TABLE semantic_assets ADD CONSTRAINT semantic_assets_asset_type_check
    CHECK (asset_type IN ('business_object', 'business_term', 'metric', 'data_asset', 'analysis_model'));
ALTER TABLE semantic_candidates DROP CONSTRAINT semantic_candidates_candidate_kind_check;
ALTER TABLE semantic_candidates ADD CONSTRAINT semantic_candidates_candidate_kind_check
    CHECK (candidate_kind IN ('data_asset', 'join'));
ALTER TABLE usage_events DROP CONSTRAINT usage_events_asset_type_filter_check;
ALTER TABLE usage_events ADD CONSTRAINT usage_events_asset_type_filter_check
    CHECK (asset_type_filter IS NULL OR asset_type_filter IN ('business_object', 'business_term', 'metric', 'data_asset', 'analysis_model'));

UPDATE relation_type_policies SET
    source_types = ARRAY['business_object', 'business_term', 'metric', 'data_asset', 'analysis_model'],
    target_types = ARRAY['business_object', 'business_term', 'metric', 'data_asset', 'analysis_model'],
    schema_version = '2.0.0';
UPDATE relation_type_policies SET source_types = ARRAY['metric'], target_types = ARRAY['business_object', 'analysis_model'] WHERE predicate = 'measures';
UPDATE relation_type_policies SET source_types = ARRAY['business_term', 'data_asset'], target_types = ARRAY['business_object', 'analysis_model'] WHERE predicate = 'describes';
UPDATE relation_type_policies SET source_types = ARRAY['metric', 'data_asset'], target_types = ARRAY['metric', 'data_asset'] WHERE predicate = 'derived_from';
UPDATE relation_type_policies SET source_types = ARRAY['metric', 'analysis_model'], target_types = ARRAY['business_term'] WHERE predicate = 'filters_by';
UPDATE relation_type_policies SET source_types = ARRAY['analysis_model'] WHERE predicate = 'contains';

COMMIT;

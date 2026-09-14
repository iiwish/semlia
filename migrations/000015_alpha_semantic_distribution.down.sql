BEGIN;

DROP TRIGGER IF EXISTS release_objects_capture_snapshot ON release_objects;
DROP FUNCTION IF EXISTS capture_release_object_snapshot();
DROP TABLE IF EXISTS semantic_resolution_events;
DROP TABLE IF EXISTS query_validation_results;
DROP TABLE IF EXISTS query_validation_runs;
DROP TABLE IF EXISTS semantic_refusals;
DROP TABLE IF EXISTS resolved_semantic_plans;
DROP TABLE IF EXISTS semantic_queries;
DROP TRIGGER IF EXISTS consumer_bindings_version_bump ON consumer_bindings;
DROP FUNCTION IF EXISTS enforce_consumer_binding_version_bump();
DROP TABLE IF EXISTS consumer_bindings;
DROP TABLE IF EXISTS consumers;
DROP TABLE IF EXISTS release_object_snapshots;

COMMIT;

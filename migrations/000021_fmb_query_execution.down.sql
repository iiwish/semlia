BEGIN;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM query_execution_runs) OR EXISTS(SELECT 1 FROM release_execution_binding_pins)
 OR EXISTS(SELECT 1 FROM release_execution_relations) OR EXISTS(SELECT 1 FROM resolved_semantic_plans WHERE execution_status='requires_execution_validation') THEN
  RAISE EXCEPTION 'migration 21 downgrade requires preserving execution history; restore a verified schema20 backup instead' USING ERRCODE='55000';
 END IF;
END $$;
ALTER TABLE resolved_semantic_plans DROP CONSTRAINT resolved_semantic_plans_execution_status_check;
ALTER TABLE resolved_semantic_plans ADD CONSTRAINT resolved_semantic_plans_execution_status_check CHECK(execution_status IN('not_configured','ready'));
CREATE OR REPLACE FUNCTION capture_release_object_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE snapshot jsonb;
BEGIN
 CASE NEW.object_type
  WHEN 'physical_binding' THEN SELECT to_jsonb(v) INTO snapshot FROM physical_bindings v WHERE v.workspace_id=NEW.workspace_id AND v.id=NEW.object_id AND v.version=NEW.version;
  WHEN 'model_grain' THEN SELECT to_jsonb(v) INTO snapshot FROM model_grains v WHERE v.workspace_id=NEW.workspace_id AND v.id=NEW.object_id AND v.version=NEW.version;
  WHEN 'entity_key' THEN SELECT to_jsonb(v) INTO snapshot FROM entity_keys v WHERE v.workspace_id=NEW.workspace_id AND v.id=NEW.object_id AND v.version=NEW.version;
  WHEN 'join_contract' THEN SELECT to_jsonb(v) INTO snapshot FROM join_contracts v WHERE v.workspace_id=NEW.workspace_id AND v.id=NEW.object_id AND v.version=NEW.version;
  ELSE RAISE EXCEPTION 'unsupported release object type' USING ERRCODE='23514';
 END CASE;
 IF snapshot IS NULL THEN RAISE EXCEPTION 'release object version is unavailable' USING ERRCODE='23514'; END IF;
 INSERT INTO release_object_snapshots(workspace_id,release_id,object_type,object_id,version,payload,position,created_at)
 VALUES(NEW.workspace_id,NEW.release_id,NEW.object_type,NEW.object_id,NEW.version,snapshot,NEW.position,NEW.created_at);
 RETURN NEW;
END $$;
-- Downgrade removes only the additive capability, never grants execution.
UPDATE client_credentials SET allowed_actions=array_remove(allowed_actions,'semantic.execute')
 WHERE 'semantic.execute'=ANY(allowed_actions) AND cardinality(allowed_actions)>1;
UPDATE client_credentials SET allowed_actions=ARRAY['semantic.resolve'], revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP)
 WHERE allowed_actions=ARRAY['semantic.execute'];
ALTER TABLE client_credentials DROP CONSTRAINT client_credentials_allowed_actions_check;
ALTER TABLE client_credentials ADD CONSTRAINT client_credentials_allowed_actions_check
 CHECK (cardinality(allowed_actions)>0 AND allowed_actions <@ ARRAY['asset.read','semantic.resolve']::text[]);
DROP TABLE query_execution_runs;
DROP FUNCTION protect_query_execution_run();
DROP TRIGGER release_object_snapshots_capture_execution ON release_object_snapshots;
DROP FUNCTION capture_release_execution_relation();
DROP TABLE release_execution_binding_pins;
DROP TABLE release_execution_relations;
COMMIT;

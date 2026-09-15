BEGIN;
ALTER TABLE resolved_semantic_plans DROP CONSTRAINT resolved_semantic_plans_execution_status_check;
ALTER TABLE resolved_semantic_plans ADD CONSTRAINT resolved_semantic_plans_execution_status_check
 CHECK(execution_status IN ('not_configured','ready','requires_execution_validation'));
ALTER TABLE client_credentials DROP CONSTRAINT client_credentials_allowed_actions_check;
ALTER TABLE client_credentials ADD CONSTRAINT client_credentials_allowed_actions_check
 CHECK (cardinality(allowed_actions)>0 AND allowed_actions <@ ARRAY['asset.read','semantic.resolve','semantic.execute']::text[]);

-- Capture only future publication facts. Legacy releases have no executable
-- provenance and are intentionally not backfilled from mutable catalog rows.
CREATE TABLE release_execution_relations (
 workspace_id uuid NOT NULL,
 release_id uuid NOT NULL,
 dataset_id uuid NOT NULL,
 payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
 PRIMARY KEY (release_id, dataset_id),
 FOREIGN KEY (workspace_id, release_id) REFERENCES releases(workspace_id,id) ON DELETE RESTRICT
);
CREATE TRIGGER release_execution_relations_immutable BEFORE UPDATE OR DELETE ON release_execution_relations
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TABLE release_execution_binding_pins (
 workspace_id uuid NOT NULL,release_id uuid NOT NULL,binding_id uuid NOT NULL,version integer NOT NULL,
 dataset_id uuid NOT NULL,payload jsonb NOT NULL,
 PRIMARY KEY(release_id,binding_id,version),
 FOREIGN KEY(workspace_id,release_id) REFERENCES releases(workspace_id,id) ON DELETE RESTRICT
);
CREATE TRIGGER release_execution_binding_pins_immutable BEFORE UPDATE OR DELETE ON release_execution_binding_pins
 FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

-- Reusing an already published object version copies its original immutable
-- snapshot even when a newer, unpublished draft is present in the registry.
CREATE OR REPLACE FUNCTION capture_release_object_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE snapshot jsonb;
BEGIN
 SELECT payload INTO snapshot FROM release_object_snapshots
 WHERE workspace_id=NEW.workspace_id AND object_type=NEW.object_type AND object_id=NEW.object_id AND version=NEW.version
 ORDER BY created_at,release_id LIMIT 1;
 IF snapshot IS NULL THEN
  CASE NEW.object_type
   WHEN 'physical_binding' THEN SELECT to_jsonb(v) INTO snapshot FROM physical_bindings v WHERE v.workspace_id=NEW.workspace_id AND v.id=NEW.object_id AND v.version=NEW.version;
   WHEN 'model_grain' THEN SELECT to_jsonb(v) INTO snapshot FROM model_grains v WHERE v.workspace_id=NEW.workspace_id AND v.id=NEW.object_id AND v.version=NEW.version;
   WHEN 'entity_key' THEN SELECT to_jsonb(v) INTO snapshot FROM entity_keys v WHERE v.workspace_id=NEW.workspace_id AND v.id=NEW.object_id AND v.version=NEW.version;
   WHEN 'join_contract' THEN SELECT to_jsonb(v) INTO snapshot FROM join_contracts v WHERE v.workspace_id=NEW.workspace_id AND v.id=NEW.object_id AND v.version=NEW.version;
   ELSE RAISE EXCEPTION 'unsupported release object type' USING ERRCODE='23514';
  END CASE;
 END IF;
 IF snapshot IS NULL THEN RAISE EXCEPTION 'release object version is unavailable' USING ERRCODE='23514'; END IF;
 INSERT INTO release_object_snapshots(workspace_id,release_id,object_type,object_id,version,payload,position,created_at)
 VALUES(NEW.workspace_id,NEW.release_id,NEW.object_type,NEW.object_id,NEW.version,snapshot,NEW.position,NEW.created_at);
 RETURN NEW;
END $$;

CREATE FUNCTION capture_release_execution_relation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE dataset uuid; frozen jsonb; prior_release uuid;
BEGIN
 IF NEW.object_type <> 'physical_binding' THEN RETURN NEW; END IF;
 dataset := (NEW.payload->>'dataset_id')::uuid;
 SELECT release_id INTO prior_release FROM release_object_snapshots
 WHERE workspace_id=NEW.workspace_id AND object_type=NEW.object_type AND object_id=NEW.object_id
   AND version=NEW.version AND release_id<>NEW.release_id ORDER BY created_at,release_id LIMIT 1;
 IF prior_release IS NOT NULL THEN
  SELECT payload INTO frozen FROM release_execution_binding_pins WHERE workspace_id=NEW.workspace_id AND release_id=prior_release AND binding_id=NEW.object_id AND version=NEW.version;
  IF frozen IS NULL THEN RETURN NEW; END IF;
  IF frozen IS NOT NULL AND EXISTS(SELECT 1 FROM release_execution_relations WHERE release_id=NEW.release_id AND dataset_id=dataset AND payload<>frozen) THEN
   RAISE EXCEPTION 'release contains conflicting physical provenance' USING ERRCODE='23514';
  END IF;
  INSERT INTO release_execution_relations(workspace_id,release_id,dataset_id,payload)
  VALUES(NEW.workspace_id,NEW.release_id,dataset,frozen) ON CONFLICT DO NOTHING;
  INSERT INTO release_execution_binding_pins VALUES(NEW.workspace_id,NEW.release_id,NEW.object_id,NEW.version,dataset,frozen);
  RETURN NEW;
 END IF;
 SELECT jsonb_build_object(
   'datasetId', d.id::text, 'datasetRevisionId', dr.id::text,
   'sourceId', s.id::text, 'sourceRevisionId', dr.source_revision_id::text,
   'sourceLocator', s.normalized_locator, 'adapterKind', s.adapter_kind,
   'schema', split_part(d.qualified_name,'.',1), 'relation', split_part(d.qualified_name,'.',2),
   'fields', (SELECT COALESCE(jsonb_agg(jsonb_build_object(
       'fieldId', f.id::text, 'revisionId', fr.id::text, 'name', f.name, 'dataType', fr.data_type
     ) ORDER BY fr.ordinal),'[]'::jsonb)
     FROM physical_fields f JOIN physical_field_revisions fr
       ON fr.workspace_id=f.workspace_id AND fr.physical_field_id=f.id AND fr.id=f.current_revision_id
     WHERE f.workspace_id=d.workspace_id AND f.physical_dataset_id=d.id AND fr.dataset_revision_id=dr.id)
 ) INTO frozen
 FROM physical_datasets d JOIN physical_dataset_revisions dr
   ON dr.workspace_id=d.workspace_id AND dr.physical_dataset_id=d.id AND dr.id=d.current_revision_id
 JOIN source_connections s ON s.workspace_id=d.workspace_id AND s.id=d.source_connection_id
 JOIN source_revisions sr ON sr.workspace_id=s.workspace_id AND sr.source_connection_id=s.id AND sr.id=dr.source_revision_id
 WHERE d.workspace_id=NEW.workspace_id AND d.id=dataset AND s.adapter_kind='postgresql_catalog'
   AND dr.dataset_kind='table' AND d.qualified_name ~ '^[A-Za-z_][A-Za-z0-9_$]*\.[A-Za-z_][A-Za-z0-9_$]*$';
 IF frozen IS NOT NULL THEN
  IF EXISTS(SELECT 1 FROM release_execution_relations WHERE release_id=NEW.release_id AND dataset_id=dataset AND payload<>frozen) THEN
   RAISE EXCEPTION 'release contains conflicting physical provenance' USING ERRCODE='23514';
  END IF;
  INSERT INTO release_execution_relations(workspace_id,release_id,dataset_id,payload)
  VALUES(NEW.workspace_id,NEW.release_id,dataset,frozen) ON CONFLICT DO NOTHING;
  INSERT INTO release_execution_binding_pins VALUES(NEW.workspace_id,NEW.release_id,NEW.object_id,NEW.version,dataset,frozen);
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER release_object_snapshots_capture_execution AFTER INSERT ON release_object_snapshots
 FOR EACH ROW EXECUTE FUNCTION capture_release_execution_relation();

CREATE TABLE query_execution_runs (
 id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
 workspace_id uuid NOT NULL,
 runtime_run_id uuid NOT NULL,
 semantic_query_id uuid NOT NULL,
 resolved_plan_id uuid NOT NULL,
 plan_digest text NOT NULL CHECK (plan_digest ~ '^sha256:[0-9a-f]{64}$'),
 principal_id uuid NOT NULL,
 consumer_id uuid,
 credential_id uuid,
 channel text NOT NULL CHECK (channel IN ('api','mcp','cli','sdk','web')),
 source_connection_id uuid NOT NULL,
 source_revision_id uuid NOT NULL,
 adapter_version text NOT NULL,
 policy_version bigint NOT NULL CHECK(policy_version>0),
 timeout_ms bigint NOT NULL CHECK(timeout_ms>0 AND timeout_ms<=30000),
 max_rows integer NOT NULL CHECK(max_rows>0 AND max_rows<=10000),
 max_bytes integer NOT NULL CHECK(max_bytes>0 AND max_bytes<=8388608),
 cancel_requested boolean NOT NULL DEFAULT false,
 idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 1 AND 128),
 state text NOT NULL CHECK (state IN ('running','succeeded','failed','cancelled','unknown')),
 error_code text CHECK (error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
 row_count integer NOT NULL DEFAULT 0 CHECK (row_count>=0),
 byte_count integer NOT NULL DEFAULT 0 CHECK (byte_count>=0),
 result_digest text CHECK (result_digest ~ '^sha256:[0-9a-f]{64}$'),
 trace_id text NOT NULL CHECK (trace_id ~ '^[0-9a-f]{32}$'),
 started_at timestamptz NOT NULL,
 deadline_at timestamptz NOT NULL,
 finished_at timestamptz,
 UNIQUE(workspace_id,id),
 UNIQUE(workspace_id,principal_id,idempotency_key),
 FOREIGN KEY (workspace_id,runtime_run_id) REFERENCES runtime_runs(workspace_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (workspace_id,semantic_query_id) REFERENCES semantic_queries(workspace_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (workspace_id,resolved_plan_id) REFERENCES resolved_semantic_plans(workspace_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (workspace_id,principal_id) REFERENCES principals(workspace_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (workspace_id,consumer_id) REFERENCES consumers(workspace_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (workspace_id,credential_id) REFERENCES client_credentials(workspace_id,id) ON DELETE RESTRICT,
 FOREIGN KEY (workspace_id,source_connection_id,source_revision_id) REFERENCES source_revisions(workspace_id,source_connection_id,id) ON DELETE RESTRICT,
 CHECK ((state='running' AND finished_at IS NULL) OR (state<>'running' AND finished_at IS NOT NULL)),
 CHECK ((state='succeeded' AND result_digest IS NOT NULL AND error_code IS NULL) OR (state<>'succeeded' AND result_digest IS NULL))
);
CREATE INDEX query_execution_runs_workspace_state ON query_execution_runs(workspace_id,state,deadline_at);

CREATE FUNCTION protect_query_execution_run() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.state<>'running' THEN RAISE EXCEPTION 'execution metadata is immutable' USING ERRCODE='55000'; END IF;
 IF OLD.cancel_requested AND NOT NEW.cancel_requested THEN RAISE EXCEPTION 'cancellation is monotonic'; END IF;
 IF (to_jsonb(NEW)-ARRAY['state','error_code','row_count','byte_count','result_digest','finished_at','cancel_requested']) IS DISTINCT FROM
    (to_jsonb(OLD)-ARRAY['state','error_code','row_count','byte_count','result_digest','finished_at','cancel_requested']) THEN
  RAISE EXCEPTION 'execution provenance is immutable' USING ERRCODE='55000';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER query_execution_runs_protect BEFORE UPDATE OR DELETE ON query_execution_runs
 FOR EACH ROW EXECUTE FUNCTION protect_query_execution_run();
COMMIT;

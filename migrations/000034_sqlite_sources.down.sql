BEGIN;

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM source_connections WHERE source_kind = 'sqlite')
        OR EXISTS (SELECT 1 FROM source_artifacts WHERE artifact_kind = 'sqlite') THEN
        RAISE EXCEPTION 'SQLite sources exist; downgrade requires a verified backup restore';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION capture_release_execution_relation() RETURNS trigger LANGUAGE plpgsql AS $$
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


ALTER TABLE source_artifacts DROP CONSTRAINT source_artifacts_media_type_check,
    ADD CONSTRAINT source_artifacts_media_type_check CHECK (media_type IN (
        'text/csv',
        'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
        'text/markdown', 'text/plain', 'application/sql', 'application/json'
    ));

ALTER TABLE source_artifacts DROP CONSTRAINT source_artifacts_artifact_kind_check,
    ADD CONSTRAINT source_artifacts_artifact_kind_check
        CHECK (artifact_kind IN ('csv', 'xlsx', 'markdown', 'sql', 'dbt_manifest', 'dbt_catalog'));

ALTER TABLE source_artifact_sets DROP CONSTRAINT source_artifact_sets_source_kind_check,
    ADD CONSTRAINT source_artifact_sets_source_kind_check
        CHECK (source_kind IN ('sql_bundle', 'dbt_bundle', 'file'));

ALTER TABLE source_connections DROP CONSTRAINT source_connections_source_kind_check,
    ADD CONSTRAINT source_connections_source_kind_check
        CHECK (source_kind IN ('postgresql', 'sql_bundle', 'dbt_bundle', 'file'));

COMMIT;

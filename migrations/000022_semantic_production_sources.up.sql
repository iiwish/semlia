BEGIN;

ALTER TABLE source_revisions ADD CONSTRAINT source_revisions_source_identity_key UNIQUE(workspace_id,source_connection_id,id);
ALTER TABLE discovery_runs ADD CONSTRAINT discovery_runs_source_identity_key UNIQUE(workspace_id,source_connection_id,id);
ALTER TABLE physical_dataset_revisions ADD CONSTRAINT physical_dataset_revisions_snapshot_identity_key UNIQUE(workspace_id,physical_dataset_id,id);
ALTER TABLE physical_field_revisions ADD CONSTRAINT physical_field_revisions_snapshot_identity_key UNIQUE(workspace_id,physical_field_id,id);
ALTER TABLE code_artifacts ADD CONSTRAINT code_artifacts_snapshot_identity_key UNIQUE(workspace_id,id,source_revision_id);
ALTER TABLE lineage_edges ADD CONSTRAINT lineage_edges_snapshot_identity_key UNIQUE(workspace_id,id,source_revision_id,upstream_dataset_id,downstream_dataset_id,edge_kind);

CREATE TABLE source_snapshots (
    id uuid PRIMARY KEY CHECK (substring(id::text,15,1)='7'),
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    adapter_version text NOT NULL CHECK (length(adapter_version) BETWEEN 1 AND 128),
    scope_digest text NOT NULL CHECK(scope_digest ~ '^sha256:[0-9a-f]{64}$'),
    content_digest text NOT NULL CHECK(content_digest ~ '^sha256:[0-9a-f]{64}$'),
    history_quality text NOT NULL CHECK(history_quality IN ('verified','unverifiable')),
    coverage_status text NOT NULL CHECK(coverage_status IN ('complete','partial','failed')),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workspace_id,id),
    UNIQUE(workspace_id,source_connection_id,id),
    UNIQUE(workspace_id,source_connection_id,source_revision_id,adapter_version,scope_digest,content_digest),
    FOREIGN KEY(workspace_id,source_connection_id,source_revision_id) REFERENCES source_revisions(workspace_id,source_connection_id,id) ON DELETE RESTRICT
);
CREATE INDEX source_snapshots_read_idx ON source_snapshots(workspace_id,source_connection_id,created_at DESC,id DESC);

CREATE TABLE source_snapshot_runs (
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    run_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    PRIMARY KEY(workspace_id,run_id),
    FOREIGN KEY(workspace_id,source_connection_id,run_id) REFERENCES discovery_runs(workspace_id,source_connection_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,source_connection_id,snapshot_id) REFERENCES source_snapshots(workspace_id,source_connection_id,id) ON DELETE RESTRICT
);

CREATE TABLE source_snapshot_scope (
    workspace_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    coverage_key text NOT NULL CHECK(length(coverage_key) BETWEEN 1 AND 256),
    selector text NOT NULL CHECK(length(selector) BETWEEN 1 AND 1024),
    config_digest text NOT NULL CHECK(config_digest ~ '^sha256:[0-9a-f]{64}$'),
    status text NOT NULL CHECK(status IN ('complete','partial','failed')),
    enumeration_complete boolean NOT NULL,
    diagnostic_codes text[] NOT NULL CHECK(cardinality(diagnostic_codes)<=64),
    PRIMARY KEY(workspace_id,snapshot_id,coverage_key),
    FOREIGN KEY(workspace_id,snapshot_id) REFERENCES source_snapshots(workspace_id,id) ON DELETE RESTRICT,
    CHECK(status<>'complete' OR enumeration_complete)
);

CREATE TABLE source_code_revisions (
    id uuid PRIMARY KEY CHECK(substring(id::text,15,1)='7'),
    workspace_id uuid NOT NULL,
    code_artifact_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    adapter_version text NOT NULL CHECK(length(adapter_version) BETWEEN 1 AND 128),
    path text NOT NULL CHECK(length(path) BETWEEN 1 AND 1024),
    language text NOT NULL CHECK(length(language) BETWEEN 1 AND 128),
    blob_oid text NOT NULL CHECK(blob_oid ~ '^sha256:[0-9a-f]{64}$'),
    content_bytes bytea NOT NULL CHECK(octet_length(content_bytes) BETWEEN 1 AND 52428800),
    content_digest text NOT NULL CHECK(content_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workspace_id,id),
    UNIQUE(workspace_id,code_artifact_id,id),
    UNIQUE(workspace_id,code_artifact_id,adapter_version,content_digest),
    FOREIGN KEY(workspace_id,code_artifact_id,source_revision_id) REFERENCES code_artifacts(workspace_id,id,source_revision_id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,source_revision_id) REFERENCES source_revisions(workspace_id,id) ON DELETE RESTRICT,
    CHECK(blob_oid='sha256:'||encode(sha256(content_bytes),'hex'))
);

CREATE TABLE source_lineage_revisions (
    id uuid PRIMARY KEY CHECK(substring(id::text,15,1)='7'),
    workspace_id uuid NOT NULL,
    lineage_edge_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    adapter_version text NOT NULL CHECK(length(adapter_version) BETWEEN 1 AND 128),
    upstream_object_id uuid NOT NULL,
    upstream_revision_id uuid NOT NULL,
    downstream_object_id uuid NOT NULL,
    downstream_revision_id uuid NOT NULL,
    edge_kind text NOT NULL,
    code_revision_id uuid,
    confidence numeric NOT NULL CHECK(confidence BETWEEN 0 AND 1),
    content_digest text NOT NULL CHECK(content_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workspace_id,id),
    UNIQUE(workspace_id,lineage_edge_id,id),
    UNIQUE(workspace_id,lineage_edge_id,adapter_version,content_digest),
    FOREIGN KEY(workspace_id,lineage_edge_id,source_revision_id,upstream_object_id,downstream_object_id,edge_kind) REFERENCES lineage_edges(workspace_id,id,source_revision_id,upstream_dataset_id,downstream_dataset_id,edge_kind) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,source_revision_id) REFERENCES source_revisions(workspace_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,upstream_object_id,upstream_revision_id) REFERENCES physical_dataset_revisions(workspace_id,physical_dataset_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,downstream_object_id,downstream_revision_id) REFERENCES physical_dataset_revisions(workspace_id,physical_dataset_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,code_revision_id) REFERENCES source_code_revisions(workspace_id,id) ON DELETE RESTRICT
);

CREATE TABLE source_snapshot_members (
    workspace_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    kind text NOT NULL CHECK(kind IN ('dataset','field','code','lineage')),
    object_id uuid NOT NULL,
    revision_id uuid NOT NULL,
    historical_name text NOT NULL CHECK(length(historical_name) BETWEEN 1 AND 1024),
    historical_locator text NOT NULL CHECK(length(historical_locator) BETWEEN 1 AND 2048),
    content_digest text NOT NULL CHECK(content_digest ~ '^sha256:[0-9a-f]{64}$'),
    coverage_key text NOT NULL,
    parent_object_id uuid,
    parent_revision_id uuid,
    dataset_object_id uuid GENERATED ALWAYS AS (CASE WHEN kind='dataset' THEN object_id END) STORED,
    dataset_revision_id uuid GENERATED ALWAYS AS (CASE WHEN kind='dataset' THEN revision_id END) STORED,
    field_object_id uuid GENERATED ALWAYS AS (CASE WHEN kind='field' THEN object_id END) STORED,
    field_revision_id uuid GENERATED ALWAYS AS (CASE WHEN kind='field' THEN revision_id END) STORED,
    code_object_id uuid GENERATED ALWAYS AS (CASE WHEN kind='code' THEN object_id END) STORED,
    code_revision_id uuid GENERATED ALWAYS AS (CASE WHEN kind='code' THEN revision_id END) STORED,
    lineage_object_id uuid GENERATED ALWAYS AS (CASE WHEN kind='lineage' THEN object_id END) STORED,
    lineage_revision_id uuid GENERATED ALWAYS AS (CASE WHEN kind='lineage' THEN revision_id END) STORED,
    parent_kind text GENERATED ALWAYS AS (CASE WHEN kind='field' THEN 'dataset' END) STORED,
    PRIMARY KEY(workspace_id,snapshot_id,kind,object_id),
    UNIQUE(workspace_id,snapshot_id,kind,object_id,revision_id),
    CHECK((kind='field' AND parent_object_id IS NOT NULL AND parent_revision_id IS NOT NULL) OR (kind<>'field' AND parent_object_id IS NULL AND parent_revision_id IS NULL)),
    FOREIGN KEY(workspace_id,snapshot_id,coverage_key) REFERENCES source_snapshot_scope(workspace_id,snapshot_id,coverage_key) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,dataset_object_id,dataset_revision_id) REFERENCES physical_dataset_revisions(workspace_id,physical_dataset_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,field_object_id,field_revision_id) REFERENCES physical_field_revisions(workspace_id,physical_field_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,code_object_id,code_revision_id) REFERENCES source_code_revisions(workspace_id,code_artifact_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,lineage_object_id,lineage_revision_id) REFERENCES source_lineage_revisions(workspace_id,lineage_edge_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,snapshot_id,parent_kind,parent_object_id,parent_revision_id) REFERENCES source_snapshot_members(workspace_id,snapshot_id,kind,object_id,revision_id) ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED
);

CREATE TABLE source_snapshot_diagnostics (
    workspace_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK(ordinal>0),
    code text NOT NULL CHECK(code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    severity text NOT NULL CHECK(severity IN ('info','warning','blocker')),
    coverage_key text NOT NULL,
    locator text NOT NULL DEFAULT '' CHECK(length(locator)<=2048),
    message text NOT NULL CHECK(length(message)<=2048),
    PRIMARY KEY(workspace_id,snapshot_id,ordinal),
    FOREIGN KEY(workspace_id,snapshot_id,coverage_key) REFERENCES source_snapshot_scope(workspace_id,snapshot_id,coverage_key) ON DELETE RESTRICT
);

CREATE TABLE source_effective_snapshots (
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    scope_digest text NOT NULL,
    snapshot_id uuid NOT NULL,
    version bigint NOT NULL CHECK(version>0),
    PRIMARY KEY(workspace_id,source_connection_id,scope_digest),
    FOREIGN KEY(workspace_id,source_connection_id,snapshot_id) REFERENCES source_snapshots(workspace_id,source_connection_id,id) ON DELETE RESTRICT
);
CREATE TABLE source_coverage_heads (
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    coverage_key text NOT NULL,
    selector_digest text NOT NULL CHECK(selector_digest ~ '^sha256:[0-9a-f]{64}$'),
    latest_attempt_run_id uuid NOT NULL,
    latest_attempt_status text NOT NULL CHECK(latest_attempt_status IN ('complete','partial','failed')),
    latest_verified_snapshot_id uuid,
    version bigint NOT NULL CHECK(version>0),
    PRIMARY KEY(workspace_id,source_connection_id,coverage_key,selector_digest),
    FOREIGN KEY(workspace_id,source_connection_id,latest_attempt_run_id) REFERENCES discovery_runs(workspace_id,source_connection_id,id) ON DELETE RESTRICT,
    FOREIGN KEY(workspace_id,source_connection_id,latest_verified_snapshot_id) REFERENCES source_snapshots(workspace_id,source_connection_id,id) ON DELETE RESTRICT
);

CREATE FUNCTION validate_source_snapshot_member() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source_id uuid; valid boolean;
BEGIN
    SELECT source_connection_id INTO source_id FROM source_snapshots WHERE workspace_id=NEW.workspace_id AND id=NEW.snapshot_id;
    IF NEW.kind='dataset' THEN
        SELECT EXISTS(SELECT 1 FROM physical_datasets WHERE workspace_id=NEW.workspace_id AND id=NEW.object_id AND source_connection_id=source_id) INTO valid;
    ELSIF NEW.kind='field' THEN
        SELECT EXISTS(SELECT 1 FROM physical_fields f JOIN physical_field_revisions r ON r.workspace_id=f.workspace_id AND r.physical_field_id=f.id
          WHERE f.workspace_id=NEW.workspace_id AND f.id=NEW.object_id AND f.physical_dataset_id=NEW.parent_object_id AND r.id=NEW.revision_id AND r.dataset_revision_id=NEW.parent_revision_id) INTO valid;
    ELSIF NEW.kind='code' THEN
        SELECT EXISTS(SELECT 1 FROM source_code_revisions r JOIN source_revisions s ON s.workspace_id=r.workspace_id AND s.id=r.source_revision_id
          WHERE r.workspace_id=NEW.workspace_id AND r.id=NEW.revision_id AND s.source_connection_id=source_id) INTO valid;
    ELSE
        SELECT EXISTS(SELECT 1 FROM source_lineage_revisions l JOIN source_revisions s ON s.workspace_id=l.workspace_id AND s.id=l.source_revision_id
          JOIN source_snapshot_members u ON u.workspace_id=l.workspace_id AND u.snapshot_id=NEW.snapshot_id AND u.kind='dataset' AND u.object_id=l.upstream_object_id AND u.revision_id=l.upstream_revision_id
          JOIN source_snapshot_members d ON d.workspace_id=l.workspace_id AND d.snapshot_id=NEW.snapshot_id AND d.kind='dataset' AND d.object_id=l.downstream_object_id AND d.revision_id=l.downstream_revision_id
          WHERE l.workspace_id=NEW.workspace_id AND l.id=NEW.revision_id AND s.source_connection_id=source_id
          AND (l.code_revision_id IS NULL OR EXISTS(SELECT 1 FROM source_snapshot_members c WHERE c.workspace_id=l.workspace_id AND c.snapshot_id=NEW.snapshot_id AND c.kind='code' AND c.revision_id=l.code_revision_id))) INTO valid;
    END IF;
    IF NOT valid THEN RAISE EXCEPTION 'snapshot member identity or revision mismatch' USING ERRCODE='23503'; END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER source_snapshot_member_integrity AFTER INSERT ON source_snapshot_members DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_source_snapshot_member();

CREATE FUNCTION validate_source_snapshot_head() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_TABLE_NAME='source_effective_snapshots' THEN
        IF NOT EXISTS(SELECT 1 FROM source_snapshots s WHERE s.workspace_id=NEW.workspace_id AND s.id=NEW.snapshot_id AND s.source_connection_id=NEW.source_connection_id AND s.scope_digest=NEW.scope_digest AND s.history_quality='verified' AND s.coverage_status='complete') THEN
            RAISE EXCEPTION 'effective snapshot must be verified complete' USING ERRCODE='23514';
        END IF;
    ELSIF NEW.latest_verified_snapshot_id IS NOT NULL THEN
        IF NOT EXISTS(SELECT 1 FROM source_snapshots s JOIN source_snapshot_scope c ON c.workspace_id=s.workspace_id AND c.snapshot_id=s.id
          WHERE s.workspace_id=NEW.workspace_id AND s.id=NEW.latest_verified_snapshot_id AND s.history_quality='verified' AND c.coverage_key=NEW.coverage_key AND c.status='complete' AND c.enumeration_complete
          AND NEW.selector_digest='sha256:'||encode(sha256(convert_to(c.selector,'UTF8')),'hex')) THEN
            RAISE EXCEPTION 'coverage head must reference a verified complete unit' USING ERRCODE='23514';
        END IF;
    END IF;
    IF TG_OP='UPDATE' AND NEW.version<>OLD.version+1 THEN RAISE EXCEPTION 'snapshot head version mismatch' USING ERRCODE='40001'; END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER source_effective_snapshots_valid BEFORE INSERT OR UPDATE ON source_effective_snapshots FOR EACH ROW EXECUTE FUNCTION validate_source_snapshot_head();
CREATE TRIGGER source_coverage_heads_valid BEFORE INSERT OR UPDATE ON source_coverage_heads FOR EACH ROW EXECUTE FUNCTION validate_source_snapshot_head();

-- Legacy runs do not prove their full member set, names, scope, or historical bytes.
INSERT INTO source_snapshots(id,workspace_id,source_connection_id,source_revision_id,adapter_version,scope_digest,content_digest,history_quality,coverage_status,created_at)
SELECT id,workspace_id,source_connection_id,source_revision_id,adapter_version,
'sha256:'||encode(sha256('legacy-unknown-scope'::bytea),'hex'),
'sha256:'||encode(sha256(convert_to('legacy-unverifiable:'||id::text,'UTF8')),'hex'),
'unverifiable',CASE WHEN status='failed' THEN 'failed' ELSE 'partial' END,COALESCE(completed_at,created_at)
FROM discovery_runs WHERE source_revision_id IS NOT NULL;
INSERT INTO source_snapshot_scope(workspace_id,snapshot_id,coverage_key,selector,config_digest,status,enumeration_complete,diagnostic_codes)
SELECT workspace_id,id,'unknown','unknown',scope_digest,coverage_status,false,ARRAY['HISTORY_UNVERIFIABLE'] FROM source_snapshots;
INSERT INTO source_snapshot_diagnostics(workspace_id,snapshot_id,ordinal,code,severity,coverage_key,message)
SELECT workspace_id,id,1,'HISTORY_UNVERIFIABLE','blocker','unknown','Historical membership, names and coverage cannot be proven.' FROM source_snapshots;
INSERT INTO source_snapshot_runs(workspace_id,source_connection_id,run_id,snapshot_id)
SELECT workspace_id,source_connection_id,id,id FROM source_snapshots;

CREATE FUNCTION reject_sealed_snapshot_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS(SELECT 1 FROM source_snapshot_runs WHERE workspace_id=NEW.workspace_id AND snapshot_id=NEW.snapshot_id) THEN
        RAISE EXCEPTION 'published source snapshot is sealed' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER source_snapshot_scope_sealed BEFORE INSERT ON source_snapshot_scope FOR EACH ROW EXECUTE FUNCTION reject_sealed_snapshot_insert();
CREATE TRIGGER source_snapshot_members_sealed BEFORE INSERT ON source_snapshot_members FOR EACH ROW EXECUTE FUNCTION reject_sealed_snapshot_insert();
CREATE TRIGGER source_snapshot_diagnostics_sealed BEFORE INSERT ON source_snapshot_diagnostics FOR EACH ROW EXECUTE FUNCTION reject_sealed_snapshot_insert();

CREATE FUNCTION validate_source_snapshot_seal() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE expected_status text; unit_count integer; snapshot_quality text; actual_status text;
BEGIN
    SELECT count(*),CASE WHEN bool_and(status='complete' AND enumeration_complete) THEN 'complete'
      WHEN bool_and(status='failed') THEN 'failed' ELSE 'partial' END INTO unit_count,expected_status
      FROM source_snapshot_scope WHERE workspace_id=NEW.workspace_id AND snapshot_id=NEW.snapshot_id;
    SELECT history_quality,coverage_status INTO snapshot_quality,actual_status FROM source_snapshots WHERE workspace_id=NEW.workspace_id AND id=NEW.snapshot_id;
    IF unit_count NOT BETWEEN 1 AND 256 OR expected_status<>actual_status THEN
        RAISE EXCEPTION 'snapshot coverage does not match its declared completeness' USING ERRCODE='23514';
    END IF;
    IF snapshot_quality='verified' AND EXISTS(SELECT 1 FROM source_snapshot_scope WHERE workspace_id=NEW.workspace_id AND snapshot_id=NEW.snapshot_id AND coverage_key='unknown') THEN
        RAISE EXCEPTION 'unknown coverage cannot establish verified history' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM source_snapshot_diagnostics d JOIN source_snapshot_scope c
      ON c.workspace_id=d.workspace_id AND c.snapshot_id=d.snapshot_id AND c.coverage_key=d.coverage_key
      WHERE d.workspace_id=NEW.workspace_id AND d.snapshot_id=NEW.snapshot_id AND d.severity='blocker' AND c.status='complete') THEN
        RAISE EXCEPTION 'blocked coverage cannot be complete' USING ERRCODE='23514';
    END IF;
    IF EXISTS(SELECT 1 FROM discovery_runs r JOIN source_snapshots s ON s.workspace_id=r.workspace_id AND s.id=NEW.snapshot_id
      WHERE r.workspace_id=NEW.workspace_id AND r.id=NEW.run_id AND (r.source_revision_id IS DISTINCT FROM s.source_revision_id OR r.adapter_version<>s.adapter_version)) THEN
        RAISE EXCEPTION 'snapshot run revision mismatch' USING ERRCODE='23503';
    END IF;
    RETURN NULL;
END; $$;
CREATE CONSTRAINT TRIGGER source_snapshot_runs_integrity AFTER INSERT ON source_snapshot_runs DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_source_snapshot_seal();

CREATE FUNCTION protect_source_snapshot_artifact_bytes() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.state IN ('deleting','deleted') AND EXISTS(SELECT 1 FROM source_revision_artifacts pin JOIN source_snapshots s
      ON s.workspace_id=pin.workspace_id AND s.source_revision_id=pin.source_revision_id
      WHERE pin.workspace_id=NEW.workspace_id AND pin.content_sha256=NEW.content_sha256) THEN
        RAISE EXCEPTION 'artifact bytes are retained by immutable source history' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
END; $$;
CREATE TRIGGER artifact_object_retention_source_history BEFORE UPDATE ON artifact_object_retention FOR EACH ROW EXECUTE FUNCTION protect_source_snapshot_artifact_bytes();
CREATE TRIGGER source_snapshots_immutable BEFORE UPDATE OR DELETE ON source_snapshots FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER source_snapshot_runs_immutable BEFORE UPDATE OR DELETE ON source_snapshot_runs FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER source_snapshot_scope_immutable BEFORE UPDATE OR DELETE ON source_snapshot_scope FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER source_snapshot_members_immutable BEFORE UPDATE OR DELETE ON source_snapshot_members FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER source_snapshot_diagnostics_immutable BEFORE UPDATE OR DELETE ON source_snapshot_diagnostics FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER source_code_revisions_immutable BEFORE UPDATE OR DELETE ON source_code_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER source_lineage_revisions_immutable BEFORE UPDATE OR DELETE ON source_lineage_revisions FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

COMMIT;

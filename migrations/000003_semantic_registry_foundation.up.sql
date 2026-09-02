BEGIN;

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE FUNCTION reject_immutable_registry_row()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% rows are immutable', TG_TABLE_NAME USING ERRCODE = '55000';
END;
$$;

CREATE TABLE source_connections (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    adapter_kind text NOT NULL CHECK (adapter_kind ~ '^[a-z][a-z0-9_]{1,63}$'),
    name text NOT NULL CHECK (name <> ''),
    normalized_locator text NOT NULL CHECK (normalized_locator <> ''),
    credential_ref text CHECK (credential_ref IS NULL OR credential_ref ~ '^[a-z][a-z0-9_./:-]{2,255}$'),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'paused', 'error', 'deleted')),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (
        jsonb_typeof(metadata) = 'object'
        AND NOT (metadata ?| ARRAY['password', 'token', 'secret', 'private_key', 'credential'])
    ),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, adapter_kind, normalized_locator)
);

CREATE INDEX source_connections_workspace_updated_idx
    ON source_connections (workspace_id, updated_at DESC, id);

CREATE TABLE source_revisions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    external_revision text,
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    adapter_version text NOT NULL CHECK (adapter_version <> ''),
    observed_at timestamptz NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (source_connection_id, content_digest),
    FOREIGN KEY (workspace_id, source_connection_id)
        REFERENCES source_connections (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX source_revisions_source_observed_idx
    ON source_revisions (source_connection_id, observed_at DESC, id);

CREATE TRIGGER source_revisions_immutable
    BEFORE UPDATE OR DELETE ON source_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE discovery_runs (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    source_revision_id uuid,
    adapter_version text NOT NULL CHECK (adapter_version <> ''),
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    error_code text CHECK (error_code IS NULL OR error_code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    stats jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(stats) = 'object'),
    started_at timestamptz,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, source_connection_id)
        REFERENCES source_connections (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_revision_id)
        REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT,
    CONSTRAINT discovery_runs_time_state CHECK (
        (status = 'queued' AND started_at IS NULL AND completed_at IS NULL)
        OR (status = 'running' AND started_at IS NOT NULL AND completed_at IS NULL)
        OR (status IN ('succeeded', 'failed', 'cancelled') AND started_at IS NOT NULL AND completed_at IS NOT NULL)
    ),
    CONSTRAINT discovery_runs_error_state CHECK (
        (status = 'failed' AND error_code IS NOT NULL) OR (status <> 'failed' AND error_code IS NULL)
    )
);

CREATE UNIQUE INDEX discovery_runs_success_projection_key
    ON discovery_runs (source_revision_id, adapter_version)
    WHERE status = 'succeeded';

CREATE INDEX discovery_runs_workspace_created_idx
    ON discovery_runs (workspace_id, created_at DESC, id);

CREATE TABLE discovery_findings (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    discovery_run_id uuid NOT NULL REFERENCES discovery_runs (id) ON DELETE CASCADE,
    sequence integer NOT NULL CHECK (sequence > 0),
    code text NOT NULL CHECK (code ~ '^[A-Z][A-Z0-9_]{2,63}$'),
    severity text NOT NULL CHECK (severity IN ('info', 'warning', 'error')),
    locator text,
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details) = 'object'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (discovery_run_id, sequence)
);

CREATE TABLE physical_datasets (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_connection_id uuid NOT NULL,
    external_key text NOT NULL CHECK (external_key <> ''),
    qualified_name text NOT NULL CHECK (qualified_name <> ''),
    current_revision_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (source_connection_id, external_key),
    FOREIGN KEY (workspace_id, source_connection_id)
        REFERENCES source_connections (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX physical_datasets_workspace_name_idx
    ON physical_datasets (workspace_id, qualified_name, id);
CREATE INDEX physical_datasets_qualified_name_trgm_idx
    ON physical_datasets USING gin (qualified_name gin_trgm_ops);

CREATE TABLE physical_dataset_revisions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    physical_dataset_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    dataset_kind text NOT NULL CHECK (dataset_kind IN ('table', 'view', 'materialized_view', 'model', 'external')),
    locator text NOT NULL CHECK (locator <> ''),
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (physical_dataset_id, id),
    UNIQUE (physical_dataset_id, source_revision_id),
    FOREIGN KEY (workspace_id, physical_dataset_id)
        REFERENCES physical_datasets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_revision_id)
        REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT
);

ALTER TABLE physical_datasets
    ADD CONSTRAINT physical_datasets_current_revision_fkey
    FOREIGN KEY (id, current_revision_id)
    REFERENCES physical_dataset_revisions (physical_dataset_id, id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TRIGGER physical_dataset_revisions_immutable
    BEFORE UPDATE OR DELETE ON physical_dataset_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE physical_fields (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    physical_dataset_id uuid NOT NULL,
    external_key text NOT NULL CHECK (external_key <> ''),
    name text NOT NULL CHECK (name <> ''),
    current_revision_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (physical_dataset_id, external_key),
    FOREIGN KEY (workspace_id, physical_dataset_id)
        REFERENCES physical_datasets (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX physical_fields_dataset_name_idx
    ON physical_fields (physical_dataset_id, name, id);
CREATE INDEX physical_fields_name_trgm_idx
    ON physical_fields USING gin (name gin_trgm_ops);

CREATE TABLE physical_field_revisions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    physical_field_id uuid NOT NULL,
    dataset_revision_id uuid NOT NULL,
    ordinal integer NOT NULL CHECK (ordinal > 0),
    data_type text NOT NULL CHECK (data_type <> ''),
    nullable boolean NOT NULL,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (physical_field_id, id),
    UNIQUE (dataset_revision_id, ordinal),
    FOREIGN KEY (workspace_id, physical_field_id)
        REFERENCES physical_fields (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, dataset_revision_id)
        REFERENCES physical_dataset_revisions (workspace_id, id) ON DELETE RESTRICT
);

ALTER TABLE physical_fields
    ADD CONSTRAINT physical_fields_current_revision_fkey
    FOREIGN KEY (id, current_revision_id)
    REFERENCES physical_field_revisions (physical_field_id, id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TRIGGER physical_field_revisions_immutable
    BEFORE UPDATE OR DELETE ON physical_field_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE code_artifacts (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    path text NOT NULL CHECK (path <> '' AND path !~ '(^|/)\.\.(/|$)'),
    blob_oid text,
    language text NOT NULL CHECK (language ~ '^[a-z][a-z0-9_+-]{0,31}$'),
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (source_revision_id, path),
    FOREIGN KEY (workspace_id, source_revision_id)
        REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE lineage_edges (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    source_revision_id uuid NOT NULL,
    upstream_dataset_id uuid NOT NULL,
    downstream_dataset_id uuid NOT NULL,
    edge_kind text NOT NULL CHECK (edge_kind IN ('reads_from', 'writes_to', 'derived_from')),
    code_artifact_id uuid,
    confidence numeric(4, 3) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (source_revision_id, upstream_dataset_id, downstream_dataset_id, edge_kind),
    CHECK (upstream_dataset_id <> downstream_dataset_id),
    FOREIGN KEY (workspace_id, source_revision_id)
        REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, upstream_dataset_id)
        REFERENCES physical_datasets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, downstream_dataset_id)
        REFERENCES physical_datasets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, code_artifact_id)
        REFERENCES code_artifacts (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX lineage_edges_upstream_idx
    ON lineage_edges (workspace_id, upstream_dataset_id, edge_kind, downstream_dataset_id);
CREATE INDEX lineage_edges_downstream_idx
    ON lineage_edges (workspace_id, downstream_dataset_id, edge_kind, upstream_dataset_id);

CREATE TABLE semantic_assets (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    namespace text NOT NULL CHECK (namespace ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'),
    key text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*$'),
    asset_type text NOT NULL CHECK (asset_type IN ('concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment')),
    lifecycle_state text NOT NULL DEFAULT 'draft' CHECK (lifecycle_state IN ('draft', 'active', 'deprecated', 'archived')),
    current_revision_id uuid,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, namespace, key)
);

CREATE INDEX semantic_assets_workspace_updated_idx
    ON semantic_assets (workspace_id, updated_at DESC, id);

CREATE TABLE asset_revisions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL,
    asset_id uuid NOT NULL,
    sequence bigint NOT NULL CHECK (sequence > 0),
    schema_version text NOT NULL CHECK (schema_version ~ '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'),
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    content jsonb NOT NULL CHECK (jsonb_typeof(content) = 'object'),
    created_by text NOT NULL CHECK (created_by <> ''),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (asset_id, id),
    UNIQUE (asset_id, sequence),
    UNIQUE (asset_id, content_digest),
    FOREIGN KEY (workspace_id, asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT
);

ALTER TABLE semantic_assets
    ADD CONSTRAINT semantic_assets_current_revision_fkey
    FOREIGN KEY (id, current_revision_id)
    REFERENCES asset_revisions (asset_id, id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE INDEX asset_revisions_asset_sequence_idx
    ON asset_revisions (asset_id, sequence DESC, id);
CREATE INDEX asset_revisions_content_search_idx
    ON asset_revisions USING gin (to_tsvector('simple', content::text));

CREATE TRIGGER asset_revisions_immutable
    BEFORE UPDATE OR DELETE ON asset_revisions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE resource_aliases (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workspace_id uuid NOT NULL,
    namespace text NOT NULL CHECK (namespace ~ '^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$'),
    key text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]*$'),
    resource_type text NOT NULL DEFAULT 'semantic_asset' CHECK (resource_type = 'semantic_asset'),
    resource_id uuid NOT NULL,
    retired_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, namespace, key),
    FOREIGN KEY (workspace_id, resource_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT
);

CREATE TABLE evidence_artifacts (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    evidence_type text NOT NULL CHECK (evidence_type IN ('declared', 'constrained', 'derived', 'observed', 'inferred')),
    source_revision_id uuid,
    locator text NOT NULL CHECK (locator <> ''),
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, evidence_type, locator, content_digest),
    FOREIGN KEY (workspace_id, source_revision_id)
        REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX evidence_artifacts_source_idx
    ON evidence_artifacts (workspace_id, source_revision_id, id);

CREATE TRIGGER evidence_artifacts_immutable
    BEFORE UPDATE OR DELETE ON evidence_artifacts
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE revision_evidence_links (
    workspace_id uuid NOT NULL,
    asset_revision_id uuid NOT NULL,
    evidence_artifact_id uuid NOT NULL,
    role text NOT NULL CHECK (role IN ('supports', 'constrains', 'observes', 'conflicts')),
    field_path text,
    note text,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (asset_revision_id, evidence_artifact_id, role),
    FOREIGN KEY (workspace_id, asset_revision_id)
        REFERENCES asset_revisions (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, evidence_artifact_id)
        REFERENCES evidence_artifacts (workspace_id, id) ON DELETE RESTRICT
);

CREATE TRIGGER revision_evidence_links_immutable
    BEFORE UPDATE OR DELETE ON revision_evidence_links
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE relation_type_policies (
    predicate text PRIMARY KEY,
    plane text NOT NULL CHECK (plane IN ('taxonomy', 'semantic', 'dependency')),
    source_types text[] NOT NULL CHECK (cardinality(source_types) > 0),
    target_types text[] NOT NULL CHECK (cardinality(target_types) > 0),
    matching_types boolean NOT NULL DEFAULT false,
    cardinality text NOT NULL CHECK (cardinality IN ('one_to_one', 'one_to_many', 'many_to_one', 'many_to_many')),
    reasoning text NOT NULL CHECK (reasoning IN ('directed', 'symmetric')),
    schema_version text NOT NULL CHECK (schema_version ~ '^[0-9]+\.[0-9]+\.[0-9]+$')
);

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
    ('disjoint_with', 'taxonomy', ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], ARRAY['concept', 'entity', 'semantic_model', 'dimension', 'measure', 'metric', 'segment'], true, 'many_to_many', 'symmetric', '1.0.0');

CREATE TABLE semantic_relations (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    subject_asset_id uuid NOT NULL,
    predicate text NOT NULL REFERENCES relation_type_policies (predicate) ON DELETE RESTRICT,
    object_asset_id uuid NOT NULL,
    plane text NOT NULL CHECK (plane IN ('taxonomy', 'semantic', 'dependency')),
    assertion_state text NOT NULL CHECK (assertion_state IN ('asserted', 'inferred', 'candidate', 'deprecated')),
    source_revision_id uuid,
    evidence_artifact_id uuid,
    inference_rule text,
    created_by text NOT NULL CHECK (created_by <> ''),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, subject_asset_id, predicate, object_asset_id, assertion_state),
    CHECK (subject_asset_id <> object_asset_id),
    CHECK ((assertion_state = 'inferred' AND inference_rule IS NOT NULL) OR assertion_state <> 'inferred'),
    FOREIGN KEY (workspace_id, subject_asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, object_asset_id)
        REFERENCES semantic_assets (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, source_revision_id)
        REFERENCES source_revisions (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, evidence_artifact_id)
        REFERENCES evidence_artifacts (workspace_id, id) ON DELETE RESTRICT
);

CREATE INDEX semantic_relations_subject_idx
    ON semantic_relations (workspace_id, subject_asset_id, predicate, object_asset_id);
CREATE INDEX semantic_relations_object_idx
    ON semantic_relations (workspace_id, object_asset_id, predicate, subject_asset_id);

CREATE FUNCTION validate_semantic_relation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    policy relation_type_policies%ROWTYPE;
    subject_type text;
    object_type text;
BEGIN
    SELECT * INTO STRICT policy FROM relation_type_policies WHERE predicate = NEW.predicate;
    SELECT asset_type INTO STRICT subject_type FROM semantic_assets
        WHERE workspace_id = NEW.workspace_id AND id = NEW.subject_asset_id;
    SELECT asset_type INTO STRICT object_type FROM semantic_assets
        WHERE workspace_id = NEW.workspace_id AND id = NEW.object_asset_id;
    IF NEW.plane <> policy.plane
       OR NOT (subject_type = ANY(policy.source_types))
       OR NOT (object_type = ANY(policy.target_types))
       OR (policy.matching_types AND subject_type <> object_type) THEN
        RAISE EXCEPTION 'semantic relation violates predicate policy' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER semantic_relations_validate
    BEFORE INSERT OR UPDATE ON semantic_relations
    FOR EACH ROW EXECUTE FUNCTION validate_semantic_relation();
CREATE TRIGGER semantic_relations_immutable
    BEFORE UPDATE OR DELETE ON semantic_relations
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();

CREATE TABLE ontology_revisions (
    id uuid PRIMARY KEY CHECK (substring(id::text, 15, 1) = '7'),
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE RESTRICT,
    sequence bigint NOT NULL CHECK (sequence > 0),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'superseded')),
    content_digest text NOT NULL CHECK (content_digest ~ '^sha256:[0-9a-f]{64}$'),
    created_by text NOT NULL CHECK (created_by <> ''),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at timestamptz,
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, sequence),
    CONSTRAINT ontology_revisions_publication_time CHECK (
        (status = 'draft' AND published_at IS NULL) OR (status <> 'draft' AND published_at IS NOT NULL)
    )
);

CREATE TABLE ontology_revision_relations (
    workspace_id uuid NOT NULL,
    ontology_revision_id uuid NOT NULL,
    semantic_relation_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (ontology_revision_id, semantic_relation_id),
    FOREIGN KEY (workspace_id, ontology_revision_id)
        REFERENCES ontology_revisions (workspace_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (workspace_id, semantic_relation_id)
        REFERENCES semantic_relations (workspace_id, id) ON DELETE RESTRICT
);

CREATE FUNCTION validate_ontology_relation_membership()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    revision_status text;
    relation_state text;
BEGIN
    SELECT status INTO STRICT revision_status FROM ontology_revisions WHERE id = NEW.ontology_revision_id;
    SELECT assertion_state INTO STRICT relation_state FROM semantic_relations WHERE id = NEW.semantic_relation_id;
    IF revision_status = 'published' AND relation_state NOT IN ('asserted', 'inferred') THEN
        RAISE EXCEPTION 'published ontology cannot include candidate or deprecated relation' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION validate_ontology_publication()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.status = 'published' AND OLD.status <> 'published' THEN
        IF NOT EXISTS (
            SELECT 1 FROM ontology_revision_relations WHERE ontology_revision_id = NEW.id
        ) OR EXISTS (
            SELECT 1
            FROM ontology_revision_relations member
            JOIN semantic_relations relation ON relation.id = member.semantic_relation_id
            WHERE member.ontology_revision_id = NEW.id
              AND relation.assertion_state NOT IN ('asserted', 'inferred')
        ) THEN
            RAISE EXCEPTION 'ontology publication requires accepted relation membership' USING ERRCODE = '23514';
        END IF;
    END IF;
    IF OLD.status <> 'draft' AND NEW.status <> OLD.status AND NOT (OLD.status = 'published' AND NEW.status = 'superseded') THEN
        RAISE EXCEPTION 'invalid ontology revision status transition' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER ontology_revision_relations_validate
    BEFORE INSERT OR UPDATE ON ontology_revision_relations
    FOR EACH ROW EXECUTE FUNCTION validate_ontology_relation_membership();
CREATE TRIGGER ontology_revision_relations_immutable
    BEFORE UPDATE OR DELETE ON ontology_revision_relations
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_registry_row();
CREATE TRIGGER ontology_revisions_validate_publication
    BEFORE UPDATE ON ontology_revisions
    FOR EACH ROW EXECUTE FUNCTION validate_ontology_publication();

COMMIT;

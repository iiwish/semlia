# M1 Semantic Registry Data Model

## Storage Contract

PostgreSQL owns transactional state, uniqueness, current indexes, audit and delivery. Git owns reviewable canonical content and release projections. Raw source artifacts may be retained by content digest, but core references are normalized columns rather than JSON paths.

All public domain IDs use UUIDv7 in `uuid` columns and TypeID at boundaries. Local sequences use `bigint GENERATED ALWAYS AS IDENTITY` only where they never leave the database.

Foundation jobs use `run_` identities; audit and outbox rows use `evt_` identities. They share neither a prefix nor a typed API wrapper even when an outbox event was produced by a run.

## Aggregate Map

| Aggregate | Core tables | Invariant |
| --- | --- | --- |
| Workspace | `workspaces`, `resource_aliases` | Slug and semantic address uniqueness are workspace scoped. |
| Source | `source_connections`, `source_revisions`, `discovery_runs`, `discovery_findings` | Source revisions are immutable and content-addressed; credentials are external references. |
| Physical graph | `physical_datasets`, `physical_fields`, `code_artifacts`, `lineage_edges`, `join_observations` | Every node and edge cites a source revision and locator. |
| Semantic registry | `semantic_assets`, `asset_revisions`, `evidence_artifacts`, `revision_evidence_links` | Stable asset identity is separate from immutable revision identity and local sequence. |
| Ontology projection | `semantic_relations`, `ontology_revisions`, `ontology_revision_relations` | Only registered predicates and valid endpoint types enter a published projection. |
| Delivery and learning | existing `audit_events`, `jobs`, `outbox_events`; new `usage_events` | Domain change, audit and outbox enqueue commit atomically; usage payload is minimal and retention bounded. |

## Key Shapes

### Source and discovery

- `source_connections(id, workspace_id, adapter_kind, name, locator, credential_ref, status, created_at, updated_at)`
- `source_revisions(id, source_connection_id, external_revision, content_digest, observed_at, metadata)` with unique `(source_connection_id, content_digest)`
- `discovery_runs(id, source_connection_id, source_revision_id, status, started_at, completed_at, error_code, stats)`
- `discovery_findings(id bigint identity, discovery_run_id, code, severity, locator, details)`

`credential_ref` names a secret-provider entry and never contains a token. `metadata`, `stats` and `details` are schema-bounded JSONB facets, not foreign-key substitutes.

### Physical graph

- `physical_datasets(id, workspace_id, source_connection_id, external_key, qualified_name, current_revision_id)`
- `physical_dataset_revisions(id, physical_dataset_id, source_revision_id, dataset_kind, locator, content_digest, metadata)`
- `physical_fields(id, physical_dataset_id, external_key, name, current_revision_id)`
- `physical_field_revisions(id, physical_field_id, dataset_revision_id, ordinal, data_type, nullable, metadata)`
- `code_artifacts(id, workspace_id, source_revision_id, path, blob_oid, language, content_digest)`
- `lineage_edges(id, workspace_id, source_revision_id, upstream_dataset_id, downstream_dataset_id, edge_kind, code_artifact_id, confidence)`
- `join_observations(id, workspace_id, source_revision_id, left_field_id, right_field_id, observation_kind, confidence, evidence_artifact_id)`

Stable cross-revision matching uses adapter-owned external keys plus explicit reconciliation. Qualified-name fallback creates a new identity and an unresolved rename finding; it never silently assumes a renamed object is unchanged.

### Semantic assets and revisions

- `semantic_assets(id, workspace_id, namespace, key, asset_type, current_revision_id, lifecycle_state, created_at, updated_at)`
- `asset_revisions(id, asset_id, sequence, schema_version, content_digest, content, created_by, created_at)`
- `resource_aliases(id bigint identity, workspace_id, resource_type, namespace, key, resource_id, retired_at)`
- `evidence_artifacts(id, workspace_id, evidence_type, source_revision_id, locator, content_digest, metadata, created_at)`
- `revision_evidence_links(asset_revision_id, evidence_artifact_id, role, note)`

Required constraints include unique `(workspace_id, namespace, key)`, unique `(asset_id, sequence)`, unique evidence identity by workspace/type/digest/locator, a deferrable current-revision foreign key, and immutable revision/evidence triggers.

### Typed relations and ontology

- `semantic_relations(id, workspace_id, subject_asset_id, predicate, object_asset_id, assertion_state, source_revision_id, evidence_artifact_id, created_at)`
- `ontology_revisions(id, workspace_id, sequence, status, content_digest, created_by, created_at, published_at)`
- `ontology_revision_relations(ontology_revision_id, semantic_relation_id)`

M1 predicates are closed and versioned: `broader_than`, `narrower_than`, `equivalent_to`, `disjoint_with`, `synonym_of`, `contains`, `measures`, `describes`, `filters_by`, `depends_on` and `derived_from`. Endpoint type rules live in domain validation and are mirrored by database checks where practical. `asserted`, `inferred`, `candidate` and `deprecated` are explicit states; inference provenance must identify the rule and inputs. Published ontology revisions contain only asserted or validated inferred relations.

The ontology is a governed projection over semantic assets, not an independent truth store. Recursive CTEs serve bounded hierarchy and impact reads; a graph database requires benchmark evidence before adoption.

## Migration Strategy

1. Add UUID columns and deterministic backfill mappings for M0 workspace, audit, job and outbox rows.
2. Rewrite all foreign keys and subject references in one controlled migration, validate orphan counts, then make UUID columns authoritative.
3. Add M1 tables and indexes in a separate forward migration; never edit `000001_m0_foundation`.
4. Keep the API on TypeID throughout the migration. Raw UUIDs stay inside PostgreSQL adapters and diagnostic-only tooling.
5. Prove up/down/up on an empty database and forward migration on an M0 fixture containing jobs, outbox and audit data.

## Index And Search Baseline

- B-tree indexes cover workspace, source revision, lifecycle, semantic address and deterministic cursor order.
- PostgreSQL full-text search plus `pg_trgm` covers names, qualified names and definitions for the 10,000-table M1 benchmark.
- Search ranking is explainable and tested against a golden set. Vector search is deferred until lexical and structural retrieval evidence shows a gap.
- Recursive relation queries are capped at three hops and return a textual fallback from the same result set.

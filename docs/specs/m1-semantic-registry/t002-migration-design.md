# T002 PostgreSQL Foundation Design

## Status

Design complete; implementation remains blocked until T001 is explicitly accepted.

## Scope

T002 converts the four M0 foundation aggregates from unconstrained text identity to UUIDv7, adds the normalized M1 source/physical/semantic/evidence/ontology schema, regenerates sqlc models and introduces repository ports. It does not parse source files, expose HTTP routes or write Git content.

Two forward migrations keep review and rollback boundaries visible:

| Migration | Responsibility |
| --- | --- |
| `000002_foundation_uuidv7` | Convert workspace, audit, job and outbox keys to native `uuid`; retain reversible legacy aliases. |
| `000003_semantic_registry_foundation` | Add source, physical observation, semantic revision, evidence and finite ontology tables. |

`000001_m0_foundation` remains byte-for-byte unchanged.

## Foundation Identity Conversion

### Backfill rule

Existing M0 rows need valid UUIDv7 values even on PostgreSQL 17. Migration `000002` uses a migration-local deterministic transform:

1. The first 48 bits encode the row `created_at` Unix timestamp in milliseconds.
2. The remaining bits come from a deterministic hash of `resource_kind`, a separator and the legacy ID.
3. The UUID version nibble is fixed to `7` and the RFC 9562 variant bits to `10`.
4. A unique constraint makes any theoretical collision abort the migration rather than merge identities.
5. The helper function is dropped before the migration commits. All post-migration runtime generation remains in Go `pkg/identity`.

This is a compatibility transform, not a second runtime ID generator. It is deterministic so a failed migration can be diagnosed and replayed, needs no PostgreSQL extension, and works on the PostgreSQL 17 compatibility floor.

### Column transition

For each foundation table:

1. Drop dependent primary/foreign keys and ID-ordered indexes by explicit name.
2. Rename `id` to `legacy_id`; rename child `workspace_id` to `legacy_workspace_id`.
3. Add new `id uuid` and `workspace_id uuid` columns.
4. Backfill workspace IDs first, then child IDs and child workspace references by join.
5. Assert non-null counts, uniqueness, UUID version 7 and zero orphan references in a guarded `DO` block.
6. Add UUID primary/foreign keys and recreate operational indexes with their canonical names.
7. Keep nullable, unique legacy columns through M1 for rollback and old-log resolution. New writes leave them null.

The audit immutability trigger is dropped immediately before its controlled backfill and recreated before commit. No other mutation bypass is introduced.

### Down behavior

The down migration first drops M1 foreign keys and UUID indexes. For foundation rows created after migration, it fills missing legacy IDs with the UUID text form; M0 accepts any non-empty stable text value. It resolves missing child legacy workspace IDs from the parent legacy value, restores text columns and original constraints, then removes UUID columns.

Down migration is lossless for identity and row relationships but deliberately does not promise to recreate the user-facing TypeID string in SQL. TypeID is an exchange encoding, not stored state.

## M1 Schema

### Identity additions

T002 extends the central prefix registry and OpenAPI components before generating sqlc/repository code:

| Prefix | Resource |
| --- | --- |
| `src_` | SourceConnection |
| `srv_` | SourceRevision |
| `pds_` | PhysicalDataset |
| `pdr_` | PhysicalDatasetRevision |
| `pfd_` | PhysicalField |
| `pfr_` | PhysicalFieldRevision |
| `cod_` | CodeArtifact |
| `lin_` | LineageEdge |

These are distinct wrappers. Physical revisions do not reuse `rev_`, which remains AssetRevision identity.

### Source aggregates

`source_connections`

- Stable `src_` identity, workspace ownership, adapter kind, display name and normalized locator.
- `credential_ref` is an opaque secret-manager reference. Tokens, passwords and private keys are rejected from metadata.
- Status is one of `active`, `paused`, `error`, `deleted`.
- Unique `(workspace_id, adapter_kind, normalized_locator)`.

`source_revisions`

- Stable `srv_` identity, source connection, external revision, content digest and observation time.
- Immutable after insert.
- Unique `(source_connection_id, content_digest)` makes repeated snapshots idempotent.

`discovery_runs` and `discovery_findings`

- `run_` identity represents queued/running/succeeded/failed/cancelled discovery.
- A run may target an already-known source revision but cannot produce two successful projections for the same adapter version and content digest.
- Findings use a database-local identity column because they never leave the run aggregate; public references use `(run_id, sequence)`.

### Physical observation model

`physical_datasets` and `physical_fields` are stable source-scoped identities. Adapters provide a normalized `external_key`; when no stable upstream key exists, the adapter derives it from the qualified name and reports possible renames as unresolved findings rather than silently reusing identity.

Immutable `physical_dataset_revisions` and `physical_field_revisions` attach observed shape to a `source_revision_id`. Current pointers are projections only and never overwrite history. Dataset and field revision IDs use `pdr_` and `pfr_`; they cannot be passed where an `rev_` AssetRevision is required.

`code_artifacts` store source revision, repository path, blob OID, language and content digest, not full repository content. `lineage_edges` cite upstream/downstream stable dataset IDs, the observing source revision and optional code artifact.

### Semantic registry

`semantic_assets`

- Stable `ast_` identity with workspace-scoped `(namespace, key)` uniqueness.
- Seven primary asset types from the confirmed contract.
- Lifecycle: `draft`, `active`, `deprecated`, `archived`.
- `current_revision_id` is nullable only while the first revision is created in the same transaction and is a deferrable foreign key.

`asset_revisions`

- Immutable `rev_` identity, local positive sequence, schema version, SHA-256 digest and schema-bounded JSON content.
- Unique `(asset_id, sequence)` and `(asset_id, content_digest)`.
- The revision trigger rejects UPDATE and DELETE.

`resource_aliases`

- Database-local identity column plus workspace, namespace, key, resource type and stable UUID target.
- Alias reuse for another resource is prohibited even after retirement.

### Evidence

`evidence_artifacts` use `evd_` identity and carry evidence authority, optional source revision, locator, digest and bounded metadata. `revision_evidence_links` is a normalized many-to-many relation with role and optional field path. Evidence rows and links used by an immutable revision cannot be mutated; corrections create new evidence and a new asset revision.

### Finite ontology

`relation_type_policies` is a versioned, seeded registry for the eleven accepted predicates. It owns plane, allowed source/target asset types, matching-type requirement, cardinality and reasoning mode.

`semantic_relations` use `rel_` identity and cite stable subject/object assets, source revision, evidence and assertion state. A trigger reads `relation_type_policies` and asset endpoint types to reject invalid combinations. Self-relations are rejected except where a future predicate policy explicitly permits them.

`ontology_revisions` use `ont_` identity and workspace-local sequence. `ontology_revision_relations` freezes membership. Only `asserted` or validated `inferred` relations may enter a published revision; candidate and deprecated relations remain queryable but cannot enter the published projection.

Bounded hierarchy and impact reads use recursive CTEs capped at three hops. T002 adds supporting indexes but no graph database.

## Transaction And Repository Boundaries

- Source snapshot persistence inserts source revision, discovery run result, physical observations, findings, audit and outbox in one transaction.
- Semantic revision persistence inserts revision, evidence links and relation candidates, updates the asset current pointer, and enqueues audit/outbox atomically.
- Repository interfaces use domain and `pkg/identity` types. `pgtype.UUID` is confined to the PostgreSQL adapter.
- Every query takes `workspace_id` explicitly even when a foreign-key chain could infer it.
- Not-found, conflict and invariant violations map to stable repository errors; raw PostgreSQL errors do not escape the adapter.

## Required Indexes

- Deterministic cursor order: `(workspace_id, updated_at DESC, id)` for stable objects and `(asset_id, sequence DESC)` for revisions.
- Discovery idempotency: source/content digest and run adapter-version uniqueness.
- Physical lookup: workspace/source/external key, qualified name and source revision.
- Catalog search: generated `tsvector` over semantic name/definition and physical qualified names plus `pg_trgm` indexes.
- Relation reads: subject/predicate/object and object/predicate/subject composites.
- Evidence lookup: workspace/type/digest and source revision.

Search extensions are created explicitly and verified on PostgreSQL 17 and 18. If `pg_trgm` is unavailable, migration fails with an actionable prerequisite rather than silently changing search behavior.

## Acceptance Matrix

| ID | Test | Required assertion |
| --- | --- | --- |
| DB-001 | Empty up/down/up | Versions 1 -> 3 -> 0 -> 3 are clean and table inventories match. |
| DB-002 | Populated M0 upgrade | Every legacy row maps to UUIDv7; counts, FKs, job leases, audit immutability and outbox state survive. |
| DB-003 | New typed writes | sqlc/repository writes and reads typed IDs without exposing `pgtype.UUID`. |
| DB-004 | Workspace isolation | Cross-workspace source, asset, evidence and relation references fail. |
| DB-005 | Revision immutability | Asset/source/physical revisions and evidence links reject mutation and deletion. |
| DB-006 | Address and sequence | Duplicate semantic address or revision sequence fails; same key in another workspace succeeds. |
| DB-007 | Discovery idempotency | Same source digest returns the existing source revision and does not duplicate observations. |
| DB-008 | Relation policy | Wrong plane, endpoint type, assertion state and invalid self-edge fail in PostgreSQL as well as domain code. |
| DB-009 | Ontology publication | Candidate/deprecated or cross-workspace relations cannot enter a published ontology revision. |
| DB-010 | Query plans | Cursor, claim, catalog and relation queries use the intended indexes with sequential scans disabled. |
| DB-011 | Rollback identity | New UUID-era foundation rows survive down migration as stable non-empty M0 text IDs. |
| DB-012 | PostgreSQL floor | Migration and repository suite passes on PostgreSQL 17 and canonical PostgreSQL 18. |

## Stop Conditions

- A populated M0 row cannot be converted without losing identity or relationship information.
- PostgreSQL 17 requires a runtime UUIDv7 or extension-specific function.
- sqlc forces public/domain packages to depend on `pgtype.UUID`.
- A relation or ontology invariant can only be documented, not enforced in domain and database tests.
- Down migration would delete or merge a row created after the UUID migration.

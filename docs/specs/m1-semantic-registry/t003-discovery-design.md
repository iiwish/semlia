# T003 Source Discovery Design

## Status

Confirmed through the founder's explicit T002 acceptance and instruction to complete T003 on 2026-09-02.

## Outcome

T003 introduces one domain-neutral discovery port with three adapters:

| Adapter | Accepted input | Output |
| --- | --- | --- |
| Catalog | Versioned Semlia catalog snapshot JSON | Physical datasets and fields with stable external keys |
| PostgreSQL SQL | Versioned DDL and view SQL files | Tables, views, fields, code artifacts and direct lineage |
| dbt | `manifest.json` v12 plus `catalog.json` v1 | Models, sources, columns, code locators and dependency lineage |

All adapters emit the same deterministic snapshot. They do not write PostgreSQL directly and never expose parser or artifact DTO types outside their adapter packages.

## Reuse Decisions

- Adopt direct `github.com/pganalyze/pg_query_go/v6` use behind the PostgreSQL SQL adapter. The library uses the PostgreSQL 17 parser and provides structured parse trees, normalization and fingerprints.
- Pin dbt artifact compatibility to the published manifest v12 and catalog v1 contracts at `schemas.getdbt.com`. The adapter verifies the published schema identifiers before mapping versioned DTOs; unknown versions emit one terminal unresolved finding and no partial projection.
- Use the standard library SHA-256 and canonical JSON encoding for content and object fingerprints. TypeID remains the only identity generator.
- Reuse the T002 normalized tables, sqlc, pgx transactions, job/outbox runtime and repository error boundary. Add no migration, workflow engine, graph store, vector store or custom SQL parser.

## Discovery Contract

An adapter receives immutable artifact bytes, an external revision, locator and observation time. It returns:

- adapter kind and version;
- a source content digest;
- stable physical datasets and fields;
- optional code artifacts and direct lineage references;
- explicit findings for unsupported statements, unresolved view fields, missing dependencies, possible renames or artifact incompatibility.

Every dataset and field carries a deterministic fingerprint over its normalized shape. Collection order, JSON map order, whitespace and SQL comments do not alter the fingerprint when semantic structure is unchanged.

## Incremental Persistence

The application service generates IDs and asks one repository transaction to persist the snapshot:

1. Insert or resolve the immutable source revision by `(source_connection_id, content_digest)`.
2. If the revision already exists, return the existing successful run without duplicating physical rows.
3. Upsert stable datasets and fields by adapter-owned external key.
4. Insert immutable physical revisions only when their fingerprint changed; otherwise retain the current pointer.
5. Insert code artifacts, resolved lineage and ordered findings.
6. Insert a succeeded discovery run with statistics and commit the complete projection atomically.

Qualified-name-only matching never silently claims a rename. A new external key creates a new identity; a matching prior qualified name produces `POSSIBLE_RENAME` for reconciliation.

## Failure Boundary

- Invalid bytes or malformed supported artifacts return a typed adapter error and write no partial snapshot.
- Unsupported dbt schema versions return a terminal unresolved finding with zero projected objects.
- Valid SQL statements outside the supported table/view subset emit `UNSUPPORTED_SQL_STATEMENT` and allow supported statements in the same artifact to continue.
- Unresolved lineage emits a finding and does not create an orphan edge.
- Credentials, raw database rows and rendered query results are never accepted by the adapter contract.

## Acceptance Matrix

| ID | Proof |
| --- | --- |
| DISC-001 | Catalog fixture maps stable datasets and fields and is order-independent. |
| DISC-002 | PostgreSQL parser handles quoted tables, fields, views and direct lineage without regular expressions. |
| DISC-003 | Invalid SQL fails closed; supported and unsupported statements produce deterministic findings. |
| DISC-004 | dbt manifest v12/catalog v1 map models, sources, fields and dependencies. |
| DISC-005 | Unknown dbt versions produce no partial projection and one terminal finding. |
| DISC-006 | Replaying identical content returns the existing source revision/run without duplicate observations. |
| DISC-007 | Changed field shape creates new immutable revisions only for affected objects. |
| DISC-008 | Unresolved dependencies and possible renames remain explicit findings. |
| DISC-009 | One PostgreSQL transaction preserves workspace isolation and rolls back on invariant failure. |
| DISC-010 | Parser build and focused benchmark evidence are recorded. |

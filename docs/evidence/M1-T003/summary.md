# M1 T003 Evidence Summary

## Metadata

| Field | Value |
| --- | --- |
| Task | T003 Source discovery adapters |
| Status | `Needs_Review` |
| Implementation | `7ac5197` |
| Date | 2026-09-02 |
| Packet | `docs/specs/m1-semantic-registry/packets/T003.yaml` |

## Delivered

- Added one adapter-neutral discovery contract for immutable input bytes, deterministic snapshots,
  physical datasets, fields, code artifacts, direct lineage and explicit findings.
- Added a strict versioned Semlia Catalog JSON adapter.
- Added a PostgreSQL DDL/view adapter backed by `pg_query_go/v6`, with tables, views, materialized
  views, fields, direct lineage and unsupported-statement findings. No regular-expression SQL parser
  or PostgreSQL AST type crosses the adapter boundary.
- Added a dbt manifest v12/catalog v1 adapter validated against embedded published JSON Schemas.
  Unknown versions produce one terminal finding and zero physical projection.
- Added an application registry/service and one pgx transaction that resolves immutable source
  revisions, replays successful content idempotently, creates only changed physical revisions,
  records code/lineage/findings and commits the completed discovery run atomically.
- Kept rename and unresolved-lineage claims explicit. A qualified-name match with a new external key
  produces `POSSIBLE_RENAME`; unresolved endpoints produce `UNRESOLVED_LINEAGE` without an orphan edge.

## Reuse Proof

- Direct parser dependency: `github.com/pganalyze/pg_query_go/v6 v6.2.2`.
- Direct JSON Schema dependency: `github.com/santhosh-tekuri/jsonschema/v6 v6.0.2`.
- Embedded dbt catalog v1 schema SHA-256:
  `2b7556f5c30cdaf581f0078aa9d1417d0d05f79de41ace3a6219a2135d03fac5`.
- Embedded dbt manifest v12 schema SHA-256:
  `bbe3ef98aa87e33e7c26401a4cdcb48d22b68655ecbbf5a8c7df8e2ba5852df9`.
- Reused the T002 schema, TypeID identity package, sqlc generation, pgx transaction boundary and
  repository error mapping. T003 adds no migration or new infrastructure service.

## Database Proof

- An identical PostgreSQL SQL snapshot returns the existing source revision and successful run and
  creates no duplicate physical observations.
- A changed table shape creates a new source revision, one affected dataset revision and coherent
  field revisions while retaining the unchanged view revision.
- Possible renames, unresolved lineage and unsupported dbt schema versions persist ordered findings.
- Invalid self-lineage and cross-workspace source references roll back the complete transaction and
  leave no source revision, run, physical dataset or lineage residue.
- An unsupported dbt version persists a failed discovery run with one finding and no physical rows.

## Validation

| Command | Result |
| --- | --- |
| `go test ./internal/domain/discovery/... ./internal/adapters/discovery/... ./internal/application/discovery/...` | Pass |
| `go test -race ./internal/domain/discovery/... ./internal/adapters/discovery/...` | Pass |
| `go test -count=1 ./tests/integration/discovery/... ./tests/integration/db/...` | Pass; discovery 7.951s, database 17.943s |
| `go test ./internal/adapters/discovery/postgresql -run '^$' -bench BenchmarkPostgreSQLDDLAdapter -benchmem -count=1` | Pass; 25,095 ns/op, 5,592 B/op, 79 allocs/op on Apple M5 |
| `make db-generate-check` | Pass |
| `make check-source` | Pass; format, vet/lint, typecheck, Go/TypeScript tests, contract/sqlc/Web drift and release build |
| `git diff --check` | Pass |

## Review

The adapters operate on artifact bytes rather than credentials, database rows or rendered query
results. Stable external keys and canonical fingerprints stay in the shared domain snapshot while
all versioned artifact DTOs, parser traversal and published schemas remain private to their adapter.
Persistence uses the T002 normalized model instead of JSONB projections or a second graph store.

T003 is ready for founder acceptance. T004 remains blocked until that explicit acceptance.

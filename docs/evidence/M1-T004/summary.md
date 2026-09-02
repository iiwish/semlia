# M1 T004 Evidence Summary

## Metadata

| Field | Value |
| --- | --- |
| Task | T004 Catalog and Revision API |
| Status | `Accepted` |
| Implementation | Pending commit |
| Date | 2026-09-02 |
| Packet | `docs/specs/m1-semantic-registry/packets/T004.yaml` |

## Delivered

- Added generated Catalog, Revision, Evidence, Relation and Discovery Run OpenAPI contracts with
  TypeID-only public identities and stable safe errors.
- Added deterministic filter-bound cursor pagination and PostgreSQL address/full-text catalog search.
- Added immutable revision and evidence reads plus bounded three-hop relation traversal.
- Added asset and revision mutations that commit domain state, audit and outbox records in one
  PostgreSQL transaction.
- Wired the catalog service into the production HTTP runtime while preserving database-optional
  readiness behavior.

## Database Proof

- Real PostgreSQL proves immutable historical revisions, current-pointer movement, deterministic
  cursor replay, exact-address and full-text search, workspace isolation and ordered discovery reads.
- Invalid evidence references roll back the asset/revision, audit and outbox rows together.
- Relation traversal is bounded to three hops and deduplicates nodes and edges.

## Validation

| Command | Result |
| --- | --- |
| `make contracts-check` | Pass |
| `make db-generate-check` | Pass |
| `go test -race ./internal/domain/catalog/... ./internal/application/catalog/... ./internal/platform/http/...` | Pass |
| `go test -count=1 ./tests/integration/catalog/...` | Pass; PostgreSQL 18 container |
| `make check-source` | Pass; format, lint, typecheck, all Go/TypeScript tests, drift and release build |
| `git diff --check` | Pass |

## Review

The API exposes only prefix-checked public identifiers and bounded query controls. PostgreSQL stays
the transactional/search authority; T004 adds no search service, graph database, vector store,
workflow engine or Git writer. The founder's continuous execution authorization accepts this task
without a separate approval pause and advances T005.

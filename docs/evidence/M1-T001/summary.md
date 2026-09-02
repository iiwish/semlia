# M1 T001 Evidence Summary

## Metadata

| Field | Value |
| --- | --- |
| Task | T001 Identity and public contracts |
| Status | `Accepted` |
| Implementation | `ffb2eb7`; review correction `7ba8289` |
| Date | 2026-09-02 |
| Accepted | 2026-09-02 by founder |
| Packet | `docs/specs/m1-semantic-registry/packets/T001.yaml` |

## Delivered

- Pinned stable `go.jetify.com/typeid` v1.3.0 and wrapped it in public `pkg/identity` types.
- Added runtime prefix validation, UUIDv7 enforcement, UUID/TypeID round trip, zero-ID rejection and JSON/text encoding.
- Separated durable run identity (`run_`) from audit/outbox event identity (`evt_`) and narrowed EventEnvelope fields to their exact generated types.
- Added compile-time-distinct Go resource IDs through generated OpenAPI aliases and branded TypeScript ID helpers.
- Replaced the obsolete uppercase ULID contract with lowercase TypeID patterns and M1 resource schemas.
- Added dependency-free semantic address, immutable revision and typed ontology relation validation aligned with the accepted prototype.
- Added no migration, repository, route, handler, Cube, Temporal, graph or vector behavior.

## Validation

| Command | Result |
| --- | --- |
| `go test ./pkg/identity/... ./internal/domain/semantic/... ./tests/contracts/...` | Pass |
| `pnpm --filter @semlia/sdk-typescript test` | Pass |
| `make contracts-check` | Pass |
| `make check-source` | Pass; formatting, lint, typecheck, all Go/JS tests, generated-contract drift, migration drift, embedded Web drift and release build |
| `git diff --check` | Pass |

The complete Go run includes the real PostgreSQL integration, worker, repository, smoke and acceptance packages. Frontend tests remain green: prototype 35 tests and production Web 5 tests.

## Review

The implementation matches TDR-0003: PostgreSQL-facing UUID values and public TypeIDs remain one identity, prefixes are centrally registered, and auto-increment behavior is not introduced. Generated Go contracts import the public identity package rather than an `internal` package, so downstream Go consumers are not blocked by visibility rules. Review also proved event and run identifiers are not interchangeable.

Residual work belongs to T002: add the forward M0 identity migration, M1 tables, sqlc queries and real PostgreSQL invariant tests. Founder acceptance on 2026-09-02 closes T001.

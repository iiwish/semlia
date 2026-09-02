# M1 T002 Evidence Summary

## Metadata

| Field | Value |
| --- | --- |
| Task | T002 PostgreSQL registry foundation |
| Status | `Accepted` |
| Implementation | `a6987af` |
| Date | 2026-09-02 |
| Packet | `docs/specs/m1-semantic-registry/packets/T002.yaml` |

## Delivered

- Converted M0 workspace, job, audit and outbox identities to native PostgreSQL UUIDv7 columns while retaining reversible legacy aliases.
- Added the normalized source, discovery, physical observation, semantic asset revision, evidence, typed relation and finite ontology schema.
- Added TypeID wrappers and public contract schemas for source and physical resources without exposing `pgtype.UUID` beyond the PostgreSQL adapter.
- Added sqlc operations and a domain-neutral repository port for idempotent source revisions, atomic asset revision pointers, evidence, relations and ontology publication.
- Preserved M0 job lease, idempotency, audit immutability and outbox behavior after the identity migration.
- Kept T002 free of HTTP routes, source parsers, Git writes, graph databases, vectors, Cube and Temporal.

## Database Proof

- Empty PostgreSQL 18 databases pass `up -> down -> up` at migration version 3 with the exact table inventory.
- Populated M0 databases deterministically backfill UUIDv7 identities, preserve row counts, relationships and job/outbox state, then restore original legacy text identities on rollback.
- Rows created after migration receive stable UUID text identities if rolled back to M0.
- PostgreSQL 17 applies, exercises the typed repository, rolls back and reapplies the same migrations.
- Source, asset, physical and evidence revisions reject mutation; workspace-crossing references fail; duplicate addresses and sequences fail.
- Relation endpoint policies and ontology publication rules reject invalid planes, endpoints and candidate membership.

## Validation

| Command | Result |
| --- | --- |
| `make contracts-check` | Pass |
| `make db-generate-check` | Pass |
| `go test -count=1 ./tests/integration/db/... ./tests/integration/worker/...` | Pass; PostgreSQL 18/17 database suite 80.410s and worker suite 43.116s |
| `go test -race ./internal/domain/semantic/... ./pkg/identity/...` | Pass |
| `go test ./...` | Pass |
| `make check-source` | Pass; format, vet/lint, typecheck, Go/TypeScript tests, generated-contract and migration drift, Web embed and release build |
| `git diff --check` | Pass |

## Review

The migration keeps `000001_m0_foundation` unchanged, drops the audit immutability trigger only for the bounded identity backfill and recreates it before commit. UUID version constraints, composite workspace foreign keys, immutable revision triggers, the seeded eleven-predicate policy registry and ontology publication triggers enforce the confirmed design in PostgreSQL.

Repository methods exchange only Semlia identity and domain types. Stable conflict, not-found and invariant errors replace raw PostgreSQL failures. Asset revision insertion and current-pointer movement share one real PostgreSQL transaction. Replayed source snapshots and evidence identities return the existing immutable row rather than issuing a prohibited no-op update.

Founder accepted T002 on 2026-09-02 and authorized T003 source discovery adapter execution.

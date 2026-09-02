# M1 Semantic Registry Backend Work Graph

## Status

- `Ready`: dependencies, checklist, analysis and packet are complete.
- `Running`: implementation is active.
- `Needs_Review`: implementation and evidence pass; user acceptance remains.
- `Accepted`: explicitly accepted or included in an accepted milestone.
- `Blocked`: a named condition prevents safe progress.

## Work Units

### T001 Identity and public contracts

Status: Accepted
Depends on: M0 Accepted, TDR-0002, TDR-0003
Blocks: T002, T004

Deliver a shared prefix-safe TypeID/UUIDv7 package, correct the obsolete ULID OpenAPI contract, define M1 resource schemas and introduce domain types for immutable assets, revisions, evidence and typed relations. No persistence or endpoint behavior is added.

Validation: focused unit tests, `make contracts`, `make contracts-check`, `go test ./...`, `go vet ./...`.

Packet: `docs/specs/m1-semantic-registry/packets/T001.yaml`.

Execution: implementation `ffb2eb7`; focused identity/domain/contract tests, TypeScript typecheck and the complete `make check-source` gate pass. Evidence: `docs/evidence/M1-T001/summary.md`.

Acceptance: founder explicitly accepted T001 on 2026-09-02.

### T002 PostgreSQL registry foundation

Status: Accepted
Depends on: T001
Blocks: T003, T004

Add tested forward migrations for M0 identity conversion and the normalized M1 schema, sqlc queries and repository ports. Prove empty and populated M0 upgrades, downgrade/upgrade, invariants, immutability and workspace isolation.

Design: `docs/specs/m1-semantic-registry/t002-migration-design.md`. Founder confirmed the design on 2026-09-02. Packet: `docs/specs/m1-semantic-registry/packets/T002.yaml`.

Execution: implementation `a6987af`; PostgreSQL 17/18 migration lifecycle, populated M0 rollback, typed repository, workspace isolation, immutability, relation policy and ontology publication tests pass. Evidence: `docs/evidence/M1-T002/summary.md`.

Acceptance: founder explicitly accepted T002 on 2026-09-02 and authorized T003 execution.

### T003 Source discovery adapters

Status: Needs_Review
Depends on: T002
Blocks: T004

Implement Catalog, PostgreSQL DDL/View SQL and dbt artifact adapters. Use maintained parsers/schema validation, immutable source revisions, incremental fingerprints and explicit unresolved findings.

Design: `docs/specs/m1-semantic-registry/t003-discovery-design.md`. Packet: `docs/specs/m1-semantic-registry/packets/T003.yaml`.

Execution: implementation `7ac5197`; adapter contract, race, real PostgreSQL incremental/rollback,
sqlc drift, parser benchmark and complete `make check-source` gates pass. Evidence:
`docs/evidence/M1-T003/summary.md`.

Acceptance: founder review is required before T004 execution.

### T004 Catalog and revision API

Status: Draft
Depends on: T002, T003
Blocks: T005, T006

Implement application services and generated OpenAPI handlers for discovery run state, catalog search, asset detail, revisions, evidence and bounded relations. Mutations commit domain rows, audit and outbox atomically.

### T005 Git content projection

Status: Draft
Depends on: T004
Blocks: T007

Implement the repository-neutral Git port and local Git adapter. Project deterministic canonical content from committed revisions, use optimistic base revisions and make outbox replay idempotent.

### T006 Audit, outbox and usage

Status: Draft
Depends on: T004
Blocks: T007

Complete M1 event envelopes, delivery handlers and the two privacy-bounded usage producers. Add retention and deletion behavior without raw search text or customer fact rows.

### T007 Production acceptance

Status: Draft
Depends on: T005, T006
Blocks: M1 milestone acceptance

Run the exact-ref source-to-detail journey, 10,000-table benchmark, migration/recovery tests, contract drift checks, security/release gates and independent spec, code-quality and QA reviews.

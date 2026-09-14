# ALPHA-T005 Test Results

Validation date: 2026-09-04

## Passed Gates

- `make check-source`: formatting, vet, workspace lint, TypeScript checks, Go/Web tests, OpenAPI
  drift, sqlc drift, embedded Web drift and release build passed.
- `make contracts-check`: generated Go and TypeScript OpenAPI artifacts match the canonical contract.
- `make db-generate-check`: generated PostgreSQL access code matches the canonical queries.
- `git diff --check`: passed.

## Domain And Application Evidence

- SemanticQuery validation covers invalid filter, time, ordering and limit semantics.
- Deterministic matching covers stable address and material ambiguity.
- Plan tests cover missing physical binding, missing join, many-to-many grain, EntityKey inclusion,
  complete time-range digesting and `not_configured` execution.
- Application tests cover no release, explicit cross-workspace release, inactive/expired/stale
  bindings, inactive/unauthorized consumers, authorization-safe ambiguity and exact-reference
  non-disclosure.
- HTTP tests cover authentication, capability denial, method contracts, dependency absence, unknown
  fields, arbitrary SQL rejection and a persisted refusal response.

## PostgreSQL Evidence

- Migration 15 passed empty and populated up/down/up lifecycle coverage on PostgreSQL 18 and the
  supported PostgreSQL 17 compatibility image.
- Real repository tests resolve current and pinned bindings across later publication, mutable asset
  pointer changes and governed-object version changes.
- A simulated rollback release becomes current while the pinned binding remains on its original
  release.
- Persisted plans retain the same digest on retrieval, and forced transaction failure leaves no
  partial SemanticQuery, plan, validation or event row.
- PhysicalBinding, ModelGrain and EntityKey snapshot versions are asserted from the selected release.

## Performance And Live Runtime

- 10,000 released assets, 25 samples on Darwin arm64: p50 `2.027083ms`, p95 `4.259458ms`, below the
  one-second Alpha budget.
- The rebuilt Docker stack migrated to schema `0.7.0`, reported ready at
  `http://127.0.0.1:18081/`, and resolved a real published metric through the REST endpoint.
- The live plan returned `intent=describe`, an immutable release ID, empty arrays rather than null
  for required collections, a canonical plan digest, passed validation and
  `executionStatus=not_configured`.

## Artifacts

- Full command logs are retained locally in `/tmp/semlia-t005-*.log` for this execution session.
- The live response is retained at `/tmp/semlia-t005-live-resolution.json`.

# FMB-T003 Test Results

Date: 2026-09-05  
Final packet: FMB-T003-A003

## TDD Record

RED tests exposed missing Operations contracts, static Web data, an always-empty audit cursor,
runtime replay version changes, absent export content, missing historical backfill, non-atomic
discovery completion, incomplete audit object targeting, weak cursor validation and stale E2E
assertions. Each was retained as a focused regression test before the matching implementation.

## Backend And Migration

Passed:

- Operations domain/application/HTTP focused tests
- Operations, worker, discovery and database integration suites
- empty migration 17 up/down/up
- populated migration 16 -> 17 -> 16 -> 17 with historical discovery, validation, Agent and
  semantic-resolution runs restored into Runtime
- PostgreSQL 17 and 18 integration coverage
- `make contracts-check`
- `make db-generate-check`
- `git diff --check`

Scenario coverage includes append-only/monotonic events, idempotent owning projection, lease
recovery/dead-letter, degraded discovery truth, cross-workspace composite constraints, audit filters
and targets, stable filter/workspace-bound cursors, malformed cursor 400 responses, export replay
fingerprint/creator/expiry, immutable snapshot download and settings version conflicts.

## Web And Desktop

Passed:

- focused Operations tests: 52 tests
- full Web suite: 15 files, 113 tests
- full `product-journey` desktop and compact-desktop matrix: 18/18
- targeted Audit/Runtime Playwright scenario: 2/2 at 1440x900 and 1024x768
- lint with zero errors and one existing `KnowledgeViews.tsx` Fast Refresh warning
- typecheck and production build
- real runtime state partition for audit-only, runtime-only and manage-only sessions
- export POST confirmation retained across list-refresh failure; content digest verified before download
- dialog focus, Escape restoration, reduced motion, minimum 10px text and no horizontal overflow

## Full Gates

`make check-source` passed after the final Web embed synchronization, including Go tests, Web tests,
contract drift, migration drift, Web embed drift and release build. The local log is
`/tmp/semlia-fmb-t003-final-check-source.log`.

The complete stack was rebuilt at migration 17 and exercised with `make smoke`; its result is in
`/tmp/semlia-fmb-t003-final-smoke.log`.


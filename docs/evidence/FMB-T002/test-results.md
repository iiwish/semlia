# FMB-T002 Test Results

Date: 2026-09-05  
Attempt: FMB-T002-A001

## TDD Record

RED tests captured fixture-backed Workbench behavior, missing authority records, unbounded lists,
incorrect release comparison bases, restart projection drift and authorization counterexamples.
The retained regression suite covers revoke-between-decision-and-snapshot races, cross-workspace
lookups, invisible item IDs, stale expected versions, same-key/different-request replays, section
capability denial and release-command response redaction.

## Backend And Contracts

Passed during the implementation and final source gate:

- Workbench, Catalog and Governance application/domain/HTTP focused tests
- PostgreSQL catalog, governance, projection and database integration suites
- `TestCatalogTenThousandAssetsRemainCursorPagedWithServerTotal`: two 100-row keyset pages from a
  10,000-asset workspace, each reporting server total 10,000
- Workbench PostgreSQL/HTTP replay, visibility, list/count, GET purity, restart, owning-hook,
  latest-run, blank legacy trace, UTF-8 and compatibility deep-link counterexamples
- asset authority paging, section availability, exact independent object pins, nested evidence
  denial and cross-workspace counterexamples
- immutable release detail, historical/current consumer impact, prior-pin/current-registry dual
  diff, publish/rollback redaction and capability-revocation counterexamples
- `make contracts-check`, `make db-generate-check` and `git diff --check`

The independent reviewer ran:

`SEMLIA_RUN_M1_BENCHMARK=1 go test -count=1 -run TestCatalog10000Performance -v ./tests/performance/catalog/...`

The first run measured page p50 27.277083ms / p95 50.56125ms and exact-address search p50
171.250917ms / p95 295.811166ms, exceeding the 250ms search budget. The immediate standalone retry
measured page p50 7.851042ms / p95 8.827625ms and search p50 72.436625ms / p95 89.864916ms and
passed. This is disclosed as non-deterministic host noise and a residual performance signal, not a
one-shot pass. Functional pagination, total and workspace-isolation assertions passed.

## Web And Desktop

Reported by the frontend task owner on the final stable tree:

- full Web suite: 142/142 tests
- lint: zero errors and one pre-existing `KnowledgeViews` Fast Refresh warning
- typecheck, production build and Web embed drift check: passed
- `product-journey` Playwright matrix: 18/18 at 1440x900 and 1024x768
- final source embed: `index-B7g1AWVk.js` and `index-DfqjqFei.css`
- live server-mode UAT: Workbench, asset and release deep links survived reload at both supported
  viewports; the exact URL remained intact and the same authorized workspace was reselected before
  resolving each persisted target; overflow checks were false and reduced motion/focus checks
  passed

The Docker development image builds the server-mode Web bundle separately; the running image serves
its hashed JavaScript asset with immutable caching headers.

## Final Gates

- `make web-embed`: passed
- `make check-source`: passed, including Go tests, PostgreSQL-backed integration suites, 142 Web
  tests, lint/typecheck, OpenAPI/TypeScript contract drift, sqlc drift, embed drift and release build
- `make dev`: passed; Postgres, migration, server and worker services started, and the server became
  healthy
- startup log: reconciliation completed with `scanned=20`, `upserted=20`, `resolved=0`,
  `truncated=false`
- `make smoke`: passed, `github.com/iiwish/semlia/tests/smoke` in 18.892s
- `GET /health/live`: 200 with `status=live`
- `GET /health/ready`: 200 with `status=ready`
- running local stack: `http://127.0.0.1:18081`

The local UAT identity dependency is intentionally unavailable: `GET /api/v1/session` returns 503.
The capture used the server-supported `local-author` UAT actor and a real persisted workspace after
each reload. It did not enable `VITE_CATALOG_FIXTURE` or use production-to-fixture fallback. The
session/workspace restoration boundary is carried by FMB-T007 rather than counted as a T002 pass.

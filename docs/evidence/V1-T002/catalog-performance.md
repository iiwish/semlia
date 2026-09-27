# Catalog Search Performance

## Status

T005's fifth complete gate attempt fails the unchanged enabled catalog search budget: exact-address search p95 is **340.854583 ms**, above **250 ms**. No RC.4 bundle is built by that attempt. A007's bounded isolated diagnosis identifies an index-incompatible, joined full-text filter; one narrowly scoped predicate alternative preserves the diagnostic result and uses the existing GIN index. Phase 2 implements only two SQL predicate replacements and SQLC generation. New semantic regressions pass on both the original and optimized SQL; relevant uncached packages and SQLC consistency pass. Independent review finds no blocking issue. The first unchanged focused performance run after the repair passes, with search p95 **12.627750 ms**. A complete subsequent candidate gate and RC.4 model acceptance remain pending.

No model, retained V1 runtime, existing application/source database, query golden or acceptance threshold changes are part of this work. The diagnostic comparison is not a passed release benchmark.

## Performance RED

The original [T005 gate evidence](../V1-T005/release-gates.md) and private `.semlia/v1-acceptance/current/final-gates-5-5TJIVi/` receipts retain the complete failed attempt. The enabled, uncached performance command exits 1 after 18.402 seconds, with 10,000 assets and 25 samples unchanged:

| Measurement | p50 | p95 | p95 Budget | Result |
| --- | --- | --- | --- | --- |
| Catalog first page | 12.549625 ms | 16.967083 ms | 150 ms | Passed |
| Catalog exact-address search | 118.770417 ms | 340.854583 ms | 250 ms | Failed |
| Resolver | 4.684792 ms | 6.010458 ms | 1 second | Passed |

The preceding complete gate's page p95 of 144.618209 ms remains a measured narrow pass, not a reason to retune or retry the current page test. This repair targets the actual search failure. The fifth gate's separate Docker observer failure also remains recorded, with cause unclassified; it is not silently reinterpreted as the catalog failure's cause.

## Bounded Diagnosis

A001's private diagnostic helper fails during second-page column access before collecting search plans, because it assumes a fixed result-column index. Its failed receipt and cleanup remain preserved. Root authorizes exactly one named repair using column `FieldDescriptions`; the fresh A002 fixture completes once. This is not a rerun of the failed release performance gate or a multi-query optimization search.

Private diagnostic artifacts reside under `.semlia/v1-acceptance/current/catalog-performance-diagnostic/`, with A002 in `attempt-2/`. `safe-plan-summary.json`, `diagnostic-result.json`, the twelve `*-plan.json` files, 53 `trace-*.json` files and `after.json` retain measurements and identity/preservation checks. Directories/files use `0700`/`0600`. The diagnosis uses a fresh owned PostgreSQL 18.6 fixture with the benchmark's 10,000-row data distribution, not an existing database.

The application executes List and Count serially (`internal/application/catalog/service.go:240-254`). Both original statements join assets to their current revisions and apply the address/full-text OR after the join. The full-text predicate uses `to_tsvector('simple', COALESCE(revision.content::text, ''))`, while the existing index uses `to_tsvector('simple', content::text)`. The measured plans, rather than this static mismatch alone, establish the expensive path:

- Both original plans hash-join 10,000 asset rows and 10,000 revision rows, remove 9,999 joined rows by the filter, hit 590 shared buffers, read zero blocks from disk and do not use the existing GIN.
- The only candidate predicate checks the current revision ID against a workspace-scoped full-text subquery using the existing index expression. Its address `ILIKE`, rank, cursor, filtering, sort and total semantics remain unchanged.
- The candidate uses `asset_revisions_content_search_idx` through a bitmap index scan. Count no longer needs the revision join. List still scans revisions for display fields and may hash-join them, but filters assets before that join and does not apply the expensive full-text expression to all joined candidates. It is not claimed to eliminate every full scan or reduce every buffer count: measured List/Count hits are 821/431.
- The exact-address diagnostic has one matching asset; its full-text branch returns no candidate. The complete List result digest and Count equal the originals. Separate mixed-match integration tests are necessary to prove full-text-only and overlapping-result semantics.

Recorded `EXPLAIN ANALYZE` execution times are:

| Plan Mode | Original List | Original Count | Candidate List | Candidate Count |
| --- | --- | --- | --- | --- |
| auto | 71.300 ms | 59.580 ms | 8.202 ms | 5.933 ms |
| force_custom_plan | 56.353 ms | 59.378 ms | 8.475 ms | 5.721 ms |
| force_generic_plan | 66.557 ms | 72.978 ms | 11.587 ms | 7.039 ms |

The default pgx statement-cache capacity is 512 with cached-statement execution. After the original auto sequence, List has 48 generic and five custom executions; Count has zero generic and 53 custom executions. PostgreSQL reports JIT available and enabled, but **none of the twelve plans contains a JIT section**. This evidence does not establish a JIT failure, and neither a cluster-wide JIT change nor a query-mode override is used as the repair.

All ten diagnostic preservation checks pass: Docker identity, existing containers/volumes/networks, protected files, earlier artifacts, retained runtime/native state, original security-image identity and source fingerprint. All four exact-fixture absence checks pass after cleanup. A001's failed helper is retained independently, not converted into a success.

## Semantic Baseline

The new `tests/integration/catalog/search_contract_test.go` reuses the package's normal Testcontainers PostgreSQL 18 setup and migrations. It does not change the benchmark. Its two top-level tests and eleven subtests cover:

- Exact, prefix, full-text-only and substring matches in one result set, with explicit rank tiers and complete rank/update-time/ID ordering.
- A result that matches both address and full text appears only once; an exact hit does not suppress other matches.
- Appending a revision can add a current full-text match or remove a historical-only match.
- Workspace isolation and type/lifecycle/combined filters, plus empty search, no result and trim/case behavior.
- An address match without any revision remains visible with its original title/summary fallback and zero rank.
- Equal-rank, equal-update-time results cross one-item page boundaries without duplication; all six pages retain the same total and expected order.
- Existing `%`, `_` and escaped-underscore address-pattern behavior remains unchanged.

Before the SQL owner edits either predicate, the test owner runs:

```bash
env GOENV=off GOWORK=off GOFLAGS= go test -count=1 \
  ./tests/integration/catalog -run '^TestCatalogSearchContract' -v
```

The first run passes in **8.818 seconds**, with all two top-level tests and eleven subtests successful. The fresh owned PostgreSQL container `20e7259fb0f2` is explicitly stopped and terminated by the existing test cleanup; the command ends with exit 0. The heavy-test window is then handed to the query owner. This is an intentional **baseline GREEN**, not a fabricated RED: the contract is unchanged and the real performance-gate failure above is the performance RED.

## Optimized Verification

The implementation changes only the two full-text matching predicates in `db/queries/catalog.sql` and the generated `internal/adapters/postgres/sqlc/catalog.sql.go`. It preserves current-revision selection, the outer rank's null fallback, address wildcard semantics, all filters/cursors and Count without a cursor. No schema/index, adapter behavior, API contract, empty-search optimization, caching layer or performance threshold changes.

The query owner runs:

```bash
make db-generate
make db-generate-check
go test -count=1 -v ./internal/application/catalog \
  ./internal/adapters/postgres ./tests/integration/catalog
```

SQLC generation and consistency exit 0 in 0.781 and 0.763 seconds. The uncached Go run exits 0 with **20 top-level tests / 31 including subtests passed**, no skip lines, and package durations **1.096 / 0.567 / 13.128 seconds**. This includes the unchanged new semantic tests against the optimized query and the package's existing catalog journeys. Private command logs and receipts are in `catalog-performance-diagnostic/phase-2/{generate,sqlc,focused}.{json,log}` beneath the protected acceptance root. Both product files are frozen for independent review.

Independent review finds no blocking issue and verifies the two-predicate scope, unchanged address patterns/rank/cursors/totals/current-revision filtering, and the new semantic cases. Its uncached run of the same three packages with `GOENV=off`, `GOWORK=off` and empty `GOFLAGS` passes in **0.869 / 1.085 / 13.877 seconds**. Direct parsing of the twelve saved EXPLAIN files independently confirms all six original plans omit GIN, all six candidate plans use the existing GIN and all twelve omit JIT. The reviewer does not run the performance benchmark.

## Focused Performance GREEN

After independent review, the query owner runs the unchanged enabled performance gate exactly once:

```bash
SEMLIA_RUN_M1_BENCHMARK=1 go test -count=1 -v \
  ./tests/performance/catalog ./tests/performance/distribution
```

The command exits **0**, no signal, in **12.832 seconds**. Both tests run uncached with the original 10,000 assets, 25 samples, warmup, queries and thresholds; neither is skipped. Package durations are 9.499 seconds for catalog and 0.758 seconds for resolver.

| Measurement | p50 | p95 | Unchanged p95 Budget | Result |
| --- | --- | --- | --- | --- |
| Catalog first page | 6.839709 ms | 9.047333 ms | 150 ms | Passed |
| Catalog exact-address search | 11.621375 ms | 12.627750 ms | 250 ms | Passed |
| Resolver | 2.346375 ms | 2.989750 ms | 1 second | Passed |

Private `phase-2/performance.json` binds the same source fingerprint before and after: `sha256:a413933ffe5c282a1ea845a31ff730070271b251baeeb1f245c9f01cef92ad86`. The focused result is a new authorized post-repair measurement, not a replacement or silent retry of the unchanged fifth gate's failure.

The first postflight observer exits 1 because one newly owned Testcontainers Ryuk reaper remains briefly present; all 18 original containers retain their states. Its `after.json` keeps `containers: false`, with the other eight preservation checks true. Root authorizes waiting for that owned reaper's natural exit, without manual deletion or another performance run. The separate `after-settled.json` then records all nine checks true: Docker identity, containers, volumes, networks, protected files, earlier artifacts, retained runtime, native state and original security-image identity. Both observer receipts remain preserved. The last diff check exits 0 and all sessions end.

A passing focused result does not replace the complete frozen-source T005 gates or the RC.4 one-shot real-model cohort. No new model call, retained-runtime restart or governance mutation occurs in this repair.

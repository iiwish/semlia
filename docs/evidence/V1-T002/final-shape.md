# Final-Candidate Result Shape

## Status

RC.2's fixed final cohort remains **3/4 positive cases and 2/2 negative cases**, with five provider attempts and four executions. Its original ledger and receipts are retained unchanged. T002-A004's generic prompt clarification, per-candidate cohort guard, local regression checks and independent reviews pass. A fresh candidate, complete T005 gates and explicit authorization are required before any further model acceptance.

No live V1 API, database, browser or model operation was performed during this repair. No compiler behavior, query schema, provider, published knowledge, golden SQL, expected values or result comparator changed.

## RC.2 Evidence

The tested candidate is `1.0.0-rc.20260927.2`, with frozen source `sha256:9d0a1252b1c6d2f44bec80592ad902875d21d26f2b078959b117f91e04af713a`, executable `sha256:8f2e66098008210a5bf08c1f35621528072574d548afc8e418be4de2959505b2` and manifest `sha256:841581071749e1cd7b67fd988f7d452168891264afeb2a3ebb6366e1279c2b67`. Independent read-only file hashing and the startup, runtime-version and before/after receipts match these identities. The direct embedded entry is `http://127.0.0.1:52688/`; candidate mode has no Vite server and the runtime version has no environment override.

The preceding embedded preflight, run `1790445281423`, passes all four cases in 5.2 seconds. Both desktop sizes complete normal-password author login, source inspection and natural editable revision entry, plus consumer keyboard/reduced-motion checks. All recorded errors and denied responses are zero. Run-specific before/child/after/head receipts match the launcher receipt; the child exits 0 with no signal or spawn error. Model/agent/execution totals remain 17/17/19 with no unfinished work.

The final cohort `8dd636a237b0bfd82f86cf1e` uses six unique new keys and one attempt per case. All six attempts finish; none is selected out or retried.

| Case | Ask | Execute | Model Attempts | Executions | Result |
| --- | --- | --- | --- | --- | --- |
| Total | 200 | 200 | 1 | 1 | Passed |
| Region | 200 | 200 | 1 | 1 | Passed |
| Monthly | 200 | 200 | 1 | 1 | Passed |
| Old-customer average | 200 | 200 | 1 | 1 | Failed strict scalar output shape |
| Denied identity | 403 | Not called | 0 | 0 | Passed; no model or execution |
| Natural ambiguity | 200 | Not called | 1 | 0 | Passed; clarification with no plan or execution |

The protected `final-candidate.json` ledger's before values match `candidate2-model-before.json`; its after values match `candidate2-model-after.json`: 22 model steps, 22 agent runs, 23 executions, zero unfinished work. Both ends retain the correct model revision `rev_01m3ev2rhdfw2vcm1atafxx7vd` at release `rls_01m3f2pbg0f1br2ba4hbtgjgrs` and the same running binary. Offline replay of the original assertion functions over saved receipts independently reproduces all five passes and the one failure without service requests.

## Failure Classification

The failed question asks for a single period's old-customer average. The typed interpretation selects the correct `demo_202609.average`, current-region filter `华东`, old-customer predicate and half-open August interval. Its extra `timeRange.granularity: month` requests an unasked-for time bucket.

The executed result contains two columns, `demo_202609.average` and `demo_202609.orders.paid_at.period`, with one row containing numeric **200** and `2026-08-01T00:00:00Z`. The unchanged independent SQL and golden contract expect one scalar column containing **200**. A pure offline call to the original `reconcileExecution` reproduces `ERR_ASSERTION`, actual column count 2 versus expected 1. No response projection or column deletion is used to mark this attempt passed.

The saved plan pins match the published average, amount, distinct-order count and old-customer term revisions. The persisted production payload defines average as sum of valid payment amount divided by distinct order count; the old-customer predicate uses first payment before `period_start`. The compiled SQL contains those aggregates and filters, the correct half-open bounds, and UTC month grouping requested by the interpretation. With this single whole-month window, its month-derived period start equals the independent SQL's August start. The observed numeric value is correct; the unwanted extra grouping changes the output shape. The compiler's explicit-granularity behavior is not a defect.

The acceptance driver correctly rejects extra columns. Its generic failure label lacks this diagnostic detail, but that is not a reason to loosen its contract. The final driver executed independent SQL during the original run but did not persist its returned rows separately; this read-only diagnosis does not claim to have rerun that source query. It uses the original typed receipts, saved production content, unchanged SQL/golden contract and later identity/counter proof.

## Prompt Repair

The only behavior change is one generic paragraph in `internal/application/governance/ask.go`:

- `from/to` restrict the interval; `granularity` adds a calendar grouping and output dimension.
- A time bucket requires an explicitly requested time series, periodic breakdown or comparison across time periods.
- Fixed-window aggregates of any duration omit granularity and time-based ordering.
- Non-time breakdowns and comparisons retain only the requested dimensions, without an added time bucket or time-based order.
- Calendar words or the word comparison alone do not imply temporal grouping.

The prompt contains no acceptance-specific assets, dates, questions or answer values. Existing half-open bounds, privacy and injection instructions remain intact. The schema, gate, query domain and compiler behavior are unchanged; correct ungrouped responses are not rewritten, and existing explicit monthly queries retain their two-column behavior.

### RED

```bash
go test -count=1 ./internal/application/governance \
  -run '^TestAskPromptSeparatesTimeFiltersFromRequestedGrouping$'
```

Exit 1 in 0.741 seconds. All six generic grouping-contract fragments are missing from the original prompt. The regression was added and run before the prompt change.

### GREEN

```bash
go test -count=1 ./internal/application/governance ./internal/domain/execution \
  -run 'TestAskPrompt|TestAskGatePreservesUngroupedTimeWindows|TestAskTimeBucketInterpretation|TestFixedTimeWindowGroupingIsExplicit|TestCalendarGrouping|TestCompareAndBreakdown|TestCalendarOrdering'
go test -count=1 ./internal/application/governance \
  ./internal/domain/execution ./internal/domain/distribution
git diff --check -- internal/application/governance/ask.go \
  internal/application/governance/ask_idempotency_test.go \
  internal/domain/execution/compiler_test.go
```

All exit 0. The focused packages take 0.721 and 1.110 seconds; the full uncached three-package run takes 0.846, 1.231 and 0.412 seconds respectively.

The independent reviewer finds no blocking issue and repeats all three packages uncached with `GOENV=off`, `GOWORK=off` and empty `GOFLAGS`: all pass in 1.491, 1.079 and 0.686 seconds respectively.

The gate regressions preserve an ungrouped full-year interval, its exact bounds and non-time filters for aggregate, non-time breakdown and non-time comparison. They assert no introduced granularity or ordering. Compiler regressions compare scalar, category breakdown, category comparison and explicit monthly comparison over the same interval: exact output columns, preserved typed parameters, no implicit time grouping/order, and retained explicit month grouping. Existing calendar-ordering and privacy tests remain in the full package run. These checks prove the local contracts, not future model adherence.

## Candidate Cohort Guard

The separately owned implementation is limited to `scripts/acceptance/v1/final-candidate.mjs`, `v1-final-candidate.test.mjs` and `v1-model.mjs`. Its author reports the following real-filesystem regressions; independent review finds no blocking issue.

A cohort requires an authenticated ready candidate runtime, matching private launch/owner records, manifest and executable digests, schema, direct embedded address and the actual `server` file hash. Source mode and missing or inconsistent candidate metadata are rejected before a model call. The run reserves a new private `final-candidates/{identity-digest}.json` ledger with exclusive creation; the digest combines verified manifest and binary identity. A repeat of either identity is rejected, with no resume path. An existing legacy `final-candidate.json` with the same binary also rejects the run, and no path deletes or overwrites that legacy evidence.

Any unfinished prior ledger, unknown entry, malformed identity, symlink, foreign owner or untrusted permission state fails closed. The guard checks directory inventory and file inode/hash identity while preserving prior bytes. It records version, source/archive/manifest/binary digests, published head and model pin, the API's `generationConfigRevision` for the exact configured provider/model, the fixture fingerprint and a digest of all six unchanged questions/expected-data cases. Runtime identity is checked before each attempt and again after the cohort; configuration and head are checked at both ends. Postflight identity drift makes the whole cohort fail even if individual cases passed.

`maxAttemptsPerCase` remains one; four positive cases, denied access and natural ambiguity remain fixed. An unknown HTTP outcome stops the run while retaining pending entries and the original denominators. The command lock is released only when its inode is still owned. This adds traceable per-candidate evidence, not an automatic retry mechanism or permission for another provider call.

### Guard RED/GREEN

The initial filesystem tests expose two failures: source identity is not enforced, and the old global ledger blocks a genuinely distinct candidate. Further negative tests expose two inventory/false-completion failures and one missing model-generation-configuration identity check before their fixes. Each corresponding focused run becomes GREEN. Fixtures use real private temporary directories, candidate files, ledger files and preserved-byte checks; request callbacks remain synthetic, with no network, model or database calls.

```bash
node --test scripts/acceptance/v1-final-candidate.test.mjs
node --test scripts/acceptance/v1*.test.mjs
node --check scripts/acceptance/v1/final-candidate.mjs
node --check scripts/acceptance/v1-model.mjs
git diff --check
```

The author's final focused run passes 18/18 tests, zero skips, in 0.254 seconds. The full acceptance-script unit set passes 55/55 tests, zero skips, in 3.222 seconds. Both implementation syntax checks and the diff check exit 0. The existing RC.2 ledger and all its five provider attempts remain failed history; a new candidate's actual model adherence is still unverified.

The independent reviewer repeats the complete acceptance-script unit set: 55/55 passed, zero skips, in 3.234 seconds, with a clean diff check. Review covers the API configuration revision, exact-candidate refusal, partial/foreign/symlink/inventory protections, preserved legacy bytes, unknown-outcome denominators and final identity drift. All local validation sessions ended; no provider or retained-runtime operation occurred.

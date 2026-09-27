# V1-T002 A001 Backend Evidence

Date: 2026-09-26. Scope: durable Ask ownership, replay, cancellation, model-output gating, and time-bucket query validation. Real-provider acceptance is recorded separately in `model-acceptance.md`; the tests below use synthetic providers and disposable PostgreSQL databases.

## Contract

- Migration 33 adds `ask_requests`, keyed by workspace, canonical requesting principal, and request key. Its data consists of IDs, digests, state, a fencing token, deadline, terminal error code, and timestamps. It has no question, prompt, raw model response, or free-text result column.
- Claim creation and agent/runtime attribution commit in one transaction before provider invocation. Resolver keys use the owned agent-run ID, so distinct principals sharing a caller key cannot share a query accidentally.
- A completed request reuses the same run/query/plan. Changed question/context yields `ASK_KEY_CONFLICT`. Running requests return retryable `ASK_IN_PROGRESS`; terminal failures do not call the provider again. Cancelled or uncertain work requires an explicit new request key to try again.
- Replay authorizes the current principal and assets and compares the current authorized knowledge/release digest. It never silently switches an existing interpretation to a different release. Current, explicit-release, and pinned-binding paths are covered. Disabling model configuration does not invalidate a completed result.
- Cancellation uses a bounded detached context only for fenced database finalization. Provider calls remain bounded by the caller context, the 60-second provider timeout, and the persisted claim deadline. Startup/periodic reconciliation closes abandoned requests as `ASK_OUTCOME_UNKNOWN`, without reclaiming their model work.
- Model output is capped at `min(configured TokenLimit, 16384)`. Truncation and non-normal finish reasons are rejected before semantic resolution. Accepted finish reasons are `stop` and `end_turn`. Safe diagnostics include static schema paths/error kinds, domain validation codes/messages, and response metadata scalars, never model values or property names.
- Compare and breakdown accept validated nonempty time granularity as a grouping/comparison selector. Missing granularity is not a substitute for a selector. Invalid time ranges, unsupported granularity, and missing measures remain invalid.
- Calendar ordering by the same time selector uses the selected period expression. An unrelated unselected time field remains invalid. Time ranges are half-open `[from, to)`; the Ask prompt and OpenAPI description state inclusive/exclusive semantics and the next-month boundary for a full month. Existing stored parameters are never normalized or rewritten.

## TDD Evidence

Initial RED command:

```sh
go test ./internal/application/governance -run 'TestAsk(ReplaysClarification|RejectsMalformedContext)' -count=1
```

Observed repeated clarification called the provider twice and created two runs; malformed explicit context caused a nil-pointer panic.

Time-bucket RED command:

```sh
go test ./internal/domain/distribution ./internal/domain/execution -run 'Test(TimeBuckets|CompareAndBreakdown)' -count=1
```

Both compare/month and breakdown/month were rejected by intent validation before the existing calendar SQL compiler could run.

An additional PostgreSQL RED assertion showed that a late owner attempting to finish an already expired record returned `ASK_OUTCOME_ALREADY_RECORDED`, while replay returned `ASK_OUTCOME_UNKNOWN`. The repository returns the stable unknown code for this case; the focused PostgreSQL Ask suite passed after that fix.

Calendar-ordering RED command:

```sh
go test -count=1 ./internal/domain/execution ./internal/application/governance -run 'TestCalendarOrderingUsesOnlySelectedPeriod|TestAskPromptStatesHalfOpenCalendarBounds'
```

The compiler built a valid month-grouping expression but rejected ordering by that same time selector as `EXECUTION_PLAN_UNSUPPORTED`. The prompt also lacked the half-open interval description. The same command passed after the limited mapping/prompt fix, including ASC/DESC, unrelated-field rejection, and ungrouped-field rejection.

GREEN commands:

```sh
go test -race -count=1 ./internal/application/governance ./internal/application/distribution ./internal/domain/distribution ./internal/domain/execution ./internal/platform/http
go test -count=1 ./internal/adapters/postgres ./internal/platform/schema ./scripts/release
go test -count=1 ./internal/domain/execution ./internal/application/governance ./cmd/semlia
go test -count=1 -p=1 -timeout=5m ./tests/integration/db ./tests/integration/governance -run '^TestAsk'
go test -count=1 -p=1 -timeout=20m ./tests/integration/db ./tests/integration/governance
make contracts-check
make db-generate-check
git diff --check
```

These passed, including the real PostgreSQL claim/replay tests, published-plan replay, HTTP terminal code equality, configuration revision digest changes, asset-grant revocation, expiry-read races, atomic rollback on runtime-attribution failure, forged-owner rejection, cancellation, and terminal failures. Calendar compile tests assert month truncation, UTC grouping, two output columns, and parameterized time bounds for both compare and breakdown.

The output-budget test covers configured limits 1024, 12000, and 32768, verifying that the request preserves lower configured limits and caps the last case at 16384. It passed under the race detector.

The final full integration command completed with exit 0: database package 51.608 seconds, governance package 73.247 seconds. Earlier full runs were not GREEN: one caught the new HTTP fixture defect, and another compiled before the expiry-code fix and failed its new regression assertion. An intermediate full run was explicitly terminated when the calendar-ordering defect required another production change; it is not counted as a pass. All A001 command sessions are complete.

The migration test applies 32 then 33 with legacy attribution present, verifies preservation, permits empty-history down/up, and refuses down migration when Ask history exists. It also verifies the new table has no raw/payload/JSON/binary columns. Application queries use database time for claim and completion.

During iteration, disposable migration setup exposed a PL/pgSQL CASE-parenthesis error, the new concurrency fixture omitted an authorization trace ID, the new HTTP fixture omitted its distribution dependency, and a new gate fixture omitted required empty arrays. The command-readiness test also retained a schema-32 expectation and was updated to 33. These fixture/migration defects were corrected before their respective GREEN runs. Contract generation initially detected the expected operation-description change; regeneration and the check then passed. No failed real-provider attempt is replaced by these synthetic results.

## Changed Surfaces

- Ask domain/application/repository modules and their focused tests; migration 33, SQLC input/generated output, schema-version assertions.
- Distribution context validation, authorized-knowledge digest, expected-release check, time-bucket validation, and execution calendar ordering/tests.
- HTTP Ask terminal mapping, startup expiry reconciliation, readiness-version test, and API operation/time-range documentation with generated Go/TypeScript comments. Response shapes remain unchanged.
- Published-plan and migration integration coverage. No existing test fixture data or shared runtime was modified by A001.

## Boundaries

The orchestrator/A002 owns V1 runtime migration/restarts and actual model acceptance. A001 did not migrate or restart the original Semlia database. Migration 33 DDL is frozen after integration; existing Ask/model evidence must be preserved.

A001 independently reviewed A003's atomic spec-reference preservation change and ran its focused production-baseline tests successfully. The complete T002 outcome still depends on the orchestrator's independent review and normal-endpoint real-provider acceptance, not this backend evidence alone.

Unknown outcomes cannot be safely resumed or automatically retried. A process failure after semantic-query persistence but before terminal Ask commit can leave an unreferenced query, with the Ask claim conservatively marked unknown. Safe first-response validation details are not persisted, so replay preserves status/code but uses a generic diagnostic. Current authorization/context changes can intentionally prevent replay even when the original interpretation was valid.

An earlier read-only release-audit tool output exposed credential values from a private local compose configuration. The orchestrator was notified; no values are included here, no credential rotation was performed, and this work does not claim that the overall session had no disclosure.

# V1-T002 Real-Model Acceptance

Status: Passed, with all iterative failures retained. Date: 2026-09-26. Attempt: A002.

## Environment and Boundaries

The normal V1 server/worker environment uses the isolated T001 application/source databases, normal password sessions and the copied `semantic-core` default `deepseek-flash` model. No model stub, golden-answer prompt, direct governance-state insertion or existing-environment mutation is used. Application and provider errors are exposed only as stable status/code metadata. Private API receipts and attempt ledgers stay under `.semlia/v1-acceptance/current/` with protected permissions.

Each explicitly named validation round allows at most two attempts per question. Another round requires an orchestrator-approved diagnosis and named code/configuration correction. All prior attempts remain in the denominator. A started or unknown-outcome request is not automatically retried. Main questions explicitly select the published `demo_202609.model`; the verifier compares successful normal Ask/Execute results against independently executed PostgreSQL SQL and fixed expected results. The optional old-customer ratio question supplies the canonical real-model query for subsequent deterministic governance replay.

## Schema 32 Baseline

Command: `node scripts/acceptance/v1-model.mjs baseline`.

| Case | Attempt | Ask Result | Attributed Model Calls | Executions | Result |
| --- | --- | --- | --- | --- | --- |
| August total | 1 | 422 `AI_OUTPUT_INVALID` | 1 | 0 | Failed |
| August region breakdown | 1 | 422 `AI_OUTPUT_INVALID` | 1 | 0 | Failed |
| July/August comparison | 1 | 422 `AI_OUTPUT_INVALID` | 1 | 0 | Failed |
| Unauthorized actor | 1 | 403 `NO_MATCHING_GRANT` | 0 | 0 | Passed |
| Ambiguous question | 1 | 200 clarification | 1 | 0 | Passed |

The first three failures expose the stable gate violation `payload is not a JSON object`. They are not proven truncation failures because this baseline did not record a provider finish reason. Provider authentication and clarification work. The three failed agent runs take approximately 10.4-11.0 seconds and terminate as failed; clarification terminates as succeeded. No query execution is created. Positive baseline: 0/3 attempts; negatives: 2/2; attributed provider attempts: 4. The optional ratio case is not yet attempted. No diagnostic provider calls are added.

The initial baseline driver records failed cases and returned process success; the acceptance CLI gates subsequent invocations with a nonzero exit when required cases remain unpassed. The ledger, not the initial process exit, is the baseline outcome evidence.

## Schema 33 and Reader Configuration

Commands: `node scripts/acceptance/v1.mjs down`, `node scripts/acceptance/v1.mjs up`, then `node scripts/acceptance/v1-model.mjs retry`. The owned application database is schema 33. The original `semlia` database remains schema 32; original configuration and database preservation checks pass.

| Case | Global Attempt | Ask Result | Execute Result |
| --- | --- | --- | --- |
| August total | 2 | 200, valid published-model plan | Failed `EXECUTION_ROLE_NOT_READ_ONLY` |
| August region breakdown | 2 | 200, valid published-model plan | Failed `EXECUTION_ROLE_NOT_READ_ONLY` |
| July/August comparison | 2 | 422 `AI_OUTPUT_INVALID` | Not called |
| Old-customer ratio | 1 | 422 `AI_OUTPUT_TRUNCATED` | Not called |
| Unauthorized actor | 2 | 403 `NO_MATCHING_GRANT` | Not called; zero new agent runs, model steps and executions |
| Ambiguous question | 2 | 200 clarification | Not called; zero executions |

The reader role passes discovery permissions but initially lacks the execution adapter's required `log_min_error_statement=panic` setting. After all calls finish, `node scripts/acceptance/v1.mjs harden-reader` verifies the owned role/database markers and applies only that role-scoped setting. It does not alter global PostgreSQL configuration or weaken the adapter guard. Provisioning and runtime recovery now require that setting; RED/GREEN tests cover missing and incorrect values.

`node scripts/acceptance/v1-model.mjs execute-replay` executes the two existing real-model plans with explicit new execution keys and no new Ask/provider calls. Both executions succeed: total 1500; regions East 1000 and South 500, matching independent SQL. The regional verifier first assumed dimension-first column order while the real compiler emits measure-first columns. A failing regression proves that defect; semantic-column-name alignment fixes it. `node scripts/acceptance/v1-model.mjs reverify-receipts` revalidates the saved successful result without another execution or provider call. The original failed executions and initial comparison failure remain in the ledger, with a separate comparison-review record.

The named `ask-budget-16384-safe-diagnostics` round applies only to the monthly and ratio cases. It retains the same configured model, 60-second provider timeout and strict output/finish gates. Monthly global attempt 3 returns 422 `AI_OUTPUT_INVALID` with `/query: semantic query is invalid`; its safe metadata is `finishReason=stop`, prompt tokens 3479, completion tokens 5049 and content bytes 683. Ratio global attempt 2 succeeds through Ask and Execute and reconciles to 200; its metadata is `stop`, prompt tokens 3477, completion tokens 6318 and content bytes 873.

Monthly global attempt 4, the second attempt in this named round, follows the bounded time-granularity compare-validation correction. Ask succeeds with a published-model monthly comparison plan. Execute returns 422 `EXECUTION_PLAN_UNSUPPORTED` before creating an execution. The natural model output orders by the time attribute, but the compiler's selected-expression guard does not map that attribute to its selected monthly `date_trunc` expression. The plan and original output are retained; no order is removed and no query is rewritten to hide the defect. The model's upper bound is August 31 at 23:59:59 UTC while execution uses an exclusive bound; this last-second semantic gap is recorded separately from the synthetic rows' numeric comparison.

After the latest restart, `node scripts/acceptance/v1-model.mjs replay` replays the four successful Ask plans, denied request and clarification with their original keys. All six responses preserve their terminal identity/status; agent-run, model-step and execution counters do not increase. No extra provider call is made. All monthly attempts remain recorded.

## Strict Window and Compiler Verification

`node scripts/acceptance/v1-model.mjs audit-windows` independently checks saved canonical query bounds, not just sample values. Total attempt 2, regional attempt 2 and ratio attempt 2 use exactly `[2026-08-01,2026-09-01)`. Monthly attempt 4 uses `[2026-07-01,2026-08-31T23:59:59Z)` and fails the temporal contract. A RED/GREEN regression rejects this missing final second even when the sample rows would match.

After the time-bucket ordering compiler correction and explicit half-open-window prompt are integrated, `node scripts/acceptance/v1-model.mjs execute-compiler-replay` executes the original, unchanged monthly attempt 4 plan under an explicit new execution key. Its July 600/August 1500 results match independent SQL with zero model calls. This proves the compiler correction only: its semantic-window outcome remains failed.

The separately authorized `ask-half-open-time-window` round permits only monthly, at most two explicit attempts, with unchanged question, provider/model, 60-second timeout and 16384 output cap. `node scripts/acceptance/v1-model.mjs round-half-open` passes on its first attempt (global monthly attempt 5): normal Ask 200, Execute 200, exact `[2026-07-01,2026-09-01)` month window and independent SQL 600/1500. Safe model metadata is `finishReason=stop`, prompt tokens 3549, completion tokens 3996, content bytes 496. No second attempt is used.

All four positive cases have strict semantic-window and independent-data evidence. This is iterative repair evidence, not a claim that all cases passed their first call on one frozen binary.

## Driver and Tests

- `scripts/acceptance/v1-model.mjs`: bounded explicit baseline/retry commands and deterministic governance rehearsal command, sharing the protected operation lock.
- `scripts/acceptance/v1/model.mjs`: normal session HTTP requests, protected attempt receipts, explicit model/release/plan pin checks, independent SQL reconciliation and negative zero-execution assertions.
- `scripts/acceptance/v1/correction.mjs`: a single-model structured binding fault/correction/rollback sequence using normal production operations and explicit-release Resolve/Execute replay. It requires a successful real-model ratio query and does not invoke the provider per stage.
- `scripts/acceptance/v1-model.test.mjs`: RED was observed before each new helper existed. GREEN covers exact result values/row completeness, readonly successful execution, refusal versus arbitrary error, zero execution growth, model/release pins and structured binding restoration.

Test command: `node --test scripts/acceptance/v1*.test.mjs`. All 18 tests pass, including the prepared final-candidate cohort tests. Tests do not call any provider. Additional real RED cases reject CSRF 403 as authorization proof, detect denied-request model activity, detect measure/dimension column-order assumptions, reject a missing final second and refuse unapproved model rounds. The half-open repair round is explicitly restricted to monthly with a two-attempt ceiling.

## Governance Rehearsal Contract

The active rehearsal maps semantic payment amount to the synthetic `is_valid` numeric field while preserving the order ID's unique-key binding. Its definition, author confirmation and independent review explicitly label it synthetic fault injection, never a correct business definition. Correct content maps payment amount to the physical amount field. Only isolated model content changes; source data and fixture goldens are untouched.

The completed replay sequence is correct baseline 200, injected fault 1, independently reviewed correction 200, rollback restoring the historical fault 1, and final normally governed restoration 200. Every replay pins the new release and its exact model revision. All stage release/revision/plan/execution IDs and both failed and successful replay evidence are retained privately. This is deterministic governance regression, separate from the real-model main-flow acceptance denominator.

An earlier order-ID fault scenario produced one successful explicit-release baseline replay and a 400 `INVALID_ARGUMENT` at creation, with no operation/release created. Its target and failed receipt remain in `correction-acceptance.json` and `correction-fault-create.json`. The defect is the production change replay's local-reference resolver treating atomic `spec` value roots as production references instead of retaining typed knowledge references. This is a code defect, not evidence of a deliberate wrong-mapping guard. The alternative amount-mapping scenario has independent `correction-amount-acceptance.json` receipts and `v1-amount-rehearsal-*` idempotency keys; it never overwrites the earlier scenario.

Command: `node scripts/acceptance/v1-model.mjs governance`. All five stages pass with zero model calls. The baseline revision is `rev_01m3ev2rhdfw2vcm1atafxx7vd`; the injected-fault revision is `rev_01m3eyp47pej9s0wrbdc1z495c`.

| Stage | Release | Value | Model Revision |
| --- | --- | --- | --- |
| Baseline | `rls_01m3ev2rhdfw29emz0hz8j0bzm` | 200 | Baseline |
| Fault | `rls_01m3eyp47kej9s4bwb4b7rkbzd` | 1 | Fault |
| Correction | `rls_01m3eyp8y4ejfvs1vra0twyhn7` | 200 | Baseline |
| Rollback | `rls_01m3eypc98ejjvct15fvcyzq1f` | 1 | Fault |
| Restored | `rls_01m3eypfcbejqrva87ty99k2n0` | 200 | Baseline |

The final normal-API release/revision lookup and deep content comparison prove that the current head contains the correct original model content. No injected fault remains. A governance command also reports this head proof on intermediate failure; an unverified or fault-bearing head cannot be called restored.

Lifecycle verification retains the original 13 operation/release identities and their immutable baseline manifest while reporting actual current totals. It preserves `baseline-report.json` as historical evidence and writes current counts to `runtime-verification.json`; it does not report zero current model calls after T002. Reinitialization does not replace an existing correction ledger or move the production release head.

## Final T002 Accounting

`node scripts/acceptance/v1.mjs verify` and a repeat `node scripts/acceptance/v1.mjs init` both pass: 17 releases, 16 operations, 10 five-type assets, 2 source datasets/8 fields, independent-review refusal verified, 6 normal password sessions. The restored release remains the current head. The repeated initialization creates no additional model runs, executions or governance records.

The real application database contains 13 agent runs and 13 model steps: 11 positive-question provider attempts (total 2, region 2, monthly 5, ratio 2) plus 2 clarification attempts. Seven agent runs succeed and six fail. Two unauthorized requests invoke no model. The full Ask ledger contains 15 attempts; none are removed. There are 13 persisted executions, 11 succeeded and 2 failed, including explicit repair and governance replays. Rejected pre-execution plans create no execution row and remain visible in their request receipts.

After restoration, the six original-key replay probes pass again: the five model-bearing requests return 409 `ASK_CONTEXT_CHANGED`, and the denied actor remains 403 `NO_MATCHING_GRANT`; all agent-run/model-step/execution counters remain unchanged. This verifies release-aware durable replay rather than returning a stale current-context plan.

Original-environment preservation checks pass: the original `semlia` database stays schema 32, the isolated V1 database stays schema 33, original workspace/account/source/release counts and provider/model fingerprints match, and existing environment/compose file hashes are unchanged. Private accounting receipts are `t002-preservation.json`, `model-acceptance.json`, `correction-amount-acceptance.json`, `runtime-verification.json` and `t002-safe-model-metadata.json` under `.semlia/v1-acceptance/current/`. Full private receipts, secrets and provider text are not published.

## Prepared Final Candidate Cohort

`node scripts/acceptance/v1-model.mjs final-candidate` is an explicit, not-yet-invoked entry for the final frozen candidate. The orchestrator must authorize its execution separately. It does not use prior per-case successes to select or skip cases: it attempts all four positives once each, one denied actor and the natural ambiguous question `最近那个指标怎么样？`. Each case has a new cohort-specific key; a protected existing `final-candidate.json` refuses a second run rather than resuming or retrying. Unknown HTTP outcomes stop the cohort; failures and unattempted cases remain in the fixed 4-positive/2-negative first-pass denominator.

The entry confirms correct model content through the normal API, fixes the current restored release and model revision for all requests, records the owned runtime binary and full fixture fingerprints, checks exact date windows and independent SQL, and reports provider-count deltas. It never rewrites the T002 repair history or fixture goldens. Final first-pass quality is reported separately from T002 iterative repair results. The preparation tests establish the fixed allowlist, natural ambiguity and nonshrinking denominator; no final-candidate provider call has occurred.

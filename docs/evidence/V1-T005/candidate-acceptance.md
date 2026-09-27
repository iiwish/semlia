# Candidate Acceptance

## Status

`1.0.0-rc.20260927.6` passes exact-binary startup, runtime-version readback, embedded browser checks, delegated CUA acceptance and its fixed real-model cohort: **4/4 positives and 2/2 negatives**. The user explicitly delegates browser acceptance and authorizes commit, push and PR merge after passing validation; PR CI is the merge gate. This is not personal user testing or a formal signed 1.0 release. Historical candidate results below retain their original outcomes.

## RC.6 Identity And Acceptance

| Field | Value |
| --- | --- |
| Version | `1.0.0-rc.20260927.6` |
| Source | `sha256:8bdaf67c031c83f8adf9fa6ce3a4e1c0f6d9b491315f3c41dab62b9b4a72e263` |
| Binary | `sha256:a513a5d379042877e0e2e53bf60a5bb3e74bd36bb565018695c22ea768703b6e` |
| Manifest | `sha256:3bcf4fd1bf26f0beb39c9b6e5075a6031d759c00ff2b6e90ad8e8e93a4833fda` |
| Archive | `sha256:ea6ee90f553611ea5182acce1b6bf4bbbea355bcade0ddf6d5bda276b1a87945` |
| Published head | `rls_01m3fm52csedprcf4dptew0kzz` |
| Model revision | `rev_01m3fm52cvedpsrdm7bjxzy5bk` |
| Configuration | `sha256:f53085427f9ea4cf20000bca5621c0fcf7630265809dfb7ccc5ff8331259279b` |

Root's actual candidate-check, owned RC.5 down and same-byte up-candidate all exit 0. Runtime `buildVersion` exactly matches RC.6, without version override. The candidate-preflight CLI exits 0; run `1790517174201` passes all four embedded tests without new models or executions. [Round 8](release-gates.md) distinguishes seven fresh checks from unchanged-input Go/Node/performance evidence inherited from Round 7; these are not represented as a full RC.6 local source rerun. Manifest remains `sourceDirty: true`, `local_candidate`, `unreviewed`, migration 33.

Cohort `be17fc8c25fe4a8fcc0aac96` runs once during `2026-09-27T13:54:18.654Z` to `13:55:29.694Z`, actual CLI exit 0. Its immutable ledger is `.semlia/v1-acceptance/current/final-candidates/36a07c341157860b505a2e136a5b2dc2a5081c1d100bb6f8b8e24dfa3d31391c.json`.

| Case | Ask | Execute | Models / Executions | Result |
| --- | --- | --- | --- | --- |
| Total | 200 | 200 | 1 / 1 | 1500 |
| Region | 200 | 200 | 1 / 1 | East 1000, South 500 |
| Monthly | 200 | 200 | 1 / 1 | July 600, August 1500 |
| Old-customer average | 200 | 200 | 1 / 1 | Scalar 200 |
| Denied | 403 | Not called | 0 / 0 | NO_MATCHING_GRANT |
| Ambiguous | 200 | Not called | 1 / 0 | Clarification |

Independent read-only review checks saved responses, exact columns and fixed goldens, plan IDs/digests/pins, model configuration, approved knowledge basis and actual installed binary digest. Average uses `region eq 华东`, `old eq true`, `[2026-08-01, 2026-09-01)` UTC, no dimensions, ordering or time granularity. SQL contains canonical category equality, not LIKE/ILIKE or GROUP BY; one column and one row contain 200. Review performs no new API, SQL or model call; independent source SQL comparison was performed by the actual cohort driver, not repeated during saved-evidence review.

Cohort counters move from **34/34/32 to 39/39/36** (model steps/agent runs/executions), zero unfinished. Candidate, head, configuration and knowledge basis remain stable; thirteen watched historical files and the separate RC.4 ledger remain unchanged. The failed glossary-preparation receipt remains failed and is linked to its independent approved reconciliation, not rewritten.

[Actual CUA acceptance](browser-merge.md) additionally confirms cold state, full-address search before/after detail, known version filters, source contents, revision cancellation and one consumer question with explicit read-only execution. That separate UI attempt returns scalar 200 with exact category and dates, adding **one model and one execution**, for cumulative **40/40/37**, zero unfinished. Eight saved screenshots at both supported sizes pass independent visual review. A CUA wait-for-button probe times out while the model is still running; the original request subsequently completes without resubmission or additional model call.

RC.4 CUA catalog-status and RC.5 CUA search failures are preserved with their fixes. RC.5 does not run a model cohort. Earlier cohorts are not retried or replaced by the RC.6 result.

## RC.4 Identity

| Field | Value |
| --- | --- |
| Version | `1.0.0-rc.20260927.4` |
| Frozen source | `sha256:a413933ffe5c282a1ea845a31ff730070271b251baeeb1f245c9f01cef92ad86` |
| Binary | `sha256:7142017faffe5032511cf73e5ca140ae54d0c30e17ea3092b28bd26988476b48` |
| Manifest | `sha256:8279a8c1aa2652b0554dcb33c3c847cc93748ec9061573050178e38c93e272db` |
| Archive | `sha256:63c7086c5c4d283ca4b228ff04acc44d89e2c495913799b1a0f29c0e1d4e9681` |
| Migration | 33 |
| Published head | `rls_01m3fm52csedprcf4dptew0kzz` |
| Model revision | `rev_01m3fm52cvedpsrdm7bjxzy5bk` |
| Provider configuration revision | `sha256:f53085427f9ea4cf20000bca5621c0fcf7630265809dfb7ccc5ff8331259279b` |

Root runs the actual `candidate-check`, owned `down` and `up-candidate` commands; all exit 0. The verified release executable is installed without recompilation. `/api/v1/system/info` returns HTTP 200 and the exact RC.4 version with no `SEMLIA_BUILD_VERSION` override. The retained entry is `http://127.0.0.1:52688/`, candidate mode, no Vite. Independent read-only review verifies the artifact bytes, lifecycle sequence, stored runtime identity and version receipt against the [Round 6 artifacts](release-gates.md#rc4-identity).

## RC.4 Embedded Browser

`node scripts/acceptance/v1-browser.mjs candidate-preflight` exits 0. Run `1790457937505` reports **4/4 passed in 8.8 seconds**, child exit 0 and stable candidate identity. Normal-password author login, source inspection and natural revision entry work at 1440x900 and 1024x768; consumer keyboard/focus and reduced-motion preference checks pass at both sizes. Root visually inspects both natural-revision screenshots; independent review opens all six source/revision/keyboard screenshots and finds no blocker. Compact desktop uses normal vertical scrolling without incoherent overlap. Preflight enters revision and cancels; it does not submit a change or exhaustively inspect every animation.

Before/after counters remain **27 model steps, 27 agent runs, 27 executions, zero unfinished**. The approved supplemented head remains unchanged. These four read-only candidate checks do not replace the 12 source-built production-browser cases in the standard gate or the historical T003 live governance journey.

## RC.4 Fixed Model Cohort

The actual CLI runs once from `2026-09-26T21:32:10.275Z` to `21:33:35.568Z` and exits **0**. Its cohort window is `21:32:11.223Z` to `21:33:34.789Z`. Cohort `adfb39ae44a8985bedf4a7f2` contains all six fixed cases, each attempted once, with no retry, omitted failure, changed question or relaxed comparator.

| Case | Ask | Execute | Provider Calls | Executions | Result |
| --- | --- | --- | --- | --- | --- |
| Total | 200 | 200 | 1 | 1 | Passed, 1500 |
| Region | 200 | 200 | 1 | 1 | Passed, 1000 / 500 |
| Monthly | 200 | 200 | 1 | 1 | Passed, July 600 / August 1500 |
| Old-customer average | 200 | 200 | 1 | 1 | Passed, scalar 200 |
| Denied identity | 403 | Not called | 0 | 0 | Passed, `NO_MATCHING_GRANT` |
| Natural ambiguity | 200 | Not called | 1 | 0 | Passed, clarification without execution |

The average's actual saved typed query uses `region eq 华东`, the published old-customer predicate equal to `true`, and `[2026-08-01T00:00:00Z, 2026-09-01T00:00:00Z)`. It requests the average metric with aggregate intent, no dimensions and no time granularity. The result contains exactly one column and one row, numeric value **200**. Root separately asserts these seven semantic/pin/result checks; numeric agreement from a `contains` or fuzzy filter is not accepted as category evidence. All four executions pass the unchanged independent SQL, row-shape and published-plan pin checks.

The immutable ledger is `.semlia/v1-acceptance/current/final-candidates/ff36b7591e1ca33f190b403ffbdef867b1f890565198f0a1cc37bacb5ba1baf9.json`. It records **4/4 positives, 2/2 negatives, five provider calls and four executions**, with stable before/after candidate, model configuration and knowledge basis. Root postflight confirms **32 model steps, 32 agent runs, 31 executions, zero unfinished**, and unchanged bytes/metadata for all 13 watched historical cohort and glossary evidence files. Private root command, before/after and semantic receipts are retained at `0600`.

Independent backend review rechecks all four saved execution responses against the unchanged goldens, exact columns, windows, plan IDs/digests and published pins; verifies both negative cases and all counters; and rehashes the executable and 13 watched historical files. The average's saved execution parameters contain the canonical category, not the rejected alias. The saved knowledge DTOs satisfy the pure approved-basis verifier with the independently recorded original revision digest. No live API, SQL or model call is made during review, and no blocker is found. An initial review-only assertion incorrectly equates the outer command window with the inner cohort window; correcting it to containment does not change either receipt or any product result.

This cohort uses the explicitly approved, normally governed two-category glossary in the model definition. Its basis binds the failed A005 preparation receipt, successful A006 GET-only reconciliation and approved pointer. It is not a first pass against the original knowledge. The [value-grounding record](../V1-T002/value-grounding.md) preserves the original wrapper exit 1; no historical outcome is rewritten. The glossary is LLM guidance, not a deterministic alias resolver or complete member-mapping product.

Through the RC.4 fixed cohort there are **32** provider calls: T002 13, T003 4, RC.2 5, RC.3 5, RC.4 5. These are attempts, not 32 successes. Each candidate has its own denominator. The original browser failures, RC.2/RC.3 model failures and [search performance failure](../V1-T002/catalog-performance.md) remain in the evidence.

## Historical RC.3 Result

`1.0.0-rc.20260927.3` passes its frozen-source standard gates, exact-binary startup and direct embedded browser checks. Its fixed real-model cohort fails one of four positive cases; both negative cases pass. It is not an accepted candidate. No cohort is retried or continued.

The earlier RC.2 failure is preserved in [final-shape](../V1-T002/final-shape.md). The first candidate's embedded route failure and all standard-gate attempts remain in [release-gates](release-gates.md).

## RC.3 Identity

| Field | Value |
| --- | --- |
| Version | `1.0.0-rc.20260927.3` |
| Frozen source | `sha256:b36d6ee7cd81208b67d980dde2a91f1bd37efbdddc66bbfe8d69c9f2ecb52d36` |
| Binary | `sha256:0cff6ed4ee22b16ce8f21d75124d558ab023e3c2d29136efac69e62b5c3b172e` |
| Manifest | `sha256:c83a920cf05d6f4ad8d4f360f481fe64eb8eaf02c11ab36cd71dbf7f4b090aee` |
| Archive | `sha256:aba3a42bf3862946032c13e0ee21498cedbd7efec6b2c986dd2342691fd73cc2` |
| Migration | 33 |
| Published head | `rls_01m3f2pbg0f1br2ba4hbtgjgrs` |
| Model revision | `rev_01m3ev2rhdfw2vcm1atafxx7vd` |
| Provider configuration revision | `sha256:f53085427f9ea4cf20000bca5621c0fcf7630265809dfb7ccc5ff8331259279b` |

The root coordinator runs the actual `candidate-check` CLI, explicitly stops only the owned V1 runtime, then runs `up-candidate` with the same four verified files. Both commands exit 0. Startup installs the verified executable without recompilation. Direct `/api/v1/system/info` reports the exact RC.3 version; `SEMLIA_BUILD_VERSION` is absent from the runtime environment. The retained entry is `http://127.0.0.1:52688/`, candidate mode, no Vite.

## Embedded Browser

`node scripts/acceptance/v1-browser.mjs candidate-preflight` exits 0. Run `1790448240287` reports **4/4 passed in 5.0 seconds**, with child exit 0, no signal or spawn error, and matching before/after candidate identities. Normal-password author login, source inspection and natural revision entry work at 1440x900 and 1024x768; consumer keyboard/focus and reduced-motion checks pass at both sizes. Root visually inspects both natural-revision screenshots: no incoherent overlap, with normal vertical scrolling on compact desktop.

Run-specific private before/child/after/head receipts are all private. Counters remain **22 model steps, 22 agent runs, 23 executions, zero unfinished**. The correct published head is unchanged. No model call, execution or governance mutation occurs during preflight.

## Fixed Model Cohort

The root explicitly authorizes one fixed four-positive/two-negative cohort after the gates and embedded preflight. The CLI runs once from `2026-09-26T18:45:29.229Z` to `18:46:45.114Z` and exits **1**. Cohort `88ce36d84da6f3b72f2198b7` has six unique keys, one attempt per case, all finished; there are no retries or omitted failures.

| Case | Ask | Execute | Provider Calls | Executions | Result |
| --- | --- | --- | --- | --- | --- |
| Total | 200 | 200 | 1 | 1 | Passed |
| Region | 200 | 200 | 1 | 1 | Passed |
| Monthly | 200 | 200 | 1 | 1 | Passed |
| Old-customer average | 200 | 200 | 1 | 1 | Failed value reconciliation |
| Denied identity | 403 | Not called | 0 | 0 | Passed, `NO_MATCHING_GRANT` |
| Natural ambiguity | 200 | Not called | 1 | 0 | Passed, clarification without execution |

The failed case has the correct scalar shape, aggregate intent, metric, old-customer predicate and half-open August interval, with no unrequested granularity. Its region filter is `华东地区`; the source/golden contract uses `华东`. The unchanged strict validator rejects its single `null` result against expected **200**. The result is not converted to zero, normalized after execution or counted as success.

The per-candidate ledger is `.semlia/v1-acceptance/current/final-candidates/6df39d95d2d8c9d5c67dc85b18105eee6064e850ede31031df98d7e59524f212.json`. It records **3/4 positive, 2/2 negative, five provider calls and four executions**, with `identityStable: true`. Independent root postflight verifies the same candidate, head and API model-configuration revision; counters are **27 model steps, 27 agent runs, 27 executions, zero unfinished**. The legacy RC.2 ledger remains byte-identical. Actual model interpretation remains a blocking candidate issue, not a failed source or browser gate.

## Environment Boundary

The fourth standard-gate round preserves the original configuration, database baseline and Docker resources, all earlier candidates and the legacy cohort ledger. At that round's preflight, the original native owner/socket and its earlier supervisor PID are absent; the initial observer exits 1 with `ENOENT`. Its stop cause is not established, and it was not restarted or modified. This state is recorded separately from the healthy isolated V1 candidate, rather than claiming continuous original-native availability.

The prior private-configuration disclosure remains recorded. Credential rotation is not completed; production use requires approved rotation and preservation of ciphertext recovery. This local candidate does not imply signing, publication, remote deployment, business acceptance or enterprise SLA.

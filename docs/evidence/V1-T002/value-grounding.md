# Published Category Value Grounding

## Status

RC.3's fixed final cohort remains **3/4 positive cases and 2/2 negative cases**, with five provider attempts and four executions. Its complete failed ledger and all earlier receipts are retained. A005's one authorized preparation publishes the definition-only glossary through five successful ordinary governance actions but exits **1** on a receipt-verifier DTO mismatch. That original failed/incomplete ledger remains unchanged. A006's separately authorized **GET-only reconciliation exits 0**, verifies the already applied publication and creates a distinct approved supplement pointer without repeating any governance mutation; independent read-only verification finds no blocking issue. Counters remain 27 model steps / 27 agent runs / 27 executions, with zero unfinished work. RC.4's complete candidate gates and fresh model cohort are still pending; neither glossary publication nor reconciliation proves model adherence.

The prompt owner performed only source/private-receipt inspection and offline tests. No retained-runtime, API, database, browser or model operation was performed. The schema, domain, compiler, provider, fixed questions, source rows, golden SQL, expected values and comparator are unchanged.

## RC.3 Diagnosis

The saved cohort is `88ce36d84da6f3b72f2198b7`, stored in the protected `final-candidates/6df39d95d2d8c9d5c67dc85b18105eee6064e850ede31031df98d7e59524f212.json` beneath the owned acceptance root. It binds candidate `1.0.0-rc.20260927.3` to source `sha256:b36d6ee7cd81208b67d980dde2a91f1bd37efbdddc66bbfe8d69c9f2ecb52d36`, executable `sha256:0cff6ed4ee22b16ce8f21d75124d558ab023e3c2d29136efac69e62b5c3b172e` and manifest `sha256:c83a920cf05d6f4ad8d4f360f481fe64eb8eaf02c11ab36cd71dbf7f4b090aee`.

All six attempts finished once. Total, region, monthly, denied identity and natural ambiguity passed. Old-customer average returned HTTP 200 for both Ask and Execute but failed numeric reconciliation. Counters changed from 22 model steps / 22 agent runs / 23 executions to 27 / 27 / 27; unfinished work stayed zero. The ledger records stable runtime/configuration/head identity, release `rls_01m3f2pbg0f1br2ba4hbtgjgrs`, and model revision `rev_01m3ev2rhdfw2vcm1atafxx7vd`. Full candidate gate history is retained separately in [candidate acceptance](../V1-T005/candidate-acceptance.md).

Selected typed fields from the protected receipts show:

- Intent `aggregate`, the correct average measure, no dimensions or temporal granularity, and one result column.
- Correct old-customer predicate `eq true` and half-open interval `[2026-08-01T00:00:00Z, 2026-09-01T00:00:00Z)`.
- Customer `region` filter `eq "华东地区"`, passed unchanged into a SQL parameter.
- Successful execution with `columns: ["demo_202609.average"]` and `rows: [[null]]`; the unchanged independent SQL/golden contract expects scalar `200` with exact category `华东`.

The source fixture contains only region categories `华东` and `华南` (`internal/demodata/scenario.go:152`). Its customer member declares name/type/history semantics but no category values or aliases (`:77`, `:103`). The ten stored published definitions likewise contain no field-specific mapping from the business phrase to its canonical value. The acceptance generator replaces the older demo model's answer-bearing definition with a general, answer-free description (`cmd/v1-acceptance/main.go:26`).

`AskKnowledge` discloses only authorized released assets, including their definition and each member's `id`, `name` and `valueType` (`internal/application/distribution/interpretation.go:60-74`). The actual provider request consists only of the prompt, unchanged question and this knowledge payload (`internal/application/governance/ask.go:193-196`). The acceptance driver sends only its fixed question and explicit release context; it does not attach fixture queries, source records, golden SQL or expected answers (`scripts/acceptance/v1/final-candidate.mjs:11`, `:137`).

This is a missing published category-value grounding problem, not a compiler or comparator defect. `KnowledgeMember` has no value-dictionary field (`internal/domain/semantic/knowledge.go:28-35`). Asset aliases support asset selection, not filter-value rewriting (`internal/domain/distribution/resolver.go:292-306`). Ordinary filters preserve the typed value and bind it exactly (`internal/domain/execution/compiler.go:382-422`). Exact equality with the unrecognized category matches no rows; the derived average's zero-denominator policy returns null. Loosening the filter, changing golden data or converting null into an expected answer would conceal the failure.

### Earlier Success Limitation

The preserved T002 receipt `model-old-customer-average-2-ask.json` used **`contains "华东"`**, with the correct scalar/time shape. Its numeric reconciliation passed on this fixed source, but it does not prove exact-category interpretation. RC.2 used **`eq "华东"`** on the same model revision but incorrectly added month grouping; RC.3 corrected the shape but used **`eq "华东地区"`**. None of these older records is overwritten or relabeled as a later successful repair. They show why numeric equality on a small sample cannot establish correct categorical semantics.

## Repair Boundary

The product change is one generic paragraph in `internal/application/governance/ask.go`:

- Use field-specific canonical category values and alias mappings only when explicitly stated in authorized published knowledge.
- Use the stated canonical value with exact `eq` or `in` matching for a mapped category name.
- Do not invent suffix removal, fuzzy mapping or a `contains`/`LIKE` replacement for an exact category condition.
- Clarify missing or ambiguous required mappings without producing a query.
- Preserve directly specified literals when no mapping is required, and preserve explicitly requested substring matching.

The prompt contains no acceptance-specific category, question, date, asset or answer. Existing privacy, knowledge-as-data, half-open time and explicit-grouping instructions remain intact. No post-model rewriting or deterministic alias resolver is introduced.

The packet's supplemental knowledge uses only the existing model `definition`: an explicitly synthetic, field-specific glossary for the two source categories and their business names, without records, questions, dates or aggregate answers. The original fixture and published baseline remain immutable. A separate approved receipt must prove the exact original content plus the allowed suffix, unchanged spec/references/other objects, normal author confirmation/submission, distinct reviewer approval and publisher release. Merely accepting the latest head is not sufficient.

Existing predicate business terms could encode category equality as an executable governed literal (`internal/domain/distribution/model_plan.go:378-443`). That would require new terms and a model-reference revision and still require correct term selection by the model. It is a broader alternative, not a reason to invent a new member dictionary or weaken this packet's definition-only boundary.

## Prompt Validation

The regression tests were written and executed before the prompt implementation.

```bash
env GOENV=off GOWORK=off GOFLAGS= go test -count=1 \
  ./internal/application/governance \
  -run 'TestAskPromptGroundsCategoryValuesInPublishedKnowledge|TestAskGatePreservesDeclaredCategoryFilters|TestAskCategoryMappingClarificationHasNoQuery'
```

**RED:** exit 1, 0.759 seconds. All six new category-grounding prompt clauses were absent. Existing privacy/injection clauses and the gate-preservation tests did not cause the failure.

```bash
env GOENV=off GOWORK=off GOFLAGS= go test -count=1 \
  ./internal/application/governance \
  -run 'TestAskPrompt|TestAskGatePreserves|TestAskCategoryMappingClarificationHasNoQuery|TestAskSchemaDiagnostics|TestAskTimeBucket'
env GOENV=off GOWORK=off GOFLAGS= go test -count=1 \
  ./internal/application/governance ./internal/application/distribution \
  ./internal/domain/execution
```

**GREEN:** focused package exit 0 in 0.727 seconds. Full uncached packages pass in 0.508, 0.899 and 1.318 seconds respectively. Four gate cases preserve exact canonical strings, a direct literal with a meaningful suffix, an exact value set and an explicit substring operator. Clarification has no executable query, and a mixed clarification/query response is rejected. These are local prompt/schema contracts, not model-adherence simulations.

Independent review finds no blocking issue in the two prompt-owned files. The reviewer repeats all three packages uncached with the same environment controls: governance 0.711 seconds, distribution 1.478 seconds, execution 1.105 seconds, all passed. Review also confirms the evidence distinguishes prompt/schema validation from model adherence and retains the RC.3 failure and earlier substring-match limitation. No live operation was performed by the reviewer.

## Supplement Driver And Review

The separately owned driver adds `prepare-values` to the existing exclusively locked CLI. It reserves a private one-shot `value-knowledge.json` before mutation and never repeats or resumes a partial or uncertain preparation. Five normal actions create the definition-only operation, confirm its business declaration, submit it, approve it as a distinct reviewer and publish it as the publisher with an expected-head precondition. Polling reads validation state; it does not repeat a mutation. All request and result evidence remains private.

The source proof follows the released public `region` reference through the model member binding, data-asset member, source snapshot, dataset/field and physical binding. The dedicated synthetic reader queries only distinct category values in an explicitly read-only transaction, bounded to detect unexpected extra values. Its role, database, schema ownership marker and exact two-category set must match. It does not read customer/order records or measures, and these source values are not appended as an ad hoc provider message.

The original fixture fingerprint, source SQL fingerprint, saved production content and the original release at immutable `state.latestRelease` remain strict anchors. Normal API readback verifies every original `state.published` asset/object pin. Before any mutation, the current full manifest must equal that original manifest, including its digest and model revision. After publication, only the model revision may differ; its content must equal the original plus the single glossary suffix. A separate `value-knowledge-approved.json` pointer is created only after complete readback of confirmations, validation, proposal/review, independent principals, publication attribution, content and the final manifest, with unchanged model/agent/execution counters.

`governanceHeadProof` no longer writes historical correction evidence. Without a supplement it retains strict original-content verification; with a supplement it accepts only the exact complete approved receipt, matching current release/revision/manifest/content and freshly read original release. Missing, partial, foreign, symlinked or changed evidence fails closed. Candidate cohorts retain the original fixture/fixed-case identities and additionally bind the approved knowledge basis and protected receipt/pointer identities before and after; the one-cohort-per-candidate rule is unchanged.

### Driver RED/GREEN

The driver's author reports these real-filesystem regressions, all using synthetic API callbacks and no live service:

- The initial definition-only stub fails its single target-contract test before implementation.
- A 26-test run exposes three failures: historical ledger mutation, rejection of a legitimate supplement, and failure to reject cohort knowledge-basis drift. The corrected run passes 26/26.
- A filesystem counterexample shows that a newly appearing `final-candidates` inventory still permits all five mutations. The inventory guard is corrected. API fixture alignment also removes nonexistent `operationId`/`version` assumptions from proposal detail; this 12-test phase has six failures before passing 12/12 in 0.344 seconds. Fixture-shape corrections are distinguished from product defects.
- The initial full 68/68 GREEN does not cover acceptance of an already-drifted baseline. Read-only review identifies that the latest model's unchanged content alone does not prove the other released objects are unchanged. Two real RED cases show another asset's drift still permits five mutations and a modified original-release receipt is not rejected; the 14-test run fails in 0.444 seconds.
- The strict immutable-original-manifest anchor repairs those failures: 14/14 pass in 0.359 seconds. Additional counterexamples reject model same-content/new-revision drift, other asset/object/digest drift and inconsistent original API pins before any POST.
- Independent review then identifies an actual HTTP DTO mismatch: release-manifest objects and the original published-object state omit `contentDigest`. The earlier 71/71 GREEN uses an overcomplete fixture and does not prove this route is usable. A real-shape fixture produces 11 failures in a 16-test run in 0.178 seconds. The corrected run passes 16/16 in 0.435 seconds.
- A further lifecycle counterexample covers the release DTO's derived `projectionStatus` changing. Whole-object equality incorrectly rejects an otherwise identical later proof: 17 tests produce one failure in 0.378 seconds. That initial fixture used `succeeded`; the actual release-projection transition is `pending` to `ready`. After correcting the fixture literal to `ready`, the focused 17/17 run passes in 0.388 seconds. The original RED proves the overly broad equality mechanism, not the validity of its initial status literal. The explicitly approved correction excludes only this derived field from immutable release-fact equality; raw receipts still retain it. Release identity, manifests, attribution, pins and publisher remain strict, with mutation counterexamples rejected.

The DTO correction preserves exact manifest kind/ID/version equality and obtains the binding digest from the immutable published production target, where it is actually exposed. A normal GET of that binding's fixed publication proves its operation/version/set-digest attribution, target declaration, source references and required SHA-256 content digest. Wrong declaration, operation, release, version, set digest or missing/invalid target digest is rejected. It does not add a fictitious manifest field or weaken the complete-manifest comparison.

```bash
node --test scripts/acceptance/v1-value-knowledge.test.mjs
node --test scripts/acceptance/v1*.test.mjs
node --check scripts/acceptance/v1/value-knowledge.mjs
node --check scripts/acceptance/v1-value-knowledge.test.mjs
node --check scripts/acceptance/v1-model.mjs
node --check scripts/acceptance/v1/correction.mjs
node --check scripts/acceptance/v1/final-candidate.mjs
node --check scripts/acceptance/v1-final-candidate.test.mjs
node --check scripts/acceptance/v1-model.test.mjs
git diff --check
```

The author's latest full acceptance-script unit run passes **73/73**, zero skips, in 3.359 seconds. All seven syntax checks and the diff check exit 0. The preceding 71/71 run in 3.384 seconds and 72/72 run in 3.338 seconds remain intermediate results, not evidence that subsequent DTO/lifecycle findings were already covered.

Final independent driver review finds no remaining blocking issue in the then-covered offline cases. It verifies the fixed original full-manifest/pin anchor, actual binding DTO proof and the single derived projection-status exclusion. The reviewer repeats the complete Node set: **73/73 passed**, zero skips, in **3.370 seconds**. Review is offline only; it does not prove the subsequent preparation command or model adherence.

## Authorized Live Preparation Failure

Root executes one authorized live preparation after a read-only preflight verifies the actual source binding, dedicated reader and complete original manifest. All five mutations occur exactly once through their normal API roles:

| Action | Actor | HTTP Status | Replayed |
| --- | --- | --- | --- |
| Create | Author | 201 | false |
| Business confirmation | Author | 201 | false |
| Submit | Author | 202 | false |
| Review | Independent reviewer | 201 | false |
| Publish | Publisher | 201 | false |

The resulting operation `prodop_01m3fm50n7edmt8ta2nmjf1y96` is released at head `rls_01m3fm52csedprcf4dptew0kzz`, model revision `rev_01m3fm52cvedpsrdm7bjxzy5bk`. The wrapper exits **1** during verification, not during publication. Its expected `beforePins` shape omits the actual semantic before-pin `contentDigest`; this is a separate DTO contract failure from the earlier object-manifest digest finding. The protected `value-knowledge.json` remains `state: failed`, `complete: false`; its preserved digest is `sha256:72a6e8e53d4422de387a100552fe70f88ff80d39e54db3d9faec5c79823171f4`. `value-knowledge-approved.json` is absent.

Root's independent normal-API readback, recorded in `value-knowledge-root-after-failed.json`, confirms the model content is exactly the original plus the approved definition suffix, only the model revision differs in the full manifest, and the actual before-pin content digest matches the fixed original revision. This proof does not turn the failed preparation command into a pass or manufacture its missing pointer. An offline clone check attributes the remaining verifier failure to this DTO field; the original failed ledger is not edited.

Both the preparation's before/after counters and root's independent postflight show **27 model steps, 27 agent runs, 27 executions, zero unfinished work**: zero new model calls and zero query-execution API calls. Root's preservation receipt confirms unchanged original state, fixture, golden data, model/correction ledgers, legacy cohort and RC.3 cohort bytes/metadata, and unchanged original environment.

These failed-preparation facts were integrated by reading only allowlisted receipt fields. No additional API, database, runtime or model action was performed by the evidence owner. The permitted A006 recovery boundary is a reviewed validator correction followed by explicitly authorized **GET-only reconciliation** of the existing published operation. It must preserve the failed ledger and must not repeat create, confirmation, submission, review or publication.

## A006 Offline Reconciliation

The A006 implementation separates publication-fact verification from the original attempt's terminal state. It requires the actual `beforePins.contentDigest` and cross-checks it with the fixed original catalog revision and original released production target, including declaration, target ID and release linkage. It does not discard the digest, change the original failed flags or manufacture a successful clone of the failed attempt.

`reconcile-values` uses the existing exclusive command lock and a separate exclusive-create `value-knowledge-reconciliation.json`. It accepts only the known five finished, successful, unreplayed mutations and refuses unknown, partial, drifted or already reconciled attempts. Workspace transport is GET-only; normal session authentication is a separate permitted operation, not a login-limit bypass. Explicit read-only counter queries and the dedicated category reader provide preservation checks. No model, execution, source-test or governance mutation endpoint belongs to this reconciliation path.

Fresh facts must identify the exact publication saved in the original successful publish response, not an arbitrary latest head. The proof re-reads original/before/after releases and revisions, both production operations, confirmation witness, proposal/review attribution and source binding facts. Only a completely verified, counter-stable result may create the approved pointer. That pointer and the candidate knowledge basis bind both the new reconciliation and the preserved original failure/root-command digests.

### A006 RED/GREEN

The implementation owner's offline runs report:

- The real `beforePins.contentDigest` fixture produces **8 failures in 18 tests**, 0.354 seconds. Exact original-digest verification repairs it: **18/18 pass**, 0.454 seconds.
- The reconciliation stub produces **2 failures in 22 tests**, 0.801 seconds. The GET-only, one-shot proof path passes **22/22**, 1.040 seconds.
- A real-filesystem counterexample exposes an overbroad permanent inventory comparison that would reject adding a future candidate ledger after successful reconciliation: **1 failure**, 0.119 seconds. The correction keeps the entire inventory fixed during reconciliation, preserves every historical file's bytes and identity afterward, and permits later new entries in `final-candidates` without relaxing preservation of earlier entries.

```bash
node --test scripts/acceptance/v1-value-knowledge.test.mjs
node --test scripts/acceptance/v1*.test.mjs
node --check scripts/acceptance/v1/value-knowledge.mjs
node --check scripts/acceptance/v1-value-knowledge.test.mjs
node --check scripts/acceptance/v1-model.mjs
node --check scripts/acceptance/v1/correction.mjs
node --check scripts/acceptance/v1/final-candidate.mjs
node --check scripts/acceptance/v1-final-candidate.test.mjs
node --check scripts/acceptance/v1-model.test.mjs
git diff --check
```

The owner's complete V1 Node set passes **83/83**, zero skips, in **4.059 seconds**. All seven syntax checks and the diff check exit 0. A direct offline `verifySupplementFacts` check on the saved real DTO facts passes without modifying or cloning success flags; the five monitored private evidence files retain their bytes, inode, mtime and ctime. All sessions end without live API, SQL, runtime or model activity.

Independent A006 review finds no blocking issue and repeats the full Node set: **83/83 passed**, zero skips, in **4.055 seconds**. Its direct check of saved failed DTO facts passes without cloning or changing the attempt flags. The offline original-revision digest is taken from the independently saved root preflight basis, not from a newly performed API request. Five original evidence files retain bytes, file identity, timestamps and mode; the original ledger remains failed/incomplete and the root command retains exit 1. No approved pointer or reconciliation receipt exists at this review point.

The offline checks above do not themselves establish approved-pointer creation or published-baseline adoption. The subsequent authorized live result is recorded separately below; the original A005 command remains failed and all prior model denominators remain unchanged.

## A006 Live Read-Only Result

Root runs `node scripts/acceptance/v1-model.mjs reconcile-values` once. The protected root command receipt records exit **0**, no signal, and a 2.778-second command interval. Independent root postflight also succeeds. The new reconciliation ledger is `kind: read-only-reconciliation`, `state: verified`, `complete: true`; it binds, rather than rewrites, the original failed wrapper receipt and its exit-1 root command.

Fresh readback retains head `rls_01m3fm52csedprcf4dptew0kzz` and model revision `rev_01m3fm52cvedpsrdm7bjxzy5bk`. The resulting head proof reports `correctBaselineContent: true`, **`originalBaselineContent: false`**, and knowledge basis `approved-values` verified by read-only reconciliation. This distinction is intentional: the original fixture baseline is unchanged, while the exact definition-only supplement is separately approved.

The new approved pointer binds reconciliation digest `sha256:9e7da97a7193989cc8980ee39d6afe7d3a576ec418fe23804aaad362ec3619a4`, pointer digest `sha256:d49663b588c6b78e99bad07f1f64b35f762e7fe0de903b96d2a01fa8ef410e9c`, original failed receipt digest `sha256:72a6e8e53d4422de387a100552fe70f88ff80d39e54db3d9faec5c79823171f4` and original exit-1 command digest. These are a separate evidence chain, not a replacement successful preparation ledger.

The before, reconciliation and independent after receipts all show **27 model steps, 27 agent runs, 27 executions and zero unfinished work**. Root postflight records **zero governance mutations, zero model calls and zero query executions** during reconciliation. It confirms preservation of all **13 original monitored files**, including the failed preparation, root command/preflight/failure/postflight records, state, fixture, golden data, earlier acceptance ledgers and the RC.3 cohort, plus the original environment.

Evidence paths beneath the protected acceptance root are `value-knowledge-reconciliation-root-before.json`, `value-knowledge-reconciliation-root-command.json`, `value-knowledge-reconciliation-root-after.json`, `value-knowledge-reconciliation.json` and `value-knowledge-approved.json`. The evidence owner reads only their allowlisted structured status, counters, identity and preservation fields, not raw stdout, stderr, credentials or full API responses.

Independent read-only verification finds no blocking issue. The reviewer recomputes digests, device/inode, mode/owner and modification/change timestamps for all 13 historical files and confirms equality with the saved before receipt; three original configuration hashes also match. The command exits, failed original flags, new verified receipt, exact release/revision, unchanged 27/27/27 counters and zero unfinished work match across receipts. RC.3 executable/manifest identities are unchanged. Pure `approvedKnowledgeBasis` validation passes over the complete saved DTO facts and pointer, binding the reconciliation, pointer, original failure and original command digests.

This independent check uses local JSON/file reads, hashes and pure validation only, without API, SQL, model, test, runtime or file-modification operations. Its old revision digest comes from the independently saved original root preflight basis; it does not claim a fresh API read. Database/environment preservation is attributed to root's postflight `originalEnvironmentPreserved` receipt rather than a new independent database query. The evidence is sufficient to return to T005 candidate gates, not to relabel the original wrapper as successful or declare RC.4/1.0 accepted.

Any future numeric PASS must also undergo a separate typed-filter semantic audit: this exact-category question must use the canonical value with exact `eq`/`in`, not an accidentally matching substring/fuzzy condition. The original numeric comparator and fixed cases remain unchanged. Published glossary facts and local tests cannot guarantee that a model will follow them.

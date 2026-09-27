# V1-T003 Final Source Regression

## Scope

Execution packet: `docs/specs/v1-convergence/packets/V1-T003-A004.yaml`.

Only one integration case and its API imports in `web/src/governance.test.tsx` are changed. The file retains all 14 cases, including both legacy GovernanceRuntime checkpoint/submit hook tests. No application behavior, timing threshold, worker setting, skip or retry policy is changed. All data is synthetic; no browser runtime, Docker, database or model operation is performed.

## Failure

The first final `check-source` run failed with 339 of 340 nested Vitest tests passing. Its sole frontend failure was the revision-workbench integration case expecting the removed `提交审核` button and legacy proposal APIs. The rendered control was `保存修订并继续确认`. The host script-shell isolation precondition passed; this is a stale deterministic assertion, not evidence of resource contention.

Original reproduction, before editing:

```sh
pnpm --dir web exec vitest run src/governance.test.tsx -t 'creates and submits real proposals from the revision workbench'
```

Exit 1, one failed case / 13 excluded by the name filter, 2.05 seconds. The assertion failed at the old line 213 after 599 ms: `Unable to find an accessible element with the role "button" and name "提交审核"`. The default test configuration is unchanged.

## Contract Coverage

The replacement case uses the canonical `metric` spec rather than an unsupported legacy string field. It preserves calculation-rule editing: the user changes a typed derived expression from payment plus confirmed refund to payment minus confirmed refund, using the real structured editor and published-reference dialogs to pin both operands. It does not replace the calculation edit with a definition-only change.

The Workbench, ProductApp, revision command/preparation helper, reference picker and resulting production panel are real components. Only catalog and production API boundaries are mocked. Assertions require:

- One production draft with the entire expected body, exact asset and base revision, unchanged owner/identity/content, and exact atomic `spec` before/after values.
- Exact current-asset and immutable-revision reads, applied release attribution and producer operation version 3, and verified source snapshot reads.
- Preserved snapshot coverage, source evidence, target evidence IDs and both dependency pins; consumed candidates are cleared.
- While the create response is pending, the save button is disabled, the workbench remains visible, the URL is unchanged and the production view is absent.
- Only a successful response opens `/work/operations/prodop_revision`. The reason appears as unconfirmed context, the business-rule declaration is empty, its confirmation checkbox is unchecked and the record-confirmation action is disabled.
- No implicit production confirmation, submit, validation, review, publish, replacement or generation; no legacy proposal create/submit, review or publish.

Existing Workbench/command tests retain double-submit, frozen request, unknown-result same-key retry and identity/late-result fencing coverage. They are rerun, not replaced.

## Verification

An intermediate synthetic reference fixture omitted `currentRevision.content`, causing one focused test failure and a component exception. The fixture was completed; no product code or assertion was weakened. An intermediate typecheck reported missing fixture fields; complete typed catalog details and snapshot-member metadata resolve those errors.

```sh
pnpm --dir web exec vitest run src/governance.test.tsx -t 'saves a pinned calculation revision'
pnpm --dir web exec vitest run src/governance.test.tsx src/KnowledgeRevisionWorkbench.test.tsx src/knowledgeProductionRevision.test.tsx src/ProductApp.test.tsx src/SemanticProductionPanel.test.tsx
pnpm --dir web typecheck
pnpm --dir web lint
```

- Named replacement: exit 0, one passed / 13 excluded by the name filter, 2.22 seconds.
- Complete related suites: exit 0, 5 files / 100 tests passed, 9.56 seconds, including all 14 governance cases.
- Typecheck: exit 0.
- Lint: exit 0, four existing warnings in KnowledgeViews, ModelConfigurationView and embeddingRuntime; no new warning.

Independent reviewer `/root/v1_backend_audit` reported no blocking findings, independently passed the same 5 files / 100 tests in 9.07 seconds, and confirmed `git diff --check` exit 0. All implementer and reviewer sessions ended before handoff. The implementation typecheck and lint results above apply to the frozen test file.

T005's full source gate must rerun from these frozen inputs. These component results do not establish candidate-binary, live governance, model-quality or final release acceptance.

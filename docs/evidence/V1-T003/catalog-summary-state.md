# Catalog Summary State

## Scope and Observed Failure

Packet: `docs/specs/v1-convergence/packets/V1-T003-A005.yaml`.

Root's normal consumer browser inspection of the preserved RC.4 found that a cold knowledge directory labelled ten published assets as unpublished `@0`, with `0/6` readiness and six attention items. Opening the model's authoritative detail showed production version 5; returning changed only the loaded row. This is a real browser failure, not a passing RC.4 catalog acceptance result. Its original screenshot and accessibility evidence remain unchanged.

The list DTO does not supply authoritative publication, version sequence or readiness. `catalogRuntime` already projected unknown deployment/workflow for summaries, but the list converted those unknowns to negative assertions and complete-looking version counts. Detail responses with missing, forbidden or failed release authority also require unknown, not an inferred draft.

## Bounded Changes

- Summary projection retains unknown publication/workflow, uses a pending-version label and does not manufacture six failed readiness gates from absent detail content.
- Directory rows show pending publication/readiness without a zero version, progress bar, attention count or all-clear statement. With root's same-cause scope approval, cold responsibility and relationship/implementation columns also show pending values rather than unassigned ownership or zero bindings.
- Only an available release section with a revision pin, or explicit `not_released`, establishes a known publication state. Missing/forbidden/failed sections and available sections without a pin remain unavailable; the detail header and technical production pointer do not claim unpublished.
- Formal/draft filters and their counts remain unavailable while any listed state is unknown, a list request has failed/is loading, or a catalog page remains unread. The notice explicitly says that all versions are displayed. A requested formal/draft deep link cannot silently produce a definitive empty result. Once the complete loaded catalog has known states, existing filtering resumes; the All count remains the loaded-row count alongside the existing loaded/total summary.
- Existing published, explicit unreleased draft and simultaneous published/draft classifications remain intact. No detail prefetch, new request, API, backend, permission, dependency or domain model was added. Known detail owner/relationship values are unchanged.

Implementation changes are limited to `ProductApp.tsx`, `catalogRuntime.tsx` and small directory-only styles. Regression changes are limited to `Catalog.test.tsx` and `ProductApp.test.tsx`.

## RED and GREEN

Commands use the unchanged default Vitest configuration, without retries, worker overrides, threshold changes or new test skips.

```sh
pnpm --dir web test src/Catalog.test.tsx src/ProductApp.test.tsx
pnpm --dir web test src/Catalog.test.tsx -t 'does not infer draft or unpublished'
pnpm --dir web test src/Catalog.test.tsx -t 'loads a real registry view and opens asset detail'
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web typecheck
git diff --check -- web/src/ProductApp.tsx web/src/catalogRuntime.tsx web/src/Catalog.test.tsx web/src/ProductApp.test.tsx web/src/styles.css
```

| Check | Actual result |
| --- | --- |
| Initial focused RED, unmodified implementation | Exit 1; 10 failed / 84 passed |
| Detail unavailable-label RED | Exit 1; 4 failed / 31 excluded by the name filter |
| Detail neutral-status RED | Exit 1; 4 failed / 31 excluded by the name filter |
| Cold responsibility/implementation RED | Exit 1; 1 failed / 34 excluded by the name filter |
| Final focused GREEN | Exit 0; 2 files / 94 tests passed, 8.22 s |
| Full frontend suite | Exit 0; 40 files / 350 tests passed, 13.13 s; no skipped tests |
| Lint | Exit 0; no errors; four existing warnings in KnowledgeViews, ModelConfigurationView and embeddingRuntime |
| Typecheck | Exit 0 |
| Scoped diff whitespace check | Exit 0 |

An intermediate focused run had five failures because a deep-linked detail response completed before the directory's existing debounced list request. The tests now await the real directory row after returning, without altering the debounce, timeout or application behavior. That failed log is retained alongside all RED/GREEN logs. A subsequent focused run passed all 94 cases before the final same-cause field/neutral-style additions; the final result above includes those additions.

Coverage includes cold draft/released deep links, mixed loaded/unloaded rows, explicit unpublished detail, current versus older published basis, missing/forbidden/failed release authority, a malformed available-without-pin section, and pagination. The pagination case reads only the first detail, loads another summary without an extra detail request, then explicitly opens the final detail before full version counts become available. Assertions check detail-request counts, preserving the lazy boundary.

Private test logs are in `/tmp/semlia-catalog-state-qr4Eaz` with directory mode `0700` and file mode `0600`. No raw credentials, private configuration or model responses are included here.

## Handoff and Limits

Independent reviewer `/root/v1_backend_audit` reported no blocking findings after reviewing the source and saved RED/GREEN output, including the 94-test focused result. This is an independent review, not an independent test rerun or new-candidate browser pass. Root also reviewed the state/filter changes.

Source is frozen. All implementer test sessions have ended. No build, live browser/API/model/database operation, publication, commit or previous candidate/cohort evidence edit was performed in this repair.

The summary API still cannot support authoritative cold version filtering; the UI exposes that limitation rather than fetching every asset or guessing. Component tests do not establish visual or embedded-executable acceptance. Root must rebuild a new exact candidate and repeat ordinary browser acceptance, including the cold directory, at 1440x900 and 1024x768. Prior RC.4 gates/cohort remain historical evidence, not proof of this changed source.

## Production Browser Assertion Alignment

Round 7's first real `check-browser` run failed the compact-desktop identity/workspace case at the old `production.spec.ts:12` expectation that the draft filter is always selected. The production browser suite recorded 11 passes and one failure. That RED and its protected error context are retained, not overwritten or reclassified as a passing gate.

Both Playwright projects share the launcher's single owned workspace. The recorded order was desktop's six cases followed by compact-desktop's six cases. Desktop's workflow created and restored three semantic assets plus seven governed objects before compact-desktop opened its cold catalog. Offline inspection of the saved desktop DTO confirms the restored manifest equals the published manifest. Thus the first project sees a complete empty catalog, while the later project receives nonempty summaries with unknown publication states. The failure context shows the draft filter labelled pending, disabled and not selected, consistent with A005's intended contract.

Root authorized a test-only alignment in the first case of `web/e2e-production/production.spec.ts`. The test captures the actual page's catalog GET response and requires HTTP 200, an item array, total equal to the item count and no next cursor. It then uses that response, not the observed UI, to choose mutually exclusive expectations:

- Empty: zero rows; draft zero is enabled and selected; formal zero is enabled; All zero is not selected; no unavailable-state notice.
- Nonempty: every returned row is present; All is selected with the exact count; formal/draft pending controls are disabled and not selected; the explicit unavailable notice is visible; every row has pending publication/readiness without unpublished, `@0`, failed/all-clear readiness or a progress bar.

No OR assertion, fixture reset, timeout increase, retry, product change or alteration to the full source-production/review/publish/persistence/rollback workflow was made. Root independently reviewed the strict branching and the response's omitted-next-cursor contract.

Static checks after this test-only change: `pnpm --dir web lint` exit 0 with the same four existing warnings; `pnpm --dir web typecheck` exit 0; scoped `git diff --check` exit 0. Protected logs are `production-assertion-lint.log` and `production-assertion-typecheck.log` in the private directory above. All sessions ended. The implementer did not rerun the browser suite; the corrected real browser GREEN remains pending the release worker's separately named run.

# Catalog Search Membership

## Scope and Failure

Packet: `docs/specs/v1-convergence/packets/V1-T003-A006.yaml`.

Root's normal consumer CUA inspection of RC.5 found a stable mismatch after opening the published model and returning to the directory: searching its full address `demo_202609.model` reported one loaded result out of one, but displayed zero rows and an empty-result message. Name search showed the same model. The original protected RC.5 accessibility/screenshot evidence and executable are preserved.

`catalogRuntime` correctly limits `catalogAssetIds` to the server's query page while retaining off-page details separately. `ProductApp` then applied a second substring test using incomplete display fields. The projected item key omits the namespace, and these fields do not contain the full revision content searched by PostgreSQL FTS. Both qualified addresses and valid server-only content matches could therefore be discarded. Merely adding the namespace would not fix the FTS case.

## Repair

The current server page remains the sole asset-search membership boundary. ProductApp no longer rejects those assets using a local substring test. It retains the existing local version, domain, semantic type and governed-object selections, as well as the chosen directory sort. Local text matching only narrows the presentation of loaded child objects; when no displayed child contains the server query, the asset's typed children remain available to expand.

No runtime cache, cursor, API, SQL, permission or lifecycle behavior changes. Cached off-page details cannot re-enter the list, and the repair does not fetch additional details. Displayed/local-filtered counts remain distinct from loaded/server-total counts.

The test-only catalog provider supplies its own deterministic query/type membership and matching total instead of relying on ProductApp to fake filtering. This is explicitly synthetic substring behavior, not an implementation or validation of PostgreSQL FTS. Synthetic asset data, model/source fixtures and golden answers are unchanged; no production-only test-mode flag was added.

## RED and GREEN

All commands use the existing configuration without increased timeouts, retries, worker overrides or skipped tests.

```sh
pnpm --dir web test src/Catalog.test.tsx src/ProductApp.test.tsx
pnpm --dir web test src/ProductApp.test.tsx -t 'makes the catalog fixture|preserves local domain'
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web typecheck
git diff --check -- web/src/ProductApp.tsx web/src/Catalog.test.tsx web/src/ProductApp.test.tsx web/src/testing/catalogFixture.tsx web/e2e-production/production.spec.ts
```

| Check | Actual result |
| --- | --- |
| Original implementation, new API-membership RED | Exit 1; 4 failed / 94 passed, 23.53 s |
| Original test adapter, fixture-membership RED | Exit 1; 1 failed / 1 passed / 59 excluded by the name filter, 3.52 s |
| Focused GREEN | Exit 0; 2 files / 100 tests passed, 15.42 s |
| Full frontend suite | Exit 0; 40 files / 356 tests passed, 40.71 s; no skips |
| Lint | Exit 0; no errors; the same four existing warnings in KnowledgeViews, ModelConfigurationView and embeddingRuntime |
| Typecheck | Exit 0 |
| Scoped diff check | Exit 0 |

The API-boundary tests cover full-address search both cold and after authoritative detail caching, content-only matches absent from summaries, pagination of those matches, exclusion of cached off-page details, a genuine empty server result, and an older first-page response arriving after the new query. Existing stale append-page coverage remains. Detail-call counts enforce no N+1 reads. Fixture and UI regressions retain local domain/type/governed-object filtering and explicitly distinguish one displayed row from two loaded/server rows.

Logs are preserved under `/tmp/semlia-catalog-search-wmAEnr`, with directory mode `0700` and log mode `0600`. Public evidence contains no private configuration, credentials or model responses.

## Browser Regression and Handoff

The existing production browser workflow retains every source generation, confirmation, review, publish, persistence and rollback assertion. A read-only regression is appended after those steps: open the known published customer asset, return, search its exact qualified address, require a successful complete one-item catalog response with that exact address, and require the unique visible published row with matching loaded/displayed counts. It saves a separate `catalog-search-{project}.png` screenshot when run at either supported desktop size.

This appended real-browser step was written but not executed by the implementer. Root and independent reviewer `/root/v1_backend_audit` reviewed the frozen implementation and strict browser assertions without blocking findings. The review is static/source and saved-test-evidence review, not an independent browser run.

Source is frozen and all implementer sessions have ended. No build, live API/browser/model/database operation, publication or Git write occurred in this repair. RC.5 remains historical failed-search evidence. The new executable and both actual desktop browser regressions remain for the release worker/root to validate before any model cohort or Git publication.

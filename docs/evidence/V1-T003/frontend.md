# V1-T003 Frontend Evidence

## Scope

The frontend uses exact asset, immutable revision and answer-release references for Ask evidence. Historical content and member names are loaded through the existing catalog revision endpoint, in a cache separate from the current editable asset. Correction loads the same asset's current baseline and keeps the answer pin in unconfirmed revision context. Missing, unauthorized or mismatched historical content never falls back to current content or a different asset. Natural production revision evidence is recorded in `natural-correction.md`.

Workspace changes invalidate in-flight cache responses, including a switch away and back. Concurrent exact reads are deduplicated. Account or principal changes remount the catalog and retained Ask subtree; a same-principal authorization-version refresh preserves already-read content under the existing session behavior. Latest capabilities and server authorization govern subsequent actions.

Ask is mounted only after first use. Leaving Ask aborts pending client requests, prevents late responses from replacing another question, and pauses execution-history work. Completed answer and ephemeral query rows remain in memory for back navigation; reactivation does not execute a plan. Cancellation copy does not claim that client abort confirms server cancellation. Consumers without `asset.propose` can inspect evidence or refine a question but cannot enter correction through these Ask/evidence controls.

## RED / GREEN

All component input is synthetic. No model requests were made by A001.

- Initial focused Vitest: 2 failed / 24 passed. Exact historical evidence rendered current catalog content instead, and the Ask callback provided an asset ID instead of the complete pin. Both tests pass after the implementation.
- Ask cancellation RED: 1 failed / 7 passed; no AbortSignal reached the Ask client. GREEN forwards the signal and rejects a late answer after navigation and another question.
- Execution inactivity RED: 2 failed / 4 passed; a never-active panel read history and an active execution was not aborted when hidden. GREEN covers no initial history request, cancellation/late-response fencing, retained completed rows and no automatic re-execution.
- Correction permission RED: 1 failed / 14 passed; the consumer correction button was enabled. GREEN keeps issue reporting/refinement available and disables correction with an explicit permission message.
- Identity boundary RED: the old catalog/answer subtree survived a same-workspace principal switch. GREEN remounts it on account/principal changes, while preserving it for a same-principal authorization-version refresh.
- Additional cases cover an uncached current correction baseline, historical lookup 403/404/mismatched revision, request deduplication, delayed A-to-B-to-A workspace responses, immutable-cache/current-cache separation, pinned member names, and structured work routes with scope/release/origin parameters.

## Commands

- `pnpm --dir web typecheck`: exit 0 after the final implementation.
- `pnpm --dir web lint`: exit 0; four existing warnings (Fast Refresh exports in KnowledgeViews/ModelConfigurationView and two embeddingRuntime effect-cleanup ref warnings).
- Focused component run: 6 files / 108 tests passed before the final identity and historical-member regressions; the identity follow-up run passed 5 files / 104 tests.
- `pnpm --dir web test`: 38 files / 312 tests passed before the final identity and historical-member regressions. A final 315-test run concurrent with the isolated harness build returned 311 passed / 4 failed: the first asynchronous load exceeded Testing Library's 1-second wait in AuditRuntimeView, SourceContents and two Catalog cases. No assertion or timeout was relaxed. A single-worker full rerun follows below.
- `pnpm --dir web exec vitest run --maxWorkers=1`: exit 0, 38 files / 315 tests passed (123.28 seconds), with the final identity and historical-member regressions included.
- `git diff --check`: exit 0.

## Browser Boundary

`scripts/dev/production-acceptance.sh --suite desktop` is the existing isolated protocol-stub harness, not proof of real configured-model behavior. Its runner uses one Playwright worker, a fresh random `spacc_` owner, fresh PostgreSQL credentials/database, loopback-only ephemeral ports, and cleanup restricted by Compose project plus exact owner labels. It does not use or modify the user's original PostgreSQL or V1 runtime.

The existing production suite retains server persistence, ten-object validation, independent reviewer/publisher, invalidated approvals, publication, rollback, rollback restoration, immutable manifest and matching-existing-asset assertions. Obsolete query routes and the removed draft-production list are replaced with current work-operation routes and the completed inbox. The separate A002 normal-password browser evidence is required for real-runtime/model acceptance.

First updated production harness run: `spacc_3794be54a7f80309`, exit 1, 10 passed / 2 failed. Both sizes passed the full ten-object governance/publish/rollback/restore assertions, then failed in the newly adapted inbox test because an operation detail does not contain the list summary's `title`. The test now reads the exact operation's summary from the normal list API before selecting its UI entry. No product assertion was weakened. The run's own containers, volumes and network were verified absent after launcher cleanup.

Final production harness run: `spacc_326550cc6ec97da4`, exit 0, **12 / 12 tests passed** in 2.6 minutes at 1440x900 and 1024x768. Both complete production journeys, including the resumed inbox record and existing-asset match, passed. Keyboard focus restoration, reduced-motion and horizontal-overflow assertions passed. Owner-filtered container, volume and network queries returned no resources after cleanup; the launcher stopped its own application process.

Synthetic screenshots were inspected at:

- `.semlia/production-acceptance/spacc_326550cc6ec97da4/matched-desktop.png`
- `.semlia/production-acceptance/spacc_326550cc6ec97da4/keyboard-match-compact_desktop.png`
- `.semlia/production-acceptance/spacc_326550cc6ec97da4/matched-compact_desktop.png`
- `.semlia/production-acceptance/spacc_326550cc6ec97da4/restored-desktop.png`
- `.semlia/production-acceptance/spacc_326550cc6ec97da4/restored-compact_desktop.png`

The desktop form and compact matching dialog are legible and correctly framed; the compact search input has visible focus. These screenshots contain only generated acceptance data. They are distinct from A002's real-runtime Ask evidence.

## Residual Boundaries

- This scope is desktop only: 1440x900 and 1024x768.
- Raw questions and query rows are not persisted by these changes. Refresh restores the pinned evidence route, not the prior in-memory Ask conversation or ephemeral rows.
- Immutable reads are still authorized by the server. Already-read content remains visible on a same-principal permission-version refresh by the existing session contract; changing identity or leaving the authenticated tree clears it.
- The standalone native validation script's current login/member/settings locators did not require changes. It was not run as a second native runtime by A001.

## Historical Header Visual Regression

The real-runtime functional journey's original compact historical screenshot, `.semlia/v1-acceptance/current/browser/1790433741115/historical-r1-compact-desktop.png`, is visually unacceptable: the title occupies one narrow grid cell and wraps vertically while the correction button stretches across the content. Functional assertions alone did not detect this failure. This image is not included in the passing visual assessment of the separate isolated-harness screenshots above.

A002's read-only reproduction `1790433971321` made no model or execution calls. At 1024x768, computed grid columns were `36px 836px`, title width was 36px, correction-button width was 836px and heading height was 200px. At 1440x900, title width was 1006px, button width was 114px and heading height was 25px. The new compact visual gate failed as expected.

The structural unit RED expected the established three-child header contract but received only two children. The historical view now uses the existing type mark, title-copy and header-side DOM structure. Known types use their actual shared type icon; unknown types retain a neutral icon slot. No global CSS or ordinary catalog header changes are included.

`pnpm --dir web exec vitest run src/Catalog.test.tsx --maxWorkers=1`: 26 / 26 passed, 5.64 seconds. `pnpm --dir web typecheck`: exit 0. A002's first read-only compact recheck measured title width 836px, button width 114px and heading height 25px; the shared compact layout correctly places the action below the title. Its additional left/right-only assertion rejected this legitimate arrangement and is replaced with a non-overlap check, not a weaker title-width requirement.

Root visual review also identified nested framing in this new historical view. A single `historical-knowledge-view` class scopes two CSS selectors that remove the outer detail frame and authority-section frames, retaining the existing header/layout and individual repeated evidence cards. The ordinary catalog is unchanged. The dedicated-class regression first failed, then the complete Catalog suite passed 26 / 26 (7.05 seconds); typecheck passed again.

Final two-size read-only screenshot verification is owned by A002 and is required before claiming the historical view visually accepted; it does not require another model, execution or governance journey.

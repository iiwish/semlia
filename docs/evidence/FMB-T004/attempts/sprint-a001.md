# FMB-T004-SPRINT-A001

## Docker Volume Follow-up

- After root repaired the exact volume-root ownership, real Docker upload/finalize returned 201. The next browser failure exposed an E2E assumption: reload restores the product's default workspace rather than persisting the test-selected workspace. Both reload points explicitly reselect and assert the test workspace before checking the original artifact set ID and exact schedule ID. No global workspace persistence behavior or product architecture is added, and persisted-data assertions remain strict.

- Root completed `GOFLAGS=-p=1 make check-source`: PASS, including PostgreSQL 17/18 database tests (25.961 s), ingestion (4.066 s), Web 161 tests, generation drift and build. `make dev` successfully upgraded the existing clean migration 17 database to 18. Formal Docker live tests then found CSV upload returning 503 while readiness returned 200; PostgreSQL-to-proposal journeys passed on both desktops.
- Root inspected the new artifact volume root and confirmed UID/GID 0:0, mode 755, while the server runs as 65532:65532. The Dockerfile pre-creates the artifact root in the builder and copies it into the runtime image with `--chown=nonroot:nonroot`, matching the existing git-content pattern. USER remains nonroot, permissions are not made world-writable, and this worker did not modify the running volume or launch Docker work.
- Local-store readiness performs an exclusive random 0600 probe within the opened root, writes one non-business byte, syncs, then closes and removes the probe. Create/write/sync/close/remove failure returns the stable store error, allowing the existing deployment readiness wrapper to reject unavailable storage. S3 probing is unchanged.
- `TestStoreReadinessRejectsConfiguredUnwritableDirectory` reproduced the old incorrect nil readiness result as RED under the actual non-root test process. GREEN verifies an unwritable configured directory is rejected, restored permissions recover readiness, ten repeated checks leave zero probe files and no descriptor growth with GC disabled. Unix permissions/descriptor checks remain Unix-tagged.
- `go test -p 1 ./internal/adapters/artifacts/local ./cmd/semlia` PASS after final regression (local 1.914 s, command cached). Root owns image rebuild and the exact nonrecursive ownership repair of the newly created volume root, plus repeated real Docker upload/readiness verification. No Docker success is inferred solely from source tests.

## Native QA Follow-up

- Final handoff: complete native PostgreSQL 16 ingestion suite passes 1.362 s on root-created `semlia_sprint_ingestion_a002_test`; latest pure Go `go test -p 1 ./internal/... ./pkg/... ./cmd/... ./tests/contracts/... ./tests/repository/... ./tests/performance/...` passes. Added checks prove reuse-path expired-lease rollback leaves both discovery and Operations running, restored lease completes the same exact run, occurrence RuntimeRunID/DiscoveryRunID/JobID/terminal state agree, and actual migration-down SQL refuses reused history with SQLSTATE 55000. A malformed dollar delimiter introduced while moving the down guard was caught as 42601 and fixed without weakening assertions. An earlier incomplete test left expired running state in its dedicated database; root preserved that database and created a fresh test database for the full run. No production or QA data was cleaned.
- Root-reported final evidence: Web 18 files/161 tests PASS 27.74 s; typecheck/lint PASS with old KnowledgeViews warning; standard `make web-embed` PASS without local-UAT flag; contracts-check PASS. SDK checks compile TypeScript only and are not runtime proof. Native live E2E both desktop viewports PASS 5.4 s; root inspected screenshots and accessibility. Root stopped worker, enqueued `run_01m1qser0mfc6va76q0hrhp860`, restarted server/PostgreSQL/worker, and observed that same run succeed with unchanged revision `srv_01m1qscxkpetjad5108dkj3swh` and one candidate. Operations CUA and API showed the exact owning run/job plus queued/running/succeeded events. Docker storage later recovered; root owns the running formal `GOFLAGS=-p=1 make check-source` gate.

- Root's isolated native PostgreSQL 16 UI journey reproduced allowed `local-author` requests returning HTTP 400 in T004 artifact/source APIs. A regression using the real authorization service reproduced that exact status before the fix. Application authorization helpers consume the canonical principal from the same successful decision; grant checks and development-only alias enablement remain authoritative. Production aliases and denied grants receive 403 without artifact writes. HTTP source/run/candidate cursors also receive the canonical principal. Schedule pause/resume/delete authorize before parsing and preserve the decision's actor/version.
- `go test -p 1 ./internal/platform/http ./internal/application/ingestion ./internal/application/discovery` passes after these changes. An initial test-pointer type error was corrected before the HTTP behavioral RED was recorded.
- Native UI then reproduced a real second-run defect: manual CSV succeeded, but schedule run-now of identical input failed with SQLSTATE 23505 at `discovery_runs_success_projection_key`, exhausting job retries. Migration 18 uses internal `projection_reused`; canonical success remains unique by source revision and adapter version. Exact revision row locking serializes publication, and only committed canonical succeeded/degraded runs are reused. Every requested run still receives its own terminal state and Operations projection; physical projections and candidates are not duplicated, while findings/statistics are copied from the canonical result.
- The migration down path explicitly rejects reused history instead of deleting run evidence. The generated SQL query excludes reused runs. No new public API field is exposed.
- Root authorized a dedicated loopback database `semlia_sprint_ingestion_test`. The harness accepts an explicit `SEMLIA_INGESTION_TEST_DATABASE_URL` only for a dedicated `semlia_*_test` name, never the QA database; the default harness still starts PostgreSQL 18. No credentials enter logs/evidence. This is supplemental PostgreSQL 16 proof, not PostgreSQL 17/18 acceptance.
- Focused native regression command: `go test -p 1 ./tests/integration/ingestion -run TestArtifactDiscoveryWorkerDoesNotRequireCredentialSecret -count=1` with the explicit isolated test URL. PASS 0.622 s after fixture field-name and missing trace-ID setup corrections. It proves first manual discovery then same-input schedule run-now terminal success with one canonical run and stable candidate/dataset-revision counts; four concurrent publishers of another adapter version publish exactly one result; a failed exact revision/version is not reused, successful retry is, and the older adapter's result is not selected.
- `make db-generate` and `git diff --check` pass. Production source froze before root rebuilt a fresh UI database. Worker-owned database sessions completed and the PostgreSQL restart window was handed back to root.
- The live E2E asserts upload status 201 immediately before awaiting finalization, so an upload failure is reported at its cause instead of surfacing solely as a later 90-second wait. Finalization status and all following journey assertions remain mandatory.

## Scope And Ownership

- Attempt date: 2026-09-05; delegated T004 implementation worker in the existing shared worktree.
- Authority: confirmed Full-Menu Beta spec/TDR and `docs/specs/full-menu-beta/packets/T004.yaml`.
- The worktree contains extensive existing Alpha/FMB changes. This attempt preserves them, does not commit Git, does not edit task/spec/packet status, and does not claim acceptance.
- Root orchestrator performs independent review and owns live-stack/browser verification. No additional implementation worker is delegated by this worker.

## Implemented Surface

- `web/src/LiveSourcesView.tsx`: typed source/run/candidate pages and load-more controls, discriminated PostgreSQL/file/SQL/dbt sources, exact source version mutations, durable artifact import/registration/detail/update, schedule create/edit/pause/resume/delete/run-now, persisted occurrence links to Operations.
- `web/src/ingestionRuntime.tsx`: failed-response retries retain logical command identities and confirmed stage identities; source and schedule creation remain authoritative server commands.
- `web/src/App.tsx`: an authorized empty workspace mounts the product shell, allowing its first durable source to be created.
- `web/src/LiveSourcesView.test.tsx`, `web/src/ingestionRuntime.test.tsx`, `web/src/App.test.tsx`: pagination/version, file/SQL/error/permissions/remount, empty workspace, ambiguous retry and active-run polling regressions.
- `web/src/styles.css`: bounded dialogs, compact schedule cards and unframed occurrence lists using existing tokens.
- `web/e2e-live/source-live.spec.ts`: real empty-workspace CSV import, reload, discovery/candidates, schedule CRUD/run-now/reload and Operations link assertions at the existing two desktop targets. Inputs are CSV bytes; no API success is mocked.
- `internal/adapters/artifacts/local/{store.go,lock_unix.go,lock_windows.go,store_test.go,recovery_unix_test.go}`: close both successful digest verification handles; shared staging directory, writer locks and bounded stale-spool recovery, platform-specific lock implementations.
- `internal/domain/ingestion/model.go`, `internal/application/ingestion/service.go`: optional temporary-store recovery port integrated with artifact retention cleanup.
- `internal/adapters/discovery/files/{adapter.go,adapter_test.go}`: reject formulas in every permitted OOXML part, including table definitions and unreferenced worksheets; retain existing byte/archive/context limits.
- `internal/adapters/postgres/schedules.go`, `internal/domain/ingestion/model.go`, `cmd/semlia/{main.go,readiness_test.go}`, `tests/integration/ingestion/ingestion_test.go`: consistent workspace-before-schedule locking and explicit retryable transaction failures without terminating all worker lanes.

## Design Contract

Operational desktop refinement for semantic engineers; no landing page, new visual language, mobile scope or fabricated preview data. Existing neutral canvas `#f7f8fa`, white surface, `#dde2ea` borders, `#315fd5` command accent and existing warning/danger tokens are retained. Schedule cards use a 6 px radius, 12 px gaps, 16 px padding and a minimum 260 px grid track. Dialog width is capped at 620 px with bounded scrolling; headings are 16 px. Familiar Lucide icons, named controls, focus trapping/restoration and reduced-motion behavior carry interaction feedback. QA targets remain 1440x900 and 1024x768. Browser screenshots are a separate gate, not inferred from component tests.

## RED Evidence

- Initial `pnpm --dir web typecheck`: exit 2, missing pagination/discriminator/version adaptation and stale ingestion test capability/type contracts.
- Initial updated LiveSources tests: old array consumer rejects typed pages; persistent scheduling, load-more and expected-version assertions fail.
- `go test -p 1 ./internal/adapters/artifacts/local -run TestStoreDedupeClosesVerifiedFiles -count=1`: exit 1, verified-file handles increase from 7 to 27 on replay and 28 to 48 on the publication-race path. The first FD test harness used `os.ReadDir` on macOS `/dev/fd`; corrected to `Readdirnames` before recording this behavioral failure.
- `go test -p 1 ./internal/adapters/discovery/files -run TestXLSXRejectsFormulasOutsideReferencedCells -count=1`: exit 1, calculated-column, totals-row and unreferenced-sheet formulas were each accepted with nil error.
- `go test -p 1 ./tests/integration/ingestion -run TestDueScheduleAcquiresWorkspaceBeforeScheduleRow -count=1`: exit 1, workspace holder cannot acquire schedule row (`55P03`). An earlier test setup omitted required `scheduled_for`; that constraint failure was corrected before the lock-order reproduction.
- `go test -p 1 ./cmd/semlia -run TestScheduleLoopSurvivesTransientDatabaseFailure -count=1`: exit 1, scheduler exits after the first retryable error (`calls=1`).
- Empty workspace App regression: exit 1, catalog entry is rendered instead of the authorized shell.
- Stale-spool recovery regression: initially missing recovery API (build RED); follow-up caught directory cursor EOF handling before GREEN.
- Ambiguous create retry regression: stage invoked twice; polling regression: old background reload does not request known run details and resets the source window.

## Verified Results

- Focused source/runtime baseline: 14 tests passed after typed UI wiring.
- `go test -p 1 ./internal/adapters/artifacts/local ./internal/application/ingestion ./internal/adapters/discovery/files ./cmd/semlia`: passed after storage/formula/retry changes.
- Lock-order PostgreSQL barrier test: passed in 58.414 seconds after the fix.
- `pnpm --dir web typecheck`: passed after the first complete UI wiring.
- `pnpm --dir web lint`: exit 0; one pre-existing `KnowledgeViews.tsx` fast-refresh warning, no errors.
- `make contracts-check`: passed.
- `make db-generate-check`: passed.
- `git diff --check`: passed.
- `GOOS=windows GOARCH=amd64 go build ./internal/adapters/artifacts/local`: passed after platform-specific locks.

## Environment And Open Gates

- The first full Web run accidentally used `pnpm test -- <filter>`, which Vitest treated as the full suite. Under heavy shared-machine load it reported 24 failures, mostly timeouts plus the intended T004 RED cases. This is not passing evidence.
- A subsequent full single-worker Web run reported 154 passed and 4 failed: three 15-second ProductApp timeouts and one AuditRuntime loading timeout. No timeout or assertion was relaxed. Final sequential rerun results are pending below.
- The orchestrator reported Docker/OrbStack becoming fully unavailable while rebuilding the live stack. Further PostgreSQL/Compose retries are paused by explicit coordination; final live journeys, migration 18 round-trip, browser visual/accessibility evidence and `make check-source` are not claimed as passed.
- The confirmed lock-order test ran against real PostgreSQL before the Docker outage. Root-owned prior T004 ingestion/discovery/worker suite results remain separate evidence.
- Hosted shared S3, real OIDC/provider environments, crash/restart human journeys and release acceptance are not inferred from local unit tests.
- The dedicated staging scanner covers current `.staging/.artifact-*` writes. Unreleased pre-attempt temporary files created beside digest objects are not assigned a guessed owner or automatically removed by this scanner.

## Final Sequential Rerun

- Complete Web suite: 18 files / 160 tests passed in 81.62 seconds, without test timeout changes.
- Final focused App/LiveSources/ingestion-runtime suite: 28 tests passed in 6.51 seconds, including terminal-candidate refresh and focus preservation after active-run polling. The candidate refresh merges persisted facts and independently fences stale candidate append requests.
- Latest typecheck passed after preserving source-run list metadata while merging discovery details and accounting for DOM/Node timer-spy type overlap.
- Lint exit 0. A newly observed ref-cleanup warning was removed with a stable invalidation callback; the final warning-only lint recheck is held with the remaining gates.
- Unix-specific recovery/FD tests are isolated behind a Unix build tag. Windows artifact-store test binary cross-compilation passed; Windows runtime filesystem behavior is not claimed as executed.
- All selected pure Go packages passed: `go test -p 1 ./internal/... ./pkg/... ./cmd/... ./tests/contracts/... ./tests/repository/... ./tests/performance/...`.
- Small ContextPanel text correction removes obsolete file/automation Prototype claims from production navigation; it introduces no behavior change.
- Root's final live-stack build failed with `no space left on device`; host Data volume is 100% with approximately 394 MiB free. The worker starts no further compiler/container work. All worker-owned sessions have completed.
- Live journeys, latest screenshots, full `make check-source`, final migration round-trip and the complete delivery diff snapshot remain coordinated follow-up gates, not passing assertions. An old image listening on port 18081 is explicitly excluded from latest-UI evidence.
- Retryable handling applies to known PG errors preserved at the schedule repository boundary. Unknown enqueue failures already redacted by a different repository boundary are not covered by that claim.

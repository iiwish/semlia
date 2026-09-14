# Full-Menu Beta Three-Hour Sprint

Status: Completed within the three-hour budget. Full-Menu Beta remains incomplete.

## Execution Contract

- Authorized by the founder on 2026-09-05 in the current Codex task.
- Start: 2026-09-05 10:02:01 Asia/Shanghai (02:02:01 UTC).
- Deadline: 2026-09-05 13:02:01 Asia/Shanghai (05:02:01 UTC).
- Reserve the final 25 minutes for integrated validation and an evidence-based handoff.
- Execute the confirmed FMB work graph in order, starting with T004; do not expand M4/M5 scope.
- Preserve all pre-existing work. No automatic commit, push, release acceptance or deployment to an unknown external host.
- Missing external infrastructure is an evidence gap, never permission to simulate a successful hosted run.
- Completion of this timeboxed sprint does not imply completion or acceptance of Full-Menu Beta.

## Starting State

- HEAD: `63241d5`; local tracking comparison reports 25 commits ahead of `origin/main`.
- Worktree: 101 tracked changed files and 126 untracked entries, including prior Alpha/FMB work.
- FMB-T001/T002/T003: Needs_Review. T004: Running. T005/T006/T007: Draft.
- Prior read-only verification found Go internal/contract/repository tests and focused PostgreSQL
  integration tests passing, Web typecheck failing, and Web tests at 149 passed / 1 failed.
- The current implementation owner is the T004 worker. Shared contracts and composition are serialized.

## Results

T004 and T005 are Needs_Review after passing local implementation and review gates. T005 runs from
11:43 through its final source correction at 12:32 Asia/Shanghai. T006/T007 remain Draft and are not
started during the final integration window. Each task retains its own evidence; no task or release
is marked Accepted by the sprint.

| Area | Final Result |
| --- | --- |
| File, SQL/dbt ingestion and scheduling | T004 implementation and real Docker desktop journeys pass. |
| Persistent embedding rebuild and released search | T005 implementation, actual pgvector storage, cancellation, checkpoint reuse, activation and local deterministic browser journeys pass. |
| Source gate | All Go packages, PostgreSQL 17/18 migrations, 170 Web tests, contract/sqlc/Web drift and binary build pass. |
| Browser gates | 26 fixture cases, 4 real Docker source cases and 2 real pgvector/deterministic-provider cases pass across the two supported desktops. |
| Smoke and security | Live Docker smoke passes; dependency/secret and final local-container scans pass with no HIGH/CRITICAL finding. |
| Release readiness | Machine interfaces, read-only query execution, external provider/hosted validation, recall quality and scale evidence remain open. |

### Baseline Validation

- `go test -count=1 -timeout=8m ./internal/... ./pkg/... ./cmd/... ./tests/integration/...
  ./tests/contracts/... ./tests/repository/... ./tests/performance/...`: failed. Internal,
  command, contract, repository and performance packages passed; integration packages timed out
  starting their PostgreSQL containers before domain assertions could run.
- The host runs other projects and OrbStack has 16 GB available. Database-heavy rechecks use
  `go test -p 1` to avoid the observed simultaneous container-start contention; assertions and
  test coverage are unchanged. Other projects' containers are not stopped.
- `go test -p 1 -count=1 -timeout=8m ./tests/integration/ingestion/...
  ./tests/integration/discovery/... ./tests/integration/db/... ./tests/integration/worker/...`:
  ingestion (43.238 s), discovery (52.316 s), and worker (66.531 s) passed. The database package
  failed with connection loss after its PostgreSQL 17 lifecycle test replaced an unhealthy
  Testcontainers reaper. PostgreSQL 18 then returned EOF/connection-refused errors. This is not
  recorded as a passing migration gate; focused migration revalidation remains required.
- Backend review identified four T004 fixes: replayed artifact file handles, formulas in every
  allowed XLSX worksheet/table part, workspace/schedule lock-order inversion, and orphaned local
  staging files after process loss. The implementation worker owns fixes and regression evidence.
- T005, T006 and T007 execution packets are Draft. Their creation does not activate implementation
  or satisfy dependency review gates.
- The first `make dev` run built the local Web bundle successfully, then failed during the Docker
  build with an RPC EOF. At 10:28 Asia/Shanghai the Docker socket and port 18081 were unavailable
  and `orb status` reported `Stopped`. No container-stop or runtime-stop command was issued by this
  sprint. Pure source verification continues independently; this failed build is not browser proof.
- `orb start` restored Docker without resetting containers or volumes. A second build exposed
  two TypeScript errors in the new polling implementation/test, which the worker corrected.
- The third `make dev` passed local and container Web compilation, then failed compiling the S3
  adapter with `no space left on device`. At 10:40, the host Data volume had 394 MiB available.
  Docker reported 11.49 GB of reclaimable build cache. The founder was asked to release disk
  space or authorize build-cache-only cleanup; no image, container, volume or project deletion
  is authorized by that question. Port 18081 still serves the earlier image, not the corrected UI.
- T004 worker reports the sequential Web suite passing (18 files, 160 tests), followed by the
  final focused source/runtime/App suite (28 tests), typecheck and the complete pure Go
  internal/pkg/cmd/contracts/repository/performance suite. Their exact commands and scope are
  recorded in `docs/evidence/FMB-T004/`; migration, live browser and full source gates remain open.
- At 10:52 the host had 2.4 GiB free, allowing one bounded rebuild attempt. Native and container
  compilation completed, but Docker image extraction still failed. Inspecting the existing
  Semlia PostgreSQL container showed the Docker filesystem at 97.1/97.3 GB used with 133.7 MB
  free. No further Docker build or database-container retries are started against that disk.
- At 11:27, a read-only disk check shows 37.1 GB available on a 134.2 GB Docker filesystem.
  The sprint did not prune images, caches or volumes. Formal serial source/database gates resume
  under `GOFLAGS=-p=1 make check-source`. The existing development database is clean at migration
  17, so migration 18 can be tested without resetting its data or manually modifying its schema.

### Isolated Native UI Verification

- An isolated password-authenticated PostgreSQL 16.15 instance uses loopback port 55439 and the
  ignored `.semlia/sprint-native/postgres` data directory. It does not reuse other database data.
- The current source builds natively with the explicitly local-UAT Web flag. Migrations through
  18 apply, and native server/worker use the same local artifact root. Readiness passes on
  `http://127.0.0.1:18082`.
- This environment is supplementary live UI evidence only. PostgreSQL 16 is not substituted for
  the required PostgreSQL 17/18 migration, Testcontainers, Docker or hosted release gates.
- The first real browser run exposes a request-path defect: source lists and artifact list/upload
  return `400 INVALID_ARGUMENT` while catalog and governance requests succeed for `local-author`.
  The T004 worker is fixing the local-UAT/canonical-principal boundary with regression coverage.
- After canonical-principal correction, CSV import, artifact detail, refresh and the first discovery
  succeed on both desktops. The run-now journey fails on a second identical discovery: PostgreSQL
  reports `discovery_runs_success_projection_key`, and the run ends `DISCOVERY_FAILED`. The worker
  is separating per-attempt run history from reused canonical projection, retaining at-most-once
  candidate writes. Both failures remain real RED evidence, not infrastructure classifications.
- Manual UI verification in workspace `wsp_01m1qrj86xfph98379x7bxtxgj` registers configured-root
  SQL and uploads supported dbt manifest/catalog fixtures. SQL run
  `run_01m1qrknkafpnsgvswc5agetk4` completes with one dataset and three fields; dbt run
  `run_01m1qrr5dkfps89ys4f82vbmty` completes with two datasets, two fields and one lineage edge.
  Hard reload preserves both run IDs and three candidates. These fixtures contain only synthetic
  orders metadata and use the native PostgreSQL 16 environment described above.
- The corrected projection code builds into the native binary and migration 18 applies to fresh
  `semlia_sprint_ui_a002`; the earlier RED database is preserved without manual schema patching.
  The real CSV/schedule Playwright journey passes at both desktop sizes (2 tests, 5.4 s), including
  upload/finalize, reload, candidate creation, schedule CRUD, run-now, owning-link target and serious/
  critical accessibility checks. Both schedule screenshots were visually inspected without overlap.
- Controlled recovery preserves queued run `run_01m1qser0mfc6va76q0hrhp860`: the worker is stopped,
  the UI submits the run and displays queued state, then the server and isolated PostgreSQL are
  stopped and restarted before the worker resumes. Hard reload shows the same run completed with
  source revision `srv_01m1qscxkpetjad5108dkj3swh`; the workspace still has one candidate. This is
  graceful restart evidence, not a claim of an in-flight SIGKILL or hosted failover test.
- Independent read-only review finds no confirmed new P1/P2 blocker in canonical-principal
  resolution or projection reuse. Focused missing assertions for lease-loss rollback, exact
  Operations terminal identity and rollback refusal are assigned to the implementation worker.
- Root Web typecheck and lint pass, with one existing KnowledgeViews fast-refresh warning.
  TypeScript SDK `test` and `typecheck` both run `tsc --noEmit`; they prove compilation contracts,
  not runtime transport parity.
- Root's final sequential Web suite passes 18 files / 161 tests in 27.74 s. Standard `make web-embed`
  restores production-default embedded assets; only the separate native QA binary retains the
  explicit local-UAT build. Contract drift, database generation drift and formatting checks pass.
- A real Operations deep link for the recovered run displays its exact run/job identity, success
  and persisted queued/running/terminal events. The Operations API independently reports the same
  run and source identity; this is not an assertion based only on a rendered link target.

### Formal Stack Verification

- `GOFLAGS=-p=1 make check-source` passes every source gate: format, lint, typecheck, all Go
  packages, 161 Web tests, contract/sqlc/embedded-Web drift and the binary build. PostgreSQL 17/18
  migration tests pass (database package 25.961 s), including populated migration 17-to-18-to-17-to-18;
  PostgreSQL 18 ingestion passes in 4.066 s. Conditional hosted/smoke checks are not implied.
- `make dev` builds and starts the corrected Docker image and upgrades the existing clean
  migration-17 development database without data reset. Its first full source browser run passes
  both PostgreSQL-to-proposal journeys but fails both CSV uploads with HTTP 503. This is a real
  deployment defect: the newly created artifact volume is owned by 0:0 with mode 755, while the
  image's nonroot identity is 65532:65532. Local storage readiness only checks directory existence
  and incorrectly returns ready. The worker owns image initialization and writable-probe fixes.
- Root verifies the running image's nonroot UID/GID and changes only the new artifact volume's
  root directory owner to 65532:65532. No recursive permission change, chmod 777, volume deletion,
  application-root execution or other-project change is used. A fresh-volume image proof remains
  required after the Dockerfile fix; fixing this existing volume alone is not that proof.
- The final image initializes a fresh isolated artifact volume as 65532:65532 / 755 without a
  permission repair, and UID 65532 writes a synthetic probe. Only this sprint-created proof
  container and volume are removed after verification. The real Docker source suite then passes
  all four desktop cases in 10.1 s. The final complete source-gate repeat also passes, including
  161 Web tests and fresh PostgreSQL 18 ingestion (3.666 s).
- T004 enters Needs_Review at 11:43 with a cumulative source snapshot and independent review;
  it is not Accepted. T005 starts only after shared-file ownership returns. The isolated native
  server/worker/PostgreSQL are stopped while their evidence data is preserved. Docker development
  remains available on port 18081.

## External Inputs

Hosted TLS/OIDC, live model and Embedding configuration have been requested without requesting
plaintext secrets. No hosted acceptance is claimed at sprint start.

## T005 Review And Validation

- The implementation adds migration 19, immutable released-summary generations, pinned configuration,
  durable vector checkpoints, atomic activation, explicit cancellation and authorized active-index
  search. The deterministic resolver is not replaced by similarity scoring.
- Independent review and RED/GREEN tests close credential lookup and JSON-null validation issues,
  an active-generation query race, a workspace/job FK lock cycle, slow polling starvation,
  StrictMode abort handling, identity-scoped UI state, modal keyboard behavior and dimension-form
  bounds. The data-model document matches the implemented JSON configuration and ordinal keys.
- A real isolated PostgreSQL 17/pgvector database and a clearly synthetic HTTP embedding provider
  verify the entire transport-to-storage-to-search path. They do not establish external model
  compatibility or semantic relevance. The existing PostgreSQL 18 development volume is not
  replaced or downgraded.
- The first final gates expose a real lint error and migration-test assumptions about version 18.
  The corrected gates preserve all assertions and pass. Full failure and fix details remain under
  `docs/evidence/FMB-T005/`, including the actual failed fixture-import attempts.
- Final source validation passes 20 Web files / 170 tests. Real embedding E2E passes 2 cases in
  13.1 s; the complete fixture suite passes 26 cases in 31.9 s; Docker source regression passes
  4 cases in 10.7 s; live smoke passes in 18.714 s.
- The final local image is `sha256:4e5917579a7a4edd2e3a797d1b77b7afbb6d39632b07e0e516a36d8790788ffa`.
  Its security scan and the dependency/secret scan pass. This is a local-UAT image, not a hosted
  production authentication or release-acceptance proof.

## Handoff

- The latest development stack remains usable at `http://127.0.0.1:18081`; PostgreSQL is clean at
  migration 19 and readiness passes. Without a configured pgvector/model deployment, embedding
  rebuild reports unavailable rather than fabricating progress.
- The isolated native QA server, worker and deterministic provider are stopped. The separate
  `semlia-sprint-embedding-qa-20260905` container is stopped with its database volume preserved.
  The earlier native PostgreSQL 16 evidence data is also preserved and stopped.
- No commit, push, user-data deletion, Docker prune or change to another project's containers is
  performed. The worktree contains 103 tracked changed entries and 145 untracked entries at final
  audit, including substantial pre-existing work; these counts are not sprint-authored file counts.
- T004 and T005 have separate frozen cumulative source snapshots. They include pre-existing work
  under each packet's allowed paths and must not be presented as isolated task diffs.
- Next implementation packet: T006 machine credentials, REST/MCP/CLI/SDK parity and signed webhook
  delivery. T007 follows the dependency gate for bounded read-only execution and hosted acceptance.
- T005 release acceptance still needs a real provider run, recall-quality baseline, 5,000-chunk scale
  evaluation and stronger in-flight crash proof. Hosted TLS/OIDC, external receiver and source
  evidence must use real supplied configuration; no stub is substituted for them.

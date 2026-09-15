# FMB-T004 Test Results

Date: 2026-09-05. Attempt: FMB-T004-SPRINT-A001.

| Command / Check | Result |
| --- | --- |
| `pnpm --dir web exec vitest run --maxWorkers=1` | Latest root-run PASS: 18 files, 161 tests, 27.74 s; earlier 160-test pass and heavy-load failures retained in attempt |
| `pnpm --dir web exec vitest run src/LiveSourcesView.test.tsx src/ingestionRuntime.test.tsx src/App.test.tsx --maxWorkers=1` | PASS: 3 files, 28 tests, 6.51 s; terminal-candidate, pagination/focus, retry and empty-workspace regressions included |
| `pnpm --dir web typecheck` | PASS after run-list metadata preservation and browser/Node timer-spy typing corrections |
| `pnpm --dir web lint` | Root final rerun PASS; existing KnowledgeViews fast-refresh warning only |
| `go test -p 1 ./internal/adapters/artifacts/local ./internal/application/ingestion ./internal/adapters/discovery/files ./cmd/semlia` | PASS |
| `go test -p 1 ./internal/adapters/artifacts/local ./cmd/semlia` after Docker ownership/readiness defect | PASS: local 1.914 s; non-root unwritable directory RED then GREEN, repeated successful probes leave zero files/descriptors; Docker image/live revalidation owned by root |
| `go test -p 1 ./tests/integration/ingestion -run TestDueScheduleAcquiresWorkspaceBeforeScheduleRow -count=1` | PASS: real PostgreSQL, 58.414 s; preceding RED reproduced `55P03` |
| `make contracts-check` | PASS |
| `make db-generate-check` | PASS |
| `git diff --check` | PASS |
| `go test -p 1 ./internal/platform/http ./internal/application/ingestion ./internal/application/discovery` | PASS after canonical-authority fix: real authorization service local-UAT HTTP upload/list/source/run/candidate paths, production alias denial, denied writes, and schedule pause/resume/delete actor regressions |
| Explicit isolated native database: `go test -p 1 ./tests/integration/ingestion -run TestArtifactDiscoveryWorkerDoesNotRequireCredentialSecret -count=1` | PASS, PostgreSQL 16 only: manual CSV then same-input schedule run-now reuses canonical projection without duplicate candidate/dataset revisions; four concurrent publishers of another adapter version create one canonical result; failed publication is not reused, successful retry is reusable. Final focused run 0.622 s |
| Explicit isolated native database: `go test -p 1 ./tests/integration/ingestion -count=1` | PASS 1.362 s on PostgreSQL 16 in `semlia_sprint_ingestion_a002_test`; includes final lease-loss atomic rollback/retry, exact Operations run/job/terminal mapping, and migration-down SQLSTATE 55000 reuse-history rejection |
| `make db-generate` after projection reuse migration | PASS; internal `projection_reused` column and canonical-success query generated without exposing a new public API field |
| `GOOS=windows GOARCH=amd64 go build ./internal/adapters/artifacts/local` | PASS |
| `GOOS=windows GOARCH=amd64 go test -c -o /tmp/semlia-local-artifacts.test.exe ./internal/adapters/artifacts/local` | PASS; Windows binary compiled, not executed on macOS |
| `go test -p 1 ./internal/... ./pkg/... ./cmd/... ./tests/contracts/... ./tests/repository/... ./tests/performance/...` | PASS: all selected packages, including local/S3 artifact stores, discovery, ingestion, HTTP, configuration and repository contracts |
| Migration 18 populated up/down/up | PASS in the formal PostgreSQL 17/18 database suite (25.961 s), including populated version-17 source/run preservation; no data or assertion relaxed |
| Live CSV import/reload/discovery/candidates/schedule lifecycle | Root-run native PostgreSQL 16 PASS: both desktops, 5.4 s, real CSV/artifact/discovery/candidate/schedule CRUD/run-now/owning link/deletion |
| Desktop screenshots and accessibility | Root reports both desktop screenshots inspected without overlap; journey accessibility assertions PASS |
| Native worker/server/database restart | Root PASS: queued run `run_01m1qser0mfc6va76q0hrhp860` survives worker stop and server/PostgreSQL restart, then succeeds with unchanged revision `srv_01m1qscxkpetjad5108dkj3swh` and exactly one candidate; Operations CUA/API show exact run/job and queued-running-succeeded events |
| `make web-embed` | Root PASS; standard embedded assets regenerated without local-UAT flag |
| SDK tests | Root reports TypeScript compilation contract (`tsc --noEmit`) PASS; not an SDK runtime test |
| `GOFLAGS=-p=1 make check-source` | Final repeat PASS after writable-readiness/image fix: all gates, Web 161 (11.24 s), PostgreSQL 18 ingestion 3.666 s, drift/build. Prior formal run also passes PostgreSQL 17/18 database tests 25.961 s and ingestion 4.066 s |
| `make dev` | PASS; existing clean migration-17 development data upgrades to 18 without reset |
| Docker live source journeys on port 18081 | PASS: 4 tests, 10.1 s, both PostgreSQL-to-proposal and CSV/schedule workflows on both desktops, using current image and actual shared artifact storage |
| Fresh named artifact volume | PASS: current image initializes a new isolated volume as 65532:65532 / 755; UID 65532 writes a synthetic probe without chown. Only this disposable proof container/volume is cleaned afterward |

## Failure Classification

The first corrected Docker image exposed a deployment defect absent from native execution: an empty named artifact volume was root-owned, so uploads returned 503 while readiness incorrectly passed. Image initialization and the local writable readiness probe received focused RED/GREEN fixes and independent review. Root repaired only the newly created development volume's root ownership, then separately proved correct initialization using a fresh volume without a permission repair. A subsequent E2E timeout came from assuming hard reload preserves workspace selection; the test restores and asserts its workspace before checking the same artifact/schedule IDs. It does not replace those records or weaken persistence assertions.

Earlier type errors, missing persistent UI assertions, descriptor leaks, accepted hidden formulas, inverted schedule lock order and lost-response duplicate requests were treated as code defects and received fixes plus regression tests. An initial spool cursor bug and invalid test setup were corrected and separately recorded in the attempt.

Heavy-load Web timeouts were not ignored: a fresh sequential full run passed without increasing test timeouts. The Docker/OrbStack outage and nested testcontainers lifecycle failures are environment evidence, not product success. No live backend is replaced by a fixture to close a gate.

The Docker build reported `no space left on device`; root later observed sufficient free Docker storage and resumed the formal source gate. A separately authenticated loopback PostgreSQL 16 runtime provides supplemental native validation, not a substitute for PostgreSQL 17/18 container/release gates. Test harness opt-in requires `SEMLIA_INGESTION_TEST_DATABASE_URL` with a dedicated `semlia_*_test` database name; the default still provisions PostgreSQL 18. Credentials are not recorded in evidence. Worker-owned database commands are finished.

Final added assertions caught a malformed dollar-quoted migration-down guard (SQLSTATE 42601) introduced while moving the guard. The delimiter was corrected, the strict expected SQLSTATE 55000/reuse-history message assertion was retained, and the complete native suite passed. An earlier aborted lease-loss test left an expired running job in its dedicated test database; that failed state was preserved and root supplied a new test database rather than cleaning shared data. The lease test verifies a non-nil repository error and atomic rollback because the existing repository error boundary does not retain the exported lease-error type.

Native UI RED exposed two actual defects: an allowed local-UAT principal alias was reparsed instead of consuming the authorization decision's canonical ID, and repeated same-input discovery conflicted with the successful-projection uniqueness index. Both received production fixes and regression coverage. The projection index still uniquely constrains canonical successful publication; a row lock serializes publication for the exact source revision, reuse additionally matches adapter version, only successful/degraded canonical results qualify, and rollback refuses to erase reused run history.

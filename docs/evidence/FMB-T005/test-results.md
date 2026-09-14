# FMB-T005 Test Results

All timestamps are 2026-09-05 Asia/Shanghai. No external provider is used.

## RED Evidence

- Domain/vector tests precede the new package implementation and fail on undefined symbols.
- Initial Web runtime test fails because `embeddingRuntime` is absent.
- Provider regression accepts JSON `null` as numeric zero and resolves the credential twice before correction; both assertions fail against that implementation.
- Optional-schema integration fails with `metadata migration must not silently enable pgvector` at 12:08. Migration 19 requires explicit extension enablement.
- Real PostgreSQL job-lock barriers for claim/cancellation and fail/cancellation both produce SQLSTATE `55P03` when the job holder needs a workspace FK lock. Changing only embedding workspace locking to `FOR NO KEY UPDATE` closes the cycle.
- Modal Tab leaves the dialog and the 3,000-ms polling response never reaches terminal state at 12:12. Focus containment and single-flight polling close these failures.
- StrictMode's aborted first request leaves the panel loading at 12:17. Request-token cleanup, an asynchronous initialization boundary and abort checks close this failure.
- Local-UAT author capability lacks `workspace.manage` at 12:15 while the server maps that alias to its existing workspace-admin role. Only that already-granted development capability is added; reviewer/publisher and ordinary-build header boundaries remain tested.
- An obsolete ProductApp fixture test expects browser-generated `42%` progress and fails. Its replacement asserts no fabricated progress or runnable rebuild without the real API; persisted execution is tested separately against real PostgreSQL and HTTP.
- Initial provider timeout-test cleanup waits indefinitely for an unread request context. That test process is terminated; the fixture is bounded to 100 ms against a 20-ms client timeout. This is a test-harness correction, not a relaxed production timeout.
- Full lint initially rejects the effect's direct load invocation. An explicit microtask initialization fixes it without disabling lint.

## Completed Commands

| Command | Actual Outcome |
| --- | --- |
| `go test -p 1 ./internal/domain/embedding/... ./internal/adapters/embedding/... ./internal/application/embedding/...` and focused package runs | Domain/provider pass; application has no separate unit-test file and is exercised by the integrated service/worker suite. |
| `go test -p 1 ./tests/integration/embedding ./internal/platform/http ./cmd/semlia -count=1` | PASS: embedding 3.592 s, HTTP 0.469 s, command 0.668 s. |
| `go test -p 1 ./internal/adapters/embedding ./tests/integration/embedding -count=1` | Latest expanded suite PASS: provider 0.612 s, real pgvector integration 4.065 s. |
| `pnpm --dir web exec vitest run --maxWorkers=1` | PASS at 12:16: 19 files, 166 tests, 24.49 s. This precedes the additional StrictMode test. |
| `pnpm --dir web exec vitest run src/embeddingRuntime.test.tsx --maxWorkers=1` | Latest PASS at 12:19: 5 tests, 748 ms, including StrictMode, slow polling and focus containment. |
| `pnpm --dir web typecheck` | PASS before the final asynchronous initialization change; final whole-source gate is orchestrator-owned. |
| `pnpm --dir web exec eslint src/embeddingRuntime.tsx` | PASS at 12:19 with two ref-cleanup warnings and no rule suppression. |
| `make contracts` / `make db-generate` | PASS generation; final drift checks are orchestrator-owned. |
| `git diff --check` | PASS at 12:20. |
| `pnpm --dir web exec vitest run src/ModelConfigurationView.test.tsx --maxWorkers=1` | Dimension regression RED at 12:32: 2 incorrectly invalid and 4097 incorrectly valid; 4096 valid. GREEN after matching the native input range to 1..4096: 3 tests, 794 ms. Valid dimensions save through the form; 4097 remains rejected. Focused ESLint also passes. |
| `pnpm --dir web exec playwright test --config playwright.config.ts --project compact-desktop --workers=1 --grep 'system settings separates LLM inference from Embedding indexing' --timeout=10000` | RED at 12:26: obsolete fixture journey waits for the correctly disabled rebuild button. GREEN after replacing invented progress assertions: 1 test, 2.1 s. The Vite fixture environment has no real embedding API; the test requires the visible unavailable-state error, disabled rebuild, zero fabricated progress/run links and zero embedding writes. No successful backend response is mocked. |

## Real pgvector Assertions

The isolated `pgvector/pgvector:pg17` Testcontainer proves metadata migration 18 state survives 19 up/down/up; missing extension and extension-only/missing-column states report unavailable; the explicit administrator enable script makes vector storage usable; rebuild captures the exact released revision; incomplete and wrong-dimension batches do not activate; restart reuses persisted checkpoints; a lost lease cannot activate; exact config/content reuses vectors while a model change does not; a real HTTP batch is checkpointed and activated by the job worker; three provider failures terminate the generation without changing the active index; cancellation retains the active index; retired-generation query vectors cannot search a new active generation; per-asset denial filters results; authorization-version changes reject the response; a newly published release excludes old revision results and invokes lexical fallback; and populated index history rejects migration down.

The concurrency regression holds the same job-row lock used by claim/fail, waits until cancellation blocks on it, and requests the workspace `KEY SHARE` lock required by runtime-event foreign keys. It is a deterministic database lock-compatibility barrier, not an end-to-end process scheduling benchmark.

## Final Orchestrator Gates

| Command | Actual Outcome |
| --- | --- |
| `GOFLAGS=-p=1 make check-source` | Final PASS after the dimension-form fix: all Go packages, PostgreSQL 17/18 lifecycle, 20 Web files / 170 tests, contract/sqlc/embedded-Web drift and binary build. |
| Explicit isolated `embedding-live.spec.ts` | PASS on both desktops: 2 tests, 13.1 s; real API/persistence and deterministic provider transport. |
| `SEMLIA_E2E_PORT=4176 pnpm --dir web exec playwright test --workers=1` | PASS: 26 fixture/transport-mock browser tests, 31.9 s. |
| Docker `source-live.spec.ts` at migration 19 | PASS: 4 desktop cases, 10.7 s; no fixture fallback. |
| `make smoke` | PASS against Docker development: 18.714 s. |
| `make security-check` and final-image script repeat | PASS: dependency/secret scan and current local Docker image scan, no HIGH/CRITICAL finding. |
| `make dev` / `make web-embed-check` | PASS; latest development image runs at port 18081 and standard embedded assets have no local-UAT flag. |

Two embedding ref-cleanup warnings and one existing KnowledgeViews fast-refresh warning remain; no lint error or disabled rule is hidden.

## Remaining Acceptance

Live external-model execution, hosted deployment, semantic-quality evaluation, 5,000-chunk scale and in-flight SIGKILL recovery remain unverified. The passing local gates do not substitute for those release-acceptance artifacts.

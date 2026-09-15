# FMB-T003 Delivery Evidence

Status: Needs_Review candidate  
Final packet: FMB-T003-A003  
Date: 2026-09-05

## Outcome

Migration `000017` establishes the shared operational projection: runtime settings, durable runtime
runs and append-only events, bounded audit exports, indexed audit targets and the storage contract
for future Workbench attention items. Existing discovery, validation, Agent and semantic-resolution
runs are backfilled and future owning writes project in the same transaction.

The Operations API and Web surface provide independently authorized Audit and Runtime lists/details,
stable filter-bound cursor pagination, immutable redacted export download, runtime settings with
optimistic versioning and honest deployment status. Retry/cancel capabilities remain false and the
server returns `RUN_ACTION_UNSUPPORTED`; no generic job mutation is advertised.

## Authority And Privacy

- Audit responses use explicit allowlisted fields and the immutable `audit_event_targets` projection;
  raw payloads never reach the API or export.
- Export content is a persisted bounded snapshot. The download digest and response headers are
  verified before the Web enables download.
- Runtime DTOs use owning-domain identity/state and safe structured events. Discovery terminal state
  is projected atomically and a degraded run cannot be overwritten by generic job success.
- DSNs, cookies, Bearer/provider/signing secrets, prompts and job/outbox payloads are excluded or
  redacted.
- Workspace runtime settings contain only retry ceiling, statement/webhook timeout, query row/byte
  limits and run-metadata retention. Worker concurrency, audit retention, queue policy, OIDC,
  encryption and telemetry destinations remain deployment-owned.

## Scope And Preservation

The worktree already contained reviewed FMB-T001 and approved Alpha/M3 changes. T003 added or
updated only its migration, Operations projection/API/Web surface, owning-write projection hooks,
composition/readiness, focused tests, generated contracts and synchronized embedded Web assets.
Unrelated `.omo/` and other user work were preserved.

Primary task-owned files:

- `migrations/000017_fmb_operations_runtime.{up,down}.sql`
- `db/queries/audit.sql`, `db/queries/operations.sql`, `db/queries/jobs.sql`, `db/sqlc.yaml`
- `internal/domain/operations/**`, `internal/application/operations/**`
- `internal/adapters/postgres/operations.go` and focused owning-write projection hooks
- `internal/application/jobs/**`, `internal/platform/config/**`
- `internal/platform/http/operations.go` and focused routing/tests
- `cmd/semlia/main.go`, readiness and `compose.yaml`
- OpenAPI, generated Go/TypeScript contracts and client exports
- `web/src/AuditRuntimeView.tsx`, `operations.ts`, `operationsRuntime.tsx`, ProductApp/styles/tests
- focused Operations, worker, discovery and migration integration tests
- synchronized `internal/platform/web/static/**` and this evidence directory

## Review

Three review/fix cycles closed export-content, historical backfill, terminal atomicity, audit-target,
workspace-FK, idempotency, deployment-status, cursor, server-command-truth, E2E and typography
counterexamples. The final independent review reported no P0, P1 or P2 findings and approved
transition to `Needs_Review`.

## Visual Proof

- `screenshots/desktop-audit-runtime.png` at 1440x900
- `screenshots/compact-desktop-audit-runtime.png` at 1024x768

These screenshots use the explicitly disclosed fixture build only for stable visual data. The same
component's real runtime path, authorization, error handling and no-fallback behavior are covered by
typed runtime/HTTP tests. Computed visual checks found no horizontal overflow and no visible
Operations text below 10px.

## Residual Risk

- A live OIDC tenant and hosted telemetry exporter are external Beta acceptance inputs. Telemetry is
  reported as not configured rather than simulated.
- The production bundle remains above Vite's 500 kB warning threshold; final performance/release
  work must either split the bundle or explicitly accept measured load evidence.


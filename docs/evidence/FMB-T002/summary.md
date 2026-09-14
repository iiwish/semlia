# FMB-T002 Delivery Evidence

Status: Needs_Review evidence  
Attempt: FMB-T002-A001  
Date: 2026-09-05

## Outcome

The Workbench, catalog asset detail and release detail are backed by persisted, server-authorized
records. Workbench list, detail and mutation paths use stable `ati_*` identities, cursor-bound
authorization snapshots, persisted idempotency receipts and deterministic owning-condition
projections. Startup reconciliation is bounded and restart-safe; owner writes also update or close
attention items without waiting for a restart. GET requests do not create or mutate attention data.

Catalog reads expose explicit authority and availability per section, typed owning facts and
cursor-paged section records. Asset, revision and release collections include server totals. Release
detail keeps immutable manifest pins separate from the current registry and from the nearest prior
pin, and sensitive governed-object, impact and diff sections require `binding.read`.

## Dependency And Ownership Preflight

- FMB-T001 A003 supplied the reviewed authorization snapshot and immutable role-version boundary.
- FMB-T003 A003 supplied migration `000017`, runtime/audit owning data and the `attention_items`
  storage contract. FMB-T002 adds no migration and does not revise migration `000017`.
- Shared OpenAPI, generated contracts, HTTP composition, `ProductApp`, styles and embedded Web files
  were edited only after the orchestrator transferred ownership from T003. The frontend stopped
  editing before final embed, source gate and local-stack rebuild.
- The worktree already contained approved Alpha/M3 and reviewed FMB-T001/FMB-T003 work. The scoped
  receipt in `diff.patch` records T002 ownership without attributing the full dirty-tree diff to this
  task.

## Authority Map

| Rendered surface | Server authority | Basis exposed to the client |
| --- | --- | --- |
| Workbench identity/state | `attention_items` plus immutable owning projections | `ati_*`, rule version, condition key, item version, route target |
| Workbench visibility/actions | one consistent authorization snapshot plus owning workflow state | mine/team/initiated view, authorization version, target-scoped capability and SoD checks |
| Asset definition/evidence | `semantic_assets` current revision and persisted revision evidence | exact revision ID; `evidence.read` denial redacts records and nested evidence |
| Physical model | versioned physical binding, grain and entity-key snapshots | exact owning object version and independent release basis |
| Joins | versioned join-contract snapshot | exact direction, type, expression and key pairs with independent release basis |
| Relations/lineage | persisted semantic relations and lineage evidence | cursor-paged owning rows; no invented revision or release basis |
| Validation/trust | persisted validation runs and evidence | real run records; absent runs report `not_configured`, never passed/healthy |
| Consumer impact | active binding and its exact effective release | consumer, environment, mode, status and effective release |
| Release manifest | immutable release asset/object pins | selected release ID and exact pinned versions |
| Release comparisons | nearest same-target prior pin and live registry pointer | distinct prior-pin diff and selected-pin-versus-current-registry diff |

Unauthorized sections return `forbidden` without protected rows or nested evidence. Missing owning
facts return `not_configured` or `not_released` according to the asset and object release state;
failed reads remain explicit rather than falling back to fixtures.

## Review And Scale

Focused review cycles closed authorization races, workspace and target existence leaks, Workbench
PATCH visibility, replay fingerprinting, version conflicts, restart races, legacy trace handling,
UTF-8 truncation, latest-run selection, deep-link stability, section capability leakage and mutable
release substitutions. The final independent Go/PostgreSQL suites are green; the independent final
spec/security verdict found no P0/P1 blocker and the orchestrator advanced the task to `Needs_Review`.

The 10,000-asset integration scenario proves stable 100-row cursor pages and an authorization-aware
server total of 10,000. The independent performance run had one host-noisy search p95 failure and an
immediate strong pass; both measurements are disclosed in `test-results.md` rather than presenting a
one-shot result.

## Visual Proof

The frontend validation completed 142/142 unit tests and the 18/18 desktop/compact-desktop product
journey. Live server-mode UAT at `http://127.0.0.1:18081` captured Workbench, asset detail, release
detail and dual-diff evidence at 1440x900 and 1024x768:

- `screenshots/desktop-workbench-detail.png`
- `screenshots/desktop-asset-detail.png`
- `screenshots/desktop-release-detail.png`
- `screenshots/desktop-release-dual-diff.png`
- `screenshots/compact-desktop-workbench-detail.png`
- `screenshots/compact-desktop-asset-detail.png`
- `screenshots/compact-desktop-release-detail.png`
- `screenshots/compact-desktop-release-dual-diff.png`

`screenshots/uat-observations.json` records the real workspace, actor, `ati_*`, asset, revision and
release identities. Fixture mode was disabled. Both viewports report no global or target overflow,
reduced-motion compliance, visible keyboard focus and successful Workbench/asset/release deep-link
reload. `screenshots/manifest.md` records the capture boundary, file hashes and exact assertions.

The local UAT stack has no available identity session (`/api/v1/session` returns 503), so reload
preserved each exact URL and the harness reselected the same authorized workspace before asserting
the persisted target. Requests used the server's explicit `local-author` UAT actor boundary, not a
fixture build or a client fallback. Session-backed workspace restoration remains a FMB-T007 local
identity boundary rather than evidence claimed by this task.

## Residual Risk

- The compatibility legacy relations endpoint is bounded and fails explicitly when its cap would
  truncate results; the canonical catalog UI uses the cursor-paged authority records endpoint.
- The production bundle remains above Vite's 500 kB warning threshold. This is visible in the build
  output and remains a final hosted-readiness/performance decision for FMB-T007.
- The task is `Needs_Review`; final acceptance remains founder-owned.

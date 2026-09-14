# FMB-T004 Orchestrator Review

Result: Needs_Review, not Accepted. Date: 2026-09-05.

## Scope And Correctness

- Sources and schedules use real typed APIs, immutable artifact pins, owning run records and current authorization decisions.
- Independent read-only review finds no remaining confirmed P1/P2 blocker in the final scoped fixes.
- Reviewed closures cover digest replay descriptors, hidden workbook formulas, scheduler lock order, bounded spool recovery, request retry identity, polling pagination/focus, canonical actors and exact successful-projection reuse.
- Reuse preserves per-attempt run/Operations identity, excludes failed or different-adapter projections, does not duplicate candidates, and retains commit lease fencing.
- Migration 18 rollback refuses to erase reused history; populated version-17 data round-trip and PostgreSQL 17/18 lifecycle checks pass.
- The final local storage probe performs an exclusive non-business write, sync, close and delete. A fresh image-initialized volume belongs to UID/GID 65532 and permits an unprivileged probe without a manual permission repair.

## QA Evidence

- Final `GOFLAGS=-p=1 make check-source`: PASS, including all source gates, 161 Web tests, PostgreSQL 18 ingestion, generated drift and binary build.
- Docker live source suite: four desktop tests PASS in 10.1 s. No API responses are mocked.
- Screenshots and serious/critical accessibility assertions pass at 1440x900 and 1024x768.
- Native PostgreSQL 16 provides additional real SQL/dbt import and controlled queued-run/server/database/worker restart proof; it is not presented as PostgreSQL 17/18 proof.
- Operations API and UI deep link resolve the exact recovered owning run and terminal events.
- Additional real Docker UI verification in `wsp_01m1qtvmbmer2b4mpxyysfkm73` uploads dbt manifest/catalog and registers synthetic configured-root SQL. dbt run `run_01m1qtwwener7tv8jkkr23yfej` succeeds with two datasets, two fields and one lineage edge; SQL run `run_01m1qtza20eraaawgv702zh6sy` succeeds with one dataset and three fields. Hard reload followed by explicit workspace selection preserves both run IDs and three candidates. The SQL fixture is isolated at `sprint-sql-20260905/orders.sql` inside the development content volume.

## Snapshot

- Cumulative allowed-boundary diff: `diff.patch`, 116 files, including pre-existing Alpha/FMB changes.
- SHA-256: `81bbb9e0586cd64077cb975b02d2913f8f5d1426cc6314e367e5c899a0dce432`.
- Verified Docker image: `sha256:8ad9fbda794e8e7c86e9625b2d75713087b6aaf02f61ef73f1a1a4560e1bc6ef`.
- No commit, push, reset, broad cache cleanup or user-data deletion is performed.

## Remaining Acceptance

Hosted S3/OIDC/live-provider journeys, external release acceptance and Windows runtime filesystem
execution are not claimed. Graceful restart proof does not imply in-flight process-kill or hosted
failover proof. Old unowned temporary files are not deleted by guessing ownership. Known scheduler
retry handling does not cover errors whose database type was erased by another repository boundary.

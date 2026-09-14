# FMB-T006 Sprint A001

Status: Needs_Review

The founder authorized this second three-hour sprint on 2026-09-05. Its window is
04:49:54-07:49:54 UTC, with implementation cutoff at 07:24:54 UTC.

## Preflight

- Packet: `docs/specs/full-menu-beta/packets/T006.yaml`.
- FMB-T001 through T005 are Needs_Review with passing local source and review evidence.
- Canonical resolver dependency is recorded in Alpha T005 and M3 T001-T003.
- Baseline HEAD: `63241d5300cbffc0f62bc842cac132aa13560076`.
- Dirty worktree contains substantial prior Alpha/FMB work and must be preserved.
- T006 is the sole implementation owner; T007 remains out of this sprint.
- Docker PostgreSQL storage has 40.7 GiB available. Local preview runs on port 18081
  with clean migration 19. Root owns preview builds/restarts; database tests run serially.

## Required Return

Changed-file inventory, actual RED/GREEN commands, migration lifecycle, credential and
scope security, real channel parity, signed receiver/SSRF tests, desktop evidence and
explicit residual gaps. No external-provider or hosted evidence is inferred from fixtures.

## Execution Checkpoint

At 06:00 UTC, the integrated backend and real Settings view are implemented. Source
handoffs to root serialize the preview build; no migration21 or T007 work is included.
Production routes, schemas and web sources are frozen during image builds. Test-only
and evidence work continues when it cannot change build inputs.

Actual RED/GREEN and focused security/channel/receiver results are in
`../test-results.md`. The latest focused channel proof uses a real PostgreSQL18
container, official MCP Go SDK v1.7.0, a real `go run` CLI process and Node's TypeScript
strip-types runtime. The golden digest is compared within each seeded run, not fixed
across random release IDs.

Parent-approved bounded extension: `web/src/localUAT.ts` and its test align only the
local-author UI projection with the existing server workspace_admin's member.manage
and role.assign. Reviewer/publisher SOD and production compile gating are preserved.

## Final Handoff

Full source, uncached database, real desktop, fixture regression, smoke and security
gates pass. Root's runtime review caught and closed workspace grant normalization,
stale prototype disclosure, checkbox layout and pending-refresh focus restoration.
Test-only corrections preserve immutable binding versions, idempotent revocation,
server-owned checkbox state and animation-stable geometry rather than weakening
product constraints. Earlier RED and host-contention results are retained separately.

The 69-path sprint-relative patch and SHA-256 manifest are frozen under FMB-T006.
All source owners are released. T007 is not started; public HTTPS receiver and hosted
acceptance remain explicit external gaps. No founder acceptance, commit or push is
asserted. The local preview remains available on port18081 with clean migration20.

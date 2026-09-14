# FMB-T005 Sprint Attempt

Status: Completed; task Needs_Review, no release acceptance claim.

- Authorized by the confirmed Full-Menu Beta graph and the founder's three-hour sprint request.
- Activated: 2026-09-05 11:43 Asia/Shanghai, after T004 enters Needs_Review and returns shared files.
- Packet: `docs/specs/full-menu-beta/packets/T005.yaml`.
- Implementation cutoff: 2026-09-05 12:37:01 Asia/Shanghai. Final handoff deadline: 13:02:01.
- One delegated implementation owner; no concurrent T006/T007 implementation or automatic commits.
- Preserve the dirty worktree and migration 18. Real provider and hosted proof are not inferred from deterministic fixtures.
- Docker storage has recovered; use serial database tests. A local `pgvector/pgvector:pg17` image is available.
- The validated T004 Docker development stack remains at `http://127.0.0.1:18081` until a deliberate reviewed rebuild.
- The supplemental native PostgreSQL 16 server/worker/database are stopped; their RED/GREEN data is preserved under ignored `.semlia/sprint-native`.

Implementation source is frozen at 12:32 after the dimension-form correction. The repository-backed pipeline, typed HTTP/Web surface, real pgvector suite and deterministic provider transport tests are complete within the implementation window. Final integrated source gates pass with 170 Web tests. Real desktop/compact-desktop embedding E2E passes in 13.1 seconds; source migration regression, smoke, fixture E2E and security scan results are retained in the orchestrator records.

- Delivered scope and changed-file groups: `../summary.md`.
- Actual RED/GREEN commands, assertions and residual validation: `../test-results.md`.
- Isolated live QA, including real failed fixture attempts and HTTP runs: `../orchestrator-qa.md`.
- Reproducible synthetic-only inputs: `../ui-fixture.sql` and `../deterministic-provider.mjs`.

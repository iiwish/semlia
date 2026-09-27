# V1-T001 Browser Baseline

Status: Failed baseline; Needs_Review. This record is test evidence, not 1.0 acceptance.

## Execution

- Date: 2026-09-26, approximately 20:17-20:20 Asia/Shanghai.
- Attempt: V1-T001-A002, existing production browser lane only.
- Source HEAD: `2bf50c33c18cda1d56b01de143841790443e891a`.
- Command: `bash scripts/dev/production-acceptance.sh --suite desktop`.
- Exit code: `1`.
- Playwright: 12 tests, one worker, no retries; 8 passed and 4 failed in 1.6 minutes.
- Viewports: desktop `1440x900`; compact desktop `1024x768`.
- Preflight Go test: `./internal/testsupport/productionacceptance` passed from cache. The launcher completed the frontend build, acceptance-server build, isolated PostgreSQL startup and readiness check before Playwright.
- No application implementation, browser assertions, initializer or private configuration was edited. This attempt made no external-model calls.

## Isolation and Cleanup

The launcher and server validation were inspected before execution:

- The launcher generates a fresh random `spacc_` owner, refuses preexisting resources with that Compose project label, and creates a new owner-marked run directory.
- The Compose stack uses a dedicated PostgreSQL container and named volume, with a random port bound only to `127.0.0.1`. It does not connect to an existing application database.
- The server requires a matching owner marker, database name and role, a non-default loopback database port, and a loopback HTTP address. The launcher supplies those values rather than reading an existing private environment file.
- Cleanup targets only the generated Compose project. Before removal it checks the secondary `io.semlia.acceptance.owner` label on every matching container, network and volume, then stops the owned server process and removes the owned Compose resources.

This run's owner is `spacc_88d6ef6041cca792`. Its local evidence directory is `.semlia/production-acceptance/spacc_88d6ef6041cca792/`.

`cleanup.log` records removal of the run's PostgreSQL container, default network and database volume. Follow-up `docker container ls -a`, `docker network ls` and `docker volume ls`, each filtered by `com.docker.compose.project=spacc_88d6ef6041cca792`, returned no resources. No other project resources or existing database were changed by this attempt.

## Actual Failures

| Test | Desktop | Compact desktop | First failing assertion |
| --- | --- | --- | --- |
| `isolated identity and server-owned production workspace` | Failed, 20.7 s | Failed, 20.6 s | `web/e2e-production/production.spec.ts:11`: after real author login and opening `/assets?section=drafts`, region `知识确认` was not found within 20 seconds. The captured page contains the `知识目录` region and active draft filter. |
| `source candidate creates a server-owned semantic draft without seeded assets` | Failed, 26.2 s | Failed, 25.3 s | `web/e2e-production/production.spec.ts:44`: saving the synthetic source candidate navigated to `/work/operations/<operation-id>`; the test expected the obsolete `production=` query parameter and timed out after 20 seconds. |

Both failures reproduce in both supported viewport sizes. The source case reached the normal author login, SQL-file source import, discovery, candidate selection and draft-save interaction. It did not reach its post-save persistence assertion, independent review, publication or rollback assertions. A successful navigation alone is not recorded as proof of persisted draft correctness.

No assertions were removed, loosened or updated. The next validation attempt must use the current route contract while retaining persistence, independent-role and governance checks.

## Passing Coverage and Limits

All four `system-status.spec.ts` scenarios passed at both sizes: ready rendering without overflow, dependency error and trace rendering, unreachable API handling, and keyboard refresh with reduced motion.

Those tests intercept the health/system API or intentionally abort it. Their eight passing results are synthetic status-UI evidence, not proof of a healthy production deployment. The production lane uses `protocol_stub` model responses; it does not validate the configured model provider, five-type executable-model questions, SQL/result reconciliation or answer correction.

## Local Artifacts

All paths below are relative to this run's local evidence directory. They are not public attachments. Trace archives can contain synthetic login/session material and must not be copied into version control.

Failure context and trace directories:

- `browser/production-isolated-identi-78d60--owned-production-workspace-desktop/`
- `browser/production-isolated-identi-78d60--owned-production-workspace-compact-desktop/`
- `browser/production-source-candidat-28dcd-draft-without-seeded-assets-desktop/`
- `browser/production-source-candidat-28dcd-draft-without-seeded-assets-compact-desktop/`

Each failure directory contains `error-context.md` and `trace.zip`. The existing production tests failed before their explicit screenshot steps; no standalone production-failure PNG was generated.

Generated screenshots show only the synthetic status scenarios:

- `browser/system-status-renders-the--81465-y-contract-without-overflow-desktop/ready.png`
- `browser/system-status-renders-the--81465-y-contract-without-overflow-compact-desktop/ready.png`
- `browser/system-status-shows-dependency-error-code-and-trace-desktop/dependency-unavailable.png`
- `browser/system-status-shows-dependency-error-code-and-trace-compact-desktop/dependency-unavailable.png`
- `browser/system-status-shows-a-safe-9d3f0-when-the-API-is-unreachable-desktop/configuration-error.png`
- `browser/system-status-shows-a-safe-9d3f0-when-the-API-is-unreachable-compact-desktop/configuration-error.png`

The private run directory also retains build/startup/cleanup logs. Existing private configuration and bootstrap credentials are not reproduced in this record.

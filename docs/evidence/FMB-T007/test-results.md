# FMB-T007 Test Results

Status: Incomplete; full source gate fails. Date: 2026-09-05.

| Command or check | Result | Evidence and scope |
| --- | --- | --- |
| `go test -p 1 ./internal/domain/execution/... ./internal/application/execution/... ./internal/adapters/execution/...` | PASS with gaps | `focused-final.log`: two compiler tests pass; service and adapter packages have no test files. |
| Distribution/execution/identity focused regression after formatting | PASS, worker-reported | `implementation-handoff.md`; does not prove real source execution. |
| First `GOFLAGS=-p=1 make check-source` | FAIL | `check-source-incomplete.log`: two new distribution files needed gofmt. |
| Formatted `GOFLAGS=-p=1 make check-source` | FAIL | `check-source-final.log`: format/lint/typecheck pass; migration and Operations tests fail. Three pre-existing lint warnings. |
| `go test -p 1 -count=1 ./tests/integration/operations` before fixture repair | FAIL | `operations-rerun.log`: expired fixed-time job lease; independently reproducible. |
| Operations after future-lease fixture repair | PASS | Independent uncached root rerun passes in3.494s; `operations-final-green.log`. |
| `pnpm -r --if-present test` | PASS | `web-sdk-tests.log`: 180 Web tests across21 files; SDK script runs `tsc --noEmit`, not a new execution journey. |
| `make contracts-check db-generate-check` | PASS | `generated-contracts.log`; generated artifacts agree with current sources. Missing execution endpoints are still missing. |
| `git diff --check` | PASS at frozen-core checkpoint | Final source snapshot also checked before handoff. |
| Existing local `/health/ready` | HTTP200 | Earlier migration20 preview; not the T007 candidate. |

## Failing Migration Cases

The full gate observes schema21 clean but tests still expect schema20:
`TestMigrationLifecycleAndTenantSchema`, `TestPostgres17MigrationLifecycle` and
`TestPopulatedM1UpgradeAndRollbackPreserveRegistryRows`. Relative rollback baselines also
fail in both migration18 lifecycle cases and `TestMigration20PreservesPopulated19AndLeaseFencing`.
Do not merely update expected numbers: migration21 needs its own populated up/down/up
and immutable physical-provenance proof before the final suite can establish safety.

## Not Established

- Real read-only execution role/session, source identity, timeout/cancel/row/byte bounds,
  current authorization, concurrent idempotency, process loss and privacy scanning.
- Real publish-to-resolve-to-execute and consistent REST/MCP/CLI/SDK/Ask/Operations paths.
- T007 live desktop/E2E, security scan, production build, release artifacts/SBOM or hosted
  acceptance. These gates were not run against an integrated candidate because none exists.

Historical T004/T005/T006 patch digests match their recorded values. The normal preview
was not rebuilt, migrated or stopped. Testcontainers used isolated test databases and
their own cleanup; no user volume was removed. No commit/push or external deployment.

# FMB-T007 Implementation Handoff

Status: Incomplete, not wired into the running application, not acceptance evidence.
Handoff time: 2026-09-05 10:30 UTC. Implementation stopped around 08:10 UTC after a quota interruption; the worker resumed only to report the actual state.

## Worker-Owned Changes

- Added `internal/domain/execution/{model.go,compiler.go,compiler_test.go}`.
- Added `internal/domain/distribution/execution.go`; modified `model.go` and `resolver.go` for execution provenance, plan digest and exact selector pinning.
- Added `internal/application/distribution/execution.go` for current execution authorization, freshness and plan reconstruction.
- Added `internal/application/execution/service.go` for pre-connection checks, durable claims and metadata-only replay.
- Added `internal/adapters/execution/postgres.go` for deployment-referenced credentials, read-only role/session checks, bounded results and redacted errors.
- Added `internal/adapters/postgres/execution.go` for source/credential checks and durable run metadata; modified `distribution.go` for frozen relation loading.
- Modified `internal/application/authorization/credential.go`, `internal/application/identity/machine.go` and `machine_test.go` for explicit execution opt-in.
- Added migration 21 up/down and `db/queries/execution.sql`; modified `db/sqlc.yaml`; ran sqlc generation, including generated `execution.sql.go` and `models.go`.
- Final validation-window fixture repair: changed only the active discovery job lease expiry in `tests/integration/operations/operations_test.go` from a fixed business timestamp to `time.Now().Add(time.Minute)`. Business event timestamps and the production lease fence are unchanged.

No Web source, KnowledgeViews, HTTP, MCP, CLI, SDK, main/config, publishing implementation, deployment or release-script files were edited by this worker. Existing dirty changes in those surfaces belong to the baseline or other authorized work. Migrations 1-20 were not edited. No commit, push, reset, external deployment or container operation was performed.

## Observed Commands

1. RED: `go test -p 1 ./internal/domain/execution/...` failed because execution had only tests and no non-test Go files.
2. GREEN: the same command passed after implementing the compiler (parameterized aggregate and legacy/unsupported refusal tests).
3. `go test -p 1 ./internal/domain/distribution ./internal/domain/execution ./internal/adapters/postgres` passed; the last package only compiled and has no tests.
4. `make db-generate` succeeded before the final migration credential-constraint adjustments.
5. `go test -p 1 ./internal/application/execution ./internal/application/distribution ./internal/application/identity` initially failed on two missing identity helpers and the old credential opt-in expectation. Those were corrected; the command then passed (execution service has no tests).
6. `go test -p 1 ./internal/adapters/postgres ./internal/application/execution ./internal/adapters/execution` passed compilation; all three packages report no test files.

7. Final formatting-only closeout: `gofmt -w internal/domain/distribution/execution.go internal/domain/distribution/model.go` completed successfully.
8. Final focused regression: `go test -p 1 ./internal/domain/execution ./internal/domain/distribution ./internal/application/distribution ./internal/application/identity` passed all four packages, including the final UseNumber and join-expression-enum edits. Execution and identity were cached; distribution domain completed in 1.191 seconds and distribution application in 0.412 seconds. These existing tests do not add missing precision, formal-join or service integration evidence.
9. Validation-window fixture RED/GREEN: root's `operations-rerun.log` records the expired fixed-clock lease failure. After changing only the active lease-expiry fixture, `go test -p 1 ./tests/integration/operations` passed in 3.993 seconds. This tests existing Operations behavior with its real PostgreSQL container; it does not provide query-execution adapter or execution-service evidence.

Command outputs exist in the worker task transcript, not standalone captured log files. Apart from the final Operations package regression, no database integration test, migration lifecycle, browser QA, contracts gate, full source gate, smoke/security/release gate or hosted verification was executed by this worker. Session 68464 was collected at handoff with exit code 0; no worker exec sessions remain live. Source is frozen after the formatting and fixture-only closeout; no production behavior or interface was added during closeout.

## Blocking Gaps

- Normal publication still creates single-target releases. Carry-forward of already published asset/object pins and their original physical snapshots is not implemented, so normal publish-to-resolve-to-execute is not proven reachable.
- Formal JoinContract left/right field references are not yet used by the snapshot loader. It still reads `content.execution.fieldPairs`; the compiler now requires the explicit `field_pairs_equal/v1` expression marker, which the loader does not populate. Joins therefore fail closed but the intended typed join contract is incomplete.
- HTTP, MCP, CLI, SDK, configuration and Ask execution controls are not wired to the execution service. Operations integration exists only as repository-written runtime metadata, not a tested end-to-end path.
- No real PostgreSQL credential, transaction, timeout/cancellation, row/byte cap, concurrent key, process-loss or sensitive-persistence test exists yet. The role-check and source-DSN-matching code is unverified against a real server.
- The migration has not been applied or rolled back against populated data. sqlc generation and migration version/readiness/contracts must be reconciled after the final schema edits.
- The last compiler and provenance changes need code-quality, security and specification review. Existing narrow tests do not prove service authorization or publication invariants.

Metadata-only replay is intentional: a repeated request never re-executes a durable claim or fabricates original result rows. Unknown/lost outcomes are terminal metadata and require an intentional new idempotency key for any new attempt. This behavior is implementation intent, not yet proven with a real database.

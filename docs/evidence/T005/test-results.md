# T005 Test Results

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T005 |
| Attempt | M0-T005-A001 |
| Date | 2026-08-10 |
| Result | Pass |

## 1. TDD Evidence

### RED

Tests for migration lifecycle, tenant schema, concurrent claim, concurrent idempotency, retry/dead-letter, expired leases, transactional outbox and publisher behavior were added before the schema or repositories.

Command:

```bash
go test ./tests/integration/db/... ./tests/integration/worker/...
```

The first setup attempt identified missing module checksums and was normalized with `go mod tidy`. The verified RED rerun failed with exit 1 because `postgres.NewMigrator`, `postgres.Store`, `jobs.Job`, `jobs.OutboxEvent` and the worker APIs did not exist.

### GREEN

Commands:

```bash
go tool sqlc generate -f db/sqlc.yaml
go test -v -count=1 ./tests/integration/db/... ./tests/integration/worker/...
```

Result: Pass, exit 0. Four database integration tests and six worker/outbox integration tests passed against two isolated `postgres:18-alpine` containers.

During GREEN, real PostgreSQL exposed one ambiguous timestamp parameter in the terminal job update and one nondeterministic test fixture order. The SQL received explicit `timestamptz` casts and the fixture received deterministic IDs; behavior assertions were not weakened.

### REFACTOR

Command:

```bash
go test -race -count=3 ./tests/integration/db/... ./tests/integration/worker/...
```

Result: Pass, exit 0.

```text
ok github.com/semlia/semlia/tests/integration/db      15.915s
ok github.com/semlia/semlia/tests/integration/worker  14.008s
race_exit=0
```

## 2. PostgreSQL Checks

| Check | Result |
| --- | --- |
| Empty PostgreSQL 18 upgrade | Pass, version 1 clean |
| Repeated upgrade | Pass, no change and no dirty state |
| Down to empty and upgrade again | Pass, identical five-table inventory |
| Tenant foreign keys | Pass for audit, jobs and outbox |
| Immutable audit update/delete | Pass, both rejected |
| Worker startup without schema | Pass, safe failure and unchanged inventory |
| Two concurrent claimers | Pass, one lease at attempt 1 |
| Wrong lease owner completion | Pass, `ErrLeaseLost` |
| Eight concurrent same-key enqueues | Pass, one returned ID and one persisted row |
| Deterministic retry/dead-letter | Pass at attempts 1 and 2 |
| Expired lease recovery | Pass for retryable and terminal paths |
| Audit/outbox rollback | Pass, both row counts remain zero |
| Audit/outbox commit | Pass, both row counts become one |
| Publisher retry/dead-letter/success | Pass; raw publisher error absent |

## 3. Query Plan Evidence

With `enable_seqscan = off`, the real PostgreSQL 18 planner returned:

```text
Index Scan using jobs_claimable_idx on jobs
  Index Cond: (available_at <= CURRENT_TIMESTAMP)
  Filter: (status = ANY ('{queued,retryable}'::text[]))

Index Scan using outbox_events_claimable_idx on outbox_events
  Index Cond: (available_at <= CURRENT_TIMESTAMP)
  Filter: (status = ANY ('{queued,retryable}'::text[]))
```

## 4. CLI Migration Cycle

A separate `postgres:18-alpine` container used a random non-default host port. No existing local database was accessed.

| Command | Result |
| --- | --- |
| `make db-migrate-up` | Pass, `migration up complete` |
| `make db-migrate-version` | Pass, `1 (dirty=false)` |
| `make db-migrate-down` | Pass, `migration down complete` |
| `make db-migrate-version` | Pass, `empty` |
| `make db-migrate-up` | Pass |
| `make db-migrate-version` | Pass, `1 (dirty=false)` |

## 5. Full Validation

| Command | Result |
| --- | --- |
| `make db-generate` | Pass, sqlc v1.31.1 |
| `make db-generate-check` | Pass, generated pgx/v5 code unchanged |
| `make db-test` | Pass |
| `go test -race -count=3 ./tests/integration/db/... ./tests/integration/worker/...` | Pass |
| `go test ./...` | Pass |
| `go vet ./...` | Pass, no findings |
| `go mod tidy -diff` | Pass, no diff |
| `make build` | Pass |
| `make contracts-check` | Pass, public contract artifacts current |
| `git diff --check` | Pass |

The governor's generic artifact validator was also attempted. It only recognizes its default `.ai-platform/**` layout and therefore reported those default paths as missing. Semlia intentionally uses the repository-native `docs/specs/**` and `docs/evidence/**` layout; direct checks confirmed the T005 packet, summary, test results, diff, `Needs_Review` task state and recorded patch SHA-256.

## 6. Result

All validation commands required by packet `M0-T005-A001` pass. T005 is ready for explicit founder acceptance.

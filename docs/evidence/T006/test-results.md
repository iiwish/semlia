# T006 Test Results

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T006 |
| Attempt | M0-T006-A001 |
| Date | 2026-08-10 |
| Result | Pass |

## 1. TDD Evidence

### RED

The local stack, HTTP integration, worker persistence and dependency recovery smoke tests were added before Compose, the image or local scripts existed.

Command:

```bash
go test ./tests/smoke/...
```

Result: expected failure, exit 1. `Test00LocalStackContract` reported missing `compose.yaml`, `compose.override.yaml`, `.env.example`, `deploy/local/Dockerfile`, `scripts/dev/compose.sh` and `scripts/dev/ensure-env.sh`. Runtime cases were gated until an explicitly started stack existed.

### GREEN

Commands:

```bash
make dev
make smoke
```

Result: Pass, exit 0.

- The first large `golang:bookworm` download was interrupted after 13m14s because one registry layer stopped progressing; no stack or database was created.
- The retry moved the pure-Go builder to official Alpine and downloaded only the executable build graph. The first successful complete start created the image, network, PostgreSQL volume, migration and healthy processes in 33.926s with previously downloaded Node/Go base layers cached.
- Final cached rebuild/start after all review fixes completed in 18.352s.
- The first full smoke suite completed in 18.529s; the final verbose suite completed in 15.324s.

### REFACTOR

Commands:

```bash
docker compose --env-file .semlia/dev.env restart worker
make smoke
make dev-down
```

Result: Pass, exit 0. Worker restart completed in 0.266s. The final post-restart smoke completed in 15.697s, and shutdown removed containers/network while retaining `semlia-local_postgres-data`.

## 2. Smoke Cases

```text
PASS Test00LocalStackContract                 0.00s
PASS Test01EmbeddedWebAndAPI                  0.01s
PASS Test02RequiredProcessesAreRunning        0.23s
PASS Test03WorkerRestartPreservesDatabaseJob  0.59s
PASS Test04PostgresFailureAndRecovery         1.14s
PASS Test05ShutdownPreservesDataVolume       12.89s
```

| Check | Result |
| --- | --- |
| Embedded index and hashed JS/CSS asset | Pass from Go server |
| Liveness, readiness and system info | Pass with 32-character trace IDs |
| Migration role | Pass, exited 0 before server/worker startup |
| Running roles | Pass for PostgreSQL, server and worker |
| Worker restart | Pass, same container and persisted future job row |
| PostgreSQL stop | Pass, live 200, Web 200, ready 503 |
| PostgreSQL recovery | Pass, same container ID and ready 200 |
| Compose down | Pass, no project containers/network |
| Named volume | Pass, retained and job row readable after restart |

## 3. Security And Image Checks

| Check | Result |
| --- | --- |
| Generated env file mode | Pass, `600` |
| Generated credential repository scan | Pass, zero matches outside `.semlia` |
| Committed usable credential | None |
| Personal absolute source path | None |
| Host HTTP binding | `127.0.0.1:8080` |
| Host PostgreSQL binding | `127.0.0.1:5433` |
| Host port 5432 access | None |
| Multi-role image identity | Pass, identical `sha256:f4d0fb59def0...` |
| Runtime user/root filesystem | `nonroot:nonroot`, read-only |
| Node runtime files | Zero |
| Worker runtime log | Valid JSON startup event |

`docker compose --env-file .semlia/dev.env config --quiet` passed. PostgreSQL 18 stores its versioned data under the named volume mounted at `/var/lib/postgresql`.

## 4. Full Validation

| Command | Result |
| --- | --- |
| `make dev` | Pass, healthy full stack |
| `make smoke` | Pass in repeated runs |
| `docker compose --env-file .semlia/dev.env restart worker` | Pass |
| `docker compose --env-file .semlia/dev.env config --quiet` | Pass |
| `docker compose --env-file .semlia/dev.env images` | Pass, three app roles use one 12.3 MB image |
| `make dev-down` | Pass, volume preserved |
| `go test ./...` | Pass |
| `go vet ./...` | Pass, no findings |
| `make build` | Pass, Web regenerated and embedded |
| `make contracts-check` | Pass, public artifacts current |
| `git diff --check` | Pass |
| Reverse patch applicability | Pass for `docs/evidence/T006/diff.patch` |

The governor's generic validator was run with `python3` because this machine has no `python` alias. It only recognizes the default `.ai-platform/**` layout and reported those default documents plus its default evidence directory as missing. Semlia intentionally uses the repository-native `docs/specs/**` and `docs/evidence/**` layout; direct checks confirmed the T006 packet, `Needs_Review` task state, summary, test results, reversible diff and recorded SHA-256.

## 5. Result

All validation commands and review passes required by packet `M0-T006-A001` pass. T006 is ready for explicit founder acceptance.

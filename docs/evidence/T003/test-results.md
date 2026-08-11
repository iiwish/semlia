# T003 Test Results

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T003 |
| Attempt | M0-T003-A001 |
| Date | 2026-08-10 |
| Result | Pass |

## 1. TDD Evidence

### RED

Command:

```bash
go test ./cmd/semlia/... ./internal/...
```

Result: Expected failure, exit 1. `cmd/semlia` did not exist; HTTP and config tests could not resolve the new application and runtime packages.

### GREEN

Command:

```bash
go test ./cmd/semlia/... ./internal/...
```

Result: Pass, exit 0. Command, configuration and HTTP behavior tests all passed.

### REFACTOR

Command:

```bash
go test -race ./cmd/semlia/... ./internal/...
go vet ./cmd/semlia/... ./internal/...
make contracts-check
```

Result: Pass, exit 0. Race detector and vet reported no findings; generated OpenAPI artifacts remained current.

## 2. Full Validation

| Command | Result |
| --- | --- |
| `go test -race ./cmd/semlia/... ./internal/...` | Pass |
| `go test ./...` | Pass |
| `go vet ./...` | Pass |
| `make contracts-check` | Pass |
| `go mod tidy -diff` | Pass, no diff |
| `make build` | Pass |
| `git diff --check` | Pass |

Test coverage includes 16 named tests plus routing subcases across command configuration, production safety, liveness independence, readiness success/failure, error envelopes, trace propagation, public system info, panic recovery and log redaction.

The governor's generic artifact validator was also attempted with `python3`. It expects a `.ai-platform/**` layout and therefore reported the canonical files as missing; Semlia intentionally uses the existing `docs/**` artifact layout, so packet, task and evidence paths were verified directly instead of creating a duplicate governance tree.

## 3. Runtime Checks

| Check | Result |
| --- | --- |
| Built binary starts on loopback | Pass |
| Liveness returns 200 and correlated trace | Pass |
| Default readiness returns 503 stable error | Pass |
| Incoming W3C trace ID is preserved | Pass |
| System info exposes only five public fields | Pass |
| Secure production doctor | Pass, exit 0 |
| Unsafe production doctor | Pass, exit 1 without secret echo |
| Graceful `SIGINT` shutdown | Pass, exit 0 |

## 4. Result

All validation commands and runtime checks required by packet `M0-T003-A001` pass. T003 is ready for founder review.

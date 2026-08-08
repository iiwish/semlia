# T001 Test Results

## Environment

| Tool | Observed |
| --- | --- |
| Go | 1.26.3 darwin/arm64 |
| Node.js | 24.15.0 |
| pnpm | 11.1.3 |
| GNU Make | 3.81 |
| Git | 2.54.0 |
| Docker client/server | 29.4.0 / 29.4.0 |
| Platform | Darwin 25.5.0 arm64 |

## RED

Command:

```bash
GO111MODULE=off go test ./tests/repository
```

Result: Expected failure.

```text
missing repository files: go.mod
tool versions contain python and uv instead of golang
legacy baseline files are present
doctor.sh has no Go check and still checks python and uv
FAIL
```

## GREEN

Command:

```bash
go test -v ./tests/repository
```

Result:

```text
7 tests passed
PASS
ok github.com/semlia/semlia/tests/repository
```

## Validation

### Environment doctor

```bash
make doctor
```

Result:

```text
Go 1.26.3: ok
Go module: ok
Node.js 24.15.0: ok
pnpm 11.1.3: ok
Docker daemon 29.4.0: reachable
Ports 4173, 8080 and 5433: available
0 warnings, 0 errors
```

### Shell syntax

```bash
bash -n scripts/doctor.sh
```

Result: Pass.

### Isolated bootstrap

```bash
GOMODCACHE="$(mktemp -d -t semlia-gomodcache)" /usr/bin/time -p make bootstrap
```

Result:

```text
go: no module dependencies to download
pnpm: already up to date
real 0.33s
```

### Determinism

Hashes before and after a second `make bootstrap`:

```text
go.mod
before: fdabb74f47bafcfad3bf921dd32f5b80f0e3947b763b7b82d0374f9d0254da70
after:  fdabb74f47bafcfad3bf921dd32f5b80f0e3947b763b7b82d0374f9d0254da70

pnpm-lock.yaml
before: 17c814b167307942d3609c7b9d916ceddb85839573ab39baa114e30edb132a1a
after:  17c814b167307942d3609c7b9d916ceddb85839573ab39baa114e30edb132a1a
```

Result: Both tracked dependency declarations remained unchanged.

### Go quality checks

```bash
go test -race ./tests/repository
go test ./...
go vet ./...
go mod tidy -diff
```

Result: Pass.

### Stable command adapters

```bash
make test-repository
pnpm check:repository
pnpm install --frozen-lockfile
```

Result: Pass.

### Git whitespace

```bash
git diff --cached --check
```

Result: Pass.

### Delivery artifact smoke validator

```bash
python3 <ai-delivery-governor-skill>/scripts/validate_delivery_artifacts.py \
  --root . --task-id T001
```

Result: Not applicable to this repository layout. The generic validator requires `.ai-platform/**`, while Semlia's confirmed canonical artifacts live under `docs/**`. No duplicate `.ai-platform` source of truth was created; the SSOT, TDR, plan, work graph, checklist, analysis, packet and evidence were reviewed directly.

# T002 Test Results

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T002 |
| Attempt | M0-T002-A001 |
| Date | 2026-08-08 |
| Result | Pass; accepted by founder |

## 1. Toolchain

```text
go version go1.26.3 darwin/arm64
node v24.15.0
pnpm 11.1.3
oapi-codegen v2.8.0
oasdiff pinned by go.mod at v1.28.0
openapi-typescript 7.13.0
openapi-fetch 0.17.0
TypeScript 5.9.3
```

`go tool oasdiff --version` reports `main` because the tool build does not embed its module tag. The authoritative reproducible version is the `github.com/oasdiff/oasdiff v1.28.0` Go tool directive and checksum lock.

## 2. RED

Initial contract test:

```bash
go test ./tests/contracts/...
```

Result: Expected failure. The test reported missing `api/openapi/semlia.v1.yaml`, `api/gen/go/types.gen.go`, `sdk/typescript/src/schema.gen.ts`, `sdk/typescript/src/client.ts` and `scripts/generate-contracts.sh`.

UTC contract hardening:

```bash
go test ./tests/contracts/...
```

Result: Expected failure in `timestamp_with_offset`; the initial `date-time` schema accepted `2026-08-08T16:00:00+08:00`. Adding the `Z$` pattern made the machine contract enforce normalized UTC.

## 3. GREEN And Refactor

### Contract generation

```bash
make contracts
make contracts-check
```

Result: Pass. Generated Go and TypeScript artifacts match the canonical specification.

### Contract test matrix

```bash
go test -v ./tests/contracts/...
go test -race ./tests/contracts/...
make test-contracts
```

Result: Pass. All 19 payload cases, artifact checks, compatibility cases and portability checks pass; race detector reports no issue.

### TypeScript client contract

```bash
pnpm --filter @semlia/sdk-typescript test
```

Result: Pass. `tsc --noEmit` validates generated paths, operation response types, error envelope, event envelope and the openapi-fetch wrapper under strict mode.

### OpenAPI validation

```bash
go tool oasdiff validate api/openapi/semlia.v1.yaml
```

Result: Pass with `No findings detected`.

### Compatibility policy

```bash
go tool oasdiff breaking --fail-on WARN \
  tests/contracts/fixtures/base.yaml \
  tests/contracts/fixtures/compatible.yaml
```

Result: Pass with no breaking changes.

```bash
go tool oasdiff breaking --fail-on WARN \
  tests/contracts/fixtures/base.yaml \
  tests/contracts/fixtures/breaking.yaml
```

Result: Expected exit 1. oasdiff reports `api-path-removed-without-deprecation` for `GET /things`.

### Generated drift probe

```bash
make contracts-check
```

Result after injecting a probe into the generated TypeScript artifact: Expected failure with a stale artifact diff. `make contracts` restored the generated artifact and the same check passed.

### Repository validation

```bash
go test ./...
go vet ./...
go mod tidy -diff
pnpm install --frozen-lockfile
make bootstrap
bash -n scripts/generate-contracts.sh
```

Result: Pass. The API generated package builds, repository and contract tests pass, Go module files are tidy, the pnpm lock is frozen, bootstrap is idempotent and the generation script has valid shell syntax.

### Deterministic hashes

```bash
shasum -a 256 api/gen/go/types.gen.go sdk/typescript/src/schema.gen.ts
```

Result after each of two generation runs:

```text
ff3839ace16d6f85dcd3954769ccaeec9011a0ffdc72ad4aadc32c159c46ff49  api/gen/go/types.gen.go
701bf6d975ce33382455643bb568f02d0e7f3a3e43e3cd23a8367aaa9f7d25b1  sdk/typescript/src/schema.gen.ts
```

### Git whitespace

```bash
git diff --cached --check
```

Result: Pass.

### Delivery artifact smoke validator

```bash
python3 <ai-delivery-governor-skill>/scripts/validate_delivery_artifacts.py \
  --root . --task-id T002
```

Result: Not applicable to this repository layout. The generic validator requires `.ai-platform/**`, while Semlia's confirmed canonical artifacts live under `docs/**`. No duplicate `.ai-platform` source of truth is created; the SSOT, TDR, plan, work graph, checklist, analysis, packet and evidence are reviewed directly.

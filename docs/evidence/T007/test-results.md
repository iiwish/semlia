# T007 Test Results

## RED-GREEN Evidence

| Command | Result | Evidence |
| --- | --- | --- |
| `go test ./tests/repository -run CI -count=1` before implementation | Expected fail | Missing workflows, Make targets, scanner scripts and release scripts were reported explicitly. |
| `go test ./tests/repository -run CI -count=1` after implementation | Pass | Workflow, permission, pinned-action, local-mapping, scan and release contracts passed. |
| Release SBOM contract before verifier | Expected fail | Contract rejected release tooling without `verify -manifest`; the first generated SBOM had zero components. |
| Release SBOM contract and real release after verifier | Pass | Rootfs package analysis found 21 components; structured verifier and checksum validation passed. |

## Final Local Validation

| Command | Result | Duration / evidence |
| --- | --- | --- |
| `make bootstrap` | Pass | Frozen Go/pnpm dependencies current; cached pnpm install 0.183s. |
| `make check` | Pass | 61.33s cached; source, smoke and security gates all passed. |
| `make check-source` | Pass | Format, vet, lint, typecheck, Go/pnpm tests, contract/migration/Web drift and build passed. |
| `make check-smoke` | Pass | 36s final run; full stack smoke passed and all project containers/network were removed. |
| `make security-check` | Pass | Final independent cached run 2.94s; dependency, secret, OS and Go-binary High/Critical findings all zero. |
| `make build` | Pass | Final explicit run 1.30s. |
| `make sbom` | Pass | Final run 4.07s; CycloneDX 1.7 with 21 components. |
| `make release` | Pass | Final run 7.30s; metadata, archive members, SBOM and checksums verified. |
| Cross release `GOOS=linux GOARCH=amd64 make release` from Darwin arm64 host | Pass | Host manifest tool ran on `GOHOSTOS/GOHOSTARCH`; target archive, SBOM and checksums verified. |
| `go test ./tests/repository -count=1` | Pass | Repository and CI contracts passed. |
| `git diff --check` | Pass | No whitespace errors. |
| `make dev-down` via smoke cleanup | Pass | No Semlia project container or network remains. |
| Governor generic artifact validator | Layout mismatch | The generic script only recognizes `.ai-platform/**` and reported its five default documents plus default evidence directory as missing. Semlia uses `docs/specs/**` and `docs/evidence/**`; direct checks confirmed packet, Blocked task state, summary, test results, patch hash and reverse applicability. |

## Security Remediation Trail

| Finding | Resolution |
| --- | --- |
| `google.golang.org/grpc` v1.80.0 High | Upgraded to v1.82.1 with matching genproto modules. |
| Go 1.26.3 standard library, 3 High | Repository, CI and release image aligned on Go 1.26.5. |
| transitive `js-yaml` 4.3.0 High | pnpm 11 workspace override fixed the frozen graph at 4.3.1. |
| Container scanner Docker socket denial | Scanner alone uses a read-only socket mount; application containers remain unchanged. |

## Hosted Validation

| Workflow | Commit | Result | Duration / evidence |
| --- | --- | --- | --- |
| [CI](https://github.com/iiwish/semlia/actions/runs/31453327615) | `44b0f49` | Pass | Source approximately 2m28s；integrated smoke approximately 2m09s。 |
| [Security](https://github.com/iiwish/semlia/actions/runs/31453335446) | `44b0f49` | Pass | Approximately 58s；dependency、secret、Debian image and Go binary High/Critical findings all zero。 |
| [Release Build](https://github.com/iiwish/semlia/actions/runs/31453335512) | `44b0f49` | Pass | Linux/Darwin x amd64/arm64 all built and uploaded；longest job approximately 1m17s。 |

Initial hosted failures were retained as regression evidence: smoke lacked its Web toolchain, and cross-target release leaked target `GOOS/GOARCH` into a host helper. Commits `8f8842d` and `44b0f49` fixed them and added repository contracts before the final green runs.

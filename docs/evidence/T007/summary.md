# T007 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T007 建立 CI、安全与供应链门禁 |
| Attempt | M0-T007-A001 |
| 状态 | Needs_Review |
| 执行日期 | 2026-08-10 至 2026-08-11 |
| Branch | main |
| Final remote commit | `44b0f49d8db1227e9c6c0d4dc1342fb20fd75dc3` |
| Repository | `https://github.com/iiwish/semlia`（Public） |
| Packet | `docs/specs/m0-foundation/packets/T007.yaml` |
| Executor | Codex direct execution; delegation was not requested and implicit sub-agents are disabled |

## 1. Delivered Scope

- Pull-request source/smoke workflows, pull-request plus scheduled security workflow, tagged/manual release workflow, Dependabot coverage and a PR template.
- Local `make check-source`, `make check-smoke`, `make security-check`, `make check`, `make sbom` and `make release` surfaces. Workflow validation steps call these Make targets instead of duplicating logic.
- Parsed repository contracts for workflow triggers, local mapping, immutable action SHAs, least privilege, scanner thresholds, release metadata and dependency monitoring.
- Trivy 0.73.0 pinned by image digest, production/development dependency and repository secret scanning, plus release image OS/Go-binary scanning with High/Critical failure thresholds.
- Go baseline aligned on 1.26.5, grpc upgraded to v1.82.1 and transitive `js-yaml` fixed at 4.3.1 after real scans exposed High findings.
- Cross-platform release archives with complete commit metadata, migrations, notices, non-empty CycloneDX SBOM, SHA-256 sums and tagged GitHub provenance attestation.
- Independent smoke CI prepares its own locked pnpm/Node/Go toolchain instead of relying on another job's filesystem.
- Cross-release target compilation is isolated from host-side manifest generation and verification, so Linux runners can build Darwin and ARM artifacts without executing target binaries.

No application, domain, database, migration, Web, SDK, prototype or Compose behavior changed.

## 2. Security Evidence

Final filesystem scan:

```text
go.mod         gomod  vulnerabilities=0
pnpm-lock.yaml pnpm   vulnerabilities=0
repository            secrets=0
```

Final release image scan:

```text
semlia:security (debian 12.15) vulnerabilities=0
app/semlia (Go binary)          vulnerabilities=0
```

The gate found grpc v1.80.0, Go 1.26.3 standard-library and `js-yaml` 4.3.0 High findings during implementation. Each finding was remediated to a scanner-reported fixed version; no ignore rule or risk acceptance was added.

## 3. Release Evidence

The final local release output contains:

```text
archive:  semlia-0.0.0-dev-darwin-arm64.tar.gz (4.2 MB)
version:  0.0.0-dev
commit:   2e53e6f630bcb07d6f3a88d1876d7c2c5646aa0e
SBOM:     CycloneDX 1.7, 21 components
checksum: archive and external SBOM verified
```

SHA-256 samples:

```text
c5ce9e508495091db613d302ae2242e754f2848f393f69e0f19e0f7bf6b2ea6c  semlia-0.0.0-dev-darwin-arm64.tar.gz
98ea7f38977b685f3fbe1f859d4c873305ac3398530624f65cc820dcc4bdc92a  semlia-0.0.0-dev-darwin-arm64.sbom.cdx.json
```

The release verifier parses metadata and SBOM JSON, rejects an empty component list, checks required archive members and recomputes both checksums.

Final hosted release run:

- Run: `https://github.com/iiwish/semlia/actions/runs/31453335512`
- Commit: `44b0f49d8db1227e9c6c0d4dc1342fb20fd75dc3`
- Linux amd64、Linux arm64、Darwin amd64、Darwin arm64 build and artifact upload: Pass。
- Longest matrix job: approximately 1m17s。

## 4. Workflow Review

- CI and security workflows have only top-level `contents: read`.
- The tagged attestation job alone receives `contents: read`, `id-token: write` and `attestations: write`.
- All remote actions are pinned to full 40-character commit SHAs.
- `pull_request_target` is absent; workflows do not write repository contents, publish images or create releases.
- Source, smoke and security jobs are independent and each has a 10-minute timeout.
- Final CI run `https://github.com/iiwish/semlia/actions/runs/31453327615`: source approximately 2m28s，smoke approximately 2m09s，both Pass。
- Final Security run `https://github.com/iiwish/semlia/actions/runs/31453335446`: approximately 58s，Pass，scan artifacts uploaded。

## 5. Review Results

Spec compliance: Pass locally with no implementation finding. All approved source, smoke, security, release, dependency-monitoring and PR-governance requirements map to packet and Make targets.

Bug and code quality: Pass with no blocking finding. Smoke cleanup preserves interrupt exit status, scan reports survive failure, the Docker socket mount is read-only, SBOMs require discovered components, checksums are recomputed and staging is excluded from uploaded release artifacts.

QA acceptance: Pass for local and hosted acceptance surfaces. RED/GREEN, clean source checks, full integrated smoke, zero-High security scans, non-empty SBOM, release metadata, checksum verification, four-platform release build and clean shutdown all passed.

Hosted failure trail:

- Initial smoke job failed because it had Go but no pnpm/Node/bootstrap state. Commit `8f8842d` added an independent locked Web toolchain and a repository contract that enforces setup order.
- Initial cross-platform release failed with `exec format error` because target `GOOS/GOARCH` affected `go run` for the manifest helper. Commit `44b0f49` pins helper execution to `GOHOSTOS/GOHOSTARCH` and adds contract coverage.
- The final runs above prove both fixes on clean GitHub-hosted Ubuntu runners.

## 6. Diff

- Patch: `docs/evidence/T007/diff.patch`
- Patch SHA-256: `76df0bd1247bb20c688807a79f0556faa8161b2058b8b35ea812d7b6bb2d06ec`
- Patch lines: 5,211; reverse applicability check passed.
- Patch scope: initial local T007 implementation before remote validation.
- Remote validation fixes are recorded by commits `8f8842d` and `44b0f49`; Git history and hosted run URLs are authoritative for the final state.
- The baseline patch includes predecessor content from the accepted P001-T006 chain in shared files. Do not use it as a task-only rollback.

## 7. Rollback And Residual Risk

- Workflow, CI and release scripts can be removed with their Make/package entry points without changing runtime behavior.
- Go/grpc/js-yaml security upgrades should not be reverted without a fresh vulnerability assessment.
- Exact base-image tags are monitored by Dependabot but are not source-pinned to registry digests; Trivy scans the resolved release image on every required security run.
- Cold builds depend on public Go, npm, Docker and Trivy registries. The hosted critical path remained below 2m30s in the final run, but registry availability remains an external dependency.

## 8. Review Handoff

The external completion gate is satisfied by complete green CI, Security and Release Build runs on final commit `44b0f49`. T007 has no known blocking finding and is in `Needs_Review`. It becomes `Accepted` only after explicit founder acceptance; T008 remains dependency-blocked until then.

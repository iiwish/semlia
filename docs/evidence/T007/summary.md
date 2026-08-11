# T007 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T007 建立 CI、安全与供应链门禁 |
| Attempt | M0-T007-A001 |
| 状态 | Blocked by external CI run gate |
| 执行日期 | 2026-08-10 |
| Branch | main |
| Base state | `2e53e6f` plus accepted, uncommitted P001 through T006 work |
| Packet | `docs/specs/m0-foundation/packets/T007.yaml` |
| Executor | Codex direct execution; delegation was not requested and implicit sub-agents are disabled |

## 1. Delivered Scope

- Pull-request source/smoke workflows, pull-request plus scheduled security workflow, tagged/manual release workflow, Dependabot coverage and a PR template.
- Local `make check-source`, `make check-smoke`, `make security-check`, `make check`, `make sbom` and `make release` surfaces. Workflow validation steps call these Make targets instead of duplicating logic.
- Parsed repository contracts for workflow triggers, local mapping, immutable action SHAs, least privilege, scanner thresholds, release metadata and dependency monitoring.
- Trivy 0.73.0 pinned by image digest, production/development dependency and repository secret scanning, plus release image OS/Go-binary scanning with High/Critical failure thresholds.
- Go baseline aligned on 1.26.5, grpc upgraded to v1.82.1 and transitive `js-yaml` fixed at 4.3.1 after real scans exposed High findings.
- Cross-platform release archives with complete commit metadata, migrations, notices, non-empty CycloneDX SBOM, SHA-256 sums and tagged GitHub provenance attestation.

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

## 4. Workflow Review

- CI and security workflows have only top-level `contents: read`.
- The tagged attestation job alone receives `contents: read`, `id-token: write` and `attestations: write`.
- All remote actions are pinned to full 40-character commit SHAs.
- `pull_request_target` is absent; workflows do not write repository contents, publish images or create releases.
- Source, smoke and security jobs are independent and each has a 10-minute timeout. The final cached local superset completed in 61.33 seconds; hosted durations remain unmeasured until a remote run exists.

## 5. Review Results

Spec compliance: Pass locally with no implementation finding. All approved source, smoke, security, release, dependency-monitoring and PR-governance requirements map to packet and Make targets.

Bug and code quality: Pass with no blocking finding. Smoke cleanup preserves interrupt exit status, scan reports survive failure, the Docker socket mount is read-only, SBOMs require discovered components, checksums are recomputed and staging is excluded from uploaded release artifacts.

QA acceptance: Pass for all local acceptance surfaces. RED/GREEN, clean source checks, full integrated smoke, zero-High security scans, non-empty SBOM, release metadata, checksum verification and clean shutdown all passed.

## 6. Diff

- Patch: `docs/evidence/T007/diff.patch`
- Patch SHA-256: `76df0bd1247bb20c688807a79f0556faa8161b2058b8b35ea812d7b6bb2d06ec`
- Patch lines: 5,211; reverse applicability check passed.
- Patch scope: 24 T007-owned files relative to repository HEAD.
- The repository has an accepted uncommitted P001-T006 chain, so shared tracked files and the untracked T006 Dockerfile include predecessor content in this HEAD-relative evidence patch. Do not use the patch as a T007-only rollback; the task-specific change list and review are authoritative.

## 7. Rollback And Residual Risk

- Workflow, CI and release scripts can be removed with their Make/package entry points without changing runtime behavior.
- Go/grpc/js-yaml security upgrades should not be reverted without a fresh vulnerability assessment.
- Exact base-image tags are monitored by Dependabot but are not source-pinned to registry digests; Trivy scans the resolved release image on every required security run.
- Cold builds depend on public Go, npm, Docker and Trivy registries. Observed uncached network work remained below ten minutes, but hosted CI timing is not yet available.

## 8. External Completion Gate

This checkout has no Git remote. No branch was pushed and no GitHub repository, run URL or hosted duration was manufactured. Local implementation and three review passes are complete, but T007 remains `Blocked` until at least one complete remote green run supplies the required URL and job durations. It is not yet eligible for `Needs_Review` or founder acceptance.

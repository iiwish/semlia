# M0 Foundation Release Report

## Metadata

| Field | Value |
| --- | --- |
| Milestone | M0 Foundation |
| Status | `Accepted` |
| Date | 2026-08-13 |
| Accepted | 2026-09-02 by founder |
| Baseline | M0 accepted predecessor work plus T008 baseline `source-revision-redacted` |
| Validated implementation | `source-revision-redacted` |
| Repository visibility | Private incubation |
| Evidence | `docs/evidence/T001` through `docs/evidence/T008` |

## Decision Summary

M0 is the accepted engineering foundation for Semlia. It provides a reproducible repository, versioned contracts, a release-shaped Go/Web/PostgreSQL/worker stack, deterministic migrations, local and hosted quality gates, security scanning, SBOM and release-bundle generation, and a measured exact-ref fresh-clone journey.

M0 is not a product release. The semantic registry, optional Cube-backed execution and trusted learning loop remain later milestone work. Public contribution and security operations retain explicit gates. Founder acceptance on 2026-09-02 opens M1 planning; M1 implementation remains governed by a confirmed work graph and Ready execution packet.

## Requirement Disposition

| Requirement area | Disposition | Evidence |
| --- | --- | --- |
| M0-FR-001 repository/toolchain | Pass | Pinned Go/Node/pnpm, task-local Compose/Buildx, controlled module path, doctor and repository contracts. |
| M0-FR-002 public contracts | Pass | OpenAPI source, generated Go boundary/TypeScript client and drift rejection/recovery. |
| M0-FR-003 API/error model | Pass | Versioned liveness, readiness and system-info with stable error code and trace ID. |
| M0-FR-004 Web status | Pass | Embedded desktop/compact-desktop UI consumes generated contracts; accepted T004 journeys cover ready and unavailable rendering. |
| M0-FR-005 database lifecycle | Pass | Explicit migrations plus real empty-database upgrade, downgrade and re-upgrade. |
| M0-FR-006 durable work | Pass | Real PostgreSQL lease, restart recovery, retry, dead-letter, outbox and concurrent-claim coverage. |
| M0-FR-007 security/observability | Pass for M0 | Production fail-closed, trace propagation, log-redaction contracts and zero-High/Critical gates. |
| M0-FR-008 delivery governance | Pass for Private incubation | Local/hosted CI mapping, exact-ref acceptance, release/SBOM/checksum assertions and three independent review passes; public GitHub controls remain gates. |

## Non-Functional Disposition

| Requirement | Result |
| --- | --- |
| M0-NFR-001 bootstrap | Pass: cold 8m24.895s; warm 567ms. |
| M0-NFR-002 pull-request feedback | Pass: complete local gate 5m33.241s; accepted T007 hosted source/smoke jobs were below 2m30s. |
| M0-NFR-003 platforms | Pass with qualification: final T008 ran on Darwin arm64; accepted T007 ran Linux CI and Linux/Darwin amd64/arm64 release builds. Windows remains a container-path support statement, not an independently tested native path. |
| M0-NFR-004 local independence | Pass: no paid SaaS, cloud account or LLM key is required; cold bootstrap needs public package/image registries. |
| M0-NFR-005 sensitive data | Pass with immutable-history qualification: live source and canonical docs contain no personal absolute path; historical evidence patches may preserve validation-time paths. Repository and secret gates are clean; generated credentials are ignored and mode 0600. |
| M0-NFR-006 real database | Pass: migrations, health and worker acceptance use isolated PostgreSQL, never SQLite. |
| M0-NFR-007 supported dependencies | Pass for incubation: supported versions are pinned and Dependabot version updates are configured; vulnerability alerts remain a public-release gate. |

## Acceptance Scenarios

AC-M0-001 through AC-M0-006 pass at exact commit `source-revision-redacted`. The final run used task-owned empty Go and pnpm caches, reached the first successful request in 10m58.187s, completed 100 requests with zero errors (p50 125µs, p95 260µs, max 493µs), survived and recovered from a real PostgreSQL outage, exercised migration and worker lifecycles, rejected generated-contract drift and insecure production configuration, completed `make check`, built a release bundle and removed task-owned state.

The real runtime outage and accepted T004 Web unavailable-state journeys are complementary: T008 proves API transition and recovery; T004 proves UI rendering of the same stable contract. T008 does not claim a new end-to-end browser outage test.

## Canonical Validation

| Measurement | Result |
| --- | --- |
| Canonical package | Pass, 1,061.991s; exit 0 |
| Clean / doctor | 297ms / 12.312s |
| Cold / warm bootstrap | 8m24.895s / 567ms |
| Development readiness / first request | 21.761s / 1ms |
| Clone to first request | 10m58.187s |
| Runtime smoke | 27.632s |
| Worker / migration / drift | 6.371s / 2.634s / 1.405s |
| Complete pull-request gate | 5m33.241s |
| Release bundle | 10.112s |

The canonical transcript is `docs/evidence/T008/fresh-clone.log`: 67 lines, 3,828 bytes, mode 0644, SHA-256 `13db9ae12000c2fe4e4e1420e5ea30c71ee1a311d7b77cad4b8e91298c773f87`. It is the exact concise outer test transcript, not the complete nested command output.

## Security And Supply Chain

- The isolated `make check` passed source, smoke and High/Critical filesystem/container gates.
- Remote actions and scanner images use immutable references; the release image remains nonroot and production configuration fails closed.
- The exact run asserted a non-empty archive, standalone CycloneDX SBOM and successful verification of both checksum subjects.
- Required exact cleanup deleted the detached checkout and release artifacts, so final-run artifact digests are not retained.
- Earlier independent `2e16d982` release hashes remain supporting evidence only: archive `cb31f8df978b49fe58f0b8af40efc6a2c7504017f69eaf3780f725c171341f30`; standalone SBOM `411e23e2da935f1068aebaac2be01aa1767b99b2d6f5011a489181683574418a`.
- Accepted T007 hosted CI, Security and Linux/Darwin release-matrix evidence remains historical at `source-revision-redacted`, produced while the repository was Public.

Private vulnerability reporting, protected required checks, vulnerability alerts, code scanning, secret scanning and successful tagged provenance under the current repository plan are not claimed. They remain public-release gates.

## Isolation And Cleanup

The launcher removed only the exact Compose project, acceptance-labeled resources, Testcontainers resources, process groups and recorded checkout root. The final audit found zero matching final-run processes and zero roots owned or recorded by the final canonical run; historical roots from earlier attempts can still exist on the host. The restrictive-mode image audit additionally proved source modes 0700/0600 became packaged 0755/0644 under `nonroot:nonroot`, then removed its unique container, `semlia:t008-modefix-1786621303-497` image tag and temporary context; its log SHA-256 is `bfe52f03f0bf218cb442d0eaf86ff6fb8f3cca31e275d728a51aa6ef3adf6eb3`. Shared canonical release/security image tags are outside this task-owned zero-image assertion and may remain as cache. `/tmp/semlia-t008-acceptance.lock` is a persistent lock path by design; it had no holder and was independently reacquirable. The report does not claim that the lock file itself was absent.

## Failure Disposition

The acceptance path deliberately retained its failure trail rather than weakening gates. It includes early release-path and checkout-cache failures; a killed Trivy/check cleanup; cold Go and Corepack issues; one qualified orphan-watchdog probe followed by a self-cleaning probe; missing isolated Buildx; restrictive migration permissions; and slow nested signal-lifecycle fixture readiness. Remedies are narrowly scoped to acceptance isolation, diagnostics, task-owned cleanup and nonroot packaging permissions. Full hashes and dispositions are recorded in `docs/evidence/T008/test-results.md`.

## Operations And Documentation

The root README links the implemented M0 journey. `docs/quickstart.md` covers authenticated Private clone through first request and shutdown. Local-development and troubleshooting guides cover task-owned credentials, ports, stack inspection, disruptive smoke behavior, database lifecycle, cleanup, security/release commands and stable error semantics.

Semlia remains pre-alpha with no supported production version, external security intake or public contribution process. Current documentation states this directly.

## Review

Spec-compliance, bug/code-quality and QA-acceptance reviews passed against `source-revision-redacted` with no blocking finding. Founder acceptance on 2026-09-02 sets this report, T008 and M0 to `Accepted`.

## Residual Release Gates

- Implement and independently validate the M1 Cube import, semantic registry, evidence and trusted-quality loop.
- Enable and prove public repository protections, required checks, vulnerability reporting and security scanners.
- Validate signed or attested tags, embedded-Web dependency inventory, upgrade compatibility and backup/restore.
- Complete public contributor onboarding, governance/support contacts and enforceable DCO checks.
- Re-run onboarding from an external public clone after repository visibility changes.

## Recommendation

Use this report and `docs/evidence/T008` as the accepted M0 baseline. Proceed with M1 through a confirmed work graph, task checklist, consistency analysis and Ready execution packet; do not treat M0 acceptance as a public-release claim.

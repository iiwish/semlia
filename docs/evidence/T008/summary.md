# T008 Evidence Summary

## Metadata

| Field | Value |
| --- | --- |
| Task | T008 fresh-clone acceptance and M0 delivery evidence |
| Attempt | `M0-T008-A001` |
| Status | `Needs_Review` |
| Date | 2026-08-13 |
| Branch | `codex/core-product-loop` |
| Baseline | `9d3608ed769cab01038d3708d657cf4b8eb79af3` |
| Validated implementation | `49f4784a27c2f10e9840455f2f711acebe2c1878` |
| Repository | `https://github.com/iiwish/semlia` (Private incubation) |
| Packet | `docs/specs/m0-foundation/packets/T008.yaml` |
| Canonical log | `/tmp/semlia-t008-acceptance-49f4784-1786623621.log` |
| Canonical log SHA-256 | `13db9ae12000c2fe4e4e1420e5ea30c71ee1a311d7b77cad4b8e91298c773f87` |
| Host | macOS 26.5.2, Darwin 25.5.0, arm64, Apple M5, 32 GiB |
| Tools | Go 1.26.5, Node.js 24.15.0, pnpm 11.1.3, Docker 29.4.0, Git 2.54.0, GNU Make 3.81 |

## Delivered Scope

- The current Private-incubation quickstart and operations guides cover authenticated clone, local startup, diagnosis, cleanup and release verification.
- `make doctor` matches the pinned Go 1.26.5 toolchain, and the live module/import namespace is the controlled `github.com/iiwish/semlia` repository.
- The opt-in acceptance launcher uses an exact detached commit, isolated Go/pnpm/Corepack/Docker CLI state, task-owned ports and Compose identity, bounded subprocess groups and exact cleanup.
- The launcher provisions and validates task-local Compose and Buildx plugins without reading ambient user plugin state.
- Restrictive fresh-clone modes are normalized during image packaging so the existing nonroot runtime can traverse migration directories and read SQL files.
- Runtime smoke recovery, migrations, worker lifecycle, generated-contract drift, production fail-closed checks, the complete pull-request gate and release packaging all execute in one exact-ref journey.

T008 adds no Cube integration, semantic asset, authentication, Agent, MCP or M1 product behavior.

## Canonical Result

The exact-ref launcher validated `49f4784a27c2f10e9840455f2f711acebe2c1878` with exit code 0. `TestFreshCloneAcceptance` passed in 1,061.55 seconds and the package passed in 1,061.991 seconds. The retained `docs/evidence/T008/fresh-clone.log` is the exact 67-line, 3,828-byte, mode-0644 concise outer Go-test transcript, with exactly one `CANONICAL_FULL_EXIT=0` marker; it is not a complete transcript of every nested command.

| Measurement | Result | Target |
| --- | --- | --- |
| Clean | 297ms | informational |
| Doctor | 12.312s | informational |
| Cold bootstrap | 8m24.895s | at most 10m |
| Warm bootstrap | 567ms | at most 2m |
| Development readiness | 21.761s | informational |
| First request after readiness | 1ms | below 1s |
| Clone to first successful request | 10m58.187s | at most 15m |
| 100 sequential requests | 0 errors; p50 125µs; p95 260µs; max 493µs | p95 at most 250ms; max below 1s |
| Smoke | 27.632s | bounded by 10-minute process timeout |
| Worker lifecycle | 6.371s | informational |
| Migration lifecycle | 2.634s | informational |
| Contract drift recovery | 1.405s | informational |
| Complete pull-request gate | 5m33.241s | at most 10m |
| Release bundle | 10.112s | informational |

These are host-qualified measurements. Go and pnpm project caches began task-owned and empty; the existing Docker daemon could retain ordinary base-image and BuildKit cache, so this is not a pristine-host image-pull benchmark.

## Acceptance Matrix

| Scenario | Result | Evidence |
| --- | --- | --- |
| AC-M0-001 Fresh clone | Pass | Exact detached commit, isolated tools/caches, doctor, cold/warm bootstrap, task-owned stack, first request in 10m58.187s and clean checkout. |
| AC-M0-002 Dependency failure | Pass | Real PostgreSQL interruption kept liveness healthy, made readiness return 503 with `DEPENDENCY_UNAVAILABLE` and a trace ID, then recovered. Accepted T004 evidence covers the matching Web unavailable state. |
| AC-M0-003 Migration safety | Pass | Isolated PostgreSQL completed empty-database upgrade, revision check, downgrade and re-upgrade in 2.634s. |
| AC-M0-004 Concurrent workers | Pass | Isolated PostgreSQL proved one successful concurrent lease plus retry, restart recovery and dead-letter behavior in 6.371s. |
| AC-M0-005 Contract drift | Pass | Deliberate generated TypeScript drift failed with an actionable path; restoration passed in 1.405s. |
| AC-M0-006 Security and release | Pass | Insecure production origin, database TLS and secret settings were rejected; `make check` passed; archive, standalone CycloneDX SBOM and both checksum subjects were asserted. |

## Isolation And Cleanup

The final journey removed only its exact Compose project, acceptance-owned scanner/SBOM containers, Testcontainers resources, recorded detached root and task subprocess groups. Post-run audit found zero exact-project containers, networks and volumes; zero acceptance ownership labels; zero Testcontainers labels; zero matching final-run processes; and zero launcher roots owned or recorded by the final canonical run. Historical launcher roots from earlier attempts can still exist on the host and are outside this exact-run assertion. The persistent `/tmp/semlia-t008-acceptance.lock` path remains by design, but no process holds it and an independent nonblocking reacquisition succeeds. Evidence therefore says the lock is unheld and reacquirable, not absent.

The detached checkout and its release directory were deleted during required cleanup. The final run asserted a non-empty archive, standalone CycloneDX SBOM and both checksum subjects before deletion, but exact-run artifact digests were not retained. Earlier independent release hashes remain supporting evidence and are not presented as hashes of the final exact-ref run.

The restrictive-mode image proof is retained at `/tmp/semlia-t008-modefix-1786621303-497.log`, SHA-256 `bfe52f03f0bf218cb442d0eaf86ff6fb8f3cca31e275d728a51aa6ef3adf6eb3`. It records source migration directory/file modes 0700/0600, packaged modes 0755/0644, two SQL files, one directory and final image user `nonroot:nonroot`. Its unique container, `semlia:t008-modefix-1786621303-497` image tag and temporary build context were removed. Shared canonical release/security image tags are deliberately outside this task-owned zero-image assertion and may remain as host cache.

## Failure Trail

Every material failed or qualified attempt remains part of the evidence:

| Attempt | Evidence | Finding | Remedy |
| --- | --- | --- | --- |
| `2923bd2ab4049fa8adfcf55b7cd037ae7ba940b4` | `/tmp/semlia-t008-acceptance-2923bd2.log`, SHA-256 `6a35c667bffd0567c9db9b62d40a70cc518e9b72050d645c7a7ca45f80653281` | Release used an absolute SBOM source path and cleanup met read-only module files. | Kept release output checkout-local and explicitly cleaned the isolated module cache. |
| `2ca99d685ed7b3ba26fc38f9979f94942bc728a7` | `/tmp/semlia-t008-acceptance-2ca99d6.log`, SHA-256 `ff8b2abd82641b2bc01bf7012c357faa076279d5ee5bab60a6b5ebf892fbb340` | Checkout-local Go cache was correctly scanned as source, exposing transitive fixtures and old dependencies. | Moved all tool caches outside source; no security threshold or ignore rule changed. |
| `0c3f72e5f6ca99b319cb869c93649acbba3e0a40` | `/tmp/semlia-t008-acceptance-0c3f72e.log`, SHA-256 `b55c64ae2700a1dfa730113c7eceaf05ef3006f6f0ef7b7d74acee0ffbd599e9` | The check deadline expired after the last visible Trivy database-update stage, and cleanup commands inherited cancellation. | Added process-tree cancellation, independent bounded cleanup and acceptance ownership labels. |
| `8476d7f8659d624aed509189c38176142f7487ae` / `07f1fe2dcfe257790f7f418dae17f5fc9c6256a5` | Logs SHA-256 `f2dbde2fc74f274ab9830a0e49b488361578837ca3d63b99a8b22200089b8a23` and `fa4cfe7a016ebb44632c22429d910610fbee6447c08ebbe5b0300d32885eb20a` | Cold Go toolchain output broke exact parsing and restrictive cache modes resisted removal; cold Corepack then rejected the host CA chain. | Accepted download preamble, normalized cache cleanup and built a task-owned merged CA bundle without weakening TLS verification. |
| Cold Corepack capability probe | `/tmp/semlia-t008-corepack-cold-b82b8de-1786615320.log`, SHA-256 `ef17245becf3808045e587e5bdb54ed92b98e5fa70b2c161715cdc3ed599f0f7` | Capability passed, but the first probe left an orphan watchdog and required exact manual cleanup, so it is a qualified failure. | Corrected supervision; `/tmp/semlia-t008-corepack-cold2-b82b8de-1786615698.log`, SHA-256 `46028a7464c6c1682cb4be0e3df4d58e4cc5240d2acff674d3a1d7719833757d`, passed and self-cleaned. |
| `b82b8de0f61a7db8b0755407e7c99a1604a0043d` | `/tmp/semlia-t008-acceptance-b82b8de-1786615819.log`, SHA-256 `8815f7881552d1e0a1a08fb39a158329cacf9573189b1968f9576fc4dffbc5f8` | Isolated Docker config had Compose but no Buildx, so the BuildKit-only Dockerfile failed. | `fe174ac` provisions, validates and later revalidates task-local Compose and Buildx from exact physical plugin candidates. |
| `fe174acaaa9292be1178206334ce7fadc2694b5c` | `/tmp/semlia-t008-acceptance-fe174ac-1786619362.log`, SHA-256 `f1894d7ddd2032b3cb8f506ebfef22ac2a626a89026c7c2d9a7a4bd64aec81f0` | Buildx succeeded, then the nonroot image could not read restrictive-mode migration SQL and migration exited 1. | `96c0c57` normalizes only packaged migration directory/file modes and proves the final runtime remains nonroot. |
| `96c0c57e0591d0d72fec7bcc414d1bd467520a2a` | `/tmp/semlia-t008-acceptance-96c0c57-1786621712.log`, SHA-256 `fba707dfa5b6cd1532b8fa1691bfa9a01233cc17ed40d802bc353bac674b25cb` | Under nested `make check`, two signal-lifecycle fixtures repeated the complete canonical preflight with fresh Go cache; their fixed 10s/30s marker waits were shorter than the permitted 15s phase/45s aggregate preflight. | `49f4784` gives only those lifecycle tests immediate validated preflight stubs and raises marker waits to 50s/75s; signal forwarding, exit 143 and cleanup assertions are unchanged. |

Two full journeys passed before final validation but were superseded by independent review: `ca5013e940d6368c80fa903dcdb4bd456a0b0584` (host-log SHA-256 `5b1c382514e62f522e463e63f1efe477d993e3e9be993cc920e67ea992a5e018`) and `2e16d982345f74e277ed16c080ca53c877e0ab5a` (SHA-256 `2f9d90211dd862590a184736c476a9d724b6c200045a2f191059f550e63583ad`). Review found false-green or isolation risks, so neither is the canonical record. `a1087bd9586121e3be154aed0744543b781acad1` failed at migration and its checkout cleanup was killed (SHA-256 `aef75d4a8880de3208429daf7194f1a15c21e3399840c3e61ff569c73a263723`). `a4d41989bac85ca12f98b11bda22f8e3d14f464c` has only a three-line clean/doctor transcript and is classified interrupted/incomplete, not failed. A noncanonical direct run at `6f7f6446717e33d0a4dcdd40d69e8e35dae26d71` was immediately rejected by design for lacking the canonical explicit tool path (SHA-256 `0d4f45165aa1c7839aa8ad87398b6a7af6963529c3b1b8eb0db853bed87e369f`).

Earlier passing candidates were not promoted because later independent review found false-green or isolation risks in inherited Go/Make controls, pnpm store resolution, reduced-motion timing, signal handling, cleanup identity and cold-tool behavior. The remedies remain covered by focused tests and the final canonical journey.

## Diff And Rollback

- Patch: `docs/evidence/T008/diff.patch`.
- Patch scope: committed implementation only, exact baseline `9d3608ed769cab01038d3708d657cf4b8eb79af3` through exact implementation `49f4784a27c2f10e9840455f2f711acebe2c1878`.
- Diff summary: 47 files, 9,832 insertions and 86 deletions. Patch serialization is 10,648 lines and 427,305 bytes; SHA-256 `eb4596b0092f0eff205a801289f95c59414bf9decbef78004ecb79c2d8f62fef`.
- Applicability: forward application passes against an exact baseline worktree and reverse application passes against an exact final-commit worktree.
- The module namespace migration is mechanically reversible only while consumers have not adopted `github.com/iiwish/semlia`; documentation and acceptance automation can be reverted independently. No destructive rollback command is prescribed.

## Residual Risk

- M0 is an engineering foundation, not the semantic product loop. Cube import, registry browsing, evidence traceability, quality observations and governed proposals remain M1 work.
- The final exact-ref release artifacts were cleaned and therefore cannot be independently rehashed; only their in-run structure and checksum-subject assertions remain in canonical evidence.
- Public repository protection, vulnerability intake, code/secret scanning under the current visibility, signed or attested tags, backup/restore, upgrade compatibility and public fresh-runner onboarding remain release gates.
- The SBOM covers the release binary/rootfs but does not provide a separate inventory of Web dependencies embedded in the binary.
- Complete host acceptance remains serialized while shared release/security image tags exist, and cold runs depend on public Go, npm, Docker and Trivy registries.

## Review And Handoff

Spec-compliance, bug/code-quality and QA-acceptance reviews passed against exact implementation `49f4784a27c2f10e9840455f2f711acebe2c1878` with no blocking finding. T008 is `Needs_Review`, never `Accepted` by this evidence integration. M0 acceptance and all M1 planning or implementation remain blocked until the founder explicitly accepts this post-execution evidence.

# T008 Test Results

## Metadata

| Field | Value |
| --- | --- |
| Status | `Needs_Review` |
| Validated implementation | `source-revision-redacted` |
| Canonical replay command | `SEMLIA_ACCEPTANCE_SOURCE="$(git rev-parse --show-toplevel)" SEMLIA_ACCEPTANCE_REF=source-revision-redacted ./scripts/acceptance/m0-fresh-clone.sh` |
| Canonical host log | `/tmp/semlia-t008-acceptance-49f4784-1786623621.log` |
| Retained repository log | `docs/evidence/T008/fresh-clone.log` |
| Log integrity | 67 lines, 3,828 bytes, mode 0644, SHA-256 `13db9ae12000c2fe4e4e1420e5ea30c71ee1a311d7b77cad4b8e91298c773f87` |

The retained log is the exact concise outer Go-test transcript. The nested commands intentionally emit summarized outcomes rather than one complete raw transcript.

## TDD Evidence

| Phase | Command | Result |
| --- | --- | --- |
| RED | `go test ./tests/repository ./tests/acceptance/... -run 'TestDoctorVersionMatchesPinnedToolchain|TestModuleNamespaceMatchesControlledRepository|TestM0DocumentationContract' -count=1` | Expected failure: doctor required Go 1.26.3, module/imports used the unverified namespace and current quickstart/operations/private-incubation contracts were absent. |
| Corrective RED | `go test ./tests/repository -run 'TestSmokeJourneyUsesActiveComposeProject' -count=1` in an exact baseline worktree | Expected failure: smoke resource lookup contained the literal `com.docker.compose.project=semlia-local`. |
| Packaging RED | `go test ./tests/repository -run '^TestDockerfileNormalizesMigrationModesBeforeNonrootPackaging$' -count=1` | Expected failure: restrictive clone modes left migration directories 0700 and SQL files 0600 while the final image runs as nonroot. |
| Packaging GREEN | Unique restrictive-mode image audit | Pass: source directory/file modes 0700/0600 became packaged 0755/0644; two SQL files and one directory were readable/traversable by final user `nonroot:nonroot`. Log SHA-256 `bfe52f03f0bf218cb442d0eaf86ff6fb8f3cca31e275d728a51aa6ef3adf6eb3`. |
| GREEN | `go test ./tests/repository ./tests/acceptance/... -count=1` plus focused launcher, plugin, cleanup and restrictive-image probes | Pass at the final implementation; task-local Compose/Buildx, signal isolation, process cleanup and migration readability regressions are covered. |
| REFACTOR | `make doctor && go list -m && go mod tidy -diff && make check-source` | Pass in the exact-ref journey; pinned tools, controlled module identity, generated contracts and source gates remain aligned. |

## Canonical Full Run

Result: `TestFreshCloneAcceptance` passed in 1,061.55 seconds, the package passed in 1,061.991 seconds and the launcher recorded `CANONICAL_FULL_EXIT=0`.

| Step | Result |
| --- | --- |
| `make clean` | Pass, 297ms |
| `make doctor` | Pass, 12.312s |
| Cold `make bootstrap` | Pass, 8m24.895s |
| Warm `make bootstrap` | Pass, 567ms |
| `make dev` readiness | Pass, 21.761s |
| First system-info request | Pass, 1ms; clone-to-request 10m58.187s |
| 100 sequential requests | 0 errors; p50 125us; p95 260us; max 493us |
| PostgreSQL failure/recovery | Pass; liveness 200, readiness 503 with `DEPENDENCY_UNAVAILABLE` and trace ID, recovered readiness 200 |
| `make smoke` | Pass, 27.632s |
| Worker lease/retry/restart/dead-letter | Pass on isolated PostgreSQL, 6.371s |
| Migration up/down/up | Pass on isolated PostgreSQL, 2.634s |
| Contract drift reject/recover | Pass, 1.405s |
| Production fail-closed | Pass for insecure origin, database TLS and secret settings |
| `make check` | Pass, 5m33.241s |
| `make release` and artifact assertions | Pass, 10.112s; archive, standalone CycloneDX SBOM and both checksum subjects asserted |
| Checkout and resource cleanup | Pass; exact final-run resources/processes/recorded root absent, lock path unheld and reacquirable |

## Cleanup Audit

After the canonical command exited, audit found:

- zero processes matching the final launcher, test package or generated checkout;
- zero containers, networks and volumes for the exact Compose project;
- zero Docker resources carrying the acceptance ownership label;
- zero Docker resources carrying Testcontainers labels;
- zero unique restrictive-mode probe container and image tag; its temporary build context is absent;
- zero launcher roots owned or recorded by the final canonical run; historical roots from earlier attempts are outside this assertion;
- a persistent `/tmp/semlia-t008-acceptance.lock` path with no holder, verified by independent nonblocking reacquisition.

The lock file is intentionally persistent and is not claimed absent. The restrictive-image zero assertion covers only `semlia:t008-modefix-1786621303-497`; shared canonical release/security tags may remain by design. Cleanup removed the detached checkout and exact-run release artifacts. Their archive/SBOM/checksum structure was asserted in-run, but their exact digests were not retained.

## Failed And Superseded Attempts

Files under `/tmp` below are host-local working records, not repository-retained evidence. They are listed for audit continuity; the repository retains the final canonical log and this truthful failure summary.

| Commit or probe | Result | Finding and resolution |
| --- | --- | --- |
| `source-revision-redacted` | Failed, package 796.657s | Release rejected absolute SBOM source path and cleanup met read-only module files. Release output returned to source-relative checkout state and isolated module cleanup was added. Host log SHA-256 `6a35c667bffd0567c9db9b62d40a70cc518e9b72050d645c7a7ca45f80653281`. |
| `source-revision-redacted` | Failed, package 817.983s | Trivy correctly scanned checkout-local Go cache fixtures. All tool caches moved outside source; no threshold or ignore rule changed. Host log SHA-256 `ff8b2abd82641b2bc01bf7012c357faa076279d5ee5bab60a6b5ebf892fbb340`. |
| `source-revision-redacted` | Full journey passed, later invalidated by review | Review found inherited Go/Make controls, pnpm-store ambiguity and related false-green paths. The canonical launcher removes child overrides and proves the resolved pnpm store is task-owned. Host log SHA-256 `5b1c382514e62f522e463e63f1efe477d993e3e9be993cc920e67ea992a5e018`. |
| `source-revision-redacted` | Full journey passed, later invalidated by review | Later review exposed process/signal, cleanup identity, cold-tool and Docker-plugin isolation gaps. Subsequent commits added bounded process groups, exact ownership checks and cold-path probes. Host log SHA-256 `2f9d90211dd862590a184736c476a9d724b6c200045a2f191059f550e63583ad`. |
| `source-revision-redacted` | Failed after 2,935.55s | The check deadline expired after the last visible Trivy database-update stage; cleanup inherited cancellation and was killed. Process-tree cancellation and separately bounded cleanup were added. Host log SHA-256 `b55c64ae2700a1dfa730113c7eceaf05ef3006f6f0ef7b7d74acee0ffbd599e9`. |
| `source-revision-redacted` | Failed, package 987.805s | Image build completed, migration exited 1 and checkout cleanup inherited cancellation. Later diagnostics identified restrictive migration modes; cleanup was separated from the failed command budget. Host log SHA-256 `aef75d4a8880de3208429daf7194f1a15c21e3399840c3e61ff569c73a263723`. |
| `source-revision-redacted` | Noncanonical direct invocation rejected | The test correctly required the canonical explicit tool path; wrapper/path preservation was subsequently covered without treating this as a product failure. Host log SHA-256 `0d4f45165aa1c7839aa8ad87398b6a7af6963529c3b1b8eb0db853bed87e369f`. |
| `source-revision-redacted` | Failed, package 116.410s | Cold Go toolchain emitted a download preamble and restrictive cache files resisted TempDir cleanup. Parsing and mode-aware cache cleanup were corrected. Host log SHA-256 `f2dbde2fc74f274ab9830a0e49b488361578837ca3d63b99a8b22200089b8a23`. |
| `source-revision-redacted` | Failed, package 124.518s | Cold Corepack could not validate the host CA chain. A task-owned merged CA bundle preserved TLS verification. Host log SHA-256 `fa4cfe7a016ebb44632c22429d910610fbee6447c08ebbe5b0300d32885eb20a`. |
| First cold Corepack capability probe | Qualified failure | pnpm 11.1.3 downloaded successfully, but the probe left an orphan watchdog and required exact manual cleanup. Host log SHA-256 `ef17245becf3808045e587e5bdb54ed92b98e5fa70b2c161715cdc3ed599f0f7`. |
| Corrected cold Corepack capability probe | Pass and self-clean | Target group, watchdog group and temporary root were absent; lock path was unheld. Host log SHA-256 `46028a7464c6c1682cb4be0e3df4d58e4cc5240d2acff674d3a1d7719833757d`. |
| `source-revision-redacted` | Failed, package 844.947s | Isolated Docker config provided Compose but not Buildx; BuildKit-only `RUN --mount` failed. Task-local Buildx provisioning and identity checks were added. Host log SHA-256 `8815f7881552d1e0a1a08fb39a158329cacf9573189b1968f9576fc4dffbc5f8`. |
| `source-revision-redacted` | Failed, package 840.665s | Task-local Buildx worked and built the image, then migration exited 1 because the nonroot runtime could not read restrictive-mode migration SQL. Packaging modes were normalized without changing migrations or runtime behavior. Host log SHA-256 `f1894d7ddd2032b3cb8f506ebfef22ac2a626a89026c7c2d9a7a4bd64aec81f0`. |
| `source-revision-redacted` | Failed, package 803.147s | Under nested `make check`, two signal-lifecycle fixtures repeated the complete canonical preflight with fresh Go cache; fixed 10s/30s marker waits were shorter than the allowed 15s phase/45s aggregate preflight. `49f4784` gives only those lifecycle tests immediate validated preflight stubs and 50s/75s waits while preserving signal forwarding, exit 143 and cleanup assertions. Host log SHA-256 `fba707dfa5b6cd1532b8fa1691bfa9a01233cc17ed40d802bc353bac674b25cb`. |

The `a4d4198` host log contains only early clean/doctor lines and no terminal result, so it is classified as interrupted/incomplete rather than a failed acceptance record.

## Security And Release Evidence

The final isolated `make check` passed source, integrated smoke and High/Critical filesystem/container gates. The final `make release` asserted a non-empty archive, standalone CycloneDX SBOM and successful verification of both checksum subjects. Required cleanup then removed those artifacts, so no digest in this record is attributed to the final exact run.

An earlier independent release from `source-revision-redacted` remains supporting evidence: archive SHA-256 `cb31f8df978b49fe58f0b8af40efc6a2c7504017f69eaf3780f725c171341f30` and standalone SBOM SHA-256 `411e23e2da935f1068aebaac2be01aa1767b99b2d6f5011a489181683574418a`. These hashes are not final-run artifact hashes.

Hosted CI, Security and four-platform release-build evidence remains accepted T007 history at `source-revision-redacted`, produced while the repository was Public. It is not presented as successful tagged provenance or current Private-repository security-feature validation.

## Review Result

Spec-compliance, bug/code-quality and QA-acceptance passes report no blocking finding for `source-revision-redacted`. This evidence advances T008 only to `Needs_Review`; explicit founder acceptance is still required for T008 and M0, and M1 remains blocked.

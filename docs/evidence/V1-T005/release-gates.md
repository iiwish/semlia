# V1 Final Release Gates

## Status

The eighth, explicitly **frontend-only risk-calibrated** gate attempt produces the verified local candidate **`1.0.0-rc.20260927.6`**. The reviewed [server-search membership repair](../V1-T003/catalog-search-membership.md) has fresh 40-file/356-test frontend evidence; this worker's fresh sync, full browser, smoke, security, release, manifest verification and diff checks all pass once. A complete official-input hash bridge limits the change to five approved frontend/test files plus regenerated embedded assets. Unchanged Go, Node and enabled performance evidence is explicitly inherited from Attempt 7, not described as rerun on RC.6. Exact-binary startup, candidate-specific checks and delegated user acceptance remain root-owned; Git publication and a formal signed release are not completed by this worker.

The seventh gate attempt, including an explicitly reviewed **test-only continuation**, produces the verified local candidate **`1.0.0-rc.20260927.5`**. Its first browser run passes 11/12 production cases and fails a stale compact-desktop draft-filter assertion. That failure remains intact. A 740-input hash bridge proves the subsequent change affects only `web/e2e-production/production.spec.ts`; all 739 other inputs, including product code and embedded assets, are unchanged. The named corrected browser run passes 12/12, and smoke, security, enabled uncached performance, release and verification each pass on their first execution. Earlier sync, 116 Node tests and the full source gate retain their original fingerprint rather than being relabelled as the final one. Root subsequently verifies RC.5 exact-binary startup/version and 4/4 embedded-browser cases, but actual browser search reports one loaded server match and zero visible rows. That real defect and all RC.5 artifacts remain intact; no RC.5 model cohort is run and its browser inspection adds no model calls.

The sixth complete gate attempt passes all assigned standard gates and produces the verified local candidate **`1.0.0-rc.20260927.4`**. It includes the independently reviewed [catalog search optimization](../V1-T002/catalog-performance.md). Its fresh enabled performance measurements pass the unchanged budgets: page p95 **12.223542 ms**, search p95 **22.854292 ms**, resolver p95 **6.383083 ms**. Separate [root-owned candidate checks](candidate-acceptance.md) pass exact-binary startup, runtime version, 4/4 embedded browser cases and the one-shot real-model cohort, 4/4 positives and 2/2 negatives. Subsequent delegated browser acceptance identifies a cold catalog unknown-state defect; its two additional model calls and one execution remain separate from the fixed cohort. Standard gates and the fixed cohort do not replace that acceptance or publish 1.0.

The fifth complete gate attempt targets `1.0.0-rc.20260927.4` and stops at the enabled, uncached performance gate: exact-address catalog search p95 is **340.854583 ms**, above its unchanged **250 ms** budget. Sync, all 116 packet Node tests, source, browser, smoke and security pass; no RC.4 release bundle is produced. The original failure is retained without retry. The [value-grounding evidence](../V1-T002/value-grounding.md) separately records the five successful ordinary governance mutations, the failed preparation wrapper and the successful one-shot GET-only reconciliation. Supplemental knowledge does not retroactively change the original RC.3 model result.

The fourth complete gate attempt passed all assigned standard gates and produced `1.0.0-rc.20260927.3`, after independent review of the [fixed-window prompt and candidate cohort guards](../V1-T002/final-shape.md). Root verifies exact-binary startup, runtime version and all four embedded-browser cases. Its fixed real-model cohort passes **3/4 positives and 2/2 negatives**: the average has the correct scalar shape but a noncanonical region filter, returning null. [Candidate-specific evidence](candidate-acceptance.md) records the five calls and four executions without retry; RC.3 is not accepted.

The third complete gate attempt passed all assigned standard gates and produced `1.0.0-rc.20260927.2`, after the independently reviewed [embedded route and browser-driver fixes](../V1-T004/embedded-candidate-fix.md). Root verified its runtime version and all four embedded-browser cases. Its fixed real-model cohort passed **3/4 positives and 2/2 negatives**, with five model calls and four executions. The old-customer average had the correct numeric value but an unrequested month column, so strict shape acceptance failed. [The original cohort and bounded repair evidence](../V1-T002/final-shape.md) remain intact; RC.2 is not accepted.

The second attempt passed its assigned standard gates and produced `1.0.0-rc.20260927`. The root coordinator then started that exact candidate and verified its version through `/api/v1/system/info` without an environment override. Its embedded browser preflight failed two author cases and passed two consumer cases; the wrapper also failed at `after-runtime`. That candidate is not accepted. Its unchanged artifacts, failed browser evidence and supplemental observer failures remain preserved; the standard-gate pass does not replace candidate-specific acceptance.

The first frozen-source attempt stopped at `make check-source` and produced no candidate. Its failures and protected logs remain intact below; no result from that attempt is substituted for the second attempt.

The latest built local candidate version is `1.0.0-rc.20260927.6`; the running RC.5 executable remains unchanged throughout this worker's Attempt 8 execution. These worker gates do not themselves establish delegated acceptance; root's completed candidate and browser verification is recorded in [candidate acceptance](candidate-acceptance.md). This is not a signed public release.

## Execution Boundary

- Exact toolchain: Go `1.26.6` on `darwin/arm64`, selected explicitly in `PATH`; `GOENV=off`, `GOWORK=off`, empty `GOFLAGS`, and `GOTOOLCHAIN=local`.
- The local Docker endpoint is fixed. An independent anonymous Docker configuration does not import host registry credentials, proxies or contexts.
- A fresh owned release directory and security image tag are reserved. The existing `semlia:security` tag is preserved. Existing security reports are archived privately before their directory/files are restricted to `0700`/`0600`.
- Raw logs, ownership markers, configuration fingerprints and receipts are private under `.semlia/v1-acceptance/current/final-gates-bx869i/`; they are not copied into this evidence.
- Within the worker's standard-gate runs, no real model call, candidate startup, remote deployment, push, signing or release publication occurred. Root-owned candidate checks are recorded separately.

## Attempt 1

Times below are UTC on 2026-09-26 (2026-09-27 local date).

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| `bash scripts/dev/sync-web.sh` | 16:43:53 | 8.607 s | 0 | Passed; authorized embedded-asset generation |
| Packet Node acceptance/isolation tests | 16:44:09 | 64.639 s | 0 | 68 passed, 0 failed, 0 skipped |
| `make check-source` | 16:45:26 | 177.256 s | 2 | Failed in the `tests` stage |

The Node command is:

```bash
node --test scripts/acceptance/v1*.test.mjs \
  scripts/ci/check-smoke.test.mjs scripts/ci/security-image.test.mjs \
  deploy/examples/compose.test.mjs scripts/dev/native-environment.test.mjs \
  scripts/dev/validate-native-isolation.test.mjs \
  scripts/dev/production-acceptance-isolation.test.mjs
```

`check-source` passed format, lint and typecheck. The Go test command reported 63 passed packages, 17 packages without test files and one failed package, `tests/acceptance`. Its ordinary output does not enumerate every individual opt-in skip, so this evidence does not assert zero skipped Go tests. Release-proof, generated-contract checks, generated-SQL checks, embedded-asset drift and the final build stages were not reached.

The two reported Go failures are:

- `TestM0DocumentationContract`: its required fragments do not match the frozen documentation. Quickstart uses the configured native port `18081` and explains nondestructive native smoke, while the test requires literal port-8080 readiness/system-info URLs and `preview`/`disruptive` wording. Troubleshooting uses generic status/trace identifiers but omits the required `DEPENDENCY_UNAVAILABLE` and `traceId` literals. These are documentation-contract failures, not evidence of a runtime outage.
- `TestAcceptancePnpmIgnoresHostScriptShell`: the poisoned-host-shell precondition succeeded and the isolated real frontend suite ran, but that suite reported 339/340 passing tests and 39/40 passing files. Its single failure is `src/governance.test.tsx`, `creates and submits real proposals from the revision workbench`, with an element-not-found assertion. The Go test took 36.79 s; the failing frontend case took 1.929 s. Independent read-only review located line 213 expecting `提交审核`, while the rendered action is `保存修订并继续确认`. The test also mocks/asserts the legacy proposal path rather than the natural correction's production-draft flow. This is a deterministic stale integration-test contract, not a timeout or resource-contention classification. A repair must verify production draft creation/navigation and the absence of automatic confirmation/submission, not merely change the button label.

No failed command was retried. Browser, smoke, security, enabled performance benchmarks, RC build and manifest verification were not started.

## Source Identity

Before authorized embedded-asset generation:

```text
sha256:bb6e614460e7d83d3d235b1c07f0673f9e93892d8325d47e5b62c402554cf587
```

The frozen build-input fingerprint after generation is:

```text
sha256:0825d1b97ceaf1d0f943d7da337b845aa4a9ef86e387bdf2698f36dbe3796668
```

The existing Go release fingerprint implementation produced identical before/after values for both the Node gate and the failed source gate. No behavior source was edited during execution.

## Preservation

The stopped-at-source after snapshot matches the before snapshot for all eight preservation checks: all original containers, volumes, networks, private/Compose configuration hashes and metadata, retained V1 identity, native supervisor identity, default security image tag and acceptance evidence directories. The complete inventories remain 18 containers, 72 volumes and 22 networks. Final-attempt ownership-label inventories contain zero containers, volumes or networks; no cleanup of unknown resources was attempted. The embedded index change is the explicitly authorized sync, recorded separately from environment preservation.

All command sessions for the first attempt ended. That attempt does not certify the candidate. Its separately authorized documentation-contract and natural-correction integration-test repairs precede the second source freeze.

A prior development-tool output disclosed private configuration. That earlier disclosure is not erased by private logs in this attempt; approved credential rotation remains required before production, without reproducing values or discarding encryption recovery material.

## Attempt 2

The root coordinator explicitly authorized a complete second pass after reviewing the focused repairs. This attempt uses new owned output/image identities and private receipts under `.semlia/v1-acceptance/current/final-gates-2-SLpOjs/`. The first attempt's reserved output remains unmodified and contains only its ownership marker.

Times are UTC on 2026-09-26 (2026-09-27 local date). Each command ran once in the listed serial order, using the same exact Go toolchain and controlled local Docker configuration.

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| `bash scripts/dev/sync-web.sh` | 17:03:42 | 7.867 s | 0 | Deterministic embedded assets |
| Packet Node command above | 17:03:58 | 65.371 s | 0 | 68 passed, 0 failed, 0 skipped |
| `make check-source` | 17:05:09 | 173.636 s | 0 | All nine internal stages passed |
| `make check-browser` | 17:08:18 | 103.381 s | 0 | Native checks and 12 production desktop tests passed |
| `make check-smoke` | 17:11:31 | 103.861 s | 0 | Complete owned runtime journey; Go package 70.580 s |
| `make security-check` with fresh image tag | 17:14:05 | 30.330 s | 0 | Existing HIGH/CRITICAL dependency, secret and image gates passed |
| Enabled, uncached performance command below | 17:15:01 | 9.227 s | 0 | Two tests passed, 0 skipped |
| Versioned `make release` below | 17:15:52 | 44.836 s | 0 | Fresh archive, SBOM, checksums, manifest and executable |
| Existing Go manifest verifier | 17:17:15 | 0.420 s | 0 | Archive/staging, dependency coverage and checksums verified |
| `git diff --check` | 17:19:17 | 0.120 s | 0 | Passed |

### Counts and Limits

`check-source` uses the unchanged ordinary `go test ./...`, not `-count=1`. Its 64 passed Go packages comprise **62 cached and two executed** packages; 17 packages have no test files. `tests/acceptance` was actually executed. Its isolated nested frontend command succeeded after the poisoned-shell precondition, but the Go helper suppresses successful child stdout, so that nested invocation's individual test count is not independently available in this log. The separate direct frontend invocation reports **40 files / 340 tests passed**. Lint reports zero errors and four existing warnings. The release-proof stage reports two tests passed.

Ordinary Go output does not enumerate all individual skips. The fresh-clone journey and daemon-specific isolation/cleanup probes retain their opt-in conditions and were not enabled by this packet. The source pass does not substitute for fresh-clone acceptance. The ordinary source pass also leaves the live smoke journey and catalog benchmark behind their opt-ins; this attempt subsequently enabled the real smoke gate and performance gate explicitly. Fixture-only/platform-dependent helper skips remain unchanged.

Native browser assertions cover login, member creation/suspension, password change, rejection of the old password, desktop/compact-desktop views and cookie attributes, with zero page errors. Its cleanup receipt records `owned: true`, `cleanupFailed: false`. The complete original production desktop suite reports **12 passed** without filtering or retry. The browser inputs are synthetic; this is not final-candidate real-model acceptance.

The smoke run independently exposed four owned containers, three volumes, one network and its owned image while live. Every observed owner/project label matched. Its exit checks and independent after inventory prove that all exact configured volumes/network, project-labelled resources and smoke image are absent. No original resource was adopted.

The security image is `semlia:security-final-6bd09be65921f1d605770f69c816`, retained as an owned artifact with image ID `sha256:7d9113783ac6ee2bea14555dd1f69d3816ff817933f37b74aea7731ccfe2cc41`. The existing `semlia:security` image is unchanged. Raw scanner reports, including potential secret-match text, remain private at `0600`; the original reports also remain privately archived. No scanner threshold, version or skip directory changed.

```bash
SEMLIA_RUN_M1_BENCHMARK=1 go test -count=1 -v \
  ./tests/performance/catalog ./tests/performance/distribution
```

Both benchmarks used 10,000 assets and 25 samples on `darwin/arm64`; catalog used an isolated PostgreSQL 18-alpine fixture. Catalog first-page p95 was **11.970167 ms** against 150 ms; exact-address search p95 was **104.991291 ms** against 250 ms. Resolver p95 was **4.795125 ms** against a strict one-second budget. No benchmark was skipped.

### Candidate Identity

The frozen source fingerprint before and after every second-attempt gate, including repeated internal embedded-asset synchronization, is:

```text
sha256:aa52cc10448f82ee88e8ff5e58d4dc4ed7e0ab4bde6e71c07af2342c0801abd4
```

The release invocation selects the already verified exact Go 1.26.6 executable through `GO` and `PATH`:

```bash
SEMLIA_VERSION=1.0.0-rc.20260927 \
SEMLIA_RELEASE_DIR="$PWD/build/release/v1-rc-20260927-6bd09be65921f1d605770f69c816" \
GO="$(go env GOROOT)/bin/go" make release
```

The manifest records version `1.0.0-rc.20260927`, commit `2bf50c33c18cda1d56b01de143841790443e891a`, target `darwin/arm64`, migration version **33**, API `v1`, content schema `0.9.0`, Go `go1.26.6`, `sourceDirty: true`, `artifactKind: local_candidate`, and `acceptance: unreviewed`. The dirty source fingerprint, not the base commit alone, identifies this candidate. The CycloneDX 1.6 SBOM contains 92 components, including 79 Go and nine npm components; the existing verifier checks dependency coverage rather than inferring completeness from these counts.

The exact four-piece paths are:

```text
build/release/v1-rc-20260927-6bd09be65921f1d605770f69c816/semlia-1.0.0-rc.20260927-darwin-arm64.tar.gz
build/release/v1-rc-20260927-6bd09be65921f1d605770f69c816/semlia-1.0.0-rc.20260927-darwin-arm64.sbom.cdx.json
build/release/v1-rc-20260927-6bd09be65921f1d605770f69c816/SHA256SUMS
build/release/v1-rc-20260927-6bd09be65921f1d605770f69c816/staging/run.I5Z11T/semlia-1.0.0-rc.20260927-darwin-arm64/release.json
```

The candidate executable is the `semlia` file beside that staged manifest. Its size is 46,779,442 bytes and SHA-256 is `b960b872ea904354165a27e4f2f189df2665afda7a3ca77d8259e49ac93e106f`. The archive SHA-256 is `5893b26a1f1c5c68db5e970c49de6cf2bc75afb119da182c90d65720b7ab5de3`; external SBOM SHA-256 is `5ce8288f6d744aee5223597b744535080dadcc20a7bc03ea164bfbcda0808419`.

Independent nonexecuting Go build-info inspection confirms the module entry, exact toolchain, target, `CGO_ENABLED=0`, trimpath and VCS revision. Two supplemental observer failures remain visible rather than being rewritten as success:

- The first browser after-snapshot comparator included the authorized regenerated embedded index's file timestamps in the original configuration comparison and exited 1. Its only difference was that generated file's metadata; content/source hashes were unchanged. The corrected comparison preserves the original private/Compose configuration timestamp and permission requirements and records embedded regeneration separately. No browser gate was rerun.
- An extra binary observer attempted to read the injected version from `buildinfo.Settings[-ldflags]` and exited 1 because this trimpath binary does not retain that field. The original `09-identity.json` is unchanged; a separate supported-field receipt records the verified fields and left runtime version pending at this worker's handoff. Neither the official release/manifest gates nor a missing metadata field prove runtime version equality. The root coordinator subsequently confirmed it through the candidate's `/api/v1/system/info`, without rewriting the failed observer. No binary was started or rebuilt by this worker to mask that limitation.

### Final Preservation

The final snapshot reports all eight original-environment checks true: the 18 original containers, 72 volumes and 22 networks are unchanged; original configuration hashes, permissions, timestamps, retained V1 identity and native supervisor are unchanged; the default security image and original evidence directories are preserved. All precise smoke/native/production resource absence checks pass, including native control/owner state removal. Final-attempt scanner-container inventories are empty. Only the explicitly owned security image, candidate artifacts, protected logs and synthetic browser evidence are retained.

All command sessions ended and the heavy local execution window was released. The subsequent root-owned candidate preflight failed as described in Status; this second-attempt standard-gate evidence does not claim candidate acceptance, completed 1.0 user acceptance or publication.

## Attempt 3

The root coordinator authorized another complete serial pass after independent review of the A004 fixes. The version is `1.0.0-rc.20260927.2`; output and security image identities are new and do not overwrite either prior attempt. Private logs and receipts are under `.semlia/v1-acceptance/current/final-gates-3-lwIsU4/` at `0700`/`0600`.

The before snapshot explicitly identifies the already-running **old candidate**, not a source runtime. Its original binary digest, supervisor, owner and launch records are the preservation baseline. No candidate lifecycle operation or model request is part of this worker's gate run.

Times are UTC on 2026-09-26 (2026-09-27 local date). Every listed command ran once; there was no failed-gate retry or carry-forward of prior passing results.

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| `bash scripts/dev/sync-web.sh` | 17:42:24 | 8.470 s | 0 | Deterministic embedded assets |
| Full packet Node command | 17:42:43 | 65.559 s | 0 | 72 passed, 0 failed, 0 skipped; includes four A004 tests |
| `make check-source` | 17:43:55 | 72.006 s | 0 | All nine stages passed |
| `make check-browser` | 17:45:31 | 109.335 s | 0 | Native assertions and 12 production desktop tests passed |
| `make check-smoke` | 17:47:55 | 95.596 s | 0 | Full runtime journey; Go package 69.675 s |
| Fresh-tag `make security-check` | 17:50:02 | 6.151 s | 0 | Unchanged HIGH/CRITICAL dependency, secret and image gates passed |
| Enabled `go test -count=1 -v` performance command | 17:50:42 | 9.765 s | 0 | Both performance tests passed, 0 skipped |
| `make release` with the new version and owned output | 17:51:06 | 17.647 s | 0 | Fresh four-piece bundle and executable |
| Existing Go manifest verifier | 17:51:50 | 0.424 s | 0 | Provenance, dependency coverage, staging/archive and checksums verified |
| `git diff --check` | 17:53:16 | 0.124 s | 0 | Passed |

### Actual Coverage

The unchanged ordinary source gate reports 64 passing Go packages: **61 cached and three executed**, plus 17 packages without tests. `tests/acceptance` is cached in this attempt, so its nested frontend suite is not claimed as newly executed. The separate direct frontend command actually reports **40 files / 340 tests passed**. Lint remains zero errors and four existing warnings. Release-proof has two passing tests. The existing opt-in and individual-skip limitations documented in Attempt 2 remain applicable; no cache or skip is silently counted as an uncached runtime proof.

The native browser result has every login/account/password/desktop/cookie assertion true and zero page errors. Its owned cleanup succeeds. The production suite reports **12 passed**, unfiltered. During smoke, independent observation again verifies four containers, three volumes, one network and the built image with matching owner/project labels. After cleanup, exact names, project labels and the smoke image are absent.

The same two uncached performance tests use 10,000 assets and 25 samples each. Catalog first-page p95 is **12.628209 ms** against 150 ms; search p95 is **107.703875 ms** against 250 ms; resolver p95 is **2.741208 ms** against a strict one-second budget. The catalog PostgreSQL fixture is isolated and removed.

The new retained security image is `semlia:security-final-f140049fbd7deb249374d9cd1d30`, with ID `sha256:64e1ef86d41b9c8347ef14002388e88384bbff45c078d0aafa3c55684ab3c470`. Existing security reports were verified against their private Attempt 2 copies before overwrite; this attempt's raw reports also have private copies. The default `semlia:security` tag is unchanged. No scan version, threshold or skip rule changes.

### RC.2 Identity

The source fingerprint is identical before and after every gate:

```text
sha256:9d0a1252b1c6d2f44bec80592ad902875d21d26f2b078959b117f91e04af713a
```

The release uses exact Go `1.26.6` and the same explicit Go environment controls described above:

```bash
SEMLIA_VERSION=1.0.0-rc.20260927.2 \
SEMLIA_RELEASE_DIR="$PWD/build/release/v1-rc-20260927.2-f140049fbd7deb249374d9cd1d30" \
GO="$(go env GOROOT)/bin/go" make release
```

Exact four-piece paths:

```text
build/release/v1-rc-20260927.2-f140049fbd7deb249374d9cd1d30/semlia-1.0.0-rc.20260927.2-darwin-arm64.tar.gz
build/release/v1-rc-20260927.2-f140049fbd7deb249374d9cd1d30/semlia-1.0.0-rc.20260927.2-darwin-arm64.sbom.cdx.json
build/release/v1-rc-20260927.2-f140049fbd7deb249374d9cd1d30/SHA256SUMS
build/release/v1-rc-20260927.2-f140049fbd7deb249374d9cd1d30/staging/run.9zOID6/semlia-1.0.0-rc.20260927.2-darwin-arm64/release.json
```

The executable is `semlia` beside the staged manifest: 46,779,442 bytes, SHA-256 `8f2e66098008210a5bf08c1f35621528072574d548afc8e418be4de2959505b2`. The manifest SHA-256 is `841581071749e1cd7b67fd988f7d452168891264afeb2a3ebb6366e1279c2b67`; archive SHA-256 is `00d998a96c4cbcd6b5bbea1c0ba8770f648629c944897aa882c4bac987959d92`; external SBOM SHA-256 is `c155a646c3b07ec5ade50399da2238ecd4d37c168e7e98d59ba1dab7bb4268ba`.

Manifest metadata records version `1.0.0-rc.20260927.2`, base commit `2bf50c33c18cda1d56b01de143841790443e891a`, target `darwin/arm64`, migration **33**, API `v1`, content schema `0.9.0`, exact Go `go1.26.6`, `sourceDirty: true`, `local_candidate`, and `acceptance: unreviewed`. The SBOM remains CycloneDX 1.6 with 92 components, including 79 Go and nine npm components. Independent nonexecuting build-info inspection verifies the entry module, toolchain, target, CGO disabled, trimpath and VCS revision; it does not infer runtime version from an absent linker-flags field. Runtime version remains for root-owned readback on this exact new executable.

### RC.2 Preservation

The final snapshot confirms all eight original-environment checks and every precise cleanup check. The inventories remain **18 containers, 72 volumes and 22 networks**. Private/Compose configuration bytes, permissions and timestamps, the running old candidate's identity and supervisor, the default security tag and original evidence directories are unchanged. Independent byte hashes also prove that all old candidate artifact files and the active old executable remain unchanged. Authorized embedded synchronization metadata is recorded separately, with content and source hashes stable.

The new candidate, owned security image, synthetic evidence and private logs are retained; no unowned resource was removed. All command sessions ended. The worker has released the heavy execution window and handed the verified RC.2 paths and hashes to root, without starting the candidate, invoking a model, signing, tagging, pushing or publishing.

## Attempt 4

The root coordinator authorized a new complete serial pass for `1.0.0-rc.20260927.3` after accepting the independently reviewed T002-A004 prompt and cohort-ledger changes. Private logs and receipts are under `.semlia/v1-acceptance/current/final-gates-4-AuzVzg/`, with private directories and files at `0700`/`0600`. The original RC.2 model ledger and both earlier candidate artifact sets are preservation inputs, not overwritten result slots.

### Baseline Exception

The initial read-only observer exited 1 because `node scripts/dev/native.mjs status` encountered `ENOENT`. At the start of this attempt, the original native owner file and control socket were absent, and the supervisor PID recorded in Attempt 3 was no longer present. Attempt 3 had verified that runtime running. The cause of this intervening stop is **unclassified**; neither this worker nor root issued a native stop during this attempt. The first observer failure remains preserved.

Root explicitly accepted the current stopped/absent native state as the new before baseline. The observer was adjusted only to record that actual state rather than require it to be running. No repository gate, runtime, configuration or threshold was changed, and the original native runtime was not restarted or adopted. The retained V1 **RC.2 candidate** remained ready and is separately identified in both snapshots. This evidence does not claim continuous operation of the original native runtime between attempts.

### Serial Gates

Times are UTC on 2026-09-26 (2026-09-27 local date). Every standard gate ran once after the fresh freeze; no failed standard gate was retried.

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| `bash scripts/dev/sync-web.sh` | 18:30:50 | 8.069 s | 0 | Deterministic embedded assets |
| Full packet Node command | 18:31:06 | 63.693 s | 0 | 88 passed, 0 failed, 0 skipped |
| `make check-source` | 18:32:22 | 174.708 s | 0 | All nine stages passed |
| `make check-browser` | 18:35:21 | 114.728 s | 0 | Complete native assertions and 12 production desktop tests |
| `make check-smoke` | 18:39:04 | 100.020 s | 0 | Full owned runtime journey; Go package 71.952 s |
| Fresh-tag `make security-check` | 18:40:52 | 6.722 s | 0 | Unchanged dependency, secret and image gates |
| Enabled `go test -count=1 -v` performance command | 18:41:15 | 12.223 s | 0 | Both tests passed, 0 skipped |
| Versioned `make release` in a fresh owned output | 18:41:34 | 18.636 s | 0 | Fresh four-piece bundle and executable |
| Existing Go manifest verifier | 18:42:02 | 0.319 s | 0 | Archive, staging, dependency coverage and checksums verified |
| `git diff --check` | 18:42:19 | 0.044 s | 0 | Passed |

The ordinary source gate reports 64 passing Go packages: **55 cached and nine actually executed**, plus 17 packages without tests. Actual executions include governance integration (102.508 s), DB integration (96.961 s), authorization and M1 integration, command/application/HTTP/domain tests and repository contracts. `tests/acceptance` is cached, so its nested frontend suite is not claimed as newly executed. The separate direct frontend invocation reports **40 files / 340 tests passed**. Lint remains zero errors and four existing warnings. The unchanged opt-in and individual-skip limitations in Attempt 2 still apply.

The native browser assertions are all true with zero page errors; its cleanup receipt is `owned: true`, `cleanupFailed: false`. Production reports **12 passed**, with no filtering or retry. Independent smoke observation verifies four containers, three volumes, one network and the image under matching owner/project labels. Every exact smoke volume/network name, project-labelled resource and smoke image is absent afterward. Native and production test resources and scanner containers are also absent.

Both uncached benchmarks used 10,000 assets and 25 samples. Catalog page p95 was **144.618209 ms** against 150 ms; search p95 was **192.117583 ms** against 250 ms; resolver p95 was **4.794042 ms** against one second. The page result passes with a small observed margin; it is retained as measured, not rerun to obtain a more favorable sample. No performance test was skipped.

Security used the fresh tag `semlia:security-final-1a82464c96a01420d34c8343966a`, retained at image ID `sha256:a1fef0469981c25f6a9805006584e49d078e7c5950756c802297798b1ce01e02`. The original `semlia:security` tag is unchanged. Earlier scanner reports were privately archived before their authorized replacement; new reports and raw logs remain private. Scan severity, tool versions and skip rules are unchanged.

### RC.3 Identity

The existing Go fingerprint verifier reports this source digest before and after every post-sync gate:

```text
sha256:b36d6ee7cd81208b67d980dde2a91f1bd37efbdddc66bbfe8d69c9f2ecb52d36
```

The release invocation uses exact Go `1.26.6` with the same controlled Go and anonymous local-Docker environment:

```bash
SEMLIA_VERSION=1.0.0-rc.20260927.3 \
SEMLIA_RELEASE_DIR="$PWD/build/release/v1-rc-20260927.3-1a82464c96a01420d34c8343966a" \
GO="$(go env GOROOT)/bin/go" make release
```

Exact four-piece paths:

```text
build/release/v1-rc-20260927.3-1a82464c96a01420d34c8343966a/semlia-1.0.0-rc.20260927.3-darwin-arm64.tar.gz
build/release/v1-rc-20260927.3-1a82464c96a01420d34c8343966a/semlia-1.0.0-rc.20260927.3-darwin-arm64.sbom.cdx.json
build/release/v1-rc-20260927.3-1a82464c96a01420d34c8343966a/SHA256SUMS
build/release/v1-rc-20260927.3-1a82464c96a01420d34c8343966a/staging/run.Tgv2yd/semlia-1.0.0-rc.20260927.3-darwin-arm64/release.json
```

The executable beside the staged manifest is 46,779,442 bytes, SHA-256 `0cff6ed4ee22b16ce8f21d75124d558ab023e3c2d29136efac69e62b5c3b172e`. Manifest SHA-256 is `c83a920cf05d6f4ad8d4f360f481fe64eb8eaf02c11ab36cd71dbf7f4b090aee`; archive SHA-256 is `aba3a42bf3862946032c13e0ee21498cedbd7efec6b2c986dd2342691fd73cc2`; external SBOM SHA-256 is `d3f28aff5e22e0fdedbf1e7c89a7959b65240f2bb5cbc216fa8cb915c63090b5`.

The manifest records version `1.0.0-rc.20260927.3`, base commit `2bf50c33c18cda1d56b01de143841790443e891a`, `darwin/arm64`, migration **33**, API `v1`, content schema `0.9.0`, exact `go1.26.6`, dirty source, `local_candidate`, and `acceptance: unreviewed`. The CycloneDX 1.6 SBOM contains 92 components, including 79 Go and nine npm components. Nonexecuting binary inspection verifies the module, exact toolchain, target, CGO disabled, trimpath and VCS revision. Runtime version remains for root-owned readback; no absent linker-flags field is treated as version proof.

### RC.3 Preservation

All ten before/after preservation checks pass: Docker daemon identity; original 18 containers, 72 volumes and 22 networks; recorded original private/Compose configuration hashes and metadata; both prior candidate artifact sets; retained RC.2 runtime identity; original native stopped/absent state; default security tag; and existing evidence directory inventory. The recorded file comparison explicitly includes the legacy `final-candidate.json` and active RC.2 executable, with unchanged bytes, permissions and timestamps. All smoke/native/production/scanner cleanup checks pass. No unknown resource was removed.

Private `artifact-paths.json`, `09-identity.json`, `gate-summary.json`, `after.json` and the preserved first observer failure provide the exact handoff. All command sessions ended. Candidate-specific checks and the one-shot real-model cohort are not part of this worker's completed standard gates; root must authorize and perform them against these exact RC.3 artifacts.

## Attempt 5

Root authorized a complete new serial pass after independent review and successful read-only reconciliation of the supplemental value knowledge. Private logs and receipts are under `.semlia/v1-acceptance/current/final-gates-5-5TJIVi/` at `0700`/`0600`. The before snapshot protects all three old candidate artifact sets, the active RC.3 executable, both historical model cohorts, the failed preparation and root command evidence, the reconciliation receipt and approved pointer. Original native remains stopped/absent with its cause unclassified; it is not restarted.

### Serial Results

Times are UTC on 2026-09-26 (2026-09-27 local date). Each listed repository gate ran exactly once.

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| `bash scripts/dev/sync-web.sh` | 20:20:37 | 14.341 s | 0 | Deterministic embedded assets |
| Full packet Node command | 20:20:52 | 82.141 s | 0 | 116 passed, 0 failed, 0 skipped |
| `make check-source` | 20:22:17 | 328.606 s | 0 | All nine stages passed |
| `make check-browser` | 20:28:00 | 161.258 s | 0 | Complete native assertions and 12 production desktop tests |
| `make check-smoke` | 20:32:34 | 123.438 s | 0 | Full owned journey; Go package 78.497 s |
| Fresh-tag `make security-check` | 20:39:47 | 10.264 s | 0 | Unchanged dependency, secret and image gates |
| Enabled `go test -count=1 -v` performance command | 20:39:59 | 18.402 s | 1 | Catalog search budget failed; resolver passed |

The ordinary source gate reports 64 passing Go packages: **56 cached and eight actually executed**, plus 17 packages without tests. DB integration (199.140 s), governance integration (202.669 s), authorization, M1, command, application governance, HTTP and repository packages actually ran. `tests/acceptance` is cached; its nested frontend invocation is not claimed as a fresh execution. The separate direct frontend run reports **40 files / 340 tests passed**. Lint reports zero errors and four existing warnings. Existing opt-in and individual-skip limitations remain unchanged.

The native browser result has every assertion true and zero page errors, with `owned: true` and `cleanupFailed: false`. Production reports **12 passed**, unfiltered. Their owned resources are absent after execution.

### Supplemental Observer Stop

The actual smoke command and package both pass. Its extra private live-ownership observer records four containers, three volumes, one network and an image with matching owner/project labels, but also records `PRIVATE_COMMAND_FAILED` from one read-only Docker observation. The private wrapper exits 1 after the successful smoke because it requires no observer error. The recorded error does not identify the command or Docker diagnostic, so its cause remains **unclassified**. A list/inspect race during normal cleanup is a possible mechanism, not a proven account of this event.

Execution stops at this supplemental failure. An independent first after-observation verifies absence of the actual observed container/network IDs, all observed volumes, the owned image and project-labelled containers. The complete original network inventory is unchanged. The collector's additional `_default` name probe is not presented as proof of the actual Compose network name; actual IDs and the complete inventory provide the network cleanup evidence. Original configuration, runtime, ledger and artifact preservation also pass. Root and independent review explicitly authorize continuation of only the still-unrun gates, recorded in private `root-continuation-authorization.json`. The failed wrapper receipt and initial `after.json` remain unchanged; no smoke rerun or product-source change occurs.

### Performance Failure

Both tests use 10,000 assets and 25 samples on `darwin/arm64`; the catalog test provisions and removes its own PostgreSQL 18-alpine fixture. Catalog page p50/p95 are **12.549625 / 16.967083 ms**, passing the 150 ms p95 budget. Exact-address search p50/p95 are **118.770417 / 340.854583 ms**, failing the unchanged 250 ms budget in `TestCatalog10000Performance`. The resolver p50/p95 are **4.684792 / 6.010458 ms**, passing its strict one-second budget. Neither test is skipped or cached. The failed catalog test is not rerun.

The log contains no query plan or per-sample trace. A separate post-failure resource observation records ten logical CPUs, load averages about 6.66/6.97/7.72, about 34% CPU idle and 13 GiB compressed memory; it is not contemporaneous evidence for the failed samples and does not establish resource pressure as their cause. Tests have no `t.Parallel`, but the unchanged two-package Go command does not force package serialization. Static inspection identifies repeated list/count search queries with cross-table address/full-text predicates as a diagnostic lead, not a confirmed execution-plan finding. No database, source, concurrency setting or threshold is modified.

Release build, manifest verification and the final release diff gate are **not reached**. The new owned release directory contains only its ownership marker, not an RC.4 archive, SBOM, manifest or executable.

### Source and Preservation

Every completed post-sync command, including the failed performance gate, retains the same source fingerprint:

```text
sha256:5926802fc848c645c40adff8c9f6f8704eb1f16f8ba4906545d2dca96bb13431
```

Security uses the retained owned image `semlia:security-final-e502162d6b7670822fa40d4a1857`, ID `sha256:cdc88b1c531e4cc140d99ce3deea9ed1b96fd25c0ee4fbc3300440824623aeef`. The default `semlia:security` tag is unchanged. Both new scanner reports and the archived original reports remain private at `0600`; their directory is `0700`. No scanner threshold, version or skip rule changes.

The separate final `after-final.json` preserves the earlier snapshot and confirms all ten comparisons: daemon identity; original 18 containers, 72 volumes and 22 networks; original configuration and recorded evidence bytes/metadata; all three old candidate artifact sets; active RC.3 runtime identity; original native stopped state; default security tag; and original evidence directories. All actual observed smoke resources, smoke image and scanner containers are absent. Browser cleanup also remains proven. No unknown resource is removed.

All execution sessions end after the genuine performance failure. The heavy execution window is released. No candidate is started, no model is called, no governance mutation is made, and no RC.4 candidate is claimed by this attempt.

## Attempt 6

Root authorizes a complete fresh serial gate run after the A007 query optimization, baseline and optimized-query semantic regression tests, independent review and one unchanged focused performance test. Attempt 5's real performance failure, its supplemental observer failure, both diagnostic attempts and A007's initial asynchronous-cleanup observation remain immutable. None is rewritten as a pass or reused as this attempt's final gate result.

Private logs and receipts are under `.semlia/v1-acceptance/current/final-gates-6-Y7mDN2/`, with directories/files at `0700`/`0600`. This attempt reserves a new release output and security image. The exact Go toolchain and clean local Docker boundary remain as specified above. There is no candidate startup, real model request, governance mutation, remote deployment or publication by this worker.

### Complete Gates

Times are UTC on 2026-09-26 (2026-09-27 local date). Every command runs exactly once in the listed order, without changing thresholds, sample counts, concurrency flags or skip conditions.

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| `bash scripts/dev/sync-web.sh` | 21:09:14 | 13.458 s | 0 | Deterministic embedded assets |
| Full packet Node command | 21:09:28 | 79.216 s | 0 | 116 passed, 0 failed, 0 skipped |
| `make check-source` | 21:10:48 | 327.647 s | 0 | All nine stages passed |
| `make check-browser` | 21:16:30 | 163.803 s | 0 | Native assertions and 12 production desktop tests |
| `make check-smoke` | 21:19:16 | 127.038 s | 0 | Complete owned journey; Go package 81.065 s |
| Fresh-tag `make security-check` | 21:21:26 | 10.958 s | 0 | Unchanged dependency, secret and image gates |
| Enabled, uncached performance command | 21:21:37 | 14.212 s | 0 | Both tests passed, 0 skipped |
| Versioned `make release` in the fresh owned output | 21:22:20 | 33.814 s | 0 | Fresh four-piece bundle and executable |
| Existing Go manifest verifier | 21:22:54 | 0.383 s | 0 | Provenance, dependency coverage, archive/staging and checksums verified |
| `git diff --check` | 21:22:55 | 0.059 s | 0 | Passed |

The ordinary source gate reports **64 passing Go packages: 51 cached and 13 actually executed**, plus 17 packages without tests. Catalog, DB, governance, authorization, discovery, ingestion, M1, operations, projection and usage integration packages actually execute, alongside command, repository and catalog-performance packages. The ordinary catalog-performance package retains its benchmark opt-in, so package execution here is not an enabled benchmark claim. `tests/acceptance` is cached; the nested frontend run is not claimed as newly executed. The separate direct frontend invocation reports **40 files / 340 tests passed**. Lint reports zero errors and four existing warnings. The previously documented individual-skip and opt-in limitations remain applicable.

Native browser assertions all pass with zero page errors and successful owned cleanup. The production suite reports **12 passed**, with no filtering or retry. Smoke observation captures one complete set of four containers, three volumes, one network and the image under matching owner/project labels, then stops polling. Independent cleanup verification checks actual container/network IDs, all configured volume names and the actual `_backend` network name, project labels and the image. All are absent. Native and production browser resources are also absent.

The enabled performance command uses the unchanged 10,000-asset fixtures and 25 samples. Catalog page p50/p95 are **9.309334 / 12.223542 ms** against a 150 ms p95 budget; search p50/p95 are **16.406709 / 22.854292 ms** against 250 ms; resolver p50/p95 are **5.251083 / 6.383083 ms** against a strict one-second budget. Neither benchmark is skipped or cached. These are fresh Attempt 6 results, not the diagnostic or focused-test measurements.

Security uses retained image `semlia:security-final-7d7687952d6725f4f60787a99b4f`, ID `sha256:511af1b952af738438442a236fe1e81069efc0ae8b5b4b919fe805b5aa64cd67`. The default `semlia:security` tag is unchanged. Original and current scanner reports remain privately archived; no severity, tool version or skip rule changes.

### RC.4 Identity

The source fingerprint is identical after sync and before/after every subsequent gate:

```text
sha256:a413933ffe5c282a1ea845a31ff730070271b251baeeb1f245c9f01cef92ad86
```

The release command uses exact Go `1.26.6`:

```bash
SEMLIA_VERSION=1.0.0-rc.20260927.4 \
SEMLIA_RELEASE_DIR="$PWD/build/release/v1-rc-20260927.4-7d7687952d6725f4f60787a99b4f" \
GO="$(go env GOROOT)/bin/go" make release
```

Exact four-piece paths:

```text
build/release/v1-rc-20260927.4-7d7687952d6725f4f60787a99b4f/semlia-1.0.0-rc.20260927.4-darwin-arm64.tar.gz
build/release/v1-rc-20260927.4-7d7687952d6725f4f60787a99b4f/semlia-1.0.0-rc.20260927.4-darwin-arm64.sbom.cdx.json
build/release/v1-rc-20260927.4-7d7687952d6725f4f60787a99b4f/SHA256SUMS
build/release/v1-rc-20260927.4-7d7687952d6725f4f60787a99b4f/staging/run.k5OPy7/semlia-1.0.0-rc.20260927.4-darwin-arm64/release.json
```

The executable is `semlia` beside the staged manifest, 46,779,442 bytes, SHA-256 `7142017faffe5032511cf73e5ca140ae54d0c30e17ea3092b28bd26988476b48`. Manifest SHA-256 is `8279a8c1aa2652b0554dcb33c3c847cc93748ec9061573050178e38c93e272db`; archive SHA-256 is `63c7086c5c4d283ca4b228ff04acc44d89e2c495913799b1a0f29c0e1d4e9681`; external SBOM SHA-256 is `67b64cc2a897aa7ce3c6bb53866b8d421175adeccf22f2d1d0dbc8c9da22ca74`; checksum-file SHA-256 is `41841205740563ea7a43e90282efb76d52e091aa746af25d9dc9868fd8820dbf`.

The manifest records version `1.0.0-rc.20260927.4`, base commit `2bf50c33c18cda1d56b01de143841790443e891a`, target `darwin/arm64`, migration **33**, API `v1`, content schema `0.9.0`, exact `go1.26.6`, `sourceDirty: true`, `artifactKind: local_candidate` and `acceptance: unreviewed`. The CycloneDX 1.6 SBOM contains 92 components, including 79 Go and nine npm components. Nonexecuting Go build-info inspection verifies the module, toolchain, target, CGO disabled, trimpath and VCS revision. It does not infer runtime version from unavailable linker-flags metadata or start the executable.

### RC.4 Preservation

All **11** original-environment comparisons pass: daemon identity; the original 18 containers, 72 volumes and 22 networks; recorded private/Compose configuration and knowledge/cohort files; 15 files across the three older candidate artifact sets; 386 files across prior gate and A007 diagnostic evidence; active RC.3 runtime identity; original native stopped state; default security tag; and existing evidence directories. Hashes and recorded permissions, identities and timestamps are unchanged. Initial final inventory already matches, with no retiring reaper present, so the bounded settlement mechanism performs zero waits. Initial, settlement-summary and final receipts are retained separately.

All actual-ID, configured-name, label and image cleanup checks pass, including scanner-container absence. The only retained new outputs are the owned candidate, owned security image, private logs and synthetic browser evidence. All execution sessions end and the heavy local window is released. Root receives `artifact-paths.json`, `09-identity.json`, `gate-summary.json`, `after-initial.json`, `after-settlement.json` and `after.json` for independent exact-binary acceptance; this worker's evidence does not substitute for those pending candidate-specific checks.

## Attempt 7

Root authorizes a fresh complete serial gate run after the bounded cold catalog unknown-state repair and independent review. The release target is `1.0.0-rc.20260927.5`. Private logs and receipts are under `.semlia/v1-acceptance/current/final-gates-7-0INv4I/`, with directories/files at `0700`/`0600`. Exact Go `1.26.6`, the fixed local daemon and a fresh anonymous Docker configuration remain in use. All earlier candidates, model cohorts and failed attempts remain immutable.

Times are UTC on 2026-09-27. Every listed command runs once, without filtering tests, retries or threshold changes.

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| `bash scripts/dev/sync-web.sh` | 12:48:58 | 5.848 s | 0 | Embedded assets generated once, then source frozen |
| Full packet Node command | 12:49:09 | 64.738 s | 0 | 116 passed, 0 failed, 0 skipped |
| `make check-source` | 12:50:18 | 244.270 s | 0 | All standard source stages passed |
| `make check-browser` | 12:54:31 | 150.246 s | 2 | Native assertions passed; production 11 passed, 1 failed |

The source gate reports **64 passing Go packages: 60 cached and four actually executed**, plus 17 packages without tests. The executed packages are `cmd/semlia`, `internal/platform/web`, `tests/acceptance` and `tests/repository`. The acceptance package executes its nested frontend contract, but successful Go output does not expose a separate nested test count. The direct frontend invocation reports **40 files / 350 tests passed**. Lint reports zero errors and four existing warnings. Ordinary Go output does not enumerate all individual opt-in skips; no claim of zero Go skips or enabled performance execution is made.

The failed production case is **`[compact-desktop] isolated identity and server-owned production workspace`**, at `web/e2e-production/production.spec.ts:12`. After navigating to `/assets?section=drafts`, the draft version-filter button remains `aria-pressed="false"` instead of the expected `"true"` for the 20,000 ms assertion window. The suite reports **11 passed / 1 failed** in 1.8 minutes. This records the observed routing/filter-state failure, not an inferred resource or model failure. The unmodified raw log, error context and screenshots remain private under `.semlia/production-acceptance/spacc_efeaa6f6867c6397/`.

Native validation passes all assertions, with zero page errors and cleanup `{ owned: true, cleanupFailed: false }`. The standard browser gate exits 2 and the private fail-stop wrapper exits 1; neither is rewritten. Smoke, security, enabled performance, release, manifest verification and the final release diff gate are **not reached**. The reserved release directory contains only its ownership marker, not an RC.5 archive, SBOM, manifest or executable. The reserved security tag is not built.

### Source and Preservation

After the single sync, every completed gate retains the same source fingerprint:

```text
sha256:dea735585a9c034646ff1b29acbecec299a10b4bdab19060839ff99225731dbe
```

The separate first `after-stopped.json` exits 0 with all **12** preservation comparisons true. It verifies the original 18 containers, 72 volumes and 22 networks; 62 protected files including private configuration, all three model cohorts, value-knowledge receipts and delegated-browser evidence; 20 files across all four old candidate artifact sets; 434 prior gate/diagnostic evidence files; active RC.4 identity; stopped original native state; unchanged default security tag; and prior evidence directories. Recorded hashes, identities, permissions and timestamps are unchanged. Read-only counters remain schema 33, **34 model steps / 34 agent runs / 32 executions / 0 unfinished**. No settling reaper is present and no wait or manual resource removal is needed.

Independent cleanup checks find no native owner-labelled containers or native owner/control files, and no production project-labelled containers, volumes or networks. The configured production volume and network names are absent from the full inventories. The private `04-browser-failure-evidence.json` retains these cleanup facts independently of the failed product assertion.

All command sessions end after the browser failure. No failed command is retried; no model, candidate lifecycle, governance mutation, Git write or remote publication is performed. The heavy execution window is released for root diagnosis.

### Reviewed Test-Only Continuation

Root reviews a correction to the first production-browser test: an empty catalog retains the usable draft filter, while a nonempty catalog with unknown publication summaries must show all rows, disable draft/published filters and avoid false unpublished/readiness claims. The production behavior does not change. Root separately accepts the author's lint/type/diff checks and explicitly authorizes one named corrected browser run followed by the first execution of gates 05-10. This is not an unreviewed rerun of the failed test or a fresh all-same-fingerprint gate run.

The private `continuation-before.json` directly captures all **740** formal fingerprint inputs before the test edit; its recomputed hash equals the original frozen `dea735...` value. `continuation-bridge.json` confirms the same path set and exactly one changed file, `web/e2e-production/production.spec.ts`. That file's SHA-256 changes from `42a2b8969dfc3e379fdb5432ba4230173c5cf2a015057a67028f8e31e01cd564` to `99136c8be1d1d20ca0c2d74a5d67feaedd45e120489ec3879258c0d79119849b`. All other file hashes are identical. The original freeze, failed-browser metadata, wrapper failure and stopped-after receipt also retain their recorded hashes.

The release fingerprint includes browser tests, even though this test file is not part of the Vite executable entry graph, TypeScript build include or Vitest `src` test include. Consequently, the official Go fingerprint correctly changes to:

```text
sha256:252f14bdd557730542d4c268333741c94f4a3d85dca77cf0d18eba2814e139fa
```

This new fingerprint is fixed in `continuation-freeze.json` and matches before/after every continuation gate and the final release manifest. Sync, Node and full source tests are not repeated; their original evidence is retained and linked through the reviewed single-file bridge. The old `04-check-browser` failure is not overwritten by `04-check-browser-corrected`.

Times below are UTC on 2026-09-27.

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| Named corrected `make check-browser` | 13:11:16 | 328.566 s | 0 | Native assertions and 12/12 production cases |
| First `make check-smoke` | 13:17:05 | 161.365 s | 0 | Full owned journey; Go package 86.552 s |
| First fresh-tag `make security-check` | 13:21:17 | 42.548 s | 0 | Unchanged dependency, secret and image gates |
| First enabled, uncached performance command | 13:22:20 | 18.815 s | 0 | Both original benchmarks passed, neither skipped |
| First versioned `make release` | 13:23:06 | 41.630 s | 0 | New RC.5 four-piece bundle and executable |
| Existing Go manifest verifier | 13:24:14 | 0.463 s | 0 | Archive, provenance, dependencies and checksums verified |
| `git diff --check` | 13:24:25 | 0.053 s | 0 | Passed |

The corrected browser suite reports **12 passed in 3.6 minutes**; native assertions all pass with zero page errors and successful owned cleanup. There is no test filter or automatic retry. Smoke captures a complete owner/project-labelled live snapshot once, then independently verifies actual container/network IDs, configured volume/network names, project labels and image absence. Browser and scanner resources are also absent.

The unchanged enabled `-count=1` performance command uses 10,000 assets and 25 samples. Catalog page p50/p95 are **21.981167 / 26.270542 ms** against a 150 ms p95 budget; search p50/p95 are **35.500458 / 39.588416 ms** against 250 ms; resolver p50/p95 are **12.638250 / 23.442916 ms** against a strict one-second budget. Both tests execute without cache or skip. Ordinary source-gate cache and skip limitations remain as recorded above.

Security retains only the fresh owned image `semlia:security-final-b78d99678745327248f9359248b5`, ID `sha256:048bff5c8656f9d3e18b694b0a641116f63481390142e2cca1033967962b9df9`. The default security tag is unchanged. New and archived scanner reports remain private at `0600` under `0700` directories. No scanner threshold, tool version or skip rule changes.

### RC.5 Identity and Preservation

The fresh release command uses exact Go `1.26.6`, version `1.0.0-rc.20260927.5` and owned output `build/release/v1-rc-20260927.5-b78d99678745327248f9359248b5`. Exact four-piece paths are:

```text
build/release/v1-rc-20260927.5-b78d99678745327248f9359248b5/semlia-1.0.0-rc.20260927.5-darwin-arm64.tar.gz
build/release/v1-rc-20260927.5-b78d99678745327248f9359248b5/semlia-1.0.0-rc.20260927.5-darwin-arm64.sbom.cdx.json
build/release/v1-rc-20260927.5-b78d99678745327248f9359248b5/SHA256SUMS
build/release/v1-rc-20260927.5-b78d99678745327248f9359248b5/staging/run.4LNO5z/semlia-1.0.0-rc.20260927.5-darwin-arm64/release.json
```

The executable is `semlia` beside the staged manifest, 46,779,442 bytes, SHA-256 `1cecf1dd952ea38d115c66383e0f274466a125afbff7c13e9ebac998018b37b4`. Manifest SHA-256 is `4a195f6d61715f51b6988b740c1e4334db0da76c2cb0873b26fa8acdbc020f2b`; archive SHA-256 is `293fb61d1d8c6e1bc5db15e9b1a0e83f0382a041b57a9a403e74bb2cbaf8f959`; external SBOM SHA-256 is `a2a2446066c9395386150682f064fde197d5d9c355fcdd0d657cb55864d23363`; checksum-file SHA-256 is `852a4cfb82847c377ee2361d98d78d7cdc1d063947b7b306532f883d645bce86`.

The manifest records base commit `2bf50c33c18cda1d56b01de143841790443e891a`, `darwin/arm64`, migration **33**, API `v1`, content schema `0.9.0`, exact `go1.26.6`, `sourceDirty: true`, `artifactKind: local_candidate` and `acceptance: unreviewed`. The CycloneDX 1.6 SBOM contains 92 components, including 79 Go and nine npm components. Nonexecuting build-info checks verify module, toolchain, target, disabled CGO, trimpath and VCS revision; runtime version verification remains a separate root-owned check. The Linux smoke/security image is a same-source check, not this Darwin executable.

The separate `after-final.json` compares against the **original Round 7 before snapshot**, not a weakened continuation baseline. All **12** comparisons and all **10** smoke/scanner cleanup assertions pass. The original 18 containers, 72 volumes and 22 networks, 62 protected files, 20 files across all four old candidate sets, 434 prior gate/diagnostic files, active RC.4 executable and stopped original native state are unchanged. Read-only counters remain **34 model steps / 34 agent runs / 32 executions / 0 unfinished**. Default security identity and previous evidence remain intact. Initial final inventory already matches, so settlement records no retiring reaper and zero waits. Both the initial failed phase and successful continuation retain their own cleanup and preservation receipts.

All execution sessions end and the heavy local window is released. Root receives `artifact-paths.json`, `09-identity.json`, `gate-summary.json`, the before/bridge/new-freeze receipts and `after-final.json`. This worker does not start RC.5, call models, write governance state, commit, push, sign or publish. Candidate-specific and delegated-browser acceptance must use these exact new artifact bytes.

## Attempt 8: Frontend-Only RC.6

Root explicitly authorizes a risk-calibrated follow-up after the A006 search-membership repair and independent review. Private receipts and raw logs are under `.semlia/v1-acceptance/current/final-gates-8-Bs4TW1/`, with directories/files at `0700`/`0600`. The five older candidates, three historical model cohorts, failed glossary preparation, separate reconciliation and all RC.5 startup/browser failure evidence remain preserved. This worker performs no model, governance, candidate lifecycle or Git operation.

### Proof Boundary

The saved RC.5 before receipt contains all **740** formal fingerprint inputs and exactly matches RC.5's final `252f14...` fingerprint. The pre-sync bridge finds only these five approved changes:

- `web/src/ProductApp.tsx`
- `web/src/Catalog.test.tsx`
- `web/src/ProductApp.test.tsx`
- `web/src/testing/catalogFixture.tsx`
- `web/e2e-production/production.spec.ts`

The pre-sync fingerprint is `sha256:c2bf60e18766cd07818a856f75a27936d04a41749b2ed23380e257438848105c`. One explicit sync then replaces the JavaScript bundle and updates embedded `index.html`; the CSS is unchanged. The post-sync bridge permits only those generated asset differences in addition to the five approved files. The final official Go fingerprint, unchanged before/after all subsequent fresh gates, is:

```text
sha256:8bdaf67c031c83f8adf9fa6ce3a4e1c0f6d9b491315f3c41dab62b9b4a72e263
```

Within the formal input set, all other Go, generated SQL, model, driver, API and dependency inputs are unchanged. The official fingerprint does **not** include top-level `db/` or `tests/`; this evidence does not claim those source trees are covered by that 740-file hash proof. The executed SQLC output is unchanged, and Attempt 7's SQLC drift gate passed. Both benchmark definition files additionally match the fixed `2bf50c33c18cda1d56b01de143841790443e891a` Git blobs, with no tracked or untracked performance-test changes.

Root explicitly accepts inherited Attempt 7 results: **116 Node tests, zero failures/skips**; the full source gate's **64 Go packages, 60 cached and four executed**, plus 17 no-test packages; and the actually enabled, uncached 10,000-asset/25-sample performance tests. Their original source fingerprints and receipts remain intact. Inherited p95 values are page **26.270542 ms**, search **39.588416 ms**, resolver **23.442916 ms**, within the unchanged 150 ms / 250 ms / one-second budgets. These are **not fresh RC.6 measurements**, and ordinary Go opt-in/individual-skip limitations remain applicable. Full source CI is required again on the committed PR tree.

The repair author separately runs fresh frontend validation, accepted by root and independent review: focused **100 passed**, full **40 files / 356 tests passed with zero skips**, lint **zero errors / four existing warnings**, typecheck and scoped diff check both zero. Exact commands, RED results and coverage are in the [A006 evidence](../V1-T003/catalog-search-membership.md). The release worker does not count those as a second frontend run.

### Fresh Gates

Times are UTC on 2026-09-27. Each listed gate runs once without retries, filters or changed thresholds.

| Gate | Start | Duration | Exit | Result |
| --- | --- | --- | --- | --- |
| `bash scripts/dev/sync-web.sh` | 13:41:58 | 9.822 s | 0 | Generated assets and post-sync hash bridge |
| `make check-browser` | 13:42:35 | 150.183 s | 0 | Native assertions and 12 production cases |
| `make check-smoke` | 13:45:37 | 120.643 s | 0 | Complete owned journey; Go package 75.187 s |
| Fresh-tag `make security-check` | 13:48:43 | 16.013 s | 0 | Unchanged dependency, secret and image gates |
| Versioned `make release` | 13:49:46 | 34.636 s | 0 | New RC.6 four-piece bundle and executable |
| Existing Go manifest verifier | 13:50:49 | 0.247 s | 0 | Provenance, dependency coverage, archive/staging and checksums |
| `git diff --check` | 13:51:01 | 0.032 s | 0 | Passed |

Native browser assertions all pass with zero page errors and successful owned cleanup. Production reports **12 passed in 1.6 minutes**, including the new strict qualified-address search assertion after real publication at both supported desktop sizes. No assertion is filtered out or automatically retried. The protocol-stub production test is not a new real-provider model cohort.

Smoke captures one complete live ownership snapshot and then stops polling. Independent checks confirm actual container/network IDs, configured volume/network names, project labels and image absence. Native/production browser resources and scanner containers are absent. Security retains the fresh owned image `semlia:security-final-31c372dd31167fe6fd5f7d39f7a3`, ID `sha256:dbf752af8928688c24ea2fe4ee31d5f2b611a38b2281adec221cbe135a30db3e`; the default tag is unchanged. All original and new security reports remain private.

### RC.6 Identity

Exact four-piece paths are:

```text
build/release/v1-rc-20260927.6-31c372dd31167fe6fd5f7d39f7a3/semlia-1.0.0-rc.20260927.6-darwin-arm64.tar.gz
build/release/v1-rc-20260927.6-31c372dd31167fe6fd5f7d39f7a3/semlia-1.0.0-rc.20260927.6-darwin-arm64.sbom.cdx.json
build/release/v1-rc-20260927.6-31c372dd31167fe6fd5f7d39f7a3/SHA256SUMS
build/release/v1-rc-20260927.6-31c372dd31167fe6fd5f7d39f7a3/staging/run.LJDvEI/semlia-1.0.0-rc.20260927.6-darwin-arm64/release.json
```

The executable is `semlia` beside the staged manifest, 46,779,442 bytes, SHA-256 `a513a5d379042877e0e2e53bf60a5bb3e74bd36bb565018695c22ea768703b6e`. Manifest SHA-256 is `3bcf4fd1bf26f0beb39c9b6e5075a6031d759c00ff2b6e90ad8e8e93a4833fda`; archive SHA-256 is `ea6ee90f553611ea5182acce1b6bf4bbbea355bcade0ddf6d5bda276b1a87945`; external SBOM SHA-256 is `c7b4d5e1dd0057ecf1561494c456783f9d0029c823e55f40a7e595b68cd072f6`; checksum-file SHA-256 is `49a17d8ac7883d8d2b2f0e8511aefb2941ddd212664f58ac3868ee343f3e9ecb`.

The manifest records version `1.0.0-rc.20260927.6`, base commit `2bf50c33c18cda1d56b01de143841790443e891a`, `darwin/arm64`, migration **33**, API `v1`, content schema `0.9.0`, exact Go `1.26.6`, `sourceDirty: true`, `artifactKind: local_candidate` and `acceptance: unreviewed`. The CycloneDX 1.6 SBOM contains 92 components, including 79 Go and nine npm components. Nonexecuting build-info checks pass; startup/version remains a separate root-owned verification of these exact bytes. The Linux security/smoke image is not the Darwin candidate executable.

### Preservation and Limits

All **12** preservation comparisons and **10** owned-resource cleanup assertions pass. They cover the original 18 containers, 72 volumes and 22 networks by exact identity/state, 76 protected files, 25 files across five older candidate artifact sets, 495 prior gate/diagnostic files, retained RC.5 runtime, stopped original native state, default security tag and evidence directories. Read-only counters remain schema 33, **34 model steps / 34 agent runs / 32 executions / 0 unfinished**. No known retiring test reaper remains, and settlement performs zero waits.

Preflight also observes one unrelated external Compose container and network, making the whole-machine totals 19/72/23. Root explicitly classifies them as external/user work to observe without touching. During this attempt that external container is replaced under the same name and is observed exited afterward; its network identity remains the same. Full private observations retain both identities. This does not change any original Semlia preservation assertion, and the evidence does not claim the entire machine stayed unchanged. No external resource is started, stopped, removed or adopted by this worker.

All execution sessions end and the heavy window is released. The exact paths, manifest identity, fresh gate results, explicitly inherited results, both hash bridges and final preservation are in `artifact-paths.json`, `09-identity.json`, `gate-summary.json`, `bridge-pre-sync.json`, `bridge-post-sync.json` and `after-final.json`. Root must use the new RC.6 bytes for candidate startup and browser/model acceptance before the authorized Git handoff. Historical failures, previous configuration disclosure and the requirement to rotate credentials before production remain unchanged.

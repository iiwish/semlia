# FMB-T007 A002 Verification

Status: Source, security, smoke and candidate archive gates pass. Auth desktop5/6;
the outstanding contrast finding prevents complete acceptance.

## Root Commands

| Command | Log | Outcome |
| --- | --- | --- |
| `make web-embed` | `root-web-embed.log` | PASS, exit 0, before Go compilation |
| `GOFLAGS=-p=1 make check-source` | `root-check-source.log` | Interrupted, exit 130 after confirmed OpenAPI and acceptance failures; not a complete suite result |
| `make web-embed` after fixes | `root-web-embed-final.log` | PASS, exit 0 |
| `GOMAXPROCS=2 VITEST_MAX_WORKERS=1 GOFLAGS=-p=1 make check-source` | `root-check-source-final.log` | Exit2: all Go integration packages pass; acceptance hits Web and package wall-clock timeouts |
| `go test -p 1 -count=1 ./tests/acceptance` with scheduler caps | `root-acceptance-rerun.log` | PASS, exit0, uncached 186.411s |
| Web Vitest with one worker | `root-web-tests-final.log` | PASS, 22files/183tests, 68.30s |
| `make contracts-check` / `make db-generate-check` | `root-contract-drift.log` / `root-sqlc-drift.log` | Both PASS |
| `make security-check` | `root-security.log` | PASS, exit0; filesystem/secret and production image HIGH/CRITICAL: zero findings |
| `make dev` | `root-preview-upgrade.log` | PASS, actual migration20-to-21, server/worker healthy, volumes preserved |
| `make smoke` | `root-smoke.log` | PASS, exit0, 21.050s |
| Fixture Playwright, both desktop sizes | `root-fixture-desktop.log` | PASS, 26/26, 32.2s |
| Live distribution Playwright, both desktop sizes | `root-live-desktop.log` | PASS, 2/2, 5.8s |
| Actual release with explicit Go1.26.6 | `root-release.log`, `root-release-final.log`, `root-release-verified.log` | Failed attempts retained: Go PURL case, missing production Web dependencies, archive input path respectively; final rerun required |
| Single-command source gate after dual scanner | `root-check-source-verified.log` | Exit2: repository fixture expects one Docker invocation, actual four; per-invocation validated label behavior remains enforced |
| Final single-command source gate | `root-check-source-closure.log` | PASS, exit0: format/lint/types, all tests, contract/SQL/embed drift and build; already-running verification finishes after the implementation window |
| Actual release/archive after AppleDouble exclusion | `root-release-archive-final.log` | PASS, exit0: strict Go/Web dependency, source/staging provenance, archive consistency and official schema validation |
| Auth desktop first/final | `root-auth-desktop.log`, `root-auth-desktop-final.log` | First 2PASS/4FAIL: obsolete fixture lacks authoritative capabilities. Final 5PASS/1FAIL: compact-desktop serious color contrast 4.31:1 on member-state-active against required4.5; assertion retained |

The first root source attempt passed formatting/lint/type checks, then found a `$ref`
description sibling violation, a LiveSources focus test failure and four launcher
wall-clock failures under load. Its isolated acceptance subprocess ran Vitest without
a worker cap. The final command caps Vitest and Go scheduling; no launcher test deadline
or production execution budget is weakened. The OpenAPI reference is repaired and its
focused contracts pass independently.

## Focused Evidence

- `execution-repeat-final.log`: two successful repetitions of the real execution,
  channel, process-loss, cancellation, pin and rollback proof at that source checkpoint.
- `live-ask-shell-bounded.log`: real isolated AskView/service/PostgreSQL, disclosed
  deterministic Chat, both desktop sizes, keyboard, reduced motion, Axe, reload privacy
  and composer/execution non-overlap. These screenshots are not production OIDC proof.
- `machine-current-cancel-green.log`: the new current-binding cancellation subtest
  passes; the combined run fails in an older short-budget process-loss fixture.
- `final-execution-default-budget.log`: actual source-lock timeout/caller cancel and
  process kill/deadline replay reach their assertions; the combined run fails at the
  SDK probe and exits 1 after 315.990 seconds, including delayed container cleanup.
  Safe SDK error-code diagnostics are added; the cause requires final verification.
- `contracts-ref-sibling-green.log`: contracts and distribution pass after the `$ref`
  repair. The subsequent focused release tests also pass.
- `migration21-lifecycle-pass.log`: empty 21-to-20-to-21 and fail-closed populated
  downgrade with preserved pins and an honestly retained dirty marker.
- `migration21-legacy-pins.log`: a schema20 legacy binding cannot borrow a schema21
  sibling binding's physical provenance.

Consult `implementation.md` for earlier RED/GREEN commands and the original historical
publication HTTP422. Logs whose filenames contain `green` can still have an overall
failure. The actual exit and named assertion determine their evidence strength.

## Acceptance Boundaries

External hosted acceptance is not supplied; see `hosted-matrix.md`. No skipped or
interrupted gate is presented as passing. The final source and tar gate results are
recorded separately from earlier focused successes.

## Runtime Identity

- Development image: `sha256:9ac094fe853693eeb6e356ecfe8865adcff4d0c1d2067db9427945539716562b`.
- Scanned production image: `sha256:41ccf2d65ae7d5074db66237d8b8c6c23634f5a14ef8b4636654178ada4f56d9`.
- Actual preview `/health/ready`: HTTP200 ready. Database schema_migrations: `21|f`.
- Actual `/api/v1/system/info`: buildVersion `local-2e53e6f`, schemaVersion `0.9.0`.
  The configured build label is stale; schemaVersion is the API field, not the SQL
  migration number. Neither is presented as the candidate tar identity.
- The production image, launched separately without identity configuration, returns
  session HTTP503 and renders an inaccessible-workspace login state, without UAT
  fallback. The isolated probe container is removed; user preview is retained.

## Verified Local Artifact

Actual `make release` passes with Go1.26.6, migration21, production UAT0, version
`0.2.0-beta.1-a002`, HEAD `source-revision-redacted`, sourceDirty=true,
artifactKind=local_candidate, acceptance=unreviewed. The exact candidate source digest
is `sha256:8e09fdd8fea7a2d99e9a5ca4cb13e568d921013bc395d1c44a755f684cb2f070`.

Archive SHA256: `5917f839d9f68bea6e1015bc46b63975a537e791b6f501aa8c73b0ec8f6c6b55`.
External SBOM SHA256: `a2bf29bd5c31a25666080e6a8e69abe5f595eb93f61ba8d242ec4ed3278716fe`.
`shasum -a 256 -c SHA256SUMS` passes both. Root starts the actual staged binary on
isolated localhost18090 with a clean environment: `/api/v1/system/info` returns
buildVersion `0.2.0-beta.1-a002`. The probe exits0 on SIGINT and is not left running.
The candidate does not include subsequent test-only fixture changes; runtime and
release source remain frozen. No signature, hosted deployment or founder acceptance
is inferred from a locally verified tar.

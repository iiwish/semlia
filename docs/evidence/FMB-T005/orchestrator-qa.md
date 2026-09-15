# T005 Orchestrator QA

Status: Needs_Review. This record contains actual local deterministic evidence, not hosted or live-model acceptance.

## Environment

- An isolated `pgvector/pgvector:pg17` container, `semlia-sprint-embedding-qa-20260905`, exposes PostgreSQL only on `127.0.0.1:55440`.
- Database `semlia_embedding_qa` uses a generated password in an ignored mode-600 file. The existing PostgreSQL 18 development volume is untouched.
- Native server/worker use the same isolated database and an explicitly local-UAT Web build on `http://127.0.0.1:18083`.
- `deterministic-provider.mjs` listens only on `127.0.0.1:18084`; responses are delayed five seconds and contain synthetic two-dimensional vectors. This proves transport and persistence, not semantic quality or an external provider.
- Workspace `wsp_01m1qw7pq6ec49wfveresbbx21` / UUID `01a06fc3-dae6-7308-9e3f-6ec3b2b5f441` is created through the real API. Provider and default model `qa-deterministic` are also created through the real API as `local-author`.
- The synthetic released corpus is inserted only into this isolated QA database using `ui-fixture.sql`; it is not evidence of the governed publishing journey.

## Actual Attempts

- The first fixture import fails the UUID-version constraint. The second fails the SHA-256 digest constraint. Both transactions roll back; neither is recorded as a pass. The corrected fixture commits one released asset and its revision and release membership.
- The initial QA Web build uses an incorrect environment-variable name and displays the unavailable identity dependency. Rebuilding with the repository's `VITE_LOCAL_UAT_IDENTITIES=1` flag restores the explicit local-UAT entry; production authentication is not bypassed.
- Migration 19 applies to the fresh database, and the explicit pgvector-enable script commits successfully.
- Real API rebuild `run_01m1qwcjvhfctve0m2bn5w1gc8` starts at 12:14:21 Asia/Shanghai with one chunk and zero vectors, and activates at 12:14:26 with one vector.
- A real `/embedding-search?q=revenue` request returns `mode=vector`, asset `ast_0g00000000e008000000000001`, exact released revision `rev_0g00000000e008000000000002`, address `qa.revenue`, and a finite score. The provider transport is the synthetic fixture above.
- At 1440x900, the real UI opens the confirmation dialog: Tab from the primary action wraps to the close button; Escape closes the dialog and restores focus to the rebuild trigger.
- With the isolated worker stopped, the UI creates `run_01m1qwm23pffs919ftrcqssx73`. The stored vector checkpoint is reused (1/1), but state remains building until activation. Hard reload preserves the same generation. UI cancellation changes it to cancelled while the earlier active generation remains available.
- At 1024x768, the UI creates `run_01m1qwpt6affvs5spxg247ap0z`. The server is gracefully stopped, the isolated QA PostgreSQL container is restarted, and server/worker resume. The same generation activates at 12:20:22; hard reload shows its exact ID, active state and 1/1 checkpoint. This is graceful queued-work recovery with reused vectors, not an in-flight SIGKILL or multi-batch crash claim.
- The real UI search returns `qa.revenue` and its exact released revision using `mode=vector`. The Operations link opens the exact recovered generation and job with persisted queued/running/succeeded events.
- Both supported desktop widths have `document.documentElement.scrollWidth == innerWidth`; screenshots show legible IDs, controls and no overlap. Screenshots are retained in `screenshots/`.
- The first final source gate stops at the new panel's `react-hooks/set-state-in-effect` lint error. The worker fixes scheduling without disabling the rule and supplies a passing StrictMode regression. A full gate repeat is required.
- The next full source gate exposes three migration-test expectation failures: the new workspace FK is absent from the expected inventory, and two migration-18 tests step down once from version 19 but assert version 17. The inventory is extended and both tests explicitly move 19-to-17 while retaining every 17-to-18-to-17-to-18 data assertion.
- The final `GOFLAGS=-p=1 make check-source` passes: PostgreSQL 17/18 database package 26.315 s, all Go packages, Web 19 files / 167 tests, contract/sqlc/embedded-Web drift and binary build. The lint gate reports zero errors, one existing KnowledgeViews warning and two embedding ref-cleanup warnings.
- `make security-check` passes using its configured local Dockerfile image `semlia:security` (`sha256:2ae254c3aeba5bff37d6de80f56256656ab85b714117163aba4f492fd2ef9d57`). Trivy reports no HIGH/CRITICAL dependency or container vulnerabilities and no blocking secret finding. This is the repository's scanner gate, not hosted security acceptance or a production authentication proof; the scanned local Dockerfile enables local-UAT Web controls.
- A second real workspace, `wsp_01m1qx056afc69d6xrv9j32hfr`, has no configured model. Its UI displays an explicit unavailable state and a disabled rebuild action, with no previous workspace index or search results retained.
- The final dimension-form correction has passing validity regressions for 2, 4,096 and 4,097. A complete source-gate repeat passes 20 Web files / 170 tests and all Go, migration, drift and build gates. The fresh local-UAT binary passes the real two-desktop embedding E2E again (2 tests, 13.1 s).
- The real UI edits the existing two-dimensional model's capability to `synthetic transport QA validated` and saves successfully. The API independently returns that persisted capability and `embeddingDimension=2`.
- The complete fixture E2E suite passes 26 tests in 31.9 s. The current Docker stack at migration 19 passes real source E2E (4 tests, 10.7 s) and `make smoke` (18.714 s).
- The final development rebuild starts successfully and serves `/health/ready` as ready on port 18081. Its image is `sha256:4e5917579a7a4edd2e3a797d1b77b7afbb6d39632b07e0e516a36d8790788ffa`. Re-running the security script against this exact `semlia:local` image also passes with no HIGH/CRITICAL finding. Standard embedded assets use the production-default build flag; only local development and isolated QA binaries expose explicit local-UAT controls.

## Review Findings

The independent reviewer confirms a workspace/job lock-order inversion, slow-response polling starvation, missing dialog focus containment, and stale identity/authorization UI state. Fixes and regressions are present. Follow-up review identifies and then closes a StrictMode aborted-request deadlock in the polling fix. Final read-only review and the dimension-form regression close the confirmed implementation blockers. Local integrated validation passes; external model, recall quality, scale and hosted release acceptance remain open.

The local-UAT author initially has no `workspace.manage` Web capability despite the server alias resolving to the existing `workspace_admin` role. The packet permits adding only this already-granted development capability; reviewer/publisher separation and production identity remain unchanged.

## Final Environment State

The isolated server on 18083, worker and synthetic provider on 18084 are stopped. Container
`semlia-sprint-embedding-qa-20260905` is stopped, not removed. Its volume
`fb9c6deba1fa9c4751e916b6cd30f38884fd0f3d04b8a972d40408231bd609f2` and ignored
`.semlia/sprint-embedding` runtime/configuration files are preserved. The normal development stack
continues on port 18081 at migration 19. Temporary browser viewport overrides are cleared.

To resume this synthetic QA, start only its named container, run the documented deterministic
provider, and use `bash .semlia/sprint-embedding/run.sh server` and `worker` in separate processes.
The two existing QA workspaces and released fixture must not be imported again without a fresh
isolated database. These helpers do not configure a hosted or real external provider.

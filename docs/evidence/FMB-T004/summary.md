# FMB-T004 Delivery Evidence

Status: Needs_Review. Shared-file ownership is handed off after source, migration, desktop and independent review gates pass on 2026-09-05.

## Implementation

The Sources workspace consumes typed, paginated production APIs for PostgreSQL and immutable file/SQL/dbt sources. Users can import supported files, register SQL under the configured server root, inspect exact artifact sets, replace source artifacts with optimistic source versions, launch discovery and inspect persisted candidates. Authorized empty workspaces expose the product shell rather than blocking first-source creation.

The automation workspace manages persisted five-field Cron schedules, IANA timezones, misfire policy, pause/resume, versioned edits, deletion and run-now occurrences. Owning Operations paths are returned by the server. Unsupported or failed backend operations remain visible errors, not fixture success.

The sprint also closes independently identified storage and scheduler defects: digest replay descriptors are closed; all permitted OOXML parts are checked for formulas; stale spool cleanup is bounded and excludes locked writers; scheduled transactions use the same workspace-before-schedule lock order as user mutations. Known retryable PostgreSQL failures retained at the schedule repository boundary do not terminate all worker lanes.

UI retry attempts retain command keys and staged artifact identities after lost responses. Active-run polling preserves loaded pagination and dialog focus; terminal transitions merge newly persisted candidates without erasing existing results.

Artifact, schedule and discovery commands use the canonical actor from the successful authorization decision, including explicitly enabled local-UAT aliases. Repeated discovery of an exact source revision and adapter version has independent run history while reusing one canonical projection without duplicating candidates. Failed projections cannot be reused, concurrent publishers are serialized, and migration rollback refuses to erase reuse history.

The local image pre-owns its artifact volume mountpoint for the non-root runtime user. Local readiness verifies actual write/sync/cleanup capability using an exclusive non-business probe instead of treating directory existence as writable storage.

## Verification

Detailed RED/GREEN command records and inherited-worktree boundaries are in [the sprint attempt](attempts/sprint-a001.md). Gate results are maintained in [test results](test-results.md).

- The latest full sequential Web suite passed 161 tests; targeted behavior regressions also passed.
- Current Web type checking passes. Lint passes with the repository's existing KnowledgeViews fast-refresh warning; no assertion or timeout is weakened.
- Storage/parser/application/command tests pass, including replay/publication descriptor checks, unreferenced-sheet/table formulas, spool recovery and scheduler retry behavior.
- The PostgreSQL lock-order barrier passes after reproducing the original inverted lock with SQLSTATE `55P03`.
- OpenAPI and database generation drift checks pass. Windows artifact-store build and test cross-compilation pass.
- Supplemental native PostgreSQL 16 ingestion suite passes, including canonical projection concurrency, failure/retry, reuse-path lease rollback, exact Operations identity and rollback-history protection. Both desktop live journeys and root-controlled worker/server/database restart pass without duplicated candidates.
- Formal PostgreSQL 17/18 migration and PostgreSQL 18 integration suites pass. Current Docker source journeys pass all four desktop cases, and a fresh named artifact volume initializes for the nonroot application identity without manual permission changes.

## Acceptance Boundary

This evidence does not mark FMB-T004 Accepted. PostgreSQL 17/18 migration round-trip, PostgreSQL 18 integration and real Docker desktop journeys pass independently of supplemental native PostgreSQL 16 recovery proof. The final complete source gate passes after the writable-readiness and image-volume initialization refinement, including 161 Web tests and fresh PostgreSQL 18 ingestion verification. Earlier environment failures and real deployment defects remain in the test and attempt records.

The cumulative allowed-boundary snapshot in `diff.patch` and `cumulative-files.txt` includes existing Alpha/FMB work plus this task's current changes. It is not a claim that all 116 files were authored during the sprint. The attempt record identifies this worker's specific implementation and fixes; no pre-existing work is reverted or committed.

The live CSV/schedule Playwright journey is implemented in `web/e2e-live/source-live.spec.ts` and uses only real API responses. Screenshot targets are 1440x900 and 1024x768. Hosted S3, OIDC/provider configuration and human crash/restart acceptance remain distinct from deterministic local tests.

Unreleased pre-attempt `.artifact-*` files beside digest objects are not automatically deleted by guessing their ownership. Current writes use the dedicated `.staging` directory with locked-writer protection. Retryable handling does not claim to recover unknown enqueue errors whose PostgreSQL type was already erased by another repository boundary.

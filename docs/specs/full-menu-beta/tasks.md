# Full-Menu Beta 0.2.0 Work Graph

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.2.0 |
| Status | Confirmed |
| Plan | `docs/specs/full-menu-beta/plan.md` 0.2.0 Confirmed |
| Last updated | 2026-09-05 |
| Execution authorization | Continuous delegated execution approved through FMB-T007 on 2026-09-04 |

## Status Contract

- `Draft`: the task is approved in the graph but its dependencies or packet are incomplete.
- `Ready`: dependencies, approved contract, analysis/checklist coverage and execution packet are complete.
- `Running`: one bounded execution attempt is active.
- `Needs_Review`: implementation, tests and evidence pass orchestrator review; release acceptance remains.
- `Accepted`: explicitly accepted or included in the accepted Full-Menu Beta release.
- `Blocked`: a named technical or external condition prevents meaningful progress.
- `Superseded`: a later confirmed decision replaces the task without rewriting accepted history.

Approval of this graph authorizes continuous execution. A dependency at `Needs_Review` with passing
evidence is sufficient to prepare and start the next packet. Shared OpenAPI, generated contracts,
sqlc output, HTTP routing, worker registration, readiness and `ProductApp.tsx` changes are serialized.

## Epic And Stories

### Epic FMB-E001: Truthful Full-Menu Beta

Goal: invited human and machine users can complete every supported desktop workflow through real,
durable and secure product paths.

| Story | User outcome | Tasks |
| --- | --- | --- |
| FMB-US-001 | A security administrator manages and inspects real authorization state. | FMB-T001 |
| FMB-US-002 | A steward works from authoritative workbench, asset and release information. | FMB-T002 |
| FMB-US-003 | An operator inspects audit and run state and manages supported runtime policy. | FMB-T003 |
| FMB-US-004 | A source operator ingests databases, files and versioned artifacts manually or on schedule. | FMB-T004 |
| FMB-US-005 | An administrator rebuilds and atomically activates a durable embedding index. | FMB-T005 |
| FMB-US-006 | A machine consumer uses credentials, REST, MCP, CLI, webhooks and the TypeScript SDK consistently. | FMB-T006 |
| FMB-US-007 | An authorized consumer executes a validated plan through a bounded read-only adapter in hosted Beta. | FMB-T007 |

## Task Summary

| Task | Status | Depends on | Migration | Packet |
| --- | --- | --- | --- | --- |
| FMB-T001 | Needs_Review | Confirmed plan and graph | 000016 | `packets/T001.yaml` (Completed) |
| FMB-T002 | Needs_Review | FMB-T001, FMB-T003 | None | `packets/T002.yaml` (Completed) |
| FMB-T003 | Needs_Review | FMB-T001 | 000017 | `packets/T003.yaml` (Completed) |
| FMB-T004 | Needs_Review | FMB-T003 | 000018 | `packets/T004.yaml` (Completed) |
| FMB-T005 | Needs_Review | FMB-T003 | 000019 | `packets/T005.yaml` (Completed) |
| FMB-T006 | Needs_Review | FMB-T001, FMB-T003 | 000020 | `packets/T006.yaml` (Needs_Review) |
| FMB-T007 | Needs_Review | FMB-T002, FMB-T004, FMB-T005, FMB-T006 | 000021 | `packets/T007.yaml` (A002 + authorized overtime) |

## Task Details

### FMB-T001: Technical boundary and real authorization baseline

Status: Needs_Review  
Priority: P0  
Depends on: Full-Menu Beta plan and work graph Confirmed; Alpha identity/session and M2 authorization foundations present  
Blocks: FMB-T002, FMB-T003, FMB-T006  
Story / Requirement: FMB-US-001; FMB-FR-001, FMB-FR-002; FMB-NFR-001, FMB-NFR-002, FMB-NFR-005  
Parallel: No  
Conflicts with: FMB-T002 through FMB-T007 shared OpenAPI, generated contracts, HTTP routing, readiness and Web shell files

Goal:
Establish the 0.9.0/contract-9 additive boundary and replace Access Control fixtures and client-side
authority with persisted, audited, server-evaluated role, role-version, assignment and inspection
flows.

Allowed files:
- `api/openapi/semlia.v1.yaml`, `api/gen/go/types.gen.go`
- `migrations/000016_fmb_authorization_admin.{up,down}.sql`, `db/sqlc.yaml`
- `db/queries/authorization.sql`, generated `internal/adapters/postgres/sqlc/**`
- `internal/domain/authorization/**`, `internal/application/authorization/**`
- `internal/adapters/postgres/authorization.go`, `internal/platform/http/**`, `cmd/semlia/main.go`, `cmd/semlia/readiness*`
- `sdk/typescript/src/**`
- `web/src/AccessControlView.tsx`, `web/src/authorization.tsx`, `web/src/sessionRuntime.tsx`, their focused tests, and the minimal shell wiring in `web/src/ProductApp.tsx`
- generated embedded Web artifacts under `internal/platform/web/static/**`
- authorization integration/contract tests, boundary documentation and `docs/evidence/FMB-T001/**`

Test targets:
- `internal/domain/authorization/**`, `internal/application/authorization/**`
- `tests/integration/authorization/**`, `tests/integration/db/**`, `internal/platform/http/**`
- `web/src/AccessControlView.test.tsx`, `web/src/authorization.test.tsx`, `web/src/sessionRuntime.test.tsx`

Deliverables:
- OpenAPI 0.9.0 and contract version 9 with real authorization administration contracts.
- Migration 000016, repositories, commands, handlers and generated clients.
- Real Access Control role catalogue, custom-role derivation/editing, assignments and effective-access inspector.
- Corrected production boundary inventory with no authorization fixture success path.

Acceptance criteria:
- System roles cannot be edited or deleted; custom role changes create immutable versions.
- Assignment create, expiry and revoke atomically bump workspace authorization version and affect the next protected request.
- Human-only actions, authorization ceilings, final-admin safety and separation of duties fail closed and are audited.
- Effective-access inspection uses the server evaluator and does not grant access itself.
- Cross-workspace callers cannot learn principal, role, assignment or resource existence.

Definition of Done:
Focused RED/GREEN tests, populated migration up/down/up, generated drift, source gates and three-pass
review pass; evidence exists under `docs/evidence/FMB-T001/`; the orchestrator marks the task
`Needs_Review` rather than `Accepted`.

Validation commands:
- `go test ./internal/domain/authorization/... ./internal/application/authorization/... ./internal/platform/http/...`
- `go test ./tests/integration/authorization/... ./tests/integration/db/...`
- `pnpm --dir web test -- AccessControlView authorization sessionRuntime`
- `make contracts-check`
- `make db-generate-check`
- `make check-source`
- `git diff --check`

TDD plan:
- RED: prove fixture-only writes, stale authorization versions, custom-role mutation, conflicting assignment, expiry/revocation and cross-workspace inspection fail the required contract.
- GREEN: add the minimum migration, application commands/queries, handlers and generated-client Web integration.
- REFACTOR: remove production fixture authority, centralize server DTO mapping and retain one evaluator path.

Packet path:
- `docs/specs/full-menu-beta/packets/T001.yaml`

Evidence required:
- Changed-files inventory and preservation note for pre-existing Alpha/M3 work.
- RED/GREEN/REFACTOR commands and results.
- Migration, authorization decision, audit/redaction and browser evidence.
- Contract diff, review findings and residual risk.

### FMB-T002: Real workbench, authoritative asset detail and release detail

Status: Needs_Review  
Priority: P1  
Depends on: FMB-T001 and FMB-T003 Needs_Review or Accepted  
Blocks: FMB-T007  
Story / Requirement: FMB-US-002; FMB-FR-001, FMB-FR-003, FMB-FR-004; FMB-NFR-001, FMB-NFR-003, FMB-NFR-005  
Parallel: No  
Conflicts with: FMB-T003 and FMB-T006 shared OpenAPI, generated client and `ProductApp.tsx` integration zones

Goal:
Replace workbench and detail fixtures with authorized read projections over canonical proposal,
review, validation, catalog-revision and immutable-release state.

Allowed files:
- additive workbench/catalog/release portions of `api/openapi/semlia.v1.yaml` and generated contracts
- catalog, governance, release and projection application/repository query files
- focused Web workbench, knowledge, release and shell files and tests
- focused integration/E2E tests and `docs/evidence/FMB-T002/**`

Test targets:
- workbench projection and HTTP contract tests
- catalog revision and immutable release-manifest repository tests
- workbench/detail React tests and desktop Playwright journeys

Deliverables:
- Cursor-paged workbench items with actor relationship and valid next actions.
- Authoritative asset revision, evidence, binding, validation and lineage detail.
- Immutable release manifest, object-version, compatibility and rollback detail.
- Honest empty/unavailable rendering for data absent from canonical storage.

Acceptance criteria:
- Workbench counts and filters survive refresh and equal the underlying governed records.
- “Mine” and allowed actions derive from server identity, authorization and separation-of-duty state.
- Release detail never reads mutable current objects in place of manifest-pinned versions.
- No static deadline, owner, migration, validation or release success appears as canonical data.

Definition of Done:
Focused tests, contract drift, 10,000-asset bounded-query evidence, both desktop viewports and review
pass with evidence under `docs/evidence/FMB-T002/`.

Validation commands:
- `go test ./internal/application/catalog/... ./internal/application/governance/... ./internal/application/projection/... ./internal/platform/http/...`
- `go test ./tests/integration/...`
- `pnpm --dir web test -- ProductApp Knowledge`
- `make contracts-check`
- `make check-source`
- `git diff --check`

TDD plan:
- RED: fixture workbench state, mutable release leakage, unauthorized next actions and cross-workspace details fail.
- GREEN: implement bounded read projections and generated-client UI state.
- REFACTOR: share projection DTO mapping without introducing a second task write model.

Packet path:
- `docs/specs/full-menu-beta/packets/T002.yaml` after dependency evidence.

Evidence required:
- Projection consistency and query-plan evidence.
- Refresh, authorization, empty/error and desktop visual evidence.
- Changed files, command results, reviews and residual risk.

### FMB-T003: Audit and Runtime common foundation

Status: Needs_Review  
Priority: P0  
Depends on: FMB-T001 Needs_Review or Accepted  
Blocks: FMB-T002, FMB-T004, FMB-T005, FMB-T006  
Story / Requirement: FMB-US-003; FMB-FR-001, FMB-FR-004, FMB-FR-005; FMB-NFR-001, FMB-NFR-002, FMB-NFR-003, FMB-NFR-004  
Parallel: No  
Conflicts with: FMB-T002 and later tasks sharing contracts, sqlc output, worker routing, readiness and Web shell

Goal:
Provide one redacted operational read contract and durable runtime-policy base that later ingestion,
embedding, webhook and execution runs can join without sharing domain write models.

Allowed files:
- additive Operations/Runtime OpenAPI and generated contracts
- `migrations/000017_fmb_operations_runtime.{up,down}.sql`
- audit/jobs/runtime SQL, repositories, application services, handlers and worker runtime
- `web/src/AuditRuntimeView.tsx`, focused shell/runtime files and tests
- integration/E2E tests, operations documentation and `docs/evidence/FMB-T003/**`

Test targets:
- cursor/filter authorization and redaction tests for audit and all existing run kinds
- runtime-policy concurrency/retention and optimistic-update tests
- retry/cancel capability and restart behavior tests
- Audit/Runtime React and desktop E2E tests

Deliverables:
- Audit list/detail/export and canonical run list/detail contracts.
- Versioned future-run defaults for retry ceiling, statement/webhook timeout, query row/byte limits
  and run-metadata retention.
- Domain-declared retry/cancel capability and stable run status/reason codes.
- Deployment-managed telemetry/config status rendered read-only.

Acceptance criteria:
- Raw job/outbox payloads, prompts, DSNs and provider errors never leak through Operations.
- Future-run default changes are authorized, audited, version checked and applied only to runs
  created after the change.
- Unsupported retry/cancel/settings controls cannot report success.
- Existing validation, discovery, agent and semantic runs survive process/database restart and remain queryable.
- Audit retention, worker concurrency, queue policy, OTel endpoint/auth, OIDC, encryption roots and
  outbound trust configuration remain deployment-owned and read-only in Web.

Definition of Done:
Migration lifecycle, focused run/audit tests, redaction scan, Web states and source gates pass with
evidence under `docs/evidence/FMB-T003/`.

Validation commands:
- `go test ./internal/application/jobs/... ./internal/application/usage/... ./internal/platform/http/...`
- `go test ./tests/integration/worker/... ./tests/integration/db/...`
- `pnpm --dir web test -- AuditRuntime`
- `make contracts-check`
- `make db-generate-check`
- `make check-source`
- `git diff --check`

TDD plan:
- RED: fake runs, raw payload leakage, stale policy writes, unsafe retry/cancel and restart loss fail.
- GREEN: implement migration, discriminated projections, policy commands and real Web bindings.
- REFACTOR: keep write ownership inside each domain and centralize only the operational projection.

Packet path:
- `docs/specs/full-menu-beta/packets/T003.yaml` after dependency evidence.

Evidence required:
- Migration, redaction, paging, policy and restart results.
- UI screenshots/states, changed files, reviews and residual risk.

### FMB-T004: File, SQL/dbt artifact ingestion and scheduling

Status: Needs_Review  
Priority: P1  
Depends on: FMB-T003 Needs_Review or Accepted  
Blocks: FMB-T007  
Story / Requirement: FMB-US-004; FMB-FR-001, FMB-FR-006; FMB-NFR-001, FMB-NFR-002, FMB-NFR-003, FMB-NFR-004  
Parallel: No  
Conflicts with: FMB-T005 and FMB-T006 shared jobs, contracts, sqlc output, readiness and Sources shell

Goal:
Make the visible file-source, SQL/dbt artifact and schedule paths durable, bounded and traceable into
the existing discovery and candidate pipeline.

Allowed files:
- additive source/upload/artifact/schedule OpenAPI and generated contracts
- `migrations/000018_fmb_file_ingestion_schedules.{up,down}.sql`
- discovery domain/application/adapters, file storage/parsers, scheduler/jobs, SQL and generated sqlc
- Sources/File/Automation Web surfaces and focused tests
- fixtures, integration/E2E tests, operations documentation and `docs/evidence/FMB-T004/**`

Test targets:
- CSV/XLSX/Markdown magic, MIME, quota, archive, active-content and parser tests
- artifact-root traversal/symlink and dbt schema-version tests
- schedule timezone/DST/missed occurrence/overlap/restart/idempotency tests
- upload-to-candidate and scheduled-run desktop journeys

Deliverables:
- Content-addressed upload storage and source metadata for CSV, XLSX and Markdown.
- Versioned SQL and supported dbt manifest/catalog registration.
- Manual ingestion plus durable five-field-cron/IANA-timezone schedules and occurrences.
- Run/findings/candidate integration through T003 Operations.

Acceptance criteria:
- Refresh/restart preserves sources, inputs, schedules and runs without duplicate candidate writes.
- File names, MIME headers, ZIP entries, SQL paths and dbt documents are treated as untrusted input.
- Overlapping schedule occurrences enqueue at most one run and record skip/failure reasons.
- Unsupported, encrypted, macro-enabled, oversized or unsafe inputs fail without partial canonical state.

Definition of Done:
Migration lifecycle, parser/security corpus, scheduler recovery, real Web workflow and full source gate
pass with evidence under `docs/evidence/FMB-T004/`.

Validation commands:
- `go test ./internal/application/discovery/... ./internal/adapters/discovery/... ./internal/application/jobs/...`
- `go test ./tests/integration/db/... ./tests/integration/worker/...`
- `pnpm --dir web test -- LiveSources JourneyViews`
- `make contracts-check`
- `make db-generate-check`
- `make check-source`
- `git diff --check`

TDD plan:
- RED: unsafe files/artifacts, duplicate occurrences, DST, overlap, restart and partial-write cases fail.
- GREEN: add bounded storage/parsers, schedules and existing-pipeline integration.
- REFACTOR: share ingestion envelopes while preserving type-specific parser boundaries.

Packet path:
- `docs/specs/full-menu-beta/packets/T004.yaml` after dependency evidence.

Evidence required:
- Parser corpus and security findings.
- Migration, idempotency, restart and scheduled-run results.
- Browser journey, changed files, reviews and residual risk.

### FMB-T005: Persistent embedding-index rebuild

Status: Needs_Review  
Priority: P1  
Depends on: FMB-T003 Needs_Review or Accepted  
Blocks: FMB-T007  
Story / Requirement: FMB-US-005; FMB-FR-001, FMB-FR-007; FMB-NFR-001, FMB-NFR-002, FMB-NFR-003, FMB-NFR-004  
Parallel: No  
Conflicts with: FMB-T004 and FMB-T006 shared jobs, model-provider boundary, contracts, sqlc, readiness and settings shell

Goal:
Replace the timer-based embedding rebuild with a leased, resumable pgvector generation pipeline and
an atomic active-index switch used for bounded semantic candidate recall.

Allowed files:
- additive embedding-index OpenAPI and generated contracts
- `migrations/000019_fmb_embedding_index.{up,down}.sql`
- embedding domain/application/provider/postgres/job modules and model configuration integration
- PostgreSQL/pgvector development and release-image configuration
- Model Configuration, Operations integration and focused Web tests
- integration/performance/E2E tests, runbook and `docs/evidence/FMB-T005/**`

Test targets:
- stable corpus snapshot/chunk checksum and exact-dimension tests
- provider batching, timeout, retry, cancellation and restart tests
- incomplete-generation rejection and concurrent atomic-switch tests
- authorization-filtered recall and deterministic-resolver non-regression
- rebuild/progress/failure desktop journeys

Deliverables:
- pgvector deployment support and migration 000019.
- Immutable corpus/generation/vector storage, progress and active pointer.
- Leased rebuild worker with provider batching, validation, cancel and safe cleanup.
- Real Model Configuration rebuild UI and Operations run detail.

Acceptance criteria:
- Failed or canceled rebuilds never replace the active generation.
- Model/dimension/corpus changes create a new generation and cannot mix vector shapes.
- Active recall filters workspace, authorization and release membership before candidate disclosure.
- The deterministic resolver remains the final selector; vector similarity cannot bypass governance.

Definition of Done:
Migration/recovery, real provider credential run when available, deterministic failure fixtures,
recall quality baseline, Web E2E and source/security gates pass with evidence under
`docs/evidence/FMB-T005/`.

Validation commands:
- `go test ./internal/domain/... ./internal/application/... ./internal/adapters/postgres/...`
- `go test ./tests/integration/db/... ./tests/integration/worker/...`
- `pnpm --dir web test -- ModelConfiguration AuditRuntime`
- `make contracts-check`
- `make db-generate-check`
- `make check-source`
- `git diff --check`

TDD plan:
- RED: timer-only state, mixed dimensions, partial activation, restart duplication, cancel and unauthorized recall fail.
- GREEN: implement immutable generations, leased batching, validation, atomic switch and real UI.
- REFACTOR: isolate embedding provider and storage ports from the deterministic resolver.

Packet path:
- `docs/specs/full-menu-beta/packets/T005.yaml` after dependency evidence.

Evidence required:
- Generation IDs/checksums, provider mode, recovery and activation results.
- Recall/security/performance evidence, changed files, reviews and residual risk.

### FMB-T006: Machine credentials and distribution interfaces

Status: Needs_Review  
Priority: P0  
Depends on: FMB-T001 and FMB-T003 Needs_Review or Accepted; M3-T001 through M3-T004 implementation evidence present  
Blocks: FMB-T007  
Story / Requirement: FMB-US-006; FMB-FR-001, FMB-FR-008, FMB-FR-009, FMB-FR-010; FMB-NFR-001, FMB-NFR-002, FMB-NFR-003, FMB-NFR-004, FMB-NFR-005  
Parallel: No  
Conflicts with: all tasks editing OpenAPI/generated clients, authentication middleware, outbox dispatcher, CLI command tree, worker routing or Integration Settings

Goal:
Deliver secure machine authentication and REST/MCP/CLI/Webhook/TypeScript SDK parity over the
canonical distribution service.

Allowed files:
- additive client-credential/webhook/interface OpenAPI and generated contracts
- `migrations/000020_fmb_distribution_interfaces.{up,down}.sql`
- identity/authorization/distribution domain and application modules
- machine-auth middleware, MCP adapter, CLI command tree and TypeScript SDK
- webhook subscription/delivery/signing/SSRF modules and jobs/outbox routing
- Integration Settings Web surface and focused tests
- integration/conformance/E2E/security tests, interface documentation and `docs/evidence/FMB-T006/**`

Test targets:
- credential one-time display, digest, expiry, revoke, rotation and cross-workspace tests
- REST/MCP/CLI golden conformance for resources, resolve, plan and refusal
- webhook allowlist, HMAC, replay, DNS rebinding/SSRF, retry and dead-letter tests
- TypeScript SDK compile/contract and Integration Settings browser tests

Deliverables:
- Accountable machine principals, API clients and credential lifecycle.
- Bearer middleware with existing action-based authorization and attribution.
- Official-SDK MCP adapter and Go CLI commands using canonical application services.
- Signed per-subscription webhook delivery with independent retry state.
- Generated TypeScript SDK and truthful live interface configuration UI.

Acceptance criteria:
- Raw credentials and webhook secrets are returned once and never stored, logged or audited in plaintext.
- Revocation/expiry takes effect on the next request and channel behavior matches REST reason codes and plan digests.
- MCP/CLI cannot access repositories, hidden candidates, draft releases or arbitrary SQL.
- One outbox event fans out to independent delivery records without corrupting Git projection delivery state.
- Webhook destinations are revalidated on delivery and external failures become bounded retry/dead-letter state.

Definition of Done:
Migration lifecycle, channel conformance, credential and SSRF security suites, external receiver test,
SDK drift and source gates pass with evidence under `docs/evidence/FMB-T006/`.

Validation commands:
- `go test ./internal/application/distribution/... ./internal/application/authorization/... ./internal/platform/http/... ./cmd/semlia/...`
- `go test ./tests/integration/... ./tests/contracts/...`
- `pnpm --dir sdk/typescript test`
- `pnpm --dir web test -- IntegrationSettings`
- `make contracts-check`
- `make db-generate-check`
- `make check-source`
- `git diff --check`

TDD plan:
- RED: plaintext, expired/revoked tokens, channel drift, cross-workspace access, webhook replay/SSRF and fanout loss fail.
- GREEN: implement credential, adapters, delivery ledger, generated SDK and real UI.
- REFACTOR: keep transport adapters thin and centralize distribution/authorization services.

Packet path:
- `docs/specs/full-menu-beta/packets/T006.yaml` after dependency evidence.

Evidence required:
- Credential and signature redaction proof.
- REST/MCP/CLI/SDK golden conformance and external webhook receiver results.
- Migration/recovery results, changed files, reviews and residual risk.

### FMB-T007: Read-only PostgreSQL execution adapter and hosted final gate

Status: Needs_Review  
Priority: P0  
Depends on: FMB-T002, FMB-T004, FMB-T005 and FMB-T006 Needs_Review or Accepted  
Blocks: Full-Menu Beta 0.2.0 acceptance recommendation  
Story / Requirement: FMB-US-007; FMB-FR-001, FMB-FR-011, FMB-FR-012; all FMB-NFR requirements  
Parallel: No  
Conflicts with: every active FMB implementation attempt; this is the serialized integration and release task

Execution state: Authorized overtime completes local governed PostgreSQL execution,
immutable physical pins, formal joins, precise results, five-channel integration and
migration21 preview. Source, all desktop suites, security, smoke and exact final-source
candidate archive gates pass, including entry-motion contrast. Founder confirms no
server is available and authorizes local closeout first; hosted gates are deferred,
not falsely accepted. Current evidence is `attempts/a002/overtime/summary.md` under
`docs/evidence/FMB-T007/`. Formal acceptance remains founder-owned.

Goal:
Execute only persisted, validated plans through one bounded read-only PostgreSQL adapter, then prove
the complete product on local deterministic infrastructure and the configured hosted Beta.

Allowed files:
- additive execution OpenAPI and generated contracts
- `migrations/000021_fmb_query_execution.{up,down}.sql`
- distribution/execution domain, application, PostgreSQL adapter, HTTP/MCP/CLI/SDK integration
- Ask/plan/result/Operations Web surfaces and tests
- final readiness/config, Compose/release/security scripts and deployment/runbook files
- full integration/E2E/performance/security tests and `docs/evidence/FMB-T007/**`
- FMB/Alpha/M2/M3 status and release-report documentation after evidence review

Test targets:
- plan-to-parameterized-SQL golden tests and invalid-plan rejection
- database read-only role, one-statement, timeout, cancel, concurrency, row/byte limit and truncation tests
- cross-workspace/source/binding/release authorization and provenance tests
- full authenticated desktop, REST, MCP, CLI, webhook, schedule and recovery journeys
- migration 15-to-21, security, release, SBOM and hosted acceptance tests

Deliverables:
- Migration 000021 and execution record/provenance contract.
- Read-only PostgreSQL adapter with bounded streaming and cancellation.
- Real Ask/plan execution and result states across authorized channels.
- Final migration/readiness version 21, hosted deployment evidence and Beta release report.

Acceptance criteria:
- No public field or tool accepts raw SQL; invalid/refused/stale plans never reach the source.
- The source session is read-only and bounded by timeout, rows, bytes and workspace concurrency.
- Raw fact rows are not persisted or emitted to audit, usage, logs or webhooks.
- Process, worker and database restart preserve durable state and do not duplicate execution.
- Every production-menu success action survives refresh and traces to canonical API/persistence evidence.
- Hosted OIDC, HTTPS, live model/embedding, external webhook and read-only source evidence is real; a missing input is recorded and never replaced by a stub claim.

Definition of Done:
All focused and standard gates, migration lifecycle, desktop E2E, independent specification/security/
quality review and evidence inventory pass. The orchestrator may recommend Full-Menu Beta acceptance;
it does not fabricate unavailable hosted evidence or mark the release Accepted on its own.

Validation commands:
- `go test ./...`
- `pnpm -r --if-present test`
- `make contracts-check`
- `make db-generate-check`
- `make check-source`
- `make check-smoke`
- `make security-check`
- `make release`
- `git diff --check`

TDD plan:
- RED: raw SQL, mutation, stacked statement, stale plan, unauthorized source, timeout, cancellation, row/byte overflow and restart duplicate cases fail.
- GREEN: implement typed SQL generation, bounded adapter, execution records and real channel/UI result paths.
- REFACTOR: keep execution behind a port and preserve resolution as a separately authorized capability.

Packet path:
- `docs/specs/full-menu-beta/packets/T007.yaml` after dependency evidence.

Evidence required:
- Migration and release artifact digests.
- Read-only/limit/cancel/provenance and no-retained-results proof.
- Complete route truthfulness inventory and desktop screenshots.
- Deterministic versus hosted evidence matrix, external-input gaps, reviews and residual risk.

## Requirement Traceability

| Requirement | Tasks |
| --- | --- |
| FMB-FR-001 | FMB-T001 through FMB-T007 |
| FMB-FR-002 | FMB-T001, FMB-T007 |
| FMB-FR-003 | FMB-T002, FMB-T007 |
| FMB-FR-004 | FMB-T002, FMB-T003, FMB-T007 |
| FMB-FR-005 | FMB-T003, FMB-T007 |
| FMB-FR-006 | FMB-T004, FMB-T007 |
| FMB-FR-007 | FMB-T005, FMB-T007 |
| FMB-FR-008 | FMB-T006, FMB-T007 |
| FMB-FR-009 | FMB-T006, FMB-T007 |
| FMB-FR-010 | FMB-T006, FMB-T007 |
| FMB-FR-011 | FMB-T007 |
| FMB-FR-012 | FMB-T007 |
| FMB-NFR-001 | FMB-T001 through FMB-T007 |
| FMB-NFR-002 | FMB-T001, FMB-T003 through FMB-T007 |
| FMB-NFR-003 | FMB-T002 through FMB-T007 |
| FMB-NFR-004 | FMB-T003 through FMB-T007 |
| FMB-NFR-005 | FMB-T001, FMB-T002, FMB-T006, FMB-T007 |

## Approval Gate

The founder approved this work graph on 2026-09-04. FMB-T001 through FMB-T006 are `Needs_Review` with
passing local implementation and review evidence. T006 evidence includes real machine/channel,
signed receiver, migration, browser and security gates under `docs/evidence/FMB-T006/`.
Live provider, recall-quality, scale, public HTTPS receiver and hosted acceptance remain open for
T005 and the final Beta gate. The fourth T007 sprint window is 2026-09-05 10:45:36-13:45:36 UTC,
with implementation cutoff at 13:20:36 UTC. It resumes the frozen incomplete A001 handoff;
the original scope remains intact. Only the founder accepts the release.

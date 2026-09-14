# Full-Menu Beta Planning Analysis

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.2.0 |
| Status | Confirmed |
| Requirements | `docs/specs/full-menu-beta/spec.md` 0.2.0 Confirmed |
| Decision record | `docs/specs/full-menu-beta/technology-decision-record.md` 0.2.0 Confirmed |
| Data model | `docs/specs/full-menu-beta/data-model.md` 0.2.0 Confirmed |
| Last updated | 2026-09-04 |

## Result

The approved Beta is an implementation program rather than a finishing sweep. Existing M0-M3 and
Alpha code supply identity, authorization evaluation, catalog, governance, job/outbox, PostgreSQL
discovery, release resolution and Ask foundations. Six menu families still lack an authoritative
server contract or a complete real execution path.

The plan uses seven dependency-ordered tasks. The operations projection is delivered early because
artifact imports, schedules, Embedding rebuilds, webhooks and query execution all need durable run
visibility and audit attribution.

## Reuse Inventory

| Area | Reusable implementation | Missing Beta contract |
| --- | --- | --- |
| Identity/Authz | OIDC Code+PKCE, opaque sessions, CSRF/Origin checks, memberships, action vocabulary, evaluator, decision audit | Persisted role/grant/effective-access administration UI APIs; machine principal authentication |
| Catalog/Governance | Assets, immutable revisions, relations, evidence, proposals, validation, reviews, release and rollback | Authoritative composite asset detail; no fixture/default enrichment |
| Operations | `audit_events`, leased `jobs`, `outbox_events`, worker dispatcher/router, trace IDs | Audit reads/export, unified runs, progress/events, operator settings and commands |
| Sources | Live read-only PostgreSQL collector, safe SQL artifact loader/parser, dbt and catalog adapters, generic discovery service | Upload store and API, adapter orchestration, file adapters, schedules |
| Retrieval | Postgres lexical search, deterministic resolver, model settings with Embedding dimension | Embedding client, durable vector versions, rebuild lifecycle and active-index read path |
| Distribution | Consumers, bindings, SemanticQuery/plan REST, generated TypeScript transport | Credentials, bearer middleware, MCP, CLI, webhook management and supported SDK facade |
| Execution | Released plan/refusal and `semantic.execute` action | Executable physical snapshot, compiler, adapter, limits, results and run records |
| Hosted delivery | Production-fail-closed OIDC implementation, local Compose and readiness command | Real tenant/TLS/provider, production bundle separation, recovery and release rehearsal |

## Evidence Classification

### Access Control

The product contains two distinct facts:

- Server-side protected commands already call the Semlia authorization evaluator and browser
  sessions expose real workspace capabilities.
- The Access Control management view uses fixture roles, principals and grants and keeps changes in
  local React state. OpenAPI does not expose role catalog, scoped grant mutation or effective-access
  inspection endpoints.

Therefore access-control prototype evidence is UX evidence, not production-administration evidence.
The Beta closes the management path without replacing the existing server evaluator.

### Asset Detail

The Catalog API and M1 evidence prove real asset, revision, relation and evidence retrieval. The Web
projection assigns defaults or empty collections to several implementation, release, validation,
binding and impact fields. Therefore the six-tab screen is not uniformly authoritative. Beta
acceptance is field-owner based: each rendered fact identifies the API/revision/release that owns it,
and absent facts remain visibly absent.

## Work Graph

| Task | Goal | Primary dependencies | Completion signal |
| --- | --- | --- | --- |
| FMB-T001 | Contract integrity, real Authz administration, authoritative asset detail, production build and hosted readiness foundation | Alpha 0.1.0 | Evidence authority documented; Authz/detail APIs and production-mode gates proven |
| FMB-T002 | Persisted Workbench aggregation | T001; existing governance/source records; T003 runtime projection contract | Stable authorized items deep-link to real objects and update idempotently |
| FMB-T003 | Audit and Runtime operations foundation | T001 | Filtered audit/export and normalized durable run list/detail/settings are real |
| FMB-T004 | File, versioned SQL/dbt ingestion and scheduling | T003 | Upload-to-revision and schedule-to-run paths survive restart and duplicate workers |
| FMB-T005 | Durable Embedding rebuild and active-index retrieval | T003; T004 artifact/chunk port when file content participates | Checkpointed rebuild atomically activates a consumed index |
| FMB-T006 | Machine credentials, REST bearer, MCP, CLI, webhook and TypeScript SDK parity | T001; T003; existing distribution services | Independent client proves equivalent resolution across channels and signed delivery |
| FMB-T007 | Read-only PostgreSQL execution, hosted end-to-end and final release gate | T003, T004, T006; T005 for full search journey | Controlled aggregate execution and all hosted/recovery/security gates pass |

Execution order is:

`T001 -> T003 -> {T002, T004, T005, T006} -> T007`

T002 can start after the T003 read-model contract is fixed. T004 and T005 may proceed in parallel
after agreeing on artifact and chunk ownership. T006 should not change authentication middleware in
parallel with another task changing the same composition root. T007 is the only integrated release
gate and must consume completed contracts rather than define them late.

## Requirement Traceability

| Requirement | Task coverage |
| --- | --- |
| FR-001 through FR-003 | T001, T002, T007 |
| FR-004 | T002, T003, T007 |
| FR-005 | T003, T007 |
| FR-006 | T004, T007 |
| FR-007 | T005, T007 |
| FR-008 and FR-009 | T006, T007 |
| FR-010 | T003, T006, T007 |
| FR-011 | T003, T004, T006, T007 |
| FR-012 | T001, T007 |
| NFR-001 through NFR-005 | T001-T007, integrated in T007 |

## Cross-Contract Decisions

| Concern | Resolution |
| --- | --- |
| Workbench ownership versus M4 attention | Beta persists a small operational attention projection and manual assignment state; it does not add autonomous scoring, policy actions or automatic remediation. |
| Audit retention versus immutable trigger | Audit retention is deployment policy in 0.2.0. The workspace UI may display it read-only; application users cannot delete audit rows. |
| Runtime settings versus process configuration | Workspace settings cover future-run defaults such as retry and time limits. Worker pool size, OTel destination and secret-store configuration remain deployment-owned. |
| Upload server versus worker filesystem | Both use one artifact-store port and shared durable backing. A server-local temporary path is never the authoritative artifact reference. |
| Provider secret entry | Model settings reference a configured secret; the UI must not claim a provider works until the server/worker can resolve and test that secret. |
| Consumer versus principal | A machine credential belongs to one consumer and resolves to one server-owned machine principal. Credential actions can only narrow that principal's grants. |
| CSRF and bearer auth | Cookie requests retain Origin+CSRF checks. Bearer requests are non-cookie authenticated, reject mixed credentials and never infer workspace from an untrusted header. |
| Channel parity | Channel adapters translate transport only. Resolution, authorization, binding and execution live in shared application services. |
| Query plan sufficiency | Execution remains disabled until a release-pinned physical plan contains exact source, dataset, field, join and expression references required by the compiler. |
| Embedding availability | Lexical retrieval remains a disclosed fallback. A failed vector rebuild never disables the previous active index or silently reports success. |

## Risk Register

| Risk | Severity | Required control |
| --- | --- | --- |
| Access Control page appears persistent while using fixture state | Critical product-trust risk | Replace management state with APIs or disable mutation; no prototype success copy in real runtime |
| Asset detail combines real core fields with synthetic auxiliary fields | High product-trust risk | Per-section authoritative projections and explicit unavailable states |
| Credential or provider secret leakage | Critical security risk | One-time secret return, verifier digest, structured redaction tests and no secret-bearing jobs |
| Upload traversal, decompression or spreadsheet execution abuse | High security risk | Size/count/type limits, safe extraction, no formula execution, content digest and isolated parser |
| Webhook SSRF or replay | Critical security risk | Destination policy, pinned validated IP, redirect refusal, signed timestamped envelope and idempotent receiver contract |
| Duplicate scheduled work on multiple workers | High consistency risk | Database occurrence key plus lease and unique enqueue constraint |
| Embedding dimension or partial-index activation | High correctness risk | Staging version, dimension/count validation and atomic active pointer |
| Query compiler permits arbitrary SQL or persists fact rows | Critical data risk | Typed-plan compiler, single-statement SELECT gate, read-only transaction, limits and metadata-only persistence |
| Mutable workspace UI controls deployment OTel/outbound endpoints | High exfiltration risk | Deployment-only configuration or strict administrator allowlist |
| Production image includes local-UAT identities | Critical authentication risk | Build-time assertion and hosted smoke test |

## Verification Strategy

- Contract: OpenAPI validation, generated Go/TypeScript drift checks and golden MCP/CLI output.
- Authorization: allow/deny tests for every new action, cross-workspace non-disclosure, expired and
  revoked identities, and proof that denial occurs before an external adapter call.
- Persistence: integration tests across process restart, idempotency replay, overlapping workers and
  migration from a populated Alpha database.
- Security: log/audit redaction, token storage inspection, upload adversarial corpus, webhook SSRF
  cases and SQL compiler injection cases.
- Hosted: real OIDC login/logout/revocation, TLS cookie behavior, live model/Embedding connection,
  machine credential journey, webhook receiver and read-only execution source.
- Desktop: keyboard, focus, reduced motion, empty/error/loading states and no overflow at 1440x900
  and 1024x768.

## Planning Verdict

No further product approval is required to start or continue FMB-T001 through FMB-T007. Each task
must preserve its own review evidence, and T007 must reject release if any supported menu still
depends on fixtures, simulated success or an unverified external boundary.

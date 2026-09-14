# Semlia Full-Menu Beta Specification

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.2.0 |
| Status | Confirmed |
| Source | Founder approval of Full-Menu Beta 0.2.0 and continuous FMB-T001 through FMB-T007 execution on 2026-09-04 |
| Product baseline | `docs/SSOT.md`, Controlled-User Alpha 0.1.0, M1-M3 accepted implementation evidence |
| Last updated | 2026-09-04 |

## Product Positioning

Full-Menu Beta 0.2.0 turns the complete desktop navigation into an honest, persisted product.
Every supported menu reads authoritative workspace state, performs mutations through server-side
authorization, and shows durable outcomes or explicit unavailable states. The release proves both
human workflows and headless semantic consumption in one hosted environment.

The Beta remains a single-region modular monolith backed by PostgreSQL and Git release artifacts.
It is not the M4 autonomous-governance milestone, a plugin marketplace, or a high-availability and
multi-region operations commitment.

## Evidence Authority

The following evidence boundaries are normative for this specification:

| Surface | Authoritative current statement |
| --- | --- |
| Access Control | The server-side authorization evaluator, stable action vocabulary, session capability projection, default-deny behavior and protected domain commands are real. `docs/evidence/access-control-prototype/T005/summary.md` proves only the session-only frontend interaction model. The Access Control page still uses repository fixtures and local React state; it does not prove role, grant or effective-access administration through persisted APIs. The Access Control row in `docs/evidence/ALPHA-T006/route-disclosure-inventory.md` is not evidence of a complete persisted administration surface. |
| Asset detail | `docs/evidence/M1-T008/summary.md` proves the real catalog asset, current revision, relation and revision-evidence read path. The six-tab UX evidence proves layout and navigation only. Release state, PhysicalBinding, JoinContract, validation, trust, implementation and consumer-impact sections are authoritative only when loaded from their owning APIs; defaults, empty projections and fixture enrichment are not completion evidence. |
| Prototype evidence | Prototype screenshots and browser journeys remain valid design evidence within their named scope. They never substitute for database, API, restart, authorization or hosted-environment evidence. |

No Beta acceptance may cite an older prototype artifact as proof of server persistence or production
authorization.

## Target Users

- Workspace administrator who manages members, roles, machine clients, runtime policy and sources.
- Semantic engineer who imports source artifacts, runs discovery, rebuilds semantic indexes and
  governs knowledge.
- Reviewer, publisher and auditor who resolve assigned work and inspect attributable evidence.
- Application developer or Agent operator who consumes released semantics through REST, MCP, CLI,
  webhook and the TypeScript SDK.
- Data consumer who executes one governed, read-only PostgreSQL aggregate query from a validated
  released plan.

## Core Journeys

1. A hosted invited user signs in with OIDC and receives workspace-scoped capabilities.
2. An administrator inspects real roles, grants and effective access, then issues a scoped machine
   credential whose secret is shown once.
3. A semantic engineer connects PostgreSQL or imports a versioned SQL, dbt or supported file bundle,
   runs discovery manually or on a schedule, and follows the durable run to its evidence.
4. The Workbench shows authorized actionable work derived from real proposals, validations,
   discovery, runtime failures and compatibility state, with stable links to the owning object.
5. An auditor filters immutable audit events, follows trace-linked runs, and exports an authorized,
   redacted result set.
6. An administrator starts an Embedding rebuild, observes checkpointed progress, and activates the
   validated index without interrupting the previous search index.
7. A developer resolves the same released SemanticQuery through REST bearer authentication, MCP,
   CLI and the TypeScript SDK, receiving equivalent plans and refusal codes.
8. A subscribed endpoint receives signed, retryable, versioned webhook events.
9. An authorized consumer executes a validated plan through the PostgreSQL read-only adapter under
   strict time, row and byte limits and inspects provenance without Semlia retaining fact rows.

## Functional Requirements

### FR-001 Honest full-menu state

Each primary menu has a real read path, a real command path where the UI presents a command, and
loading, empty, denied, failure and retry states. A real API error never falls back to fixtures.
Unavailable capabilities are disabled with a precise reason and never emit persisted-success UI.

### FR-002 Real authorization administration

Access Control lists system and custom roles, stable permissions, scoped grants, expiry and
separation-of-duties conflicts from persisted server state. Authorized administrators can create
and revise custom roles, grant and revoke roles, and inspect an effective decision with reason code
and authorization version. Every protected endpoint independently evaluates principal, workspace,
action, resource and scope; hiding controls is not enforcement. System roles remain immutable and
the final workspace administrator cannot be removed.

### FR-003 Authoritative asset detail

Asset detail composes the exact catalog asset and revision with released state, semantic definition,
relations, PhysicalBinding, JoinContract, validation, evidence, trust and consumer-impact data from
their owning persisted contracts. Every section identifies its release or revision basis. Missing
data is rendered as unavailable or incomplete, not synthesized. A governed edit enters the normal
proposal workflow and cannot mutate released truth in place.

### FR-004 Persisted Workbench

The Workbench exposes a server-owned, authorization-filtered projection of actionable review,
validation, discovery, runtime and compatibility work. Items have stable IDs, kind, state, priority,
risk, assignee or audience, timestamps, evidence summary and a typed target link. Refresh and process
restart preserve identity and state. Resolving the owning condition updates or closes the item
idempotently.

### FR-005 Audit and Runtime

Authorized users can list, filter, paginate and inspect audit events without reading secret values.
Exports use the same authorized filter and record an attributable export event. Runtime lists and
details normalize jobs, discovery runs, validation and Agent runs, Embedding rebuilds, webhook
deliveries and query executions under one typed projection with trace, state, attempts, phase,
progress and links to source records. Runtime settings are persisted with optimistic versioning and
separate read and manage permissions.

### FR-006 Versioned artifact import and scheduling

Semlia supports live read-only PostgreSQL discovery and content-addressed imports for versioned SQL,
dbt manifest/catalog bundles and supported `.csv`, `.xlsx` and `.md` files. Uploads are bounded,
validated, redacted and stored through a shared artifact-store port. SQL and dbt imports reuse the
existing adapters and persist immutable source revisions, physical graph, lineage, findings,
evidence and candidates atomically. Schedules have an IANA timezone, validated expression, pause
state, next and last occurrence, misfire policy and idempotent enqueue behavior. Manual and scheduled
runs call the same application command.

### FR-007 Durable Embedding rebuild

One configured Embedding provider can create a checkpointed, workspace-scoped index version from
canonical knowledge chunks. Rebuilds validate model, dimension and completeness before an atomic
automatic active-version switch; starting a rebuild is the operator's activation intent rather than
a later browser-only approval. Failed or cancelled rebuilds leave the previous index serving. Unchanged
content reuses vectors by digest. At least one production search or resolver candidate path consumes
the active index and records whether lexical fallback was used.

### FR-008 Machine credentials and REST bearer authentication

Administrators issue credentials for an active consumer identity with explicit actions, resource
scope and expiry. Rotation atomically replaces the credential with zero grace and preserves its
original expiry. The plaintext secret is returned exactly once; storage
contains a verifier digest and non-secret prefix only. Bearer authentication resolves the credential
to a server-owned workspace principal and consumer binding, checks credential and consumer state,
and applies the same authorization evaluator as browser requests. Revocation, suspension and expiry
take effect on the next request. Cookie and bearer authentication cannot be combined ambiguously.

### FR-009 REST, MCP, CLI and TypeScript SDK parity

REST remains the canonical public contract. MCP resources and tools, CLI commands and the committed
TypeScript SDK call the same application services for describe, search, resolve, plan inspection and
supported execution. The same request, principal, binding and release produce the same plan digest,
refusal code and authorization outcome across channels. Channel attribution is explicit in audit,
usage and semantic-resolution events. Examples use routes and package APIs that exist in the
release.

### FR-010 Webhook delivery

Administrators manage versioned webhook subscriptions with event filters, endpoint state and a
rotatable signing secret. External envelopes have a stable version, event ID, occurrence time,
workspace-safe payload and trace reference. Deliveries use HMAC signature plus timestamp, bounded
timeouts, idempotent retries and dead-letter visibility. Private, loopback, link-local and otherwise
disallowed destinations are rejected by default; redirects are refused. Subscription edits and
signing rotation cancel outstanding deliveries from older versions rather than retargeting them.

### FR-011 Controlled PostgreSQL query execution

An authorized request may execute only a server-validated, release-pinned plan through one
PostgreSQL read-only adapter. The compiler produces one parameterized `SELECT` from trusted physical
metadata; clients cannot submit arbitrary SQL. Execution uses a dedicated read-only credential,
transaction and statement timeout, row and byte caps, cancellation and provenance. DDL, DML,
multi-statement input, stale or tampered plans, unresolved joins and unauthorized assets are refused
before the adapter is called. Semlia retains run metadata and result digest, not fact rows.

### FR-012 Hosted readiness and release gate

The release runs behind TLS with a real OIDC tenant, exact redirect configuration and a live model
provider credential. The production Web bundle excludes local-UAT identity behavior. Readiness
distinguishes database, migration, OIDC configuration, required worker and critical provider state.
Backup, restore, upgrade and rollback are rehearsed. Final acceptance exercises human and machine
journeys against the hosted build with no fixture fallback.

## Non-Functional Requirements

### NFR-001 Security and privacy

- Authentication and authorization fail closed; workspace boundaries are enforced in repositories
  and services as well as HTTP routing.
- Browser secrets, bearer tokens, signing secrets, provider keys, source credentials, SQL parameter
  values and fact rows never enter logs, traces, audit payloads or webhook envelopes.
- Upload processing prevents traversal, symlink escape, decompression abuse, formula execution and
  unsupported content masquerading.
- Webhook and configurable outbound endpoints enforce allowlists or SSRF-safe resolution.
- Dependency, secret and release-image scans report no unresolved High or Critical findings.

### NFR-002 Reliability and consistency

- Jobs, schedules, rebuilds, deliveries and executions have idempotency keys and explicit terminal
  state. Lease recovery does not create duplicate authoritative projections.
- State transitions and owning audit/outbox events commit atomically where they describe one command.
- Restart preserves sessions, credentials, schedules, run visibility and active index selection.

### NFR-003 Performance and limits

- Primary menu lists return their first page within 1 second at p95 on the Beta reference dataset,
  excluding external provider, warehouse and webhook latency.
- Long operations return a durable run immediately and expose progress without blocking navigation.
- Query execution applies configured statement, row and byte caps before releasing a result.

### NFR-004 Observability

Every boundary request and asynchronous run carries a trace ID. Metrics distinguish queue delay,
attempts, provider latency, schedule lag, webhook delivery and query execution without reconstructing
product facts from audit payloads.

### NFR-005 Desktop accessibility

All menu workflows are keyboard operable with visible focus and reduced-motion support at 1440x900
and 1024x768. The minimum supported viewport is 1024px; no mobile acceptance scope is introduced.

## Functional Scope

- One hosted single-region Semlia deployment with one OIDC issuer.
- Multiple workspaces with invited human users and scoped machine consumers.
- Real Access Control administration and authoritative asset detail.
- Workbench, Audit and Runtime as persisted operational surfaces.
- PostgreSQL, SQL, dbt and bounded file ingestion with manual and scheduled runs.
- Durable Embedding rebuild and active-index-backed retrieval.
- REST bearer, MCP, CLI, webhooks and the TypeScript SDK.
- One controlled read-only PostgreSQL query execution adapter.
- Hosted security, recovery and end-to-end readiness evidence.

## Non-Goals

- M4 autonomous scoring, autonomous proposal generation, canary governance, automatic publish or
  automatic rollback.
- Plugin marketplace, third-party plugin lifecycle or arbitrary connector SDK execution.
- High availability, multi-region replication, zero-downtime regional failover or public SLA.
- Arbitrary SQL console, write-back, dashboard builder, retained warehouse results or unrestricted
  row-level exploration.
- Python SDK, mobile navigation, native desktop packaging or multiple simultaneous OIDC issuers.

## Release Acceptance

Full-Menu Beta 0.2.0 is complete only when:

1. No supported menu depends on fixtures or page-only success state in authenticated runtime.
2. Access Control and every asset-detail section meet their evidence boundaries above.
3. All seven technical tasks have reproducible contract, integration, authorization, restart and
   desktop evidence with no blocking review finding.
4. REST, MCP, CLI and TypeScript SDK parity is demonstrated by equal plan digests and refusals.
5. One hosted human journey and one independent machine-client journey complete against TLS, real
   OIDC, a live model provider and a read-only PostgreSQL execution source.
6. Backup/restore, migration rollback where supported, credential revocation, webhook dead-letter,
   Embedding rebuild recovery and query limits are exercised.

## Approval Effect

Status `Confirmed` authorizes technical planning and continuous execution of FMB-T001 through
FMB-T007 without per-task founder approval. Task completion still requires its declared evidence;
version 0.2.0 release acceptance remains a separate final verdict.

# Semlia Controlled-User Alpha Specification

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Source | Founder request on 2026-09-04; `docs/SSOT.md` M1-M3 and S-001 through S-005 |
| Last updated | 2026-09-04 |
| Review | Founder approved specification 0.1.0 and recommended technical planning on 2026-09-04 |

## Product Positioning

Semlia Controlled-User Alpha is a self-hosted product slice for 5 to 10 invited users in one
trusted organization. It proves that authenticated people can connect one read-only PostgreSQL
source, govern discovered semantic knowledge into an immutable release, and consume that release
through a real semantic query and Ask flow.

This Alpha is not a public release, a production security-support commitment, or completion of
the M4 Continuous Governance and M5 Open Ecosystem milestones.

## Target Users

- Workspace administrator: admits members, assigns the initial workspace roles, and configures the
  Alpha environment.
- Semantic engineer: connects a source, runs discovery, inspects evidence, and proposes semantic
  changes.
- Independent reviewer and publisher: review and release changes under the existing separation-of-
  duties rules.
- Semantic consumer: asks a governed question and inspects the resolved plan, evidence, release,
  and refusal reason.

## User Stories And Scenarios

### US-001 Invited user access

An invited person signs in through one configured OIDC provider, receives a server-side session,
and sees only workspaces where an active membership exists. Logout, expiry, revocation, and removed
membership take effect without trusting a client-supplied principal identifier.

### US-002 PostgreSQL source to governed candidate

An authorized semantic engineer configures one read-only PostgreSQL source, stores its credential
through the protected secret boundary, verifies connectivity, starts discovery, and sees the
persisted run, discovered datasets and fields, evidence, and semantic candidates.

### US-003 Governed release

The engineer converts a discovered candidate into a proposal. Independent users validate, review,
publish, reload, and roll back the change through the existing immutable M2 workflow.

### US-004 Trusted semantic consumption

An authenticated consumer submits a SemanticQuery or natural-language Ask request against an
explicit current or pinned release. Semlia returns an explainable ResolvedSemanticPlan when the
request is unambiguous and refuses not-found, ambiguous, unauthorized, stale-binding, and invalid-
plan cases with stable reason codes.

### US-005 Honest product surface

Every primary navigation entry either executes a real persisted backend workflow in the Alpha
scope or carries a visible prototype disclosure. Prototype actions never report persisted success
and never substitute fixtures after a real API failure.

## Core User Journey

1. The administrator configures one OIDC provider and admits the Alpha users to a workspace.
2. A semantic engineer signs in, configures a read-only PostgreSQL source, tests it, and starts a
   discovery run.
3. Semlia persists the source revision, physical graph, evidence, and semantic candidates without
   collecting warehouse fact rows.
4. The engineer opens a candidate, creates a proposal, and submits it for validation.
5. Independent reviewer and publisher sessions approve and publish an immutable release.
6. A consumer asks a question, inspects the matched semantic assets, joins, filters, release and
   evidence, and receives either a ResolvedSemanticPlan or a specific refusal.
7. A publisher rolls back the release; subsequent unpinned queries resolve against the new current
   release while pinned consumers retain their declared release binding.

## Functional Requirements

### FR-001 OIDC authentication

The service supports one standards-compliant OIDC issuer through Authorization Code with PKCE,
validates issuer, audience, signature, state and nonce, and exposes sign-in, callback, current-
session and logout endpoints. Provider secrets never enter browser-visible configuration.

### FR-002 Server-side session

The service issues an opaque session cookie whose verifier is stored only as a hash. The cookie is
HttpOnly, SameSite=Lax, Secure outside local development, scoped to the service, and has explicit
idle and absolute expiry. Logout, administrative revocation, expiry, and subject suspension deny
the next protected request.

### FR-003 Membership and identity authority

OIDC issuer plus subject maps to one human principal. A workspace membership maps that principal
to active or suspended access and existing role bindings. Protected API identity is derived from
the validated session; `X-Semlia-Principal` remains a development-only local-UAT mechanism and is
not accepted as external-user authentication. Unsafe cookie-authenticated requests enforce Origin
and CSRF protection.

### FR-004 Controlled admission

Workspace administrators can create, inspect, suspend and revoke invitations or memberships for
the 5 to 10 Alpha users. Admission is allowlist-based; successful OIDC authentication without an
active invitation or membership does not create workspace access. At least one active workspace
administrator must remain.

### FR-005 PostgreSQL source configuration and credential protection

An authorized user can create, update, test, disable and rotate one PostgreSQL source connection.
Connection secrets use a versioned authenticated-encryption envelope rooted in the configured
Semlia secret key, are redacted from APIs, logs, traces, audit payloads and Agent inputs, and are
never stored as ordinary plaintext fields. The source account is read-only and connection testing
has bounded timeouts.

### FR-006 Discovery run and semantic candidates

An authorized user can start an asynchronous discovery run and inspect queued, running, succeeded,
degraded and failed states. A successful run persists source revision, datasets, fields, keys,
lineage, versioned SQL evidence and semantic candidates through the existing M1 contracts. Retry
and restart do not silently duplicate canonical objects, and failure never reports imported data
as current.

### FR-007 Live governed authoring proof

One configured OpenAI-compatible model setting can generate a schema-valid proposal through the
existing M2 gate. Provider unavailability records a failed run and creates no proposal. The seeded
non-provider path remains available for deterministic regression tests but is not presented as
live generation evidence.

### FR-008 SemanticQuery and immutable release binding

The service accepts a typed SemanticQuery containing intent, measures, dimensions, filters, time
range and optional release or consumer binding. Resolution reads published assets and governance
objects from an immutable release, never draft or merely approved state. A successful response is
a typed ResolvedSemanticPlan containing selected assets and revisions, joins, filters, grouping,
ordering, release identity, evidence references and validation status.

### FR-009 Ambiguity and refusal contract

Resolution refuses requests with stable codes for no match, multiple material matches, missing
join path, incompatible grain, unauthorized asset, stale release binding and invalid plan. The
response names candidate identities and the minimum clarification needed without inventing a
choice or executing SQL.

### FR-010 Real Ask flow

The Ask surface sends a real API request, shows progress, and renders the authoritative answer
summary, ResolvedSemanticPlan, evidence and release. Natural-language interpretation may use the
configured model, but deterministic resolution and validation remain server-owned. Model or API
failure renders an error or refusal and never falls back to fixture content.

### FR-011 Prototype boundary

Primary navigation remains complete. Surfaces outside FR-001 through FR-010 display a consistent
prototype label, keep mutations session-only, and state that refresh clears them. Their controls
cannot emit a persisted-success notification.

### FR-012 Audit, migration and recovery

Authentication, membership, secret operations, source changes, discovery, semantic resolution,
refusals, proposal generation, review, publish and rollback emit attributable audit or usage facts
with trace IDs. Schema migrations support populated upgrade, downgrade where contractually safe,
restart recovery and current-version readiness checks.

## Non-Functional Requirements

### NFR-001 Security and privacy

- Authentication and authorization fail closed.
- Session, CSRF, invitation and source-secret tokens have at least 128 bits of entropy and are
  stored or compared through one-way or authenticated cryptographic primitives as appropriate.
- Warehouse access is metadata-only by default; customer fact rows, raw query results, hidden model
  reasoning and recoverable source credentials are not retained.
- Dependency, secret and release-image scans report zero High or Critical findings.

### NFR-002 Reliability and recovery

- Source discovery and validation work is leased and retryable through the existing job framework.
- A process or database restart preserves sessions, source configuration, run status, proposals,
  reviews, releases and bindings.
- A partial connector or model failure is explicit and does not advance semantic authority.

### NFR-003 Performance

- Session validation and deterministic semantic resolution each complete within 1 second at p95 on
  the Alpha reference stack with 10,000 catalog assets, excluding OIDC, model and source-network
  latency.
- The UI exposes progress for operations that exceed 1 second and never blocks navigation while a
  discovery or model job is running.

### NFR-004 Accessibility and desktop support

Sign-in, source setup, discovery, governance, Ask, refusal and session controls are keyboard
operable with visible focus and reduced-motion support at 1440x900 and 1024x768. No mobile-specific
scope is introduced.

### NFR-005 Observability

Every boundary request carries a trace ID. Health and readiness distinguish database, migration,
session-store and worker failure without exposing credentials, tokens or provider responses.

## Functional Scope

- One self-hosted Semlia instance and one configured OIDC issuer.
- One organization, multiple workspaces, and 5 to 10 invited human users.
- One PostgreSQL/compatible read-only source using Catalog metadata and versioned SQL evidence.
- Existing M1 catalog and M2 governed authoring, plus the M3 REST SemanticQuery and Ask slice.
- Current-release and explicitly pinned-release resolution.
- Desktop and compact-desktop browser acceptance.

## Non-Goals

- Public signup, password authentication, account recovery, SCIM or multiple simultaneous identity
  providers.
- M4 scoring, attention queues, shadow evaluation, canary, automatic publish or automatic rollback.
- M5 plugin SDK, marketplace, Helm, high availability, multi-region operation or public support SLA.
- Arbitrary warehouse query execution, BI dashboarding, persisted customer fact results or a general
  SQL editor.
- Mobile navigation, mobile acceptance, offline mode or native desktop packaging.
- Complete MCP, CLI, webhook and external Agent ecosystem acceptance in this Alpha sprint.

## Edge Cases

- The OIDC callback is replayed, state or nonce is missing, or the subject email changes.
- An invited identity authenticates with a different issuer or subject than the accepted mapping.
- The final workspace administrator is suspended or removes their own membership.
- A session is active while membership, roles or authorization version changes.
- A source credential rotates during discovery, the database is unreachable, or permissions are
  reduced after a successful connection test.
- Two discovery runs overlap or observe the same database revision.
- A query has multiple equally strong asset matches, no valid join path, incompatible grains, or a
  release that was rolled back after the consumer pinned it.
- The model emits invalid structured output or becomes unavailable after a run is recorded.

## Constraints And Assumptions

- PostgreSQL remains authoritative for sessions, identity mappings, memberships, workflow and
  indexes; Git remains authoritative for released semantic content.
- The existing authorization action vocabulary, separation-of-duties rules, TypeIDs, job/outbox
  framework and immutable release model are reused.
- An OIDC issuer, client credentials and exact callback URL are available before hosted user
  acceptance. Implementation and automated tests use a standards-compliant local test issuer.
- A read-only PostgreSQL sample source with representative schemas and versioned SQL artifacts is
  available before the source-to-query acceptance run.
- The 4 to 7 day estimate assumes one focused product slice, immediate review decisions, no schema
  reset of existing M1/M2 work, and no expansion into the Non-Goals.

## Data And Integration Needs

- OIDC issuer discovery/JWKS, client ID, client secret, redirect URI and invited issuer-subject or
  email mapping.
- Session, invitation, membership and external-identity records with expiry, revocation and audit
  attribution.
- Versioned encrypted source credential material and a redacted connection projection.
- Existing source revisions, physical assets, evidence, semantic assets, governance objects and
  immutable release manifests.
- SemanticQuery, ResolvedSemanticPlan, resolution candidate, validation and refusal contracts.

## Success Criteria

- Five invited users can sign in and complete their permitted workflow without using local actor
  aliases or copying principal TypeIDs.
- One PostgreSQL source reaches a persisted successful discovery run and produces inspectable
  physical metadata, evidence and at least one governable semantic candidate.
- One candidate completes proposal, validation, independent review, publish and rollback with all
  facts visible after browser and service restart.
- Ten golden SemanticQuery cases produce the expected release-pinned plan; ambiguity, not-found,
  unauthorized and invalid-join cases produce the expected refusal code.
- One real Ask journey uses the configured provider and published release, while provider failure
  produces no fixture answer and no false persisted-success state.
- Source, integration, security, release, migration/recovery and both supported desktop E2E gates
  pass with evidence tied to the tested build.

## Acceptance Criteria

- AC-001: A request without a valid session receives `401`; a valid session without workspace
  permission receives `403`; client principal headers cannot alter either outcome.
- AC-002: Logout, session revocation and membership suspension deny the next protected request.
- AC-003: APIs and observability output expose no OIDC client secret, session verifier or source
  password; database source secrets are authenticated ciphertext.
- AC-004: A browser user can configure and test a read-only PostgreSQL source, run discovery, reload,
  and inspect persisted run and candidate state.
- AC-005: Distinct authenticated users complete propose, review and publish; author-reviewer and
  reviewer-publisher conflicts remain denied and audited.
- AC-006: SemanticQuery resolves only immutable released content and returns a complete validated
  ResolvedSemanticPlan for the golden success cases.
- AC-007: Ambiguous or invalid requests return stable refusal codes and an actionable clarification;
  no silent candidate selection occurs.
- AC-008: Ask displays real API state, evidence and release attribution at both supported viewports;
  network and model failures render as failures without fixture fallback.
- AC-009: Every out-of-scope page is visibly marked as prototype and cannot report persistent
  success.
- AC-010: Populated migration, database interruption/recovery, service restart, source retry,
  complete-stack smoke, security scan and release artifact checks pass.

## Clarifications

- The delivery target is a controlled Alpha for 5 to 10 users, not the complete M4/M5 roadmap.
- The sprint is time-boxed to 4 to 7 days with Codex and keeps only one source type, one OIDC issuer
  and the REST Ask/SemanticQuery path on the critical path.
- Full primary navigation remains visible; implementation status is communicated per surface rather
  than by removing menus.
- The founder approved version 0.1.0 on 2026-09-04 and authorized creation of the recommended
  technical plan and execution sequence.

## Open Questions

There are no blocking requirement questions. OIDC credentials and the read-only PostgreSQL sample
are acceptance inputs supplied before hosted user testing, not product-contract decisions.

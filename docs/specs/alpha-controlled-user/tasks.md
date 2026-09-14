# Controlled-User Alpha Work Graph

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Plan | `docs/specs/alpha-controlled-user/plan.md` 0.1.0 Confirmed |
| Last updated | 2026-09-04 |

## Status Contract

- `Draft`: defined but not approved for packet creation.
- `Ready`: dependencies, approved contracts, checklist, analysis and execution packet are complete.
- `Running`: implementation is active.
- `Needs_Review`: implementation and evidence pass; founder acceptance remains.
- `Accepted`: explicitly accepted or included in an accepted Alpha release.
- `Blocked`: a named external or technical condition prevents meaningful progress.
- `Superseded`: a later confirmed decision replaces the task.

Founder approval of this graph authorizes continuous execution through T007. A dependency at
`Needs_Review` with passing evidence is sufficient for the next packet; `Accepted` remains an
explicit founder verdict rather than a routine implementation pause.

## Work Units

### T001 Session identity and controlled admission

Status: Needs_Review
Depends on: Controlled-User Alpha spec Confirmed; Alpha TDR/plan/tasks approved
Blocks: T002, T003, T005

Deliver the identity/session migration, generic OIDC Authorization Code plus PKCE flow, opaque hashed
sessions, CSRF and Origin enforcement, external identity mapping, active membership filtering,
invitations, final-admin protection, revocation and an idempotent first-admin bootstrap command.
Introduce request-context identity and ensure session mode ignores external principal headers while
development-only local UAT remains explicit.

Red scenarios: replayed callback, invalid issuer/audience/signature/state/nonce, expired or revoked
session, suspended membership, header spoofing, cross-workspace access, missing CSRF, invalid Origin,
duplicate invite acceptance and final-admin removal.

Validation: focused domain tests; local issuer contract tests; real PostgreSQL populated migration;
HTTP 401/403 contract; session restart and revocation tests; config/readiness tests; standard source
gates.

Packet: `docs/specs/alpha-controlled-user/packets/T001.yaml`.

### T002 Authenticated Web shell and member administration

Status: Needs_Review
Depends on: T001 Needs_Review or Accepted
Blocks: T007

Connect sign-in, callback completion, current session, workspace switching, logout and member/
invitation administration to generated APIs. Use one session-aware client and CSRF injection path.
Preserve the accepted desktop shell and navigation; render loading, unauthenticated, denied, empty
membership, expired session and provider failure states without local fixture fallback.

Red scenarios: callback error, user with no admitted workspace, `401` during navigation, `403` after
role change, final-admin command denial and refresh after logout.

Validation: Testing Library session/capability cases; two-principal live browser flow; keyboard,
visible focus, axe and screenshots at 1440x900 and 1024x768; production build.

Packet: `docs/specs/alpha-controlled-user/packets/T002.yaml`.

### T003 Protected source and asynchronous discovery backend

Status: Needs_Review
Depends on: T001 Needs_Review or Accepted
Blocks: T004, T005

Deliver the source/discovery migration, encrypted credential envelope and rotation, source CRUD and
connection test APIs, a read-only live PostgreSQL catalog collector, physical key and Join
observations, allowlisted versioned SQL input,
queued discovery jobs, exact-run persistence, degraded/failed semantics, candidate projection and
candidate decision endpoints. Extend the existing M1 snapshot persistence rather than replacing it.

Red scenarios: plaintext or DSN leakage, wrong encryption key/version, elevated source role,
connection timeout, artifact path traversal, overlapping runs, worker crash, duplicate snapshot,
credential rotation during a run, parser warning and terminal collection failure.

Validation: real PostgreSQL source fixture and privileges; ciphertext/associated-data assertions;
logs/audit/API redaction scan; job retry/restart/idempotency; populated migration lifecycle; source
and candidate HTTP contracts; standard gates.

Packet: `docs/specs/alpha-controlled-user/packets/T003.yaml`.

### T004 Real Source and candidate-to-proposal Web

Status: Needs_Review
Depends on: T003 Needs_Review or Accepted
Blocks: T007

Connect PostgreSQL source setup, write-only credential rotation, connection test, discovery start,
run list/detail/findings and semantic candidate list/detail/conversion to real APIs. Conversion opens
the existing M2 proposal workbench with stable source evidence. Label file sources, generic build
tasks and other non-Alpha source actions as Prototype and prevent fake persisted-success feedback.

Red scenarios: secret field prefill, connection failure, queued/running/degraded/failed run, refresh
during a run, retry, candidate already converted, proposal failure and API loss without fixture
fallback.

Validation: component/API tests; live source-to-candidate-to-proposal browser flow; navigation and
viewport regression at 1440x900 and 1024x768; zero serious/critical axe findings.

Packet: `docs/specs/alpha-controlled-user/packets/T004.yaml`.

### T005 M3 Alpha release resolver

Status: Needs_Review
Depends on: T001 and T003 Needs_Review or Accepted; M2 immutable release path present
Blocks: T006

Deliver the M3 Alpha OpenAPI and persistence contracts, consumer and binding administration,
release snapshot projection, typed SemanticQuery, deterministic candidate resolver, plan validator,
immutable ResolvedSemanticPlan and refusal records, generated TypeScript client and attributable
usage/audit facts. Resolution supports current, explicit and binding-selected release contexts and
never consults draft state after selection.

Red scenarios: no release, missing/expired/inactive binding, explicit cross-workspace release,
no-match, materially tied candidates, missing binding, missing join path, incompatible grain,
unauthorized candidate, stale object version, invalid filter/time semantics, concurrent publish and
resolver restart.

Validation: domain golden/property tests; real PostgreSQL release snapshot tests; stable digest
across restart; current/pinned behavior across publish and rollback; 10,000-asset p95 benchmark;
OpenAPI/sqlc/SDK drift; standard gates.

Packet: `docs/specs/alpha-controlled-user/packets/T005.yaml`.

### T006 Real Ask and honest product boundary

Status: Needs_Review
Depends on: T005 Needs_Review or Accepted; existing M2 model provider path
Blocks: T007

Add schema-constrained Ask interpretation through the configured model, agent-run attribution and
the shared T005 resolver. Replace Ask fixtures and timers with real pending, definition, plan,
not-configured execution, clarification, refusal and provider error states. Audit all navigation
surfaces and standardize Prototype disclosure and session-only mutation feedback outside the Alpha
real scope.

Red scenarios: invalid model JSON, model timeout/unavailable, prompt attempts to inject SQL/source
credentials, ambiguity, unauthorized asset, no execution adapter, API failure and stale release.
No path may display fixture evidence, a Cube execution claim or fabricated numeric values.

Validation: schema contract tests; configured live provider evidence plus deterministic provider
stub failures; React behavior and accessibility tests; live browser Ask plan/refusal flow at both
supported viewports; route disclosure inventory.

Packet: `docs/specs/alpha-controlled-user/packets/T006.yaml`.

### T007 Controlled-User Alpha acceptance and M2/T011 closeout

Status: Needs_Review
Depends on: T002, T004 and T006 Needs_Review or Accepted
Blocks: Alpha 0.1.0 acceptance recommendation; M2/T011 closeout review

Exercise the entire OIDC-to-source-to-governance-to-query journey with separate engineer, reviewer,
publisher and consumer sessions. Include live proposal generation, publish, reload, rollback,
current and pinned resolution, refusal, logout and session revocation. Prove restart/recovery,
migration, performance, privacy, security, release artifact and desktop accessibility gates.

Red scenarios: provider unavailable, source unavailable, database restart mid-session, worker restart
mid-discovery, credential rotation, publish/rollback race, stale binding, permission removal and
forbidden prototype success.

Validation: `make check-source`, `make check-smoke`, `make security-check`, `make release`, populated
up/down/up migrations, live and deterministic E2E at 1440x900 and 1024x768, 10,000-asset benchmark,
release digest verification and independent specification/security/quality review.

Completion updates M2/T011 to `Needs_Review` only when its remaining evidence is real. The founder
accepts M2/T011 and Alpha 0.1.0 separately after reviewing the evidence and controlled user flow.

Packet: `docs/specs/alpha-controlled-user/packets/T007.yaml`.

## Requirement Traceability

| Requirement | Tasks |
| --- | --- |
| FR-001, FR-002, FR-003, FR-004 | T001, T002, T007 |
| FR-005, FR-006 | T003, T004, T007 |
| FR-007 | T006, T007 |
| FR-008, FR-009 | T005, T006, T007 |
| FR-010 | T006, T007 |
| FR-011 | T002, T004, T006, T007 |
| FR-012 | T001, T003, T005, T007 |
| NFR-001 | T001, T003, T005, T006, T007 |
| NFR-002 | T001, T003, T005, T007 |
| NFR-003 | T001, T005, T007 |
| NFR-004 | T002, T004, T006, T007 |
| NFR-005 | T001, T003, T005, T006, T007 |

## Approval Gate

The founder approved the Alpha TDR, M3 specification, technical plan and this work graph on
2026-09-04. T001 is Ready; later tasks remain Draft until their dependency evidence and packets are
complete.

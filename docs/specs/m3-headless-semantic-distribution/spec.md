# M3 Headless Semantic Distribution Specification

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Source | `docs/SSOT.md` v0.7.0 Confirmed; `docs/specs/alpha-controlled-user/spec.md` 0.1.0 Confirmed |
| Last updated | 2026-09-04 |
| Review | Founder approved technical baseline 0.1.0 on 2026-09-04 |

## Purpose

M3 makes immutable Semlia releases consumable without trusting an Agent, client, or model to pick
tables, definitions, grain, or joins. A consumer submits a typed `SemanticQuery`; Semlia resolves
only released assets and release-pinned governance objects, returns an explainable
`ResolvedSemanticPlan`, or refuses with a stable reason and the minimum required clarification.

The milestone contract is broader than the Controlled-User Alpha delivery profile. The Alpha
implements the REST SemanticQuery and Ask path, current and explicit release selection, a minimal
consumer binding contract, deterministic resolution, validation, refusal, attribution, and the
generated TypeScript client. MCP, CLI, webhooks, external execution adapters, and independent
consumer acceptance remain part of the full M3 milestone and are not claimed by Alpha 0.1.0.

## Users And Consumers

- Human semantic consumer using the Ask workspace.
- First-party Web client using the versioned REST contract.
- Registered application or Agent bound to a current or pinned release.
- Platform operator diagnosing resolution, refusal, authorization, and compatibility outcomes.

## Delivery Profiles

| Capability | Alpha 0.1.0 | Full M3 |
| --- | --- | --- |
| Versioned REST read and resolution | Required | Required |
| Natural-language Ask to typed SemanticQuery | Required | Required |
| Current and explicit release selection | Required | Required |
| Minimal consumer registration and binding | Required | Required |
| Deterministic plan and refusal | Required | Required |
| TypeScript generated client | Required | Required |
| Warehouse result execution | Not included | Optional adapter |
| MCP read resources | Prototype label only | Required |
| CLI consumption | Not included | Required |
| Webhooks and ecosystem delivery | Not included | Required |
| Fluxale plus independent Agent acceptance | Not included | Exit criterion |

## Functional Requirements

### M3-FR-001 Versioned read surface

REST exposes release, released-asset, release-object, consumer, binding, SemanticQuery and resolved
plan resources under `/api/v1`. Public DTOs have explicit schema versions, generated Go and
TypeScript types, workspace isolation, stable TypeIDs and the existing error envelope.

### M3-FR-002 Typed SemanticQuery

`SemanticQuery` contains a versioned intent, measure selectors, dimension selectors, filters,
optional time range, ordering and limit, plus exactly one resolution context: current release,
explicit release ID, or active consumer binding. A selector uses a stable asset reference or a
search term; an empty or structurally invalid query is rejected before resolution.

Alpha intents are `describe`, `aggregate`, `breakdown`, and `compare`. The contract expresses
semantic intent only and cannot carry arbitrary SQL, table names, ungoverned expressions, or an
execution credential.

### M3-FR-003 Immutable release resolution

Current resolution selects the highest published release sequence for the workspace. Explicit and
binding-based resolution select the exact immutable release they name. Assets use release manifest
revisions. PhysicalBinding, ModelGrain, EntityKey and JoinContract use release object entries and
their pinned versions. Draft assets, current mutable pointers and objects absent from the selected
release never enter a plan.

### M3-FR-004 Deterministic candidate selection

The resolver applies a versioned deterministic order: explicit TypeID or stable address, normalized
exact name or alias, then bounded lexical search. A lexical result is accepted only when one
authorized candidate clears the calibrated confidence and separation rules. Material ties return
an ambiguity refusal. The model may propose selectors but cannot select the final asset, binding or
join.

### M3-FR-005 ResolvedSemanticPlan

A successful plan records the SemanticQuery digest, selected release, resolver version, selected
asset and revision identities, object versions, physical bindings, grain, joins, predicates,
grouping, ordering, evidence references, validation results, execution capability and a canonical
plan digest. The plan is immutable and reproducible from the selected release.

### M3-FR-006 Plan validation and refusal

Validation checks release membership, authorization, binding availability and freshness, selector
type, filter compatibility, time semantics, grain compatibility, join reachability and JoinContract
cardinality. No-match, ambiguity, missing binding, missing join path, incompatible grain,
unauthorized asset, stale release binding and invalid plan produce stable refusal codes. Candidate
identities are returned only when the caller is authorized to read them.

### M3-FR-007 Execution boundary

Resolution and execution are separate capabilities. A valid plan can report
`executionStatus=not_configured` without failing semantic resolution. Alpha 0.1.0 never sends a plan
to a warehouse and never returns a fabricated fact result. Full M3 may add a typed adapter after an
independent security and execution contract is confirmed.

### M3-FR-008 Consumers and bindings

Authorized administrators can register, inspect, suspend and revoke consumers and create current
or pinned release bindings with purpose, environment, compatibility constraint and expiry.
Resolution through a binding fails closed when the consumer or binding is inactive, expired,
incompatible or outside the workspace.

### M3-FR-009 Ask interpretation

Ask submits natural language to the configured model as a schema-constrained interpretation run.
The service stores the input digest and run metadata but not the raw prompt by default. Valid model
output enters the same deterministic resolver as direct SemanticQuery. Invalid output, provider
failure, ambiguity and resolution refusal remain explicit; no fixture or heuristic answer replaces
them.

### M3-FR-010 Ask response semantics

For `describe`, Ask can summarize released definitions and cited evidence. For data intents, Alpha
returns a plan explanation and clearly states that data execution is not configured; it does not
display numeric results. Every response identifies the release and source evidence or a refusal
reason and clarification request.

### M3-FR-011 Attribution and privacy

Resolution, refusal and feedback events identify workspace, principal, release, optional consumer
and binding, channel, outcome, reason code, trace ID and idempotency key. Raw prompts, fact rows,
full query results, source credentials and hidden model reasoning are not retained. Audit, usage,
outbox and telemetry facts keep their existing separate responsibilities.

### M3-FR-012 Channel parity and compatibility

All delivery channels share one canonical SemanticQuery, plan and refusal contract. Full M3 adds
MCP and CLI conformance over the same application service rather than reimplementing resolution.
Breaking schema changes require a new major spec version, compatibility report and migration path.

## Non-Functional Requirements

### M3-NFR-001 Correctness

- Identical SemanticQuery plus release plus resolver version produces the same plan digest.
- A plan cannot reference an asset revision or governed object version outside its release.
- Resolver and validation failure never executes a query or creates a successful plan.

### M3-NFR-002 Security

- Authentication is required for all workspace consumption endpoints.
- `semantic.resolve`, `binding.read`, `binding.manage` and future `semantic.execute` remain distinct
  server-side actions.
- Unauthorized and cross-workspace resources do not leak names, candidates, plans or existence.

### M3-NFR-003 Performance

- Alpha deterministic resolution completes within 1 second at p95 on the reference stack with
  10,000 catalog assets, excluding model and external network latency.
- Full M3 retains the SSOT plan-validation target of 2 seconds p95.

### M3-NFR-004 Reliability

- Resolution records and usage attribution commit atomically or remain absent.
- Process and database restart preserve bindings, queries, plans and refusal facts.
- Provider failure and absent execution adapters do not corrupt release or plan authority.

### M3-NFR-005 Accessibility

The Ask result, plan, evidence, clarification and refusal states are keyboard operable with visible
focus and reduced-motion support at 1440x900 and 1024x768.

## Alpha API Shape

The technical plan may refine names without weakening this behavior:

- `POST /api/v1/workspaces/{workspaceId}/semantic-queries:resolve`
- `GET /api/v1/workspaces/{workspaceId}/semantic-queries/{queryId}`
- `GET /api/v1/workspaces/{workspaceId}/resolved-semantic-plans/{planId}`
- `POST /api/v1/workspaces/{workspaceId}/ask`
- `GET|POST /api/v1/workspaces/{workspaceId}/consumers`
- `GET|PATCH /api/v1/workspaces/{workspaceId}/consumers/{consumerId}`
- `GET|POST /api/v1/workspaces/{workspaceId}/consumer-bindings`
- `GET|PATCH /api/v1/workspaces/{workspaceId}/consumer-bindings/{bindingId}`

## Acceptance Scenarios

1. An authenticated consumer resolves an explicit stable metric and dimension against the current
   release and receives a plan containing only manifest-pinned revisions and object versions.
2. The same canonical query against the same release produces the same plan digest after restart.
3. A pinned binding continues to resolve its release after a later publish or rollback; a current
   binding follows the highest release sequence.
4. Two materially equal authorized matches return `AMBIGUOUS_MATCH`, candidate identities and one
   specific clarification request; no plan is created.
5. Missing JoinContract, incompatible grain and stale binding each return their own refusal code.
6. A caller without `semantic.resolve` receives 403; an unauthenticated caller receives 401; neither
   response leaks release content.
7. Ask `describe` cites released definitions and evidence. Ask for a numeric result explains the
   resolved plan and `not_configured` execution state without inventing a number.
8. Invalid model output and provider failure create failed run evidence and never fall back to the
   existing fixture answer.
9. The generated TypeScript client passes contract drift checks and drives the real Web Ask flow.

## Full M3 Exit Criterion

M3 is Accepted only when Fluxale and at least one independent Agent consume production-style
released semantics exclusively through Semlia, including REST, MCP and CLI conformance, binding
attribution, failure/refusal behavior and compatibility evidence. Alpha 0.1.0 proves only the
explicit delivery profile in this specification.

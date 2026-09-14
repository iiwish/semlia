# Full-Menu Beta Requirements Checklist

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.2.0 |
| Status | Confirmed |
| Source research | `docs/specs/full-menu-beta/research.md` 0.2.0 Confirmed |
| Last updated | 2026-09-04 |

## Checklist Semantics

This is the implementation acceptance checklist for every visible authenticated menu. `Confirmed`
means the requirement contract is approved; unchecked items remain required until linked evidence
proves them in a production-mode build.

## Production Truthfulness

- [ ] The production Web build cannot enable fixture catalog, fixture governance, local-UAT identity,
  static menu records, or simulated command paths. [Release]
- [ ] A production backend rejects development identity headers even if a client sends them. [Security]
- [ ] No production route displays Prototype content, generated operational records, preview bindings,
  fixed progress/logs, or fixture-derived authorization decisions. [Reliability]
- [ ] No timer, toast, or component-state mutation is treated as proof that a command succeeded. [Reliability]
- [ ] Unsupported capabilities are absent or disabled with an accurate reason. [Clarity]
- [ ] Every success notification follows authoritative confirmation and the result survives hard reload
  and a second authenticated session. [Durability]

## Semantic Ask And Workbench

- [ ] Ask executes through an approved read-only adapter and distinguishes result, semantic refusal,
  execution refusal, unavailable adapter, timeout, cancellation, and provider failure. [Coverage]
- [ ] Ask never fabricates values, evidence, latency, query execution, or a successful downstream channel. [Safety]
- [ ] Conversation history and titles are durable when their controls are visible; otherwise those
  controls are absent. [Consistency]
- [ ] Workbench items, counters, filters, ordering, assignments, dates, and resource links come from an
  authoritative workspace feed with pagination. [Data]
- [ ] Every workbench deep link resolves the same asset, proposal, review, release, run, or audit object
  after reload. [Navigation]

## Knowledge Assets

- [ ] Catalog list and search expose server totals or cursors and do not silently stop at a fixed first page. [Scale]
- [ ] Asset Overview waits for authoritative detail and does not synthesize owner, release, readiness,
  quality, or health claims after a failed request. [Truthfulness]
- [ ] Definition drafts are persisted or the save-draft command is absent. [Durability]
- [ ] Definition revision comparison loads two immutable server revisions and renders a real diff. [Correctness]
- [ ] Ontology relations and consistency status are backed by persisted evidence and revision comparison. [Correctness]
- [ ] Implementation loads authoritative physical bindings and JoinContracts with explicit empty and
  unavailable states. [Completeness]
- [ ] Trust uses persisted validation runs and quality evidence; lack of a run cannot pass a validation rule. [Safety]
- [ ] A local form check is labeled as form completeness and never as a passed server precheck. [Clarity]
- [ ] Delivery and impact use real releases, registry pointers, consumer bindings, and impact analysis. [Data]

## Changes And Releases

- [ ] Proposal creation, validation, review, decision, batching, publish, and rollback use authoritative
  governance commands with idempotency and stale-version handling. [Workflow]
- [ ] Asset Versions uses real consumer bindings rather than preview data. [Data]
- [ ] Release comparison reads the current registry pointer and selected pinned revision independently. [Correctness]
- [ ] Conflict, forbidden, dependency, policy, and partial-failure outcomes remain visible and actionable. [Recovery]

## Data Access

- [ ] PostgreSQL create, test, credential rotation, disable, discovery, candidate review, and deletion
  remain fully live and secret-redacted. [Security]
- [ ] File ingestion provides staged upload, preview, validation, durable source revision, progress,
  cancellation or retry, and malformed/oversized/unsupported-file handling; otherwise the File entry is absent. [Coverage]
- [ ] Access Automation persists schedule/build-task create, edit, pause, resume, delete, and run-now
  commands with validated cron and timezone behavior; otherwise the entry is absent. [Coverage]
- [ ] Access Runs provides workspace-level paged history, stages, durable logs, and retry/cancel rules. [Operations]
- [ ] Deleting or disabling a source does not make its historical runs or audit evidence unreachable. [Audit]
- [ ] Candidate Assets reports empty, degraded, stale-run, permission, and load-failure states explicitly. [Edge Cases]

## System Settings

- [ ] Members and invitations use identity APIs, navigation counts are authoritative, and local-UAT
  identities cannot enter a release build. [Identity]
- [ ] Roles and Permissions reads persisted system/custom roles and persists allowed custom-role changes
  with immutable-system-role and version-conflict enforcement. [Authorization]
- [ ] Role Assignments persists principal/group/scope bindings and enforces separation-of-duties and
  last-admin constraints on the server. [Authorization]
- [ ] Effective Access calls the authoritative evaluator and displays decision, reason code, matched
  binding/policy, scope, and authorization version. [Authorization]
- [ ] Session capabilities include custom-role effects and refresh on workspace, identity, role, or
  authorization-version change. [Consistency]
- [ ] LLM controls expose only provider/model combinations implemented by a tested runtime adapter. [Configuration]
- [ ] Embedding rebuild is a durable job with real index version, progress/heartbeat, logs, build/swap,
  cancel, retry, terminal failure, and restart recovery. [Operations]
- [ ] Interfaces and Integrations persists machine-client create, rotate, revoke, enable, and disable;
  secrets are shown once and never recoverable afterward. [Security]
- [ ] REST, MCP, CLI, SDK, and Webhook entries appear only when their artifact, authentication,
  compatibility contract, endpoint, and smoke test exist. [Distribution]
- [ ] Copy commands write the intended value to the clipboard and report permission or write failure. [Interaction]
- [ ] Audit and Runtime reads paged immutable audit events and real job/run stages, progress, logs,
  actors, outcomes, resources, trace IDs, and timestamps. [Observability]
- [ ] Audit export downloads a real redacted artifact and records the export action. [Audit]
- [ ] Runtime settings persist with permission and version enforcement. [Configuration]
- [ ] OpenTelemetry probe contacts the configured endpoint and cannot report a simulated pass. [Observability]

## Shared States And Consistency

- [ ] Every menu and subpage has loading, empty, recoverable error with retry, and forbidden states. [UX]
- [ ] Every mutation has pending, validation failure, authorization failure, conflict, network failure,
  and confirmed-success behavior appropriate to its risk. [UX]
- [ ] A `401` leads to session recovery or sign-in without exposing stale protected content. [Security]
- [ ] A `403` is authoritative, explains the blocked action, and refreshes capability state. [Authorization]
- [ ] Optimistic updates reconcile with the returned server version and roll back visibly on failure. [Consistency]
- [ ] Catalog, workbench, runs, audit, clients, and every unbounded collection use authoritative pagination. [Scale]
- [ ] Cross-resource links and filters remain valid after reload, workspace switch, and logout/login. [Navigation]
- [ ] Secrets, DSNs, tokens, model keys, and client credentials are absent from URLs, logs, traces,
  audit payloads, persistent browser storage, and fixture evidence. [Privacy]

## Desktop, Keyboard And Accessibility

- [ ] At 1440x900, every route and subpage keeps navigation, context, toolbar, content, dialogs, and
  primary commands visible or deliberately scrollable without overlap. [Desktop]
- [ ] At 1024x768, every route and subpage remains usable without page-level horizontal scrolling;
  dense data grids use an explicit internal scroll region. [Compact Desktop]
- [ ] Navigation rails, tabs, filters, tables, rows, menus, forms, and commands are fully keyboard reachable. [Keyboard]
- [ ] Focus indicators remain visible against every supported state and are not clipped by containers. [Accessibility]
- [ ] Dialogs trap focus, close with Escape when safe, and restore focus to the invoking control. [Keyboard]
- [ ] Async validation, errors, progress, and completion are announced without unexpected focus movement. [Accessibility]
- [ ] Reduced-motion preference disables nonessential movement without hiding state changes. [Accessibility]
- [ ] Long names, translated labels, errors, identifiers, and timestamps do not overlap or escape controls. [Layout]

## External Acceptance Inputs

- [ ] Hosted OIDC issuer, client credentials, redirect URLs, and representative role identities are
  supplied for hosted authentication evidence. [External]
- [ ] Public hostname, TLS, allowed origins, cookie policy, and encryption/session key material are
  supplied and validated for the release environment. [External]
- [ ] A representative read-only PostgreSQL source and expected catalog objects are supplied without
  requiring collection of customer fact rows. [External]
- [ ] Valid, malformed, oversized, and unsupported non-sensitive file samples are supplied for ingestion evidence. [External]
- [ ] Credentials exist for every selectable LLM/embedding provider; providers without credentials may
  show an explicit unconfigured state but still require an implemented adapter. [External]
- [ ] A read-only query execution target, sample dataset, and expected results are supplied before Ask
  execution can be accepted as successful. [External]
- [ ] An OpenTelemetry endpoint and authentication are supplied for telemetry probe evidence. [External]
- [ ] Every visible distribution channel has a release artifact and endpoint inventory supplied for smoke testing. [External]
- [ ] Missing external infrastructure blocks only the corresponding hosted evidence and never activates
  fixture fallback, simulated success, or an unverified capability claim. [Boundary]

## Release Evidence Gate

- [ ] Production-mode automated checks prove that fixture and local-UAT code paths are disabled. [Evidence]
- [ ] API, persistence, authorization, secret-redaction, conflict, retry, and restart tests cover each
  implemented command family. [Evidence]
- [ ] Playwright journeys cover sign-in, source/file/schedule, run, candidate, proposal, review, release,
  Ask execution, and audit trace with distinct user roles. [Evidence]
- [ ] Screenshot evidence covers every visible menu and subpage at 1440x900 and 1024x768. [Evidence]
- [ ] No unchecked requirement is waived implicitly; each exception has an explicit scope decision that
  removes or disables the corresponding visible capability. [Release]

# Full-Menu Beta Technical Research

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.2.0 |
| Status | Confirmed |
| Date | 2026-09-04 |
| Scope | All visible Web menus and subpages in authenticated runtime |
| Supported viewports | 1440x900 normal desktop; 1024x768 compact desktop |

## Research Conclusion

Full-Menu Beta means every visible authenticated route is backed by authoritative runtime data and
durable commands. A disclosed prototype is useful during development but is not an accepted Beta
implementation. Fixture data remains available only in an explicitly selected test build and cannot
be imported, rendered, or used as a fallback by the production runtime.

The current repository has a strong live foundation for identity, PostgreSQL discovery, governance,
release operations, model-provider configuration, semantic resolution, and member administration.
It is not yet a full-menu Beta because several visible surfaces still synthesize state, retain changes
only in React state, simulate progress, or acknowledge commands without completing an operation.

## Visible Menu Capability Baseline

| Menu / subpage | Current authoritative capability | Beta gap and minimum closure |
| --- | --- | --- |
| Semantic Ask / New conversation | Resolves released semantic assets through the live semantic resolver | Add a supported execution adapter with result, refusal, timeout, and unavailable states. Persist conversations if history/title controls remain; otherwise remove those controls. Never substitute a generated numeric answer for an unavailable execution path. |
| Workbench | Can navigate to some live governance resources | Replace static tasks, counts, dates, and resource IDs with a workspace-scoped work-item feed. Support server pagination, filter, sort, empty/error states, and deep links that survive reload. |
| Knowledge assets / Catalog | Lists and creates assets through the catalog API | Add cursor pagination beyond the current fixed first page and report partial-load state explicitly. |
| Knowledge assets / Overview | Loads a live asset summary | Load an authoritative detail contract before presenting release, owner, quality, or readiness claims. Detail-fetch failure must not leave a synthetic summary looking current. |
| Knowledge assets / Definition | Displays type-specific definition fields | Persist drafts or remove the save-draft command. Revision comparison must load two real immutable revisions and render their diff. |
| Knowledge assets / Ontology relations | Displays projected relations | Replace generated consistency state with server evidence. Revision comparison must use authoritative ontology revisions. |
| Knowledge assets / Implementation | Displays physical bindings and join contracts in the product shape | Load real binding and join-contract versions, including loading, empty, unavailable, and permission states. |
| Knowledge assets / Trust | Displays quality, validation, and freshness sections | Replace local or vacuous pass conditions with persisted validation runs and source-derived quality evidence. A local form-completeness check cannot be labeled as a passed governance precheck. |
| Knowledge assets / Delivery and impact | Displays deployment and consumer sections | Join the asset with real releases, registry pointers, consumer bindings, and impact analysis. Do not default every asset to unreleased or no consumers. |
| Changes and releases / Proposal workflow | Creates, validates, reviews, decides, batches, publishes, and rolls back through governance APIs | Preserve the live path and add consistent stale-version, conflict, forbidden, retry, and idempotent command handling. |
| Changes and releases / Asset versions | Loads real release records | Replace preview consumer bindings and same-revision comparisons with real consumer-binding and registry-pointer reads. |
| Data access / Data sources / PostgreSQL | Creates, tests, rotates, disables, discovers, and deletes through live APIs | Keep source history addressable after deletion and expose durable credential/version outcomes without revealing secrets. |
| Data access / Data sources / File | Visible entry currently reports prototype status | Add staged upload, schema preview, import validation, persisted source revision, progress, failure recovery, and file-size/type constraints, or remove the entry from the Beta menu. |
| Data access / Access automation | Product-shaped schedule controls only | Add durable schedule/build-task CRUD, cron and timezone validation, pause/resume, run-now, next-run calculation, and audit events, or remove the entry. |
| Data access / Access runs / Runs | Polls live PostgreSQL discovery runs for the selected source | Add workspace-level run history, paging, retry/cancel rules, durable logs, and deleted-source traceability. Polling while a run is queued or running is valid runtime behavior. |
| Data access / Access runs / Candidate assets | Loads live candidates for the selected discovery run | Preserve live loading/error behavior and add stable deep links and explicit empty/degraded states. |
| System settings / Members | Lists members and invitations and performs supported administration through identity APIs | Replace fixed navigation counts with server totals and guarantee that local-UAT identity injection is impossible in production builds. |
| System settings / Access control / Roles and permissions | Product-shaped role editor backed by static definitions | Add persisted system/custom role reads and custom-role CRUD with version conflicts, immutable-system-role rules, and audit records. |
| System settings / Access control / Role assignments | Product-shaped assignment editor backed by component state | Add principal, group, scope, and binding APIs; enforce separation of duties and last-admin rules on the server; refetch after mutation. |
| System settings / Access control / Effective access | Uses a local evaluator over fixture bindings | Add authoritative server-side decision inspection with reason codes, matched bindings, authorization version, and fail-closed error handling. Session capabilities must include custom roles. |
| System settings / Model configuration / LLM | Provider, model, default, and credential revision operations use live governance APIs | Disable unsupported providers in selectable controls or implement them end to end. Provider tests and errors must reflect the configured runtime adapter. |
| System settings / Model configuration / Embedding | Configuration shell is visible | Replace fixed progress and fixed logs with durable rebuild jobs, index build/swap state, progress or heartbeat, cancel/retry, failure recovery, and restart persistence. |
| System settings / Interfaces and integrations | Static channel and machine-client product shape | Add machine-client list/create/rotate/revoke/enable APIs with one-time secret delivery. Show only shipped REST/MCP/CLI/SDK/Webhook channels, use real endpoint and health data, and make copy commands perform clipboard writes with failure feedback. |
| System settings / Audit and runtime / Runs | Static run and log examples | Use the shared workspace job/run service with paging, filters, stages, logs, progress, resource links, and retry/cancel policy. |
| System settings / Audit and runtime / Audit | Static audit examples | Add immutable, paged audit reads with actor, action, resource, outcome, trace, time, redaction, and export download. |
| System settings / Audit and runtime / Settings | Component-local settings and simulated telemetry probe | Persist allowed runtime settings, enforce versions and permissions, and execute a real OpenTelemetry connectivity test before reporting success. |

`/status` is an operational route rather than a visible product menu. Its live, ready, and system
information checks remain part of release diagnostics but do not replace authenticated menu health.

## Truthfulness Boundary

The production runtime must not import or fall back to `governanceFixture`, static workbench tasks,
static audit/run records, static authorization principals/roles/bindings, preview releases, fixed job
logs, or timer-completed commands. A command succeeds only after an authoritative service response;
the UI then refetches or reconciles the returned version. Unsupported operations are absent or
disabled with an accurate reason, never acknowledged with a success toast.

Development fixture mode remains isolated behind an explicit test build. Fixture and local-UAT
flags default off, release builds assert that they are off, and a backend production profile rejects
development identity headers regardless of frontend configuration.

## Shared Runtime Contract

- Every menu read has loading, empty, recoverable error, forbidden, and stale-data behavior.
- Every command has pending, confirmed success, validation failure, authorization failure, conflict,
  network failure, and retry or reconciliation behavior appropriate to the operation.
- Workspace switch, session refresh, role change, and authorization-version mismatch refresh visible
  capabilities and data without relying on role display names.
- Pagination is authoritative for catalog, workbench, runs, audit, clients, and other unbounded lists.
- Deep links use durable server identifiers and resolve after reload, logout/login, and cross-page navigation.
- Secrets, tokens, DSNs, provider keys, and machine-client credentials never enter logs, traces, audit
  payloads, fixture snapshots, URLs, or persistent browser storage.

## Desktop And Interaction Boundary

Semlia supports desktop only for this release. Acceptance targets are 1440x900 and 1024x768. Each
target must keep navigation, contextual rail, page toolbar, tables, dialogs, and primary commands
reachable without page-level horizontal scrolling or overlapping content. Dense tables may use an
explicit internal scroll region.

All navigation, tabs, filters, rows, menus, dialogs, and commands are keyboard reachable with a
visible focus indicator. Dialogs trap focus, close with Escape when safe, and restore focus to their
invoker. Async changes are announced without moving focus unexpectedly. Reduced-motion preference
disables nonessential transition motion.

## External Input Boundary

| External input | Required for | Boundary |
| --- | --- | --- |
| Hosted OIDC issuer, client ID/secret, redirect URLs, and test identities | Hosted authentication and role acceptance | Local standards-compliant issuer can prove implementation; hosted release evidence requires owner-supplied provider values. Secrets stay outside Git. |
| Public hostname, TLS termination, cookie origin, and encryption/session key material | Hosted security rehearsal | Deployment owner supplies environment-specific values. The application validates secure-cookie, origin, redirect, and key requirements at startup. |
| Read-only representative PostgreSQL source and expected catalog objects | Live source/discovery acceptance | The source owner supplies credentials and expected schemas. Semlia does not collect customer fact rows and never emits the DSN. |
| File samples, size/type policy, and expected imported schema | File-ingestion acceptance | Product owner supplies representative valid, malformed, oversized, and unsupported samples without production-sensitive data. |
| Supported model-provider credentials and model availability | LLM and embedding acceptance | Provider secrets remain external. Providers without a tested runtime adapter are not selectable in Beta. |
| Query execution target, read-only credentials, sample dataset, and expected results | Semantic Ask execution acceptance | The execution adapter is allowlisted and read-only. Without this input, the UI can prove resolution and explicit non-execution only, not successful answers. |
| OpenTelemetry endpoint and authentication | Runtime telemetry probe acceptance | The deployment owner supplies endpoint and secret; probe failures remain failures and do not fall back to a simulated pass. |
| Distribution-channel release artifacts and endpoint inventory | REST, MCP, CLI, SDK, or Webhook visibility | A channel appears only when its executable artifact, compatibility contract, authentication flow, and smoke test exist. |

External credentials and infrastructure can block hosted evidence, but they do not justify fixture
fallback, simulated success, or unverified capability claims in the shipped interface.

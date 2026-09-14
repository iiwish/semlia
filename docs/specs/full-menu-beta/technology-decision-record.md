# Full-Menu Beta Technology Decision Record

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.2.0 |
| Status | Confirmed |
| Requirements | `docs/specs/full-menu-beta/spec.md` 0.2.0 Confirmed |
| Data model | `docs/specs/full-menu-beta/data-model.md` 0.2.0 Confirmed |
| Last updated | 2026-09-04 |
| Approval | Founder approved Full-Menu Beta and continuous FMB-T001 through FMB-T007 execution on 2026-09-04 |

## Decision Summary

### FMB-TDR-001 One production authorization authority

The existing Semlia `Authorizer` contract and PostgreSQL role/grant model remain the production
authority. Beta exposes role catalog, scoped grant mutation and effective-access inspection through
that application boundary. The Access Control Web surface consumes those APIs and may keep only
unsaved form state locally. Casbin is not introduced as a second policy source or a Beta dependency.

This decision treats access-control prototype evidence as interaction evidence only. Server command
tests, persisted API tests and cross-workspace denial evidence are required for production claims.

### FMB-TDR-002 Asset detail is an authoritative composition

The Catalog asset/revision API remains the identity and definition base. Release, governance,
PhysicalBinding, JoinContract, validation, evidence and consumer-impact sections load exact records
from their owning services. The Web may compose calls, while a server read model may aggregate them
when consistency or performance requires it; neither path may fill unknown fields with fixtures or
plausible defaults.

Every section carries an authority state: `available`, `not_configured`, `not_released`, `forbidden`
or `failed`. The six-tab UX contract is preserved without treating layout evidence as data evidence.

### FMB-TDR-003 Runtime is a projection, not a replacement event store

`runtime_runs` and append-only `runtime_run_events` provide a normalized operations read model over
existing jobs and domain runs. Domain-specific tables retain payload authority. Projection updates
are idempotent and trace-linked, so the UI can list and inspect mixed run types without interpreting
job payload JSON.

Audit remains a separate attributable fact stream, usage remains a metering stream, and OTel remains
technical telemetry. Product metrics are not reconstructed from audit or traces.

### FMB-TDR-004 Workbench uses durable explainable attention items

Beta persists a narrow `attention_items` projection because stable identity, assignment, dismissal
and restart behavior cannot be supplied by a browser-side merge. Items are derived from real
governance and runtime conditions and keep an owning target/evidence link.

The model excludes M4 autonomous scoring and actions. Priority and risk are explainable enums from
versioned rules or explicit operators; no automatic publish, rollback or remediation is added.

### FMB-TDR-005 Shared content-addressed artifact storage

HTTP upload and worker processing use one `ArtifactStore` port. Stored objects are addressed by a
workspace-scoped content digest and immutable storage key. The server never passes an arbitrary host
path to a worker. Local deployments may use a shared mounted volume; hosted deployments use an
S3-compatible object-store adapter with the same contract. The hosted adapter accepts only
server-generated keys and uses bucket/region/endpoint configuration plus the standard AWS
credential chain; clients never supply a storage key or host path.

Versioned SQL and dbt bundles reuse the current parsers and generic discovery service. CSV, XLSX and
Markdown use isolated typed adapters with strict size, row, sheet and cell limits. Parsing does not
execute spreadsheet formulas, Markdown code, macros or imported SQL.

The Beta upload ceiling is 50 MiB raw content. Archive expansion is limited to 200 MiB, 4,096
entries, 50 MiB per entry and a 100:1 expansion ratio. CSV/XLSX processing is limited to 64 sheets,
250,000 rows, 512 columns, 5,000,000 cells and 1 MiB per cell; Markdown is limited to 10 MiB and a
bounded AST. Any XLSX formula, macro, encryption, OLE/ActiveX object, external relationship or
unsafe ZIP entry rejects the workbook.

### FMB-TDR-006 Database-backed scheduling with occurrence idempotency

A scheduler computes occurrences from a validated expression and IANA timezone, claims due rows by
database lease, and inserts one unique occurrence for `(schedule, scheduled_for)`. The occurrence
enqueues the same application command used by a manual run. Multiple schedulers may race safely;
only one job becomes authoritative.

Misfire policy is `skip` or one coalesced `run_once`. Unbounded catch-up is rejected. Schedule edits
use optimistic versioning and never rewrite completed occurrences.

Spring-forward nonexistent wall minutes are recorded as skipped. A repeated fall-back wall minute
runs once at the earlier offset and is deduplicated by a stored wall-clock key. Overlap with an
active run produces terminal `OVERLAP_ACTIVE_RUN`. Scheduled execution is an explicit delegation
created by an authorized operator: later creator role changes do not silently disable it, while a
disabled/deleted source or unavailable pinned credential produces a stable skipped occurrence.

### FMB-TDR-007 Staged pgvector index with lexical fallback

Embedding storage is behind a `VectorIndexRepository`. The Beta PostgreSQL adapter uses pgvector,
pins the configured model and dimension in each index version, writes to a staging version, validates
count and dimension, and atomically swaps the active pointer. Provider batches are checkpointed and
deduplicated by content/model/dimension digest.

Lexical retrieval remains available and is explicitly recorded as fallback. Hosted readiness checks
the vector extension and active-index compatibility. If pgvector is unavailable, rebuild is
`not_configured`; the UI cannot simulate progress or activation.

### FMB-TDR-008 Opaque hashed machine credentials

A machine credential belongs to a persisted consumer and server-owned workspace principal. It uses
a public credential ID plus at least 256 random secret bits. Semlia returns the plaintext once and
stores a versioned verifier digest, prefix, actions, scope, expiry and revocation state. Rotation
creates a linked new row and atomically revokes the old credential. Beta uses zero grace and
preserves the original expiry; nonzero grace is not a supported request option.

Credential actions only narrow current principal authorization. Consumer suspension, principal
suspension, grant removal, expiry and revocation deny the next request. Token values never enter
audit, logs, traces, jobs or API list responses.

### FMB-TDR-009 Separate cookie and bearer authentication modes

Browser sessions retain Secure/HttpOnly cookies and Origin+CSRF enforcement. REST bearer middleware
validates the machine credential and places the resolved principal, consumer, workspace and channel
in request context. A request containing both a Semlia session cookie and bearer credential is
rejected as ambiguous. Workspace identity is never accepted from a bearer claim or client header
without validating the stored credential relationship.

Rate limits are keyed by credential and workspace. Authentication returns 401 for invalid credential
material; authorization returns the existing safe 403/404 policy without leaking other workspaces.

### FMB-TDR-010 REST application services own channel semantics

REST OpenAPI is the canonical public contract. MCP, CLI and TypeScript SDK adapters call the same
application services and use the same typed SemanticQuery, release binding, resolver, refusal and
execution contracts. Transport adapters do not implement independent matching or policy logic.

MCP uses the official protocol SDK and Streamable HTTP for hosted use, plus stdio for the local CLI
where appropriate. The `semlia` binary gains explicit `mcp` and semantic query commands. The
TypeScript package exports a supported high-level client over generated transport. Python examples
and package claims are excluded from 0.2.0.

### FMB-TDR-011 Versioned external webhook envelopes

Webhook events are created from committed domain/outbox events through a registered publisher. An
external envelope has its own schema version and allowlisted payload; internal outbox JSON is never
sent directly. Each delivery signs `timestamp + event id + body digest/body` with HMAC and includes
an idempotency key.

Destination validation resolves DNS and rejects loopback, link-local, private and deployment-denied
ranges by default. Production delivery requires HTTPS on port 443, pins a validated IP for the
connection, disables environment proxies and refuses redirects. Delivery uses bounded timeouts,
retry with jitter, terminal dead-letter state and manual replay under `runtime.manage`.

Webhook signing rotation uses zero grace. Editing or rotating a subscription cancels outstanding
deliveries from older subscription versions; they are never silently retargeted or signed with
a different configuration. Historical encrypted key versions remain server-only audit material.

### FMB-TDR-012 Typed-plan PostgreSQL execution only

The Beta execution adapter accepts a persisted resolved-plan ID and digest, never SQL text. A compiler
loads the exact release-pinned physical snapshot, validates ownership and freshness, quotes trusted
identifiers, binds user values as parameters, and emits exactly one aggregate `SELECT`. Unsupported
expressions or joins are refused rather than approximated.

The adapter uses a dedicated execution credential in a forced read-only transaction with statement
timeout, cancellation, row and byte caps. It records source/release/plan/adapter provenance, counts
and result digest but not SQL values or returned rows. `semantic.execute` is checked before compiling
or connecting.

### FMB-TDR-013 Deployment-owned operational configuration

Worker process concurrency, OTel endpoints, secret-store roots, OIDC issuer/client configuration and
outbound-network policy remain deployment-owned configuration. Workspace runtime settings control
only bounded future-run defaults. This prevents a workspace administrator from redirecting telemetry
or changing infrastructure trust boundaries through the product UI.

Provider configuration is considered usable only when the server or worker can resolve and test its
secret reference. Storing a credential digest or environment-variable name alone is not a successful
connection test.

### FMB-TDR-014 Hosted acceptance is the release authority

The production artifact is built without local-UAT identities. Final readiness requires TLS, real
OIDC, live Chat and Embedding providers, the worker, pgvector compatibility, shared artifact storage,
backup/restore and one dedicated read-only PostgreSQL execution source. Local deterministic tests
remain regression evidence but cannot replace the hosted human and machine-client journeys.

## Alternatives Considered

| Alternative | Decision |
| --- | --- |
| Treat Access Control fixtures as a persisted UI | Rejected; server authorization and administration state must share one authority |
| Keep synthetic release/binding/validation values in asset detail | Rejected; unknown data must stay explicit |
| Build Workbench by merging APIs only in the browser | Rejected; stable identity, assignment and authorization filtering require a server projection |
| Reuse `jobs.payload` as Runtime API | Rejected; payload is an internal command envelope and not a versioned product read model |
| Let users edit OTel endpoint and worker concurrency per workspace | Rejected; these are deployment trust and process controls |
| Store uploads on the Web server filesystem | Rejected; the worker and hosted replicas need shared durable ownership |
| Write vectors into the active index during rebuild | Rejected; partial indexes would change live behavior |
| Store retrievable API tokens | Rejected; verifier-only storage limits credential disclosure |
| Implement separate resolver logic in MCP or CLI | Rejected; channel drift would violate semantic parity |
| Expose internal outbox payloads as webhooks | Rejected; external contracts need versioning, filtering and redaction |
| Accept arbitrary SQL for query execution | Rejected; the Beta executes only governed typed plans |
| Add Python SDK in 0.2.0 | Deferred; the TypeScript SDK is the supported SDK surface |
| Add M4 autonomy, plugin marketplace or HA/multi-region | Deferred beyond Full-Menu Beta |

## Consequences

- OpenAPI and shared domain/application contracts land before Web, MCP, CLI and SDK adapters.
- New migrations add operational projections, attention items, artifacts/schedules, Embedding index,
  machine credentials, webhooks and execution metadata without resetting Alpha data.
- HTTP composition gains bearer authentication and services for authorization administration,
  operations, artifacts, schedules, Embedding, webhooks and execution.
- Worker composition gains artifact discovery, scheduler, Embedding and webhook handlers with
  explicit idempotency and progress projection.
- The release image requires pgvector compatibility and shared artifact storage in addition to the
  existing PostgreSQL, Git and OIDC dependencies.
- UI prototype notices are removed only after the corresponding real contract passes integration and
  browser tests.

## Approval Effect

Status `Confirmed` fixes FMB-TDR-001 through FMB-TDR-014 as the 0.2.0 technical baseline and
authorizes continuous FMB-T001 through FMB-T007 execution. Implementations may refine names and
migration layout without reopening approval when they preserve these security, authority,
idempotency and evidence contracts. Any relaxation of query isolation, secret handling, workspace
boundaries or hosted acceptance requires an explicit decision update.

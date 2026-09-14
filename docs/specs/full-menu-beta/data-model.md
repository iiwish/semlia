# Full-Menu Beta Data Model

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.2.0 |
| Status | Confirmed |
| Requirements | `docs/specs/full-menu-beta/spec.md` 0.2.0 Confirmed |
| Last updated | 2026-09-04 |

## Modeling Rules

- PostgreSQL remains authoritative for identity, authorization, operational state, source metadata,
  credentials, schedules, index selection and delivery state.
- Git and immutable release manifests remain authoritative for released semantic content.
- All workspace-owned rows use `(workspace_id, id)` identity and workspace-qualified foreign keys.
- Public IDs use the repository TypeID/UUIDv7 pattern. Secrets never appear in public IDs or JSON
  payload columns.
- Append-only events are not used as mutable work queues. Durable projections keep explicit links to
  their owning event or domain object.
- Runs retain metadata, digests and bounded diagnostics; warehouse fact rows and recoverable secrets
  are never retained.

## Existing Aggregates Reused

| Aggregate | Beta use |
| --- | --- |
| `user_accounts`, `external_identities`, `workspace_memberships`, sessions and invitations | Human authentication and workspace principal resolution |
| `principals`, roles, permissions, role bindings and authorization decisions | One authorization source for human and machine requests |
| `semantic_assets`, asset revisions, relations and evidence | Catalog and asset-detail core |
| Governance proposals, validations, reviews, release objects and releases | Workbench targets and immutable semantic authority |
| `source_connections`, source credentials, source revisions, discovery runs and candidates | Live PostgreSQL and imported-source lifecycle |
| `jobs`, `outbox_events`, `audit_events`, `usage_events`, Agent runs and semantic resolution events | Operational execution and attributable projections |
| `consumers`, `consumer_bindings`, semantic queries and plans | Headless identity, release binding and resolution |
| model providers and model settings | Chat and Embedding provider selection |

## Authorization Administration

The existing role and grant model remains authoritative. Beta adds APIs and read projections rather
than a second authorization store.

### Machine principal relationship

Each machine-enabled `consumer` references one workspace-scoped `agent` principal through the
immutable `consumer_machine_principals` relationship. Explicit principal creation grants no roles;
the existing role-binding administration remains the sole grant path. A credential never creates
an implicit principal during authentication.

Credential actions are an upper bound and are intersected with current principal grants:

`effective actions = credential actions ∩ current principal authorization`

Changing a role or suspending the principal therefore affects the next bearer request without
reissuing credentials.

## Operational Projection

### `runtime_runs`

| Column | Contract |
| --- | --- |
| `workspace_id`, `id` | Workspace-qualified TypeID identity |
| `kind` | `discovery`, `validation`, `agent`, `semantic_resolution`, `embedding_rebuild`, `webhook_delivery`, `query_execution`, `audit_export` |
| `source_type`, `source_id` | Owning aggregate discriminator and immutable public ID |
| `job_id` | Nullable link to the leased job |
| `trace_id`, `idempotency_key` | Correlation and replay control |
| `state` | `queued`, `running`, `succeeded`, `degraded`, `failed`, `cancelled`, `dead_letter` |
| `phase`, `progress_current`, `progress_total` | Optional bounded progress; no invented percentage |
| `attempt`, `max_attempts` | Retry projection |
| `requested_by_principal_id` | Attributable initiator |
| `started_at`, `finished_at`, `created_at`, `updated_at` | Lifecycle timestamps |
| `error_code`, `error_summary` | Redacted, stable diagnostics |
| `version` | Optimistic update version |

`UNIQUE (workspace_id, kind, source_type, source_id)` prevents duplicate projections for one owning
run. Domain-specific tables remain authoritative for their payload; `runtime_runs` is the shared
operational index.

### `runtime_run_events`

Append-only phase, attempt, progress and bounded diagnostic events keyed by `runtime_run_id` and
monotonic `sequence`. Payload schema is versioned and redaction-checked.

### `runtime_settings`

One versioned row per workspace for future-run defaults: retry ceiling, statement timeout, webhook
timeout, query row/byte limits and run metadata retention. Worker concurrency, OTel destination,
encryption roots and provider secrets remain deployment configuration and are not stored here.

### Audit read/export

`audit_events` remains immutable. New indexes support workspace, time, actor, event type, object and
trace filters. Export is represented by a runtime run and a short-lived artifact reference; export
creation emits a separate audit event. Application-level audit retention does not delete rows in
0.2.0.

## Workbench Projection

### `attention_items`

| Column | Contract |
| --- | --- |
| `workspace_id`, `id` | Stable item identity |
| `kind` | `review`, `validation`, `source`, `runtime`, `compatibility` |
| `dedupe_key` | Stable condition key unique within a workspace |
| `state` | `open`, `in_progress`, `resolved`, `dismissed` |
| `priority`, `risk` | Explainable enum values, never autonomous numeric scoring |
| `target_type`, `target_id`, `target_route` | Typed deep-link target |
| `assignee_principal_id`, `audience_role_id` | Optional direct or role audience |
| `title`, `summary`, `reason_code` | Server-derived bounded presentation facts |
| `evidence_ref`, `trace_id` | Owning evidence and trace |
| `opened_at`, `due_at`, `resolved_at`, `updated_at` | Lifecycle timestamps |
| `version` | Optimistic command version |

Projection handlers upsert by `dedupe_key` from real governance and runtime transitions. Manual
assignment and dismissal update only operator-owned fields. There is no autonomous remediation,
auto-publish or M4 scoring model.

## Artifact Sources And Scheduling

### Source type

`source_connections` gains a canonical kind:

- `postgresql`: live catalog discovery with encrypted connection credential.
- `sql_bundle`: versioned `.sql` artifacts.
- `dbt_bundle`: validated `manifest.json` plus optional `catalog.json`.
- `file`: bounded `.csv`, `.xlsx` or `.md` content.

PostgreSQL-specific host, port, database and SSL fields remain in a typed configuration object or
companion table and are invalid for artifact-only kinds. The API uses a discriminated union rather
than one bag of optional fields.

### `artifact_objects`

| Column | Contract |
| --- | --- |
| `workspace_id`, `content_sha256` | Workspace-scoped content identity and deduplication key |
| `byte_size`, `media_type`, `storage_key` | Immutable safe metadata and opaque store identity |
| `created_at`, `expires_at` | Storage lifecycle; expiry never erases source evidence metadata |

The object row is immutable. Multiple sources may reference one object digest without sharing it
across workspaces.

### `source_artifacts`

| Column | Contract |
| --- | --- |
| `workspace_id`, `id`, `source_connection_id` | Ownership |
| `artifact_kind`, `schema_version` | Parser selection and contract version |
| `content_sha256` | Nullable object reference for accepted content; rejected input keeps safe failure metadata only |
| `byte_size`, `media_type` | Immutable identity and limits |
| `original_name` | Sanitized display metadata |
| `status` | `uploaded`, `validated`, `rejected`, `consumed` |
| `uploaded_by_principal_id`, `created_at` | Attribution |

The artifact references a workspace-local immutable object; the store key is never exposed to the
client. Raw file retention follows deployment policy; source revisions and evidence retain digests
and safe object identity even after raw artifact expiry.

### `discovery_run_artifacts`

Each artifact-backed discovery run pins the exact finalized `source_artifacts` set and content
digests at enqueue time. The worker loads only those immutable references; it never resolves a
source's mutable current artifact set after leasing the job. This preserves retry and enqueue-to-run
determinism even when a source is updated concurrently.

### `source_schedules`

| Column | Contract |
| --- | --- |
| `workspace_id`, `id`, `source_connection_id` | Ownership |
| `expression`, `timezone` | Validated schedule and IANA timezone |
| `misfire_policy` | `skip` or `run_once`; no unbounded catch-up |
| `enabled`, `next_run_at`, `last_run_at` | Scheduler state |
| `credential_version` | Optional pinned source credential policy |
| `created_by_principal_id`, `version`, timestamps | Attribution and optimistic concurrency |

### `source_schedule_occurrences`

Each computed occurrence has `(schedule_id, scheduled_for)` uniqueness, a normalized
`wall_clock_key`, enqueue state, stable reason, `job_id` and runtime run link. A second uniqueness
boundary on `(schedule_id, wall_clock_key)` makes fall-back wall minutes run once. This is the
database idempotency boundary across multiple scheduler processes.

## Embedding Index

### `embedding_index_versions`

| Column | Contract |
| --- | --- |
| `workspace_id`, `id`, `release_id` | Index identity and immutable published corpus |
| `config`, `config_digest` | Pinned provider/setting IDs, endpoint, model, dimension, credential environment/revision and chunking version |
| `corpus_digest`, `chunk_count`, `vector_count` | Completeness proof |
| `state`, `error_code` | `building`, `active`, `failed`, `cancelled`, `retired` and a safe reason code |
| `job_id`, `runtime_run_id`, `created_by_principal_id` | Operation and attribution links |
| `idempotency_key` | Workspace-unique rebuild intent |
| `created_at`, `updated_at` | Lifecycle timestamps; Operations events retain transition times |

Only one active version exists per workspace. Activation locks the workspace index pointer, verifies
dimension and expected counts, marks the new version active and the old version retired in one
transaction.
Validation is performed inside the activation transaction, not through separately persisted
`validating` or `ready` states. A partial unique index permits at most one building generation and
one active generation per workspace. Workspace locking uses `FOR NO KEY UPDATE`, compatible with
runtime-event foreign-key locks while serializing authorization-version changes.
The rebuild request is an activation intent: successful validation performs the switch automatically.
A cancelled version is terminal and never becomes the active pointer.

### `knowledge_chunks`

Canonical retrieval text comes from the captured release's exact asset revisions. Each generation
contains at most 5,000 `released-summary/v1` chunks, one address/name/title/description summary per
asset, with each safe-text value bounded to 8,192 bytes. Rows contain workspace/generation identity,
ordinal, asset ID, revision ID, address, safe text and content digest. The primary key is
`(index_version_id, ordinal)`. Raw source bytes and unpublished revisions are excluded.

### `embedding_items`

One vector per `(index_version_id, ordinal)` references the exact knowledge chunk and stores its
dimension and vector value. Chunk identity and pinned configuration are joined from the owning
chunk/generation. The primary key prevents duplicate checkpoint writes. The Beta adapter uses
PostgreSQL pgvector with workspace/generation/release-filtered exact cosine search. Reuse requires
the same workspace, full configuration digest and chunk digest. Query results are filtered through
current per-asset authorization and a final workspace authorization-version check.

Migration 19 creates vector storage when the extension is already enabled; metadata-only servers
remain usable with lexical retrieval. Installing pgvector for the existing PostgreSQL major and
running `deploy/local/enable-pgvector.sql` is an explicit administrator operation. Requests never
perform extension or schema DDL. Downgrade refuses existing generation history.

The active search pointer is separate from model settings so changing a configured default model
does not expose a partial index.

## Machine Credentials

### `client_credentials`

| Column | Contract |
| --- | --- |
| `workspace_id`, `id`, `consumer_id`, `principal_id`, `binding_id` | Bound machine identity and release-binding context |
| `name`, `token_prefix` | Non-secret administration fields |
| `verifier_digest`, `verifier_version` | One-way verifier; never plaintext token |
| `allowed_actions`, `scope_type`, `scope_id` | Credential upper bound |
| `issued_by_principal_id`, `issued_at`, `expires_at` | Attribution and lifetime |
| `revoked_at`, `revoked_by_principal_id` | Immediate revocation |
| `rotated_from_id` | Explicit rotation chain; atomic old-token revocation with zero grace |
| `last_used_at` | Coarsened operational timestamp |

The token is `public credential id/prefix + at least 256 bits of random secret`. The secret is
returned only from the create/rotate command response and is never recoverable.

Bearer authentication loads by public ID, performs constant-time verifier comparison, then checks
workspace, credential, consumer, principal, binding and expiry state before authorization.

## Webhooks

### `webhook_subscriptions`

Workspace-owned endpoint, enabled state, versioned event filters, signing-secret reference,
destination-policy result, created/updated attribution and optimistic version.

### `webhook_signing_secrets`

Versioned authenticated-encryption envelope bound to workspace, subscription and signing version.
List responses expose only version and last four display characters. Create/rotate responses return
the new signing secret once. Rotation uses zero grace and cancels outstanding deliveries from older
subscription versions. Historical encrypted key rows are not exposed through public read APIs.

### `webhook_deliveries`

Immutable event ID, subscription ID/version, event type/version, payload digest, attempt state,
response status class, bounded redacted error, next attempt, runtime run and outbox reference.
Delivery bodies are regenerated from a versioned external envelope, not exposed internal outbox
payloads. `(subscription_id, event_id)` is unique.

## Executable Semantic Plans

The released physical projection required for execution contains:

- immutable release, asset and governance-object versions;
- source connection and source revision;
- adapter kind and compiler contract version;
- physical dataset locator and exact qualified relation name;
- physical field locators and data types;
- governed measure expression, aggregation and grain;
- explicit JoinContract field pairs, join type and cardinality;
- typed filters, grouping, ordering and bounded limit.

All fields enter the canonical plan digest. Free-form client SQL is not part of the request.

### `query_execution_runs`

| Column | Contract |
| --- | --- |
| `workspace_id`, `id`, `runtime_run_id` | Run identity |
| `semantic_query_id`, `resolved_plan_id`, `plan_digest` | Immutable semantic input |
| `consumer_id`, `credential_id`, `principal_id`, `channel` | Attribution |
| `source_connection_id`, `source_revision_id`, `adapter_version` | Physical provenance |
| `state`, `refusal_code`, `error_code` | Outcome |
| `row_count`, `byte_count`, `result_digest` | Bounded result metadata |
| `started_at`, `finished_at` | Timing |

Compiled SQL text, bound values and returned fact rows are not retained. A bounded structural query
fingerprint may be retained only if it cannot reconstruct sensitive values.

## API Read Models

- `AuthorizationCatalog`, `RoleDetail`, `ScopedGrant`, `EffectiveAccessDecision`.
- `AuthoritativeAssetDetail` with per-section authority, revision and availability.
- `WorkbenchPage` and `AttentionItemDetail`.
- `AuditEventPage`, `AuditExport`, `RuntimeRunPage`, `RuntimeRunDetail`, `RuntimeSettings`.
- `Source`, `SourceArtifact`, `SourceSchedule`, `DiscoveryRun`.
- `EmbeddingIndexVersion`, `EmbeddingRebuildRun` and search-fallback disclosure.
- `ClientCredentialSummary`, one-time `IssuedCredential`, `WebhookSubscription`, `WebhookDelivery`.
- `ResolvedSemanticPlan`, `QueryExecutionRequest` by plan ID/digest, and bounded
  `QueryExecutionResult`.

All list contracts use stable cursor pagination ordered by timestamp plus ID. Every response is
workspace authorized before repository data is projected.

## Retention And Redaction

- Audit records follow deployment retention and remain immutable to application users.
- Runtime events and export artifacts have explicit bounded retention; owning domain records remain.
- Raw uploaded artifacts follow configured retention, while immutable digest/evidence metadata stays.
- Inactive Embedding versions may be collected only after no active pointer or run references them.
- Webhook response bodies, provider bodies, query rows and secrets are not retained.
- Redaction tests cover DSNs, passwords, bearer tokens, cookies, signing secrets, model keys, SQL
  values, raw prompts and spreadsheet cell data.

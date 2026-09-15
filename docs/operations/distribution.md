# Machine Distribution

## Identity And Credentials

An administrator uses **System Settings / Interfaces and Integrations** to create an
accountable machine identity. Creation grants no authority. A separate role assignment
grants its current workspace capabilities; `consumer_developer` supports released
semantic consumption. Reviewer and publisher separation of duties remains enforced.

Register a consumer and its current or pinned release binding. The first credential
persists the consumer's immutable machine-principal association. All later credentials
for that consumer must use the same principal. Credential actions and workspace,
asset or release scope are upper bounds on the principal's current grants.

Credentials contain 256 random bits and are returned once. The database stores a
SHA-256 verifier, public credential identifier and metadata, never the token. Lists,
audits and errors exclude tokens and verifiers. Store the returned token in the calling
application's secret manager, not browser storage or source control.

Rotation is atomic and immediate, with zero grace. It preserves the scope, actions
and expiry and records `rotatedFromId`. The old token is invalid on the next request.
Nonempty unsupported rotation options, including grace requests, are rejected.
Revocation is permanent. Suspension of the principal, consumer or binding and current
grant changes are checked on every request, including existing MCP sessions.

Bearer and session-cookie authentication cannot be combined. Machine workspace,
principal, consumer and binding are derived from the verifier record. Caller headers
cannot override them. Browser sessions retain their existing CSRF and Origin checks.
Known query/plan IDs and idempotency replay recheck current scope and asset grants.
Historical structural refusals without asset provenance fail closed.

The process-local limiter permits 120 requests per credential per minute and 1,200
per workspace per minute. Its storage expires at minute boundaries and is bounded.
HTTP 429 includes `Retry-After: 60`. Multi-replica aggregate limits require an upstream
gateway; the in-process counters are not a distributed quota.

## REST, MCP, CLI And SDK

All channels use the canonical released-semantic resolver. No tool exposes draft
assets, arbitrary SQL or query execution. Definitions, plan digests, refusal codes and
known-ID inspection share the same public projection. Request IDs, timestamps and
recorded channel naturally differ between independent requests.

REST routes below are relative to `/api/v1/workspaces/{workspaceId}`:

- `POST /semantic-queries:resolve` and `POST /semantic-describe` accept a `query`,
  `idempotencyKey` and channel. The server constrains channel provenance.
- `GET /semantic-search?q=...` searches the credential-bound immutable release.
- `GET /semantic-queries/{queryId}` inspects a stored plan or refusal.
- `GET /resolved-semantic-plans/{planId}` inspects an authorized plan.
- `POST /mcp` provides official MCP Go SDK stateless Streamable HTTP.

MCP exposes `semantic_describe`, `semantic_search`, `semantic_resolve`, `semantic_plan`
and `semantic_query`, plus `semlia://contract`. Every HTTP request authenticates again.
The CLI's `semlia mcp` command bridges stdio to this same authenticated endpoint; it
does not host a separate resolver or cache permission decisions.

Configure `SEMLIA_API_URL`, `SEMLIA_WORKSPACE_ID` and `SEMLIA_API_TOKEN` in the calling
process environment. Use HTTPS. The CLI permits loopback HTTP only for local testing.
Tokens are not accepted as positional command-line arguments.

```sh
semlia semantic describe '<semantic-query-json>'
semlia semantic search revenue
semlia semantic resolve '<semantic-query-json>'
semlia semantic plan rsp_...
semlia semantic query smq_...
semlia mcp
```

`SEMLIA_IDEMPOTENCY_KEY` optionally fixes CLI resolve/describe idempotency.
The TypeScript package exports `createSemanticClient({ baseUrl, workspaceId,
bearerToken })` with `describe`, `search`, `resolve`, `plan` and `query` methods.
It forces cookie omission and rejects mixed bearer/browser options. SDK publication to
an external registry and hosted third-party MCP acceptance are separate delivery steps.

## Webhooks

Configure a high-entropy `SEMLIA_SECRET_KEY` of at least 32 characters consistently on
the server and worker. Webhook signing uses a dedicated HKDF-derived AES-256-GCM
encryption key. Per-subscription signing secrets are encrypted with workspace,
subscription and signing-version associated data. A lost root key makes existing
signing material unavailable; back it up through the deployment secret manager.

Subscriptions support `release.published` and `catalog.asset.changed`. The public,
versioned event contains only event identity/type, workspace, occurrence time, trace
and allowlisted asset/release IDs. Internal metadata and full outbox payloads are not
forwarded. Asset-change notifications contain identifiers, not draft definitions.

Production destinations require HTTPS port 443 and forbid userinfo, query strings and
fragments. Every resolved IPv4/IPv6 address is checked against private, loopback,
link-local and reserved ranges before connection. Dialing pins the validated IP while
TLS verifies the endpoint hostname. Environment proxies are ignored; redirects are
refused, not followed. Validation and sending use bounded timeouts. Explicit loopback
network allowances exist only in the local transport test constructor.

The receiver verifies these headers:

- `X-Semlia-Event-ID` and `Idempotency-Key`: stable event ID, matching `body.id`.
- `X-Semlia-Timestamp`: Unix seconds for this delivery attempt.
- `X-Semlia-Signature`: `v1=` plus hex HMAC-SHA256 of `timestamp + "." + body`.
- `X-Semlia-Signing-Version`: the captured signing version.

Apply a bounded timestamp tolerance, use constant-time signature comparison and
deduplicate successful processing by event ID. Retries preserve the exact event body
and identity but generate a fresh timestamp/signature. Delivery is at least once.

Each subscription has its own durable delivery record, lease and retry state, independent
of Git projection success. First fanout is frozen even when it finds no subscribers;
an outbox retry cannot retroactively subscribe a newly created endpoint.

Endpoint/filter edits, disabling and signing rotation increment the subscription version.
Outstanding older-version deliveries are cancelled before transport instead of being
retargeted or signed with another version. In-flight attempts can finish with the version
they already captured. There is no dual-sign grace window.

Failures retry with exponential delay capped at a 256-second base and 75%-125% jitter.
Eight failed attempts produce a dead letter. A human with `runtime.manage` can replay
that dead letter once, adding eight attempts without changing its body, event identity
or subscription version. Changed/disabled subscriptions and duplicate/stale replay
commands fail closed. Replay is audited. Lease-loss completion is abandoned without
stopping other worker lanes.

Integration Settings shows durable delivery summaries and links to their owning
Operations run. A successful save followed by a failed refresh is reported separately;
stale controls cannot mutate. If consumer creation succeeds but binding creation fails,
refresh exposes the unbound consumer and **Create Missing Binding** recovers that
consumer without registering it twice.

## Migration

Migration 20 adds verifier credentials, immutable consumer-machine associations,
encrypted webhook signing versions, fanout receipts and independent delivery state.
The populated migration 19 -> 20 -> 19 -> 20 test preserves existing releases.
Downgrade removes T006-owned tables but retains the additive CLI/SDK/MCP channel
vocabulary in historical semantic query/event rows, avoiding destructive history edits.
Runtime readiness requires clean migration 21.

## Read-Only Execution

Execution accepts only a persisted plan ID, its opaque server-owned digest, and an explicit idempotency key. The canonical command is `POST /api/v1/workspaces/{workspaceId}/resolved-semantic-plans/{planId}:execute`. `GET /query-executions/{runId}` returns metadata, and `POST /query-executions/{runId}:cancel` records a monotonic cancellation request. REST, `semantic_execute`/`semantic_execution`/`semantic_cancel` MCP tools, CLI `semantic execute|execution|cancel`, SDK and Ask share the execution service. Public plans exclude private physical source locators; clients never recompute the digest.

Each selected physical binding version must own an immutable publication pin. Published releases carry forward published asset/object pins, not unrelated drafts. Existing object versions inherit their exact physical projection. Legacy bindings without such a pin remain non-executable even if another binding on the same dataset has one. Conflicting physical projections within one release are refused atomically. Supported compilation is typed PostgreSQL aggregation, filters, ordering and one-to-one inner/left equijoins using the governed field pairs and the explicit `field_pairs_equal/v1` expression marker. Free SQL, arbitrary expressions, unsafe cardinalities, unpinned or unsupported types are refused.

`SEMLIA_EXECUTION_SOURCES` is a JSON array of `{workspaceId,sourceId,dsnEnv}` references. `dsnEnv` must name `SEMLIA_EXECUTION_DSN_*`; the protected environment supplies its DSN separately. Source authority (host, port and database) must match the frozen source locator exactly. Multi-host/fallback DSNs and TLS downgrade are rejected. Production requires verified TLS; `SEMLIA_EXECUTION_ALLOW_PLAINTEXT=true` is an explicit development/test-only exception, including local Compose network hostnames. Compose operators must pass each named DSN through their private environment override. Discovery credentials are never reused automatically.

The dedicated execution role must have only SELECT on the required ordinary tables and schema USAGE. It must lack elevated roles, database CREATE/TEMP, schema CREATE, table/column DML and sequence mutation privileges, including reachable role memberships. Each transaction is read-only and uses extended protocol-bound parameters. Source logging policy must disable statement and duration/sample logging, set `log_parameter_max_length_on_error=0` and `log_min_error_statement=panic`; the adapter rejects a role whose current policy does not meet this boundary. Configure these settings through the database administrator, not by granting elevated capabilities to the execution role. External database auditing/extensions, replicas and infrastructure logs require a separate deployment privacy review; application tests cannot certify those systems.

Workspace Operations policy is read for each new run and clamped by deployment hard limits. Defaults are 10 seconds, 1000 rows, 1 MiB and two concurrent runs per workspace. The adapter accepts at most 30 seconds, 10000 rows and 8 MiB. Effective limits and policy version are captured on the run. Cancellation does not release concurrency until the owner stops or its absolute deadline becomes unknown. A process-lost claim is never reclaimed for the same key.

Rows exist only in the successful request response and browser memory. Exact numeric cells use decimal strings. Metadata-only replay never returns stored rows or silently executes again; a deliberate new key is needed for a new execution. Run/audit/runtime records contain bounded provenance, counters, safe codes and result digests, not SQL, driver errors, rows or credentials. The existing semantic resolution record retains canonical user filter input by contract; this is separate from execution result retention.

Migration 21 down is permitted only without execution pins/history or executable-plan status. It restores the schema-20 capture function and status constraint. With execution history it refuses atomically; the migration runner records a dirty marker requiring operator review. Do not force a destructive downgrade. Restore a verified schema-20 backup when such rollback is required.

Release bundles are local candidates marked `unreviewed`, with migration version 21 and a source fingerprint covering Go, embedded Web and build inputs. Dirty HEAD is not an exact source identity. Build inputs are fingerprinted before and after compilation, and the SBOM includes compiled Go plus the lockfile Web graph. Acceptance, signing and hosted evidence remain independent gates.

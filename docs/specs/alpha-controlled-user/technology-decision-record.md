# Controlled-User Alpha Technology Decision Record

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Requirements | `docs/specs/alpha-controlled-user/spec.md` 0.1.0 Confirmed |
| Research | `docs/specs/alpha-controlled-user/research.md` 0.1.0 |
| M3 contract | `docs/specs/m3-headless-semantic-distribution/spec.md` 0.1.0 Ready_For_User_Review |
| Last updated | 2026-09-04 |
| Review gate | Founder approved technical baseline 0.1.0 and continuous T001-T007 execution on 2026-09-04 |

## Decision Summary

### ALPHA-TDR-001 Generic OIDC with Authorization Code and PKCE

Semlia supports one configured issuer using `github.com/coreos/go-oidc/v3` and
`golang.org/x/oauth2`. Each login uses cryptographically random state, nonce and PKCE S256 verifier.
The callback validates issuer, client audience, signature, expiry, state and nonce before mapping
`(issuer, subject)` to an external identity. Provider access and refresh tokens are discarded after
identity establishment unless a later confirmed requirement needs them.

OIDC configuration is process-level environment configuration for Alpha, not tenant-editable data:
issuer, client ID, client secret and exact redirect URL. Startup fails readiness in session mode when
required configuration is absent or issuer discovery fails.

### ALPHA-TDR-002 Opaque PostgreSQL sessions

The browser receives an opaque 256-bit session token in a `__Host-semlia_session` cookie in secure
environments and a separately named `semlia_session_dev` cookie in explicit local HTTP development.
PostgreSQL stores only its SHA-256 digest, explicit idle and absolute expiry, account identity and
revocation state. Cookies are HttpOnly, SameSite=Lax and path `/`; the `__Host-` cookie is always
Secure. Logout and administrative revocation invalidate the next request.

`GET /api/v1/session` returns the current account, membership-filtered workspace principals,
capabilities, authorization version and a separate CSRF token. Unsafe cookie-authenticated requests require both
the CSRF header and an allowlisted Origin. State-changing auth callbacks remain protected by their
single-use OIDC transaction instead of the application CSRF token.

### ALPHA-TDR-003 Request context is the only production identity authority

An authentication middleware validates the session and stores a typed account in request context.
Workspace routing resolves that account through an active membership to the existing workspace-
scoped human principal; application handlers and services read that context rather than accepting
an actor string directly.
In external session mode, incoming `X-Semlia-Principal` is ignored and never overrides the session.
The header remains available only when development mode explicitly enables local UAT identities.

The existing Semlia evaluator remains the Alpha authorization implementation because it already
enforces the project-owned action vocabulary, scopes, human-only duties, default deny and immutable
decision audit. The earlier Casbin adapter direction remains a replaceable evaluator option, not an
Alpha launch dependency; no second policy source is introduced during this sprint.

### ALPHA-TDR-004 Allowlisted admission with an idempotent bootstrap command

There is no public signup. An administrator invites an exact issuer-subject or verified email. The
first successful callback atomically consumes the invite, creates or activates the human principal,
binds the external identity, activates membership and grants the declared role. Later email changes
do not remap identity.

A one-time idempotent CLI bootstrap command creates the first workspace and administrator invitation
without opening an unauthenticated HTTP write endpoint. The service refuses suspension or removal
of the final active workspace administrator.

### ALPHA-TDR-005 Versioned local envelope encryption for source credentials

Alpha stores source secrets in a dedicated encrypted record. A 32-byte encryption key is derived
from the deployment root `SEMLIA_SECRET_KEY` using the Go standard library HKDF-SHA256 and a
versioned context. AES-256-GCM encrypts the secret with a fresh random nonce; workspace ID, source ID
and credential version are authenticated associated data. The envelope records algorithm and key
version, and only the worker-side credential port can decrypt it.

API responses contain a redacted connection summary and credential version. Rotating a credential
creates a new version and retires the previous one without mutating run attribution. An external KMS
is a later adapter, not a parallel Alpha implementation.

### ALPHA-TDR-006 One combined PostgreSQL source boundary

One source configuration combines non-secret PostgreSQL catalog options, one encrypted credential
version and an optional relative directory of versioned `.sql` artifacts under the server-mounted
`SEMLIA_SOURCE_ARTIFACT_ROOT`. Arbitrary host paths, remote Git credentials and browser-side database
connections are excluded.

The live collector connects with bounded dial, statement and transaction timeouts, rejects elevated
superuser/role-creation/database-creation identities, and reads catalog metadata inside a forced
read-only transaction. It emits a canonical catalog snapshot including key and Join observations.
The existing `postgresql_sql` adapter parses the allowlisted versioned SQL artifacts. A merge step
combines both outputs into one snapshot and persistence contract while keeping catalog collection
and SQL parsing distinct.

### ALPHA-TDR-007 Discovery is a leased job with atomic projection

Starting discovery creates a queued run and job in one transaction. The job contains identifiers,
never credentials. A worker leases the job, resolves the exact credential version, collects and
canonicalizes a snapshot, and commits source revision, physical graph, key and Join observations, findings, evidence,
discovery candidates, audit and outbox state atomically against the existing run.

Warnings may produce `degraded` with an imported snapshot and visible findings. Terminal errors
produce `failed`, retain diagnostics, and do not advance current physical revisions or candidate
authority. The canonical snapshot digest makes retries and overlapping runs idempotent.

### ALPHA-TDR-008 Discovery candidates are non-authoritative persisted outputs

Discovery candidates are tied to one source revision and carry a stable digest, proposed asset kind,
stable key, source evidence and structured proposal input. They are not semantic assets and cannot
be queried as released knowledge. Conversion is an idempotent application command that creates the
minimum draft semantic asset/revision when needed and opens an M2 proposal; dismissal and conversion
decisions are append-only and attributable.

### ALPHA-TDR-009 Release snapshot projection is the resolver authority

The resolver first fixes one immutable release: highest sequence for `current`, exact release for a
pinned request, or the release selected by an active binding. It then loads semantic assets through
`release_assets` and PhysicalBinding, ModelGrain, EntityKey and JoinContract through
`release_objects` at their exact recorded versions. Current mutable asset pointers or governance
object versions are never substituted.

The release snapshot repository exposes a single typed projection so direct SemanticQuery, Ask and
future MCP/CLI channels cannot implement different release semantics.

### ALPHA-TDR-010 Deterministic resolver, validators and refusal

Resolver version `semantic-resolver/v1` uses explicit IDs or stable addresses first, exact normalized
names and aliases second, and bounded PostgreSQL lexical search last. Search only produces a winner
when one authorized candidate clears calibrated acceptance and separation rules. Otherwise the
service returns `NO_MATCH` or `AMBIGUOUS_MATCH` with authorized candidates and minimum clarification.

The validator registry checks release membership, asset type, binding, time semantics, grain and
JoinContract reachability. Blockers create a refusal, not a plan. Authorization denial uses HTTP 403
and redacts unauthorized candidates. A successful immutable `ResolvedSemanticPlan` contains a
canonical digest and `executionStatus=not_configured` in Alpha.

### ALPHA-TDR-011 Model interpretation cannot override resolution

Ask uses the existing configured OpenAI-compatible provider and a versioned JSON Schema to translate
natural language into SemanticQuery. The model sees only released semantic projections needed for
interpretation, not source credentials, fact rows or unrestricted schema. The service records the
agent run, input digest, model/config revision, output digest and validation result, but not the raw
prompt or hidden reasoning.

Direct and model-produced SemanticQuery enter the same resolver. Provider errors, invalid output and
ambiguity are visible failures/refusals. For `describe`, the service can summarize released content;
for data intents it explains the validated plan and absence of execution. Fixture answers and
fabricated numerical results are deleted from the production path.

### ALPHA-TDR-012 Prototype boundaries stay explicit

Primary navigation remains complete. Authenticated Alpha pages for session/member administration,
sources/discovery/candidates, M2 governance, releases and Ask use real APIs. MCP/client setup,
automations, advanced quality, external runtime and other later surfaces keep a consistent
`Prototype` disclosure, session-only state where interaction is useful, and no persisted-success
toast. Real API failure never switches to fixtures.

## Alternatives Considered

| Alternative | Decision |
| --- | --- |
| Browser-held JWT session | Rejected because revocation, membership changes and logout would be harder to enforce immediately |
| Password authentication | Rejected by the confirmed Alpha scope |
| Trust OIDC email as permanent identity | Rejected; `(issuer, subject)` remains the stable identity |
| Add Redis for sessions | Rejected; PostgreSQL is already authoritative and Alpha scale is tiny |
| Store a plaintext DSN in `source_connections.metadata` | Rejected by S-002 and database checks |
| Treat `postgresql_sql` as a live connector | Rejected; it parses supplied artifacts and does not connect to a catalog |
| Let the LLM select tables and joins | Rejected by SSOT D-015 |
| Execute SQL to make Ask appear complete | Rejected; execution is outside Alpha and false results are worse than a clear boundary |
| Implement MCP, CLI and webhooks in the same sprint | Deferred to full M3 acceptance |

## Consequences

- Three forward migrations are expected: identity/session, source/discovery, and semantic
  distribution.
- HTTP composition gains authentication and CSRF middleware plus service options for identity,
  source and semantic distribution.
- The worker registers discovery jobs in addition to validation and outbox processing.
- OpenAPI is extended before handlers; generated Go and TypeScript artifacts stay committed.
- The Web adds a sign-in/session gate and replaces Sources and Ask timer/fixture state with real
  query state while preserving the accepted desktop information architecture.
- Full M3 remains open after Alpha 0.1.0 because MCP, CLI, webhooks, external execution and external
  consumer acceptance are not delivered.

## Approval Effect

Approval confirms ALPHA-TDR-001 through ALPHA-TDR-012, the M3 Alpha profile, implementation plan and
work graph. It authorizes creation of the first Ready execution packet and sequential execution
through the Alpha acceptance task, without claiming task or milestone acceptance before evidence and
founder review.

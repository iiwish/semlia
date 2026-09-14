# Controlled-User Alpha Technical Research

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Ready_For_User_Review |
| Date | 2026-09-04 |
| Scope | Authentication, sessions, source credentials, live PostgreSQL discovery, semantic resolution and Ask |

## Repository Findings

| Surface | Existing reusable foundation | Gap that remains |
| --- | --- | --- |
| Identity and authorization | Principals, roles, scoped bindings, stable action vocabulary, separation of duties and immutable authorization events | HTTP identity is still supplied by `X-Semlia-Principal`; no OIDC, session, membership or invitation authority exists |
| Workspace access | Workspace-scoped repositories and `workspace.read/manage` actions | Workspace listing and bootstrap are not session-membership filtered |
| Source registry | `source_connections`, immutable `source_revisions`, discovery runs, physical graph and findings | No protected source CRUD/test/run API, credential store, live catalog collector or worker route |
| SQL discovery | `postgresql_sql` parses versioned DDL and view SQL with `pg_query` | It consumes in-memory files and is not a live PostgreSQL connection adapter |
| Jobs | PostgreSQL leased jobs, retries and worker registration | Discovery is synchronous and creates its run inside snapshot persistence rather than executing an existing queued run |
| Governed release | Immutable release assets, release objects, publish and rollback-as-new-release | No release-bound query projection or resolver exists |
| Query distribution | `semantic.resolve`, `binding.read/manage`, and `semantic.execute` action identifiers already exist | No SemanticQuery, consumer, binding, plan, validation, refusal or usage persistence exists |
| Ask | A polished React screen and interaction states exist | Answers, evidence and latency are fixture/timer behavior and can display fabricated numerical results |
| Model integration | Persisted model provider/settings and OpenAI-compatible structured proposal generation | No Ask interpretation schema or resolver integration exists |

## OIDC And PKCE

The repository ADR already selects `github.com/coreos/go-oidc/v3` and `golang.org/x/oauth2` once
multi-user authentication is confirmed. The current official `go-oidc` release line is v3.20.0.
OIDC Authorization Code Flow keeps tokens at the token endpoint and requires the client to validate
the ID token and subject. `golang.org/x/oauth2` exposes `GenerateVerifier`,
`S256ChallengeOption`, and `VerifierOption`, so Semlia does not need custom PKCE construction.

Alpha uses state, nonce and a unique S256 verifier per attempt. State is stored only as a digest;
nonce is compared by digest; the verifier is short-lived and encrypted server-side. An OIDC subject
is authoritative only as the tuple `(issuer, subject)`. Email is admission evidence only when the
provider marks it verified; it never replaces an already bound subject.

## Session Boundary

An opaque random cookie is smaller and easier to revoke than a browser-held JWT for this server-side
application. PostgreSQL already sits on every protected path and the cohort is 5 to 10 users, so a
separate Redis session service adds no value. The database stores only SHA-256 token digests,
absolute and idle expiry, revocation state and the mapped principal. Unsafe requests require an
in-memory CSRF token returned by the session endpoint plus an allowlisted Origin.

The current authorization service remains the Semlia-owned decision engine for the Alpha. The
confirmed access-control TDR keeps engine adapters behind that interface; replacing a working,
audited evaluator with Casbin does not reduce an Alpha risk and is therefore not on this critical
path.

## Source Credential Boundary

The existing `credential_ref` is a reference, not secret storage. Alpha adds one versioned encrypted
credential record per source rotation. The API accepts a DSN or structured PostgreSQL credential,
normalizes non-secret connection fields, encrypts the recoverable secret using AES-256-GCM with a
key derived from `SEMLIA_SECRET_KEY`, and returns only a credential version and redacted summary.
Associated data binds ciphertext to workspace, source and credential version. Logs, traces, audit
payloads, error messages and Agent input never contain the DSN or plaintext.

An external KMS or Secrets Manager remains a later adapter. The envelope format includes algorithm
and key version so Alpha data can migrate without changing source records.

## Live PostgreSQL Discovery

The live collector and the existing SQL parser solve different halves of the problem and are
composed rather than conflated:

1. A `postgres_catalog` collector connects with bounded timeouts, forces a read-only transaction,
   reads schemas, relations, columns, primary/unique/foreign keys and view definitions, and emits a
   canonical artifact bundle.
2. A server-mounted, allowlisted artifact directory can contribute versioned `.sql` files. Secure
   path joining prevents traversal and the directory is never selected as an arbitrary host path.
3. The existing `postgresql_sql` parser consumes canonical view/DDL and versioned SQL artifacts.
4. A merge step produces one canonical discovery snapshot for the existing atomic persistence
   boundary.

Start-run creates a queued `discovery_runs` row and a job payload containing only workspace, source,
run and credential-version identifiers. The worker loads and decrypts the secret, connects, collects
metadata and commits the snapshot against that run. Warnings produce `degraded`; terminal failure
does not advance current physical revisions or candidate state.

## Semantic Resolution

The immutable release manifest already pins semantic asset revisions and the release-object table
pins PhysicalBinding, ModelGrain, EntityKey and JoinContract versions. The Alpha resolver should
therefore read a release projection instead of current mutable rows. Candidate selection is
deterministic; model output may identify terms but cannot decide an asset or JoinContract.

No external execution adapter is in the approved Alpha scope. A resolved data request is still
useful as a validated plan, but the Web must say it was not executed. The current hard-coded values,
Cube claim and fixed evidence list in `AskView` must be removed from the real path.

## Sources

- [go-oidc releases](https://github.com/coreos/go-oidc/releases)
- [Go OAuth2 package and PKCE helpers](https://pkg.go.dev/golang.org/x/oauth2)
- [OpenID Connect Core 1.0](https://openid.net/specs/openid-connect-core-1_0.html)
- [RFC 7636: Proof Key for Code Exchange](https://www.rfc-editor.org/rfc/rfc7636.html)
- `docs/adr/0002-m1-product-and-semantic-execution.md`
- `internal/application/authorization/service.go`
- `internal/application/discovery/service.go`
- `internal/adapters/discovery/postgresql/adapter.go`
- `migrations/000003_semantic_registry_foundation.up.sql`
- `migrations/000007_m2_governance_objects.up.sql`
- `web/src/KnowledgeViews.tsx`


# Controlled-User Alpha Data Model

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Source | `docs/specs/alpha-controlled-user/spec.md` 0.1.0 Confirmed |
| Last updated | 2026-09-04 |

## Storage Authority

PostgreSQL owns identity mappings, sessions, memberships, invitations, encrypted source credential
envelopes, discovery and semantic resolution records. Git continues to own released semantic
content; immutable release manifests in PostgreSQL are the query resolver index. Fact rows, full
query results, raw Ask prompts, provider tokens and plaintext source credentials are not stored.

All new public identities use UUIDv7 internally and TypeIDs on the wire. Proposed prefixes are
`usr_` user account, `ses_` session, `ivn_` invitation, `ext_` external identity, `scr_` source credential, `scd_`
semantic discovery candidate, `csm_` consumer, `cbd_` consumer binding, `smq_` SemanticQuery,
`rsp_` ResolvedSemanticPlan and `qvr_` query validation run.

## Identity And Admission

| Table | Key fields | Invariant |
| --- | --- | --- |
| `user_accounts` | display name, status, timestamps | one browser session belongs to one account; an account has no workspace authority by itself |
| `external_identities` | account, issuer, subject, verified email, status | `(issuer, subject)` is globally unique; identity remapping is explicit and audited |
| `workspace_memberships` | workspace, account, workspace principal, status, admitted_by, timestamps | only active membership grants workspace visibility; the last active administrator cannot be suspended or removed |
| `workspace_invitations` | workspace, issuer, subject or verified email, role, token digest, expiry, state | one active invitation per admission identity; token is never stored plaintext; acceptance is single-use |
| `oidc_login_attempts` | state digest, nonce digest, encrypted verifier, return path, expiry, consumed_at | single-use, short-lived and independent of browser-supplied identity |
| `sessions` | token digest, account, idle/absolute expiry, CSRF digest, revoked_at, last_seen_at | token digest is unique; expiry/revocation is checked before every protected request |

Existing workspace-scoped `principals`, roles, permissions and `role_bindings` remain the
authorization authority. Invitation acceptance creates or reuses the account and external identity,
then creates the workspace principal, membership and initial role binding in one transaction. Route
authorization resolves the session account through the active workspace membership to that
workspace principal.

## Source And Discovery

| Table | Key fields | Invariant |
| --- | --- | --- |
| `source_credentials` | source, version, key version, algorithm, nonce, ciphertext, created_by, retired_at | ciphertext is envelope-bound to workspace/source/version; API reads never return it |
| `source_connections` | existing identity plus active credential version and non-secret catalog/artifact options | source config has no plaintext secret and one active credential version |
| `discovery_runs` | existing run plus requested credential version and queued/running/degraded terminal states | terminal failure never advances the imported current projection |
| `physical_key_observations` | source revision, dataset, ordered field refs, key kind and confidence | catalog-observed primary, unique and foreign keys are immutable source facts, not released semantic contracts |
| `join_observations` | source revision, left/right datasets and fields, observation kind and confidence | observations may seed a JoinContract proposal but cannot authorize a plan |
| `semantic_candidates` | run, source revision, candidate kind/key, proposal input, evidence refs, digest | candidate output is tied to one immutable source revision and canonical digest |
| `semantic_candidate_decisions` | candidate, decision, actor, optional proposal, timestamp | conversion or dismissal is append-only and idempotent |

The existing source revision, physical dataset/field revision, code artifact, lineage, finding and
evidence tables are reused. Physical key and Join observations close the existing M1 schema gap;
they remain distinct from governed EntityKey and JoinContract. Start-run and job enqueue commit together. Successful or degraded
snapshot import, candidate creation, audit and outbox facts commit together against the pre-created
run.

## Semantic Distribution

| Table | Key fields | Invariant |
| --- | --- | --- |
| `consumers` | workspace, stable key, kind, status, owner, metadata | inactive consumer cannot resolve through a binding |
| `consumer_bindings` | consumer, environment, mode current/pinned, release, compatibility, expiry, status | pinned mode requires one release; current mode stores no release ID |
| `semantic_queries` | principal, consumer/binding, selected release, spec version, canonical request, digest, channel | selected release is fixed before candidate resolution; raw Ask text is absent |
| `resolved_semantic_plans` | query, release, resolver version, canonical plan, plan digest, execution status | immutable; one successful plan per query and resolver version |
| `query_validation_runs` | query, plan, validator/version, input digest, status, timestamps | immutable result set explains plan validity |
| `query_validation_results` | run, severity, code, message, details | blocker prevents plan creation |
| `semantic_refusals` | query, reason code, authorized candidate refs, clarification, details | immutable; query has either a plan or a refusal, never both |

Release resolution joins `release_assets` to exact `asset_revisions` and `release_objects` to exact
versions of PhysicalBinding, ModelGrain, EntityKey and JoinContract. Current mutable pointers are
not consulted after the release is selected.

## Migration Strategy

1. Identity/session migration: create admission tables and indexes, preserve existing principals
   and role bindings, and seed no external identity or reusable session.
2. Source/discovery migration: add encrypted credentials, run orchestration fields, degraded state
   and discovery candidate records without rewriting existing source revisions.
3. Distribution migration: add consumers, bindings, queries, plans, validations and refusals; no
   backfill invents historic consumption.
4. Every migration runs empty and populated up/down/up tests. A downgrade may refuse while new
   active sessions, credential envelopes or plan records exist when dropping them would violate the
   documented recovery contract.

## Retention And Redaction

- Expired login attempts and sessions are deleted after the configured audit window; revocation
  facts remain in audit events.
- Retired source credential ciphertext remains only while referenced by a run or recovery window,
  then is deleted by an audited retention job.
- Structured queries, plans and refusals are retained for the Alpha audit window; raw Ask text and
  provider responses are never persisted.
- Audit payload checks reject DSNs, passwords, tokens, cookie values, PKCE verifiers and encrypted
  credential blobs.

# ALPHA-T005 Delivery Summary

## Outcome

Semlia exposes the Alpha semantic-distribution backend through versioned REST and generated Go and
TypeScript contracts. Authorized callers can register consumers, maintain current or pinned release
bindings, submit a typed SemanticQuery, and retrieve the immutable query, plan, validation or refusal
record.

Resolution supports `describe`, `aggregate`, `breakdown` and `compare`. It accepts only semantic
selectors, filters, time ranges, ordering and limits; arbitrary SQL, physical table authority and
execution credentials are rejected. Exact TypeID or semantic address precedes normalized name and
alias matching, followed by bounded lexical matching with explicit ambiguity handling.

## Immutable Authority

Migration 15 captures every released PhysicalBinding, ModelGrain, EntityKey and JoinContract as an
immutable JSON snapshot in the publishing transaction. The resolver first selects one current,
explicit or binding-pinned release and then reads only that release's asset revisions and object
snapshots. Later changes to current asset revisions or governed objects cannot alter an existing
release or its resolved plan.

The plan records intent, measures, grouping, filters, time range, ordering, selected revisions and
object versions. Its digest excludes request and plan identities but includes the complete semantic
shape, selected release and resolver version, so the same inputs reproduce the same digest after a
process restart.

## Failure And Security Behavior

No release, inactive/expired/stale bindings, inactive or unauthorized consumers, no match,
ambiguity, missing physical bindings, missing joins, incompatible grain and invalid query semantics
produce stable refusal codes. Candidate IDs are included only after per-asset authorization.
Unauthorized and nonexistent exact references return the same non-disclosing refusal shape.

Each successful plan or refusal commits the query, plan/refusal, validation, resolution event and
audit event atomically. Attribution retains principal, optional consumer and binding, release,
channel, trace and digests without raw Ask text, source credentials, fact rows or provider reasoning.

## Product Boundary

The resolver proves semantic authority but does not execute warehouse queries. Successful plans
therefore report `executionStatus=not_configured`; no numeric result is fabricated. The real Ask
interpretation and product UI consume this same service in T006.

## Residual Risk

The 10,000-asset gate measures the deterministic in-process resolver and plan builder; the final T007
performance pass retains full-stack latency and sustained-load coverage. The production Web bundle
also remains above Vite's 500 kB advisory threshold.

# M1 Backend Consistency Analysis

## T001 Decision

Result: Clear to execute.

T001 is the smallest dependency-free backend step after M0. It corrects the current OpenAPI `ResourceId` description and pattern, which still claim an uppercase ULID, before those generated types spread into M1 persistence and handlers. It introduces only a shared boundary package and domain contracts; migration and endpoint risks stay isolated in later tasks.

## Cross-Contract Checks

| Concern | Resolution |
| --- | --- |
| PostgreSQL compatibility | UUIDv7 is generated in Go; PostgreSQL 17 stores native `uuid`. |
| Human readability | APIs use short resource prefixes; product surfaces use semantic addresses. |
| Core model coupling | TypeID library is wrapped by public `pkg/identity`; domain types and generated contracts expose Semlia wrappers, not the upstream concrete type. |
| SQL parsing | Existing indirect `pg_query_go` presence is not treated as adoption; T003 must make the dependency direct and isolate its AST. |
| dbt compatibility | Versioned published schemas define accepted inputs; unknown versions fail as unresolved, not partial success. |
| Workflow engine | Existing PostgreSQL jobs/outbox meet M1 bounded work; Temporal would add operational and semantic duplication. |
| Ontology | Closed predicates, endpoint rules, assertion state and revision membership match the calibrated prototype and avoid an unconstrained property graph. |
| Git-native | Git is a content/projection adapter; PostgreSQL owns transactions, current indexes and job state. |

## Risks

- The stable TypeID Go v1 API uses generics and a transitive UUID implementation. The wrapper must keep both out of domain contracts and pin the module version.
- Converting M0 text IDs is the highest-risk M1 database change. T002 must use a populated M0 fixture and verify every foreign key, audit subject and outbox reference.
- `pg_query_go` uses PostgreSQL parser code and may affect build time and portability. T003 must benchmark builds and keep a fixture-only fallback classification for unsupported dialects.
- Search quality cannot be inferred from latency. T007 records lexical/structural golden-set metrics separately from performance.

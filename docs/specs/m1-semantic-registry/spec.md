# M1 Semantic Registry Backend Specification

## Metadata

| Field | Value |
| --- | --- |
| Milestone | M1 Semantic Registry backend |
| Version | 1.0.0 |
| Status | Confirmed |
| Confirmed | 2026-09-02 by founder authorization to begin M1 |
| Product contract | `docs/SSOT.md` |
| Technical direction | `docs/adr/0002-m1-product-and-semantic-execution.md` |
| Identity contract | `docs/adr/0003-resource-identifiers.md` |

## Outcome

M1 delivers the first production semantic-registry vertical slice: a versioned repository or database source is discovered into physical metadata, evidence-backed semantic assets and typed relationships; clients can list and read the immutable revisions through OpenAPI; the canonical content can be projected to Git without making Git or an LLM the transactional authority.

The backend remains a modular Go monolith with PostgreSQL. It does not introduce a graph database, vector database, Temporal, Cube dependency, or custom SQL parser as a prerequisite.

## Required Capabilities

- Generate UUIDv7 identities in the application, persist them as PostgreSQL `uuid`, and expose the same values as prefix-checked TypeIDs.
- Register read-only source connections and immutable source revisions without storing credentials in domain payloads.
- Discover PostgreSQL catalog metadata, versioned DDL/View SQL and dbt manifest/catalog artifacts through adapter contracts.
- Persist physical datasets, fields, code artifacts, lineage edges and unresolved discovery findings with provenance.
- Create stable semantic assets and immutable asset revisions with semantic addresses, evidence links and explicit lifecycle state.
- Store typed semantic relations with assertion state and validate their allowed endpoint types.
- Project a bounded ontology view from accepted asset revisions and typed relations; PostgreSQL remains the query authority.
- Provide cursor-paginated catalog list/detail APIs and stable error codes through the generated OpenAPI boundary.
- Commit canonical human-reviewable content and release projections through a Git content adapter after database commit/outbox delivery.
- Record only the two M1 usage signals `catalog.asset.read` and `catalog.search.completed`; search text and customer fact rows are never retained.

## Acceptance Scenarios

| ID | Scenario | Required proof |
| --- | --- | --- |
| AC-M1-001 | Identity round trip | Every public resource prefix round-trips TypeID to UUIDv7; wrong prefixes and non-v7 UUIDs fail. |
| AC-M1-002 | Incremental discovery | A changed source snapshot updates affected projections, preserves prior source revisions and does not duplicate unchanged assets. |
| AC-M1-003 | Immutable revision | A semantic edit creates a new revision; prior content, evidence and relations remain addressable. |
| AC-M1-004 | Evidence traceability | A catalog detail resolves source revision, code/database locator, evidence digest and semantic revision without orphan references. |
| AC-M1-005 | Ontology constraint | Invalid endpoint or predicate combinations are rejected; accepted relations appear in hierarchy, semantic and impact projections. |
| AC-M1-006 | Catalog API | 10,000-table fixture supports deterministic cursor pagination and measured search without leaking database UUIDs. |
| AC-M1-007 | Git projection | Replaying a committed outbox event produces deterministic content and is idempotent. |
| AC-M1-008 | Privacy and recovery | Usage events omit raw search text; failed discovery and Git delivery can retry without partial domain state. |

## Boundaries

- PostgreSQL 17 remains the compatibility target; PostgreSQL 18 `uuidv7()` is not required.
- `go.jetify.com/typeid` is the identity implementation; business packages do not implement Base32 or UUID generation.
- `pg_query_go` is used only behind the SQL adapter boundary and must not leak parser AST types into the domain.
- dbt artifacts are validated against their published JSON schemas and mapped through versioned adapter DTOs.
- Cube remains an optional later adapter with contract tests. It cannot appear in core domain types.
- The existing PostgreSQL job/outbox runtime handles bounded discovery and projection work. Temporal is reconsidered only when durable workflows need multi-day timers, external compensation, or cross-service orchestration that this runtime cannot express safely.

## Non-Goals

- LLM-generated proposals, automated approval and release policy routing belong to M2.
- Semantic query resolution, MCP distribution and governed execution belong to M3.
- Full OWL/RDF reasoning, arbitrary predicates and unbounded graph traversal are not M1 ontology requirements.
- Production Web reconstruction is a separate M1 stream and consumes only generated API contracts.


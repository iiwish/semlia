# T004 Catalog And Revision API Design

## Status

Confirmed by the founder's T003 acceptance and continuous M1 execution authorization on
2026-09-02.

## Outcome

T004 exposes the transactional semantic registry through one generated OpenAPI contract and a
thin HTTP adapter. A client can create an asset with its first immutable revision, add later
revisions, search and paginate the catalog, inspect complete revision/evidence provenance, read
bounded typed relations and inspect discovery run results.

PostgreSQL remains the query and transaction authority. Public payloads use TypeIDs and semantic
addresses; database UUIDs, sqlc rows and PostgreSQL errors never cross the adapter boundary.

## Public Surface

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/workspaces/{workspaceId}/catalog/assets` | Deterministic cursor-paginated catalog search |
| `POST` | `/api/v1/workspaces/{workspaceId}/catalog/assets` | Create a stable asset and initial immutable revision |
| `GET` | `/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}` | Read current asset detail and provenance summary |
| `GET` | `/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions` | List immutable revisions newest first |
| `POST` | `/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions` | Append and atomically select a new current revision |
| `GET` | `/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions/{revisionId}` | Read one immutable revision with evidence |
| `GET` | `/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/relations` | Read incoming/outgoing typed relations, capped at three hops |
| `GET` | `/api/v1/workspaces/{workspaceId}/discovery-runs/{runId}` | Read discovery status, statistics and ordered findings |

Mutation bodies contain canonical semantic content and optional references to existing evidence
artifacts. T004 does not accept credentials, raw database rows, search telemetry or arbitrary SQL.

## Pagination And Search

- `limit` defaults to 50 and is capped at 200.
- The opaque URL-safe cursor records the last deterministic `(rank, updated_at, id)` boundary and
  the normalized filter digest. A cursor cannot be reused with different search or type filters.
- Empty search orders by `(updated_at DESC, id DESC)`.
- Search uses PostgreSQL full-text matching over the current revision plus address-prefix matching.
  Ranking is explainable: exact address, address prefix, full-text rank, then deterministic recency.
- Invalid cursors and filters return stable `INVALID_ARGUMENT`; exhausted pages omit `nextCursor`.

## Transaction Boundary

Asset creation inserts the stable asset, revision, evidence links, audit event and outbox event in
one pgx transaction, then selects the current revision. Appending a revision locks the asset,
allocates the next sequence in the transaction, inserts immutable content and evidence links,
updates the current pointer and records matching audit/outbox rows before commit.

The outbox payload is a versioned resource-change notification; Git delivery is implemented by
T005. T006 completes event delivery policy and usage retention, but T004 mutations already obey the
atomic domain/audit/outbox invariant.

## Failure Boundary

- Wrong TypeID prefixes, non-v7 IDs, malformed JSON, unknown enum values and invalid cursors fail
  before repository work.
- Workspace-crossing asset, revision and evidence references return `NOT_FOUND` without revealing
  another workspace's existence.
- Semantic address or revision conflicts return `CONFLICT`.
- Invalid state or evidence links return `INVARIANT_VIOLATION`.
- Internal database detail is logged only through stable operation/error fields and is never sent
  to the client.

## Acceptance Matrix

| ID | Proof |
| --- | --- |
| API-001 | Generated Go and TypeScript contracts contain every path, enum and response schema. |
| API-002 | Asset creation atomically creates the asset, initial revision, audit and outbox records. |
| API-003 | Revision append preserves old content and atomically moves the current pointer. |
| API-004 | Catalog cursor pagination is deterministic, filter-bound and duplicate-free. |
| API-005 | Search returns exact address and content matches with stable ordering. |
| API-006 | Detail and revision reads expose TypeIDs, semantic address, content and evidence provenance. |
| API-007 | Bounded relation reads enforce workspace isolation and a maximum depth of three. |
| API-008 | Discovery run reads preserve ordered findings and typed statistics. |
| API-009 | Invalid input, conflict, not-found and invariant failures use stable safe envelopes. |
| API-010 | Handler, repository and real PostgreSQL API journeys pass with contract drift checks. |

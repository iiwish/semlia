# M1 Semantic Registry Completion Audit

## Result

The M1 backend vertical slice is Accepted. Current repository state proves every required capability
and acceptance scenario in `docs/specs/m1-semantic-registry/spec.md`. PostgreSQL remains the
transactional and query authority; Git is an outbox-delivered human-reviewable projection.

## Requirement Matrix

| Requirement | Authoritative proof | Result |
| --- | --- | --- |
| UUIDv7 storage and prefix-safe TypeIDs | `pkg/identity`, migration UUID constraints, identity and contract suites | Pass |
| Immutable source discovery | Catalog, PostgreSQL SQL and dbt adapters; incremental discovery integration suite | Pass |
| Physical metadata and provenance | Normalized source/dataset/field/code/lineage tables and repository tests | Pass |
| Semantic assets, immutable revisions and evidence | Database immutability triggers plus atomic Catalog integration journey | Pass |
| Typed relations and finite ontology | Closed predicate policies in domain/PostgreSQL, ontology publication constraints and plane-projection integration tests | Pass |
| Generated OpenAPI boundary | Catalog, revision, evidence, relation and discovery routes plus contract drift suite | Pass |
| Git-native delivery | Exact-revision loader, deterministic `go-git` adapter and idempotent outbox replay tests | Pass |
| Privacy-bounded usage | Only `catalog.asset.read` and `catalog.search.completed`; raw-search rejection and retention tests | Pass |

## Acceptance Scenarios

| Scenario | Direct evidence | Result |
| --- | --- | --- |
| AC-M1-001 Identity round trip | Public prefix matrix rejects wrong prefixes and non-v7 UUIDs | Pass |
| AC-M1-002 Incremental discovery | Changed snapshots preserve prior revisions and unchanged replay is idempotent | Pass |
| AC-M1-003 Immutable revision | Append journey preserves prior content/evidence and database mutation is rejected | Pass |
| AC-M1-004 Evidence traceability | Exact-ref journey resolves source revision, locator, digest and semantic revision through HTTP | Pass |
| AC-M1-005 Ontology constraint | Invalid policies fail; taxonomy, semantic and dependency projections each return only their accepted relation plane | Pass |
| AC-M1-006 Catalog API | 10,000 assets paginate without duplicates and meet measured local budgets without UUID leakage | Pass |
| AC-M1-007 Git projection | Replay is a no-op and stale-base delivery leaves the repository unchanged | Pass |
| AC-M1-008 Privacy and recovery | Usage omits raw text; discovery, worker and Git failures retry without partial domain state | Pass |

## Current Validation

Passed on 2026-09-02 against the completion worktree:

- `go test -count=1 ./tests/integration/catalog/...`, including all three ontology relation planes.
- `make check`: source, PostgreSQL integration, generated-artifact drift, Compose recovery and
  dependency/container security scans all pass; High/Critical findings are zero.
- `SEMLIA_RUN_M1_BENCHMARK=1 go test -count=1 -v ./tests/performance/catalog/...`: 10,000 assets,
  25 samples, page p50/p95 `19.056/26.262 ms`, exact-search p50/p95 `96.234/111.388 ms`; budgets
  `150/250 ms` pass.
- `make release`: the native archive, CycloneDX SBOM and `SHA256SUMS` are generated.
- `git diff --check` passes.

The production Catalog workspace in T008 is an additional visible client of this accepted backend
and retains its own founder review state.

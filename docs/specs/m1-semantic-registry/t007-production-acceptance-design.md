# T007 Production Acceptance Design

## Release Candidate

T007 validates the integrated M1 backend as one release candidate. It does not add a new database,
workflow engine, search service or inference feature. Failures found by review are fixed in the owning
boundary and the relevant proof is rerun.

## Exact-Reference Journey

One real PostgreSQL journey uses a source artifact with an explicit immutable external revision:

1. register a credential-reference-only Catalog source;
2. discover the exact artifact into source/physical revisions;
3. create source-backed evidence and a semantic asset revision;
4. retrieve detail and provenance through the production HTTP handler;
5. deliver its committed outbox envelope into the local Git projection;
6. verify the usage read signal, immutable IDs and absence of raw UUID/credential leakage.

## 10,000-Asset Gate

The benchmark fixture creates 10,000 table-like semantic assets with immutable current revisions.
After warm-up it executes deterministic first-page reads and exact-address search 25 times. On the
local acceptance host:

- first-page p95 must be at most 150 ms;
- exact-address search p95 must be at most 250 ms;
- every search returns the golden address and cursor pages contain no duplicate identities.

The evidence records host, PostgreSQL image, sample count, p50 and p95. These are M1 regression
budgets, not universal hosted-service SLAs.

## Release Gates

- PostgreSQL 17/18 up/down/up and populated-M0 recovery.
- Contract, sqlc, Web embed and migration drift.
- Source tests, race tests, full local-stack smoke, dependency/secret/image security and release
  artifact generation.
- Spec-compliance, maintainer code-quality and QA acceptance reviews over the complete M1 diff.

## Acceptance Matrix

| ID | Scenario |
| --- | --- |
| ACC-001 | Exact source revision reaches HTTP detail, evidence, Git and usage without identity leakage. |
| ACC-002 | 10,000-asset page/search budgets and golden result pass. |
| ACC-003 | PostgreSQL 17/18 migration and retry recovery pass. |
| ACC-004 | Contract/sqlc/Web/migration drift and full source gates pass. |
| ACC-005 | Stack smoke, security scan and release bundle pass. |
| ACC-006 | Three review lenses have no unresolved P0/P1/P2 finding. |

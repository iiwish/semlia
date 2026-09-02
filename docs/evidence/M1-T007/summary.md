# M1 T007 Production Acceptance Evidence

## Result

T007 is Accepted under the founder's continuous M1 execution authorization. The backend release
candidate passes the exact-reference journey, 10,000-asset performance budgets, recovery, smoke,
security, release and three review lenses with no unresolved P0/P1/P2 finding.

## Exact-Reference Journey

`go test -count=1 ./tests/integration/m1/...` passes against PostgreSQL 18. The journey registers a
credential-reference-only Catalog source, discovers immutable source revision
`7f3c9a2e4d1b8c6f0a9e3d7b5c2f1a8e6d4c3b2a`, creates evidence and a semantic metric, reads it through
the production HTTP handler, projects the committed revision to Git and publishes the usage event.
The assertions reject credential material and internal UUID leakage.

## Performance

Command: `SEMLIA_RUN_M1_BENCHMARK=1 go test -count=1 -v ./tests/performance/catalog/...`

| Host | PostgreSQL | Assets | Samples | Page p50 / p95 | Exact search p50 / p95 | Budget |
| --- | --- | ---: | ---: | --- | --- | --- |
| darwin/arm64 | 18-alpine | 10,000 | 25 | 7.851 / 8.184 ms | 36.659 / 38.778 ms | Pass: 150 / 250 ms |

The benchmark also verifies the golden semantic address and duplicate-free cursor pagination. These
figures are local regression evidence, not a hosted-service SLA.

## Release Gates

| Gate | Result |
| --- | --- |
| PostgreSQL 17/18 and retry suites | Pass |
| `make check-source` | Pass |
| `make check-smoke` | Pass, including worker restart and PostgreSQL failure/recovery |
| `make security-check` | Pass, zero repository, lockfile, OS and Go binary High/Critical findings |
| `make release` | Pass, archive, CycloneDX SBOM and SHA256SUMS generated |
| `git diff --check` | Pass |

The first security run correctly failed on newly disclosed fixed advisories. The repository baseline
was advanced to Go 1.26.6 and the scanner-specified fixed dependency versions before the successful
gate was recorded.

## Review Evidence

The spec-compliance, maintainer and QA review results and fixed findings are recorded in
`docs/evidence/M1-T007/reviews.md`.

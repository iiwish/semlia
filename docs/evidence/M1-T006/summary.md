# M1 T006 Evidence Summary

## Metadata

| Field | Value |
| --- | --- |
| Task | T006 Audit, outbox and usage |
| Status | `Accepted` |
| Implementation | Pending commit |
| Date | 2026-09-02 |
| Packet | `docs/specs/m1-semantic-registry/packets/T006.yaml` |

## Delivered

- Catalog transactions enqueue complete `semlia.events/v1` envelopes with TypeID identity,
  workspace, source, type, time, trace and versioned catalog data.
- Added fail-closed event routing and a cancellable leased dispatcher loop.
- Configured the production worker to run jobs and Git projection delivery together when
  `SEMLIA_GIT_REPOSITORY` is set; local Compose provides a dedicated persistent volume.
- Added migration `000004_usage_events` and typed sqlc persistence for exactly
  `catalog.asset.read` and `catalog.search.completed`.
- Added server-side read and search producers, workspace-scoped HMAC-SHA256 fingerprints, closed
  token/result buckets, idempotency and bounded 90-day deletion.

## Privacy And Recovery Proof

- The schema has no raw query or arbitrary payload column. A distinctive search phrase is absent
  from `row_to_json(usage_events)` while its HMAC and closed facets are present.
- Same trace/idempotency attribution produces one row. Successful read attribution contains the
  exact asset/revision; search attribution cannot contain asset/revision IDs.
- Matched, zero-result and failed outcomes are closed by application and database constraints.
- Bounded cleanup deletes one expired row and preserves current rows.
- PostgreSQL 17 and 18 pass the complete four-migration lifecycle and workspace foreign-key proof.

## Validation

| Command | Result |
| --- | --- |
| `go test -race ./internal/domain/usage/... ./internal/application/usage/... ./internal/application/catalog/... ./internal/application/jobs/... ./internal/application/projection/...` | Pass |
| `go test -count=1 ./tests/integration/usage/... ./tests/integration/projection/... ./tests/integration/catalog/... ./tests/integration/db/... ./tests/integration/worker/...` | Pass |
| `make db-generate-check` | Pass |
| `make check-source` | Pass; 99s test phase including acceptance, all drift gates and release build |
| `git diff --check` | Pass |

## Review

Usage remains a minimal trusted product signal rather than generic clickstream ingest. Audit,
outbox, usage and OpenTelemetry facts remain separate. No raw search text, customer fact rows,
Kafka, event store, stream processor or workflow engine is introduced.

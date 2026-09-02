# T006 Audit, Outbox And Usage Design

## Event Facts

Audit, reliable delivery, product usage and operational telemetry remain separate facts.

- Catalog mutations commit immutable audit rows and queued outbox rows in the same transaction.
- Every M1 outbox payload is a complete `semlia.events/v1` envelope with event TypeID, type, source,
  workspace TypeID, UTC time, trace ID and versioned data.
- The outbox router delivers only explicitly registered types. Unknown types fail closed and are not
  marked published.
- The worker runs the existing leased job worker and outbox dispatcher together when a local Git
  repository is configured.

## Usage Contract

M1 stores exactly two server-produced events:

| Event | Trigger | Allowed outcome |
| --- | --- | --- |
| `catalog.asset.read` | A current asset revision is returned | `succeeded` |
| `catalog.search.completed` | A non-empty catalog search completes or fails | `matched`, `zero_result`, `failed` |

`usage_events` has typed columns rather than an arbitrary payload. It stores data version,
idempotency key, actor attribution when available, asset/revision IDs for reads, trusted channel,
outcome, reason code, trace ID, occurred/received/expiry times, and privacy-bounded search facets.

Search text is normalized only in application memory. The stored fingerprint is a workspace-scoped
HMAC-SHA256 derived from `SEMLIA_SECRET_KEY`; token count, result count and filters are reduced to
closed buckets. No column can store raw search text or customer fact rows.

## Retention

- Usage expires after 90 days by default.
- Cleanup deletes an ordered bounded batch of expired rows and is safe to repeat.
- Published outbox rows use the existing seven-day operational retention contract; retryable and
  dead-letter rows are retained for operator action.

## Acceptance Matrix

| ID | Scenario |
| --- | --- |
| EVT-001 | Catalog mutation persists a valid complete envelope atomically with audit/domain state. |
| EVT-002 | Registered Git delivery publishes; unsupported event types fail closed. |
| USE-001 | Successful detail read records only the returned asset/revision attribution. |
| USE-002 | Search records HMAC, buckets and filters without raw text for matched/zero/failed outcomes. |
| USE-003 | Repeated idempotency keys do not duplicate usage rows. |
| USE-004 | Retention deletes only expired rows within the requested bound. |
| USE-005 | Worker runtime delivers catalog events into its configured local Git repository. |

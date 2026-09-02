# PostgreSQL persistence

Semlia uses PostgreSQL 18 as its only transactional database. Schema changes are explicit: application and worker startup never run DDL. Use `semlia migrate` or the root `db-migrate-*` Make targets.

## Migration baseline

`000001_m0_foundation` owns the M0 workspace identity, immutable audit facts, leased jobs, and transactional outbox tables. `000002` converts public identities to UUIDv7, `000003` owns the semantic registry, and `000004` owns privacy-bounded usage facts. Down migrations remove tables in dependency order. PostgreSQL check constraints keep lease, terminal-state, ontology and usage shapes consistent.

## Query and index policy

Job and outbox claim queries order available work by `available_at`, `created_at`, and `id`, then lock one row with `FOR UPDATE SKIP LOCKED`. The matching partial indexes only contain claimable states. Separate partial indexes support expired-lease scans, while workspace/status indexes support operational history views. Integration tests use `EXPLAIN` with sequential scans disabled to detect query/index drift.

## Retention guidance

- Retain succeeded jobs for 30 days and dead-letter jobs for 90 days.
- Retain published outbox events for 7 days; retain retryable and dead-letter events until resolved.
- Retain usage events for 90 days; the bounded cleanup query deletes expired rows safely.
- Retain audit events according to the product's audit policy.

Job, outbox and audit retention remain operational policy. Usage cleanup is explicit and does not rely on automatic partition deletion.

# PostgreSQL persistence

Semlia uses PostgreSQL 18 as its only transactional database. Schema changes are explicit: application and worker startup never run DDL. Use `semlia migrate` or the root `db-migrate-*` Make targets.

## Migration baseline

`000001_m0_foundation` owns the M0 workspace identity, immutable audit facts, leased jobs, and transactional outbox tables. The down migration removes those tables in dependency order. PostgreSQL check constraints keep lease and terminal-state fields consistent.

## Query and index policy

Job and outbox claim queries order available work by `available_at`, `created_at`, and `id`, then lock one row with `FOR UPDATE SKIP LOCKED`. The matching partial indexes only contain claimable states. Separate partial indexes support expired-lease scans, while workspace/status indexes support operational history views. Integration tests use `EXPLAIN` with sequential scans disabled to detect query/index drift.

## Retention guidance

- Retain succeeded jobs for 30 days and dead-letter jobs for 90 days.
- Retain published outbox events for 7 days; retain retryable and dead-letter events until resolved.
- Retain audit events according to the product's audit policy. M0 does not implement audit deletion.

Retention is operational guidance for a later maintenance job. This migration deliberately contains no automatic deletion or partition policy.

# ALPHA-T003 Delivery Summary

## Outcome

Semlia has one production-shaped PostgreSQL source-to-candidate backend path. Source credentials are
write-only API inputs stored as versioned AES-256-GCM envelopes. Discovery starts by atomically
creating an exact queued run and lease-backed job, pins the credential version, collects only
PostgreSQL catalog metadata through a verified read-only role, and commits physical observations,
findings and non-authoritative semantic candidates against that same run.

The public contract is version `0.6.0` (`x-semlia-contract-version: 6`) and migration 14 is the
required schema. Source list/create/detail/update/delete, credential rotation, connection test, run
start/list/detail, candidate list/detail and append-only candidate decision operations are present.
Source responses contain connection coordinates and credential version but never password,
ciphertext, nonce, derived key material or a reusable DSN.

## Security Boundary

- The application derives a 256-bit envelope key from `SEMLIA_SECRET_KEY` with a domain-separated
  HKDF context. Workspace ID, source ID, credential version and key version are authenticated as
  associated data; substitution and unknown key versions fail closed.
- A fresh 96-bit nonce is generated for every credential version. Retired envelopes remain readable
  only so already queued runs can use their pinned version.
- The collector rejects superuser, create-role, create-database, replication, bypass-RLS,
  database-create and schema-create capabilities. Queries execute in a read-only transaction with
  bounded connection, statement and lock timeouts.
- Catalog queries include only relations on which the source role has `SELECT`; customer fact rows
  are never selected.
- SQL evidence paths are relative `.sql` paths below `SEMLIA_SOURCE_ARTIFACT_ROOT`. Absolute paths,
  traversal, symbolic-link components, non-SQL files and oversized collections are rejected.
- The release image statically links the existing PostgreSQL parser and retains the distroless,
  non-root runtime image.

## Persistence And Recovery

- Migration 14 adds `source_credentials`, exact run/job and credential-version fields,
  `physical_key_observations`, `join_observations`, `semantic_candidates` and immutable
  `semantic_candidate_decisions`.
- One partial unique index prevents overlapping queued/running runs for a source. Repeated start
  requests with the same idempotency key return the same run.
- Worker retry and restart preserve run identity. A crash before job completion can resume the same
  run; a completed run is idempotently accepted when the job lease is recovered.
- Successful and degraded runs can advance physical projections and create deterministic candidate
  content. Connection, credential and unsafe-role failures terminate without calling projection
  persistence.
- Candidate dismiss/convert decisions are append-only, attributable and idempotent. Conversion
  references an existing M2 proposal; candidates remain non-authoritative discovery output.

## Deferred Boundary

The desktop source and candidate screens are still prototype-backed. T004 connects them to these
APIs and proves the browser source-to-candidate-to-proposal journey. Hosted identity-provider and
founder sample-source evidence remains part of T007 acceptance.

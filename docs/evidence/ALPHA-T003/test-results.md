# ALPHA-T003 Test Results

Validation date: 2026-09-04

## Passed Gates

- `go test ./...`: all Go unit, contract, acceptance, repository, performance, smoke-package and
  PostgreSQL integration suites passed.
- `make check-source`: format, Go vet, workspace lint, TypeScript checks, Go/Web tests, OpenAPI drift,
  sqlc migration drift, embedded Web drift and release build passed.
- `make check-smoke`: complete Compose build, migration, server/worker health, embedded Web/API,
  worker restart, PostgreSQL recovery and cleanup passed after the source artifact root was aligned
  with the mounted worker volume.
- `make security-check`: dependency, secret, Debian runtime and Go binary scans passed with zero
  detected vulnerabilities and zero detected secrets.
- `make contracts-check`, `make db-generate-check`, and `git diff --check` pass as part of the source
  gate or final verification.

## Focused Evidence

- Credential unit tests prove ciphertext excludes plaintext, correct decrypt, AAD substitution
  refusal, unknown key-version refusal and minimum root-key length.
- Artifact loader tests prove allowed versioned SQL reads and rejection of traversal, absolute path,
  symbolic link, non-SQL and unconfigured-root cases.
- The PostgreSQL 18 integration creates a dedicated read-only login and real PK/FK schema, tests the
  connection, queues a run, rotates the active credential before claim, processes the pinned prior
  credential, and verifies an exact successful run with two candidates, two key observations and one
  join observation.
- The same integration proves start idempotency, overlapping-run refusal, ciphertext persistence,
  append-only/idempotent dismiss and conversion decisions, and rejection of the elevated database
  owner as a source role.
- Migration integration covers empty and populated up/down/up lifecycles on PostgreSQL 18 and the
  supported PostgreSQL 17 compatibility path.
- OpenAPI validation proves the source response rejects a `password` field and generated Go and
  TypeScript clients match contract version 6.

## Residual Risk

- The existing production JavaScript bundle remains above Vite's 500 kB warning threshold. This is
  a pre-existing performance warning and does not affect the T003 backend acceptance path.
- Real browser use of the new APIs is intentionally deferred to T004; T003 evidence exercises the
  same service and persistence chain directly plus the generated HTTP contract.

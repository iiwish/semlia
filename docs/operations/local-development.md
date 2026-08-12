# Local Development Runbook

Semlia's M0 local environment mirrors the release process closely enough to exercise migrations, runtime health, worker behavior, security scans, SBOM creation, and release packaging. It is a development environment, not production deployment guidance.

## Stack Topology

Docker Compose starts four services:

| Service | Role | Expected state |
| --- | --- | --- |
| `postgres` | PostgreSQL 18 data store | Running and healthy |
| `migrate` | Applies committed SQL migrations before the application starts | Exited successfully |
| `server` | Serves health endpoints, the M0 API, and embedded Web status application | Running and healthy |
| `worker` | Executes durable jobs backed by PostgreSQL | Running |

The server binds to `127.0.0.1:8080` and PostgreSQL to `127.0.0.1:5433` by default. Internal services use the Compose-only `backend` network.

## Standard Journey

Run commands from the repository root:

```bash
make doctor
make bootstrap
make dev
make smoke
make dev-down
```

`make bootstrap` is safe to repeat and uses `go.mod`, `go.sum`, and `pnpm-lock.yaml`. `make dev` waits up to four minutes for the full dependency chain. `make smoke` expects an already-running stack and verifies the embedded Web surface, API, trace IDs, migration completion, worker persistence, PostgreSQL failure and recovery, and shutdown persistence. The smoke journey is disruptive: it briefly stops PostgreSQL and recreates this checkout's containers and network while preserving the data volume. Never run it against a shared project identity.

Inspect state and logs with:

```bash
./scripts/dev/compose.sh ps --all
./scripts/dev/compose.sh logs --follow server worker
./scripts/dev/compose.sh logs migrate postgres
```

## Local Credentials and Ports

The first stack start creates `.semlia/dev.env` with mode `0600`. It contains generated development-only credentials, ports, the build identifier, and `COMPOSE_PROJECT_NAME=semlia-local`. The file is ignored by Git. Never reuse its values in a shared or production environment.

To change ports, stop the stack and edit only these entries:

```dotenv
SEMLIA_HTTP_PORT=18080
SEMLIA_POSTGRES_PORT=15433
```

Keep `COMPOSE_PROJECT_NAME` stable for a normal local checkout so subsequent commands address the same resources. Automated or parallel acceptance runs must use a unique `COMPOSE_PROJECT_NAME` and unoccupied host ports.

If `.semlia/dev.env` has an invalid credential shape or is a symbolic link, the startup script fails closed. Remove the invalid local file and run `make dev` to regenerate it.

## Database Lifecycle

The `migrate` service applies all committed migrations before `server` or `worker` starts. For an externally configured test database, maintainers can inspect and control the migration state with:

```bash
SEMLIA_DATABASE_URL='postgresql://...' make db-migrate-version
SEMLIA_DATABASE_URL='postgresql://...' make db-migrate-up
SEMLIA_DATABASE_URL='postgresql://...' make db-migrate-down
```

Do not run these commands against production: Semlia has no supported production release or upgrade policy yet.

`make dev-down` preserves the Compose data volume. To deliberately delete only the current project's local database after stopping the services:

```bash
./scripts/dev/compose.sh down --volumes --remove-orphans
```

Never use a global Docker prune as Semlia cleanup. It can remove unrelated projects' resources.

## Validation and Release-Shaped Builds

Use the narrowest useful check while iterating:

```bash
make check-source
make check-smoke
make security-check
make check
make release
make sbom
```

`make check-source` verifies formatting, repository and API contracts, generated artifacts, lint, types, unit and integration tests, and builds. `make check-smoke` owns stack startup and cleanup. `make security-check` scans dependencies, secrets, and the release container. `make release` writes a checksummed bundle for the selected target platform under ignored `build/release/`; the hosted workflow runs the supported platform matrix.

Security reports, protected branches, hosted code scanning, and tagged provenance are public-release gates. Their presence must not be inferred from successful local commands during Private incubation.

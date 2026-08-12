# Local Troubleshooting

Start with `make doctor`, then inspect the Compose state and recent logs:

```bash
./scripts/dev/compose.sh ps --all
./scripts/dev/compose.sh logs --tail 200 postgres migrate server worker
```

## Doctor Reports a Version Error

Semlia requires the exact Go, Node.js, and pnpm versions in `.tool-versions`. Activate those versions in the current shell, confirm `go version`, `node --version`, and `pnpm --version`, then rerun `make doctor`. Running from outside the repository also fails the Go module check.

## A Default Port Is Busy

Stop the other listener or edit `SEMLIA_HTTP_PORT` and `SEMLIA_POSTGRES_PORT` in `.semlia/dev.env` after running `make dev-down`. Do not change the container-side ports in Compose. Run `make doctor` again to confirm that the defaults are no longer relevant to the chosen configuration.

## Local Environment File Is Rejected

`.semlia/dev.env` must be a regular file with generated 64-character hexadecimal credentials. The setup refuses symbolic links and malformed values. Remove only this local ignored file, then regenerate it:

```bash
rm .semlia/dev.env
make dev
```

Do not add credentials to `.env.example` or commit the generated file.

## Migration Does Not Complete

Inspect `postgres` and `migrate` logs. The application services wait for PostgreSQL health and a successful migration exit. A migration failure intentionally prevents `server` and `worker` startup.

For an isolated test database, inspect the committed migration state with `make db-migrate-version`. Generated database code must remain current; run `make db-generate-check` before editing generated files.

## Readiness Returns 503

`/health/live` reports whether the process is alive. `/health/ready` also checks required dependencies. When PostgreSQL is unavailable, readiness returns HTTP 503 with stable code `DEPENDENCY_UNAVAILABLE` and a `traceId`; liveness and the embedded status application remain available.

Check PostgreSQL and restart it without deleting data:

```bash
./scripts/dev/compose.sh ps postgres
./scripts/dev/compose.sh logs --tail 200 postgres server
./scripts/dev/compose.sh start postgres
curl --fail --show-error http://127.0.0.1:8080/health/ready
```

The trace ID can be matched against structured server logs. If recovery stalls, run `make dev-down` followed by `make dev`.

## Contract or Generated-File Drift

Verify committed OpenAPI artifacts without rewriting them:

```bash
make contracts-check
make db-generate-check
```

If a source contract was intentionally changed, regenerate through the repository commands, review all generated diffs, and rerun `make check-source`. Do not hand-edit generated API or sqlc output.

## Smoke or Security Check Leaves Resources

Both gates are designed to clean up their own Compose containers and networks. Run `make dev-down` to remove normal local containers while preserving data. For deliberate deletion of only Semlia's current Compose data volume, use `./scripts/dev/compose.sh down --volumes --remove-orphans`.

Never use `docker system prune`, `docker volume prune`, or cleanup commands targeted at another `COMPOSE_PROJECT_NAME`.

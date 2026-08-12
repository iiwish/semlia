# Semlia Quickstart

Semlia is in Private incubation and pre-alpha. This quickstart exercises the implemented M0 engineering foundation: a Go control server, PostgreSQL, migration job, worker, embedded status Web application, contracts, and operational checks. The semantic registry and Cube-backed product loop begin in M1 and are not available yet. The current inspectable product prototype uses repository-local mock data.

## Prerequisites

Install the exact tool versions from `.tool-versions`:

- Go 1.26.5
- Node.js 24.15.0
- pnpm 11.1.3
- Git, GNU Make, and a running Docker engine with Compose

The repository is private during incubation, so an authenticated GitHub account with repository access is required. Start from a clean clone:

```bash
git clone https://github.com/iiwish/semlia.git
cd semlia
```

Check the host from the repository root:

```bash
make doctor
```

The supported quickstart uses the default HTTP and PostgreSQL host ports `8080` and `5433`; resolve an occupied default before starting. Port overrides are an advanced local-development option documented in the [runbook](operations/local-development.md), not part of the commands below.

## Start Semlia

Install dependencies exactly as locked:

```bash
make bootstrap
```

On the first run, this command downloads Go modules and pnpm packages. Start the local stack:

```bash
make dev
```

`make dev` creates `.semlia/dev.env` with local random credentials, builds the release-shaped image, waits for PostgreSQL, runs migrations, and starts the server and worker. Wait for the command to finish successfully before making requests.

Verify the complete local journey:

```bash
make smoke
curl --fail --show-error http://127.0.0.1:8080/health/ready
curl --fail --show-error http://127.0.0.1:8080/api/v1/system/info
```

`make smoke` is disruptive: it briefly stops PostgreSQL and recreates this checkout's containers and network while preserving its data volume. Do not run it against a shared development stack or while another command is using the same `COMPOSE_PROJECT_NAME`.

Open `http://127.0.0.1:8080` to inspect the embedded status application. A healthy readiness response includes a 32-character `traceId`. The system-info response reports the build and runtime identity.

## Stop Semlia

```bash
make dev-down
```

This removes task containers and the Compose network but preserves the local PostgreSQL volume. To remove this repository's local data deliberately, see the [local development runbook](operations/local-development.md).

For failures, use the [troubleshooting guide](operations/troubleshooting.md). Run the full pull-request gate with `make check`; it is substantially slower because it builds images and runs source, integrated smoke, dependency, secret, and release-container checks.

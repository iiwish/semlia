# Semlia Quickstart

Semlia is in Private incubation and pre-alpha. This quickstart runs the production-shaped local
stack: Go control server, PostgreSQL, migrations, worker, embedded desktop Web application,
generated contracts and operational checks. The M1 workspace and semantic Catalog are live;
future-milestone authoring, release and administration surfaces identify their preview or session
scope in the product.

## Prerequisites

Install the exact tool versions from `.tool-versions`:

- Go 1.26.6
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

Open `http://127.0.0.1:8080` to use the embedded Semlia workspace. Create or select a workspace,
then use **知识资产** to create, search and inspect PostgreSQL-backed semantic assets. System
diagnostics are available at `http://127.0.0.1:8080/status`. A healthy readiness response includes
a 32-character `traceId`; system info reports the build and runtime identity.

## Stop Semlia

```bash
make dev-down
```

This removes task containers and the Compose network but preserves the local PostgreSQL volume. To remove this repository's local data deliberately, see the [local development runbook](operations/local-development.md).

For failures, use the [troubleshooting guide](operations/troubleshooting.md). Run the full pull-request gate with `make check`; it is substantially slower because it builds images and runs source, integrated smoke, dependency, secret, and release-container checks.

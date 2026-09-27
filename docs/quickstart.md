# Semlia Quickstart

Semlia's scoped 1.0 is a local release candidate under private incubation, pending final candidate gates and user acceptance. Source discovery, five knowledge types, independent review/publication and published-model Ask/Execute are real workflows. See the [current scope](specs/v1-convergence/spec.md) and [evidence index](README.md#交付与证据); this guide does not assert a signed public release or enterprise SLA.

The default development path runs a native Go server, worker and Vite against an **existing PostgreSQL 18** instance. It supports desktop browsers at 1024px or wider and does not create or replace a database service.

## 1. Tools and Repository

Install the exact versions in `.tool-versions`: Go 1.26.6, Node.js 24.15.0 and pnpm 11.1.3, plus Git and GNU Make. The repository doctor and full integration gates also require a running Docker-compatible engine with Compose; OrbStack is suitable on macOS. Database administration requires an authorized PostgreSQL client or operator.

```bash
git clone https://github.com/iiwish/semlia.git
cd semlia
make doctor
make bootstrap
```

Repository access is required during private incubation. Busy default-port warnings are not permission to stop another listener. Select unused native ports instead.

## 2. Own the Database

Identify the existing PostgreSQL 18 instance and its host port. Have its authorized administrator create a **new, explicitly owned development database and dedicated login**. Do not reuse an infrastructure administrator account, adopt an unrelated database or start another PostgreSQL merely to follow this guide.

For a fresh local database, use an interactive administrator `psql` session with an approved private connection profile. The committed migrations use the trusted `pg_trgm` extension; installing a vector extension is not required. The following names are examples; first verify that they are unused:

```sql
CREATE ROLE semlia_dev LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
\password semlia_dev
CREATE DATABASE semlia_dev OWNER semlia_dev TEMPLATE template0;
REVOKE CONNECT, TEMPORARY ON DATABASE semlia_dev FROM PUBLIC;
```

Enter a generated password at the hidden prompt and record ownership privately. This local role owns only its dedicated development database. Shared or production deployments should separate approved migration and application privileges; the running application must never use the PostgreSQL administrator credential.

For an existing Semlia database, first verify [backup and recovery](operations/backup-recovery.md) in a new target. Upgrade evidence covers schema 32 initialized with identity/workspace data to schema 33, not arbitrary legacy seven-type prototype data.

## 3. Protected Configuration

```bash
./scripts/dev/ensure-env.sh
```

This creates `.semlia/dev.env` only when absent and generates local random secret material; it does **not** provision PostgreSQL. Preserve an existing file, its credentials and `SEMLIA_SECRET_KEY`; do not delete it to cure a validation error. Keep `.semlia/` private and both environment files at mode `0600`.

Using a local editor, create `.semlia/native.env` with the owned database settings below. The URI is a placeholder, not a usable credential:

```dotenv
SEMLIA_DATABASE_URL=postgresql://semlia_dev:<URL-encoded-password>@127.0.0.1:<existing-pg-port>/semlia_dev?sslmode=disable
SEMLIA_NATIVE_WEB_PORT=18081
SEMLIA_NATIVE_API_PORT=18080
SEMLIA_SEMANTIC_PRODUCTION_ENABLED=true
SEMLIA_PRODUCTION_GENERATION_GRANTS=[]
```

```bash
chmod 600 .semlia/dev.env .semlia/native.env
```

The wrapper requires `SEMLIA_POSTGRES_PASSWORD` and `SEMLIA_SECRET_KEY` in its private configuration; an explicit `SEMLIA_DATABASE_URL` selects the actual database credential instead of the generated default. Passwords must not appear in shell arguments, Git, screenshots or chat. `sslmode=disable` is a loopback development example, not production policy. Use a shell without unrelated Semlia or Compose overrides; native development is not the isolated acceptance harness.

The wrapper fixes `.semlia/native/content` as both Git-projected content and source-input root, and `.semlia/native/artifacts` as uploaded content storage. Both processes use the same stores and key. Native environment overrides do not relocate these stores; advanced deployments use explicit runtime configuration.

## 4. Migration and First Account

```bash
go build -o build/semlia ./cmd/semlia
node scripts/dev/native.mjs migrate version
node scripts/dev/native.mjs migrate up
node scripts/dev/native.mjs migrate version
bash scripts/dev/account.sh bootstrap-local-admin semantic-core "Semantic Core" admin
```

An empty database may report no migration version before `up`; afterwards it must report **33**, not dirty. The account command requires an interactive local terminal and prompts twice without echoing the password. It establishes an ordinary workspace administrator, not a fixture identity or anonymous signup. Repeating bootstrap does not reset an existing password.

## 5. Start and Check

```bash
make dev
make smoke
curl --fail --show-error http://127.0.0.1:18081/health/ready
curl --fail --show-error http://127.0.0.1:18080/api/v1/system/info
```

Use the URL printed by `make dev` if you selected other ports, then sign in. The command compiles Go, checks configuration/schema and supervises server, worker and Vite. It does **not** migrate automatically, create PostgreSQL, build an application Docker image or stop unknown listeners. `make smoke` checks the running native supervisor, Web proxy and API readiness; it does not recreate containers or inject an outage. Diagnostics remain at `/status`.

An empty installation has no model or business knowledge. Configure an enabled workspace LLM/provider and place its credential variable only in protected server/worker configuration. Register a synthetic PostgreSQL source with a dedicated read-only account, discover its schema, curate the five knowledge types and independently review/publish an analysis model. Execution additionally requires an explicit `SEMLIA_EXECUTION_SOURCES` mapping and an execution-only read-only credential; discovery configuration alone does not authorize execution. See [source credentials](operations/local-development.md#数据来源凭据).

The feature flag does not grant paid generation, confirm rules or publish proposals automatically. Ask clarification/refusal is a valid outcome; execution is a separate explicit action.

## 6. Stop or Use Containers

```bash
make dev-down
```

This stops only owned native processes, preserving PostgreSQL, containers and data. Do not delete environment files, volumes or ownership records to work around an error.

Container self-hosting is a separate operator workflow. The [self-host Compose example](../deploy/examples/README.md) requires an approved immutable image, existing database network, explicit credentials and durable paths; it does not create a database, role or secret. Server and worker both wait for successful migration. The root development Compose stack can provision disposable PostgreSQL for an explicitly isolated test project; it must not adopt or clean up a shared installation.

See [troubleshooting](operations/troubleshooting.md) and [release verification](operations/release-verification.md). `make check-smoke` owns a separate Docker test project. `make security-check SEMLIA_SECURITY_IMAGE=<fresh-owned-tag>` builds and scans a dedicated image. Neither is the everyday `make smoke` command.

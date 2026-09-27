# Local Troubleshooting

Identify the runtime before taking action. Normal `make dev` runs native server/worker/Vite; it is not a Compose deployment. The isolated V1 acceptance harness and container deployments have separate ownership receipts and lifecycle commands. Never stop a process, container or database solely because its name or port looks familiar.

## Tools and Readiness

Run `make doctor` from the repository root and use the versions in `.tool-versions`. `make smoke` checks the already-running normal native supervisor and Web/API readiness. Native logs are under `.semlia/native/` (`server.log`, `worker.log`, `web.log`, `supervisor.log`); inspect them privately and share only allowlisted status/trace identifiers, not full environment, DSNs or raw provider errors.

`/health/live` is process liveness; `/health/ready` includes required dependencies. HTTP 503 is not permission to rebuild or reset PostgreSQL. Verify the configured endpoint, dedicated role, schema 33/not dirty, encryption key and durable stores. Restart only the explicitly owned runtime after correcting its configuration.

A missing required dependency returns the stable `DEPENDENCY_UNAVAILABLE` code with a `traceId`. Correlate that identifier with protected structured server logs to identify the failing dependency; share only the code and trace identifier, not raw configuration or credential-bearing logs. Recovery requires a successful readiness check, not merely a live process.

For an explicitly identified Compose installation, use its exact project name, Compose files and protected environment files for `ps` and private logs. Do not default to another installation's Compose wrapper or inherited `COMPOSE_FILE`.

## Port Occupied

Inspect ownership with `lsof -nP -iTCP:<port> -sTCP:LISTEN`. Choose unused `SEMLIA_NATIVE_WEB_PORT` and `SEMLIA_NATIVE_API_PORT` values in `.semlia/native.env`; they must differ. Do not stop an unknown listener. Doctor warns about common default ports and may still warn when an intentional override is configured. Use the URL printed by `make dev` rather than assuming port 8080.

## Private Environment Rejected

Preserve `.semlia/dev.env`, `.semlia/native.env`, backups and encryption keys. Do **not** follow a regeneration suggestion by deleting an existing environment file: a new password does not change the existing PostgreSQL role, and a new encryption key cannot decrypt existing source credentials.

Verify locally that files are regular, not symbolic links, protected with `0600`, and contain the expected entries. `ensure-env.sh` expects 64-character hexadecimal default secret values; an explicit `SEMLIA_DATABASE_URL` must refer to the owned database with its real credential. Repair from a trusted private record or use a separately approved credential rotation. Never paste the file, parsed configuration, cookie or credential value into tool output, a ticket or chat.

The default native entry merges both local configuration files and retains the caller environment. An explicit `SEMLIA_NATIVE_ENV_FILE` selects a required complete independent file and also requires `SEMLIA_NATIVE_STATE_DIR` inside the current repository's `.semlia/` tree. Missing inputs fail instead of falling back. The explicit branch passes only tool-environment values and its own configuration, not ordinary local credentials, execution sources or caller LAN settings. The file must not contain the reserved `SEMLIA_NATIVE_ENV_FILE` or `SEMLIA_NATIVE_STATE_DIR` controls. Use the same two controls for startup, status, migrations, account commands and shutdown. See [T004 browser isolation evidence](../evidence/V1-T004/browser-gate-isolation.md); final RC verification remains separate.

## Migration or Login Fails

Native startup checks schema but does not migrate. For a fresh owned database, build the binary and use the explicit [quickstart migration/account sequence](../quickstart.md#4-migration-and-first-account). For existing data, first verify a full [backup and restore](backup-recovery.md) into a new owned target. Do not force a migration version or downgrade a live schema-33 database to make startup pass.

The current upgrade evidence is schema 32 with initialized identity/workspace data to 33, not arbitrary historic seven-type prototype compatibility. Container server and worker must wait for `migrate` to complete successfully; a failed migration correctly blocks them.

Use `scripts/dev/account.sh` interactively to bootstrap the first administrator or reset a known local account. Login rate limits apply to successful attempts too; wait for the normal cooldown instead of deleting budget rows, changing identity or altering client IP. Password recovery revokes existing account sessions.

## Ask, Source and Correction Failures

Ask requires an enabled supported LLM configuration, its private credential variable and an authorized published analysis model. Clarification/refusal is not a result table. Inspect stable error categories and exact release/revision pins; do not change providers, timestamps or expected SQL answers silently during acceptance.

Source discovery and query execution are separate configurations. Verify the discovery source's encrypted credential and read-only role, then the execution-only `SEMLIA_EXECUTION_SOURCES` mapping and named DSN. Do not enable an unsafe-source permission bypass. Plaintext loopback development is a transport exception only.

SQL registration/discovery does not imply content preview support: the SQL adapter returns HTTP 422 for preview. Use a supported preview type such as Markdown when checking artifact recovery. A correction's pending request key is memory-only; after a page reload or unknown network result, inspect existing operations before submitting again. Historical producer lookup stops after 64 steps and fails explicitly rather than inventing a baseline. [Local development](local-development.md#桌面与恢复边界) describes these limits.

## Generated Drift and Cleanup

```bash
make contracts-check
make db-generate-check
```

Use generators only for an intentional source change; review their diffs, never edit generated output by hand. Final release gates require one frozen source fingerprint.

`make dev-down` stops owned native processes only: it does not remove Docker containers, networks or volumes. `make check-smoke` is a separate isolated destructive test of its **own** disposable stack. Inspect exact ownership labels/receipts before any cleanup; preserve unknown resources and report a collision. Do not delete ownership files merely because control is unavailable. Never run global Docker prune or blanket volume deletion.

Run security gates with a fresh owned `SEMLIA_SECURITY_IMAGE` tag to protect existing images. A prior development-tool output disclosed private configuration; arrange approved rotation before production, without reproducing values or discarding encryption recovery material. Candidate gate results and unresolved checks are recorded under [T005 evidence](../evidence/V1-T005/).

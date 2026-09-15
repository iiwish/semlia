# FMB-T007 Review Plan

Status: Review in progress. Scope: third three-hour sprint, 2026-09-05.

One implementation worker owns source changes. The orchestrator independently reviews
contracts, changes, runtime behavior and evidence. A read-only reviewer checks execution
security. Implementation stops expanding at 10:20:34 UTC; the final deadline is 10:45:34 UTC.

## Required Local Proof

| Boundary | Acceptance evidence |
| --- | --- |
| Published physical provenance | Future release transactions freeze relation, fields, source revision and supported semantic projection. Old releases without this projection refuse execution. No historical live backfill. |
| Compiler | Typed supported aggregate plan, quoted identifiers and bound values; no arbitrary expressions, SQL, unsafe joins or mutable physical lookup. Full projection enters the digest. |
| Authority | Current principal, credential, consumer, binding, release and asset authority checked before compilation/connection, replay and known-ID reads. Explicit execute opt-in does not widen existing credentials. |
| Source safety | Dedicated least-privileged read-only role and transaction, source locator matches configured connection, bounded timeout/concurrency/rows/bytes and cancellation. |
| Durable lifecycle | Atomic claim before source query; concurrent requests and process loss cannot replay external work. Successful replay returns metadata only. |
| Privacy | No source credentials, compiled SQL, bound values, fact rows or raw driver diagnostics in new execution records, runtime, audit or public errors. |
| Channels and UI | REST, MCP, CLI, SDK and Ask share the same application service. Operations exposes durable metadata. Desktop 1440x900 and compact desktop 1024x768; keyboard, refresh and truthful unavailable states. |
| Upgrade and recovery | Populated schema15-to21 and migration21 down/up, immutable release behavior, server/worker/database recovery without deleting user volumes. |
| Candidate artifacts | Stable generated files before source gates; production bundle excludes local-UAT identities; artifact version, migration, checksums, SBOM and source fingerprint match the tested candidate. |

## Execution Rules

- Serialize database suites and Docker builds. Complete embedded-Web generation before
  Go source gates. Preserve the normal local preview and all existing dirty work.
- Use `make smoke` against the running preview. `make check-smoke` owns teardown and is
  reserved for a separately isolated stack, not the user's preview.
- Record RED failures and authoritative GREEN reruns. Do not relabel fixture routing,
  loopback receivers or local providers as hosted acceptance.
- Freeze a sprint-relative source patch against `.semlia/sprint-t007/baseline.json`.
  Prior sprint evidence remains historical and immutable.
- No commit, push, external deployment or founder acceptance is performed automatically.

## External Boundary

`hosted-matrix.md` is the external-input and evidence ledger. A locally passing candidate
does not satisfy FR-012's hosted HTTPS/OIDC/live-provider/receiver/source/OTel/recovery
requirements. Full-Menu Beta acceptance remains founder-owned.

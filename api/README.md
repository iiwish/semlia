# Public Contracts

`openapi/semlia.v1.yaml` is the canonical Semlia HTTP and shared-envelope contract. Generated Go and TypeScript files are committed review artifacts, not independent sources of truth.

## Versioning

- API paths use a major version such as `v1`.
- `info.version` is the semantic version of the complete contract bundle.
- `x-semlia-contract-version` is the machine-readable contract format generation.
- Released contracts remain immutable. Breaking changes require a new major API or envelope version and an explicit migration decision.

## Change Policy

- Additive optional fields and endpoints are normally compatible.
- Removing or renaming fields, narrowing accepted values, adding required input, or changing response types is breaking.
- Public identifiers, timestamps, errors and events use shared component schemas.
- Contract files never include semantic asset fields before their product specification is confirmed.

Run `make contracts` after editing the specification and `make contracts-check` before review.

Production business definitions require an explicit authorized-human POST to
`production-operations/{operationId}/business-rule-confirmations`. A `confirm`
binds selected declared evidence, or an explicit `declaration` supplied by an
authorized human, to the exact version and target content; a
`revoke` appends a withdrawal. GET with `version` returns latest events and current
validity, including the original human declaration for independent review.
Human declarations create immutable evidence atomically without SQL seeding.
The TypeScript production client exposes `recordBusinessRule` and
`businessRules`. Confirmers are contributors, not independent reviewers or
publishers. Saved drafts remain allowed without confirmation, but validation
and publication fail closed. Migration 28 and validator `1.0.0-production.2`
are required; older validation receipts must be rerun, not backfilled.

## Source History

Source history exposes four `GET` routes under
`/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots`: list, snapshot
detail, `/members`, and `/diagnostics`. Every read evaluates `source.read` for the
source. A snapshot or source outside the requested relation returns `404`; a known
source denied by the current capability decision returns `403`. These routes are
not added to the machine-credential allowlist.

Lists default to 50 items, accept limits from 1 through 200, and return at most
1 MiB of JSON per page. `nextCursor` is either null or an opaque signed token
bound to the route, workspace, source, snapshot, member-kind filter, upper
watermark, and last emitted key. Clients must preserve the filter and resource
when continuing a page. Invalid or tampered cursors return `INVALID_CURSOR`.
Later snapshots do not enter an existing list traversal. The TypeScript
`createSourceSnapshotClient` provides the corresponding typed reads.

Snapshot members retain observed names, locators, and exact revisions, including
unchanged revisions. Fields identify their exact parent dataset revision. Code
revisions retain verified content-addressed bytes up to the existing 50 MiB
internal staging boundary. Public HTTP artifact upload rejects SQL; `ImportSQL`
uses the local artifact loader with an 8 MiB aggregate limit. The dbt adapter
retains its 10 MiB raw-code limit. Lineage references exact
dataset and code revisions. Snapshot-dependent artifact objects are protected
from normal retention cleanup. Partial and failed attempts do not replace the
last complete effective snapshot; their unit heads still expose the latest
attempt so earlier success cannot masquerade as fresh completeness.

Coverage keys are bounded stable identities. SQL logical paths retain their full
selector up to 1024 bytes, with long keys encoded as a namespaced SHA-256 digest.
Diagnostic codes are coalesced within each coverage unit at their maximum
severity. A blocker invalidates its affected unit without downgrading unrelated
complete units.

Legacy runs without reconstructible immutable evidence have `unverifiable`
history with unknown, incomplete coverage and no fabricated current members.
Discovery-run responses include a nullable `snapshotId`. Snapshot identity and
evidence do not grant approval, create semantic assets, or publish releases.

## Production Generation

`POST /api/v1/workspaces/{workspaceId}/production-operations/{operationId}/generation`
queues an existing `AgentRun` and job against an exact draft version and input
digest. The request requires `Idempotency-Key`, a model setting/configuration
revision, instruction, and token/cost ceilings. Model/provider lists expose
`generationConfigRevision`, which pins both configurations but does not grant
spending permission. `SEMLIA_PRODUCTION_GENERATION_GRANTS` is empty by default;
an operator must configure matching principal, workspace, model, revision and
conservative pricing bounds. Client ceilings can only reduce those bounds.

The initial response is `202` with a recovery `Location`. Replaying the same key
returns `200` and the original run without another provider invocation.
`GET .../generation/{runId}` reauthorizes the exact operation/source/output scope.
Only `succeeded` exposes an immutable schema-gated output and digest. Missing
vendor cost is `null`; `outcome_unknown` never triggers automatic model retry.
Provider mode is server-derived, not a client assertion. Raw prompts, failed
provider output and hidden reasoning are not returned.

Generation does not change the draft or publish. Explicit `PUT` with
`suggestionRunId` and `expectedVersion` applies or corrects suggestions and records
the original output digest, human actor and server-computed delta. The resulting
version follows the same validation, review and separation-of-duties gates.
The TypeScript production client exposes `generate`, `generation` and `replace`.
Production routes do not expand the machine-credential allowlist.

## Machine Distribution

The workspace-scoped semantic REST and MCP routes accept server-derived machine
bearer context. Browser and bearer authentication cannot be combined. Credential
administration and webhook commands retain browser-session CSRF/Origin requirements.
Known-ID reads and replay intersect current grants with credential scopes.

The official MCP Go SDK v1.7.0 implements Streamable HTTP and the `semlia mcp` stdio
bridge. [Upstream SDK](https://github.com/modelcontextprotocol/go-sdk) defines the
protocol transport API; Semlia's canonical resolver controls domain behavior.
`semlia semantic` and TypeScript `createSemanticClient` preserve REST projections.

See [Machine Distribution Operations](../docs/operations/distribution.md) for credential
bootstrap, one-time secret handling, exact rotation semantics, signed webhook receiver
verification, retries and destination restrictions. Local client parity does not imply
external package publication or hosted third-party acceptance.

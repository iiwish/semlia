# FMB-T006 Summary

Status: Needs_Review. Local implementation and review gates pass; founder acceptance is not claimed.

## Delivered Scope

- Accountable agent principals without implicit grants, immutable consumer-agent
  association and verifier-only one-time credentials. Rotation is atomic, zero-grace
  and expiry-preserving. Current principal, consumer, binding, action and resource
  restrictions apply to bearer requests, known IDs and replay.
- Browser CSRF/Origin behavior is retained; mixed cookie/bearer and forged machine
  context are rejected. Credential/workspace rate limits have bounded expiring state
  and HTTP429/Retry-After behavior.
- Canonical released-semantic REST, official MCP Streamable HTTP, `semlia semantic`,
  `semlia mcp` stdio and TypeScript SDK methods share the resolver and public projection.
  No T007 execution implementation or arbitrary SQL tools are included.
- Durable independent webhook fanout, encrypted versioned signing material, stable
  HMAC event identity, pinned SSRF-safe transport, bounded jittered retries, fenced
  leases, dead-letter state, audited one-time replay and Operations projection.
- Real Integration Settings with server-owned lists and commands, explicit grant,
  one-time secret dismissal, unbound-consumer recovery, subscription endpoint/filter
  editing and durable delivery visibility. Fixture mode cannot simulate credentials.

## Implementation Files

Primary application/domain files:
`internal/application/{identity,authorization,distribution,webhooks,jobs}`,
`internal/domain/{identity,webhooks}` and `pkg/identity/distribution.go`.

Adapters and composition:
`internal/adapters/{mcp,webhooks}`,
`internal/adapters/postgres/{identity,webhooks}.go`,
`internal/platform/http/{handler,identity,machine,distribution,webhooks}.go`,
`cmd/semlia/{main,readiness,semantic,mcp}.go`.

Persistence/contracts:
`migrations/000020_fmb_distribution_interfaces.{up,down}.sql`,
`db/queries/{identity,webhooks}.sql`, `db/sqlc.yaml`, generated sqlc files,
`api/openapi/semlia.v1.yaml`, generated Go/TypeScript contracts, `go.mod`, `go.sum`,
`sdk/typescript/src/{client,index,ids}.ts`.

Frontend and proof:
`web/src/{IntegrationSettingsView,ProductApp}.tsx`, `web/src/integrations.ts`,
`web/src/styles.css`, their focused tests, approved localUAT capability/test extension,
`web/e2e-live/distribution-live.spec.ts`, new machine/webhook database integration tests,
updated migration expectations, `docs/operations/distribution.md` and this evidence
directory. Root owns generated embedded web artifacts and the exact baseline-relative
changed-file manifest, preserving prior dirty Alpha/FMB work.

## Evidence

See [Test Results](test-results.md) and [Sprint Attempt](attempts/sprint-a001.md).
The complete source gate passes, including 180 Web tests and contract/sqlc/embedded
drift. An uncached real PostgreSQL suite passes in 34.972 seconds, covering credential
denial audit, action/release restrictions, concurrent rotation/revocation, all four
channel entry points, signed receiver/fanout/retry and populated migration20 lifecycle.

Real Docker browser journeys pass at 1440x900 and 1024x768: explicit identity/grant,
consumer/binding, one-time credentials, next-request rotation/revocation, persisted
webhook create/edit/filter/disable/signing rotation and reload. Broad fixture journeys
pass 26/26 with truthful unavailable integration controls. Serious/critical Axe findings
are zero on the checked integration surface. Root inspected screenshots and keyboard
focus, secret dismissal and desktop overflow; no mobile acceptance is claimed.

The configured HIGH/CRITICAL dependency, secret and production-image security gate
passes. Earlier product, test-fixture and host-contention REDs remain visible in
`review.md` and separate logs; they are not substituted for final GREEN results.

## Frozen Handoff

The sprint-relative review patch covers 69 implementation, contract, operation-document
and generated-artifact paths, excluding governance/evidence records. `changed-files.txt`
and `source-manifest.json` record the precise before/after boundary; it is not a
cumulative HEAD diff. Patch SHA-256:
`d5bcfd0df7b77beb9e770fb29cf5dac9d4d4c8fe34e6b9d65a1c9b887dc1152f`.
Baseline HEAD is `source-revision-redacted`. No commit or push is made.
The existing dirty Alpha/FMB work and frozen T004/T005 patches are preserved.

## Bounds

- No hosted receiver, external MCP client installation or published SDK-registry
  acceptance is inferred from local tests.
- Rate counters are process-local; multi-replica quotas require an upstream gateway.
- Webhook edits/rotation cancel old-version queued deliveries; in-flight attempts can
  finish their captured version. There is no dual-sign or credential grace window.
- Manual dead-letter replay is limited to one extra eight-attempt batch, retaining the
  original event, payload and subscription version.
- The additive channel vocabulary remains on downgrade to preserve historic records.
- T007 remains out of scope. Full task acceptance requires root review and founder
  confirmation after final evidence, not merely implementation completion.

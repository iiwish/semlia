# FMB-T006 Review Plan

Status: Pending implementation

## Security And Persistence

| Boundary | Required counterexample |
| --- | --- |
| Credential material | At least 256 random secret bits; verifier-only storage; no raw secret in list, audit, logs or browser persistence |
| Authentication modes | Bearer plus either Semlia session cookie is rejected; browser Origin/CSRF remains enforced |
| Current authority | Grant removal, principal/consumer suspension, credential revoke and expiry deny the next protected request |
| Context ownership | Forged workspace, principal, consumer, binding and channel inputs cannot widen server-derived authority |
| Scope | Restricted credential cannot read excluded asset IDs through resolve, search, describe, stored plan, refusal candidates or idempotent replay |
| Rotation | New credential is distinct; old credential has explicit bounded grace and cannot extend its original expiry |
| Migration | Populated 19-to-20 up/down/up preserves pre-T006 rows and restarts cleanly |
| Webhook envelope | Only allowlisted external fields; committed event identity and workspace agree; no internal outbox forwarding |
| Webhook signing | Receiver verifies timestamp, event ID and body signature; replay/duplicates use a stable idempotency key |
| Outbound safety | DNS and actual dial target remain checked; IPv4/IPv6 private, loopback, link-local, redirects and DNS changes fail closed |
| Fanout | Failing one receiver does not suppress another receiver or corrupt Git projection outcomes |
| Recovery | Retry is bounded and durable; dead-letter replay requires runtime.manage and preserves event identity |

## Channel Conformance

The same principal, credential scope, consumer binding, released snapshot and semantic
input must produce equivalent digests, refusal codes and authorization outcomes through
actual REST, MCP, CLI and TypeScript SDK entry points. Canonical application-service
unit tests alone do not prove transport conformance. Hosted MCP and external consumer
acceptance are separate from local protocol execution.

## Desktop Verification

Verify at 1440x900 and 1024x768, with no mobile scope. Use real persisted integration
state. Check create, rotate, revoke, refresh, permission denial and errors; one-time
secret dismissal, focus trap/return and Escape; clipboard failure must not claim success.
Screenshots must not retain an actual credential or signing secret.

## Release Boundary

T006 may reach Needs_Review only after implementation and review proof. No task or
Full-Menu Beta release is Accepted by the orchestrator. External receiver, OIDC/HTTPS,
live embedding quality and T007 execution gaps remain explicit.

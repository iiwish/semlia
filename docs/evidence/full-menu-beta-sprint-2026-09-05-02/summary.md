# Full-Menu Beta Sprint 2

Status: Complete for the authorized local sprint. FMB-T006 is Needs_Review, not Accepted.

Budget window: 2026-09-05 04:49:54-07:49:54 UTC
(12:49:54-15:49:54 Asia/Shanghai). All final gates verified by 06:58:53 UTC,
approximately 2 hours 9 minutes after the start and within the three-hour limit.

## Delivered

FMB-T006 provides accountable machine identities, explicit grants and verifier-only
credentials; consistent released-semantic REST/MCP/CLI/TypeScript SDK operations;
durable signed webhooks with bounded retries, lease fencing and SSRF protections;
and real Integration Settings. Migration20 is clean in the normal local preview.

Runtime review closes workspace grant ID normalization, readiness/capability alignment,
stale prototype disclosure, checkbox layout and immediate-secret-dismissal focus return.
No T007 execution implementation, migration21, commit or push is included.

## Verification

| Gate | Result |
| --- | --- |
| Complete source gate | PASS; all Go packages, 180 Web tests, SDK, lint/typecheck, contract/sqlc/embedded drift and build |
| Uncached PostgreSQL suite | PASS, 34.972s; lifecycle/concurrency/audit/scope/channel/receiver/retry/migration proof |
| Real Docker desktop journeys | 2/2 PASS, 5.8s at 1440x900 and 1024x768 |
| Complete fixture browser regression | 26/26 PASS, 30.6s |
| Normal stack smoke | PASS, 18.281s |
| Security gate | PASS; configured HIGH/CRITICAL dependency/image findings zero, no secret finding |
| Root visual and keyboard inspection | PASS for inspected desktop states; screenshots exclude one-time secrets |

Detailed results, raw GREEN/RED logs and review are in
[FMB-T006](../FMB-T006/summary.md). Earlier interrupted or failed runs remain explicit
and are not substituted for final passing evidence.

## Handoff

Preview: http://127.0.0.1:18081/

The frozen sprint-relative patch covers 69 implementation, contract, operation-document
and generated-artifact paths, excluding governance and evidence. SHA-256:
`d5bcfd0df7b77beb9e770fb29cf5dac9d4d4c8fe34e6b9d65a1c9b887dc1152f`.
Its manifest records exact before/after hashes. The initial machine verifier test,
created just before baseline capture, is treated as sprint-new; the two approved
local-UAT files use the prior frozen T005 state as their baseline.

Production image scanned:
`sha256:0025fc330787a2a454cda0a54b09b6a0767ab7c25b43b59929b289241a29703f`.
Normal local-UAT preview image:
`sha256:ae5191d2b1a0e0ef570196088af6cb9f0cfdb1561b58c1d36d8737b24ced71e2`.
Baseline HEAD remains `source-revision-redacted`; prior dirty work and
frozen T004/T005 patches are preserved. No unrelated container or data volume is pruned.

## Remaining Beta Work

T001-T006 have local Needs_Review evidence. T007 remains Draft: bounded read-only
PostgreSQL execution plus hosted, recovery, scale/performance and final release gates.
T005 still requires live-provider/recall-quality/scale evidence. Public HTTPS webhook
receiver and third-party hosted MCP/SDK acceptance are unproven; local real receiver
and actual client-process tests do not stand in for those deployment checks.

Process-local quotas require an upstream gateway for multi-replica aggregate limits.
Zero-grace rotation and cancellation of old-version queued webhooks are explicit Beta
policies. Three existing lint warnings and the existing large-bundle advisory remain.
Only founder confirmation can mark the Beta release Accepted.

# FMB-T007 A002 Sprint Handoff

Status: Incomplete. Full-Menu Beta is not Accepted.
Window: 2026-09-05 10:45:36-13:45:36 UTC. No further implementation expansion.

## Delivered

Real governed publication, immutable physical snapshots, typed formal joins and
exact-decimal persisted-plan execution work through REST, MCP, CLI, TypeScript SDK
and Ask. Actual PostgreSQL proofs cover current authorization, dedicated read-only
role, bounded execution, cancellation, concurrent idempotency, process loss and
metadata-only recovery. Independent source review closes its P1/P2 findings.

Root final single-command source gate passes, including all tests, drift checks and
build; the already-running verification finishes after the implementation window.
Fixture desktop26 and live
distribution desktop2 pass. Security dependency/secret and production-image scans
have zero HIGH/CRITICAL findings. Migration20-to-21 preview upgrade, readiness and
smoke pass while preserving user volumes and a private pre-upgrade backup archive.

The actual darwin-arm64 candidate release passes full Go/Web SBOM completeness,
official CycloneDX validation, strict archive/provenance and checksum verification.
Its real executable reports the candidate version. It is explicitly a dirty-source,
unreviewed local candidate, not a signed or accepted production release.

## Remaining Work

1. Fix compact-desktop member active-state contrast: Axe reports4.31:1, required4.5:1.
   Auth regression is5/6, not a passing suite. Do not disable this assertion.
2. Repeat the green full source chain and affected desktop tests after the UI fix.
3. Regenerate embed and rebuild/rescan the exact final reviewed artifact; the verified
   tar predates the last test-only fixture updates and must retain its own source digest.
4. Obtain approved hosted HTTPS/OIDC/live providers/source/webhook/OTel/recovery inputs.
   Deterministic Chat and isolated synthetic PostgreSQL are not external acceptance.

One historical publication HTTP422 remains preserved without a conclusive cause;
subsequent repeated execution and root integration runs pass. Timing failures and
earlier release failures remain in their original logs, not replaced by later passes.

## Evidence And Safety

See `test-results.md`, `review.md`, `hosted-matrix.md`, `implementation.md` and
`sprint-diff.patch` with `source-manifest.json`. Six existing browser files lack an
exact preflight baseline because of the capture glob; current copies are explicitly
in `current-source-supplement/`, not falsely marked new in the attempt diff.

T004, T005, T006 and A001 frozen patch hashes match their preflight values. No commit,
push, reset, external deployment or destructive volume action is performed. The local
development preview remains at http://127.0.0.1:18081 with explicit UAT disclosure.
The goal remains unfinished rather than marked complete merely because time expires.

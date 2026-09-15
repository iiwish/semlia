# FMB-T006 Review

Status: Local review complete; Needs_Review for founder acceptance.

Final verdict: no unresolved implementation blocker in the inspected T006 scope.
Spec, code, security/privacy and local QA passes are complete. Hosted/public receiver
acceptance and the T007 release gate remain outside this conclusion.

## Early Credential Pass

An independent read-only reviewer inspected the emerging credential, authorization and
distribution implementation. This is not final signoff.

- Provisional P2: stored plan-building refusals with no candidate IDs can bypass current
  asset reauthorization on known-ID reads or idempotent replay. Missing binding, join
  and grain outcomes need persisted asset provenance or a fail-closed fallback.
- Root transport check: MCP must use a server-derived trace ID, not an optional or
  forged `X-Trace-ID` request header.
- Root contract check: shared response projection must keep lower-camel definition
  fields; direct serialization of untagged domain structs changes the REST contract.

All three items were relayed to the sole implementation worker. Fixes and regression
tests are pending review.

Reviewer command passed:
`go test -p 1 ./internal/application/identity ./internal/application/authorization ./internal/application/distribution`.

The reviewer found no confirmed P1 in the inspected token entropy/verifier, context
derivation or atomic rotation/revoke paths. Real PostgreSQL lifecycle, concurrency,
mixed-auth and credential-narrowed replay proof remains required.

## Early Webhook Pass

- Root payload check: committed outbox payloads contain IDs inside `data`; extracting
  top-level IDs creates empty external payloads. Require versioned nested parsing.
- Independent provisional P2: an expired/reclaimed delivery lease returns a conflict
  that terminates the webhook loop and all worker lanes. A stale attempt should be
  abandoned without stopping unrelated processing.
- Independent provisional P2: claiming an old delivery replaces its pinned versions
  and endpoint with current subscription configuration, silently retargeting historical
  events. Preserve configuration provenance or explicitly cancel stale-version work.

All findings were relayed for fixes and regression tests. The reviewer found no concrete
SSRF bypass in checked-address pinning, proxy/redirect denial, TLS or timeout behavior.
Signing secrets use authenticated encryption bound to workspace, subscription and version.
Fanout receipts and individual deliveries commit transactionally.

Serial webhook transport and jobs unit tests passed. The webhook application did not
yet have its own test file at this early checkpoint. Real receiver, lease recovery and
rotation/backlog evidence is pending.

## Fix Verification

Independent source re-review closes refusal provenance, stale-lease loop termination and
old-version retargeting. Focused refusal, stale-attempt continuation, DNS-rebinding,
pinned-dial, redirect and mixed-DNS unit tests pass. Production constructs outbound
clients without test-network overrides.

The final source pass identified a credential-denial audit short-circuit. Production
distribution decisions route through the shared evaluator after the fix; a new real
PostgreSQL denial-audit assertion awaits the final database run.

The canonical Beta rotation policy is zero grace, with original credential expiry
preserved and old-version webhook backlog cancelled. Specification, TDR and data model
state that boundary directly. Nonzero grace is not a supported request option.

## Runtime RED Signals

- The first normal Docker upgrade reached clean migration 20 but readiness still
  required 19. The version check and focused readiness tests are corrected.
- Both real desktop journeys found local-author's create-machine control disabled.
  The bounded local-UAT capability fixture includes member.manage and role.assign;
  reviewer/publisher separation and production gating remain unchanged.
- A rebuild caught type errors in newly added UI tests; typecheck passes after repair.
- During Docker contention, root's full Web suite returned 175/177 with an existing
  ingestion timeout and source-focus timing assertion. The authoritative serial rerun
  passes all 177 tests across 21 files in 25.89 seconds without changing those tests.
- The next real browser journey reaches machine creation but its explicit grant
  returns HTTP400: the UI sends a workspace TypeID while the authorization service
  expects its UUID. Boundary compatibility and regression proof are pending.
- Root desktop inspection found a stale prototype notice above the live integration
  view and oversized, misaligned form checkboxes. These are bounded UI corrections,
  not evidence of simulated persistence; final screenshot verification is pending.

No unrelated containers or data volumes were stopped, removed or pruned. Root paused
only this preview's server/worker during its image rebuild to reduce polling load.

## Final Source Closures

The independent reviewer confirms the audited production evaluator path, one/two
distinct known credential actions, and canonical zero-grace rotation contract.
No production blocker remains in those three inspected changes.

The new rotate/revoke concurrency test must allow both commands to succeed when
rotation wins first, because revocation is intentionally idempotent. Only competing
rotations require exactly one winner. The corrected test passes against real
PostgreSQL; production revocation semantics are retained.

Final read-only review confirms workspace TypeID normalization preserves the
application's exact route-workspace comparison. Six HTTP cases accept the current
workspace's TypeID/UUID and reject foreign, malformed and wrong-prefix inputs without
persisting a binding. No remaining scope bypass or production blocker was identified
in this bounded review.

## Source Gate

`GOFLAGS=-p=1 make check-source` passes all eight gates: format, lint, typecheck,
tests, contract drift, sqlc drift, embedded-web drift and release build. Web tests are
180/180 across 21 files. A final uncached database package run passes in 34.972 seconds,
including denial audit, action/release restrictions and concurrent credential cases.
Three pre-existing lint warnings and the existing large-bundle advisory remain.

Two additional validation REDs are preserved. Root initially overlapped embedded-web
generation with Go compilation, producing obsolete asset-path build errors; the
complete gate was rerun with stable generated files. The next run exposed the new
binding-suspension fixture missing the database-required version increment. Only that
fixture was corrected; the full uncontended rerun passes. See `check-source.log` and
the separate RED logs.

## Runtime Follow-Up

Normal Docker readiness is healthy at migration20. Final `make smoke` passes in 18.281
seconds. `make security-check` passes the configured HIGH/CRITICAL dependency and
production-image gates, with no secret finding. The scanned production image is
`sha256:0025fc330787a2a454cda0a54b09b6a0767ab7c25b43b59929b289241a29703f`.

Root CUA inspection verifies the obsolete connected prototype notice is absent,
compact-desktop checkboxes are correctly sized/aligned, and an unsubmitted dialog
traps focus and restores its trigger without overflow. Actual browser credential
issuance succeeds after correcting the test's implicit-label locator and waiting for
dialog animation before geometry measurement.

The real issuance journey exposed pending-refresh focus loss when its invoker was
temporarily disabled. A deferred, one-close-only restoration fixes this without delaying
secret clearing or stealing focus from a user-selected control. Focused regression and
both actual desktop journeys pass with the original focus assertion retained.

Broad fixture E2E passes 26/26. Legacy cross-flow assertions for simulated clients are
replaced with truthful unavailable/disabled assertions; role inspection, assignment
conflict and separation-of-duties assertions remain intact. The final regression run
uses a sprint-specific screenshot directory.

Real Docker E2E passes 2/2 in 5.8 seconds, including credential issuance/rotation/revoke,
next-request token behavior, webhook edit/filter/disable/signing rotation and persistent
reload. The checkbox test waits for the authoritative PATCH rather than demanding
optimistic state. Secrets are absent after dismissal; the live browser test avoids
secret traces and cleans up its credentials/subscriptions.

The 69-path sprint-relative snapshot is frozen and unchanged after final gates. Prior
T004/T005 patch hashes and baseline HEAD are unchanged. No commit/push is made.

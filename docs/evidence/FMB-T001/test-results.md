# FMB-T001 Test Results

Date: 2026-09-05  
Final attempt: FMB-T001-A003

## TDD Record

- A001 RED established the missing persisted authorization administration contract and fixture-only
  Access Control behavior before the first implementation.
- A002 frontend RED produced four failures for inspect-only/assign-only capability partition and
  one-sided 403 handling; GREEN closed them.
- A003 frontend RED produced two failures for manage-only navigation and access; GREEN closed them.
- A002/A003 backend counterexample tests were added for role-version propagation and regrant,
  invitation policy bypass, authorization TOCTOU, final-admin concurrency, downgrade semantics,
  policy-denial audit, role-action immutability, role-update SoD/human-only, identity decision-version
  races and logical-expiry regrant. The worker reported the new tests failing before the matching
  implementation; raw RED console output was not retained.

## Focused And Full Gates

Passed:

- `go test ./internal/domain/authorization/... ./internal/application/authorization/... ./internal/platform/http/...`
- `go test ./tests/integration/authorization/... ./tests/integration/db/... ./tests/integration/identity/...`
- `pnpm --dir web test -- AccessControlView authorization sessionRuntime ProductApp`
- full Web suite: 13 files, 96 tests
- `pnpm --dir web lint` with zero errors; one existing Fast Refresh warning in `KnowledgeViews.tsx`
- `pnpm --dir web typecheck`
- `make contracts-check`
- `make db-generate-check`
- `make check-source`
- `git diff --check`
- `make dev`
- `make smoke`: `github.com/iiwish/semlia/tests/smoke` passed in 24.335 seconds

The final `make check-source` passed Go tests, Web tests, contract drift, migration drift, Web embed
drift and release build. Its log is `/tmp/semlia-fmb-t001-a003-check-source.log` for the current local
execution environment.

## Migration And Security Scenarios

Passed integration coverage includes:

- empty migration through version 16 and populated 15 -> 16 -> 15 -> 16 lifecycle
- temporary/revoked/expired binding rollback semantics
- immutable custom-role version promotion with affected-binding SoD and human-only revalidation
- revoke/expiry followed by regrant, including logically expired invitation acceptance
- grantor ceiling and authorization-decision version conflict
- invitation creation/acceptance policy and member suspension TOCTOU
- final active administrator protection under concurrent member/binding writes
- immutable attributable policy-denial audit without invitation email/token payloads
- system/custom `role_actions` write guards
- cross-workspace non-disclosure and production no-fixture fallback

## Desktop Checks

Playwright captured the Access Control role surface at 1440x900 and 1024x768 after the disclosure and
content were visible. Manual image inspection found no overlap, clipping or blank primary surface.
Keyboard/focus and reduced-motion behavior remain covered by the focused React/Playwright suite.


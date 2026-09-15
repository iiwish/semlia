# FMB-T001 Delivery Evidence

Status: Needs_Review candidate  
Attempt: FMB-T001-A003  
Date: 2026-09-05

## Outcome

OpenAPI `0.9.0` / contract `9`, migration `000016`, server-authoritative role administration,
immutable custom-role versions, assignment lifecycle, effective-access inspection and the real Web
Access Control runtime are implemented. Production API failures do not fall back to fixtures.

The authorization and identity writers share the workspace authorization-version boundary. Role
updates revalidate affected grants, invitation and member-state writes revalidate their original
decisions in the transaction, final-administrator protection is serialized, and logically expired
bindings cannot cause a false successful grant.

## Scope And Preservation

The worktree already contained approved Alpha and M3 changes. The implementation retained that
baseline and did not revert `.omo/` or unrelated discovery, governance and distribution work.
Task ownership covered these files or generated groups:

- `api/openapi/semlia.v1.yaml`, generated Go/TypeScript contracts and TypeScript client exports
- `migrations/000016_fmb_authorization_admin.{up,down}.sql`, `db/sqlc.yaml`, authorization and
  identity queries, generated sqlc files
- authorization domain/application/PostgreSQL code and tests
- identity invitation, grant and member-state application/PostgreSQL code and tests
- authorization/identity HTTP routing, composition and readiness version 16
- `AccessControlView`, authorization admin/runtime, session and `ProductApp` integration/tests
- synchronized embedded Web assets
- authorization, identity, database and contract integration tests
- the corrected Alpha route disclosure inventory and this evidence directory

The complete task receipt is indexed in `diff.patch`; it is intentionally not a repository-wide raw
patch because the baseline was already dirty and a raw patch would falsely attribute approved Alpha
and M3 work to FMB-T001.

## Review

Independent review ran after each implementation attempt. A001 and A002 found authorization-policy,
concurrency, downgrade and capability-partition counterexamples. A003 closed all P0/P1 findings.

Final independent result:

- P0/P1: none; suitable for `Needs_Review`
- P2: manage-only users see an honest `role.read` dependency state rather than blind role creation
- P3: a dedicated UpdateRole-ceiling test would improve naming clarity; general ceiling and the
  transaction revalidation paths are covered

## Visual Proof

- `screenshots/desktop-access-control.png` at 1440x900
- `screenshots/compact-desktop-access-control.png` at 1024x768

The screenshots use `VITE_CATALOG_FIXTURE=1` and visibly disclose the fixture boundary. They prove
desktop layout, focusable navigation and non-overlap only. Real persistence, authorization and
failure behavior are proven by HTTP/PostgreSQL tests and the running-stack smoke test, not by fixture
data.

## Residual Risk

- Authorization list pagination and general mutation idempotency remain shared Beta contract work
  and must be closed by FMB-T007 before release.
- Principal labels/pickers, scope display labels and richer inspection explanation are usability
  enhancements; raw stable IDs are shown instead of fabricated summaries.
- The Web bundle remains above Vite's 500 kB warning threshold. This is non-blocking for T001 but is
  tracked for the final performance gate.


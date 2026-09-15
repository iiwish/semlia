# Controlled-User Alpha T002 Evidence

## Status

`Needs_Review`. The authenticated Web shell and member administration implementation are complete.
The approved sequence continues with T003; the broader Alpha is not yet ready for external users.

## Delivered

- One generated, session-aware browser client sends same-origin cookies, retains the CSRF verifier
  only in memory, injects it on unsafe requests, and emits centralized `401` and `403` lifecycle
  events.
- A single React session runtime owns initial authentication, account and membership state,
  workspace selection, capability projection, authorization refresh and logout.
- The production Web entry renders explicit loading, sign-in, callback/provider failure,
  no-membership, expired-session and dependency-error states without mounting fixture data.
- Browser OIDC callback failures return to the application error state for HTML navigation while
  preserving the existing JSON error contract for API clients.
- The accepted primary rail, secondary context menu and desktop workspace remain present after
  authentication. The top bar adds compact account and logout controls beside the membership-
  filtered workspace selector.
- Settings member administration uses generated APIs for member and invitation listing, invitation
  creation, suspension, reactivation and revocation. `403` capability changes and `409` final-admin
  rejection stay visible without applying an optimistic state change.
- Fixture preview and explicit development-only local UAT remain isolated and regression-tested.

## Review

Specification review: pass for the T002 functional and UI boundaries. Session and CSRF verifiers
are not rendered or persisted in browser storage. Unknown server role IDs fail closed in the client
capability projection; the server remains authoritative for every command.

Design review: pass at 1440x900 and 1024x768. The sign-in gate is intentionally quiet rather than a
marketing page, the existing navigation is preserved, member rows do not overflow, keyboard focus
is visible, and scoped axe checks report no violations.

## Residual Risk

The browser two-principal check uses real generated request handling with Playwright network
fixtures so it can deterministically exercise administrator and auditor states. The lower layers
are covered by T001 real PostgreSQL and local standards-compliant OIDC integration tests. A hosted
identity-provider tenant and fully live invited-user browser journey remain T007 acceptance work.

The production bundle reports the pre-existing single-chunk size warning (about 702 kB minified).
It does not block the controlled Alpha but remains a performance follow-up after the core vertical
paths are complete.

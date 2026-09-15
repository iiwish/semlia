# Controlled-User Alpha 0.1.0 Delivery Summary

## Recommendation

`Ready for founder review and local controlled-user rehearsal.`

T001 through T007 are implemented and have reproducible evidence. The local stack exercises real
PostgreSQL persistence, read-only source discovery, candidate conversion, independent governance,
immutable release resolution and configured-model Ask without fixture fallback. The stack is ready
at `http://127.0.0.1:18081/` on schema `0.8.0` and migration `15`.

External invited-user testing remains conditional on founder-supplied deployment inputs: one real
OIDC issuer/client configuration, one live OpenAI-compatible provider credential and a reachable
host with TLS. Local-UAT identities are development-only and are not external authentication.

## Delivered Alpha

- Standards-based OIDC Authorization Code plus PKCE, hashed opaque sessions, CSRF/Origin checks,
  controlled admission, membership filtering, revocation and final-administrator protection.
- Read-only PostgreSQL source creation, credential encryption and rotation, connection tests,
  leased discovery, persisted findings/candidates and candidate-to-proposal conversion.
- Independent author, reviewer and publisher governance with validation, policy decisions,
  immutable releases, reload persistence and rollback as a new release.
- Current, explicit and pinned SemanticQuery resolution with immutable plans, stable digests,
  authorization-safe ambiguity handling and attributable refusals.
- Real Ask transport through a configured OpenAI-compatible endpoint, hash-only run attribution,
  released definitions and plans, explicit `not_configured` execution and no fixture fallback.
- Complete five-module desktop navigation with visible Prototype/session-only disclosure on
  out-of-scope surfaces.

## Closeout Decision

Alpha T007 is `Needs_Review`; founder acceptance is intentionally separate. M2/T011 remains
`Running` because its contract requires an external authentication run and live provider evidence.
Full M3 MCP, CLI, webhook and execution-adapter work remains deferred and is not part of Alpha
0.1.0.

The remaining non-blocking engineering risk is the Vite production bundle size advisory. It does
not affect correctness or the current desktop acceptance path, but code splitting should be planned
before broader distribution.

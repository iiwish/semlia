# Controlled-User Alpha T001 Test Results

## Final Validation

| Command | Result | Evidence |
| --- | --- | --- |
| `go test ./internal/domain/identity/... ./internal/application/identity/...` | Pass | One-time OIDC transaction, nonce rejection, digested session/CSRF storage, logout revocation and idempotent bootstrap behavior passed. |
| `go test ./internal/adapters/oidc/...` | Pass | Local discovery, JWKS signature, issuer/audience, Authorization Code exchange and PKCE S256 contract passed. |
| `go test ./internal/platform/config/... ./internal/platform/http/...` | Pass | Production config, 401/403, Origin/CSRF, cross-workspace denial, session projection and spoofed-principal rejection passed. |
| `go test -count=1 ./tests/integration/identity/...` | Pass | Real PostgreSQL bootstrap, invited admission, restart persistence, callback replay, final-admin guard, audit redaction and revocation passed. |
| `go test -count=1 ./tests/integration/db/...` | Pass | PostgreSQL 18 and 17 migration 13 empty and populated up/down/up lifecycle passed. |
| `make contracts-check` | Pass | OpenAPI 0.5.0 and generated Go/TypeScript contracts are current. |
| `make check-source` | Pass | Format, vet, lint, typecheck, all Go/Web tests, contract/sqlc/embed drift and build passed. |
| `make check-smoke` | Pass | Full Compose migration 13, server/worker readiness, persistence and recovery smoke passed. |
| `git diff --check` | Pass | No whitespace errors. |

## Validation Note

The first full `make check-source` attempt hit the existing five-second acceptance-test timing
budget at 5.79 seconds while all integration suites ran concurrently. The exact failing test passed
alone in 4.68 seconds, and the complete `make check-source` rerun passed, including the same
acceptance suite in 132.14 seconds. This was treated as a transient load-sensitive test event, not
silently omitted.

## Behavioral Locks

- Invalid, expired or replayed state and mismatched nonce never create a session.
- Session and CSRF plaintext are never persisted; the PKCE verifier is encrypted with associated
  state data and removed by one-time consumption.
- A valid session survives service restart, while logout and account/session revocation deny the
  next request.
- Suspended or revoked memberships disappear from the next session membership projection; a
  workspace route without an active matching membership returns `403`.
- Session mode rejects principal-header-only requests with `401` and uses the membership principal
  even when a spoofed header accompanies a valid cookie.
- Unsafe requests require both an exact allowlisted Origin and session-bound CSRF verifier.
- The last active membership-backed workspace administrator cannot be suspended or revoked.

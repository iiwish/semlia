# Controlled-User Alpha 0.1.0 Acceptance Matrix

| Criterion | Result | Evidence and boundary |
| --- | --- | --- |
| AC-001 | Pass | HTTP/session tests prove `401`, `403`, cross-workspace denial and ignored spoofed principal headers in session mode. |
| AC-002 | Pass | Identity service and PostgreSQL integration prove logout, revocation, membership suspension and restart-stable denial. |
| AC-003 | Pass | Ciphertext/AAD tests, API/log/audit scans and the security gate prove secrets and verifiers are not exposed. |
| AC-004 | Pass | Live Playwright configures and tests a real read-only PostgreSQL source, runs discovery, reloads and inspects persisted candidates at both viewports. |
| AC-005 | Conditional | Independent local author/reviewer/publisher identities and server-side separation-of-duty gates pass. A hosted multi-user OIDC browser run requires the founder's issuer and deployment. |
| AC-006 | Pass | Current, explicit and binding-selected resolution reads immutable release snapshots and returns validated plans with stable digests. |
| AC-007 | Pass | No-match, ambiguity, invalid semantics, stale binding, missing join/grain and unauthorized cases return stable refusals without silent selection. |
| AC-008 | Pass | Local configured-provider Ask success and provider-503 failure render real state, attribution and no fallback at both viewports. A production provider account remains an external acceptance input. |
| AC-009 | Pass | Route inventory, component tests and full browser regression prove visible Prototype/session-only boundaries without persisted-success claims. |
| AC-010 | Pass | Populated migrations, restart/recovery, retry, smoke, security, release, checksum, SBOM and image checks pass. |

## Review Boundary

The automated and local-live acceptance surface is complete. The two conditional external inputs do
not weaken the local Alpha implementation: production/session mode remains fail-closed until valid
OIDC configuration exists, and Ask reports a configuration/provider error until a live credential
is supplied. Founder acceptance should follow one hosted rehearsal with those inputs.

# Controlled-User Alpha T001 Evidence

## Status

`Needs_Review`. T001 implementation and validation are complete; founder acceptance remains a
separate verdict and does not pause the approved T002-T007 execution sequence.

## Delivered

- Generic OIDC discovery, Authorization Code exchange, PKCE S256, ID-token verification, state,
  nonce and one-time callback enforcement.
- Global accounts and issuer/subject external identities mapped to active workspace memberships and
  the existing workspace-scoped authorization principals. Verified email is invitation matching
  input only and cannot remap an established identity.
- Opaque PostgreSQL sessions with token and CSRF digests, idle and absolute expiry, immediate
  revocation and active-membership reload on every request.
- Session-mode HTTP authentication, exact Origin and CSRF enforcement for unsafe methods, secure
  `__Host-` production cookie, development-only alternate cookie, and request-context principal
  authority. `X-Semlia-Principal` is removed and ignored in session mode.
- Controlled invitations, workspace member administration, authorization-version invalidation,
  immutable audit facts, cross-workspace denial and transactionally serialized final-administrator
  protection.
- Idempotent `bootstrap-admin` command for the first workspace invitation, OIDC/production config
  fail-closed validation, migration 13, generated SQL/OpenAPI/Go/TypeScript contracts and readiness
  version advancement.

## Review

Specification compliance review: pass for T001 requirements and forbidden changes. Provider,
session and invitation secrets are absent from API bodies, logs and audit payloads. Existing local
UAT remains explicitly development-only.

Bug and quality review: pass after adding workspace-level serialization to prevent concurrent final
administrator removal, transactionally coupling invitation/admission audit writes, filtering the
workspace list by active session memberships, and exercising a real local OIDC issuer contract.

## Residual Risk

No hosted identity provider credential was supplied, so evidence uses a standards-compliant local
issuer rather than a third-party tenant. Provider-specific tenant configuration and invited-user UX
belong to T002/T007 acceptance. T001 does not claim the broader Alpha is ready.

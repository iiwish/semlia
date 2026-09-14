# Controlled-User Alpha Requirements Checklist

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Completed |
| Source spec | `docs/specs/alpha-controlled-user/spec.md` 0.1.0 Confirmed |
| Last updated | 2026-09-04 |

## Checklist Scope

This checklist tests whether the controlled-user Alpha requirement contract is complete, clear,
consistent and measurable enough for founder review. It does not assert implementation status.

## Requirement Quality Checks

- [x] The target users, controlled cohort size and self-hosted Alpha boundary are explicit. [Clarity]
- [x] The end-to-end journey covers authentication, source ingestion, governance and consumption.
  [Completeness]
- [x] Authentication, authorization, membership and development-only local identities have separate
  authority boundaries. [Consistency]
- [x] Sign-in, callback, session expiry, logout, revocation and suspension states are defined.
  [Coverage]
- [x] CSRF, cookie, token entropy, secret redaction and fail-closed requirements are present.
  [Security]
- [x] Source setup defines create, test, rotation, disablement, timeout and failure behavior.
  [Coverage]
- [x] Discovery defines asynchronous states, idempotency expectations and degraded/failure behavior.
  [Edge Cases]
- [x] The requirement contract excludes collection and persistence of customer fact rows. [Privacy]
- [x] SemanticQuery success and refusal outcomes are separately specified. [Completeness]
- [x] Immutable release binding and rolled-back or pinned release behavior are defined. [Consistency]
- [x] Ask failure explicitly forbids fixture fallback and false success. [Reliability]
- [x] Every `FR-*` maps to at least one measurable acceptance or success criterion. [Traceability]
- [x] Performance bounds separate local deterministic latency from source, OIDC and model networks.
  [Testability]
- [x] Desktop viewport, keyboard, focus and reduced-motion requirements match project instructions.
  [Accessibility]
- [x] Migration, restart, retry, recovery, security and release evidence are included. [Operations]
- [x] M4, M5, mobile, arbitrary SQL execution and complete ecosystem work are explicit Non-Goals.
  [Scope]
- [x] The 4 to 7 day estimate names the dependencies and constraints that make it credible.
  [Ambiguity]
- [x] External OIDC credentials and a read-only PostgreSQL sample are identified as acceptance inputs.
  [Integration]

## Findings Summary

| Severity | Count | Summary |
| --- | ---: | --- |
| Critical | 0 | No constitution, security or authority contradiction. |
| High | 0 | No missing requirement prevents safe technical planning after founder approval. |
| Medium | 2 | Hosted acceptance depends on OIDC credentials and a representative read-only PostgreSQL source. |
| Low | 1 | Complete MCP, CLI, webhook and external Agent acceptance remains outside this Alpha slice. |

## Resolution Notes

- The OIDC implementation remains provider-neutral and is testable with a local standards-compliant
  issuer; founder-supplied provider credentials are needed only for hosted acceptance.
- The source contract uses one PostgreSQL-compatible read-only connection and preserves the existing
  M1 adapter boundary; additional connectors remain outside the sprint.
- The headless distribution slice includes REST SemanticQuery and Ask because these prove the user
  journey. MCP, CLI and broader ecosystem work remain later milestones.

## User Review Gate

The founder approved `spec.md` 0.1.0 on 2026-09-04. Technical decisions, plan and work graph proceed
to their independent review gate before execution packets and implementation.

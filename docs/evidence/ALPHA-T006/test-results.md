# ALPHA-T006 Test Results

Validation date: 2026-09-04

## Passed Gates

- `make check-source`: formatting, vet, workspace lint, type checks, Go/Web tests, OpenAPI drift,
  sqlc drift, embedded Web drift and release build passed after synchronizing the updated Web bundle.
- `make contracts-check`: generated Go and TypeScript artifacts match OpenAPI `0.8.0`, contract
  version `8`.
- `git diff --check`: passed.
- Focused Go Ask, distribution, HTTP and contract packages passed.
- TypeScript SDK and Web type checks passed.
- Web suite passed with 9 files and 74 tests; the final focused Ask suite passed 5 tests.
- Focused Playwright regression passed 6 tests across `1440x900` and `1024x768` on an isolated
  port, including the disabled fixture Ask boundary.

## Application And Contract Evidence

- Valid provider output becomes a server-owned SemanticQuery and uses the shared immutable-release
  resolver; released definition summaries come from pinned release revisions.
- Clarification completes an attributable run without calling the resolver.
- Invalid JSON, unknown fields, raw SQL, credential-shaped fields and invalid semantic input fail
  closed without a plan.
- Authorization occurs before provider work; provider failure records a failed hash-only agent run.
- OpenAPI validates Ask request and response shape and rejects raw-question response fields.
- React tests cover released definition and plan rendering, `not_configured`, clarification,
  ambiguity refusal, provider failure, missing default model and absence of fabricated values.

## Live Runtime Evidence

- The Docker runtime reported ready at `http://127.0.0.1:18081/` with schema `0.8.0`.
- A deterministic local OpenAI-compatible provider returned one schema-constrained interpretation.
  Ask resolved release `rls_01m1nsfj04ewqvs9ens43g7a36`, rendered its immutable definition and
  reported `executionStatus=not_configured`.
- A provider HTTP 503 produced `PROVIDER_UNAVAILABLE`, a visible `未回退` state and a persisted failed
  agent run containing only hashes and attribution.
- Persistence scans for both live questions returned `false` for `agent_runs`, `agent_steps`,
  `semantic_queries`, `audit_events`, `usage_events` and `outbox_events`.
- The temporary provider and model were removed, the stub was stopped, and the server was recreated
  without its test credential. The final readiness probe passed.

## Visual Evidence

- `screenshots/desktop-resolved-plan.png`
- `screenshots/desktop-provider-refusal.png`
- `screenshots/compact-desktop-resolved-plan.png`
- `screenshots/compact-desktop-provider-refusal.png`

Full command output is retained locally in `/tmp/semlia-t006-*.log` for this execution session.

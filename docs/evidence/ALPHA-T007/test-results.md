# Controlled-User Alpha 0.1.0 Test Results

Validation date: 2026-09-04

## Release Gates

| Gate | Result | Evidence |
| --- | --- | --- |
| `make check-source` | Pass | Formatting, vet, lint, type checks, Go/Web/SDK tests, contract/sqlc/migration/embed drift and release build passed. |
| `make check-smoke` | Pass | Compose migration, API/Web readiness, worker restart, PostgreSQL interruption/recovery and persisted-state recovery passed. |
| `make security-check` | Pass | Dependency and secret findings: 0; Debian runtime and Go binary vulnerabilities: 0. |
| `make release` | Pass | Darwin arm64 archive, CycloneDX SBOM and checksums generated and verified. |
| Cross compilation | Pass | CGO-free release packaging passed for Linux amd64/arm64 and Darwin amd64/arm64. |
| Populated migrations | Pass | PostgreSQL 18 and supported PostgreSQL 17 up/down/up suites passed through migration 15. |
| `git diff --check` | Pass | No whitespace errors. |

## Functional And Browser Evidence

- Auth/session Playwright: 6/6 passed at `1440x900` and `1024x768`, including sign-in, two session
  projections, member invitation and CSRF logout behavior.
- Live-stack Playwright: 4/4 passed at both viewports. The browser configured a real read-only
  PostgreSQL source, ran discovery, created a proposal, completed independent review/publish,
  reloaded persisted state and rolled the release back.
- Deterministic product Playwright: 26/26 passed at both viewports, including keyboard navigation,
  complete menu visibility, prototype boundaries, governed flow and status/error surfaces.
- Focused identity, source, governance, distribution and Ask suites passed. Current/pinned release
  selection, rollback, stale binding, ambiguity, provider failure, session restart/revocation,
  credential rotation and elevated-source refusal are covered.
- Session validation on a PostgreSQL reference stack containing 10,000 assets: 25 samples,
  p50 `656.875us`, p95 `1.400541ms`, budget `<1s`.
- Deterministic resolution with 10,000 released assets: 25 samples, p50 `863.541us`,
  p95 `2.33775ms`, budget `<1s`.

## Runtime Proof

- Current local image: `sha256:414dae922852e43c2f879e75f1f9a7f0e97dcd8119606606a49c0e0d967512bd`.
- Security-scanned image: `sha256:c14c8d9caf61158b2eac702d61c822663ebb1833e6c41af8f38f6b48d6c93ab8`.
- Runtime contract: API `v1`, schema `0.8.0`, migration `15`, dirty `false`.
- Desktop live release/rollback: `rls_01m1p85wr8fd2rjvm6w6xj84nj` ->
  `rls_01m1p85x5sfd883rxpj7f2zjzv`.
- Compact-desktop live release/rollback: `rls_01m1p862ekfdzadzpvw13ttmzw` ->
  `rls_01m1p862w4fe4s56bef6dxj4nw`.
- Ask query `smq_01m1p4vy0zef7amedn0c2c1v9t` resolved release
  `rls_01m1nsfj04ewqvs9ens43g7a36` to plan `rsp_01m1p4vy13ef79wzm2fxqy9exe`
  with digest `sha256:2ca1a60515ef8d762a8d16ba6b8db522c4332dbf9ddf2f76da83291c68e04650`.
- Runtime readiness trace IDs are recorded only in redacted form, for example `d25069a2...`.

Long command output is retained locally under `/tmp/semlia-t007-*.log` for this execution session.

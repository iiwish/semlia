# ALPHA-T004 Test Results

Validation date: 2026-09-04

## Passed Gates

- `pnpm --filter @semlia/web test`: 8 test files and 69 tests passed.
- `pnpm --filter @semlia/web typecheck`: passed.
- `pnpm --filter @semlia/web lint`: passed.
- `make check-source`: formatting, vet, workspace lint, TypeScript, Go/Web tests, OpenAPI drift,
  migration/sqlc drift, embedded Web drift and release build passed.
- `git diff --check`: passed.

## Focused Component Evidence

Seven live-source component tests cover API loss without fixture fallback, password non-echo,
empty credential rotation, failed connection feedback, degraded run detail, successful real
candidate-to-proposal conversion, proposal failure retaining a pending candidate and honest
Prototype automation feedback.

## Live Browser Evidence

`pnpm --filter @semlia/web exec playwright test --config playwright.live.config.ts` passed four
tests across desktop and compact-desktop projects:

- The existing three-person governance journey remained functional across reloads at both
  viewports.
- A real restricted PostgreSQL source was created through the UI, connection-tested, discovered and
  converted from an exact persisted candidate into a governed M2 proposal at both viewports.
- The journey asserted that fixture source data and the submitted password were absent from the DOM,
  all five product navigation areas remained present, and the page had no horizontal overflow.
- Automated axe scans reported zero serious or critical findings on the source and proposal views.

## Artifacts

- `screenshots/desktop-source-workspace.png`
- `screenshots/compact-desktop-source-workspace.png`
- `screenshots/desktop-source-proposal.png`
- `screenshots/compact-desktop-source-proposal.png`
- Full command logs are retained locally in `/tmp/semlia-t004-*.log` for this execution session.

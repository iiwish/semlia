# T005 Test Results

## RED

The first browser run produced 9 passes and 7 failures. The new access-control journey passed at desktop but exposed a compact-dialog closing issue. Existing journeys also contained stale navigation, compact member-column and ambiguous close-button assertions.

## GREEN

The next run produced 10 passes and 6 failures. Both access-control viewport journeys passed, including screenshots and overflow assertions. The remaining failures were isolated to three existing cross-product journeys: downstream-run navigation, vector rebuild progress projection and an ambiguous audit-dialog close button.

After correcting those exercised-path contracts, the complete Playwright suite passed:

`pnpm --filter @semlia/product-prototype test:e2e -- --grep "database-to-answer|system settings|audit and runtime"`

- 16 passed across desktop and compact-desktop.
- The package script forwarded an extra separator, so Playwright ran the full suite rather than only the requested grep subset.

## REFACTOR

- `pnpm --filter @semlia/product-prototype lint`: passed.
- `pnpm --filter @semlia/product-prototype typecheck`: passed.
- `pnpm --filter @semlia/product-prototype test`: passed, 2 files and 35 tests.
- `pnpm --filter @semlia/product-prototype test:e2e --grep "access control remains explainable"`: passed, 2 viewport tests after role-editor bounds and permission annotations were added.
- `pnpm --filter @semlia/product-prototype test:e2e`: passed, all 16 desktop and compact-desktop journeys.
- `pnpm --filter @semlia/product-prototype build`: passed; Vite emitted a non-blocking chunk-size warning for the 570.53 kB minified JavaScript bundle.
- `git diff --check`: passed.

## Viewports

- Desktop: 1440x900.
- Compact desktop: 1024x768.
- Mobile is outside the approved product surface.

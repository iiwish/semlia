# T004 Test Results

## RED

Focused tests failed because machine client expiry, scoped permissions, revocation and protected-release identity controls did not exist.

## GREEN

`pnpm --filter @semlia/product-prototype exec vitest run src/App.test.tsx -t "scopes machine clients|blocks protected publish conflicts"`

Passed: 2 focused tests.

## REFACTOR

- `pnpm --filter @semlia/product-prototype exec vitest run src/App.test.tsx`: passed, 31 tests.
- `pnpm --filter @semlia/product-prototype typecheck`: passed.
- `pnpm --filter @semlia/product-prototype lint`: passed.

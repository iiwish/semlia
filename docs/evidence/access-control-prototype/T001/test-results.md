# T001 Test Results

## RED

`pnpm --filter @semlia/product-prototype test -- src/authorization.test.tsx`

Failed as expected because `src/authorization.tsx` did not exist. The command also ran the existing App suite because of the package script argument shape; the existing 25 component tests passed.

## GREEN

`pnpm --filter @semlia/product-prototype exec vitest run src/authorization.test.tsx`

Passed: 1 file, 4 tests.

## REFACTOR

- `pnpm --filter @semlia/product-prototype typecheck`: passed.
- `pnpm --filter @semlia/product-prototype lint`: passed with no warnings.

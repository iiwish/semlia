# T003 Test Results

## RED

Three focused tests failed because the assignment and inspector tabs were placeholders and member rows did not expose authorization state.

## GREEN

The focused assignment-success, SoD-conflict and effective-access tests pass.

## REFACTOR

- `pnpm --filter @semlia/product-prototype exec vitest run src/App.test.tsx`: passed, 29 tests.
- `pnpm --filter @semlia/product-prototype typecheck`: passed.
- `pnpm --filter @semlia/product-prototype lint`: passed.

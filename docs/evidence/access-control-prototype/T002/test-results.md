# T002 Test Results

## RED

The focused `inspects roles` test failed because `访问控制` did not exist in system settings.

The approved refinement produced a second RED result because permission annotations, `基于此角色创建` and the governed custom-role editor did not exist.

## GREEN

`pnpm --filter @semlia/product-prototype exec vitest run src/App.test.tsx -t "inspects roles"`

Passed: 1 focused test, including annotated permission inspection, derived-role creation and custom-role editing.

## REFACTOR

- `pnpm --filter @semlia/product-prototype test`: passed, 2 files and 35 tests.
- `pnpm --filter @semlia/product-prototype test:e2e`: passed, 16 tests across both supported viewports.
- `pnpm --filter @semlia/product-prototype typecheck`: passed.
- `pnpm --filter @semlia/product-prototype lint`: passed.
- `pnpm --filter @semlia/product-prototype build`: passed with the recorded non-blocking bundle-size warning.

# T004 Test Results

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | T004 |
| Attempt | M0-T004-A001 |
| Date | 2026-08-10 |
| Result | Pass |

## 1. TDD Evidence

### RED

Command:

```bash
pnpm --filter @semlia/web test
```

Result: Expected failure, exit 1. Vitest could not resolve `web/src/App.tsx` after the four-state tests were added first.

### GREEN

Command:

```bash
pnpm --filter @semlia/web test
```

Result: Pass, exit 0. One test file and five component tests passed for loading, ready, dependency unavailable, configuration error and refresh.

### REFACTOR

Commands:

```bash
pnpm --filter @semlia/web lint
pnpm --filter @semlia/web typecheck
pnpm --filter @semlia/web build
pnpm --filter @semlia/web test:e2e
```

Result: Final pass. The first Playwright run found an incorrect request-count assertion under React Strict Mode (6 passed, 2 failed). The assertion was corrected to compare one refresh against the observed initial request count; the final run passed all 8 cases without changing product behavior.

## 2. Full Validation

| Command | Result |
| --- | --- |
| `pnpm install --frozen-lockfile` | Pass |
| `pnpm --filter @semlia/web lint` | Pass |
| `pnpm --filter @semlia/web typecheck` | Pass |
| `pnpm --filter @semlia/web test` | Pass, 5 tests |
| `pnpm --filter @semlia/web build` | Pass, 207.49 kB JS / 6.61 kB CSS before gzip |
| `pnpm --filter @semlia/web test:e2e` | Pass, 8 tests across 2 desktop projects |
| `pnpm --filter @semlia/sdk-typescript test` | Pass |
| `make contracts-check` | Pass |
| `git diff --check` | Pass |

## 3. Browser And Accessibility Checks

| Check | Result |
| --- | --- |
| 1440x900 ready, dependency and configuration states | Pass |
| 1024x768 ready, dependency and configuration states | Pass |
| Horizontal overflow and clipped trace ID | Pass |
| Semantic heading, `status` and `alert` regions | Pass |
| Keyboard focus and Enter refresh | Pass |
| Reduced-motion spinner and transition suppression | Pass |
| Real T003 backend through Vite proxy | Pass; truthful 503 dependency state |

## 4. Result

All validation commands and visual checks required by packet `M0-T004-A001` pass. T004 is ready for founder review.

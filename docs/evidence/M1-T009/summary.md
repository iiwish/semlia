# M1 T009 Evidence Summary

## Outcome

Semlia has one production frontend package: `@semlia/web`. It preserves the accepted desktop
product experience at `/`, keeps system diagnostics at `/status`, and removes the duplicate
`prototypes/product` package and its run targets.

The workspace selector, semantic asset search and type filters, asset creation, detail loading,
revisions, evidence and relations use generated M1 API contracts and PostgreSQL. API failures are
shown as failures and never replaced with fixture data. Product areas whose backend milestones are
not implemented remain explicitly marked as preview, simulation or session-only behavior.

## Runtime Proof

The local Compose stack served the embedded application at `http://127.0.0.1:18081/`. The live
Semantic Core workspace returned five PostgreSQL-backed assets. Opening `Net revenue` loaded
revision `@1`, its summary and current revision reference from the API. Browser console errors and
warnings were empty, horizontal overflow checks were clean, and `/status` remained available.

## Validation

- `pnpm --filter @semlia/web lint`: passed.
- `pnpm --filter @semlia/web typecheck`: passed.
- `pnpm --filter @semlia/web test`: passed, 41 tests.
- `pnpm --filter @semlia/web build`: passed.
- `pnpm --filter @semlia/web test:e2e`: passed at 1440x900 and 1024x768.
- `make check-source`: passed, including Go tests, acceptance suites, contract and migration drift,
  embedded Web drift, and release build.
- `make check-smoke`: passed against the Compose stack.
- Desktop browser QA: passed at normal and compact desktop sizes with no incoherent overlap or
  horizontal overflow.
- `git diff --check`: passed.

## Review Boundary

T009 proves the visible M1 semantic Catalog path and single-frontend architecture. Ask, release,
ingestion, advanced governance and administration remain product-complete previews until their
corresponding backend milestones are implemented; their UI does not claim durable persistence.

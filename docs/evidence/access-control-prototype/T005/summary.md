# T005 Evidence Summary

## Result

The phase-one access-control prototype is complete for the supported desktop surface. Immutable system roles, governed derived/custom-role editing, annotated permission configuration, scoped assignment, effective-access inspection, capability-gated navigation, member authorization projection, machine-client permissions, expiry and revocation, and protected-release separation of duties are covered by component and browser journeys at 1440x900 and 1024x768.

The global runtime handoff also preserves vector-index item progress, and the long governance journey explicitly returns from the downstream run center before continuing to candidate release.

## Changed Files

- `prototypes/product/e2e/product-journey.spec.ts`
- `prototypes/product/src/App.test.tsx`
- `prototypes/product/src/App.tsx`
- `prototypes/product/src/AccessControlView.tsx`
- `prototypes/product/src/AuditRuntimeView.tsx`
- `prototypes/product/src/IntegrationSettingsView.tsx`
- `prototypes/product/src/authorization.tsx`
- `prototypes/product/src/data.ts`
- `prototypes/product/src/styles.css`
- `prototypes/product/src/types.ts`
- `docs/evidence/access-control-prototype/T005/**`

## Review

- Spec compliance: pass. All five approved stories and their allow, deny, conflict and inspection paths are represented.
- Bug and code quality: pass. Frontend guards use stable action identifiers; role names remain presentation data. Server-side enforcement is not implied.
- QA acceptance: pass. Lint, typecheck, 35 component/unit tests, production build and 16 Playwright journeys pass.
- Visual acceptance: pass. All 14 screenshots were inspected; no incoherent overlap or unsupported mobile treatment was found. The role editor remains fully bounded at both viewports, and automated horizontal-overflow checks pass.

## Residual Risk

- The Vite build reports one JavaScript chunk above 500 kB. This is a performance follow-up, not a phase-one functional blocker.
- Authorization state is mock session data. Casbin policy storage, server enforcement, audit persistence, group expansion and final-admin transactional protection remain backend work.
- ABAC is intentionally deferred. The action and resource contracts leave room for later contextual conditions without coupling the frontend to an ABAC expression language.

## Acceptance State

T001 through T005 are `Accepted` for integration into `main` as of 2026-09-01.

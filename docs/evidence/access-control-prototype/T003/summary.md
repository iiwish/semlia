# T003 Evidence Summary

## Result

Access control now includes scoped role assignments, two-step preview, blocking separation-of-duty feedback, session authorization-version updates and an effective-access inspector. The member directory preserves business titles while projecting authorization roles separately, including an explicit suspended state.

## Changed Files

- `prototypes/product/src/App.tsx`
- `prototypes/product/src/AccessControlView.tsx`
- `prototypes/product/src/styles.css`
- `prototypes/product/src/App.test.tsx`

## Review

- Spec compliance: pass for success, conflict, allow, deny and member projection flows.
- Bug and code quality: pass. Decision output exposes reason, role, binding, scope and authorization version.
- QA acceptance: 29 App component tests, typecheck and lint pass.

## Residual Risk

Group expansion and final-admin mutation enforcement are represented at the product-contract level but require backend Casbin policy and transactional enforcement. The frontend is intentionally not a security boundary.

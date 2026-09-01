# T004 Evidence Summary

## Result

Machine clients now receive explicit action permissions, expiry and assignment source; expired and revoked clients remain visually and textually distinct. Protected review and publish dialogs expose actor identity, independent-role requirements and a selectable conflict state that blocks publication.

## Changed Files

- `prototypes/product/src/IntegrationSettingsView.tsx`
- `prototypes/product/src/App.tsx`
- `prototypes/product/src/styles.css`
- `prototypes/product/src/App.test.tsx`

## Review

- Spec compliance: pass for minimum client permissions, expiry, revocation and protected-release SoD.
- Bug and code quality: pass. Existing independent publisher path remains operable.
- QA acceptance: 31 App component tests, typecheck and lint pass.

## Residual Risk

Credential revocation and release authorization are prototype state only. Production enforcement requires server-side Casbin decisions and audit persistence.

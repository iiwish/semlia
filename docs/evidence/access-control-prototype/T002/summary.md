# T002 Evidence Summary

## Result

System settings exposes `访问控制` immediately after `成员`. The default surface is a dense role catalog with system/custom role state, assignment counts, SoD indicators and a grouped permission matrix. System roles are immutable and expose `基于此角色创建`; derived and custom roles can be edited only through an annotated permission editor with diff, assignment impact and high-risk review.

All 27 permission points have concise product-behavior annotations in role detail and permission configuration.

## Changed Files

- `prototypes/product/src/App.tsx`
- `prototypes/product/src/AccessControlView.tsx`
- `prototypes/product/src/data.ts`
- `prototypes/product/src/types.ts`
- `prototypes/product/src/styles.css`
- `prototypes/product/src/App.test.tsx`

## Review

- Spec compliance: pass for role navigation, inspection, system-role locking, derived-role creation, custom-role editing and permission annotations.
- Bug and code quality: pass. Settings-index callers were migrated to the new five-item order.
- QA acceptance: 35 component/unit tests, 16 desktop browser journeys, typecheck, lint and build pass.

## Residual Risk

The production backend must enforce permission-management ceilings, reauthentication and immutable audit. The current editor remains session-only and cannot modify real authorization state.

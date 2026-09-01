# Semlia Access Control Frontend Prototype Plan

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Confirmed spec | `docs/specs/access-control/product-design.md` 0.1.0 |
| Technology decisions | `docs/specs/access-control/technology-decision-record.md` 0.1.0 |
| Last updated | 2026-09-01 |

## Delivery objective

Extend `prototypes/product` with a complete session-only RBAC administration and permission-aware UX that demonstrates roles, scoped assignments, effective-access explanation, machine permissions, and separation of duties at 1440x900 and 1024x768.

## Scope boundary

Allowed product area:

- `prototypes/product/src/**`
- `prototypes/product/e2e/product-journey.spec.ts`
- Access-control specification and evidence under `docs/specs/access-control/**` and `docs/evidence/access-control-prototype/**`

Excluded:

- Production `web/**`.
- Go backend, migrations, OpenAPI, generated SDKs, and Casbin dependency installation.
- OIDC login, SCIM, ABAC expression editing, warehouse row or column policies.
- Repository-wide refactors and unrelated visual changes.

## Architecture

### Authorization projection

Add `authorization.tsx` with:

- Stable `PermissionAction` union imported from `types.ts`.
- `CapabilityProvider` containing current principal, workspace, authorization version, and session capabilities.
- `useCan(action)` for workspace-level actions.
- `useResourceCan(action, resource)` for resource-specific decisions.
- `RequireCapability` for structural visibility.
- A deterministic decision helper for the effective-access inspector and SoD fixtures.

The provider uses fixture data in `data.ts`; refresh resets state.

### Access-control module

Add `AccessControlView.tsx` containing:

- Role catalog toolbar and role rows.
- Role detail dialog with grouped permissions, scope policy, assignments, incompatible roles, and history.
- Annotated permission points that explain the product behavior represented by each stable action identifier.
- Derived/custom-role editor with metadata, permission selection, diff preview, assignment impact and high-risk permission review.
- Assignment list with principal filters, scope, expiry, source and access state.
- Assignment dialog with principal, role, scope, effective period, preview and conflict state.
- Effective-access inspector with principal, action, resource, context, decision explanation and authorization version.

### Existing-surface integration

- `App.tsx`: add the settings entry, preserve active state and command navigation, enrich member directory with authorization projection, and route to the access-control view.
- `IntegrationSettingsView.tsx`: add explicit client capabilities, expiry, revocation and assignment source to client creation/detail flows.
- Change and release views: add the current actor and SoD gate explanation to protected review/publish actions, plus an access-control deep link where the current navigation model supports it.
- `styles.css`: add desktop and compact-desktop layouts using current tokens, radii, density and focus patterns.

## State model

Prototype state includes:

- Current principal and authorization version.
- System roles and custom-role copies.
- Principal-role-scope assignments.
- API client permissions and expiry.
- Effective-access decision fixtures.
- Session-created assignments and custom roles.
- SoD conflicts and resolved preview state.

Every session mutation increments a display authorization version and produces a prototype audit/toast message. Refresh restores fixtures.

## UI behavior

- `角色与权限` is the access-control default tab.
- Role rows are full-width and support keyboard selection.
- System roles show a locked state and allow `基于此角色创建`; they never expose direct permission editing.
- Derived and custom roles can be edited after an explicit permission-diff and risk review.
- Role detail groups permissions by product domain and hides empty groups.
- Assignment creation uses a two-stage dialog: configure, then review effective changes.
- Conflicts block confirmation and name the violated SoD constraint.
- Effective-access checks are explicit commands and do not run on every form change.
- List empty states keep filter reset visible.
- Restricted actions use hidden versus disabled behavior defined by the spec.

## Testing strategy

### Component and unit tests

- Capability checks use action identifiers and react to provider updates.
- Role catalog, tabs, system-role detail and custom-role copy are operable.
- Assignment preview, successful session assignment and SoD conflict are covered.
- Effective-access allow and deny explanations are covered.
- Member roles and client scopes are visible without replacing business titles.
- Protected release actions expose SoD state.

### Browser tests

- Desktop and compact-desktop paths cover settings navigation, role inspection, assignment, conflict, effective-access inspection, client scope, and release SoD.
- No page-level horizontal overflow or clipped access-control controls.
- Keyboard focus enters dialogs, Escape closes them, and focus returns to the invoker.
- Existing core journey remains operable.

### Validation commands

```text
pnpm --filter @semlia/product-prototype lint
pnpm --filter @semlia/product-prototype typecheck
pnpm --filter @semlia/product-prototype test
pnpm --filter @semlia/product-prototype build
pnpm --filter @semlia/product-prototype test:e2e
```

## Visual QA

- Capture `角色与权限`, role detail, role assignment review, SoD conflict, effective-access result, member projection, and client scope at 1440x900.
- Capture the role catalog, assignment dialog, and member projection at 1024x768.
- Inspect every saved image and reject blank, clipped, loading, or wrong-state evidence.
- Confirm document overflow, dense-table wrapping, button text fit, status labels, and dialog viewport fit.

## Evidence contract

Evidence lives under `docs/evidence/access-control-prototype/` and records:

- Changed files and diff summary.
- RED and GREEN command results for each task.
- Final lint, typecheck, component test, build and Playwright results.
- Accepted desktop and compact-desktop screenshots.
- Known residual risks and the production-backend boundary.

## Delivery sequence

1. Authorization types, fixtures, capability context and focused tests.
2. Access-control settings navigation, role catalog and role detail.
3. Scoped assignment workflow, member projection and effective-access inspector.
4. Client scope and separation-of-duties integration.
5. Full regression, visual QA, evidence and review.

## Completion gate

The delivery is complete only when all five validation commands pass, screenshots prove the supported viewports, the prototype boundary is visible, no protected action relies only on client-side role names, and review finds no blocking spec or UX issue.

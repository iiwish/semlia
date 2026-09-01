# Semlia Access Control Frontend Work Graph

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Confirmed spec | `docs/specs/access-control/product-design.md` 0.1.0 |
| Plan | `docs/specs/access-control/plan.md` 0.1.0 Confirmed |
| Last updated | 2026-09-01 |

## Epic E001: Workspace access-control prototype

Deliver a permission-aware Semlia prototype with stable capability contracts, access-control administration, scoped assignments, machine access, separation of duties, and desktop evidence.

## Story S001: Authorization projection foundation

### T001: Add typed authorization model and capability projection

Status: Accepted
Priority: P0
Depends on: None
Blocks: T002, T003, T004
Story / Requirement: US-004, FR-003, FR-011, FR-012, NFR-001, NFR-002
Parallel: No
Conflicts with: None

Goal:
Create the stable permission vocabulary, authorization fixtures, capability provider, hooks, deterministic decision helper, and focused unit tests without changing visible navigation.

Allowed files:
- `prototypes/product/src/types.ts`
- `prototypes/product/src/data.ts`
- `prototypes/product/src/authorization.tsx`
- `prototypes/product/src/authorization.test.tsx`

Test targets:
- `prototypes/product/src/authorization.test.tsx`

Deliverables:
- Permission-action types, principals, roles, scopes, bindings and decisions.
- `CapabilityProvider`, `useCan`, `useResourceCan` and `RequireCapability`.
- Deterministic allow, deny and SoD decision fixtures.

Acceptance criteria:
- Tests prove action-based checks, provider updates, resource checks and explainable deny behavior.
- Role display names are not used as guard identifiers.
- No browser storage or remote request is introduced.

Definition of Done:
- Focused tests, typecheck and lint pass.
- Diff remains inside allowed files.

Validation commands:
- `pnpm --filter @semlia/product-prototype test -- src/authorization.test.tsx`
- `pnpm --filter @semlia/product-prototype typecheck`
- `pnpm --filter @semlia/product-prototype lint`

TDD plan:
- RED: add failing provider, hook, resource and decision tests.
- GREEN: implement the minimum typed model and helpers.
- REFACTOR: consolidate permission groups and fixture builders while tests remain green.

Packet path:
- `docs/specs/access-control/packets/T001.yaml`

Evidence required:
- RED and GREEN results, changed files, diff summary and residual risk.

## Story S002: Roles and permissions administration

### T002: Add access-control navigation, role catalog and role detail

Status: Accepted
Priority: P0
Depends on: T001
Blocks: T003, T005
Story / Requirement: US-001, FR-001, FR-002, FR-004, FR-005, NFR-005
Parallel: No
Conflicts with: T003, T004

Goal:
Add `访问控制` to system settings and deliver the default `角色与权限` surface with immutable system-role inspection, annotated permission points and governed custom-role editing.

Allowed files:
- `prototypes/product/src/App.tsx`
- `prototypes/product/src/AccessControlView.tsx`
- `prototypes/product/src/data.ts`
- `prototypes/product/src/types.ts`
- `prototypes/product/src/styles.css`
- `prototypes/product/src/App.test.tsx`

Test targets:
- `prototypes/product/src/App.test.tsx`

Deliverables:
- Stable settings navigation and header state.
- Role catalog with system/custom state, scopes, assignment counts, permission summary and risk.
- Role detail dialog with grouped, annotated permissions, assignments, incompatibilities and history.
- Session-only derived-role creation and custom-role editing with diff, impact and risk review.

Acceptance criteria:
- Access control appears immediately after members.
- Role rows and dialog work by keyboard.
- System roles are visibly locked, derivable and never directly editable.
- Every permission point has a short explanation and custom-role changes are reviewed before saving.
- The page has no horizontal overflow at 1440 and 1024.

Definition of Done:
- RED/GREEN component tests pass.
- Focus and prototype boundary are verified.

Validation commands:
- `pnpm --filter @semlia/product-prototype test -- src/App.test.tsx`
- `pnpm --filter @semlia/product-prototype typecheck`
- `pnpm --filter @semlia/product-prototype lint`

TDD plan:
- RED: add navigation, annotated permission, locked-state, derived-role and custom-edit assertions.
- GREEN: implement the new view, editor and session state.
- REFACTOR: align repeated rows/dialogs with existing settings patterns.

Packet path:
- `docs/specs/access-control/packets/T002.yaml`

Evidence required:
- RED/GREEN results, screenshots at both supported viewports, changed files and residual risk.

## Story S003: Scoped assignments and decision explanation

### T003: Add role assignments, member projection and effective-access inspector

Status: Accepted
Priority: P0
Depends on: T001, T002
Blocks: T004, T005
Story / Requirement: US-001, US-002, US-003, FR-006, FR-007, FR-008, FR-009, NFR-002, NFR-004
Parallel: No
Conflicts with: T002, T004

Goal:
Complete the access-control module with scoped assignments, preview and SoD conflicts; enrich the member directory; and expose explainable effective-access checks.

Allowed files:
- `prototypes/product/src/App.tsx`
- `prototypes/product/src/AccessControlView.tsx`
- `prototypes/product/src/styles.css`
- `prototypes/product/src/App.test.tsx`

Test targets:
- `prototypes/product/src/App.test.tsx`

Deliverables:
- `角色分配` tab with principal, role, scope, expiry, state and source filters.
- Two-stage assignment dialog and session authorization-version update.
- Blocking SoD conflict with recovery guidance.
- `有效权限检查` with allow and deny explanations.
- Member rows with business title preserved and separate authorization projection.

Acceptance criteria:
- Successful assignment remains visible during the session and resets on refresh.
- Conflicted assignment cannot be confirmed.
- Effective-access result names reason, role/binding, scope path and authorization version.
- The final-admin and suspended-member states are represented.

Definition of Done:
- Component tests cover success, conflict, allow, deny and member projection.
- Supported viewports remain stable.

Validation commands:
- `pnpm --filter @semlia/product-prototype test -- src/App.test.tsx`
- `pnpm --filter @semlia/product-prototype typecheck`
- `pnpm --filter @semlia/product-prototype lint`

TDD plan:
- RED: add assignment, conflict, inspector and member-projection tests.
- GREEN: implement state and interactions.
- REFACTOR: extract stable row and decision-summary helpers without expanding scope.

Packet path:
- `docs/specs/access-control/packets/T003.yaml`

Evidence required:
- RED/GREEN results, state screenshots, changed files and residual risk.

## Story S004: Machine scope and protected-release integration

### T004: Add client permissions and separation-of-duties gates

Status: Accepted
Priority: P1
Depends on: T001, T003
Blocks: T005
Story / Requirement: US-003, US-005, FR-007, FR-010, FR-011, FR-013
Parallel: No
Conflicts with: T003

Goal:
Apply the shared permission vocabulary to API clients and make protected review and publish actions explain their separation-of-duties state.

Allowed files:
- `prototypes/product/src/IntegrationSettingsView.tsx`
- `prototypes/product/src/App.tsx`
- `prototypes/product/src/JourneyViews.tsx`
- `prototypes/product/src/styles.css`
- `prototypes/product/src/App.test.tsx`

Test targets:
- `prototypes/product/src/App.test.tsx`

Deliverables:
- Client permission selection grouped by product domain.
- Client expiry, revocation, assignment source and effective permission summary.
- Review and publish surfaces showing current actor, required independent role and blocking conflict.
- Prototype audit/toast feedback for permission-sensitive mutations.

Acceptance criteria:
- A client cannot be configured beyond the mock creator capability set.
- Revoked and expired clients are distinguishable without color alone.
- Protected release conflict blocks the action and links conceptually to effective-access inspection.

Definition of Done:
- Focused component tests, typecheck and lint pass.
- Existing integration and release behavior remains operable.

Validation commands:
- `pnpm --filter @semlia/product-prototype test -- src/App.test.tsx`
- `pnpm --filter @semlia/product-prototype typecheck`
- `pnpm --filter @semlia/product-prototype lint`

TDD plan:
- RED: add client scope, expiry, revocation and protected-release conflict assertions.
- GREEN: implement minimum shared-capability integration.
- REFACTOR: reuse permission grouping and status helpers.

Packet path:
- `docs/specs/access-control/packets/T004.yaml`

Evidence required:
- RED/GREEN results, integration/release screenshots, changed files and residual risk.

## Story S005: Regression and evidence

### T005: Complete desktop QA, regression and delivery evidence

Status: Accepted
Priority: P0
Depends on: T002, T003, T004
Blocks: None
Story / Requirement: US-001, US-002, US-003, US-004, US-005, all acceptance criteria
Parallel: No
Conflicts with: None

Goal:
Add the full access-control browser journey, resolve task-related regressions, capture visual evidence and perform spec, engineering and QA review.

Allowed files:
- `prototypes/product/e2e/product-journey.spec.ts`
- `prototypes/product/src/App.test.tsx`
- `prototypes/product/src/App.tsx`
- `prototypes/product/src/AccessControlView.tsx`
- `prototypes/product/src/IntegrationSettingsView.tsx`
- `prototypes/product/src/JourneyViews.tsx`
- `prototypes/product/src/authorization.tsx`
- `prototypes/product/src/styles.css`
- `docs/evidence/access-control-prototype/**`

Test targets:
- `prototypes/product/e2e/product-journey.spec.ts`
- `prototypes/product/src/App.test.tsx`
- `prototypes/product/src/authorization.test.tsx`

Deliverables:
- Desktop and compact-desktop Playwright journey.
- Accepted screenshots and overflow checks.
- Final evidence summary, test results and review findings.

Acceptance criteria:
- All five plan validation commands pass.
- Role, assignment, conflict, inspector, member, client and release states have inspected evidence.
- No blocking spec, accessibility-risk, layout or regression finding remains.

Definition of Done:
- Fresh full-suite output is recorded.
- Evidence matches the actual diff and residual risk.

Validation commands:
- `pnpm --filter @semlia/product-prototype lint`
- `pnpm --filter @semlia/product-prototype typecheck`
- `pnpm --filter @semlia/product-prototype test`
- `pnpm --filter @semlia/product-prototype build`
- `pnpm --filter @semlia/product-prototype test:e2e`

TDD plan:
- RED: add access-control e2e expectations before final regression fixes.
- GREEN: resolve only failures caused by the approved implementation and existing stale assertions on exercised paths.
- REFACTOR: remove test duplication while preserving behavior coverage.

Packet path:
- `docs/specs/access-control/packets/T005.yaml`

Evidence required:
- Full commands with exit status, screenshot manifest, diff summary, three-layer review and residual risk.

## Approval gate

The user accepted T001 through T005 for integration into `main` on 2026-09-01. The frontend access-control prototype delivery is complete.

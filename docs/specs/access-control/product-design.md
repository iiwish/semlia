# Semlia Access Control Product Design

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Source | User request and `docs/SSOT.md` v0.7.0 |
| Last updated | 2026-09-01 |
| Review gate | User approved first-phase requirements, technical plan and execution on 2026-09-01 |

## Product positioning

Semlia Access Control governs who or what can inspect, change, validate, publish, bind, execute, and administer semantic assets inside a workspace. It combines understandable workspace roles with resource scopes, separation of duties, explainable decisions, scoped machine credentials, and immutable audit evidence.

The first phase establishes RBAC as the stable authorization foundation. Conditional attributes extend role grants in a later phase without replacing the role model or exposing a general-purpose policy language to ordinary administrators.

## Target users

- Platform administrators configure identities, groups, roles, machine clients, and workspace boundaries.
- Security administrators inspect effective access, handle separation-of-duties conflicts, and review authorization events.
- Semantic stewards and asset owners receive scoped authoring and review permissions.
- Reviewers and publishers approve and release changes without receiving unrelated administrative privileges.
- Application engineers create scoped API clients and diagnose denied calls.
- Auditors inspect decisions without gaining mutation permissions.

## User stories

### US-001 Manage workspace access

As a platform administrator, I can assign a principal one or more roles in a clear scope and see the resulting access before saving.

### US-002 Understand effective access

As a security administrator, I can select a principal, action, and resource and see whether access is allowed, which binding produced the result, and which authorization version was evaluated.

### US-003 Enforce separation of duties

As an organization operating protected assets, I can prevent the same principal from being the only proposer, reviewer, and publisher of a protected change.

### US-004 Operate a permission-aware product

As any user, I see only the navigation and commands relevant to my access, while contextual denials explain what is missing and how to recover.

### US-005 Scope machine access

As an application engineer, I can create or update an API client with explicit actions, workspace scope, environment, expiry, and revocation state.

## Core journey

1. An administrator opens `系统设置 / 访问控制`.
2. The administrator reviews system and custom roles, their scope, member count, machine assignments, and risk level.
3. The administrator opens a role to inspect grouped permissions and inherited access.
4. The administrator assigns the role to a user, group, service account, or API client within a workspace, semantic domain, asset, source, environment, release, or consumer scope.
5. Semlia previews added and removed capabilities and checks separation-of-duties conflicts.
6. The administrator confirms the mock or real assignment according to the current product boundary.
7. The assignment receives a new authorization version and an immutable audit event.
8. The administrator uses `有效权限检查` to verify a representative resource decision.

## Functional requirements

### FR-001 Access-control navigation

`系统设置` contains a stable `访问控制` entry immediately after `成员`. The entry is visible only to principals with `role.read`, `role.assign`, `policy.read`, or `authorization.inspect`.

### FR-002 Role catalog

The access-control landing surface lists system and custom roles with role name, role type, allowed scopes, member and machine assignment counts, permission summary, separation-of-duties risk, and state.

### FR-003 Permission vocabulary

Permissions use stable action identifiers grouped by product domain:

- Workspace: `workspace.read`, `workspace.manage`.
- Identity: `member.read`, `member.manage`, `group.manage`.
- Access control: `role.read`, `role.manage`, `role.assign`, `authorization.inspect`.
- Knowledge: `asset.read`, `asset.propose`, `asset.edit`, `evidence.read`.
- Governance: `proposal.review`, `validation.run`, `release.publish`, `release.rollback`.
- Sources: `source.read`, `source.manage`, `ingestion.run`.
- Delivery: `binding.read`, `binding.manage`, `semantic.resolve`, `semantic.execute`.
- Operations: `audit.read`, `runtime.read`, `runtime.manage`.

Role names and navigation labels never become authorization identifiers.

### FR-004 System roles

The first phase provides read-only system roles:

- Workspace Admin.
- Security Admin.
- Semantic Steward.
- Asset Owner.
- Reviewer.
- Publisher.
- Source Operator.
- Consumer Developer.
- Auditor.

System roles cannot be edited or deleted. Administrators create an editable derived custom role from a system role; the derived role retains its base-role identity for inspection and upgrade comparison.

### FR-005 Role detail

Role detail shows grouped allowed actions, scope constraints, inherited permissions, current assignments, incompatible roles, recent changes, and a plain-language summary. Every permission point includes a short product-behavior annotation. Empty permission groups remain hidden.

System-role detail exposes `基于此角色创建` but never an edit command. Derived and custom roles expose an editor with role metadata, annotated permission selection, a permission diff, affected assignment count, and high-risk permission review before saving.

### FR-006 Role assignment

Administrators can assign a role to a user, group, service account, API client, or Agent. Every assignment requires workspace, scope type, scope value, effective time, and optional expiry. The review step previews effective capability changes.

### FR-007 Separation of duties

The first phase blocks these combinations for protected releases:

- A proposal author cannot be the only reviewer.
- A reviewer cannot be the only publisher when the policy requires two-person control.
- A Security Admin cannot approve their own privilege escalation.
- The final Workspace Admin cannot remove or disable their own last administrative binding.

The UI names the conflict, affected scope, policy source, and permitted recovery actions.

### FR-008 Effective-access inspector

Authorized administrators can inspect a principal, action, resource, and context. The result shows allow or deny, reason code, matched role and binding, scope path, authorization version, and missing capability when denied.

### FR-009 Member directory projection

The member directory shows effective workspace roles, scoped assignment count, and access state. Selecting a member opens assignments and recent authorization changes without mixing job title with authorization role.

### FR-010 Machine access

API clients and service accounts use the same permission vocabulary and scope model. Credentials include expiry and revocation. A token cannot gain actions beyond the principal or client binding that created it.

### FR-011 Frontend capability contract

React receives session capabilities and resource-specific allowed actions from the backend. UI guards evaluate actions, not role names. A backend denial always overrides rendered UI state.

### FR-012 Backend authorization contract

Every protected domain read and command performs server-side authorization using principal, workspace, action, resource, scope, and context. The decision includes a stable reason code and authorization version.

### FR-013 Authorization audit

Role changes, assignments, removals, privilege escalation attempts, machine credential changes, policy conflicts, and protected denials produce immutable audit events without exposing secret values.

## Non-functional requirements

### NFR-001 Security

- Authorization is deny-by-default.
- Workspace scoping is enforced in authorization and persistence queries.
- Client-side guards never constitute a security control.
- Authorization caches are keyed by workspace authorization version.
- Revocation and privilege removal invalidate future protected commands without waiting for a time-based cache expiry.
- Secrets and raw tokens never appear in capability responses or audit payloads.

### NFR-002 Explainability

Every administrative authorization check returns a stable decision, reason code, evaluated authorization version, and determining role, binding, or constraint where the caller may inspect that information.

### NFR-003 Performance

- A warm in-process single authorization check has p95 latency no higher than 10 ms under the first benchmark workspace.
- Batch capability evaluation is used for list and navigation surfaces; React does not issue one authorization request per button.
- Role and permission list pages remain usable with 10,000 principals and 1,000 role bindings in a workspace through server pagination and search.

### NFR-004 Reliability

- Role assignment writes are transactional with authorization-version creation and outbox/audit recording.
- Concurrent edits use optimistic concurrency and return a recoverable conflict.
- An unavailable optional external policy adapter fails closed for protected operations.

### NFR-005 Accessibility

- Role lists, permission groups, assignment dialogs, conflicts, and effective-access results support keyboard operation and visible focus.
- Permission state uses label and icon in addition to color.
- The 1440x900 and 1024x768 product viewports have no clipped actions or horizontal page overflow.

### NFR-006 Observability

Authorization denials expose trace ID and reason code. Metrics separate allowed, denied, error, cache hit, cache miss, and evaluation latency without recording sensitive attributes.

## First-phase product surface

The first phase contains:

- `系统设置 / 成员`: roles, assignment count, access state, member access detail.
- `系统设置 / 访问控制 / 角色与权限`: system-role catalog and role detail.
- `系统设置 / 访问控制 / 角色分配`: principal and scoped binding list plus assignment workflow.
- `系统设置 / 访问控制 / 有效权限检查`: explainable decision inspector.
- `系统设置 / 接口与集成`: scoped client permissions, expiry, and revocation.
- Existing change and release flows: separation-of-duties messaging and authorization-aware actions.

## Functional scope

- Workspace-scoped RBAC.
- System roles and custom-role copies.
- User, group, service-account, client, and Agent principals.
- Scoped role bindings.
- Separation-of-duties constraints for protected releases and privilege changes.
- Capability projection for React.
- Server-side command authorization and explainable decisions.
- Authorization-version invalidation and immutable audit.

## Non-goals

- General-purpose ABAC expression authoring UI.
- Warehouse row-level or column-level security implementation.
- Cross-organization role federation.
- SCIM provisioning.
- OpenFGA-style arbitrary relationship tuples.
- User-authored explicit deny policies.
- Automatic role mining or AI-generated privilege grants.

## Edge cases

- The last Workspace Admin attempts to remove or expire their own binding.
- A suspended member retains group-derived assignments.
- An OIDC group changes while the user has an active session.
- A role is changed while a permission-sensitive dialog is open.
- A client token outlives its assignment or principal.
- Two administrators edit the same custom role concurrently.
- A resource moves between semantic domains after a scoped assignment.
- A caller has global navigation access but lacks permission for one resource instance.
- A stale frontend capability set renders an action that the backend denies.
- An unknown action identifier appears after a rolling upgrade.

## Constraints and assumptions

- PostgreSQL remains the authority for identity mappings, roles, bindings, scopes, authorization versions, and audit references.
- Authentication and authorization are separate; OIDC proves identity and does not replace Semlia authorization.
- Data-plane row and column enforcement remains in the configured warehouse or execution adapter.
- The current prototype remains session-only and must label simulated authorization mutations.
- Permission vocabulary is versioned as a public service contract.

## Data and integration needs

- OIDC subject, issuer, email, display name, and group claims mapped to Semlia principals.
- Workspace membership, groups, roles, permissions, bindings, scopes, authorization versions, and separation-of-duties constraints.
- Session capability and resource allowed-action API projections.
- Audit event schemas for role, binding, token, conflict, and decision inspection operations.
- Optional Casbin adapter behind the project-owned authorization interface.
- CEL policy conditions in a later version using a typed environment controlled by Semlia.

## Success criteria

- An administrator can assign a scoped role and verify the resulting effective access without reading policy source code.
- Every protected release demonstrates that unauthorized, self-approved, and insufficient-separation paths are rejected.
- Every frontend command maps to a stable backend action identifier.
- A permission removal prevents the next protected command and refreshes the visible capability state.
- API clients cannot invoke actions or scopes outside their current binding.
- Authorization decisions are attributable to a workspace, principal, action, resource, authorization version, and reason code.

## Acceptance criteria

- The first-phase surfaces are operable at 1440x900 and 1024x768 with no horizontal overflow.
- System roles, permission groups, scoped assignments, SoD conflicts, and effective-access decisions are represented with realistic mock data in the prototype.
- Prototype mutations disclose session-only behavior and reset on refresh.
- Keyboard tests cover access-control navigation, role inspection, assignment review, conflict recovery, and effective-access inspection.
- Backend planning defines the authorization request/decision contract, schema ownership, cache invalidation, audit events, and failure-closed behavior before implementation begins.
- The existing lint, typecheck, component-test, build, and desktop/compact-desktop Playwright suites pass after implementation.

## Clarifications

- RBAC is part of the open-source core and release boundary.
- ABAC enters as conditional constraints after the RBAC vocabulary, scopes, decision contract, and audit model are stable.
- Frontend authorization improves usability; backend authorization is authoritative.

## Open questions

There are no blocking product questions for first-phase technical planning. The product contract was approved by the user on 2026-09-01.

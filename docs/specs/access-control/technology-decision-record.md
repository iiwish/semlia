# Semlia Access Control Technology Decision Record

## Metadata

| Field | Value |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Source | `docs/specs/access-control/product-design.md` 0.1.0 Confirmed |
| Last updated | 2026-09-01 |
| Review gate | User approved the technical plan and execution on 2026-09-01 |

## Decision summary

### TDR-001 Semlia owns the authorization contract

Permission identifiers, scopes, role semantics, authorization versions, reason codes, separation-of-duties constraints, and audit payloads remain Semlia-owned contracts. Third-party engines implement a project-owned `Authorizer` interface and cannot leak engine-specific policy formats into the frontend or public API.

### TDR-002 Casbin is the first backend evaluator adapter

The production backend plan uses Casbin as the first in-process evaluator because domain-scoped RBAC maps to workspaces and avoids adding a mandatory policy service. PostgreSQL remains the authority for principals, roles, permissions, assignments, scopes, and authorization versions.

The frontend prototype does not add the Casbin dependency. It models the future request and decision contract with repository-local fixtures and deterministic session state.

### TDR-003 React consumes capabilities, not roles

The prototype adds a typed authorization projection containing stable permission identifiers, workspace capabilities, resource allowed actions, authorization version, and explainable decisions. Components use `useCan` and resource action checks. Role names appear only as administration content.

### TDR-004 Access control is a dedicated settings responsibility

`系统设置` contains `成员` followed by `访问控制`. Access control contains three task tabs:

1. `角色与权限`.
2. `角色分配`.
3. `有效权限检查`.

Role detail and assignment review use centered dialogs consistent with the existing settings surfaces. The page avoids a permanent nested master-detail card.

### TDR-005 Prototype mutations are session-only

Role assignment, custom-role copy, client scope, and conflict-recovery actions update React session state only. Every mutation dialog and result message discloses that refresh resets the prototype. No browser persistence or remote request is added.

### TDR-006 Separation of duties is deterministic

The prototype uses explicit conflict fixtures and reason codes. It does not ask an LLM to decide authorization. Protected release actions show the current actor role and required independent role, and conflict states provide a direct path to access-control inspection.

### TDR-007 Frontend library choice stays minimal

The first phase implements a small project-local capability context instead of adding CASL. CASL remains an adapter option if later resource-instance and field conditions make the local API insufficient. This avoids adding a second policy language before a real backend capability contract exists.

### TDR-008 System roles are immutable product contracts

System-role identifiers and permission definitions remain version-managed Semlia contracts. Administrators customize access by deriving a custom role, not by mutating a built-in role. Derived roles retain `baseRoleId`, use the same stable permission vocabulary, and receive explicit permission annotations and change review before session persistence.

## Constitution check

| Principle | Result | Evidence |
| --- | --- | --- |
| Preserve product scope at 1024px minimum | Satisfied | Plan validates 1440x900 and 1024x768 only |
| Quiet operational workspace | Satisfied | Tables, tabs, dialogs, status and explicit commands follow existing patterns |
| No nested cards | Satisfied | Repeated roles and assignments use rows; individual summaries may use flat cards |
| Server authorization is authoritative | Satisfied | Frontend contract is explicitly a usability projection |
| Prototype boundary remains visible | Satisfied | Session-only disclosures and reset behavior are required |
| Accessibility and reduced motion | Satisfied | Keyboard, focus, labels, non-color state and reduced-motion tests are planned |
| Preserve user work | Satisfied | Implementation is restricted to the prototype and feature evidence files |

## Alternatives considered

### Use CASL immediately

Rejected for the first prototype phase. CASL improves conditional rendering but does not remove the need for Semlia action identifiers and backend decisions. A small typed capability layer is sufficient for the approved surface and easier to replace.

### Put access control inside the member page

Rejected. Members are identities; roles, machine principals, scopes, effective decisions, and separation of duties form an independent administrative responsibility.

### Use one large permission matrix as the landing page

Rejected. It scales poorly, exposes implementation vocabulary before role intent, and makes common assignment work inefficient. The matrix belongs inside role detail by permission group.

### Prototype ABAC rules now

Rejected. The approved first phase stabilizes RBAC vocabulary, scopes, decisions, and audit. CEL conditions enter after these contracts are proven.

## Risks

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Prototype implies production security | Users may over-trust mock enforcement | Persistent prototype disclosure and session-only mutation copy |
| Role names leak into UI guards | Future custom roles break behavior | Tests require stable action identifiers in guards |
| Access-control page becomes a dense matrix | Poor scanability at 1024px | Role catalog first, grouped permissions in detail dialogs |
| Capability fixtures drift from backend plan | Rework when APIs arrive | Shared request/decision types and documented public action vocabulary |
| Permission changes are hard to understand | Unsafe privilege assignments | Effective-access preview, change summary and conflict reason codes |
| Existing Playwright failures hide regressions | False completion signal | Fix task-related failures and require the full suite to pass |

## Supporting artifacts

- `docs/specs/access-control/research.md`
- `docs/specs/access-control/product-design.md`
- `docs/specs/access-control/plan.md`
- `docs/specs/access-control/tasks.md`
- `docs/specs/access-control/checklists/requirements.md`

## Consequences for tasks

- The prototype receives a new access-control module, typed authorization fixtures, and capability context.
- Existing member, integration, change, and release surfaces receive scoped authorization projections rather than isolated role labels.
- No backend, migration, OpenAPI, Go dependency, or production `web/**` change is part of this delivery.
- Production Casbin integration remains a separately governed backend delivery after the frontend contract is accepted.

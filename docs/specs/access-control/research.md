# Semlia Access Control Reuse Research

## Metadata

| Field | Value |
| --- | --- |
| Status | Ready_For_User_Review |
| Date | 2026-09-01 |
| Scope | RBAC foundation, future ABAC conditions, React capability UX, Go authorization runtime |
| Product baseline | `docs/SSOT.md` v0.7.0 |

## Decision summary

Semlia owns the authorization domain model, permission vocabulary, decision contract, audit semantics, and administration experience. A third-party engine may evaluate policies behind a project-owned interface, but it does not become the product SSOT.

The recommended first implementation uses:

- Production identity: OIDC/OAuth 2.1 through `go-oidc` and `x/oauth2`, matching the current ADR.
- Authorization storage: normalized Semlia PostgreSQL tables for roles, permissions, bindings, principals, groups, scopes, and authorization versions.
- Authorization evaluation: a project-owned `Authorizer` interface with an in-process Casbin adapter evaluated as the first implementation candidate.
- Conditional policy: CEL in a later phase for typed, versioned conditions; CEL is not the role and binding store.
- React: a small typed `CapabilityProvider`, `useCan` hook, route guard, and action guard fed by backend capability responses. CASL remains optional until instance-level UI rules justify it.

## Candidate comparison

| Candidate | Fit | Strengths | Costs and risks | Recommendation |
| --- | --- | --- | --- | --- |
| Casbin | High for phase 1 | Go-native, embedded, domain-scoped RBAC, role hierarchy, conditions, mature management APIs | Generic string policy model, cache invalidation and policy storage still belong to Semlia, limited product administration UX | Use behind `Authorizer` after a focused contract spike |
| Cerbos PDP | Medium | Purpose-built RBAC/ABAC, resource policies, derived roles, stateless PDP, Go client | Adds a PDP deployment boundary; full authoring/distribution experience can introduce product or hosted-control-plane coupling | Keep as an adapter option when external PDP isolation is required |
| OpenFGA | Medium later | Strong relationship-based authorization for owner, member, parent, and inherited resource relationships | Separate service and tuple store; does not replace contextual policy evaluation; excessive for workspace RBAC | Revisit only when relationship scale and inheritance exceed the relational model |
| OPA/Rego | Medium later | General policy engine, rich context, bundles and policy testing | Rego learning and operational surface are larger than the first authorization requirement | Do not use in phase 1 |
| Cedar | Medium later | Authorization-specific PARC model, default deny, explicit forbid, diagnostics | Go integration and operational path are less aligned with the current Go modular monolith | Track, do not adopt in phase 1 |
| CEL | High as an extension | Fast, safe, typed, compile-once/evaluate-many expressions; already aligned with the release-policy ADR | Expression language only; no role, binding, persistence, administration, or relationship engine | Use for phase 2 policy conditions |
| CASL React | Medium | React context, declarative `Can`, reactive ability updates, instance and field checks | Duplicating backend policy rules in the browser creates drift and false security | Optional UI adapter; do not make it the policy source |

## Why Casbin is the first adapter candidate

Casbin supports role hierarchies, domain-scoped RBAC, and conditions. A workspace can map to a Casbin domain so one principal can have different roles in different workspaces. It stays in-process, which matches the modular-monolith boundary and avoids requiring an additional policy service for self-hosted users.

Semlia still stores canonical authorization facts in its own schema. The adapter compiles a workspace authorization version into an evaluator and returns a Semlia decision object. This prevents Casbin policy strings, model configuration, or adapter APIs from leaking into HTTP contracts, audit events, migrations, or React code.

## Frontend authorization principle

Frontend authorization is a usability projection, not a security boundary.

The backend returns coarse session capabilities and resource-specific actions:

```json
{
  "authorizationVersion": "authzv_01...",
  "workspaceId": "workspace_01...",
  "capabilities": [
    "member.read",
    "role.read",
    "role.assign",
    "audit.read"
  ]
}
```

Resource responses expose actions calculated for that resource and current context:

```json
{
  "id": "asset_01...",
  "name": "净收入",
  "allowedActions": ["asset.read", "proposal.create"],
  "authorizationVersion": "authzv_01..."
}
```

React uses stable action identifiers instead of role names:

```ts
const canAssignRole = useCan("role.assign");
const canPublish = useResourceCan(asset.allowedActions, "release.publish");
```

UI behavior:

- Hide navigation and commands that the principal cannot ever use in the current workspace.
- Keep contextually relevant but temporarily blocked actions visible and disabled, with a short reason.
- Treat every `403` as authoritative, show a recoverable message, and refresh capabilities.
- Refresh capabilities after workspace switch, identity refresh, role change, and authorization-version mismatch.
- Never infer permissions from display role names, OIDC groups, client-side route names, or disabled buttons.

## Backend decision contract

Every protected application command evaluates:

```text
principal + workspace + action + resource + scope + context
  -> allow | deny
  -> reason code
  -> matched role/binding/policy version
  -> authorization version
```

The application service calls `Authorizer.Check` before state transition or protected read. Handlers do not contain role-name conditionals. Database queries remain workspace-scoped even after authorization succeeds.

Suggested interface:

```go
type Authorizer interface {
    Check(ctx context.Context, request AuthorizationRequest) (AuthorizationDecision, error)
    CheckMany(ctx context.Context, requests []AuthorizationRequest) ([]AuthorizationDecision, error)
}
```

The first phase uses deny-by-default RBAC, scope inheritance, and separation-of-duties constraints. Explicit policy deny and arbitrary CEL conditions enter after the base decision vocabulary and audit model are stable.

## Source references

- [Casbin RBAC overview](https://casbin.apache.org/docs/rbac-overview/)
- [Casbin RBAC with domains](https://casbin.apache.org/docs/rbac-with-domains/)
- [Cerbos policy model](https://docs.cerbos.dev/cerbos/latest/policies/index.html)
- [OpenFGA authorization concepts](https://openfga.dev/docs/concepts)
- [OPA access-control comparison](https://www.openpolicyagent.org/docs/comparisons/access-control-systems)
- [Cedar authorization model](https://docs.cedarpolicy.com/auth/authorization.html)
- [CEL overview](https://cel.dev/overview/cel-overview)
- [CASL React integration](https://github.com/stalniy/casl/blob/master/packages/casl-react/README.md)

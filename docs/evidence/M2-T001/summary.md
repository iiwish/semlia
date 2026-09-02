# M2 T001 Evidence Summary

## Metadata

| Field | Value |
| --- | --- |
| Task | T001 Authorization foundation (actor identity, RBAC enforcement, authorization audit) |
| Status | `Ready for review` |
| Implementation | Working tree (not committed; left for review per packet) |
| Date | 2026-09-02 |
| Packet | `docs/specs/m2-governed-authoring/packets/T001.yaml` |

## Delivered

- Migration `000005_m2_authorization_foundation` (+ down): `principals` (workspace-scoped, agent principals accountable to a human `owner_principal_id` in the same workspace), `roles`, `role_actions`, `actions`, `role_bindings` (frontend scope types `workspace|domain|asset|source|environment|release|consumer`), and immutable `authorization_events`, per `data-model.md` key shapes.
- Seeded the full FR-003 action vocabulary (27 identifiers grouped by product domain, with a `requires_human` flag on privilege administration and protected release governance) and the nine FR-004 system roles with permission sets mirroring `web/src/data.ts`. Seeding lives in a re-callable `seed_authorization_vocabulary()` SQL function and is proven idempotent.
- `workspaces.authorization_version bigint` (default 1) is the per-workspace authorization version; `BumpWorkspaceAuthorizationVersion` moves it monotonically and is the hook future binding changes must call transactionally (NFR-001 revocation invalidation).
- Domain layer `internal/domain/authorization`: `Principal` (human|agent), `Role`, `RoleBinding`, `Resource`, `Decision`, `DecisionEvent`, `DenialError`, reason codes and scope types matching `web/src/authorization.tsx` / `web/src/types.ts` exactly (`ROLE_GRANT`, `SESSION_CAPABILITY`, `NO_MATCHING_GRANT`, `PRINCIPAL_INACTIVE`, `SEPARATION_OF_DUTY`; no new codes).
- Application layer `internal/application/authorization`: the single capability-evaluation enforcement point (`Evaluator.Evaluate(principal, workspace, action, resource) → decision{allowed, reasonCode, authorizationVersion}`). Deny-by-default; every decision (allow and deny) writes an `authorization_events` row; agent principals are rejected with `SEPARATION_OF_DUTY` on any `requires_human` action they hold a binding for.
- HTTP actor resolution for private incubation: `X-Semlia-Principal` carries the principal's public TypeID (`prn_…`). Protected commands denied with 403 + stable reason code; denials override any client-side capability state (FR-011/FR-012). No storage UUIDs on the wire; new prefixes `prn_` (Principal) and `bnd_` (RoleBinding) registered in `pkg/identity` per ADR-0003.
- Enforcement wired into the two existing protected write commands inside the catalog application service: asset create → `asset.propose` at workspace scope (a new asset proposes new governed content; M1 assets start in draft lifecycle), revision append → `asset.edit` against the asset resource. Mapping documented at both call sites.
- Seeding of default actors: an idempotent trigger plus backfill gives every workspace (existing local ones included) one system-granted human principal holding the workspace-admin binding (`granted_by IS NULL` identifies the default actor). Requests without the principal header act as that seeded admin, so M1 flows and tests pass with zero behavior change; unknown, inactive, or unbound principals are denied.
- sqlc: `db/queries/authorization.sql` + generated `internal/adapters/postgres/sqlc/authorization.sql.go`; repository `internal/adapters/postgres/authorization.go` follows the M1 store style. `DeleteRole` refuses every deletion (roles non-deletable, FR-004) and a database trigger rejects direct SQL `DELETE` as defense in depth. No proposal/validation/release/agent-run tables or endpoints were added; `api/openapi/**` untouched.

## TDD

Red state was recorded before implementation: the new evaluator, HTTP, DB and full-stack test packages failed against the unimplemented foundation (`no required module provides package …/internal/domain/authorization`, then behavioral failures). The four packet red scenarios and their locks:

| Red scenario | Locking tests |
| --- | --- |
| Deny-by-default + stable reason code + authorization event | `internal/application/authorization` `TestDenyByDefaultRecordsEventWithStableReasonCode`; `tests/integration/authorization` `TestProtectedWriteWithoutMatchingGrantIsDeniedAndAudited` (asserts the persisted deny row with `NO_MATCHING_GRANT`, version 1) |
| Workspace scoping: foreign binding never grants | `TestCrossWorkspaceBindingNeverGrantsAccess` (unit, fake repo) and `TestBindingInAnotherWorkspaceNeverGrants` (full stack: ws-A admin acting on ws-B → 403) |
| Agent principals: scoped allow / human-only reject | `TestAgentPrincipalScopedCapabilityAllowsAndHumanOnlyActionRejects` (unit); `TestAgentPrincipalActsWhereAllowedAndIsBlockedFromHumanDuties` (agent + steward binding creates an asset via HTTP → allow event; agent holding publisher binding is denied `release.publish` with `SEPARATION_OF_DUTY`) |
| Role vocabulary: idempotent seed, non-deletable | `TestAuthorizationVocabularySeedsNineSystemRolesIdempotently` (re-invokes the seed function; 27 actions / 9 roles / role-action set unchanged) and `TestSystemRolesCannotBeDeleted` (repository `ErrRolesImmutable` + database rejection) |

Refactor step: capability evaluation lives behind one application-layer interface (`authorizationapp.Evaluator`) consumed by the catalog service via `WithAuthorizer`, so T002+ services reuse the same enforcement point.

## Validation

| Command | Result |
| --- | --- |
| `go test ./...` | Pass — all packages, including fresh `-count=1` runs of `tests/integration/{db,authorization,catalog,usage,discovery,projection,worker,m1}`, `tests/contracts`, `tests/repository` (real PostgreSQL 18 containers; PG17 compatibility suite green) |
| `go vet ./...` | Pass |
| `make contracts-check` | Pass — no contract drift; OpenAPI untouched |
| `pnpm --filter @semlia/web typecheck` | Pass |
| `make db-generate-check` | Pass — sqlc artifacts current with migrations |
| `git diff --check` | Pass |

## Review Boundary

Deviations and decisions for reviewer attention:

1. **Pre-existing `tests/acceptance` compile breakage fixed**: at the base commit `process_group.go` referenced `environmentValue`, defined only in `fresh_clone_test.go`, so `go test ./...` failed before any M2 change. The helpers were relocated verbatim into a new non-test file `tests/acceptance/environment.go`; no test logic changed.
2. **Existing test-expectation updates**: `tests/integration/db/database_test.go` now expects migration version 5, the seven new tables, `workspaces` FKs for `principals`/`authorization_events`, and one extra downgrade step. No assertions were weakened.
3. **Nil evaluator in unit harnesses**: mirroring the optional usage service, `catalogapp.WithAuthorizer(nil)` (the zero value) disables enforcement. Production wiring in `cmd/semlia` always attaches the deny-by-default evaluator; enforcement tests use the real PostgreSQL-backed evaluator.
4. **`actions` table added beyond the data-model table list**: the FR-003 vocabulary and the `requires_human` flag (packet must-do 6) required a home; a normalized `actions` + `role_actions` pair mirrors the `relation_type_policies` seeded-vocabulary precedent. `authorization_events.action` intentionally has no FK so unresolvable-action denials can always be audited.
5. **Deterministic seeding**: `semlia_seed_uuidv7` (md5 entropy, UUIDv7 bits, PG17-safe per ADR-0003) stays as a schema function because the runtime seed trigger resolves it; ids are stable per workspace, making the trigger and the backfill idempotent.

Residual work belongs to T002+: binding/assignment management endpoints with authorization-version bumps, capability and effective-access projections, and proposal/release paths that reuse this evaluator unchanged.

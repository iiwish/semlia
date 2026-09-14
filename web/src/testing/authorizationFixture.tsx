import { AuthorizationAdminApiError, type AuthorizationAdminApi, type AuthorizationMutationResult } from "../authorizationAdmin";
import { evaluateAuthorization, findSeparationOfDutyConflicts } from "../authorization";
import { authorizationBindings, authorizationPrincipals, authorizationRoles, authorizationVersion } from "./data";
import type { AuthorizationBinding, AuthorizationRole } from "../types";

export function createAuthorizationFixtureApi(workspaceId: string): AuthorizationAdminApi {
  let roles: AuthorizationRole[] = authorizationRoles.map((role) => ({ ...role, version: role.version ?? 1, permissions: [...role.permissions] }));
  let bindings: AuthorizationBinding[] = authorizationBindings.map((binding) => ({
    ...binding,
    workspaceId,
    roleVersion: binding.roleVersion ?? 1,
    version: binding.version ?? 1,
    scope: { ...binding.scope },
  }));
  let revision = Number(authorizationVersion.match(/(\d+)$/)?.[1] ?? "1");

  const mutation = <T,>(value: T): AuthorizationMutationResult<T> => {
    revision += 1;
    return { value, authorizationVersion: revision };
  };

  return {
    async listRoles() {
      return roles.map((role) => ({ ...role, permissions: [...role.permissions] }));
    },
    async listBindings() {
      return bindings.map((binding) => ({ ...binding, scope: { ...binding.scope } }));
    },
    async createRole(_workspaceId, input) {
      const role: AuthorizationRole = {
        id: `ROLE-CUSTOM-${Date.now()}`,
        name: input.name,
        description: input.description,
        category: "custom",
        permissions: [...input.permissions],
        version: 1,
        workspaceId,
      };
      roles = [...roles, role];
      return mutation(role);
    },
    async updateRole(_workspaceId, roleId, input) {
      const current = roles.find((role) => role.id === roleId);
      if (!current) throw new AuthorizationAdminApiError("角色不存在。", 404, "NOT_FOUND");
      if (current.category === "system") throw new AuthorizationAdminApiError("系统角色不可修改。", 409, "SYSTEM_ROLE_IMMUTABLE");
      if (current.version !== input.expectedVersion) throw new AuthorizationAdminApiError("角色版本已更新，请刷新后重试。", 409, "VERSION_CONFLICT");
      const role = { ...current, name: input.name, description: input.description, permissions: [...input.permissions], version: input.expectedVersion + 1 };
      roles = roles.map((item) => item.id === roleId ? role : item);
      return mutation(role);
    },
    async createBinding(_workspaceId, input) {
      const role = roles.find((item) => item.id === input.roleId);
      if (!role || role.version !== input.expectedRoleVersion) throw new AuthorizationAdminApiError("角色版本已更新，请刷新后重试。", 409, "VERSION_CONFLICT");
      const scope = { ...input.scope, protected: input.scope.id === "METRIC-NET-REVENUE" || bindings.some(binding => binding.scope.id === input.scope.id && binding.scope.protected) };
      const conflicts = findSeparationOfDutyConflicts({ principalId: input.principalId, roleId: input.roleId, scope, roles, bindings });
      if (conflicts.some((conflict) => conflict.blocking)) throw new AuthorizationAdminApiError(conflicts[0].message, 409, "SEPARATION_OF_DUTY");
      const binding: AuthorizationBinding = {
        id: `BIND-SESSION-${String(bindings.length + 1).padStart(3, "0")}`,
        workspaceId,
        principalId: input.principalId,
        roleId: input.roleId,
        roleVersion: input.expectedRoleVersion,
        scope: { ...input.scope },
        assignedBy: "林悦",
        assignedAt: "2026-09-01 16:20",
        expiresAt: input.expiresAt,
        status: "active",
        version: 1,
      };
      bindings = [...bindings, binding];
      return mutation(binding);
    },
    async revokeBinding(_workspaceId, bindingId, input) {
      const current = bindings.find((binding) => binding.id === bindingId);
      if (!current) throw new AuthorizationAdminApiError("角色分配不存在。", 404, "NOT_FOUND");
      if (current.version !== input.expectedVersion) throw new AuthorizationAdminApiError("角色分配版本已更新，请刷新后重试。", 409, "VERSION_CONFLICT");
      const binding: AuthorizationBinding = { ...current, status: "revoked", version: input.expectedVersion + 1, revokedAt: "2026-09-04T10:00:00Z", revocationReason: input.reason };
      bindings = bindings.map((item) => item.id === bindingId ? binding : item);
      return mutation(binding);
    },
    async inspect(_workspaceId, input) {
      return evaluateAuthorization({
        principalId: input.principalId,
        action: input.action,
        resource: input.resource,
        principals: authorizationPrincipals,
        roles,
        bindings,
        authorizationVersion: `authzv-2026.09.01-${String(revision).padStart(3, "0")}`,
      });
    },
  };
}

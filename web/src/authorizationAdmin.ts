import type { components } from "@semlia/sdk-typescript";

import { apiClient, sessionCSRFToken } from "./apiClient";
import type {
  AuthorizationBinding,
  AuthorizationDecision,
  AuthorizationRole,
  AuthorizationScope,
  PermissionAction,
} from "./types";

type RoleDTO = components["schemas"]["AuthorizationRole"];
type BindingDTO = components["schemas"]["AuthorizationRoleBinding"];
type DecisionDTO = components["schemas"]["AuthorizationDecision"];

export interface CreateAuthorizationRoleInput {
  name: string;
  description: string;
  permissions: PermissionAction[];
}

export interface UpdateAuthorizationRoleInput extends CreateAuthorizationRoleInput {
  expectedVersion: number;
}

export interface CreateAuthorizationBindingInput {
  principalId: string;
  roleId: string;
  expectedRoleVersion: number;
  scope: AuthorizationScope;
  expiresAt?: string;
}

export interface RevokeAuthorizationBindingInput {
  expectedVersion: number;
  reason: string;
}

export interface InspectAuthorizationInput {
  principalId: string;
  action: PermissionAction;
  resource: AuthorizationScope;
}

export interface AuthorizationMutationResult<T> {
  value: T;
  authorizationVersion: number;
}

export interface AuthorizationAdminApi {
  listRoles(workspaceId: string, signal?: AbortSignal): Promise<AuthorizationRole[]>;
  listBindings(workspaceId: string, signal?: AbortSignal): Promise<AuthorizationBinding[]>;
  createRole(workspaceId: string, input: CreateAuthorizationRoleInput): Promise<AuthorizationMutationResult<AuthorizationRole>>;
  updateRole(workspaceId: string, roleId: string, input: UpdateAuthorizationRoleInput): Promise<AuthorizationMutationResult<AuthorizationRole>>;
  createBinding(workspaceId: string, input: CreateAuthorizationBindingInput): Promise<AuthorizationMutationResult<AuthorizationBinding>>;
  revokeBinding(workspaceId: string, bindingId: string, input: RevokeAuthorizationBindingInput): Promise<AuthorizationMutationResult<AuthorizationBinding>>;
  inspect(workspaceId: string, input: InspectAuthorizationInput): Promise<AuthorizationDecision>;
}

export class AuthorizationAdminApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly traceId: string;

  constructor(message: string, status: number, code = "REQUEST_FAILED", traceId = "") {
    super(message);
    this.name = "AuthorizationAdminApiError";
    this.status = status;
    this.code = code;
    this.traceId = traceId;
  }
}

export const authorizationAdminApi: AuthorizationAdminApi = {
  async listRoles(workspaceId, signal) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/authorization/roles", {
      params: { path: { workspaceId } },
      signal,
    });
    if (!result.data) throw authorizationFailure(result.error, result.response.status, "无法读取角色目录。");
    return result.data.items.map(roleFromDTO);
  },

  async listBindings(workspaceId, signal) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/authorization/role-bindings", {
      params: { path: { workspaceId } },
      signal,
    });
    if (!result.data) throw authorizationFailure(result.error, result.response.status, "无法读取角色分配。");
    return result.data.items.map(bindingFromDTO);
  },

  async createRole(workspaceId, input) {
    const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/authorization/roles", {
      params: { path: { workspaceId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
      body: { name: input.name, description: input.description, actions: input.permissions },
    });
    if (!result.data) throw authorizationFailure(result.error, result.response.status, "无法创建自定义角色。");
    return { value: roleFromDTO(result.data.role), authorizationVersion: result.data.authorizationVersion };
  },

  async updateRole(workspaceId, roleId, input) {
    const result = await apiClient.PATCH("/api/v1/workspaces/{workspaceId}/authorization/roles/{roleId}", {
      params: { path: { workspaceId, roleId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
      body: { expectedVersion: input.expectedVersion, name: input.name, description: input.description, actions: input.permissions },
    });
    if (!result.data) throw authorizationFailure(result.error, result.response.status, "无法更新自定义角色。");
    return { value: roleFromDTO(result.data.role), authorizationVersion: result.data.authorizationVersion };
  },

  async createBinding(workspaceId, input) {
    const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/authorization/role-bindings", {
      params: { path: { workspaceId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
      body: {
        principalId: input.principalId,
        roleId: input.roleId,
        expectedRoleVersion: input.expectedRoleVersion,
        scope: scopeToDTO(input.scope),
        expiresAt: input.expiresAt || undefined,
      },
    });
    if (!result.data) throw authorizationFailure(result.error, result.response.status, "无法创建角色分配。");
    return { value: bindingFromDTO(result.data.binding), authorizationVersion: result.data.authorizationVersion };
  },

  async revokeBinding(workspaceId, bindingId, input) {
    const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/authorization/role-bindings/{bindingId}:revoke", {
      params: { path: { workspaceId, bindingId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
      body: input,
    });
    if (!result.data) throw authorizationFailure(result.error, result.response.status, "无法撤销角色分配。");
    return { value: bindingFromDTO(result.data.binding), authorizationVersion: result.data.authorizationVersion };
  },

  async inspect(workspaceId, input) {
    const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/authorization:inspect", {
      params: { path: { workspaceId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
      body: { principalId: input.principalId, action: input.action, resource: scopeToDTO(input.resource) },
    });
    if (!result.data) throw authorizationFailure(result.error, result.response.status, "无法检查有效权限。");
    return decisionFromDTO(result.data, input.resource);
  },
};

function roleFromDTO(role: RoleDTO): AuthorizationRole {
  return {
    id: role.id,
    workspaceId: role.workspaceId,
    name: role.name,
    description: role.description,
    category: role.category,
    permissions: [...role.actions],
    version: role.version,
    createdAt: role.createdAt,
  };
}

function bindingFromDTO(binding: BindingDTO): AuthorizationBinding {
  return {
    id: binding.id,
    workspaceId: binding.workspaceId,
    principalId: binding.principalId,
    roleId: binding.roleId,
    roleVersion: binding.roleVersion,
    scope: scopeFromDTO(binding.scope),
    assignedBy: binding.grantedBy ?? "system",
    assignedAt: binding.grantedAt,
    expiresAt: binding.expiresAt,
    expiredAt: binding.expiredAt,
    revokedAt: binding.revokedAt,
    revokedBy: binding.revokedBy,
    revocationReason: binding.revocationReason,
    status: binding.status,
    version: binding.version,
  };
}

function decisionFromDTO(decision: DecisionDTO, resource: AuthorizationScope): AuthorizationDecision {
  return {
    allowed: decision.allowed,
    action: decision.action,
    principalId: decision.principalId,
    reasonCode: decision.reasonCode,
    explanation: decisionExplanation(decision),
    authorizationVersion: String(decision.authorizationVersion),
    roleId: decision.roleId,
    bindingId: decision.bindingId,
    scope: resource,
  };
}

function scopeFromDTO(scope: components["schemas"]["AuthorizationScope"]): AuthorizationScope {
  return { type: scope.type, id: scope.id, domainId: scope.domainId, label: scope.id };
}

function scopeToDTO(scope: AuthorizationScope): components["schemas"]["AuthorizationScope"] {
  return { type: scope.type, id: scope.id, domainId: scope.domainId };
}

function decisionExplanation(decision: DecisionDTO): string {
  if (decision.allowed) return decision.roleId ? `角色 ${decision.roleId} 通过绑定 ${decision.bindingId ?? "unknown"} 授予此操作。` : "服务端授权检查允许此操作。";
  if (decision.reasonCode === "PRINCIPAL_INACTIVE") return "主体不存在、已停用或已撤销，不能获得权限。";
  if (decision.reasonCode === "SEPARATION_OF_DUTY") return "该操作违反职责分离约束。";
  return `服务端没有找到覆盖当前资源的 ${decision.action} 授权。`;
}

function requiredCSRFToken(): string {
  const token = sessionCSRFToken();
  if (!token) throw new AuthorizationAdminApiError("会话校验信息已失效，请刷新后重试。", 401, "SESSION_EXPIRED");
  return token;
}

function authorizationFailure(value: unknown, status: number, fallback: string): AuthorizationAdminApiError {
  if (value && typeof value === "object") {
    const message = "message" in value && typeof value.message === "string" ? value.message : fallback;
    const code = "code" in value && typeof value.code === "string" ? value.code : "REQUEST_FAILED";
    const traceId = "traceId" in value && typeof value.traceId === "string" ? value.traceId : "";
    return new AuthorizationAdminApiError(message, status, code, traceId);
  }
  return new AuthorizationAdminApiError(fallback, status);
}

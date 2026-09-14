import type { components } from "@semlia/sdk-typescript";

import { apiClient, sessionCSRFToken } from "./apiClient";

export type SessionResponse = components["schemas"]["SessionResponse"];
export type SessionWorkspace = components["schemas"]["SessionWorkspace"];
export type WorkspaceMembership = components["schemas"]["WorkspaceMembership"];
export type WorkspaceInvitation = components["schemas"]["WorkspaceInvitation"];
export type MembershipStatus = components["schemas"]["MembershipStatus"];

export class IdentityApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly traceId: string;

  constructor(message: string, status: number, code = "REQUEST_FAILED", traceId = "") {
    super(message);
    this.name = "IdentityApiError";
    this.status = status;
    this.code = code;
    this.traceId = traceId;
  }
}

export async function getSession(signal?: AbortSignal): Promise<SessionResponse> {
  const result = await apiClient.GET("/api/v1/session", { signal });
  if (!result.data) throw identityFailure(result.error, result.response.status, "无法读取当前会话。");
  return result.data;
}

export async function getAuthMethods(signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/auth/methods", { signal });
  if (!result.data) throw identityFailure(result.error, result.response.status, "无法读取登录方式。");
  return result.data;
}

export async function passwordLogin(username: string, password: string): Promise<void> {
  const result = await apiClient.POST("/api/v1/auth/password/login", { body: { username, password } });
  if (result.response.status !== 204) {
    const message = result.response.status === 429 ? "尝试次数过多，请稍后再试。" : result.response.status === 401 ? "账号或密码不正确。" : "登录未完成，请稍后重试。";
    throw new IdentityApiError(message, result.response.status);
  }
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  const result = await apiClient.POST("/api/v1/session/password", { params: { header: { "X-Semlia-CSRF": requiredCSRFToken() } }, body: { currentPassword, newPassword } });
  if (result.response.status !== 204) throw new IdentityApiError(result.response.status === 401 ? "当前密码不正确或会话已失效。" : result.response.status === 429 ? "尝试次数过多，请稍后再试。" : "密码未修改，请确认新密码不少于 6 个字符且不是常见弱密码。", result.response.status);
}

export async function createPasswordMember(workspaceId: string, input: { username: string; displayName: string; password: string; roleId: string }): Promise<void> {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/members", { params: { path: { workspaceId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } }, body: input });
  if (result.response.status !== 201) throw new IdentityApiError(result.response.status === 409 ? "账号已存在或授权状态发生变化，请刷新后重试。" : "未能创建账号，请检查账号、密码与角色权限。", result.response.status);
}

export async function deleteSession(): Promise<void> {
  const result = await apiClient.DELETE("/api/v1/session", {
    params: { header: { "X-Semlia-CSRF": requiredCSRFToken() } },
  });
  if (result.response.status !== 204) throw identityFailure(result.error, result.response.status, "无法退出当前会话。");
}

export async function listWorkspaceMembers(workspaceId: string, signal?: AbortSignal): Promise<WorkspaceMembership[]> {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/members", {
    params: { path: { workspaceId } },
    signal,
  });
  if (!result.data) throw identityFailure(result.error, result.response.status, "无法读取工作区成员。");
  return result.data.items;
}

export async function listWorkspaceInvitations(workspaceId: string, signal?: AbortSignal): Promise<WorkspaceInvitation[]> {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/invitations", {
    params: { path: { workspaceId } },
    signal,
  });
  if (!result.data) throw identityFailure(result.error, result.response.status, "无法读取待处理邀请。");
  return result.data.items;
}

export async function createWorkspaceInvitation(workspaceId: string, input: { email?: string; subject?: string; roleId: string }): Promise<WorkspaceInvitation> {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/invitations", {
    params: {
      path: { workspaceId },
      header: { "X-Semlia-CSRF": requiredCSRFToken() },
    },
    body: input,
  });
  if (!result.data) throw identityFailure(result.error, result.response.status, "无法创建邀请。");
  return result.data;
}

export async function updateWorkspaceMembership(workspaceId: string, membershipId: string, status: MembershipStatus): Promise<WorkspaceMembership> {
  const result = await apiClient.PATCH("/api/v1/workspaces/{workspaceId}/members/{membershipId}", {
    params: {
      path: { workspaceId, membershipId },
      header: { "X-Semlia-CSRF": requiredCSRFToken() },
    },
    body: { status },
  });
  if (!result.data) throw identityFailure(result.error, result.response.status, "无法更新成员状态。");
  return result.data;
}

function requiredCSRFToken(): string {
  const token = sessionCSRFToken();
  if (!token) throw new IdentityApiError("会话校验信息已失效，请刷新后重试。", 401, "SESSION_EXPIRED");
  return token;
}

function identityFailure(value: unknown, status: number, fallback: string): IdentityApiError {
  if (value && typeof value === "object") {
    const message = "message" in value && typeof value.message === "string" ? value.message : fallback;
    const code = "code" in value && typeof value.code === "string" ? value.code : "REQUEST_FAILED";
    const traceId = "traceId" in value && typeof value.traceId === "string" ? value.traceId : "";
    return new IdentityApiError(message, status, code, traceId);
  }
  return new IdentityApiError(fallback, status);
}

import type { components } from "@semlia/sdk-typescript";

import { apiClient, sessionCSRFToken } from "./apiClient";

export type OperationsAuditEvent = components["schemas"]["OperationsAuditEvent"];
export type OperationsAuditFilter = components["schemas"]["OperationsAuditFilter"];
export type OperationsAuditExport = components["schemas"]["OperationsAuditExport"];
export type OperationsAuditExportContent = components["schemas"]["OperationsAuditExportContent"];
export type OperationsRuntimeRun = components["schemas"]["OperationsRuntimeRun"];
export type OperationsRuntimeRunDetail = components["schemas"]["OperationsRuntimeRunDetail"];
export type OperationsRuntimeRunEvent = components["schemas"]["OperationsRuntimeRunEvent"];
export type OperationsRuntimeRunKind = components["schemas"]["OperationsRuntimeRunKind"];
export type OperationsRuntimeRunState = components["schemas"]["OperationsRuntimeRunState"];
export type OperationsRuntimePolicy = components["schemas"]["OperationsRuntimePolicy"];
export type OperationsRuntimeSettings = components["schemas"]["OperationsRuntimeSettings"];
export type UpdateOperationsRuntimeSettingsInput = Omit<components["schemas"]["UpdateOperationsRuntimeSettingsRequest"], "expectedVersion">;

export interface OperationsPage<T> {
  items: T[];
  limit: number;
  nextCursor?: string;
}

export interface OperationsRunFilter {
  kind?: OperationsRuntimeRunKind;
  state?: OperationsRuntimeRunState;
  sourceType?: string;
  sourceId?: string;
  traceId?: string;
}

export interface OperationsAuditExportDownload {
  content: string;
  contentDigest: string;
  contentType: string;
  filename: string;
}

export interface OperationsApi {
  listAuditEvents(workspaceId: string, filter?: OperationsAuditFilter, cursor?: string, signal?: AbortSignal): Promise<OperationsPage<OperationsAuditEvent>>;
  createAuditExport(workspaceId: string, filter: OperationsAuditFilter, idempotencyKey: string): Promise<OperationsAuditExport>;
  downloadAuditExport(workspaceId: string, exportId: string): Promise<OperationsAuditExportDownload>;
  listRuns(workspaceId: string, filter?: OperationsRunFilter, cursor?: string, signal?: AbortSignal): Promise<OperationsPage<OperationsRuntimeRun>>;
  getRun(workspaceId: string, runId: string, signal?: AbortSignal): Promise<OperationsRuntimeRunDetail>;
  retryRun(workspaceId: string, runId: string): Promise<void>;
  cancelRun(workspaceId: string, runId: string): Promise<void>;
  getPolicy(workspaceId: string, signal?: AbortSignal): Promise<OperationsRuntimePolicy>;
  updatePolicy(workspaceId: string, input: UpdateOperationsRuntimeSettingsInput & { expectedVersion: number }): Promise<OperationsRuntimePolicy>;
}

export class OperationsApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly traceId: string;

  constructor(message: string, status: number, code = "REQUEST_FAILED", traceId = "") {
    super(message);
    this.name = "OperationsApiError";
    this.status = status;
    this.code = code;
    this.traceId = traceId;
  }
}

export const operationsApi: OperationsApi = {
  async listAuditEvents(workspaceId, filter = {}, cursor, signal) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/operations/audit-events", {
      params: { path: { workspaceId }, query: { limit: 50, cursor, ...filter } },
      signal,
    });
    if (!result.data) throw operationsFailure(result.error, result.response.status, "无法读取审计事件。");
    return { items: result.data.items.map((item) => ({ ...item })), limit: result.data.page.limit, nextCursor: result.data.page.nextCursor };
  },

  async createAuditExport(workspaceId, filter, idempotencyKey) {
    const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/operations/audit-exports", {
      params: { path: { workspaceId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
      body: { idempotencyKey, filter },
    });
    if (!result.data) throw operationsFailure(result.error, result.response.status, "无法创建审计导出。");
    return { ...result.data };
  },

  async downloadAuditExport(workspaceId, exportId) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/operations/audit-exports/{exportId}/content", {
      params: { path: { workspaceId, exportId } },
      parseAs: "text",
    });
    if (typeof result.data !== "string") throw operationsFailure(result.error, result.response.status, "无法下载审计导出内容。");
    return {
      content: result.data,
      contentDigest: result.response.headers.get("X-Content-SHA256") ?? "",
      contentType: result.response.headers.get("Content-Type") ?? "application/json",
      filename: safeExportFilename(result.response.headers.get("Content-Disposition"), exportId),
    };
  },

  async listRuns(workspaceId, filter = {}, cursor, signal) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/operations/runs", {
      params: { path: { workspaceId }, query: { limit: 50, cursor, ...filter } },
      signal,
    });
    if (!result.data) throw operationsFailure(result.error, result.response.status, "无法读取运行记录。");
    return { items: result.data.items.map((item) => cloneRun(item)), limit: result.data.page.limit, nextCursor: result.data.page.nextCursor };
  },

  async getRun(workspaceId, runId, signal) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/operations/runs/{runId}", {
      params: { path: { workspaceId, runId } },
      signal,
    });
    if (!result.data) throw operationsFailure(result.error, result.response.status, "无法读取运行详情。");
    return { run: cloneRun(result.data.run), events: result.data.events.map((event) => ({ ...event })).sort((a, b) => a.sequence - b.sequence) };
  },

  async retryRun(workspaceId, runId) {
    const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/operations/runs/{runId}:retry", {
      params: { path: { workspaceId, runId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
    });
    if (!result.response.ok) throw operationsFailure(result.error, result.response.status, "无法重试此运行。");
  },

  async cancelRun(workspaceId, runId) {
    const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/operations/runs/{runId}:cancel", {
      params: { path: { workspaceId, runId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
    });
    if (!result.response.ok) throw operationsFailure(result.error, result.response.status, "无法取消此运行。");
  },

  async getPolicy(workspaceId, signal) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/runtime-policy", {
      params: { path: { workspaceId } },
      signal,
    });
    if (!result.data) throw operationsFailure(result.error, result.response.status, "无法读取运行策略。");
    return clonePolicy(result.data);
  },

  async updatePolicy(workspaceId, input) {
    const result = await apiClient.PATCH("/api/v1/workspaces/{workspaceId}/runtime-policy", {
      params: { path: { workspaceId }, header: { "X-Semlia-CSRF": requiredCSRFToken() } },
      body: input,
    });
    if (!result.data) throw operationsFailure(result.error, result.response.status, "无法更新运行策略。");
    return clonePolicy(result.data);
  },
};

export function createOperationsIdempotencyKey(prefix = "operations"): string {
  const value = typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `${prefix}-${value}`;
}

function requiredCSRFToken(): string {
  const token = sessionCSRFToken();
  if (!token) throw new OperationsApiError("会话校验信息已失效，请刷新后重试。", 401, "SESSION_EXPIRED");
  return token;
}

function cloneRun(run: OperationsRuntimeRun): OperationsRuntimeRun {
  return { ...run, capabilities: { ...run.capabilities } };
}

function clonePolicy(policy: OperationsRuntimePolicy): OperationsRuntimePolicy {
  return { settings: { ...policy.settings }, deployment: { ...policy.deployment } };
}

function operationsFailure(value: unknown, status: number, fallback: string): OperationsApiError {
  if (value && typeof value === "object") {
    const message = "message" in value && typeof value.message === "string" ? value.message : fallback;
    const code = "code" in value && typeof value.code === "string" ? value.code : "REQUEST_FAILED";
    const traceId = "traceId" in value && typeof value.traceId === "string" ? value.traceId : "";
    return new OperationsApiError(message, status, code, traceId);
  }
  return new OperationsApiError(fallback, status);
}

function safeExportFilename(contentDisposition: string | null, exportId: string): string {
  const match = contentDisposition?.match(/filename="?([^";]+)"?/i);
  const candidate = match?.[1]?.replace(/[^a-zA-Z0-9._-]/g, "");
  return candidate?.endsWith(".json") ? candidate : `audit-export-${exportId.replace(/[^a-zA-Z0-9_-]/g, "")}.json`;
}

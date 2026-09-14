import type { components } from "@semlia/sdk-typescript";

import { apiClient, sessionCSRFToken } from "./apiClient";

export type WorkbenchAttentionItem = components["schemas"]["WorkbenchAttentionItem"];
export type WorkbenchAttentionCounts = components["schemas"]["WorkbenchAttentionCounts"];
export type WorkbenchAttentionPage = components["schemas"]["WorkbenchAttentionPage"];
export type WorkbenchAttentionKind = components["schemas"]["WorkbenchAttentionKind"];
export type WorkbenchAttentionState = components["schemas"]["WorkbenchAttentionState"];
export type WorkbenchPriority = components["schemas"]["WorkbenchPriority"];
export type WorkbenchSort = components["schemas"]["WorkbenchSort"];
export type WorkbenchView = components["schemas"]["WorkbenchView"];
export type UpdateWorkbenchAttentionItemRequest = components["schemas"]["UpdateWorkbenchAttentionItemRequest"];

export interface WorkbenchFilter {
  view: WorkbenchView;
  search?: string;
  kind?: WorkbenchAttentionKind;
  state?: WorkbenchAttentionState;
  priority?: WorkbenchPriority;
  risk?: WorkbenchPriority;
  sort: WorkbenchSort;
  limit?: number;
}

export interface WorkbenchApi {
  listItems(workspaceId: string, filter: WorkbenchFilter, cursor?: string, signal?: AbortSignal): Promise<WorkbenchAttentionPage>;
  getItem(workspaceId: string, attentionItemId: string, signal?: AbortSignal): Promise<WorkbenchAttentionItem>;
  updateItem(workspaceId: string, attentionItemId: string, body: UpdateWorkbenchAttentionItemRequest, idempotencyKey: string): Promise<WorkbenchAttentionItem>;
}

export class WorkbenchApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly traceId: string;

  constructor(message: string, status: number, code = "REQUEST_FAILED", traceId = "") {
    super(message);
    this.name = "WorkbenchApiError";
    this.status = status;
    this.code = code;
    this.traceId = traceId;
  }
}

export const workbenchApi: WorkbenchApi = {
  async listItems(workspaceId, filter, cursor, signal) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/workbench/items", {
      params: {
        path: { workspaceId },
        query: {
          view: filter.view,
          search: filter.search,
          kind: filter.kind,
          state: filter.state,
          priority: filter.priority,
          risk: filter.risk,
          sort: filter.sort,
          limit: filter.limit ?? 50,
          cursor,
        },
      },
      signal,
    });
    if (!result.data) throw workbenchFailure(result.error, result.response.status, "无法读取工作台待办。");
    return clonePage(result.data);
  },

  async getItem(workspaceId, attentionItemId, signal) {
    const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/workbench/items/{attentionItemId}", {
      params: { path: { workspaceId, attentionItemId } },
      signal,
    });
    if (!result.data) throw workbenchFailure(result.error, result.response.status, "无法读取待办详情。");
    return cloneItem(result.data);
  },

  async updateItem(workspaceId, attentionItemId, body, idempotencyKey) {
    const result = await apiClient.PATCH("/api/v1/workspaces/{workspaceId}/workbench/items/{attentionItemId}", {
      params: {
        path: { workspaceId, attentionItemId },
        header: {
          "X-Semlia-CSRF": requiredCSRFToken(),
          "Idempotency-Key": idempotencyKey,
        },
      },
      body,
    });
    if (!result.data) throw workbenchFailure(result.error, result.response.status, "无法更新待办。");
    return cloneItem(result.data);
  },
};

export function createWorkbenchIdempotencyKey(prefix = "workbench"): string {
  const value = typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `${prefix}-${value}`;
}

function requiredCSRFToken(): string {
  const token = sessionCSRFToken();
  if (!token) throw new WorkbenchApiError("会话校验信息已失效，请刷新后重试。", 401, "SESSION_EXPIRED");
  return token;
}

function cloneItem(item: WorkbenchAttentionItem): WorkbenchAttentionItem {
  return { ...item, nextActions: [...item.nextActions] };
}

function clonePage(page: WorkbenchAttentionPage): WorkbenchAttentionPage {
  return {
    items: page.items.map(cloneItem),
    counts: { ...page.counts },
    page: { ...page.page },
  };
}

function workbenchFailure(value: unknown, status: number, fallback: string): WorkbenchApiError {
  if (value && typeof value === "object") {
    const message = "message" in value && typeof value.message === "string" ? value.message : fallback;
    const code = "code" in value && typeof value.code === "string" ? value.code : "REQUEST_FAILED";
    const traceId = "traceId" in value && typeof value.traceId === "string" ? value.traceId : "";
    return new WorkbenchApiError(message, status, code, traceId);
  }
  return new WorkbenchApiError(fallback, status);
}

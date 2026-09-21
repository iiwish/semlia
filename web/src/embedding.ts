import type { components } from "@semlia/sdk-typescript";
import { apiClient } from "./apiClient";

export type EmbeddingStatus = components["schemas"]["EmbeddingIndexStatus"];
export type EmbeddingIndex = components["schemas"]["EmbeddingIndexVersion"];
export type EmbeddingSearchResult = components["schemas"]["EmbeddingSearchResult"];

/** Server-reported blockers for starting a rebuild, mirrored from the domain. */
export const REASON_NOT_CONFIGURED = "not_configured";
export const REASON_NO_PUBLISHED_RELEASE = "no_published_release";

/**
 * Server error envelope preserved as a typed error so the panel can explain the
 * real blocker instead of one opaque "unavailable" state.
 */
export class EmbeddingApiError extends Error {
  constructor(readonly code: string, message: string, readonly traceId?: string) {
    super(message);
    this.name = "EmbeddingApiError";
  }
}

const startMessages: Record<string, string> = {
  EMBEDDING_NO_PUBLISHED_RELEASE: "当前工作区还没有已发布的版本。向量索引只覆盖已发布知识，请先在“变更与发布”完成一次发布。",
  EMBEDDING_NOT_CONFIGURED: "尚未就绪：需要安装 pgvector、启用默认索引模型，并在部署环境注入该模型声明的密钥环境变量。",
  FORBIDDEN: "当前账号没有管理工作区的权限，无法重建向量索引。",
  VERSION_CONFLICT: "工作区状态已变化，请刷新后重试。",
  EMBEDDING_UNAVAILABLE: "向量索引服务暂时不可用，请稍后重试。",
};

/** Reads the contract error envelope; openapi-fetch hands back the parsed body as `error`. */
function apiError(error: unknown): EmbeddingApiError {
  const payload = error as { code?: string; message?: string; traceId?: string } | null | undefined;
  const code = typeof payload?.code === "string" && payload.code !== "" ? payload.code : "EMBEDDING_UNAVAILABLE";
  return new EmbeddingApiError(code, typeof payload?.message === "string" ? payload.message : "", typeof payload?.traceId === "string" ? payload.traceId : undefined);
}

function localized(failure: EmbeddingApiError, fallback: string): EmbeddingApiError {
  return new EmbeddingApiError(failure.code, startMessages[failure.code] ?? fallback, failure.traceId);
}

export interface EmbeddingApi {
  status(workspaceId: string, signal?: AbortSignal): Promise<EmbeddingStatus>;
  start(workspaceId: string, key: string): Promise<EmbeddingIndex>;
  cancel(workspaceId: string, indexId: string): Promise<void>;
  search(workspaceId: string, query: string, signal?: AbortSignal): Promise<EmbeddingSearchResult>;
}

export const embeddingApi: EmbeddingApi = {
  async status(workspaceId, signal) {
    const { data, error } = await apiClient.GET("/api/v1/workspaces/{workspaceId}/embedding-index", { params: { path: { workspaceId } }, signal });
    if (error || !data) throw new EmbeddingApiError("EMBEDDING_UNAVAILABLE", "无法读取向量索引状态。");
    return data;
  },
  async start(workspaceId, key) {
    const { data, error } = await apiClient.POST("/api/v1/workspaces/{workspaceId}/embedding-index", { params: { path: { workspaceId }, header: { "Idempotency-Key": key } } });
    if (error || !data) throw localized(apiError(error), "无法启动重建，请检查服务配置与权限。");
    return data;
  },
  async cancel(workspaceId, indexId) {
    const { error } = await apiClient.POST("/api/v1/workspaces/{workspaceId}/embedding-index/{indexId}/cancel", { params: { path: { workspaceId, indexId } } });
    if (error) throw new EmbeddingApiError("EMBEDDING_UNAVAILABLE", "取消重建失败。");
  },
  async search(workspaceId, q, signal) {
    const { data, error } = await apiClient.GET("/api/v1/workspaces/{workspaceId}/embedding-search", { params: { path: { workspaceId }, query: { q } }, signal });
    if (error || !data) throw new EmbeddingApiError("EMBEDDING_UNAVAILABLE", "检索已发布知识失败。");
    return data;
  },
};

import type { components } from "@semlia/sdk-typescript";

import { apiClient } from "./apiClient";

export type SourceConnection = components["schemas"]["SourceConnection"];
export type CreateSourceRequest = components["schemas"]["CreateSourceRequest"];
export type UpdateSourceRequest = components["schemas"]["UpdateSourceRequest"];
export type SourceDiscoveryRun = components["schemas"]["SourceDiscoveryRun"];
export type DiscoveryRun = components["schemas"]["DiscoveryRun"];
export type SemanticCandidate = components["schemas"]["SemanticCandidate"];
export type SemanticCandidateStatus = components["schemas"]["SemanticCandidateStatus"];
export type SemanticCandidateDecision = components["schemas"]["SemanticCandidateDecision"];
export type SourceConnectionPage = components["schemas"]["SourceConnectionPage"];
export type SourceDiscoveryRunPage = components["schemas"]["SourceDiscoveryRunPage"];
export type SemanticCandidatePage = components["schemas"]["SemanticCandidatePage"];

export interface PageRequest {
  cursor?: string;
  signal?: AbortSignal;
}

export interface SourcePageRequest extends PageRequest {
  sourceId?: string;
  limit?: number;
}

export interface CandidatePageRequest extends PageRequest {
  sourceId?: string;
  status?: SemanticCandidateStatus;
}

export class DiscoveryApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly traceId: string;

  constructor(message: string, status: number, code = "REQUEST_FAILED", traceId = "") {
    super(message);
    this.name = "DiscoveryApiError";
    this.status = status;
    this.code = code;
    this.traceId = traceId;
  }
}

export async function listSources(workspaceId: string, options: SourcePageRequest = {}): Promise<SourceConnectionPage> {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/sources", {
    params: { path: { workspaceId }, query: { limit: options.limit ?? 50, cursor: options.cursor, sourceId: options.sourceId } }, signal: options.signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取数据来源。");
  return clonePage(result.data);
}

export async function createSource(workspaceId: string, input: CreateSourceRequest): Promise<SourceConnection> {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/sources", {
    params: { path: { workspaceId } }, body: input,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法创建 PostgreSQL 来源。");
  return result.data;
}

export async function updateSource(workspaceId: string, sourceId: string, input: UpdateSourceRequest): Promise<SourceConnection> {
  const result = await apiClient.PATCH("/api/v1/workspaces/{workspaceId}/sources/{sourceId}", {
    params: { path: { workspaceId, sourceId } }, body: input,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法更新 PostgreSQL 来源。");
  return result.data;
}

export async function deleteSource(workspaceId: string, sourceId: string, expectedVersion: number): Promise<SourceConnection> {
  const result = await apiClient.DELETE("/api/v1/workspaces/{workspaceId}/sources/{sourceId}", {
    params: { path: { workspaceId, sourceId } }, body: { expectedVersion },
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法删除 PostgreSQL 来源。");
  return result.data;
}

export async function rotateSourceCredential(workspaceId: string, sourceId: string, password: string, expectedVersion: number): Promise<SourceConnection> {
  const result = await apiClient.PUT("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/credential", {
    params: { path: { workspaceId, sourceId } }, body: { password, expectedVersion },
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法轮换来源凭据。");
  return result.data;
}

export async function testSourceConnection(workspaceId: string, sourceId: string): Promise<void> {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/test", {
    params: { path: { workspaceId, sourceId } },
  });
  if (!result.data) throw failure(result.error, result.response.status, "连接测试失败。");
}

export async function listSourceRuns(workspaceId: string, sourceId: string, options: PageRequest = {}): Promise<SourceDiscoveryRunPage> {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/discovery-runs", {
    params: { path: { workspaceId, sourceId }, query: { limit: 50, cursor: options.cursor } }, signal: options.signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取发现运行。");
  return clonePage(result.data);
}

export async function startSourceRun(workspaceId: string, sourceId: string, idempotencyKey: string): Promise<SourceDiscoveryRun> {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/discovery-runs", {
    params: { path: { workspaceId, sourceId } }, body: { idempotencyKey },
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法启动发现运行。");
  return result.data;
}

export async function getDiscoveryRun(workspaceId: string, runId: string, signal?: AbortSignal): Promise<DiscoveryRun> {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/discovery-runs/{runId}", {
    params: { path: { workspaceId, runId } }, signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取运行详情。");
  return result.data;
}

export async function listSemanticCandidates(workspaceId: string, options: CandidatePageRequest = {}): Promise<SemanticCandidatePage> {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/semantic-candidates", {
    params: { path: { workspaceId }, query: { status: options.status, sourceId: options.sourceId, limit: 50, cursor: options.cursor } }, signal: options.signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取语义候选。");
  return clonePage(result.data);
}

export async function getSemanticCandidate(workspaceId: string, candidateId: string, signal?: AbortSignal): Promise<SemanticCandidate> {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/semantic-candidates/{candidateId}", {
    params: { path: { workspaceId, candidateId } }, signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取候选详情。");
  return result.data;
}

export async function decideSemanticCandidate(
  workspaceId: string,
  candidateId: string,
  input: { action: "dismiss" | "convert"; proposalId?: string; reason: string; idempotencyKey: string },
): Promise<SemanticCandidateDecision> {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/semantic-candidates/{candidateId}/decisions", {
    params: { path: { workspaceId, candidateId } }, body: input,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法记录候选决策。");
  return result.data;
}

function failure(value: unknown, status: number, fallback: string): DiscoveryApiError {
  if (value && typeof value === "object") {
    const message = "message" in value && typeof value.message === "string" ? value.message : fallback;
    const code = "code" in value && typeof value.code === "string" ? value.code : "REQUEST_FAILED";
    const traceId = "traceId" in value && typeof value.traceId === "string" ? value.traceId : "";
    return new DiscoveryApiError(message, status, code, traceId);
  }
  return new DiscoveryApiError(fallback, status);
}

function clonePage<T extends { id: string }>(page: { items: T[]; total: number; limit: number; nextCursor?: string }) {
  return { items: page.items.map((item) => ({ ...item })), total: page.total, limit: page.limit, nextCursor: page.nextCursor };
}

import { apiClient } from "./apiClient";
import type { components } from "@semlia/sdk-typescript";

export type Snapshot = components["schemas"]["SourceSnapshot"];
export type Member = components["schemas"]["SnapshotMember"];
export type FilePreview = components["schemas"]["ArtifactPreview"];

function unwrap<T>({ data, error, response }: { data?: T; error?: unknown; response: Response }): T {
  if (data !== undefined) return data;
  const detail = error as { code?: string; traceId?: string } | undefined;
  const message = response.status === 403 ? "没有读取此来源内容的权限。" : response.status === 401 ? "登录已失效，请重新登录。" : response.status === 410 ? "此版本的原文件已过期或未保留，无法预览。" : response.status === 404 ? "来源版本或文件不存在，可能已被移除。" : response.status === 422 ? "文件格式或内容不符合安全预览要求。" : "来源内容读取失败，请重试。";
  throw new Error(`${message}${detail?.traceId ? `（trace ${detail.traceId}）` : ""}`);
}
export const sourceContentsAPI = {
  async snapshots(workspaceId: string, sourceId: string, cursor?: string, signal?: AbortSignal) {
    return unwrap(await apiClient.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots", { params: { path: { workspaceId, sourceId }, query: { cursor, limit: 50 } }, signal }));
  },
  async members(workspaceId: string, sourceId: string, snapshotId: string, cursor?: string, signal?: AbortSignal) {
    return unwrap(await apiClient.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}/members", { params: { path: { workspaceId, sourceId, snapshotId }, query: { cursor, limit: 200 } }, signal }));
  },
  async set(workspaceId: string, artifactSetId: string, signal?: AbortSignal) {
    return unwrap(await apiClient.GET("/api/v1/workspaces/{workspaceId}/ingestion/artifact-sets/{artifactSetId}", { params: { path: { workspaceId, artifactSetId } }, signal }));
  },
  async preview(workspaceId: string, artifactSetId: string, artifactId: string, signal?: AbortSignal) {
    return unwrap(await apiClient.GET("/api/v1/workspaces/{workspaceId}/ingestion/artifact-sets/{artifactSetId}/members/{artifactId}/preview", { params: { path: { workspaceId, artifactSetId, artifactId } }, signal }));
  },
};

export function schemaOf(member: Member): string {
  // Older snapshots only retain an unquoted qualified name. Ambiguous names stay intact.
  const parts = member.name.split(".");
  return parts.length === 2 && parts.every(Boolean) ? parts[0] : "未记录独立 schema";
}

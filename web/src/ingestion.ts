import type { components } from "@semlia/sdk-typescript";

import { apiClient, sessionCSRFToken } from "./apiClient";

export type ArtifactKind = components["schemas"]["ArtifactKind"];
export type PublicArtifactKind = components["schemas"]["PublicArtifactKind"];
export type ArtifactStatus = components["schemas"]["ArtifactStatus"];
export type IngestionArtifact = components["schemas"]["IngestionArtifact"];
export type ArtifactSet = components["schemas"]["ArtifactSet"];
export type FinalizeArtifactSetInput = components["schemas"]["FinalizeArtifactSetRequest"];
export type RegisterSQLArtifactsInput = components["schemas"]["RegisterSQLArtifactsRequest"];
export type SourceSchedule = components["schemas"]["SourceSchedule"];
export type SourceScheduleInput = components["schemas"]["CreateSourceScheduleRequest"];
export type SourceScheduleUpdateInput = Omit<components["schemas"]["UpdateSourceScheduleRequest"], "expectedVersion">;
export type SourceScheduleOccurrence = components["schemas"]["SourceScheduleOccurrence"];

export interface CursorPage<T> {
  items: T[];
  total: number;
  limit: number;
  nextCursor?: string;
}

export interface ArtifactFilter {
  kind?: ArtifactKind;
  status?: ArtifactStatus;
}

export interface ExistingSourceTarget {
  sourceId: string;
  expectedSourceVersion: number;
}

export interface IngestionApi {
  listArtifacts(workspaceId: string, filter?: ArtifactFilter, cursor?: string, signal?: AbortSignal): Promise<CursorPage<IngestionArtifact>>;
  stageArtifact(workspaceId: string, file: File, kind: PublicArtifactKind, idempotencyKey: string, target?: ExistingSourceTarget): Promise<IngestionArtifact>;
  getArtifactSet(workspaceId: string, artifactSetId: string, signal?: AbortSignal): Promise<ArtifactSet>;
  finalizeArtifactSet(workspaceId: string, input: FinalizeArtifactSetInput, idempotencyKey: string): Promise<ArtifactSet>;
  registerSQL(workspaceId: string, input: RegisterSQLArtifactsInput, idempotencyKey: string): Promise<ArtifactSet>;
  listSchedules(workspaceId: string, sourceId: string, cursor?: string, signal?: AbortSignal): Promise<CursorPage<SourceSchedule>>;
  getSchedule(workspaceId: string, scheduleId: string, signal?: AbortSignal): Promise<SourceSchedule>;
  createSchedule(workspaceId: string, sourceId: string, input: SourceScheduleInput, idempotencyKey: string): Promise<SourceSchedule>;
  updateSchedule(workspaceId: string, scheduleId: string, input: SourceScheduleUpdateInput & { expectedVersion: number }, idempotencyKey: string): Promise<SourceSchedule>;
  deleteSchedule(workspaceId: string, scheduleId: string, expectedVersion: number, idempotencyKey: string): Promise<SourceSchedule>;
  pauseSchedule(workspaceId: string, scheduleId: string, expectedVersion: number, idempotencyKey: string): Promise<SourceSchedule>;
  resumeSchedule(workspaceId: string, scheduleId: string, expectedVersion: number, idempotencyKey: string): Promise<SourceSchedule>;
  runScheduleNow(workspaceId: string, scheduleId: string, expectedVersion: number, idempotencyKey: string): Promise<SourceScheduleOccurrence>;
  listOccurrences(workspaceId: string, scheduleId: string, cursor?: string, signal?: AbortSignal): Promise<CursorPage<SourceScheduleOccurrence>>;
}

export class IngestionApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly traceId: string;

  constructor(message: string, status: number, code = "REQUEST_FAILED", traceId = "") {
    super(message);
    this.name = "IngestionApiError";
    this.status = status;
    this.code = code;
    this.traceId = traceId;
  }
}

export async function listIngestionArtifacts(workspaceId: string, filter: ArtifactFilter = {}, cursor?: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/ingestion/artifacts", {
    params: { path: { workspaceId }, query: { ...filter, limit: 50, cursor } },
    signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取持久工件。", result.response);
  return clonePage(result.data);
}

export async function stageIngestionArtifact(workspaceId: string, file: File, kind: PublicArtifactKind, idempotencyKey: string, target?: ExistingSourceTarget) {
  if (file.size <= 0) throw new IngestionApiError("工件不能为空。", 400, "EMPTY_ARTIFACT");
  if (file.size > 50 * 1024 * 1024) throw new IngestionApiError("单个工件不能超过 50 MiB。", 413, "ARTIFACT_TOO_LARGE");
  const rawBody = new Uint8Array(await readFileBuffer(file));
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/ingestion/artifacts", {
    params: {
      path: { workspaceId },
      header: {
        "Idempotency-Key": idempotencyKey,
        "X-Semlia-CSRF": sessionCSRFToken(),
        "X-Artifact-Kind": kind,
        "X-File-Name": file.name,
        ...(target ? {
          "X-Source-Id": target.sourceId,
          "X-Expected-Source-Version": target.expectedSourceVersion,
        } : {}),
      },
    },
    headers: { "Content-Type": mediaTypeFor(kind, file.type) },
    body: rawBody as unknown as string,
    bodySerializer: (body) => body as unknown as BodyInit,
  });
  if (!result.data) throw failure(result.error, result.response.status, "工件上传或校验失败。", result.response);
  return { ...result.data };
}

function readFileBuffer(file: File): Promise<ArrayBuffer> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.addEventListener("load", () => {
      if (reader.result instanceof ArrayBuffer) resolve(reader.result);
      else reject(new IngestionApiError("无法读取所选文件。", 400, "FILE_READ_FAILED"));
    }, { once: true });
    reader.addEventListener("error", () => reject(new IngestionApiError("无法读取所选文件。", 400, "FILE_READ_FAILED")), { once: true });
    reader.readAsArrayBuffer(file);
  });
}

export async function getIngestionArtifactSet(workspaceId: string, artifactSetId: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/ingestion/artifact-sets/{artifactSetId}", {
    params: { path: { workspaceId, artifactSetId } },
    signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取工件集合。", result.response);
  return cloneArtifactSet(result.data);
}

export async function finalizeArtifactSet(workspaceId: string, input: FinalizeArtifactSetInput, idempotencyKey: string) {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/ingestion/artifact-sets:finalize", {
    params: { path: { workspaceId }, header: commandHeaders(idempotencyKey) },
    body: input,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法创建持久文件来源。", result.response);
  return cloneArtifactSet(result.data);
}

export async function registerConfiguredSQLArtifacts(workspaceId: string, input: RegisterSQLArtifactsInput, idempotencyKey: string) {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/ingestion/sql-registrations", {
    params: { path: { workspaceId }, header: commandHeaders(idempotencyKey) },
    body: input,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法注册配置根目录内的 SQL 工件。", result.response);
  return cloneArtifactSet(result.data);
}

export async function listSourceSchedules(workspaceId: string, sourceId: string, cursor?: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/schedules", {
    params: { path: { workspaceId, sourceId }, query: { limit: 50, cursor } },
    signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取接入计划。", result.response);
  return clonePage(result.data);
}

export async function getSourceSchedule(workspaceId: string, scheduleId: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}", {
    params: { path: { workspaceId, scheduleId } },
    signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取接入计划详情。", result.response);
  return { ...result.data };
}

export async function createSourceSchedule(workspaceId: string, sourceId: string, input: SourceScheduleInput, idempotencyKey: string) {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/schedules", {
    params: { path: { workspaceId, sourceId }, header: commandHeaders(idempotencyKey) },
    body: input,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法创建接入计划。", result.response);
  return { ...result.data };
}

export async function updateSourceSchedule(workspaceId: string, scheduleId: string, input: SourceScheduleUpdateInput & { expectedVersion: number }, idempotencyKey: string) {
  const result = await apiClient.PATCH("/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}", {
    params: { path: { workspaceId, scheduleId }, header: commandHeaders(idempotencyKey) },
    body: input,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法更新接入计划。", result.response);
  return { ...result.data };
}

export async function deleteSourceSchedule(workspaceId: string, scheduleId: string, expectedVersion: number, idempotencyKey: string) {
  const result = await apiClient.DELETE("/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}", {
    params: { path: { workspaceId, scheduleId }, header: commandHeaders(idempotencyKey) },
    body: { expectedVersion },
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法删除接入计划。", result.response);
  return { ...result.data };
}

export async function pauseSourceSchedule(workspaceId: string, scheduleId: string, expectedVersion: number, idempotencyKey: string) {
  return scheduleCommand("/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}:pause", workspaceId, scheduleId, expectedVersion, idempotencyKey, "无法暂停接入计划。");
}

export async function resumeSourceSchedule(workspaceId: string, scheduleId: string, expectedVersion: number, idempotencyKey: string) {
  return scheduleCommand("/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}:resume", workspaceId, scheduleId, expectedVersion, idempotencyKey, "无法恢复接入计划。");
}

export async function runSourceScheduleNow(workspaceId: string, scheduleId: string, expectedVersion: number, idempotencyKey: string) {
  const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}:run-now", {
    params: { path: { workspaceId, scheduleId }, header: commandHeaders(idempotencyKey) },
    body: { expectedVersion },
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法立即运行接入计划。", result.response);
  return { ...result.data };
}

export async function listSourceScheduleOccurrences(workspaceId: string, scheduleId: string, cursor?: string, signal?: AbortSignal) {
  const result = await apiClient.GET("/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}/occurrences", {
    params: { path: { workspaceId, scheduleId }, query: { limit: 50, cursor } },
    signal,
  });
  if (!result.data) throw failure(result.error, result.response.status, "无法读取计划执行记录。", result.response);
  return clonePage(result.data);
}

export const ingestionApi: IngestionApi = {
  listArtifacts: listIngestionArtifacts,
  stageArtifact: stageIngestionArtifact,
  getArtifactSet: getIngestionArtifactSet,
  finalizeArtifactSet,
  registerSQL: registerConfiguredSQLArtifacts,
  listSchedules: listSourceSchedules,
  getSchedule: getSourceSchedule,
  createSchedule: createSourceSchedule,
  updateSchedule: updateSourceSchedule,
  deleteSchedule: deleteSourceSchedule,
  pauseSchedule: pauseSourceSchedule,
  resumeSchedule: resumeSourceSchedule,
  runScheduleNow: runSourceScheduleNow,
  listOccurrences: listSourceScheduleOccurrences,
};

export function createIngestionIdempotencyKey(prefix: string) {
  const suffix = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `${prefix}-${suffix}`;
}

async function scheduleCommand(
  path: "/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}:pause" | "/api/v1/workspaces/{workspaceId}/schedules/{scheduleId}:resume",
  workspaceId: string,
  scheduleId: string,
  expectedVersion: number,
  idempotencyKey: string,
  fallback: string,
) {
  const result = await apiClient.POST(path, {
    params: { path: { workspaceId, scheduleId }, header: commandHeaders(idempotencyKey) },
    body: { expectedVersion },
  });
  if (!result.data) throw failure(result.error, result.response.status, fallback, result.response);
  return { ...result.data };
}

function commandHeaders(idempotencyKey: string) {
  return { "Idempotency-Key": idempotencyKey, "X-Semlia-CSRF": sessionCSRFToken() };
}

function mediaTypeFor(kind: PublicArtifactKind, declared: string) {
  if (kind === "csv") return "text/csv";
  if (kind === "markdown") return "text/markdown";
  if (kind === "xlsx") return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";
  if (kind === "dbt_manifest" || kind === "dbt_catalog") return "application/json";
  return declared;
}

function clonePage<T>(page: CursorPage<T>): CursorPage<T> {
  return { items: page.items.map((item) => ({ ...item })), total: page.total, limit: page.limit, nextCursor: page.nextCursor };
}

function cloneArtifactSet(value: ArtifactSet): ArtifactSet {
  return { ...value, members: value.members.map((item) => ({ ...item })) };
}

function failure(value: unknown, status: number, fallback: string, response?: Response) {
  if (value && typeof value === "object") {
    const message = "message" in value && typeof value.message === "string" ? value.message : fallback;
    const code = "code" in value && typeof value.code === "string" ? value.code : "REQUEST_FAILED";
    const traceId = "traceId" in value && typeof value.traceId === "string" ? value.traceId : response?.headers.get("X-Trace-ID") ?? "";
    return new IngestionApiError(message, status, code, traceId);
  }
  return new IngestionApiError(fallback, status, "REQUEST_FAILED", response?.headers.get("X-Trace-ID") ?? "");
}

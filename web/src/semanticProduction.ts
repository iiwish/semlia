import type { components } from "@semlia/sdk-typescript";
import { apiClient as client } from "./apiClient";

type Schema = components["schemas"];
export type ProductionInput = Schema["ProductionInput"];
export type SourceSnapshot = Schema["SourceSnapshot"];
export type SnapshotMember = Schema["SnapshotMember"];
export type AssetContent = Schema["AssetContent"];
export type TargetKind = Schema["TargetKind"];
export type PhysicalReference = Schema["PhysicalReference"];
export type ProductionContent = AssetContent | Schema["BindingContent"] | Schema["GrainContent"] | Schema["EntityKeyContent"] | Schema["JoinContent"];
// The generated allOf includes Record<string, never> for polymorphic content.
// Keep the wire fields, replacing only that impossible intersection in the editor.
export type ProductionTarget = {
  localKey: string; kind: TargetKind; title: string; content: ProductionContent;
  changes: Schema["Change"][]; evidenceIds: string[];
} & ({ intent: "create"; identityKey: string; reuseIdentity?: Schema["ReintroductionIdentity"] } | { intent: "update"; targetId: string; baseRevisionId?: string; baseObjectVersion?: number });
export type ProductionOperation = Omit<Schema["ProductionOperation"], "targets"> & { targets: Array<Omit<Schema["TargetResult"], "declaration"> & { declaration: ProductionTarget }> };
export type OperationSummary = Schema["OperationSummary"];
export type ProductionRelease = Schema["ProductionRelease"];
export type GenerationResult = Schema["GenerationResult"];
export type BusinessRuleWitness = Schema["ProductionBusinessRuleWitness"];
export type ProductionDraft = { input: ProductionInput; targets: ProductionTarget[]; supersedesOperationId?: string };
export type ProductionListFilter = { candidateId?: string; sourceId?: string; createdBy?: string; cursor?: string; limit?: number };

export class ProductionApiError extends Error {
  constructor(message: string, public readonly code: string, public readonly status: number, public readonly traceId?: string) { super(message); this.name = "ProductionApiError"; }
}

const errorLabels: Record<string, string> = {
  FORBIDDEN: "当前主体无权执行此操作", NO_MATCHING_GRANT: "当前权限不覆盖所选对象", UNAUTHENTICATED: "请重新登录", VERSION_CONFLICT: "版本已变化，请读取服务器版本后重新核对", INPUT_STALE: "来源版本已过期，请重新选择来源", INPUT_INCOMPLETE: "来源覆盖不完整，不能提交", EVIDENCE_MISSING: "缺少可验证的证据", GENERATION_NOT_AUTHORIZED: "当前主体与模型配置没有服务端生成额度授权", GENERATION_OUTCOME_UNKNOWN: "模型调用结果未知，未自动重新调用", VALIDATION_REQUIRED: "验证结果已失效，需要重新验证和审核", SEPARATION_OF_DUTY: "参与建模的主体不能审核或发布同一生产集合", ALREADY_PRODUCED: "候选已有生产记录，请恢复已有操作", IDENTITY_CONFLICT: "业务身份已存在，请明确匹配已有对象", HEAD_CONFLICT: "发布基线已变化，请重新核对", BASELINE_CONFLICT: "对象基础版本已变化，请重新建模", CHANGE_SET_FROZEN: "当前版本已冻结，请创建纠正版本", NO_APPROVING_REVIEW: "缺少当前验证版本的独立审核", PRODUCTION_BUSINESS_RULE_UNCONFIRMED: "业务规则尚未由有权主体确认",
};
export function productionMessage(error: unknown): string {
  if (error instanceof ProductionApiError) return `${errorLabels[error.code] ?? error.message}${error.traceId ? `（trace ${error.traceId}）` : ""}`;
  return error instanceof Error ? error.message : "请求失败，请核对服务器状态。";
}
function dataOrThrow<T>(result: { data?: T; error?: unknown; response: Response }): T {
  if (result.data !== undefined) return result.data;
  const error = result.error && typeof result.error === "object" ? result.error as Record<string, unknown> : {};
  throw new ProductionApiError(typeof error.message === "string" ? error.message : "语义生产请求失败", typeof error.code === "string" ? error.code : "REQUEST_FAILED", result.response.status, typeof error.traceId === "string" ? error.traceId : result.response.headers.get("X-Trace-ID") ?? undefined);
}
const pathFor = (workspaceId: string, operationId: string) => ({ workspaceId, operationId });
const headersFor = (key: string) => ({ "Idempotency-Key": key });

export const productionAPI = {
  async list(workspaceId: string, query: ProductionListFilter = {}, signal?: AbortSignal) {
    return dataOrThrow(await client.GET("/api/v1/workspaces/{workspaceId}/production-operations", { params: { path: { workspaceId }, query: { limit: 50, ...query } }, signal }));
  },
  async get(workspaceId: string, operationId: string, signal?: AbortSignal, version?: number): Promise<ProductionOperation> {
    return dataOrThrow(await client.GET("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}", { params: { path: pathFor(workspaceId, operationId), query: version ? { version } : {} }, signal })) as unknown as ProductionOperation;
  },
  async create(workspaceId: string, body: ProductionDraft, key: string) {
    return dataOrThrow(await client.POST("/api/v1/workspaces/{workspaceId}/production-operations", { params: { path: { workspaceId }, header: headersFor(key) }, body: body as unknown as Schema["CreateProductionRequest"] }));
  },
  async replace(workspaceId: string, operationId: string, body: ProductionDraft & { expectedVersion: number; suggestionRunId?: string }, key: string) {
    return dataOrThrow(await client.PUT("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}", { params: { path: pathFor(workspaceId, operationId), header: headersFor(key) }, body: body as unknown as Schema["ReplaceProductionRequest"] }));
  },
  async submit(workspaceId: string, operationId: string, body: Schema["SubmitProductionRequest"], key: string) {
    return dataOrThrow(await client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/submit", { params: { path: pathFor(workspaceId, operationId), header: headersFor(key) }, body }));
  },
  async validate(workspaceId: string, operationId: string, body: Schema["ValidateProductionRequest"], key: string) {
    return dataOrThrow(await client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/validations", { params: { path: pathFor(workspaceId, operationId), header: headersFor(key) }, body }));
  },
  async review(workspaceId: string, operationId: string, body: Schema["ReviewProductionRequest"], key: string) {
    return dataOrThrow(await client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/reviews", { params: { path: pathFor(workspaceId, operationId), header: headersFor(key) }, body }));
  },
  async rules(workspaceId: string, operationId: string, version: number, signal?: AbortSignal) {
    return dataOrThrow(await client.GET("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/business-rule-confirmations", { params: { path: pathFor(workspaceId, operationId), query: { version } }, signal })).items;
  },
  async recordRule(workspaceId: string, operationId: string, body: Schema["ProductionBusinessRuleRequest"], key: string) {
    return dataOrThrow(await client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/business-rule-confirmations", { params: { path: pathFor(workspaceId, operationId), header: headersFor(key) }, body }));
  },
  async generate(workspaceId: string, operationId: string, body: Schema["GenerateProductionRequest"], key: string) {
    return dataOrThrow(await client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/generation", { params: { path: pathFor(workspaceId, operationId), header: headersFor(key) }, body })) as unknown as GenerationResult;
  },
  async generation(workspaceId: string, operationId: string, runId: string, signal?: AbortSignal) {
    return dataOrThrow(await client.GET("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/generation/{runId}", { params: { path: { workspaceId, operationId, runId } }, signal })) as unknown as GenerationResult;
  },
  async publish(workspaceId: string, operationId: string, body: Schema["PublishProductionRequest"], key: string) {
    return dataOrThrow(await client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/publish", { params: { path: pathFor(workspaceId, operationId), header: headersFor(key) }, body }));
  },
  async release(workspaceId: string, releaseId: string, signal?: AbortSignal) {
    return dataOrThrow(await client.GET("/api/v1/workspaces/{workspaceId}/production-releases/{releaseId}", { params: { path: { workspaceId, releaseId } }, signal }));
  },
  async rollback(workspaceId: string, releaseId: string, body: Schema["RollbackProductionRequest"], key: string) {
    return dataOrThrow(await client.POST("/api/v1/workspaces/{workspaceId}/production-releases/{releaseId}/rollback", { params: { path: { workspaceId, releaseId }, header: headersFor(key) }, body }));
  },
};
export type ProductionAPI = typeof productionAPI;

export async function getProductionSnapshot(workspaceId: string, sourceId: string, snapshotId: string, signal?: AbortSignal) {
  return dataOrThrow(await client.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}", { params: { path: { workspaceId, sourceId, snapshotId } }, signal }));
}
export async function listProductionMembers(workspaceId: string, sourceId: string, snapshotId: string, cursor?: string, signal?: AbortSignal) {
  return dataOrThrow(await client.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}/members", { params: { path: { workspaceId, sourceId, snapshotId }, query: { limit: 200, ...(cursor ? { cursor } : {}) } }, signal }));
}
export function makeAssetTarget(localKey: string, title: string, address: string, assetType: AssetContent["assetType"], principalId: string): ProductionTarget {
  return { intent: "create", kind: "semantic_asset", localKey, title, identityKey: address, content: { address, assetType, displayName: title, definition: null, scope: null, ownerPrincipalId: principalId }, changes: [], evidenceIds: [] };
}
export function buildProductionInput(snapshot: SourceSnapshot, candidate: { id: string; contentDigest: string }, targetKeys: string[], primaryTargetKey: string): ProductionInput {
  const coverageKeys = snapshot.coverage.filter((unit) => unit.status === "complete" && unit.enumerationComplete).map((unit) => unit.key);
  if (snapshot.historyQuality !== "verified" || !coverageKeys.length || !targetKeys.includes(primaryTargetKey)) throw new Error("没有可验证的来源覆盖或候选主目标，尚未创建草稿。");
  return { snapshots: [{ sourceId: snapshot.sourceId, snapshotId: snapshot.id, digest: snapshot.contentDigest, coverageKeys }], candidates: [{ candidateId: candidate.id, snapshotId: snapshot.id, digest: candidate.contentDigest, targetKeys, primaryTargetKey }], evidence: [], dependencies: [] };
}
export function physicalReference(snapshotId: string, member: SnapshotMember): PhysicalReference {
  if (member.kind !== "dataset" && member.kind !== "field") throw new Error("请选择固定版本的数据集或字段。");
  return { snapshotId, kind: member.kind, objectId: member.objectId, revisionId: member.revisionId };
}
export function targetChanges(before: ProductionContent, after: ProductionContent): Schema["Change"][] {
  const a = before as unknown as Record<string, unknown>, b = after as unknown as Record<string, unknown>;
  return [...new Set([...Object.keys(a), ...Object.keys(b)])].sort().flatMap((fieldPath): Schema["Change"][] => {
    if (JSON.stringify(a[fieldPath]) === JSON.stringify(b[fieldPath])) return [];
    if (!(fieldPath in a)) return [{ fieldPath, op: "add", afterValue: b[fieldPath] }];
    if (!(fieldPath in b)) return [{ fieldPath, op: "remove", beforeValue: a[fieldPath] }];
    return [{ fieldPath, op: "update", beforeValue: a[fieldPath], afterValue: b[fieldPath] }];
  });
}
export const progressLabels: Record<OperationSummary["progress"], string> = { draft: "建模中", no_change: "无实质变化", validating: "验证中", needs_correction: "需要纠正", in_review: "待独立审核", ready_to_publish: "可发布", released: "已发布", rejected: "已拒绝" };
export const kindLabels: Record<TargetKind, string> = { semantic_asset: "语义资产", physical_binding: "物理绑定", model_grain: "模型粒度", entity_key: "实体键", join_contract: "Join 合约" };

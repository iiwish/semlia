import { getAsset, getAssetRevision } from "./catalog";
import { knowledgeReferences, type KnowledgeSpec } from "./knowledge";
import { getProductionSnapshot, productionAPI, ProductionApiError, productionMessage, targetChanges, type AssetContent, type ProductionDraft, type ProductionOperation } from "./semanticProduction";
import type { KnowledgeRevisionSubmission } from "./types";

function canonical(value: unknown): string {
  return JSON.stringify(value, (_key, item: unknown) => item && typeof item === "object" && !Array.isArray(item) ? Object.fromEntries(Object.entries(item).sort(([a], [b]) => a.localeCompare(b))) : item) ?? "undefined";
}

export function sameRevisionSubmission(a: KnowledgeRevisionSubmission, b: KnowledgeRevisionSubmission) {
  return canonical(a) === canonical(b);
}

export async function prepareKnowledgeProductionRevision(workspaceId: string, submission: KnowledgeRevisionSubmission, signal?: AbortSignal): Promise<ProductionDraft> {
  const detail = await getAsset(workspaceId, submission.assetId, signal);
  signal?.throwIfAborted();
  const baseline = detail.currentRevision;
  if (detail.id !== submission.assetId || !baseline || baseline.assetId !== submission.assetId || baseline.id !== submission.baseRevisionId || detail.currentRevisionId !== baseline.id) throw new Error("知识基线已变化，请重新打开当前知识后核对修订。");
  const released = detail.authoritySections.find((section) => section.kind === "released_state");
  if (released?.availability !== "available" || !released.releaseId || released.revisionId !== baseline.id) throw new Error("当前修订不是可追溯的已发布基线；请先处理现有草稿，不会自动改用其他版本。");
  const revision = await getAssetRevision(workspaceId, submission.assetId, baseline.id, signal);
  signal?.throwIfAborted();
  if (revision.id !== baseline.id || revision.assetId !== detail.id || revision.contentDigest !== baseline.contentDigest) throw new Error("知识修订的不可变内容与当前基线不匹配。");
  const content = revision.content;
  if (content.address !== detail.address || content.assetType !== detail.assetType || typeof content.displayName !== "string" || typeof content.ownerPrincipalId !== "string" || !content.spec || typeof content.spec !== "object" || Array.isArray(content.spec)) throw new Error("此知识缺少规范生产内容，不能创建生产修订。");
  const desired = structuredClone(content);
  const changed = new Set<string>();
  for (const change of submission.changes) {
    if (!["definition", "spec"].includes(change.field) || changed.has(change.field) || canonical(content[change.field]) !== canonical(change.before)) throw new Error("修订字段或原值与编辑基线不匹配，请重新核对。");
    if (change.field === "definition" ? typeof change.after !== "string" : !change.after || typeof change.after !== "object" || Array.isArray(change.after)) throw new Error("修订内容不符合规范字段类型。");
    desired[change.field] = structuredClone(change.after);
    changed.add(change.field);
  }
  const changes = targetChanges(content as AssetContent, desired as AssetContent);
  if (!changes.length) throw new Error("没有需要保存的知识变化。");

  let releaseId: string | undefined = released.releaseId;
  const visited = new Set<string>();
  let origin: { operation: ProductionOperation; evidenceIds: string[] } | undefined;
  for (let depth = 0; releaseId && depth < 64; depth++) {
    signal?.throwIfAborted();
    if (visited.has(releaseId)) throw new Error("发布来源链存在循环，不能确认修订依据。");
    visited.add(releaseId);
    const release = await productionAPI.release(workspaceId, releaseId, signal);
    if (release.id !== releaseId) throw new Error("发布来源标识不匹配。");
    // A rollback restores existing pins; its attribution did not produce that revision.
    if (release.attribution.role === "applied" && release.protection.rollbackDepth === 0 && !release.rolledBackReleaseId && release.afterManifest.assets.some((pin) => pin.assetId === detail.id && pin.revisionId === baseline.id)) {
      const attribution = release.attribution;
      const operation = await productionAPI.get(workspaceId, attribution.operationId, signal, attribution.version);
      if (operation.summary.id !== attribution.operationId || operation.version !== attribution.version || operation.setDigest !== attribution.setDigest) throw new Error("生产来源版本与发布归因不匹配。");
      const target = operation.targets.find((item) => item.targetId === detail.id && item.declaration.kind === "semantic_asset");
      if (target?.outcome === "proposal") {
        if (target.contentDigest !== revision.contentDigest || !target.proposalId || !attribution.proposalIds.includes(target.proposalId)) throw new Error("生产对象与指定知识修订不匹配。");
        origin = { operation, evidenceIds: target.declaration.evidenceIds };
        break;
      }
    }
    releaseId = release.beforeHead.presence === "present" ? release.beforeHead.releaseId : undefined;
  }
  if (!origin) throw new Error("未找到指定知识修订的可信生产来源（最多回溯 64 次）。");
  const input = structuredClone(origin.operation.input);
  if (!input.snapshots.length) throw new Error("生产来源没有固定快照。");
  for (const pin of input.snapshots) {
    const snapshot = await getProductionSnapshot(workspaceId, pin.sourceId, pin.snapshotId, signal);
    signal?.throwIfAborted();
    if (snapshot.id !== pin.snapshotId || snapshot.sourceId !== pin.sourceId || snapshot.contentDigest !== pin.digest || snapshot.historyQuality !== "verified" || !pin.coverageKeys.length || !pin.coverageKeys.every((key) => snapshot.coverage.some((unit) => unit.key === key && unit.status === "complete" && unit.enumerationComplete))) throw new Error("固定来源快照或覆盖无法验证，未创建修订。");
  }
  input.candidates = [];
  const dependencies = new Map<string, (typeof input.dependencies)[number]>();
  for (const dependency of [...input.dependencies, ...knowledgeReferences(desired.spec as KnowledgeSpec).map((ref) => ({ kind: "semantic_asset" as const, targetId: ref.assetId, revisionId: ref.revisionId, releaseId: ref.releaseId }))]) {
    const key = `${dependency.kind}:${dependency.targetId}`;
    const existing = dependencies.get(key);
    if (existing && canonical(existing) !== canonical(dependency)) throw new Error("依赖的固定版本冲突，不能自动改用最新版本。");
    dependencies.set(key, dependency);
  }
  input.dependencies = [...dependencies.values()];
  return { input, targets: [{ intent: "update", kind: "semantic_asset", localKey: "revision", title: content.displayName, targetId: detail.id, baseRevisionId: baseline.id, content: desired as AssetContent, changes, evidenceIds: [...origin.evidenceIds] }] };
}

export class KnowledgeRevisionUncertainError extends Error {
  constructor(detail?: string) { super(`${detail ? `${detail}。` : ""}保存结果尚未确认。请重试同一保存请求，不要修改或新建修订。`); this.name = "KnowledgeRevisionUncertainError"; }
}

export function createKnowledgeRevisionCommand(workspaceId: string, submission: KnowledgeRevisionSubmission, signal?: AbortSignal) {
  const frozen = structuredClone(submission);
  const key = `web-revision-${crypto.randomUUID()}`;
  let draft: ProductionDraft | undefined;
  let result: Awaited<ReturnType<typeof productionAPI.create>> | undefined;
  let uncertain = false;
  let pending: Promise<Awaited<ReturnType<typeof productionAPI.create>>> | undefined;
  const run = () => {
    if (result) return Promise.resolve(result);
    if (pending) return pending;
    pending = (async () => {
      draft ??= await prepareKnowledgeProductionRevision(workspaceId, frozen, signal);
      signal?.throwIfAborted();
      try {
        const response = await productionAPI.create(workspaceId, structuredClone(draft), key, signal);
        signal?.throwIfAborted();
        if (!response.operationId) throw new TypeError("保存响应缺少生产记录标识");
        result = response;
        return response;
      } catch (error) {
        if (!signal?.aborted && (uncertain || !(error instanceof ProductionApiError) || error.status < 400 || error.status >= 500 || error.status === 408 || error.code === "IDEMPOTENCY_IN_PROGRESS")) {
          uncertain = true;
          throw new KnowledgeRevisionUncertainError(error instanceof ProductionApiError ? productionMessage(error) : undefined);
        }
        throw error;
      }
    })().finally(() => { pending = undefined; });
    return pending;
  };
  return { submission: frozen, run };
}

import { OperationsApiError, type OperationsApi, type OperationsAuditEvent, type OperationsAuditFilter, type OperationsPage, type OperationsRunFilter, type OperationsRuntimeRun, type OperationsRuntimeRunDetail, type OperationsRuntimePolicy } from "../operations";

export function createOperationsFixtureApi(): OperationsApi {
  let runs = fixtureRuns.map(cloneRun);
  let policy = clonePolicy(fixturePolicy);
  let exportSequence = 0;
  const exportContents = new Map<string, { content: string; contentDigest: string }>();

  return {
    async listAuditEvents(_workspaceId, filter = {}, cursor) {
      const filtered = fixtureAuditEvents.filter((item) => matchesAudit(item, filter));
      return fixturePage(filtered, cursor);
    },
    async createAuditExport(_workspaceId, filter) {
      exportSequence += 1;
      const id = `audit-export-${exportSequence}`;
      const items = fixtureAuditEvents.filter((item) => matchesAudit(item, filter));
      const contentDigest = String(exportSequence).padStart(64, "0");
      exportContents.set(id, { content: JSON.stringify(items), contentDigest });
      return { id, runtimeRunId: `run-audit-export-${exportSequence}`, artifactId: `artifact-audit-export-${exportSequence}`, format: "json", rowCount: items.length, contentDigest, createdAt: "2026-09-01T12:30:00Z", expiresAt: "2026-09-02T12:30:00Z" };
    },
    async downloadAuditExport(_workspaceId, exportId) {
      const stored = exportContents.get(exportId);
      if (!stored) throw new OperationsApiError("审计导出不存在或已过期。", 404, "NOT_FOUND");
      return { ...stored, contentType: "application/json", filename: `audit-export-${exportId}.json` };
    },
    async listRuns(_workspaceId, filter = {}, cursor) {
      const filtered = runs.filter((item) => matchesRun(item, filter));
      return fixturePage(filtered, cursor);
    },
    async getRun(_workspaceId, runId) {
      const run = runs.find((item) => item.id === runId);
      if (!run) throw new OperationsApiError("运行记录不存在。", 404, "NOT_FOUND");
      return { run: cloneRun(run), events: (fixtureRunEvents[runId] ?? []).map((item) => ({ ...item })) };
    },
    async retryRun(_workspaceId, runId) {
      const run = runs.find((item) => item.id === runId);
      if (!run?.capabilities.retry) throw new OperationsApiError("该运行不支持重试。", 409, "UNSUPPORTED_OPERATION");
      runs = runs.map((item) => item.id === runId ? { ...item, state: "queued", phase: "等待重试", attempt: item.attempt + 1, capabilities: { retry: false, cancel: true }, version: item.version + 1, updatedAt: "2026-09-01T12:31:00Z" } : item);
    },
    async cancelRun(_workspaceId, runId) {
      const run = runs.find((item) => item.id === runId);
      if (!run?.capabilities.cancel) throw new OperationsApiError("该运行不支持取消。", 409, "UNSUPPORTED_OPERATION");
      runs = runs.map((item) => item.id === runId ? { ...item, state: "cancelled", phase: "已取消", capabilities: { retry: false, cancel: false }, version: item.version + 1, finishedAt: "2026-09-01T12:31:00Z", updatedAt: "2026-09-01T12:31:00Z" } : item);
    },
    async getPolicy() {
      return clonePolicy(policy);
    },
    async updatePolicy(_workspaceId, input) {
      if (input.expectedVersion !== policy.settings.version) throw new OperationsApiError("设置版本已更新。", 409, "VERSION_CONFLICT");
      policy = { ...policy, settings: { ...input, version: input.expectedVersion + 1, updatedAt: "2026-09-01T12:31:00Z" } };
      return clonePolicy(policy);
    },
  };
}

function fixturePage<T>(items: T[], cursor?: string): OperationsPage<T> {
  const offset = cursor ? Number(cursor) : 0;
  const limit = 3;
  const pageItems = items.slice(offset, offset + limit);
  const nextOffset = offset + pageItems.length;
  return { items: pageItems, limit, nextCursor: nextOffset < items.length ? String(nextOffset) : undefined };
}

function matchesAudit(item: OperationsAuditEvent, filter: OperationsAuditFilter): boolean {
  return (!filter.actorId || item.actorId === filter.actorId)
    && (!filter.eventType || item.eventType === filter.eventType)
    && (!filter.objectType || item.objectType === filter.objectType)
    && (!filter.objectId || item.objectId === filter.objectId)
    && (!filter.traceId || item.traceId === filter.traceId)
    && (!filter.from || item.createdAt >= filter.from)
    && (!filter.to || item.createdAt <= filter.to);
}

function matchesRun(item: OperationsRuntimeRun, filter: OperationsRunFilter): boolean {
  return (!filter.kind || item.kind === filter.kind)
    && (!filter.state || item.state === filter.state)
    && (!filter.sourceType || item.sourceType === filter.sourceType)
    && (!filter.sourceId || item.sourceId === filter.sourceId)
    && (!filter.traceId || item.traceId === filter.traceId);
}

function cloneRun(run: OperationsRuntimeRun): OperationsRuntimeRun {
  return { ...run, capabilities: { ...run.capabilities } };
}

function clonePolicy(value: OperationsRuntimePolicy): OperationsRuntimePolicy {
  return { settings: { ...value.settings }, deployment: { ...value.deployment } };
}

const fixtureRuns: OperationsRuntimeRun[] = [
  { id: "RUN-IDX-260901-1132", kind: "embedding_rebuild", sourceType: "knowledge_index", sourceId: "知识目录向量索引重建", sourceVersionDigest: "sha256:fixture-index", jobId: "job-index-1", traceId: "tr_51a90d", idempotencyKey: "fixture-index-rebuild", requestedByPrincipalId: "prn-local-admin", state: "running", phase: "生成向量", progressCurrent: 1436, progressTotal: 3420, attempt: 1, maxAttempts: 3, capabilities: { retry: false, cancel: true }, version: 2, startedAt: "2026-09-01T11:32:00Z", createdAt: "2026-09-01T11:32:00Z", updatedAt: "2026-09-01T11:33:18Z" },
  { id: "RUN-240901-1208", kind: "discovery", sourceType: "source", sourceId: "PostgreSQL Analytics", sourceVersionDigest: "sha256:fixture-source", jobId: "job-discovery-1", traceId: "tr_8f31c2", idempotencyKey: "fixture-discovery", state: "running", phase: "生成候选版本", progressCurrent: 68, progressTotal: 100, attempt: 1, maxAttempts: 3, capabilities: { retry: false, cancel: true }, version: 4, startedAt: "2026-09-01T12:08:00Z", createdAt: "2026-09-01T12:08:00Z", updatedAt: "2026-09-01T12:11:42Z" },
  { id: "RUN-240901-1145", kind: "webhook_delivery", sourceType: "integration", sourceId: "commerce.orders_model", sourceVersionDigest: "sha256:fixture-webhook", idempotencyKey: "fixture-webhook", state: "queued", phase: "等待执行", attempt: 0, maxAttempts: 3, capabilities: { retry: false, cancel: true }, version: 1, createdAt: "2026-09-01T11:45:00Z", updatedAt: "2026-09-01T11:45:00Z" },
  { id: "RUN-240831-2214", kind: "validation", sourceType: "proposal", sourceId: "customer.customer", sourceVersionDigest: "sha256:fixture-validation", jobId: "job-validation-1", traceId: "tr_validation_failed", idempotencyKey: "fixture-validation", state: "failed", phase: "Cube 编译", attempt: 3, maxAttempts: 3, errorCode: "COMPILE_FAILED", errorSummary: "编译器返回安全摘要；原始输出未进入运行投影。", capabilities: { retry: true, cancel: false }, version: 5, startedAt: "2026-08-31T22:14:00Z", finishedAt: "2026-08-31T22:20:08Z", createdAt: "2026-08-31T22:14:00Z", updatedAt: "2026-08-31T22:20:08Z" },
];

const fixtureRunEvents: Record<string, OperationsRuntimeRunDetail["events"]> = {
  "RUN-IDX-260901-1132": [
    { id: "evt-run-index-1", sequence: 1, eventType: "state", state: "running", summary: "运行已由服务端接受。", createdAt: "2026-09-01T11:32:00Z" },
    { id: "evt-run-index-2", sequence: 2, eventType: "progress", phase: "生成向量", progressCurrent: 1436, progressTotal: 3420, summary: "服务端记录了批次进度。", createdAt: "2026-09-01T11:33:18Z" },
  ],
  "RUN-240901-1208": [{ id: "evt-run-discovery-1", sequence: 1, eventType: "phase", phase: "生成候选版本", summary: "服务端已进入候选版本阶段。", createdAt: "2026-09-01T12:11:42Z" }],
  "RUN-240831-2214": [{ id: "evt-run-validation-1", sequence: 1, eventType: "diagnostic", errorCode: "COMPILE_FAILED", summary: "编译器返回安全摘要；原始输出未进入运行投影。", createdAt: "2026-08-31T22:20:08Z" }],
};

const fixtureAuditEvents: OperationsAuditEvent[] = [
  { id: "AUD-98124", eventType: "runtime.discovery.started", actorId: "system-scheduler", objectType: "runtime_run", objectId: "RUN-240901-1208", channel: "system", outcome: "accepted", reasonCode: "SCHEDULED", traceId: "tr_8f31c2", summary: "系统调度创建了增量发现运行。", createdAt: "2026-09-01T12:10:42Z" },
  { id: "AUD-98123", eventType: "runtime.embedding_rebuild.started", actorId: "prn-local-admin", objectType: "knowledge_index", objectId: "知识目录向量索引重建", channel: "web", outcome: "accepted", reasonCode: "USER_REQUESTED", traceId: "tr_51a90d", summary: "用户请求重建知识目录向量索引。", createdAt: "2026-09-01T11:32:00Z" },
  { id: "AUD-98120", eventType: "semantic.resolve.completed", actorId: "client-codex-mcp", objectType: "semantic_asset", objectId: "commerce.net_revenue", channel: "mcp", outcome: "succeeded", reasonCode: "COMPLETED", traceId: "tr_d219a4", summary: "语义检索调用完成。", createdAt: "2026-09-01T11:18:36Z" },
  { id: "AUD-98108", eventType: "validation.failed", actorId: "prn-local-admin", objectType: "proposal", objectId: "commerce.average_order_value@9", channel: "web", outcome: "failed", reasonCode: "EVIDENCE_GAP", traceId: "tr_f0226e", summary: "发布验证发现证据覆盖不足。", createdAt: "2026-09-01T09:17:54Z" },
];

const fixturePolicy: OperationsRuntimePolicy = {
  settings: { retryCeiling: 3, statementTimeoutMs: 30_000, webhookTimeoutMs: 10_000, queryRowLimit: 10_000, queryByteLimit: 10_485_760, runMetadataRetentionDays: 90, version: 7, updatedAt: "2026-09-01T12:00:00Z" },
  deployment: { workerConfigured: true, telemetryConfigured: true, oidcConfigured: true, encryptionConfigured: true, auditRetention: "deployment_managed" },
};

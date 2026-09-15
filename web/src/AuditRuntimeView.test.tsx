import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import { AuditRuntimeView } from "./AuditRuntimeView";
import { OperationsRuntimeProvider, type OperationsAccess } from "./operationsRuntime";
import type { OperationsApi, OperationsRuntimeRun } from "./operations";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";

describe("Audit and Runtime view", () => {
  it("renders only persisted run facts and server events", async () => {
    const user = userEvent.setup();
    renderView({ api: makeApi(), access: { auditRead: false, runtimeRead: true, runtimeManage: false } });

    const row = await screen.findByRole("button", { name: "查看运行 proposal-42" });
    expect(row).toHaveTextContent("无进度数据");
    expect(screen.queryByText("日志投递")).not.toBeInTheDocument();
    expect(screen.queryByText("24 小时失败")).not.toBeInTheDocument();
    await user.click(row);

    const dialog = await screen.findByRole("dialog", { name: "proposal-42" });
    expect(within(dialog).getAllByText("服务端诊断摘要")).toHaveLength(2);
    expect(within(dialog).queryByText("任务创建")).not.toBeInTheDocument();
    expect(within(dialog).getByText("没有可计算的进度")).toBeVisible();
  });

  it("partitions audit-only access without requesting runtime data", async () => {
    const api = makeApi();
    renderView({ api, access: { auditRead: true, runtimeRead: false, runtimeManage: false } });

    expect(await screen.findByRole("tab", { name: "审计日志" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByRole("tab", { name: "运行记录" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "运行设置" })).not.toBeInTheDocument();
    expect(api.listRuns).not.toHaveBeenCalled();
    expect(api.getPolicy).not.toHaveBeenCalled();
  });

  it("shows an honest manage-only context requirement", () => {
    const api = makeApi();
    renderView({ api, access: { auditRead: false, runtimeRead: false, runtimeManage: true } });

    expect(screen.getByText("需要运行读取上下文")).toBeVisible();
    expect(screen.getByText(/runtime\.read/)).toBeVisible();
    expect(api.listRuns).not.toHaveBeenCalled();
    expect(api.getPolicy).not.toHaveBeenCalled();
  });

  it("downloads a server-confirmed export only after digest verification", async () => {
    const user = userEvent.setup();
    const api = makeApi();
    const createObjectURL = vi.fn().mockReturnValue("blob:audit-export");
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, "createObjectURL", { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, "revokeObjectURL", { configurable: true, value: revokeObjectURL });
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    renderView({ api, access: { auditRead: true, runtimeRead: false, runtimeManage: false } });

    await screen.findByText("runtime.run.failed");
    await user.click(screen.getByRole("button", { name: "创建审计导出" }));
    const record = await screen.findByRole("status", { name: "审计导出状态" });
    expect(record).toHaveTextContent("artifact-1");
    expect(record).toHaveTextContent("a".repeat(64));
    await user.click(within(record).getByRole("button", { name: "下载 JSON" }));
    expect(await within(record).findByText("内容已由服务端返回")).toBeVisible();
    expect(api.downloadAuditExport).toHaveBeenCalledWith(workspaceId, "export-1");
    expect(createObjectURL).toHaveBeenCalledOnce();
  });

  it("keeps a confirmed export usable when the audit-list refresh fails", async () => {
    const listAuditEvents = vi.fn()
      .mockResolvedValueOnce({ items: [auditEvent()], limit: 50 })
      .mockRejectedValueOnce(new Error("审计列表刷新暂时不可用。"));
    const api = makeApi({ listAuditEvents });
    renderView({ api, access: { auditRead: true, runtimeRead: false, runtimeManage: false } });

    await screen.findByText("runtime.run.failed");
    await userEvent.click(screen.getByRole("button", { name: "创建审计导出" }));
    const record = await screen.findByRole("status", { name: "审计导出状态" });
    expect(record).toHaveTextContent("服务端已创建导出记录");
    expect(await within(record).findByRole("alert")).toHaveTextContent("导出记录已保留");
    expect(within(record).getByRole("button", { name: "下载 JSON" })).toBeEnabled();
  });

  it("reports a forbidden audit slice without hiding an available runtime slice", async () => {
    const forbidden = Object.assign(new Error("审计事件访问被拒绝。"), { status: 403, code: "FORBIDDEN" });
    renderView({ api: makeApi({ listAuditEvents: vi.fn().mockRejectedValue(forbidden) }), access: { auditRead: true, runtimeRead: true, runtimeManage: false } });

    await screen.findByRole("button", { name: "查看运行 proposal-42" });
    await userEvent.click(screen.getByRole("tab", { name: "审计日志" }));
    expect(await screen.findByText("审计事件访问被拒绝。")).toBeVisible();
    await userEvent.click(screen.getByRole("tab", { name: "运行记录" }));
    expect(screen.getByRole("button", { name: "查看运行 proposal-42" })).toBeVisible();
  });
});

function renderView({ api, access }: { api: OperationsApi; access: OperationsAccess }) {
  render(<OperationsRuntimeProvider workspaceId={workspaceId} api={api} access={access}>
    <AuditRuntimeView onInitialRunHandled={() => {}} onNotify={() => {}} />
  </OperationsRuntimeProvider>);
}

function makeApi(overrides: Partial<OperationsApi> = {}): OperationsApi {
  return {
    listAuditEvents: vi.fn().mockResolvedValue({ items: [auditEvent()], limit: 50 }),
    createAuditExport: vi.fn().mockResolvedValue(exportRecord()),
    downloadAuditExport: vi.fn().mockResolvedValue({ content: JSON.stringify([auditEvent()]), contentDigest: "a".repeat(64), contentType: "application/json", filename: "audit.json" }),
    listRuns: vi.fn().mockResolvedValue({ items: [run()], limit: 50 }),
    getRun: vi.fn().mockResolvedValue({ run: run(), events: [{ id: "run-event-1", sequence: 1, eventType: "diagnostic", errorCode: "VALIDATION_FAILED", summary: "服务端诊断摘要", createdAt: "2026-09-01T12:01:00Z" }] }),
    retryRun: vi.fn().mockResolvedValue(undefined),
    cancelRun: vi.fn().mockResolvedValue(undefined),
    getPolicy: vi.fn().mockResolvedValue(policy()),
    updatePolicy: vi.fn().mockResolvedValue(policy(8)),
    ...overrides,
  };
}

function run(overrides: Partial<OperationsRuntimeRun> = {}): OperationsRuntimeRun {
  return { id: "run-1", kind: "validation", sourceType: "proposal", sourceId: "proposal-42", sourceVersionDigest: "sha256:v1", traceId: "trace-run-1", idempotencyKey: "idem-run-1", state: "failed", phase: "validation", attempt: 1, maxAttempts: 3, errorCode: "VALIDATION_FAILED", errorSummary: "服务端诊断摘要", capabilities: { retry: true, cancel: false }, version: 1, createdAt: "2026-09-01T12:00:00Z", updatedAt: "2026-09-01T12:01:00Z", ...overrides };
}

function auditEvent() {
  return { id: "evt-1", eventType: "runtime.run.failed", actorId: "principal-1", objectType: "runtime_run", objectId: "run-1", channel: "web", outcome: "failed", reasonCode: "PROVIDER_FAILURE", traceId: "trace-1", summary: "运行失败。", createdAt: "2026-09-01T12:00:00Z" };
}

function exportRecord() {
  return { id: "export-1", runtimeRunId: "run-export-1", artifactId: "artifact-1", format: "json" as const, rowCount: 1, contentDigest: "a".repeat(64), createdAt: "2026-09-01T12:00:00Z", expiresAt: "2026-09-02T12:00:00Z" };
}

function policy(version = 7) {
  return { settings: { retryCeiling: 3, statementTimeoutMs: 30_000, webhookTimeoutMs: 10_000, queryRowLimit: 10_000, queryByteLimit: 10_485_760, runMetadataRetentionDays: 90, version, updatedAt: "2026-09-01T12:00:00Z" }, deployment: { workerConfigured: true, telemetryConfigured: true, oidcConfigured: true, encryptionConfigured: true, auditRetention: "deployment_managed" as const } };
}

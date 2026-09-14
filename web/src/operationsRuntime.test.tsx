import { useState } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, vi } from "vitest";

import { clearSessionCSRFToken } from "./apiClient";
import { getSession } from "./identity";
import { operationsApi, OperationsApiError, type OperationsApi } from "./operations";
import { OperationsRuntimeProvider, useOperationsRuntime } from "./operationsRuntime";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const fullAccess = { auditRead: true, runtimeRead: true, runtimeManage: true };

afterEach(() => {
  vi.unstubAllGlobals();
  clearSessionCSRFToken();
});

describe("operations client", () => {
  it("uses exact cursor filters plus CSRF and versioned mutation bodies", async () => {
    const requests: Request[] = [];
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      requests.push(request);
      if (request.url.endsWith("/api/v1/session")) return sessionResponse();
      if (request.url.includes("/operations/audit-events")) return json({ items: [auditEvent()], page: { limit: 25, nextCursor: "next-audit" } });
      if (request.url.endsWith("/operations/audit-exports")) return json(exportRecord(), 201);
      if (request.url.endsWith("/operations/audit-exports/export-1/content")) return new Response(JSON.stringify([auditEvent()]), { status: 200, headers: { "Content-Type": "application/json", "X-Content-SHA256": "a".repeat(64), "Content-Disposition": "attachment; filename=server-audit.json" } });
      return json(policy(8));
    }));

    await getSession();
    const page = await operationsApi.listAuditEvents(workspaceId, { actorId: "principal-1", eventType: "runtime.run.failed", traceId: "trace-1" }, "cursor-audit");
    await operationsApi.createAuditExport(workspaceId, { actorId: "principal-1" }, "export-key-1");
    const download = await operationsApi.downloadAuditExport(workspaceId, "export-1");
    await operationsApi.updatePolicy(workspaceId, { expectedVersion: 7, retryCeiling: 3, statementTimeoutMs: 30_000, webhookTimeoutMs: 10_000, queryRowLimit: 10_000, queryByteLimit: 10_485_760, runMetadataRetentionDays: 90 });

    expect(page).toMatchObject({ nextCursor: "next-audit", limit: 25 });
    const listURL = new URL(requests.find((request) => request.url.includes("/operations/audit-events"))!.url);
    expect(Object.fromEntries(listURL.searchParams)).toMatchObject({ limit: "50", cursor: "cursor-audit", actorId: "principal-1", eventType: "runtime.run.failed", traceId: "trace-1" });
    const exportRequest = requests.find((request) => request.url.endsWith("/operations/audit-exports"))!;
    expect(exportRequest.headers.get("X-Semlia-CSRF")).toBe("csrf-operations-tests-1234567890");
    expect(await exportRequest.json()).toEqual({ idempotencyKey: "export-key-1", filter: { actorId: "principal-1" } });
    expect(download).toMatchObject({ contentDigest: "a".repeat(64), filename: "server-audit.json" });
    expect(JSON.parse(download.content)).toEqual([auditEvent()]);
    expect(await requests.find((request) => request.url.endsWith("/runtime-policy") && request.method === "PATCH")!.json()).toMatchObject({ expectedVersion: 7 });
  });

  it("keeps server conflicts explicit", async () => {
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => request.url.endsWith("/api/v1/session")
      ? sessionResponse()
      : json({ code: "VERSION_CONFLICT", message: "设置版本已更新。", traceId: "trace-conflict", details: {} }, 409)));
    await getSession();
    await expect(operationsApi.updatePolicy(workspaceId, { expectedVersion: 4, retryCeiling: 2, statementTimeoutMs: 30_000, webhookTimeoutMs: 10_000, queryRowLimit: 5_000, queryByteLimit: 1_048_576, runMetadataRetentionDays: 30 }))
      .rejects.toMatchObject({ status: 409, code: "VERSION_CONFLICT", traceId: "trace-conflict" } satisfies Partial<OperationsApiError>);
  });
});

describe("operations runtime", () => {
  it("requests only capability-authorized resources", async () => {
    const api = makeApi();
    render(<OperationsRuntimeProvider workspaceId={workspaceId} api={api} access={{ auditRead: true, runtimeRead: false, runtimeManage: false }}><Probe /></OperationsRuntimeProvider>);

    await screen.findByText("evt-1");
    expect(api.listAuditEvents).toHaveBeenCalledOnce();
    expect(api.listRuns).not.toHaveBeenCalled();
    expect(api.getPolicy).not.toHaveBeenCalled();
  });

  it("keeps independent data when another capability surface is forbidden", async () => {
    const forbidden = Object.assign(new Error("没有审计读取权限。"), { status: 403, code: "FORBIDDEN" });
    const api = makeApi({ listAuditEvents: vi.fn().mockRejectedValue(forbidden) });
    render(<OperationsRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><Probe /></OperationsRuntimeProvider>);

    await waitFor(() => expect(screen.getByTestId("audit-state")).toHaveTextContent("forbidden"));
    expect(screen.getByText("run-1")).toBeVisible();
    expect(screen.getByTestId("runs-state")).toHaveTextContent("ready");
    expect(screen.getByText("没有审计读取权限。")).toBeVisible();
  });

  it("appends cursor pages without losing existing records when append fails", async () => {
    const listRuns = vi.fn()
      .mockResolvedValueOnce({ items: [run("run-1")], nextCursor: "cursor-2", limit: 50 })
      .mockResolvedValueOnce({ items: [run("run-1"), run("run-2")], nextCursor: "cursor-3", limit: 50 })
      .mockRejectedValueOnce(new Error("下一页读取失败。"));
    const api = makeApi({ listRuns });
    const user = userEvent.setup();
    render(<OperationsRuntimeProvider workspaceId={workspaceId} api={api} access={{ auditRead: false, runtimeRead: true, runtimeManage: false }}><Probe /></OperationsRuntimeProvider>);

    await screen.findByText("run-1");
    await user.click(screen.getByRole("button", { name: "加载更多运行" }));
    expect(await screen.findByText("run-2")).toBeVisible();
    expect(screen.getAllByText("run-1")).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: "加载更多运行" }));
    expect(await screen.findByText("下一页读取失败。")).toBeVisible();
    expect(screen.getByText("run-1")).toBeVisible();
  });

  it("uses authoritative run events and blocks unsupported commands", async () => {
    const api = makeApi({
      getRun: vi.fn().mockResolvedValue({ run: run("run-1", { capabilities: { retry: false, cancel: false } }), events: [runEvent(2, "服务端诊断"), runEvent(1, "服务端开始")] }),
    });
    const user = userEvent.setup();
    render(<OperationsRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><Probe /></OperationsRuntimeProvider>);

    await screen.findByText("run-1");
    await user.click(screen.getByRole("button", { name: "打开运行" }));
    expect(await screen.findByText("服务端开始")).toBeVisible();
    expect(screen.getByText("服务端诊断")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "尝试重试" }));
    expect(screen.getByTestId("command-error")).toHaveTextContent("UNSUPPORTED_OPERATION");
    expect(api.retryRun).not.toHaveBeenCalled();
  });

  it("confirms settings only after PATCH and authoritative GET refetch", async () => {
    const getPolicy = vi.fn().mockResolvedValueOnce(policy(7)).mockResolvedValueOnce(policy(8));
    const api = makeApi({ getPolicy, updatePolicy: vi.fn().mockResolvedValue(policy(8)) });
    const user = userEvent.setup();
    render(<OperationsRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><MutationProbe /></OperationsRuntimeProvider>);

    await waitFor(() => expect(screen.getByTestId("policy-version")).toHaveTextContent("7"));
    await user.click(screen.getByRole("button", { name: "保存设置" }));
    await waitFor(() => expect(screen.getByTestId("policy-version")).toHaveTextContent("8"));
    expect(api.updatePolicy).toHaveBeenCalledOnce();
    expect(getPolicy).toHaveBeenCalledTimes(2);
  });

  it("keeps digest mismatch and version conflict as explicit command failures", async () => {
    const versionConflict = Object.assign(new Error("设置版本已更新。"), { status: 409, code: "VERSION_CONFLICT" });
    const api = makeApi({
      downloadAuditExport: vi.fn().mockResolvedValue({ content: "[]", contentDigest: "b".repeat(64), contentType: "application/json", filename: "audit.json" }),
      updatePolicy: vi.fn().mockRejectedValue(versionConflict),
    });
    const user = userEvent.setup();
    render(<OperationsRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><FailureProbe /></OperationsRuntimeProvider>);

    await waitFor(() => expect(screen.getByTestId("failure-policy-version")).toHaveTextContent("7"));
    await user.click(screen.getByRole("button", { name: "创建导出" }));
    await waitFor(() => expect(screen.getByTestId("created-export")).toHaveTextContent("export-1"));
    await user.click(screen.getByRole("button", { name: "下载导出" }));
    expect(await screen.findByTestId("download-failure")).toHaveTextContent("conflict:EXPORT_DIGEST_MISMATCH");
    await user.click(screen.getByRole("button", { name: "保存冲突设置" }));
    expect(await screen.findByTestId("settings-failure")).toHaveTextContent("conflict:VERSION_CONFLICT");
  });

  it("retains a confirmed export when the follow-up audit refresh fails", async () => {
    let rejectRefresh: (reason: Error) => void = () => undefined;
    const listAuditEvents = vi.fn()
      .mockResolvedValueOnce({ items: [auditEvent()], limit: 50 })
      .mockImplementationOnce(() => new Promise((_, reject) => { rejectRefresh = reject; }));
    const api = makeApi({ listAuditEvents });
    const user = userEvent.setup();
    render(<OperationsRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><ExportRefreshProbe /></OperationsRuntimeProvider>);

    await screen.findByText("evt-1");
    await user.click(screen.getByRole("button", { name: "创建待刷新导出" }));
    await waitFor(() => expect(screen.getByTestId("retained-export")).toHaveTextContent("confirmed:export-1"));

    await act(async () => rejectRefresh(new Error("刷新暂时不可用。")));
    expect(await screen.findByTestId("export-refresh-warning")).toHaveTextContent("导出已创建，但审计事件刷新失败：刷新暂时不可用。");
    expect(screen.getByTestId("retained-export")).toHaveTextContent("confirmed:export-1");
    expect(screen.getByTestId("export-command-error")).toBeEmptyDOMElement();
  });
});

function Probe() {
  const runtime = useOperationsRuntime();
  return <div>
    <output data-testid="audit-state">{runtime.auditState}</output>
    <output data-testid="runs-state">{runtime.runsState}</output>
    <output data-testid="command-error">{runtime.runCommandErrorCode}</output>
    <span>{runtime.auditError}</span><span>{runtime.runsAppendError}</span>
    {runtime.auditEvents.map((item) => <span key={item.id}>{item.id}</span>)}
    {runtime.runs.map((item) => <span key={item.id}>{item.id}</span>)}
    {runtime.runDetail?.events.map((item) => <span key={item.id}>{item.summary}</span>)}
    {runtime.runsNextCursor && <button type="button" onClick={() => void runtime.loadMoreRuns()}>加载更多运行</button>}
    <button type="button" onClick={() => void runtime.openRun("run-1")}>打开运行</button>
    <button type="button" onClick={() => void runtime.retryRun("run-1").catch(() => undefined)}>尝试重试</button>
  </div>;
}

function MutationProbe() {
  const runtime = useOperationsRuntime();
  const [result, setResult] = useState("");
  return <div>
    <output data-testid="policy-version">{runtime.policy?.settings.version}</output>
    <output>{result}</output>
    <button type="button" onClick={async () => {
      await runtime.updateSettings({ retryCeiling: 4, statementTimeoutMs: 40_000, webhookTimeoutMs: 12_000, queryRowLimit: 20_000, queryByteLimit: 20_971_520, runMetadataRetentionDays: 120 });
      setResult("done");
    }}>保存设置</button>
  </div>;
}

function FailureProbe() {
  const runtime = useOperationsRuntime();
  return <div>
    <output data-testid="failure-policy-version">{runtime.policy?.settings.version}</output>
    <output data-testid="created-export">{runtime.exportRecord?.id}</output>
    <output data-testid="download-failure">{runtime.downloadState}:{runtime.downloadErrorCode}</output>
    <output data-testid="settings-failure">{runtime.settingsCommandState}:{runtime.settingsCommandErrorCode}</output>
    <button type="button" onClick={() => void runtime.createAuditExport().catch(() => undefined)}>创建导出</button>
    <button type="button" onClick={() => void runtime.downloadAuditExport().catch(() => undefined)}>下载导出</button>
    <button type="button" onClick={async () => { try { await runtime.updateSettings({ retryCeiling: 4, statementTimeoutMs: 40_000, webhookTimeoutMs: 12_000, queryRowLimit: 20_000, queryByteLimit: 20_971_520, runMetadataRetentionDays: 120 }); } catch { /* surfaced by runtime */ } }}>保存冲突设置</button>
  </div>;
}

function ExportRefreshProbe() {
  const runtime = useOperationsRuntime();
  return <div>
    {runtime.auditEvents.map((item) => <span key={item.id}>{item.id}</span>)}
    <output data-testid="retained-export">{runtime.exportState}:{runtime.exportRecord?.id}</output>
    <output data-testid="export-command-error">{runtime.exportError}</output>
    <output data-testid="export-refresh-warning">{runtime.exportRefreshWarning}</output>
    <button type="button" onClick={() => void runtime.createAuditExport()}>创建待刷新导出</button>
  </div>;
}

function makeApi(overrides: Partial<OperationsApi> = {}): OperationsApi {
  return {
    listAuditEvents: vi.fn().mockResolvedValue({ items: [auditEvent()], limit: 50 }),
    createAuditExport: vi.fn().mockResolvedValue(exportRecord()),
    downloadAuditExport: vi.fn().mockResolvedValue({ content: JSON.stringify([auditEvent()]), contentDigest: "a".repeat(64), contentType: "application/json", filename: "audit.json" }),
    listRuns: vi.fn().mockResolvedValue({ items: [run("run-1")], limit: 50 }),
    getRun: vi.fn().mockResolvedValue({ run: run("run-1"), events: [] }),
    retryRun: vi.fn().mockResolvedValue(undefined),
    cancelRun: vi.fn().mockResolvedValue(undefined),
    getPolicy: vi.fn().mockResolvedValue(policy(7)),
    updatePolicy: vi.fn().mockResolvedValue(policy(8)),
    ...overrides,
  };
}

function auditEvent() {
  return { id: "evt-1", eventType: "runtime.run.failed", actorId: "principal-1", objectType: "runtime_run", objectId: "run-1", channel: "web", outcome: "failed", reasonCode: "PROVIDER_FAILURE", traceId: "trace-1", summary: "运行失败。", createdAt: "2026-09-01T12:00:00Z" };
}

function run(id: string, overrides = {}) {
  return { id, kind: "validation" as const, sourceType: "proposal", sourceId: "proposal-1", sourceVersionDigest: "sha256:v1", idempotencyKey: `idem-${id}`, state: "failed" as const, phase: "validate", attempt: 1, maxAttempts: 3, capabilities: { retry: true, cancel: false }, version: 1, createdAt: "2026-09-01T12:00:00Z", updatedAt: "2026-09-01T12:01:00Z", ...overrides };
}

function runEvent(sequence: number, summary: string) {
  return { id: `event-${sequence}`, sequence, eventType: "diagnostic" as const, summary, createdAt: `2026-09-01T12:0${sequence}:00Z` };
}

function policy(version: number) {
  return { settings: { retryCeiling: 3, statementTimeoutMs: 30_000, webhookTimeoutMs: 10_000, queryRowLimit: 10_000, queryByteLimit: 10_485_760, runMetadataRetentionDays: 90, version, updatedAt: "2026-09-01T12:00:00Z" }, deployment: { workerConfigured: true, telemetryConfigured: true, oidcConfigured: true, encryptionConfigured: true, auditRetention: "deployment_managed" as const } };
}

function exportRecord() {
  return { id: "export-1", runtimeRunId: "run-export-1", artifactId: "artifact-1", format: "json" as const, rowCount: 1, contentDigest: "a".repeat(64), createdAt: "2026-09-01T12:00:00Z", expiresAt: "2026-09-02T12:00:00Z" };
}

function sessionResponse() {
  return json({ account: { id: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Admin" }, workspaces: [{ id: workspaceId, slug: "alpha", displayName: "Alpha", principalId: "prn_01arz3ndektsv4rrffq69g5fav", roleIds: ["workspace_admin"], capabilities: ["audit.read", "runtime.read", "runtime.manage"], authorizationVersion: 7 }], expiresAt: "2026-09-04T18:00:00Z", traceId: "trace-session" }, 200, { "X-Semlia-CSRF": "csrf-operations-tests-1234567890" });
}

function json(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

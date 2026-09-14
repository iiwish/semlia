/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import {
  createOperationsIdempotencyKey,
  operationsApi,
  OperationsApiError,
  type OperationsApi,
  type OperationsAuditEvent,
  type OperationsAuditExport,
  type OperationsAuditExportDownload,
  type OperationsAuditFilter,
  type OperationsRunFilter,
  type OperationsRuntimePolicy,
  type OperationsRuntimeRun,
  type OperationsRuntimeRunDetail,
  type UpdateOperationsRuntimeSettingsInput,
} from "./operations";

export type OperationsDataState = "idle" | "loading" | "ready" | "empty" | "forbidden" | "error";
export type OperationsCommandState = "idle" | "pending" | "confirmed" | "forbidden" | "conflict" | "unsupported" | "error";

export interface OperationsAccess {
  auditRead: boolean;
  runtimeRead: boolean;
  runtimeManage: boolean;
}

export interface OperationsRuntimeValue {
  workspaceId: string;
  access: OperationsAccess;
  auditEvents: OperationsAuditEvent[];
  auditFilter: OperationsAuditFilter;
  auditState: OperationsDataState;
  auditError: string;
  auditErrorCode: string;
  auditNextCursor?: string;
  auditLoadingMore: boolean;
  auditAppendError: string;
  runs: OperationsRuntimeRun[];
  runFilter: OperationsRunFilter;
  runsState: OperationsDataState;
  runsError: string;
  runsErrorCode: string;
  runsNextCursor?: string;
  runsLoadingMore: boolean;
  runsAppendError: string;
  policy: OperationsRuntimePolicy | null;
  policyState: OperationsDataState;
  policyError: string;
  policyErrorCode: string;
  selectedRunId: string | null;
  runDetail: OperationsRuntimeRunDetail | null;
  runDetailState: OperationsDataState;
  runDetailError: string;
  runDetailErrorCode: string;
  exportRecord: OperationsAuditExport | null;
  exportState: OperationsCommandState;
  exportError: string;
  exportErrorCode: string;
  exportRefreshWarning: string;
  downloadState: OperationsCommandState;
  downloadError: string;
  downloadErrorCode: string;
  settingsCommandState: OperationsCommandState;
  settingsCommandError: string;
  settingsCommandErrorCode: string;
  runCommandState: OperationsCommandState;
  runCommandError: string;
  runCommandErrorCode: string;
  refreshAudit: () => Promise<void>;
  applyAuditFilter: (filter: OperationsAuditFilter) => Promise<void>;
  loadMoreAudit: () => Promise<void>;
  refreshRuns: () => Promise<void>;
  applyRunFilter: (filter: OperationsRunFilter) => Promise<void>;
  loadMoreRuns: () => Promise<void>;
  refreshPolicy: () => Promise<void>;
  openRun: (runId: string) => Promise<void>;
  closeRun: () => void;
  createAuditExport: () => Promise<OperationsAuditExport>;
  downloadAuditExport: () => Promise<OperationsAuditExportDownload>;
  updateSettings: (input: UpdateOperationsRuntimeSettingsInput) => Promise<OperationsRuntimePolicy>;
  retryRun: (runId: string) => Promise<void>;
  cancelRun: (runId: string) => Promise<void>;
}

export type { OperationsApi } from "./operations";

const OperationsRuntimeContext = createContext<OperationsRuntimeValue | null>(null);

export function OperationsRuntimeProvider({ children, workspaceId, access, api: providedApi }: {
  children: ReactNode;
  workspaceId: string;
  access: OperationsAccess;
  api?: OperationsApi;
}) {
  const api = providedApi ?? operationsApi;
  const [auditEvents, setAuditEvents] = useState<OperationsAuditEvent[]>([]);
  const [auditFilter, setAuditFilter] = useState<OperationsAuditFilter>({});
  const [auditStatus, setAuditStatus] = useState(() => status(access.auditRead ? "loading" : "idle"));
  const [auditNextCursor, setAuditNextCursor] = useState<string>();
  const [auditLoadingMore, setAuditLoadingMore] = useState(false);
  const [auditAppendError, setAuditAppendError] = useState("");
  const [runs, setRuns] = useState<OperationsRuntimeRun[]>([]);
  const [runFilter, setRunFilter] = useState<OperationsRunFilter>({});
  const [runsStatus, setRunsStatus] = useState(() => status(access.runtimeRead ? "loading" : "idle"));
  const [runsNextCursor, setRunsNextCursor] = useState<string>();
  const [runsLoadingMore, setRunsLoadingMore] = useState(false);
  const [runsAppendError, setRunsAppendError] = useState("");
  const [policy, setPolicy] = useState<OperationsRuntimePolicy | null>(null);
  const [policyStatus, setPolicyStatus] = useState(() => status(access.runtimeRead ? "loading" : "idle"));
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [runDetail, setRunDetail] = useState<OperationsRuntimeRunDetail | null>(null);
  const [runDetailStatus, setRunDetailStatus] = useState(() => status("idle"));
  const [exportRecord, setExportRecord] = useState<OperationsAuditExport | null>(null);
  const [exportCommand, setExportCommand] = useState(() => command("idle"));
  const [exportRefreshWarning, setExportRefreshWarning] = useState("");
  const [downloadCommand, setDownloadCommand] = useState(() => command("idle"));
  const [settingsCommand, setSettingsCommand] = useState(() => command("idle"));
  const [runCommand, setRunCommand] = useState(() => command("idle"));
  const auditRequestSequence = useRef(0);
  const runsRequestSequence = useRef(0);
  const policyRequestSequence = useRef(0);
  const runDetailRequestSequence = useRef(0);
  const previousWorkspaceId = useRef(workspaceId);

  const loadAudit = useCallback(async (nextFilter: OperationsAuditFilter, signal?: AbortSignal, throwOnFailure = false) => {
    const requestSequence = ++auditRequestSequence.current;
    if (!access.auditRead) {
      setAuditEvents([]);
      setAuditNextCursor(undefined);
      setAuditStatus(status("idle"));
      return;
    }
    setAuditStatus(status("loading"));
    setAuditEvents([]);
    setAuditNextCursor(undefined);
    setAuditAppendError("");
    try {
      const page = await api.listAuditEvents(workspaceId, nextFilter, undefined, signal);
      if (signal?.aborted || requestSequence !== auditRequestSequence.current) return;
      setAuditEvents(page.items);
      setAuditNextCursor(page.nextCursor);
      setAuditStatus(status(page.items.length === 0 ? "empty" : "ready"));
      setExportRefreshWarning("");
    } catch (reason) {
      if (signal?.aborted || requestSequence !== auditRequestSequence.current) return;
      const detail = errorDetail(reason, "审计事件读取失败。");
      setAuditEvents([]);
      setAuditNextCursor(undefined);
      setAuditStatus(failureStatus(detail));
      if (throwOnFailure) throw reason;
    }
  }, [access.auditRead, api, workspaceId]);

  const loadRuns = useCallback(async (nextFilter: OperationsRunFilter, signal?: AbortSignal, throwOnFailure = false) => {
    const requestSequence = ++runsRequestSequence.current;
    if (!access.runtimeRead) {
      setRuns([]);
      setRunsNextCursor(undefined);
      setRunsStatus(status("idle"));
      return;
    }
    setRunsStatus(status("loading"));
    setRuns([]);
    setRunsNextCursor(undefined);
    setRunsAppendError("");
    try {
      const page = await api.listRuns(workspaceId, nextFilter, undefined, signal);
      if (signal?.aborted || requestSequence !== runsRequestSequence.current) return;
      setRuns(page.items);
      setRunsNextCursor(page.nextCursor);
      setRunsStatus(status(page.items.length === 0 ? "empty" : "ready"));
    } catch (reason) {
      if (signal?.aborted || requestSequence !== runsRequestSequence.current) return;
      const detail = errorDetail(reason, "运行记录读取失败。");
      setRuns([]);
      setRunsNextCursor(undefined);
      setRunsStatus(failureStatus(detail));
      if (throwOnFailure) throw reason;
    }
  }, [access.runtimeRead, api, workspaceId]);

  const loadPolicy = useCallback(async (signal?: AbortSignal, throwOnFailure = false) => {
    const requestSequence = ++policyRequestSequence.current;
    if (!access.runtimeRead) {
      setPolicy(null);
      setPolicyStatus(status("idle"));
      return;
    }
    setPolicyStatus(status("loading"));
    try {
      const nextPolicy = await api.getPolicy(workspaceId, signal);
      if (signal?.aborted || requestSequence !== policyRequestSequence.current) return;
      setPolicy(nextPolicy);
      setPolicyStatus(status("ready"));
    } catch (reason) {
      if (signal?.aborted || requestSequence !== policyRequestSequence.current) return;
      const detail = errorDetail(reason, "运行策略读取失败。");
      setPolicy(null);
      setPolicyStatus(failureStatus(detail));
      if (throwOnFailure) throw reason;
    }
  }, [access.runtimeRead, api, workspaceId]);

  useEffect(() => {
    if (previousWorkspaceId.current === workspaceId) return;
    previousWorkspaceId.current = workspaceId;
    setAuditEvents([]);
    setRuns([]);
    setPolicy(null);
    setRunDetail(null);
    setSelectedRunId(null);
    setExportRecord(null);
    setExportCommand(command("idle"));
    setExportRefreshWarning("");
    setDownloadCommand(command("idle"));
  }, [workspaceId]);

  useEffect(() => {
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      void loadAudit({}, controller.signal);
      void loadRuns({}, controller.signal);
      void loadPolicy(controller.signal);
    }, 0);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [loadAudit, loadPolicy, loadRuns]);

  const refreshAudit = useCallback(() => loadAudit(auditFilter), [auditFilter, loadAudit]);
  const applyAuditFilter = useCallback(async (filter: OperationsAuditFilter) => {
    setAuditFilter(filter);
    setExportRecord(null);
    setExportCommand(command("idle"));
    setExportRefreshWarning("");
    setDownloadCommand(command("idle"));
    await loadAudit(filter);
  }, [loadAudit]);
  const loadMoreAudit = useCallback(async () => {
    if (!access.auditRead || !auditNextCursor || auditLoadingMore) return;
    const requestSequence = auditRequestSequence.current;
    setAuditLoadingMore(true);
    setAuditAppendError("");
    try {
      const page = await api.listAuditEvents(workspaceId, auditFilter, auditNextCursor);
      if (requestSequence !== auditRequestSequence.current) return;
      setAuditEvents((current) => mergeById(current, page.items));
      setAuditNextCursor(page.nextCursor);
    } catch (reason) {
      if (requestSequence === auditRequestSequence.current) setAuditAppendError(errorDetail(reason, "下一页审计事件读取失败。").message);
    } finally {
      setAuditLoadingMore(false);
    }
  }, [access.auditRead, api, auditFilter, auditLoadingMore, auditNextCursor, workspaceId]);

  const refreshRuns = useCallback(() => loadRuns(runFilter), [loadRuns, runFilter]);
  const applyRunFilter = useCallback(async (filter: OperationsRunFilter) => {
    setRunFilter(filter);
    await loadRuns(filter);
  }, [loadRuns]);
  const loadMoreRuns = useCallback(async () => {
    if (!access.runtimeRead || !runsNextCursor || runsLoadingMore) return;
    const requestSequence = runsRequestSequence.current;
    setRunsLoadingMore(true);
    setRunsAppendError("");
    try {
      const page = await api.listRuns(workspaceId, runFilter, runsNextCursor);
      if (requestSequence !== runsRequestSequence.current) return;
      setRuns((current) => mergeById(current, page.items));
      setRunsNextCursor(page.nextCursor);
    } catch (reason) {
      if (requestSequence === runsRequestSequence.current) setRunsAppendError(errorDetail(reason, "下一页运行记录读取失败。").message);
    } finally {
      setRunsLoadingMore(false);
    }
  }, [access.runtimeRead, api, runFilter, runsLoadingMore, runsNextCursor, workspaceId]);

  const refreshPolicy = useCallback(() => loadPolicy(), [loadPolicy]);

  const openRun = useCallback(async (runId: string) => {
    const requestSequence = ++runDetailRequestSequence.current;
    setSelectedRunId(runId);
    setRunDetail(null);
    if (!access.runtimeRead) {
      setRunDetailStatus(status("forbidden", "查看运行详情需要 runtime.read 能力。", "FORBIDDEN"));
      return;
    }
    setRunDetailStatus(status("loading"));
    try {
      const detail = await api.getRun(workspaceId, runId);
      if (requestSequence !== runDetailRequestSequence.current) return;
      setRunDetail({ run: { ...detail.run, capabilities: { ...detail.run.capabilities } }, events: [...detail.events].sort((a, b) => a.sequence - b.sequence) });
      setRunDetailStatus(status("ready"));
    } catch (reason) {
      if (requestSequence !== runDetailRequestSequence.current) return;
      const detail = errorDetail(reason, "运行详情读取失败。");
      setRunDetailStatus(failureStatus(detail));
    }
  }, [access.runtimeRead, api, workspaceId]);

  const closeRun = useCallback(() => {
    runDetailRequestSequence.current += 1;
    setSelectedRunId(null);
    setRunDetail(null);
    setRunDetailStatus(status("idle"));
    setRunCommand(command("idle"));
  }, []);

  const createAuditExport = useCallback(async () => {
    if (!access.auditRead) throw commandFailure(setExportCommand, "审计导出需要 audit.read 能力。", "FORBIDDEN", 403);
    setExportCommand(command("pending"));
    setExportRecord(null);
    setExportRefreshWarning("");
    setDownloadCommand(command("idle"));
    try {
      const record = await api.createAuditExport(workspaceId, auditFilter, createOperationsIdempotencyKey("audit-export"));
      setExportRecord(record);
      setExportCommand(command("confirmed"));
      void loadAudit(auditFilter, undefined, true).catch((reason) => {
        const detail = errorDetail(reason, "审计事件刷新失败。");
        setExportRefreshWarning(`导出已创建，但审计事件刷新失败：${detail.message}`);
      });
      return record;
    } catch (reason) {
      const detail = errorDetail(reason, "审计导出创建失败。");
      setExportCommand(command(commandStateFor(detail), detail.message, detail.code));
      throw reason;
    }
  }, [access.auditRead, api, auditFilter, loadAudit, workspaceId]);

  const downloadAuditExport = useCallback(async () => {
    if (!access.auditRead) throw commandFailure(setDownloadCommand, "审计导出下载需要 audit.read 能力。", "FORBIDDEN", 403);
    if (!exportRecord) throw commandFailure(setDownloadCommand, "请先创建服务端审计导出。", "EXPORT_REQUIRED", 400, "error");
    setDownloadCommand(command("pending"));
    try {
      const download = await api.downloadAuditExport(workspaceId, exportRecord.id);
      if (!download.contentDigest || download.contentDigest.toLowerCase() !== exportRecord.contentDigest.toLowerCase()) {
        throw new OperationsApiError("下载内容摘要与导出记录不一致。", 409, "EXPORT_DIGEST_MISMATCH");
      }
      setDownloadCommand(command("confirmed"));
      return download;
    } catch (reason) {
      const detail = errorDetail(reason, "审计导出下载失败。");
      setDownloadCommand(command(commandStateFor(detail), detail.message, detail.code));
      throw reason;
    }
  }, [access.auditRead, api, exportRecord, workspaceId]);

  const updateSettings = useCallback(async (input: UpdateOperationsRuntimeSettingsInput) => {
    if (!access.runtimeManage) throw commandFailure(setSettingsCommand, "更新运行设置需要 runtime.manage 能力。", "FORBIDDEN", 403);
    if (!access.runtimeRead || !policy) throw commandFailure(setSettingsCommand, "缺少 runtime.read，无法加载设置版本上下文。", "CONTEXT_REQUIRED", 403);
    setSettingsCommand(command("pending"));
    try {
      await api.updatePolicy(workspaceId, { ...input, expectedVersion: policy.settings.version });
      const confirmed = await api.getPolicy(workspaceId);
      setPolicy(confirmed);
      setPolicyStatus(status("ready"));
      setSettingsCommand(command("confirmed"));
      return confirmed;
    } catch (reason) {
      const detail = errorDetail(reason, "运行设置更新失败。");
      setSettingsCommand(command(commandStateFor(detail), detail.message, detail.code));
      throw reason;
    }
  }, [access.runtimeManage, access.runtimeRead, api, policy, workspaceId]);

  const executeRunCommand = useCallback(async (kind: "retry" | "cancel", runId: string) => {
    if (!access.runtimeManage) throw commandFailure(setRunCommand, "管理运行需要 runtime.manage 能力。", "FORBIDDEN", 403);
    if (!access.runtimeRead) throw commandFailure(setRunCommand, "缺少 runtime.read，无法加载运行能力上下文。", "CONTEXT_REQUIRED", 403);
    const target = runDetail?.run.id === runId ? runDetail.run : runs.find((item) => item.id === runId);
    if (!target || !target.capabilities[kind]) throw commandFailure(setRunCommand, "该运行未声明此操作的安全能力。", "UNSUPPORTED_OPERATION", 400, "unsupported");
    setRunCommand(command("pending"));
    try {
      await (kind === "retry" ? api.retryRun(workspaceId, runId) : api.cancelRun(workspaceId, runId));
      const detail = await api.getRun(workspaceId, runId);
      setRunDetail({ run: { ...detail.run, capabilities: { ...detail.run.capabilities } }, events: [...detail.events].sort((a, b) => a.sequence - b.sequence) });
      setRunDetailStatus(status("ready"));
      await loadRuns(runFilter, undefined, true);
      setRunCommand(command("confirmed"));
    } catch (reason) {
      const detail = errorDetail(reason, `运行${kind === "retry" ? "重试" : "取消"}失败。`);
      setRunCommand(command(commandStateFor(detail), detail.message, detail.code));
      throw reason;
    }
  }, [access.runtimeManage, access.runtimeRead, api, loadRuns, runDetail, runFilter, runs, workspaceId]);

  const retryRun = useCallback((runId: string) => executeRunCommand("retry", runId), [executeRunCommand]);
  const cancelRun = useCallback((runId: string) => executeRunCommand("cancel", runId), [executeRunCommand]);

  const value = useMemo<OperationsRuntimeValue>(() => ({
    workspaceId, access,
    auditEvents, auditFilter, auditState: auditStatus.state, auditError: auditStatus.error, auditErrorCode: auditStatus.errorCode, auditNextCursor, auditLoadingMore, auditAppendError,
    runs, runFilter, runsState: runsStatus.state, runsError: runsStatus.error, runsErrorCode: runsStatus.errorCode, runsNextCursor, runsLoadingMore, runsAppendError,
    policy, policyState: policyStatus.state, policyError: policyStatus.error, policyErrorCode: policyStatus.errorCode,
    selectedRunId, runDetail, runDetailState: runDetailStatus.state, runDetailError: runDetailStatus.error, runDetailErrorCode: runDetailStatus.errorCode,
    exportRecord, exportState: exportCommand.state, exportError: exportCommand.error, exportErrorCode: exportCommand.errorCode, exportRefreshWarning,
    downloadState: downloadCommand.state, downloadError: downloadCommand.error, downloadErrorCode: downloadCommand.errorCode,
    settingsCommandState: settingsCommand.state, settingsCommandError: settingsCommand.error, settingsCommandErrorCode: settingsCommand.errorCode,
    runCommandState: runCommand.state, runCommandError: runCommand.error, runCommandErrorCode: runCommand.errorCode,
    refreshAudit, applyAuditFilter, loadMoreAudit, refreshRuns, applyRunFilter, loadMoreRuns, refreshPolicy, openRun, closeRun, createAuditExport, downloadAuditExport, updateSettings, retryRun, cancelRun,
  }), [access, applyAuditFilter, applyRunFilter, auditAppendError, auditEvents, auditFilter, auditLoadingMore, auditNextCursor, auditStatus, cancelRun, closeRun, createAuditExport, downloadAuditExport, downloadCommand, exportCommand, exportRecord, exportRefreshWarning, loadMoreAudit, loadMoreRuns, openRun, policy, policyStatus, refreshAudit, refreshPolicy, refreshRuns, retryRun, runCommand, runDetail, runDetailStatus, runFilter, runs, runsAppendError, runsLoadingMore, runsNextCursor, runsStatus, selectedRunId, settingsCommand, updateSettings, workspaceId]);

  return <OperationsRuntimeContext.Provider value={value}>{children}</OperationsRuntimeContext.Provider>;
}

export function useOperationsRuntime(): OperationsRuntimeValue {
  const value = useContext(OperationsRuntimeContext);
  if (!value) throw new Error("useOperationsRuntime must be used inside OperationsRuntimeProvider");
  return value;
}

interface ResourceStatus {
  state: OperationsDataState;
  error: string;
  errorCode: string;
}

interface CommandStatus {
  state: OperationsCommandState;
  error: string;
  errorCode: string;
}

function status(state: OperationsDataState, error = "", errorCode = ""): ResourceStatus {
  return { state, error, errorCode };
}

function command(state: OperationsCommandState, error = "", errorCode = ""): CommandStatus {
  return { state, error, errorCode };
}

function errorDetail(reason: unknown, fallback: string): { message: string; code: string; status: number } {
  if (reason && typeof reason === "object") {
    return {
      message: "message" in reason && typeof reason.message === "string" ? reason.message : fallback,
      code: "code" in reason && typeof reason.code === "string" ? reason.code : "REQUEST_FAILED",
      status: "status" in reason && typeof reason.status === "number" ? reason.status : 0,
    };
  }
  return { message: fallback, code: "REQUEST_FAILED", status: 0 };
}

function failureStatus(detail: { message: string; code: string; status: number }): ResourceStatus {
  return status(detail.status === 403 ? "forbidden" : "error", detail.message, detail.code);
}

function commandStateFor(detail: { status: number; code: string }): OperationsCommandState {
  if (detail.status === 403) return "forbidden";
  if (detail.status === 409) return "conflict";
  if (detail.code === "UNSUPPORTED_OPERATION") return "unsupported";
  return "error";
}

function commandFailure(setter: (value: CommandStatus) => void, message: string, code: string, statusCode: number, state: OperationsCommandState = "forbidden"): OperationsApiError {
  const error = new OperationsApiError(message, statusCode, code);
  setter(command(state, message, code));
  return error;
}

function mergeById<T extends { id: string }>(current: T[], incoming: T[]): T[] {
  const records = new Map(current.map((item) => [item.id, item]));
  incoming.forEach((item) => records.set(item.id, item));
  return [...records.values()];
}

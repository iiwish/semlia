/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import {
  createWorkbenchIdempotencyKey,
  workbenchApi,
  WorkbenchApiError,
  type UpdateWorkbenchAttentionItemRequest,
  type WorkbenchApi,
  type WorkbenchAttentionCounts,
  type WorkbenchAttentionItem,
  type WorkbenchFilter,
  type WorkbenchView,
} from "./workbench";

export type WorkbenchDataState = "idle" | "loading" | "ready" | "empty" | "forbidden" | "error";
export type WorkbenchCommandState = "idle" | "pending" | "confirmed" | "forbidden" | "conflict" | "error";

export interface WorkbenchAccess {
  read: boolean;
  manage: boolean;
}

interface Status {
  state: WorkbenchDataState;
  message: string;
  code: string;
}

interface CommandStatus {
  state: WorkbenchCommandState;
  message: string;
  code: string;
}

export interface WorkbenchRuntimeValue {
  workspaceId: string;
  access: WorkbenchAccess;
  items: WorkbenchAttentionItem[];
  counts: WorkbenchAttentionCounts | null;
  countsByView: Partial<Record<WorkbenchView, WorkbenchAttentionCounts>>;
  filter: WorkbenchFilter;
  state: WorkbenchDataState;
  error: string;
  errorCode: string;
  nextCursor?: string;
  loadingMore: boolean;
  appendError: string;
  selectedItemId: string | null;
  selectedItem: WorkbenchAttentionItem | null;
  detailState: WorkbenchDataState;
  detailError: string;
  detailErrorCode: string;
  commandState: WorkbenchCommandState;
  commandError: string;
  commandErrorCode: string;
  refreshWarning: string;
  applyFilter: (filter: WorkbenchFilter) => Promise<void>;
  refresh: () => Promise<void>;
  loadMore: () => Promise<void>;
  openItem: (attentionItemId: string) => Promise<void>;
  closeItem: () => void;
  updateItem: (input: Omit<UpdateWorkbenchAttentionItemRequest, "expectedVersion">) => Promise<WorkbenchAttentionItem>;
}

export type { WorkbenchApi } from "./workbench";

const defaultFilter: WorkbenchFilter = { view: "mine", sort: "priority_desc", limit: 50 };
const WorkbenchRuntimeContext = createContext<WorkbenchRuntimeValue | null>(null);

export function WorkbenchRuntimeProvider({ children, workspaceId, access, api: providedApi }: {
  children: ReactNode;
  workspaceId: string;
  access: WorkbenchAccess;
  api?: WorkbenchApi;
}) {
  const api = providedApi ?? workbenchApi;
  const [items, setItems] = useState<WorkbenchAttentionItem[]>([]);
  const [counts, setCounts] = useState<WorkbenchAttentionCounts | null>(null);
  const [countsByView, setCountsByView] = useState<Partial<Record<WorkbenchView, WorkbenchAttentionCounts>>>({});
  const [filter, setFilter] = useState<WorkbenchFilter>(defaultFilter);
  const [listStatus, setListStatus] = useState<Status>(() => access.read ? status("loading") : status("forbidden", "查看工作台需要 workspace.read 能力。", "FORBIDDEN"));
  const [nextCursor, setNextCursor] = useState<string>();
  const [loadingMore, setLoadingMore] = useState(false);
  const [appendError, setAppendError] = useState("");
  const [selectedItemId, setSelectedItemId] = useState<string | null>(null);
  const [selectedItem, setSelectedItem] = useState<WorkbenchAttentionItem | null>(null);
  const [detailStatus, setDetailStatus] = useState<Status>(() => status("idle"));
  const [commandStatus, setCommandStatus] = useState<CommandStatus>(() => command("idle"));
  const [refreshWarning, setRefreshWarning] = useState("");
  const listRequestSequence = useRef(0);
  const detailRequestSequence = useRef(0);
  const activeWorkspaceId = useRef(workspaceId);

  const loadList = useCallback(async (nextFilter: WorkbenchFilter, signal?: AbortSignal) => {
    const requestSequence = ++listRequestSequence.current;
    if (!access.read) {
      setItems([]);
      setCounts(null);
      setNextCursor(undefined);
      setListStatus(status("forbidden", "查看工作台需要 workspace.read 能力。", "FORBIDDEN"));
      return;
    }
    setListStatus(status("loading"));
    setItems([]);
    setCounts(null);
    setNextCursor(undefined);
    setAppendError("");
    try {
      const page = await api.listItems(workspaceId, nextFilter, undefined, signal);
      if (signal?.aborted || requestSequence !== listRequestSequence.current) return;
      setItems(page.items);
      setCounts(page.counts);
      setCountsByView((current) => ({ ...current, [nextFilter.view]: { ...page.counts } }));
      setNextCursor(page.page.nextCursor);
      setListStatus(status(page.items.length === 0 ? "empty" : "ready"));
    } catch (reason) {
      if (signal?.aborted || requestSequence !== listRequestSequence.current) return;
      const detail = errorDetail(reason, "工作台待办读取失败。");
      setItems([]);
      setCounts(null);
      setNextCursor(undefined);
      setListStatus(failureStatus(detail));
    }
  }, [access.read, api, workspaceId]);

  useEffect(() => {
    const workspaceChanged = activeWorkspaceId.current !== workspaceId;
    activeWorkspaceId.current = workspaceId;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setItems([]);
      setCounts(null);
      setCountsByView({});
      setFilter(defaultFilter);
      if (workspaceChanged) {
        detailRequestSequence.current += 1;
        setSelectedItemId(null);
        setSelectedItem(null);
        setDetailStatus(status("idle"));
        setCommandStatus(command("idle"));
        setRefreshWarning("");
      } else if (!access.read) {
        setSelectedItem(null);
        setDetailStatus(status("forbidden", "查看待办详情需要 workspace.read 能力。", "FORBIDDEN"));
      }
      void loadList(defaultFilter, controller.signal);
    }, 0);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [access.read, loadList, workspaceId]);

  const applyFilter = useCallback(async (nextFilter: WorkbenchFilter) => {
    setFilter(nextFilter);
    setRefreshWarning("");
    await loadList(nextFilter);
  }, [loadList]);

  const refresh = useCallback(() => loadList(filter), [filter, loadList]);

  const loadMore = useCallback(async () => {
    if (!access.read || !nextCursor || loadingMore) return;
    const requestSequence = listRequestSequence.current;
    setLoadingMore(true);
    setAppendError("");
    try {
      const page = await api.listItems(workspaceId, filter, nextCursor);
      if (requestSequence !== listRequestSequence.current) return;
      setItems((current) => mergeItems(current, page.items));
      setCounts(page.counts);
      setCountsByView((current) => ({ ...current, [filter.view]: { ...page.counts } }));
      setNextCursor(page.page.nextCursor);
    } catch (reason) {
      if (requestSequence === listRequestSequence.current) setAppendError(errorDetail(reason, "下一页待办读取失败。").message);
    } finally {
      setLoadingMore(false);
    }
  }, [access.read, api, filter, loadingMore, nextCursor, workspaceId]);

  const openItem = useCallback(async (attentionItemId: string) => {
    const requestSequence = ++detailRequestSequence.current;
    setSelectedItemId(attentionItemId);
    setSelectedItem(null);
    setCommandStatus(command("idle"));
    setRefreshWarning("");
    if (!access.read) {
      setDetailStatus(status("forbidden", "查看待办详情需要 workspace.read 能力。", "FORBIDDEN"));
      return;
    }
    setDetailStatus(status("loading"));
    try {
      const item = await api.getItem(workspaceId, attentionItemId);
      if (requestSequence !== detailRequestSequence.current) return;
      setSelectedItem(item);
      setDetailStatus(status("ready"));
    } catch (reason) {
      if (requestSequence !== detailRequestSequence.current) return;
      const detail = errorDetail(reason, "待办详情读取失败。");
      setDetailStatus(failureStatus(detail));
    }
  }, [access.read, api, workspaceId]);

  const closeItem = useCallback(() => {
    detailRequestSequence.current += 1;
    setSelectedItemId(null);
    setSelectedItem(null);
    setDetailStatus(status("idle"));
    setCommandStatus(command("idle"));
    setRefreshWarning("");
  }, []);

  const refreshAfterMutation = useCallback(async (confirmed: WorkbenchAttentionItem) => {
    try {
      const page = await api.listItems(workspaceId, filter);
      setItems(page.items);
      setCounts(page.counts);
      setCountsByView((current) => ({ ...current, [filter.view]: { ...page.counts } }));
      setNextCursor(page.page.nextCursor);
      setListStatus(status(page.items.length === 0 ? "empty" : "ready"));
      setRefreshWarning("");
    } catch (reason) {
      setItems((current) => mergeItems(current, [confirmed]));
      setRefreshWarning(`待办已由服务端确认，但列表刷新失败：${errorDetail(reason, "请稍后刷新。").message}`);
    }
  }, [api, filter, workspaceId]);

  const updateItem = useCallback(async (input: Omit<UpdateWorkbenchAttentionItemRequest, "expectedVersion">) => {
    if (!access.manage) throw commandFailure(setCommandStatus, "更新待办需要 workspace.manage 能力。", "FORBIDDEN", 403);
    if (!selectedItem) throw commandFailure(setCommandStatus, "缺少服务端待办版本上下文，请重新打开待办。", "CONTEXT_REQUIRED", 409, "conflict");
    setCommandStatus(command("pending"));
    setRefreshWarning("");
    try {
      const confirmed = await api.updateItem(
        workspaceId,
        selectedItem.id,
        { ...input, expectedVersion: selectedItem.version },
        createWorkbenchIdempotencyKey("workbench-update"),
      );
      setSelectedItem(confirmed);
      setItems((current) => mergeItems(current, [confirmed]));
      setCommandStatus(command("confirmed"));
      void refreshAfterMutation(confirmed);
      return confirmed;
    } catch (reason) {
      const detail = errorDetail(reason, "待办更新失败。");
      setCommandStatus(command(commandStateFor(detail), detail.message, detail.code));
      throw reason;
    }
  }, [access.manage, api, refreshAfterMutation, selectedItem, workspaceId]);

  const value = useMemo<WorkbenchRuntimeValue>(() => ({
    workspaceId,
    access,
    items,
    counts,
    countsByView,
    filter,
    state: listStatus.state,
    error: listStatus.message,
    errorCode: listStatus.code,
    nextCursor,
    loadingMore,
    appendError,
    selectedItemId,
    selectedItem,
    detailState: detailStatus.state,
    detailError: detailStatus.message,
    detailErrorCode: detailStatus.code,
    commandState: commandStatus.state,
    commandError: commandStatus.message,
    commandErrorCode: commandStatus.code,
    refreshWarning,
    applyFilter,
    refresh,
    loadMore,
    openItem,
    closeItem,
    updateItem,
  }), [access, appendError, applyFilter, closeItem, commandStatus, counts, countsByView, detailStatus, filter, items, listStatus, loadMore, loadingMore, nextCursor, openItem, refresh, refreshWarning, selectedItem, selectedItemId, updateItem, workspaceId]);

  return <WorkbenchRuntimeContext.Provider value={value}>{children}</WorkbenchRuntimeContext.Provider>;
}

export function useWorkbenchRuntime(): WorkbenchRuntimeValue {
  const value = useContext(WorkbenchRuntimeContext);
  if (!value) throw new Error("useWorkbenchRuntime must be used inside WorkbenchRuntimeProvider.");
  return value;
}

function status(state: WorkbenchDataState, message = "", code = ""): Status {
  return { state, message, code };
}

function command(state: WorkbenchCommandState, message = "", code = ""): CommandStatus {
  return { state, message, code };
}

function errorDetail(reason: unknown, fallback: string): { message: string; code: string; status: number } {
  if (reason instanceof WorkbenchApiError) return { message: reason.message, code: reason.code, status: reason.status };
  return { message: reason instanceof Error ? reason.message : fallback, code: "REQUEST_FAILED", status: 0 };
}

function failureStatus(detail: { message: string; code: string; status: number }): Status {
  return status(detail.status === 403 ? "forbidden" : "error", detail.message, detail.code);
}

function commandStateFor(detail: { status: number }): WorkbenchCommandState {
  if (detail.status === 403) return "forbidden";
  if (detail.status === 409) return "conflict";
  return "error";
}

function commandFailure(
  setter: (value: CommandStatus) => void,
  message: string,
  code: string,
  httpStatus: number,
  state: WorkbenchCommandState = "forbidden",
): WorkbenchApiError {
  setter(command(state, message, code));
  return new WorkbenchApiError(message, httpStatus, code);
}

function mergeItems(current: WorkbenchAttentionItem[], incoming: WorkbenchAttentionItem[]): WorkbenchAttentionItem[] {
  const byId = new Map(current.map((item) => [item.id, item]));
  incoming.forEach((item) => byId.set(item.id, item));
  return [...byId.values()];
}

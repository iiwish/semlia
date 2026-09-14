/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";

import {
  createIngestionIdempotencyKey,
  ingestionApi,
  IngestionApiError,
  type ArtifactFilter,
  type ArtifactSet,
  type CursorPage,
  type IngestionApi,
  type IngestionArtifact,
  type PublicArtifactKind,
  type RegisterSQLArtifactsInput,
  type SourceSchedule,
  type SourceScheduleInput,
  type SourceScheduleOccurrence,
  type SourceScheduleUpdateInput,
} from "./ingestion";

export interface IngestionAccess {
  read: boolean;
  manage: boolean;
  run: boolean;
}

export interface ResourceStatus {
  state: "idle" | "loading" | "ready" | "empty" | "forbidden" | "error";
  error: string;
}

export interface PagedCollection<T> {
  items: T[];
  total: number;
  nextCursor?: string;
  status: ResourceStatus;
  loadingMore: boolean;
  appendError: string;
}

export interface ArtifactSourceInput {
  sourceName: string;
  sourceId?: string;
  expectedSourceVersion?: number;
  files: Array<{ file: File; kind: PublicArtifactKind }>;
}

export interface IngestionRuntimeValue {
  access: IngestionAccess;
  artifacts: IngestionArtifact[];
  artifactTotal: number;
  artifactNextCursor?: string;
  artifactStatus: ResourceStatus;
  artifactsLoadingMore: boolean;
  artifactsAppendError: string;
  artifactFilter: ArtifactFilter;
  confirmedArtifactSet: ArtifactSet | null;
  confirmedArtifactSets: Record<string, ArtifactSet>;
  schedulesBySource: Record<string, PagedCollection<SourceSchedule>>;
  occurrencesBySchedule: Record<string, PagedCollection<SourceScheduleOccurrence>>;
  selectedSchedule: SourceSchedule | null;
  selectedScheduleStatus: ResourceStatus;
  lastOccurrence: SourceScheduleOccurrence | null;
  refreshWarning: string;
  setArtifactFilter: (filter: ArtifactFilter) => void;
  refreshArtifacts: () => Promise<void>;
  loadMoreArtifacts: () => Promise<void>;
  createArtifactSource: (input: ArtifactSourceInput) => Promise<ArtifactSet>;
  registerSQLSource: (input: RegisterSQLArtifactsInput) => Promise<ArtifactSet>;
  loadArtifactSet: (artifactSetId: string) => Promise<ArtifactSet>;
  loadSchedules: (sourceId: string) => Promise<SourceSchedule[]>;
  loadMoreSchedules: (sourceId: string) => Promise<void>;
  openSchedule: (scheduleId: string) => Promise<SourceSchedule>;
  closeSchedule: () => void;
  createSchedule: (sourceId: string, input: SourceScheduleInput) => Promise<SourceSchedule>;
  updateSchedule: (schedule: SourceSchedule, input: SourceScheduleUpdateInput) => Promise<SourceSchedule>;
  deleteSchedule: (schedule: SourceSchedule) => Promise<SourceSchedule>;
  pauseSchedule: (schedule: SourceSchedule) => Promise<SourceSchedule>;
  resumeSchedule: (schedule: SourceSchedule) => Promise<SourceSchedule>;
  runScheduleNow: (schedule: SourceSchedule) => Promise<SourceScheduleOccurrence>;
  loadOccurrences: (scheduleId: string) => Promise<SourceScheduleOccurrence[]>;
  loadMoreOccurrences: (scheduleId: string) => Promise<void>;
  clearRefreshWarning: () => void;
}

const idleStatus: ResourceStatus = { state: "idle", error: "" };
const IngestionRuntimeContext = createContext<IngestionRuntimeValue | null>(null);

export function IngestionRuntimeProvider({ children, workspaceId, access, api = ingestionApi }: {
  children: ReactNode;
  workspaceId: string;
  access: IngestionAccess;
  api?: IngestionApi;
}) {
  const [artifacts, setArtifacts] = useState<IngestionArtifact[]>([]);
  const [artifactTotal, setArtifactTotal] = useState(0);
  const [artifactNextCursor, setArtifactNextCursor] = useState<string>();
  const [artifactStatus, setArtifactStatus] = useState<ResourceStatus>(access.read ? idleStatus : { state: "forbidden", error: "需要 source.read 权限读取持久工件。" });
  const [artifactsLoadingMore, setArtifactsLoadingMore] = useState(false);
  const [artifactsAppendError, setArtifactsAppendError] = useState("");
  const [artifactFilter, setArtifactFilterState] = useState<ArtifactFilter>({});
  const [confirmedArtifactSet, setConfirmedArtifactSet] = useState<ArtifactSet | null>(null);
  const [confirmedArtifactSets, setConfirmedArtifactSets] = useState<Record<string, ArtifactSet>>({});
  const [schedulesBySource, setSchedulesBySource] = useState<Record<string, PagedCollection<SourceSchedule>>>({});
  const [occurrencesBySchedule, setOccurrencesBySchedule] = useState<Record<string, PagedCollection<SourceScheduleOccurrence>>>({});
  const [selectedSchedule, setSelectedSchedule] = useState<SourceSchedule | null>(null);
  const [selectedScheduleStatus, setSelectedScheduleStatus] = useState<ResourceStatus>(idleStatus);
  const [lastOccurrence, setLastOccurrence] = useState<SourceScheduleOccurrence | null>(null);
  const [refreshWarning, setRefreshWarning] = useState("");
  const workspaceRef = useRef(workspaceId);
  const artifactRequestRef = useRef(0);
  const scheduleRequestsRef = useRef<Record<string, number>>({});
  const occurrenceRequestsRef = useRef<Record<string, number>>({});
  const commandAttemptsRef = useRef(new Map<string, { key: string; staged: IngestionArtifact[] }>());
  const fileIdentitiesRef = useRef(new WeakMap<File, number>());
  const nextFileIdentityRef = useRef(0);

  const commandAttempt = useCallback((fingerprint: string) => {
    const attempts = commandAttemptsRef.current;
    const existing = attempts.get(fingerprint);
    if (existing) return existing;
    const value = { key: createIngestionIdempotencyKey("ingestion-command"), staged: [] as IngestionArtifact[] };
    attempts.set(fingerprint, value);
    return value;
  }, []);

  useLayoutEffect(() => {
    workspaceRef.current = workspaceId;
  }, [workspaceId]);

  const assertManage = useCallback(() => {
    if (!access.manage) throw new IngestionApiError("需要 source.manage 权限执行此命令。", 403, "FORBIDDEN");
  }, [access.manage]);

  const assertRun = useCallback(() => {
    if (!access.run) throw new IngestionApiError("需要 ingestion.run 权限执行此命令。", 403, "FORBIDDEN");
  }, [access.run]);

  const assertManageAndRun = useCallback(() => {
    assertManage();
    assertRun();
  }, [assertManage, assertRun]);

  const refreshArtifacts = useCallback(async () => {
    if (!access.read) {
      setArtifactStatus({ state: "forbidden", error: "需要 source.read 权限读取持久工件。" });
      return;
    }
    if (!workspaceId) {
      setArtifactStatus({ state: "empty", error: "" });
      return;
    }
    const request = ++artifactRequestRef.current;
    setArtifactStatus({ state: "loading", error: "" });
    try {
      const page = await api.listArtifacts(workspaceId, artifactFilter);
      if (workspaceRef.current !== workspaceId || artifactRequestRef.current !== request) return;
      setArtifacts(page.items);
      setArtifactTotal(page.total);
      setArtifactNextCursor(page.nextCursor);
      setArtifactsAppendError("");
      setArtifactStatus({ state: page.items.length > 0 ? "ready" : "empty", error: "" });
    } catch (reason) {
      if (workspaceRef.current !== workspaceId || artifactRequestRef.current !== request) return;
      const message = messageFor(reason, "持久工件读取失败。");
      setArtifactStatus({ state: isForbidden(reason) ? "forbidden" : "error", error: message });
      throw reason;
    }
  }, [access.read, api, artifactFilter, workspaceId]);

  useEffect(() => {
    void refreshArtifacts().catch(() => undefined);
  }, [refreshArtifacts]);

  const setArtifactFilter = useCallback((filter: ArtifactFilter) => {
    artifactRequestRef.current += 1;
    setArtifactFilterState(filter);
  }, []);

  const loadMoreArtifacts = useCallback(async () => {
    if (!access.read || !artifactNextCursor || artifactsLoadingMore) return;
    const request = artifactRequestRef.current;
    setArtifactsLoadingMore(true);
    setArtifactsAppendError("");
    try {
      const page = await api.listArtifacts(workspaceId, artifactFilter, artifactNextCursor);
      if (workspaceRef.current !== workspaceId || artifactRequestRef.current !== request) return;
      setArtifacts((current) => mergeById(current, page.items));
      setArtifactTotal(page.total);
      setArtifactNextCursor(page.nextCursor);
    } catch (reason) {
      if (workspaceRef.current === workspaceId && artifactRequestRef.current === request) setArtifactsAppendError(messageFor(reason, "更多工件读取失败。"));
    } finally {
      if (workspaceRef.current === workspaceId && artifactRequestRef.current === request) setArtifactsLoadingMore(false);
    }
  }, [access.read, api, artifactFilter, artifactNextCursor, artifactsLoadingMore, workspaceId]);

  const refreshArtifactsAfterCommand = useCallback(async () => {
    if (!access.read) return;
    try {
      await refreshArtifacts();
      setRefreshWarning("");
    } catch (reason) {
      setRefreshWarning(messageFor(reason, "命令已由服务端确认，但工件列表刷新失败。"));
    }
  }, [access.read, refreshArtifacts]);

  const rememberArtifactSet = useCallback((value: ArtifactSet) => {
    setConfirmedArtifactSet(value);
    setConfirmedArtifactSets((current) => ({ ...current, [value.id]: value }));
  }, []);

  const createArtifactSource = useCallback(async (input: ArtifactSourceInput) => {
    assertManage();
    setRefreshWarning("");
    const fileIds = input.files.map(({ file, kind }) => {
      let id = fileIdentitiesRef.current.get(file);
      if (id === undefined) { id = ++nextFileIdentityRef.current; fileIdentitiesRef.current.set(file, id); }
      return { id, kind };
    });
    const fingerprint = JSON.stringify([workspaceId, "artifact-source", input.sourceName, input.sourceId, input.expectedSourceVersion, fileIds]);
    const attempt = commandAttempt(fingerprint);
    const staged = attempt.staged;
    for (const [index, item] of input.files.entries()) {
      if (staged[index]) continue;
      const target = input.sourceId && input.expectedSourceVersion !== undefined
        ? { sourceId: input.sourceId, expectedSourceVersion: input.expectedSourceVersion }
        : undefined;
      const artifact = await api.stageArtifact(workspaceId, item.file, item.kind, `${attempt.key}:upload:${index}`, target);
      if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换，未继续组合工件。", 409, "WORKSPACE_CHANGED");
      staged.push(artifact);
      setArtifacts((current) => mergeById(current, [artifact]));
    }
    const value = await api.finalizeArtifactSet(workspaceId, {
      sourceName: input.sourceName,
      artifactIds: staged.map((item) => item.id),
      ...(input.sourceId ? { sourceId: input.sourceId } : {}),
      ...(input.expectedSourceVersion !== undefined ? { expectedSourceVersion: input.expectedSourceVersion } : {}),
    }, `${attempt.key}:finalize`);
    if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换，服务端结果未应用到当前页面。", 409, "WORKSPACE_CHANGED");
    rememberArtifactSet(value);
    commandAttemptsRef.current.delete(fingerprint);
    await refreshArtifactsAfterCommand();
    return value;
  }, [api, assertManage, commandAttempt, refreshArtifactsAfterCommand, rememberArtifactSet, workspaceId]);

  const registerSQLSource = useCallback(async (input: RegisterSQLArtifactsInput) => {
    assertManage();
    setRefreshWarning("");
    const fingerprint = JSON.stringify([workspaceId, "sql-register", input]);
    const value = await api.registerSQL(workspaceId, input, commandAttempt(fingerprint).key);
    if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换，服务端结果未应用到当前页面。", 409, "WORKSPACE_CHANGED");
    rememberArtifactSet(value);
    commandAttemptsRef.current.delete(fingerprint);
    await refreshArtifactsAfterCommand();
    return value;
  }, [api, assertManage, commandAttempt, refreshArtifactsAfterCommand, rememberArtifactSet, workspaceId]);

  const loadArtifactSet = useCallback(async (artifactSetId: string) => {
    if (!access.read) throw new IngestionApiError("需要 source.read 权限读取工件集合。", 403, "FORBIDDEN");
    const value = await api.getArtifactSet(workspaceId, artifactSetId);
    if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换，服务端结果未应用到当前页面。", 409, "WORKSPACE_CHANGED");
    setConfirmedArtifactSets((current) => ({ ...current, [value.id]: value }));
    return value;
  }, [access.read, api, workspaceId]);

  const loadSchedules = useCallback(async (sourceId: string) => {
    if (!access.read) {
      setSchedulesBySource((current) => ({ ...current, [sourceId]: emptyCollection("forbidden", "需要 source.read 权限读取接入计划。") }));
      return [];
    }
    const request = (scheduleRequestsRef.current[sourceId] ?? 0) + 1;
    scheduleRequestsRef.current[sourceId] = request;
    setSchedulesBySource((current) => ({ ...current, [sourceId]: { ...(current[sourceId] ?? emptyCollection()), status: { state: "loading", error: "" }, loadingMore: false, appendError: "" } }));
    try {
      const page = await api.listSchedules(workspaceId, sourceId);
      if (workspaceRef.current !== workspaceId || scheduleRequestsRef.current[sourceId] !== request) return [];
      setSchedulesBySource((current) => ({ ...current, [sourceId]: collectionFromPage(page) }));
      return page.items;
    } catch (reason) {
      if (workspaceRef.current === workspaceId && scheduleRequestsRef.current[sourceId] === request) {
        setSchedulesBySource((current) => ({ ...current, [sourceId]: { ...(current[sourceId] ?? emptyCollection()), status: { state: isForbidden(reason) ? "forbidden" : "error", error: messageFor(reason, "接入计划读取失败。") }, loadingMore: false, appendError: "" } }));
      }
      throw reason;
    }
  }, [access.read, api, workspaceId]);

  const loadMoreSchedules = useCallback(async (sourceId: string) => {
    const currentPage = schedulesBySource[sourceId];
    if (!access.read || !currentPage?.nextCursor || currentPage.loadingMore) return;
    const request = scheduleRequestsRef.current[sourceId] ?? 0;
    setSchedulesBySource((current) => ({ ...current, [sourceId]: { ...current[sourceId], loadingMore: true, appendError: "" } }));
    try {
      const page = await api.listSchedules(workspaceId, sourceId, currentPage.nextCursor);
      if (workspaceRef.current !== workspaceId || scheduleRequestsRef.current[sourceId] !== request) return;
      setSchedulesBySource((current) => ({ ...current, [sourceId]: { ...current[sourceId], items: mergeById(current[sourceId].items, page.items), total: page.total, nextCursor: page.nextCursor, loadingMore: false } }));
    } catch (reason) {
      if (workspaceRef.current === workspaceId && scheduleRequestsRef.current[sourceId] === request) setSchedulesBySource((current) => ({ ...current, [sourceId]: { ...current[sourceId], loadingMore: false, appendError: messageFor(reason, "更多接入计划读取失败。") } }));
    }
  }, [access.read, api, schedulesBySource, workspaceId]);

  const openSchedule = useCallback(async (scheduleId: string) => {
    if (!access.read) throw new IngestionApiError("需要 source.read 权限读取接入计划。", 403, "FORBIDDEN");
    setSelectedScheduleStatus({ state: "loading", error: "" });
    try {
      const value = await api.getSchedule(workspaceId, scheduleId);
      if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换。", 409, "WORKSPACE_CHANGED");
      setSelectedSchedule(value);
      setSelectedScheduleStatus({ state: "ready", error: "" });
      setSchedulesBySource((current) => ({ ...current, [value.sourceId]: upsertCollection(current[value.sourceId], value) }));
      return value;
    } catch (reason) {
      if (workspaceRef.current === workspaceId) setSelectedScheduleStatus({ state: isForbidden(reason) ? "forbidden" : "error", error: messageFor(reason, "接入计划详情读取失败。") });
      throw reason;
    }
  }, [access.read, api, workspaceId]);

  const closeSchedule = useCallback(() => {
    setSelectedSchedule(null);
    setSelectedScheduleStatus(idleStatus);
  }, []);

  const refreshScheduleAfterCommand = useCallback(async (sourceId: string) => {
    if (!access.read) return;
    try {
      await loadSchedules(sourceId);
      setRefreshWarning("");
    } catch (reason) {
      setRefreshWarning(messageFor(reason, "命令已由服务器确认，但计划列表刷新失败。"));
    }
  }, [access.read, loadSchedules]);

  const applySchedule = useCallback((value: SourceSchedule) => {
    setSchedulesBySource((current) => ({ ...current, [value.sourceId]: upsertCollection(current[value.sourceId], value) }));
    setSelectedSchedule((current) => current?.id === value.id ? value : current);
  }, []);

  const createSchedule = useCallback(async (sourceId: string, input: SourceScheduleInput) => {
    assertManageAndRun();
    setRefreshWarning("");
    const fingerprint = JSON.stringify([workspaceId, "schedule-create", sourceId, input]);
    const value = await api.createSchedule(workspaceId, sourceId, input, commandAttempt(fingerprint).key);
    if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换，服务端结果未应用到当前页面。", 409, "WORKSPACE_CHANGED");
    applySchedule(value);
    commandAttemptsRef.current.delete(fingerprint);
    await refreshScheduleAfterCommand(sourceId);
    return value;
  }, [api, applySchedule, assertManageAndRun, commandAttempt, refreshScheduleAfterCommand, workspaceId]);

  const updateSchedule = useCallback(async (schedule: SourceSchedule, input: SourceScheduleUpdateInput) => {
    if (input.enabled) assertManageAndRun();
    else assertManage();
    setRefreshWarning("");
    const value = await api.updateSchedule(workspaceId, schedule.id, { ...input, expectedVersion: schedule.version }, createIngestionIdempotencyKey("schedule-update"));
    if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换，服务端结果未应用到当前页面。", 409, "WORKSPACE_CHANGED");
    applySchedule(value);
    await refreshScheduleAfterCommand(value.sourceId);
    return value;
  }, [api, applySchedule, assertManage, assertManageAndRun, refreshScheduleAfterCommand, workspaceId]);

  const mutateSchedule = useCallback(async (schedule: SourceSchedule, action: "delete" | "pause" | "resume") => {
    if (action === "resume") assertManageAndRun();
    else assertManage();
    setRefreshWarning("");
    const command = action === "delete" ? api.deleteSchedule : action === "pause" ? api.pauseSchedule : api.resumeSchedule;
    const value = await command(workspaceId, schedule.id, schedule.version, createIngestionIdempotencyKey(`schedule-${action}`));
    if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换，服务端结果未应用到当前页面。", 409, "WORKSPACE_CHANGED");
    applySchedule(value);
    await refreshScheduleAfterCommand(value.sourceId);
    return value;
  }, [api, applySchedule, assertManage, assertManageAndRun, refreshScheduleAfterCommand, workspaceId]);

  const deleteSchedule = useCallback((schedule: SourceSchedule) => mutateSchedule(schedule, "delete"), [mutateSchedule]);
  const pauseSchedule = useCallback((schedule: SourceSchedule) => mutateSchedule(schedule, "pause"), [mutateSchedule]);
  const resumeSchedule = useCallback((schedule: SourceSchedule) => mutateSchedule(schedule, "resume"), [mutateSchedule]);

  const runScheduleNow = useCallback(async (schedule: SourceSchedule) => {
    assertRun();
    const value = await api.runScheduleNow(workspaceId, schedule.id, schedule.version, createIngestionIdempotencyKey("schedule-run-now"));
    if (workspaceRef.current !== workspaceId) throw new IngestionApiError("工作区已切换，服务端结果未应用到当前页面。", 409, "WORKSPACE_CHANGED");
    setLastOccurrence(value);
    setOccurrencesBySchedule((current) => ({ ...current, [schedule.id]: upsertCollection(current[schedule.id], value) }));
    return value;
  }, [api, assertRun, workspaceId]);

  const loadOccurrences = useCallback(async (scheduleId: string) => {
    if (!access.read) {
      setOccurrencesBySchedule((current) => ({ ...current, [scheduleId]: emptyCollection("forbidden", "需要 source.read 权限读取计划执行记录。") }));
      return [];
    }
    const request = (occurrenceRequestsRef.current[scheduleId] ?? 0) + 1;
    occurrenceRequestsRef.current[scheduleId] = request;
    setOccurrencesBySchedule((current) => ({ ...current, [scheduleId]: { ...(current[scheduleId] ?? emptyCollection()), status: { state: "loading", error: "" }, loadingMore: false, appendError: "" } }));
    try {
      const page = await api.listOccurrences(workspaceId, scheduleId);
      if (workspaceRef.current !== workspaceId || occurrenceRequestsRef.current[scheduleId] !== request) return [];
      setOccurrencesBySchedule((current) => ({ ...current, [scheduleId]: collectionFromPage(page) }));
      return page.items;
    } catch (reason) {
      if (workspaceRef.current === workspaceId && occurrenceRequestsRef.current[scheduleId] === request) setOccurrencesBySchedule((current) => ({ ...current, [scheduleId]: { ...(current[scheduleId] ?? emptyCollection()), status: { state: isForbidden(reason) ? "forbidden" : "error", error: messageFor(reason, "计划执行记录读取失败。") }, loadingMore: false, appendError: "" } }));
      throw reason;
    }
  }, [access.read, api, workspaceId]);

  const loadMoreOccurrences = useCallback(async (scheduleId: string) => {
    const currentPage = occurrencesBySchedule[scheduleId];
    if (!access.read || !currentPage?.nextCursor || currentPage.loadingMore) return;
    const request = occurrenceRequestsRef.current[scheduleId] ?? 0;
    setOccurrencesBySchedule((current) => ({ ...current, [scheduleId]: { ...current[scheduleId], loadingMore: true, appendError: "" } }));
    try {
      const page = await api.listOccurrences(workspaceId, scheduleId, currentPage.nextCursor);
      if (workspaceRef.current !== workspaceId || occurrenceRequestsRef.current[scheduleId] !== request) return;
      setOccurrencesBySchedule((current) => ({ ...current, [scheduleId]: { ...current[scheduleId], items: mergeById(current[scheduleId].items, page.items), total: page.total, nextCursor: page.nextCursor, loadingMore: false } }));
    } catch (reason) {
      if (workspaceRef.current === workspaceId && occurrenceRequestsRef.current[scheduleId] === request) setOccurrencesBySchedule((current) => ({ ...current, [scheduleId]: { ...current[scheduleId], loadingMore: false, appendError: messageFor(reason, "更多计划执行记录读取失败。") } }));
    }
  }, [access.read, api, occurrencesBySchedule, workspaceId]);

  const value = useMemo<IngestionRuntimeValue>(() => ({
    access, artifacts, artifactTotal, artifactNextCursor, artifactStatus, artifactsLoadingMore,
    artifactsAppendError, artifactFilter, confirmedArtifactSet, confirmedArtifactSets,
    schedulesBySource, occurrencesBySchedule, selectedSchedule, selectedScheduleStatus,
    lastOccurrence, refreshWarning, setArtifactFilter, refreshArtifacts, loadMoreArtifacts,
    createArtifactSource, registerSQLSource, loadArtifactSet, loadSchedules, loadMoreSchedules,
    openSchedule, closeSchedule, createSchedule, updateSchedule, deleteSchedule, pauseSchedule,
    resumeSchedule, runScheduleNow, loadOccurrences, loadMoreOccurrences,
    clearRefreshWarning: () => setRefreshWarning(""),
  }), [access, artifactFilter, artifactNextCursor, artifactStatus, artifactTotal, artifacts, artifactsAppendError, artifactsLoadingMore, closeSchedule, confirmedArtifactSet, confirmedArtifactSets, createArtifactSource, createSchedule, deleteSchedule, lastOccurrence, loadArtifactSet, loadMoreArtifacts, loadMoreOccurrences, loadMoreSchedules, loadOccurrences, loadSchedules, occurrencesBySchedule, openSchedule, pauseSchedule, refreshArtifacts, refreshWarning, registerSQLSource, resumeSchedule, runScheduleNow, schedulesBySource, selectedSchedule, selectedScheduleStatus, setArtifactFilter, updateSchedule]);

  return <IngestionRuntimeContext.Provider value={value}>{children}</IngestionRuntimeContext.Provider>;
}

export function useIngestionRuntime() {
  const value = useContext(IngestionRuntimeContext);
  if (!value) throw new Error("useIngestionRuntime must be used inside IngestionRuntimeProvider");
  return value;
}

function emptyCollection<T>(state: ResourceStatus["state"] = "idle", error = ""): PagedCollection<T> {
  return { items: [], total: 0, status: { state, error }, loadingMore: false, appendError: "" };
}

function collectionFromPage<T>(page: CursorPage<T>): PagedCollection<T> {
  return { items: page.items, total: page.total, nextCursor: page.nextCursor, status: { state: page.items.length > 0 ? "ready" : "empty", error: "" }, loadingMore: false, appendError: "" };
}

function upsertCollection<T extends { id: string }>(collection: PagedCollection<T> | undefined, value: T): PagedCollection<T> {
  const current = collection ?? emptyCollection<T>();
  const existed = current.items.some((item) => item.id === value.id);
  return { ...current, items: mergeById(current.items, [value]), total: existed ? current.total : current.total + 1, status: { state: "ready", error: "" } };
}

function mergeById<T extends { id: string }>(current: T[], additions: T[]) {
  const values = new Map(current.map((item) => [item.id, item]));
  additions.forEach((item) => values.set(item.id, item));
  return [...values.values()];
}

function isForbidden(reason: unknown) {
  return reason instanceof IngestionApiError && reason.status === 403;
}

function messageFor(reason: unknown, fallback: string) {
  return reason instanceof Error && reason.message ? reason.message : fallback;
}

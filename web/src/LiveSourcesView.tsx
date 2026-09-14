import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronRight,
  ChevronLeft,
  CircleAlert,
  Clock3,
  Database,
  FileUp,
  GitPullRequestArrow,
  KeyRound,
  LoaderCircle,
  Pencil,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Search,
  Server,
  Trash2,
  X,
} from "lucide-react";

import { useCan, useResourceCan } from "./authorization";
import { useCatalogRuntime } from "./catalogRuntime";
import {
  createSource,
  deleteSource,
  DiscoveryApiError,
  getDiscoveryRun,
  listSemanticCandidates,
  listSourceRuns,
  listSources,
  rotateSourceCredential,
  startSourceRun,
  testSourceConnection,
  updateSource,
  type CreateSourceRequest,
  type DiscoveryRun,
  type SemanticCandidate,
  type SourceConnection,
  type SourceDiscoveryRun,
} from "./discovery";
import { SemanticProductionWorkspace } from "./SemanticProductionPanel";
import { IngestionRuntimeProvider, useIngestionRuntime } from "./ingestionRuntime";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableOpenButton, TablePanel, TableFooter, TableRow, TableViewport } from "./components/ui/table";
import { ActionMenu } from "./components/ui/action-menu";
import type { ArtifactSet, PublicArtifactKind, SourceSchedule, SourceScheduleOccurrence } from "./ingestion";
import "./ingestion-workspace.css";

interface LiveSourcesViewProps {
  focusIndex: number;
  navigationEpoch?: number;
  runDetailBackRequestEpoch: number;
  initialRunId?: string;
  initialSourceId?: string;
  onRunDetailOpenChange: (open: boolean) => void;
  onOpenProposal: (proposalId: string) => void;
}

type ViewState = "loading" | "ready" | "error";
type RunTab = "runs" | "candidates";

const runLabels: Record<SourceDiscoveryRun["status"], string> = {
  queued: "排队中",
  running: "运行中",
  succeeded: "已完成",
  degraded: "有警告",
  failed: "失败",
  cancelled: "已取消",
};

const candidateLabels: Record<SemanticCandidate["status"], string> = {
  pending: "待处理",
  dismissed: "已忽略",
  converted: "已转提案",
};

export function LiveSourcesView(props: LiveSourcesViewProps) {
  const { workspaceId } = useCatalogRuntime();
  const read = useCan("source.read");
  const manage = useCan("source.manage");
  const run = useCan("ingestion.run");
  const principalId = useResourceCan("source.read", { type: "workspace", id: workspaceId }).principalId;
  return <IngestionRuntimeProvider key={`${workspaceId}:${principalId}:${read}:${manage}:${run}`} workspaceId={workspaceId} access={{ read, manage, run }}><LiveSourcesWorkspace {...props} /></IngestionRuntimeProvider>;
}

function LiveSourcesWorkspace({ focusIndex, navigationEpoch = 0, runDetailBackRequestEpoch, initialRunId, initialSourceId, onRunDetailOpenChange, onOpenProposal }: LiveSourcesViewProps) {
  const { workspaceId } = useCatalogRuntime();
  const ingestion = useIngestionRuntime();
  const canManage = useCan("source.manage");
  const canRun = useCan("ingestion.run");
  const [sources, setSources] = useState<SourceConnection[]>([]);
  const [sourcePageItems, setSourcePageItems] = useState<SourceConnection[]>([]);
  const [sourcePageNextCursor, setSourcePageNextCursor] = useState<string>();
  const [sourcePageIndex, setSourcePageIndex] = useState(0);
  const [sourcePageCursors, setSourcePageCursors] = useState<Array<string | undefined>>([undefined]);
  const [sourcePageSize, setSourcePageSize] = useState(50);
  const [sourcePageLoading, setSourcePageLoading] = useState(false);
  const [sourceCursor, setSourceCursor] = useState<string>();
  const [sourceTotal, setSourceTotal] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [runCursors, setRunCursors] = useState<Record<string, string | undefined>>({});
  const [candidateCursor, setCandidateCursor] = useState<string>();
  const [importTarget, setImportTarget] = useState<SourceConnection | "new" | null>(null);
  const [artifactDetail, setArtifactDetail] = useState<ArtifactSet | null>(null);
  const [runs, setRuns] = useState<SourceDiscoveryRun[]>([]);
  const [candidates, setCandidates] = useState<SemanticCandidate[]>([]);
  const [state, setState] = useState<ViewState>("loading");
  const [error, setError] = useState("");
  const [query, setQuery] = useState("");
  const [sourceFilter, setSourceFilter] = useState("");
  const [runTab, setRunTab] = useState<RunTab>("runs");
  const [resultQuery, setResultQuery] = useState("");
  const [resultStatus, setResultStatus] = useState("");
  const [sourceDialog, setSourceDialog] = useState<{ mode: "create" | "edit" | "credential"; source?: SourceConnection } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<SourceConnection | null>(null);
  const [detailSourceId, setDetailSourceId] = useState("");
  const [sourceDetailTab, setSourceDetailTab] = useState("概览");
  const [actionNotice, setActionNotice] = useState<{ message: string; failed?: boolean; runId?: string } | null>(null);
  const [selectedRunId, setSelectedRunId] = useState(initialRunId ?? "");
  const [selectedRun, setSelectedRun] = useState<DiscoveryRun | null>(null);
  const [runDetailEpoch, setRunDetailEpoch] = useState(0);
  const [selectedCandidate, setSelectedCandidate] = useState<SemanticCandidate | null>(null);
  const [testingSourceId, setTestingSourceId] = useState("");
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; message: string }>>({});
  const [startingSourceId, setStartingSourceId] = useState("");
  const focusedSourceRef = useRef<HTMLTableRowElement>(null);
  const openedRunRef = useRef("");
  const loadEpochRef = useRef(0);
  const candidateEpochRef = useRef(0);
  const appendBusyRef = useRef(false);
  const backEpochRef = useRef(runDetailBackRequestEpoch);
  const navigationEpochRef = useRef(navigationEpoch);
  const invalidateLoads = useCallback(() => { loadEpochRef.current++; }, []);

  const load = useCallback(async (signal?: AbortSignal) => {
    if (!workspaceId) return;
    const epoch = ++loadEpochRef.current;
    candidateEpochRef.current++;
    setSourcePageLoading(true);
    setState((current) => current === "ready" ? current : "loading");
    try {
      const sourcePage = await listSources(workspaceId, { signal, limit: sourcePageSize });
      let sourceItems = sourcePage.items;
      if (initialSourceId && !sourceItems.some((source) => source.id === initialSourceId)) {
        const exactPage = await listSources(workspaceId, { sourceId: initialSourceId, signal });
        sourceItems = mergeById(sourceItems, exactPage.items);
      }
      const runGroups = await Promise.all(sourceItems.map((source) => listSourceRuns(workspaceId, source.id, { signal })));
      const candidatePage = await listSemanticCandidates(workspaceId, { signal });
      if (signal?.aborted || epoch !== loadEpochRef.current) return;
      setSources((current) => mergeById(current, sourceItems));
      setSourcePageItems(sourceItems);
      setSourcePageNextCursor(sourcePage.nextCursor);
      setSourcePageIndex(0);
      setSourcePageCursors([undefined]);
      setSourceCursor(sourcePage.nextCursor);
      setSourceTotal(sourcePage.total);
      setRuns(runGroups.flatMap((page) => page.items).sort((left, right) => right.createdAt.localeCompare(left.createdAt)));
      setRunCursors(Object.fromEntries(sourceItems.map((source, index) => [source.id, runGroups[index].nextCursor])));
      setCandidates(candidatePage.items);
      setCandidateCursor(candidatePage.nextCursor);
      setState("ready");
      setError("");
    } catch (reason) {
      if (signal?.aborted || epoch !== loadEpochRef.current) return;
      setState("error");
      setError(messageFor(reason, "数据接入服务暂时不可用。"));
    }
    finally { if (!signal?.aborted && epoch === loadEpochRef.current) setSourcePageLoading(false); }
  }, [initialSourceId, sourcePageSize, workspaceId]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => { controller.abort(); invalidateLoads(); };
  }, [invalidateLoads, load]);

  useEffect(() => {
    const active = runs.filter((run) => run.status === "queued" || run.status === "running");
    if (!active.length) return;
    const controller = new AbortController();
    let polling = false;
    const timer = window.setInterval(() => {
      if (polling) return;
      polling = true;
      const epoch = loadEpochRef.current;
      void Promise.all(active.map((run) => getDiscoveryRun(workspaceId, run.id, controller.signal)))
        .then(async (updated) => {
          if (controller.signal.aborted || epoch !== loadEpochRef.current) return;
          if (updated.some((run) => run.status !== "queued" && run.status !== "running")) {
            const candidateEpoch = ++candidateEpochRef.current;
            try {
              const page = await listSemanticCandidates(workspaceId, { signal: controller.signal });
              if (!controller.signal.aborted && epoch === loadEpochRef.current && candidateEpoch === candidateEpochRef.current) {
                setCandidates((current) => mergeById(current, page.items));
                setCandidateCursor(page.nextCursor);
              }
            } catch (reason) {
              if (!controller.signal.aborted && epoch === loadEpochRef.current) setError(messageFor(reason, "运行已结束；候选刷新失败。"));
            }
          }
          if (controller.signal.aborted || epoch !== loadEpochRef.current) return;
          setRuns((current) => current.map((run) => ({ ...run, ...updated.find((detail) => detail.id === run.id) })));
          setSelectedRun((current) => updated.find((run) => run.id === current?.id) ?? current);
        })
        .catch((reason) => { if (!controller.signal.aborted && epoch === loadEpochRef.current) setError(messageFor(reason, "运行状态刷新失败。")); })
        .finally(() => { polling = false; });
    }, 2500);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [runs, workspaceId]);

  useEffect(() => {
    if (navigationEpochRef.current === navigationEpoch) return;
    navigationEpochRef.current = navigationEpoch;
    setSelectedRunId("");
    setSelectedRun(null);
    setSelectedCandidate(null);
    setDetailSourceId("");
    setSourceDetailTab("概览");
    setSourceDialog(null);
  }, [navigationEpoch]);

  useEffect(() => {
    if (backEpochRef.current === runDetailBackRequestEpoch) return;
    backEpochRef.current = runDetailBackRequestEpoch;
    setSelectedRunId("");
    setSelectedRun(null);
  }, [runDetailBackRequestEpoch]);

  useEffect(() => {
    onRunDetailOpenChange(Boolean(selectedRunId));
    return () => onRunDetailOpenChange(false);
  }, [onRunDetailOpenChange, selectedRunId]);

  useEffect(() => {
    if (selectedRunId) openedRunRef.current = selectedRunId;
    else if (focusIndex === 2 && openedRunRef.current) {
      document.getElementById(`discovery-row-${openedRunRef.current}`)?.focus({ preventScroll: true });
      openedRunRef.current = "";
    }
  }, [focusIndex, selectedRunId]);

  useEffect(() => {
    if (initialSourceId) setQuery("");
  }, [initialSourceId]);

  useEffect(() => {
    if (state !== "ready" || !initialSourceId || !sources.some((source) => source.id === initialSourceId)) return;
    const target = focusedSourceRef.current;
    target?.focus({ preventScroll: true });
    target?.scrollIntoView?.({ block: "nearest" });
    queueMicrotask(() => {
      if (target?.isConnected) target.focus({ preventScroll: true });
    });
  }, [initialSourceId, sources, state]);

  useEffect(() => {
    if (!selectedRunId || !workspaceId) {
      setSelectedRun(null);
      return;
    }
    const controller = new AbortController();
    getDiscoveryRun(workspaceId, selectedRunId, controller.signal)
      .then((detail) => { if (!controller.signal.aborted) { setSelectedRun(detail); setError(""); } })
      .catch((reason) => { if (!controller.signal.aborted) setError(messageFor(reason, "运行详情加载失败。")); });
    return () => controller.abort();
  }, [runDetailEpoch, selectedRunId, workspaceId]);

  const refresh = () => void load();
  const visibleSources = sourcePageItems.filter((source) => !query.trim() || `${source.name} ${source.sourceKind === "postgresql" ? `${source.host} ${source.database} ${source.username}` : source.sourceKind}`.toLocaleLowerCase("zh-CN").includes(query.trim().toLocaleLowerCase("zh-CN")));
  const visibleRuns = runs.filter((run) => (!sourceFilter || run.sourceConnectionId === sourceFilter) && (!resultStatus || run.status === resultStatus) && `${sourceName(run.sourceConnectionId)} ${run.id}`.toLowerCase().includes(resultQuery.toLowerCase()));
  const visibleCandidates = candidates.filter((candidate) => (!sourceFilter || candidate.sourceConnectionId === sourceFilter) && (!resultStatus || candidate.status === resultStatus) && candidate.title.toLowerCase().includes(resultQuery.toLowerCase()));
  const sourceRangeStart = sourceTotal === 0 ? 0 : sourcePageIndex * sourcePageSize + 1;
  const sourceRangeEnd = sourceTotal === 0 ? 0 : Math.min(sourcePageIndex * sourcePageSize + sourcePageItems.length, sourceTotal);
  function sourceName(sourceId: string) { return sources.find((source) => source.id === sourceId)?.name ?? sourceId; }
  const loadSourcePage = async (cursor: string | undefined, pageIndex: number) => {
    if (!workspaceId || sourcePageLoading || appendBusyRef.current) return;
    appendBusyRef.current = true;
    const epoch = ++loadEpochRef.current;
    setSourcePageLoading(true);
    setLoadingMore(true);
    try {
      const page = await listSources(workspaceId, { cursor, limit: sourcePageSize });
      const pages = await Promise.all(page.items.map((source) => listSourceRuns(workspaceId, source.id)));
      if (epoch !== loadEpochRef.current) return;
      setSources((current) => mergeById(current, page.items));
      setSourcePageItems(page.items);
      setSourcePageNextCursor(page.nextCursor);
      setSourcePageIndex(pageIndex);
      setSourcePageCursors((current) => { const next = current.slice(0, pageIndex); next[pageIndex] = cursor; return next; });
      setSourceCursor(page.nextCursor);
      setSourceTotal(page.total);
      setRuns((current) => mergeById(current, pages.flatMap((value) => value.items)).sort((left, right) => right.createdAt.localeCompare(left.createdAt)));
      setRunCursors((current) => ({ ...current, ...Object.fromEntries(page.items.map((source, index) => [source.id, pages[index].nextCursor])) }));
      setError("");
    } catch (reason) {
      if (epoch === loadEpochRef.current) setError(messageFor(reason, "更多来源读取失败。"));
    } finally {
      appendBusyRef.current = false;
      if (epoch === loadEpochRef.current) { setLoadingMore(false); setSourcePageLoading(false); }
    }
  };

  const loadMoreSources = async () => {
    if (!sourcePageNextCursor) return;
    await loadSourcePage(sourcePageNextCursor, sourcePageIndex + 1);
  };

  const loadSourceRunsForDetail = async (sourceId: string) => {
    if (appendBusyRef.current || !runCursors[sourceId]) return;
    appendBusyRef.current = true;
    try {
      const page = await listSourceRuns(workspaceId, sourceId, { cursor: runCursors[sourceId] });
      setRuns((current) => mergeById(current, page.items));
      setRunCursors((current) => ({ ...current, [sourceId]: page.nextCursor }));
    } catch (reason) { setError(messageFor(reason, "运行历史读取失败。")); }
    finally { appendBusyRef.current = false; }
  };

  const loadPreviousSources = async () => {
    if (sourcePageIndex === 0) return;
    await loadSourcePage(sourcePageCursors[sourcePageIndex - 1], sourcePageIndex - 1);
  };

  const loadMoreResults = async (kind: RunTab = runTab) => {
    if (appendBusyRef.current) return;
    appendBusyRef.current = true;
    const epoch = loadEpochRef.current;
    setLoadingMore(true);
    try {
      if ((kind === "candidates" || selectedRunId) && candidateCursor) {
        const candidateEpoch = candidateEpochRef.current;
        const page = await listSemanticCandidates(workspaceId, { cursor: candidateCursor });
        if (epoch !== loadEpochRef.current || candidateEpoch !== candidateEpochRef.current) return;
        setCandidates((current) => mergeById(current, page.items));
        setCandidateCursor(page.nextCursor);
      } else {
        for (const [sourceId, cursor] of Object.entries(runCursors)) {
          if (!cursor || (sourceFilter && sourceFilter !== sourceId)) continue;
          const page = await listSourceRuns(workspaceId, sourceId, { cursor });
          if (epoch !== loadEpochRef.current) return;
          setRuns((current) => mergeById(current, page.items).sort((a, b) => b.createdAt.localeCompare(a.createdAt)));
          setRunCursors((current) => ({ ...current, [sourceId]: page.nextCursor }));
        }
      }
    } catch (reason) { if (epoch === loadEpochRef.current) setError(messageFor(reason, "更多接入结果读取失败。")); }
    finally { appendBusyRef.current = false; setLoadingMore(false); }
  };

  const handleTest = async (source: SourceConnection) => {
    setTestingSourceId(source.id);
    setActionNotice(null);
    setTestResult((current) => { const next = { ...current }; delete next[source.id]; return next; });
    try {
      await testSourceConnection(workspaceId, source.id);
      setTestResult((current) => ({ ...current, [source.id]: { ok: true, message: "只读元数据连接通过" } }));
      setActionNotice({ message: `${source.name}：连接测试通过。` });
    } catch (reason) {
      setTestResult((current) => ({ ...current, [source.id]: { ok: false, message: messageFor(reason, "连接测试失败。") } }));
      setActionNotice({ message: `${source.name}：${messageFor(reason, "连接测试失败。")}`, failed: true });
    } finally {
      setTestingSourceId("");
    }
  };

  const handleStart = async (source: SourceConnection) => {
    setStartingSourceId(source.id);
    setActionNotice(null);
    try {
      const run = await startSourceRun(workspaceId, source.id, idempotencyKey("discovery", source.id));
      setRuns((current) => [run, ...current.filter((item) => item.id !== run.id)]);
      setRunTab("runs");
      setActionNotice({ message: `${source.name}：发现任务已创建，${runLabels[run.status]}。`, runId: run.id });
      setError("");
    } catch (reason) {
      setError(messageFor(reason, "发现运行启动失败。"));
    } finally {
      setStartingSourceId("");
    }
  };

  if (selectedCandidate) return <CandidateDialog candidate={selectedCandidate} onClose={() => { setSelectedCandidate(null); refresh(); }} onOpenProposal={onOpenProposal} />;
  if (focusIndex === 1) return <ScheduleWorkspace key={navigationEpoch} sources={sources} runs={runs} candidates={candidates} onOpenProposal={onOpenProposal} loading={state === "loading"} sourceError={error} onRefresh={refresh} sourceCursor={sourceCursor} loadingMoreSources={loadingMore} onLoadMoreSources={() => void loadMoreSources()} />;
  if (selectedRunId) return <>
    <RunDetail detail={selectedRun?.id === selectedRunId ? selectedRun : null} error={error} sourceName={selectedRun?.id === selectedRunId ? sourceName(selectedRun.sourceConnectionId) : ""} candidates={candidates} onOpenCandidate={setSelectedCandidate} onBack={() => setSelectedRunId("")} backLabel={focusIndex === 0 ? detailSourceId ? "返回来源" : "返回数据来源" : "返回运行记录"} onRetry={() => { setSelectedRun(null); setRunDetailEpoch((value) => value + 1); }} />
  </>;
  if (focusIndex === 2) {
    return (
      <section className="view source-live-view source-list-page ingestion-results-page" aria-label="接入运行">
        <header className="source-commandbar">
          <div className="source-tabs" role="tablist" aria-label="接入结果">
            <button type="button" role="tab" aria-selected={runTab === "runs"} onClick={() => { setRunTab("runs"); setResultStatus(""); setResultQuery(""); }}><Clock3 size={15} />运行记录 <span>{runs.length}</span></button>
            <button type="button" role="tab" aria-selected={runTab === "candidates"} onClick={() => { setRunTab("candidates"); setResultStatus(""); setResultQuery(""); }}><GitPullRequestArrow size={15} />语义候选 <span>{candidates.filter((item) => item.status === "pending").length}</span></button>
          </div>
          <div className="source-command-actions">
            <select aria-label="按来源筛选" value={sourceFilter} onChange={(event) => setSourceFilter(event.target.value)}><option value="">全部来源</option>{sources.map((source) => <option key={source.id} value={source.id}>{source.name}</option>)}</select>
            <button className="secondary-button" type="button" aria-label="刷新接入结果" title="刷新" onClick={refresh}><RefreshCw size={15} />刷新</button>
          </div>
        </header>
        <div className="ingestion-filterbar"><label className="connection-search"><Search size={15} /><input type="search" aria-label="搜索接入结果" placeholder={runTab === "runs" ? "搜索来源或运行 ID" : "搜索候选名称"} value={resultQuery} onChange={(event) => setResultQuery(event.target.value)} /></label><select aria-label="结果状态" value={resultStatus} onChange={(event) => setResultStatus(event.target.value)}><option value="">全部状态</option>{Object.entries(runTab === "runs" ? runLabels : candidateLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></div>
        {state === "error" && <LoadFailure message={error} onRetry={refresh} />}
        {state === "loading" && <Loading label="正在读取接入运行" />}
        {state === "ready" && <TablePanel aria-label="接入结果列表">
        {state === "ready" && runTab === "runs" && <RunList runs={visibleRuns} sourceName={sourceName} onOpen={setSelectedRunId} />}
        {state === "ready" && runTab === "candidates" && <CandidateList candidates={visibleCandidates} sourceName={sourceName} onOpen={setSelectedCandidate} onOpenProposal={onOpenProposal} />}
        <TableFooter><span className="source-table-summary">显示 {runTab === "runs" ? visibleRuns.length : visibleCandidates.length} 条{runTab === "runs" ? "运行记录" : "语义候选"} · 已加载 {runTab === "runs" ? runs.length : candidates.length} 条</span><div className="schedule-page-actions">
        {(runTab === "candidates" ? candidateCursor : Object.entries(runCursors).some(([id, cursor]) => cursor && (!sourceFilter || id === sourceFilter))) && <button className="secondary-button" disabled={loadingMore} onClick={() => void loadMoreResults()}>加载更多接入结果</button>}
        {sourceCursor && <button className="secondary-button" disabled={loadingMore} onClick={() => void loadMoreSources()}>加载更多来源</button>}
        </div></TableFooter></TablePanel>}
        {error && state !== "error" && <p role="alert">{error}</p>}
      </section>
    );
  }

  return (
    <section className={`view source-live-view source-list-page${detailSourceId ? " has-source-detail" : ""}`} aria-label="数据来源">
      <div hidden={Boolean(detailSourceId)} className="source-list-content">
      <header className="source-commandbar">
        <div className="source-tabs" role="tablist" aria-label="来源类型">
          <button type="button" role="tab" aria-selected="true"><Database size={15} />全部来源 <span>{sourceTotal}</span></button>
        </div>
        <div className="source-command-actions">
          <label className="connection-search"><Search size={15} /><input type="search" aria-label="搜索数据来源" placeholder="搜索名称、主机或数据库" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
          <button className="secondary-button" type="button" onClick={refresh}><RefreshCw size={15} />刷新</button>
          <button className="primary-button" type="button" disabled={!canManage} title={!canManage ? "需要 source.manage 权限" : undefined} onClick={() => setSourceDialog({ mode: "create" })}><Plus size={15} />新建连接</button>
          <button className="secondary-button" type="button" disabled={!canManage} onClick={() => setImportTarget("new")}><FileUp size={15} />导入工件</button>
        </div>
      </header>
      {testingSourceId && <div className="source-action-notice" role="status"><LoaderCircle className="is-spinning" size={16} /><span>正在测试 {sourceName(testingSourceId)} 的连接…</span></div>}
      {actionNotice && <div className={`source-action-notice${actionNotice.failed ? " is-danger" : ""}`} role={actionNotice.failed ? "alert" : "status"}>{actionNotice.failed ? <CircleAlert size={16} /> : <CheckCircle2 size={16} />}<span>{actionNotice.message}</span>{actionNotice.runId && <button type="button" className="secondary-button" onClick={() => setSelectedRunId(actionNotice.runId!)}>查看运行<ChevronRight size={14} /></button>}<button className="icon-button" type="button" aria-label="关闭操作结果" onClick={() => setActionNotice(null)}><X size={14} /></button></div>}
      {error && state !== "error" && <div className="source-inline-notice" role="status"><AlertTriangle size={15} /><span>{error}</span><button type="button" aria-label="关闭提示" onClick={() => setError("")}><X size={14} /></button></div>}
      {state === "error" && <LoadFailure message={error} onRetry={refresh} />}
      {ingestion.refreshWarning && <p role="status">服务端已保存；列表刷新失败：{ingestion.refreshWarning}</p>}
      {state === "loading" && <Loading label="正在读取数据来源" />}
      {state === "ready" && (
        <TablePanel role="tabpanel" aria-label="数据来源列表">
          {initialSourceId && !sources.some((source) => source.id === initialSourceId) && <div className="source-inline-notice" role="alert"><CircleAlert size={15} /><span>找不到目标来源 <code>{initialSourceId}</code>；未自动选择其他来源。</span></div>}
          <TableViewport>
            <Table className="source-data-table" aria-label="数据来源">
              <colgroup><col style={{ width: "32%" }} /><col style={{ width: "14%" }} /><col style={{ width: "21%" }} /><col style={{ width: "11%" }} /><col style={{ width: "22%" }} /></colgroup>
              <TableHeader>
                <TableRow>
                  <TableHead scope="col">名称</TableHead>
                  <TableHead scope="col">连接检查</TableHead>
                  <TableHead scope="col">最近发现</TableHead>
                  <TableHead scope="col">来源状态</TableHead>
                  <TableHead scope="col" className="source-actions-heading">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {visibleSources.map((source) => (
                  <TableRow
                    className={`source-data-row${source.id === initialSourceId ? " is-deep-link-target" : ""}`}
                    key={source.id}
                    ref={source.id === initialSourceId ? focusedSourceRef : undefined}
                    tabIndex={source.id === initialSourceId ? -1 : undefined}
                    aria-current={source.id === initialSourceId ? "true" : undefined}
                    data-source-id={source.id}
                    onOpen={() => setDetailSourceId(source.id)}
                  >
                    <TableCell><span className="source-primary"><span className="source-mark">{source.sourceKind === "postgresql" ? <Server size={18} /> : <FileUp size={18} />}</span><span><TableOpenButton id={`source-open-${source.id}`} aria-label={`查看来源 ${source.name}`} title={source.name} onClick={() => setDetailSourceId(source.id)}><strong>{source.name}</strong></TableOpenButton><small title={source.sourceKind === "postgresql" ? `${source.host}:${source.port}/${source.database}` : sourceKindLabel(source)}>{sourceKindLabel(source)}{source.sourceKind === "postgresql" ? ` · ${source.database}` : ""}</small></span></span></TableCell>
                    <TableCell className="source-status">{source.sourceKind !== "postgresql" ? "文件来源" : testingSourceId === source.id ? "检查中" : testResult[source.id] ? <span className={testResult[source.id].ok ? "is-success" : "is-danger"}>{testResult[source.id].message}</span> : <span className="table-secondary">本次会话未检查</span>}</TableCell>
                    <TableCell><LatestSourceRun runs={runs.filter((run) => run.sourceConnectionId === source.id)} onOpen={setSelectedRunId} /></TableCell>
                    <TableCell className="source-status"><span className={`health-label source-health-${source.status}`}><i />{sourceStatusLabel(source.status)}</span></TableCell>
                    <TableCell className="source-row-actions-cell">
                      <span className="source-row-actions source-primary-actions">
                        <button className="source-discover-button" type="button" aria-label={`启动发现 ${source.name}`} title={!canRun ? "需要 ingestion.run 权限" : source.status !== "active" ? "来源未启用" : "读取元数据并生成语义候选"} disabled={!canRun || source.status !== "active" || Boolean(startingSourceId)} onClick={() => void handleStart(source)}>{startingSourceId === source.id ? <LoaderCircle className="is-spinning" size={14} /> : <Play size={14} />}<span>{startingSourceId === source.id ? "提交中" : "启动发现"}</span></button>
                        <ActionMenu label={`更多操作 ${source.name}`} actions={[
                          source.sourceKind === "postgresql"
                            ? { label: testingSourceId === source.id ? "测试中" : "测试连接", icon: <CheckCircle2 size={15} />, disabled: !canManage || Boolean(testingSourceId), reason: !canManage ? "需要 source.manage 权限" : undefined, onSelect: () => void handleTest(source) }
                            : { label: "查看工件", icon: <Search size={15} />, onSelect: () => void ingestion.loadArtifactSet(source.activeArtifactSetId).then(setArtifactDetail).catch((reason) => setError(messageFor(reason, "工件读取失败。"))) },
                          { label: "编辑连接", icon: <Pencil size={15} />, disabled: !canManage, reason: !canManage ? "需要 source.manage 权限" : undefined, onSelect: () => setSourceDialog({ mode: "edit", source }) },
                          source.sourceKind === "postgresql"
                            ? { label: "轮换凭据", icon: <KeyRound size={15} />, disabled: !canManage, reason: !canManage ? "需要 source.manage 权限" : undefined, onSelect: () => setSourceDialog({ mode: "credential", source }) }
                            : { label: "更新工件", icon: <FileUp size={15} />, disabled: !canManage, reason: !canManage ? "需要 source.manage 权限" : undefined, onSelect: () => setImportTarget(source) },
                          { label: "删除连接", icon: <Trash2 size={15} />, danger: true, disabled: !canManage, reason: !canManage ? "需要 source.manage 权限" : undefined, onSelect: () => setDeleteTarget(source) },
                        ]} />
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
                {visibleSources.length === 0 && <TableRow><TableCell colSpan={5}><Empty icon={<Database size={19} />} title={query ? "已加载来源中没有匹配项" : "还没有数据来源"} detail="" /></TableCell></TableRow>}
              </TableBody>
            </Table>
          </TableViewport>
          <TableFooter>
            <span className="source-table-summary">{sourceTotal === 0 ? "共 0 个来源" : `显示 ${sourceRangeStart}-${sourceRangeEnd}，共 ${sourceTotal} 个来源`}</span>
            <label className="source-page-size"><span>每页</span><select aria-label="每页行数" value={sourcePageSize} onChange={(event) => { setSourcePageSize(Number(event.target.value)); setSourcePageIndex(0); setSourcePageCursors([undefined]); }}><option value={25}>25</option><option value={50}>50</option><option value={100}>100</option></select><span>行</span></label>
            <div className="source-pagination" aria-label="来源分页">
              <button className="icon-button" type="button" aria-label="上一页来源" title="上一页" disabled={sourcePageIndex === 0 || sourcePageLoading} onClick={() => void loadPreviousSources()}><ChevronLeft size={15} /></button>
              <span>第 {sourcePageIndex + 1} 页</span>
              <button className="icon-button" type="button" aria-label="下一页来源" title="下一页" disabled={!sourcePageNextCursor || sourcePageLoading} onClick={() => void loadMoreSources()}><ChevronRight size={15} /></button>
            </div>
          </TableFooter>
        </TablePanel>
      )}
      </div>
      {detailSourceId && <SourceWorkspace
        source={sources.find((source) => source.id === detailSourceId)}
        tab={sourceDetailTab} onTabChange={setSourceDetailTab}
        runs={runs.filter((run) => run.sourceConnectionId === detailSourceId)} candidates={candidates}
        testResult={testResult[detailSourceId]} busy={Boolean(startingSourceId || testingSourceId)} canManage={canManage} canRun={canRun}
        onClose={() => { const id = detailSourceId; setDetailSourceId(""); setSourceDetailTab("概览"); queueMicrotask(() => document.getElementById(`source-open-${id}`)?.focus()); }}
        onTest={() => void handleTest(sources.find((source) => source.id === detailSourceId)!)}
        onStart={() => void handleStart(sources.find((source) => source.id === detailSourceId)!)}
        onEdit={() => setSourceDialog({ mode: "edit", source: sources.find((source) => source.id === detailSourceId) })}
        onOpenRun={setSelectedRunId} onOpenProposal={onOpenProposal}

        onOpenArtifact={(source) => { if (source.sourceKind !== "postgresql") void ingestion.loadArtifactSet(source.activeArtifactSetId).then(setArtifactDetail).catch((reason) => setError(messageFor(reason, "工件读取失败。"))); }}
        onUpdateArtifact={(source) => setImportTarget(source)} onRefresh={refresh}
        hasMoreRuns={Boolean(runCursors[detailSourceId])} onMoreRuns={() => void loadSourceRunsForDetail(detailSourceId)}
      />}
      {detailSourceId && actionNotice && <div className="source-action-notice" role={actionNotice.failed ? "alert" : "status"}><span>{actionNotice.message}</span>{actionNotice.runId && <button className="secondary-button" onClick={() => setSelectedRunId(actionNotice.runId!)}>查看运行<ChevronRight size={14} /></button>}</div>}
      {detailSourceId && error && <p role="alert">{error}</p>}
      {importTarget && <ArtifactImportDialog source={importTarget === "new" ? undefined : importTarget} onClose={() => setImportTarget(null)} onSaved={(value) => { setArtifactDetail(value); setImportTarget(null); refresh(); }} />}
      {artifactDetail && <ArtifactSetDialog value={artifactDetail} onClose={() => setArtifactDetail(null)} />}
      {sourceDialog && <SourceDialog mode={sourceDialog.mode} source={sourceDialog.source} workspaceId={workspaceId} onClose={() => setSourceDialog(null)} onSaved={(saved) => { setSources((current) => [saved, ...current.filter((item) => item.id !== saved.id)]); setSourcePageItems((current) => [saved, ...current.filter((item) => item.id !== saved.id)].slice(0, sourcePageSize)); setSourceTotal((current) => current + (sources.some((item) => item.id === saved.id) ? 0 : 1)); setTestResult((current) => { const next = { ...current }; delete next[saved.id]; return next; }); if (sourceDialog.mode === "create") { setDetailSourceId(saved.id); setSourceDetailTab("概览"); } setSourceDialog(null); setError(""); setActionNotice({ message: `${saved.name}：${sourceDialog.mode === "credential" ? "凭据已更新" : "连接已保存"}。` }); }} />}
      {deleteTarget && <ConfirmDelete source={deleteTarget} workspaceId={workspaceId} onClose={() => setDeleteTarget(null)} onDeleted={() => { setSources((current) => current.filter((item) => item.id !== deleteTarget.id)); setSourcePageItems((current) => current.filter((item) => item.id !== deleteTarget.id)); setSourceTotal((current) => Math.max(0, current - 1)); setRuns((current) => current.filter((item) => item.sourceConnectionId !== deleteTarget.id)); setActionNotice({ message: `${deleteTarget.name}：连接已删除。` }); setDeleteTarget(null); }} />}
    </section>
  );
}

function SourceDialog({ mode, source, workspaceId, onClose, onSaved }: { mode: "create" | "edit" | "credential"; source?: SourceConnection; workspaceId: string; onClose: () => void; onSaved: (source: SourceConnection) => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  const [name, setName] = useState(source?.name ?? "");
  const postgres = source?.sourceKind === "postgresql" ? source : undefined;
  const [host, setHost] = useState(postgres?.host ?? "");
  const [port, setPort] = useState(String(postgres?.port ?? 5432));
  const [database, setDatabase] = useState(postgres?.database ?? "");
  const [username, setUsername] = useState(postgres?.username ?? "");
  const [password, setPassword] = useState("");
  const [sslMode, setSSLMode] = useState<CreateSourceRequest["sslMode"]>(postgres?.sslMode ?? "require");
  const [status, setStatus] = useState<SourceConnection["status"]>(source?.status ?? "active");
  const [artifactPaths, setArtifactPaths] = useState(postgres?.artifactPaths?.join("\n") ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const title = mode === "create" ? "新建 PostgreSQL 连接" : mode === "edit" ? (postgres ? "编辑 PostgreSQL 连接" : "编辑数据来源") : "轮换连接凭据";

  useEffect(() => {
    const handleKey = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKey);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKey);
  }, [onClose]);

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      let saved: SourceConnection;
      if (mode === "credential" && source) {
        saved = await rotateSourceCredential(workspaceId, source.id, password, source.version);
      } else if (mode === "edit" && source) {
        saved = await updateSource(workspaceId, source.id, { name: name.trim(), status, artifactPaths: splitPaths(artifactPaths), expectedVersion: source.version });
      } else {
        saved = await createSource(workspaceId, { name: name.trim(), host: host.trim(), port: Number(port), database: database.trim(), username: username.trim(), password, sslMode, artifactPaths: splitPaths(artifactPaths) });
      }
      setPassword("");
      onSaved(saved);
    } catch (reason) {
      setPassword("");
      setError(messageFor(reason, "来源保存失败。"));
    } finally {
      setSaving(false);
    }
  };

  const valid = mode === "credential" ? Boolean(password) : mode === "edit" ? Boolean(name.trim()) : Boolean(name.trim() && host.trim() && Number(port) > 0 && database.trim() && username.trim() && password);
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog source-live-dialog" role="dialog" aria-modal="true" aria-labelledby="source-live-dialog-title">
        <header><div><span className="panel-kicker">受保护来源</span><h2 id="source-live-dialog-title">{title}</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭连接配置" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body source-live-form">
          {mode === "credential" ? <><div className="source-secret-boundary"><KeyRound size={18} /><span><strong>凭据只写</strong><small>Semlia 不会回显或预填旧密码。保存后输入会立即清空。</small></span></div><label className="field-span-2"><span>新密码</span><input type="password" autoComplete="new-password" aria-label="新密码" value={password} onChange={(event) => setPassword(event.target.value)} /></label></> : <>
            <label className="field-span-2"><span>连接名称</span><input aria-label="连接名称" value={name} onChange={(event) => setName(event.target.value)} /></label>
            {mode === "create" && <><label><span>主机</span><input aria-label="主机" value={host} onChange={(event) => setHost(event.target.value)} /></label><label><span>端口</span><input aria-label="端口" inputMode="numeric" value={port} onChange={(event) => setPort(event.target.value)} /></label><label><span>数据库</span><input aria-label="数据库" value={database} onChange={(event) => setDatabase(event.target.value)} /></label><label><span>只读用户名</span><input aria-label="只读用户名" value={username} onChange={(event) => setUsername(event.target.value)} /></label><label><span>SSL 模式</span><select aria-label="SSL 模式" value={sslMode} onChange={(event) => setSSLMode(event.target.value as CreateSourceRequest["sslMode"])}><option value="require">require</option><option value="verify-ca">verify-ca</option><option value="verify-full">verify-full</option><option value="disable">disable</option></select></label><label><span>密码</span><input type="password" autoComplete="new-password" aria-label="密码" value={password} onChange={(event) => setPassword(event.target.value)} /></label></>}
            {mode === "edit" && <label><span>状态</span><select aria-label="连接状态" value={status} onChange={(event) => setStatus(event.target.value as SourceConnection["status"])}><option value="active">启用</option><option value="paused">暂停</option></select></label>}
            {(mode === "create" || postgres) && <label className="field-span-2"><span>版本化 SQL 路径 <small>可选，每行一个</small></span><textarea aria-label="版本化 SQL 路径" value={artifactPaths} onChange={(event) => setArtifactPaths(event.target.value)} placeholder="models/orders.sql" /></label>}
          </>}
          {error && <div className="source-form-error" role="alert"><CircleAlert size={15} /><span>{error}</span></div>}
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={!valid || saving} onClick={() => void save()}>{saving ? <LoaderCircle className="is-spinning" size={15} /> : mode === "credential" ? <KeyRound size={15} /> : <Database size={15} />}{saving ? "正在保存" : mode === "credential" ? "保存新凭据" : "保存连接"}</button></div></footer>
      </section>
    </div>
  );
}

function ConfirmDelete({ source, workspaceId, onClose, onDeleted }: { source: SourceConnection; workspaceId: string; onClose: () => void; onDeleted: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return <div className="dialog-backdrop" role="presentation"><section className="review-dialog compact-dialog" role="alertdialog" aria-modal="true" aria-labelledby="delete-live-source-title"><header><div><h2 id="delete-live-source-title">删除数据来源</h2></div><button className="icon-button" type="button" aria-label="关闭删除确认" onClick={onClose}><X size={18} /></button></header><div className="dialog-body"><p>删除 <strong>{source.name}</strong> 后不能再启动新运行；历史运行和证据继续保留。</p>{error && <div className="source-form-error" role="alert"><CircleAlert size={15} />{error}</div>}</div><footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="danger-button" type="button" disabled={busy} onClick={() => { setBusy(true); deleteSource(workspaceId, source.id, source.version).then(onDeleted).catch((reason) => setError(messageFor(reason, "删除失败。"))).finally(() => setBusy(false)); }}><Trash2 size={15} />{busy ? "正在删除" : "确认删除"}</button></div></footer></section></div>;
}

function RunList({ runs, sourceName, onOpen }: { runs: SourceDiscoveryRun[]; sourceName: (id: string) => string; onOpen: (id: string) => void }) {
  return <TableViewport className="ingestion-results-table"><Table className="source-data-table" aria-label="接入运行">
    <colgroup><col style={{ width: "36%" }} /><col style={{ width: "15%" }} /><col style={{ width: "24%" }} /><col style={{ width: "25%" }} /></colgroup>
    <TableHeader><TableRow><TableHead scope="col">来源与运行</TableHead><TableHead scope="col">状态</TableHead><TableHead scope="col">发现结果</TableHead><TableHead scope="col">更新时间</TableHead></TableRow></TableHeader>
    <TableBody>{runs.map((run) => <TableRow key={run.id} onOpen={() => onOpen(run.id)}>
      <TableCell><span className="source-primary"><span className="source-mark"><Clock3 size={18} /></span><span><TableOpenButton id={`discovery-row-${run.id}`} aria-label={`查看运行 ${run.id}`} title={run.id} onClick={() => onOpen(run.id)}><strong>{sourceName(run.sourceConnectionId)}</strong></TableOpenButton><small className="table-secondary" title={run.id}><code>{shortId(run.id)}</code> · {run.credentialVersion !== undefined ? `凭据 v${run.credentialVersion}` : run.artifactSetId ? `工件 ${shortId(run.artifactSetId)}` : "输入身份未提供"}</small></span></span></TableCell>
      <TableCell><span className={`source-run-state source-run-${run.status}`}>{run.status === "queued" || run.status === "running" ? <LoaderCircle className="is-spinning" size={13} /> : run.status === "failed" ? <CircleAlert size={13} /> : run.status === "degraded" ? <AlertTriangle size={13} /> : <CheckCircle2 size={13} />}{runLabels[run.status]}</span></TableCell>
      <TableCell><RunStats stats={run.stats} /></TableCell><TableCell><time>{formatTime(run.updatedAt)}</time></TableCell>
    </TableRow>)}
    {runs.length === 0 && <TableRow><TableCell colSpan={4}><Empty icon={<Clock3 size={19} />} title="还没有发现运行" detail="" /></TableCell></TableRow>}</TableBody>
  </Table></TableViewport>;
}

function CandidateList({ candidates, sourceName, onOpen }: { candidates: SemanticCandidate[]; sourceName: (id: string) => string; onOpen: (candidate: SemanticCandidate) => void; onOpenProposal: (proposalId: string) => void }) {
  const open = onOpen;
  return <TableViewport className="ingestion-results-table"><Table className="source-data-table" aria-label="语义候选">
    <colgroup><col style={{ width: "36%" }} /><col style={{ width: "16%" }} /><col style={{ width: "23%" }} /><col style={{ width: "25%" }} /></colgroup>
    <TableHeader><TableRow><TableHead scope="col">候选与来源</TableHead><TableHead scope="col">状态</TableHead><TableHead scope="col">来源 revision</TableHead><TableHead scope="col">创建时间</TableHead></TableRow></TableHeader>
    <TableBody>{candidates.map((candidate) => <TableRow key={candidate.id} onOpen={() => open(candidate)}>
      <TableCell><span className="source-primary"><span className="source-mark"><GitPullRequestArrow size={18} /></span><span><TableOpenButton aria-label={`查看语义候选 ${candidate.title}`} title={candidate.title} onClick={() => open(candidate)}><strong>{candidate.title}</strong></TableOpenButton><small className="table-secondary">{sourceName(candidate.sourceConnectionId)}</small></span></span></TableCell>
      <TableCell>{candidateLabels[candidate.status]}<small className="table-secondary">{candidate.candidateKind}</small></TableCell>
      <TableCell><code title={candidate.sourceRevisionId}>{shortId(candidate.sourceRevisionId)}</code></TableCell><TableCell><time>{formatTime(candidate.createdAt)}</time></TableCell>
    </TableRow>)}
    {candidates.length === 0 && <TableRow><TableCell colSpan={4}><Empty icon={<GitPullRequestArrow size={19} />} title="还没有语义候选" detail="" /></TableCell></TableRow>}</TableBody>
  </Table></TableViewport>;
}
function RunDetail({ detail, error, sourceName, onRetry, candidates = [], onOpenCandidate, onBack, backLabel = "返回运行记录" }: { detail: DiscoveryRun | null; error: string; sourceName: string; onRetry: () => void; candidates?: SemanticCandidate[]; onOpenCandidate?: (candidate: SemanticCandidate) => void; onBack?: () => void; backLabel?: string }) {
  if (!detail) return <section className="view source-live-view ingestion-detail" aria-label="接入运行详情">{onBack && <button className="ingestion-back" onClick={onBack}><ChevronLeft size={16} />{backLabel}</button>}{error ? <LoadFailure message={error} onRetry={onRetry} /> : <Loading label="正在读取运行详情" />}</section>;
  const active = detail.status === "running" || detail.status === "queued";
  return <section className="view source-live-view ingestion-detail" aria-label="接入运行详情">
    {onBack && <button className="ingestion-back" onClick={onBack}><ChevronLeft size={16} />{backLabel}</button>}
    <header className="ingestion-object-heading"><div><span className="panel-kicker">发现运行 · {formatTime(detail.createdAt)}</span><h2>{sourceName}</h2></div><span className={`source-run-state source-run-${detail.status}`}>{active ? <LoaderCircle className="is-spinning" size={15} /> : <CheckCircle2 size={15} />}{runLabels[detail.status]}</span></header>
    <div className="ingestion-summary-grid" aria-label="发现结果摘要">{Object.entries(detail.stats).filter(([key, value]) => key !== "findings" && typeof value === "number").map(([key, value]) => <article key={key}><span>{statLabel(key)}</span><strong>{String(value)}</strong></article>)}<article><span>诊断项</span><strong>{detail.findings.length}</strong></article><article><span>完成时间</span><strong className="is-time">{detail.completedAt ? formatTime(detail.completedAt) : active ? "进行中" : "未记录"}</strong></article></div>
    <RunCandidates key={`${detail.id}:${detail.sourceRevisionId ?? ""}:${detail.status}`} detail={detail} updates={candidates} onOpen={onOpenCandidate} />
    <section className="source-run-findings ingestion-section"><header><h3>诊断与警告</h3><span>{detail.findings.length} 项</span></header>{detail.errorCode && <div className="source-run-error"><CircleAlert size={16} /><strong>{detail.errorCode}</strong></div>}{detail.findings.length ? detail.findings.map((finding) => <article key={`${finding.sequence}-${finding.code}`}><span className={`finding-severity finding-${finding.severity}`}>{finding.severity}</span><span><strong>{finding.code}</strong><small>{finding.locator || "全局"}</small></span><code>{JSON.stringify(finding.details)}</code></article>) : <p className="ingestion-muted">{active ? "运行尚未结束" : "本次运行未记录诊断项"}</p>}</section>
    <details className="ingestion-technical"><summary>运行与来源标识</summary><dl className="ingestion-metadata"><dt>运行 ID</dt><dd><code>{detail.id}</code></dd><dt>来源版本</dt><dd><code>{detail.sourceRevisionId ?? "尚未生成"}</code></dd><dt>适配器</dt><dd>{detail.adapterVersion}</dd><dt>开始时间</dt><dd>{formatTime(detail.startedAt ?? detail.createdAt)}</dd></dl></details>
    {error && <LoadFailure message={error} onRetry={onRetry} />}
  </section>;
}

function RunCandidates({ detail, updates, onOpen }: { detail: DiscoveryRun; updates: SemanticCandidate[]; onOpen?: (candidate: SemanticCandidate) => void }) {
  const { workspaceId } = useCatalogRuntime();
  const [items, setItems] = useState<SemanticCandidate[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [retry, setRetry] = useState(0);
  const controllerRef = useRef<AbortController | null>(null);
  const busyRef = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    controllerRef.current = controller;
    void listSemanticCandidates(workspaceId, { sourceId: detail.sourceConnectionId, signal: controller.signal })
      .then((page) => { if (!controller.signal.aborted) { setItems(page.items); setCursor(page.nextCursor); } })
      .catch((reason) => { if (!controller.signal.aborted) setError(messageFor(reason, "运行候选读取失败。")); })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [workspaceId, detail.sourceConnectionId, retry]);
  const loadMore = async () => {
    if (!cursor || busyRef.current) return;
    busyRef.current = true;
    setLoading(true); setError("");
    const signal = controllerRef.current?.signal;
    try {
      const page = await listSemanticCandidates(workspaceId, { sourceId: detail.sourceConnectionId, cursor, signal });
      if (!signal?.aborted) { setItems((current) => mergeById(current, page.items)); setCursor(page.nextCursor); }
    } catch (reason) { if (!signal?.aborted) setError(messageFor(reason, "更多运行候选读取失败。")); }
    finally { busyRef.current = false; if (!signal?.aborted) setLoading(false); }
  };
  const related = mergeById(items, updates.filter((candidate) => candidate.status !== "pending"))
    .filter((candidate) => candidate.sourceConnectionId === detail.sourceConnectionId && (candidate.discoveryRunId === detail.id || Boolean(detail.sourceRevisionId && candidate.sourceRevisionId === detail.sourceRevisionId)));
  const active = ["queued", "running"].includes(detail.status);
  return <section className="ingestion-section" aria-label="关联候选"><header><div><h3>来源版本的语义候选</h3><span>{`已加载 ${related.length} 项 · 尚未发布为语义事实`}</span></div></header>
    {loading && <Loading label="正在读取运行候选" />}
    {error && <LoadFailure message={error} onRetry={() => { setLoading(true); setError(""); setRetry((value) => value + 1); }} />}
    {related.length > 0 && <div className="ingestion-candidate-grid">{related.map((candidate) => <button className="ingestion-candidate-card" key={candidate.id} onClick={() => onOpen?.(candidate)}><GitPullRequestArrow size={18} /><span><strong>{candidate.title}</strong><small>{candidateLabels[candidate.status]} · {candidate.discoveryRunId === detail.id ? "本次发现" : "同一来源版本的已有候选"}</small></span><ChevronRight size={15} /></button>)}</div>}
    {!loading && !error && !related.length && <Empty icon={<GitPullRequestArrow size={19} />} title={active ? "等待发现结果" : cursor ? "已加载结果中暂无关联候选" : "暂无关联候选"} detail="" />}
    {cursor && <button className="secondary-button" disabled={loading} onClick={() => void loadMore()}>加载更多候选</button>}
  </section>;
}

function CandidateDialog({ candidate, onClose, onOpenProposal }: { candidate: SemanticCandidate; onClose: () => void; onOpenProposal: (proposalId: string) => void }) {
  return <SemanticProductionWorkspace candidateId={candidate.id} onBack={() => { window.history.replaceState({}, "", "/sources"); onClose(); }} onOpenProposal={onOpenProposal} onOperationSelected={(id, releaseId) => window.history.replaceState({}, "", id ? `/governance?production=${encodeURIComponent(id)}${releaseId ? `&productionRelease=${encodeURIComponent(releaseId)}` : ""}` : "/governance?productionList=1")} />;
}

function ArtifactImportDialog({ source, onClose, onSaved }: { source?: SourceConnection; onClose: () => void; onSaved: (value: ArtifactSet) => void }) {
  const runtime = useIngestionRuntime();
  const [name, setName] = useState(source?.name ?? "");
  const [kind, setKind] = useState<"file" | "sql_bundle" | "dbt_bundle">(source && source.sourceKind !== "postgresql" ? source.sourceKind : "file");
  const [files, setFiles] = useState<File[]>([]);
  const [paths, setPaths] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const save = async () => {
    setBusy(true); setError("");
    try {
      const target = source ? { sourceId: source.id, expectedSourceVersion: source.version } : {};
      const value = kind === "sql_bundle"
        ? await runtime.registerSQLSource({ sourceName: name.trim(), paths: splitPaths(paths), ...target })
        : await runtime.createArtifactSource({ sourceName: name.trim(), ...target, files: files.map((file) => ({ file, kind: artifactKind(file, kind) })) });
      onSaved(value);
    } catch (reason) { setError(messageFor(reason, "工件保存失败。")); }
    finally { setBusy(false); }
  };
  return <IngestionDialog title={source ? "更新来源工件" : "导入工件"} onClose={onClose} busy={busy}>
    <div className="source-live-form">
      <label className="field-span-2"><span>来源名称</span><input aria-label="来源名称" value={name} onChange={(event) => setName(event.target.value)} /></label>
      <label className="field-span-2"><span>工件类型</span><select aria-label="工件类型" disabled={Boolean(source)} value={kind} onChange={(event) => { setKind(event.target.value as typeof kind); setFiles([]); setError(""); }}><option value="file">CSV / XLSX / Markdown</option><option value="sql_bundle">版本化 SQL</option><option value="dbt_bundle">dbt manifest / catalog</option></select></label>
      {kind === "sql_bundle" ? <label className="field-span-2"><span>配置根目录内 SQL 路径</span><textarea aria-label="配置根目录内 SQL 路径" placeholder="models/orders.sql" value={paths} onChange={(event) => setPaths(event.target.value)} /></label> : <label className="field-span-2"><span>{kind === "dbt_bundle" ? "manifest.json 与可选 catalog.json" : "CSV、XLSX 或 Markdown 文件"}</span><input key={kind} type="file" multiple aria-label="上传工件" accept={kind === "dbt_bundle" ? ".json" : ".csv,.xlsx,.md"} onChange={(event) => { setFiles(Array.from(event.target.files ?? [])); setError(""); }} /></label>}
      {files.length > 0 && <ul className="ingestion-file-preview field-span-2">{files.map((file, index) => <li key={`${file.name}:${index}`}><strong>{file.name}</strong><span>{file.size.toLocaleString()} bytes</span></li>)}</ul>}
      {error && <p className="source-form-error field-span-2" role="alert">{error}</p>}
    </div>
    <footer><span /><button className="primary-button" disabled={busy || !runtime.access.manage || !name.trim() || (kind === "sql_bundle" ? !splitPaths(paths).length : !files.length)} onClick={() => void save()}><FileUp size={15} />{busy ? "正在校验并保存" : "校验并保存来源"}</button></footer>
  </IngestionDialog>;
}

function artifactKind(file: File, sourceKind: "file" | "dbt_bundle"): PublicArtifactKind {
  const name = file.name.toLowerCase();
  if (sourceKind === "dbt_bundle") {
    if (name === "manifest.json") return "dbt_manifest";
    if (name === "catalog.json") return "dbt_catalog";
    throw new Error("dbt 工件必须命名为 manifest.json 或 catalog.json。");
  }
  if (name.endsWith(".csv")) return "csv";
  if (name.endsWith(".xlsx")) return "xlsx";
  if (name.endsWith(".md")) return "markdown";
  throw new Error("仅支持 CSV、XLSX 和 Markdown 文件。");
}

function ArtifactSetDialog({ value, onClose }: { value: ArtifactSet; onClose: () => void }) {
  return <IngestionDialog title="持久工件集合" onClose={onClose}>
    <dl className="ingestion-metadata"><dt>集合</dt><dd><code>{value.id}</code></dd><dt>来源</dt><dd><code>{value.sourceId}</code></dd><dt>摘要</dt><dd><code>{value.setDigest}</code></dd></dl>
    <ul className="ingestion-file-preview">{value.members.map((member) => <li key={member.artifactId}><span><strong>{member.logicalPath}</strong><small>{member.kind} · {member.byteSize.toLocaleString()} bytes · {member.contentAvailability}</small></span><code>{shortId(member.contentDigest)}</code></li>)}</ul>
  </IngestionDialog>;
}

function ScheduleWorkspace({ sources, runs = [], candidates = [], loading, sourceError, onRefresh, sourceCursor, loadingMoreSources, onLoadMoreSources, embedded = false, onOpenProposal }: { sources: SourceConnection[]; runs?: SourceDiscoveryRun[]; candidates?: SemanticCandidate[]; loading: boolean; sourceError: string; onRefresh: () => void; sourceCursor?: string; loadingMoreSources: boolean; onLoadMoreSources: () => void; embedded?: boolean; onOpenProposal: (id: string) => void }) {
  const runtime = useIngestionRuntime();
  const newSourceId = sources[0]?.id ?? "";
  const [scheduleQuery, setScheduleQuery] = useState("");
  const [editing, setEditing] = useState<SourceSchedule | "new" | null>(null);
  const [deleting, setDeleting] = useState<SourceSchedule | null>(null);
  const [selectedId, setSelectedId] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [runId, setRunId] = useState("");
  const [runDetail, setRunDetail] = useState<DiscoveryRun | null>(null);
  const [runError, setRunError] = useState("");
  const [retry, setRetry] = useState(0);
  const { workspaceId } = useCatalogRuntime();
  const [candidate, setCandidate] = useState<SemanticCandidate | null>(null);
  const [scheduleStatus, setScheduleStatus] = useState("");
  const openScheduleRun = (id: string) => { setRunDetail(null); setRunError(""); setRunId(id); };
  const sourceKey = sources.map((source) => source.id).join(",");
  const sourceIds = useMemo(() => sourceKey ? sourceKey.split(",") : [], [sourceKey]);
  const loadSchedules = runtime.loadSchedules;
  const loadOccurrences = runtime.loadOccurrences;
  useEffect(() => {
    sourceIds.forEach((sourceId) => { void loadSchedules(sourceId).catch(() => undefined); });
  }, [loadSchedules, sourceIds]);
  useEffect(() => { if (selectedId) void loadOccurrences(selectedId).catch(() => undefined); }, [loadOccurrences, selectedId]);
  useEffect(() => {
    if (!runId) return;
    const controller = new AbortController();
    let polling = false;
    let terminal = false;
    const read = async () => {
      if (polling || terminal) return;
      polling = true;
      try { const detail = await getDiscoveryRun(workspaceId, runId, controller.signal); if (!controller.signal.aborted) { setRunDetail(detail); setRunError(""); terminal = !["queued", "running"].includes(detail.status); } }
      catch (reason) { if (!controller.signal.aborted) setRunError(messageFor(reason, "运行读取失败。")); }
      finally { polling = false; }
    };
    void read();
    const timer = window.setInterval(() => void read(), 5000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [workspaceId, runId, retry]);
  const occurrences = runtime.occurrencesBySchedule[selectedId];
  const schedulePages = sourceIds.map((sourceId) => ({
    sourceId,
    source: sources.find((item) => item.id === sourceId),
    page: runtime.schedulesBySource[sourceId],
  })).filter((item): item is typeof item & { source: SourceConnection } => Boolean(item.source));
  const visibleSchedules = schedulePages.flatMap(({ source, page }) => (page?.items ?? [])
    .filter((schedule) => !schedule.deletedAt)
    .map((schedule) => ({ schedule, sourceName: source.name })));
  const selectedSchedule = visibleSchedules.find(({ schedule }) => schedule.id === selectedId);
  const scheduleKey = visibleSchedules.map(({ schedule }) => schedule.id).join(",");
  useEffect(() => {
    if (scheduleKey) scheduleKey.split(",").forEach((id) => { void loadOccurrences(id).catch(() => undefined); });
  }, [scheduleKey, loadOccurrences]);
  const filteredSchedules = visibleSchedules.filter(({ schedule, sourceName }) => (!scheduleStatus || (schedule.enabled ? "enabled" : "paused") === scheduleStatus) && `${sourceName} ${scheduleSummary(schedule.expression)} ${schedule.expression} ${schedule.id} ${schedule.timezone}`.toLowerCase().includes(scheduleQuery.trim().toLowerCase()));
  const scheduleLoading = schedulePages.some(({ page }) => page?.status.state === "loading");
  const scheduleErrors = schedulePages.filter(({ page }) => page?.status.error);
  const scheduleReady = schedulePages.length > 0 && schedulePages.every(({ page }) => page && ["ready", "empty"].includes(page.status.state));
  const act = async (schedule: SourceSchedule, command: () => Promise<unknown>) => {
    setBusy(schedule.id); setError("");
    try { await command(); } catch (reason) { setError(messageFor(reason, "计划命令失败。")); }
    finally { setBusy(""); }
  };
  if (candidate) return <CandidateDialog candidate={candidate} onClose={() => { setCandidate(null); onRefresh(); }} onOpenProposal={onOpenProposal} />;
  if (runId) return <RunDetail detail={runDetail} error={runError} sourceName={sources.find((source) => source.id === runDetail?.sourceConnectionId)?.name ?? "发现运行"} candidates={candidates} onOpenCandidate={setCandidate} onRetry={() => setRetry((value) => value + 1)} onBack={() => { setRunId(""); onRefresh(); }} backLabel="返回计划" />;
  return <section className={`view source-live-view source-list-page schedule-list-page${embedded ? " ingestion-embedded" : ""}`} aria-label="接入自动化">
    <header className="source-commandbar"><div className="source-tabs" role="tablist" aria-label="计划类型"><button type="button" role="tab" aria-selected="true"><Clock3 size={15} />全部计划 <span>{visibleSchedules.length}</span></button></div><div className="source-command-actions"><label className="connection-search"><Search size={15} /><input type="search" aria-label="搜索接入计划" placeholder="搜索来源、计划或时区" value={scheduleQuery} onChange={(event) => setScheduleQuery(event.target.value)} /></label><button className="secondary-button" title="刷新" aria-label="刷新计划" onClick={() => { onRefresh(); sourceIds.forEach((sourceId) => { void loadSchedules(sourceId).catch(() => undefined); }); if (selectedId) void loadOccurrences(selectedId).catch(() => undefined); }}><RefreshCw size={15} />刷新</button><button className="primary-button" disabled={!sources.length || !runtime.access.manage || !runtime.access.run} title={!sources.length ? "需要先创建来源" : "需要 source.manage 与 ingestion.run 权限"} onClick={() => setEditing("new")}><Plus size={15} />新建计划</button></div></header>
    {loading && <Loading label="正在读取来源" />}
    <div className="ingestion-filterbar"><select aria-label="计划状态" value={scheduleStatus} onChange={(event) => setScheduleStatus(event.target.value)}><option value="">全部状态</option><option value="enabled">已启用</option><option value="paused">已暂停</option></select></div>
    {sourceCursor && <button className="secondary-button" disabled={loadingMoreSources} onClick={onLoadMoreSources}>加载更多来源</button>}
    {(sourceError || error) && <p role="alert">{error || sourceError}</p>}
    {runtime.refreshWarning && <p role="status">服务端已保存；列表刷新失败：{runtime.refreshWarning}</p>}
    {scheduleLoading && <Loading label="正在读取计划" />}
    {scheduleErrors.map(({ sourceId, source, page }) => <LoadFailure key={sourceId} message={`${source.name}：${page?.status.error ?? "接入计划读取失败。"}`} onRetry={() => void loadSchedules(sourceId).catch(() => undefined)} />)}
    <TablePanel aria-label="接入计划列表"><TableViewport><Table className="source-data-table" aria-label="接入计划">
      <colgroup><col style={{ width: "27%" }} /><col style={{ width: "23%" }} /><col style={{ width: "12%" }} /><col style={{ width: "15%" }} /><col style={{ width: "23%" }} /></colgroup>
      <TableHeader><TableRow><TableHead scope="col">来源与频率</TableHead><TableHead scope="col">下次执行</TableHead><TableHead scope="col">状态</TableHead><TableHead scope="col">最近执行</TableHead><TableHead scope="col" className="source-actions-heading">操作</TableHead></TableRow></TableHeader>
      <TableBody>{filteredSchedules.map(({ schedule, sourceName }) => <TableRow key={schedule.id} onOpen={() => setSelectedId(schedule.id)}>
        <TableCell><span className="source-primary"><span className="source-mark"><Clock3 size={18} /></span><span><TableOpenButton aria-label={`查看计划 ${schedule.id}`} title={sourceName} onClick={() => setSelectedId(schedule.id)}><strong>{sourceName}</strong></TableOpenButton><small title={schedule.expression}>{scheduleSummary(schedule.expression)}</small></span></span></TableCell>
        <TableCell>{schedule.nextRunAt ? formatTime(schedule.nextRunAt) : "未安排"}<small className="table-secondary">{schedule.timezone}</small></TableCell>
        <TableCell><span className={`health-label source-health-${schedule.enabled ? "active" : "paused"}`}><i />{schedule.enabled ? "启用" : "暂停"}</span><small className="table-secondary">v{schedule.version}</small></TableCell>
        <TableCell>{runtime.occurrencesBySchedule[schedule.id]?.items[0] ? <OccurrenceResult occurrence={runtime.occurrencesBySchedule[schedule.id].items[0]} runs={runs} /> : runtime.occurrencesBySchedule[schedule.id]?.status.error ? "读取失败" : runtime.occurrencesBySchedule[schedule.id]?.status.state === "empty" ? "暂无执行" : "读取中"}</TableCell>
        <TableCell className="source-row-actions-cell"><div className="source-row-actions source-primary-actions">
          <button className="source-discover-button" type="button" aria-label={`立即运行计划 ${schedule.id}`} title={!runtime.access.run ? "需要 ingestion.run 权限" : "立即运行计划"} disabled={!runtime.access.run || Boolean(busy)} onClick={() => void act(schedule, async () => { await runtime.runScheduleNow(schedule); setSelectedId(schedule.id); })}>{busy === schedule.id ? <LoaderCircle className="is-spinning" size={14} /> : <Play size={14} />}<span>{busy === schedule.id ? "处理中" : "立即运行"}</span></button>
          <ActionMenu label={`更多操作计划 ${schedule.id}`} actions={[
            { label: "编辑计划", icon: <Pencil size={15} />, disabled: !runtime.access.manage || Boolean(busy), reason: !runtime.access.manage ? "需要 source.manage 权限" : undefined, onSelect: () => setEditing(schedule) },
            { label: schedule.enabled ? "暂停计划" : "恢复计划", icon: schedule.enabled ? <Pause size={15} /> : <Play size={15} />, disabled: !runtime.access.manage || (!schedule.enabled && !runtime.access.run) || Boolean(busy), reason: !runtime.access.manage ? "需要 source.manage 权限" : !schedule.enabled && !runtime.access.run ? "需要 ingestion.run 权限" : undefined, onSelect: () => void act(schedule, () => schedule.enabled ? runtime.pauseSchedule(schedule) : runtime.resumeSchedule(schedule)) },
            { label: "执行记录", icon: <Clock3 size={15} />, onSelect: () => setSelectedId(schedule.id) },
            { label: "删除计划", icon: <Trash2 size={15} />, danger: true, disabled: !runtime.access.manage || Boolean(busy), reason: !runtime.access.manage ? "需要 source.manage 权限" : undefined, onSelect: () => setDeleting(schedule) },
          ]} />
        </div></TableCell>
      </TableRow>)}
      {scheduleReady && filteredSchedules.length === 0 && <TableRow><TableCell colSpan={5}><Empty icon={<Clock3 size={19} />} title={scheduleQuery ? "没有匹配的接入计划" : "暂无接入计划"} detail="" /></TableCell></TableRow>}</TableBody>
    </Table></TableViewport>
    <TableFooter><span className="source-table-summary">显示 {filteredSchedules.length} 个计划 · 已加载 {visibleSchedules.length} 个计划</span><div className="schedule-page-actions">
    {schedulePages.map(({ sourceId, source, page }) => <div key={`${sourceId}-pagination`}>{page?.appendError && <p role="alert">{source.name}：{page.appendError}</p>}{page?.nextCursor && <button className="secondary-button" disabled={page.loadingMore} onClick={() => void runtime.loadMoreSchedules(sourceId)}>加载更多计划 · {source.name}</button>}</div>)}
    </div></TableFooter></TablePanel>
    {selectedId && <IngestionDialog title="接入计划详情" onClose={() => setSelectedId("")}>
      <div className="list-detail-heading"><span className="source-mark"><Clock3 size={20} /></span><div><h3>{selectedSchedule?.sourceName ?? "来源不可用"}</h3><span>{selectedSchedule ? scheduleSummary(selectedSchedule.schedule.expression) : ""} · {selectedSchedule?.schedule.timezone}</span></div></div>
      <dl className="ingestion-metadata list-detail-metadata"><dt>计划状态</dt><dd>{selectedSchedule ? selectedSchedule.schedule.enabled ? "已启用" : "已暂停" : "不可用"}</dd><dt>下次执行</dt><dd>{selectedSchedule?.schedule.nextRunAt ? formatTime(selectedSchedule.schedule.nextRunAt) : "未安排"}</dd><dt>错过执行时</dt><dd>{selectedSchedule?.schedule.misfirePolicy === "skip" ? "跳过" : "合并补跑一次"}</dd></dl>
      {selectedSchedule && <div className="ingestion-dialog-footer"><button className="secondary-button" disabled={!runtime.access.manage || Boolean(busy)} onClick={() => { setEditing(selectedSchedule.schedule); setSelectedId(""); }}><Pencil size={15} />编辑计划</button><button className="secondary-button" disabled={!runtime.access.manage || (!selectedSchedule.schedule.enabled && !runtime.access.run) || Boolean(busy)} onClick={() => void act(selectedSchedule.schedule, () => selectedSchedule.schedule.enabled ? runtime.pauseSchedule(selectedSchedule.schedule) : runtime.resumeSchedule(selectedSchedule.schedule))}>{selectedSchedule.schedule.enabled ? <Pause size={15} /> : <Play size={15} />}{selectedSchedule.schedule.enabled ? "暂停计划" : "恢复计划"}</button></div>}
      {error && <p role="alert">{error}</p>}
      <section className="ingestion-occurrences" aria-label="计划执行记录"><h3>执行记录</h3>{occurrences?.status.state === "loading" && <Loading label="正在读取执行记录" />}{occurrences?.status.error && <LoadFailure message={occurrences.status.error} onRetry={() => void loadOccurrences(selectedId).catch(() => undefined)} />}{occurrences?.status.state === "empty" && <p>暂无执行记录</p>}{occurrences?.items.map((occurrence) => <article key={occurrence.id}><span>{formatTime(occurrence.eligibleAt)}<small className="table-secondary">{occurrence.triggerKind === "run_now" ? "手动触发" : "定时触发"}</small></span><OccurrenceResult occurrence={occurrence} runs={runs} />{(occurrence.discoveryRunId || occurrence.runtimeRunId) && <button className="ingestion-link" onClick={() => openScheduleRun(occurrence.discoveryRunId ?? occurrence.runtimeRunId!)}>打开运行<ChevronRight size={14} /></button>}</article>)}{occurrences?.appendError && <p role="alert">{occurrences.appendError}</p>}{occurrences?.nextCursor && <button className="secondary-button" disabled={occurrences.loadingMore} onClick={() => void runtime.loadMoreOccurrences(selectedId)}>加载更多执行记录</button>}</section>
      <details className="ingestion-technical"><summary>计划标识与 Cron</summary><dl className="ingestion-metadata"><dt>计划 ID</dt><dd><code>{selectedId}</code></dd><dt>Cron</dt><dd><code>{selectedSchedule?.schedule.expression}</code></dd><dt>版本</dt><dd>{selectedSchedule?.schedule.version}</dd></dl></details>
    </IngestionDialog>}
    {editing && <ScheduleDialog sources={sources} sourceId={editing === "new" ? newSourceId : editing.sourceId} schedule={editing === "new" ? undefined : editing} onClose={() => setEditing(null)} />}
    {deleting && <IngestionDialog title="删除接入计划" onClose={() => setDeleting(null)} busy={busy === deleting.id}><p>删除此计划后，历史执行记录继续保留。</p>{error && <p role="alert">{error}</p>}<footer><span /><button className="danger-button" disabled={busy === deleting.id} onClick={() => void act(deleting, async () => { await runtime.deleteSchedule(deleting); setDeleting(null); })}><Trash2 size={15} />确认删除计划</button></footer></IngestionDialog>}
  </section>;
}

function ScheduleDialog({ sources, sourceId, schedule, onClose }: { sources: SourceConnection[]; sourceId: string; schedule?: SourceSchedule; onClose: () => void }) {
  const runtime = useIngestionRuntime();
  const [selectedSourceId, setSelectedSourceId] = useState(sourceId);
  const [expression, setExpression] = useState(schedule?.expression ?? "0 2 * * *");
  const initialDaily = /^(\d{1,2}) (\d{1,2}) \* \* \*$/.exec(schedule?.expression ?? "0 2 * * *");
  const [mode, setMode] = useState(initialDaily ? "daily" : "custom");
  const [time, setTime] = useState(initialDaily ? `${initialDaily[2].padStart(2, "0")}:${initialDaily[1].padStart(2, "0")}` : "02:00");
  const [timezone, setTimezone] = useState(schedule?.timezone ?? "Asia/Shanghai");
  const [misfirePolicy, setMisfirePolicy] = useState<SourceSchedule["misfirePolicy"]>(schedule?.misfirePolicy ?? "skip");
  const [enabled, setEnabled] = useState(schedule?.enabled ?? true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const save = async () => {
    setBusy(true); setError("");
    try {
      const input = { expression: expression.trim(), timezone: timezone.trim(), misfirePolicy };
      if (schedule) await runtime.updateSchedule(schedule, { ...input, enabled });
      else await runtime.createSchedule(selectedSourceId, input);
      onClose();
    } catch (reason) { setError(messageFor(reason, "计划保存失败。")); }
    finally { setBusy(false); }
  };
  const updateFrequency = (nextMode: string, nextTime: string) => {
    setMode(nextMode); setTime(nextTime);
    if (nextMode !== "custom" && nextTime) {
      const [hour, minute] = nextTime.split(":").map(Number);
      setExpression(`${minute} ${nextMode === "hourly" ? "*" : hour} * * ${nextMode === "weekdays" ? "1-5" : "*"}`);
    }
  };
  return <IngestionDialog title={schedule ? "编辑接入计划" : "新建接入计划"} onClose={onClose} busy={busy}>
    <fieldset disabled={busy} className="source-live-form schedule-form ingestion-fieldset">
      {!schedule && <label className="field-span-2"><span>计划来源</span><select aria-label="计划来源" value={selectedSourceId} onChange={(event) => setSelectedSourceId(event.target.value)}><option value="">选择来源</option>{sources.map((source) => <option key={source.id} value={source.id}>{source.name}</option>)}</select></label>}
      <label><span>执行频率</span><select aria-label="执行频率" value={mode} onChange={(event) => updateFrequency(event.target.value, time)}><option value="daily">每天</option><option value="weekdays">工作日（周一至周五）</option><option value="hourly">每小时</option><option value="custom">自定义 Cron</option></select></label>
      {mode !== "custom" && <label><span>{mode === "hourly" ? "每小时的分钟" : "执行时间"}</span>{mode === "hourly" ? <input aria-label="每小时的分钟" type="number" min={0} max={59} value={Number(time.split(":")[1])} onChange={(event) => updateFrequency(mode, `00:${event.target.value.padStart(2, "0")}`)} /> : <input type="time" aria-label="执行时间" value={time} onChange={(event) => updateFrequency(mode, event.target.value)} />}</label>}
      {mode === "custom" && <label className="field-span-2"><span>Cron 表达式</span><input aria-label="Cron 表达式" value={expression} onChange={(event) => setExpression(event.target.value)} /></label>}
      <label><span>时区</span><input list="ingestion-timezones" aria-label="IANA 时区" value={timezone} onChange={(event) => setTimezone(event.target.value)} /><datalist id="ingestion-timezones"><option value="Asia/Shanghai" /><option value="UTC" /><option value="Asia/Tokyo" /><option value="America/New_York" /><option value="Europe/London" /></datalist></label>
      <label><span>错过执行</span><select aria-label="错过执行" value={misfirePolicy} onChange={(event) => setMisfirePolicy(event.target.value as typeof misfirePolicy)}><option value="skip">跳过</option><option value="run_once">合并补跑一次</option></select></label>
      <div className="ingestion-schedule-preview field-span-2"><Clock3 size={18} /><span><strong>{scheduleSummary(expression)}</strong><small>{timezone}</small></span></div>
      {schedule && <label className="schedule-enabled-field field-span-2"><input type="checkbox" aria-label="启用计划" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} /><span>启用计划</span></label>}
      {error && <p role="alert" className="source-form-error field-span-2">{error}</p>}
    </fieldset><footer className="schedule-dialog-actions"><button className="secondary-button" disabled={busy} onClick={onClose}>取消</button><button className="primary-button" disabled={busy || !selectedSourceId || !expression.trim() || !timezone.trim() || (mode !== "custom" && !time) || !runtime.access.manage || (enabled && !runtime.access.run)} onClick={() => void save()}><Clock3 size={15} />{busy ? "正在保存" : schedule ? "保存计划" : "保存并启用计划"}</button></footer>
  </IngestionDialog>;
}

function scheduleSummary(expression: string) {
  const parts = expression.trim().split(/\s+/);
  const [minute, hour, day, month, weekday] = parts;
  const validMinute = /^\d{1,2}$/.test(minute ?? "") && Number(minute) < 60;
  const validHour = /^\d{1,2}$/.test(hour ?? "") && Number(hour) < 24;
  if (parts.length === 5 && validMinute && day === "*" && month === "*") {
    if (hour === "*" && weekday === "*") return `每小时 ${minute.padStart(2, "0")} 分`;
    if (validHour && ["*", "1-5"].includes(weekday)) return `${weekday === "*" ? "每天" : "周一至周五"} ${hour.padStart(2, "0")}:${minute.padStart(2, "0")}`;
  }
  return `自定义 · ${expression}`;
}

function OccurrenceResult({ occurrence, runs }: { occurrence: SourceScheduleOccurrence; runs: SourceDiscoveryRun[] }) {
  const { workspaceId } = useCatalogRuntime();
  const id = occurrence.discoveryRunId ?? occurrence.runtimeRunId;
  const knownStatus = runs.find((run) => run.id === id)?.status;
  const [status, setStatus] = useState<DiscoveryRun["status"]>();
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    if (!id || occurrence.state === "skipped") return;
    if (knownStatus && !["queued", "running"].includes(knownStatus)) return;
    const controller = new AbortController();
    let pending = false;
    let terminal = false;
    const read = async () => {
      if (pending || terminal) return;
      pending = true;
      try { const result = await getDiscoveryRun(workspaceId, id, controller.signal); if (!controller.signal.aborted) { setStatus(result.status); setFailed(false); terminal = !["queued", "running"].includes(result.status); } }
      catch { if (!controller.signal.aborted) setFailed(true); }
      finally { pending = false; }
    };
    void read();
    const timer = window.setInterval(() => void read(), 5000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [id, knownStatus, occurrence.state, workspaceId]);
  if (occurrence.state === "skipped") return <span>已跳过<small className="table-secondary">{occurrence.reasonCode ?? "未提供原因"}</small></span>;
  const resolved = knownStatus && !["queued", "running"].includes(knownStatus) ? knownStatus : status ?? knownStatus;
  return <span>{resolved ? runLabels[resolved] : failed ? "状态读取失败" : "正在读取"}<small className="table-secondary">已触发</small></span>;
}

function IngestionDialog({ title, children, onClose, busy = false }: { title: string; children: ReactNode; onClose: () => void; busy?: boolean }) {
  const ref = useRef<HTMLElement>(null);
  const closeRef = useRef(onClose);
  const busyRef = useRef(busy);
  useEffect(() => { closeRef.current = onClose; busyRef.current = busy; }, [busy, onClose]);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const element = ref.current;
    element?.querySelector<HTMLElement>("button")?.focus();
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !busyRef.current) closeRef.current();
      if (event.key !== "Tab") return;
      const controls = [...(element?.querySelectorAll<HTMLElement>('button, input:not([type="hidden"]), select, textarea, a[href], summary, [tabindex]') ?? [])].filter((control) => {
        if (control.matches(':disabled') || (control.hasAttribute("tabindex") && Number(control.getAttribute("tabindex")) < 0) || control.closest('[hidden], [inert]')) return false;
        for (let ancestor: HTMLElement | null = control; ancestor && ancestor !== element; ancestor = ancestor.parentElement) {
          const style = getComputedStyle(ancestor);
          if (style.display === "none" || style.visibility === "hidden") return false;
          if (ancestor instanceof HTMLDetailsElement && !ancestor.open && !ancestor.querySelector(":scope > summary")?.contains(control)) return false;
        }
        return true;
      });
      const first = controls[0]; const last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    element?.addEventListener("keydown", handleKey);
    return () => { element?.removeEventListener("keydown", handleKey); previous?.focus(); };
  }, []);
  return <div className="dialog-backdrop" role="presentation"><section ref={ref} className="review-dialog source-live-dialog ingestion-dialog" role="dialog" aria-modal="true" aria-label={title}><header><h2>{title}</h2><button className="icon-button" aria-label={`关闭${title}`} disabled={busy} onClick={onClose}><X size={18} /></button></header><div className="dialog-body">{children}</div></section></div>;
}

function SourceWorkspace({ source, runs, candidates, testResult, busy, canManage, canRun, onClose, onTest, onStart, onEdit, onOpenRun, onOpenProposal, onOpenArtifact, onUpdateArtifact, onRefresh, hasMoreRuns, onMoreRuns, tab, onTabChange }: {
  source?: SourceConnection; runs: SourceDiscoveryRun[]; candidates: SemanticCandidate[]; testResult?: { ok: boolean; message: string }; busy: boolean; canManage: boolean; canRun: boolean;
  onClose: () => void; onTest: () => void; onStart: () => void; onEdit: () => void; onOpenRun: (id: string) => void; onOpenArtifact: (source: SourceConnection) => void; onUpdateArtifact: (source: SourceConnection) => void; onRefresh: () => void; hasMoreRuns: boolean; onMoreRuns: () => void;
  onOpenProposal: (id: string) => void;
  tab: string; onTabChange: (tab: string) => void;
}) {
  const runtime = useIngestionRuntime();
  const loadSchedules = runtime.loadSchedules;
  const sourceId = source?.id;
  useEffect(() => { if (sourceId) void loadSchedules(sourceId).catch(() => undefined); }, [sourceId, loadSchedules]);
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => { closeRef.current?.focus(); }, []);
  if (!source) return <p role="alert">来源已不可用。<button onClick={onClose}>返回数据来源</button></p>;
  const latest = [...runs].sort((a, b) => b.createdAt.localeCompare(a.createdAt))[0];
  const plans = runtime.schedulesBySource[source.id];
  return <section className="ingestion-detail source-object" aria-label="数据来源详情">
    <button ref={closeRef} className="ingestion-back" onClick={onClose}><ChevronLeft size={16} />返回数据来源</button>
    <header className="ingestion-object-heading"><div><span className="panel-kicker">{sourceKindLabel(source)} · {sourceStatusLabel(source.status)}</span><h2>{source.name}</h2><span className="ingestion-muted">{source.sourceKind === "postgresql" ? `${source.host}:${source.port}/${source.database}` : "版本化文件来源"}</span></div><div className="ingestion-object-actions">{source.sourceKind === "postgresql" && <button className="secondary-button" disabled={!canManage || busy} onClick={onTest}><CheckCircle2 size={15} />测试连接</button>}<button className="primary-button" disabled={!canRun || busy || source.status !== "active"} onClick={onStart}><Play size={15} />启动发现</button></div></header>
    <div className="source-tabs ingestion-object-tabs" role="tablist" aria-label="来源详情视图">{["概览", "运行历史", "自动化", "连接设置"].map((name) => <button key={name} role="tab" aria-selected={tab === name} onClick={() => onTabChange(name)}>{name}</button>)}</div>
    {tab === "概览" && <>
      <div className="ingestion-summary-grid"><article><span>连接检查</span><strong className="is-time">{source.sourceKind === "postgresql" ? testResult ? testResult.ok ? "检查通过" : "检查失败" : "本次会话未检查" : "文件来源"}</strong></article><article><span>最近发现</span><strong className="is-time">{latest ? runLabels[latest.status] : "尚未运行"}</strong><small>{latest ? formatTime(latest.createdAt) : ""}</small></article><article><span>自动化</span><strong className="is-time">{plans?.status.error ? "读取失败" : plans && ["ready", "empty"].includes(plans.status.state) ? `${plans.items.filter((plan) => plan.enabled && !plan.deletedAt).length} 个计划已启用` : "读取中"}</strong><button className="ingestion-link" onClick={() => onTabChange("自动化")}>管理计划<ChevronRight size={14} /></button></article></div>
      <section className="ingestion-section"><header><h3>最近发现结果</h3>{latest && <button className="secondary-button" onClick={() => onOpenRun(latest.id)}>查看结果<ChevronRight size={15} /></button>}</header>{latest ? <RunStats stats={latest.stats} /> : <Empty icon={<Database size={20} />} title="尚无发现结果" detail="" />}</section>
      <section className="ingestion-section"><header><h3>采集输入</h3></header><dl className="ingestion-metadata">{source.sourceKind === "postgresql" ? <><dt>元数据来源</dt><dd>{source.database} · {source.username} 可访问的 Catalog</dd><dt>SQL 工件</dt><dd>{source.artifactPaths?.length ? source.artifactPaths.join("\n") : "未配置"}</dd></> : <><dt>工件集合</dt><dd><button className="ingestion-link" onClick={() => onOpenArtifact(source)}>查看已保存文件<ChevronRight size={14} /></button></dd></>}</dl></section>
    </>}
    {tab === "运行历史" && <><RunList runs={runs} sourceName={() => source.name} onOpen={onOpenRun} />{hasMoreRuns && <button className="secondary-button" onClick={onMoreRuns}>加载更多运行</button>}</>}
    {tab === "自动化" && <ScheduleWorkspace sources={[source]} runs={runs} candidates={candidates} onOpenProposal={onOpenProposal} loading={false} sourceError="" onRefresh={onRefresh} loadingMoreSources={false} onLoadMoreSources={() => {}} embedded />}
    {tab === "连接设置" && <section className="ingestion-section"><header><h3>连接配置</h3><button className="secondary-button" disabled={!canManage} onClick={onEdit}><Pencil size={15} />编辑配置</button></header><dl className="ingestion-metadata"><dt>来源 ID</dt><dd><code>{source.id}</code></dd>{source.sourceKind === "postgresql" ? <><dt>地址</dt><dd><code>{source.host}:{source.port}</code></dd><dt>数据库</dt><dd>{source.database}</dd><dt>连接用户</dt><dd>{source.username}</dd><dt>SSL 模式</dt><dd>{source.sslMode}</dd><dt>凭据版本</dt><dd>v{source.credentialVersion}</dd></> : <><dt>工件集合</dt><dd>{source.activeArtifactSetId}<button className="secondary-button" disabled={!canManage} onClick={() => onUpdateArtifact(source)}><FileUp size={15} />更新工件</button></dd></>}<dt>创建时间</dt><dd>{formatTime(source.createdAt)}</dd><dt>更新时间</dt><dd>{formatTime(source.updatedAt)}</dd></dl></section>}
  </section>;
}

function LatestSourceRun({ runs, onOpen }: { runs: SourceDiscoveryRun[]; onOpen: (id: string) => void }) {
  const latest = [...runs].sort((a, b) => b.createdAt.localeCompare(a.createdAt))[0];
  return latest ? <button className="ingestion-link" aria-label={`最近发现 ${latest.id}`} onClick={() => onOpen(latest.id)}><span>{runLabels[latest.status]}<small className="table-secondary">{formatTime(latest.createdAt)}</small></span><ChevronRight size={14} /></button> : <span className="table-secondary">尚未运行</span>;
}

function sourceKindLabel(source: SourceConnection) {
  return { postgresql: "PostgreSQL", file: "文件", sql_bundle: "SQL", dbt_bundle: "dbt" }[source.sourceKind];
}

function mergeById<T extends { id: string }>(current: T[], additions: T[]): T[] {
  const values = new Map(current.map((item) => [item.id, item]));
  additions.forEach((item) => values.set(item.id, item));
  return [...values.values()];
}

function LoadFailure({ message, onRetry }: { message: string; onRetry: () => void }) {
  return <div className="source-load-failure" role="alert"><CircleAlert size={20} /><span><strong>真实数据加载失败</strong><small>{message}</small></span><button className="secondary-button" type="button" onClick={onRetry}><RefreshCw size={15} />重试</button></div>;
}

function Loading({ label }: { label: string }) {
  return <div className="source-live-loading" role="status"><LoaderCircle className="is-spinning" size={18} />{label}</div>;
}

function Empty({ icon, title, detail }: { icon: ReactNode; title: string; detail: string }) {
  return <div className="source-empty-state" role="status">{icon}<span><strong>{title}</strong><small>{detail}</small></span></div>;
}

function RunStats({ stats }: { stats: Record<string, unknown> }) {
  const values = Object.entries(stats).filter(([, value]) => typeof value === "number").slice(0, 2);
  return <span className="source-run-stats">{values.length ? values.map(([key, value]) => <small key={key}>{statLabel(key)} <strong>{String(value)}</strong></small>) : <small>暂无统计</small>}</span>;
}

function statLabel(key: string) { return ({ datasets: "数据集", fields: "字段", lineage: "血缘关系", candidates: "候选", tables: "表", views: "视图", findings: "诊断项" } as Record<string, string>)[key] ?? key; }

function splitPaths(value: string): string[] {
  return value.split(/\r?\n/).map((item) => item.trim()).filter(Boolean);
}

function sourceStatusLabel(status: SourceConnection["status"]): string {
  return status === "active" ? "启用" : status === "paused" ? "暂停" : "已删除";
}

function formatTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).format(date);
}

function shortId(value: string): string {
  return value.length > 24 ? `${value.slice(0, 15)}…${value.slice(-6)}` : value;
}

function idempotencyKey(action: string, subject: string): string {
  return `${action}:${subject}:${globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`}`;
}

function messageFor(reason: unknown, fallback: string): string {
  if (reason instanceof DiscoveryApiError) return reason.traceId ? `${reason.message}（trace ${reason.traceId}）` : reason.message;
  return reason instanceof Error ? reason.message : fallback;
}

import { createPortal } from "react-dom";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  Activity,
  Ban,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  Clock3,
  Download,
  FileClock,
  Filter,
  RefreshCw,
  RotateCcw,
  ServerCog,
  Settings2,
  ShieldCheck,
  X,
} from "lucide-react";

import type {
  OperationsAuditEvent,
  OperationsAuditFilter,
  OperationsRunFilter,
  OperationsRuntimePolicy,
  OperationsRuntimeRun,
  OperationsRuntimeRunKind,
  OperationsRuntimeRunState,
  UpdateOperationsRuntimeSettingsInput,
} from "./operations";
import { useOperationsRuntime, type OperationsAccess, type OperationsDataState } from "./operationsRuntime";

type RuntimeTab = "runs" | "audit" | "settings";

const runKinds: OperationsRuntimeRunKind[] = ["discovery", "validation", "agent", "semantic_resolution", "embedding_rebuild", "webhook_delivery", "query_execution", "audit_export"];
const runStates: OperationsRuntimeRunState[] = ["queued", "running", "succeeded", "degraded", "failed", "cancelled", "dead_letter"];

const kindLabels: Record<OperationsRuntimeRunKind, string> = {
  discovery: "发现",
  validation: "验证",
  agent: "Agent",
  semantic_resolution: "语义解析",
  embedding_rebuild: "向量索引重建",
  webhook_delivery: "Webhook 投递",
  query_execution: "查询执行",
  audit_export: "审计导出",
};

const stateLabels: Record<OperationsRuntimeRunState, string> = {
  queued: "排队中",
  running: "运行中",
  succeeded: "成功",
  degraded: "有警告",
  failed: "失败",
  cancelled: "已取消",
  dead_letter: "死信",
};

interface RunFilterDraft {
  kind: "" | OperationsRuntimeRunKind;
  state: "" | OperationsRuntimeRunState;
  sourceType: string;
  sourceId: string;
  traceId: string;
}

interface AuditFilterDraft {
  actorId: string;
  eventType: string;
  objectType: string;
  objectId: string;
  traceId: string;
  from: string;
  to: string;
}

type SettingsDraft = Record<keyof UpdateOperationsRuntimeSettingsInput, string>;

const emptyRunFilter: RunFilterDraft = { kind: "", state: "", sourceType: "", sourceId: "", traceId: "" };
const emptyAuditFilter: AuditFilterDraft = { actorId: "", eventType: "", objectType: "", objectId: "", traceId: "", from: "", to: "" };

export function AuditRuntimeView({ initialRunId, onInitialRunHandled, onNotify }: {
  initialRunId?: string;
  onInitialRunHandled: () => void;
  onNotify: (message: string) => void;
}) {
  const runtime = useOperationsRuntime();
  const { access } = runtime;
  const [selectedTab, setSelectedTab] = useState<RuntimeTab>(() => access.runtimeRead || access.runtimeManage ? "runs" : "audit");
  const activeTab = availableTab(selectedTab, access);
  const [runDraft, setRunDraft] = useState<RunFilterDraft>(emptyRunFilter);
  const [auditDraft, setAuditDraft] = useState<AuditFilterDraft>(emptyAuditFilter);
  const [selectedAudit, setSelectedAudit] = useState<OperationsAuditEvent | null>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const openRuntimeRun = runtime.openRun;

  useEffect(() => {
    if (!initialRunId) return;
    void openRuntimeRun(initialRunId);
    onInitialRunHandled();
  }, [initialRunId, onInitialRunHandled, openRuntimeRun]);

  const openRun = (runId: string) => {
    returnFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    void openRuntimeRun(runId);
  };
  const openAudit = (event: OperationsAuditEvent) => {
    returnFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setSelectedAudit(event);
  };
  const restoreFocus = () => window.setTimeout(() => returnFocusRef.current?.focus(), 0);
  const closeRun = () => { runtime.closeRun(); restoreFocus(); };
  const closeAudit = () => { setSelectedAudit(null); restoreFocus(); };

  const runMetrics = useMemo(() => ({
    running: runtime.runs.filter((run) => run.state === "running").length,
    queued: runtime.runs.filter((run) => run.state === "queued").length,
    attention: runtime.runs.filter((run) => run.state === "failed" || run.state === "degraded" || run.state === "dead_letter").length,
  }), [runtime.runs]);

  return <section className="view settings-view audit-runtime-view" aria-label="审计与运行">
    <header className="audit-runtime-header">
      <div className="audit-runtime-tabs" role="tablist" aria-label="审计与运行视图">
        {(access.runtimeRead || access.runtimeManage) && <button type="button" role="tab" aria-selected={activeTab === "runs"} onClick={() => setSelectedTab("runs")}><Activity size={14} />运行记录</button>}
        {access.auditRead && <button type="button" role="tab" aria-selected={activeTab === "audit"} onClick={() => setSelectedTab("audit")}><ShieldCheck size={14} />审计日志</button>}
        {(access.runtimeRead || access.runtimeManage) && <button type="button" role="tab" aria-selected={activeTab === "settings"} onClick={() => setSelectedTab("settings")}><Settings2 size={14} />运行设置</button>}
      </div>
    </header>

    {activeTab === "runs" && (access.runtimeRead ? <>
      <section className="audit-runtime-metrics" aria-label="运行状态概览">
        <article><span><Activity size={14} />运行中</span><strong>{runMetrics.running}</strong><small>当前页服务端记录</small></article>
        <article><span><Clock3 size={14} />排队中</span><strong>{runMetrics.queued}</strong><small>当前页服务端记录</small></article>
        <article><span><CircleAlert size={14} />需要关注</span><strong>{runMetrics.attention}</strong><small>失败、警告或死信</small></article>
      </section>
      <RunFilters draft={runDraft} onChange={setRunDraft} onApply={() => void runtime.applyRunFilter(runFilterFromDraft(runDraft))} onReset={() => { setRunDraft(emptyRunFilter); void runtime.applyRunFilter({}); }} />
      <section className="audit-runtime-table" aria-label="全局运行记录">
        <div className="audit-runtime-run-head" aria-hidden="true"><span>来源对象</span><span>类型与阶段</span><span>进度</span><span>更新时间</span><span>状态</span><span /></div>
        <div className="audit-runtime-table-body">
          <DataBoundary state={runtime.runsState} error={runtime.runsError} emptyTitle="没有运行记录" emptyDetail="当前筛选没有服务端运行记录。" onRetry={() => void runtime.refreshRuns()} />
          {(runtime.runsState === "ready" || runtime.runsState === "loading") && runtime.runs.map((run) => <RunRow key={run.id} run={run} onOpen={() => openRun(run.id)} />)}
        </div>
      </section>
      <PaginationBar nextCursor={runtime.runsNextCursor} loading={runtime.runsLoadingMore} error={runtime.runsAppendError} onLoad={() => void runtime.loadMoreRuns()} label="运行记录" />
    </> : <RuntimeReadRequired />)}

    {activeTab === "audit" && <>
      <AuditFilters draft={auditDraft} onChange={setAuditDraft} onApply={() => void runtime.applyAuditFilter(auditFilterFromDraft(auditDraft))} onReset={() => { setAuditDraft(emptyAuditFilter); void runtime.applyAuditFilter({}); }} onExport={() => void runtime.createAuditExport().then((record) => onNotify(`审计导出 ${record.id} 已由服务端确认。`)).catch(() => undefined)} exportPending={runtime.exportState === "pending"} />
      <CommandBoundary state={runtime.exportState} error={runtime.exportError} conflictLabel="审计导出请求冲突，请刷新后重试。" />
      {runtime.exportRecord && <section className="operations-export-record" role="status" aria-label="审计导出状态">
        <header><CheckCircle2 size={15} /><strong>服务端已创建导出记录</strong></header>
        <dl><div><dt>Artifact ID</dt><dd><code>{runtime.exportRecord.artifactId}</code></dd></div><div><dt>内容摘要</dt><dd><code>{runtime.exportRecord.contentDigest}</code></dd></div><div><dt>行数</dt><dd>{runtime.exportRecord.rowCount.toLocaleString("zh-CN")}</dd></div><div><dt>有效期</dt><dd>{formatDateTime(runtime.exportRecord.expiresAt)}</dd></div></dl>
        {runtime.exportRefreshWarning && <p className="operations-export-refresh-warning" role="alert"><CircleAlert size={14} /><span><strong>导出记录已保留</strong>{runtime.exportRefreshWarning}</span></p>}
        <div className="operations-export-delivery"><span><ShieldCheck size={14} /><span><strong>不可变导出快照</strong>下载前会校验响应摘要与服务端导出记录一致。</span></span><button className="primary-button" type="button" disabled={runtime.downloadState === "pending"} onClick={() => void runtime.downloadAuditExport().then((download) => { saveTextDownload(download.content, download.contentType, download.filename); onNotify(`审计导出 ${runtime.exportRecord?.id} 已下载。`); }).catch(() => undefined)}>{runtime.downloadState === "pending" ? <RefreshCw className="is-spinning" size={14} /> : <Download size={14} />}{runtime.downloadState === "pending" ? "下载中" : "下载 JSON"}</button></div>
        {runtime.downloadState === "confirmed" && <p className="is-confirmed"><CheckCircle2 size={14} /><span><strong>内容已由服务端返回</strong>本次下载与导出记录的内容摘要一致。</span></p>}
        <CommandBoundary state={runtime.downloadState} error={runtime.downloadError} conflictLabel="下载摘要不一致，已阻止保存。" />
      </section>}
      <section className="audit-runtime-table audit-event-table" aria-label="审计事件">
        <div className="audit-event-head" aria-hidden="true"><span>时间</span><span>事件</span><span>操作者</span><span>目标对象</span><span>渠道</span><span>结果</span><span>Trace ID</span><span /></div>
        <div className="audit-runtime-table-body">
          <DataBoundary state={runtime.auditState} error={runtime.auditError} emptyTitle="没有审计事件" emptyDetail="当前筛选没有服务端审计事件。" onRetry={() => void runtime.refreshAudit()} />
          {(runtime.auditState === "ready" || runtime.auditState === "loading") && runtime.auditEvents.map((event) => <button className="audit-event-row" type="button" key={event.id} aria-label={`查看审计事件 ${event.eventType}`} onClick={() => openAudit(event)}><time>{formatDateTime(event.createdAt)}</time><span><strong>{event.eventType}</strong><small>{event.id} · {event.reasonCode}</small></span><span><strong>{event.actorId}</strong></span><span>{event.objectType} · {event.objectId}</span><code>{event.channel}</code><span className={`audit-runtime-status is-${outcomeTone(event.outcome)}`}><i />{event.outcome}</span><code>{event.traceId}</code><ChevronRight size={14} /></button>)}
        </div>
      </section>
      <PaginationBar nextCursor={runtime.auditNextCursor} loading={runtime.auditLoadingMore} error={runtime.auditAppendError} onLoad={() => void runtime.loadMoreAudit()} label="审计事件" />
      <footer className="audit-integrity-note"><ShieldCheck size={14} /><span><strong>审计事件不可编辑</strong><small>页面仅消费服务端脱敏投影，不读取或推导原始任务载荷。</small></span></footer>
    </>}

    {activeTab === "settings" && (access.runtimeRead ? <SettingsPanel runtime={runtime} onNotify={onNotify} /> : <RuntimeReadRequired />)}

    {runtime.selectedRunId && createPortal(<ModalFrame labelId="runtime-run-dialog-title" className="audit-runtime-detail-dialog" onClose={closeRun}>
      <RunDetailBody runtime={runtime} onClose={closeRun} onNotify={onNotify} />
    </ModalFrame>, document.body)}

    {selectedAudit && createPortal(<ModalFrame labelId="audit-event-dialog-title" className="audit-event-detail-dialog" onClose={closeAudit}>
      <AuditDetailBody event={selectedAudit} onClose={closeAudit} />
    </ModalFrame>, document.body)}
  </section>;
}

function RunFilters({ draft, onChange, onApply, onReset }: { draft: RunFilterDraft; onChange: (draft: RunFilterDraft) => void; onApply: () => void; onReset: () => void }) {
  return <form className="operations-filter-panel" aria-label="运行记录筛选" onSubmit={(event) => { event.preventDefault(); onApply(); }}>
    <label><span>运行类型</span><select aria-label="筛选运行类型" value={draft.kind} onChange={(event) => onChange({ ...draft, kind: event.target.value as RunFilterDraft["kind"] })}><option value="">全部类型</option>{runKinds.map((kind) => <option key={kind} value={kind}>{kindLabels[kind]}</option>)}</select></label>
    <label><span>运行状态</span><select aria-label="筛选运行状态" value={draft.state} onChange={(event) => onChange({ ...draft, state: event.target.value as RunFilterDraft["state"] })}><option value="">全部状态</option>{runStates.map((state) => <option key={state} value={state}>{stateLabels[state]}</option>)}</select></label>
    <TextFilter label="来源类型" value={draft.sourceType} onChange={(value) => onChange({ ...draft, sourceType: value })} />
    <TextFilter label="来源 ID" value={draft.sourceId} onChange={(value) => onChange({ ...draft, sourceId: value })} />
    <TextFilter label="Trace ID" value={draft.traceId} onChange={(value) => onChange({ ...draft, traceId: value })} />
    <FilterActions onReset={onReset} />
  </form>;
}

function AuditFilters({ draft, onChange, onApply, onReset, onExport, exportPending }: { draft: AuditFilterDraft; onChange: (draft: AuditFilterDraft) => void; onApply: () => void; onReset: () => void; onExport: () => void; exportPending: boolean }) {
  return <form className="operations-filter-panel operations-audit-filters" aria-label="审计事件筛选" onSubmit={(event) => { event.preventDefault(); onApply(); }}>
    <TextFilter label="操作者 ID" value={draft.actorId} onChange={(value) => onChange({ ...draft, actorId: value })} />
    <TextFilter label="事件类型" value={draft.eventType} onChange={(value) => onChange({ ...draft, eventType: value })} />
    <TextFilter label="对象类型" value={draft.objectType} onChange={(value) => onChange({ ...draft, objectType: value })} />
    <TextFilter label="对象 ID" value={draft.objectId} onChange={(value) => onChange({ ...draft, objectId: value })} />
    <TextFilter label="Trace ID" value={draft.traceId} onChange={(value) => onChange({ ...draft, traceId: value })} />
    <label><span>起始时间</span><input type="datetime-local" aria-label="审计起始时间" value={draft.from} onChange={(event) => onChange({ ...draft, from: event.target.value })} /></label>
    <label><span>结束时间</span><input type="datetime-local" aria-label="审计结束时间" value={draft.to} onChange={(event) => onChange({ ...draft, to: event.target.value })} /></label>
    <FilterActions onReset={onReset} />
    <button className="secondary-button operations-export-button" type="button" disabled={exportPending} onClick={onExport}>{exportPending ? <RefreshCw className="is-spinning" size={14} /> : <FileClock size={14} />}创建审计导出</button>
  </form>;
}

function TextFilter({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return <label><span>{label}</span><input type="text" maxLength={160} aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} /></label>;
}

function FilterActions({ onReset }: { onReset: () => void }) {
  return <div className="operations-filter-actions"><button className="secondary-button" type="button" onClick={onReset}><RotateCcw size={14} />重置</button><button className="primary-button" type="submit"><Filter size={14} />应用筛选</button></div>;
}

function RunRow({ run, onOpen }: { run: OperationsRuntimeRun; onOpen: () => void }) {
  const progress = progressPercent(run);
  return <button className="audit-runtime-run-row" type="button" aria-label={`查看运行 ${run.sourceId}`} onClick={onOpen}>
    <span><strong>{run.sourceId}</strong><code>{run.id}</code></span>
    <span><strong>{kindLabels[run.kind]}</strong><small>{run.phase || "未记录阶段"}</small></span>
    <span className="audit-run-progress-cell"><strong>{progress === undefined ? "无进度数据" : `${progress}%`}</strong>{progress !== undefined && <i><b style={{ width: `${progress}%` }} /></i>}</span>
    <span><strong>{formatDateTime(run.updatedAt)}</strong><small>尝试 {run.attempt} / {run.maxAttempts}</small></span>
    <span className={`audit-runtime-status is-${runTone(run.state)}`}><i />{stateLabels[run.state]}</span><ChevronRight size={14} />
  </button>;
}

function SettingsPanel({ runtime, onNotify }: { runtime: ReturnType<typeof useOperationsRuntime>; onNotify: (message: string) => void }) {
  if (runtime.policyState !== "ready" || !runtime.policy) return <DataBoundary state={runtime.policyState} error={runtime.policyError} emptyTitle="运行设置不可用" emptyDetail="服务端没有返回运行策略。" onRetry={() => void runtime.refreshPolicy()} />;
  return <ReadySettingsPanel key={runtime.policy.settings.version} runtime={runtime} policy={runtime.policy} onNotify={onNotify} />;
}

function ReadySettingsPanel({ runtime, policy, onNotify }: { runtime: ReturnType<typeof useOperationsRuntime>; policy: OperationsRuntimePolicy; onNotify: (message: string) => void }) {
  const [draft, setDraft] = useState<SettingsDraft>(() => settingsDraftFromPolicy(policy));
  const input = settingsInputFromDraft(draft);
  const dirty = input ? settingsChanged(policy, input) : true;
  const updateField = (key: keyof SettingsDraft, value: string) => setDraft((current) => ({ ...current, [key]: value }));
  const save = () => {
    if (!input) return;
    void runtime.updateSettings(input).then((confirmed) => onNotify(`运行设置版本 ${confirmed.settings.version} 已由服务端确认。`)).catch(() => undefined);
  };
  return <>
    <div className={`audit-settings-savebar${dirty ? " is-dirty" : ""}`}>
      <span className="audit-settings-save-state">{dirty ? <CircleAlert size={14} /> : <CheckCircle2 size={14} />}<span><strong>{dirty ? "有未保存的更改" : "已与服务端版本一致"}</strong><small>{input ? `当前版本 ${policy.settings.version}` : "请修正超出边界的数值"}</small></span></span>
      {runtime.access.runtimeManage ? <button className="primary-button" type="button" disabled={!dirty || !input || runtime.settingsCommandState === "pending"} onClick={save}>{runtime.settingsCommandState === "pending" ? "保存中" : "保存更改"}</button> : <span className="operations-readonly-badge">只读 · 缺少 runtime.manage</span>}
    </div>
    <CommandBoundary state={runtime.settingsCommandState} error={runtime.settingsCommandError} conflictLabel="设置已被其他会话更新，请重新载入后再保存。" />
    <div className="audit-settings-groups">
      <section className="audit-settings-group" aria-labelledby="runtime-limit-settings-title"><header><div><ServerCog size={15} /><span><h2 id="runtime-limit-settings-title">工作区运行边界</h2><small>仅影响后续运行；全部值由服务端校验版本和范围</small></span></div></header><div className="audit-setting-rows operations-setting-grid">
        <NumberSetting label="失败重试上限" detail="1 至 10 次" min={1} max={10} value={draft.retryCeiling} disabled={!runtime.access.runtimeManage} onChange={(value) => updateField("retryCeiling", value)} />
        <NumberSetting label="语句超时" detail={`100 至 300000 毫秒；查询执行取较小值，默认部署上限 10000 毫秒`} min={100} max={300000} value={draft.statementTimeoutMs} disabled={!runtime.access.runtimeManage} onChange={(value) => updateField("statementTimeoutMs", value)} />
        <NumberSetting label="Webhook 超时" detail="100 至 120000 毫秒" min={100} max={120000} value={draft.webhookTimeoutMs} disabled={!runtime.access.runtimeManage} onChange={(value) => updateField("webhookTimeoutMs", value)} />
        <NumberSetting label="查询行数上限" detail="1 至 100000 行；查询执行取较小值，默认部署上限 1000 行" min={1} max={100000} value={draft.queryRowLimit} disabled={!runtime.access.runtimeManage} onChange={(value) => updateField("queryRowLimit", value)} />
        <NumberSetting label="查询字节上限" detail="1024 至 104857600 字节；查询执行取较小值，默认部署上限 1 MiB" min={1024} max={104857600} value={draft.queryByteLimit} disabled={!runtime.access.runtimeManage} onChange={(value) => updateField("queryByteLimit", value)} />
        <NumberSetting label="运行元数据留存" detail="1 至 365 天" min={1} max={365} value={draft.runMetadataRetentionDays} disabled={!runtime.access.runtimeManage} onChange={(value) => updateField("runMetadataRetentionDays", value)} />
      </div></section>
      <DeploymentStatus policy={policy} />
    </div>
  </>;
}

function NumberSetting({ label, detail, min, max, value, disabled, onChange }: { label: string; detail: string; min: number; max: number; value: string; disabled: boolean; onChange: (value: string) => void }) {
  return <label><span><strong>{label}</strong><small>{detail}</small></span><input type="number" aria-label={label} min={min} max={max} step={1} value={value} disabled={disabled} onChange={(event) => onChange(event.target.value)} /></label>;
}

function DeploymentStatus({ policy }: { policy: OperationsRuntimePolicy }) {
  const values = [["后台 Worker", policy.deployment.workerConfigured], ["遥测", policy.deployment.telemetryConfigured], ["OIDC", policy.deployment.oidcConfigured], ["加密根", policy.deployment.encryptionConfigured]] as const;
  return <section className="audit-settings-group operations-deployment-status" aria-labelledby="deployment-status-title"><header><div><ShieldCheck size={15} /><span><h2 id="deployment-status-title">部署状态</h2><small>部署拥有，只读展示；工作区不能修改端点、密钥或信任根</small></span></div></header><div className="operations-deployment-grid">
    {values.map(([label, configured]) => <article key={label}><span className={`audit-runtime-status is-${configured ? "success" : "warning"}`}><i />{configured ? "已配置" : "未配置"}</span><strong>{label}</strong></article>)}
    <article><span className="audit-runtime-status is-queued"><i />部署管理</span><strong>审计留存</strong></article>
  </div></section>;
}

function RunDetailBody({ runtime, onClose, onNotify }: { runtime: ReturnType<typeof useOperationsRuntime>; onClose: () => void; onNotify: (message: string) => void }) {
  const detail = runtime.runDetail;
  if (runtime.runDetailState !== "ready" || !detail) return <><header><div><span className="content-label">运行详情</span><h2 id="runtime-run-dialog-title">{runtime.selectedRunId}</h2></div><CloseButton label="关闭运行详情" onClose={onClose} /></header><div className="dialog-body"><DataBoundary state={runtime.runDetailState} error={runtime.runDetailError} emptyTitle="没有运行详情" emptyDetail="服务端未返回此运行。" onRetry={() => runtime.selectedRunId && void runtime.openRun(runtime.selectedRunId)} /></div></>;
  const { run, events } = detail;
  const progress = progressPercent(run);
  const perform = (kind: "retry" | "cancel") => {
    const promise = kind === "retry" ? runtime.retryRun(run.id) : runtime.cancelRun(run.id);
    void promise.then(() => onNotify(`${kind === "retry" ? "重试" : "取消"}请求已由服务端确认，并重新载入运行状态。`)).catch(() => undefined);
  };
  return <>
    <header><div><span className="content-label">{kindLabels[run.kind]} · {stateLabels[run.state]}</span><h2 id="runtime-run-dialog-title">{run.sourceId}</h2><code>{run.id}</code></div><CloseButton label="关闭运行详情" onClose={onClose} /></header>
    <div className="dialog-body audit-runtime-detail-body">
      <section className="audit-run-detail-overview" aria-label="运行进度"><header><div><small>服务端阶段</small><strong>{run.phase || "未记录阶段"}</strong><span>{run.sourceType} · {run.sourceId}</span></div><b>{progress === undefined ? "--" : `${progress}%`}</b></header>
        {progress === undefined ? <p className="operations-no-progress">没有可计算的进度</p> : <div className="audit-run-detail-progress"><i style={{ width: `${progress}%` }} /></div>}
        <dl><div><dt>创建时间</dt><dd>{formatDateTime(run.createdAt)}</dd></div><div><dt>更新时间</dt><dd>{formatDateTime(run.updatedAt)}</dd></div><div><dt>尝试次数</dt><dd>{run.attempt} / {run.maxAttempts}</dd></div><div><dt>状态</dt><dd>{stateLabels[run.state]}</dd></div>{run.progressCurrent !== undefined && run.progressTotal !== undefined && <div><dt>处理进度</dt><dd>{run.progressCurrent.toLocaleString("zh-CN")} / {run.progressTotal.toLocaleString("zh-CN")}</dd></div>}{run.traceId && <div><dt>Trace ID</dt><dd><code>{run.traceId}</code></dd></div>}{run.jobId && <div><dt>Job ID</dt><dd><code>{run.jobId}</code></dd></div>}{run.errorCode && <div><dt>错误码</dt><dd><code>{run.errorCode}</code></dd></div>}{run.errorSummary && <div><dt>错误摘要</dt><dd>{run.errorSummary}</dd></div>}</dl>
      </section>
      <section className="audit-run-detail-log" aria-labelledby="runtime-run-event-title"><header><FileClock size={15} /><div><h3 id="runtime-run-event-title">服务端运行事件</h3><small>按持久化 sequence 排序</small></div></header><div role="log" aria-label="全局运行事件">{events.length === 0 ? <div className="audit-runtime-empty"><strong>没有运行事件</strong><span>服务端尚未记录阶段或诊断事件。</span></div> : events.map((event) => <div key={event.id}><time>{formatDateTime(event.createdAt)}</time><code>#{event.sequence}</code><strong>{event.eventType}</strong><span>{event.summary || event.phase || event.state || event.errorCode || "无摘要"}</span></div>)}</div></section>
      <CommandBoundary state={runtime.runCommandState} error={runtime.runCommandError} conflictLabel="运行状态已变化，请查看重新载入后的能力。" />
    </div>
    <footer><span className="model-dialog-boundary"><ShieldCheck size={13} />仅显示服务端脱敏运行投影</span><div>{runtime.access.runtimeManage && run.capabilities.retry && <button className="secondary-button" type="button" disabled={runtime.runCommandState === "pending"} onClick={() => perform("retry")}><RefreshCw size={14} />重试</button>}{runtime.access.runtimeManage && run.capabilities.cancel && <button className="secondary-button" type="button" disabled={runtime.runCommandState === "pending"} onClick={() => perform("cancel")}><Ban size={14} />取消运行</button>}<button className="primary-button" type="button" onClick={onClose}>关闭</button></div></footer>
  </>;
}

function AuditDetailBody({ event, onClose }: { event: OperationsAuditEvent; onClose: () => void }) {
  return <><header><div><span className="content-label">{event.channel} · {event.outcome}</span><h2 id="audit-event-dialog-title">{event.eventType}</h2><code>{event.id}</code></div><CloseButton label="关闭审计事件详情" onClose={onClose} /></header><div className="dialog-body audit-event-detail-body"><p>{event.summary}</p><dl><div><dt>操作者</dt><dd>{event.actorId}</dd></div><div><dt>目标对象</dt><dd>{event.objectType}<small>{event.objectId}</small></dd></div><div><dt>发生时间</dt><dd>{formatDateTime(event.createdAt)}</dd></div><div><dt>Trace ID</dt><dd><code>{event.traceId}</code></dd></div><div><dt>原因码</dt><dd><code>{event.reasonCode}</code></dd></div><div><dt>结果</dt><dd>{event.outcome}</dd></div></dl></div><footer><span className="model-dialog-boundary"><ShieldCheck size={13} />不可编辑的服务端审计投影</span><div><button className="primary-button" type="button" onClick={onClose}>关闭</button></div></footer></>;
}

function ModalFrame({ labelId, className, onClose, children }: { labelId: string; className: string; onClose: () => void; children: ReactNode }) {
  const dialogRef = useRef<HTMLElement>(null);
  useEffect(() => {
    const dialog = dialogRef.current;
    const focusables = () => [...(dialog?.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled]), select:not([disabled]), [href], [tabindex]:not([tabindex="-1"])') ?? [])];
    focusables()[0]?.focus();
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); onClose(); return; }
      if (event.key !== "Tab") return;
      const items = focusables();
      if (!items.length) return;
      const first = items[0];
      const last = items.at(-1)!;
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [onClose]);
  return <div className="dialog-backdrop audit-runtime-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}><section ref={dialogRef} className={`review-dialog ${className}`} role="dialog" aria-modal="true" aria-labelledby={labelId}>{children}</section></div>;
}

function CloseButton({ label, onClose }: { label: string; onClose: () => void }) {
  return <button className="icon-button" type="button" aria-label={label} onClick={onClose}><X size={16} /></button>;
}

function RuntimeReadRequired() {
  return <div className="operations-access-boundary" role="status"><CircleAlert size={18} /><span><strong>需要运行读取上下文</strong>当前会话可管理运行，但缺少 <code>runtime.read</code>，因此不能安全加载运行能力或设置版本。页面不会发起无权请求。</span></div>;
}

function DataBoundary({ state, error, emptyTitle, emptyDetail, onRetry }: { state: OperationsDataState; error: string; emptyTitle: string; emptyDetail: string; onRetry: () => void }) {
  if (state === "ready" || state === "idle") return null;
  if (state === "loading") return <div className="audit-runtime-empty" role="status"><RefreshCw className="is-spinning" size={18} /><strong>正在从服务端读取</strong><span>请稍候。</span></div>;
  if (state === "empty") return <div className="audit-runtime-empty" role="status"><FileClock size={18} /><strong>{emptyTitle}</strong><span>{emptyDetail}</span></div>;
  return <div className="operations-error-boundary" role="alert"><CircleAlert size={18} /><span><strong>{state === "forbidden" ? "访问被拒绝" : "读取失败"}</strong>{error}</span><button className="secondary-button" type="button" onClick={onRetry}>重试</button></div>;
}

function CommandBoundary({ state, error, conflictLabel }: { state: string; error: string; conflictLabel: string }) {
  if (!error) return null;
  return <div className={`operations-command-boundary is-${state}`} role="alert"><CircleAlert size={15} /><span><strong>{state === "conflict" ? conflictLabel : state === "forbidden" ? "操作被拒绝" : state === "unsupported" ? "操作不受支持" : "操作失败"}</strong>{error}</span></div>;
}

function PaginationBar({ nextCursor, loading, error, onLoad, label }: { nextCursor?: string; loading: boolean; error: string; onLoad: () => void; label: string }) {
  if (!nextCursor && !error) return null;
  return <div className="operations-pagination">{error && <span role="alert"><CircleAlert size={14} />{error}</span>}{nextCursor && <button className="secondary-button" type="button" disabled={loading} onClick={onLoad}>{loading ? "读取中" : `加载更多${label}`}</button>}</div>;
}

function runFilterFromDraft(draft: RunFilterDraft): OperationsRunFilter {
  return compact({ kind: draft.kind || undefined, state: draft.state || undefined, sourceType: draft.sourceType.trim() || undefined, sourceId: draft.sourceId.trim() || undefined, traceId: draft.traceId.trim() || undefined });
}

function availableTab(selected: RuntimeTab, access: OperationsAccess): RuntimeTab {
  if (selected === "audit" && access.auditRead) return selected;
  if ((selected === "runs" || selected === "settings") && (access.runtimeRead || access.runtimeManage)) return selected;
  return access.runtimeRead || access.runtimeManage ? "runs" : "audit";
}

function auditFilterFromDraft(draft: AuditFilterDraft): OperationsAuditFilter {
  return compact({ actorId: draft.actorId.trim() || undefined, eventType: draft.eventType.trim() || undefined, objectType: draft.objectType.trim() || undefined, objectId: draft.objectId.trim() || undefined, traceId: draft.traceId.trim() || undefined, from: dateTimeToISO(draft.from), to: dateTimeToISO(draft.to) });
}

function compact<T extends object>(value: T): T {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined && item !== "")) as T;
}

function dateTimeToISO(value: string): string | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}

function progressPercent(run: OperationsRuntimeRun): number | undefined {
  if (run.progressCurrent === undefined || run.progressTotal === undefined || run.progressTotal <= 0) return undefined;
  return Math.max(0, Math.min(100, Math.round((run.progressCurrent / run.progressTotal) * 100)));
}

function formatDateTime(value?: string): string {
  if (!value) return "未记录";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false }).format(date);
}

function runTone(state: OperationsRuntimeRunState): string {
  if (state === "succeeded") return "success";
  if (state === "running") return "running";
  if (state === "queued" || state === "cancelled") return "queued";
  if (state === "degraded") return "warning";
  return "danger";
}

function outcomeTone(outcome: string): string {
  const normalized = outcome.toLowerCase();
  if (["succeeded", "success", "accepted", "allowed", "completed"].includes(normalized)) return "success";
  if (["failed", "failure", "denied", "rejected", "error"].includes(normalized)) return "danger";
  return "warning";
}

function settingsDraftFromPolicy(policy: OperationsRuntimePolicy): SettingsDraft {
  return {
    retryCeiling: String(policy.settings.retryCeiling),
    statementTimeoutMs: String(policy.settings.statementTimeoutMs),
    webhookTimeoutMs: String(policy.settings.webhookTimeoutMs),
    queryRowLimit: String(policy.settings.queryRowLimit),
    queryByteLimit: String(policy.settings.queryByteLimit),
    runMetadataRetentionDays: String(policy.settings.runMetadataRetentionDays),
  };
}

function settingsInputFromDraft(draft: SettingsDraft): UpdateOperationsRuntimeSettingsInput | null {
  const values = {
    retryCeiling: Number(draft.retryCeiling),
    statementTimeoutMs: Number(draft.statementTimeoutMs),
    webhookTimeoutMs: Number(draft.webhookTimeoutMs),
    queryRowLimit: Number(draft.queryRowLimit),
    queryByteLimit: Number(draft.queryByteLimit),
    runMetadataRetentionDays: Number(draft.runMetadataRetentionDays),
  };
  if (!Number.isInteger(values.retryCeiling) || values.retryCeiling < 1 || values.retryCeiling > 10) return null;
  if (!Number.isInteger(values.statementTimeoutMs) || values.statementTimeoutMs < 100 || values.statementTimeoutMs > 300000) return null;
  if (!Number.isInteger(values.webhookTimeoutMs) || values.webhookTimeoutMs < 100 || values.webhookTimeoutMs > 120000) return null;
  if (!Number.isInteger(values.queryRowLimit) || values.queryRowLimit < 1 || values.queryRowLimit > 100000) return null;
  if (!Number.isInteger(values.queryByteLimit) || values.queryByteLimit < 1024 || values.queryByteLimit > 104857600) return null;
  if (!Number.isInteger(values.runMetadataRetentionDays) || values.runMetadataRetentionDays < 1 || values.runMetadataRetentionDays > 365) return null;
  return values;
}

function settingsChanged(policy: OperationsRuntimePolicy, input: UpdateOperationsRuntimeSettingsInput): boolean {
  return (Object.keys(input) as (keyof UpdateOperationsRuntimeSettingsInput)[]).some((key) => policy.settings[key] !== input[key]);
}

function saveTextDownload(content: string, contentType: string, filename: string): void {
  const href = URL.createObjectURL(new Blob([content], { type: contentType }));
  const anchor = document.createElement("a");
  anchor.href = href;
  anchor.download = filename;
  anchor.hidden = true;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  window.setTimeout(() => URL.revokeObjectURL(href), 0);
}

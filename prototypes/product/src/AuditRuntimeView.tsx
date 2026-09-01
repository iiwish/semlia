import { createPortal } from "react-dom";
import { useEffect, useMemo, useState } from "react";
import { Activity, CheckCircle2, ChevronRight, CircleAlert, Clock3, FileClock, Network, Search, Settings2, ShieldCheck, TerminalSquare, X, Zap } from "lucide-react";

import type { EmbeddingRebuildRun } from "./ModelConfigurationView";

type RuntimeTab = "runs" | "audit" | "settings";
type RuntimeStatus = "运行中" | "排队中" | "成功" | "有警告" | "失败";

interface RuntimeRun {
  id: string;
  name: string;
  type: "向量索引" | "知识构建" | "数据同步" | "MCP 调用" | "发布验证";
  trigger: string;
  object: string;
  status: RuntimeStatus;
  progress: number;
  phase: string;
  startedAt: string;
  duration: string;
  relation?: string;
}

interface AuditEvent {
  id: string;
  time: string;
  event: string;
  category: "知识" | "模型" | "接口" | "数据" | "系统";
  actor: string;
  actorType: string;
  object: string;
  channel: "Web" | "MCP" | "API" | "CLI" | "系统任务";
  result: "成功" | "失败";
  traceId: string;
  summary: string;
}

const baseRuns: RuntimeRun[] = [
  { id: "RUN-240901-1208", name: "客户增长知识增量构建", type: "知识构建", trigger: "定时任务", object: "PostgreSQL Analytics", status: "运行中", progress: 68, phase: "生成候选版本", startedAt: "2026-09-01 12:08", duration: "3 分 42 秒", relation: "上游接入 RUN-240824-1432" },
  { id: "RUN-240901-1145", name: "订单经营模型元数据同步", type: "数据同步", trigger: "Semantic Release CI", object: "commerce.orders_model", status: "排队中", progress: 0, phase: "等待执行槽位", startedAt: "2026-09-01 11:45", duration: "等待 46 秒" },
  { id: "RUN-240901-1031", name: "华东区净收入语义检索", type: "MCP 调用", trigger: "Codex MCP Workspace", object: "commerce.net_revenue", status: "成功", progress: 100, phase: "响应已返回", startedAt: "2026-09-01 10:31", duration: "1.8 秒" },
  { id: "RUN-240901-0917", name: "客单价候选版本发布验证", type: "发布验证", trigger: "林悦 · Web", object: "commerce.average_order_value @9", status: "有警告", progress: 100, phase: "证据覆盖需确认", startedAt: "2026-09-01 09:17", duration: "2 分 14 秒" },
  { id: "RUN-240831-2214", name: "客户维度全量校准", type: "知识构建", trigger: "系统任务", object: "customer.customer", status: "失败", progress: 74, phase: "Cube 编译失败", startedAt: "2026-08-31 22:14", duration: "6 分 08 秒" },
];

const auditEvents: AuditEvent[] = [
  { id: "AUD-98124", time: "12:10:42", event: "启动知识增量构建", category: "知识", actor: "system-scheduler", actorType: "系统任务", object: "客户增长知识域", channel: "系统任务", result: "成功", traceId: "tr_8f31c2", summary: "根据每小时调度策略启动增量知识构建，并创建运行 RUN-240901-1208。" },
  { id: "AUD-98123", time: "11:32:00", event: "重建向量索引", category: "模型", actor: "林悦", actorType: "成员", object: "text-embedding-3-large", channel: "Web", result: "成功", traceId: "tr_51a90d", summary: "使用默认 Embedding 模型重建全部知识目录向量，现有索引在切换前保持服务。" },
  { id: "AUD-98120", time: "11:18:36", event: "调用语义检索", category: "接口", actor: "Codex MCP Workspace", actorType: "MCP 客户端", object: "commerce.net_revenue", channel: "MCP", result: "成功", traceId: "tr_d219a4", summary: "通过 MCP 调用已发布知识，返回 3 个知识块和 1 个 Cube 查询结果。" },
  { id: "AUD-98116", time: "10:44:09", event: "轮换接口凭据", category: "接口", actor: "周明", actorType: "成员", object: "Fluxale Production", channel: "Web", result: "成功", traceId: "tr_6248de", summary: "客户端凭据已轮换。旧凭据进入短暂过渡期，密钥内容未写入审计事件。" },
  { id: "AUD-98108", time: "09:17:54", event: "发布验证未通过", category: "知识", actor: "林悦", actorType: "成员", object: "commerce.average_order_value @9", channel: "Web", result: "失败", traceId: "tr_f0226e", summary: "发布验证发现证据覆盖不足，候选版本保持未发布状态。" },
  { id: "AUD-98097", time: "08:42:21", event: "同步元数据", category: "数据", actor: "Semantic Release CI", actorType: "API 客户端", object: "commerce.orders_model", channel: "API", result: "成功", traceId: "tr_7741bc", summary: "通过 API 提交元数据版本，未检测到破坏性 Schema 变化。" },
];

function runTone(status: RuntimeStatus) {
  if (status === "成功") return "success";
  if (status === "有警告") return "warning";
  if (status === "失败") return "danger";
  if (status === "运行中") return "running";
  return "queued";
}

function auditTone(result: AuditEvent["result"]) {
  return result === "成功" ? "success" : "danger";
}

function runLogs(run: RuntimeRun) {
  if (run.type === "向量索引") return [
    ["11:32:00", "INFO", "任务创建", "现有索引继续提供检索服务。"],
    ["11:32:14", "DONE", "扫描知识块", "发现 3,420 个可索引知识块。"],
    ["11:32:16", "INFO", "生成向量", "开始批量写入临时索引。"],
    ["11:33:18", "RUNNING", run.phase, `当前完成 ${run.progress}%。`],
  ];
  const latest = run.status === "运行中"
    ? ["最近", "RUNNING", run.phase, `任务仍在运行，当前进度 ${run.progress}%。`]
    : run.status === "排队中"
      ? ["最近", "WAITING", run.phase, "等待可用执行槽位，尚未占用运行资源。"]
      : run.status === "失败"
        ? ["最近", "ERROR", run.phase, "运行已停止，等待处理后重试。"]
        : run.status === "有警告"
          ? ["最近", "WARN", run.phase, "运行完成，但存在需要人工判断的结果。"]
          : ["最近", "DONE", run.phase, "运行已完成并归档。"];
  return [
    [run.startedAt.slice(-5), "INFO", "任务创建", `${run.trigger} 已创建本次运行。`],
    [run.startedAt.slice(-5), "INFO", "加载上下文", `已锁定关联对象 ${run.object}${run.relation ? `，关联 ${run.relation}` : ""}。`],
    latest,
  ];
}

export function AuditRuntimeView({ embeddingRebuildRun, initialRunId, onInitialRunHandled, onNotify }: { embeddingRebuildRun: EmbeddingRebuildRun | null; initialRunId?: string; onInitialRunHandled: () => void; onNotify: (message: string) => void }) {
  const [activeTab, setActiveTab] = useState<RuntimeTab>("runs");
  const [runQuery, setRunQuery] = useState("");
  const [runStatus, setRunStatus] = useState("全部状态");
  const [runType, setRunType] = useState("全部类型");
  const [selectedRunId, setSelectedRunId] = useState<string | null>(() => initialRunId ?? null);
  const [auditQuery, setAuditQuery] = useState("");
  const [auditChannel, setAuditChannel] = useState("全部渠道");
  const [auditResult, setAuditResult] = useState("全部结果");
  const [selectedAuditId, setSelectedAuditId] = useState<string | null>(null);
  const [settings, setSettings] = useState({ auditRetention: "180", runRetention: "90", failureRetention: "365", concurrency: "4", queue: "FIFO · 风险优先", timeout: "30", retries: "2", otelEnabled: true, otelEndpoint: "https://otel.semlia.internal/v1" });
  const [savedSettings, setSavedSettings] = useState(settings);

  const runs = useMemo(() => {
    if (!embeddingRebuildRun) return baseRuns;
    const rebuild: RuntimeRun = { id: embeddingRebuildRun.id, name: "知识目录向量索引重建", type: "向量索引", trigger: "林悦 · Web", object: embeddingRebuildRun.modelId, status: embeddingRebuildRun.status === "已完成" ? "成功" : embeddingRebuildRun.status, progress: embeddingRebuildRun.progress, phase: embeddingRebuildRun.phase, startedAt: embeddingRebuildRun.startedAt.slice(0, 16), duration: embeddingRebuildRun.duration };
    return [rebuild, ...baseRuns];
  }, [embeddingRebuildRun]);

  useEffect(() => {
    if (initialRunId) onInitialRunHandled();
  }, [initialRunId, onInitialRunHandled]);

  const visibleRuns = runs.filter((run) => {
    const query = runQuery.trim().toLowerCase();
    return (!query || `${run.id} ${run.name} ${run.type} ${run.trigger} ${run.object}`.toLowerCase().includes(query)) && (runStatus === "全部状态" || run.status === runStatus) && (runType === "全部类型" || run.type === runType);
  });
  const visibleAuditEvents = auditEvents.filter((event) => {
    const query = auditQuery.trim().toLowerCase();
    return (!query || `${event.id} ${event.event} ${event.actor} ${event.object} ${event.traceId}`.toLowerCase().includes(query)) && (auditChannel === "全部渠道" || event.channel === auditChannel) && (auditResult === "全部结果" || event.result === auditResult);
  });
  const selectedRun = runs.find((run) => run.id === selectedRunId) ?? null;
  const selectedAudit = auditEvents.find((event) => event.id === selectedAuditId) ?? null;
  const settingsDirty = JSON.stringify(settings) !== JSON.stringify(savedSettings);

  const saveSettings = () => {
    setSavedSettings(settings);
    onNotify("审计与运行设置已保存。后续任务将使用新的运行参数。");
  };

  return <section className="view settings-view audit-runtime-view" aria-label="审计与运行">
    <header className="audit-runtime-header">
      <div className="audit-runtime-tabs" role="tablist" aria-label="审计与运行视图">
        <button type="button" role="tab" aria-selected={activeTab === "runs"} onClick={() => setActiveTab("runs")}><Activity size={14} />运行记录</button>
        <button type="button" role="tab" aria-selected={activeTab === "audit"} onClick={() => setActiveTab("audit")}><ShieldCheck size={14} />审计日志</button>
        <button type="button" role="tab" aria-selected={activeTab === "settings"} onClick={() => setActiveTab("settings")}><Settings2 size={14} />运行设置</button>
      </div>
    </header>

    {activeTab === "runs" && <>
      <section className="audit-runtime-metrics" aria-label="运行状态概览">
        <article><span><Activity size={14} />运行中</span><strong>{runs.filter((run) => run.status === "运行中").length}</strong><small>当前活动任务</small></article>
        <article><span><Clock3 size={14} />队列等待</span><strong>{runs.filter((run) => run.status === "排队中").length}</strong><small>并发上限 {settings.concurrency}</small></article>
        <article><span><CircleAlert size={14} />24 小时失败</span><strong>1</strong><small>1 项等待处理</small></article>
        <article><span><Network size={14} />日志投递</span><strong className="is-healthy">正常</strong><small>最近投递 12 秒前</small></article>
      </section>
      <div className="audit-runtime-commandbar">
        <label><Search size={14} /><input type="search" aria-label="搜索运行记录" placeholder="搜索任务、对象、Run ID" value={runQuery} onChange={(event) => setRunQuery(event.target.value)} />{runQuery && <button type="button" aria-label="清除运行搜索" onClick={() => setRunQuery("")}><X size={13} /></button>}</label>
        <select aria-label="筛选运行类型" value={runType} onChange={(event) => setRunType(event.target.value)}><option>全部类型</option>{[...new Set(runs.map((run) => run.type))].map((type) => <option key={type}>{type}</option>)}</select>
        <select aria-label="筛选运行状态" value={runStatus} onChange={(event) => setRunStatus(event.target.value)}><option>全部状态</option>{["运行中", "排队中", "成功", "有警告", "失败"].map((status) => <option key={status}>{status}</option>)}</select>
      </div>
      <section className="audit-runtime-table" aria-label="全局运行记录">
        <div className="audit-runtime-run-head" aria-hidden="true"><span>任务</span><span>类型与触发</span><span>关联对象</span><span>进度</span><span>开始与耗时</span><span>状态</span><span /></div>
        <div className="audit-runtime-table-body">
          {visibleRuns.map((run) => <button className="audit-runtime-run-row" type="button" key={run.id} aria-label={`查看运行 ${run.name}`} onClick={() => setSelectedRunId(run.id)}><span><strong>{run.name}</strong><code>{run.id}</code></span><span><strong>{run.type}</strong><small>{run.trigger}</small></span><span>{run.object}</span><span className="audit-run-progress-cell"><strong>{run.phase}</strong><small>{run.progress}%</small><i><b style={{ width: `${run.progress}%` }} /></i></span><span><strong>{run.startedAt}</strong><small>{run.duration}</small></span><span className={`audit-runtime-status is-${runTone(run.status)}`}><i />{run.status}</span><ChevronRight size={14} /></button>)}
          {visibleRuns.length === 0 && <div className="audit-runtime-empty"><Search size={18} /><strong>没有匹配的运行记录</strong><span>调整搜索词、任务类型或状态。</span></div>}
        </div>
      </section>
    </>}

    {activeTab === "audit" && <>
      <div className="audit-runtime-commandbar audit-log-commandbar">
        <label><Search size={14} /><input type="search" aria-label="搜索审计日志" placeholder="搜索事件、操作者、对象或 Trace ID" value={auditQuery} onChange={(event) => setAuditQuery(event.target.value)} />{auditQuery && <button type="button" aria-label="清除审计搜索" onClick={() => setAuditQuery("")}><X size={13} /></button>}</label>
        <select aria-label="筛选调用渠道" value={auditChannel} onChange={(event) => setAuditChannel(event.target.value)}><option>全部渠道</option>{["Web", "MCP", "API", "CLI", "系统任务"].map((channel) => <option key={channel}>{channel}</option>)}</select>
        <select aria-label="筛选审计结果" value={auditResult} onChange={(event) => setAuditResult(event.target.value)}><option>全部结果</option><option>成功</option><option>失败</option></select>
        <button className="secondary-button audit-export-button" type="button" onClick={() => onNotify("当前筛选范围的审计事件已准备导出。原型不会生成真实文件。")}>导出当前结果</button>
      </div>
      <section className="audit-runtime-table audit-event-table" aria-label="审计事件">
        <div className="audit-event-head" aria-hidden="true"><span>时间</span><span>事件</span><span>操作者 / 客户端</span><span>目标对象</span><span>渠道</span><span>结果</span><span>Trace ID</span><span /></div>
        <div className="audit-runtime-table-body">
          {visibleAuditEvents.map((event) => <button className="audit-event-row" type="button" key={event.id} aria-label={`查看审计事件 ${event.event}`} onClick={() => setSelectedAuditId(event.id)}><time>{event.time}</time><span><strong>{event.event}</strong><small>{event.id} · {event.category}</small></span><span><strong>{event.actor}</strong><small>{event.actorType}</small></span><span>{event.object}</span><code>{event.channel}</code><span className={`audit-runtime-status is-${auditTone(event.result)}`}><i />{event.result}</span><code>{event.traceId}</code><ChevronRight size={14} /></button>)}
          {visibleAuditEvents.length === 0 && <div className="audit-runtime-empty"><Search size={18} /><strong>没有匹配的审计事件</strong><span>调整搜索词、调用渠道或结果。</span></div>}
        </div>
      </section>
      <footer className="audit-integrity-note"><ShieldCheck size={14} /><span><strong>审计事件不可篡改</strong><small>凭据、Token 与敏感请求内容始终脱敏。</small></span></footer>
    </>}

    {activeTab === "settings" && <>
      <div className={`audit-settings-savebar${settingsDirty ? " is-dirty" : ""}`}>
        <span className="audit-settings-save-state">{settingsDirty ? <CircleAlert size={14} /> : <CheckCircle2 size={14} />}<span><strong>{settingsDirty ? "有未保存的更改" : "所有更改已保存"}</strong><small>{settingsDirty ? "等待保存" : "配置已生效"}</small></span></span>
        <button className="primary-button" type="button" disabled={!settingsDirty} onClick={saveSettings}>保存更改</button>
      </div>
      <div className="audit-settings-groups">
        <section className="audit-settings-group" aria-labelledby="retention-settings-title"><header><div><ShieldCheck size={15} /><span><h2 id="retention-settings-title">记录留存</h2><small>审计事件和运行日志的工作区保留周期</small></span></div></header><div className="audit-setting-rows">
          <label><span><strong>审计事件</strong><small>成员、客户端与系统操作记录</small></span><select aria-label="审计事件留存" value={settings.auditRetention} onChange={(event) => setSettings((current) => ({ ...current, auditRetention: event.target.value }))}><option value="90">90 天</option><option value="180">180 天</option><option value="365">365 天</option></select></label>
          <label><span><strong>运行日志</strong><small>阶段、进度和执行输出</small></span><select aria-label="运行日志留存" value={settings.runRetention} onChange={(event) => setSettings((current) => ({ ...current, runRetention: event.target.value }))}><option value="30">30 天</option><option value="90">90 天</option><option value="180">180 天</option></select></label>
          <label><span><strong>失败记录</strong><small>便于长期复盘和问题追踪</small></span><select aria-label="失败记录留存" value={settings.failureRetention} onChange={(event) => setSettings((current) => ({ ...current, failureRetention: event.target.value }))}><option value="180">180 天</option><option value="365">365 天</option></select></label>
        </div></section>
        <section className="audit-settings-group" aria-labelledby="scheduler-settings-title"><header><div><Zap size={15} /><span><h2 id="scheduler-settings-title">任务调度</h2><small>控制后台任务的并发、排队和失败恢复</small></span></div></header><div className="audit-setting-rows">
          <label><span><strong>最大并发任务</strong><small>超出后进入等待队列</small></span><input type="number" min="1" max="16" aria-label="最大并发任务" value={settings.concurrency} onChange={(event) => setSettings((current) => ({ ...current, concurrency: event.target.value }))} /></label>
          <label><span><strong>队列策略</strong><small>同优先级任务按创建时间执行</small></span><select aria-label="队列策略" value={settings.queue} onChange={(event) => setSettings((current) => ({ ...current, queue: event.target.value }))}><option>FIFO · 风险优先</option><option>严格 FIFO</option></select></label>
          <label><span><strong>任务超时</strong><small>超时后终止并写入失败记录</small></span><select aria-label="任务超时" value={settings.timeout} onChange={(event) => setSettings((current) => ({ ...current, timeout: event.target.value }))}><option value="15">15 分钟</option><option value="30">30 分钟</option><option value="60">60 分钟</option></select></label>
          <label><span><strong>失败重试</strong><small>仅自动重试可恢复错误</small></span><select aria-label="失败重试次数" value={settings.retries} onChange={(event) => setSettings((current) => ({ ...current, retries: event.target.value }))}><option value="0">不重试</option><option value="1">1 次</option><option value="2">2 次</option><option value="3">3 次</option></select></label>
        </div></section>
        <section className="audit-settings-group" aria-labelledby="observability-settings-title"><header><div><Network size={15} /><span><h2 id="observability-settings-title">可观测性</h2><small>向工作区外部的观测系统投递运行遥测</small></span></div><span className="audit-settings-health"><i />连接正常</span></header><div className="audit-setting-rows">
          <label><span><strong>OpenTelemetry</strong><small>运行指标、日志与 Trace</small></span><span className="audit-settings-toggle"><input type="checkbox" aria-label="启用 OpenTelemetry" checked={settings.otelEnabled} onChange={(event) => setSettings((current) => ({ ...current, otelEnabled: event.target.checked }))} /><i /></span></label>
          <label className="audit-setting-endpoint"><span><strong>OTLP Endpoint</strong><small>最近成功投递：12 秒前</small></span><input type="url" aria-label="OpenTelemetry Endpoint" value={settings.otelEndpoint} onChange={(event) => setSettings((current) => ({ ...current, otelEndpoint: event.target.value }))} /><button className="secondary-button" type="button" onClick={() => onNotify("OpenTelemetry 连接测试通过，日志投递正常。")}>测试连接</button></label>
          <div className="audit-setting-enforced"><span><ShieldCheck size={14} /><span><strong>敏感字段脱敏</strong><small>密钥、Token 和请求敏感内容不会进入日志</small></span></span><b>强制启用</b></div>
        </div></section>
      </div>
    </>}

    {selectedRun && createPortal(<div className="dialog-backdrop audit-runtime-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setSelectedRunId(null); }}><section className="review-dialog audit-runtime-detail-dialog" role="dialog" aria-modal="true" aria-labelledby="runtime-run-dialog-title"><header><div><span className="content-label">{selectedRun.type} · {selectedRun.status}</span><h2 id="runtime-run-dialog-title">{selectedRun.name}</h2><code>{selectedRun.id}</code></div><button className="icon-button" type="button" aria-label="关闭运行详情" onClick={() => setSelectedRunId(null)}><X size={16} /></button></header><div className="dialog-body audit-runtime-detail-body"><section className="audit-run-detail-overview" aria-label="运行进度"><header><div><small>当前阶段</small><strong>{selectedRun.phase}</strong><span>{selectedRun.object}</span></div><b>{selectedRun.progress}%</b></header><div className="audit-run-detail-progress"><i style={{ width: `${selectedRun.progress}%` }} /></div><dl><div><dt>触发来源</dt><dd>{selectedRun.trigger}</dd></div><div><dt>开始时间</dt><dd>{selectedRun.startedAt}</dd></div><div><dt>运行时长</dt><dd>{selectedRun.duration}</dd></div><div><dt>状态</dt><dd>{selectedRun.status}</dd></div>{selectedRun.relation && <div><dt>关联运行</dt><dd>{selectedRun.relation}</dd></div>}</dl></section><section className="audit-run-detail-log" aria-labelledby="runtime-run-log-title"><header><FileClock size={15} /><div><h3 id="runtime-run-log-title">执行日志</h3><small>最近记录</small></div></header><div role="log" aria-label="全局运行执行日志">{runLogs(selectedRun).map(([time, level, stage, message]) => <div key={`${time}-${stage}`}><time>{time}</time><code>{level}</code><strong>{stage}</strong><span>{message}</span></div>)}</div></section></div><footer><span className="model-dialog-boundary"><ShieldCheck size={13} />运行记录已归档，可供审计追溯</span><div><button className="primary-button" type="button" onClick={() => setSelectedRunId(null)}>关闭</button></div></footer></section></div>, document.body)}

    {selectedAudit && createPortal(<div className="dialog-backdrop audit-runtime-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setSelectedAuditId(null); }}><section className="review-dialog audit-event-detail-dialog" role="dialog" aria-modal="true" aria-labelledby="audit-event-dialog-title"><header><div><span className="content-label">{selectedAudit.category} · {selectedAudit.result}</span><h2 id="audit-event-dialog-title">{selectedAudit.event}</h2><code>{selectedAudit.id}</code></div><button className="icon-button" type="button" aria-label="关闭审计事件详情" onClick={() => setSelectedAuditId(null)}><X size={16} /></button></header><div className="dialog-body audit-event-detail-body"><p>{selectedAudit.summary}</p><dl><div><dt>操作者 / 客户端</dt><dd>{selectedAudit.actor}<small>{selectedAudit.actorType}</small></dd></div><div><dt>调用渠道</dt><dd>{selectedAudit.channel}</dd></div><div><dt>目标对象</dt><dd>{selectedAudit.object}</dd></div><div><dt>发生时间</dt><dd>2026-09-01 {selectedAudit.time}</dd></div><div><dt>Trace ID</dt><dd><code>{selectedAudit.traceId}</code></dd></div><div><dt>执行结果</dt><dd>{selectedAudit.result}</dd></div></dl><section><header><TerminalSquare size={14} /><h3>事件边界</h3></header><div><span><CheckCircle2 size={14} />调用身份和目标对象已记录</span><span><CheckCircle2 size={14} />关联运行与 Trace 可追溯</span><span><ShieldCheck size={14} />凭据和敏感请求内容已脱敏</span></div></section></div><footer><span className="model-dialog-boundary"><ShieldCheck size={13} />不可篡改审计事件</span><div><button className="primary-button" type="button" onClick={() => setSelectedAuditId(null)}>关闭</button></div></footer></section></div>, document.body)}
  </section>;
}

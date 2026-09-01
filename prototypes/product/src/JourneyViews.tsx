import { useCallback, useEffect, useRef, useState } from "react";
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Box,
  Braces,
  CheckCircle2,
  ChevronDown,
  CircleAlert,
  CircleHelp,
  Clock3,
  Database,
  FileSpreadsheet,
  FileSearch,
  FileClock,
  FileUp,
  Eye,
  EyeOff,
  LoaderCircle,
  Maximize2,
  Pencil,
  Play,
  Plus,
  Radio,
  RefreshCw,
  Search,
  Server,
  Settings2,
  Sparkles,
  Trash2,
  X,
  ZoomIn,
  ZoomOut,
} from "lucide-react";
interface SourcesViewProps {
  discoveryState: "ready" | "running" | "complete";
  focusIndex: number;
  navigationEpoch: number;
  runDetailBackRequestEpoch: number;
  initialRunId?: string;
  onRunDetailOpenChange: (open: boolean) => void;
  onOpenActivity: () => void;
  onOpenGlobalRun: (runId: string) => void;
  onRunDiscovery: () => void;
  onOpenGovernanceProposals: (runId: string) => void;
  onNotify: (message: string) => void;
}

interface SessionSource {
  category: "database" | "file";
  name: string;
  kind: string;
  endpoint: string;
  targetDataset?: string;
}

interface FileDataset {
  id: string;
  name: string;
  kind: "CSV" | "XLSX" | "MD";
  endpoint: string;
  detail: string;
  status: string;
  encoding: string;
  delimiter: string;
  sheet: string;
  firstRowHeader: boolean;
  chunking: "按标题层级" | "按固定长度";
  includeCodeBlocks: boolean;
}

function getFileKind(fileName: string): FileDataset["kind"] | null {
  const extension = fileName.toLowerCase().split(".").pop();
  if (extension === "xlsx") return "XLSX";
  if (extension === "csv") return "CSV";
  if (extension === "md" || extension === "markdown") return "MD";
  return null;
}

interface DatabaseConnection {
  id: string;
  name: string;
  kind: string;
  host: string;
  port: string;
  database: string;
  username: string;
  sslEnabled: boolean;
  detail: string;
  status: string;
}

type DatabaseDraft = Omit<DatabaseConnection, "id" | "detail" | "status">;

interface BuildTask {
  id: string;
  name: string;
  sources: string[];
  mode: "元数据增量" | "全量校准";
  checkpoint?: string;
  trigger: "手动运行" | "定时调度";
  cronExpression: string;
  timezone: "Asia/Shanghai" | "UTC" | "Asia/Tokyo" | "America/Los_Angeles" | "Europe/London";
  enabled: boolean;
  status: "就绪" | "已完成" | "有警告" | "停用";
  lastRun: string;
}

interface BuildRun {
  id: string;
  taskName: string;
  sourceRevision: string;
  startedAt: string;
  duration: string;
  status: "已完成" | "有警告" | "运行中";
  trigger: "定时调度" | "手动运行";
  mode: "元数据增量" | "全量校准" | "向量索引重建" | "数据库同步" | "文件导入" | "元数据扫描";
  kind?: "knowledge-build" | "embedding-rebuild" | "ingestion";
  previousSourceRevision?: string;
  sourceCount: number;
  physicalChanges: number;
  addedObjects?: number;
  modifiedObjects?: number;
  removedObjects?: number;
  unchangedObjects?: number;
  knowledgeBlocks: number;
  proposals: number;
  progress?: number;
  phase?: string;
  modelId?: string;
  processedItems?: number;
  totalItems?: number;
  sourceName?: string;
  outputSummary?: string;
  checkpoint?: string;
  downstreamRunId?: string;
}

type BuildTaskDraft = Omit<BuildTask, "id" | "status" | "lastRun" | "checkpoint">;
type BuildRunResultTab = "changes" | "evidence" | "proposals";

interface BuildRunResultItem {
  id: string;
  title: string;
  kind: string;
  summary: string;
  state: string;
  warning?: boolean;
  facts: Array<[string, string]>;
}

function buildRunDiff(run: BuildRun) {
  const added = run.addedObjects ?? Math.ceil(run.physicalChanges * 0.25);
  const removed = run.removedObjects ?? (run.physicalChanges > 2 ? 1 : 0);
  const modified = run.modifiedObjects ?? Math.max(0, run.physicalChanges - added - removed);
  return { added, modified, removed, unchanged: run.unchangedObjects ?? 1240 + run.sourceCount * 380 };
}

function buildRunResultItems(...items: BuildRunResultItem[]) {
  return items;
}

function expandBuildRunResultItems(items: BuildRunResultItem[], total: number, titles: string[], idPrefix: string) {
  return Array.from({ length: total }, (_, index) => {
    if (items[index]) return items[index];
    const template = items[index % items.length];
    const sequence = index + 1;
    return {
      ...template,
      id: `${idPrefix}-${sequence}`,
      title: titles[index - items.length] ?? `${template.kind} ${sequence}`,
      warning: false,
      facts: template.facts.map(([label, value]) => [label, label.includes("编号") || label.includes("来源") ? `${idPrefix.toUpperCase()}-${String(sequence).padStart(2, "0")}` : value]),
    } satisfies BuildRunResultItem;
  });
}

const RUN_GRAPH_WIDTH = 1060;
const RUN_GRAPH_HEIGHT = 900;

interface RunResultCanvasProps {
  runId: string;
  groups: Record<BuildRunResultTab, BuildRunResultItem[]>;
  activeTab: BuildRunResultTab;
  activeItem: BuildRunResultItem | null;
  onSelect: (tab: BuildRunResultTab, itemId: string) => void;
}

function RunResultCanvas({ runId, groups, activeTab, activeItem, onSelect }: RunResultCanvasProps) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<{ pointerId: number; startX: number; startY: number; originX: number; originY: number } | null>(null);
  const [transform, setTransform] = useState({ scale: .68, x: 18, y: 18 });
  const evidenceStep = 41;
  const changeStep = 50;
  const proposalStep = 76;
  const evidenceStart = 86;
  const changeStart = 176;
  const proposalStart = 260;
  const graphHeight = Math.max(RUN_GRAPH_HEIGHT, evidenceStart + Math.max(0, groups.evidence.length - 1) * evidenceStep + 80, changeStart + Math.max(0, groups.changes.length - 1) * changeStep + 80, proposalStart + Math.max(0, groups.proposals.length - 1) * proposalStep + 80);

  const fitCanvas = useCallback(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const width = viewport.clientWidth || 760;
    const height = viewport.clientHeight || 560;
    const scale = Math.min(1, Math.max(.28, Math.min((width - 36) / RUN_GRAPH_WIDTH, (height - 36) / graphHeight)));
    setTransform({ scale, x: Math.max(18, (width - RUN_GRAPH_WIDTH * scale) / 2), y: Math.max(18, (height - graphHeight * scale) / 2) });
  }, [graphHeight]);

  useEffect(() => {
    fitCanvas();
    const viewport = viewportRef.current;
    if (!viewport || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(fitCanvas);
    observer.observe(viewport);
    return () => observer.disconnect();
  }, [fitCanvas, runId]);

  const zoomCanvas = useCallback((nextScale: number, clientX?: number, clientY?: number) => {
    setTransform((current) => {
      const scale = Math.min(1.6, Math.max(.28, nextScale));
      const viewport = viewportRef.current;
      const rect = viewport?.getBoundingClientRect();
      const anchorX = clientX !== undefined && rect ? clientX - rect.left : (viewport?.clientWidth ?? 760) / 2;
      const anchorY = clientY !== undefined && rect ? clientY - rect.top : (viewport?.clientHeight ?? 560) / 2;
      const ratio = scale / current.scale;
      return { scale, x: anchorX - (anchorX - current.x) * ratio, y: anchorY - (anchorY - current.y) * ratio };
    });
  }, []);

  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const handleWheel = (event: WheelEvent) => {
      event.preventDefault();
      if (event.ctrlKey || event.metaKey) {
        const rect = viewport.getBoundingClientRect();
        const anchorX = event.clientX - rect.left;
        const anchorY = event.clientY - rect.top;
        const zoomFactor = Math.min(1.25, Math.max(.8, Math.exp(-event.deltaY * .01)));
        setTransform((current) => {
          const scale = Math.min(1.6, Math.max(.28, current.scale * zoomFactor));
          const ratio = scale / current.scale;
          return { scale, x: anchorX - (anchorX - current.x) * ratio, y: anchorY - (anchorY - current.y) * ratio };
        });
        return;
      }
      const multiplier = event.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16 : 1;
      setTransform((current) => ({ ...current, x: current.x - event.deltaX * multiplier, y: current.y - event.deltaY * multiplier }));
    };
    viewport.addEventListener("wheel", handleWheel, { passive: false });
    return () => viewport.removeEventListener("wheel", handleWheel);
  }, []);

  return (
    <div
      ref={viewportRef}
      className="build-run-canvas-viewport"
      role="group"
      aria-label="运行结果关系图"
      onPointerDown={(event) => {
        if (event.button !== 0 || (event.target as HTMLElement).closest("button")) return;
        dragRef.current = { pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, originX: transform.x, originY: transform.y };
        event.currentTarget.setPointerCapture(event.pointerId);
      }}
      onPointerMove={(event) => {
        const drag = dragRef.current;
        if (!drag || drag.pointerId !== event.pointerId) return;
        setTransform((current) => ({ ...current, x: drag.originX + event.clientX - drag.startX, y: drag.originY + event.clientY - drag.startY }));
      }}
      onPointerUp={(event) => {
        if (dragRef.current?.pointerId !== event.pointerId) return;
        dragRef.current = null;
        event.currentTarget.releasePointerCapture(event.pointerId);
      }}
    >
      <div className="build-run-canvas-toolbar" role="toolbar" aria-label="关系图画布工具">
        <button type="button" aria-label="缩小关系图" title="缩小" onClick={() => zoomCanvas(transform.scale - .12)}><ZoomOut size={15} /></button>
        <span aria-label={`当前缩放 ${Math.round(transform.scale * 100)}%`}>{Math.round(transform.scale * 100)}%</span>
        <button type="button" aria-label="放大关系图" title="放大" onClick={() => zoomCanvas(transform.scale + .12)}><ZoomIn size={15} /></button>
        <button type="button" aria-label="适应画布" title="适应画布" onClick={fitCanvas}><Maximize2 size={15} /></button>
      </div>
      <div className="build-run-canvas-pan-hint">双指平移 · 捏合缩放</div>
      <div className="build-run-canvas-board" style={{ height: graphHeight, transform: `translate3d(${transform.x}px, ${transform.y}px, 0) scale(${transform.scale})` }}>
        <svg className="build-run-canvas-links" style={{ height: graphHeight }} viewBox={`0 0 ${RUN_GRAPH_WIDTH} ${graphHeight}`} aria-hidden="true">
          <defs><marker id="run-result-arrow" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 Z" /></marker></defs>
          {groups.evidence.map((item, index) => { const target = index % Math.max(1, groups.changes.length); return <path key={`evidence-link-${item.id}`} d={`M280 ${evidenceStart + index * evidenceStep} C335 ${evidenceStart + index * evidenceStep} 355 ${changeStart + target * changeStep} 410 ${changeStart + target * changeStep}`} />; })}
          {groups.changes.map((item, index) => { const target = index % Math.max(1, groups.proposals.length); return <path key={`change-link-${item.id}`} d={`M650 ${changeStart + index * changeStep} C705 ${changeStart + index * changeStep} 725 ${proposalStart + target * proposalStep} 780 ${proposalStart + target * proposalStep}`} />; })}
        </svg>
        <section className="build-run-canvas-lane build-run-canvas-lane-evidence" aria-label="关联证据节点">
          <header><span><FileSearch size={15} />关联证据</span><b>{groups.evidence.length}</b></header>
          <div>{groups.evidence.map((item) => <button className={activeTab === "evidence" && activeItem?.id === item.id ? "build-run-result-node build-run-result-node-active" : "build-run-result-node"} key={item.id} type="button" aria-label={`查看关联证据 ${item.title}`} aria-pressed={activeTab === "evidence" && activeItem?.id === item.id} onClick={() => onSelect("evidence", item.id)}><span><small>{item.kind}</small><strong>{item.title}</strong></span><ArrowRight size={13} /></button>)}</div>
        </section>
        <section className="build-run-canvas-lane build-run-canvas-lane-changes" aria-label="变化对象节点">
          <header><span><Braces size={15} />变化对象</span><b>{groups.changes.length}</b></header>
          <div>{groups.changes.map((item) => <button className={`${activeTab === "changes" && activeItem?.id === item.id ? "build-run-result-node build-run-result-node-active" : "build-run-result-node"}${item.warning ? " build-run-result-node-warning" : ""}`} key={item.id} type="button" aria-label={`查看变化对象 ${item.title}`} aria-pressed={activeTab === "changes" && activeItem?.id === item.id} onClick={() => onSelect("changes", item.id)}><span><small>{item.kind}</small><strong>{item.title}</strong></span><ArrowRight size={13} /></button>)}</div>
        </section>
        <section className="build-run-canvas-lane build-run-canvas-lane-proposals" aria-label="候选版本节点">
          <header><span><Sparkles size={15} />候选版本</span><b>{groups.proposals.length}</b></header>
          <div>{groups.proposals.map((item) => <button className={`${activeTab === "proposals" && activeItem?.id === item.id ? "build-run-result-node build-run-result-node-active" : "build-run-result-node"}${item.warning ? " build-run-result-node-warning" : ""}`} key={item.id} type="button" aria-label={`查看候选版本 ${item.title}`} aria-pressed={activeTab === "proposals" && activeItem?.id === item.id} onClick={() => onSelect("proposals", item.id)}><span><small>{item.kind}</small><strong>{item.title}</strong></span><ArrowRight size={13} /></button>)}</div>
        </section>
      </div>
    </div>
  );
}

function BuildRunLogDialog({ run, onClose }: { run: BuildRun; onClose: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  const time = run.startedAt.slice(-8);
  const logEntries = run.kind === "embedding-rebuild" ? [
    [time, "INFO", "创建任务", `锁定目标模型 ${run.modelId}，现有索引继续提供服务`],
    ["11:32:04", "INFO", "扫描知识块", `发现 ${run.totalItems?.toLocaleString("zh-CN")} 个待索引知识块`],
    ["11:32:18", "INFO", "生成向量", `已处理 ${run.processedItems?.toLocaleString("zh-CN")} / ${run.totalItems?.toLocaleString("zh-CN")} 个知识块`],
    ["11:33:18", "INFO", "当前进度", `${run.phase} · ${run.progress}%`],
  ] : run.kind === "ingestion" ? [
    [time, "INFO", "连接来源", `已使用只读凭据连接 ${run.sourceName}`],
    [time, "INFO", "读取检查点", `从 ${run.previousSourceRevision ?? "首次基线"} 开始读取`],
    [time, "INFO", run.mode, run.outputSummary ?? `识别 ${run.physicalChanges} 项来源变化`],
    [time, run.status === "有警告" ? "WARN" : "INFO", "保存检查点", run.status === "有警告" ? "存在解析异常，检查点保持不变" : `${run.checkpoint} 已保存`],
    ...(run.downstreamRunId ? [[time, "INFO", "触发下游", `已创建全局运行 ${run.downstreamRunId}`]] : []),
  ] : [
    [time, "INFO", "启动运行", `${run.trigger}触发，只读连接 ${run.sourceCount} 个数据来源`],
    ["14:32:19", "INFO", "固定基线", `读取基线 ${run.previousSourceRevision ?? "首次基线"}`],
    ["14:32:31", "INFO", "计算差异", `识别 ${run.physicalChanges} 个变化对象，未变化对象复用上次结果`],
    ["14:33:04", "INFO", "关联证据", `归档 ${run.knowledgeBlocks} 项 Catalog、DDL 与使用证据`],
    ["14:34:36", run.status === "有警告" ? "WARN" : "INFO", "形成候选", `生成 ${run.proposals} 个关联候选版本`],
    ["14:34:36", "INFO", "完成运行", `运行耗时 ${run.duration}，Checkpoint ${run.status === "已完成" ? "已推进" : "保持不变"}`],
  ];

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog run-log-dialog" role="dialog" aria-modal="true" aria-labelledby="build-run-log-title">
        <header><div><h2 id="build-run-log-title">执行日志</h2><span>{run.id} · {run.taskName}</span></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭执行日志" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body run-log-body">
          <div className="run-log-summary"><span className={`build-task-status ${run.status === "有警告" ? "build-task-status-warning" : run.status === "运行中" ? "build-task-status-running" : ""}`}><i />{run.status}</span><span>{logEntries.length} 条日志</span><span>{run.duration}</span></div>
          <div className="run-log-list" role="log" aria-label="运行执行日志">
            {logEntries.map(([entryTime, level, stage, message], index) => <div className={level === "WARN" ? "run-log-entry run-log-entry-warning" : "run-log-entry"} key={`${stage}-${index}`}><time>{entryTime}</time><code>{level}</code><strong>{stage}</strong><span>{message}</span></div>)}
          </div>
        </div>
      </section>
    </div>
  );
}

const cronPresets = [
  { expression: "*/15 * * * *", label: "每 15 分钟" },
  { expression: "0 * * * *", label: "每小时" },
  { expression: "0 2 * * *", label: "每天 02:00" },
  { expression: "0 9 * * 1-5", label: "工作日 09:00" },
  { expression: "0 3 * * 1", label: "每周一 03:00" },
  { expression: "0 4 1 * *", label: "每月 1 日 04:00" },
] as const;

function isValidCronExpression(expression: string) {
  const parts = expression.trim().split(/\s+/);
  return parts.length === 5 && parts.every((part) => /^[-\dA-Za-z*/,]+$/.test(part));
}

function describeCronSchedule(expression: string) {
  return cronPresets.find((preset) => preset.expression === expression.trim())?.label ?? `Cron · ${expression.trim() || "待配置"}`;
}

function ConnectionDialog({ initialConnection, onClose, onSave }: { initialConnection?: DatabaseConnection; onClose: () => void; onSave: (source: DatabaseDraft) => void }) {
  const [name, setName] = useState(initialConnection?.name ?? "");
  const [kind, setKind] = useState(initialConnection?.kind ?? "PostgreSQL");
  const [host, setHost] = useState(initialConnection?.host ?? "");
  const [port, setPort] = useState(initialConnection?.port ?? "5432");
  const [database, setDatabase] = useState(initialConnection?.database ?? "");
  const [username, setUsername] = useState(initialConnection?.username ?? "");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [sslEnabled, setSslEnabled] = useState(initialConnection?.sslEnabled ?? true);
  const [testStatus, setTestStatus] = useState<"idle" | "testing" | "success" | "error">("idle");
  const closeRef = useRef<HTMLButtonElement>(null);
  const testTimerRef = useRef<number | undefined>(undefined);
  const canTest = Boolean(name.trim() && host.trim() && port.trim() && database.trim() && username.trim() && (initialConnection || password));

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => { document.removeEventListener("keydown", handleKeyDown); if (testTimerRef.current) window.clearTimeout(testTimerRef.current); };
  }, [onClose]);

  const invalidateTest = () => { if (testStatus !== "idle") setTestStatus("idle"); };
  const handleKindChange = (nextKind: string) => {
    setKind(nextKind);
    setPort(nextKind === "PostgreSQL" ? "5432" : nextKind === "MySQL" ? "3306" : "1433");
    invalidateTest();
  };
  const handleTest = () => {
    setTestStatus("testing");
    testTimerRef.current = window.setTimeout(() => setTestStatus(host.toLowerCase().includes("invalid") ? "error" : "success"), 650);
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog source-dialog" role="dialog" aria-modal="true" aria-labelledby="connection-dialog-title">
        <header><div><h2 id="connection-dialog-title">{initialConnection ? "编辑数据库连接" : "连接数据库"}</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭数据库连接" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body source-setup-form">
          <div className="source-form-grid source-form-grid-flat"><label><span>数据库类型 <b>*</b></span><select aria-label="数据库类型" value={kind} onChange={(event) => handleKindChange(event.target.value)}><option>PostgreSQL</option><option>MySQL</option><option>SQL Server</option></select></label><label><span>连接名称 <b>*</b></span><input aria-label="连接名称" value={name} onChange={(event) => { setName(event.target.value); invalidateTest(); }} placeholder="例如 Finance Warehouse" /></label><label className="field-span-2"><span>主机地址 <b>*</b></span><input aria-label="主机地址" value={host} onChange={(event) => { setHost(event.target.value); invalidateTest(); }} placeholder="db.example.internal" /></label><label><span>端口 <b>*</b></span><input aria-label="端口" inputMode="numeric" value={port} onChange={(event) => { setPort(event.target.value); invalidateTest(); }} /></label><label><span>数据库名称 <b>*</b></span><input aria-label="数据库名称" value={database} onChange={(event) => { setDatabase(event.target.value); invalidateTest(); }} placeholder="analytics" /></label><label><span>用户名 <b>*</b></span><input aria-label="用户名" autoComplete="off" value={username} onChange={(event) => { setUsername(event.target.value); invalidateTest(); }} placeholder="semlia_reader" /></label><label><span>密码 {initialConnection ? "" : <b>*</b>}</span><span className="password-input"><input type={showPassword ? "text" : "password"} aria-label="密码" autoComplete="new-password" value={password} onChange={(event) => { setPassword(event.target.value); invalidateTest(); }} placeholder={initialConnection ? "留空表示使用已保存凭据" : "输入只读账号密码"} /><button type="button" aria-label={showPassword ? "隐藏密码" : "显示密码"} title={showPassword ? "隐藏密码" : "显示密码"} onClick={() => setShowPassword((current) => !current)}>{showPassword ? <EyeOff size={16} /> : <Eye size={16} />}</button></span></label><label className="switch-field field-span-2"><span>传输安全</span><span><input type="checkbox" aria-label="启用 SSL" checked={sslEnabled} onChange={(event) => { setSslEnabled(event.target.checked); invalidateTest(); }} />启用 SSL 加密连接</span></label></div>
          <div className="source-capability-strip" aria-label="数据库增量能力">
            <div><Braces size={15} /><span><small>访问范围</small><strong>只读 Catalog</strong></span></div>
            <div><RefreshCw size={15} /><span><small>平台计算增量</small><strong>SourceRevision 差异</strong></span></div>
            <div className="source-capability-muted"><Radio size={15} /><span><small>行级变化</small><strong>CDC 未配置</strong></span></div>
          </div>
          <div className={`source-test-panel source-test-${testStatus}`} role="status" aria-live="polite"><span className="source-test-icon">{testStatus === "testing" ? <LoaderCircle className="spin" size={18} /> : testStatus === "success" ? <CheckCircle2 size={18} /> : testStatus === "error" ? <CircleAlert size={18} /> : <Database size={18} />}</span><span><strong>{testStatus === "testing" ? "正在测试连接" : testStatus === "success" ? "连接测试通过" : testStatus === "error" ? "连接测试失败" : "保存前需要测试连接"}</strong><small>{testStatus === "success" ? "认证成功，可读取 Catalog；首次运行将生成元数据快照与 SourceRevision。" : testStatus === "error" ? "无法访问主机，请检查地址、端口和网络策略。" : "测试网络、只读账号和 Catalog 读取能力。"}</small></span><button className="secondary-button" type="button" disabled={!canTest || testStatus === "testing"} onClick={handleTest}>{testStatus === "testing" ? <LoaderCircle className="spin" size={15} /> : <Activity size={15} />}{testStatus === "success" ? "重新测试" : "测试连接"}</button></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={testStatus !== "success"} onClick={() => onSave({ name: name.trim(), kind, host: host.trim(), port, database: database.trim(), username: username.trim(), sslEnabled })}><Database size={16} />{initialConnection ? "保存修改" : "保存并建立基线"}</button></div></footer>
      </section>
    </div>
  );
}

function FileImportDialog({ existingDatasets = [], initialTargetDataset, onClose, onImport }: { existingDatasets?: Array<{ name: string; kind: FileDataset["kind"] }>; initialTargetDataset?: string; onClose: () => void; onImport: (source: SessionSource) => void }) {
  const [file, setFile] = useState<File | null>(null);
  const [datasetName, setDatasetName] = useState("");
  const [importMode, setImportMode] = useState(initialTargetDataset ? "snapshot" : "create");
  const [targetDataset, setTargetDataset] = useState(initialTargetDataset ?? existingDatasets[0]?.name ?? "");
  const [encoding, setEncoding] = useState("UTF-8");
  const [delimiter, setDelimiter] = useState(",");
  const [sheet, setSheet] = useState("全部工作表");
  const [firstRowHeader, setFirstRowHeader] = useState(true);
  const [chunking, setChunking] = useState<"按标题层级" | "按固定长度">("按标题层级");
  const [includeCodeBlocks, setIncludeCodeBlocks] = useState(true);
  const [checkStatus, setCheckStatus] = useState<"idle" | "checking" | "success" | "error">("idle");
  const closeRef = useRef<HTMLButtonElement>(null);
  const onCloseRef = useRef(onClose);
  const checkTimerRef = useRef<number | undefined>(undefined);
  const fileKind = file ? getFileKind(file.name) : null;
  const isXlsx = fileKind === "XLSX";
  const isMarkdown = fileKind === "MD";
  const compatibleDatasets = initialTargetDataset
    ? existingDatasets.filter((dataset) => dataset.name === initialTargetDataset)
    : fileKind ? existingDatasets.filter((dataset) => dataset.kind === fileKind) : existingDatasets;
  const effectiveDatasetName = importMode === "snapshot" ? targetDataset : datasetName.trim();
  const target = existingDatasets.find((dataset) => dataset.name === targetDataset);
  const targetKindMismatch = Boolean(fileKind && importMode === "snapshot" && target && target.kind !== fileKind);
  const checkError = !fileKind ? "仅支持 CSV、XLSX 和 Markdown 文件。" : targetKindMismatch ? `文件类型需要与 ${targetDataset} 的 ${target?.kind} 格式一致。` : file && file.size > 50 * 1024 * 1024 ? "文件超过 50 MB，请拆分后重新导入。" : "";

  useEffect(() => { onCloseRef.current = onClose; }, [onClose]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onCloseRef.current(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => { document.removeEventListener("keydown", handleKeyDown); if (checkTimerRef.current) window.clearTimeout(checkTimerRef.current); };
  }, []);

  const invalidateCheck = () => { if (checkStatus !== "idle") setCheckStatus("idle"); };
  const handleCheck = () => {
    if (!file) return;
    setCheckStatus("checking");
    checkTimerRef.current = window.setTimeout(() => setCheckStatus(checkError ? "error" : "success"), 300);
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog source-dialog" role="dialog" aria-modal="true" aria-labelledby="file-import-dialog-title">
        <header><div><h2 id="file-import-dialog-title">导入文件</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭文件导入" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body source-setup-form">
          <div className="source-form-grid source-form-grid-flat">
            <label className="file-picker field-span-2"><span>本地文件 <b>*</b></span><input type="file" aria-label="选择文件" accept=".csv,.xlsx,.md,.markdown,text/csv,text/markdown,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" onChange={(event) => { const nextFile = event.target.files?.[0] ?? null; const nextKind = nextFile ? getFileKind(nextFile.name) : null; setFile(nextFile); if (nextFile && !initialTargetDataset) setDatasetName(nextFile.name.replace(/\.(csv|xlsx|md|markdown)$/i, "")); if (importMode === "snapshot" && !initialTargetDataset) { const compatibleTargets = nextKind ? existingDatasets.filter((dataset) => dataset.kind === nextKind) : existingDatasets; setTargetDataset((current) => compatibleTargets.some((dataset) => dataset.name === current) ? current : compatibleTargets[0]?.name ?? ""); } invalidateCheck(); }} /><small>{file ? `${file.name} · ${Math.max(1, Math.ceil(file.size / 1024))} KB` : "CSV / XLSX / Markdown · 最大 50 MB"}</small></label>
            <label><span>导入方式</span><select aria-label="导入方式" value={importMode} onChange={(event) => { const nextMode = event.target.value; setImportMode(nextMode); if (nextMode === "snapshot") setTargetDataset((current) => compatibleDatasets.some((dataset) => dataset.name === current) ? current : compatibleDatasets[0]?.name ?? ""); invalidateCheck(); }}><option value="create">创建新文件来源</option><option value="snapshot" disabled={compatibleDatasets.length === 0}>更新已有文件来源</option></select></label>
            {importMode === "snapshot" ? <label><span>目标来源 <b>*</b></span><select aria-label="目标来源" value={targetDataset} onChange={(event) => { setTargetDataset(event.target.value); invalidateCheck(); }}>{compatibleDatasets.map((dataset) => <option key={dataset.name} value={dataset.name}>{dataset.name} · {dataset.kind}</option>)}</select></label> : <label><span>来源名称 <b>*</b></span><input aria-label="来源名称" value={datasetName} onChange={(event) => { setDatasetName(event.target.value); invalidateCheck(); }} placeholder={isMarkdown ? "例如 指标口径手册" : "例如 2026 年经营目标"} /></label>}
            {fileKind && (isMarkdown ? <><label><span>分块方式</span><select aria-label="分块方式" value={chunking} onChange={(event) => { setChunking(event.target.value as "按标题层级" | "按固定长度"); invalidateCheck(); }}><option>按标题层级</option><option>按固定长度</option></select></label><label className="switch-field"><span>内容解析</span><span><input type="checkbox" aria-label="保留代码块" checked={includeCodeBlocks} onChange={(event) => { setIncludeCodeBlocks(event.target.checked); invalidateCheck(); }} />保留代码块</span></label></> : <>{isXlsx ? <label><span>工作表范围</span><select aria-label="工作表范围" value={sheet} onChange={(event) => { setSheet(event.target.value); invalidateCheck(); }}><option>全部工作表</option><option>仅第一个工作表</option></select></label> : <label><span>文件编码</span><select aria-label="文件编码" value={encoding} onChange={(event) => { setEncoding(event.target.value); invalidateCheck(); }}><option>UTF-8</option><option>GB18030</option></select></label>}{!isXlsx && <label><span>字段分隔符</span><select aria-label="字段分隔符" value={delimiter} onChange={(event) => { setDelimiter(event.target.value); invalidateCheck(); }}><option value=",">逗号 (,)</option><option value="\t">制表符 (Tab)</option><option value=";">分号 (;)</option></select></label>}<label className="switch-field"><span>表头设置</span><span><input type="checkbox" aria-label="首行为字段名" checked={firstRowHeader} onChange={(event) => { setFirstRowHeader(event.target.checked); invalidateCheck(); }} />首行为字段名</span></label></>)}
          </div>
          <div className={`source-test-panel source-test-${checkStatus}`} role="status" aria-live="polite"><span className="source-test-icon">{checkStatus === "checking" ? <LoaderCircle className="spin" size={18} /> : checkStatus === "success" ? <CheckCircle2 size={18} /> : checkStatus === "error" ? <CircleAlert size={18} /> : <FileSpreadsheet size={18} />}</span><span><strong>{checkStatus === "checking" ? "正在检查文件" : checkStatus === "success" ? "文件检查通过" : checkStatus === "error" ? "文件检查失败" : "导入前需要检查文件"}</strong><small>{checkStatus === "success" ? `${importMode === "snapshot" ? `将更新 ${targetDataset} · ` : ""}${isMarkdown ? `${chunking} · ${includeCodeBlocks ? "保留代码块" : "忽略代码块"}` : isXlsx ? sheet : `${encoding} · ${delimiter === "," ? "逗号分隔" : delimiter === "\t" ? "制表符分隔" : "分号分隔"}`}。` : checkStatus === "error" ? checkError : isMarkdown ? "检查 Markdown 标题、链接和代码块结构。" : "检查格式、编码、表头和字段类型。"}</small></span><button className="secondary-button" type="button" disabled={!file || !effectiveDatasetName || checkStatus === "checking"} onClick={handleCheck}>{checkStatus === "checking" ? <LoaderCircle className="spin" size={15} /> : <FileSearch size={15} />}{checkStatus === "success" ? "重新检查" : "检查文件"}</button></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={!file || !fileKind || !effectiveDatasetName || checkStatus !== "success"} onClick={() => file && fileKind && onImport({ category: "file", name: importMode === "snapshot" ? targetDataset : datasetName.trim(), kind: fileKind, endpoint: `${file.name} · ${Math.max(1, Math.ceil(file.size / 1024))} KB`, targetDataset: importMode === "snapshot" ? targetDataset : undefined })}><FileUp size={16} />{importMode === "snapshot" ? "导入新版本" : "导入并开始构建"}</button></div></footer>
      </section>
    </div>
  );
}

function FileDatasetDialog({ dataset, onClose, onSave }: { dataset: FileDataset; onClose: () => void; onSave: (dataset: FileDataset) => void }) {
  const [name, setName] = useState(dataset.name);
  const [encoding, setEncoding] = useState(dataset.encoding);
  const [delimiter, setDelimiter] = useState(dataset.delimiter);
  const [sheet, setSheet] = useState(dataset.sheet);
  const [firstRowHeader, setFirstRowHeader] = useState(dataset.firstRowHeader);
  const [chunking, setChunking] = useState(dataset.chunking);
  const [includeCodeBlocks, setIncludeCodeBlocks] = useState(dataset.includeCodeBlocks);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog source-dialog file-dataset-dialog" role="dialog" aria-modal="true" aria-labelledby="file-dataset-dialog-title">
        <header><div><h2 id="file-dataset-dialog-title">编辑文件来源</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭文件来源编辑" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body source-setup-form">
          <div className="source-form-grid source-form-grid-flat">
            <label><span>来源名称 <b>*</b></span><input aria-label="来源名称" value={name} onChange={(event) => setName(event.target.value)} /></label>
            <label><span>文件类型</span><input aria-label="文件类型" value={dataset.kind} disabled /></label>
            <label className="field-span-2"><span>源文件</span><input aria-label="源文件" value={dataset.endpoint} disabled /></label>
            {dataset.kind === "MD" ? <><label><span>分块方式</span><select aria-label="分块方式" value={chunking} onChange={(event) => setChunking(event.target.value as "按标题层级" | "按固定长度")}><option>按标题层级</option><option>按固定长度</option></select></label><label className="switch-field"><span>内容解析</span><span><input type="checkbox" aria-label="保留代码块" checked={includeCodeBlocks} onChange={(event) => setIncludeCodeBlocks(event.target.checked)} />保留代码块</span></label></> : <>{dataset.kind === "XLSX" ? <label><span>工作表范围</span><select aria-label="工作表范围" value={sheet} onChange={(event) => setSheet(event.target.value)}><option>全部工作表</option><option>仅第一个工作表</option></select></label> : <><label><span>文件编码</span><select aria-label="文件编码" value={encoding} onChange={(event) => setEncoding(event.target.value)}><option>UTF-8</option><option>GB18030</option></select></label><label><span>字段分隔符</span><select aria-label="字段分隔符" value={delimiter} onChange={(event) => setDelimiter(event.target.value)}><option value=",">逗号 (,)</option><option value="\t">制表符 (Tab)</option><option value=";">分号 (;)</option></select></label></>}<label className="switch-field"><span>表头设置</span><span><input type="checkbox" aria-label="首行为字段名" checked={firstRowHeader} onChange={(event) => setFirstRowHeader(event.target.checked)} />首行为字段名</span></label></>}
          </div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={!name.trim()} onClick={() => onSave({ ...dataset, name: name.trim(), encoding, delimiter, sheet, firstRowHeader, chunking, includeCodeBlocks })}><FileSpreadsheet size={16} />保存修改</button></div></footer>
      </section>
    </div>
  );
}

function BuildTaskDialog({ initialTask, availableSources, onClose, onSave }: { initialTask?: BuildTask; availableSources: Array<{ name: string; kind: string; category: "database" | "file" }>; onClose: () => void; onSave: (task: BuildTaskDraft) => void }) {
  const [name, setName] = useState(initialTask?.name ?? "");
  const [selectedSources, setSelectedSources] = useState<string[]>(initialTask?.sources ?? []);
  const [sourcePickerOpen, setSourcePickerOpen] = useState(false);
  const [sourceQuery, setSourceQuery] = useState("");
  const [sourceCategory, setSourceCategory] = useState<"all" | "database" | "file">("all");
  const [mode, setMode] = useState<BuildTask["mode"]>(initialTask?.mode ?? "元数据增量");
  const [trigger, setTrigger] = useState<BuildTask["trigger"]>(initialTask?.trigger ?? "手动运行");
  const [cronExpression, setCronExpression] = useState(initialTask?.cronExpression ?? "0 2 * * *");
  const [scheduleTemplate, setScheduleTemplate] = useState(() => cronPresets.some((preset) => preset.expression === initialTask?.cronExpression) ? initialTask?.cronExpression ?? "0 2 * * *" : "custom");
  const [timezone, setTimezone] = useState<BuildTask["timezone"]>(initialTask?.timezone ?? "Asia/Shanghai");
  const [enabled, setEnabled] = useState(initialTask?.enabled ?? true);
  const nameRef = useRef<HTMLInputElement>(null);
  const sourceSearchRef = useRef<HTMLInputElement>(null);
  const sourceSelectorRef = useRef<HTMLDivElement>(null);
  const cronIsValid = isValidCronExpression(cronExpression);
  const canSave = Boolean(name.trim() && selectedSources.length > 0 && (trigger === "手动运行" || cronIsValid));
  const normalizedSourceQuery = sourceQuery.trim().toLocaleLowerCase("zh-CN");
  const visibleSources = availableSources.filter((source) => (sourceCategory === "all" || source.category === sourceCategory) && (!normalizedSourceQuery || `${source.name} ${source.kind}`.toLocaleLowerCase("zh-CN").includes(normalizedSourceQuery)));
  const selectedSourceSummary = selectedSources.length === 0
    ? "尚未选择数据来源"
    : `${selectedSources.slice(0, 2).join("、")}${selectedSources.length > 2 ? ` 等 ${selectedSources.length} 个来源` : ""}`;
  const readiness = !name.trim()
    ? { tone: "pending", title: "请填写任务名称", detail: "名称将显示在构建运行和审计记录中。" }
    : selectedSources.length === 0
      ? { tone: "pending", title: "请选择数据来源", detail: "至少固定一个来源，任务才可以保存。" }
      : trigger === "定时调度" && !cronIsValid
        ? { tone: "pending", title: "请检查 Cron 表达式", detail: "定时调度使用标准 5 段 Cron 格式。" }
        : { tone: "ready", title: "配置完整", detail: `${selectedSources.length} 个来源 · ${mode} · ${trigger === "定时调度" ? describeCronSchedule(cronExpression) : "仅手动运行"}` };

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      if (sourcePickerOpen) setSourcePickerOpen(false);
      else onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose, sourcePickerOpen]);

  useEffect(() => { nameRef.current?.focus(); }, []);
  useEffect(() => { if (sourcePickerOpen) sourceSearchRef.current?.focus(); }, [sourcePickerOpen]);
  useEffect(() => {
    if (!sourcePickerOpen) return;
    const handlePointerDown = (event: PointerEvent) => {
      if (!sourceSelectorRef.current?.contains(event.target as Node)) setSourcePickerOpen(false);
    };
    document.addEventListener("pointerdown", handlePointerDown);
    return () => document.removeEventListener("pointerdown", handlePointerDown);
  }, [sourcePickerOpen]);

  const toggleSource = (sourceName: string) => {
    setSelectedSources((current) => current.includes(sourceName) ? current.filter((name) => name !== sourceName) : [...current, sourceName]);
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog source-dialog build-task-dialog" role="dialog" aria-modal="true" aria-labelledby="build-task-dialog-title">
        <header className="build-task-dialog-header">
          <div className="build-task-dialog-heading"><span className="build-task-dialog-mark">{initialTask ? <Settings2 size={18} /> : <Plus size={18} />}</span><h2 id="build-task-dialog-title">{initialTask ? "编辑构建任务" : "新建构建任务"}</h2></div>
          <div className="build-task-dialog-header-actions"><label className="build-task-header-toggle"><span>{enabled ? "已启用" : "已停用"}</span><input type="checkbox" aria-label="启用任务" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} /><i aria-hidden="true" /></label><button className="icon-button" type="button" aria-label="关闭构建任务" onClick={onClose}><X size={18} /></button></div>
        </header>
        <div className="dialog-body build-task-form">
          <div className="build-task-section">
            <label className="build-task-name"><span>任务名称 <b>*</b></span><input ref={nameRef} type="text" aria-label="任务名称" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如 经营数据元数据同步" /></label>
          </div>

          <div className="build-task-section">
            <div ref={sourceSelectorRef} className={`build-source-selector${sourcePickerOpen ? " build-source-selector-open" : ""}`}>
              <span className="build-source-selector-label"><span>数据来源 <b>*</b></span><em>{selectedSources.length > 0 ? `${selectedSources.length} 个已选` : "未选择"}</em></span>
              <button className="build-source-selector-trigger" type="button" aria-haspopup="dialog" aria-expanded={sourcePickerOpen} aria-controls="build-source-picker" onClick={() => { if (sourcePickerOpen) setSourcePickerOpen(false); else { setSourceQuery(""); setSourceCategory("all"); setSourcePickerOpen(true); } }}>
                <span className="build-source-selector-icon"><Database size={17} /></span>
                <span><strong>{selectedSources.length > 0 ? `已选择 ${selectedSources.length} 个数据来源` : "选择数据来源"}</strong><small>{selectedSourceSummary}</small></span>
                <ChevronDown size={17} aria-hidden="true" />
              </button>
              {sourcePickerOpen && <div id="build-source-picker" className="build-source-picker" role="dialog" aria-label="数据来源选择器">
                <div className="build-source-picker-toolbar">
                  <label className="build-source-search"><Search size={15} aria-hidden="true" /><input ref={sourceSearchRef} type="search" aria-label="搜索可用数据来源" value={sourceQuery} onChange={(event) => setSourceQuery(event.target.value)} placeholder="搜索来源名称或类型" /></label>
                  <div className="build-source-category-filter" role="group" aria-label="筛选数据来源类型">{([['all', '全部'], ['database', '数据库'], ['file', '文件']] as const).map(([value, label]) => <button key={value} type="button" aria-pressed={sourceCategory === value} onClick={() => setSourceCategory(value)}>{label}</button>)}</div>
                </div>
                <div className="build-source-picker-list" role="group" aria-label="可用数据来源">
                  {visibleSources.map((source) => <label key={source.name}><input type="checkbox" aria-label={`选择数据来源 ${source.name}`} checked={selectedSources.includes(source.name)} onChange={() => toggleSource(source.name)} /><span className={source.category === "file" ? "source-mark source-mark-file" : "source-mark"}>{source.category === "file" ? <FileSpreadsheet size={15} /> : <Database size={15} />}</span><span><strong>{source.name}</strong><small>{source.kind} · {source.category === "file" ? "版本化快照" : "只读连接"}</small></span><CheckCircle2 size={16} aria-hidden="true" /></label>)}
                  {visibleSources.length === 0 && <div className="build-source-picker-empty" role="status"><Search size={16} /><span><strong>没有匹配的来源</strong><small>调整搜索词或来源类型。</small></span></div>}
                </div>
                <footer><span>已选择 {selectedSources.length} 个来源</span><div><button type="button" disabled={selectedSources.length === 0} onClick={() => setSelectedSources([])}>清空</button><button type="button" onClick={() => setSourcePickerOpen(false)}>完成选择</button></div></footer>
              </div>}
            </div>
          </div>

          <div className="build-task-section">
            <div className="build-mode-label"><span>分析方式</span><span className="build-mode-help"><button className="icon-button" type="button" aria-label="查看元数据增量说明" aria-describedby="build-mode-help-tooltip"><CircleHelp size={15} /></button><span id="build-mode-help-tooltip" className="build-mode-tooltip" role="tooltip"><strong>元数据快照，不是业务数据快照</strong><small>首次运行生成完整元数据快照与 SourceRevision；后续分析 Catalog、DDL、View 与约束差异。失败运行不会推进 checkpoint。</small></span></span></div>
            <fieldset className="build-mode-field"><legend className="visually-hidden">分析方式</legend><div><label><input type="radio" name="build-mode" value="元数据增量" checked={mode === "元数据增量"} onChange={() => setMode("元数据增量")} /><span><strong>元数据增量</strong><small>比较平台生成的 SourceRevision，仅处理差异对象</small></span></label><label><input type="radio" name="build-mode" value="全量校准" checked={mode === "全量校准"} onChange={() => setMode("全量校准")} /><span><strong>全量校准</strong><small>周期性重扫全部元数据，修正可能的来源漂移</small></span></label></div></fieldset>
            <fieldset className="build-trigger-field"><legend>触发方式</legend><div className="build-trigger-control"><label><input type="radio" name="build-trigger" value="手动运行" checked={trigger === "手动运行"} onChange={() => setTrigger("手动运行")} /><span><Radio size={15} />手动运行</span></label><label><input type="radio" name="build-trigger" value="定时调度" checked={trigger === "定时调度"} onChange={() => setTrigger("定时调度")} /><span><Clock3 size={15} />定时调度</span></label></div></fieldset>
            {trigger === "定时调度" && <div className="build-schedule-config">
              <label><span>计划模板</span><select aria-label="计划模板" value={scheduleTemplate} onChange={(event) => { const expression = event.target.value; setScheduleTemplate(expression); if (expression !== "custom") setCronExpression(expression); }}>{cronPresets.map((preset) => <option key={preset.expression} value={preset.expression}>{preset.label}</option>)}<option value="custom">自定义 Cron</option></select></label>
              <label className="build-cron-expression"><span>Cron 表达式 <em>5 段</em></span><input type="text" aria-label="Cron 表达式" aria-invalid={!cronIsValid} value={cronExpression} onChange={(event) => { setCronExpression(event.target.value); setScheduleTemplate("custom"); }} spellCheck={false} /></label>
              <label><span>时区</span><select aria-label="调度时区" value={timezone} onChange={(event) => setTimezone(event.target.value as BuildTask["timezone"])}><option>Asia/Shanghai</option><option>UTC</option><option>Asia/Tokyo</option><option>America/Los_Angeles</option><option>Europe/London</option></select></label>
            </div>}
          </div>
        </div>
        <footer className="build-task-dialog-footer"><div className={`build-task-save-state build-task-save-state-${readiness.tone}`} role="status">{canSave ? <CheckCircle2 size={17} /> : <CircleAlert size={17} />}<span><strong>{readiness.title}</strong><small>{readiness.detail}</small></span></div><div className="build-task-dialog-actions"><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={!canSave} onClick={() => onSave({ name: name.trim(), sources: selectedSources, mode, trigger, cronExpression: cronExpression.trim(), timezone, enabled })}>{initialTask ? <Settings2 size={16} /> : <Plus size={16} />}{initialTask ? "保存修改" : "创建任务"}</button></div></footer>
      </section>
    </div>
  );
}

function DeleteSourceDialog({ title, name, description, onClose, onConfirm }: { title: string; name: string; description: string; onClose: () => void; onConfirm: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog delete-connection-dialog" role="alertdialog" aria-modal="true" aria-labelledby="delete-source-title" aria-describedby="delete-source-description">
        <header><div><h2 id="delete-source-title">{title}</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭删除确认" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body delete-confirmation"><span className="delete-confirmation-icon"><Trash2 size={20} /></span><div><strong>{name}</strong><p id="delete-source-description">{description}</p></div></div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="danger-button" type="button" onClick={onConfirm}><Trash2 size={16} />确认删除</button></div></footer>
      </section>
    </div>
  );
}

export function SourcesView({ discoveryState, focusIndex, navigationEpoch, runDetailBackRequestEpoch, initialRunId, onRunDetailOpenChange, onOpenActivity, onOpenGlobalRun, onRunDiscovery, onOpenGovernanceProposals, onNotify }: SourcesViewProps) {
  const scanCompleted = discoveryState === "complete";
  const scanRunning = discoveryState === "running";
  const [creatingSource, setCreatingSource] = useState(false);
  const [importingFile, setImportingFile] = useState(false);
  const [reimportTarget, setReimportTarget] = useState<FileDataset | null>(null);
  const [editingDatabase, setEditingDatabase] = useState<DatabaseConnection | null>(null);
  const [deletingDatabase, setDeletingDatabase] = useState<DatabaseConnection | null>(null);
  const [editingFile, setEditingFile] = useState<FileDataset | null>(null);
  const [deletingFile, setDeletingFile] = useState<FileDataset | null>(null);
  const [creatingTask, setCreatingTask] = useState(false);
  const [editingTask, setEditingTask] = useState<BuildTask | null>(null);
  const [deletingTask, setDeletingTask] = useState<BuildTask | null>(null);
  const [runningTaskId, setRunningTaskId] = useState<string | null>(null);
  const [selectedRunId, setSelectedRunId] = useState<string | null>(initialRunId ?? null);
  const [selectedRunEpoch, setSelectedRunEpoch] = useState(initialRunId ? navigationEpoch : -1);
  const [selectedRunBackEpoch, setSelectedRunBackEpoch] = useState(runDetailBackRequestEpoch);
  const [viewingRunLog, setViewingRunLog] = useState(false);
  const [activeRunResultTab, setActiveRunResultTab] = useState<BuildRunResultTab>("evidence");
  const [activeRunResultItemId, setActiveRunResultItemId] = useState("evidence-catalog");
  const [runFilter, setRunFilter] = useState<"all" | "running" | "completed" | "warning">("all");
  const [runQuery, setRunQuery] = useState("");
  const [taskQuery, setTaskQuery] = useState("");
  const [taskFilter, setTaskFilter] = useState<"all" | "running" | "warning" | "disabled">("all");
  const [activeSourceTab, setActiveSourceTab] = useState<"database" | "file">("database");
  const [databaseConnections, setDatabaseConnections] = useState<DatabaseConnection[]>([
    { id: "postgres-analytics", name: "PostgreSQL Analytics", kind: "PostgreSQL", host: "analytics.internal", port: "5432", database: "analytics", username: "analytics_ro", sslEnabled: true, detail: "36 个 schema · 只读元数据", status: "健康" },
  ]);
  const [fileDatasets, setFileDatasets] = useState<FileDataset[]>([
    { id: "fy2026-target", name: "FY2026 营收目标", kind: "XLSX", endpoint: "revenue_target_2026.xlsx", detail: "3 个工作表 · 1,248 行", status: "已就绪", encoding: "UTF-8", delimiter: ",", sheet: "全部工作表", firstRowHeader: true, chunking: "按标题层级", includeCodeBlocks: true },
  ]);
  const [buildTasks, setBuildTasks] = useState<BuildTask[]>([
    { id: "business-knowledge", name: "经营数据元数据同步", sources: ["PostgreSQL Analytics", "FY2026 营收目标"], mode: "元数据增量", checkpoint: "src-ecommerce@9f2e8a", trigger: "定时调度", cronExpression: "0 2 * * *", timezone: "Asia/Shanghai", enabled: true, status: "就绪", lastRun: "今天 14:32" },
    { id: "customer-growth", name: "客户增长源增量扫描", sources: ["PostgreSQL Analytics"], mode: "元数据增量", checkpoint: "src-customer@71ac03", trigger: "手动运行", cronExpression: "0 2 * * *", timezone: "Asia/Shanghai", enabled: true, status: "有警告", lastRun: "昨天 18:44" },
    { id: "finance-baseline", name: "财务目标文件校准", sources: ["FY2026 营收目标"], mode: "全量校准", checkpoint: "src-finance@c20df1", trigger: "定时调度", cronExpression: "0 4 1 * *", timezone: "Asia/Shanghai", enabled: false, status: "停用", lastRun: "8 月 18 日" },
  ]);
  const [historicalRuns] = useState<BuildRun[]>([
    { id: "RUN-240824-1432", taskName: "经营数据元数据同步", sourceName: "PostgreSQL Analytics", previousSourceRevision: "src-ecommerce@2bc018", sourceRevision: "src-ecommerce@9f2e8a", checkpoint: "schema:9f2e8a", startedAt: "2026-08-24 14:32:18", duration: "2 分 18 秒", status: "已完成", trigger: "定时调度", mode: "数据库同步", kind: "ingestion", sourceCount: 1, physicalChanges: 12, addedObjects: 3, modifiedObjects: 8, removedObjects: 1, unchangedObjects: 2834, knowledgeBlocks: 0, proposals: 0, outputSummary: "2,846 对象 · 12 项变化", downstreamRunId: "RUN-240901-1208" },
    { id: "RUN-240823-1844", taskName: "客户增长源增量扫描", sourceName: "PostgreSQL Analytics", previousSourceRevision: "src-customer@647d12", sourceRevision: "src-customer@71ac03", checkpoint: "schema:71ac03", startedAt: "2026-08-23 18:44:07", duration: "1 分 42 秒", status: "有警告", trigger: "手动运行", mode: "元数据扫描", kind: "ingestion", sourceCount: 1, physicalChanges: 4, addedObjects: 1, modifiedObjects: 2, removedObjects: 1, unchangedObjects: 1462, knowledgeBlocks: 0, proposals: 0, outputSummary: "1,466 对象 · 1 项解析异常" },
    { id: "RUN-240818-0915", taskName: "财务目标文件校准", sourceName: "FY2026 营收目标", previousSourceRevision: "file:a184e0", sourceRevision: "file:c20df1", checkpoint: "file:c20df1", startedAt: "2026-08-18 09:15:42", duration: "46 秒", status: "已完成", trigger: "定时调度", mode: "文件导入", kind: "ingestion", sourceCount: 1, physicalChanges: 3, addedObjects: 1, modifiedObjects: 2, removedObjects: 0, unchangedObjects: 1245, knowledgeBlocks: 0, proposals: 0, outputSummary: "1,248 行 · 3 个工作表", downstreamRunId: "RUN-240901-1145" },
    { id: "RUN-240817-1431", taskName: "经营数据元数据同步", sourceName: "PostgreSQL Analytics", previousSourceRevision: "src-ecommerce@f08ae3", sourceRevision: "src-ecommerce@2bc018", checkpoint: "schema:2bc018", startedAt: "2026-08-17 14:31:05", duration: "2 分 05 秒", status: "已完成", trigger: "定时调度", mode: "数据库同步", kind: "ingestion", sourceCount: 1, physicalChanges: 3, knowledgeBlocks: 0, proposals: 0, outputSummary: "2,834 对象 · 3 项变化" },
    { id: "RUN-240816-1908", taskName: "客户增长源增量扫描", sourceName: "PostgreSQL Analytics", previousSourceRevision: "src-customer@51b9c0", sourceRevision: "src-customer@647d12", checkpoint: "schema:647d12", startedAt: "2026-08-16 19:08:36", duration: "1 分 37 秒", status: "已完成", trigger: "手动运行", mode: "元数据扫描", kind: "ingestion", sourceCount: 1, physicalChanges: 0, knowledgeBlocks: 0, proposals: 0, outputSummary: "1,462 对象 · 无变化" },
  ]);
  const [sourceQuery, setSourceQuery] = useState("");

  useEffect(() => {
    if (!selectedRunId) return;
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") setSelectedRunId(null); };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [selectedRunId]);

  const sources = [
    ...databaseConnections.map((connection) => ({ id: connection.id, category: "database" as const, name: connection.name, detail: connection.detail, kind: connection.kind, endpoint: connection.username, icon: <Server size={18} />, status: connection.status, databaseConnection: connection, fileDataset: null })),
    ...fileDatasets.map((dataset) => ({ id: dataset.id, category: "file" as const, name: dataset.name, detail: dataset.detail, kind: dataset.kind, endpoint: dataset.endpoint, icon: <FileSpreadsheet size={18} />, status: dataset.status, databaseConnection: null, fileDataset: dataset })),
  ];
  const normalizedSourceQuery = sourceQuery.trim().toLocaleLowerCase("zh-CN");
  const databaseCount = sources.filter((source) => source.category === "database").length;
  const fileCount = sources.filter((source) => source.category === "file").length;
  const existingFileDatasets = fileDatasets.map((dataset) => ({ name: dataset.name, kind: dataset.kind }));
  const visibleSources = sources.filter((source) => source.category === activeSourceTab && (!normalizedSourceQuery || `${source.name} ${source.kind} ${source.detail} ${source.endpoint}`.toLocaleLowerCase("zh-CN").includes(normalizedSourceQuery)));
  const availableTaskSources = sources.map((source) => ({ name: source.name, kind: source.kind, category: source.category }));
  const getTaskStatus = (task: BuildTask) => task.id === runningTaskId ? (scanRunning ? "运行中" : scanCompleted ? "已完成" : task.status) : task.status;
  const normalizedTaskQuery = taskQuery.trim().toLocaleLowerCase("zh-CN");
  const visibleTasks = buildTasks.filter((task) => {
    const status = getTaskStatus(task);
    const matchesFilter = taskFilter === "all" || (taskFilter === "running" && status === "运行中") || (taskFilter === "warning" && status === "有警告") || (taskFilter === "disabled" && status === "停用");
    return matchesFilter && (!normalizedTaskQuery || `${task.name} ${task.sources.join(" ")} ${task.mode}`.toLocaleLowerCase("zh-CN").includes(normalizedTaskQuery));
  });
  const buildRuns = historicalRuns;
  const normalizedRunQuery = runQuery.trim().toLocaleLowerCase("zh-CN");
  const visibleBuildRuns = buildRuns.filter((run) => {
    const matchesFilter = runFilter === "all" || (runFilter === "running" && run.status === "运行中") || (runFilter === "completed" && run.status === "已完成") || (runFilter === "warning" && run.status === "有警告");
    const matchesQuery = !normalizedRunQuery || `${run.id} ${run.taskName}`.toLocaleLowerCase("zh-CN").includes(normalizedRunQuery);
    return matchesFilter && matchesQuery;
  });
  const selectedRun = selectedRunEpoch === navigationEpoch && selectedRunBackEpoch === runDetailBackRequestEpoch ? buildRuns.find((run) => run.id === selectedRunId) ?? null : null;
  const runDetailOpen = Boolean(selectedRun);
  useEffect(() => {
    onRunDetailOpenChange(runDetailOpen);
    return () => onRunDetailOpenChange(false);
  }, [onRunDetailOpenChange, runDetailOpen]);
  const selectedRunDiff = selectedRun && selectedRun.kind !== "embedding-rebuild" ? buildRunDiff(selectedRun) : null;
  const runReferenceSuffix = selectedRun?.id.slice(-4) ?? "0000";
  const runResultGroups: Record<BuildRunResultTab, BuildRunResultItem[]> = selectedRun ? {
    changes: selectedRun.physicalChanges > 0 ? expandBuildRunResultItems(buildRunResultItems(
      { id: "change-orders-amount", title: "orders.amount", kind: "字段类型修改", summary: "从 numeric(12,2) 调整为 numeric(18,2)", state: selectedRun.status === "有警告" ? "待确认" : "已解析", warning: selectedRun.status === "有警告", facts: [["对象路径", "commerce.orders.amount"], ["影响范围", "客单价、净收入等 3 个语义资产"], ["变化来源", `CHANGESET-${runReferenceSuffix}-01`], ["处理建议", selectedRun.status === "有警告" ? "确认精度变化是否影响下游计算" : "沿用现有语义绑定"]] },
      { id: "change-customer-region", title: "customer.region_code", kind: "新增字段", summary: "Catalog 中检测到新的区域编码字段", state: "已解析", facts: [["对象路径", "commerce.customer.region_code"], ["影响范围", "业务区域 1 个候选资产"], ["变化来源", `CHANGESET-${runReferenceSuffix}-02`], ["处理建议", "已生成字段描述与候选映射"]] },
      { id: "change-orders-constraint", title: "orders_customer_id_fkey", kind: "约束修改", summary: "外键引用切换到 customer.customer_id", state: "已归档", facts: [["对象路径", "commerce.orders.orders_customer_id_fkey"], ["影响范围", "2 条 Join 关系"], ["变化来源", `CHANGESET-${runReferenceSuffix}-03`], ["处理建议", "重新验证关联完整性"]] },
    ), selectedRun.physicalChanges, ["orders.currency_code", "customer.segment", "payment.status", "shipment.delivered_at", "products.category_id", "orders.channel", "customer.lifecycle_stage", "invoice.tax_amount", "warehouse.region_id"], "change") : [],
    evidence: selectedRun.knowledgeBlocks > 0 ? expandBuildRunResultItems(buildRunResultItems(
      { id: "evidence-catalog", title: "Catalog 结构指纹", kind: "系统证据", summary: "记录本次 revision 的 schema、table、column 与 constraint", state: "已溯源", facts: [["证据类型", "Catalog snapshot"], ["来源", selectedRun.sourceRevision], ["关联范围", `${selectedRun.physicalChanges} 个变化对象`], ["记录编号", `EVIDENCE-${runReferenceSuffix}-01`]] },
      { id: "evidence-ddl", title: "DDL 差异片段", kind: "结构证据", summary: "保留本次字段和约束变化的前后定义", state: "已溯源", facts: [["证据类型", "DDL diff"], ["来源", selectedRun.previousSourceRevision ?? "首次基线"], ["关联范围", "字段、约束与 View"], ["记录编号", `EVIDENCE-${runReferenceSuffix}-02`]] },
      { id: "evidence-usage", title: "查询使用摘要", kind: "影响证据", summary: "定位最近使用变化对象的查询和语义资产", state: "已归档", facts: [["证据类型", "Usage summary"], ["来源", "最近 7 天只读统计"], ["关联范围", "3 个消费者"], ["记录编号", `EVIDENCE-${runReferenceSuffix}-03`]] },
    ), selectedRun.knowledgeBlocks, ["约束定义快照", "View SQL 差异", "字段注释变更", "索引定义摘要", "查询依赖引用", "BI 消费关系", "模型血缘片段", "字段质量画像", "Schema 权限摘要", "枚举值分布", "API 字段引用", "任务调度引用", "语义绑定快照", "历史版本对照", "命名规则校验"], "evidence") : [],
    proposals: selectedRun.proposals > 0 ? expandBuildRunResultItems(buildRunResultItems(
      { id: "proposal-aov", title: "客单价 @9", kind: "候选资产版本", summary: "退款订单口径与金额字段精度调整", state: "待审核", warning: selectedRun.status === "有警告", facts: [["候选版本", "客单价 @9"], ["涉及变化", "orders.amount 字段类型修改"], ["验证状态", selectedRun.status === "有警告" ? "1 项影响待确认" : "结构验证通过"], ["治理状态", "等待业务负责人审核"]] },
      { id: "proposal-revenue", title: "净收入 @13", kind: "候选资产版本", summary: "补充字段变化证据并刷新引用", state: "待审核", facts: [["候选版本", "净收入 @13"], ["涉及变化", "金额字段与退款口径"], ["验证状态", "证据完整"], ["治理状态", "等待财务负责人审核"]] },
      { id: "proposal-region", title: "业务区域 @4", kind: "候选资产版本", summary: "新增区域编码映射和兼容别名", state: "待决策", facts: [["候选版本", "业务区域 @4"], ["涉及变化", "customer.region_code 新增"], ["验证状态", "兼容性检查通过"], ["治理状态", "等待版本级决策"]] },
    ), selectedRun.proposals, ["退款金额 @7", "客户分群 @12", "履约时效 @5"], "proposal") : [],
  } : { changes: [], evidence: [], proposals: [] };
  const activeRunResultItem = runResultGroups[activeRunResultTab].find((item) => item.id === activeRunResultItemId) ?? runResultGroups[activeRunResultTab][0] ?? null;
  const selectRunGraphItem = (tab: BuildRunResultTab, itemId?: string) => {
    setActiveRunResultTab(tab);
    setActiveRunResultItemId(itemId ?? runResultGroups[tab][0]?.id ?? "");
  };
  const openBuildRun = (run: BuildRun) => {
    const defaultTab: BuildRunResultTab = run.knowledgeBlocks > 0 ? "evidence" : run.physicalChanges > 0 ? "changes" : "proposals";
    const defaultItemId = defaultTab === "evidence" ? "evidence-catalog" : defaultTab === "changes" ? "change-orders-amount" : "proposal-aov";
    setSelectedRunId(run.id);
    setSelectedRunEpoch(navigationEpoch);
    setSelectedRunBackEpoch(runDetailBackRequestEpoch);
    setActiveRunResultTab(defaultTab);
    setActiveRunResultItemId(defaultItemId);
  };
  const runBuildTask = (task: BuildTask) => {
    if (!task.enabled || scanRunning) return;
    if (runningTaskId && scanCompleted) setBuildTasks((current) => current.map((item) => item.id === runningTaskId ? { ...item, status: "已完成", lastRun: "刚刚" } : item));
    setRunningTaskId(task.id);
    onRunDiscovery();
  };

  const sourceRegistry = (
    <section className="source-table" role="tabpanel" aria-labelledby={`${activeSourceTab}-source-tab`}>
      <div className="source-table-head" aria-hidden="true"><span>名称</span><span>类型</span><span>来源</span><span>状态与增量能力</span><span>操作</span></div>
      <div className="source-table-body">
        {visibleSources.map((source) => <div className="source-table-row" key={source.id}><span className="source-primary"><span className={source.category === "file" ? "source-mark source-mark-file" : "source-mark"}>{source.icon}</span><span><strong>{source.name}</strong><small>{source.detail}</small></span></span><span className="source-cell" data-label="类型">{source.kind}</span><code className="source-cell" data-label="来源">{source.endpoint}</code><span className="source-status"><span className="health-label"><i />{source.status}</span><small>{source.category === "database" ? "元数据快照差异 · CDC 未配置" : source.kind === "MD" ? "版本化文档" : "版本化快照"}</small></span><span className="source-row-actions">{source.fileDataset && <button className="icon-button" type="button" aria-label={`重新导入 ${source.name}`} title="重新导入" onClick={() => { setReimportTarget(source.fileDataset); setImportingFile(true); }}><RefreshCw size={15} /></button>}<button className="icon-button" type="button" aria-label={`编辑 ${source.name}`} title={source.category === "database" ? "编辑连接" : "编辑文件来源"} onClick={() => source.databaseConnection ? setEditingDatabase(source.databaseConnection) : source.fileDataset && setEditingFile(source.fileDataset)}><Pencil size={15} /></button><button className="icon-button source-delete-button" type="button" aria-label={`删除 ${source.name}`} title={source.category === "database" ? "删除连接" : "删除文件来源"} onClick={() => source.databaseConnection ? setDeletingDatabase(source.databaseConnection) : source.fileDataset && setDeletingFile(source.fileDataset)}><Trash2 size={15} /></button></span></div>)}
        {visibleSources.length === 0 && <div className="source-empty-state" role="status"><Search size={18} /><span><strong>没有匹配的来源</strong><small>可以尝试搜索名称、类型或文件名。</small></span></div>}
      </div>
    </section>
  );

  return (
    <section className="view view-sources">
      {focusIndex === 0 ? (
        <div className="source-commandbar">
          <div className="source-tabs" role="tablist" aria-label="数据来源类型">
            <button id="database-source-tab" type="button" role="tab" aria-selected={activeSourceTab === "database"} onClick={() => { setActiveSourceTab("database"); setSourceQuery(""); }}>数据库连接 <span>{databaseCount}</span></button>
            <button id="file-source-tab" type="button" role="tab" aria-selected={activeSourceTab === "file"} onClick={() => { setActiveSourceTab("file"); setSourceQuery(""); }}>文件导入 <span>{fileCount}</span></button>
          </div>
          <div className="source-command-actions"><label className="connection-search"><Search size={16} aria-hidden="true" /><input type="search" aria-label="搜索数据来源" value={sourceQuery} onChange={(event) => setSourceQuery(event.target.value)} placeholder={activeSourceTab === "database" ? "搜索数据库连接" : "搜索导入文件"} /></label>{activeSourceTab === "database" ? <button className="primary-button" type="button" onClick={() => setCreatingSource(true)}><Database size={16} />连接数据库</button> : <button className="primary-button" type="button" onClick={() => { setReimportTarget(null); setImportingFile(true); }}><FileUp size={16} />导入文件</button>}</div>
        </div>
      ) : null}

      {focusIndex === 0 && sourceRegistry}

      {focusIndex === 1 && <>
        <div className="build-task-commandbar">
          <div className="build-task-filters" role="group" aria-label="筛选构建任务">
            {([['all', '全部'], ['running', '运行中'], ['warning', '异常'], ['disabled', '停用']] as const).map(([value, label]) => <button type="button" key={value} aria-pressed={taskFilter === value} onClick={() => setTaskFilter(value)}>{label}{value === "all" && <span>{buildTasks.length}</span>}</button>)}
          </div>
          <div className="build-task-command-actions"><label className="connection-search"><Search size={16} aria-hidden="true" /><input type="search" aria-label="搜索构建任务" value={taskQuery} onChange={(event) => setTaskQuery(event.target.value)} placeholder="搜索任务或数据来源" /></label><button className="primary-button" type="button" onClick={() => setCreatingTask(true)}><Plus size={16} />新建构建任务</button></div>
        </div>

        <section className="build-task-table" aria-label="接入自动化列表">
          <div className="build-task-table-head" aria-hidden="true"><span>任务名称</span><span>数据来源</span><span>方式</span><span>最近运行</span><span>状态</span><span>操作</span></div>
          <div className="build-task-table-body">{visibleTasks.map((task) => { const status = getTaskStatus(task); return <div className="build-task-row" key={task.id}><span className="build-task-primary"><span className="build-task-mark"><Box size={17} /></span><span><strong>{task.name}</strong><small>{task.trigger === "定时调度" ? describeCronSchedule(task.cronExpression) : "手动运行"}</small></span></span><span className="build-task-source-cell"><strong>{task.sources.length} 个来源</strong><small>{task.sources.join("、")}</small></span><span className="build-task-mode"><strong>{task.mode}</strong></span><button className="build-task-last-run build-task-last-run-link" type="button" aria-label={`查看 ${task.name} 的接入运行`} onClick={() => { onOpenActivity(); setRunQuery(task.name); setSelectedRunId(null); }}>{task.id === runningTaskId && scanCompleted ? "刚刚" : task.lastRun}</button><span className={`build-task-status build-task-status-${status === "有警告" ? "warning" : status === "停用" ? "disabled" : status === "运行中" ? "running" : "ready"}`}><i />{status}</span><span className="source-row-actions"><button className="icon-button" type="button" aria-label={`运行 ${task.name}`} title="立即运行" disabled={!task.enabled || scanRunning} onClick={() => runBuildTask(task)}>{task.id === runningTaskId && scanRunning ? <LoaderCircle className="spin" size={15} /> : <Play size={15} />}</button><button className="icon-button" type="button" aria-label={`编辑 ${task.name}`} title="编辑任务" onClick={() => setEditingTask(task)}><Pencil size={15} /></button><button className="icon-button source-delete-button" type="button" aria-label={`删除 ${task.name}`} title="删除任务" disabled={task.id === runningTaskId && scanRunning} onClick={() => setDeletingTask(task)}><Trash2 size={15} /></button></span></div>; })}{visibleTasks.length === 0 && <div className="source-empty-state" role="status"><Search size={18} /><span><strong>没有匹配的接入自动化</strong><small>调整搜索词或状态筛选。</small></span></div>}</div>
        </section>
      </>}

      {focusIndex === 2 && !selectedRun && <>
        <div className="build-task-commandbar build-run-commandbar">
          <div className="build-task-filters" role="group" aria-label="筛选接入运行">
            {([['all', '全部', buildRuns.length], ['running', '运行中', buildRuns.filter((run) => run.status === "运行中").length], ['completed', '已完成', buildRuns.filter((run) => run.status === "已完成").length], ['warning', '有警告', buildRuns.filter((run) => run.status === "有警告").length]] as const).map(([value, label, count]) => <button type="button" key={value} aria-pressed={runFilter === value} onClick={() => { setRunFilter(value); setSelectedRunId(null); }}>{label}<span>{count}</span></button>)}
          </div>
          <div className="build-task-command-actions"><label className="connection-search"><Search size={16} aria-hidden="true" /><input type="search" aria-label="搜索接入运行" value={runQuery} onChange={(event) => setRunQuery(event.target.value)} placeholder="搜索 Run ID、任务或来源" /></label></div>
        </div>

        <section className="build-run-list-surface" aria-label="接入运行">
          <div className="build-run-list-head" aria-hidden="true"><span>Run ID</span><span>接入任务</span><span>开始时间</span><span>耗时</span><span>类型</span><span>接入结果</span><span>状态</span><span /></div>
          <div className="build-run-list-body">
            {visibleBuildRuns.map((run) => <button className="build-run-list-row" type="button" key={run.id} aria-label={`查看运行 ${run.id} ${run.taskName}`} onClick={() => openBuildRun(run)}>
              <span className="build-run-list-id"><code>{run.id}</code></span>
              <span className="build-run-list-task">{run.taskName}</span>
              <span className="build-run-list-time">{run.startedAt}</span>
              <span className="build-run-list-duration">{run.duration}</span>
              <span className="build-run-list-mode">{run.mode}</span>
              <span className="build-run-list-output"><strong>{run.outputSummary}</strong><small>{run.sourceName}</small></span>
              <span className={`build-task-status ${run.status === "有警告" ? "build-task-status-warning" : run.status === "运行中" ? "build-task-status-running" : ""}`}><i />{run.status}</span>
              <ArrowRight size={15} />
            </button>)}
            {visibleBuildRuns.length === 0 && <div className="source-empty-state" role="status"><Search size={18} /><span><strong>没有匹配的接入运行</strong><small>调整搜索词或状态筛选。</small></span></div>}
          </div>
        </section>

        <footer className="ingestion-run-boundary"><Activity size={14} /><span><strong>这里只记录数据进入系统的过程</strong><small>知识增量构建、索引重建和发布验证统一在“审计与运行”中查看。</small></span></footer>

      </>}

      {focusIndex === 2 && selectedRun && <>
        <section className="build-record-detail build-run-detail-page" aria-label="接入运行详情">
          <section className="build-run-summary-surface" aria-labelledby="build-run-detail-title">
            <header className="build-run-detail-header">
              <div className="build-run-detail-identity">
                <div className="build-run-detail-title"><span>{selectedRun.mode} · {selectedRun.sourceName ?? `${selectedRun.sourceCount} 个来源`} · {selectedRun.duration}</span><h1 id="build-run-detail-title">{selectedRun.taskName}</h1><code>{selectedRun.id}</code></div>
              </div>
              {selectedRunDiff && <dl className="build-run-title-diff" aria-label="元数据版本差异">
                <div title="Catalog 新成员"><dd>{selectedRunDiff.added}</dd><dt>新增</dt></div>
                <div title="DDL、字段或约束变化"><dd>{selectedRunDiff.modified}</dd><dt>修改</dt></div>
                <div title="相对上次元数据版本缺失"><dd>{selectedRunDiff.removed}</dd><dt>删除</dt></div>
              </dl>}
              <div className="build-run-detail-summary">
                <span className={`build-task-status build-run-page-status ${selectedRun.status === "有警告" ? "build-task-status-warning" : ""}`}><i />{selectedRun.status}</span>
                <span className="build-run-detail-timing"><small>开始时间</small><strong>{selectedRun.startedAt}</strong></span>
                <button className="icon-button build-run-log-button" type="button" aria-label="查看执行日志" title="查看执行日志" onClick={() => setViewingRunLog(true)}><FileClock size={16} /></button>
                {selectedRun.proposals > 0 && <button className="primary-button" type="button" aria-label={`打开 ${selectedRun.proposals} 个关联候选版本`} onClick={() => onOpenGovernanceProposals(selectedRun.id)}><Sparkles size={16} />打开关联候选版本<span aria-hidden="true">{selectedRun.proposals}</span></button>}
              </div>
            </header>
          </section>

          {selectedRun.status === "有警告" && <div className="build-record-warning" role="status"><AlertTriangle size={16} /><span><strong>1 项接入异常</strong><small>发现无法解析的旧区域别名；检查点未推进，也没有触发下游知识构建。</small></span></div>}

          {selectedRun.kind === "embedding-rebuild" ? <section className="embedding-run-detail" aria-label="向量索引重建进度">
            <header><div><span className="content-label">当前阶段</span><h2>{selectedRun.phase}</h2><p>线上检索继续使用现有索引；新索引完成质量校验后再原子切换。</p></div><strong>{selectedRun.progress}%</strong></header>
            <div className="embedding-run-progress-track" aria-label={`向量索引重建进度 ${selectedRun.progress}%`}><i style={{ width: `${selectedRun.progress}%` }} /></div>
            <ol className="embedding-run-stages">
              {([['排队', 5], ['扫描知识块', 15], ['生成向量', 40], ['写入新索引', 70], ['质量校验', 90], ['切换生效', 100]] as const).map(([stage, threshold], index) => { const current = selectedRun.phase === stage; const complete = !current && selectedRun.progress! >= threshold; return <li className={current ? "is-current" : complete ? "is-complete" : ""} key={stage}><span>{complete ? <CheckCircle2 size={14} /> : index + 1}</span><strong>{stage}</strong></li>; })}
            </ol>
            <dl className="embedding-run-facts">
              <div><dt>目标模型</dt><dd>{selectedRun.modelId}</dd></div>
              <div><dt>处理进度</dt><dd>{selectedRun.processedItems?.toLocaleString("zh-CN")} / {selectedRun.totalItems?.toLocaleString("zh-CN")} 个知识块</dd></div>
              <div><dt>索引切换</dt><dd>校验通过后自动生效</dd></div>
              <div><dt>当前服务</dt><dd>现有索引持续可用</dd></div>
            </dl>
          </section> : selectedRun.kind === "ingestion" ? <section className="ingestion-run-detail" aria-labelledby="ingestion-run-detail-title">
            <header><div><span className="content-label">接入结果</span><h2 id="ingestion-run-detail-title">{selectedRun.outputSummary}</h2><p>记录来源读取、快照差异和检查点推进；知识生成由独立的全局运行负责。</p></div>{selectedRun.downstreamRunId && <button className="secondary-button" type="button" onClick={() => onOpenGlobalRun(selectedRun.downstreamRunId!)}><ArrowRight size={15} />打开下游运行</button>}</header>
            <div className="ingestion-run-facts"><article><small>数据来源</small><strong>{selectedRun.sourceName}</strong><span>只读连接</span></article><article><small>来源版本</small><strong>{selectedRun.sourceRevision}</strong><span>可追溯快照</span></article><article><small>检查点</small><strong>{selectedRun.checkpoint}</strong><span>{selectedRun.status === "已完成" ? "已推进" : "保持不变"}</span></article><article><small>结构变化</small><strong>{selectedRun.physicalChanges}</strong><span>{selectedRunDiff ? `${selectedRunDiff.added} 新增 · ${selectedRunDiff.modified} 修改 · ${selectedRunDiff.removed} 删除` : "无变化"}</span></article></div>
            <ol className="ingestion-run-stages" aria-label="接入阶段"><li className="is-complete"><CheckCircle2 size={14} /><span><strong>连接来源</strong><small>只读凭据验证通过</small></span></li><li className="is-complete"><CheckCircle2 size={14} /><span><strong>读取快照</strong><small>{selectedRun.sourceRevision}</small></span></li><li className={selectedRun.status === "有警告" ? "is-warning" : "is-complete"}>{selectedRun.status === "有警告" ? <AlertTriangle size={14} /> : <CheckCircle2 size={14} />}<span><strong>计算差异</strong><small>{selectedRun.outputSummary}</small></span></li><li className={selectedRun.downstreamRunId ? "is-complete" : "is-muted"}>{selectedRun.downstreamRunId ? <CheckCircle2 size={14} /> : <Clock3 size={14} />}<span><strong>触发下游</strong><small>{selectedRun.downstreamRunId ?? "未创建知识构建运行"}</small></span></li></ol>
          </section> : <section className="build-run-section build-run-output-surface" aria-labelledby="build-run-output-title">
            <header className="build-run-result-header"><div><h2 id="build-run-output-title">运行结果关系</h2><span>关联证据 → 变化对象 → 候选版本</span></div>{selectedRun.status === "有警告" ? <button className="secondary-button" type="button" onClick={() => selectRunGraphItem("changes", "change-orders-amount")}><AlertTriangle size={15} />定位异常</button> : null}</header>
            <div className="build-run-result-workspace">
              <RunResultCanvas runId={selectedRun.id} groups={runResultGroups} activeTab={activeRunResultTab} activeItem={activeRunResultItem} onSelect={selectRunGraphItem} />
              <aside className="build-run-result-detail" aria-label="选中结果详情">
                {activeRunResultItem ? <><header><span className={activeRunResultItem.warning ? "build-run-result-detail-icon build-run-result-detail-icon-warning" : "build-run-result-detail-icon"}>{activeRunResultTab === "changes" ? <Braces size={16} /> : activeRunResultTab === "evidence" ? <FileSearch size={16} /> : <Sparkles size={16} />}</span><div><small>{activeRunResultItem.kind}</small><h3>{activeRunResultItem.title}</h3></div></header><p>{activeRunResultItem.summary}</p><dl>{activeRunResultItem.facts.map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl><footer><span className={activeRunResultItem.warning ? "build-run-result-state build-run-result-state-warning" : "build-run-result-state"}><i />{activeRunResultItem.state}</span><small>节点详情来自本次运行归档。</small></footer></> : <div className="build-run-result-detail-empty"><CheckCircle2 size={18} /><strong>没有可检查的详情</strong></div>}
              </aside>
            </div>
          </section>}
        </section>
      </>}

      {creatingSource && <ConnectionDialog onClose={() => setCreatingSource(false)} onSave={(source) => { const id = `database-${Date.now()}`; setDatabaseConnections((current) => [...current, { ...source, id, detail: "只读 Catalog · 等待首次基线", status: "健康" }]); setActiveSourceTab("database"); setCreatingSource(false); onNotify(`${source.name} 已连接，首次增量分析将建立元数据基线。`); }} />}
      {editingDatabase && <ConnectionDialog initialConnection={editingDatabase} onClose={() => setEditingDatabase(null)} onSave={(source) => { setDatabaseConnections((current) => current.map((connection) => connection.id === editingDatabase.id ? { ...connection, ...source, status: "健康" } : connection)); setEditingDatabase(null); onNotify(`${source.name} 的连接配置已更新。`); }} />}
      {deletingDatabase && <DeleteSourceDialog title="删除数据库连接" name={deletingDatabase.name} description="删除后将停止同步；已生成的知识块和构建记录仍会保留。" onClose={() => setDeletingDatabase(null)} onConfirm={() => { const deletedName = deletingDatabase.name; setDatabaseConnections((current) => current.filter((connection) => connection.id !== deletingDatabase.id)); setDeletingDatabase(null); onNotify(`${deletedName} 已删除，相关知识块和构建记录继续保留。`); }} />}
      {importingFile && <FileImportDialog existingDatasets={existingFileDatasets} initialTargetDataset={reimportTarget?.name} onClose={() => { setImportingFile(false); setReimportTarget(null); }} onImport={(source) => { const kind = source.kind as FileDataset["kind"]; if (source.targetDataset) { setFileDatasets((current) => current.map((dataset) => dataset.name === source.targetDataset ? { ...dataset, kind, endpoint: source.endpoint, detail: kind === "MD" ? "新版本 · 本次原型会话" : "新快照 · 本次原型会话", status: "已就绪" } : dataset)); } else { const id = `file-${Date.now()}`; setFileDatasets((current) => [...current, { id, name: source.name, kind, endpoint: source.endpoint, detail: kind === "MD" ? "Markdown 文档 · 本次原型会话" : "本次原型会话", status: "已就绪", encoding: "UTF-8", delimiter: ",", sheet: "全部工作表", firstRowHeader: true, chunking: "按标题层级", includeCodeBlocks: true }]); } setActiveSourceTab("file"); setImportingFile(false); setReimportTarget(null); onNotify(`${source.name} 已导入并加入知识构建来源。`); }} />}
      {editingFile && <FileDatasetDialog dataset={editingFile} onClose={() => setEditingFile(null)} onSave={(dataset) => { setFileDatasets((current) => current.map((file) => file.id === dataset.id ? dataset : file)); setEditingFile(null); onNotify(`${dataset.name} 的文件来源设置已更新。`); }} />}
      {deletingFile && <DeleteSourceDialog title="删除文件来源" name={deletingFile.name} description="删除后不再参与后续构建；已生成的知识块和构建记录仍会保留。" onClose={() => setDeletingFile(null)} onConfirm={() => { const deletedName = deletingFile.name; setFileDatasets((current) => current.filter((dataset) => dataset.id !== deletingFile.id)); setDeletingFile(null); onNotify(`${deletedName} 已删除，相关知识块和构建记录继续保留。`); }} />}
      {creatingTask && <BuildTaskDialog availableSources={availableTaskSources} onClose={() => setCreatingTask(false)} onSave={(task) => { const id = `build-task-${Date.now()}`; setBuildTasks((current) => [...current, { ...task, id, status: task.enabled ? "就绪" : "停用", lastRun: "尚未运行" }]); setCreatingTask(false); setTaskFilter("all"); onNotify(`${task.name} 已创建。`); }} />}
      {editingTask && <BuildTaskDialog initialTask={editingTask} availableSources={availableTaskSources} onClose={() => setEditingTask(null)} onSave={(task) => { setBuildTasks((current) => current.map((item) => item.id === editingTask.id ? { ...item, ...task, status: task.enabled ? (item.status === "停用" ? "就绪" : item.status) : "停用" } : item)); setEditingTask(null); onNotify(`${task.name} 的任务配置已更新。`); }} />}
      {deletingTask && <DeleteSourceDialog title="删除构建任务" name={deletingTask.name} description="删除任务不会删除数据来源、知识块或历史构建记录。" onClose={() => setDeletingTask(null)} onConfirm={() => { const deletedId = deletingTask.id; const deletedName = deletingTask.name; setBuildTasks((current) => current.filter((task) => task.id !== deletedId)); setDeletingTask(null); onNotify(`${deletedName} 已删除，历史构建记录继续保留。`); }} />}
      {viewingRunLog && selectedRun && <BuildRunLogDialog run={selectedRun} onClose={() => setViewingRunLog(false)} />}
    </section>
  );
}

import { createElement, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  ArrowUpDown,
  BadgeCheck,
  BookOpenCheck,
  Box,
  Boxes,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  CircleDot,
  Database,
  FileCheck2,
  Fingerprint,
  GitPullRequestArrow,
  LayoutDashboard,
  ListFilter,
  Link2,
  LockKeyhole,
  Network,
  PackageCheck,
  PanelLeftClose,
  PanelRightClose,
  PanelRightOpen,
  RotateCcw,
  Search,
  Settings2,
  ShieldCheck,
  Sigma,
  Sparkles,
  TableProperties,
  Tags,
  MessageSquareText,
  Users,
  X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { assetVersionReleases, assets, proposals } from "./data";
import { CapabilityProvider, useCan } from "./authorization";
import { AccessControlView } from "./AccessControlView";
import { assetTypeProfileFor, evaluateAssetTypeRules, type AssetEmptyStateConfig } from "./assetTypeProfiles";
import { AuditRuntimeView } from "./AuditRuntimeView";
import { SourcesView } from "./JourneyViews";
import { IntegrationSettingsView } from "./IntegrationSettingsView";
import { AskView } from "./KnowledgeViews";
import { KnowledgeRevisionWorkbench } from "./KnowledgeRevisionWorkbench";
import { ModelConfigurationView, type EmbeddingRebuildRun } from "./ModelConfigurationView";
import { SemanticGraph } from "./SemanticGraph";
import type { Asset, AssetType, AssetVersionRelease, KnowledgeRevisionRequest, KnowledgeRevisionSubmission, NavigateToView, Proposal, ValidationState, ViewId } from "./types";

interface NavigationItem {
  id: ViewId;
  label: string;
  icon: LucideIcon;
  activeViews?: ViewId[];
}

const primaryNavigation: NavigationItem[] = [
  { id: "ask", label: "语义问答", icon: MessageSquareText },
  { id: "overview", label: "工作台", icon: LayoutDashboard },
  { id: "assets", label: "知识资产", icon: Boxes },
  { id: "releases", label: "变更与发布", icon: GitPullRequestArrow },
  { id: "sources", label: "数据接入", icon: Database },
];

const systemNavigation: NavigationItem[] = [
  { id: "settings", label: "系统设置", icon: Settings2 },
];

const navigation = [...primaryNavigation, ...systemNavigation];

const assetTabs = ["概览", "定义", "本体关系", "实现", "可信度", "交付与影响"] as const;
type AssetTab = (typeof assetTabs)[number];

function assetTabsFor(asset: Asset): AssetTab[] {
  const profile = assetTypeProfileFor(asset);
  const hasExecutableImplementation = profile.implementationMode !== "not_applicable" || asset.bindings.length > 0 || asset.joinContracts.length > 0;
  return hasExecutableImplementation ? [...assetTabs] : assetTabs.filter((tab) => tab !== "实现");
}
type CatalogObjectType = AssetType | "语义关系" | "物理绑定" | "JoinContract";
type CatalogStatus = Asset["status"] | "已验证" | "需重验";

interface KnowledgeCatalogItem {
  id: string;
  assetId: string;
  openTab: AssetTab;
  name: string;
  key: string;
  type: CatalogObjectType;
  status: CatalogStatus;
  domain: string;
  owner: string;
  maintainer: string;
  scope: string;
  detail: string;
  aliases: string[];
}

const initialRecentAssetIds = [
  "metric_01J4NETREVENUE8W4Q9D7K2",
  "model_01J4ORDERS7K8M2Q5N9P",
  "segment_01J4HIGHVALUE4X7P2K9M6N",
];

const assetTypes: AssetType[] = ["业务概念", "业务实体", "语义模型", "维度", "度量", "指标", "分群"];
const catalogObjectTypes: CatalogObjectType[] = [...assetTypes, "语义关系", "物理绑定", "JoinContract"];

function assetTypeIcon(type: AssetType) {
  if (type === "指标") return Sigma;
  if (type === "度量") return CircleDot;
  if (type === "维度") return Tags;
  if (type === "业务实体") return Fingerprint;
  if (type === "语义模型") return TableProperties;
  if (type === "分群") return ListFilter;
  return BookOpenCheck;
}

function AssetTypeMark({ type, size = 16 }: { type: AssetType; size?: number }) {
  return <span className={`asset-type-mark asset-type-${type}`}>{createElement(assetTypeIcon(type), { size })}</span>;
}

function CatalogTypeMark({ type }: { type: CatalogObjectType }) {
  if (assetTypes.includes(type as AssetType)) return <AssetTypeMark type={type as AssetType} />;
  const Icon = type === "语义关系" ? Network : type === "物理绑定" ? Link2 : GitPullRequestArrow;
  return <span className={`catalog-type-mark catalog-type-${type}`}>{createElement(Icon, { size: 16 })}</span>;
}

function AssetEmptyState({ config, icon: Icon, assetName, onAction }: { config: AssetEmptyStateConfig; icon: LucideIcon; assetName: string; onAction?: (message: string) => void }) {
  return (
    <div className={`asset-inline-empty asset-empty-${config.kind}`}>
      <Icon size={21} />
      <span><strong>{config.title}</strong><small>{config.detail}</small></span>
      {config.action && onAction && <button type="button" onClick={() => onAction(`${assetName}：${config.action}已进入原型待办。`)}>{config.action}</button>}
    </div>
  );
}

function relationLabel(type: Asset["relations"][number]["type"]) {
  return ({ measures: "衡量", describes: "描述", depends_on: "依赖", derived_from: "来源于", filters_by: "按其筛选", synonym_of: "同义于", contains: "包含" } as Record<Asset["relations"][number]["type"], string>)[type];
}

function assetContractFields(asset: Asset) {
  const labels: Record<AssetType, [string, string, string, string, string]> = {
    指标: ["计算表达式", "结果粒度", "时间语义", "单位与格式", "聚合规则"],
    度量: ["度量表达式", "执行粒度", "时间维度", "计量单位", "聚合函数"],
    维度: ["取值表达式", "主体粒度", "时间适用", "值类型", "聚合行为"],
    业务实体: ["实体键表达式", "身份粒度", "生效时间", "实体单位", "聚合行为"],
    语义模型: ["模型定义", "行粒度", "默认时间", "输出单位", "成员行为"],
    分群: ["成员条件", "评估粒度", "快照时间", "输出类型", "聚合行为"],
    业务概念: ["规范表达式", "适用粒度", "时间语义", "表示形式", "聚合行为"],
  };
  const spec = asset.revisionRecord.typeSpec;
  const [expression, grain, time, unit, aggregation] = labels[asset.type];
  const fields = [
    { label: expression, value: spec.expression, code: true, fieldPath: "spec.expression" },
    { label: grain, value: spec.grain, fieldPath: "spec.grain" },
    { label: time, value: spec.defaultTime, fieldPath: "spec.default_time" },
    { label: unit, value: spec.unit, fieldPath: "spec.unit" },
    { label: aggregation, value: spec.aggregation, fieldPath: "spec.aggregation" },
  ];
  if (spec.kind === "metric") fields.push({ label: "指标类型与维度", value: `${spec.metricKind} · ${spec.allowedDimensions.join(" / ")}`, fieldPath: "spec.allowed_dimensions" });
  else if (spec.kind === "measure") fields.push({ label: "可加性与空值", value: `${spec.additivity} · ${spec.nullHandling}`, fieldPath: "spec.additivity" });
  else if (spec.kind === "dimension") fields.push({ label: "值域语义", value: `${spec.valueType} · ${spec.nullSemantics}`, fieldPath: "spec.value_semantics" });
  else if (spec.kind === "entity") fields.push({ label: "身份策略", value: spec.identityPolicy, fieldPath: "spec.identity_policy" });
  else if (spec.kind === "model") fields.push({ label: "公开成员", value: spec.publicMembers.join(" · "), fieldPath: "spec.public_members" });
  else if (spec.kind === "segment") fields.push({ label: "生效与刷新", value: `${spec.effectiveTime} · ${spec.refreshPolicy}`, fieldPath: "spec.effective_time" });
  else fields.push({ label: "消歧规则", value: spec.disambiguationRule, fieldPath: "wiki.disambiguation" });
  return fields;
}

function deploymentLabel(asset: Asset) {
  if (asset.deployment.state === "production") return "生产";
  if (asset.deployment.state === "staging") return "预发布";
  return "未发布";
}

function compatibilityLabel(state: Asset["consumerBindings"][number]["compatibility"]) {
  return ({ compatible: "兼容", conditional: "需确认", breaking: "不兼容", not_evaluated: "未评估" } as const)[state];
}

const knowledgeCatalogItems: KnowledgeCatalogItem[] = assets.flatMap((asset) => {
  const assetItem: KnowledgeCatalogItem = {
    id: asset.id,
    assetId: asset.id,
    openTab: "概览",
    name: asset.name,
    key: asset.key,
    type: asset.type,
    status: asset.status,
    domain: asset.domain,
    owner: asset.owner,
    maintainer: asset.maintainer,
    scope: `${asset.relations.length} 条关系 · ${asset.bindings.length} 个绑定`,
    detail: `${asset.joinContracts.length} 个 JoinContract`,
    aliases: asset.aliases,
  };
  const relations: KnowledgeCatalogItem[] = asset.relations.map((relation) => ({
    id: `relation:${asset.id}:${relation.id}`,
    assetId: asset.id,
    openTab: "本体关系",
    name: `${asset.name}${relationLabel(relation.type)}${relation.targetName}`,
    key: relation.id,
    type: "语义关系",
    status: relation.release === "draft" ? "草稿" : "已发布",
    domain: asset.domain,
    owner: asset.owner,
    maintainer: asset.maintainer,
    scope: `${asset.name} → ${relation.targetName}`,
    detail: relation.type,
    aliases: [],
  }));
  const bindings: KnowledgeCatalogItem[] = asset.bindings.map((binding) => ({
    id: `binding:${asset.id}:${binding.id}`,
    assetId: asset.id,
    openTab: "实现",
    name: `${binding.dataset} 映射至 ${asset.name}`,
    key: binding.id,
    type: "物理绑定",
    status: binding.state,
    domain: asset.domain,
    owner: asset.owner,
    maintainer: asset.maintainer,
    scope: `${binding.dataset} → ${asset.name}`,
    detail: binding.sourceRevision,
    aliases: [],
  }));
  const joins: KnowledgeCatalogItem[] = asset.joinContracts.map((join) => ({
    id: `join:${asset.id}:${join.id}`,
    assetId: asset.id,
    openTab: "实现",
    name: `${asset.name} 关联 ${join.target}`,
    key: join.id,
    type: "JoinContract",
    status: join.state,
    domain: asset.domain,
    owner: asset.owner,
    maintainer: asset.maintainer,
    scope: `${asset.name} ↔ ${join.target}`,
    detail: join.cardinality,
    aliases: [],
  }));
  return [assetItem, ...relations, ...bindings, ...joins];
});

function readinessSummary(asset: Asset) {
  const applicable = asset.readiness.filter((gate) => gate.state !== "not_applicable");
  const passed = applicable.filter((gate) => gate.state === "passed").length;
  return { passed, total: applicable.length, warnings: applicable.length - passed };
}

function statusTone(status: CatalogStatus) {
  if (status === "已发布" || status === "已验证") return "success";
  if (status === "需关注" || status === "需重验") return "warning";
  return "neutral";
}

function validationIcon(state: ValidationState) {
  if (state === "passed") return <CheckCircle2 size={16} />;
  if (state === "warning") return <AlertTriangle size={16} />;
  return <CircleAlert size={16} />;
}

function validationLabel(name: string) {
  return ({ "Schema references": "Schema 引用", "Evidence links": "证据引用", "Cube compile": "Cube 编译", "Result regression": "结果回归", "Policy G1": "策略 G1", "Consumer impact": "消费影响" } as Record<string, string>)[name] ?? name;
}

function StatusBadge({ tone, children }: { tone: "success" | "warning" | "danger" | "neutral" | "violet" | "info"; children: React.ReactNode }) {
  return <span className={`status-badge status-${tone}`}>{children}</span>;
}

const changeReleaseContextItems = [
  { label: "资产版本", meta: "3 个候选 · 6 条已发布", tone: "warning" as const },
];

const contextItems: Record<ViewId, Array<{ label: string; meta: string; tone?: "warning" | "danger" }>> = {
  ask: [
    { label: "新会话", meta: "开始提问" },
    { label: "8 月收入诊断", meta: "刚刚" },
    { label: "客单价口径", meta: "昨天" },
    { label: "知识覆盖检查", meta: "3 天前" },
  ],
  sources: [
    { label: "数据来源", meta: "1 库 · 1 文件" },
    { label: "接入自动化", meta: "3 项策略", tone: "warning" },
    { label: "接入运行", meta: "1 条异常" },
  ],
  overview: [],
  assets: [],
  releases: changeReleaseContextItems,
  settings: [
    { label: "成员", meta: "24 名成员" },
    { label: "访问控制", meta: "9 个系统角色" },
    { label: "模型配置", meta: "LLM · Embedding" },
    { label: "接口与集成", meta: "4 种接口" },
    { label: "审计与运行", meta: "全局记录" },
  ],
};

const contextTitles: Record<ViewId, string> = {
  ask: "语义问答",
  sources: "数据接入",
  overview: "工作台",
  assets: "知识资产",
  releases: "变更与发布",
  settings: "系统设置",
};

const initialAskConversationTitles: Record<number, string> = {
  0: "新会话",
  1: "8 月收入诊断",
  2: "客单价口径",
  3: "知识覆盖检查",
};

const contextSectionLabels: Record<ViewId, string> = {
  ask: "最近会话",
  overview: "工作台",
  assets: "知识目录",
  releases: "变更与发布",
  sources: "数据接入",
  settings: "平台管理",
};

const contextPanelWidthRange = { min: 184, max: 440 } as const;

function clampContextPanelWidth(width: number) {
  return Math.min(contextPanelWidthRange.max, Math.max(contextPanelWidthRange.min, width));
}

function initialContextPanelWidth() {
  if (typeof window === "undefined") return 264;
  if (window.innerWidth <= 1180) return 212;
  if (window.innerWidth <= 1280) return 224;
  return 264;
}

function ActivityRail({ view, onChange }: { view: ViewId; onChange: (view: ViewId) => void }) {
  const renderItem = (item: (typeof navigation)[number]) => {
    const Icon = item.icon;
    const active = item.activeViews?.includes(view) ?? view === item.id;
    return (
      <button
        className={active ? "rail-item rail-item-active" : "rail-item"}
        key={item.id}
        type="button"
        title={item.label}
        aria-current={active ? "page" : undefined}
        aria-label={item.label}
        onClick={() => onChange(item.id)}
      >
        <Icon size={18} strokeWidth={1.8} />
        <span className="rail-label">{item.label}</span>
        {item.id === "overview" && <span className="rail-count">3</span>}
      </button>
    );
  };
  return (
    <aside className="activity-rail">
      <button className="rail-brand" type="button" title="返回语义问答" aria-label="返回语义问答" onClick={() => onChange("ask")}>
        <Fingerprint size={20} strokeWidth={1.8} />
      </button>

      <nav className="rail-navigation" aria-label="Semlia 主功能">
        {primaryNavigation.map(renderItem)}
      </nav>

      <nav className="rail-system" aria-label="Semlia 平台管理">
        {systemNavigation.map(renderItem)}
      </nav>
    </aside>
  );
}

function ContextPanel({ view, activeIndex, width, recentAssets, activeAssetId, assetDetailOpen, activeWorkbenchTask, askConversationTitles, conversationSearchRequestEpoch, onSelect, onOpenRecentAsset, onOpenWorkbenchTask, onOpenCatalogSearch, onOpenWorkbenchSearch, onCollapse, onResize }: { view: ViewId; activeIndex: number; width: number; recentAssets: Asset[]; activeAssetId: string; assetDetailOpen: boolean; activeWorkbenchTask: WorkbenchTask | null; askConversationTitles: Record<number, string>; conversationSearchRequestEpoch: number; onSelect: (index: number) => void; onOpenRecentAsset: (id: string) => void; onOpenWorkbenchTask: (task: WorkbenchTask) => void; onOpenCatalogSearch: () => void; onOpenWorkbenchSearch: () => void; onCollapse: () => void; onResize: (width: number) => void }) {
  const isAssetCatalog = view === "assets";
  const hasContextSearch = view !== "sources" && view !== "releases" && view !== "settings";
  const canReadRoles = useCan("role.read");
  const canAssignRoles = useCan("role.assign");
  const canInspectAuthorization = useCan("authorization.inspect");
  const canViewAccessControl = canReadRoles || canAssignRoles || canInspectAuthorization;
  const resizeState = useRef<{ pointerId: number; startX: number; startWidth: number } | null>(null);
  const contextListRef = useRef<HTMLDivElement>(null);
  const conversationSearchRef = useRef<HTMLInputElement>(null);
  const [conversationSearch, setConversationSearch] = useState("");
  const normalizedConversationSearch = conversationSearch.trim().toLowerCase();
  const visibleConversations = contextItems.ask.map((item, index) => ({ item, index, title: askConversationTitles[index] ?? item.label })).filter(({ item, title }) => !normalizedConversationSearch || `${title} ${item.meta}`.toLowerCase().includes(normalizedConversationSearch));
  useEffect(() => {
    const handleResizeMove = (event: PointerEvent) => {
      const state = resizeState.current;
      if (!state || state.pointerId !== event.pointerId) return;
      onResize(clampContextPanelWidth(state.startWidth + event.clientX - state.startX));
    };
    const handleResizeEnd = (event: PointerEvent) => {
      if (resizeState.current?.pointerId !== event.pointerId) return;
      resizeState.current = null;
      document.body.classList.remove("context-panel-resizing");
    };
    window.addEventListener("pointermove", handleResizeMove);
    window.addEventListener("pointerup", handleResizeEnd);
    window.addEventListener("pointercancel", handleResizeEnd);
    return () => {
      window.removeEventListener("pointermove", handleResizeMove);
      window.removeEventListener("pointerup", handleResizeEnd);
      window.removeEventListener("pointercancel", handleResizeEnd);
      document.body.classList.remove("context-panel-resizing");
    };
  }, [onResize]);
  useEffect(() => {
    const hasSelectedObject = (view === "assets" && assetDetailOpen) || (view === "overview" && activeWorkbenchTask);
    if (!hasSelectedObject) return;
    const activeItem = contextListRef.current?.querySelector<HTMLElement>('[aria-current="page"]');
    if (activeItem && typeof activeItem.scrollIntoView === "function") activeItem.scrollIntoView({ block: "nearest" });
  }, [activeAssetId, activeWorkbenchTask, assetDetailOpen, view]);
  useEffect(() => {
    if (view === "ask" && conversationSearchRequestEpoch > 0) conversationSearchRef.current?.focus();
  }, [conversationSearchRequestEpoch, view]);
  const handleResizeStart = (event: React.PointerEvent<HTMLDivElement>) => {
    resizeState.current = { pointerId: event.pointerId, startX: event.clientX, startWidth: width };
    document.body.classList.add("context-panel-resizing");
    event.preventDefault();
  };
  const handleResizeKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "ArrowLeft") onResize(clampContextPanelWidth(width - 16));
    else if (event.key === "ArrowRight") onResize(clampContextPanelWidth(width + 16));
    else if (event.key === "Home") onResize(contextPanelWidthRange.min);
    else if (event.key === "End") onResize(contextPanelWidthRange.max);
    else return;
    event.preventDefault();
  };
  return (
    <aside className={hasContextSearch ? "context-panel" : "context-panel context-panel-no-search"} aria-label="治理上下文">
      <header className="context-panel-header">
        <div className="context-section-label">{contextSectionLabels[view]}</div>
        <button className="context-collapse-button" type="button" aria-label="收起二级菜单" title="收起二级菜单" onClick={onCollapse}><PanelLeftClose size={16} strokeWidth={1.8} /></button>
      </header>
      {view === "ask" ? <label className="context-search context-search-input"><Search size={14} /><input ref={conversationSearchRef} type="search" aria-label="搜索会话" placeholder="搜索会话" value={conversationSearch} onChange={(event) => setConversationSearch(event.target.value)} />{conversationSearch ? <button className="context-search-clear" type="button" aria-label="清除会话搜索" title="清除会话搜索" onClick={() => { setConversationSearch(""); conversationSearchRef.current?.focus(); }}><X size={13} /></button> : <kbd>⌘ K</kbd>}</label> : hasContextSearch && <button className="context-search" type="button" aria-label={isAssetCatalog ? "搜索知识目录" : "搜索待办"} onClick={isAssetCatalog ? onOpenCatalogSearch : onOpenWorkbenchSearch}><Search size={14} /><span>{isAssetCatalog ? "搜索知识目录" : "搜索待办"}</span><kbd>⌘ K</kbd></button>}
      <div className="context-list" ref={contextListRef}>
        {view === "assets" ? recentAssets.map((asset) => (
          <button className={assetDetailOpen && asset.id === activeAssetId ? "context-item context-recent-asset context-item-active" : "context-item context-recent-asset"} type="button" key={asset.id} aria-label={`打开最近资产 ${asset.name}`} aria-current={assetDetailOpen && asset.id === activeAssetId ? "page" : undefined} onClick={() => onOpenRecentAsset(asset.id)}>
            <span className="context-asset-mark"><AssetTypeMark type={asset.type} size={14} /></span>
            <span><strong>{asset.name}</strong><small>{asset.type} · {asset.domain}</small></span>
            <ChevronRight size={14} />
          </button>
        )) : view === "overview" ? workbenchTasks.filter((task) => task.scope === "mine").sort((left, right) => ({ P0: 0, P1: 1, P2: 2 }[left.priority] - { P0: 0, P1: 1, P2: 2 }[right.priority]) || right.ageMinutes - left.ageMinutes).map((task) => (
          <button className={activeWorkbenchTask?.id === task.id ? "context-item context-workbench-task context-item-active" : "context-item context-workbench-task"} type="button" key={task.id} aria-label={`打开待办详情 ${task.title}`} aria-current={activeWorkbenchTask?.id === task.id ? "page" : undefined} onClick={() => onOpenWorkbenchTask(task)}>
            <span className={`context-task-mark context-task-mark-${task.priority.toLowerCase()}`} title={`${task.priority} · ${task.risk}`}>{task.priority === "P0" ? <AlertTriangle size={14} /> : <CircleAlert size={14} />}</span>
            <span><strong>{task.title}</strong><small><code>{task.id}</code> · {task.state}</small></span>
            <ChevronRight size={14} />
          </button>
        )) : view === "ask" ? visibleConversations.length > 0 ? visibleConversations.map(({ item, index, title }) => (
          <button className={index === activeIndex ? "context-item context-item-active" : "context-item"} type="button" key={item.label} onClick={() => onSelect(index)}>
            <span className={`context-dot${item.tone ? ` context-dot-${item.tone}` : ""}`} />
            <span><strong>{title}</strong><small>{item.meta}</small></span>
            <ChevronRight size={14} />
          </button>
        )) : <div className="context-search-empty"><Search size={16} /><strong>没有匹配会话</strong><span>尝试搜索其他会话标题。</span></div> : contextItems[view].map((item, index) => ({ item, index })).filter(({ index }) => view !== "settings" || index !== 1 || canViewAccessControl).map(({ item, index }) => (
          <button className={index === activeIndex ? "context-item context-item-active" : "context-item"} type="button" key={item.label} onClick={() => onSelect(index)}>
            <span className={`context-dot${item.tone ? ` context-dot-${item.tone}` : ""}`} />
            <span><strong>{item.label}</strong><small>{item.meta}</small></span>
            <ChevronRight size={14} />
          </button>
        ))}
      </div>
      <div className="context-resize-handle" role="separator" aria-label="调整二级菜单宽度" aria-orientation="vertical" aria-valuemin={contextPanelWidthRange.min} aria-valuemax={contextPanelWidthRange.max} aria-valuenow={Math.round(width)} tabIndex={0} onPointerDown={handleResizeStart} onKeyDown={handleResizeKeyDown} />
    </aside>
  );
}

function AccessControlGate({ children }: { children: ReactNode }) {
  const canReadRoles = useCan("role.read");
  const canAssignRoles = useCan("role.assign");
  const canInspectAuthorization = useCan("authorization.inspect");
  const allowed = canReadRoles || canAssignRoles || canInspectAuthorization;
  if (allowed) return children;
  return <section className="view settings-view access-control-view" aria-label="访问控制"><div className="access-inspector-empty"><LockKeyhole size={22} /><strong>没有访问控制权限</strong><span>需要角色读取、角色分配或有效权限检查能力。</span></div></section>;
}

function Topbar({ view, contextLabel, onNewConversation, onRenameContext, onBack, backLabel = "返回" }: { view: ViewId; contextLabel?: string; onNewConversation: () => void; onRenameContext?: (title: string) => void; onBack?: () => void; backLabel?: string }) {
  const [isEditingTitle, setIsEditingTitle] = useState(false);
  const [draftTitle, setDraftTitle] = useState(contextLabel ?? "");
  const titleInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isEditingTitle) titleInputRef.current?.focus();
  }, [isEditingTitle]);

  const commitTitle = () => {
    const nextTitle = draftTitle.trim();
    if (!nextTitle) setDraftTitle(contextLabel ?? "");
    else if (nextTitle !== contextLabel) onRenameContext?.(nextTitle);
    setIsEditingTitle(false);
  };

  const cancelTitle = () => {
    setDraftTitle(contextLabel ?? "");
    setIsEditingTitle(false);
  };

  return (
    <header className="topbar">
      <div className="workspace-breadcrumb">{onBack && <button className="topbar-back-button" type="button" aria-label={backLabel} title={backLabel} onClick={onBack}><ArrowLeft size={16} /></button>}{contextLabel && (onRenameContext ? (isEditingTitle ? <input ref={titleInputRef} className="workspace-title-input" aria-label="编辑会话标题" value={draftTitle} onChange={(event) => setDraftTitle(event.target.value)} onBlur={commitTitle} onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); commitTitle(); } if (event.key === "Escape") { event.preventDefault(); cancelTitle(); } }} /> : <button className="workspace-title-button" type="button" aria-label={`编辑会话标题：${contextLabel}`} onClick={() => setIsEditingTitle(true)}>{contextLabel}</button>) : <strong>{contextLabel}</strong>)}</div>
      <div className="topbar-actions">
        {view === "ask" && <button className="primary-button" type="button" onClick={onNewConversation}><MessageSquareText size={16} />新建会话</button>}
      </div>
    </header>
  );
}

type WorkbenchScope = "mine" | "created" | "team";
type WorkbenchTaskKind = "版本审核" | "运行诊断" | "迁移确认";
type WorkbenchRisk = "高风险" | "中风险" | "低风险";

interface WorkbenchTask {
  id: string;
  title: string;
  source: string;
  object: string;
  objectType: string;
  action: string;
  kind: WorkbenchTaskKind;
  priority: "P0" | "P1" | "P2";
  risk: WorkbenchRisk;
  owner: string;
  timing: string;
  ageMinutes: number;
  state: "验证阻塞" | "待审核" | "待诊断" | "等待他人";
  scope: Exclude<WorkbenchScope, "team">;
  target: "candidate-region" | "candidate-aov" | "candidate-revenue" | "source-run";
}

const workbenchTasks: WorkbenchTask[] = [
  { id: "WB-104", title: "旧区域别名无法安全废弃", source: "解析反馈", object: "业务区域 @4", objectType: "候选资产版本", action: "确认 3 个消费者迁移", kind: "迁移确认", priority: "P0", risk: "高风险", owner: "我", timing: "已逾期 2 小时", ageMinutes: 1080, state: "验证阻塞", scope: "mine", target: "candidate-region" },
  { id: "WB-103", title: "确认客单价退款订单口径", source: "RUN-240824-1432", object: "客单价 @9", objectType: "候选资产版本", action: "审核结构化差异", kind: "版本审核", priority: "P1", risk: "中风险", owner: "我", timing: "等待 12 分钟", ageMinutes: 12, state: "待审核", scope: "mine", target: "candidate-aov" },
  { id: "WB-102", title: "检查客户增长域运行异常", source: "PostgreSQL Analytics", object: "RUN-240823-1844", objectType: "接入自动化运行", action: "诊断字段变化", kind: "运行诊断", priority: "P1", risk: "中风险", owner: "我", timing: "今天到期", ageMinutes: 244, state: "待诊断", scope: "mine", target: "source-run" },
  { id: "WB-101", title: "补充净收入财务口径证据", source: "RUN-240824-1432", object: "净收入 @13", objectType: "候选资产版本", action: "等待财务负责人审核", kind: "版本审核", priority: "P2", risk: "低风险", owner: "林悦", timing: "等待 1 小时", ageMinutes: 61, state: "等待他人", scope: "created", target: "candidate-revenue" },
];

function OverviewView({ onOpenTask, discoveryState, focusSearchRequestEpoch }: { onOpenTask: (task: WorkbenchTask) => void; discoveryState: "ready" | "running" | "complete"; focusSearchRequestEpoch: number }) {
  const [taskScope, setTaskScope] = useState<WorkbenchScope>("mine");
  const [taskKind, setTaskKind] = useState<"全部类型" | WorkbenchTaskKind>("全部类型");
  const [taskRisk, setTaskRisk] = useState<"全部风险" | WorkbenchRisk>("全部风险");
  const [taskSort, setTaskSort] = useState<"风险优先" | "等待最久">("风险优先");
  const [taskSearch, setTaskSearch] = useState("");
  const taskSearchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (focusSearchRequestEpoch > 0) taskSearchRef.current?.focus();
  }, [focusSearchRequestEpoch]);

  const normalizedTaskSearch = taskSearch.trim().toLowerCase();
  const visibleTasks = workbenchTasks
    .filter((task) => taskScope === "team" || task.scope === taskScope)
    .filter((task) => taskKind === "全部类型" || task.kind === taskKind)
    .filter((task) => taskRisk === "全部风险" || task.risk === taskRisk)
    .filter((task) => !normalizedTaskSearch || `${task.title} ${task.id} ${task.source} ${task.object} ${task.action} ${task.owner}`.toLowerCase().includes(normalizedTaskSearch))
    .sort((left, right) => taskSort === "等待最久" ? right.ageMinutes - left.ageMinutes : ({ P0: 0, P1: 1, P2: 2 }[left.priority] - { P0: 0, P1: 1, P2: 2 }[right.priority]) || right.ageMinutes - left.ageMinutes);
  const mineCount = workbenchTasks.filter((task) => task.scope === "mine").length;
  const createdCount = workbenchTasks.filter((task) => task.scope === "created").length;

  return (
    <section className="view view-overview">
      <div className="workbench-commandbar">
        <div className="segment-control" aria-label="待办范围">
          <button type="button" aria-pressed={taskScope === "mine"} onClick={() => setTaskScope("mine")}>待我处理 <span>{mineCount}</span></button>
          <button type="button" aria-pressed={taskScope === "created"} onClick={() => setTaskScope("created")}>我发起 <span>{createdCount}</span></button>
          <button type="button" aria-pressed={taskScope === "team"} onClick={() => setTaskScope("team")}>团队 <span>{workbenchTasks.length}</span></button>
        </div>
        <label className="workbench-search"><Search size={15} /><input ref={taskSearchRef} type="search" aria-label="搜索待办" placeholder="搜索工作项、资产或运行" value={taskSearch} onChange={(event) => setTaskSearch(event.target.value)} /></label>
        <label className="workbench-select"><span>类型</span><select aria-label="筛选待办类型" value={taskKind} onChange={(event) => setTaskKind(event.target.value as "全部类型" | WorkbenchTaskKind)}><option>全部类型</option><option>版本审核</option><option>运行诊断</option><option>迁移确认</option></select></label>
        <label className="workbench-select"><span>风险</span><select aria-label="筛选待办风险" value={taskRisk} onChange={(event) => setTaskRisk(event.target.value as "全部风险" | WorkbenchRisk)}><option>全部风险</option><option>高风险</option><option>中风险</option><option>低风险</option></select></label>
      </div>
      <div className="workbench-queue-meta"><span>显示 <strong>{visibleTasks.length}</strong> / {taskScope === "team" ? workbenchTasks.length : taskScope === "mine" ? mineCount : createdCount} 项</span><label>排序<select aria-label="待办排序" value={taskSort} onChange={(event) => setTaskSort(event.target.value as "风险优先" | "等待最久")}><option>风险优先</option><option>等待最久</option></select></label></div>
      <section className="workbench-task-table" aria-label="工作台待办队列">
        <div className="workbench-task-head" aria-hidden="true"><span>工作项</span><span>关联对象</span><span>需要的动作</span><span>风险</span><span>负责人 / 时限</span><span>状态</span><span /></div>
        <div className="workbench-task-body">
          {visibleTasks.map((task) => <button className={`workbench-task-row${task.priority === "P0" ? " workbench-task-row-critical" : ""}`} type="button" key={task.id} onClick={() => onOpenTask(task)} aria-label={`打开待办 ${task.title}`}>
            <span className="task-primary"><strong>{task.title}</strong><small className="task-source"><code>{task.id}</code> · {task.source}</small><small className="task-compact-action">{task.action}</small></span>
            <span className="task-cell"><strong>{task.object}</strong><small>{task.objectType}</small></span>
            <span className="task-cell"><strong>{task.action}</strong><small>{task.kind}</small></span>
            <span><span className={`task-risk task-risk-${task.priority.toLowerCase()}`}>{task.priority} · {task.risk.replace("风险", "")}</span></span>
            <span className="task-cell task-owner"><strong>{task.owner}</strong><small>{task.timing}</small></span>
            <span><StatusBadge tone={task.state === "验证阻塞" ? "danger" : task.state === "等待他人" ? "neutral" : "warning"}>{discoveryState === "running" && task.target === "source-run" ? "诊断中" : task.state}</StatusBadge></span>
            <ChevronRight size={15} />
          </button>)}
          {visibleTasks.length === 0 && <div className="workbench-empty"><ListFilter size={20} /><strong>没有匹配的待办</strong><span>调整范围、类型、风险或搜索条件。</span><button type="button" onClick={() => { setTaskScope("mine"); setTaskKind("全部类型"); setTaskRisk("全部风险"); setTaskSearch(""); }}>清除筛选</button></div>}
        </div>
      </section>
    </section>
  );
}

const memberDirectory = [
  { id: "EMP-10001", name: "林悦", email: "yue.lin@semlia.example", department: "收入分析", title: "语义产品经理" },
  { id: "EMP-10002", name: "陈嘉", email: "jia.chen@semlia.example", department: "客户运营", title: "业务分析师" },
  { id: "EMP-10003", name: "周澄", email: "cheng.zhou@semlia.example", department: "数据平台", title: "数据工程师" },
  { id: "EMP-10004", name: "宋宁", email: "ning.song@semlia.example", department: "电商经营", title: "经营分析师" },
  { id: "EMP-10005", name: "许言", email: "yan.xu@semlia.example", department: "语义平台", title: "本体工程师" },
  { id: "EMP-10006", name: "魏然", email: "ran.wei@semlia.example", department: "客户运营", title: "增长策略专家" },
  { id: "EMP-10007", name: "顾清", email: "qing.gu@semlia.example", department: "财务分析", title: "财务分析师" },
  { id: "EMP-10008", name: "沈舟", email: "zhou.shen@semlia.example", department: "数据平台", title: "平台工程师" },
  { id: "EMP-10009", name: "叶岚", email: "lan.ye@semlia.example", department: "电商经营", title: "指标负责人" },
  { id: "EMP-10010", name: "唐可", email: "ke.tang@semlia.example", department: "语义平台", title: "知识运营" },
  { id: "EMP-10011", name: "贺川", email: "chuan.he@semlia.example", department: "收入分析", title: "高级分析师" },
  { id: "EMP-10012", name: "陆遥", email: "yao.lu@semlia.example", department: "客户运营", title: "用户研究员" },
  { id: "EMP-10013", name: "方衡", email: "heng.fang@semlia.example", department: "数据平台", title: "数据架构师" },
  { id: "EMP-10014", name: "韩知", email: "zhi.han@semlia.example", department: "财务分析", title: "经营规划" },
  { id: "EMP-10015", name: "余青", email: "qing.yu@semlia.example", department: "语义平台", title: "语义工程师" },
  { id: "EMP-10016", name: "姜禾", email: "he.jiang@semlia.example", department: "电商经营", title: "业务运营" },
  { id: "EMP-10017", name: "程屿", email: "yu.cheng@semlia.example", department: "收入分析", title: "数据科学家" },
  { id: "EMP-10018", name: "邵真", email: "zhen.shao@semlia.example", department: "客户运营", title: "增长分析师" },
  { id: "EMP-10019", name: "罗一", email: "yi.luo@semlia.example", department: "数据平台", title: "可靠性工程师" },
  { id: "EMP-10020", name: "梁简", email: "jian.liang@semlia.example", department: "语义平台", title: "质量工程师" },
  { id: "EMP-10021", name: "秦舒", email: "shu.qin@semlia.example", department: "财务分析", title: "财务数据专员" },
  { id: "EMP-10022", name: "夏安", email: "an.xia@semlia.example", department: "电商经营", title: "商品分析师" },
  { id: "EMP-10023", name: "白璟", email: "jing.bai@semlia.example", department: "收入分析", title: "商业分析师" },
  { id: "EMP-10024", name: "吴桐", email: "tong.wu@semlia.example", department: "语义平台", title: "知识工程师" },
] as const;

function MembersSettingsView() {
  const [searchQuery, setSearchQuery] = useState("");
  const normalizedQuery = searchQuery.trim().toLowerCase();
  const visibleMembers = memberDirectory.filter((member) => !normalizedQuery || `${member.id} ${member.name} ${member.email} ${member.department} ${member.title}`.toLowerCase().includes(normalizedQuery));

  return (
    <section className="view settings-view member-directory-view" aria-label="成员目录">
      <div className="member-directory-toolbar">
        <span><strong>{visibleMembers.length}</strong><small>{searchQuery ? `匹配成员 · 共 ${memberDirectory.length} 名` : "成员"}</small></span>
        <label><Search size={14} /><input type="search" aria-label="搜索成员" placeholder="搜索员工 ID、姓名或邮箱" value={searchQuery} onChange={(event) => setSearchQuery(event.target.value)} />{searchQuery && <button type="button" aria-label="清除成员搜索" title="清除搜索" onClick={() => setSearchQuery("")}><X size={13} /></button>}</label>
      </div>
      <section className="member-directory-table" aria-label="成员列表">
        <div className="member-directory-head" role="row"><span>员工 ID</span><span>姓名</span><span>邮箱</span><span>部门</span><span>职位</span><span>授权角色</span><span>状态</span></div>
        <div className="member-directory-body">
          {visibleMembers.map((member) => {
            const role = member.id === "EMP-10001" ? "Workspace Admin" : member.id === "EMP-10009" ? "Reviewer" : "未分配";
            const suspended = member.id === "EMP-10008";
            return <div className="member-directory-row" role="row" key={member.id}><code>{member.id}</code><strong>{member.name}</strong><span className="member-email">{member.email}</span><span>{member.department}</span><span>{member.title}</span><span className={role === "未分配" ? "member-role member-role-empty" : "member-role"}>{role}</span><span className={suspended ? "member-state member-state-suspended" : "member-state"}>{suspended ? "已停用" : "在职"}</span></div>;
          })}
          {visibleMembers.length === 0 && <div className="member-directory-empty"><Users size={19} /><strong>没有匹配成员</strong><span>尝试搜索其他员工 ID、姓名、邮箱或部门。</span><button type="button" onClick={() => setSearchQuery("")}>清除搜索</button></div>}
        </div>
      </section>
      <div className="member-directory-disclosure">业务职位与授权角色分别维护 · 原型授权数据仅用于演示</div>
    </section>
  );
}

function SettingsView({ focusIndex, rebuildRun, auditRunRequestId, onAuditRunRequestHandled, onStartEmbeddingRebuild, onOpenRebuildRun, onNotify }: { focusIndex: number; rebuildRun: EmbeddingRebuildRun | null; auditRunRequestId?: string; onAuditRunRequestHandled: () => void; onStartEmbeddingRebuild: (modelId: string) => void; onOpenRebuildRun: () => void; onNotify: (message: string) => void }) {
  if (focusIndex === 0) return <MembersSettingsView />;
  if (focusIndex === 1) return <AccessControlGate><AccessControlView onNotify={onNotify} /></AccessControlGate>;
  if (focusIndex === 2) return <ModelConfigurationView rebuildRun={rebuildRun} onStartEmbeddingRebuild={onStartEmbeddingRebuild} onOpenRebuildRun={onOpenRebuildRun} onNotify={onNotify} />;
  if (focusIndex === 3) return <IntegrationSettingsView onNotify={onNotify} />;
  return <AuditRuntimeView embeddingRebuildRun={rebuildRun} initialRunId={auditRunRequestId} onInitialRunHandled={onAuditRunRequestHandled} onNotify={onNotify} />;
}

function AssetOverview({ asset, onOpenTab }: { asset: Asset; onOpenTab: (tab: AssetTab) => void }) {
  const warnings = asset.readiness.filter((gate) => gate.state === "warning");
  const contractFields = assetContractFields(asset).slice(1);
  const profile = assetTypeProfileFor(asset);
  const typeRules = evaluateAssetTypeRules(asset);
  const typeBlockers = typeRules.filter((rule) => rule.severity === "blocker" && rule.state === "failed");
  const typeWarnings = typeRules.filter((rule) => rule.state === "warning");
  const totalBlockingCount = asset.qualitySnapshot.blockingCount + typeBlockers.length;
  const totalWarningCount = asset.qualitySnapshot.warningCount + typeWarnings.length;
  const isProductionReady = asset.deployment.state === "production" && asset.qualitySnapshot.state !== "blocked" && typeBlockers.length === 0;
  return (
    <div className="asset-overview-v2">
      <section className="asset-semantic-snapshot" aria-labelledby="asset-semantic-snapshot-title">
        <header className="asset-overview-section-heading"><div><span className="content-label">当前 revision 的核心语义</span><h2 id="asset-semantic-snapshot-title">{asset.type}摘要</h2></div><button type="button" onClick={() => onOpenTab("定义")}>查看权威定义<ChevronRight size={14} /></button></header>
        <p className="asset-type-focus">{profile.definitionFocus}</p>
        <div className="asset-formula-lead"><span>{assetContractFields(asset)[0].label}</span><code>{asset.revisionRecord.typeSpec.expression}</code></div>
        <dl className="asset-semantic-facts">{contractFields.map((field) => <div key={field.label}><dt>{field.label}</dt><dd>{field.value}</dd></div>)}</dl>
        <nav className="asset-overview-lenses" aria-label="资产专业视图">
          <button type="button" onClick={() => onOpenTab("本体关系")}><Network size={15} /><span><strong>本体关系</strong><small>{asset.relations.length} 条直接关系 · {asset.ontologyContext.revisionId}</small></span><ChevronRight size={13} /></button>
          {assetTabsFor(asset).includes("实现") && <button type="button" onClick={() => onOpenTab("实现")}><Link2 size={15} /><span><strong>实现</strong><small>{asset.bindings.length} 个绑定 · {asset.joinContracts.length} 个 JoinContract</small></span><ChevronRight size={13} /></button>}
          <button type="button" onClick={() => onOpenTab("可信度")}><ShieldCheck size={15} /><span><strong>可信度</strong><small>{asset.claims.length} 项主张 · {asset.validationRuns.length} 项验证</small></span><ChevronRight size={13} /></button>
          <button type="button" onClick={() => onOpenTab("交付与影响")}><Users size={15} /><span><strong>交付与影响</strong><small>{asset.consumerBindings.length} 个消费者 · {deploymentLabel(asset)}</small></span><ChevronRight size={13} /></button>
        </nav>
      </section>

      <aside className={`asset-current-assessment ${isProductionReady ? "assessment-ready" : "assessment-attention"}`} aria-label="生产可用判断">
        <header><span className="content-label">生产可用判断</span><StatusBadge tone={isProductionReady ? "success" : "warning"}>{isProductionReady ? "可稳定消费" : "存在阻断"}</StatusBadge></header>
        <div className="asset-assessment-score"><span>{isProductionReady ? <BadgeCheck size={22} /> : <AlertTriangle size={22} />}</span><div><strong>{isProductionReady ? "当前 revision 可用于生产" : totalBlockingCount > 0 ? `${totalBlockingCount} 项阻断需要处理` : `${warnings.length + typeWarnings.length} 项门禁需要确认`}</strong><small>{isProductionReady ? "部署、证据、验证与类型门禁均满足当前策略" : "可信判断由质量快照与类型 Profile 共同计算，不改变资产 revision"}</small></div></div>
        <dl className="asset-assessment-context"><div><dt>质量阻断</dt><dd>{asset.qualitySnapshot.blockingCount}</dd></div><div><dt>类型门禁</dt><dd>{typeBlockers.length}</dd></div><div><dt>非阻断提醒</dt><dd>{totalWarningCount}</dd></div><div><dt>生产消费者</dt><dd>{asset.consumerBindings.length} 个</dd></div></dl>
        {warnings.length > 0 || typeBlockers.length > 0 || typeWarnings.length > 0 ? <div className="asset-assessment-issues">{typeBlockers.map((rule) => <div key={rule.id}><span><AlertTriangle size={13} /></span><p><strong>{rule.label}</strong><small>{rule.detail}</small></p></div>)}{warnings.slice(0, Math.max(0, 3 - typeBlockers.length)).map((gate) => <div key={gate.id}><span><AlertTriangle size={13} /></span><p><strong>{gate.label}</strong><small>{gate.detail}</small></p></div>)}{typeBlockers.length === 0 && warnings.length === 0 && typeWarnings.slice(0, 2).map((rule) => <div key={rule.id}><span><AlertTriangle size={13} /></span><p><strong>{rule.label}</strong><small>{rule.detail}</small></p></div>)}</div> : <div className="asset-no-blockers"><CheckCircle2 size={15} /><span><strong>没有阻断门禁</strong><small>{asset.claims.length} 项字段主张、{asset.validationRuns.length} 项验证运行与 {typeRules.length} 项类型规则均可追溯</small></span></div>}
        <div className="asset-assessment-owners"><span>业务 owner <strong>{asset.owner}</strong></span><span>实现维护人 <strong>{asset.maintainer}</strong></span></div>
        <button className="asset-assessment-action" type="button" onClick={() => onOpenTab("可信度")}>查看可信度详情<ChevronRight size={14} /></button>
      </aside>
    </div>
  );
}

function AssetDefinition({ asset, onNotify, onStartRevision }: { asset: Asset; onNotify: (message: string) => void; onStartRevision: (request: KnowledgeRevisionRequest) => void }) {
  const contractFields = assetContractFields(asset);
  const priorRevision = `@${Math.max(asset.revisionRecord.sequence - 1, 0)}`;
  const profile = assetTypeProfileFor(asset);
  return (
    <div className="asset-spec-layout">
      <section className="asset-type-template" aria-label={`${asset.type}内容模板`}>
        <div><span className="content-label">{asset.type}内容模板</span><h3>{profile.definitionTitle}</h3><p>{profile.definitionFocus}</p></div>
        <dl><div><dt>本体重点</dt><dd>{profile.ontologyFocus}</dd></div><div><dt>可信度重点</dt><dd>{profile.trustFocus}</dd></div><div><dt>交付重点</dt><dd>{profile.deliveryFocus}</dd></div></dl>
        <div className="asset-required-fields"><span>必填契约</span>{profile.requiredFields.map((field) => <strong key={field}>{field}</strong>)}</div>
      </section>
      <section className="asset-wiki-authority" aria-labelledby="asset-wiki-title">
        <header className="asset-wiki-heading"><div><span className="content-label">LLM Wiki</span><h3 id="asset-wiki-title">权威语义页</h3></div><button className="secondary-button" type="button" onClick={() => onStartRevision({ assetId: asset.id, fieldPath: "definition.boundary", origin: "definition" })}><GitPullRequestArrow size={15} />提出修订</button></header>
        <div className="asset-wiki-lead"><span><Sparkles size={17} /></span><div><strong>面向人和 AI 的同一份受治理含义</strong><p>{asset.wikiContext.canonicalSummary}</p></div></div>
        <div className="asset-wiki-context-grid">
          <div><span>AI 检索词</span><p>{asset.wikiContext.retrievalTerms.join(" · ")}</p></div>
          <div><span>消歧规则</span><p>{asset.wikiContext.disambiguationRules.join(" ")}</p></div>
          <div><span>可回答问题</span><p>{asset.wikiContext.groundedQuestions.length > 0 ? asset.wikiContext.groundedQuestions.join("；") : "当前 revision 尚未声明典型问题。"}</p></div>
        </div>
        <footer><span><BookOpenCheck size={14} />上下文由 <code>{asset.wikiContext.assetRevisionId}</code> 编译，更新于 {asset.wikiContext.compiledAt}</span><button type="button" onClick={() => onNotify(`正在比较 ${asset.name} ${priorRevision} → ${asset.revision} 的字段级差异。`)}>比较 {priorRevision} → {asset.revision}<ChevronRight size={13} /></button></footer>
      </section>
      <section className="asset-spec-main">
        <div className="asset-spec-heading"><div><span className="content-label">类型专属语义</span><h3>{asset.type}契约</h3></div><code>{asset.revisionRecord.schemaVersion}</code></div>
        <div className="asset-definition-statement"><span>规范业务定义 · {asset.revision}<button type="button" aria-label="修订业务定义" onClick={() => onStartRevision({ assetId: asset.id, fieldPath: "definition.boundary", origin: "definition" })}>修订此主张<ChevronRight size={12} /></button></span><p>{asset.revisionRecord.definition}</p></div>
        <dl className="asset-contract-field-list">{contractFields.map((field) => <div key={field.label}><dt>{field.label}<button type="button" aria-label={`修订${field.label}`} onClick={() => onStartRevision({ assetId: asset.id, fieldPath: field.fieldPath, origin: "definition" })}>修订<ChevronRight size={11} /></button></dt><dd className={field.code ? "asset-contract-code" : undefined}>{field.code ? <code>{field.value}</code> : field.value}</dd></div>)}<div><dt>可搜索别名</dt><dd>{asset.aliases.length > 0 ? asset.aliases.join(" · ") : "未设置"}</dd></div></dl>
      </section>
      <section className="asset-boundary-section" aria-label="定义边界与示例">
        <div><span className="content-label">口径包含<button type="button" aria-label="修订口径包含" onClick={() => onStartRevision({ assetId: asset.id, fieldPath: "definition.includes", origin: "definition" })}>修订</button></span><ul>{asset.includes.map((item) => <li key={item}><Check size={14} />{item}</li>)}</ul></div>
        <div><span className="content-label">明确排除<button type="button" aria-label="修订明确排除" onClick={() => onStartRevision({ assetId: asset.id, fieldPath: "definition.excludes", origin: "definition" })}>修订</button></span><ul>{asset.excludes.map((item) => <li key={item}><X size={14} />{item}</li>)}</ul></div>
        <div><span className="content-label">典型用法</span><ul>{asset.examples.map((item) => <li key={item}><MessageSquareText size={14} />{item}</li>)}</ul></div>
      </section>
    </div>
  );
}

function AssetRelations({ asset, focusedObjectId, onNotify }: { asset: Asset; focusedObjectId?: string; onNotify: (message: string) => void }) {
  const profile = assetTypeProfileFor(asset);
  return (
    <div className="asset-relations-layout">
      <section className="asset-ontology-context" aria-labelledby="asset-ontology-title">
        <header className="asset-ontology-heading"><div><span className="content-label">本体上下文</span><h3 id="asset-ontology-title">{asset.domain}本体</h3></div><span className={`asset-ontology-consistency asset-ontology-${asset.ontologyContext.consistencyState}`}><CheckCircle2 size={13} />{asset.ontologyContext.consistencyState === "consistent" ? "一致性通过" : "存在待确认关系"}</span></header>
        <p className="asset-view-focus">{profile.ontologyFocus}</p>
        <div className="asset-ontology-path"><span>领域路径</span>{asset.ontologyContext.domainPath.map((item, index) => <span key={item}><strong>{item}</strong>{index < asset.ontologyContext.domainPath.length - 1 && <ChevronRight size={12} />}</span>)}<span><strong>{asset.name}</strong></span></div>
        {asset.relations.length > 0 ? <SemanticGraph mode="lineage" assetName={asset.name} assetType={asset.type} assetRevision={asset.revision} ontologyRevision={asset.ontologyContext.revisionId} relations={asset.relations} upstream={asset.upstream} downstream={asset.downstream} consumerName={asset.consumers[0]?.name ?? "尚未绑定"} /> : <AssetEmptyState config={profile.emptyStates.relations} icon={Network} assetName={asset.name} onAction={onNotify} />}
        <dl className="asset-ontology-summary"><div><dt>上位概念</dt><dd>{asset.ontologyContext.parentConcepts.join(" · ")}</dd></div><div><dt>直接关系</dt><dd>{asset.relations.length} 条</dd></div><div><dt>关系类型约束</dt><dd>{asset.ontologyContext.relationConstraints.length} 项</dd></div><div><dt>发布范围</dt><dd>{asset.ontologyContext.publishedIn ?? "候选本体"}</dd></div></dl>
      </section>
      {asset.relations.length > 0 && <section className="asset-relation-list" aria-labelledby="asset-relations-title">
        <header><div><span className="content-label">可审计明细</span><h3 id="asset-relations-title">类型化关系</h3></div><span>{asset.relations.length} 条当前 revision 关系</span></header>
        <div className="asset-relation-table-head" aria-hidden="true"><span>方向</span><span>关系</span><span>目标资产</span><span>证据与版本</span></div>
        <div className="asset-relation-cards">
          {asset.relations.map((relation) => <article key={relation.id} className={focusedObjectId === relation.id ? "asset-object-focused" : undefined} aria-current={focusedObjectId === relation.id ? "true" : undefined}><div className="relation-source"><span className="relation-type-mark"><ArrowRight size={15} /></span><span><strong>{relation.direction === "outgoing" ? "出向" : "入向"}</strong><small>当前资产为{relation.direction === "outgoing" ? "来源" : "目标"}</small></span></div><div className="relation-predicate"><strong>{relationLabel(relation.type)}</strong><code>{relation.type} · {relation.id}</code></div><div className="relation-target"><strong>{relation.targetName}</strong><small><code>{relation.targetId}</code></small></div><div className="relation-evidence"><span><strong>{relation.evidence}</strong><small>{relation.release}</small></span><StatusBadge tone={relation.release === "draft" ? "warning" : "info"}>{relation.release === "draft" ? "草稿" : "已发布"}</StatusBadge></div></article>)}
        </div>
      </section>}
    </div>
  );
}

function AssetMapping({ asset, focusedObjectId, onNotify }: { asset: Asset; focusedObjectId?: string; onNotify: (message: string) => void }) {
  const profile = assetTypeProfileFor(asset);
  return (
    <div className="asset-mapping-layout">
      <section aria-labelledby="bindings-title">
        <header className="asset-section-heading"><div><span className="content-label">物理层</span><h3 id="bindings-title">PhysicalBinding</h3></div><span>{asset.bindings.length} 项版本化实现</span></header>
        {asset.bindings.length > 0 ? <div className="asset-object-table asset-binding-table"><div className="asset-object-table-head" aria-hidden="true"><span>数据集与绑定</span><span>来源与适配器</span><span>执行语义</span><span>表达式</span><span>状态</span></div>{asset.bindings.map((binding) => <article key={binding.id} className={focusedObjectId === binding.id ? "asset-object-focused" : undefined} aria-current={focusedObjectId === binding.id ? "true" : undefined}><div className="asset-object-primary"><span><Link2 size={15} /></span><span><strong>{binding.dataset}</strong><code>{binding.id}</code></span></div><div><code>{binding.sourceRevision}</code><small>{binding.adapter}</small></div><div><strong>{binding.grain}</strong><small>{binding.defaultTime || "不适用"}</small></div><code className="asset-object-expression" title={binding.expression}>{binding.expression}</code><StatusBadge tone={binding.state === "已验证" ? "success" : "warning"}>{binding.state}</StatusBadge></article>)}</div> : <AssetEmptyState config={profile.emptyStates.bindings} icon={Link2} assetName={asset.name} onAction={onNotify} />}
      </section>
      <section aria-labelledby="join-contracts-title">
        <header className="asset-section-heading"><div><span className="content-label">连接约束</span><h3 id="join-contracts-title">JoinContract</h3></div><span>{asset.joinContracts.length} 项生产契约</span></header>
        {asset.joinContracts.length > 0 ? <div className="asset-object-table asset-join-table"><div className="asset-object-table-head" aria-hidden="true"><span>目标与契约</span><span>连接键</span><span>基数</span><span>粒度与防护</span><span>状态</span></div>{asset.joinContracts.map((contract) => <article key={contract.id} className={focusedObjectId === contract.id ? "asset-object-focused" : undefined} aria-current={focusedObjectId === contract.id ? "true" : undefined}><div className="asset-object-primary"><span><GitPullRequestArrow size={15} /></span><span><strong>{contract.target}</strong><code>{contract.id}</code></span></div><code className="asset-object-expression" title={contract.keys}>{contract.keys}</code><div><strong>{contract.cardinality}</strong><small>{contract.grainImpact}</small></div><div><strong>{contract.guardrail}</strong><small>生产连接防护</small></div><StatusBadge tone={contract.state === "已发布" ? "success" : "warning"}>{contract.state}</StatusBadge></article>)}</div> : <AssetEmptyState config={profile.emptyStates.joins} icon={Network} assetName={asset.name} onAction={onNotify} />}
      </section>
    </div>
  );
}

function AssetOntology({ asset, focusedObject, onNotify }: { asset: Asset; focusedObject?: KnowledgeCatalogItem; onNotify: (message: string) => void }) {
  return <div className="asset-ontology-view"><AssetRelations asset={asset} focusedObjectId={focusedObject?.key} onNotify={onNotify} /></div>;
}

function AssetExecution({ asset, focusedObject, onNotify }: { asset: Asset; focusedObject?: KnowledgeCatalogItem; onNotify: (message: string) => void }) {
  const requiresAttention = asset.bindings.some((binding) => binding.state === "需重验") || asset.joinContracts.some((contract) => contract.state === "需关注");
  const profile = assetTypeProfileFor(asset);
  return (
    <div className="asset-implementation-view">
      <section className="asset-implementation-bar" aria-label="实现摘要"><div><span className="asset-implementation-state"><ShieldCheck size={16} /><strong>{requiresAttention ? "实现需要重验" : asset.bindings.length > 0 ? "当前实现可执行" : "实现尚未建立"}</strong></span><span>{profile.implementationFocus}</span></div><dl><div><dt>PhysicalBinding</dt><dd>{asset.bindings.length}</dd></div><div><dt>JoinContract</dt><dd>{asset.joinContracts.length}</dd></div><div><dt>执行适配器</dt><dd>{new Set(asset.bindings.map((binding) => binding.adapter)).size}</dd></div></dl></section>
      <div className="asset-implementation-sections"><AssetMapping asset={asset} focusedObjectId={focusedObject?.key} onNotify={onNotify} /></div>
    </div>
  );
}

function AssetEvidence({ asset, onNotify, onStartRevision }: { asset: Asset; onNotify: (message: string) => void; onStartRevision: (request: KnowledgeRevisionRequest) => void }) {
  const reproducibleRuns = asset.validationRuns.filter((run) => run.reproducible).length;
  const profile = assetTypeProfileFor(asset);
  const typeRules = evaluateAssetTypeRules(asset);
  const typeBlockingCount = typeRules.filter((rule) => rule.severity === "blocker" && rule.state === "failed").length;
  const typeWarningCount = typeRules.filter((rule) => rule.state === "warning").length;
  const totalBlockingCount = asset.qualitySnapshot.blockingCount + typeBlockingCount;
  const totalWarningCount = asset.qualitySnapshot.warningCount + typeWarningCount;
  const healthState = totalBlockingCount > 0 ? "blocked" : totalWarningCount > 0 ? "warning" : "healthy";
  const healthLabel = healthState === "healthy" ? "可信度健康" : healthState === "blocked" ? "存在阻断" : "需要关注";
  return (
    <div className="asset-assurance-layout">
      <section className="asset-trust-summary" aria-label="可信度摘要">
        <div className="asset-trust-verdict"><span><ShieldCheck size={18} /></span><span><strong>{healthLabel}</strong><small>{profile.trustFocus}</small></span><StatusBadge tone={healthState === "healthy" ? "success" : healthState === "blocked" ? "danger" : "warning"}>{totalBlockingCount > 0 ? `${totalBlockingCount} 项阻断` : `${totalWarningCount} 项提醒`}</StatusBadge></div>
        <dl><div><dt>证据覆盖</dt><dd>{asset.evidenceLinks.length} / {asset.claims.length} 项主张</dd></div><div><dt>可复现验证</dt><dd>{reproducibleRuns} / {asset.validationRuns.length} 项运行</dd></div><div><dt>适用策略</dt><dd><code>{asset.qualitySnapshot.policyVersion}</code></dd></div><div><dt>评估时间</dt><dd>{asset.qualitySnapshot.evaluatedAt}</dd></div></dl>
      </section>
      <section aria-labelledby="asset-type-rules-title">
        <header className="asset-section-heading"><div><span className="content-label">{asset.type}发布门禁</span><h3 id="asset-type-rules-title">类型验证规则</h3></div><span>{typeRules.filter((rule) => rule.state === "passed").length} / {typeRules.length} 通过</span></header>
        <div className="asset-type-rule-table" role="table" aria-label={`${asset.type}验证规则`}><div className="asset-type-rule-head" role="row"><span role="columnheader">规则</span><span role="columnheader">级别</span><span role="columnheader">结果</span><span role="columnheader">说明与修复</span></div>{typeRules.map((rule) => <article role="row" key={rule.id} className={`asset-type-rule-${rule.state}`}><div role="cell"><strong>{rule.label}</strong><code>{rule.id}</code></div><span role="cell"><StatusBadge tone={rule.severity === "blocker" ? "neutral" : "info"}>{rule.severity === "blocker" ? "发布门禁" : "质量提醒"}</StatusBadge></span><span role="cell"><StatusBadge tone={rule.state === "passed" ? "success" : rule.state === "failed" ? "danger" : rule.state === "warning" ? "warning" : "neutral"}>{rule.state === "passed" ? "通过" : rule.state === "failed" ? "失败" : rule.state === "warning" ? "需关注" : "不适用"}</StatusBadge></span><div role="cell"><strong>{rule.detail}</strong><small>{rule.state === "passed" ? rule.description : rule.remediation}</small></div></article>)}</div>
      </section>
      <section aria-labelledby="evidence-title">
        <header className="asset-section-heading"><div><span className="content-label">字段级主张</span><h3 id="evidence-title">主张与证据</h3></div><span>{asset.claims.length} 项主张 · {asset.evidenceArtifacts.length} 项证据</span></header>
        {asset.evidence.length === 0 ? <AssetEmptyState config={profile.emptyStates.evidence} icon={BookOpenCheck} assetName={asset.name} onAction={onNotify} /> : <div className="asset-evidence-table asset-evidence-table-actionable" role="table" aria-label="字段主张与证据"><div className="asset-evidence-table-head" role="row"><span role="columnheader">字段主张</span><span role="columnheader">证据</span><span role="columnheader">权威类型</span><span role="columnheader">来源 revision</span><span role="columnheader">状态</span><span role="columnheader" className="visually-hidden">操作</span></div>{asset.evidence.map((evidence, index) => { const claim = asset.claims[index]; const artifact = asset.evidenceArtifacts[index]; const link = asset.evidenceLinks[index]; return <article role="row" key={evidence.id}><div role="cell"><strong>{claim?.label ?? evidence.supports}</strong><small><code>{claim?.fieldPath ?? "definition"}</code></small></div><div role="cell" className="asset-evidence-primary"><span><FileCheck2 size={15} /></span><span><strong>{evidence.label}</strong><code>{evidence.id} · {link?.strength === "primary" ? "主要证据" : "佐证"}</code></span></div><code role="cell">{evidence.authority}</code><div role="cell"><strong>{artifact?.sourceRevision ?? evidence.source}</strong><small>{artifact?.observedAt ?? evidence.verifiedAt}</small></div><span role="cell"><StatusBadge tone={evidence.state === "已验证" ? "success" : "warning"}>{link?.polarity === "contradicts" ? "存在冲突" : evidence.state}</StatusBadge></span><button role="cell" className="asset-claim-revise" type="button" aria-label={`修订主张 ${claim?.label ?? evidence.supports}`} onClick={() => onStartRevision({ assetId: asset.id, fieldPath: claim?.fieldPath ?? "definition.boundary", claimId: claim?.claimId, origin: "evidence", context: `核对证据 ${evidence.id} 支持的知识主张。` })}>修订<ChevronRight size={12} /></button></article>; })}</div>}
      </section>
      <section aria-labelledby="asset-validation-title">
        <header className="asset-section-heading"><div><span className="content-label">可复现检查</span><h3 id="asset-validation-title">最近验证运行</h3></div><span>{asset.validationRuns.length} 项运行</span></header>
        <div className="asset-validation-table" role="table" aria-label="最近验证运行"><div className="asset-validation-table-head" role="row"><span role="columnheader">检查</span><span role="columnheader">结果</span><span role="columnheader">运行与说明</span></div>{asset.validations.map((validation, index) => { const run = asset.validationRuns[index]; return <article role="row" key={validation.name} className={`asset-validation-${validation.state}`}><div role="cell"><span>{validationIcon(validation.state)}</span><strong>{validationLabel(validation.name)}</strong></div><span role="cell"><StatusBadge tone={validation.state === "passed" ? "success" : validation.state === "warning" ? "warning" : "danger"}>{validation.state === "passed" ? "通过" : validation.state === "warning" ? "需关注" : "失败"}</StatusBadge></span><small role="cell"><strong>{run?.runId}</strong>{validation.detail} · validator {run?.validatorVersion}</small></article>; })}</div>
      </section>
    </div>
  );
}

function AssetUsage({ asset, onNotify }: { asset: Asset; onNotify: (message: string) => void }) {
  const latestConsumerResolution = asset.consumers[0]?.lastResolved ?? "尚无解析记录";
  const profile = assetTypeProfileFor(asset);
  return (
    <div className="asset-usage-layout">
      <section className="asset-delivery-summary" aria-labelledby="asset-delivery-title">
        <header className="asset-section-heading"><div><span className="content-label">稳定交付面</span><h3 id="asset-delivery-title">交付地址与消费状态</h3></div><StatusBadge tone={asset.deployment.state === "production" ? "success" : "warning"}>{deploymentLabel(asset)}</StatusBadge></header>
        <p className="asset-view-focus">{profile.deliveryFocus}</p>
        <div className="asset-delivery-endpoints"><div><span>REST</span><code title={`/v1/semantic/assets/${asset.identity.key}`}>/v1/semantic/assets/{asset.identity.key}</code></div><div><span>MCP</span><code>semlia.resolve_asset</code></div><div><span>CLI</span><code title={`semlia query ${asset.identity.key}`}>semlia query {asset.identity.key}</code></div><div><span>SDK</span><code title={`client.assets.resolve("${asset.identity.key}")`}>client.assets.resolve(...)</code></div></div>
        <dl className="asset-delivery-facts"><div><dt>生产消费者</dt><dd>{asset.consumerBindings.length} 个</dd></div><div><dt>最近解析</dt><dd>{latestConsumerResolution}</dd></div><div><dt>默认版本约束</dt><dd><code>{asset.consumerBindings[0]?.versionConstraint ?? asset.revision}</code></dd></div><div><dt>兼容性</dt><dd>{compatibilityLabel(asset.consumerBindings[0]?.compatibility ?? "not_evaluated")}</dd></div></dl>
      </section>
      <section className="asset-release-contract" aria-labelledby="asset-release-title"><header className="asset-section-heading"><div><span className="content-label">环境部署</span><h3 id="asset-release-title">当前生产指针</h3></div><StatusBadge tone={asset.deployment.state === "production" ? "success" : "warning"}>{deploymentLabel(asset)}</StatusBadge></header><dl><div><dt>稳定资产 ID</dt><dd><code title={asset.identity.assetId}>{asset.identity.assetId}</code></dd></div><div><dt>消费 key</dt><dd><code title={asset.identity.key}>{asset.identity.key}</code></dd></div><div><dt>已部署 revision</dt><dd><code>{asset.deployment.revisionId ?? "未部署"}</code></dd></div><div><dt>发布批次</dt><dd><code title={asset.deployment.releaseId}>{asset.deployment.releaseId ?? "尚未发布"}</code></dd></div></dl><p><ShieldCheck size={15} />生产指针只引用不可变 revision；release 记录发布来源，二者不共享版本身份。</p></section>
      <section aria-labelledby="asset-consumers-title"><header className="asset-section-heading"><div><span className="content-label">已注册使用方</span><h3 id="asset-consumers-title">消费者绑定</h3></div><span>{asset.consumerBindings.length} 个契约</span></header>{asset.consumers.length === 0 ? <AssetEmptyState config={profile.emptyStates.consumers} icon={Users} assetName={asset.name} onAction={onNotify} /> : <div className="asset-consumer-table" role="table" aria-label="消费者绑定"><div className="asset-consumer-table-head" role="row"><span role="columnheader">使用方</span><span role="columnheader">接口类型</span><span role="columnheader">版本约束</span><span role="columnheader">最近解析</span><span role="columnheader">兼容性</span></div>{asset.consumers.map((consumer, index) => { const binding = asset.consumerBindings[index]; return <article role="row" key={binding?.bindingId ?? consumer.name}><div role="cell" className="asset-consumer-primary"><span><Box size={15} /></span><strong>{consumer.name}</strong></div><span role="cell">{consumer.kind}</span><code role="cell">{binding?.versionConstraint ?? consumer.binding}</code><span role="cell">{consumer.lastResolved}</span><span role="cell"><StatusBadge tone={binding?.compatibility === "compatible" ? "success" : binding?.compatibility === "breaking" ? "danger" : "warning"}>{compatibilityLabel(binding?.compatibility ?? "not_evaluated")}</StatusBadge></span></article>; })}</div>}</section>
    </div>
  );
}

const knowledgeRevisionLaunchTargets = [
  { label: "业务定义", fieldPath: "definition.boundary" },
  { label: "计算表达式", fieldPath: "spec.expression" },
  { label: "口径包含", fieldPath: "definition.includes" },
  { label: "明确排除", fieldPath: "definition.excludes" },
  { label: "消歧规则", fieldPath: "wiki.disambiguation" },
] as const;

function KnowledgeRevisionLauncher({ asset, active, onStart }: { asset: Asset; active: boolean; onStart: (request: KnowledgeRevisionRequest) => void }) {
  const [open, setOpen] = useState(false);
  const launcherRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const firstOptionRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    if (!open) return;
    const closeOnOutsidePointer = (event: PointerEvent) => {
      if (!launcherRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      setOpen(false);
      triggerRef.current?.focus();
    };
    document.addEventListener("pointerdown", closeOnOutsidePointer);
    document.addEventListener("keydown", closeOnEscape);
    window.requestAnimationFrame(() => firstOptionRef.current?.focus());
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsidePointer);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [open]);

  const targetContext = knowledgeRevisionLaunchTargets.map((target) => {
    const exactClaim = asset.claims.find((claim) => claim.fieldPath === target.fieldPath);
    const relatedClaim = target.fieldPath.startsWith("definition.")
      ? asset.claims.find((claim) => claim.fieldPath === "definition.boundary")
      : undefined;
    const claim = exactClaim ?? relatedClaim;
    const link = asset.evidenceLinks.find((item) => item.claimId === claim?.claimId);
    return { ...target, claim, evidenceId: link?.evidenceId };
  });

  return (
    <div className="asset-revision-launcher" ref={launcherRef}>
      <button ref={triggerRef} className={`primary-button asset-revision-trigger${active ? " asset-revision-trigger-active" : ""}`} type="button" aria-haspopup="dialog" aria-expanded={open} onClick={() => setOpen((current) => !current)}>
        <GitPullRequestArrow size={15} />修订知识<ChevronDown size={13} />
      </button>
      {open && <section className="asset-revision-menu" role="dialog" aria-label="选择知识修订对象">
        <header><div><span className="content-label">创建候选修订</span><h2>选择修订对象</h2></div><code>基于 {asset.revision}</code></header>
        <div className="asset-revision-options">
          {targetContext.map((target, index) => <button ref={index === 0 ? firstOptionRef : undefined} key={target.fieldPath} type="button" onClick={() => { onStart({ assetId: asset.id, fieldPath: target.fieldPath, claimId: target.claim?.claimId, origin: "definition" }); setOpen(false); }}>
            <span><strong>{target.label}</strong><code>{target.fieldPath}</code></span>
            <span><small>{target.claim ? `${target.claim.label} · ${target.evidenceId ?? "待补证据"}` : "尚无 Claim，将在候选版本中创建"}</small><ChevronRight size={14} /></span>
          </button>)}
        </div>
        <footer><ShieldCheck size={14} /><span>来源资料与已发布版本保持只读</span></footer>
      </section>}
    </div>
  );
}

function AssetsView({ selectedId, domainFilter, requestedTab, requestedDetail, requestedCatalogType, requestedRevision, focusSearchRequestEpoch, detailBackRequestEpoch, onSelect, onDetailChange, onNotify, onRevisionSubmit }: { selectedId: string; domainFilter: string; requestedTab: AssetTab; requestedDetail: boolean; requestedCatalogType: CatalogObjectType | "全部"; requestedRevision: KnowledgeRevisionRequest | null; focusSearchRequestEpoch: number; detailBackRequestEpoch: number; onSelect: (id: string) => void; onDetailChange: (open: boolean) => void; onNotify: (message: string) => void; onRevisionSubmit: (submission: KnowledgeRevisionSubmission) => void }) {
  const [query, setQuery] = useState("");
  const [type, setType] = useState<CatalogObjectType | "全部">(requestedCatalogType);
  const [assetStatus, setAssetStatus] = useState<Asset["status"] | "全部">("全部");
  const [domain, setDomain] = useState(domainFilter);
  const [sort, setSort] = useState<"attention" | "name" | "type">("attention");
  const [tab, setTab] = useState<AssetTab>(requestedTab);
  const [mode, setMode] = useState<"catalog" | "detail">(requestedDetail ? "detail" : "catalog");
  const [expandedAssetIds, setExpandedAssetIds] = useState<Set<string>>(() => new Set());
  const [focusedObjectId, setFocusedObjectId] = useState<string>();
  const [lastOpenedItemId, setLastOpenedItemId] = useState<string>();
  const [revisionRequest, setRevisionRequest] = useState<KnowledgeRevisionRequest | null>(requestedRevision);
  const [selectedDetailBackEpoch, setSelectedDetailBackEpoch] = useState(detailBackRequestEpoch);
  const catalogScrollPosition = useRef(0);
  const catalogSearchRef = useRef<HTMLInputElement>(null);
  const domains = useMemo(() => Array.from(new Set(assets.map((asset) => asset.domain))), []);
  const normalizedQuery = query.trim().toLowerCase();
  const isGovernedObjectType = type === "语义关系" || type === "物理绑定" || type === "JoinContract";
  const catalogGroups = useMemo(() => {
    const matchesText = (item: KnowledgeCatalogItem) => `${item.name} ${item.key} ${item.aliases.join(" ")} ${item.domain} ${item.scope} ${item.detail}`.toLowerCase().includes(normalizedQuery);
    const groups = assets.flatMap((asset) => {
      const assetItem = knowledgeCatalogItems.find((item) => item.id === asset.id)!;
      const children = knowledgeCatalogItems.filter((item) => item.assetId === asset.id && item.id !== asset.id);
      const typedChildren = isGovernedObjectType ? children.filter((item) => item.type === type) : children;
      const matchingChildren = normalizedQuery.length > 0 ? typedChildren.filter(matchesText) : typedChildren;
      const matchesAssetType = type === "全部" || isGovernedObjectType || asset.type === type;
      const matchesGovernedType = !isGovernedObjectType || typedChildren.length > 0;
      const matchesStatus = assetStatus === "全部" || asset.status === assetStatus;
      const matchesDomain = domain === "全部" || asset.domain === domain;
      const assetMatchesQuery = normalizedQuery.length === 0 || matchesText(assetItem);
      const matchesQuery = assetMatchesQuery || matchingChildren.length > 0;
      if (!matchesAssetType || !matchesGovernedType || !matchesStatus || !matchesDomain || !matchesQuery) return [];
      return [{ asset, assetItem, children, visibleChildren: normalizedQuery.length > 0 && !assetMatchesQuery ? matchingChildren : typedChildren }];
    });
    return groups.sort((left, right) => {
      if (sort === "name") return left.asset.name.localeCompare(right.asset.name, "zh-CN");
      if (sort === "type") return left.asset.type.localeCompare(right.asset.type, "zh-CN") || left.asset.name.localeCompare(right.asset.name, "zh-CN");
      const statusOrder: Record<Asset["status"], number> = { "需关注": 0, "草稿": 1, "已发布": 2 };
      const statusDelta = statusOrder[left.asset.status] - statusOrder[right.asset.status];
      if (statusDelta !== 0) return statusDelta;
      const readinessDelta = readinessSummary(right.asset).warnings - readinessSummary(left.asset).warnings;
      if (readinessDelta !== 0) return readinessDelta;
      return left.asset.name.localeCompare(right.asset.name, "zh-CN");
    });
  }, [assetStatus, domain, isGovernedObjectType, normalizedQuery, sort, type]);
  const visibleChildCount = catalogGroups.reduce((total, group) => total + group.visibleChildren.length, 0);
  const selected = assets.find((asset) => asset.id === selectedId) ?? assets[0];
  const focusedObject = focusedObjectId ? knowledgeCatalogItems.find((item) => item.key === focusedObjectId) : undefined;
  const hasFilters = normalizedQuery.length > 0 || type !== "全部" || assetStatus !== "全部" || domain !== "全部";

  useEffect(() => {
    if (focusSearchRequestEpoch > 0) catalogSearchRef.current?.focus();
  }, [focusSearchRequestEpoch]);

  const scrollWorkspaceTo = (top: number) => {
    window.requestAnimationFrame(() => {
      const workspace = document.querySelector<HTMLElement>(".workspace-canvas");
      if (!workspace) return;
      if (typeof workspace.scrollTo === "function") workspace.scrollTo({ top });
      else workspace.scrollTop = top;
    });
  };

  useEffect(() => {
    if (selectedDetailBackEpoch === detailBackRequestEpoch) return;
    window.requestAnimationFrame(() => {
      const workspace = document.querySelector<HTMLElement>(".workspace-canvas");
      if (!workspace) return;
      if (typeof workspace.scrollTo === "function") workspace.scrollTo({ top: catalogScrollPosition.current });
      else workspace.scrollTop = catalogScrollPosition.current;
    });
  }, [detailBackRequestEpoch, selectedDetailBackEpoch]);

  const openDetail = (item: KnowledgeCatalogItem) => {
    const workspace = document.querySelector<HTMLElement>(".workspace-canvas");
    catalogScrollPosition.current = workspace?.scrollTop ?? 0;
    setLastOpenedItemId(item.id);
    setSelectedDetailBackEpoch(detailBackRequestEpoch);
    setRevisionRequest(null);
    setFocusedObjectId(item.id === item.assetId ? undefined : item.key);
    onSelect(item.assetId);
    onDetailChange(true);
    setTab(item.openTab);
    setMode("detail");
    scrollWorkspaceTo(0);
  };

  const toggleAsset = (assetId: string) => {
    setExpandedAssetIds((current) => {
      const next = new Set(current);
      if (next.has(assetId)) next.delete(assetId);
      else next.add(assetId);
      return next;
    });
  };

  const activeMode = selectedDetailBackEpoch === detailBackRequestEpoch ? mode : "catalog";

  if (activeMode === "detail" && selected) {
    const openAssetTab = (nextTab: AssetTab) => { setTab(nextTab); setFocusedObjectId(undefined); scrollWorkspaceTo(0); };
    const visibleAssetTabs = assetTabsFor(selected);
    const activeTab = visibleAssetTabs.includes(tab) ? tab : "概览";
    return (
      <section className="view view-assets asset-detail-page">
        <section className="asset-detail" aria-label="语义资产详情">
          <header className="asset-detail-header">
            <div className="asset-header-main"><AssetTypeMark type={selected.identity.type} size={19} /><div className="asset-title-copy"><span className="panel-kicker">{selected.identity.type} · <code>{selected.identity.key}</code></span><h1>{selected.revisionRecord.name}</h1><p>{selected.revisionRecord.definition}</p></div><div className="asset-header-side"><div className="asset-header-state"><div><span>当前 revision</span><strong>{selected.revision}</strong></div><span className="asset-header-release"><StatusBadge tone={selected.deployment.state === "production" ? "success" : "warning"}>{deploymentLabel(selected)}</StatusBadge><span className={`asset-health-state asset-health-${selected.qualitySnapshot.state}`}>{selected.qualitySnapshot.state === "healthy" ? "健康" : selected.qualitySnapshot.state === "blocked" ? "阻断" : "需关注"}</span><code>{selected.deployment.releaseId ?? "尚未发布"}</code></span></div><KnowledgeRevisionLauncher asset={selected} active={Boolean(revisionRequest)} onStart={setRevisionRequest} /></div></div>
          </header>
          {revisionRequest ? <KnowledgeRevisionWorkbench key={`${revisionRequest.assetId}:${revisionRequest.fieldPath}:${revisionRequest.claimId ?? "new"}`} asset={selected} request={revisionRequest} onCancel={() => setRevisionRequest(null)} onNotify={onNotify} onSubmit={onRevisionSubmit} /> : <>
            <div className="asset-tabs" role="tablist" aria-label="资产详情视图">
              {visibleAssetTabs.map((item) => (
                <button key={item} role="tab" type="button" aria-selected={activeTab === item} onClick={() => openAssetTab(item)}>{item}</button>
              ))}
            </div>
            <div className="asset-tab-content" role="tabpanel">
              {activeTab === "概览" && <AssetOverview asset={selected} onOpenTab={openAssetTab} />}
              {activeTab === "定义" && <AssetDefinition asset={selected} onNotify={onNotify} onStartRevision={setRevisionRequest} />}
              {activeTab === "本体关系" && <AssetOntology asset={selected} focusedObject={focusedObject} onNotify={onNotify} />}
              {activeTab === "实现" && <AssetExecution asset={selected} focusedObject={focusedObject} onNotify={onNotify} />}
              {activeTab === "可信度" && <AssetEvidence asset={selected} onNotify={onNotify} onStartRevision={setRevisionRequest} />}
              {activeTab === "交付与影响" && <AssetUsage asset={selected} onNotify={onNotify} />}
            </div>
          </>}
        </section>
      </section>
    );
  }

  return (
    <section className="view view-assets asset-directory">
      <section className="asset-catalog" aria-label="知识目录">
        <div className="catalog-toolbar">
          <label className="search-field">
            <Search size={16} />
            <input ref={catalogSearchRef} type="search" aria-label="搜索知识目录" placeholder="名称、ID、资产或数据对象" value={query} onChange={(event) => setQuery(event.target.value)} />
            <kbd>⌘ K</kbd>
          </label>
          <label className="catalog-filter"><span>对象</span><select aria-label="筛选知识对象类型" value={type} onChange={(event) => setType(event.target.value as CatalogObjectType | "全部")}><option value="全部">全部对象</option><optgroup label="语义资产">{assetTypes.map((item) => <option key={item} value={item}>{item}</option>)}</optgroup><optgroup label="治理对象">{catalogObjectTypes.filter((item) => !assetTypes.includes(item as AssetType)).map((item) => <option key={item} value={item}>{item}</option>)}</optgroup></select></label>
          <label className="catalog-filter"><span>资产状态</span><select aria-label="筛选资产发布状态" value={assetStatus} onChange={(event) => setAssetStatus(event.target.value as Asset["status"] | "全部")}><option value="全部">全部状态</option><option value="已发布">已发布</option><option value="需关注">需关注</option><option value="草稿">草稿</option></select></label>
          <label className="catalog-filter"><span>语义域</span><select aria-label="筛选语义域" value={domain} onChange={(event) => setDomain(event.target.value)}><option value="全部">全部语义域</option>{domains.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>
          <button className="catalog-clear-button" type="button" aria-label="清除全部筛选" title="清除全部筛选" disabled={!hasFilters} onClick={() => { setQuery(""); setType("全部"); setAssetStatus("全部"); setDomain("全部"); }}><X size={15} /></button>
        </div>
        <div className="catalog-summary">
          <span>显示 <strong>{catalogGroups.length}</strong> / {assets.length} 个语义资产{isGovernedObjectType && <> · {visibleChildCount} 个{type}</>}</span>
          <label className="catalog-sort"><ArrowUpDown size={14} /><span className="visually-hidden">排序</span><select aria-label="知识目录排序" value={sort} onChange={(event) => setSort(event.target.value as "attention" | "name" | "type")}><option value="attention">需关注优先</option><option value="name">按名称</option><option value="type">按类型</option></select></label>
        </div>
        <div className="asset-table-head" aria-hidden="true"><span /><span>语义资产</span><span>类型与语义域</span><span>关系与实现</span><span>负责人</span><span>就绪度</span><span>发布状态</span><span /></div>
        <div className="asset-list">
          {catalogGroups.map(({ asset, assetItem, children, visibleChildren }) => {
            const readiness = readinessSummary(asset);
            const isAutoExpanded = isGovernedObjectType || (normalizedQuery.length > 0 && visibleChildren.length > 0);
            const isExpanded = expandedAssetIds.has(asset.id) || isAutoExpanded;
            return (
              <section className="asset-group" aria-label={`${asset.name} 对象组`} key={asset.id}>
                <div className={`asset-row asset-row-parent ${lastOpenedItemId === asset.id ? "asset-row-returned" : ""}`}>
                  <button className="asset-expand-button" type="button" aria-label={isAutoExpanded ? `${asset.name}的匹配对象已展开` : `${isExpanded ? "收起" : "展开"}${asset.name}的关系与实现`} aria-expanded={isExpanded} disabled={children.length === 0 || isAutoExpanded} onClick={() => toggleAsset(asset.id)}>{isExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}</button>
                  <button className="asset-row-open" type="button" aria-label={`打开语义资产 ${asset.name}`} onClick={() => openDetail(assetItem)}>
                    <span className="asset-row-identity"><AssetTypeMark type={asset.type} /><span className="asset-row-copy"><strong>{asset.name}</strong><code>{asset.key}</code></span></span>
                    <span className="asset-row-taxonomy"><strong>{asset.type}</strong><small>{asset.domain}</small></span>
                    <span className="asset-row-scope"><strong>{asset.relations.length} 关系 · {asset.bindings.length} 绑定</strong><small>{asset.joinContracts.length} 个 JoinContract</small></span>
                    <span className="asset-row-owner"><strong>{asset.owner}</strong><small>{asset.maintainer}</small></span>
                    <span className="asset-row-readiness"><span><b>{readiness.passed}/{readiness.total}</b><small>{readiness.warnings > 0 ? `${readiness.warnings} 项需关注` : "门禁通过"}</small></span><progress max={readiness.total} value={readiness.passed} aria-label={`${asset.name}就绪度 ${readiness.passed}/${readiness.total}`} /></span>
                    <StatusBadge tone={statusTone(asset.status)}>{asset.status}</StatusBadge>
                    <ChevronRight className="asset-row-chevron" size={16} />
                  </button>
                </div>
                {isExpanded && visibleChildren.length > 0 && <div className="asset-child-list" aria-label={`${asset.name}的关系与实现`}>
                  {visibleChildren.map((item) => <button key={item.id} type="button" className={`asset-child-row ${lastOpenedItemId === item.id ? "asset-child-row-returned" : ""}`} aria-label={`${item.name}，${item.type}，打开所属资产详情`} onClick={() => openDetail(item)}>
                    <span className="asset-row-identity"><CatalogTypeMark type={item.type} /><span className="asset-row-copy"><strong>{item.name}</strong><code>{item.key}</code></span></span>
                    <span className="asset-row-taxonomy"><strong>{item.type}</strong><small>{item.domain}</small></span>
                    <span className="asset-row-scope"><strong>{item.scope}</strong><small>{item.detail}</small></span>
                    <span className="asset-row-owner"><strong>{item.owner}</strong><small>继承资产责任</small></span>
                    <span className="asset-child-state-kind"><strong>{item.type === "物理绑定" ? "实现校验" : item.type === "JoinContract" ? "契约状态" : "发布状态"}</strong><small>{item.type === "物理绑定" ? "来源与表达式" : "当前 revision"}</small></span>
                    <StatusBadge tone={statusTone(item.status)}>{item.status}</StatusBadge>
                    <ChevronRight className="asset-row-chevron" size={16} />
                  </button>)}
                </div>}
              </section>
            );
          })}
          {catalogGroups.length === 0 && <div className="catalog-empty"><Search size={24} /><strong>没有匹配知识资产</strong><span>调整关键词、对象类型、资产状态或语义域。</span><button type="button" onClick={() => { setQuery(""); setType("全部"); setAssetStatus("全部"); setDomain("全部"); }}>清除筛选</button></div>}
        </div>
      </section>
    </section>
  );
}

interface AssetVersionCandidate {
  key: string;
  proposal: Proposal;
  asset: Asset;
  revision: string;
  previousRevision: string;
  source: string;
}

const candidateTargets: Record<string, { revision: string; previousRevision: string; source: string }> = {
  "PROP-128": { revision: "@9", previousRevision: "@8", source: "RUN-240824-1432" },
  "PROP-126": { revision: "@13", previousRevision: "@12", source: "RUN-240824-1432" },
  "PROP-121": { revision: "@4", previousRevision: "@3", source: "消费解析反馈" },
};

function candidateTargetFor(proposal: Proposal) {
  const knownTarget = candidateTargets[proposal.id];
  if (knownTarget) return knownTarget;
  const asset = assets.find((item) => item.id === proposal.assetId);
  if (!asset) return undefined;
  return { revision: `@${asset.revisionRecord.sequence + 1}`, previousRevision: asset.revision, source: "人工知识修订" };
}

function assetVersionCandidateFor(proposal: Proposal): AssetVersionCandidate | undefined {
  const asset = assets.find((item) => item.id === proposal.assetId);
  const target = candidateTargetFor(proposal);
  return asset && target ? { key: `candidate:${proposal.id}`, proposal, asset, ...target } : undefined;
}

function assetVersionCandidatesFor(sessionProposal: Proposal | null): AssetVersionCandidate[] {
  const availableProposals = sessionProposal ? [sessionProposal, ...proposals.filter((proposal) => proposal.assetId !== sessionProposal.assetId)] : proposals;
  return availableProposals.flatMap((proposal) => {
    const candidate = assetVersionCandidateFor(proposal);
    return candidate ? [candidate] : [];
  });
}

function CandidateVersionDetail({ candidate, decision, publishedCandidateId, publishedVersion, onReview, onPublish }: { candidate: AssetVersionCandidate; decision?: "批准" | "退回"; publishedCandidateId: string; publishedVersion: string; onReview: () => void; onPublish: () => void }) {
  const { proposal, asset } = candidate;
  const passed = proposal.validations.filter((item) => item.state === "passed").length;
  const failed = proposal.validations.filter((item) => item.state === "failed").length;
  const passedValidations = proposal.validations.filter((item) => item.state === "passed");
  const attentionValidations = proposal.validations.filter((item) => item.state !== "passed");
  const isPublished = publishedCandidateId === proposal.id;
  const status = isPublished ? "会话已发布" : decision === "批准" ? "待发布" : decision === "退回" ? "已退回" : failed > 0 ? "验证阻塞" : "待审核";
  const statusTone = isPublished || decision === "批准" ? "success" : decision === "退回" || failed > 0 ? "danger" : "warning";
  const riskTone = proposal.risk === "高风险" ? "danger" : proposal.risk === "中风险" ? "warning" : "success";
  const [activeReviewTab, setActiveReviewTab] = useState<"diff" | "source" | "impact">("diff");
  const [gateCollapsed, setGateCollapsed] = useState(attentionValidations.length === 0);

  return (
    <section className="release-detail governance-release-detail asset-version-detail-v2 candidate-version-detail" aria-label={`${asset.name} ${candidate.revision} 候选资产版本详情`}>
      <header className="candidate-review-summary">
        <div className="candidate-review-title">
          <div><span className="panel-kicker">候选资产版本 · <code>{asset.key}</code></span><h2>{asset.name} <span>{candidate.revision}</span></h2><p>{proposal.summary}</p></div>
        </div>
        <dl className="candidate-review-facts">
          <div><dt>版本范围</dt><dd>{candidate.previousRevision} → {candidate.revision}</dd></div>
          <div><dt>发布门禁</dt><dd>{passed}/{proposal.validations.length} 通过</dd></div>
          <div><dt>影响对象</dt><dd>{proposal.impact.length} 个</dd></div>
          <div><dt>风险等级</dt><dd>{proposal.risk}</dd></div>
        </dl>
        <div className="candidate-review-summary-actions">
          <StatusBadge tone={statusTone}>{status}</StatusBadge>
          {!isPublished && decision === "批准" && failed === 0 ? <button className="primary-button" type="button" onClick={onPublish}><PackageCheck size={16} />模拟发布 {candidate.revision}</button> : !isPublished ? <button className="primary-button" type="button" aria-label={`审核候选版本 ${asset.name} ${candidate.revision}`} onClick={onReview}><ShieldCheck size={16} />{failed > 0 ? "审查阻塞项" : decision === "退回" ? "重新审核版本" : "审核候选版本"}</button> : null}
        </div>
      </header>

      <section className="candidate-review-workspace" aria-label="版本评审">
        <header className="candidate-review-workspace-header">
          <div className="candidate-review-tabs" role="tablist" aria-label="候选版本评审内容">
            <button type="button" role="tab" aria-selected={activeReviewTab === "diff"} onClick={() => setActiveReviewTab("diff")}><span>版本差异</span><b>{proposal.changes.length}</b></button>
            <button type="button" role="tab" aria-selected={activeReviewTab === "source"} onClick={() => setActiveReviewTab("source")}><span>变更来源</span><b>1</b></button>
            <button type="button" role="tab" aria-selected={activeReviewTab === "impact"} onClick={() => setActiveReviewTab("impact")}><span>消费影响</span><b>{proposal.impact.length}</b></button>
          </div>
        </header>
        <div className={gateCollapsed ? "candidate-review-workspace-body candidate-review-gate-collapsed" : "candidate-review-workspace-body"}>
          <main className="candidate-review-main" role="tabpanel" aria-label={activeReviewTab === "diff" ? "版本差异" : activeReviewTab === "source" ? "变更来源" : "消费影响"}>
            {activeReviewTab === "diff" && <section className="candidate-review-section candidate-version-diff" aria-labelledby="candidate-version-diff-title">
              <div className="candidate-card-heading"><div><span className="panel-kicker">版本差异</span><h3 id="candidate-version-diff-title">定义与计算变化</h3><p>以下变化会作为同一个资产版本整体生效。</p></div><span className="candidate-card-count">{candidate.previousRevision} → {candidate.revision}</span></div>
              <div className="candidate-diff-list">{proposal.changes.map((change) => (
                <article className="diff-block" key={change.field}>
                  <header><code className="diff-field">{change.field}</code><span>字段变更</span></header>
                  <div className="candidate-diff-compare">
                    <section className="candidate-diff-value candidate-diff-before"><span>当前 {candidate.previousRevision}</span><p>{change.before}</p></section>
                    <ArrowRight size={15} />
                    <section className="candidate-diff-value candidate-diff-after"><span>候选 {candidate.revision}</span><p>{change.after}</p></section>
                  </div>
                </article>
              ))}</div>
            </section>}

            {activeReviewTab === "source" && <section className="candidate-review-section candidate-change-section" aria-labelledby="candidate-change-title">
              <div className="candidate-card-heading"><div><span className="panel-kicker">版本来源</span><h3 id="candidate-change-title">包含的变更事项</h3><p>候选版本由以下变更事项生成。</p></div><span className="candidate-card-count">1 项</span></div>
              <article className="candidate-change-record">
                <span><GitPullRequestArrow size={17} /></span>
                <div><strong>{proposal.title}</strong><small><code>{proposal.id}</code> · {proposal.author} · {proposal.createdAt}</small><p>{candidate.source}</p></div>
                <StatusBadge tone={riskTone}>{proposal.risk}</StatusBadge>
              </article>
            </section>}

            {activeReviewTab === "impact" && <section className="candidate-review-section candidate-version-impact" aria-labelledby="candidate-impact-title">
              <div className="candidate-card-heading"><div><span className="panel-kicker">消费影响</span><h3 id="candidate-impact-title">版本切换影响范围</h3><p>审批前确认受影响消费者和迁移边界。</p></div><span className="candidate-card-count">{proposal.impact.length} 个对象</span></div>
              <div className="candidate-impact-list">{proposal.impact.map((item) => <span key={item}><Users size={15} /><strong>{item}</strong></span>)}</div>
            </section>}
          </main>

          <aside className={gateCollapsed ? "candidate-review-inspector candidate-review-inspector-collapsed" : "candidate-review-inspector"} aria-label="候选版本治理信息">
            <header className="candidate-review-inspector-header">
              {!gateCollapsed && <div><span className="panel-kicker">发布门禁</span><h3 id="candidate-validation-title">验证与审批</h3></div>}
              <button className="icon-button" type="button" aria-label={gateCollapsed ? "展开发布门禁" : "收起发布门禁"} title={gateCollapsed ? "展开发布门禁" : "收起发布门禁"} onClick={() => setGateCollapsed((collapsed) => !collapsed)}>{gateCollapsed ? <PanelRightOpen size={16} /> : <PanelRightClose size={16} />}</button>
            </header>
            {gateCollapsed ? <span className={attentionValidations.length > 0 ? "candidate-gate-collapsed-status candidate-gate-collapsed-status-attention" : "candidate-gate-collapsed-status"} title={`${passed}/${proposal.validations.length} 项门禁通过`}>{attentionValidations.length > 0 ? attentionValidations.length : passed}</span> : <section className="candidate-validation-section" aria-labelledby="candidate-validation-title">
              <div className="candidate-validation-count"><span>门禁通过</span><strong>{passed}/{proposal.validations.length}</strong></div>
              <div className={attentionValidations.length > 0 ? "candidate-gate-summary candidate-gate-summary-attention" : "candidate-gate-summary"}><span>{attentionValidations.length > 0 ? <AlertTriangle size={16} /> : <CheckCircle2 size={16} />}</span><div><strong>{attentionValidations.length > 0 ? `${attentionValidations.length} 项需要确认` : "全部门禁通过"}</strong><small>{attentionValidations.length > 0 ? "优先处理异常，再决定是否发布。" : "可以进入资产负责人审核。"}</small></div></div>
              <div className="validation-list candidate-attention-validations">{attentionValidations.map((item) => (
                <div className={`validation-item validation-${item.state}`} key={item.name}><span>{validationIcon(item.state)}</span><div><strong>{validationLabel(item.name)}</strong><small>{item.detail}</small></div></div>
              ))}</div>
              {passedValidations.length > 0 && <details className="candidate-passed-validations"><summary><span><CheckCircle2 size={15} />已通过检查</span><span>{passedValidations.length} 项<ChevronDown size={14} /></span></summary><div className="validation-list">{passedValidations.map((item) => (
                <div className={`validation-item validation-${item.state}`} key={item.name}><span>{validationIcon(item.state)}</span><div><strong>{validationLabel(item.name)}</strong><small>{item.detail}</small></div></div>
              ))}</div></details>}
              <div className="candidate-review-decision-note"><strong>{isPublished ? `${asset.name} ${candidate.revision} 已在本次会话发布` : decision === "批准" ? "审核通过，可以发布整个候选版本" : failed > 0 ? "验证失败，候选版本当前不可发布" : "审核针对完整资产版本"}</strong><span>{isPublished ? `发布批次 ${publishedVersion}；真实资产与消费绑定未写入后端。` : `${asset.owner} 需要基于版本差异、验证证据和消费影响完成决策。`}</span></div>
            </section>}
          </aside>
        </div>
      </section>
    </section>
  );
}

function ReleaseInspectorDialog({ mode, release, onClose }: { mode: "compare" | "manifest" | "policy"; release: AssetVersionRelease; onClose: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const title = mode === "compare" ? `比较${release.assetName}版本` : mode === "manifest" ? `${release.assetName} ${release.revision} 发布清单` : "审核与发布规则";
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog inspector-dialog" role="dialog" aria-modal="true" aria-labelledby="inspector-dialog-title">
        <header><div><span className="panel-kicker">发布检查</span><h2 id="inspector-dialog-title">{title}</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label={`关闭${title}`} onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body">
          {mode === "compare" && <div className="release-compare"><div><span>{release.assetName} · 上一版本</span><strong>{release.previousRevision}</strong><small>{release.assetKey}</small></div><ArrowRight size={18} /><div><span>{release.assetName} · 当前选择</span><strong>{release.revision}</strong><small>{release.id} · {release.publishedAt}</small></div><ul>{release.changes.map((change) => <li key={change}>{change}</li>)}</ul></div>}
          {mode === "manifest" && <div className="manifest-preview"><p>该清单固定单个语义资产修订及其物理绑定、JoinContract、校验和与消费兼容性。</p><pre>{JSON.stringify({ asset_id: release.assetId, asset_key: release.assetKey, revision: release.revision, release_batch: release.id, state: release.state === "Stable" ? "当前版本" : "历史版本", publisher: release.publisher, digest: "sha256:9f2e...a84c" }, null, 2)}</pre></div>}
          {mode === "policy" && <div className="policy-list"><div><ShieldCheck size={18} /><span><strong>至少一个变更事项通过验证</strong><small>失败或存在破坏性消费影响的事项不能进入发布候选。</small></span></div><div><Users size={18} /><span><strong>资产负责人完成审核</strong><small>G1 策略要求一名负责人明确批准。</small></span></div><div><PackageCheck size={18} /><span><strong>发布生成不可变版本</strong><small>后续变更必须创建新版本，回滚只切换消费绑定。</small></span></div></div>}
        </div>
        <footer><span /><button className="primary-button" type="button" onClick={onClose}>完成</button></footer>
      </section>
    </div>
  );
}

function ReleaseDetail({ release, publishedVersion, onCompare, onManifest, onRollback }: { release: AssetVersionRelease; publishedVersion: string; onCompare: () => void; onManifest: () => void; onRollback: () => void }) {
  const isCurrent = release.state === "Stable";
  return (
    <section className="release-detail governance-release-detail asset-version-detail-v2" aria-label={`${release.assetName} ${release.revision} 语义资产版本详情`}>
      <header className="release-detail-header">
        <div className="release-seal"><PackageCheck size={24} /></div>
        <div><span className="panel-kicker">语义资产版本 · {release.assetKey}</span><h2>{release.assetName} · {release.revision}</h2><p>{release.summary}</p></div>
        <div className="release-detail-header-actions"><StatusBadge tone={isCurrent ? "success" : "neutral"}><BadgeCheck size={13} />{isCurrent ? "当前版本" : "历史版本"}</StatusBadge><button className="text-button" type="button" onClick={onManifest}><FileCheck2 size={14} />发布清单</button><button className="text-button" type="button" onClick={onCompare}><RotateCcw size={14} />与 {release.previousRevision} 比较</button></div>
      </header>
      <div className="manifest-strip"><div><span>资产类型</span><strong>{release.assetType}</strong></div><div><span>修订版本</span><strong>{release.revision}</strong></div><div><span>发布批次</span><code>{release.id}</code></div><div><span>发布者 · 时间</span><strong>{release.publisher} · {release.publishedAt}</strong></div></div>
      <div className="asset-version-detail-grid">
        <section className="release-changes">
          <div className="subsection-heading"><div><span className="panel-kicker">版本变更</span><h3>{release.previousRevision} → {release.revision}</h3><p>{release.summary}</p></div><StatusBadge tone="success">{release.changes.length} 项已验证</StatusBadge></div>
          <div className="change-list">{release.changes.map((change) => <div key={change}><CheckCircle2 size={16} /><span>{change}</span></div>)}</div>
        </section>
        <section className="release-bindings">
          <div className="subsection-heading"><div><span className="panel-kicker">消费绑定</span><h3>使用这个资产版本的应用</h3><p>仅这些消费者会受到该 revision 切换影响。</p></div><span>{release.consumers.length} 个绑定</span></div>
          <div className="asset-version-consumer-list">{release.consumers.length > 0 ? release.consumers.map((consumer) => (
            <article key={consumer.name}>
              <span className="consumer-mark"><Box size={18} /></span>
              <div><strong>{consumer.name}</strong><small>{consumer.kind} · 最近解析 {consumer.lastResolved}</small></div>
              <code>{consumer.binding}</code>
              <StatusBadge tone="info">健康</StatusBadge>
            </article>
          )) : <div className="empty-inline">该资产版本没有活跃绑定。</div>}</div>
        </section>
      </div>
      <footer className="release-actions"><div><strong>回滚只切换 {release.assetName} 的消费绑定</strong><span>其他语义资产版本不受影响；不可变历史保持原样。</span></div><button className="secondary-button" type="button" onClick={onRollback} disabled={!publishedVersion}><RotateCcw size={16} />模拟切回 {release.previousRevision}</button></footer>
    </section>
  );
}

type AssetVersionFilter = "全部" | "待处理" | "当前版本" | "历史版本";

function ReleasesView({ decisions, publishedVersion, publishedCandidateId, rollbackPerformed, sessionProposal, initialSelectedKey, initialSourceRun, detailBackRequestEpoch, onDetailOpenChange, onReview, onPublish, onRollback }: { decisions: Partial<Record<string, "批准" | "退回">>; publishedVersion: string; publishedCandidateId: string; rollbackPerformed: boolean; sessionProposal: Proposal | null; initialSelectedKey: string | null; initialSourceRun: string | null; detailBackRequestEpoch: number; onDetailOpenChange: (open: boolean) => void; onReview: (proposal: Proposal) => void; onPublish: (proposal: Proposal) => void; onRollback: () => void }) {
  const [selectedKey, setSelectedKey] = useState<string | null>(initialSelectedKey);
  const [selectedDetailBackEpoch, setSelectedDetailBackEpoch] = useState(detailBackRequestEpoch);
  const [inspector, setInspector] = useState<"compare" | "manifest" | "policy" | null>(null);
  const [filter, setFilter] = useState<AssetVersionFilter>("全部");
  const [query, setQuery] = useState(initialSourceRun ?? "");
  const assetVersionCandidates = useMemo(() => assetVersionCandidatesFor(sessionProposal), [sessionProposal]);
  const activeSelectedKey = selectedDetailBackEpoch === detailBackRequestEpoch ? selectedKey : null;
  const selectedCandidate = assetVersionCandidates.find((candidate) => candidate.key === activeSelectedKey);
  const selectedRelease = assetVersionReleases.find((release) => `${release.assetId}:${release.revision}` === activeSelectedKey);
  const inspectorRelease = selectedRelease ?? assetVersionReleases[0];

  useEffect(() => {
    onDetailOpenChange(Boolean(selectedCandidate || selectedRelease));
    return () => onDetailOpenChange(false);
  }, [onDetailOpenChange, selectedCandidate, selectedRelease]);

  if (selectedCandidate) {
    return (
      <section className="view view-releases governance-detail-view">
        <CandidateVersionDetail candidate={selectedCandidate} decision={decisions[selectedCandidate.proposal.id]} publishedCandidateId={publishedCandidateId} publishedVersion={publishedVersion} onReview={() => onReview(selectedCandidate.proposal)} onPublish={() => onPublish(selectedCandidate.proposal)} />
      </section>
    );
  }

  if (selectedRelease) {
    return (
      <section className="view view-releases governance-detail-view">
        <ReleaseDetail release={selectedRelease} publishedVersion={publishedVersion} onCompare={() => setInspector("compare")} onManifest={() => setInspector("manifest")} onRollback={onRollback} />
        {inspector && <ReleaseInspectorDialog mode={inspector} release={inspectorRelease} onClose={() => setInspector(null)} />}
      </section>
    );
  }

  const normalizedQuery = query.trim().toLocaleLowerCase("zh-CN");
  const visibleCandidates = assetVersionCandidates.filter((candidate) => {
    const isPublished = publishedCandidateId === candidate.proposal.id;
    const matchesFilter = filter === "全部" || (filter === "待处理" && !isPublished) || (filter === "当前版本" && isPublished);
    const matchesQuery = !normalizedQuery || `${candidate.asset.name} ${candidate.asset.key} ${candidate.revision} ${candidate.proposal.id} ${candidate.proposal.title} ${candidate.source}`.toLocaleLowerCase("zh-CN").includes(normalizedQuery);
    return matchesFilter && matchesQuery;
  });
  const visibleReleases = assetVersionReleases.filter((release) => {
    const matchesFilter = filter === "全部" || (filter === "当前版本" && release.state === "Stable") || (filter === "历史版本" && release.state === "Superseded");
    const matchesQuery = !normalizedQuery || `${release.assetName} ${release.assetKey} ${release.revision} ${release.id} ${release.summary}`.toLocaleLowerCase("zh-CN").includes(normalizedQuery);
    return matchesFilter && matchesQuery;
  });
  const visibleCount = visibleCandidates.length + visibleReleases.length;

  return (
    <section className="view view-releases release-list-only">
      {rollbackPerformed && <div className="rollback-notice" role="status"><RotateCcw size={16} /><span>模拟回滚完成：所选语义资产已切回上一修订，其他资产版本保持不变。</span></div>}
      <section className="governance-list-surface release-history-surface asset-version-registry" aria-label="语义资产版本列表">
        <div className="governance-list-toolbar asset-version-toolbar">
          <div className="segment-control governance-filter" aria-label="资产版本筛选">{(["全部", "待处理", "当前版本", "历史版本"] as AssetVersionFilter[]).map((item) => <button key={item} type="button" aria-pressed={filter === item} onClick={() => setFilter(item)}>{item === "全部" ? `全部 ${assetVersionCandidates.length + assetVersionReleases.length}` : item === "待处理" ? `待处理 ${assetVersionCandidates.length}` : item}</button>)}</div>
          <label className="governance-search"><Search size={16} /><input type="search" aria-label="搜索资产版本" placeholder="搜索资产、版本、运行 ID、变更编号或发布批次" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
        </div>
        <div className="governance-list-summary"><span>显示 <strong>{visibleCount}</strong> / {assetVersionCandidates.length + assetVersionReleases.length} 条版本</span><span><ArrowUpDown size={14} />待处理优先，其次按发布时间</span></div>
        <div className="governance-table release-table">
          <div className="governance-list-head release-compact-grid" aria-hidden="true"><span>语义资产</span><span>版本</span><span>版本内容</span><span>发布批次</span><span>状态</span><span>责任人</span><span /></div>
          <div className="governance-list-body">
            {visibleCandidates.map((candidate) => {
              const { proposal, asset } = candidate;
              const failed = proposal.validations.some((item) => item.state === "failed");
              const decision = decisions[proposal.id];
              const isPublished = publishedCandidateId === proposal.id;
              const status = isPublished ? "会话已发布" : decision === "批准" ? "待发布" : decision === "退回" ? "已退回" : failed ? "验证阻塞" : "待审核";
              const tone = isPublished || decision === "批准" ? "success" : decision === "退回" || failed ? "danger" : "warning";
              return <button key={candidate.key} type="button" className="governance-list-row release-compact-grid candidate-version-row" aria-label={`查看候选资产版本 ${asset.name} ${candidate.revision}`} onClick={() => { setSelectedKey(candidate.key); setSelectedDetailBackEpoch(detailBackRequestEpoch); }}>
                <span className="governance-primary governance-primary-compact"><strong>{asset.name}</strong><small><code>{asset.key}</code><span>{asset.type}</span></small></span>
                <span className="governance-metric"><strong>{candidate.revision}</strong><small>基于 {candidate.previousRevision}</small></span>
                <span className="governance-context"><strong>{proposal.title}</strong><small>{proposal.id} · {proposal.changes.length} 项字段变更</small></span>
                <span className="governance-row-owner"><code>尚未发布</code><small>{candidate.source}</small></span>
                <StatusBadge tone={tone}>{status}</StatusBadge>
                <span className="governance-row-owner"><strong>{asset.owner}</strong><small>{proposal.author} 起草</small></span>
                <ChevronRight size={16} />
              </button>;
            })}
            {visibleReleases.map((release) => {
              const isCurrent = release.state === "Stable";
              return <button key={`${release.assetId}:${release.revision}`} type="button" className="governance-list-row release-compact-grid" aria-label={`查看语义资产版本 ${release.assetName} ${release.revision}`} onClick={() => { setSelectedKey(`${release.assetId}:${release.revision}`); setSelectedDetailBackEpoch(detailBackRequestEpoch); }}>
                <span className="governance-primary governance-primary-compact"><strong>{release.assetName}</strong><small><code>{release.assetKey}</code><span>{release.assetType}</span></small></span>
                <span className="governance-metric"><strong>{release.revision}</strong><small>上一版 {release.previousRevision}</small></span>
                <span className="governance-context"><strong>{release.summary}</strong><small>{release.changes[0]}</small></span>
                <span className="governance-row-owner"><code>{release.id}</code><small>{release.publishedAt}</small></span>
                <StatusBadge tone={isCurrent ? "success" : "neutral"}>{isCurrent ? "当前版本" : "历史版本"}</StatusBadge>
                <span className="governance-row-owner"><strong>{release.publisher}</strong><small>已签名发布</small></span>
                <ChevronRight size={16} />
              </button>;
            })}
            {visibleCount === 0 && <div className="governance-list-empty"><PackageCheck size={22} /><strong>没有匹配的资产版本</strong><span>调整状态筛选或搜索关键词。</span></div>}
          </div>
        </div>
      </section>
      {inspector && <ReleaseInspectorDialog mode={inspector} release={inspectorRelease} onClose={() => setInspector(null)} />}
    </section>
  );
}

function ReviewDialog({ proposal, onClose, onDecision }: { proposal: Proposal; onClose: () => void; onDecision: (decision: "批准" | "退回") => void }) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const target = candidateTargetFor(proposal);
  const hasFailedValidation = proposal.validations.some((item) => item.state === "failed");

  useEffect(() => {
    const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handleKeyDown);
    closeButtonRef.current?.focus();
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      previousFocus?.focus();
    };
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog" role="dialog" aria-modal="true" aria-labelledby="review-dialog-title">
        <header>
          <div><span className="panel-kicker">候选资产版本 · {proposal.id}</span><h2 id="review-dialog-title">审核 {proposal.assetName} · {target?.revision ?? "候选版本"}</h2></div>
          <button ref={closeButtonRef} className="icon-button" type="button" aria-label="关闭审核" onClick={onClose}><X size={18} /></button>
        </header>
        <div className="prototype-notice"><CircleAlert size={17} /><span>审核面向整个候选版本；原型决策仅保留在当前会话，不会写入或发布真实数据。</span></div>
        <div className="dialog-body">
          <div className="dialog-summary"><span className="proposal-author ai-author"><PackageCheck size={16} /></span><div><strong>{proposal.assetName} {target?.previousRevision} → {target?.revision}</strong><p>包含变更事项 {proposal.id}：{proposal.title}</p></div></div>
          <div className="dialog-validation">
            {proposal.validations.map((item) => (
              <div className={`validation-item validation-${item.state}`} key={item.name}><span>{validationIcon(item.state)}</span><div><strong>{validationLabel(item.name)}</strong><small>{item.detail}</small></div></div>
            ))}
          </div>
          <div className="release-sod-check"><ShieldCheck size={17} /><span><strong>职责分离检查通过</strong><small><b>陈默 · Reviewer</b> 负责本次版本评审；发布者必须是独立主体，并持有目标生产范围的 <code>release.publish</code>。</small></span></div>
          {hasFailedValidation && <div className="version-review-blocker"><CircleAlert size={16} /><span><strong>候选版本存在失败门禁</strong><small>可以退回版本，但必须修复验证后才能批准发布。</small></span></div>}
          <label className="review-note"><span>审核意见</span><textarea placeholder="记录判断依据，原型中不会保存。" /></label>
        </div>
        <footer>
          <button className="danger-button" type="button" onClick={() => onDecision("退回")}><RotateCcw size={16} />模拟退回版本</button>
          <div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={hasFailedValidation} onClick={() => onDecision("批准")}><Check size={16} />模拟批准版本</button></div>
        </footer>
      </section>
    </div>
  );
}

function CreateProposalDialog({ onClose, onCreate }: { onClose: () => void; onCreate: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog" role="dialog" aria-modal="true" aria-labelledby="create-proposal-title">
        <header><div><span className="panel-kicker">人工发起</span><h2 id="create-proposal-title">创建变更事项</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭变更事项创建" onClick={onClose}><X size={18} /></button></header>
        <div className="prototype-notice"><CircleAlert size={17} /><span>原型会创建可见的会话草稿，但不会持久化语义资产。</span></div>
        <div className="dialog-body proposal-form">
          <label><span>基础资产</span><select aria-label="基础资产" defaultValue={assets[1].id}>{assets.map((asset) => <option key={asset.id} value={asset.id}>{asset.name} · {asset.key}</option>)}</select></label>
          <label><span>变更意图</span><textarea aria-label="变更意图" defaultValue="补充客单价退款订单口径的业务边界与验证证据。" /></label>
          <div className="proposal-policy"><ShieldCheck size={18} /><span><strong>治理级别 G1</strong><small>AI 可补全草稿和运行预检，发布前必须由资产负责人审核。</small></span></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" onClick={onCreate}><Sparkles size={16} />创建会话草稿</button></div></footer>
      </section>
    </div>
  );
}

function PublishDialog({ proposal, onClose, onPublish }: { proposal: Proposal; onClose: () => void; onPublish: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  const [publisherId, setPublisherId] = useState("USR-PUBLISHER");
  const target = candidateTargetFor(proposal);
  const asset = assets.find((item) => item.id === proposal.assetId);
  const hasSeparationConflict = publisherId === "USR-REVIEWER";
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog" role="dialog" aria-modal="true" aria-labelledby="publish-dialog-title">
        <header><div><span className="panel-kicker">语义资产 · {asset?.key}</span><h2 id="publish-dialog-title">发布{proposal.assetName} {target?.revision}</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭发布确认" onClick={onClose}><X size={18} /></button></header>
        <div className="prototype-notice"><CircleAlert size={17} /><span>这是模拟发布，只更新当前浏览器会话，不会真实切换{proposal.assetName}的问答或下游消费绑定。</span></div>
        <div className="dialog-body publish-checklist">
          <div><CheckCircle2 size={17} /><span><strong>结构与引用验证</strong><small>{proposal.validations.length} / {proposal.validations.length} 通过</small></span></div>
          <div><CheckCircle2 size={17} /><span><strong>治理策略</strong><small>已满足 G1 负责人审核要求</small></span></div>
          <div><CheckCircle2 size={17} /><span><strong>消费影响</strong><small>{proposal.impact.length} 个对象已完成检查</small></span></div>
          <label className="publish-identity"><span>发布身份</span><select aria-label="发布身份" value={publisherId} onChange={(event) => setPublisherId(event.target.value)}><option value="USR-PUBLISHER">周岚 · Publisher</option><option value="USR-REVIEWER">陈默 · Reviewer</option></select><small>{hasSeparationConflict ? "与当前评审主体冲突" : "陈默评审 · 周岚发布"}</small></label>
          {hasSeparationConflict ? <div className="release-sod-conflict" role="alert"><CircleAlert size={17} /><span><strong>职责分离冲突</strong><small>受保护范围的评审者与发布者必须相互独立。请切换到周岚或其他具备生产发布权限的独立主体。</small></span></div> : <div className="release-sod-check"><ShieldCheck size={17} /><span><strong>独立发布者已确认</strong><small>当前发布身份持有生产环境 <code>release.publish</code>，且未参与本次版本评审。</small></span></div>}
          <div className="publish-target"><span>资产版本</span><code>{asset?.key} · {target?.revision}</code><small>发布批次 release-2026.08.4-session</small></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={hasSeparationConflict} onClick={onPublish}><PackageCheck size={16} />确认模拟发布并生效</button></div></footer>
      </section>
    </div>
  );
}

function CommandPalette({ onClose, onNavigate, onSelectAsset, onSelectProposal, onCreateProposal }: { onClose: () => void; onNavigate: (view: ViewId) => void; onSelectAsset: (id: string) => void; onSelectProposal: (id: string) => void; onCreateProposal: () => void }) {
  const [query, setQuery] = useState("");
  const [activeIndex, setActiveIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const close = () => { setQuery(""); onClose(); };
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") { setQuery(""); onClose(); } };
    document.addEventListener("keydown", handleKeyDown);
    inputRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const commands = [
    ...navigation.map((item) => ({ key: `view-${item.id}`, label: item.label, detail: `打开${contextTitles[item.id]}`, icon: item.icon, run: () => onNavigate(item.id) })),
    { key: "create-proposal", label: "创建变更事项", detail: "从已有资产起草一项受治理变更", icon: Sparkles, run: onCreateProposal },
    ...assets.map((asset) => ({ key: asset.id, label: asset.name, detail: `${asset.type} · ${asset.domain} · ${asset.key}`, icon: Box, run: () => onSelectAsset(asset.id) })),
    ...proposals.map((proposal) => ({ key: proposal.id, label: proposal.title, detail: `${proposal.id} · ${proposal.risk} · ${proposal.assetName}`, icon: GitPullRequestArrow, run: () => onSelectProposal(proposal.id) })),
  ];
  const normalized = query.trim().toLowerCase();
  const filtered = commands.filter((item) => `${item.label} ${item.detail}`.toLowerCase().includes(normalized)).slice(0, 10);
  const runCommand = (command: (typeof commands)[number]) => { command.run(); close(); };
  const handleSearchKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "ArrowDown") { event.preventDefault(); setActiveIndex((index) => Math.min(index + 1, filtered.length - 1)); }
    if (event.key === "ArrowUp") { event.preventDefault(); setActiveIndex((index) => Math.max(index - 1, 0)); }
    if (event.key === "Enter" && filtered[activeIndex]) runCommand(filtered[activeIndex]);
  };

  return (
    <div className="dialog-backdrop command-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) close(); }}>
      <section className="command-palette" role="dialog" aria-modal="true" aria-label="搜索与命令">
        <label className="command-search"><Search size={18} /><input ref={inputRef} value={query} onChange={(event) => { setQuery(event.target.value); setActiveIndex(0); }} onKeyDown={handleSearchKeyDown} placeholder="搜索资产、变更或功能" aria-label="搜索资产、变更或功能" /><kbd>Esc</kbd></label>
        <div className="command-results" role="listbox" aria-label="命令结果">
          {filtered.map((item, index) => { const Icon = item.icon; return <button type="button" role="option" aria-selected={index === activeIndex} key={item.key} onMouseEnter={() => setActiveIndex(index)} onClick={() => runCommand(item)}><span className="command-icon"><Icon size={16} /></span><span><strong>{item.label}</strong><small>{item.detail}</small></span><ArrowRight size={15} /></button>; })}
          {filtered.length === 0 && <div className="command-empty"><Search size={22} /><strong>没有匹配结果</strong><span>试试资产名、变更编号或“发布”。</span></div>}
        </div>
        <footer><span><kbd>↑↓</kbd> 浏览</span><span><kbd>Enter</kbd> 打开</span><span><kbd>Esc</kbd> 关闭</span></footer>
      </section>
    </div>
  );
}

export function App() {
  const [view, setView] = useState<ViewId>("ask");
  const [contextPanelOpen, setContextPanelOpen] = useState(true);
  const [contextPanelWidth, setContextPanelWidth] = useState(initialContextPanelWidth);
  const [askSessionKey, setAskSessionKey] = useState(0);
  const [contextIndex, setContextIndex] = useState(1);
  const [contextSelectionEpoch, setContextSelectionEpoch] = useState(0);
  const [sourceRunDetailOpen, setSourceRunDetailOpen] = useState(false);
  const [sourceRunBackRequestEpoch, setSourceRunBackRequestEpoch] = useState(0);
  const [sourceInitialRunId, setSourceInitialRunId] = useState<string | undefined>();
  const [embeddingRebuildRun, setEmbeddingRebuildRun] = useState<EmbeddingRebuildRun | null>(null);
  const [auditRunRequestId, setAuditRunRequestId] = useState<string | undefined>();
  const [releaseDetailOpen, setReleaseDetailOpen] = useState(false);
  const [releaseDetailBackRequestEpoch, setReleaseDetailBackRequestEpoch] = useState(0);
  const [selectedAssetId, setSelectedAssetId] = useState(assets[0].id);
  const [recentAssetIds, setRecentAssetIds] = useState(initialRecentAssetIds);
  const recentAssetOpenSequence = useRef(initialRecentAssetIds.length);
  const recentAssetOpenedAt = useRef<Record<string, number>>(Object.fromEntries(initialRecentAssetIds.map((id, index) => [id, initialRecentAssetIds.length - index])));
  const [assetTabRequest, setAssetTabRequest] = useState<AssetTab>("概览");
  const [assetDetailRequest, setAssetDetailRequest] = useState(false);
  const [assetTabRequestEpoch, setAssetTabRequestEpoch] = useState(0);
  const [assetDetailBackRequestEpoch, setAssetDetailBackRequestEpoch] = useState(0);
  const [activeWorkbenchTask, setActiveWorkbenchTask] = useState<WorkbenchTask | null>(null);
  const [knowledgeRevisionRequest, setKnowledgeRevisionRequest] = useState<KnowledgeRevisionRequest | null>(null);
  const [assetCatalogTypeRequest, setAssetCatalogTypeRequest] = useState<CatalogObjectType | "全部">("全部");
  const [askConversationTitles, setAskConversationTitles] = useState(initialAskConversationTitles);
  const [versionDetailRequest, setVersionDetailRequest] = useState<{ key: string | null; epoch: number }>({ key: null, epoch: 0 });
  const [versionSourceRun, setVersionSourceRun] = useState<string | null>(null);
  const [reviewing, setReviewing] = useState<Proposal | null>(null);
  const [decisions, setDecisions] = useState<Partial<Record<string, "批准" | "退回">>>({});
  const [discoveryState, setDiscoveryState] = useState<"ready" | "running" | "complete">("ready");
  const [publishedVersion, setPublishedVersion] = useState("");
  const [publishedCandidateId, setPublishedCandidateId] = useState("");
  const [sessionProposal, setSessionProposal] = useState<Proposal | null>(null);
  const [rollbackPerformed, setRollbackPerformed] = useState(false);
  const [creatingProposal, setCreatingProposal] = useState(false);
  const [publishing, setPublishing] = useState<Proposal | null>(null);
  const [commandOpen, setCommandOpen] = useState(false);
  const [assetSearchRequestEpoch, setAssetSearchRequestEpoch] = useState(0);
  const [workbenchSearchRequestEpoch, setWorkbenchSearchRequestEpoch] = useState(0);
  const [conversationSearchRequestEpoch, setConversationSearchRequestEpoch] = useState(0);
  const [toast, setToast] = useState("");
  const discoveryTimer = useRef<number | null>(null);
  const workspaceRef = useRef<HTMLElement>(null);

  const settleRecentAssets = useCallback(() => {
    setRecentAssetIds((current) => [...current].sort((left, right) => (recentAssetOpenedAt.current[right] ?? 0) - (recentAssetOpenedAt.current[left] ?? 0)));
  }, []);

  const openCatalogSearch = useCallback(() => {
    setCommandOpen(false);
    if (view !== "assets" || assetDetailRequest) {
      if (view !== "assets") settleRecentAssets();
      setKnowledgeRevisionRequest(null);
      setView("assets");
      setContextIndex(0);
      setAssetCatalogTypeRequest("全部");
      setAssetTabRequest("概览");
      setAssetDetailRequest(false);
      setAssetTabRequestEpoch((epoch) => epoch + 1);
    }
    setAssetSearchRequestEpoch((epoch) => epoch + 1);
  }, [assetDetailRequest, settleRecentAssets, view]);

  const openWorkbenchSearch = useCallback(() => {
    setCommandOpen(false);
    if (view !== "overview" || activeWorkbenchTask) {
      setView("overview");
      setContextIndex(0);
      setActiveWorkbenchTask(null);
    }
    setWorkbenchSearchRequestEpoch((epoch) => epoch + 1);
  }, [activeWorkbenchTask, view]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        if (view === "assets") openCatalogSearch();
        else if (view === "overview") openWorkbenchSearch();
        else if (view === "ask") setConversationSearchRequestEpoch((epoch) => epoch + 1);
        else setCommandOpen((open) => !open);
      }
    };
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      if (discoveryTimer.current !== null) window.clearTimeout(discoveryTimer.current);
    };
  }, [openCatalogSearch, openWorkbenchSearch, view]);

  useEffect(() => {
    if (workspaceRef.current) workspaceRef.current.scrollTop = 0;
  }, [contextIndex, view]);

  const showPrototypeToast = (message: string) => {
    setToast(message);
    window.setTimeout(() => setToast(""), 3600);
  };

  const rememberAsset = (id: string) => {
    setSelectedAssetId(id);
    recentAssetOpenSequence.current += 1;
    recentAssetOpenedAt.current[id] = recentAssetOpenSequence.current;
    setRecentAssetIds((current) => current.includes(id) ? current : [id, ...current].slice(0, 5));
  };

  const openCandidateVersion = (proposal: Proposal) => {
    setActiveWorkbenchTask(null);
    setVersionSourceRun(null);
    setVersionDetailRequest((current) => ({ key: `candidate:${proposal.id}`, epoch: current.epoch + 1 }));
    setView("releases");
    setContextIndex(0);
  };

  const openSourceActivity = () => {
    setActiveWorkbenchTask(null);
    setSourceInitialRunId(undefined);
    setView("sources");
    setContextIndex(2);
    setContextSelectionEpoch((epoch) => epoch + 1);
  };

  const openGlobalRun = (runId: string) => {
    setContextPanelOpen(true);
    setActiveWorkbenchTask(null);
    setAuditRunRequestId(runId);
    setView("settings");
    setContextIndex(4);
  };

  const startEmbeddingRebuild = (modelId: string) => {
    const run: EmbeddingRebuildRun = {
      id: "RUN-IDX-260901-1132",
      modelId,
      status: "运行中",
      progress: 42,
      phase: "生成向量",
      startedAt: "2026-09-01 11:32:00",
      duration: "1 分 18 秒",
      processedItems: 1436,
      totalItems: 3420,
    };
    setEmbeddingRebuildRun(run);
    showPrototypeToast(`向量索引重建任务已创建，正在使用 ${modelId} 处理全部知识目录。`);
  };

  const openEmbeddingRebuildRun = () => {
    if (!embeddingRebuildRun) return;
    setContextPanelOpen(true);
    setAuditRunRequestId(embeddingRebuildRun.id);
    setView("settings");
    setContextIndex(4);
  };

  const handleAuditRunRequest = useCallback(() => setAuditRunRequestId(undefined), []);

  const openAssetEvidence = () => {
    rememberAsset(assets[0].id);
    if (view !== "assets") settleRecentAssets();
    setKnowledgeRevisionRequest(null);
    setView("assets");
    setContextIndex(0);
    setAssetCatalogTypeRequest("全部");
    setAssetTabRequest("可信度");
    setAssetDetailRequest(true);
    setAssetTabRequestEpoch((epoch) => epoch + 1);
  };

  const openRunCandidates = (runId: string) => {
    setContextPanelOpen(true);
    setVersionSourceRun(runId);
    setVersionDetailRequest((current) => ({ key: null, epoch: current.epoch + 1 }));
    setView("releases");
    setContextIndex(0);
  };

  const openAssetDetail = (id: string) => {
    rememberAsset(id);
    if (view !== "assets") settleRecentAssets();
    setKnowledgeRevisionRequest(null);
    setView("assets");
    setContextIndex(0);
    setAssetCatalogTypeRequest("全部");
    setAssetTabRequest("概览");
    setAssetDetailRequest(true);
    setAssetTabRequestEpoch((epoch) => epoch + 1);
  };

  const openKnowledgeRevision = (request: KnowledgeRevisionRequest) => {
    rememberAsset(request.assetId);
    if (view !== "assets") settleRecentAssets();
    setKnowledgeRevisionRequest(request);
    setView("assets");
    setContextIndex(0);
    setAssetCatalogTypeRequest("全部");
    setAssetTabRequest("定义");
    setAssetDetailRequest(true);
    setAssetTabRequestEpoch((epoch) => epoch + 1);
  };

  const submitKnowledgeRevision = (submission: KnowledgeRevisionSubmission) => {
    const asset = assets.find((item) => item.id === submission.assetId);
    if (!asset) return;
    const changesExpression = submission.changes.some((change) => change.field === "spec.expression");
    const proposal: Proposal = {
      id: "PROP-SESSION-001",
      title: submission.title,
      assetName: asset.name,
      assetId: asset.id,
      author: "当前用户",
      createdAt: "刚刚",
      risk: changesExpression ? "中风险" : "低风险",
      summary: submission.summary,
      changes: submission.changes,
      validations: [
        { name: "Schema references", state: "passed", detail: `${submission.changes.length} 项知识字段符合 ${asset.revisionRecord.schemaVersion}` },
        { name: "Evidence links", state: "passed", detail: "来源资料保持只读，候选主张保留字段级证据引用" },
        { name: "Consumer impact", state: "passed", detail: `${asset.consumerBindings.length} 个消费者已纳入版本审核范围` },
        { name: "Policy G1", state: "passed", detail: `修订原因已记录：${submission.reason}` },
      ],
      impact: asset.consumers.length > 0 ? asset.consumers.map((consumer) => consumer.name) : ["当前没有生产消费者"],
    };
    setSessionProposal(proposal);
    setKnowledgeRevisionRequest(null);
    openCandidateVersion(proposal);
    showPrototypeToast(`${asset.name} 知识修订已提交为候选 ${candidateTargetFor(proposal)?.revision}，等待资产负责人审核。`);
  };

  const onDecision = (decision: "批准" | "退回") => {
    if (!reviewing) return;
    setDecisions((current) => ({ ...current, [reviewing.id]: decision }));
    setToast(`${reviewing.assetName} 候选版本已在本次原型会话中标记为${decision}，刷新后重置。`);
    setReviewing(null);
  };

  const navigate: NavigateToView = (nextView, nextContextIndex) => {
    setContextPanelOpen(true);
    setKnowledgeRevisionRequest(null);
    setActiveWorkbenchTask(null);
    setView(nextView);
    if (nextView === "assets" && view !== "assets") settleRecentAssets();
    if (nextView === "assets" && (nextContextIndex === undefined || nextContextIndex === 0)) {
      setAssetTabRequest("概览");
      setAssetDetailRequest(false);
      setAssetCatalogTypeRequest("全部");
      setAssetTabRequestEpoch((epoch) => epoch + 1);
    }
    if (nextView === "releases") {
      setVersionSourceRun(null);
      setContextIndex(0);
      setVersionDetailRequest((current) => ({ key: null, epoch: current.epoch + 1 }));
    }
    else if (nextContextIndex !== undefined) setContextIndex(nextContextIndex);
    else if (nextView === "ask") setContextIndex(1);
    else setContextIndex(0);
  };

  const selectContext = (index: number) => {
    setContextSelectionEpoch((epoch) => epoch + 1);
    if (view === "releases") {
      setVersionSourceRun(null);
      setContextIndex(0);
      setVersionDetailRequest((current) => ({ key: null, epoch: current.epoch + 1 }));
      return;
    }
    setContextIndex(index);
    if (view === "ask" && index === 0) setAskSessionKey((key) => key + 1);
    const contextAnchors: Partial<Record<ViewId, string[]>> = {
      overview: [".lifecycle-rail", ".attention-panel", ".graph-panel"],
    };
    const anchor = contextAnchors[view]?.[index];
    if (anchor) {
      window.requestAnimationFrame(() => {
        const target = document.querySelector(anchor);
        if (target && "scrollIntoView" in target && typeof target.scrollIntoView === "function") {
          target.scrollIntoView({ behavior: "smooth", block: "start" });
        }
      });
    }
  };

  const runDiscovery = () => {
    if (discoveryState === "running") return;
    setDiscoveryState("running");
    showPrototypeToast("接入运行已开始，正在读取 2 个数据来源的最新快照。");
    discoveryTimer.current = window.setTimeout(() => {
      setDiscoveryState("complete");
      showPrototypeToast("接入运行已完成：识别 8 项来源变化，并已在全局运行中心触发知识增量构建。");
    }, 450);
  };

  const publishCandidate = () => {
    if (!publishing) return;
    setPublishedVersion("release-2026.08.4-session");
    setPublishedCandidateId(publishing.id);
    setPublishing(null);
    setRollbackPerformed(false);
    showPrototypeToast(`模拟发布完成：${publishing.assetName}已切换到 ${candidateTargetFor(publishing)?.revision}，其他语义资产版本保持不变。`);
  };

  const renameConversationTitle = (title: string) => {
    if (view !== "ask") return;
    setAskConversationTitles((current) => ({ ...current, [contextIndex]: title }));
  };

  const recentAssets = recentAssetIds.map((id) => assets.find((asset) => asset.id === id)).filter((asset): asset is Asset => Boolean(asset));
  const selectedAsset = assets.find((asset) => asset.id === selectedAssetId);
  const activeWorkbenchProposal = activeWorkbenchTask?.target === "candidate-region" ? proposals[2] : activeWorkbenchTask?.target === "candidate-aov" ? proposals[0] : activeWorkbenchTask?.target === "candidate-revenue" ? proposals[1] : null;
  const activeWorkbenchCandidate = activeWorkbenchProposal ? assetVersionCandidateFor(activeWorkbenchProposal) : undefined;
  const contextLabel = view === "overview" ? activeWorkbenchTask?.title ?? "待办" : view === "assets" ? (assetDetailRequest ? selectedAsset?.name : "知识目录") : view === "ask" ? askConversationTitles[contextIndex] ?? contextItems[view][contextIndex]?.label : contextItems[view][contextIndex]?.label;

  return (
    <CapabilityProvider>
    <div className={contextPanelOpen ? "app-shell" : "app-shell context-panel-collapsed"} style={{ "--context-panel-width": `${contextPanelOpen ? contextPanelWidth : 0}px` } as React.CSSProperties}>
      <ActivityRail view={view} onChange={navigate} />
      {contextPanelOpen && <ContextPanel view={view} activeIndex={contextIndex} width={contextPanelWidth} recentAssets={recentAssets} activeAssetId={selectedAssetId} assetDetailOpen={assetDetailRequest} activeWorkbenchTask={activeWorkbenchTask} askConversationTitles={askConversationTitles} conversationSearchRequestEpoch={conversationSearchRequestEpoch} onSelect={selectContext} onOpenRecentAsset={openAssetDetail} onOpenWorkbenchTask={setActiveWorkbenchTask} onOpenCatalogSearch={openCatalogSearch} onOpenWorkbenchSearch={openWorkbenchSearch} onCollapse={() => setContextPanelOpen(false)} onResize={setContextPanelWidth} />}
      <div className="workspace">
        <Topbar key={`${view}-${contextLabel ?? ""}`} view={view} contextLabel={contextLabel} onNewConversation={() => { setAskSessionKey((key) => key + 1); setContextIndex(0); }} onRenameContext={view === "ask" ? renameConversationTitle : undefined} onBack={view === "overview" && activeWorkbenchTask ? () => setActiveWorkbenchTask(null) : view === "assets" && assetDetailRequest ? () => { setKnowledgeRevisionRequest(null); setAssetDetailRequest(false); setAssetDetailBackRequestEpoch((epoch) => epoch + 1); } : view === "sources" && contextIndex === 2 && sourceRunDetailOpen ? () => setSourceRunBackRequestEpoch((epoch) => epoch + 1) : view === "releases" && releaseDetailOpen ? () => setReleaseDetailBackRequestEpoch((epoch) => epoch + 1) : undefined} backLabel={view === "overview" ? "返回待办" : view === "assets" ? "返回知识目录" : view === "releases" ? "返回资产版本列表" : "返回接入运行"} />
        <main ref={workspaceRef} className="workspace-canvas">
          {view === "ask" && <AskView key={`${askSessionKey}-${contextIndex}`} conversationIndex={contextIndex} activeRelease={publishedVersion || assetVersionReleases[0].id} onOpenEvidence={openAssetEvidence} onStartRevision={(fieldPath, context) => openKnowledgeRevision({ assetId: assets[0].id, fieldPath, origin: "ask", context })} />}
          {view === "sources" && <SourcesView discoveryState={discoveryState} focusIndex={contextIndex} navigationEpoch={contextSelectionEpoch} runDetailBackRequestEpoch={sourceRunBackRequestEpoch} initialRunId={sourceInitialRunId} onRunDetailOpenChange={setSourceRunDetailOpen} onOpenActivity={openSourceActivity} onOpenGlobalRun={openGlobalRun} onRunDiscovery={runDiscovery} onOpenGovernanceProposals={openRunCandidates} onNotify={showPrototypeToast} />}
          {view === "overview" && !activeWorkbenchTask && <OverviewView onOpenTask={setActiveWorkbenchTask} discoveryState={discoveryState} focusSearchRequestEpoch={workbenchSearchRequestEpoch} />}
          {view === "overview" && activeWorkbenchTask?.target === "source-run" && <SourcesView key={activeWorkbenchTask.id} discoveryState={discoveryState} focusIndex={2} navigationEpoch={0} runDetailBackRequestEpoch={0} initialRunId="RUN-240823-1844" onRunDetailOpenChange={setSourceRunDetailOpen} onOpenActivity={openSourceActivity} onOpenGlobalRun={openGlobalRun} onRunDiscovery={runDiscovery} onOpenGovernanceProposals={openRunCandidates} onNotify={showPrototypeToast} />}
          {view === "overview" && activeWorkbenchCandidate && <section className="view view-overview governance-detail-view workbench-detail-view"><CandidateVersionDetail candidate={activeWorkbenchCandidate} decision={decisions[activeWorkbenchCandidate.proposal.id]} publishedCandidateId={publishedCandidateId} publishedVersion={publishedVersion} onReview={() => setReviewing(activeWorkbenchCandidate.proposal)} onPublish={() => setPublishing(activeWorkbenchCandidate.proposal)} /></section>}
          {view === "assets" && <AssetsView key={assetTabRequestEpoch} selectedId={selectedAssetId} domainFilter="全部" requestedTab={assetTabRequest} requestedDetail={assetDetailRequest} requestedCatalogType={assetCatalogTypeRequest} requestedRevision={knowledgeRevisionRequest} focusSearchRequestEpoch={assetSearchRequestEpoch} detailBackRequestEpoch={assetDetailBackRequestEpoch} onSelect={rememberAsset} onDetailChange={setAssetDetailRequest} onNotify={showPrototypeToast} onRevisionSubmit={submitKnowledgeRevision} />}
          {view === "releases" && <ReleasesView key={versionDetailRequest.epoch} decisions={decisions} publishedVersion={publishedVersion} publishedCandidateId={publishedCandidateId} rollbackPerformed={rollbackPerformed} sessionProposal={sessionProposal} initialSelectedKey={versionDetailRequest.key} initialSourceRun={versionSourceRun} detailBackRequestEpoch={releaseDetailBackRequestEpoch} onDetailOpenChange={setReleaseDetailOpen} onReview={setReviewing} onPublish={setPublishing} onRollback={() => { setRollbackPerformed(true); showPrototypeToast("模拟回滚已完成，所选语义资产已切回上一修订。"); }} />}
          {view === "settings" && <SettingsView focusIndex={contextIndex} rebuildRun={embeddingRebuildRun} auditRunRequestId={auditRunRequestId} onAuditRunRequestHandled={handleAuditRunRequest} onStartEmbeddingRebuild={startEmbeddingRebuild} onOpenRebuildRun={openEmbeddingRebuildRun} onNotify={showPrototypeToast} />}
        </main>
      </div>
      {reviewing && <ReviewDialog proposal={reviewing} onClose={() => setReviewing(null)} onDecision={onDecision} />}
      {creatingProposal && <CreateProposalDialog onClose={() => setCreatingProposal(false)} onCreate={() => { setCreatingProposal(false); openCandidateVersion(proposals[0]); showPrototypeToast("会话草稿已加入客单价候选版本，等待版本级审核。"); }} />}
      {publishing && <PublishDialog proposal={publishing} onClose={() => setPublishing(null)} onPublish={publishCandidate} />}
      {commandOpen && <CommandPalette onClose={() => setCommandOpen(false)} onNavigate={navigate} onSelectAsset={openAssetDetail} onSelectProposal={(id) => { const proposal = proposals.find((item) => item.id === id); if (proposal) openCandidateVersion(proposal); }} onCreateProposal={() => setCreatingProposal(true)} />}
      {toast && <div className="toast" role="status"><CheckCircle2 size={17} /><span>{toast}</span></div>}
    </div>
    </CapabilityProvider>
  );
}

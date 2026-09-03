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
import { LoaderCircle } from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { useCatalogRuntime } from "./catalogRuntime";
import { CatalogWorkspaceControl, CreateCatalogAssetButton } from "./CatalogControls";
import { CapabilityProvider, useCan } from "./authorization";
import { AIProposalDialog } from "./AIProposalDialog";
import { assetVersionReleases as previewAssetReleases } from "./data";
import {
  GOVERNANCE_ERROR_CODES,
  type GovernancePolicyDecision,
  type GovernanceProposalDetail,
  type GovernanceProposalState,
  type GovernanceProposalSummary,
  type GovernanceReleaseDetail,
  type GovernanceReview,
  type GovernanceReviewBatchDetail,
  type GovernanceValidationRun,
  type GovernanceValidationSeverity,
} from "./governance";
import { GovernanceRuntimeProvider, useGovernanceRuntime } from "./governanceRuntime";
import { AccessControlView } from "./AccessControlView";
import { assetTypeProfileFor, evaluateAssetTypeRules, type AssetEmptyStateConfig } from "./assetTypeProfiles";
import { AuditRuntimeView } from "./AuditRuntimeView";
import { SourcesView } from "./JourneyViews";
import { IntegrationSettingsView } from "./IntegrationSettingsView";
import { AskView } from "./KnowledgeViews";
import { KnowledgeRevisionWorkbench } from "./KnowledgeRevisionWorkbench";
import { ModelConfigurationView, type EmbeddingRebuildRun } from "./ModelConfigurationView";
import { SemanticGraph, type OntologyPerspective } from "./SemanticGraph";
import type { Asset, AssetType, CapabilitySession, KnowledgeRevisionRequest, KnowledgeRevisionSubmission, NavigateToView, Proposal, ValidationState, ViewId } from "./types";

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

const assetTypes: AssetType[] = ["业务概念", "业务实体", "语义模型", "维度", "度量", "指标", "分群"];
const catalogObjectTypes: CatalogObjectType[] = [...assetTypes, "语义关系", "物理绑定", "JoinContract"];

function assetTypeFromCatalogType(type: CatalogObjectType | "全部") {
  return ({
    业务概念: "concept",
    业务实体: "entity",
    语义模型: "semantic_model",
    维度: "dimension",
    度量: "measure",
    指标: "metric",
    分群: "segment",
  } as const)[type as AssetType] ?? "";
}

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
  return ({ measures: "衡量", describes: "描述", depends_on: "依赖", derived_from: "来源于", filters_by: "按其筛选", synonym_of: "同义于", contains: "包含", broader_than: "上位于", narrower_than: "下位于", equivalent_to: "等价于", disjoint_with: "互斥于" } as Record<Asset["relations"][number]["type"], string>)[type];
}

function relationPlaneLabel(plane: Asset["relations"][number]["plane"]) {
  return ({ taxonomy: "概念层级", semantic: "语义关系", dependency: "依赖与影响" } as const)[plane];
}

function relationAssertionLabel(state: Asset["relations"][number]["assertionState"]) {
  return ({ asserted: "人工声明", inferred: "证据推导", candidate: "候选关系", deprecated: "已废弃" } as const)[state];
}

function cardinalityLabel(value: Asset["ontologyContext"]["relationConstraints"][number]["cardinality"]) {
  return ({ one_to_one: "一对一", one_to_many: "一对多", many_to_one: "多对一", many_to_many: "多对多" } as const)[value];
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

function knowledgeCatalogItemsFor(assets: Asset[]): KnowledgeCatalogItem[] {
  return assets.flatMap((asset) => {
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
    scope: `${asset.detailLoaded === false ? "关系待加载" : `${asset.relations.length} 条关系`} · ${asset.bindings.length} 个绑定`,
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
}

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

function ContextPanel({ view, activeIndex, width, recentAssets, activeAssetId, assetDetailOpen, activeWorkbenchTask, askConversationTitles, conversationSearchRequestEpoch, releasesItems, onSelect, onOpenRecentAsset, onOpenWorkbenchTask, onOpenCatalogSearch, onOpenWorkbenchSearch, onCollapse, onResize }: { view: ViewId; activeIndex: number; width: number; recentAssets: Asset[]; activeAssetId: string; assetDetailOpen: boolean; activeWorkbenchTask: WorkbenchTask | null; askConversationTitles: Record<number, string>; conversationSearchRequestEpoch: number; releasesItems: Array<{ label: string; meta: string; tone?: "warning" | "danger" }>; onSelect: (index: number) => void; onOpenRecentAsset: (id: string) => void; onOpenWorkbenchTask: (task: WorkbenchTask) => void; onOpenCatalogSearch: () => void; onOpenWorkbenchSearch: () => void; onCollapse: () => void; onResize: (width: number) => void }) {
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
        )) : <div className="context-search-empty"><Search size={16} /><strong>没有匹配会话</strong><span>尝试搜索其他会话标题。</span></div> : (view === "releases" ? releasesItems : contextItems[view]).map((item, index) => ({ item, index })).filter(({ index }) => view !== "settings" || index !== 1 || canViewAccessControl).map(({ item, index }) => (
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
        <CatalogWorkspaceControl />
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

function AssetRelations({ asset, focusedObjectId, onNotify, onStartRevision }: { asset: Asset; focusedObjectId?: string; onNotify: (message: string) => void; onStartRevision: (request: KnowledgeRevisionRequest) => void }) {
  const { assets } = useCatalogRuntime();
  const profile = assetTypeProfileFor(asset);
  const [perspective, setPerspective] = useState<OntologyPerspective>("semantic");
  const perspectiveOptions: Array<{ id: OntologyPerspective; label: string }> = [
    { id: "taxonomy", label: "概念层级" },
    { id: "semantic", label: "语义关系" },
    { id: "dependency", label: "依赖与影响" },
  ];
  return (
    <div className="asset-relations-layout">
      <section className="asset-ontology-context" aria-labelledby="asset-ontology-title">
        <header className="asset-ontology-heading"><div><span className="content-label">本体上下文</span><h3 id="asset-ontology-title">{asset.domain}本体</h3></div><div className="asset-ontology-actions"><span className={`asset-ontology-consistency asset-ontology-${asset.ontologyContext.consistencyState}`}><CheckCircle2 size={13} />{asset.ontologyContext.consistencyState === "consistent" ? "一致性通过" : "存在待确认关系"}</span><button type="button" className="secondary-button" onClick={() => onStartRevision({ assetId: asset.id, fieldPath: "ontology.relations", origin: "definition", context: `为 ${asset.name} 提出类型化本体关系修订。` })}><GitPullRequestArrow size={14} />提出关系修订</button></div></header>
        <p className="asset-view-focus">{profile.ontologyFocus}</p>
        <div className="asset-ontology-path"><span>领域路径</span>{asset.ontologyContext.domainPath.map((item, index) => <span key={item}><strong>{item}</strong>{index < asset.ontologyContext.domainPath.length - 1 && <ChevronRight size={12} />}</span>)}<span><strong>{asset.name}</strong></span></div>
        <div className="asset-ontology-viewbar"><div className="asset-ontology-segmented" role="group" aria-label="选择本体关系视角">{perspectiveOptions.map((option) => <button key={option.id} type="button" aria-pressed={perspective === option.id} onClick={() => setPerspective(option.id)}>{option.label}</button>)}</div><span>局部一跳 · 列表为权威事实</span></div>
        {asset.relations.length > 0 ? <SemanticGraph mode="lineage" perspective={perspective} assetName={asset.name} assetType={asset.type} assetRevision={asset.revision} ontologyRevision={asset.ontologyContext.revisionId} relations={asset.relations} parentConcepts={asset.ontologyContext.parentConcepts} upstream={asset.upstream} downstream={asset.downstream} consumerName={asset.consumers[0]?.name ?? "尚未绑定"} /> : <AssetEmptyState config={profile.emptyStates.relations} icon={Network} assetName={asset.name} onAction={onNotify} />}
        <div className="asset-ontology-revision"><span><ArrowUpDown size={14} /><strong>本体 revision</strong><code>{asset.ontologyContext.previousRevisionId ?? "初始版本"}</code><ChevronRight size={12} /><code>{asset.ontologyContext.revisionId}</code></span><span><strong>+{asset.ontologyContext.revisionDelta.addedRelations}</strong> 关系 · <strong>{asset.ontologyContext.revisionDelta.removedRelations}</strong> 移除 · <strong>{asset.ontologyContext.revisionDelta.changedConstraints}</strong> 约束调整</span><button type="button" onClick={() => onNotify(`正在比较 ${asset.ontologyContext.previousRevisionId ?? "初始版本"} → ${asset.ontologyContext.revisionId} 的本体差异。`)}>比较 revision<ChevronRight size={12} /></button></div>
        {asset.ontologyContext.consistencyIssues.length > 0 && <div className="asset-ontology-issues" role="status"><AlertTriangle size={14} /><span><strong>一致性检查需要处理</strong>{asset.ontologyContext.consistencyIssues.join("；")}</span></div>}
        <dl className="asset-ontology-summary"><div><dt>上位概念</dt><dd>{asset.ontologyContext.parentConcepts.join(" · ")}</dd></div><div><dt>直接关系</dt><dd>{asset.relations.length} 条</dd></div><div><dt>关系类型约束</dt><dd>{asset.ontologyContext.relationConstraints.length} 项</dd></div><div><dt>发布范围</dt><dd>{asset.ontologyContext.publishedIn ?? "候选本体"}</dd></div></dl>
      </section>
      {asset.relations.length > 0 && <section className="asset-relation-list" aria-labelledby="asset-relations-title">
        <header><div><span className="content-label">可审计明细</span><h3 id="asset-relations-title">类型化关系</h3></div><span>{asset.relations.length} 条当前 revision 关系</span></header>
        <div className="asset-relation-table-head" aria-hidden="true"><span>关系层与方向</span><span>谓词与状态</span><span>目标资产</span><span>证据与版本</span></div>
        <div className="asset-relation-cards">
          {asset.relations.map((relation) => { const targetAsset = assets.find((candidate) => candidate.id === relation.targetId); return <article key={relation.id} className={focusedObjectId === relation.id ? "asset-object-focused" : undefined} aria-current={focusedObjectId === relation.id ? "true" : undefined}><div className="relation-source"><span className={`relation-type-mark relation-plane-${relation.plane}`}><ArrowRight size={15} /></span><span><strong>{relationPlaneLabel(relation.plane)}</strong><small>{relation.direction === "outgoing" ? "出向 · 当前资产为来源" : "入向 · 当前资产为目标"}</small></span></div><div className="relation-predicate"><strong>{relationLabel(relation.type)}</strong><code>{relation.type}</code><small>{relationAssertionLabel(relation.assertionState)} · {relation.id}</small></div><div className="relation-target"><strong>{relation.targetName}</strong><small><code title={relation.targetId}>{targetAsset?.identity.key ?? relation.targetId}</code></small></div><div className="relation-evidence"><span><strong>{relation.evidence}</strong><small>{relation.release}</small></span><StatusBadge tone={relation.assertionState === "candidate" ? "warning" : relation.assertionState === "deprecated" ? "neutral" : "info"}>{relationAssertionLabel(relation.assertionState)}</StatusBadge></div></article>; })}
        </div>
      </section>}
      {asset.ontologyContext.relationConstraints.length > 0 && <section className="asset-ontology-constraints" aria-labelledby="asset-ontology-constraints-title">
        <header><div><span className="content-label">RelationType contract</span><h3 id="asset-ontology-constraints-title">关系类型约束</h3></div><span>由 {asset.ontologyContext.revisionId} 固定</span></header>
        <div className="asset-ontology-constraint-table" role="table" aria-label="本体关系类型约束"><div className="asset-ontology-constraint-head" role="row"><span role="columnheader">谓词与逆关系</span><span role="columnheader">允许端点</span><span role="columnheader">基数与推理</span><span role="columnheader">校验结果</span></div>{asset.ontologyContext.relationConstraints.map((constraint) => <article role="row" key={constraint.relationType}><div role="cell"><strong>{constraint.label}</strong><code>{constraint.relationType} ↔ {constraint.inverseLabel}</code></div><div role="cell"><strong>{constraint.sourceTypes.join(" / ")}</strong><small>→ {constraint.targetTypes.join(" / ")}</small></div><div role="cell"><strong>{cardinalityLabel(constraint.cardinality)}</strong><small>{constraint.reasoning === "symmetric" ? "对称推理" : "有向推理"}</small></div><div role="cell"><StatusBadge tone={constraint.validationState === "valid" ? "success" : "warning"}>{constraint.validationState === "valid" ? "约束通过" : "需要确认"}</StatusBadge><small>{constraint.validationDetail}</small></div></article>)}</div>
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

function AssetOntology({ asset, focusedObject, onNotify, onStartRevision }: { asset: Asset; focusedObject?: KnowledgeCatalogItem; onNotify: (message: string) => void; onStartRevision: (request: KnowledgeRevisionRequest) => void }) {
  return <div className="asset-ontology-view"><AssetRelations asset={asset} focusedObjectId={focusedObject?.key} onNotify={onNotify} onStartRevision={onStartRevision} /></div>;
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

function AssetGovernanceEvidence({ assetId }: { assetId: string }) {
  const governance = useGovernanceRuntime();
  const proposals = useMemo(() => governance.proposals.filter((proposal) => proposal.assetId === assetId && proposal.state !== "draft"), [assetId, governance.proposals]);
  const { decisions, loadPolicyDecision, loadValidationRuns, validationRuns } = governance;
  useEffect(() => {
    for (const proposal of proposals) {
      if (!validationRuns[proposal.id]) void loadValidationRuns(proposal.id).catch(() => undefined);
      if (!decisions[proposal.id] && proposal.state !== "rejected") void loadPolicyDecision(proposal.id).catch(() => undefined);
    }
  }, [decisions, loadPolicyDecision, loadValidationRuns, proposals, validationRuns]);
  if (proposals.length === 0) return null;
  return (
    <section aria-labelledby="asset-governance-title">
      <header className="asset-section-heading"><div><span className="content-label">治理流水线</span><h3 id="asset-governance-title">提案验证与策略决策</h3></div><span>{proposals.length} 项提案</span></header>
      <div className="asset-governance-proposals">{proposals.map((proposal) => {
        const runs = validationRuns[proposal.id] ?? [];
        const decision = decisions[proposal.id] ?? null;
        return <article className="asset-governance-proposal" key={proposal.id}>
          <header><strong>{proposal.title}</strong><code>{proposal.id}</code><StatusBadge tone="info">{proposalStateLabels[proposal.state]}</StatusBadge>{decision && <StatusBadge tone={riskTone(riskLabels[decision.riskLevel] ?? "待评估")}>{riskLabels[decision.riskLevel] ?? decision.riskLevel}</StatusBadge>}{decision && <StatusBadge tone="info">{routingLabels[decision.routing] ?? decision.routing}</StatusBadge>}</header>
          {decision && <p><code>{decision.matchedRuleId}</code> · {decision.explanation}</p>}
          <div className="validation-list">{runs.map((run) => (
            <div className={`validation-item validation-${runTone(run) === "danger" ? "failed" : runTone(run) === "warning" ? "warning" : "passed"}`} key={run.id}>
              <span>{runTone(run) === "danger" ? <CircleAlert size={15} /> : runTone(run) === "warning" ? <AlertTriangle size={15} /> : <CheckCircle2 size={15} />}</span>
              <div><strong>{run.validatorId} <code>v{run.validatorVersion}</code></strong><small>{runStateLabels[run.status]} · {run.results.map((result) => `${validationSeverityLabels[result.severity]} ${result.code}`).join(" · ") || "暂无结果"}</small></div>
            </div>
          ))}
          {runs.length === 0 && <div className="validation-item"><CircleDot size={15} /><div><strong>验证运行尚未产生结果</strong><small>提案提交后由治理服务执行。</small></div></div>}
          </div>
        </article>;
      })}</div>
    </section>
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
      <AssetGovernanceEvidence assetId={asset.id} />
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

function AssetsView({ selectedId, domainFilter, requestedTab, requestedDetail, requestedCatalogType, requestedRevision, focusSearchRequestEpoch, detailBackRequestEpoch, onSelect, onDetailChange, onNotify, onRevisionSubmit, onStartAIGeneration }: { selectedId: string; domainFilter: string; requestedTab: AssetTab; requestedDetail: boolean; requestedCatalogType: CatalogObjectType | "全部"; requestedRevision: KnowledgeRevisionRequest | null; focusSearchRequestEpoch: number; detailBackRequestEpoch: number; onSelect: (id: string) => void; onDetailChange: (open: boolean) => void; onNotify: (message: string) => void; onRevisionSubmit: (submission: KnowledgeRevisionSubmission) => Promise<void>; onStartAIGeneration: (assetId: string, fieldPath: string) => void }) {
  const { assets, ensureAsset, error: catalogError, setQuery: setCatalogQuery } = useCatalogRuntime();
  const knowledgeCatalogItems = useMemo(() => knowledgeCatalogItemsFor(assets), [assets]);
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
  const domains = useMemo(() => Array.from(new Set(assets.map((asset) => asset.domain))), [assets]);
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
  }, [assetStatus, assets, domain, isGovernedObjectType, knowledgeCatalogItems, normalizedQuery, sort, type]);
  const visibleChildCount = catalogGroups.reduce((total, group) => total + group.visibleChildren.length, 0);
  const selected = assets.find((asset) => asset.id === selectedId) ?? assets[0];
  const focusedObject = focusedObjectId ? knowledgeCatalogItems.find((item) => item.key === focusedObjectId) : undefined;
  const hasFilters = normalizedQuery.length > 0 || type !== "全部" || assetStatus !== "全部" || domain !== "全部";

  useEffect(() => {
    if (focusSearchRequestEpoch > 0) catalogSearchRef.current?.focus();
  }, [focusSearchRequestEpoch]);

  useEffect(() => {
    const semanticType = assetTypeFromCatalogType(type);
    setCatalogQuery(query.trim(), semanticType);
  }, [query, setCatalogQuery, type]);

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
    void ensureAsset(item.assetId);
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
          {revisionRequest ? <KnowledgeRevisionWorkbench key={`${revisionRequest.assetId}:${revisionRequest.fieldPath}:${revisionRequest.claimId ?? "new"}`} asset={selected} request={revisionRequest} onCancel={() => setRevisionRequest(null)} onNotify={onNotify} onSubmit={onRevisionSubmit} onStartAIGeneration={() => onStartAIGeneration(selected.id, revisionRequest.fieldPath)} /> : <>
            <div className="asset-tabs" role="tablist" aria-label="资产详情视图">
              {visibleAssetTabs.map((item) => (
                <button key={item} role="tab" type="button" aria-selected={activeTab === item} onClick={() => openAssetTab(item)}>{item}</button>
              ))}
            </div>
            <div className="asset-tab-content" role="tabpanel">
              {activeTab === "概览" && <AssetOverview asset={selected} onOpenTab={openAssetTab} />}
              {activeTab === "定义" && <AssetDefinition asset={selected} onNotify={onNotify} onStartRevision={setRevisionRequest} />}
              {activeTab === "本体关系" && <AssetOntology asset={selected} focusedObject={focusedObject} onNotify={onNotify} onStartRevision={setRevisionRequest} />}
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
      {catalogError && <div className="catalog-runtime-error" role="alert"><CircleAlert size={16} /><span>{catalogError}</span></div>}
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
          <CreateCatalogAssetButton onCreated={(assetId) => { onSelect(assetId); void ensureAsset(assetId); onDetailChange(true); setMode("detail"); }} />
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
                    <span className="asset-row-scope"><strong>{asset.detailLoaded === false ? "打开后加载关系" : `${asset.relations.length} 关系`} · {asset.bindings.length} 绑定</strong><small>{asset.joinContracts.length} 个 JoinContract</small></span>
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

type ProposalRisk = Proposal["risk"];

const riskLabels: Record<string, ProposalRisk> = { low: "低风险", medium: "中风险", high: "高风险" };
const routingLabels: Record<string, string> = { expert: "专家评审", batch: "批量确认" };
const proposalStateLabels: Record<GovernanceProposalState, string> = { draft: "草稿", proposed: "验证进行中", validating: "验证进行中", in_review: "待审核", released: "已发布", rejected: "已退回" };
const validationSeverityLabels: Record<GovernanceValidationSeverity, string> = { blocker: "阻断", warning: "需关注", info: "通过", not_applicable: "不适用" };
const runStateLabels: Record<GovernanceValidationRun["status"], string> = { running: "运行中", succeeded: "已完成", failed: "失败", cancelled: "已取消" };

function governanceTimestamp(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function governanceValueLabel(value: unknown): string {
  if (value === undefined || value === null) return "（无）";
  if (Array.isArray(value)) return value.map((item) => String(item)).join("\n");
  if (typeof value === "object") return JSON.stringify(value, null, 2);
  return String(value);
}

function riskTone(risk: ProposalRisk): "danger" | "warning" | "success" | "neutral" {
  if (risk === "高风险") return "danger";
  if (risk === "中风险") return "warning";
  if (risk === "低风险") return "success";
  return "neutral";
}

interface GovernanceCandidate {
  key: string;
  proposal: GovernanceProposalSummary;
  asset?: Asset;
  targetRevision: string;
  baseRevision: string;
  decision: GovernancePolicyDecision | null;
  review: GovernanceReview | null;
}

function governanceCandidateFor(proposal: GovernanceProposalSummary, assets: Asset[], decisions: Record<string, GovernancePolicyDecision>, reviews: Record<string, GovernanceReview>): GovernanceCandidate {
  const asset = assets.find((item) => item.id === (proposal.assetId ?? proposal.targetObjectId));
  const baseSequence = asset?.revisionRecord.sequence ?? 0;
  return {
    key: `candidate:${proposal.id}`,
    proposal,
    asset,
    targetRevision: asset ? `@${baseSequence + 1}` : proposal.baseRevisionId || "候选版本",
    baseRevision: asset ? `@${baseSequence}` : proposal.baseRevisionId || "当前修订",
    decision: decisions[proposal.id] ?? null,
    review: reviews[proposal.id] ?? null,
  };
}

const reviewableStates: GovernanceProposalState[] = ["proposed", "validating", "in_review", "rejected"];

function candidateStatusFor(candidate: GovernanceCandidate): { label: string; tone: "success" | "warning" | "danger" | "neutral" } {
  if (candidate.proposal.state === "rejected") return { label: "已退回", tone: "danger" };
  if (candidate.proposal.state === "released") return { label: "已发布", tone: "success" };
  if (candidate.proposal.state === "proposed" || candidate.proposal.state === "validating") return { label: "验证进行中", tone: "warning" };
  if (candidate.review?.decision === "approved") return { label: "待发布", tone: "success" };
  return { label: "待审核", tone: "warning" };
}

function runNeedsAttention(run: GovernanceValidationRun): boolean {
  if (run.status !== "succeeded") return true;
  return run.results.some((result) => result.severity === "blocker" || result.severity === "warning");
}

function runIsPassed(run: GovernanceValidationRun): boolean {
  return run.status === "succeeded" && !run.results.some((result) => result.severity === "blocker");
}

function runTone(run: GovernanceValidationRun): "success" | "warning" | "danger" {
  if (run.status === "failed" || run.results.some((result) => result.severity === "blocker")) return "danger";
  if (run.status === "running" || run.results.some((result) => result.severity === "warning")) return "warning";
  return "success";
}

function GovernanceCandidateDetail({ candidate, publishedRelease, onReview, onPublish }: { candidate: GovernanceCandidate; publishedRelease: GovernanceReleaseDetail | null; onReview: () => void; onPublish: () => void }) {
  const governance = useGovernanceRuntime();
  const canReview = useCan("proposal.review");
  const canPublish = useCan("release.publish");
  const [detail, setDetail] = useState<GovernanceProposalDetail | null>(null);
  const [detailError, setDetailError] = useState("");
  const [runs, setRuns] = useState<GovernanceValidationRun[]>(governance.validationRuns[candidate.proposal.id] ?? []);
  const [runsError, setRunsError] = useState("");
  const [decision, setDecision] = useState<GovernancePolicyDecision | null>(candidate.decision);
  const [activeReviewTab, setActiveReviewTab] = useState<"diff" | "source" | "impact">("diff");
  const [gateCollapsed, setGateCollapsed] = useState(false);
  const { asset, proposal } = candidate;
  const isPublished = proposal.state === "released" || publishedRelease?.originProposalId === proposal.id;

  const { loadProposal, loadPolicyDecision: loadDecision, loadValidationRuns } = governance;

  useEffect(() => {
    let cancelled = false;
    loadProposal(proposal.id)
      .then((loaded) => { if (!cancelled) setDetail(loaded); })
      .catch((reason: Error) => { if (!cancelled) setDetailError(reason.message); });
    loadDecision(proposal.id)
      .then((loaded) => { if (!cancelled) setDecision(loaded); })
      .catch(() => undefined);
    return () => { cancelled = true; };
  }, [loadDecision, loadProposal, proposal.id]);

  useEffect(() => {
    let cancelled = false;
    let timer: number | null = null;
    const tick = async () => {
      try {
        const fetched = await loadValidationRuns(proposal.id);
        if (cancelled) return;
        setRuns(fetched);
        setRunsError("");
        const summary = await loadProposal(proposal.id);
        if (cancelled) return;
        setDetail(summary);
        const stillRunning = fetched.some((run) => run.status === "running") || summary.state === "validating";
        if (stillRunning) timer = window.setTimeout(() => void tick(), 1600);
      } catch (reason) {
        if (!cancelled) setRunsError(reason instanceof Error ? reason.message : "验证运行加载失败。");
      }
    };
    void tick();
    return () => {
      cancelled = true;
      if (timer !== null) window.clearTimeout(timer);
    };
  }, [loadProposal, loadValidationRuns, proposal.id]);

  const attentionRuns = runs.filter(runNeedsAttention);
  const passedRuns = runs.filter(runIsPassed);
  const status = isPublished ? "已发布" : candidateStatusFor(candidate).label;
  const statusTone = isPublished ? "success" : candidateStatusFor(candidate).tone;
  const risk = decision ? riskLabels[decision.riskLevel] ?? "待评估" : "待评估";
  const author = proposal.createdBy;

  if (detailError && !detail) {
    return <section className="release-detail governance-release-detail" aria-label={`${proposal.title} 候选资产版本详情`}>
      <div className="governance-load-error" role="alert"><CircleAlert size={18} /><span><strong>候选版本加载失败</strong>{detailError}</span></div>
    </section>;
  }

  return (
    <section className="release-detail governance-release-detail asset-version-detail-v2 candidate-version-detail" aria-label={`${asset?.name ?? proposal.title} ${candidate.targetRevision} 候选资产版本详情`}>
      <header className="candidate-review-summary">
        <div className="candidate-review-title">
          <div><span className="panel-kicker">候选资产版本 · <code>{asset?.key ?? proposal.targetObjectId}</code></span><h2>{asset?.name ?? proposal.title} <span>{candidate.targetRevision}</span></h2><p>{proposal.summary}</p></div>
        </div>
        <dl className="candidate-review-facts">
          <div><dt>版本范围</dt><dd>{candidate.baseRevision} → {candidate.targetRevision}</dd></div>
          <div><dt>发布门禁</dt><dd>{runs.length > 0 ? `${passedRuns.length}/${runs.length} 通过` : "等待验证执行"}</dd></div>
          <div><dt>影响对象</dt><dd>{asset ? asset.consumers.length : 0} 个</dd></div>
          <div><dt>风险等级</dt><dd>{risk}</dd></div>
        </dl>
        <div className="candidate-review-summary-actions">
          <StatusBadge tone={statusTone}>{status}</StatusBadge>
          {decision && <StatusBadge tone="info">{routingLabels[decision.routing] ?? decision.routing}</StatusBadge>}
          {!isPublished && candidateStatusFor(candidate).label === "待发布" && canPublish ? <button className="primary-button" type="button" onClick={onPublish}><PackageCheck size={16} />发布 {candidate.targetRevision}</button> : !isPublished && canReview && proposal.state === "in_review" ? <button className="primary-button" type="button" aria-label={`审核候选版本 ${asset?.name ?? proposal.title} ${candidate.targetRevision}`} onClick={onReview}><ShieldCheck size={16} />审核候选版本</button> : null}
        </div>
      </header>

      <section className="candidate-review-workspace" aria-label="版本评审">
        <header className="candidate-review-workspace-header">
          <div className="candidate-review-tabs" role="tablist" aria-label="候选版本评审内容">
            <button type="button" role="tab" aria-selected={activeReviewTab === "diff"} onClick={() => setActiveReviewTab("diff")}><span>版本差异</span><b>{detail?.changeSet?.length ?? 0}</b></button>
            <button type="button" role="tab" aria-selected={activeReviewTab === "source"} onClick={() => setActiveReviewTab("source")}><span>变更来源</span><b>1</b></button>
            <button type="button" role="tab" aria-selected={activeReviewTab === "impact"} onClick={() => setActiveReviewTab("impact")}><span>消费影响</span><b>{asset ? asset.consumers.length : 0}</b></button>
          </div>
        </header>
        <div className={gateCollapsed ? "candidate-review-workspace-body candidate-review-gate-collapsed" : "candidate-review-workspace-body"}>
          <main className="candidate-review-main" role="tabpanel" aria-label={activeReviewTab === "diff" ? "版本差异" : activeReviewTab === "source" ? "变更来源" : "消费影响"}>
            {activeReviewTab === "diff" && <section className="candidate-review-section candidate-version-diff" aria-labelledby="candidate-version-diff-title">
              <div className="candidate-card-heading"><div><span className="panel-kicker">版本差异</span><h3 id="candidate-version-diff-title">定义与计算变化</h3><p>以下变化会作为同一个资产版本整体生效。</p></div><span className="candidate-card-count">{candidate.baseRevision} → {candidate.targetRevision}</span></div>
              {detailError && <div className="governance-inline-error" role="alert"><CircleAlert size={15} />{detailError}</div>}
              <div className="candidate-diff-list">{(detail?.changeSet ?? []).map((change) => (
                <article className="diff-block" key={change.id}>
                  <header><code className="diff-field">{change.fieldPath}</code><span>{change.op === "add" ? "新增字段" : change.op === "remove" ? "移除字段" : "字段变更"}</span></header>
                  <div className="candidate-diff-compare">
                    <section className="candidate-diff-value candidate-diff-before"><span>当前 {candidate.baseRevision}</span><p>{governanceValueLabel(change.beforeValue)}</p></section>
                    <ArrowRight size={15} />
                    <section className="candidate-diff-value candidate-diff-after"><span>候选 {candidate.targetRevision}</span><p>{governanceValueLabel(change.afterValue)}</p></section>
                  </div>
                </article>
              ))}</div>
            </section>}

            {activeReviewTab === "source" && <section className="candidate-review-section candidate-change-section" aria-labelledby="candidate-change-title">
              <div className="candidate-card-heading"><div><span className="panel-kicker">版本来源</span><h3 id="candidate-change-title">包含的变更事项</h3><p>候选版本由以下变更事项生成。</p></div><span className="candidate-card-count">1 项</span></div>
              <article className="candidate-change-record">
                <span><GitPullRequestArrow size={17} /></span>
                <div><strong>{proposal.title}</strong><small><code>{proposal.id}</code> · {author} · {governanceTimestamp(proposal.createdAt)}</small><p>{proposal.agentRunId ? "AI 归因提案 · agentRun " + proposal.agentRunId : "人工知识修订"}</p></div>
                <StatusBadge tone={riskTone(risk)}>{risk}</StatusBadge>
              </article>
            </section>}

            {activeReviewTab === "impact" && <section className="candidate-review-section candidate-version-impact" aria-labelledby="candidate-impact-title">
              <div className="candidate-card-heading"><div><span className="panel-kicker">消费影响</span><h3 id="candidate-impact-title">版本切换影响范围</h3><p>审批前确认受影响消费者和迁移边界。</p></div><span className="candidate-card-count">{asset ? asset.consumers.length : 0} 个对象</span></div>
              {asset && asset.consumers.length > 0 ? <div className="candidate-impact-list">{asset.consumers.map((consumer) => <span key={consumer.name}><Users size={15} /><strong>{consumer.name}</strong></span>)}</div> : <div className="empty-inline">当前没有生产消费者。</div>}
            </section>}
          </main>

          <aside className={gateCollapsed ? "candidate-review-inspector candidate-review-inspector-collapsed" : "candidate-review-inspector"} aria-label="候选版本治理信息">
            <header className="candidate-review-inspector-header">
              {!gateCollapsed && <div><span className="panel-kicker">发布门禁</span><h3 id="candidate-validation-title">验证与审批</h3></div>}
              <button className="icon-button" type="button" aria-label={gateCollapsed ? "展开发布门禁" : "收起发布门禁"} title={gateCollapsed ? "展开发布门禁" : "收起发布门禁"} onClick={() => setGateCollapsed((collapsed) => !collapsed)}>{gateCollapsed ? <PanelRightOpen size={16} /> : <PanelRightClose size={16} />}</button>
            </header>
            {gateCollapsed ? <span className={attentionRuns.length > 0 ? "candidate-gate-collapsed-status candidate-gate-collapsed-status-attention" : "candidate-gate-collapsed-status"} title={runs.length > 0 ? `${passedRuns.length}/${runs.length} 项门禁通过` : "等待验证执行"}>{attentionRuns.length > 0 ? attentionRuns.length : runs.length}</span> : <section className="candidate-validation-section" aria-labelledby="candidate-validation-title">
              <div className="candidate-validation-count"><span>验证运行</span><strong>{runs.length > 0 ? `${passedRuns.length}/${runs.length}` : "等待执行"}</strong></div>
              {runsError && <div className="governance-inline-error" role="alert"><CircleAlert size={15} />{runsError}</div>}
              <div className={attentionRuns.length > 0 ? "candidate-gate-summary candidate-gate-summary-attention" : "candidate-gate-summary"}><span>{attentionRuns.length > 0 ? <AlertTriangle size={16} /> : runs.length === 0 ? <CircleDot size={16} /> : <CheckCircle2 size={16} />}</span><div><strong>{attentionRuns.length > 0 ? `${attentionRuns.length} 项需要确认` : runs.length === 0 ? "验证运行尚未产生结果" : "全部验证通过"}</strong><small>{attentionRuns.length > 0 ? "优先处理异常，再决定是否发布。" : runs.length === 0 ? "提案提交后治理服务正在执行验证。" : "可以进入资产负责人审核。"}</small></div></div>
              <div className="validation-list candidate-attention-validations">{runs.map((run) => (
                <div className={`validation-item validation-${runTone(run) === "danger" ? "failed" : runTone(run) === "warning" ? "warning" : "passed"}`} key={run.id}>
                  <span>{runTone(run) === "danger" ? <CircleAlert size={15} /> : runTone(run) === "warning" ? <AlertTriangle size={15} /> : <CheckCircle2 size={15} />}</span>
                  <div><strong>{run.validatorId} <code>v{run.validatorVersion}</code></strong><small>{runStateLabels[run.status]} · {run.results.map((result) => `${validationSeverityLabels[result.severity]} ${result.code}`).join(" · ") || "暂无结果"}</small></div>
                </div>
              ))}</div>
              {runs.some((run) => run.results.some((result) => result.message)) && <details className="candidate-passed-validations"><summary><span><FileCheck2 size={15} />验证结果明细</span><span>{runs.reduce((total, run) => total + run.results.length, 0)} 条<ChevronDown size={14} /></span></summary><div className="validation-list">{runs.flatMap((run) => run.results.map((result) => (
                <div className={`validation-item validation-${result.severity === "blocker" ? "failed" : result.severity === "warning" ? "warning" : "passed"}`} key={result.id}>
                  <span>{result.severity === "blocker" ? <CircleAlert size={15} /> : result.severity === "warning" ? <AlertTriangle size={15} /> : <CheckCircle2 size={15} />}</span>
                  <div><strong>{validationSeverityLabels[result.severity]} · {result.code}</strong><small>{result.message}</small></div>
                </div>
              )))}</div></details>}
              {decision && <section className="candidate-policy-decision" aria-label="策略决策">
                <header><span className="content-label">策略决策</span><code>{decision.ruleVersion}</code></header>
                <dl><div><dt>风险等级</dt><dd><StatusBadge tone={riskTone(riskLabels[decision.riskLevel] ?? "待评估")}>{riskLabels[decision.riskLevel] ?? decision.riskLevel}</StatusBadge></dd></div><div><dt>评审路由</dt><dd>{routingLabels[decision.routing] ?? decision.routing}</dd></div><div><dt>匹配规则</dt><dd><code>{decision.matchedRuleId}</code></dd></div><div><dt>决策原因</dt><dd><code>{decision.reasonCode}</code></dd></div></dl>
                <p>{decision.explanation}</p>
              </section>}
              {publishedRelease && <section className="candidate-release-record" aria-label="发布记录">
                <header><span className="content-label">发布记录</span><code>#{publishedRelease.sequence}</code></header>
                <dl><div><dt>发布批次</dt><dd><code>{publishedRelease.id}</code></dd></div><div><dt>清单摘要</dt><dd><code title={publishedRelease.manifestDigest}>{publishedRelease.manifestDigest.slice(0, 18)}…</code></dd></div>{publishedRelease.rolledBackToReleaseId && <div><dt>回滚来源</dt><dd><code>{publishedRelease.rolledBackToReleaseId}</code></dd></div>}</dl>
              </section>}
              <div className="candidate-review-decision-note"><strong>{isPublished ? `${asset?.name ?? proposal.title} ${candidate.targetRevision} 已发布为不可变版本` : candidate.review?.decision === "approved" ? "审核通过，可以发布整个候选版本" : proposal.state === "rejected" ? "提案已退回" : "审核针对完整资产版本"}</strong><span>{isPublished ? publishedRelease ? `发布批次 ${publishedRelease.id} · 序列 #${publishedRelease.sequence}。` : "发布记录已由治理服务创建。" : `${asset?.owner ?? "资产负责人"} 需要基于版本差异、验证证据和消费影响完成决策。`}</span></div>
            </section>}
          </aside>
        </div>
      </section>
    </section>
  );
}

function previewAssetConsumersFor(release: GovernanceReleaseDetail) {
  const assetId = release.manifest.assets[0]?.assetId;
  if (!assetId) return [];
  const match = previewAssetReleases.find((item) => item.assetId === assetId && item.consumers.length > 0);
  return match?.consumers ?? [];
}

function ReleaseInspectorDialog({ mode, release, revisionLabel, onClose }: { mode: "compare" | "manifest" | "policy"; release: GovernanceReleaseDetail; revisionLabel: string; onClose: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const title = mode === "compare" ? "发布与注册表比较" : mode === "manifest" ? `发布 #${release.sequence} 清单` : "审核与发布规则";
  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog inspector-dialog" role="dialog" aria-modal="true" aria-labelledby="inspector-dialog-title">
        <header><div><span className="panel-kicker">发布检查</span><h2 id="inspector-dialog-title">{title}</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label={`关闭${title}`} onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body">
          {mode === "compare" && <div className="release-compare"><div><span>注册表当前 revision</span><strong>{revisionLabel}</strong><small>当前注册表指针</small></div><ArrowRight size={18} /><div><span>发布固定的 revision</span><strong>{revisionLabel}</strong><small>{release.id} · #{release.sequence}</small></div><ul>{release.manifest.assets.map((asset) => <li key={`${asset.assetId}:${asset.revisionId}`}><code title={asset.revisionId}>{asset.revisionId}</code></li>)}</ul></div>}
          {mode === "manifest" && <div className="manifest-preview"><p>该清单固定发布产生的资产修订与治理对象版本；清单摘要是发布内容的规范化摘要。</p><pre>{JSON.stringify({ release_id: release.id, sequence: release.sequence, state: release.state, manifest_digest: release.manifestDigest, origin_proposal_id: release.originProposalId ?? null, rolled_back_to_release_id: release.rolledBackToReleaseId ?? null, published_by: release.publishedBy, assets: release.manifest.assets.map((asset) => ({ asset_id: asset.assetId, revision_id: asset.revisionId, position: asset.position })), objects: release.manifest.objects.map((object) => ({ object_type: object.objectType, object_id: object.objectId, version: object.version, position: object.position })) }, null, 2)}</pre></div>}
          {mode === "policy" && <div className="policy-list"><div><ShieldCheck size={18} /><span><strong>至少一个变更事项通过验证</strong><small>存在阻断发现的事项不能进入发布。</small></span></div><div><Users size={18} /><span><strong>独立主体完成审核与发布</strong><small>发布者不能是提案作者，也不能是唯一批准评审人（服务端强制）。</small></span></div><div><PackageCheck size={18} /><span><strong>发布生成不可变版本</strong><small>后续变更必须创建新版本，回滚以新发布记录表达。</small></span></div></div>}
        </div>
        <footer><span /><button className="primary-button" type="button" onClick={onClose}>完成</button></footer>
      </section>
    </div>
  );
}

function ReleaseDetail({ release, originProposal, revisionLabels, isLatest, rolledBackByReleaseId, onManifest, onCompare, onRollback }: { release: GovernanceReleaseDetail; originProposal: GovernanceProposalDetail | null; revisionLabels: Record<string, string>; isLatest: boolean; rolledBackByReleaseId: string | null; onManifest: () => void; onCompare: () => void; onRollback: (releaseId: string) => void }) {
  const canRollback = useCan("release.rollback");
  const primaryAsset = release.manifest.assets[0];
  const assetLabel = primaryAsset ? revisionLabels[primaryAsset.revisionId] ?? primaryAsset.revisionId : release.id;
  const previewConsumers = previewAssetConsumersFor(release);
  const isRollbackRelease = Boolean(release.rolledBackToReleaseId);
  return (
    <section className="release-detail governance-release-detail asset-version-detail-v2" aria-label={`发布 #${release.sequence} 详情`}>
      <header className="release-detail-header">
        <div className="release-seal"><PackageCheck size={24} /></div>
        <div><span className="panel-kicker">不可变发布记录 · <code>{release.id}</code></span><h2>{originProposal?.title ?? "发布记录"} · {assetLabel}</h2><p>发布序列 #{release.sequence} · {isRollbackRelease ? "回滚发布" : "标准发布"} · 由治理服务原子提交。</p></div>
        <div className="release-detail-header-actions"><StatusBadge tone={isLatest ? "success" : "neutral"}><BadgeCheck size={13} />{isLatest ? "当前版本" : "历史版本"}</StatusBadge><button className="text-button" type="button" onClick={onManifest}><FileCheck2 size={14} />发布清单</button><button className="text-button" type="button" onClick={onCompare}><RotateCcw size={14} />与注册表比较</button></div>
      </header>
      <div className="manifest-strip"><div><span>发布序列</span><strong>#{release.sequence}</strong></div><div><span>状态</span><strong>{release.state === "published" ? "已发布" : "已回滚"}</strong></div><div><span>发布批次</span><code>{release.id}</code></div><div><span>发布者 · 时间</span><strong>{release.publishedBy} · {governanceTimestamp(release.publishedAt)}</strong></div></div>
      {release.rolledBackToReleaseId && <div className="rollback-reference" role="status"><RotateCcw size={15} /><span>此发布回滚了 <code>{release.rolledBackToReleaseId}</code>；目标发布记录保持不变，回滚以新发布表达（P-003）。</span></div>}
      {rolledBackByReleaseId && <div className="rollback-reference" role="status"><RotateCcw size={15} /><span>此发布已被 <code>{rolledBackByReleaseId}</code> 回滚；注册表指针已由回滚发布切回。</span></div>}
      <div className="asset-version-detail-grid">
        <section className="release-changes">
          <div className="subsection-heading"><div><span className="panel-kicker">版本变更</span><h3>{originProposal ? originProposal.title : "发布清单"}</h3><p>{originProposal?.summary ?? "该发布的清单固定了以下不可变内容。"}</p></div><StatusBadge tone="success">{originProposal ? `${originProposal.changeSet?.length ?? 0} 项已验证变更` : `${release.manifest.assets.length} 项修订固定`}</StatusBadge></div>
          {originProposal?.changeSet ? <div className="change-list">{originProposal.changeSet.map((change) => <div key={change.id}><CheckCircle2 size={16} /><span><code>{change.fieldPath}</code> · {change.op === "add" ? "新增" : change.op === "remove" ? "移除" : "更新"}</span></div>)}</div> : <div className="change-list">{release.manifest.assets.map((asset) => <div key={`${asset.assetId}:${asset.revisionId}`}><CheckCircle2 size={16} /><span><code title={asset.assetId}>{asset.assetId}</code> → <code title={asset.revisionId}>{revisionLabels[asset.revisionId] ?? asset.revisionId}</code></span></div>)}{release.manifest.objects.map((object) => <div key={`${object.objectId}:${object.version}`}><CheckCircle2 size={16} /><span><code title={object.objectId}>{object.objectId}</code> · 版本 {object.version}</span></div>)}</div>}
        </section>
        <section className="release-bindings">
          <div className="subsection-heading"><div><span className="panel-kicker">清单固定</span><h3>此发布包含的不可变内容</h3><p>清单摘要 <code title={release.manifestDigest}>{release.manifestDigest.slice(0, 24)}…</code></p></div><span>{release.manifest.assets.length + release.manifest.objects.length} 项固定</span></div>
          <div className="asset-version-consumer-list">{release.manifest.assets.map((asset) => (
            <article key={`${asset.assetId}:${asset.revisionId}`}>
              <span className="consumer-mark"><Box size={18} /></span>
              <div><strong><code title={asset.assetId}>{asset.assetId}</code></strong><small>语义资产修订固定</small></div>
              <code title={asset.revisionId}>{revisionLabels[asset.revisionId] ?? asset.revisionId}</code>
              <StatusBadge tone="info">已固定</StatusBadge>
            </article>
          ))}
          {release.manifest.objects.map((object) => (
            <article key={`${object.objectId}:${object.version}`}>
              <span className="consumer-mark"><Link2 size={18} /></span>
              <div><strong><code title={object.objectId}>{object.objectId}</code></strong><small>{object.objectType} 治理对象</small></div>
              <code>版本 {object.version}</code>
              <StatusBadge tone="info">已固定</StatusBadge>
            </article>
          ))}
          {release.manifest.assets.length === 0 && release.manifest.objects.length === 0 && <div className="empty-inline">该发布没有固定内容。</div>}</div>
          {previewConsumers.length > 0 && <div className="release-consumers-preview"><span className="content-label">消费绑定 · 原型数据</span>{previewConsumers.map((consumer) => <article key={consumer.name}><span className="consumer-mark"><Box size={16} /></span><div><strong>{consumer.name}</strong><small>{consumer.kind} · 最近解析 {consumer.lastResolved}</small></div><code>{consumer.binding}</code></article>)}<small>真实消费绑定属于 M3 交付里程碑。</small></div>}
        </section>
      </div>
      <footer className="release-actions"><div><strong>回滚以新发布记录表达</strong><span>目标发布保持不变；回滚会恢复其固定修订之前的注册表指针。</span></div><button className="secondary-button" type="button" onClick={() => onRollback(release.id)} disabled={!canRollback || !isLatest || isRollbackRelease || Boolean(rolledBackByReleaseId)}><RotateCcw size={16} />回滚此发布</button></footer>
    </section>
  );
}

type AssetVersionFilter = "全部" | "待处理" | "当前版本" | "历史版本";

function ReleasesView({ initialSelectedKey, initialSourceRun, detailBackRequestEpoch, rollbackNotice, onDetailOpenChange, onReview, onPublish, onRollback, onAssembleBatches, onConfirmBatch }: { initialSelectedKey: string | null; initialSourceRun: string | null; detailBackRequestEpoch: number; rollbackNotice: GovernanceReleaseDetail | null; onDetailOpenChange: (open: boolean) => void; onReview: (candidate: GovernanceCandidate) => void; onPublish: (candidate: GovernanceCandidate) => void; onRollback: (releaseId: string) => void; onAssembleBatches: () => void; onConfirmBatch: (batch: GovernanceReviewBatchDetail) => void }) {
  const governance = useGovernanceRuntime();
  const { assets } = useCatalogRuntime();
  const canReview = useCan("proposal.review");
  const [selectedKey, setSelectedKey] = useState<string | null>(initialSelectedKey);
  const [selectedDetailBackEpoch, setSelectedDetailBackEpoch] = useState(detailBackRequestEpoch);
  const [inspector, setInspector] = useState<"compare" | "manifest" | "policy" | null>(null);
  const [filter, setFilter] = useState<AssetVersionFilter>("全部");
  const [query, setQuery] = useState(initialSourceRun ?? "");
  const [expandedBatchId, setExpandedBatchId] = useState<string | null>(null);
  const [batchDetail, setBatchDetail] = useState<GovernanceReviewBatchDetail | null>(null);
  const [batchError, setBatchError] = useState("");
  const [releaseDetail, setReleaseDetail] = useState<GovernanceReleaseDetail | null>(null);
  const [loadedReleaseId, setLoadedReleaseId] = useState("");
  const [releaseDetailError, setReleaseDetailError] = useState("");
  const [originProposal, setOriginProposal] = useState<GovernanceProposalDetail | null>(null);
  const [revisionLabels, setRevisionLabels] = useState<Record<string, string>>({});

  const candidates = useMemo(() => governance.proposals
    .filter((proposal) => proposal.state !== "draft")
    .map((proposal) => governanceCandidateFor(proposal, assets, governance.decisions, governance.reviews)), [assets, governance.decisions, governance.proposals, governance.reviews]);
  const candidateRows = useMemo(() => candidates.filter((candidate) => candidate.proposal.state !== "released"), [candidates]);
  const activeSelectedKey = selectedDetailBackEpoch === detailBackRequestEpoch ? selectedKey : null;
  const selectedCandidate = candidates.find((candidate) => candidate.key === activeSelectedKey);
  const selectedRelease = governance.releases.find((release) => `release:${release.id}` === activeSelectedKey) ?? null;
  const latestSequence = governance.releases.reduce((max, release) => Math.max(max, release.sequence), 0);
  const inspectorReleaseSource = selectedRelease ?? governance.releases[0] ?? null;
  const inspectorRelease = inspectorReleaseSource ? governance.releaseDetails[inspectorReleaseSource.id] ?? null : null;

  useEffect(() => {
    onDetailOpenChange(Boolean(selectedCandidate || selectedRelease));
    return () => onDetailOpenChange(false);
  }, [onDetailOpenChange, selectedCandidate, selectedRelease]);

  const { decisions, loadPolicyDecision } = governance;
  useEffect(() => {
    for (const candidate of candidates) {
      if (candidate.proposal.state === "rejected") continue;
      if (decisions[candidate.proposal.id]) continue;
      void loadPolicyDecision(candidate.proposal.id).catch(() => undefined);
    }
  }, [candidates, decisions, loadPolicyDecision]);

  const { releaseDetails, releases, resolveRevisionLabel } = governance;
  useEffect(() => {
    for (const release of releases) {
      const detail = releaseDetails[release.id];
      for (const asset of detail?.manifest.assets ?? []) {
        if (revisionLabels[asset.revisionId] !== undefined) continue;
        const knownAsset = assets.find((item) => item.id === asset.assetId);
        void resolveRevisionLabel(asset.assetId, asset.revisionId, knownAsset ? `@${knownAsset.revisionRecord.sequence}` : asset.revisionId)
          .then((label) => setRevisionLabels((current) => ({ ...current, [asset.revisionId]: label })))
          .catch(() => undefined);
      }
    }
  }, [assets, releaseDetails, releases, resolveRevisionLabel, revisionLabels]);

  useEffect(() => {
    if (!selectedRelease || loadedReleaseId === selectedRelease.id) return;
    let cancelled = false;
    governance.loadReleaseDetail(selectedRelease.id)
      .then((detail) => {
        if (cancelled) return;
        setReleaseDetail(detail);
        setLoadedReleaseId(detail.id);
        setReleaseDetailError("");
        if (detail.originProposalId) {
          governance.loadProposal(detail.originProposalId)
            .then((proposal) => { if (!cancelled) setOriginProposal(proposal); })
            .catch(() => undefined);
        } else setOriginProposal(null);
      })
      .catch((reason: Error) => { if (!cancelled) setReleaseDetailError(reason.message); });
    return () => { cancelled = true; };
  }, [governance, loadedReleaseId, selectedRelease]);

  const toggleBatch = (batchId: string) => {
    if (expandedBatchId === batchId) { setExpandedBatchId(null); setBatchDetail(null); return; }
    setExpandedBatchId(batchId);
    setBatchError("");
    governance.loadBatchDetail(batchId)
      .then((detail) => setBatchDetail(detail))
      .catch((reason: Error) => setBatchError(reason.message));
  };

  if (selectedCandidate) {
    const publishedSummary = governance.releases.find((release) => release.originProposalId === selectedCandidate.proposal.id) ?? null;
    const publishedRelease = publishedSummary ? governance.releaseDetails[publishedSummary.id] ?? null : null;
    return (
      <section className="view view-releases governance-detail-view">
        <GovernanceCandidateDetail candidate={selectedCandidate} publishedRelease={publishedRelease} onReview={() => onReview(selectedCandidate)} onPublish={() => onPublish(selectedCandidate)} />
      </section>
    );
  }

  if (selectedRelease) {
    if (releaseDetailError) {
      return <section className="view view-releases governance-detail-view"><div className="governance-load-error" role="alert"><CircleAlert size={18} /><span><strong>发布记录加载失败</strong>{releaseDetailError}</span></div></section>;
    }
    if (!releaseDetail || loadedReleaseId !== selectedRelease.id) {
      return <section className="view view-releases governance-detail-view"><div className="governance-load-error" role="status"><LoaderCircle className="spin" size={16} />正在加载发布记录</div></section>;
    }
    return (
      <section className="view view-releases governance-detail-view">
        <ReleaseDetail release={releaseDetail} originProposal={originProposal} revisionLabels={revisionLabels} isLatest={releaseDetail.sequence === latestSequence} rolledBackByReleaseId={governance.releases.find((item) => item.rolledBackToReleaseId === releaseDetail.id)?.id ?? null} onManifest={() => setInspector("manifest")} onCompare={() => setInspector("compare")} onRollback={onRollback} />
        {inspector && inspectorRelease && <ReleaseInspectorDialog mode={inspector} release={inspectorRelease} revisionLabel={(() => { const pin = governance.releaseDetails[inspectorRelease.id]?.manifest.assets[0]; return pin ? revisionLabels[pin.revisionId] ?? pin.revisionId : "当前注册表"; })()} onClose={() => setInspector(null)} />}
      </section>
    );
  }

  const normalizedQuery = query.trim().toLocaleLowerCase("zh-CN");
  const visibleCandidates = candidateRows.filter((candidate) => {
    const matchesFilter = filter === "全部" || filter === "待处理" && candidate.proposal.state !== "rejected";
    const matchesQuery = !normalizedQuery || `${candidate.asset?.name ?? ""} ${candidate.asset?.key ?? ""} ${candidate.targetRevision} ${candidate.proposal.id} ${candidate.proposal.title}`.toLocaleLowerCase("zh-CN").includes(normalizedQuery);
    return matchesFilter && matchesQuery;
  });
  const visibleReleases = governance.releases.filter((release) => {
    const detail = governance.releaseDetails[release.id];
    const pinCount = detail ? detail.manifest.assets.length + detail.manifest.objects.length : 0;
    const isCurrent = release.sequence === latestSequence;
    const matchesFilter = filter === "全部" || (filter === "当前版本" && isCurrent) || (filter === "历史版本" && !isCurrent);
    const matchesQuery = !normalizedQuery || `${release.id} #${release.sequence} ${release.publishedBy} ${release.manifestDigest} ${pinCount}`.toLocaleLowerCase("zh-CN").includes(normalizedQuery);
    return matchesFilter && matchesQuery;
  });
  const visibleCount = visibleCandidates.length + visibleReleases.length;
  const rowCount = candidateRows.length + governance.releases.length;

  return (
    <section className="view view-releases release-list-only">
      {rollbackNotice && <div className="rollback-notice" role="status"><RotateCcw size={16} /><span>回滚完成：已创建新的不可变发布 #{rollbackNotice.sequence}，注册表指针已切回目标发布之前的修订。</span></div>}
      {governance.proposalsError && <div className="governance-list-error" role="alert"><CircleAlert size={16} /><span>提案加载失败：{governance.proposalsError}</span></div>}
      {governance.releasesError && <div className="governance-list-error" role="alert"><CircleAlert size={16} /><span>发布记录加载失败：{governance.releasesError}</span></div>}
      {governance.batchesError && <div className="governance-list-error" role="alert"><CircleAlert size={16} /><span>评审批次加载失败：{governance.batchesError}</span></div>}
      {(governance.batches.length > 0 || canReview) && governance.proposalsState === "ready" && <section className="governance-list-surface review-batch-surface" aria-label="评审批次">
        <div className="review-batch-header">
          <div><span className="content-label">批量确认通道</span><h3>评审批次</h3></div>
          {canReview && <button className="secondary-button" type="button" onClick={onAssembleBatches}><GitPullRequestArrow size={14} />汇编批次</button>}
        </div>
        {governance.batches.length === 0 && <p className="review-batch-empty">当前没有待确认的评审批次；批量路由的提案会在这里等待一次确认。</p>}
        {governance.batches.map((batch) => <div className="review-batch-row" key={batch.id}>
          <button className="review-batch-summary" type="button" aria-expanded={expandedBatchId === batch.id} onClick={() => toggleBatch(batch.id)}>
            <span><strong><code>{batch.id}</code> · {batch.memberCount} 个成员</strong><small>分组规则 {batch.groupingRule.diffCategory} · 策略 {batch.policyVersion} · {batch.status === "open" ? "待确认" : batch.status === "confirmed" ? "已确认" : "已拒绝"}</small></span>
            <ChevronRight size={15} />
          </button>
          {expandedBatchId === batch.id && <div className="review-batch-detail">
            {batchError && <div className="governance-inline-error" role="alert"><CircleAlert size={15} />{batchError}</div>}
            {batchDetail && <>
              <dl className="review-batch-facts"><div><dt>匹配规则</dt><dd><code>{batchDetail.groupingRule.matchedRuleId}</code></dd></div><div><dt>最高风险成员</dt><dd><code>{batchDetail.maxRiskProposalId ?? "—"}</code></dd></div><div><dt>创建者</dt><dd>{batchDetail.createdBy}</dd></div></dl>
              <div className="review-batch-members">
                <span className="content-label">成员 · {batchDetail.members.length}</span>
                {batchDetail.members.map((member) => <div className="review-batch-member" key={member.proposalId}>
                  <code>{member.proposalId}</code>
                  <span>{governance.proposals.find((proposal) => proposal.id === member.proposalId)?.title ?? "提案"}</span>
                  <StatusBadge tone="info">{member.sample ? "样本" : "成员"}</StatusBadge>
                  {member.splitOut && <StatusBadge tone="warning">已拆出 · {member.splitReason ?? "规则变化"}</StatusBadge>}
                </div>)}
              </div>
              {batchDetail.exclusions.length > 0 && <div className="review-batch-exclusions"><span className="content-label">确认时拆出</span>{batchDetail.exclusions.map((member) => <div className="review-batch-member" key={member.proposalId}><code>{member.proposalId}</code><span>{member.splitReason ?? "已拆出"}</span><StatusBadge tone="warning">未应用批次决策</StatusBadge></div>)}</div>}
              {batchDetail.status === "open" && canReview && <div className="review-batch-actions"><button className="primary-button" type="button" onClick={() => onConfirmBatch(batchDetail)}><ShieldCheck size={14} />确认批次</button></div>}
            </>}
          </div>}
        </div>)}
      </section>}
      <section className="governance-list-surface release-history-surface asset-version-registry" aria-label="语义资产版本列表">
        <div className="governance-list-toolbar asset-version-toolbar">
          <div className="segment-control governance-filter" aria-label="资产版本筛选">{(["全部", "待处理", "当前版本", "历史版本"] as AssetVersionFilter[]).map((item) => <button key={item} type="button" aria-pressed={filter === item} onClick={() => setFilter(item)}>{item === "全部" ? `全部 ${candidates.length + governance.releases.length}` : item === "待处理" ? `待处理 ${candidates.filter((candidate) => candidate.proposal.state !== "released" && candidate.proposal.state !== "rejected").length}` : item}</button>)}</div>
          <label className="governance-search"><Search size={16} /><input type="search" aria-label="搜索资产版本" placeholder="搜索资产、提案、发布批次或清单摘要" value={query} onChange={(event) => setQuery(event.target.value)} /></label>
        </div>
        <div className="governance-list-summary"><span>显示 <strong>{visibleCount}</strong> / {rowCount} 条版本</span><span><ArrowUpDown size={14} />待处理优先，其次按发布序列</span></div>
        <div className="governance-table release-table">
          <div className="governance-list-head release-compact-grid" aria-hidden="true"><span>语义资产</span><span>版本</span><span>版本内容</span><span>发布批次</span><span>状态</span><span>责任人</span><span /></div>
          <div className="governance-list-body">
            {visibleCandidates.map((candidate) => {
              const status = candidateStatusFor(candidate);
              return <button key={candidate.key} type="button" className="governance-list-row release-compact-grid candidate-version-row" aria-label={`查看候选资产版本 ${candidate.asset?.name ?? candidate.proposal.title} ${candidate.targetRevision}`} onClick={() => { setSelectedKey(candidate.key); setSelectedDetailBackEpoch(detailBackRequestEpoch); }}>
                <span className="governance-primary governance-primary-compact"><strong>{candidate.asset?.name ?? candidate.proposal.title}</strong><small><code>{candidate.asset?.key ?? candidate.proposal.targetObjectId}</code><span>{candidate.asset?.type ?? "治理对象"}</span></small></span>
                <span className="governance-metric"><strong>{candidate.targetRevision}</strong><small>基于 {candidate.baseRevision}</small></span>
                <span className="governance-context"><strong>{candidate.proposal.title}</strong><small><code>{candidate.proposal.id}</code> · {candidate.decision ? `${routingLabels[candidate.decision.routing] ?? candidate.decision.routing}` : "决策生成中"}</small></span>
                <span className="governance-row-owner"><code>尚未发布</code><small>{governanceTimestamp(candidate.proposal.createdAt)}</small></span>
                <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
                <span className="governance-row-owner"><strong>{candidate.asset?.owner ?? candidate.proposal.createdBy}</strong><small>{candidate.proposal.createdBy} 起草</small></span>
                <ChevronRight size={16} />
              </button>;
            })}
            {visibleReleases.map((release) => {
              const detail = governance.releaseDetails[release.id];
              const isCurrent = release.sequence === latestSequence;
              const primaryPin = detail?.manifest.assets[0];
              const label = primaryPin ? revisionLabels[primaryPin.revisionId] ?? primaryPin.revisionId : `#${release.sequence}`;
              return <button key={release.id} type="button" className="governance-list-row release-compact-grid" aria-label={`查看发布记录 ${label} #${release.sequence}`} onClick={() => { setSelectedKey(`release:${release.id}`); setSelectedDetailBackEpoch(detailBackRequestEpoch); }}>
                <span className="governance-primary governance-primary-compact"><strong>{primaryPin ? assets.find((asset) => asset.id === primaryPin.assetId)?.name ?? primaryPin.assetId : `发布 #${release.sequence}`}</strong><small><code>{primaryPin ? assets.find((asset) => asset.id === primaryPin.assetId)?.identity.key ?? primaryPin.assetId : release.id}</code><span>{release.rolledBackToReleaseId ? "回滚发布" : "标准发布"}</span></small></span>
                <span className="governance-metric"><strong>{label}</strong><small>序列 #{release.sequence}</small></span>
                <span className="governance-context"><strong>{detail ? `${detail.manifest.assets.length + detail.manifest.objects.length} 项清单固定` : "清单加载中"}</strong><small><code title={release.manifestDigest}>{release.manifestDigest.slice(0, 16)}…</code></small></span>
                <span className="governance-row-owner"><code>{release.id}</code><small>{governanceTimestamp(release.publishedAt)}</small></span>
                <StatusBadge tone={isCurrent ? "success" : "neutral"}>{isCurrent ? "当前版本" : "历史版本"}</StatusBadge>
                <span className="governance-row-owner"><strong>{release.publishedBy}</strong><small>已签名发布</small></span>
                <ChevronRight size={16} />
              </button>;
            })}
            {visibleCount === 0 && governance.proposalsState === "ready" && governance.releasesState === "ready" && <div className="governance-list-empty"><PackageCheck size={22} /><strong>没有匹配的资产版本</strong><span>调整状态筛选或搜索关键词。</span></div>}
            {(governance.proposalsState === "idle" || governance.releasesState === "idle") && <div className="governance-list-empty" role="status"><LoaderCircle className="spin" size={18} />正在加载治理数据</div>}
          </div>
        </div>
      </section>
      {inspector && inspectorRelease && <ReleaseInspectorDialog mode={inspector} release={inspectorRelease} revisionLabel={(() => { const pin = governance.releaseDetails[inspectorRelease.id]?.manifest.assets[0]; return pin ? revisionLabels[pin.revisionId] ?? pin.revisionId : "当前注册表"; })()} onClose={() => setInspector(null)} />}
    </section>
  );
}

function ReviewDialog({ candidate, onClose, onDecided }: { candidate: GovernanceCandidate; onClose: () => void; onDecided: (review: GovernanceReview) => void }) {
  const governance = useGovernanceRuntime();
  const canReview = useCan("proposal.review");
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const [reason, setReason] = useState("");
  const [deciding, setDeciding] = useState(false);
  const [failure, setFailure] = useState<{ code: string; message: string; explanation: string; policySource?: string } | null>(null);
  const [runs, setRuns] = useState<GovernanceValidationRun[]>(governance.validationRuns[candidate.proposal.id] ?? []);
  const hasFailedValidation = runs.some((run) => run.status === "failed" || run.results.some((result) => result.severity === "blocker"));

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

  const { loadValidationRuns } = governance;
  useEffect(() => {
    let cancelled = false;
    loadValidationRuns(candidate.proposal.id)
      .then((fetched) => { if (!cancelled) setRuns(fetched); })
      .catch(() => undefined);
    return () => { cancelled = true; };
  }, [candidate.proposal.id, loadValidationRuns]);

  const decide = async (decision: "approve" | "reject") => {
    if (deciding) return;
    setDeciding(true);
    setFailure(null);
    try {
      const review = await governance.reviewProposal(candidate.proposal.id, { decision, reason: reason.trim() });
      onDecided(review);
    } catch (submitError) {
      const error = submitError as { code?: string; message?: string; details?: Record<string, unknown> };
      const details = (error.details ?? {}) as { conflict?: string; policySource?: string };
      setFailure({ code: error.code ?? "REQUEST_FAILED", message: error.message ?? "评审记录失败。", explanation: typeof details.conflict === "string" ? details.conflict : "", policySource: typeof details.policySource === "string" ? details.policySource : "" });
    } finally {
      setDeciding(false);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog" role="dialog" aria-modal="true" aria-labelledby="review-dialog-title">
        <header>
          <div><span className="panel-kicker">候选资产版本 · {candidate.proposal.id}</span><h2 id="review-dialog-title">审核 {candidate.asset?.name ?? candidate.proposal.title} · {candidate.targetRevision}</h2></div>
          <button ref={closeButtonRef} className="icon-button" type="button" aria-label="关闭审核" onClick={onClose}><X size={18} /></button>
        </header>
        <div className="dialog-body">
          <div className="dialog-summary"><span className="proposal-author ai-author"><PackageCheck size={16} /></span><div><strong>{candidate.asset?.name ?? candidate.proposal.title} {candidate.baseRevision} → {candidate.targetRevision}</strong><p>包含变更事项 {candidate.proposal.id}：{candidate.proposal.title}</p></div></div>
          <div className="dialog-validation">
            {runs.map((run) => (
              <div className={`validation-item validation-${runTone(run) === "danger" ? "failed" : runTone(run) === "warning" ? "warning" : "passed"}`} key={run.id}><span>{runTone(run) === "danger" ? <CircleAlert size={15} /> : runTone(run) === "warning" ? <AlertTriangle size={15} /> : <CheckCircle2 size={15} />}</span><div><strong>{run.validatorId} <code>v{run.validatorVersion}</code></strong><small>{runStateLabels[run.status]} · {run.results.map((result) => `${validationSeverityLabels[result.severity]} ${result.code}`).join(" · ") || "暂无结果"}</small></div></div>
            ))}
            {runs.length === 0 && <div className="validation-item"><CircleDot size={15} /><div><strong>验证运行尚未产生结果</strong><small>验证结果在候选版本页展示。</small></div></div>}
          </div>
          <div className="release-sod-check"><ShieldCheck size={17} /><span><strong>职责分离由服务端强制</strong><small>提案作者不能评审自己的提案（FR-007）；发布需要独立主体执行。冲突会被拒绝并说明原因。</small></span></div>
          {hasFailedValidation && <div className="version-review-blocker"><CircleAlert size={16} /><span><strong>候选版本存在失败门禁</strong><small>可以退回版本，但必须修复验证后才能批准发布。</small></span></div>}
          {failure && failure.code === GOVERNANCE_ERROR_CODES.SEPARATION_OF_DUTY ? <div className="release-sod-conflict" role="alert"><CircleAlert size={17} /><span><strong>职责分离冲突</strong><small><code>{failure.code}</code> · {failure.explanation || failure.message} 请由持有 <code>proposal.review</code> 能力的其他主体完成本次评审。{failure.policySource && <>政策来源：<code>{failure.policySource}</code></>}</small></span></div> : failure && <div className="model-dialog-error" role="alert"><CircleAlert size={15} /><code>{failure.code}</code> · {failure.message}</div>}
          <label className="review-note"><span>审核意见 <strong>必填</strong></span><textarea aria-label="审核意见" placeholder="记录判断依据；意见会作为不可变评审事实保存。" value={reason} onChange={(event) => { setReason(event.target.value); setFailure(null); }} /></label>
        </div>
        <footer>
          {canReview && <button className="danger-button" type="button" disabled={!reason.trim() || deciding} onClick={() => void decide("reject")}><RotateCcw size={16} />退回版本</button>}
          <span />
          <div><button className="secondary-button" type="button" onClick={onClose}>取消</button>{canReview && <button className="primary-button" type="button" disabled={!reason.trim() || deciding || hasFailedValidation} onClick={() => void decide("approve")}><Check size={16} />批准版本</button>}</div>
        </footer>
      </section>
    </div>
  );
}

function BatchConfirmDialog({ batch, onClose, onDecided }: { batch: GovernanceReviewBatchDetail; onClose: () => void; onDecided: (detail: GovernanceReviewBatchDetail) => void }) {
  const governance = useGovernanceRuntime();
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const [reason, setReason] = useState("");
  const [deciding, setDeciding] = useState(false);
  const [failure, setFailure] = useState<{ code: string; message: string; explanation: string; policySource?: string } | null>(null);

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

  const decide = async (decision: "approve" | "reject") => {
    if (deciding) return;
    setDeciding(true);
    setFailure(null);
    try {
      const detail = await governance.confirmBatch(batch.id, { decision, reason: reason.trim() });
      onDecided(detail);
    } catch (submitError) {
      const error = submitError as { code?: string; message?: string; details?: Record<string, unknown> };
      const details = (error.details ?? {}) as { conflict?: string; policySource?: string };
      setFailure({ code: error.code ?? "REQUEST_FAILED", message: error.message ?? "批次确认失败。", explanation: typeof details.conflict === "string" ? details.conflict : "", policySource: typeof details.policySource === "string" ? details.policySource : "" });
    } finally {
      setDeciding(false);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog" role="dialog" aria-modal="true" aria-labelledby="batch-confirm-title">
        <header><div><span className="panel-kicker">评审批次 · {batch.id}</span><h2 id="batch-confirm-title">确认 {batch.memberCount} 个提案</h2></div><button ref={closeButtonRef} className="icon-button" type="button" aria-label="关闭批次确认" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body">
          <p className="batch-confirm-intro">一次决策会应用到确认时仍在批次内的全部成员；已升级、失去决策或离开审核中的成员会被自动拆出并记录原因。</p>
          <div className="dialog-summary"><span className="proposal-author ai-author"><GitPullRequestArrow size={16} /></span><div><strong>分组规则 {batch.groupingRule.diffCategory}</strong><p>匹配规则 <code>{batch.groupingRule.matchedRuleId}</code> · 策略版本 {batch.policyVersion}</p></div></div>
          {failure && failure.code === GOVERNANCE_ERROR_CODES.SEPARATION_OF_DUTY ? <div className="release-sod-conflict" role="alert"><CircleAlert size={17} /><span><strong>职责分离冲突</strong><small><code>{failure.code}</code> · {failure.explanation || failure.message}{failure.policySource && <> 政策来源：<code>{failure.policySource}</code></>}</small></span></div> : failure && <div className="model-dialog-error" role="alert"><CircleAlert size={15} /><code>{failure.code}</code> · {failure.message}</div>}
          <label className="review-note"><span>确认意见 <strong>必填</strong></span><textarea aria-label="批次确认意见" placeholder="记录批量判断依据；意见会随批次审计记录保存。" value={reason} onChange={(event) => { setReason(event.target.value); setFailure(null); }} /></label>
        </div>
        <footer>
          <button className="danger-button" type="button" disabled={!reason.trim() || deciding} onClick={() => void decide("reject")}><RotateCcw size={16} />拒绝批次</button>
          <span />
          <div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={!reason.trim() || deciding} onClick={() => void decide("approve")}><Check size={16} />批准批次</button></div>
        </footer>
      </section>
    </div>
  );
}

function CreateProposalDialog({ assets, onClose, onStartWorkbench }: { assets: Asset[]; onClose: () => void; onStartWorkbench: (assetId: string) => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  const [assetId, setAssetId] = useState(assets[0]?.id ?? "");
  const [intent, setIntent] = useState("补充口径边界与验证证据，形成可审核的候选修订。");
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
        <div className="dialog-body proposal-form">
          <label><span>基础资产</span><select aria-label="基础资产" value={assetId} onChange={(event) => setAssetId(event.target.value)}>{assets.map((asset) => <option key={asset.id} value={asset.id}>{asset.name} · {asset.key}</option>)}</select></label>
          <label><span>变更意图</span><textarea aria-label="变更意图" value={intent} onChange={(event) => setIntent(event.target.value)} /></label>
          <div className="proposal-policy"><ShieldCheck size={18} /><span><strong>治理级别 G1</strong><small>进入修订工作台编辑结构化字段，提交后创建真实治理提案，发布前必须由资产负责人审核。</small></span></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={!assetId} onClick={() => onStartWorkbench(assetId)}><Sparkles size={16} />进入修订工作台</button></div></footer>
      </section>
    </div>
  );
}

function PublishDialog({ candidate, onClose, onPublished }: { candidate: GovernanceCandidate; onClose: () => void; onPublished: (release: GovernanceReleaseDetail) => void }) {
  const governance = useGovernanceRuntime();
  const canPublish = useCan("release.publish");
  const closeRef = useRef<HTMLButtonElement>(null);
  const [publishing, setPublishing] = useState(false);
  const [failure, setFailure] = useState<{ code: string; message: string; explanation: string; policySource?: string } | null>(null);
  const [runs, setRuns] = useState<GovernanceValidationRun[]>(governance.validationRuns[candidate.proposal.id] ?? []);
  const blockers = runs.reduce((total, run) => total + run.results.filter((result) => result.severity === "blocker").length, 0);
  const approved = candidate.review?.decision === "approved";

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const { loadValidationRuns } = governance;
  useEffect(() => {
    let cancelled = false;
    loadValidationRuns(candidate.proposal.id)
      .then((fetched) => { if (!cancelled) setRuns(fetched); })
      .catch(() => undefined);
    return () => { cancelled = true; };
  }, [candidate.proposal.id, loadValidationRuns]);

  const publish = async () => {
    if (publishing) return;
    setPublishing(true);
    setFailure(null);
    try {
      const release = await governance.publishProposal(candidate.proposal.id);
      onPublished(release);
    } catch (publishError) {
      const error = publishError as { code?: string; message?: string; details?: Record<string, unknown> };
      const details = (error.details ?? {}) as { conflict?: string; policySource?: string };
      setFailure({ code: error.code ?? "REQUEST_FAILED", message: error.message ?? "发布失败。", explanation: typeof details.conflict === "string" ? details.conflict : "", policySource: typeof details.policySource === "string" ? details.policySource : "" });
    } finally {
      setPublishing(false);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog" role="dialog" aria-modal="true" aria-labelledby="publish-dialog-title">
        <header><div><span className="panel-kicker">语义资产 · <code>{candidate.asset?.key ?? candidate.proposal.targetObjectId}</code></span><h2 id="publish-dialog-title">发布{candidate.asset?.name ?? candidate.proposal.title} {candidate.targetRevision}</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭发布确认" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body publish-checklist">
          <div>{runs.length === 0 ? <CircleDot size={17} /> : blockers > 0 ? <CircleAlert size={17} /> : <CheckCircle2 size={17} />}<span><strong>结构与引用验证</strong><small>{runs.length === 0 ? "验证运行尚未产生结果" : blockers > 0 ? `${blockers} 项阻断发现` : `${runs.length} 项运行全部通过`}</small></span></div>
          <div>{approved ? <CheckCircle2 size={17} /> : <CircleAlert size={17} />}<span><strong>治理策略</strong><small>{approved ? "已记录批准评审" : "尚未记录批准评审，发布会被拒绝"}</small></span></div>
          <div><CheckCircle2 size={17} /><span><strong>消费影响</strong><small>{candidate.asset ? `${candidate.asset.consumers.length} 个消费者已纳入审核范围` : "消费绑定属于后续里程碑"}</small></span></div>
          <div className="release-sod-check"><ShieldCheck size={17} /><span><strong>独立发布者由服务端强制</strong><small>发布以当前工作区身份执行；发布者不能是提案作者，也不能是唯一批准评审人（D-006 双人控制）。</small></span></div>
          {failure && failure.code === GOVERNANCE_ERROR_CODES.SEPARATION_OF_DUTY ? <div className="release-sod-conflict" role="alert"><CircleAlert size={17} /><span><strong>职责分离冲突</strong><small><code>{failure.code}</code> · {failure.explanation || failure.message}{failure.policySource && <> 政策来源：<code>{failure.policySource}</code></>}</small></span></div> : failure && <div className="model-dialog-error" role="alert"><CircleAlert size={15} /><code>{failure.code}</code> · {failure.message}</div>}
          <div className="publish-target"><span>资产版本</span><code>{candidate.asset?.key ?? candidate.proposal.targetObjectId} · {candidate.targetRevision}</code><small>发布将生成不可变清单并切换当前修订指针</small></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={!canPublish || publishing || blockers > 0} onClick={() => void publish()}><PackageCheck size={16} />{publishing ? "正在发布" : "确认发布并生效"}</button></div></footer>
      </section>
    </div>
  );
}

function CommandPalette({ proposals, onClose, onNavigate, onSelectAsset, onSelectProposal, onCreateProposal }: { proposals: GovernanceProposalSummary[]; onClose: () => void; onNavigate: (view: ViewId) => void; onSelectAsset: (id: string) => void; onSelectProposal: (id: string) => void; onCreateProposal: () => void }) {
  const { assets } = useCatalogRuntime();
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
    ...proposals.map((proposal) => ({ key: proposal.id, label: proposal.title, detail: `${proposal.id} · ${proposalStateLabels[proposal.state]} · ${proposal.createdBy}`, icon: GitPullRequestArrow, run: () => onSelectProposal(proposal.id) })),
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

export function ProductApp({ fixtureGovernance = false, session }: { fixtureGovernance?: boolean; session?: CapabilitySession }) {
  const { workspaceId } = useCatalogRuntime();
  return (
    <GovernanceRuntimeProvider workspaceId={workspaceId} fixture={fixtureGovernance}>
      <ProductApplication session={session} />
    </GovernanceRuntimeProvider>
  );
}

const workbenchTaskProposalIds: Record<Exclude<WorkbenchTask["target"], "source-run">, string> = {
  "candidate-region": "PROP-121",
  "candidate-aov": "PROP-128",
  "candidate-revenue": "PROP-126",
};

function ProductApplication({ session }: { session?: CapabilitySession }) {
  const { assets, setWorkflowStateOverlay } = useCatalogRuntime();
  const governance = useGovernanceRuntime();
  const preferredRecentAssetIds = [
    "metric_01J4NETREVENUE8W4Q9D7K2",
    "model_01J4ORDERS7K8M2Q5N9P",
    "segment_01J4HIGHVALUE4X7P2K9M6N",
  ];
  const initialRecentAssetIds = [
    ...preferredRecentAssetIds.filter((id) => assets.some((asset) => asset.id === id)),
    ...assets.map((asset) => asset.id),
  ].filter((id, index, values) => values.indexOf(id) === index).slice(0, 3);
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
  const [selectedAssetId, setSelectedAssetId] = useState(assets[0]?.id ?? "");
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
  const [reviewing, setReviewing] = useState<GovernanceCandidate | null>(null);
  const [discoveryState, setDiscoveryState] = useState<"ready" | "running" | "complete">("ready");
  const [publishedRelease, setPublishedRelease] = useState<GovernanceReleaseDetail | null>(null);
  const [rollbackNotice, setRollbackNotice] = useState<GovernanceReleaseDetail | null>(null);
  const [creatingProposal, setCreatingProposal] = useState(false);
  const [publishing, setPublishing] = useState<GovernanceCandidate | null>(null);
  const [aiGenerationRequest, setAiGenerationRequest] = useState<{ assetId: string; fieldPath: string } | null>(null);
  const [confirmingBatch, setConfirmingBatch] = useState<GovernanceReviewBatchDetail | null>(null);
  const [commandOpen, setCommandOpen] = useState(false);
  const [assetSearchRequestEpoch, setAssetSearchRequestEpoch] = useState(0);
  const [workbenchSearchRequestEpoch, setWorkbenchSearchRequestEpoch] = useState(0);
  const [conversationSearchRequestEpoch, setConversationSearchRequestEpoch] = useState(0);
  const [toast, setToast] = useState("");
  const discoveryTimer = useRef<number | null>(null);
  const workspaceRef = useRef<HTMLElement>(null);

  useEffect(() => {
    setWorkflowStateOverlay(governance.workflowStateByAsset);
  }, [governance.workflowStateByAsset, setWorkflowStateOverlay]);

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

  const openCandidateVersionById = (proposalId: string) => {
    setActiveWorkbenchTask(null);
    setVersionSourceRun(null);
    setVersionDetailRequest((current) => ({ key: `candidate:${proposalId}`, epoch: current.epoch + 1 }));
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

  const submitKnowledgeRevision = async (submission: KnowledgeRevisionSubmission) => {
    const asset = assets.find((item) => item.id === submission.assetId);
    if (!asset) return;
    const changeSet = submission.changes.map((change) => ({
      fieldPath: change.field,
      op: (change.before.trim() === "" ? "add" : change.after.trim() === "" ? "remove" : "update") as "add" | "remove" | "update",
      beforeValue: change.before.trim() === "" ? undefined : change.before,
      afterValue: change.after.trim() === "" ? undefined : change.after,
    }));
    const submitted = await governance.createAndSubmitProposal({
      targetObjectType: "semantic_asset",
      targetObjectId: asset.id,
      baseRevisionId: asset.revisionRecord.revisionId.startsWith("rev_") ? asset.revisionRecord.revisionId : undefined,
      title: submission.title,
      summary: submission.summary,
      reason: submission.reason,
      changeSet,
      createdBy: "catalog-web",
    });
    setKnowledgeRevisionRequest(null);
    openCandidateVersionById(submitted.id);
    showPrototypeToast(`${asset.name} 知识修订已提交为提案 ${submitted.id}，治理验证运行中。`);
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

  const handleReviewDecided = (review: GovernanceReview) => {
    const candidate = reviewing;
    setReviewing(null);
    showPrototypeToast(`${candidate?.asset?.name ?? candidate?.proposal.title ?? "提案"} 评审已记录：${review.decision === "approved" ? "批准" : "退回"}。`);
  };

  const handlePublished = (release: GovernanceReleaseDetail) => {
    setPublishing(null);
    setPublishedRelease(release);
    setRollbackNotice(null);
    showPrototypeToast(`发布完成：批次 ${release.id} · 序列 #${release.sequence} 已切换当前修订指针。`);
  };

  const handleRollback = async (releaseId: string) => {
    try {
      const release = await governance.rollback(releaseId);
      setRollbackNotice(release);
      showPrototypeToast(`回滚完成：已创建新的不可变发布 #${release.sequence}。`);
    } catch (rollbackError) {
      showPrototypeToast(rollbackError instanceof Error ? rollbackError.message : "回滚失败。");
    }
  };

  const handleAssembleBatches = async () => {
    try {
      const created = await governance.assembleBatches();
      showPrototypeToast(created.length > 0 ? `已汇编 ${created.length} 个评审批次。` : "当前没有符合批量路由条件的提案。");
    } catch (assembleError) {
      showPrototypeToast(assembleError instanceof Error ? assembleError.message : "评审批次汇编失败。");
    }
  };

  const handleBatchConfirmed = (detail: GovernanceReviewBatchDetail) => {
    setConfirmingBatch(null);
    showPrototypeToast(`评审批次已确认：${detail.status === "confirmed" ? "批准" : "拒绝"} · ${detail.memberCount} 个成员。`);
  };

  const renameConversationTitle = (title: string) => {
    if (view !== "ask") return;
    setAskConversationTitles((current) => ({ ...current, [contextIndex]: title }));
  };

  const recentAssets = recentAssetIds.map((id) => assets.find((asset) => asset.id === id)).filter((asset): asset is Asset => Boolean(asset));
  const selectedAsset = assets.find((asset) => asset.id === selectedAssetId);
  const activeWorkbenchProposalId = activeWorkbenchTask && activeWorkbenchTask.target !== "source-run" ? workbenchTaskProposalIds[activeWorkbenchTask.target] : undefined;
  const activeWorkbenchCandidate = useMemo(() => {
    if (!activeWorkbenchProposalId) return null;
    const proposal = governance.proposals.find((item) => item.id === activeWorkbenchProposalId);
    return proposal ? governanceCandidateFor(proposal, assets, governance.decisions, governance.reviews) : null;
  }, [activeWorkbenchProposalId, assets, governance.decisions, governance.proposals, governance.reviews]);
  const commandProposals = useMemo(() => governance.proposals
    .filter((proposal) => proposal.state !== "draft")
    .slice(0, 8), [governance.proposals]);
  const releaseContextItems = [
    { label: "资产版本", meta: `${governance.proposals.filter((proposal) => reviewableStates.includes(proposal.state) && proposal.state !== "rejected").length} 个候选 · ${governance.releases.length} 条已发布`, tone: "warning" as const },
  ];
  const contextLabel = view === "overview" ? activeWorkbenchTask?.title ?? "待办" : view === "assets" ? (assetDetailRequest ? selectedAsset?.name : "知识目录") : view === "ask" ? askConversationTitles[contextIndex] ?? contextItems[view][contextIndex]?.label : contextItems[view][contextIndex]?.label;

  return (
    <CapabilityProvider session={session}>
    <div className={contextPanelOpen ? "app-shell" : "app-shell context-panel-collapsed"} style={{ "--context-panel-width": `${contextPanelOpen ? contextPanelWidth : 0}px` } as React.CSSProperties}>
      <ActivityRail view={view} onChange={navigate} />
      {contextPanelOpen && <ContextPanel view={view} activeIndex={contextIndex} width={contextPanelWidth} recentAssets={recentAssets} activeAssetId={selectedAssetId} assetDetailOpen={assetDetailRequest} activeWorkbenchTask={activeWorkbenchTask} askConversationTitles={askConversationTitles} conversationSearchRequestEpoch={conversationSearchRequestEpoch} releasesItems={releaseContextItems} onSelect={selectContext} onOpenRecentAsset={openAssetDetail} onOpenWorkbenchTask={setActiveWorkbenchTask} onOpenCatalogSearch={openCatalogSearch} onOpenWorkbenchSearch={openWorkbenchSearch} onCollapse={() => setContextPanelOpen(false)} onResize={setContextPanelWidth} />}
      <div className="workspace">
        <Topbar key={`${view}-${contextLabel ?? ""}`} view={view} contextLabel={contextLabel} onNewConversation={() => { setAskSessionKey((key) => key + 1); setContextIndex(0); }} onRenameContext={view === "ask" ? renameConversationTitle : undefined} onBack={view === "overview" && activeWorkbenchTask ? () => setActiveWorkbenchTask(null) : view === "assets" && assetDetailRequest ? () => { setKnowledgeRevisionRequest(null); setAssetDetailRequest(false); setAssetDetailBackRequestEpoch((epoch) => epoch + 1); } : view === "sources" && contextIndex === 2 && sourceRunDetailOpen ? () => setSourceRunBackRequestEpoch((epoch) => epoch + 1) : view === "releases" && releaseDetailOpen ? () => setReleaseDetailBackRequestEpoch((epoch) => epoch + 1) : undefined} backLabel={view === "overview" ? "返回待办" : view === "assets" ? "返回知识目录" : view === "releases" ? "返回资产版本列表" : "返回接入运行"} />
        <main ref={workspaceRef} className="workspace-canvas">
          {view === "ask" && <AskView key={`${askSessionKey}-${contextIndex}`} conversationIndex={contextIndex} activeRelease={publishedRelease?.id || "当前注册表"} onOpenEvidence={openAssetEvidence} onStartRevision={(fieldPath, context) => openKnowledgeRevision({ assetId: assets[0].id, fieldPath, origin: "ask", context })} />}
          {view === "sources" && <SourcesView discoveryState={discoveryState} focusIndex={contextIndex} navigationEpoch={contextSelectionEpoch} runDetailBackRequestEpoch={sourceRunBackRequestEpoch} initialRunId={sourceInitialRunId} onRunDetailOpenChange={setSourceRunDetailOpen} onOpenActivity={openSourceActivity} onOpenGlobalRun={openGlobalRun} onRunDiscovery={runDiscovery} onOpenGovernanceProposals={openRunCandidates} onNotify={showPrototypeToast} />}
          {view === "overview" && !activeWorkbenchTask && <OverviewView onOpenTask={setActiveWorkbenchTask} discoveryState={discoveryState} focusSearchRequestEpoch={workbenchSearchRequestEpoch} />}
          {view === "overview" && activeWorkbenchTask?.target === "source-run" && <SourcesView key={activeWorkbenchTask.id} discoveryState={discoveryState} focusIndex={2} navigationEpoch={0} runDetailBackRequestEpoch={0} initialRunId="RUN-240823-1844" onRunDetailOpenChange={setSourceRunDetailOpen} onOpenActivity={openSourceActivity} onOpenGlobalRun={openGlobalRun} onRunDiscovery={runDiscovery} onOpenGovernanceProposals={openRunCandidates} onNotify={showPrototypeToast} />}
          {view === "overview" && activeWorkbenchTask && activeWorkbenchTask.target !== "source-run" && (activeWorkbenchCandidate ? <section className="view view-overview governance-detail-view workbench-detail-view"><GovernanceCandidateDetail candidate={activeWorkbenchCandidate} publishedRelease={publishedRelease?.originProposalId === activeWorkbenchCandidate.proposal.id ? publishedRelease : null} onReview={() => setReviewing(activeWorkbenchCandidate)} onPublish={() => setPublishing(activeWorkbenchCandidate)} /></section> : <section className="view view-overview governance-detail-view workbench-detail-view"><div className="governance-load-error" role="status"><CircleAlert size={18} /><span><strong>候选版本在当前工作区不可用</strong>该待办引用的候选版本不存在于治理提案列表中。</span></div></section>)}
          {view === "assets" && <AssetsView key={assetTabRequestEpoch} selectedId={selectedAssetId} domainFilter="全部" requestedTab={assetTabRequest} requestedDetail={assetDetailRequest} requestedCatalogType={assetCatalogTypeRequest} requestedRevision={knowledgeRevisionRequest} focusSearchRequestEpoch={assetSearchRequestEpoch} detailBackRequestEpoch={assetDetailBackRequestEpoch} onSelect={rememberAsset} onDetailChange={setAssetDetailRequest} onNotify={showPrototypeToast} onRevisionSubmit={submitKnowledgeRevision} onStartAIGeneration={(assetId, fieldPath) => setAiGenerationRequest({ assetId, fieldPath })} />}
          {view === "releases" && <ReleasesView key={versionDetailRequest.epoch} initialSelectedKey={versionDetailRequest.key} initialSourceRun={versionSourceRun} detailBackRequestEpoch={releaseDetailBackRequestEpoch} rollbackNotice={rollbackNotice} onDetailOpenChange={setReleaseDetailOpen} onReview={setReviewing} onPublish={setPublishing} onRollback={(releaseId) => void handleRollback(releaseId)} onAssembleBatches={() => void handleAssembleBatches()} onConfirmBatch={setConfirmingBatch} />}
          {view === "settings" && <SettingsView focusIndex={contextIndex} rebuildRun={embeddingRebuildRun} auditRunRequestId={auditRunRequestId} onAuditRunRequestHandled={handleAuditRunRequest} onStartEmbeddingRebuild={startEmbeddingRebuild} onOpenRebuildRun={openEmbeddingRebuildRun} onNotify={showPrototypeToast} />}
        </main>
      </div>
      {reviewing && <ReviewDialog candidate={reviewing} onClose={() => setReviewing(null)} onDecided={handleReviewDecided} />}
      {confirmingBatch && <BatchConfirmDialog batch={confirmingBatch} onClose={() => setConfirmingBatch(null)} onDecided={handleBatchConfirmed} />}
      {creatingProposal && <CreateProposalDialog assets={assets} onClose={() => setCreatingProposal(false)} onStartWorkbench={(assetId) => { setCreatingProposal(false); openKnowledgeRevision({ assetId, fieldPath: "definition.boundary", origin: "definition", context: "创建变更事项：" }); }} />}
      {publishing && <PublishDialog candidate={publishing} onClose={() => setPublishing(null)} onPublished={handlePublished} />}
      {aiGenerationRequest && (() => {
        const asset = assets.find((item) => item.id === aiGenerationRequest.assetId);
        if (!asset) return null;
        return <AIProposalDialog asset={asset} fieldPath={aiGenerationRequest.fieldPath} onClose={() => setAiGenerationRequest(null)} onOpenModelSettings={() => { setAiGenerationRequest(null); navigate("settings", 2); }} onProposalSubmitted={(submitted) => { setAiGenerationRequest(null); openCandidateVersionById(submitted.id); showPrototypeToast(`AI 提案 ${submitted.id} 已提交，治理验证运行中。`); }} />;
      })()}
      {commandOpen && <CommandPalette proposals={commandProposals} onClose={() => setCommandOpen(false)} onNavigate={navigate} onSelectAsset={openAssetDetail} onSelectProposal={(id) => { openCandidateVersionById(id); }} onCreateProposal={() => setCreatingProposal(true)} />}
      {toast && <div className="toast" role="status"><CheckCircle2 size={17} /><span>{toast}</span></div>}
    </div>
    </CapabilityProvider>
  );
}

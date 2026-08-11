import { useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  ArrowRight,
  BadgeCheck,
  BookOpenCheck,
  Box,
  Boxes,
  Check,
  CheckCircle2,
  ChevronRight,
  CircleAlert,
  CircleDot,
  Clock3,
  Command,
  Database,
  FileCheck2,
  Fingerprint,
  GitPullRequestArrow,
  LayoutDashboard,
  ListFilter,
  PackageCheck,
  RotateCcw,
  Search,
  ShieldCheck,
  Sparkles,
  Users,
  X,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";

import { assets, proposals, releases } from "./data";
import { ConsumersView, SourcesView } from "./JourneyViews";
import type { SessionBinding } from "./JourneyViews";
import { SemanticGraph } from "./SemanticGraph";
import type { Asset, AssetType, Proposal, ValidationState, ViewId } from "./types";

const navigation: Array<{ id: ViewId; label: string; icon: LucideIcon }> = [
  { id: "sources", label: "接入发现", icon: Database },
  { id: "overview", label: "治理概览", icon: LayoutDashboard },
  { id: "assets", label: "语义资产", icon: Boxes },
  { id: "proposals", label: "变更提案", icon: GitPullRequestArrow },
  { id: "releases", label: "发布版本", icon: PackageCheck },
  { id: "consumers", label: "消费监控", icon: Users },
];

const assetTabs = ["定义", "血缘", "证据", "消费者"] as const;
type AssetTab = (typeof assetTabs)[number];

function statusTone(status: Asset["status"]) {
  if (status === "已发布") return "success";
  if (status === "需关注") return "warning";
  return "neutral";
}

function validationIcon(state: ValidationState) {
  if (state === "passed") return <CheckCircle2 size={16} />;
  if (state === "warning") return <AlertTriangle size={16} />;
  return <CircleAlert size={16} />;
}

function StatusBadge({ tone, children }: { tone: "success" | "warning" | "danger" | "neutral" | "violet" | "info"; children: React.ReactNode }) {
  return <span className={`status-badge status-${tone}`}>{children}</span>;
}

const contextItems: Record<ViewId, Array<{ label: string; meta: string; tone?: "warning" | "danger" }>> = {
  sources: [
    { label: "工作区就绪", meta: "3 / 3 检查" },
    { label: "来源连接", meta: "3 个健康" },
    { label: "最近发现", meta: "6 个提案", tone: "warning" },
    { label: "权限边界", meta: "Metadata only" },
  ],
  overview: [
    { label: "全部治理状态", meta: "148 个资产" },
    { label: "等待审核", meta: "3 个提案", tone: "warning" },
    { label: "验证异常", meta: "1 个阻塞", tone: "danger" },
    { label: "待补充绑定", meta: "2 个消费者" },
  ],
  assets: [
    { label: "全部资产", meta: "148" },
    { label: "电商经营", meta: "58" },
    { label: "客户增长", meta: "41" },
    { label: "共享维度", meta: "29", tone: "warning" },
  ],
  proposals: [
    { label: "全部待审核", meta: "3" },
    { label: "AI 提案", meta: "2" },
    { label: "高风险变更", meta: "1", tone: "danger" },
    { label: "我的审核", meta: "1" },
  ],
  releases: [
    { label: "全部版本", meta: "3" },
    { label: "稳定版本", meta: "2" },
    { label: "历史版本", meta: "1" },
    { label: "活跃绑定", meta: "12" },
  ],
  consumers: [
    { label: "全部绑定", meta: "12 active" },
    { label: "API 与 SDK", meta: "6" },
    { label: "Agent / MCP", meta: "4" },
    { label: "运行异常", meta: "1 项", tone: "warning" },
  ],
};

const contextTitles: Record<ViewId, string> = {
  sources: "接入与发现",
  overview: "治理工作区",
  assets: "语义目录",
  proposals: "审核队列",
  releases: "发布记录",
  consumers: "消费与反馈",
};

function ActivityRail({ view, onChange }: { view: ViewId; onChange: (view: ViewId) => void }) {
  return (
    <aside className="activity-rail">
      <button className="rail-brand" type="button" title="Semlia 语义治理" aria-label="Semlia 语义治理">
        <Fingerprint size={20} strokeWidth={1.8} />
      </button>

      <nav className="rail-navigation" aria-label="Semlia 主功能">
        {navigation.map((item) => {
          const Icon = item.icon;
          return (
            <button
              className={view === item.id ? "rail-item rail-item-active" : "rail-item"}
              key={item.id}
              type="button"
              title={item.label}
              aria-current={view === item.id ? "page" : undefined}
              aria-label={item.label}
              onClick={() => onChange(item.id)}
            >
              <Icon size={18} strokeWidth={1.8} />
              <span className="rail-label">{item.label}</span>
              {item.id === "proposals" && <span className="rail-count">3</span>}
            </button>
          );
        })}
      </nav>

      <button className="rail-item rail-command" type="button" title="命令面板" aria-label="打开命令面板">
        <Command size={18} strokeWidth={1.8} />
        <span className="rail-label">命令</span>
      </button>
    </aside>
  );
}

function ContextPanel({ view, activeIndex, onSelect }: { view: ViewId; activeIndex: number; onSelect: (index: number) => void }) {
  return (
    <aside className="context-panel" aria-label="治理上下文">
      <header className="context-header">
        <div className="workspace-avatar">MX</div>
        <div><strong>{contextTitles[view]}</strong><span>示例工作区</span></div>
      </header>
      <div className="context-search" aria-hidden="true"><Search size={14} /><span>筛选当前视图</span><kbd>⌘ K</kbd></div>
      <div className="context-list">
        {contextItems[view].map((item, index) => (
          <button className={index === activeIndex ? "context-item context-item-active" : "context-item"} type="button" key={item.label} onClick={() => onSelect(index)}>
            <span className={`context-dot${item.tone ? ` context-dot-${item.tone}` : ""}`} />
            <span><strong>{item.label}</strong><small>{item.meta}</small></span>
            <ChevronRight size={14} />
          </button>
        ))}
      </div>
      <footer className="context-footer">
        <div><Database size={14} /><span>Cube Commerce</span><StatusBadge tone="success">已同步</StatusBadge></div>
        <div><ShieldCheck size={14} /><span>治理策略</span><strong>G1</strong></div>
      </footer>
    </aside>
  );
}

function Topbar({ view, onCreateProposal }: { view: ViewId; onCreateProposal: () => void }) {
  return (
    <header className="topbar">
      <div className="workspace-breadcrumb"><span>Semlia</span><ChevronRight size={13} /><strong>{contextTitles[view]}</strong></div>
      <div className="topbar-actions">
        <span className="mock-badge"><CircleDot size={13} />Prototype · Mock data</span>
        <span className="release-indicator"><span className="live-dot" />Stable · 2026.08.3</span>
        <button className="primary-button" type="button" onClick={onCreateProposal}>
          <Sparkles size={16} />创建提案
        </button>
      </div>
    </header>
  );
}

function PageHeading({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: React.ReactNode }) {
  return (
    <div className="page-heading">
      <div>
        <span className="eyebrow">{eyebrow}</span>
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      {action && <div className="page-heading-action">{action}</div>}
    </div>
  );
}

const lifecycleStages: Array<{ number: string; label: string; detail: string; view: ViewId }> = [
  { number: "01", label: "接入", detail: "3 个来源", view: "sources" },
  { number: "02", label: "发现", detail: "6 个建议", view: "sources" },
  { number: "03", label: "定义", detail: "148 个资产", view: "assets" },
  { number: "04", label: "治理", detail: "3 个待审", view: "proposals" },
  { number: "05", label: "发布", detail: "Stable", view: "releases" },
  { number: "06", label: "消费", detail: "12 个绑定", view: "consumers" },
  { number: "07", label: "反馈", detail: "1 项行动", view: "consumers" },
];

function LifecycleRail({ scanCompleted, candidateReady, publishedVersion, onNavigate }: { scanCompleted: boolean; candidateReady: boolean; publishedVersion: string; onNavigate: (view: ViewId) => void }) {
  return (
    <section className="lifecycle-rail" aria-label="语义资产生命周期">
      <div className="lifecycle-title"><span className="panel-kicker">GOVERNED LIFECYCLE</span><strong>从来源到运行反馈</strong></div>
      <div className="lifecycle-stages">
        {lifecycleStages.map((stage, index) => {
          const complete = index < 3 || (index === 3 && candidateReady) || (index >= 4 && Boolean(publishedVersion));
          const active = (index === 1 && !scanCompleted) || (index === 3 && scanCompleted && !candidateReady) || (index === 4 && candidateReady && !publishedVersion) || (index >= 5 && Boolean(publishedVersion));
          return (
            <button className={active ? "lifecycle-stage lifecycle-stage-active" : "lifecycle-stage"} type="button" key={`${stage.number}-${stage.label}`} onClick={() => onNavigate(stage.view)} aria-label={`阶段 ${stage.number}：${stage.label}`}>
              <span className={complete ? "stage-number stage-complete" : "stage-number"}>{complete ? <Check size={13} /> : stage.number}</span>
              <span><strong>{stage.label}</strong><small>{stage.detail}</small></span>
              {index < lifecycleStages.length - 1 && <i aria-hidden="true" />}
            </button>
          );
        })}
      </div>
    </section>
  );
}

function OverviewView({ openAssets, openProposal, scanCompleted, candidateReady, publishedVersion, onNavigate }: { openAssets: () => void; openProposal: (proposal: Proposal) => void; scanCompleted: boolean; candidateReady: boolean; publishedVersion: string; onNavigate: (view: ViewId) => void }) {
  return (
    <section className="view view-overview">
      <PageHeading
        eyebrow="WORKSPACE / GOVERNANCE"
        title="语义治理概览"
        description="从来源、定义、验证到消费绑定，检查工作区当前可信状态。"
        action={<button className="secondary-button" type="button" onClick={openAssets}>打开资产目录<ArrowRight size={16} /></button>}
      />

      <LifecycleRail scanCompleted={scanCompleted} candidateReady={candidateReady} publishedVersion={publishedVersion} onNavigate={onNavigate} />

      <section className="metric-strip" aria-label="工作区指标">
        <div><span>已发布资产</span><strong>148</strong><small>+6 本周</small></div>
        <div><span>可信覆盖率</span><strong>92.4%</strong><small>目标 95%</small></div>
        <div><span>待审核提案</span><strong>3</strong><small className="warning-text">1 个高风险</small></div>
        <div><span>可信解析</span><strong>1,824</strong><small>过去 7 天</small></div>
      </section>

      <div className="overview-grid">
        <section className="work-panel graph-panel" aria-labelledby="coverage-title">
          <div className="panel-heading">
            <div><span className="panel-kicker">TRUTH TRACE</span><h2 id="coverage-title">从数据源到消费者</h2></div>
            <div className="panel-meta"><span className="live-dot" />最近同步 9 分钟前</div>
          </div>
          <SemanticGraph mode="coverage" />
          <div className="coverage-footer">
            <div><strong>3</strong><span>已连接来源</span></div>
            <div><strong>8</strong><span>业务语义域</span></div>
            <div><strong>12</strong><span>绑定消费者</span></div>
            <button type="button" onClick={openAssets}>检查覆盖缺口<ChevronRight size={15} /></button>
          </div>
        </section>

        <section className="work-panel attention-panel" aria-labelledby="attention-title">
          <div className="panel-heading">
            <div><span className="panel-kicker">ATTENTION QUEUE</span><h2 id="attention-title">需要你的判断</h2></div>
            <span className="queue-count">3</span>
          </div>
          <div className="attention-list">
            <button type="button" onClick={() => openProposal(proposals[0])}>
              <span className="attention-icon violet"><Sparkles size={16} /></span>
              <span className="attention-copy"><strong>客单价口径变更</strong><small>AI 提案 · 中风险 · 12 分钟前</small></span>
              <ChevronRight size={16} />
            </button>
            <button type="button" onClick={openAssets}>
              <span className="attention-icon warning"><AlertTriangle size={16} /></span>
              <span className="attention-copy"><strong>业务区域缺少 owner</strong><small>共享维度 · 影响 3 个指标</small></span>
              <ChevronRight size={16} />
            </button>
            <button type="button" onClick={() => openProposal(proposals[2])}>
              <span className="attention-icon danger"><CircleAlert size={16} /></span>
              <span className="attention-copy"><strong>旧区域别名无法废弃</strong><small>3 个消费者尚未迁移</small></span>
              <ChevronRight size={16} />
            </button>
          </div>
          <div className="queue-summary"><Clock3 size={15} /><span>最早一项已等待 18 小时</span></div>
        </section>
      </div>

      <section className="domain-table" aria-labelledby="domain-title">
        <div className="panel-heading table-heading">
          <div><span className="panel-kicker">DOMAIN HEALTH</span><h2 id="domain-title">语义域健康度</h2></div>
          <button className="text-button" type="button" onClick={openAssets}>查看全部</button>
        </div>
        <div className="table-row table-head"><span>语义域</span><span>资产</span><span>Owner</span><span>验证覆盖</span><span>风险</span></div>
        <button className="table-row" type="button" onClick={openAssets}><strong>电商经营</strong><span>58</span><span>收入分析组</span><span><i style={{ width: "96%" }} />96%</span><StatusBadge tone="success">稳定</StatusBadge></button>
        <button className="table-row" type="button" onClick={openAssets}><strong>客户增长</strong><span>41</span><span>客户洞察组</span><span><i style={{ width: "84%" }} />84%</span><StatusBadge tone="warning">2 项关注</StatusBadge></button>
        <button className="table-row" type="button" onClick={openAssets}><strong>共享维度</strong><span>29</span><span>数据平台组</span><span><i style={{ width: "72%" }} />72%</span><StatusBadge tone="danger">Owner 缺失</StatusBadge></button>
      </section>
    </section>
  );
}

function AssetDefinition({ asset }: { asset: Asset }) {
  return (
    <div className="definition-layout">
      <section className="definition-main">
        <div className="content-block">
          <span className="content-label">业务定义</span>
          <p className="definition-copy">{asset.definition}</p>
        </div>
        <div className="content-block formula-block">
          <span className="content-label">计算表达式</span>
          <code>{asset.formula}</code>
        </div>
        <div className="property-grid">
          <div><span>数据粒度</span><strong>{asset.grain}</strong></div>
          <div><span>规范来源</span><strong>{asset.source}</strong></div>
          <div><span>稳定版本</span><strong>{asset.release}</strong></div>
          <div><span>最后验证</span><strong>{asset.updatedAt}</strong></div>
        </div>
        <div className="content-block">
          <span className="content-label">边界说明</span>
          <ul className="rule-list">
            <li><Check size={15} />包含支付成功且未被撤销的订单</li>
            <li><Check size={15} />退款按结算日冲减，不回写原支付日</li>
            <li><X size={15} />不包含测试订单和内部采购订单</li>
          </ul>
        </div>
      </section>
      <aside className="trust-rail">
        <span className="content-label">可信状态</span>
        <div className="trust-score"><strong>{asset.trustScore}</strong><span>/ 100</span></div>
        <div className="trust-meter"><i style={{ width: `${asset.trustScore}%` }} /></div>
        <p>{asset.trustScore >= 90 ? "定义、引用、验证与 owner 均完整。" : "存在待确认提案或证据覆盖缺口。"}</p>
        <dl>
          <div><dt>Owner</dt><dd>{asset.owner}</dd></div>
          <div><dt>语义域</dt><dd>{asset.domain}</dd></div>
          <div><dt>稳定 ID</dt><dd className="mono truncate" title={asset.id}>{asset.id}</dd></div>
        </dl>
      </aside>
    </div>
  );
}

function AssetEvidence({ asset }: { asset: Asset }) {
  if (asset.evidence.length === 0) {
    return <div className="empty-state"><BookOpenCheck size={28} /><strong>还没有验证证据</strong><span>该资产需要补充编译、owner 或使用证据。</span></div>;
  }
  return (
    <div className="evidence-list">
      {asset.evidence.map((evidence) => (
        <article key={evidence.id}>
          <span className="evidence-icon"><FileCheck2 size={18} /></span>
          <div><span className="panel-kicker">{evidence.kind}</span><strong>{evidence.label}</strong><small>{evidence.source}</small></div>
          <div className="evidence-time"><StatusBadge tone="success">已验证</StatusBadge><small>{evidence.verifiedAt}</small></div>
        </article>
      ))}
    </div>
  );
}

function AssetConsumers({ asset }: { asset: Asset }) {
  if (asset.consumers.length === 0) {
    return <div className="empty-state"><Users size={28} /><strong>还没有注册消费者</strong><span>发布后可通过 API、MCP 或 SDK 创建稳定 binding。</span></div>;
  }
  return (
    <div className="consumer-list">
      {asset.consumers.map((consumer) => (
        <article key={consumer.name}>
          <div className="consumer-mark"><Box size={18} /></div>
          <div><strong>{consumer.name}</strong><span>{consumer.kind}</span></div>
          <div><code>{consumer.binding}</code><small>最近解析 {consumer.lastResolved}</small></div>
          <StatusBadge tone="info">Bound</StatusBadge>
        </article>
      ))}
    </div>
  );
}

function AssetsView({ selectedId, onSelect }: { selectedId: string; onSelect: (id: string) => void }) {
  const [query, setQuery] = useState("");
  const [type, setType] = useState<AssetType | "全部">("全部");
  const [tab, setTab] = useState<AssetTab>("定义");
  const selected = assets.find((asset) => asset.id === selectedId) ?? assets[0];
  const filtered = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return assets.filter((asset) => {
      const matchesType = type === "全部" || asset.type === type;
      const matchesQuery = normalized.length === 0 || `${asset.name} ${asset.key} ${asset.domain}`.toLowerCase().includes(normalized);
      return matchesType && matchesQuery;
    });
  }, [query, type]);

  return (
    <section className="view view-assets">
      <PageHeading
        eyebrow="SEMANTIC REGISTRY / ASSETS"
        title="语义资产"
        description="查找已发布事实，检查定义、来源、关系、证据和消费范围。"
        action={<button className="secondary-button" type="button"><ListFilter size={16} />保存视图</button>}
      />
      <div className="asset-workbench">
        <section className="asset-catalog" aria-label="语义资产目录">
          <div className="catalog-toolbar">
            <label className="search-field">
              <Search size={16} />
              <input type="search" aria-label="搜索语义资产" placeholder="名称、key 或语义域" value={query} onChange={(event) => setQuery(event.target.value)} />
              <kbd>⌘ K</kbd>
            </label>
            <div className="segment-control" aria-label="资产类型筛选">
              {(["全部", "指标", "维度", "实体"] as const).map((item) => (
                <button key={item} type="button" aria-pressed={type === item} onClick={() => setType(item)}>{item}</button>
              ))}
            </div>
          </div>
          <div className="catalog-summary"><span>{filtered.length} 个资产</span><span>按可信度排序</span></div>
          <div className="asset-list">
            {filtered.map((asset) => (
              <button
                key={asset.id}
                type="button"
                className={asset.id === selected.id ? "asset-row asset-row-active" : "asset-row"}
                aria-label={`${asset.name}，${asset.type}，可信度 ${asset.trustScore}`}
                onClick={() => { onSelect(asset.id); setTab("定义"); }}
              >
                <span className={`asset-type-mark asset-type-${asset.type}`}><Box size={16} /></span>
                <span className="asset-row-copy"><strong>{asset.name}</strong><code>{asset.key}</code><small>{asset.domain} · {asset.owner}</small></span>
                <span className="asset-row-state"><b>{asset.trustScore}</b><StatusBadge tone={statusTone(asset.status)}>{asset.status}</StatusBadge></span>
              </button>
            ))}
            {filtered.length === 0 && <div className="catalog-empty"><Search size={24} /><strong>没有匹配资产</strong><span>调整关键词或资产类型。</span><button type="button" onClick={() => { setQuery(""); setType("全部"); }}>清除筛选</button></div>}
          </div>
        </section>

        <section className="asset-detail" aria-label="语义资产详情">
          <header className="asset-detail-header">
            <div className={`asset-type-mark asset-type-${selected.type}`}><Box size={19} /></div>
            <div className="asset-title-copy"><span className="panel-kicker">{selected.type} · {selected.key}</span><h2>{selected.name}</h2><p>{selected.definition}</p></div>
            <div className="asset-header-state"><StatusBadge tone={statusTone(selected.status)}><BadgeCheck size={13} />{selected.status}</StatusBadge><code>{selected.release}</code></div>
          </header>
          <div className="asset-tabs" role="tablist" aria-label="资产详情视图">
            {assetTabs.map((item) => (
              <button key={item} role="tab" type="button" aria-selected={tab === item} onClick={() => setTab(item)}>{item}</button>
            ))}
          </div>
          <div className="asset-tab-content" role="tabpanel">
            {tab === "定义" && <AssetDefinition asset={selected} />}
            {tab === "血缘" && <div className="lineage-content"><div className="lineage-copy"><span className="content-label">关系摘要</span><p>{selected.upstream.length} 个上游定义，{selected.downstream.length} 个下游资产。当前 release 中没有 unresolved reference。</p></div><SemanticGraph mode="lineage" assetName={selected.name} upstream={selected.upstream} downstream={selected.downstream} consumerName={selected.consumers[0]?.name ?? "尚未绑定"} /></div>}
            {tab === "证据" && <AssetEvidence asset={selected} />}
            {tab === "消费者" && <AssetConsumers asset={selected} />}
          </div>
        </section>
      </div>
    </section>
  );
}

function ProposalDetail({ proposal, onReview }: { proposal: Proposal; onReview: () => void }) {
  const passed = proposal.validations.filter((item) => item.state === "passed").length;
  return (
    <section className="proposal-detail" aria-label={`${proposal.id} 提案详情`}>
      <header className="proposal-detail-header">
        <div><span className="panel-kicker">{proposal.id} · {proposal.author}</span><h2>{proposal.title}</h2><p>{proposal.summary}</p></div>
        <button className="primary-button" type="button" aria-label={`审核 ${proposal.id}`} onClick={onReview}><ShieldCheck size={16} />进入审核</button>
      </header>
      <div className="proposal-metadata">
        <div><span>基础资产</span><strong>{proposal.assetName}</strong></div>
        <div><span>风险等级</span><StatusBadge tone={proposal.risk === "高风险" ? "danger" : proposal.risk === "中风险" ? "warning" : "success"}>{proposal.risk}</StatusBadge></div>
        <div><span>验证结果</span><strong>{passed}/{proposal.validations.length} passed</strong></div>
        <div><span>影响消费者</span><strong>{proposal.impact.length}</strong></div>
      </div>
      <div className="proposal-columns">
        <section className="diff-section">
          <div className="subsection-heading"><div><span className="panel-kicker">STRUCTURED PATCH</span><h3>字段变更</h3></div><StatusBadge tone="violet"><Sparkles size={13} />AI authored</StatusBadge></div>
          {proposal.changes.map((change) => (
            <article className="diff-block" key={change.field}>
              <code className="diff-field">{change.field}</code>
              <div className="diff-line diff-before"><span>−</span><p>{change.before}</p></div>
              <div className="diff-line diff-after"><span>+</span><p>{change.after}</p></div>
            </article>
          ))}
        </section>
        <section className="validation-section">
          <div className="subsection-heading"><div><span className="panel-kicker">VALIDATION RUN</span><h3>验证证据</h3></div><code>VAL-{proposal.id.slice(-3)}</code></div>
          <div className="validation-list">
            {proposal.validations.map((item) => (
              <div className={`validation-item validation-${item.state}`} key={item.name}>
                <span>{validationIcon(item.state)}</span><div><strong>{item.name}</strong><small>{item.detail}</small></div>
              </div>
            ))}
          </div>
          <div className="impact-block"><span className="content-label">影响范围</span>{proposal.impact.map((item) => <span key={item}><ArrowRight size={13} />{item}</span>)}</div>
        </section>
      </div>
    </section>
  );
}

function ProposalsView({ selectedId, decisions, onSelect, onReview }: { selectedId: string; decisions: Partial<Record<string, "批准" | "退回">>; onSelect: (id: string) => void; onReview: (proposal: Proposal) => void }) {
  const selected = proposals.find((proposal) => proposal.id === selectedId) ?? proposals[0];
  return (
    <section className="view view-proposals">
      <PageHeading eyebrow="GOVERNANCE / PROPOSALS" title="变更提案" description="比较结构化变更、验证证据与消费影响，再决定是否进入发布。" />
      <div className="proposal-workbench">
        <aside className="proposal-queue" aria-label="提案队列">
          <div className="queue-filter"><button className="filter-active" type="button">待审核 <span>3</span></button><button type="button">我的审核</button></div>
          {proposals.map((proposal) => (
            <button key={proposal.id} type="button" className={selected.id === proposal.id ? "proposal-row proposal-row-active" : "proposal-row"} onClick={() => onSelect(proposal.id)}>
              <span className={proposal.author === "Semlia AI" ? "proposal-author ai-author" : "proposal-author"}>{proposal.author === "Semlia AI" ? <Sparkles size={16} /> : proposal.author.slice(0, 1)}</span>
              <span className="proposal-row-copy"><span className="panel-kicker">{proposal.id} · {proposal.createdAt}</span><strong>{proposal.title}</strong><small>{proposal.assetName} · {proposal.author}</small></span>
              {decisions[proposal.id] ? <StatusBadge tone={decisions[proposal.id] === "批准" ? "success" : "danger"}>会话已{decisions[proposal.id]}</StatusBadge> : <StatusBadge tone={proposal.risk === "高风险" ? "danger" : proposal.risk === "中风险" ? "warning" : "success"}>{proposal.risk}</StatusBadge>}
            </button>
          ))}
        </aside>
        <ProposalDetail proposal={selected} onReview={() => onReview(selected)} />
      </div>
    </section>
  );
}

function ReleasesView({ candidateReady, publishedVersion, rollbackPerformed, onPublish, onRollback, onOpenConsumers }: { candidateReady: boolean; publishedVersion: string; rollbackPerformed: boolean; onPublish: () => void; onRollback: () => void; onOpenConsumers: () => void }) {
  const [selectedId, setSelectedId] = useState(releases[0].id);
  const selected = releases.find((release) => release.id === selectedId) ?? releases[0];
  return (
    <section className="view view-releases">
      <PageHeading
        eyebrow="DISTRIBUTION / RELEASES"
        title="不可变发布版本"
        description="每次发布固定资产 revision、验证证据与消费者 binding，任何变化都通过新版本演进。"
        action={<button className="secondary-button" type="button"><RotateCcw size={16} />比较版本</button>}
      />
      {candidateReady ? (
        <section className={publishedVersion ? "candidate-band candidate-published" : "candidate-band"} aria-labelledby="candidate-title">
          <span className="candidate-mark">{publishedVersion ? <BadgeCheck size={19} /> : <PackageCheck size={19} />}</span>
          <div><span className="panel-kicker">SESSION HANDOFF</span><h2 id="candidate-title">Release candidate</h2><p>{publishedVersion ? "本次会话已完成模拟发布，稳定 release 和 binding 均未写入后端。" : "PROP-128 已获批准，4 项验证证据和 3 个消费影响已固化到 candidate。"}</p></div>
          {publishedVersion ? <div className="candidate-result"><code>{publishedVersion}</code><StatusBadge tone="success">Session published</StatusBadge></div> : <button className="primary-button" type="button" onClick={onPublish}><PackageCheck size={16} />模拟发布 candidate</button>}
        </section>
      ) : (
        <section className="candidate-empty" aria-label="发布候选为空"><Clock3 size={17} /><span><strong>还没有 release candidate</strong>批准至少一个通过验证的提案后，系统才会准备发布清单。</span><button className="text-button" type="button">查看审核规则</button></section>
      )}
      {rollbackPerformed && <div className="rollback-notice" role="status"><RotateCcw size={16} /><span>模拟回滚完成：会话 binding 已恢复到 release-2026.08.3。</span></div>}
      <div className="release-layout">
        <aside className="release-timeline" aria-label="发布版本列表">
          <div className="timeline-axis" aria-hidden="true" />
          {releases.map((release) => (
            <button key={release.id} type="button" className={release.id === selected.id ? "release-row release-row-active" : "release-row"} onClick={() => setSelectedId(release.id)}>
              <span className="timeline-dot" />
              <span className="release-row-copy"><span>{release.publishedAt}</span><strong>{release.id}</strong><small>{release.summary}</small></span>
              <StatusBadge tone={release.state === "Stable" ? "success" : "neutral"}>{release.state}</StatusBadge>
            </button>
          ))}
        </aside>
        <section className="release-detail">
          <header className="release-detail-header">
            <div className="release-seal"><PackageCheck size={24} /></div>
            <div><span className="panel-kicker">IMMUTABLE MANIFEST</span><h2>{selected.id}</h2><p>{selected.summary}</p></div>
            <StatusBadge tone={selected.state === "Stable" ? "success" : "neutral"}><BadgeCheck size={13} />{selected.state}</StatusBadge>
          </header>
          <div className="manifest-strip"><div><span>资产 revisions</span><strong>{selected.assetCount}</strong></div><div><span>发布者</span><strong>{selected.publisher}</strong></div><div><span>发布时间</span><strong>{selected.publishedAt}</strong></div><div><span>Manifest hash</span><code>sha256:9f2e...a84c</code></div></div>
          <section className="release-changes">
            <div className="subsection-heading"><div><span className="panel-kicker">CHANGESET</span><h3>发布内容</h3></div><button className="text-button" type="button">查看 manifest</button></div>
            <div className="change-list">{selected.changes.map((change) => <div key={change}><CheckCircle2 size={16} /><span>{change}</span></div>)}</div>
          </section>
          <section className="release-bindings">
            <div className="subsection-heading"><div><span className="panel-kicker">CONSUMER BINDINGS</span><h3>已绑定消费者</h3></div><button className="text-button" type="button" onClick={onOpenConsumers}>{selected.consumers.length} active <ArrowRight size={13} /></button></div>
            {selected.consumers.length > 0 ? selected.consumers.map((consumer) => (
              <article key={consumer.name}>
                <span className="consumer-mark"><Box size={18} /></span>
                <div><strong>{consumer.name}</strong><small>{consumer.kind} · 最近解析 {consumer.lastResolved}</small></div>
                <code>{consumer.binding}</code>
                <StatusBadge tone="info">Healthy</StatusBadge>
              </article>
            )) : <div className="empty-inline">该历史 release 已没有 active binding。</div>}
          </section>
          <footer className="release-actions"><div><strong>回滚改变 binding，不改写不可变 release</strong><span>原型只记录本次会话状态。</span></div><button className="secondary-button" type="button" onClick={onRollback} disabled={!publishedVersion}><RotateCcw size={16} />模拟回滚绑定</button></footer>
        </section>
      </div>
    </section>
  );
}

function ReviewDialog({ proposal, onClose, onDecision }: { proposal: Proposal; onClose: () => void; onDecision: (decision: "批准" | "退回") => void }) {
  const closeButtonRef = useRef<HTMLButtonElement>(null);

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
          <div><span className="panel-kicker">PROTOTYPE REVIEW · {proposal.id}</span><h2 id="review-dialog-title">审核提案 {proposal.id}</h2></div>
          <button ref={closeButtonRef} className="icon-button" type="button" aria-label="关闭审核" onClick={onClose}><X size={18} /></button>
        </header>
        <div className="prototype-notice"><CircleAlert size={17} /><span>仅更新本次原型会话，不会写入或发布真实数据。</span></div>
        <div className="dialog-body">
          <div className="dialog-summary"><span className="proposal-author ai-author"><Sparkles size={16} /></span><div><strong>{proposal.title}</strong><p>{proposal.summary}</p></div></div>
          <div className="dialog-validation">
            {proposal.validations.map((item) => (
              <div className={`validation-item validation-${item.state}`} key={item.name}><span>{validationIcon(item.state)}</span><div><strong>{item.name}</strong><small>{item.detail}</small></div></div>
            ))}
          </div>
          <label className="review-note"><span>审核意见</span><textarea placeholder="记录判断依据，原型中不会保存。" /></label>
        </div>
        <footer>
          <button className="danger-button" type="button" onClick={() => onDecision("退回")}><RotateCcw size={16} />模拟退回</button>
          <div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" onClick={() => onDecision("批准")}><Check size={16} />模拟批准</button></div>
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
        <header><div><span className="panel-kicker">HUMAN INITIATED</span><h2 id="create-proposal-title">创建变更提案</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭提案创建" onClick={onClose}><X size={18} /></button></header>
        <div className="prototype-notice"><CircleAlert size={17} /><span>原型会创建可见的会话草稿，但不会持久化语义资产。</span></div>
        <div className="dialog-body proposal-form">
          <label><span>基础资产</span><select aria-label="基础资产" defaultValue={assets[1].id}>{assets.map((asset) => <option key={asset.id} value={asset.id}>{asset.name} · {asset.key}</option>)}</select></label>
          <label><span>变更意图</span><textarea aria-label="变更意图" defaultValue="补充客单价退款订单口径的业务边界与验证证据。" /></label>
          <div className="proposal-policy"><ShieldCheck size={18} /><span><strong>治理级别 G1</strong><small>AI 可补全草稿和运行预检，发布前必须由资产 owner 审核。</small></span></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" onClick={onCreate}><Sparkles size={16} />创建会话草稿</button></div></footer>
      </section>
    </div>
  );
}

function PublishDialog({ onClose, onPublish }: { onClose: () => void; onPublish: () => void }) {
  const closeRef = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog" role="dialog" aria-modal="true" aria-labelledby="publish-dialog-title">
        <header><div><span className="panel-kicker">IMMUTABLE HANDOFF</span><h2 id="publish-dialog-title">发布 release candidate</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭发布确认" onClick={onClose}><X size={18} /></button></header>
        <div className="prototype-notice"><CircleAlert size={17} /><span>这是模拟发布，只更新当前浏览器会话，不会生成真实 manifest 或改变消费者 binding。</span></div>
        <div className="dialog-body publish-checklist">
          <div><CheckCircle2 size={17} /><span><strong>结构与引用验证</strong><small>4 / 4 passed</small></span></div>
          <div><CheckCircle2 size={17} /><span><strong>治理策略</strong><small>G1 owner approval satisfied</small></span></div>
          <div><CheckCircle2 size={17} /><span><strong>消费影响</strong><small>3 个消费者可向后兼容</small></span></div>
          <div className="publish-target"><span>目标版本</span><code>release-2026.08.4-session</code></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" onClick={onPublish}><PackageCheck size={16} />确认模拟发布</button></div></footer>
      </section>
    </div>
  );
}

export function App() {
  const [view, setView] = useState<ViewId>("overview");
  const [contextIndex, setContextIndex] = useState(0);
  const [selectedAssetId, setSelectedAssetId] = useState(assets[0].id);
  const [selectedProposalId, setSelectedProposalId] = useState(proposals[0].id);
  const [reviewing, setReviewing] = useState<Proposal | null>(null);
  const [decisions, setDecisions] = useState<Partial<Record<string, "批准" | "退回">>>({});
  const [scanCompleted, setScanCompleted] = useState(false);
  const [candidateReady, setCandidateReady] = useState(false);
  const [publishedVersion, setPublishedVersion] = useState("");
  const [rollbackPerformed, setRollbackPerformed] = useState(false);
  const [sessionBindings, setSessionBindings] = useState<SessionBinding[]>([]);
  const [creatingProposal, setCreatingProposal] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [toast, setToast] = useState("");

  const showPrototypeToast = (message: string) => {
    setToast(message);
    window.setTimeout(() => setToast(""), 3600);
  };

  const openProposal = (proposal: Proposal) => {
    setSelectedProposalId(proposal.id);
    setView("proposals");
    setReviewing(proposal);
  };

  const onDecision = (decision: "批准" | "退回") => {
    if (!reviewing) return;
    setDecisions((current) => ({ ...current, [reviewing.id]: decision }));
    if (decision === "批准") setCandidateReady(true);
    setToast(`${reviewing.id} 已在本次原型会话中标记为${decision}，刷新后重置。`);
    setReviewing(null);
  };

  const navigate = (nextView: ViewId) => {
    setView(nextView);
    setContextIndex(0);
  };

  const selectContext = (index: number) => {
    setContextIndex(index);
    if (view === "overview" && index === 1) navigate("proposals");
    if (view === "overview" && index === 3) navigate("consumers");
    if (view === "sources" && index === 2 && scanCompleted) navigate("proposals");
    if (view === "releases" && index === 3) navigate("consumers");
  };

  const publishCandidate = () => {
    setPublishedVersion("release-2026.08.4-session");
    setPublishing(false);
    setRollbackPerformed(false);
    showPrototypeToast("模拟发布完成：candidate 已成为本次会话的 active release。");
  };

  return (
    <div className="app-shell">
      <ActivityRail view={view} onChange={navigate} />
      <ContextPanel view={view} activeIndex={contextIndex} onSelect={selectContext} />
      <div className="workspace">
        <Topbar view={view} onCreateProposal={() => setCreatingProposal(true)} />
        <main className="workspace-canvas">
          {view === "sources" && <SourcesView scanCompleted={scanCompleted} onRunDiscovery={() => { setScanCompleted(true); showPrototypeToast("增量发现已完成：6 个建议已进入可审核队列。"); }} onOpenProposals={() => navigate("proposals")} onOpenAssets={() => navigate("assets")} onNotify={showPrototypeToast} />}
          {view === "overview" && <OverviewView openAssets={() => navigate("assets")} openProposal={openProposal} scanCompleted={scanCompleted} candidateReady={candidateReady} publishedVersion={publishedVersion} onNavigate={navigate} />}
          {view === "assets" && <AssetsView selectedId={selectedAssetId} onSelect={setSelectedAssetId} />}
          {view === "proposals" && <ProposalsView selectedId={selectedProposalId} decisions={decisions} onSelect={setSelectedProposalId} onReview={setReviewing} />}
          {view === "releases" && <ReleasesView candidateReady={candidateReady} publishedVersion={publishedVersion} rollbackPerformed={rollbackPerformed} onPublish={() => setPublishing(true)} onRollback={() => { setRollbackPerformed(true); showPrototypeToast("模拟回滚已完成，本次会话 binding 已恢复到稳定版本。"); }} onOpenConsumers={() => navigate("consumers")} />}
          {view === "consumers" && <ConsumersView bindings={sessionBindings} activeRelease={publishedVersion || releases[0].id} onCreateBinding={(binding) => { setSessionBindings((current) => [...current, binding]); showPrototypeToast(`${binding.name} 已创建本次会话 binding。`); }} onCreateFixProposal={() => { setSelectedProposalId(proposals[2].id); navigate("proposals"); showPrototypeToast("运行异常已转换为可审核的修复提案草稿。"); }} onNotify={showPrototypeToast} />}
        </main>
      </div>
      {reviewing && <ReviewDialog proposal={reviewing} onClose={() => setReviewing(null)} onDecision={onDecision} />}
      {creatingProposal && <CreateProposalDialog onClose={() => setCreatingProposal(false)} onCreate={() => { setCreatingProposal(false); setSelectedProposalId(proposals[0].id); navigate("proposals"); showPrototypeToast("会话提案草稿已创建并进入审核队列。"); }} />}
      {publishing && <PublishDialog onClose={() => setPublishing(false)} onPublish={publishCandidate} />}
      {toast && <div className="toast" role="status"><CheckCircle2 size={17} /><span>{toast}</span></div>}
    </div>
  );
}

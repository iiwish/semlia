import { useEffect, useRef, useState } from "react";
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Braces,
  CheckCircle2,
  CircleAlert,
  Database,
  FileSearch,
  Link2,
  Play,
  Radio,
  Server,
  ShieldCheck,
  Sparkles,
  Terminal,
  X,
} from "lucide-react";

export interface SessionBinding {
  name: string;
  channel: "REST API" | "MCP" | "CLI" | "TypeScript SDK";
  release: string;
}

interface SourcesViewProps {
  scanCompleted: boolean;
  onRunDiscovery: () => void;
  onOpenProposals: () => void;
  onOpenAssets: () => void;
  onNotify: (message: string) => void;
}

interface ConsumersViewProps {
  bindings: SessionBinding[];
  activeRelease: string;
  onCreateBinding: (binding: SessionBinding) => void;
  onCreateFixProposal: () => void;
  onNotify: (message: string) => void;
}

function JourneyHeading({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: React.ReactNode }) {
  return (
    <div className="page-heading">
      <div><span className="eyebrow">{eyebrow}</span><h1>{title}</h1><p>{description}</p></div>
      {action && <div className="page-heading-action">{action}</div>}
    </div>
  );
}

export function SourcesView({ scanCompleted, onRunDiscovery, onOpenProposals, onOpenAssets, onNotify }: SourcesViewProps) {
  return (
    <section className="view view-sources">
      <JourneyHeading
        eyebrow="FOUNDATION / DISCOVERY"
        title="接入与语义发现"
        description="以只读方式连接元数据来源，验证工作区边界，再把发现结果送入可审核提案。"
        action={<button className="secondary-button" type="button" onClick={() => onNotify("连接测试通过：3 个来源均可读取元数据，未申请事实数据权限。")}>测试全部连接<CheckCircle2 size={16} /></button>}
      />

      <div className="readiness-strip" aria-label="工作区就绪状态">
        <div><span className="readiness-icon"><ShieldCheck size={18} /></span><span><small>权限边界</small><strong>仅元数据读取</strong></span><b>Ready</b></div>
        <div><span className="readiness-icon"><Database size={18} /></span><span><small>来源连接</small><strong>3 / 3 健康</strong></span><b>Ready</b></div>
        <div><span className="readiness-icon"><FileSearch size={18} /></span><span><small>发现策略</small><strong>增量 · 人工审核</strong></span><b>G1</b></div>
      </div>

      <div className="source-layout">
        <section className="source-registry" aria-labelledby="source-registry-title">
          <div className="panel-heading"><div><span className="panel-kicker">SOURCE REGISTRY</span><h2 id="source-registry-title">数据与语义来源</h2></div><span className="source-count">3 connected</span></div>
          <div className="source-list">
            <article><span className="source-mark"><Braces size={18} /></span><div><strong>Cube Commerce</strong><small>Semantic model · 42 cubes · 148 measures</small></div><code>cube://commerce</code><span className="health-label"><i />Healthy</span></article>
            <article><span className="source-mark"><Terminal size={18} /></span><div><strong>dbt Analytics</strong><small>Manifest · 214 models · 692 tests</small></div><code>main@8f2d9ac</code><span className="health-label"><i />Healthy</span></article>
            <article><span className="source-mark"><Server size={18} /></span><div><strong>PostgreSQL Metadata</strong><small>Catalog only · 36 schemas · no fact rows</small></div><code>analytics_ro</code><span className="health-label"><i />Healthy</span></article>
          </div>
          <div className="permission-boundary"><ShieldCheck size={17} /><div><strong>最小权限边界已生效</strong><span>发现任务只能读取 catalog、schema、lineage 与查询统计摘要，不能读取业务事实行。</span></div></div>
        </section>

        <section className="discovery-console" aria-labelledby="discovery-title">
          <div className="panel-heading"><div><span className="panel-kicker">AI DISCOVERY RUN</span><h2 id="discovery-title">增量语义发现</h2></div><span className={scanCompleted ? "run-state run-complete" : "run-state"}>{scanCompleted ? "Complete" : "Ready"}</span></div>
          <div className="discovery-body">
            <div className="discovery-scope"><span>本次范围</span><strong>自 release-2026.08.3 后的变更</strong><small>Cube schema、dbt manifest、数据库 catalog 与最近 7 天使用摘要</small></div>
            <ol className="discovery-steps">
              <li className={scanCompleted ? "step-complete" : "step-ready"}><CheckCircle2 size={16} /><span><strong>结构差异</strong><small>检测新增、重命名与引用变化</small></span></li>
              <li className={scanCompleted ? "step-complete" : "step-ready"}><CheckCircle2 size={16} /><span><strong>语义候选</strong><small>生成定义、owner 与证据建议</small></span></li>
              <li className={scanCompleted ? "step-complete" : "step-ready"}><CheckCircle2 size={16} /><span><strong>治理预检</strong><small>标注风险，不自动发布</small></span></li>
            </ol>
            {scanCompleted ? (
              <div className="discovery-result" role="status">
                <CheckCircle2 size={20} /><div><strong>发现完成：6 个提案</strong><span>2 个定义建议、3 个证据补充、1 个 breaking change 风险。</span></div>
                <button className="primary-button" type="button" onClick={onOpenProposals}>查看 6 个提案<ArrowRight size={16} /></button>
              </div>
            ) : (
              <button className="primary-button discovery-run-button" type="button" onClick={onRunDiscovery}><Play size={16} />运行增量发现</button>
            )}
          </div>
        </section>
      </div>

      <section className="discovery-trace" aria-labelledby="trace-title">
        <div><span className="panel-kicker">AUDIT TRACE</span><h2 id="trace-title">每条建议都保留来源证据</h2><p>AI 负责发现和起草，治理策略决定审核级别，只有批准后的 revision 才能进入 release candidate。</p></div>
        <button className="text-button" type="button" onClick={onOpenAssets}>检查现有资产证据<ArrowRight size={14} /></button>
      </section>
    </section>
  );
}

function BindingDialog({ activeRelease, onClose, onCreate }: { activeRelease: string; onClose: () => void; onCreate: (binding: SessionBinding) => void }) {
  const [name, setName] = useState("");
  const [channel, setChannel] = useState<SessionBinding["channel"]>("MCP");
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    closeRef.current?.focus();
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog binding-dialog" role="dialog" aria-modal="true" aria-labelledby="binding-dialog-title">
        <header><div><span className="panel-kicker">SESSION BINDING</span><h2 id="binding-dialog-title">创建消费绑定</h2></div><button ref={closeRef} className="icon-button" type="button" aria-label="关闭绑定创建" onClick={onClose}><X size={18} /></button></header>
        <div className="prototype-notice"><CircleAlert size={17} /><span>仅创建本次原型会话中的模拟 binding，不生成真实凭据。</span></div>
        <div className="dialog-body binding-form">
          <label><span>消费者名称</span><input aria-label="消费者名称" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如 Revenue Copilot" /></label>
          <label><span>消费通道</span><select aria-label="消费通道" value={channel} onChange={(event) => setChannel(event.target.value as SessionBinding["channel"])}><option>MCP</option><option>REST API</option><option>CLI</option><option>TypeScript SDK</option></select></label>
          <div className="binding-target"><span>绑定版本</span><code>{activeRelease}</code><small>锁定不可变 release，升级需要新的 binding revision。</small></div>
        </div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="button" disabled={!name.trim()} onClick={() => onCreate({ name: name.trim(), channel, release: activeRelease })}><Link2 size={16} />创建模拟绑定</button></div></footer>
      </section>
    </div>
  );
}

const channelDetails = [
  { name: "REST API", icon: Braces, detail: "稳定查询与资产解析", value: "POST /v1/semantic/resolve" },
  { name: "MCP", icon: Sparkles, detail: "给 AI Agent 受控语义工具", value: "semlia.resolve_asset" },
  { name: "CLI", icon: Terminal, detail: "CI 校验与本地检查", value: "semlia binding verify" },
  { name: "TypeScript SDK", icon: Link2, detail: "类型安全的产品集成", value: "client.assets.resolve()" },
] as const;

export function ConsumersView({ bindings, activeRelease, onCreateBinding, onCreateFixProposal, onNotify }: ConsumersViewProps) {
  const [creating, setCreating] = useState(false);
  const handleCreate = (binding: SessionBinding) => {
    onCreateBinding(binding);
    setCreating(false);
  };

  return (
    <section className="view view-consumers">
      <JourneyHeading
        eyebrow="DISTRIBUTION / OBSERVABILITY"
        title="消费与反馈"
        description="通过版本化接口交付可信语义，并把解析失败、漂移与真实使用重新送回治理队列。"
        action={<button className="primary-button" type="button" onClick={() => setCreating(true)}><Link2 size={16} />创建绑定</button>}
      />

      <section className="channel-band" aria-labelledby="channels-title">
        <div className="panel-heading"><div><span className="panel-kicker">DELIVERY CHANNELS</span><h2 id="channels-title">面向产品与 Agent 的稳定接口</h2></div><code>{activeRelease}</code></div>
        <div className="channel-grid">
          {channelDetails.map((channel) => {
            const Icon = channel.icon;
            return <button type="button" key={channel.name} onClick={() => onNotify(`${channel.name} 示例已选中：${channel.value}`)}><span className="channel-icon"><Icon size={18} /></span><span><strong>{channel.name}</strong><small>{channel.detail}</small><code>{channel.value}</code></span><ArrowRight size={15} /></button>;
          })}
        </div>
      </section>

      <div className="consumer-layout">
        <section className="binding-registry" aria-labelledby="bindings-title">
          <div className="panel-heading"><div><span className="panel-kicker">ACTIVE BINDINGS</span><h2 id="bindings-title">消费绑定</h2></div><span>{2 + bindings.length} active</span></div>
          <div className="binding-list">
            <article><span className="consumer-mark"><Radio size={18} /></span><div><strong>Fluxale Production</strong><small>BI Agent · 1,284 resolves / 30d</small></div><code>{activeRelease}</code><span className="health-label"><i />Healthy</span></article>
            <article><span className="consumer-mark"><Activity size={18} /></span><div><strong>经营日报</strong><small>Dashboard · 642 resolves / 30d</small></div><code>^2026.08</code><span className="health-label"><i />Healthy</span></article>
            {bindings.map((binding) => <article className="session-binding" key={`${binding.name}-${binding.channel}`}><span className="consumer-mark"><Link2 size={18} /></span><div><strong>{binding.name}</strong><small>{binding.channel} · 本次原型会话</small></div><code>{binding.release}</code><span className="health-label"><i />Session</span></article>)}
          </div>
        </section>

        <section className="feedback-panel" aria-labelledby="feedback-title">
          <div className="panel-heading"><div><span className="panel-kicker">GOVERNANCE FEEDBACK</span><h2 id="feedback-title">运行反馈</h2></div><span className="run-state run-warning">1 action</span></div>
          <div className="feedback-metrics"><div><span>可信解析成功率</span><strong>99.72%</strong><small>过去 7 天 · 2,184 次</small></div><div><span>版本漂移</span><strong>2</strong><small>消费者仍绑定旧 minor</small></div></div>
          <div className="feedback-events">
            <article><span className="feedback-icon warning"><AlertTriangle size={17} /></span><div><strong>区域维度别名解析失败</strong><small>区域预测任务 · 过去 24 小时 18 次</small></div><span>Needs action</span></article>
            <article><span className="feedback-icon"><CheckCircle2 size={17} /></span><div><strong>客单价回归恢复稳定</strong><small>发布后观测窗口 · 4 个区域均通过</small></div><span>Resolved</span></article>
          </div>
          <button className="secondary-button feedback-action" type="button" onClick={onCreateFixProposal}><Sparkles size={16} />生成修复提案</button>
        </section>
      </div>

      <div className="consumer-boundary"><CircleAlert size={16} /><span>原型仅演示 API、MCP、CLI 与 SDK 的产品合同和反馈闭环，不会调用真实服务或签发凭据。</span></div>
      {creating && <BindingDialog activeRelease={activeRelease} onClose={() => setCreating(false)} onCreate={handleCreate} />}
    </section>
  );
}

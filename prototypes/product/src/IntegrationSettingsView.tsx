import { createPortal } from "react-dom";
import { useState, type FormEvent } from "react";
import { BookOpenText, Bot, Braces, Cable, Check, Clipboard, KeyRound, Pause, Play, Plus, RotateCw, Search, Server, ShieldCheck, Terminal, X } from "lucide-react";

interface IntegrationChannel {
  id: "rest" | "mcp" | "cli" | "sdk";
  name: string;
  description: string;
  endpoint: string;
  capability: string;
  enabled: boolean;
}

interface IntegrationClient {
  id: string;
  name: string;
  channel: "REST API" | "MCP" | "CLI";
  environment: "生产" | "测试" | "开发";
  credential: string;
  lastUsed: string;
  enabled: boolean;
}

interface IntegrationGuide {
  authentication: string;
  scenario: string;
  steps: string[];
  example: string;
}

const channelIcons = { rest: Cable, mcp: Bot, cli: Terminal, sdk: Braces } as const;

const channelGuides: Record<IntegrationChannel["id"], IntegrationGuide> = {
  rest: {
    authentication: "Bearer 工作区客户端凭据",
    scenario: "服务端查询、内部应用与自动化流程",
    steps: ["创建 REST API 客户端并保存一次性凭据。", "将凭据写入 Authorization 请求头。", "调用知识检索端点并固定使用响应中的资产版本。"],
    example: "curl -s 'https://api.semlia.example/v1/knowledge/search?q=净收入' \\\n  -H 'Authorization: Bearer $SEMLIA_API_KEY'",
  },
  mcp: {
    authentication: "Bearer MCP 客户端凭据",
    scenario: "Codex、AI Agent 与支持 MCP 的工作台",
    steps: ["创建 MCP 客户端并选择对应环境。", "在 AI 客户端中登记服务器地址与凭据。", "连接后仅调用已发布的语义工具和资产版本。"],
    example: `{
  "mcpServers": {
    "semlia": {
      "url": "https://mcp.semlia.example/sse",
      "headers": { "Authorization": "Bearer \${SEMLIA_MCP_TOKEN}" }
    }
  }
}`,
  },
  cli: {
    authentication: "SEMLIA_API_KEY 环境变量",
    scenario: "本地诊断、CI 校验与批量导出",
    steps: ["创建 CLI 客户端并保存凭据。", "在本机或 CI Secret 中设置 SEMLIA_API_KEY。", "运行 query、validate 或 export 命令。"],
    example: "export SEMLIA_API_KEY='••••••••'\npnpm dlx @semlia/cli query '净收入'",
  },
  sdk: {
    authentication: "服务端工作区客户端凭据",
    scenario: "TypeScript 或 Python 应用服务",
    steps: ["安装 Semlia SDK。", "从服务端环境变量读取客户端凭据。", "初始化客户端并查询已发布知识，禁止向浏览器暴露凭据。"],
    example: "import { Semlia } from '@semlia/sdk';\n\nconst semlia = new Semlia({ apiKey: process.env.SEMLIA_API_KEY });\nconst result = await semlia.knowledge.search({ query: '净收入' });",
  },
};

export function IntegrationSettingsView({ onNotify }: { onNotify: (message: string) => void }) {
  const [channels, setChannels] = useState<IntegrationChannel[]>([
    { id: "rest", name: "REST API", description: "查询已发布语义资产、定义与版本化解析结果。", endpoint: "https://api.semlia.example/v1", capability: "12 个端点 · OpenAPI 3.1", enabled: true },
    { id: "mcp", name: "MCP Server", description: "向 AI 客户端暴露可信语义检索与资产解析工具。", endpoint: "https://mcp.semlia.example/sse", capability: "6 个工具 · Streamable HTTP", enabled: true },
    { id: "cli", name: "CLI", description: "在 CI 或终端中验证、查询和导出语义资产。", endpoint: "pnpm dlx @semlia/cli", capability: "query · validate · export", enabled: true },
    { id: "sdk", name: "SDK", description: "在应用服务中使用类型化客户端调用 Semlia。", endpoint: "@semlia/sdk", capability: "TypeScript · Python", enabled: true },
  ]);
  const [clients, setClients] = useState<IntegrationClient[]>([
    { id: "client-fluxale", name: "Fluxale Production", channel: "REST API", environment: "生产", credential: "sk_live_••••7K2A", lastUsed: "2 分钟前", enabled: true },
    { id: "client-codex", name: "Codex MCP Workspace", channel: "MCP", environment: "开发", credential: "mcp_••••91HF", lastUsed: "18 分钟前", enabled: true },
    { id: "client-ci", name: "Semantic Release CI", channel: "CLI", environment: "生产", credential: "sk_ci_••••4DPQ", lastUsed: "昨天 23:10", enabled: true },
  ]);
  const [query, setQuery] = useState("");
  const [creatingClient, setCreatingClient] = useState(false);
  const [guideChannelId, setGuideChannelId] = useState<IntegrationChannel["id"] | null>(null);
  const [createdCredential, setCreatedCredential] = useState<{ name: string; secret: string } | null>(null);
  const [draft, setDraft] = useState({ name: "", channel: "REST API" as IntegrationClient["channel"], environment: "生产" as IntegrationClient["environment"] });
  const normalizedQuery = query.trim().toLocaleLowerCase("zh-CN");
  const visibleClients = clients.filter((client) => !normalizedQuery || `${client.name} ${client.channel} ${client.environment} ${client.credential}`.toLocaleLowerCase("zh-CN").includes(normalizedQuery));
  const guideChannel = channels.find((channel) => channel.id === guideChannelId) ?? null;

  const openCreateClient = () => {
    setDraft({ name: "", channel: "REST API", environment: "生产" });
    setCreatedCredential(null);
    setCreatingClient(true);
  };

  const createClient = (event: FormEvent) => {
    event.preventDefault();
    const secret = draft.channel === "MCP" ? "mcp_session_9Fx2R7kL3a" : "sk_session_7Kp2Nc4Q8m";
    setClients((current) => [{ id: `client-${current.length + 1}`, name: draft.name.trim(), channel: draft.channel, environment: draft.environment, credential: `${secret.slice(0, 8)}••••${secret.slice(-4)}`, lastUsed: "尚未使用", enabled: true }, ...current]);
    setCreatedCredential({ name: draft.name.trim(), secret });
    onNotify(`${draft.name.trim()} 已创建。凭据只在当前窗口显示一次。`);
  };

  const closeCreateClient = () => {
    setCreatingClient(false);
    setCreatedCredential(null);
  };

  return (
    <section className="view settings-view integration-settings-view" aria-label="接口与集成">
      <div className="integration-commandbar">
        <label><Search size={14} /><input type="search" aria-label="搜索集成客户端" placeholder="搜索客户端、接口或环境" value={query} onChange={(event) => setQuery(event.target.value)} />{query && <button type="button" aria-label="清除集成搜索" title="清除搜索" onClick={() => setQuery("")}><X size={13} /></button>}</label>
        <button className="primary-button" type="button" onClick={openCreateClient}><Plus size={15} />创建客户端</button>
      </div>

      <section className="integration-channel-section" aria-labelledby="integration-channel-title">
        <header><div><h2 id="integration-channel-title">接口能力</h2><span>统一使用发布版本与工作区凭据</span></div><span className="integration-health"><i />{channels.filter((channel) => channel.enabled).length} 项服务正常</span></header>
        <div className="integration-channel-grid">
          {channels.map((channel) => { const Icon = channelIcons[channel.id]; return <article key={channel.id}>
            <header><span className={`integration-channel-mark is-${channel.id}`}><Icon size={16} /></span><span><strong>{channel.name}</strong><small>{channel.capability}</small></span><span className={channel.enabled ? "integration-channel-state is-enabled" : "integration-channel-state"}>{channel.enabled ? "已启用" : "已停用"}</span></header>
            <p>{channel.description}</p>
            <footer><button className="secondary-button integration-guide-button" type="button" onClick={() => setGuideChannelId(channel.id)}><BookOpenText size={13} />使用说明</button><button className="icon-button" type="button" aria-label={`${channel.enabled ? "停用" : "启用"} ${channel.name}`} title={channel.enabled ? "停用" : "启用"} onClick={() => { setChannels((current) => current.map((item) => item.id === channel.id ? { ...item, enabled: !item.enabled } : item)); onNotify(`${channel.name} 已${channel.enabled ? "停用" : "启用"}。`); }}>{channel.enabled ? <Pause size={14} /> : <Play size={14} />}</button></footer>
          </article>; })}
        </div>
      </section>

      <section className="integration-client-section" aria-labelledby="integration-client-title">
        <header><div><h2 id="integration-client-title">客户端凭据</h2><span>用于 MCP、API 与自动化调用，不继承浏览器会话</span></div><strong>{visibleClients.length} 个客户端</strong></header>
        <div className="integration-client-table" aria-label="集成客户端列表">
          <div className="integration-client-head" aria-hidden="true"><span>客户端</span><span>接口</span><span>环境</span><span>凭据</span><span>最近调用</span><span>状态与操作</span></div>
          {visibleClients.map((client) => <div className="integration-client-row" key={client.id}><span><strong>{client.name}</strong><small>{client.id}</small></span><span>{client.channel}</span><span>{client.environment}</span><code>{client.credential}</code><span>{client.lastUsed}</span><span className="integration-client-actions"><span className={client.enabled ? "integration-channel-state is-enabled" : "integration-channel-state"}>{client.enabled ? "启用" : "停用"}</span><button className="icon-button" type="button" aria-label={`轮换 ${client.name} 的凭据`} title="轮换凭据" onClick={() => onNotify(`${client.name} 的凭据轮换已模拟完成。`)}><RotateCw size={14} /></button><button className="icon-button" type="button" aria-label={`${client.enabled ? "停用" : "启用"}客户端 ${client.name}`} title={client.enabled ? "停用客户端" : "启用客户端"} onClick={() => setClients((current) => current.map((item) => item.id === client.id ? { ...item, enabled: !item.enabled } : item))}>{client.enabled ? <Pause size={14} /> : <Play size={14} />}</button></span></div>)}
          {visibleClients.length === 0 && <div className="integration-client-empty"><Search size={17} /><strong>没有匹配的客户端</strong><span>调整搜索词或创建新的接入客户端。</span></div>}
        </div>
      </section>

      {guideChannel && createPortal(<div className="dialog-backdrop integration-guide-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setGuideChannelId(null); }}><section className="review-dialog integration-guide-dialog" role="dialog" aria-modal="true" aria-labelledby="integration-guide-dialog-title">
        <header><div><span className="content-label">{guideChannel.capability}</span><h2 id="integration-guide-dialog-title">{guideChannel.name} 使用说明</h2></div><button className="icon-button" type="button" aria-label="关闭使用说明" title="关闭" onClick={() => setGuideChannelId(null)}><X size={16} /></button></header>
        <div className="dialog-body integration-guide-body">
          <p>{guideChannel.description}</p>
          <dl><div><dt>接入地址</dt><dd><code>{guideChannel.endpoint}</code><button className="icon-button" type="button" aria-label={`复制 ${guideChannel.name} 接入地址`} title="复制接入地址" onClick={() => onNotify(`${guideChannel.name} 接入地址已复制。`)}><Clipboard size={13} /></button></dd></div><div><dt>认证方式</dt><dd>{channelGuides[guideChannel.id].authentication}</dd></div><div><dt>适用场景</dt><dd>{channelGuides[guideChannel.id].scenario}</dd></div></dl>
          <section aria-labelledby="integration-guide-steps-title"><h3 id="integration-guide-steps-title">快速开始</h3><ol>{channelGuides[guideChannel.id].steps.map((step) => <li key={step}>{step}</li>)}</ol></section>
          <section className="integration-guide-example" aria-labelledby="integration-guide-example-title"><header><h3 id="integration-guide-example-title">调用示例</h3><button className="secondary-button" type="button" onClick={() => onNotify(`${guideChannel.name} 调用示例已复制。`)}><Clipboard size={13} />复制示例</button></header><pre><code>{channelGuides[guideChannel.id].example}</code></pre></section>
          <aside><ShieldCheck size={14} /><span><strong>调用边界</strong><small>仅返回已发布知识；客户端、调用工具、资产版本与执行结果会进入审计日志。</small></span></aside>
        </div>
        <footer><span className="model-dialog-boundary"><KeyRound size={13} />凭据不会出现在调用日志中</span><div><button className="primary-button" type="button" onClick={() => setGuideChannelId(null)}>完成</button></div></footer>
      </section></div>, document.body)}

      {creatingClient && <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) closeCreateClient(); }}><section className="review-dialog compact-dialog integration-client-dialog" role="dialog" aria-modal="true" aria-labelledby="integration-client-dialog-title">
        <header><div><span className="content-label">接口与集成</span><h2 id="integration-client-dialog-title">{createdCredential ? "保存客户端凭据" : "创建客户端"}</h2></div><button className="icon-button" type="button" aria-label="关闭客户端配置" title="关闭" onClick={closeCreateClient}><X size={16} /></button></header>
        <div className="dialog-body">{createdCredential ? <div className="integration-secret-view"><span><Check size={17} /></span><h3>{createdCredential.name} 已创建</h3><p>该凭据只显示一次。关闭窗口后只能轮换，无法再次查看。</p><label><span>客户端凭据</span><code>{createdCredential.secret}</code><button className="secondary-button" type="button" onClick={() => onNotify("客户端凭据已复制。")}>复制凭据</button></label></div> : <form id="integration-client-form" className="integration-client-form" onSubmit={createClient}>
          <label><span>客户端名称</span><input autoFocus value={draft.name} onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))} placeholder="例如：经营分析 Agent" /></label>
          <label><span>调用方式</span><select value={draft.channel} onChange={(event) => setDraft((current) => ({ ...current, channel: event.target.value as IntegrationClient["channel"] }))}><option>REST API</option><option>MCP</option><option>CLI</option></select></label>
          <label><span>环境</span><select value={draft.environment} onChange={(event) => setDraft((current) => ({ ...current, environment: event.target.value as IntegrationClient["environment"] }))}><option>生产</option><option>测试</option><option>开发</option></select></label>
          <div className="integration-client-scope"><ShieldCheck size={15} /><span><strong>当前 MVP 使用工作区全量权限</strong><small>所有 MCP/API 调用仍会记录客户端、工具、资产版本与执行结果。</small></span></div>
        </form>}</div>
        <footer><span className="model-dialog-boundary"><KeyRound size={13} />凭据由工作区托管</span><div>{createdCredential ? <button className="primary-button" type="button" onClick={closeCreateClient}>完成</button> : <><button className="secondary-button" type="button" onClick={closeCreateClient}>取消</button><button className="primary-button" type="submit" form="integration-client-form" disabled={!draft.name.trim()}><Server size={14} />创建客户端</button></>}</div></footer>
      </section></div>}
    </section>
  );
}

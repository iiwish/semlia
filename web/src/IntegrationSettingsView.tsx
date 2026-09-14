import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Ban, Bot, Braces, Cable, Check, Clipboard, KeyRound, Pencil, Play, Plus, RefreshCw, Search, ShieldCheck, Terminal, X } from "lucide-react";
import { AUTHORIZATION_STALE_EVENT } from "./apiClient";
import { integrationApi, IntegrationPartialSuccess, type CredentialInput, type IntegrationApi, type IntegrationData, type Webhook } from "./integrations";

type Dialog = "credential" | "consumer" | "binding" | "principal" | "webhook" | "secret" | "principal-result" | null;
interface Props { workspaceId?: string; canRead?: boolean; canManage?: boolean; canCreatePrincipal?: boolean; canGrant?: boolean; runtimeRead?: boolean; runtimeManage?: boolean; api?: IntegrationApi; onNotify: (message: string) => void }
const initialData: IntegrationData = { credentials: [], consumers: [], bindings: [], webhooks: [], deliveries: [] };
const stateLabels: Record<string, string> = { queued: "等待投递", running: "投递中", succeeded: "已送达", cancelled: "已取消", dead_letter: "死信" };

export function IntegrationSettingsView({ workspaceId = "", canRead = false, canManage = false, canCreatePrincipal = false, canGrant = false, runtimeRead = false, runtimeManage = false, api = integrationApi, onNotify }: Props) {
  const [data, setData] = useState(initialData);
  const [now, setNow] = useState(() => Date.now());
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [stale, setStale] = useState(false);
  const [error, setError] = useState("");
  const [dialog, setDialog] = useState<Dialog>(null);
  const [secret, setSecret] = useState("");
  const [secretKind, setSecretKind] = useState<"credential" | "webhook">("credential");
  const [editingWebhook, setEditingWebhook] = useState<Webhook | null>(null);
  const [eventTypes, setEventTypes] = useState<Webhook["eventTypes"]>(["release.published"]);
  const [principal, setPrincipal] = useState("");
  const [granted, setGranted] = useState(false);
  const [query, setQuery] = useState("");
  const [tab, setTab] = useState<"credentials" | "webhooks">("credentials");
  const [draft, setDraft] = useState({ name: "", principalId: "", consumerId: "", bindingId: "", stableKey: "", environment: "prod", endpoint: "", expiresAt: "", scopeType: "workspace" as CredentialInput["scopeType"], scopeId: "", actions: ["asset.read", "semantic.resolve"] as CredentialInput["allowedActions"] });
  const modal = useRef<HTMLElement>(null);
  const trigger = useRef<HTMLElement | null>(null);
  const pendingFocus = useRef<HTMLElement | null>(null);
  const mutation = useRef(false);
  const mounted = useRef(true);
  const sequence = useRef(0);
  const dialogEpoch = useRef(0);
  const base = `${window.location.origin}/api/v1/workspaces/${workspaceId}`;
  const disabled = !canManage || !loaded || stale || busy || loading;
  const load = useCallback(async (signal?: AbortSignal) => {
    if (!workspaceId || !canRead) return false;
    const generation = ++sequence.current;
    setLoading(true);
    try {
      const result = await api.load(workspaceId, runtimeRead, signal);
      if (!mounted.current || signal?.aborted || generation !== sequence.current) return false;
      setData(result); setNow(Date.now()); setLoaded(true); setStale(false); setError(""); return true;
    } catch {
      if (mounted.current && !signal?.aborted && generation === sequence.current) { setStale(true); setError("无法刷新集成设置，当前数据不可用于变更。"); }
      return false;
    } finally { if (mounted.current && generation === sequence.current) setLoading(false); }
  }, [api, canRead, runtimeRead, workspaceId]);
  useEffect(() => { const epoch = sequence; mounted.current = true; const controller = new AbortController(); queueMicrotask(() => { if (!controller.signal.aborted) void load(controller.signal); }); return () => { mounted.current = false; controller.abort(); epoch.current++; }; }, [load]);
  useEffect(() => { const invalidate = () => { sequence.current++; pendingFocus.current = null; setStale(true); setSecret(""); setDialog(null); setError("权限已变更，请刷新工作区后重试。"); }; window.addEventListener(AUTHORIZATION_STALE_EVENT, invalidate); return () => window.removeEventListener(AUTHORIZATION_STALE_EVENT, invalidate); }, []);
  const close = useCallback(() => { dialogEpoch.current++; pendingFocus.current = trigger.current; setDialog(null); setSecret(""); }, []);
  useEffect(() => {
    if (dialog || busy || loading) return;
    const target = pendingFocus.current;
    pendingFocus.current = null;
    if (target?.isConnected && !target.matches(":disabled") && (document.activeElement === document.body || document.activeElement === target)) target.focus();
  }, [busy, dialog, loading]);
  useEffect(() => {
    if (!dialog) return;
    modal.current?.querySelector<HTMLElement>('input:not(:disabled),select:not(:disabled),button:not(:disabled)')?.focus();
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.preventDefault(); close(); return; }
      if (event.key !== "Tab") return;
      const controls = [...(modal.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),select:not(:disabled),textarea:not(:disabled)') ?? [])];
      const first = controls[0], last = controls.at(-1);
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    document.addEventListener("keydown", key);
    return () => { document.removeEventListener("keydown", key); };
  }, [close, dialog]);
  const open = (kind: Dialog) => { dialogEpoch.current++; pendingFocus.current = null; trigger.current = document.activeElement as HTMLElement; setSecret(""); setSecretKind(kind === "webhook" ? "webhook" : "credential"); setEditingWebhook(null); setEventTypes(["release.published"]); setError(""); setDraft(current => ({ ...current, name: "", expiresAt: new Date(Date.now() + 90 * 86400000).toISOString().slice(0, 10), scopeId: workspaceId })); setDialog(kind); };
  const perform = async <T,>(work: () => Promise<T>, success: (value: T) => void) => {
    if (mutation.current || stale) return;
    mutation.current = true; setBusy(true); setError("");
    const generation = sequence.current, shownDialog = dialogEpoch.current;
    try {
      const value = await work();
      if (!mounted.current || generation !== sequence.current) return;
      if (shownDialog === dialogEpoch.current) success(value);
      onNotify("变更已保存。");
      if (!await load() && mounted.current) setError("变更已保存，但刷新失败。请刷新确认最新状态，不要重复提交。");
    } catch (cause) { if (mounted.current) { if (cause instanceof IntegrationPartialSuccess) { close(); await load(); } setError(cause instanceof Error ? cause.message : "变更失败，请重试。"); } }
    finally { mutation.current = false; if (mounted.current) setBusy(false); }
  };
  const copy = async (value: string) => { try { await navigator.clipboard.writeText(value); onNotify("已复制。"); } catch { setError("复制失败。请检查剪贴板权限，凭据仍仅在当前窗口显示。"); } };
  const submit = (event: FormEvent) => {
    event.preventDefault(); if (disabled) return;
    if (dialog === "credential") void perform(() => api.issue(workspaceId, { name: draft.name, principalId: draft.principalId, consumerId: draft.consumerId, bindingId: draft.bindingId, expiresAt: new Date(`${draft.expiresAt}T23:59:59Z`).toISOString(), allowedActions: draft.actions, scopeType: draft.scopeType, scopeId: draft.scopeType === "workspace" ? workspaceId : draft.scopeId }), result => { setSecret(result.token); setDialog("secret"); });
    if (dialog === "consumer") void perform(() => api.createConsumer(workspaceId, draft.name, draft.stableKey, draft.environment), close);
    if (dialog === "binding") void perform(() => api.createBinding(workspaceId, draft.consumerId, draft.environment), close);
    if (dialog === "principal" && canCreatePrincipal) void perform(() => api.createPrincipal(workspaceId, draft.name), result => { setPrincipal(result.id); setGranted(false); setDraft(current => ({ ...current, principalId: result.id })); setDialog("principal-result"); });
    if (dialog === "webhook") void perform(() => api.saveWebhook(workspaceId, { name: draft.name, endpoint: draft.endpoint, enabled: editingWebhook?.enabled ?? true, eventTypes, ...(editingWebhook ? { expectedVersion: editingWebhook.version } : {}) }, editingWebhook?.id), result => { if (result.signingSecret) { setSecretKind("webhook"); setSecret(result.signingSecret); setDialog("secret"); } else close(); });
  };
  const toggleWebhook = (item: Webhook) => void perform(() => api.saveWebhook(workspaceId, { name: item.name, endpoint: item.endpoint, enabled: !item.enabled, eventTypes: item.eventTypes, expectedVersion: item.version }, item.id), () => {});
  const titles: Record<Exclude<Dialog, null>, string> = { credential: "创建客户端", consumer: "登记消费方", binding: "补建消费绑定", principal: "创建机器身份", webhook: editingWebhook ? "编辑 Webhook" : "新增 Webhook", secret: "保存一次性凭据", "principal-result": "机器身份已创建" };
  const visible = data.credentials.filter(item => `${item.name} ${item.principalId} ${item.tokenPrefix}`.toLowerCase().includes(query.toLowerCase()));
  return <section className="view settings-view integration-settings-view" aria-label="接口与集成">
    <div className="integration-commandbar"><label><Search size={14}/><input aria-label="搜索集成客户端" type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索客户端或机器身份"/></label><div className="integration-command-actions">
      <button className="icon-button" title="刷新集成设置" aria-label="刷新集成设置" disabled={loading || busy || !canRead} onClick={() => void load()}><RefreshCw size={15}/></button>
      <button className="secondary-button" disabled={disabled || !canCreatePrincipal} onClick={() => open("principal")}><Bot size={14}/>创建机器身份</button>
      <button className="secondary-button" disabled={disabled} onClick={() => open("consumer")}><Plus size={14}/>登记消费方</button>
      <button className="primary-button" disabled={disabled} onClick={() => open("credential")}><KeyRound size={14}/>创建客户端</button>
    </div></div>
    {!canRead && <p role="alert">没有读取集成设置的权限。</p>}
    {loading && <p role="status">正在读取集成设置</p>}
    {error && <p className="integration-error" role="alert">{error}</p>}
    <section className="integration-channel-section" aria-label="接口能力"><div className="integration-channel-grid">
      {[{ name: "REST API", icon: Cable, endpoint: `${base}/semantic-queries:resolve` }, { name: "MCP Server", icon: Bot, endpoint: `${base}/mcp` }, { name: "CLI", icon: Terminal, endpoint: "semlia semantic resolve <query-json>" }, { name: "TypeScript SDK", icon: Braces, endpoint: "@semlia/sdk-typescript · createSemanticClient" }].map(channel => <article key={channel.name}><header><channel.icon size={17}/><strong>{channel.name}</strong></header><code className="integration-endpoint">{channel.endpoint}</code><footer><button className="icon-button" title={`复制 ${channel.name} 接入信息`} aria-label={`复制 ${channel.name} 接入信息`} onClick={() => void copy(channel.endpoint)}><Clipboard size={14}/></button></footer></article>)}
    </div></section>
    <div className="integration-tabs" role="tablist" aria-label="集成资源"><button role="tab" aria-selected={tab === "credentials"} onClick={() => setTab("credentials")}>客户端凭据 {data.credentials.length}</button><button role="tab" aria-selected={tab === "webhooks"} onClick={() => setTab("webhooks")}>Webhook {data.webhooks.length}</button></div>
    {tab === "credentials" && <section className="integration-live-list" aria-label="客户端凭据">
      {data.consumers.filter(item => item.status === "active" && !data.bindings.some(binding => binding.consumerId === item.id && binding.status === "active")).map(item => <article className="integration-live-object" key={item.id}><header><strong>{item.name}</strong><span>缺少消费绑定</span></header><code>{item.id}</code><footer><button className="secondary-button" disabled={disabled} onClick={() => { open("binding"); setDraft(current => ({ ...current, consumerId: item.id, name: item.name })); }}><Plus size={14}/>补建绑定</button></footer></article>)}
      {loaded && visible.length === 0 && <p>尚无客户端凭据</p>}
      {visible.map(item => { const active = !item.revokedAt && new Date(item.expiresAt).getTime() > now; return <article key={item.id} className="integration-live-object"><header><div><strong>{item.name}</strong><code>{item.tokenPrefix}</code></div><span>{item.revokedAt ? "已撤销" : active ? "有效" : "已过期"}</span></header><dl><div><dt>机器身份</dt><dd><code>{item.principalId}</code></dd></div><div><dt>到期时间</dt><dd>{new Date(item.expiresAt).toLocaleString("zh-CN")}</dd></div><div><dt>权限上限</dt><dd>{item.allowedActions.join(" · ")}</dd></div><div><dt>资源范围</dt><dd><code>{item.scopeId}</code></dd></div><div><dt>最近调用</dt><dd>{item.lastUsedAt ? new Date(item.lastUsedAt).toLocaleString("zh-CN") : "尚未使用"}</dd></div></dl><footer><button className="icon-button" disabled={disabled || !active} title="轮换凭据，旧凭据立即失效" aria-label={`轮换 ${item.name} 的凭据`} onClick={() => { trigger.current = document.activeElement as HTMLElement; void perform(() => api.rotate(workspaceId, item.id), result => { setSecretKind("credential"); setSecret(result.token); setDialog("secret"); }); }}><RefreshCw size={15}/></button><button className="icon-button" disabled={disabled || !active} title="永久撤销凭据" aria-label={`撤销客户端 ${item.name}`} onClick={() => void perform(() => api.revoke(workspaceId, item.id), () => {})}><Ban size={15}/></button></footer></article>; })}
    </section>}
    {tab === "webhooks" && <section className="integration-live-list" aria-label="Webhook 订阅"><header className="integration-section-title"><h2>事件订阅</h2><button className="secondary-button" disabled={disabled || !!data.webhookError} onClick={() => open("webhook")}><Plus size={14}/>新增 Webhook</button></header>
      {data.webhookError && <p role="alert">Webhook 不可用：{data.webhookError}</p>}
      {loaded && !data.webhookError && data.webhooks.length === 0 && <p>尚无 Webhook 订阅</p>}
      {data.webhooks.map(item => <article className="integration-live-object" key={item.id}><header><strong>{item.name}</strong><label><input type="checkbox" checked={item.enabled} disabled={disabled} onChange={() => toggleWebhook(item)}/>启用</label></header><code className="integration-endpoint">{item.endpoint}</code><p>{item.eventTypes.join(" · ")} · 版本 {item.version} · 签名 v{item.signingVersion} / {item.secretSuffix}</p><footer><button className="icon-button" title="编辑订阅，取消旧版本待投递事件" aria-label={`编辑 ${item.name} 的订阅`} disabled={disabled} onClick={() => { open("webhook"); setEditingWebhook(item); setEventTypes(item.eventTypes); setDraft(current => ({ ...current, name: item.name, endpoint: item.endpoint })); }}><Pencil size={15}/></button><button className="icon-button" title="轮换签名，取消旧版本待投递事件" aria-label={`轮换 ${item.name} 的签名`} disabled={disabled} onClick={() => { trigger.current = document.activeElement as HTMLElement; void perform(() => api.rotateWebhook(workspaceId, item), result => { setSecretKind("webhook"); setSecret(result.signingSecret); setDialog("secret"); }); }}><RefreshCw size={15}/></button></footer></article>)}
      <header className="integration-section-title"><h2>投递记录</h2></header>{!runtimeRead ? <p>没有读取运行记录的权限。</p> : data.deliveries.length === 0 ? <p>尚无投递记录</p> : data.deliveries.map(item => <article className="integration-live-object" key={item.id}><header><strong>{item.eventType}</strong><span>{stateLabels[item.state]}</span></header><code>{item.eventId}</code><p>尝试 {item.attempt} / {item.maxAttempts} · HTTP {item.httpStatus || "-"}{item.errorCode ? ` · ${item.errorCode}` : ""}</p><footer><a href={`/operations/runtime?run=${encodeURIComponent(item.runtimeRunId)}`}>查看运行记录</a><button className="icon-button" aria-label={`重放投递 ${item.eventId}`} title="重放死信，增加八次尝试，仅限一次" disabled={!runtimeManage || !loaded || stale || loading || busy || item.state !== "dead_letter" || item.maxAttempts !== 8} onClick={() => void perform(() => api.replayDelivery(workspaceId, item), () => {})}><Play size={15}/></button></footer></article>)}
    </section>}
    {dialog && <div className="dialog-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) close(); }}><section ref={modal} className="review-dialog compact-dialog integration-live-dialog" role="dialog" aria-modal="true" aria-labelledby="integration-dialog-title"><header><h2 id="integration-dialog-title">{titles[dialog]}</h2><button className="icon-button" title="关闭" aria-label="关闭集成窗口" onClick={close}><X size={16}/></button></header><div className="dialog-body">
      {error && <p role="alert">{error}</p>}
      {dialog === "secret" ? <div className="integration-secret-view"><Check size={20}/><p>{secretKind === "webhook" ? "签名密钥只显示一次。旧版本待投递事件会取消；已在途的请求可能以旧签名完成。" : "凭据只显示一次。关闭后无法恢复；轮换会使旧凭据立即失效。"}</p><code>{secret}</code><button className="secondary-button" onClick={() => void copy(secret)}><Clipboard size={14}/>复制凭据</button></div> : dialog === "principal-result" ? <div className="integration-secret-view"><code>{principal}</code><p>{granted ? "消费开发者角色已授予。" : "当前身份尚无角色授权。"}</p><button className="secondary-button" onClick={() => void copy(principal)}><Clipboard size={14}/>复制身份 ID</button><button className="secondary-button" disabled={!canGrant || busy || stale || granted} onClick={() => void perform(() => api.grant(workspaceId, principal), () => setGranted(true))}><ShieldCheck size={14}/>授予消费开发者角色</button></div> : <form id="integration-live-form" onSubmit={submit} className="integration-client-form">
        {dialog !== "binding" && <label><span>名称</span><input required maxLength={160} value={draft.name} onChange={event => setDraft({ ...draft, name: event.target.value })}/></label>}
        {dialog === "credential" && <><label><span>机器身份 ID</span><input required value={draft.principalId} onChange={event => setDraft({ ...draft, principalId: event.target.value })}/></label><label><span>消费方</span><select required value={draft.consumerId} onChange={event => setDraft({ ...draft, consumerId: event.target.value, bindingId: "" })}><option value="">选择消费方</option>{data.consumers.filter(item => item.status === "active").map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label><label><span>消费绑定</span><select required value={draft.bindingId} onChange={event => setDraft({ ...draft, bindingId: event.target.value })}><option value="">选择绑定</option>{data.bindings.filter(item => item.consumerId === draft.consumerId && item.status === "active").map(item => <option key={item.id} value={item.id}>{item.environment} · {item.mode}</option>)}</select></label><label><span>到期时间</span><input required type="date" value={draft.expiresAt} min={new Date().toISOString().slice(0, 10)} onChange={event => setDraft({ ...draft, expiresAt: event.target.value })}/></label><label><span>资源范围</span><select value={draft.scopeType} onChange={event => setDraft({ ...draft, scopeType: event.target.value as CredentialInput["scopeType"], scopeId: "" })}><option value="workspace">工作区</option><option value="asset">单个资产</option><option value="release">单个发布版本</option></select></label>{draft.scopeType !== "workspace" && <label><span>资源 ID</span><input required value={draft.scopeId} onChange={event => setDraft({ ...draft, scopeId: event.target.value })}/></label>}<fieldset><legend>权限上限</legend>{(["asset.read", "semantic.resolve", "semantic.execute"] as const).map(action => <label key={action}><input type="checkbox" checked={draft.actions.includes(action)} onChange={event => setDraft({ ...draft, actions: event.target.checked ? [...draft.actions, action] : draft.actions.filter(item => item !== action) })}/>{action}</label>)}</fieldset></>}
        {dialog === "consumer" && <><label><span>稳定键</span><input required pattern="[a-z0-9][a-z0-9_.-]*" maxLength={120} value={draft.stableKey} onChange={event => setDraft({ ...draft, stableKey: event.target.value })}/></label><label><span>环境</span><input required maxLength={80} value={draft.environment} onChange={event => setDraft({ ...draft, environment: event.target.value })}/></label></>}
        {dialog === "binding" && <><code>{draft.consumerId}</code><label><span>环境</span><input required maxLength={80} value={draft.environment} onChange={event => setDraft({ ...draft, environment: event.target.value })}/></label></>}
        {dialog === "webhook" && <><label><span>HTTPS 接收地址</span><input required type="url" maxLength={2048} value={draft.endpoint} onChange={event => setDraft({ ...draft, endpoint: event.target.value })}/></label><fieldset><legend>事件筛选</legend>{(["release.published", "catalog.asset.changed"] as const).map(kind => <label key={kind}><input type="checkbox" checked={eventTypes.includes(kind)} onChange={event => setEventTypes(current => event.target.checked ? [...current, kind] : current.filter(value => value !== kind))}/>{kind}</label>)}</fieldset></>}
      </form>}
    </div><footer><button className="secondary-button" onClick={close}>{dialog === "secret" || dialog === "principal-result" ? "完成" : "取消"}</button>{dialog !== "secret" && dialog !== "principal-result" && <button className="primary-button" type="submit" form="integration-live-form" disabled={disabled || !draft.name.trim() || (dialog === "credential" && draft.actions.length === 0) || (dialog === "webhook" && eventTypes.length === 0)}>{busy ? "提交中" : editingWebhook ? "保存" : "创建"}</button>}</footer></section></div>}
  </section>;
}

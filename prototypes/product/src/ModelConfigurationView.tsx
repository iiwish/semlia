import { useMemo, useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
import { ArrowUpRight, Bot, Check, CheckCircle2, ChevronDown, CircleAlert, FileClock, KeyRound, LoaderCircle, Network, Plus, RefreshCw, Settings2, ShieldCheck, Sparkles, X } from "lucide-react";

type ModelKind = "llm" | "embedding";

export interface EmbeddingRebuildRun {
  id: string;
  modelId: string;
  status: "运行中" | "已完成" | "失败";
  progress: number;
  phase: "排队" | "扫描知识块" | "生成向量" | "写入新索引" | "质量校验" | "切换生效";
  startedAt: string;
  duration: string;
  processedItems: number;
  totalItems: number;
}

interface ModelEntry {
  id: string;
  modelId: string;
  enabled: boolean;
  isDefault: boolean;
  capability: string;
  limit: string;
  dimension?: number;
}

interface ModelProvider {
  id: string;
  name: string;
  providerType: string;
  credentialRevision: number;
  enabled: boolean;
  models: ModelEntry[];
}

const initialProviders: Record<ModelKind, ModelProvider[]> = {
  llm: [
    {
      id: "llm-openai",
      name: "OpenAI Enterprise",
      providerType: "OpenAI",
      credentialRevision: 3,
      enabled: true,
      models: [
        { id: "llm-gpt-41", modelId: "gpt-4.1", enabled: true, isDefault: true, capability: "视觉 · 推理", limit: "128,000 tokens" },
        { id: "llm-o3", modelId: "o3", enabled: true, isDefault: false, capability: "推理", limit: "200,000 tokens" },
      ],
    },
    {
      id: "llm-anthropic",
      name: "Anthropic",
      providerType: "Anthropic",
      credentialRevision: 2,
      enabled: true,
      models: [
        { id: "llm-claude", modelId: "claude-sonnet-4-20250514", enabled: true, isDefault: false, capability: "视觉 · 推理", limit: "200,000 tokens" },
      ],
    },
  ],
  embedding: [
    {
      id: "embedding-openai",
      name: "OpenAI Enterprise",
      providerType: "OpenAI",
      credentialRevision: 3,
      enabled: true,
      models: [
        { id: "embedding-3-large", modelId: "text-embedding-3-large", enabled: true, isDefault: true, capability: "多语言检索", limit: "8,191 tokens", dimension: 3072 },
      ],
    },
    {
      id: "embedding-internal",
      name: "内部向量服务",
      providerType: "OpenAI 兼容",
      credentialRevision: 5,
      enabled: true,
      models: [
        { id: "embedding-bge-m3", modelId: "bge-m3", enabled: true, isDefault: false, capability: "多语言 · 稠密检索", limit: "8,192 tokens", dimension: 1024 },
      ],
    },
  ],
};

const providerOptions = ["OpenAI", "Anthropic", "Google Gemini", "OpenAI 兼容"];
const embeddingRebuildStages: EmbeddingRebuildRun["phase"][] = ["排队", "扫描知识块", "生成向量", "写入新索引", "质量校验", "切换生效"];

function embeddingRebuildLogs(run: EmbeddingRebuildRun) {
  return [
    { time: "11:32:00", level: "INFO", stage: "任务创建", message: "重建任务已创建，现有索引继续提供检索服务。" },
    { time: "11:32:03", level: "INFO", stage: "模型锁定", message: `目标模型已锁定为 ${run.modelId}。` },
    { time: "11:32:14", level: "DONE", stage: "扫描知识块", message: `扫描完成，共发现 ${run.totalItems.toLocaleString("zh-CN")} 个可索引知识块。` },
    { time: "11:32:16", level: "INFO", stage: "生成向量", message: "开始按批次生成向量并写入临时索引。" },
    { time: "11:33:18", level: "RUNNING", stage: run.phase, message: `已处理 ${run.processedItems.toLocaleString("zh-CN")} / ${run.totalItems.toLocaleString("zh-CN")} 个知识块。` },
  ];
}

function providerTone(providerType: string) {
  if (providerType === "Anthropic") return "anthropic";
  if (providerType === "Google Gemini") return "google";
  if (providerType === "OpenAI 兼容") return "compatible";
  return "openai";
}

export function ModelConfigurationView({ rebuildRun, onStartEmbeddingRebuild, onOpenRebuildRun, onNotify }: { rebuildRun: EmbeddingRebuildRun | null; onStartEmbeddingRebuild: (modelId: string) => void; onOpenRebuildRun: () => void; onNotify: (message: string) => void }) {
  const [kind, setKind] = useState<ModelKind>("llm");
  const [providers, setProviders] = useState(initialProviders);
  const [collapsedProviderIds, setCollapsedProviderIds] = useState<Set<string>>(() => new Set());
  const [providerDialogKind, setProviderDialogKind] = useState<ModelKind | null>(null);
  const [providerDraft, setProviderDraft] = useState({ name: "", providerType: "OpenAI", baseUrl: "", apiKey: "" });
  const [modelDialog, setModelDialog] = useState<{ providerId: string; model: ModelEntry | null } | null>(null);
  const [rebuildDialogOpen, setRebuildDialogOpen] = useState(false);
  const [rebuildDetailOpen, setRebuildDetailOpen] = useState(false);
  const [modelDraft, setModelDraft] = useState({ modelId: "", capability: "文本 · 推理", limit: "128000", dimension: "1024", enabled: true });

  const activeProviders = providers[kind];
  const enabledModels = useMemo(() => activeProviders.flatMap((provider) => provider.enabled ? provider.models.filter((model) => model.enabled) : []), [activeProviders]);
  const defaultModel = enabledModels.find((model) => model.isDefault) ?? null;
  const kindLabel = kind === "llm" ? "LLM" : "Embedding";

  const updateActiveProviders = (updater: (current: ModelProvider[]) => ModelProvider[]) => {
    setProviders((current) => ({ ...current, [kind]: updater(current[kind]) }));
  };

  const setDefaultModel = (modelId: string) => {
    updateActiveProviders((current) => current.map((provider) => ({
      ...provider,
      models: provider.models.map((model) => ({ ...model, isDefault: model.id === modelId })),
    })));
    const selected = activeProviders.flatMap((provider) => provider.models).find((model) => model.id === modelId);
    if (selected) onNotify(`${kindLabel} 默认模型已切换为 ${selected.modelId}。`);
  };

  const toggleProvider = (providerId: string) => {
    updateActiveProviders((current) => current.map((provider) => provider.id === providerId ? { ...provider, enabled: !provider.enabled } : provider));
  };

  const toggleModel = (providerId: string, modelId: string) => {
    updateActiveProviders((current) => current.map((provider) => provider.id === providerId ? {
      ...provider,
      models: provider.models.map((model) => model.id === modelId ? { ...model, enabled: !model.enabled } : model),
    } : provider));
  };

  const openProviderDialog = () => {
    setProviderDraft({ name: "", providerType: "OpenAI", baseUrl: "", apiKey: "" });
    setProviderDialogKind(kind);
  };

  const saveProvider = (event: FormEvent) => {
    event.preventDefault();
    if (!providerDialogKind) return;
    const nextProvider: ModelProvider = {
      id: `${providerDialogKind}-provider-${providers[providerDialogKind].length + 1}`,
      name: providerDraft.name.trim(),
      providerType: providerDraft.providerType,
      credentialRevision: 1,
      enabled: true,
      models: [],
    };
    setProviders((current) => ({ ...current, [providerDialogKind]: [...current[providerDialogKind], nextProvider] }));
    setProviderDialogKind(null);
    onNotify(`${providerDraft.name.trim()} 已加入 ${providerDialogKind === "llm" ? "LLM" : "Embedding"} 模型配置。`);
  };

  const openModelDialog = (providerId: string, model: ModelEntry | null = null) => {
    setModelDraft({
      modelId: model?.modelId ?? "",
      capability: model?.capability ?? (kind === "llm" ? "文本 · 推理" : "多语言检索"),
      limit: model?.limit.replace(/[^0-9]/g, "") ?? (kind === "llm" ? "128000" : "8192"),
      dimension: String(model?.dimension ?? 1024),
      enabled: model?.enabled ?? true,
    });
    setModelDialog({ providerId, model });
  };

  const saveModel = (event: FormEvent) => {
    event.preventDefault();
    if (!modelDialog) return;
    updateActiveProviders((current) => current.map((provider) => {
      if (provider.id !== modelDialog.providerId) return provider;
      const nextModel: ModelEntry = {
        id: modelDialog.model?.id ?? `${provider.id}-model-${provider.models.length + 1}`,
        modelId: modelDraft.modelId.trim(),
        enabled: modelDraft.enabled,
        isDefault: modelDialog.model?.isDefault ?? enabledModels.length === 0,
        capability: modelDraft.capability.trim(),
        limit: `${Number(modelDraft.limit || 0).toLocaleString("en-US")} tokens`,
        dimension: kind === "embedding" ? Number(modelDraft.dimension) : undefined,
      };
      return {
        ...provider,
        models: modelDialog.model ? provider.models.map((model) => model.id === modelDialog.model?.id ? nextModel : model) : [...provider.models, nextModel],
      };
    }));
    onNotify(`${modelDraft.modelId.trim()} 已保存到 ${kindLabel} 模型配置。`);
    setModelDialog(null);
  };

  const startEmbeddingRebuild = () => {
    setRebuildDialogOpen(false);
    if (defaultModel) onStartEmbeddingRebuild(defaultModel.modelId);
  };

  return (
    <section className="view settings-view model-config-view" aria-label="模型配置">
      <header className="model-config-header">
        <div className="model-kind-tabs" role="tablist" aria-label="模型类型">
          <button type="button" role="tab" aria-selected={kind === "llm"} onClick={() => setKind("llm")}><Bot size={14} />LLM 模型</button>
          <button type="button" role="tab" aria-selected={kind === "embedding"} onClick={() => setKind("embedding")}><Network size={14} />Embedding 模型</button>
        </div>
      </header>

      <div className="model-config-toolbar" aria-label={`${kindLabel} 模型设置`}>
        <label className="model-default-select"><span>{kind === "llm" ? "默认问答模型" : "默认索引模型"}</span><select aria-label={kind === "llm" ? "默认 LLM 模型" : "默认 Embedding 模型"} value={defaultModel?.id ?? ""} onChange={(event) => setDefaultModel(event.target.value)}>{enabledModels.length === 0 && <option value="">尚未配置</option>}{activeProviders.flatMap((provider) => provider.enabled ? provider.models.filter((model) => model.enabled).map((model) => <option value={model.id} key={model.id}>{provider.name} · {model.modelId}</option>) : [])}</select></label>
        <div className="model-toolbar-actions">
          <span className="model-vault-state"><ShieldCheck size={14} />密钥已托管</span>
          {kind === "embedding" && <button className="secondary-button model-rebuild-index" type="button" onClick={() => setRebuildDialogOpen(true)} disabled={!defaultModel || rebuildRun?.status === "运行中"}>{rebuildRun?.status === "运行中" ? <LoaderCircle className="spin" size={14} /> : <RefreshCw size={14} />}{rebuildRun?.status === "运行中" ? "正在重建" : "重建向量索引"}</button>}
          <button className="primary-button model-add-provider" type="button" onClick={openProviderDialog}><Plus size={15} />添加供应商</button>
        </div>
      </div>

      {kind === "embedding" && rebuildRun && <section className={`embedding-rebuild-status is-${rebuildRun.status === "运行中" ? "running" : rebuildRun.status === "已完成" ? "completed" : "failed"}`} aria-label="最近向量索引重建状态">
        <span className="embedding-rebuild-status-icon">{rebuildRun.status === "运行中" ? <LoaderCircle className="spin" size={15} /> : rebuildRun.status === "已完成" ? <CheckCircle2 size={15} /> : <CircleAlert size={15} />}</span>
        <span className="embedding-rebuild-status-copy"><strong>{rebuildRun.status === "运行中" ? `${rebuildRun.phase} · ${rebuildRun.progress}%` : `重建${rebuildRun.status}`}</strong><small>{rebuildRun.modelId} · {rebuildRun.processedItems.toLocaleString("zh-CN")} / {rebuildRun.totalItems.toLocaleString("zh-CN")} 个知识块</small></span>
        <span className="embedding-rebuild-progress" aria-label={`重建进度 ${rebuildRun.progress}%`}><i style={{ width: `${rebuildRun.progress}%` }} /></span>
        <button className="secondary-button" type="button" onClick={() => setRebuildDetailOpen(true)}>查看进度与日志</button>
      </section>}

      <div className="model-provider-list" aria-label={`${kindLabel} 供应商和模型`}>
        {activeProviders.map((provider) => {
          const collapsed = collapsedProviderIds.has(provider.id);
          return <section className={collapsed ? "model-provider-group is-collapsed" : "model-provider-group"} key={provider.id} aria-label={`${provider.name} 供应商`}>
            <header className="model-provider-row">
              <button className="model-icon-button model-provider-collapse" type="button" aria-label={`${collapsed ? "展开" : "折叠"} ${provider.name} 的模型`} aria-expanded={!collapsed} title={`${collapsed ? "展开" : "折叠"}模型`} onClick={() => setCollapsedProviderIds((current) => { const next = new Set(current); if (next.has(provider.id)) next.delete(provider.id); else next.add(provider.id); return next; })}><ChevronDown size={15} /></button>
              <span className={`model-provider-mark is-${providerTone(provider.providerType)}`}><Bot size={16} /></span>
              <span className="model-provider-identity"><strong>{provider.name}</strong><small>{provider.providerType} · Key v{provider.credentialRevision}</small></span>
              <span className={provider.enabled ? "model-status is-enabled" : "model-status is-disabled"}>{provider.enabled ? "已启用" : "已停用"}</span>
              <span className="model-provider-count">{provider.models.length} 个模型</span>
              <label className="model-switch" title={`${provider.enabled ? "停用" : "启用"}${provider.name}`}><input type="checkbox" aria-label={`${provider.enabled ? "停用" : "启用"}供应商 ${provider.name}`} checked={provider.enabled} onChange={() => toggleProvider(provider.id)} /><span /></label>
              <button className="model-icon-button" type="button" aria-label={`更新 ${provider.name} 的凭据`} title="更新凭据" onClick={() => onNotify(`${provider.name} 的凭据轮换已在本次原型会话中模拟完成。`)}><KeyRound size={14} /></button>
              <button className="model-icon-button" type="button" aria-label={`为 ${provider.name} 添加模型`} title="添加模型" onClick={() => openModelDialog(provider.id)}><Plus size={15} /></button>
            </header>
            <div className="model-rows" hidden={collapsed}>
              {provider.models.map((model) => <div className="model-row" key={model.id}>
                <button className={model.isDefault ? "model-default-button is-active" : "model-default-button"} type="button" aria-label={model.isDefault ? `${model.modelId} 当前为默认模型` : `设 ${model.modelId} 为默认模型`} title={model.isDefault ? "当前默认模型" : "设为默认模型"} disabled={!provider.enabled || !model.enabled} onClick={() => setDefaultModel(model.id)}>{model.isDefault ? <Check size={13} /> : <Sparkles size={13} />}</button>
                <span className="model-row-identity"><strong>{model.modelId}</strong><small>{kind === "embedding" ? `${model.dimension?.toLocaleString("en-US")} dimensions` : model.capability}</small></span>
                <span>{kind === "embedding" ? model.capability : model.limit}</span>
                <span>{kind === "embedding" ? model.limit : model.capability}</span>
                <span className={model.enabled ? "model-status is-enabled" : "model-status is-disabled"}>{model.enabled ? "可用" : "已停用"}</span>
                <label className="model-switch" title={`${model.enabled ? "停用" : "启用"}${model.modelId}`}><input type="checkbox" aria-label={`${model.enabled ? "停用" : "启用"}模型 ${model.modelId}`} checked={model.enabled} onChange={() => toggleModel(provider.id, model.id)} /><span /></label>
                <button className="model-icon-button" type="button" aria-label={`编辑模型 ${model.modelId}`} title="编辑模型" onClick={() => openModelDialog(provider.id, model)}><Settings2 size={14} /></button>
              </div>)}
              {provider.models.length === 0 && <div className="model-row-empty"><span>尚未配置模型</span><button className="secondary-button" type="button" onClick={() => openModelDialog(provider.id)}><Plus size={13} />添加模型</button></div>}
            </div>
          </section>;
        })}
      </div>

      <footer className="model-config-footnote"><ShieldCheck size={14} /><span>API Key 只写且不会回显。此页面为前端原型，所有配置在刷新后重置。</span></footer>

      {providerDialogKind && <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setProviderDialogKind(null); }}><section className="review-dialog compact-dialog model-config-dialog" role="dialog" aria-modal="true" aria-labelledby="provider-dialog-title">
        <header><div><span className="content-label">{providerDialogKind === "llm" ? "LLM" : "Embedding"}</span><h2 id="provider-dialog-title">添加模型供应商</h2></div><button className="icon-button" type="button" aria-label="关闭供应商配置" title="关闭" onClick={() => setProviderDialogKind(null)}><X size={16} /></button></header>
        <div className="dialog-body"><form id="model-provider-form" className="model-config-form" onSubmit={saveProvider}>
          <label><span>配置名称</span><input autoFocus value={providerDraft.name} onChange={(event) => setProviderDraft((current) => ({ ...current, name: event.target.value }))} placeholder="例如：企业 OpenAI" /></label>
          <label><span>供应商</span><select value={providerDraft.providerType} onChange={(event) => setProviderDraft((current) => ({ ...current, providerType: event.target.value }))}>{providerOptions.map((provider) => <option key={provider}>{provider}</option>)}</select></label>
          {providerDraft.providerType === "OpenAI 兼容" && <label className="model-form-wide"><span>Base URL</span><input value={providerDraft.baseUrl} onChange={(event) => setProviderDraft((current) => ({ ...current, baseUrl: event.target.value }))} placeholder="https://models.example.com/v1" /></label>}
          <label className="model-form-wide"><span>API Key</span><input type="password" value={providerDraft.apiKey} onChange={(event) => setProviderDraft((current) => ({ ...current, apiKey: event.target.value }))} placeholder="只写，不会再次显示" /></label>
        </form></div>
        <footer><span className="model-dialog-boundary"><ShieldCheck size={13} />凭据由工作区托管</span><div><button className="secondary-button" type="button" onClick={() => setProviderDialogKind(null)}>取消</button><button className="primary-button" type="submit" form="model-provider-form" disabled={!providerDraft.name.trim() || !providerDraft.apiKey.trim() || (providerDraft.providerType === "OpenAI 兼容" && !providerDraft.baseUrl.trim())}>添加供应商</button></div></footer>
      </section></div>}

      {modelDialog && <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setModelDialog(null); }}><section className="review-dialog compact-dialog model-config-dialog" role="dialog" aria-modal="true" aria-labelledby="model-dialog-title">
        <header><div><span className="content-label">{kindLabel}</span><h2 id="model-dialog-title">{modelDialog.model ? "编辑模型" : "添加模型"}</h2></div><button className="icon-button" type="button" aria-label="关闭模型配置" title="关闭" onClick={() => setModelDialog(null)}><X size={16} /></button></header>
        <div className="dialog-body"><form id="model-entry-form" className="model-config-form" onSubmit={saveModel}>
          <label className="model-form-wide"><span>模型 ID</span><input autoFocus value={modelDraft.modelId} onChange={(event) => setModelDraft((current) => ({ ...current, modelId: event.target.value }))} placeholder={kind === "llm" ? "例如：gpt-4.1" : "例如：text-embedding-3-large"} /></label>
          <label><span>{kind === "llm" ? "能力" : "检索能力"}</span><input value={modelDraft.capability} onChange={(event) => setModelDraft((current) => ({ ...current, capability: event.target.value }))} /></label>
          <label><span>最大输入 tokens</span><input type="number" min="1024" value={modelDraft.limit} onChange={(event) => setModelDraft((current) => ({ ...current, limit: event.target.value }))} /></label>
          {kind === "embedding" && <label><span>向量维度</span><input type="number" min="128" value={modelDraft.dimension} onChange={(event) => setModelDraft((current) => ({ ...current, dimension: event.target.value }))} /></label>}
          <label className="model-form-toggle"><input type="checkbox" checked={modelDraft.enabled} onChange={(event) => setModelDraft((current) => ({ ...current, enabled: event.target.checked }))} /><span>保存后启用模型</span></label>
        </form></div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={() => setModelDialog(null)}>取消</button><button className="primary-button" type="submit" form="model-entry-form" disabled={!modelDraft.modelId.trim() || !modelDraft.limit}>保存模型</button></div></footer>
      </section></div>}

      {rebuildDialogOpen && <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setRebuildDialogOpen(false); }}><section className="review-dialog compact-dialog model-config-dialog" role="dialog" aria-modal="true" aria-labelledby="embedding-rebuild-title">
        <header><div><span className="content-label">Embedding</span><h2 id="embedding-rebuild-title">重建向量索引</h2></div><button className="icon-button" type="button" aria-label="关闭索引重建" title="关闭" onClick={() => setRebuildDialogOpen(false)}><X size={16} /></button></header>
        <div className="dialog-body">
          <p className="embedding-rebuild-intro">使用当前默认 Embedding 模型重新生成知识目录向量。重建在后台执行，完成前线上检索继续使用现有索引。</p>
          <dl className="embedding-rebuild-summary">
            <div><dt>重建范围</dt><dd>全部知识目录</dd></div>
            <div><dt>目标模型</dt><dd>{defaultModel?.modelId ?? "尚未配置"}</dd></div>
            <div><dt>切换方式</dt><dd>构建完成并校验通过后生效</dd></div>
          </dl>
        </div>
        <footer><span className="model-dialog-boundary"><ShieldCheck size={13} />现有索引持续提供服务</span><div><button className="secondary-button" type="button" onClick={() => setRebuildDialogOpen(false)}>取消</button><button className="primary-button" type="button" onClick={startEmbeddingRebuild}>开始重建</button></div></footer>
      </section></div>}

      {rebuildDetailOpen && rebuildRun && createPortal(<div className="dialog-backdrop embedding-rebuild-modal-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setRebuildDetailOpen(false); }}><section className="review-dialog embedding-rebuild-detail-dialog" role="dialog" aria-modal="true" aria-labelledby="embedding-rebuild-detail-title">
        <header><div><span className="content-label">向量索引重建 · {rebuildRun.status}</span><h2 id="embedding-rebuild-detail-title">重建进度与日志</h2><code>{rebuildRun.id}</code></div><button className="icon-button" type="button" aria-label="关闭重建进度与日志" title="关闭" onClick={() => setRebuildDetailOpen(false)}><X size={16} /></button></header>
        <div className="dialog-body embedding-rebuild-detail-body">
          <section className="embedding-rebuild-dialog-overview" aria-label="向量索引重建进度">
            <header><div><span>当前阶段</span><strong>{rebuildRun.phase}</strong><small>线上检索继续使用现有索引，质量校验通过后自动切换。</small></div><b>{rebuildRun.progress}%</b></header>
            <div className="embedding-run-progress-track" aria-label={`向量索引重建进度 ${rebuildRun.progress}%`}><i style={{ width: `${rebuildRun.progress}%` }} /></div>
            <ol className="embedding-run-stages embedding-rebuild-dialog-stages">
              {embeddingRebuildStages.map((stage, index) => { const currentIndex = embeddingRebuildStages.indexOf(rebuildRun.phase); const current = stage === rebuildRun.phase; const complete = index < currentIndex; return <li className={current ? "is-current" : complete ? "is-complete" : ""} key={stage}><span>{complete ? <CheckCircle2 size={14} /> : index + 1}</span><strong>{stage}</strong></li>; })}
            </ol>
            <dl className="embedding-rebuild-dialog-facts">
              <div><dt>目标模型</dt><dd>{rebuildRun.modelId}</dd></div>
              <div><dt>处理进度</dt><dd>{rebuildRun.processedItems.toLocaleString("zh-CN")} / {rebuildRun.totalItems.toLocaleString("zh-CN")} 个知识块</dd></div>
              <div><dt>开始时间</dt><dd>{rebuildRun.startedAt}</dd></div>
              <div><dt>运行时长</dt><dd>{rebuildRun.duration}</dd></div>
            </dl>
          </section>
          <section className="embedding-rebuild-dialog-log" aria-labelledby="embedding-rebuild-log-title">
            <header><div><FileClock size={15} /><span><h3 id="embedding-rebuild-log-title">执行日志</h3><small>最近更新 11:33:18</small></span></div><span className="embedding-rebuild-live-state"><i />持续更新</span></header>
            <div className="embedding-rebuild-log-list" role="log" aria-label="向量索引重建执行日志">
              {embeddingRebuildLogs(rebuildRun).map((entry) => <div className={entry.level === "RUNNING" ? "embedding-rebuild-log-entry is-running" : "embedding-rebuild-log-entry"} key={`${entry.time}-${entry.stage}`}><time>{entry.time}</time><code>{entry.level}</code><strong>{entry.stage}</strong><span>{entry.message}</span></div>)}
            </div>
          </section>
        </div>
        <footer><span className="model-dialog-boundary"><ShieldCheck size={13} />完整记录已归档，可供审计追溯</span><div><button className="secondary-button" type="button" onClick={() => { setRebuildDetailOpen(false); onOpenRebuildRun(); }}>在全局运行记录中打开<ArrowUpRight size={14} /></button><button className="primary-button" type="button" onClick={() => setRebuildDetailOpen(false)}>关闭</button></div></footer>
      </section></div>, document.body)}
    </section>
  );
}

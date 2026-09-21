import { useMemo, useState, type FormEvent } from "react";
import { Bot, Check, ChevronDown, CircleAlert, KeyRound, LoaderCircle, Network, Plus, Settings2, ShieldCheck, Sparkles, X } from "lucide-react";

import { EmbeddingIndexPanel } from "./embeddingRuntime";
import { useCan } from "./authorization";
import { useGovernanceRuntime } from "./governanceRuntime";
import type { GovernanceModelProtocol } from "./governance";
import type { ViewId } from "./types";

type ModelKind = "llm" | "embedding";

const providerProtocols: Array<{ value: GovernanceModelProtocol; label: string }> = [
  { value: "openai", label: "OpenAI" },
  { value: "anthropic", label: "Anthropic" },
  { value: "gemini", label: "Google Gemini" },
  { value: "openai_compatible", label: "OpenAI 兼容" },
];

function protocolLabel(protocol: GovernanceModelProtocol): string {
  return providerProtocols.find((item) => item.value === protocol)?.label ?? protocol;
}

/**
 * The revision digest is stored as `sha256:<64 hex>`. Slicing the raw digest for
 * display produced "Key vsha256:a"; strip the algorithm prefix first so the
 * marker shows the actual revision prefix an operator can match against a key.
 */
function credentialMarker(digest: string): string {
  const hex = digest.replace(/^sha256:/, "");
  return `凭据 ${hex.slice(0, 8) || "未记录"}`;
}

/**
 * Mirrors domain.IsValidCredentialEnvName: the persisted value is an environment
 * variable NAME, so it must be UPPER_SNAKE_CASE. Anything else is rejected by the
 * API with a blanket INVALID_ARGUMENT, so validate here where the reason can be
 * shown next to the field.
 */
const credentialEnvPattern = /^[A-Z][A-Z0-9_]{0,63}$/;

export function credentialEnvError(value: string): string {
  const trimmed = value.trim();
  if (trimmed === "") return "凭据环境变量名不能为空。";
  if (!credentialEnvPattern.test(trimmed)) return "需以大写字母开头，只能含大写字母、数字和下划线，例如 SEMLIA_DEEPSEEK_API_KEY。";
  return "";
}

function providerTone(protocol: GovernanceModelProtocol) {
  if (protocol === "anthropic") return "anthropic";
  if (protocol === "gemini") return "google";
  if (protocol === "openai_compatible") return "compatible";
  return "openai";
}

export function ModelConfigurationView({ onNotify, onNavigate }: { onNotify: (message: string) => void; onNavigate?: (view: ViewId) => void }) {
  const canManage = useCan("workspace.manage");
  const governance = useGovernanceRuntime();
  const [kind, setKind] = useState<ModelKind>("llm");
  const [collapsedProviderIds, setCollapsedProviderIds] = useState<Set<string>>(() => new Set());
  const [providerDialogKind, setProviderDialogKind] = useState<ModelKind | null>(null);
  const [providerDraft, setProviderDraft] = useState({ displayName: "", protocol: "openai" as GovernanceModelProtocol, baseUrl: "", credentialEnv: "", credential: "" });
  const [providerActionError, setProviderActionError] = useState("");
  const [savingProvider, setSavingProvider] = useState(false);
  const providerEnvHint = providerDialogKind ? credentialEnvError(providerDraft.credentialEnv) : "";
  const [modelDialog, setModelDialog] = useState<{ providerId: string; modelId: string | null } | null>(null);
  const [modelActionError, setModelActionError] = useState("");
  const [savingModel, setSavingModel] = useState(false);
  const [rotationDialog, setRotationDialog] = useState<string | null>(null);
  const [rotationDraft, setRotationDraft] = useState({ credentialEnv: "", credential: "" });
  const [rotationError, setRotationError] = useState("");
  const [modelDraft, setModelDraft] = useState({ model: "", capability: "文本 · 推理", tokenLimit: "128000", dimension: "1024", enabled: true });
  const rotationEnvHint = rotationDialog ? credentialEnvError(rotationDraft.credentialEnv) : "";

  const activeProviders = useMemo(() => governance.modelProviders
    .map((entry) => ({ provider: entry.provider, models: entry.models.filter((model) => model.kind === kind) })), [governance.modelProviders, kind]);
  const enabledModels = useMemo(() => activeProviders.flatMap((provider) => provider.provider.enabled ? provider.models.filter((model) => model.enabled) : []), [activeProviders]);
  const defaultModel = enabledModels.find((model) => model.isDefault) ?? null;
  const kindLabel = kind === "llm" ? "LLM" : "Embedding";
  const modelConfigBusy = governance.modelProvidersState === "idle";

  const setDefaultModel = async (modelId: string) => {
    try {
      const setting = await governance.setDefaultModelSetting(modelId);
      onNotify(`${kindLabel} 默认模型已切换为 ${setting.model}。`);
    } catch (reason) {
      const message = reason instanceof Error ? reason.message : "默认模型切换失败。";
      onNotify(message);
    }
  };

  const toggleProvider = async (providerId: string, enabled: boolean) => {
    const provider = governance.modelProviders.find((item) => item.provider.id === providerId);
    if (!provider) return;
    try {
      await governance.updateProvider(providerId, { displayName: provider.provider.displayName, baseUrl: provider.provider.baseUrl, enabled });
    } catch (reason) {
      onNotify(reason instanceof Error ? reason.message : "供应商状态更新失败。");
    }
  };

  const toggleModel = async (settingId: string, enabled: boolean) => {
    const setting = governance.modelProviders.flatMap((item) => item.models).find((model) => model.id === settingId);
    if (!setting) return;
    try {
      await governance.updateModelSetting(settingId, { model: setting.model, capability: setting.capability, tokenLimit: setting.tokenLimit, enabled, embeddingDimension: setting.embeddingDimension });
    } catch (reason) {
      onNotify(reason instanceof Error ? reason.message : "模型状态更新失败。");
    }
  };

  const openProviderDialog = () => {
    setProviderDraft({ displayName: "", protocol: "openai", baseUrl: "", credentialEnv: "", credential: "" });
    setProviderActionError("");
    setProviderDialogKind(kind);
  };

  const saveProvider = async (event: FormEvent) => {
    event.preventDefault();
    if (!providerDialogKind || savingProvider) return;
    setSavingProvider(true);
    setProviderActionError("");
    try {
      const created = await governance.createProvider({
        protocol: providerDraft.protocol,
        displayName: providerDraft.displayName.trim(),
        baseUrl: providerDraft.protocol === "openai_compatible" ? providerDraft.baseUrl.trim() : undefined,
        credentialEnv: providerDraft.credentialEnv.trim(),
        credential: providerDraft.credential,
      });
      setProviderDialogKind(null);
      setProviderDraft({ displayName: "", protocol: "openai", baseUrl: "", credentialEnv: "", credential: "" });
      onNotify(`${created.provider.displayName} 已加入 ${providerDialogKind === "llm" ? "LLM" : "Embedding"} 模型配置。`);
    } catch (reason) {
      setProviderActionError(reason instanceof Error ? reason.message : "模型供应商创建失败。");
    } finally {
      setSavingProvider(false);
    }
  };

  const rotateCredential = async (event: FormEvent) => {
    event.preventDefault();
    if (!rotationDialog) return;
    const provider = governance.modelProviders.find((item) => item.provider.id === rotationDialog);
    if (!provider) return;
    try {
      const updated = await governance.updateProvider(rotationDialog, {
        displayName: provider.provider.displayName,
        baseUrl: provider.provider.baseUrl,
        enabled: provider.provider.enabled,
        credentialEnv: rotationDraft.credentialEnv.trim() || undefined,
        credential: rotationDraft.credential || undefined,
      });
      setRotationDialog(null);
      setRotationDraft({ credentialEnv: "", credential: "" });
      onNotify(`${updated.provider.displayName} 的凭据修订已更新为 ${credentialMarker(updated.provider.credentialRevision)}。`);
    } catch (reason) {
      setRotationError(reason instanceof Error ? reason.message : "凭据轮换失败。");
    }
  };

  const openModelDialog = (providerId: string, modelId: string | null = null) => {
    const model = governance.modelProviders.flatMap((item) => item.models).find((entry) => entry.id === modelId);
    setModelDraft({
      model: model?.model ?? "",
      capability: model?.capability ?? (kind === "llm" ? "文本 · 推理" : "多语言检索"),
      tokenLimit: String(model?.tokenLimit ?? (kind === "llm" ? 128000 : 8192)),
      dimension: String(model?.embeddingDimension ?? 1024),
      enabled: model?.enabled ?? true,
    });
    setModelActionError("");
    setModelDialog({ providerId, modelId });
  };

  const saveModel = async (event: FormEvent) => {
    event.preventDefault();
    if (!modelDialog || savingModel) return;
    setSavingModel(true);
    setModelActionError("");
    try {
      if (modelDialog.modelId) {
        await governance.updateModelSetting(modelDialog.modelId, {
          model: modelDraft.model.trim(),
          capability: modelDraft.capability.trim(),
          tokenLimit: Number(modelDraft.tokenLimit || 0),
          enabled: modelDraft.enabled,
          embeddingDimension: kind === "embedding" ? Number(modelDraft.dimension) : undefined,
        });
      } else {
        await governance.createModelSetting({
          providerId: modelDialog.providerId,
          kind,
          model: modelDraft.model.trim(),
          capability: modelDraft.capability.trim(),
          tokenLimit: Number(modelDraft.tokenLimit || 0),
          embeddingDimension: kind === "embedding" ? Number(modelDraft.dimension) : undefined,
        });
      }
      onNotify(`${modelDraft.model.trim()} 已保存到 ${kindLabel} 模型配置。`);
      setModelDialog(null);
    } catch (reason) {
      setModelActionError(reason instanceof Error ? reason.message : "模型条目保存失败。");
    } finally {
      setSavingModel(false);
    }
  };

  return (
    <section className="view settings-view model-config-view" aria-label="模型配置">
      <header className="model-config-header">
        <div className="model-kind-tabs" role="tablist" aria-label="模型类型">
          <button type="button" role="tab" aria-selected={kind === "llm"} onClick={() => setKind("llm")}><Bot size={14} />LLM 模型</button>
          <button type="button" role="tab" aria-selected={kind === "embedding"} onClick={() => setKind("embedding")}><Network size={14} />Embedding 模型</button>
        </div>
      </header>

      {governance.modelProvidersError && <div className="model-config-error" role="alert"><CircleAlert size={16} /><span>{governance.modelProvidersError}</span></div>}

      <div className="model-config-toolbar" aria-label={`${kindLabel} 模型设置`}>
        <label className="model-default-select"><span>{kind === "llm" ? "默认问答模型" : "默认索引模型"}</span><select aria-label={kind === "llm" ? "默认 LLM 模型" : "默认 Embedding 模型"} value={defaultModel?.id ?? ""} onChange={(event) => { if (event.target.value) void setDefaultModel(event.target.value); }} disabled={modelConfigBusy || !canManage}>{enabledModels.length === 0 && <option value="">尚未配置</option>}{activeProviders.flatMap((provider) => provider.provider.enabled ? provider.models.filter((model) => model.enabled).map((model) => <option value={model.id} key={model.id}>{provider.provider.displayName} · {model.model}</option>) : [])}</select></label>
        <div className="model-toolbar-actions">
          <span className="model-vault-state"><ShieldCheck size={14} />密钥只写托管</span>
          {canManage && <button className="primary-button model-add-provider" type="button" onClick={openProviderDialog}><Plus size={15} />添加供应商</button>}
        </div>
      </div>

      {kind === "embedding" && <EmbeddingIndexPanel key={governance.workspaceId} workspaceId={governance.workspaceId} canManage={canManage} onNavigate={onNavigate} />}

      <div className="model-provider-list" aria-label={`${kindLabel} 供应商和模型`}>
        {modelConfigBusy && <div className="model-config-loading" role="status"><LoaderCircle className="spin" size={16} />正在加载模型配置</div>}
        {!modelConfigBusy && activeProviders.length === 0 && <div className="model-config-empty" role="status"><Bot size={20} /><strong>尚未配置{kindLabel}供应商</strong><span>添加供应商后即可在此管理模型、默认项与凭据修订。</span></div>}
        {activeProviders.map((provider) => {
          const collapsed = collapsedProviderIds.has(provider.provider.id);
          return <section className={collapsed ? "model-provider-group is-collapsed" : "model-provider-group"} key={provider.provider.id} aria-label={`${provider.provider.displayName} 供应商`}>
            <header className="model-provider-row">
              <button className="model-icon-button model-provider-collapse" type="button" aria-label={`${collapsed ? "展开" : "折叠"} ${provider.provider.displayName} 的模型`} aria-expanded={!collapsed} title={`${collapsed ? "展开" : "折叠"}模型`} onClick={() => setCollapsedProviderIds((current) => { const next = new Set(current); if (next.has(provider.provider.id)) next.delete(provider.provider.id); else next.add(provider.provider.id); return next; })}><ChevronDown size={15} /></button>
              <span className={`model-provider-mark is-${providerTone(provider.provider.protocol)}`}><Bot size={16} /></span>
              <span className="model-provider-identity"><strong>{provider.provider.displayName}</strong><small>{protocolLabel(provider.provider.protocol)} · <code title="凭据环境变量">{provider.provider.credentialEnv}</code> · {credentialMarker(provider.provider.credentialRevision)}</small></span>
              <span className={provider.provider.enabled ? "model-status is-enabled" : "model-status is-disabled"}>{provider.provider.enabled ? "已启用" : "已停用"}</span>
              <span className="model-provider-count">{provider.models.length} 个模型</span>
              {canManage && <label className="model-switch" title={`${provider.provider.enabled ? "停用" : "启用"}${provider.provider.displayName}`}><input type="checkbox" aria-label={`${provider.provider.enabled ? "停用" : "启用"}供应商 ${provider.provider.displayName}`} checked={provider.provider.enabled} onChange={(event) => void toggleProvider(provider.provider.id, event.target.checked)} /><span /></label>}
              {canManage && <button className="model-icon-button" type="button" aria-label={`轮换 ${provider.provider.displayName} 的凭据`} title="轮换凭据" onClick={() => { setRotationDraft({ credentialEnv: provider.provider.credentialEnv, credential: "" }); setRotationError(""); setRotationDialog(provider.provider.id); }}><KeyRound size={14} /></button>}
              {canManage && <button className="model-icon-button" type="button" aria-label={`为 ${provider.provider.displayName} 添加模型`} title="添加模型" onClick={() => openModelDialog(provider.provider.id)}><Plus size={15} /></button>}
            </header>
            <div className="model-rows" hidden={collapsed}>
              {provider.models.length > 0 && <div className="model-row model-row-head">
                <span />
                <span>模型 · 能力</span>
                <span>{kind === "embedding" ? "向量维度 · 输入上限" : "输入上限"}</span>
                <span>状态</span>
                <span />
                <span />
              </div>}
              {provider.models.map((model) => <div className="model-row" key={model.id}>
                <button className={model.isDefault ? "model-default-button is-active" : "model-default-button"} type="button" aria-label={model.isDefault ? `${model.model} 当前为默认模型` : `设 ${model.model} 为默认模型`} title={model.isDefault ? "当前默认模型" : "设为默认模型"} disabled={!canManage || !provider.provider.enabled || !model.enabled} onClick={() => void setDefaultModel(model.id)}>{model.isDefault ? <Check size={13} /> : <Sparkles size={13} />}</button>
                <span className="model-row-identity"><strong>{model.model}</strong><small>{model.capability}</small></span>
                <span className="model-row-spec">{kind === "embedding" ? `${(model.embeddingDimension ?? 0).toLocaleString("en-US")} 维 · ${model.tokenLimit.toLocaleString("en-US")} tokens` : `${model.tokenLimit.toLocaleString("en-US")} tokens`}</span>
                <span className={model.enabled ? "model-status is-enabled" : "model-status is-disabled"}>{model.enabled ? "可用" : "已停用"}</span>
                {canManage && <label className="model-switch" title={`${model.enabled ? "停用" : "启用"}${model.model}`}><input type="checkbox" aria-label={`${model.enabled ? "停用" : "启用"}模型 ${model.model}`} checked={model.enabled} onChange={(event) => void toggleModel(model.id, event.target.checked)} /><span /></label>}
                {canManage && <button className="model-icon-button" type="button" aria-label={`编辑模型 ${model.model}`} title="编辑模型" onClick={() => openModelDialog(provider.provider.id, model.id)}><Settings2 size={14} /></button>}
              </div>)}
              {provider.models.length === 0 && <div className="model-row-empty"><span>该供应商在当前类型下尚未配置模型</span>{canManage && <button className="secondary-button" type="button" onClick={() => openModelDialog(provider.provider.id)}><Plus size={13} />添加模型</button>}</div>}
            </div>
          </section>;
        })}
      </div>

      <footer className="model-config-footnote"><ShieldCheck size={14} /><span>凭据只写且不会回显，模型配置通过治理 API 持久化。</span></footer>

      {providerDialogKind && <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setProviderDialogKind(null); }}><section className="review-dialog compact-dialog model-config-dialog" role="dialog" aria-modal="true" aria-labelledby="provider-dialog-title">
        <header><div><span className="content-label">{providerDialogKind === "llm" ? "LLM" : "Embedding"}</span><h2 id="provider-dialog-title">添加模型供应商</h2></div><button className="icon-button" type="button" aria-label="关闭供应商配置" title="关闭" onClick={() => setProviderDialogKind(null)}><X size={16} /></button></header>
        <div className="dialog-body"><form id="model-provider-form" className="model-config-form" onSubmit={saveProvider}>
          <label><span>配置名称</span><input autoFocus value={providerDraft.displayName} onChange={(event) => setProviderDraft((current) => ({ ...current, displayName: event.target.value }))} placeholder="例如：企业 OpenAI" /></label>
          <label><span>供应商</span><select value={providerDraft.protocol} onChange={(event) => setProviderDraft((current) => ({ ...current, protocol: event.target.value as GovernanceModelProtocol }))}>{providerProtocols.map((protocol) => <option key={protocol.value} value={protocol.value}>{protocol.label}</option>)}</select></label>
          {providerDraft.protocol === "openai_compatible" && <label className="model-form-wide"><span>Base URL</span><input value={providerDraft.baseUrl} onChange={(event) => setProviderDraft((current) => ({ ...current, baseUrl: event.target.value }))} placeholder="https://models.example.com/v1" /></label>}
          <div className="model-form-wide model-form-field"><label><span>凭据环境变量</span><input value={providerDraft.credentialEnv} onChange={(event) => setProviderDraft((current) => ({ ...current, credentialEnv: event.target.value }))} placeholder="SEMLIA_DEEPSEEK_API_KEY" aria-invalid={providerEnvHint !== ""} spellCheck={false} autoComplete="off" /></label>{providerEnvHint ? <small className="model-form-hint" role="note">{providerEnvHint}</small> : <small className="model-form-hint">与部署环境注入的变量名保持一致，平台只保存变量名与密钥摘要。</small>}</div>
          <label className="model-form-wide"><span>API Key</span><input type="password" autoComplete="off" value={providerDraft.credential} onChange={(event) => setProviderDraft((current) => ({ ...current, credential: event.target.value }))} placeholder="只写，不会再次显示" /></label>
          {providerActionError && <div className="model-dialog-error" role="alert"><CircleAlert size={15} />{providerActionError}</div>}
        </form></div>
        <footer><span className="model-dialog-boundary"><ShieldCheck size={13} />密钥由运行环境注入，平台只保存环境变量名与修订摘要</span><div><button className="secondary-button" type="button" onClick={() => setProviderDialogKind(null)}>取消</button><button className="primary-button" type="submit" form="model-provider-form" disabled={savingProvider || providerEnvHint !== "" || !providerDraft.displayName.trim() || !providerDraft.credentialEnv.trim() || !providerDraft.credential.trim() || (providerDraft.protocol === "openai_compatible" && !providerDraft.baseUrl.trim())}>{savingProvider ? <LoaderCircle className="spin" size={15} /> : null}添加供应商</button></div></footer>
      </section></div>}

      {rotationDialog && <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setRotationDialog(null); }}><section className="review-dialog compact-dialog model-config-dialog" role="dialog" aria-modal="true" aria-labelledby="rotation-dialog-title">
        <header><div><span className="content-label">凭据轮换</span><h2 id="rotation-dialog-title">更新凭据</h2></div><button className="icon-button" type="button" aria-label="关闭凭据轮换" title="关闭" onClick={() => setRotationDialog(null)}><X size={16} /></button></header>
        <div className="dialog-body"><form id="credential-rotation-form" className="model-config-form" onSubmit={rotateCredential}>
          <div className="model-form-wide model-form-field"><label><span>凭据环境变量</span><input value={rotationDraft.credentialEnv} onChange={(event) => setRotationDraft((current) => ({ ...current, credentialEnv: event.target.value }))} placeholder="留空保持不变" aria-invalid={rotationEnvHint !== ""} spellCheck={false} autoComplete="off" /></label>{rotationEnvHint && <small className="model-form-hint" role="note">{rotationEnvHint}</small>}</div>
          <label className="model-form-wide"><span>新 API Key</span><input type="password" autoComplete="off" value={rotationDraft.credential} onChange={(event) => setRotationDraft((current) => ({ ...current, credential: event.target.value }))} placeholder="只写，保存后生成新的修订摘要" /></label>
          {rotationError && <div className="model-dialog-error" role="alert"><CircleAlert size={15} />{rotationError}</div>}
        </form></div>
        <footer><span className="model-dialog-boundary"><ShieldCheck size={13} />轮换只更新修订摘要，不会回显任何密钥</span><div><button className="secondary-button" type="button" onClick={() => setRotationDialog(null)}>取消</button><button className="primary-button" type="submit" form="credential-rotation-form" disabled={rotationEnvHint !== "" || (!rotationDraft.credentialEnv.trim() && !rotationDraft.credential.trim())}>更新凭据</button></div></footer>
      </section></div>}

      {modelDialog && <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) setModelDialog(null); }}><section className="review-dialog compact-dialog model-config-dialog" role="dialog" aria-modal="true" aria-labelledby="model-dialog-title">
        <header><div><span className="content-label">{kindLabel}</span><h2 id="model-dialog-title">{modelDialog.modelId ? "编辑模型" : "添加模型"}</h2></div><button className="icon-button" type="button" aria-label="关闭模型配置" title="关闭" onClick={() => setModelDialog(null)}><X size={16} /></button></header>
        <div className="dialog-body"><form id="model-entry-form" className="model-config-form" onSubmit={saveModel}>
          <label className="model-form-wide"><span>模型 ID</span><input autoFocus value={modelDraft.model} onChange={(event) => setModelDraft((current) => ({ ...current, model: event.target.value }))} placeholder={kind === "llm" ? "例如：gpt-4.1" : "例如：text-embedding-3-large"} /></label>
          <label><span>{kind === "llm" ? "能力" : "检索能力"}</span><input value={modelDraft.capability} onChange={(event) => setModelDraft((current) => ({ ...current, capability: event.target.value }))} /></label>
          <label><span>最大输入 tokens</span><input type="number" min="1024" value={modelDraft.tokenLimit} onChange={(event) => setModelDraft((current) => ({ ...current, tokenLimit: event.target.value }))} /></label>
          {kind === "embedding" && <label><span>向量维度</span><input type="number" min="1" max="4096" value={modelDraft.dimension} onChange={(event) => setModelDraft((current) => ({ ...current, dimension: event.target.value }))} /></label>}
          <label className="model-form-toggle"><input type="checkbox" checked={modelDraft.enabled} onChange={(event) => setModelDraft((current) => ({ ...current, enabled: event.target.checked }))} /><span>保存后启用模型</span></label>
          {modelActionError && <div className="model-dialog-error" role="alert"><CircleAlert size={15} />{modelActionError}</div>}
        </form></div>
        <footer><span /><div><button className="secondary-button" type="button" onClick={() => setModelDialog(null)}>取消</button><button className="primary-button" type="submit" form="model-entry-form" disabled={savingModel || !modelDraft.model.trim() || !modelDraft.tokenLimit}>{savingModel ? <LoaderCircle className="spin" size={15} /> : null}保存模型</button></div></footer>
      </section></div>}

    </section>
  );
}

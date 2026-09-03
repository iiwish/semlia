import { useMemo, useState } from "react";
import { ArrowRight, Bot, Check, CircleAlert, LoaderCircle, MessageSquareText, Settings2, Sparkles, X } from "lucide-react";

import { GOVERNANCE_ERROR_CODES, type GovernanceProposalDetail } from "./governance";
import { useGovernanceRuntime } from "./governanceRuntime";
import type { Asset } from "./types";

interface AIProposalDialogProps {
  asset: Asset;
  fieldPath: string;
  onClose: () => void;
  onOpenModelSettings: () => void;
  onProposalSubmitted: (proposal: GovernanceProposalDetail) => void;
}

function formatChangeValue(value: unknown): string {
  if (value === undefined || value === null) return "（无）";
  if (Array.isArray(value)) return value.map((item) => String(item)).join("\n");
  if (typeof value === "object") return JSON.stringify(value, null, 2);
  return String(value);
}

function providerFailureCopy(code: string): string {
  if (code === GOVERNANCE_ERROR_CODES.PROVIDER_UNAVAILABLE) return "模型服务暂不可用，Agent 运行已记录为失败，可稍后重试。";
  if (code === GOVERNANCE_ERROR_CODES.PROVIDER_UNSUPPORTED) return "当前供应商协议不支持提案生成（anthropic/gemini 适配器尚未接入）。";
  if (code === GOVERNANCE_ERROR_CODES.AI_OUTPUT_INVALID) return "模型输出未通过提案 Schema 校验，没有创建提案。";
  return "提案生成失败。";
}

export function AIProposalDialog({ asset, fieldPath, onClose, onOpenModelSettings, onProposalSubmitted }: AIProposalDialogProps) {
  const governance = useGovernanceRuntime();
  const defaultLlm = useMemo(() => governance.modelProviders
    .filter((provider) => provider.provider.enabled)
    .flatMap((provider) => provider.models)
    .find((model) => model.kind === "llm" && model.enabled && model.isDefault) ?? null, [governance.modelProviders]);
  const [instruction, setInstruction] = useState("");
  const [generating, setGenerating] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [failure, setFailure] = useState<{ code: string; message: string } | null>(null);
  const [generated, setGenerated] = useState<{ agentRun: { id: string; model: string; configRevision: string; inputHash: string; costMicros: number; durationMs?: number; status: string }; proposal: GovernanceProposalDetail } | null>(null);

  const generate = async () => {
    if (!instruction.trim() || generating) return;
    setGenerating(true);
    setFailure(null);
    try {
      const result = await governance.generateProposal({
        targetObjectType: "semantic_asset",
        targetObjectId: asset.id,
        instruction: instruction.trim(),
      });
      setGenerated(result);
    } catch (reason) {
      const error = reason as { code?: string; message?: string };
      setFailure({ code: error.code ?? "REQUEST_FAILED", message: error.message ?? "提案生成失败。" });
    } finally {
      setGenerating(false);
    }
  };

  const submitGenerated = async () => {
    if (!generated || submitting) return;
    setSubmitting(true);
    setFailure(null);
    try {
      const submitted = await governance.submitProposal(generated.proposal.id);
      onProposalSubmitted(submitted);
    } catch (reason) {
      const error = reason as { code?: string; message?: string };
      setFailure({ code: error.code ?? "REQUEST_FAILED", message: error.message ?? "AI 提案提交失败。" });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog compact-dialog ai-proposal-dialog" role="dialog" aria-modal="true" aria-labelledby="ai-proposal-title">
        <header>
          <div><span className="content-label">AI 生成提案 · SSOT §8.6</span><h2 id="ai-proposal-title">让模型起草 {asset.name} 的知识修订</h2></div>
          <button className="icon-button" type="button" aria-label="关闭 AI 提案生成" onClick={onClose}><X size={18} /></button>
        </header>
        <div className="dialog-body">
          {!defaultLlm ? <div className="ai-proposal-empty" role="status">
            <span><Settings2 size={20} /></span>
            <div>
              <strong>尚未配置可用的默认 LLM 模型</strong>
              <p>AI 提案生成需要工作区配置一个已启用的默认 LLM。模型配置通过治理 API 持久化，凭据只写不回显。</p>
            </div>
            <button className="secondary-button" type="button" onClick={onOpenModelSettings}><Settings2 size={14} />打开模型配置</button>
          </div> : generated ? <>
            <section className="ai-proposal-run" aria-label="Agent 运行记录">
              <div><dt>Agent Run</dt><dd><code>{generated.agentRun.id}</code></dd></div>
              <div><dt>模型</dt><dd>{generated.agentRun.model}</dd></div>
              <div><dt>配置修订</dt><dd><code title={generated.agentRun.inputHash}>{generated.agentRun.configRevision}</code></dd></div>
              <div><dt>成本 / 时长</dt><dd>{generated.agentRun.costMicros} µ · {generated.agentRun.durationMs ?? 0} ms</dd></div>
            </section>
            <section className="ai-proposal-result" aria-label="生成的提案">
              <span className="proposal-author ai-author"><Bot size={16} /></span>
              <div><strong>{generated.proposal.title}</strong><p>{generated.proposal.summary}</p></div>
            </section>
            <div className="candidate-diff-list">
              {generated.proposal.changeSet.map((change) => (
                <article className="diff-block" key={change.id}>
                  <header><code className="diff-field">{change.fieldPath}</code><span>{change.op === "add" ? "新增字段" : change.op === "remove" ? "移除字段" : "字段变更"}</span></header>
                  <div className="candidate-diff-compare">
                    <section className="candidate-diff-value candidate-diff-before"><span>当前 {asset.revision}</span><p>{formatChangeValue(change.beforeValue)}</p></section>
                    <ArrowRight size={15} />
                    <section className="candidate-diff-value candidate-diff-after"><span>AI 草案</span><p>{formatChangeValue(change.afterValue)}</p></section>
                  </div>
                </article>
              ))}
            </div>
            {failure && <div className="ai-proposal-failure" role="alert"><CircleAlert size={16} /><span><strong>{providerFailureCopy(failure.code)}</strong><small><code>{failure.code}</code> · {failure.message}</small></span></div>}
          </> : <>
            <div className="dialog-summary"><span className="proposal-author ai-author"><Sparkles size={16} /></span><div><strong>{asset.name} · {fieldPath}</strong><p>生成结果是一份 AI 归因的草稿提案，提交后进入与人工修订相同的验证、评审与发布流程。</p></div></div>
            <label className="review-note"><span>生成指令 <strong>必填</strong></span><textarea aria-label="AI 生成指令" rows={5} placeholder="描述需要修正或补充的知识内容，例如口径边界、计算规则或证据缺口。" value={instruction} onChange={(event) => { setInstruction(event.target.value); setFailure(null); }} /></label>
            <div className="ai-proposal-boundary"><MessageSquareText size={14} /><span>本次调用会记录一条 Agent 运行（模型、配置修订、输入摘要、成本与时长），不会直接发布任何内容。</span></div>
            {failure && <div className="ai-proposal-failure" role="alert"><CircleAlert size={16} /><span><strong>{providerFailureCopy(failure.code)}</strong><small><code>{failure.code}</code> · {failure.message}</small></span></div>}
          </>}
        </div>
        <footer>
          <span className="model-dialog-boundary">默认模型：{defaultLlm ? `${defaultLlm.model}` : "未配置"}</span>
          <div>
            {generated ? <>
              <button className="secondary-button" type="button" onClick={onClose}>关闭</button>
              <button className="primary-button" type="button" disabled={submitting} onClick={submitGenerated}>{submitting ? <LoaderCircle className="spin" size={15} /> : <Check size={15} />}提交 AI 提案</button>
            </> : <>
              <button className="secondary-button" type="button" onClick={onClose}>取消</button>
              <button className="primary-button" type="button" disabled={!defaultLlm || !instruction.trim() || generating} onClick={generate}>{generating ? <LoaderCircle className="spin" size={15} /> : <Sparkles size={15} />}{generating ? "正在生成" : "生成提案草稿"}</button>
            </>}
          </div>
        </footer>
      </section>
    </div>
  );
}

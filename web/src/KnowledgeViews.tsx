import { useState } from "react";
import {
  ArrowRight,
  BadgeCheck,
  ChevronRight,
  CircleAlert,
  FileKey2,
  LoaderCircle,
  MessageSquareWarning,
  Send,
  ShieldCheck,
  Sparkles,
  Waypoints,
} from "lucide-react";

import { AskApiError, askReleasedSemantics, type AskResponse } from "./ask";
import { AskExecutionPanel } from "./AskExecutionPanel";

interface AskViewProps {
  workspaceId: string;
  noPublishedKnowledge?: boolean;
  onOpenKnowledge?: () => void;
  onOpenEvidence: (assetId?: string) => void;
  onStartRevision: (assetId: string | undefined, fieldPath: string, context: string) => void;
}

const answerIssueOptions = [
  { id: "definition", label: "业务定义或口径不准确", detail: "修订包含、排除、时间归属或业务边界。", fieldPath: "definition.boundary" },
  { id: "expression", label: "计算规则或实现不准确", detail: "修订公式、聚合或受治理的执行语义。", fieldPath: "spec.expression" },
  { id: "disambiguation", label: "问法被错误理解", detail: "修订检索词、同名消歧或回答范围。", fieldPath: "wiki.disambiguation" },
] as const;

export function errorMessage(error: unknown): { code: string; title: string; detail: string } {
  const code = error instanceof AskApiError ? error.code : error && typeof error === "object" && "code" in error && typeof error.code === "string" ? error.code : "REQUEST_FAILED";
  if (code === "PROVIDER_UNAVAILABLE") return { code, title: "模型服务暂不可用", detail: "本次请求已记录为失败，没有回退到演示答案。请检查默认模型及凭证后重试。" };
  if (code === "PROVIDER_UNSUPPORTED") return { code, title: "当前模型协议尚未接通", detail: "请选择已支持的 OpenAI 或 OpenAI-compatible 提供方。" };
  if (code === "CONFLICT") return { code, title: "尚未配置可用的默认模型", detail: "请先在系统设置中启用模型提供方，并为当前工作区选择默认模型。" };
  if (code === "AI_OUTPUT_INVALID") return { code, title: "模型没有返回合规的语义请求", detail: "输出已被结构化门禁拒绝，未进入语义解析，也没有生成查询结果。" };
  if (code === "NO_MATCHING_GRANT" || code === "CAPABILITY_DENIED") return { code, title: "当前身份不能使用语义问答", detail: "需要 semantic.resolve 能力；权限变化后请刷新会话。" };
  const detail = error instanceof Error ? error.message : error && typeof error === "object" && "message" in error && typeof error.message === "string" ? error.message : "请求失败，且没有使用演示数据替代。";
  return { code, title: "语义问答未完成", detail };
}

function AskResultView({ result, question, reportingIssue, selectedIssue, onOpenEvidence, onReportingIssue, onSelectIssue, onStartRevision, onRefineQuestion }: {
  result: AskResponse;
  question: string;
  reportingIssue: boolean;
  selectedIssue: (typeof answerIssueOptions)[number]["id"];
  onOpenEvidence: (assetId?: string) => void;
  onReportingIssue: (open: boolean) => void;
  onSelectIssue: (issue: (typeof answerIssueOptions)[number]["id"]) => void;
  onStartRevision: (assetId: string | undefined, fieldPath: string, context: string) => void;
  onRefineQuestion: () => void;
}) {
  const resolution = result.resolution;
  const plan = resolution?.plan;
  const refusal = resolution?.refusal;
  const [targetId, setTargetId] = useState(result.definitions.length === 1 ? result.definitions[0].assetId : "");
  const [memberId, setMemberId] = useState("");
  const memberIds = [...new Set([...(plan?.grouping ?? []), ...(plan?.filters ?? []).map((filter) => filter.selector), ...(plan?.timeRange ? [plan.timeRange.selector] : [])].filter((selector) => selector.assetId === targetId).map((selector) => selector.memberId).filter((id): id is string => Boolean(id)))];
  const releaseLabel = resolution?.releaseId ?? "尚未选择发布版本";

  if (result.interpretation.outcome === "clarification") {
    return <article className="assistant-answer ask-clarification">
      <header><span className="assistant-mark"><MessageSquareWarning size={17} /></span><div><strong>需要补充信息</strong><small>模型解释已完成 · 尚未调用语义解析器</small></div><span className="grounded-badge ask-badge-neutral">未解析</span></header>
      <div className="answer-copy"><p>{result.interpretation.clarification}</p><p>补充指标、维度、时间范围或比较对象后重新提问。</p></div>
    </article>;
  }

  if (refusal) {
    return <article className="assistant-answer ask-refusal">
      <header><span className="assistant-mark ask-mark-warning"><CircleAlert size={17} /></span><div><strong>语义解析被拒绝</strong><small>Agent run {result.agentRun.id} · 没有生成执行计划</small></div><span className="grounded-badge ask-badge-warning">{refusal.code}</span></header>
      <div className="answer-copy"><p><strong>{refusal.clarification}</strong></p><p>这是显式拒绝，不会被替换成猜测、样例值或未发布知识。</p></div>
      {refusal.candidateIds.length > 0 && <div className="ask-candidates"><span>可消歧候选</span>{refusal.candidateIds.map((id) => <code key={id}>{id}</code>)}</div>}
    </article>;
  }

  return <article className="assistant-answer">
    <header><span className="assistant-mark"><Sparkles size={17} /></span><div><strong>Semlia</strong><small>基于 {releaseLabel} · {result.definitions.length} 个已发布定义 · Agent run {result.agentRun.id}</small></div><span className="grounded-badge"><BadgeCheck size={13} />计划已验证</span></header>
    <div className="answer-copy">
      {result.definitions.length > 0 ? result.definitions.map((definition) => <p key={definition.assetId}><strong>{definition.name || definition.address}</strong>：{definition.definition || "该发布修订没有可展示的文字定义。"}</p>) : <p>语义请求已解析，但所选发布修订没有可展示的文字定义。</p>}
    </div>
    {plan && <section className="ask-plan" aria-label="已解析语义计划">
      <header><span><Waypoints size={15} />语义计划</span><code>{plan.planDigest}</code></header>
      <dl>
        <div><dt>意图</dt><dd>{plan.intent}</dd></div>
        <div><dt>分析模型</dt><dd>{plan.model ? `${plan.model.address} @ ${plan.model.revisionId}` : "未采用可执行模型"}</dd></div>
        <div><dt>资产</dt><dd>{plan.assets.length}</dd></div>
        <div><dt>受治理对象</dt><dd>{plan.objects.length}</dd></div>
        <div><dt>执行</dt><dd>{plan.executionStatus === "requires_execution_validation" ? "待执行验证" : "不可执行计划"}</dd></div>
      </dl>
      {plan.executionStatus === "not_configured" && <p><FileKey2 size={14} /><span><strong>已完成语义解析，未执行数据查询</strong>此计划缺少完整的已发布执行依据，不能连接数据源或展示结果行。</span></p>}
    </section>}
    {result.definitions.length > 0 && <div className="answer-evidence">
      <span className="content-label">已发布定义</span>
      {result.definitions.map((definition) => <button type="button" key={definition.assetId} onClick={() => onOpenEvidence(definition.assetId)}><span><strong>{definition.name || definition.address}</strong><small>{definition.assetType} · {definition.revisionId} · {definition.address}</small></span><ChevronRight size={14} /></button>)}
    </div>}
    {reportingIssue && <section className="answer-issue-panel" aria-label="指出回答中的知识问题">
      <header><span><CircleAlert size={16} /></span><div><strong>哪类知识需要修订？</strong><p>系统会定位已发布定义；当前发布版本保持只读。</p></div></header>
      <div role="radiogroup" aria-label="知识问题类型">{answerIssueOptions.map((option) => <label key={option.id}><input type="radio" name="answer-issue" value={option.id} checked={selectedIssue === option.id} onChange={() => onSelectIssue(option.id)} /><span><strong>{option.label}</strong><small>{option.detail}</small></span></label>)}</div>
      {selectedIssue !== "disambiguation" && <><label className="production-field"><span>修订对象</span><select aria-label="修订对象" value={targetId} onChange={(event) => { setTargetId(event.target.value); setMemberId(""); }}><option value="">选择涉及的知识</option>{result.definitions.map((definition) => <option key={definition.assetId} value={definition.assetId}>{definition.name || definition.address} · {definition.revisionId}</option>)}</select></label>{memberIds.length > 0 && <label className="production-field"><span>涉及成员</span><select aria-label="涉及成员" value={memberId} onChange={(event) => setMemberId(event.target.value)}><option value="">整个知识定义</option>{memberIds.map((id) => <option key={id}>{id}</option>)}</select></label>}</>}
      <footer><button className="text-button" type="button" onClick={() => onReportingIssue(false)}>取消</button>{selectedIssue === "disambiguation" ? <button className="primary-button" type="button" onClick={onRefineQuestion}>修改本次问题</button> : <button className="primary-button" type="button" disabled={!result.definitions.some((definition) => definition.assetId === targetId)} onClick={() => { const issue = answerIssueOptions.find((option) => option.id === selectedIssue) ?? answerIssueOptions[0]; onStartRevision(targetId, memberId ? `spec.members.${memberId}` : issue.fieldPath, `来自问答“${question}”：${issue.label}。${memberId ? `涉及成员：${memberId}。` : ""}`); }}><MessageSquareWarning size={15} />修订相关知识</button>}</footer>
    </section>}
    <footer><div><ShieldCheck size={14} /><span>原始问题不持久化；仅记录输入哈希、模型版本、发布版本和解析结果。</span></div>{result.definitions.length > 0 && <button className="secondary-button" type="button" aria-expanded={reportingIssue} onClick={() => onReportingIssue(!reportingIssue)}><MessageSquareWarning size={15} />指出问题</button>}</footer>
  </article>;
}

export function AskView({ workspaceId, noPublishedKnowledge = false, onOpenKnowledge, onOpenEvidence, onStartRevision }: AskViewProps) {
  const [question, setQuestion] = useState("");
  const [submittedQuestion, setSubmittedQuestion] = useState("");
  const [state, setState] = useState<"ready" | "running" | "answered" | "error">("ready");
  const [result, setResult] = useState<AskResponse | null>(null);
  const [failure, setFailure] = useState<{ code: string; title: string; detail: string } | null>(null);
  const [reportingIssue, setReportingIssue] = useState(false);
  const [selectedIssue, setSelectedIssue] = useState<(typeof answerIssueOptions)[number]["id"]>("definition");

  const ask = (nextQuestion?: string) => {
    const value = (nextQuestion ?? question).trim();
    if (!value || !workspaceId || noPublishedKnowledge || state === "running") return;
    setQuestion("");
    setSubmittedQuestion(value);
    setState("running");
    setResult(null);
    setFailure(null);
    setReportingIssue(false);
    askReleasedSemantics(workspaceId, {
        question: value,
        context: { mode: "current" },
        idempotencyKey: `web-ask-${globalThis.crypto.randomUUID()}`,
      })
      .then((response) => {
        setResult(response);
        setState("answered");
      })
      .catch((error: unknown) => {
        setFailure(errorMessage(error));
        setState("error");
      });
  };

  return <section className="view view-ask">
    <div className="ask-layout">
      <section className="conversation-panel" aria-label="语义问答会话">
        {state === "ready" ? <div className="ask-empty">
          <span className="ask-empty-icon"><Sparkles size={22} /></span>
          <h2>{noPublishedKnowledge ? "尚无已发布知识" : "今天想了解什么？"}</h2>
          {noPublishedKnowledge && <p>知识尚待确认与发布，暂不能作为问数依据。</p>}
          {noPublishedKnowledge && onOpenKnowledge && <button className="secondary-button" onClick={onOpenKnowledge}>查看待确认知识<ArrowRight size={14} /></button>}
        </div> : <div className="conversation-thread">
          <div className="user-message"><span>你</span><p>{submittedQuestion}</p></div>
          {state === "running" ? <div className="answer-loading" role="status"><LoaderCircle size={18} /><div><strong>正在解释并验证语义请求</strong><span>模型只负责结构化解释；发布版本选择与计划验证由 Semlia 执行。</span></div></div> : state === "error" && failure ? <article className="assistant-answer ask-error" role="alert"><header><span className="assistant-mark ask-mark-warning"><CircleAlert size={17} /></span><div><strong>{failure.title}</strong><small>{failure.code}</small></div><span className="grounded-badge ask-badge-warning">未回退</span></header><div className="answer-copy"><p>{failure.detail}</p></div></article> : result ? <AskResultView key={result.agentRun.id} onRefineQuestion={() => { setQuestion(submittedQuestion + "\n补充："); setReportingIssue(false); }} result={result} question={submittedQuestion} reportingIssue={reportingIssue} selectedIssue={selectedIssue} onOpenEvidence={onOpenEvidence} onReportingIssue={setReportingIssue} onSelectIssue={setSelectedIssue} onStartRevision={onStartRevision} /> : null}
        </div>}
      </section>
      <AskExecutionPanel key={`${workspaceId}:${result?.resolution?.plan?.id ?? "history"}`} workspaceId={workspaceId} plan={result?.resolution?.plan?.executionStatus === "requires_execution_validation" ? result.resolution.plan : undefined} />
      <form className="ask-composer" onSubmit={(event) => { event.preventDefault(); ask(); }}>
        <textarea aria-label="向 Semlia 提问" value={question} onChange={(event) => setQuestion(event.target.value)} placeholder="询问已发布的指标口径、维度或语义计划..." rows={2} />
        <div><span><ShieldCheck size={13} />仅使用已发布知识 · 查询需单独确认</span><button type="submit" aria-label="发送问题" disabled={noPublishedKnowledge || !question.trim() || state === "running"}><Send size={16} /></button></div>
      </form>
    </div>
  </section>;
}

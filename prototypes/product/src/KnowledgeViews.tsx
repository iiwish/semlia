import { useEffect, useRef, useState } from "react";
import {
  ArrowRight,
  BadgeCheck,
  ChevronRight,
  CircleAlert,
  LoaderCircle,
  MessageSquareWarning,
  Send,
  ShieldCheck,
  Sparkles,
} from "lucide-react";

interface AskViewProps {
  conversationIndex: number;
  activeRelease: string;
  onOpenEvidence: () => void;
  onStartRevision: (fieldPath: string, context: string) => void;
}

const suggestedQuestions = [
  "华东区 8 月净收入为什么低于目标？",
  "客单价当前采用什么退款口径？",
  "哪些核心指标缺少负责人或证据？",
];

const savedQuestions: Record<number, string> = {
  1: suggestedQuestions[0],
  2: suggestedQuestions[1],
  3: suggestedQuestions[2],
};

const knowledgeBlocks = [
  { id: "KB-2048", title: "净收入确认口径", source: "财务规则 FY2026", kind: "业务定义", asset: "commerce.net_revenue" },
  { id: "KB-1932", title: "业务区域映射", source: "dbt · dim_region", kind: "映射规则", asset: "commerce.business_region" },
  { id: "KB-1887", title: "退款订单排除规则", source: "Cube · commerce.orders", kind: "计算约束", asset: "commerce.paid_order_count" },
];

const answerIssueOptions = [
  { id: "definition", label: "业务定义或口径不准确", detail: "修订包含、排除、时间归属或业务边界。", fieldPath: "definition.boundary" },
  { id: "expression", label: "计算规则或实现不准确", detail: "修订公式、聚合或受治理的执行语义。", fieldPath: "spec.expression" },
  { id: "disambiguation", label: "问法被错误理解", detail: "修订检索词、同名消歧或回答范围。", fieldPath: "wiki.disambiguation" },
] as const;

function answerFor(question: string) {
  if (question.includes("客单价") || question.includes("退款口径")) {
    return {
      lead: <>当前生效语义版本中的客单价为 <strong>¥286.4</strong>，口径是净收入除以支付成功且未完全退款的订单数。</>,
      detail: "完全退款订单同时从净收入和订单分母中排除，部分退款只扣减净收入。该定义固定在当前 stable release，仍有一项西南区域回归差异等待负责人确认。",
    };
  }
  if (question.includes("负责人") || question.includes("证据") || question.includes("覆盖")) {
    return {
      lead: <>当前有 <strong>11 个知识对象</strong>需要补充治理信息，其中 3 个会影响核心经营问答。</>,
      detail: "业务区域维度缺少明确负责人，活跃客户的使用证据即将过期，旧区域别名还关联 3 个消费者。建议先审核高影响项，再生成新的生效语义版本。",
    };
  }
  return {
    lead: <>华东区 8 月净收入为 <strong>¥12.84M</strong>，较目标低 <strong>6.8%</strong>。主要差异来自企业渠道的已支付订单量下降，而不是退款率异常。</>,
    detail: "按当前发布口径，净收入已扣除完全退款订单。企业渠道订单量同比下降 9.4%，贡献了约 71% 的目标缺口；华东其他渠道总体接近目标。",
  };
}

export function AskView({ conversationIndex, activeRelease, onOpenEvidence, onStartRevision }: AskViewProps) {
  const [question, setQuestion] = useState("");
  const [submittedQuestion, setSubmittedQuestion] = useState(savedQuestions[conversationIndex] ?? "");
  const [state, setState] = useState<"ready" | "running" | "answered">(conversationIndex === 0 ? "ready" : "answered");
  const [reportingIssue, setReportingIssue] = useState(false);
  const [selectedIssue, setSelectedIssue] = useState<(typeof answerIssueOptions)[number]["id"]>("definition");
  const timerRef = useRef<number | null>(null);
  const answer = answerFor(submittedQuestion);

  useEffect(() => () => {
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
  }, []);

  const ask = (nextQuestion?: string) => {
    const value = (nextQuestion ?? question).trim();
    if (!value) return;
    if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    setQuestion("");
    setSubmittedQuestion(value);
    setState("running");
    setReportingIssue(false);
    timerRef.current = window.setTimeout(() => setState("answered"), 650);
  };

  return (
    <section className="view view-ask">
      <div className="ask-layout">
        <section className="conversation-panel" aria-label="语义问答会话">
          {state === "ready" ? (
            <div className="ask-empty">
              <span className="ask-empty-icon"><Sparkles size={22} /></span>
              <h2>今天想了解什么？</h2>
              <p>问题会被限制在当前生效版本的知识和可执行语义范围内。</p>
              <div className="question-suggestions">{suggestedQuestions.map((item) => <button type="button" key={item} onClick={() => ask(item)}>{item}<ArrowRight size={14} /></button>)}</div>
            </div>
          ) : (
            <div className="conversation-thread">
              <div className="user-message"><span>你</span><p>{submittedQuestion}</p></div>
              {state === "running" ? (
                <div className="answer-loading" role="status"><LoaderCircle size={18} /><div><strong>正在形成可信回答</strong><span>检索知识块、解析语义并执行受控查询...</span></div></div>
              ) : (
                <article className="assistant-answer">
                  <header><span className="assistant-mark"><Sparkles size={17} /></span><div><strong>Semlia</strong><small>基于 {activeRelease} · 3 个知识块 · Cube 查询</small></div><span className="grounded-badge"><BadgeCheck size={13} />已溯源</span></header>
                  <div className="answer-copy">
                    <p>{answer.lead}</p>
                    <p>{answer.detail}</p>
                  </div>
                  <div className="answer-evidence">
                    <span className="content-label">引用的知识块</span>
                    {knowledgeBlocks.map((block) => <button type="button" key={block.id} onClick={onOpenEvidence}><span><strong>{block.title}</strong><small>{block.id} · {block.kind} · {block.source}</small></span><ChevronRight size={14} /></button>)}
                  </div>
                  {reportingIssue && <section className="answer-issue-panel" aria-label="指出回答中的知识问题">
                    <header><span><CircleAlert size={16} /></span><div><strong>哪类知识需要修订？</strong><p>系统会定位相关 Claim，原始资料和当前发布版本保持只读。</p></div></header>
                    <div role="radiogroup" aria-label="知识问题类型">{answerIssueOptions.map((option) => <label key={option.id}><input type="radio" name="answer-issue" value={option.id} checked={selectedIssue === option.id} onChange={() => setSelectedIssue(option.id)} /><span><strong>{option.label}</strong><small>{option.detail}</small></span></label>)}</div>
                    <footer><button className="text-button" type="button" onClick={() => setReportingIssue(false)}>取消</button><button className="primary-button" type="button" onClick={() => { const issue = answerIssueOptions.find((option) => option.id === selectedIssue) ?? answerIssueOptions[0]; onStartRevision(issue.fieldPath, `来自问答“${submittedQuestion}”：${issue.label}。`); }}><MessageSquareWarning size={15} />修订相关知识</button></footer>
                  </section>}
                  <footer><div><ShieldCheck size={14} /><span>结果受 G1 策略保护，未把原始业务行发送给 LLM。</span></div><button className="secondary-button" type="button" aria-expanded={reportingIssue} onClick={() => setReportingIssue((open) => !open)}><MessageSquareWarning size={15} />指出问题</button></footer>
                </article>
              )}
            </div>
          )}

        </section>

        <form className="ask-composer" onSubmit={(event) => { event.preventDefault(); ask(); }}>
          <textarea aria-label="向 Semlia 提问" value={question} onChange={(event) => setQuestion(event.target.value)} placeholder="询问指标口径、经营变化或语义资产..." rows={2} />
          <div><span><ShieldCheck size={13} />仅使用已发布知识</span><button type="submit" aria-label="发送问题" disabled={!question.trim() || state === "running"}><Send size={16} /></button></div>
        </form>
      </div>
    </section>
  );
}

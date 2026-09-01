import { useMemo, useState } from "react";
import {
  ArrowRight,
  BookOpenCheck,
  Check,
  CheckCircle2,
  CircleAlert,
  FileCheck2,
  GitPullRequestArrow,
  LockKeyhole,
  Play,
  Save,
  ShieldCheck,
} from "lucide-react";

import type { Asset, KnowledgeRevisionRequest, KnowledgeRevisionSubmission } from "./types";

type EditableKnowledgeField = "definition" | "expression" | "includes" | "excludes" | "disambiguation";

interface KnowledgeFieldDescriptor {
  key: EditableKnowledgeField;
  label: string;
  fieldPath: string;
  help: string;
  multiline?: boolean;
  code?: boolean;
}

const knowledgeFields: KnowledgeFieldDescriptor[] = [
  { key: "definition", label: "业务定义", fieldPath: "definition.boundary", help: "说明这个知识对象是什么，以及适用的业务边界。", multiline: true },
  { key: "expression", label: "计算表达式", fieldPath: "spec.expression", help: "修改可执行语义，不直接改写来源代码或数据。", multiline: true, code: true },
  { key: "includes", label: "口径包含", fieldPath: "definition.includes", help: "每行一项，声明明确纳入当前口径的情况。", multiline: true },
  { key: "excludes", label: "明确排除", fieldPath: "definition.excludes", help: "每行一项，声明不应被解释为当前知识的情况。", multiline: true },
  { key: "disambiguation", label: "消歧规则", fieldPath: "wiki.disambiguation", help: "约束 AI 和消费者在同名、跨域或上下文不足时如何解释。", multiline: true },
];

function initialFieldFor(fieldPath: string): EditableKnowledgeField {
  if (fieldPath.includes("expression") || fieldPath.includes("binding")) return "expression";
  if (fieldPath.includes("includes")) return "includes";
  if (fieldPath.includes("excludes")) return "excludes";
  if (fieldPath.includes("disambiguation")) return "disambiguation";
  return "definition";
}

function fieldValues(asset: Asset): Record<EditableKnowledgeField, string> {
  return {
    definition: asset.revisionRecord.definition,
    expression: asset.revisionRecord.typeSpec.expression,
    includes: asset.revisionRecord.includes.join("\n"),
    excludes: asset.revisionRecord.excludes.join("\n"),
    disambiguation: asset.wikiContext.disambiguationRules.join("\n"),
  };
}

function normalize(value: string) {
  return value.trim().replace(/\r\n/g, "\n");
}

interface KnowledgeRevisionWorkbenchProps {
  asset: Asset;
  request: KnowledgeRevisionRequest;
  onCancel: () => void;
  onNotify: (message: string) => void;
  onSubmit: (submission: KnowledgeRevisionSubmission) => void;
}

export function KnowledgeRevisionWorkbench({ asset, request, onCancel, onNotify, onSubmit }: KnowledgeRevisionWorkbenchProps) {
  const publishedValues = useMemo(() => fieldValues(asset), [asset]);
  const [draftValues, setDraftValues] = useState(() => fieldValues(asset));
  const [activeField, setActiveField] = useState<EditableKnowledgeField>(() => initialFieldFor(request.fieldPath));
  const [reason, setReason] = useState(request.context ?? "");
  const [checksRun, setChecksRun] = useState(false);
  const [error, setError] = useState("");
  const descriptor = knowledgeFields.find((field) => field.key === activeField) ?? knowledgeFields[0];
  const changedFields = knowledgeFields.filter((field) => normalize(draftValues[field.key]) !== normalize(publishedValues[field.key]));
  const selectedClaim = asset.claims.find((claim) => claim.claimId === request.claimId)
    ?? asset.claims.find((claim) => claim.fieldPath === descriptor.fieldPath)
    ?? asset.claims.find((claim) => descriptor.fieldPath.startsWith(claim.fieldPath) || claim.fieldPath.startsWith(descriptor.fieldPath))
    ?? asset.claims[0];
  const selectedLink = asset.evidenceLinks.find((link) => link.claimId === selectedClaim?.claimId);
  const selectedEvidence = asset.evidence.find((evidence) => evidence.id === selectedLink?.evidenceId) ?? asset.evidence[0];
  const selectedArtifact = asset.evidenceArtifacts.find((artifact) => artifact.evidenceId === selectedEvidence?.id) ?? asset.evidenceArtifacts[0];

  const updateDraft = (value: string) => {
    setDraftValues((current) => ({ ...current, [activeField]: value }));
    setChecksRun(false);
    setError("");
  };

  const runChecks = () => {
    if (changedFields.length === 0) {
      setError("至少修改一项知识字段后才能运行检查。");
      return;
    }
    if (!reason.trim()) {
      setError("请记录修订原因，审核者需要理解为什么要改变当前知识。");
      return;
    }
    setError("");
    setChecksRun(true);
    onNotify(`${asset.name} 知识草稿预检完成：结构、证据引用与消费影响均已形成可审核结果。`);
  };

  const submit = () => {
    if (!checksRun || changedFields.length === 0 || !reason.trim()) return;
    const labels = changedFields.map((field) => field.label);
    onSubmit({
      assetId: asset.id,
      baseRevision: asset.revision,
      title: `修订${asset.name}的${labels.join("、")}`,
      summary: `修订${labels.join("、")}，保留来源资料与已发布 ${asset.revision} 的不可变记录。`,
      reason: reason.trim(),
      changes: changedFields.map((field) => ({
        field: field.fieldPath,
        before: publishedValues[field.key],
        after: draftValues[field.key].trim(),
      })),
    });
  };

  return (
    <section className="knowledge-revision-workbench" aria-label={`${asset.name} 知识修订工作台`}>
      <header className="knowledge-revision-header">
        <div className="knowledge-revision-title">
          <span className="knowledge-revision-mark"><GitPullRequestArrow size={18} /></span>
          <div><span className="panel-kicker">知识修订草稿 · 基于 {asset.revision}</span><h2>修订 {asset.name}</h2><p>只修改受治理知识；来源资料、已发布 revision 与历史证据保持只读。</p></div>
        </div>
        <div className="knowledge-revision-state"><span><LockKeyhole size={14} />已发布版本受保护</span><strong>{changedFields.length} 项待提交</strong></div>
      </header>

      <div className="knowledge-revision-layout">
        <nav className="knowledge-field-nav" aria-label="待修订知识字段">
          <span className="content-label">结构化知识</span>
          {knowledgeFields.map((field) => {
            const changed = changedFields.some((item) => item.key === field.key);
            return <button key={field.key} type="button" aria-current={activeField === field.key ? "page" : undefined} onClick={() => setActiveField(field.key)}><span><strong>{field.label}</strong><code>{field.fieldPath}</code></span>{changed ? <span className="knowledge-field-changed"><Check size={12} />已修改</span> : <ArrowRight size={13} />}</button>;
          })}
        </nav>

        <main className="knowledge-field-editor">
          <header><div><span className="content-label">候选知识字段</span><h3>{descriptor.label}</h3><p>{descriptor.help}</p></div><code>{descriptor.fieldPath}</code></header>
          <section className="knowledge-published-value" aria-label={`${descriptor.label}当前发布值`}>
            <span><LockKeyhole size={13} />当前发布值 · {asset.revision}</span>
            {descriptor.code ? <pre>{publishedValues[activeField]}</pre> : <p>{publishedValues[activeField] || "当前没有内容"}</p>}
          </section>
          <label className="knowledge-candidate-field">
            <span>候选值</span>
            <textarea className={descriptor.code ? "knowledge-code-input" : undefined} aria-label={`${descriptor.label}候选值`} rows={activeField === "expression" ? 5 : 7} value={draftValues[activeField]} onChange={(event) => updateDraft(event.target.value)} />
          </label>
          <label className="knowledge-revision-reason">
            <span>修订原因 <strong>必填</strong></span>
            <textarea aria-label="知识修订原因" rows={4} placeholder="说明当前知识哪里不准确、适用边界如何变化，以及审核者应重点检查什么。" value={reason} onChange={(event) => { setReason(event.target.value); setChecksRun(false); setError(""); }} />
          </label>
          {error && <div className="knowledge-revision-error" role="alert"><CircleAlert size={15} />{error}</div>}
        </main>

        <aside className="knowledge-revision-inspector" aria-label="证据与检查">
          <section className="knowledge-claim-context">
            <header><div><span className="content-label">定位的 Claim</span><h3>{selectedClaim?.label ?? descriptor.label}</h3></div><StatusPill tone="info">{request.origin === "ask" ? "来自问答" : request.origin === "evidence" ? "来自证据" : "来自知识页"}</StatusPill></header>
            <dl><div><dt>Claim ID</dt><dd><code>{selectedClaim?.claimId ?? "待生成"}</code></dd></div><div><dt>字段路径</dt><dd><code>{descriptor.fieldPath}</code></dd></div></dl>
          </section>
          <section className="knowledge-source-context">
            <header><span className="content-label">只读证据</span><LockKeyhole size={14} /></header>
            <div><span className="knowledge-evidence-mark"><FileCheck2 size={16} /></span><span><strong>{selectedEvidence?.label ?? "当前字段没有直接证据"}</strong><small>{selectedEvidence?.id ?? "发布前需要补充证据"}</small></span></div>
            <dl><div><dt>权威类型</dt><dd>{selectedEvidence?.authority ?? "待确认"}</dd></div><div><dt>来源 revision</dt><dd><code>{selectedArtifact?.sourceRevision ?? selectedEvidence?.source ?? "未关联"}</code></dd></div><div><dt>证据关系</dt><dd>{selectedLink?.polarity === "contradicts" ? "反驳当前主张" : "支持当前主张"}</dd></div></dl>
            <p><BookOpenCheck size={14} />修订只改变候选知识，不会覆盖这份来源或历史证据。</p>
          </section>
          <section className="knowledge-checks">
            <header><span className="content-label">候选预检</span><strong>{checksRun ? "3/3 完成" : "等待运行"}</strong></header>
            {["结构契约", "证据引用", "消费影响"].map((label) => <div key={label} className={checksRun ? "knowledge-check-passed" : undefined}><span>{checksRun ? <CheckCircle2 size={15} /> : <ShieldCheck size={15} />}</span><strong>{label}</strong><small>{checksRun ? label === "消费影响" ? `${asset.consumerBindings.length} 个消费者已纳入审核范围` : "通过" : "待运行"}</small></div>)}
          </section>
        </aside>
      </div>

      <footer className="knowledge-revision-actions">
        <div><strong>{checksRun ? "候选已经具备审核上下文" : changedFields.length > 0 ? `${changedFields.length} 项知识变化尚未检查` : "尚未修改知识"}</strong><span>{checksRun ? "提交后将进入不可变版本审核，不会直接发布。" : "运行检查后才能提交审核。"}</span></div>
        <div><button className="text-button" type="button" onClick={onCancel}>取消修订</button><button className="secondary-button" type="button" onClick={() => onNotify(`${asset.name} 知识草稿已保存在当前原型会话。`)} disabled={changedFields.length === 0}><Save size={15} />保存草稿</button>{checksRun ? <button className="primary-button" type="button" onClick={submit}><GitPullRequestArrow size={15} />提交审核</button> : <button className="primary-button" type="button" onClick={runChecks}><Play size={15} />运行检查</button>}</div>
      </footer>
    </section>
  );
}

function StatusPill({ tone, children }: { tone: "info"; children: string }) {
  return <span className={`knowledge-status-pill knowledge-status-${tone}`}>{children}</span>;
}

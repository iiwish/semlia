import { useContext, useMemo, useState } from "react";
import { CircleAlert, Plus, RefreshCw, X } from "lucide-react";
import { KnowledgeReferenceEditor, KnowledgeWorkspaceContext } from "./KnowledgeReferencePicker";
import { isStale, refresh, refreshVocabularyPins, useCurrentPins, vocabularyReferences } from "./knowledgeVocabulary";
import { knowledgeTypes, type KnowledgeReference, type KnowledgeSpec, type KnowledgeType, type KnowledgeVocabulary } from "./knowledge";

const lineFields = [
  { key: "aliases", label: "别名", help: "每行一个。人们提问时可能使用的同义词、简称或英文名；同类知识之间不能重复，除非声明为易混淆。" },
  { key: "includes", label: "口径包含", help: "每行一项，明确纳入的情况。" },
  { key: "excludes", label: "明确排除", help: "每行一项，不应被理解为这项知识的情况。" },
  { key: "examples", label: "示例", help: "每行一个具体例子，帮助读者和 AI 理解边界。" },
] as const;

type LineField = (typeof lineFields)[number]["key"];

const splitLines = (text: string) => text.split("\n").map((line) => line.trim()).filter(Boolean);
const splitTerms = (text: string) => text.split(/[,，、\n]/).map((term) => term.trim()).filter(Boolean);

// TermsInput edits a comma-separated list, keeping what the author typed until the parsed list changes elsewhere.
function TermsInput({ label, value, onChange }: { label: string; value: string[]; onChange: (value: string[]) => void }) {
  const [text, setText] = useState(value.join("，"));
  if (splitTerms(text).join("\u0000") !== value.join("\u0000")) setText(value.join("，"));
  return <input aria-label={label} value={text} onChange={(event) => { setText(event.target.value); onChange(splitTerms(event.target.value)); }} />;
}

const expectTypes: KnowledgeType[] = ["analysis_model", "metric", "business_object", "business_term"];

function StaleNotice({ stale, onRefresh, label }: { stale: boolean; onRefresh: () => void; label: string }) {
  if (!stale) return null;
  return <span className="vocabulary-stale" role="status"><CircleAlert size={13} />引用的不是当前已发布版本<button type="button" className="text-button" aria-label={`刷新${label}为当前版本`} onClick={onRefresh}><RefreshCw size={13} />刷新为当前版本</button></span>;
}

interface Props {
  type: KnowledgeType;
  value: KnowledgeVocabulary;
  onChange: (value: KnowledgeVocabulary) => void;
  spec: KnowledgeSpec;
  onSpecChange: (spec: KnowledgeSpec) => void;
  workspaceId?: string;
}

export function KnowledgeVocabularyEditor({ type, value, onChange, spec, onSpecChange, workspaceId }: Props) {
  const inheritedWorkspaceId = useContext(KnowledgeWorkspaceContext);
  const workspace = workspaceId ?? inheritedWorkspaceId;
  const [texts, setTexts] = useState<Record<LineField, string>>(() => Object.fromEntries(lineFields.map((field) => [field.key, value[field.key].join("\n")])) as Record<LineField, string>);
  const referenced = useMemo(() => vocabularyReferences(value).map((ref) => ref.assetId).filter(Boolean), [value]);
  const pins = useCurrentPins(workspace, referenced);
  const staleCount = vocabularyReferences(value).filter((ref) => isStale(ref, pins)).length;
  const [expectType, setExpectType] = useState<Record<string, KnowledgeType>>({});
  const members = type === "business_object" ? spec.members ?? [] : [];
  const updateMember = (index: number, patch: Partial<NonNullable<KnowledgeSpec["members"]>[number]>) => {
    const next = members.map((member, position) => {
      if (position !== index) return member;
      const merged = { ...member, ...patch };
      if (!merged.aliases?.length) delete merged.aliases;
      if (!merged.valueAliases?.length) delete merged.valueAliases;
      return merged;
    });
    onSpecChange({ ...spec, members: next });
  };

  return <KnowledgeWorkspaceContext.Provider value={workspace}><section className="vocabulary-editor" aria-label="业务词汇">
    <p className="vocabulary-editor-note">业务词汇只说明人们怎样称呼和区分这项知识，不改变计算；只改词汇不会让引用它的知识失效。发布前会检查别名冲突，并用问数检索重放典型问题。</p>
    {staleCount > 0 && <div className="vocabulary-stale-banner" role="status"><CircleAlert size={15} /><span>{staleCount} 个引用不是当前已发布版本，计算口径变化后旧引用会被发布校验拒绝。</span><button type="button" className="secondary-button" onClick={() => onChange(refreshVocabularyPins(value, pins))}><RefreshCw size={14} />全部刷新为当前版本</button></div>}
    {lineFields.map((field) => <label key={field.key} className="production-field vocabulary-lines"><span>{field.label}</span><small>{field.help}</small><textarea aria-label={`${field.label}（每行一项）`} rows={field.key === "aliases" ? 3 : 4} value={texts[field.key]} onChange={(event) => { const text = event.target.value; setTexts((current) => ({ ...current, [field.key]: text })); onChange({ ...value, [field.key]: splitLines(text) }); }} /></label>)}

    <fieldset className="vocabulary-group"><legend>易混淆知识</legend><small>声明后，共享的说法会按区分说明向提问人澄清，而不是让 AI 猜测。</small>
      {value.confusableWith.map((item, index) => <div className="vocabulary-row" key={index}>
        <KnowledgeReferenceEditor label={`易混淆${knowledgeTypes[type]}`} type={type} value={item.ref.assetId ? item.ref : undefined} onChange={(ref) => onChange({ ...value, confusableWith: value.confusableWith.map((entry, position) => position === index ? { ...entry, ref: ref ?? { assetId: "", revisionId: "", releaseId: "" } } : entry) })} />
        <StaleNotice stale={isStale(item.ref, pins)} label={`易混淆知识 ${index + 1}`} onRefresh={() => onChange({ ...value, confusableWith: value.confusableWith.map((entry, position) => position === index ? { ...entry, ref: refresh(entry.ref, pins) } : entry) })} />
        <label className="production-field"><span>区分说明</span><input aria-label={`易混淆知识 ${index + 1} 区分说明`} value={item.distinction} onChange={(event) => onChange({ ...value, confusableWith: value.confusableWith.map((entry, position) => position === index ? { ...entry, distinction: event.target.value } : entry) })} /></label>
        <button type="button" className="icon-button" aria-label={`移除易混淆知识 ${index + 1}`} title="移除" onClick={() => onChange({ ...value, confusableWith: value.confusableWith.filter((_, position) => position !== index) })}><X size={14} /></button>
      </div>)}
      <button type="button" className="secondary-button" onClick={() => onChange({ ...value, confusableWith: [...value.confusableWith, { ref: { assetId: "", revisionId: "", releaseId: "" }, distinction: "" }] })}><Plus size={14} />添加易混淆知识</button>
    </fieldset>

    <fieldset className="vocabulary-group"><legend>典型问题</legend><small>每个问题都必须能被问数检索召回到这项知识；可以再列出必须一起召回的模型或指标。</small>
      {value.sampleQuestions.map((item, index) => <div className="vocabulary-row" key={index}>
        <label className="production-field"><span>问题</span><input aria-label={`典型问题 ${index + 1}`} value={item.question} onChange={(event) => onChange({ ...value, sampleQuestions: value.sampleQuestions.map((entry, position) => position === index ? { ...entry, question: event.target.value } : entry) })} /></label>
        {(item.expects?.refs ?? []).map((ref, refIndex) => <div key={refIndex} className="vocabulary-expect">
          <label className="production-field"><span>知识类型</span><select aria-label={`期望召回 ${refIndex + 1} 的知识类型`} value={expectType[`${index}-${refIndex}`] ?? "analysis_model"} onChange={(event) => setExpectType((current) => ({ ...current, [`${index}-${refIndex}`]: event.target.value as KnowledgeType }))}>{expectTypes.map((option) => <option key={option} value={option}>{knowledgeTypes[option]}</option>)}</select></label>
          <KnowledgeReferenceEditor label={`期望召回 ${refIndex + 1}`} type={expectType[`${index}-${refIndex}`] ?? "analysis_model"} value={ref.assetId ? ref : undefined} onChange={(next) => {
            const refs = (item.expects?.refs ?? []).map((entry, position) => position === refIndex ? next : entry).filter((entry): entry is KnowledgeReference => !!entry);
            onChange({ ...value, sampleQuestions: value.sampleQuestions.map((entry, position) => position === index ? (refs.length ? { ...entry, expects: { refs } } : { question: entry.question }) : entry) });
          }} />
          <StaleNotice stale={isStale(ref, pins)} label={`典型问题 ${index + 1} 的期望召回 ${refIndex + 1}`} onRefresh={() => onChange({ ...value, sampleQuestions: value.sampleQuestions.map((entry, position) => position === index && entry.expects ? { ...entry, expects: { refs: entry.expects.refs.map((current, at) => at === refIndex ? refresh(current, pins) : current) } } : entry) })} />
        </div>)}
        <div className="vocabulary-row-actions">
          <button type="button" className="text-button" onClick={() => onChange({ ...value, sampleQuestions: value.sampleQuestions.map((entry, position) => position === index ? { ...entry, expects: { refs: [...(entry.expects?.refs ?? []), { assetId: "", revisionId: "", releaseId: "" }] } } : entry) })}><Plus size={13} />添加期望召回的知识</button>
          <button type="button" className="icon-button" aria-label={`移除典型问题 ${index + 1}`} title="移除" onClick={() => onChange({ ...value, sampleQuestions: value.sampleQuestions.filter((_, position) => position !== index) })}><X size={14} /></button>
        </div>
      </div>)}
      <button type="button" className="secondary-button" onClick={() => onChange({ ...value, sampleQuestions: [...value.sampleQuestions, { question: "" }] })}><Plus size={14} />添加典型问题</button>
    </fieldset>

    {members.length > 0 && <fieldset className="vocabulary-group"><legend>属性别名与取值别名</legend><small>属性别名用逗号分隔；取值别名把“江浙沪”这类说法对应到数据里的规范取值，仅适用于文本属性。</small>
      {members.map((member, index) => <div className="vocabulary-member" key={member.id}>
        <strong>{member.name || member.id}<code>{member.id}</code></strong>
        <label className="production-field"><span>属性别名</span><TermsInput label={`${member.name || member.id} 属性别名`} value={member.aliases ?? []} onChange={(aliases) => updateMember(index, { aliases })} /></label>
        {(!member.valueType || member.valueType === "string") && <>
          {(member.valueAliases ?? []).map((entry, entryIndex) => <div className="vocabulary-value" key={entryIndex}>
            <label className="production-field"><span>规范取值</span><input aria-label={`${member.name || member.id} 规范取值 ${entryIndex + 1}`} value={entry.value} onChange={(event) => updateMember(index, { valueAliases: (member.valueAliases ?? []).map((item, at) => at === entryIndex ? { ...item, value: event.target.value } : item) })} /></label>
            <label className="production-field"><span>对应说法</span><TermsInput label={`${member.name || member.id} 规范取值 ${entryIndex + 1} 的说法`} value={entry.aliases} onChange={(aliases) => updateMember(index, { valueAliases: (member.valueAliases ?? []).map((item, at) => at === entryIndex ? { ...item, aliases } : item) })} /></label>
            <button type="button" className="icon-button" aria-label={`移除 ${member.name || member.id} 规范取值 ${entryIndex + 1}`} title="移除" onClick={() => updateMember(index, { valueAliases: (member.valueAliases ?? []).filter((_, at) => at !== entryIndex) })}><X size={14} /></button>
          </div>)}
          <button type="button" className="text-button" onClick={() => updateMember(index, { valueAliases: [...(member.valueAliases ?? []), { value: "", aliases: [] }] })}><Plus size={13} />添加取值别名</button>
        </>}
      </div>)}
    </fieldset>}
  </section></KnowledgeWorkspaceContext.Provider>;
}

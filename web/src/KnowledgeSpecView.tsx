import { expressionLabel, knowledgeFieldLabels, referenceLabel, type KnowledgeExpression, type KnowledgeReference, type KnowledgeSpec } from "./knowledge";

const labels: Record<string, string> = { aggregate: "基础聚合", derived: "派生计算", definition: "仅解释定义", predicate: "定义与判定", object: "对象行级", required: "不允许为空", unknown: "未知", excluded: "排除", exclude: "排除空值", unknown_does_not_match: "未知值不匹配", current_value: "当前值", stable: "稳定身份", event_time: "事件时值", null: "返回空值", recompute_from_inputs: "从基础指标重新计算" };
function format(value: unknown): string {
  if (value === undefined || value === null || value === "") return "待定义";
  if (typeof value === "string") return labels[value] ?? value;
  if (Array.isArray(value)) return value.length ? value.map(format).join("；") : "未声明";
  if (typeof value === "object") {
    if ("assetId" in value) return referenceLabel(value as KnowledgeReference);
    if ("op" in value) return expressionLabel(value as KnowledgeExpression);
    if ("objectId" in value && "revisionId" in value) return `${value.objectId} @ ${value.revisionId}`;
    if ("semanticRef" in value && "dataRef" in value) return `${format(value.semanticRef)} → ${format(value.dataRef)}`;
    if ("name" in value && "type" in value) return `${value.name} (${value.type})`;
  }
  return String(value);
}

export function KnowledgeSpecView({ spec }: { spec?: KnowledgeSpec }) {
  if (!spec || !Object.keys(spec).length) return <p className="empty-inline">类型专属结构尚未定义</p>;
  return <div className="knowledge-spec-view"><dl>{Object.entries(spec).filter(([key]) => key !== "members").map(([key, value]) => <div key={key}><dt>{knowledgeFieldLabels[key] ?? key}</dt><dd>{format(value)}</dd></div>)}</dl>{spec.members && <section><h4>成员 · {spec.members.length}</h4><div className="knowledge-members-table"><table><thead><tr><th>成员</th><th>业务含义</th><th>值类型 / 来源字段</th><th>空值与历史</th><th>键</th></tr></thead><tbody>{spec.members.map((member) => <tr key={member.id}><td><code>{member.id}</code></td><td>{member.name ?? "待定义"}</td><td>{format(member.sourceFieldRef ?? member.valueType)}</td><td>{[member.nullPolicy, member.historyPolicy].filter(Boolean).map(format).join(" · ") || "待定义"}</td><td>{spec.keys?.includes(member.id) ? "唯一键" : ""}</td></tr>)}</tbody></table></div></section>}</div>;
}

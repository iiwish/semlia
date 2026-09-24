import { useContext, useEffect, useMemo } from "react";
import { CatalogRuntimeContext } from "./catalogRuntime";
import { knowledgeFieldLabels, knowledgeReferences, type KnowledgeExpression, type KnowledgeReference, type KnowledgeSpec } from "./knowledge";
import type { AssetType } from "./types";

const labels: Record<string, string> = {
  aggregate: "基础聚合", derived: "派生计算", definition: "仅解释定义", predicate: "定义与判定", object: "对象行级",
  required: "不允许为空", unknown: "未知", excluded: "排除", exclude: "排除空值", unknown_does_not_match: "未知值不匹配",
  current_value: "当前值", stable: "稳定身份", event_time: "事件时值", null: "返回空值", recompute_from_inputs: "从基础指标重新计算",
  string: "文本", number: "数值", integer: "整数", boolean: "布尔值", date: "日期", timestamp: "时间",
  sum: "求和", avg: "平均值", count: "计数", count_distinct: "去重计数", min: "最小值", max: "最大值",
};
const fields: Record<AssetType, (keyof KnowledgeSpec)[]> = {
  业务对象: ["grain", "keys", "identityPolicy", "lifecycle"],
  业务口径: ["subjectRef", "capability", "predicate", "parameters", "stage", "nullPolicy"],
  指标: ["kind", "inputRef", "aggregation", "expression", "filterRefs", "timeAttributeRef", "unit", "nullPolicy", "zeroDenominator", "rollup"],
  数据资产: ["coverage", "grain", "keys", "refreshFrequency", "sensitivity", "datasetRef"],
  分析模型: ["baseObjectRef", "grain", "defaultTimeAttributeRef", "publicAttributeRefs", "metricRefs", "compatibleTermRefs", "dataAssetRefs"],
};
const hasValue = (value: unknown) => value !== undefined && value !== null && value !== "" && (!Array.isArray(value) || value.length > 0);

export function KnowledgeSpecView({ spec, type }: { spec?: KnowledgeSpec; type: AssetType }) {
  const runtime = useContext(CatalogRuntimeContext);
  const refs = useMemo(() => knowledgeReferences(spec), [spec]);
  const ensureAsset = runtime?.ensureAsset;
  const detailStates = runtime?.detailStates;
  useEffect(() => {
    if (ensureAsset) for (const id of new Set(refs.map((ref) => ref.assetId))) {
      if (!detailStates?.[id] || detailStates[id].state === "idle") void ensureAsset(id);
    }
  }, [detailStates, ensureAsset, refs]);
  if (!spec || !Object.keys(spec).length) return <p className="empty-inline">尚未定义{type}内容</p>;
  const reference = (ref: KnowledgeReference): string => {
    const asset = runtime?.assets.find((item) => item.id === ref.assetId && item.revisionRecord.revisionId === ref.revisionId);
    if (!asset) return runtime?.detailStates[ref.assetId]?.state === "error" ? "关联知识（读取失败）" : "关联知识（版本未加载）";
    const member = ref.memberId ? asset.knowledgeSpec?.members?.find((item) => item.id === ref.memberId) : undefined;
    return `${asset.name}${ref.memberId ? ` · ${member?.name || "属性名称未加载"}` : ""}`;
  };
  const expression = (value: KnowledgeExpression): string => {
    if (value.op === "ref") return value.ref ? reference(value.ref) : "引用未定义";
    if (value.op === "parameter") return `$${value.parameter ?? "未定义参数"}`;
    if (value.op === "literal") return JSON.stringify(value.value);
    const symbol: Record<string, string> = { add: "+", subtract: "−", multiply: "×", divide: "÷", eq: "=", neq: "≠", lt: "<", lte: "≤", gt: ">", gte: "≥", and: "且", or: "或" };
    return `(${value.left ? expression(value.left) : "未定义"} ${symbol[value.op] ?? value.op} ${value.right ? expression(value.right) : "未定义"})`;
  };
  const format = (value: unknown): string => {
    if (!hasValue(value)) return "未定义";
    if (typeof value === "string") return labels[value] ?? value;
    if (Array.isArray(value)) return value.map(format).join("；");
    if (typeof value === "object" && value) {
      if ("assetId" in value) return reference(value as KnowledgeReference);
      if ("op" in value) return expression(value as KnowledgeExpression);
      if ("objectId" in value) return "已关联来源版本";
      if ("name" in value && "type" in value) return `${value.name}（${format(value.type)}${"required" in value && value.required ? "，必填" : "，可选"}）`;
    }
    return String(value);
  };
  const members = spec.members ?? [];
  const dataAsset = type === "数据资产";
  return <div className={`knowledge-spec-view knowledge-spec-${dataAsset ? "data" : "semantic"}`}>
    <dl>{fields[type].filter((key) => hasValue(spec[key])).map((key) => <div key={key}>
      <dt>{key === "keys" ? "唯一标识" : knowledgeFieldLabels[key]}</dt>
      <dd>{key === "keys" ? spec.keys?.map((id) => members.find((member) => member.id === id)?.name || (dataAsset ? id : "未命名属性")).join(" + ") : format(spec[key])}</dd>
    </div>)}</dl>
    {(type === "业务对象" || dataAsset) && <section aria-label={dataAsset ? "数据字段" : "业务属性"}>
      <h4>{dataAsset ? "数据字段" : "业务属性"} <span className="knowledge-count">{members.length}</span></h4>
      {members.length ? <div className="knowledge-members-table"><table><thead><tr>
        {dataAsset && <th>字段标识</th>}<th>{dataAsset ? "字段含义" : "属性"}</th><th>{dataAsset ? "来源关联" : "值类型"}</th><th>空值规则</th>{!dataAsset && <th>历史语义</th>}<th>唯一标识</th>
      </tr></thead><tbody>{members.map((member) => <tr key={member.id}>
        {dataAsset && <td><code>{member.id}</code></td>}<td>{member.name || "未命名"}</td>
        <td>{dataAsset ? member.sourceFieldRef ? "已关联来源字段" : "未关联" : format(member.valueType)}</td>
        <td>{format(member.nullPolicy)}</td>{!dataAsset && <td>{format(member.historyPolicy)}</td>}
        <td>{spec.keys ? spec.keys.includes(member.id) ? "是" : "否" : "未定义"}</td>
      </tr>)}</tbody></table></div> : <p className="empty-inline">尚未定义{dataAsset ? "数据字段" : "业务属性"}</p>}
    </section>}
    {type === "分析模型" && Boolean(spec.memberBindings?.length) && <section aria-label="属性映射"><h4>属性映射</h4><div className="knowledge-members-table"><table><thead><tr><th>业务属性</th><th>数据资产字段</th></tr></thead><tbody>{spec.memberBindings?.map((binding, index) => <tr key={index}><td>{reference(binding.semanticRef)}</td><td>{reference(binding.dataRef)}</td></tr>)}</tbody></table></div></section>}
    <details className="production-technical knowledge-technical"><summary>技术标识与版本引用</summary><pre>{JSON.stringify(spec, null, 2)}</pre></details>
  </div>;
}

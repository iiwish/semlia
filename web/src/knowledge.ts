import type { components } from "@semlia/sdk-typescript";

export type KnowledgeSpec = NonNullable<components["schemas"]["KnowledgeSpec"]>;
export type KnowledgeReference = components["schemas"]["KnowledgeReference"];
export type KnowledgeExpression = components["schemas"]["KnowledgeExpression"];
export type KnowledgeType = components["schemas"]["SemanticAssetType"];

export const knowledgeTypes = {
  business_object: "业务对象",
  business_term: "业务口径",
  metric: "指标",
  data_asset: "数据资产",
  analysis_model: "分析模型",
} as const;

export function initialKnowledgeSpec(type: KnowledgeType): KnowledgeSpec {
  switch (type) {
    case "business_object": return { members: [], keys: [] };
    case "business_term": return { capability: "definition" };
    case "metric": return { kind: "aggregate", nullPolicy: "exclude" };
    case "data_asset": return { members: [], keys: [] };
    case "analysis_model": return { publicAttributeRefs: [], metricRefs: [], compatibleTermRefs: [], dataAssetRefs: [], memberBindings: [], joinContractIds: [] };
  }
}

export const knowledgeFieldLabels: Record<string, string> = {
  grain: "业务粒度", keys: "唯一键成员", identityPolicy: "身份合并策略", lifecycle: "生命周期",
  members: "属性与字段成员", capability: "口径能力", subjectRef: "适用业务对象", predicate: "判定条件",
  parameters: "查询参数", stage: "求值阶段", kind: "计算类型", inputRef: "输入属性", aggregation: "聚合函数",
  expression: "派生计算", filterRefs: "固定业务口径", timeAttributeRef: "统计时间属性", unit: "计量单位",
  nullPolicy: "空值处理", zeroDenominator: "零分母", rollup: "跨组汇总", datasetRef: "来源数据集版本",
  coverage: "覆盖范围", refreshFrequency: "刷新周期", sensitivity: "敏感级别", baseObjectRef: "基础业务对象",
  publicAttributeRefs: "公开属性", metricRefs: "可用指标", compatibleTermRefs: "兼容业务口径",
  defaultTimeAttributeRef: "默认时间属性", dataAssetRefs: "数据资产版本", memberBindings: "成员映射", joinContractIds: "已发布连接契约",
};

export function referenceLabel(ref: KnowledgeReference) {
  return `${ref.assetId}${ref.memberId ? `.${ref.memberId}` : ""} @ ${ref.revisionId}`;
}

export function knowledgeReferences(spec?: KnowledgeSpec | null): KnowledgeReference[] {
  const refs = new Map<string, KnowledgeReference>();
  const visit = (value: unknown) => {
    if (!value || typeof value !== "object") return;
    if (Array.isArray(value)) { value.forEach(visit); return; }
    const object = value as Record<string, unknown>;
    if (typeof object.assetId === "string" && typeof object.revisionId === "string" && typeof object.releaseId === "string") {
      const ref = object as unknown as KnowledgeReference;
      refs.set(`${ref.assetId}:${ref.revisionId}:${ref.releaseId}`, ref);
    } else Object.values(object).forEach(visit);
  };
  visit(spec);
  return [...refs.values()];
}

export function expressionLabel(expression?: KnowledgeExpression): string {
  if (!expression) return "待定义";
  if (expression.op === "ref") return expression.ref ? referenceLabel(expression.ref) : "待选择引用";
  if (expression.op === "parameter") return `$${expression.parameter ?? "待定义"}`;
  if (expression.op === "literal") return JSON.stringify(expression.value);
  const symbol: Record<string, string> = { add: "+", subtract: "-", multiply: "×", divide: "÷", eq: "=", neq: "≠", lt: "<", lte: "≤", gt: ">", gte: "≥", and: "且", or: "或" };
  return `(${expressionLabel(expression.left)} ${symbol[expression.op] ?? expression.op} ${expressionLabel(expression.right)})`;
}

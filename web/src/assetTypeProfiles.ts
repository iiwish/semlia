import type { Asset, AssetType } from "./types";

export type AssetImplementationMode = "required" | "optional" | "not_applicable";
export type AssetRuleSeverity = "blocker" | "warning";
export type AssetRuleState = "passed" | "warning" | "failed" | "not_applicable";
export type AssetEmptyStateKind = "missing" | "optional" | "not_applicable";
export interface AssetEmptyStateConfig { kind: AssetEmptyStateKind; title: string; detail: string; action?: string }
export interface AssetTypeValidationResult { id: string; label: string; severity: AssetRuleSeverity; description: string; remediation: string; state: AssetRuleState; detail: string }
export interface AssetTypeProfile {
  type: AssetType;
  definitionTitle: string;
  definitionFocus: string;
  ontologyFocus: string;
  implementationFocus: string;
  trustFocus: string;
  deliveryFocus: string;
  requiredFields: string[];
  implementationMode: AssetImplementationMode;
  emptyStates: Record<"relations" | "bindings" | "joins" | "evidence" | "consumers", AssetEmptyStateConfig>;
  validationRules: AssetTypeValidationResult[];
}

const definitions: Record<AssetType, { title: string; fields: string[]; implementation: AssetImplementationMode }> = {
  业务对象: { title: "身份、属性与生命周期", fields: ["业务粒度", "唯一键成员", "身份合并策略", "生命周期", "属性成员"], implementation: "optional" },
  业务口径: { title: "业务边界与判定条件", fields: ["口径能力", "适用对象", "判定条件", "查询参数", "空值处理"], implementation: "optional" },
  指标: { title: "聚合与派生计算", fields: ["计算类型", "输入属性或指标", "统计时间", "单位", "空值与汇总"], implementation: "required" },
  数据资产: { title: "来源与使用契约", fields: ["来源数据集版本", "字段成员", "数据粒度", "唯一键", "覆盖与刷新"], implementation: "required" },
  分析模型: { title: "公开成员与数据实现", fields: ["基础对象", "业务粒度", "公开属性与指标", "成员映射", "连接契约"], implementation: "required" },
};

export const assetTypeProfiles = Object.fromEntries(Object.entries(definitions).map(([type, value]) => [type, {
  type, definitionTitle: value.title, definitionFocus: value.fields.join("、"), ontologyFocus: "检查版本固定的知识依赖与成员引用。",
  implementationFocus: "检查来源、成员映射和已发布连接契约。", trustFocus: "以业务确认与实际验证结果为准。", deliveryFocus: "仅消费已发布且授权的知识版本。",
  requiredFields: value.fields, implementationMode: value.implementation, validationRules: [],
  emptyStates: {
    relations: { kind: "optional", title: "尚无知识关系", detail: "未登记独立关系。类型结构中的版本引用单独展示。" },
    bindings: { kind: value.implementation === "required" ? "missing" : "optional", title: "尚无已验证实现", detail: "知识定义不代表来源映射已通过验证。" },
    joins: { kind: "optional", title: "尚无连接契约", detail: "单数据集不要求连接；跨数据集执行需要已发布安全路径。" },
    evidence: { kind: "missing", title: "缺少确认依据", detail: "业务确认、来源证据和验证结果各自独立。" },
    consumers: { kind: "optional", title: "尚无使用方", detail: "未登记消费绑定。" },
  },
}])) as unknown as Record<AssetType, AssetTypeProfile>;

export function assetTypeProfileFor(asset: Asset): AssetTypeProfile { return assetTypeProfiles[asset.type]; }
export function evaluateAssetTypeRules(asset: Asset): AssetTypeValidationResult[] {
  return asset.readiness.map((gate) => ({ id: gate.id, label: gate.label, severity: "blocker", description: gate.detail, remediation: "补齐对应知识并重新验证。", state: gate.state === "warning" ? "failed" : gate.state, detail: gate.detail }));
}

const findings: Record<string, string> = {
  PRODUCTION_DEFINITION_UNRESOLVED: "补充业务定义：说明这项知识代表什么，以及采用的计算口径。",
  PRODUCTION_SCOPE_UNRESOLVED: "补充适用范围：说明适用的业务、时间范围及排除条件。",
  PRODUCTION_BUSINESS_RULE_UNCONFIRMED: "请由有权限的负责人确认业务口径；AI 建议不能代替确认。",
  RISK_BLOCKER: "存在未解决的问题，修正并重新检查后才能继续审核。",
  PRODUCTION_SET_REQUIRED: "这项知识属于关联变更，请在知识确认中处理整组内容。",
  INPUT_STALE: "来源已变化，请重新核对来源与知识内容。",
  INPUT_INCOMPLETE: "来源信息不完整，请补齐来源后重新检查。",
  EVIDENCE_MISSING: "缺少来源依据，请补充可验证的证据。",
  SCHEMA_COMPLETED: "知识内容检查已完成。",
  REFERENCE_COMPLETED: "来源与引用检查已完成。",
  STRUCTURAL_COMPLETED: "数据结构检查已完成。",
  POLICY_COMPLETED: "发布规则检查已完成。",
};

export function findingMessage(code: string, severity = "blocker"): string {
  return findings[code] ?? (severity === "blocker" ? "检查未通过，请展开技术详情并联系管理员。" : severity === "warning" ? "存在需要核对的信息，请展开技术详情。" : "此项检查已完成。详情保留在技术记录中。");
}

export function validatorLabel(id: string): string {
  return ({ schema: "知识内容检查", reference: "来源与引用检查", structural: "数据结构检查", policy: "发布规则检查" } as Record<string, string>)[id] ?? "知识检查";
}

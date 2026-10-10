const findings: Record<string, string> = {
  PRODUCTION_DEFINITION_UNRESOLVED: "补充业务定义：说明这项知识代表什么，以及采用的计算口径。",
  PRODUCTION_SCOPE_UNRESOLVED: "补充适用范围：说明适用的业务、时间范围及排除条件。",
  PRODUCTION_BUSINESS_RULE_UNCONFIRMED: "请由有权限的负责人确认业务口径；AI 建议不能代替确认。",
  RISK_BLOCKER: "存在未解决的问题，修正并重新检查后才能继续审核。",
  PRODUCTION_VOCABULARY_CONFLICT: "别名冲突：同一个说法会指向两个同类知识。删去其中一个别名，或在任一方声明易混淆并写明区别。",
  PRODUCTION_SAMPLE_QUESTION_NOT_RECALLED: "典型问题检索不到这项知识：补充问题里的说法作为别名，或改写问题。",
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

// These findings carry server text naming only the contested word or the author's own question; show it verbatim.
const specificFindings = new Set(["PRODUCTION_VOCABULARY_CONFLICT", "PRODUCTION_SAMPLE_QUESTION_NOT_RECALLED", "PRODUCTION_KNOWLEDGE_INCOMPLETE"]);

export function findingDetail(finding: { code: string; severity?: string; message?: string }): string {
  return specificFindings.has(finding.code) && finding.message?.trim() ? finding.message : findingMessage(finding.code, finding.severity);
}

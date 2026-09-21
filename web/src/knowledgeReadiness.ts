import type { AssetAuthoritySection, AssetReadinessGate } from "./types";
import type { KnowledgeType } from "./knowledge";

export function knowledgeReadiness({ assetType, revisionId, content, sections = [] }: { assetType: KnowledgeType; revisionId?: string; content: Record<string, unknown>; sections?: AssetAuthoritySection[] }): AssetReadinessGate[] {
  const section = (kind: AssetAuthoritySection["kind"]) => sections.find((item) => item.kind === kind);
  const count = (kind: AssetAuthoritySection["kind"], field: string) => {
    const value = section(kind);
    return value?.availability === "available" && typeof value.values[field] === "number" ? value.values[field] : undefined;
  };
  const gate = (id: AssetReadinessGate["id"], label: string, passed: boolean, detail: string): AssetReadinessGate => ({ id, label, state: passed ? "passed" : "warning", detail });
  const validation = section("validation");
  const currentValidation = Boolean(revisionId && validation?.revisionId === revisionId);
  const runCount = count("validation", "runCount") ?? 0;
  const runs = validation?.records.filter((record) => record.kind === "validation_run") ?? [];
  const validated = currentValidation && runCount > 0 && runs.length === runCount && validation?.recordsPage.total === runCount && !validation.recordsPage.nextCursor && runs.every((run) => run.status === "succeeded") && count("validation", "blockerCount") === 0 && count("validation", "warningCount") === 0;
  const released = section("released_state");
  const published = released?.availability === "available" && Boolean(revisionId && released.revisionId === revisionId);
  const filled = (value: unknown) => typeof value === "string" && value.trim().length > 0;
  const spec = content.spec && typeof content.spec === "object" ? content.spec as Record<string, unknown> : undefined;
  const hasDefinition = filled(content.definition) && filled(content.scope) && Boolean(spec && Object.keys(spec).length);
  const explanationOnly = assetType === "business_term" && spec?.capability === "definition";
  return [
    gate("ownership", "责任归属", filled(content.ownerPrincipalId), filled(content.ownerPrincipalId) ? String(content.ownerPrincipalId) : "尚未指定业务负责人。"),
    gate("definition", "定义与结构", hasDefinition, hasDefinition ? "定义、范围和类型结构已填写，完整性以发布验证为准。" : "定义、范围或类型专属结构尚未填写。"),
    gate("evidence", "确认依据", (count("evidence", "evidenceCount") ?? 0) > 0, "证据记录不替代业务确认；集合发布仍需有效人工确认。"),
    explanationOnly ? { id: "mapping", label: "物理实现", state: "not_applicable", detail: "该口径仅供解释，不具备查询判定能力。" } : gate("mapping", "物理实现", published && validated && (count("physical_bindings", "bindingCount") ?? 0) > 0, "仅统计已发布且经过验证的实现；可读取分区不代表存在绑定。"),
    gate("validation", "当前版本验证", validated, validated ? "当前修订已有完整验证，未返回阻断或提醒。" : "当前修订缺少完整通过的验证；历史版本结果不适用于当前草稿。"),
    gate("compatibility", "当前版本发布", published, published ? "当前修订已包含在发布版本中。" : "当前知识尚未发布，不能用于正式问数。"),
  ];
}

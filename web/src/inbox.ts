import type { OperationSummary } from "./semanticProduction";
import type { WorkbenchAttentionItem } from "./workbench";

export type InboxScope = "pending" | "initiated" | "done";
export type InboxOperation = OperationSummary & { superseded?: boolean; proposalIds?: string[] };
export type InboxRow = { id: string; title: string; reason: string; action: string; group: string; state: string; updatedAt: string; operation?: InboxOperation; task?: WorkbenchAttentionItem };

const knowledgeActions: Record<string, [string, string, string]> = {
  draft: ["继续整理", "补充定义、适用范围并核对业务口径", "待补充"],
  needs_correction: ["补充与纠正", "检查未通过，需要补充或纠正知识", "待纠正"],
  rejected: ["补充与纠正", "审核未通过，需要根据审核意见纠正", "待纠正"],
  in_review: ["独立审核", "核对整组知识、来源依据与检查结果", "待审核"],
  ready_to_publish: ["确认发布", "审核已通过，等待有权限的成员发布", "待发布"],
  released: ["查看知识", "已发布的知识记录", "已发布"],
  no_change: ["查看记录", "与现有知识一致，无需发布", "无变化"],
};
const taskKinds: Record<WorkbenchAttentionItem["kind"], [string, string]> = {
  review: ["知识处理", "进入审核"], validation: ["知识处理", "检查与纠正"],
  source: ["接入异常", "检查来源"], runtime: ["运行异常", "查看诊断"], compatibility: ["问数问题", "检查使用约束"],
};
const taskStates: Record<string, string> = { open: "待处理", in_progress: "处理中", resolved: "已解决", dismissed: "已忽略" };

export function buildInbox(operations: InboxOperation[], tasks: WorkbenchAttentionItem[], scope: InboxScope): InboxRow[] {
  const rows: InboxRow[] = [];
  const proposalIds = new Set(operations.flatMap((operation) => operation.proposalIds ?? []));
  for (const operation of new Map(operations.map((item) => [item.id, item])).values()) {
    const complete = Boolean(operation.superseded) || operation.progress === "released" || operation.progress === "no_change";
    if (scope === "done" ? !complete : scope === "pending" && complete) continue;
    const info = operation.superseded ? ["查看记录", "由后继纠正记录继续处理", "已替代"] : knowledgeActions[operation.progress];
    if (!info) continue;
    rows.push({ id: `knowledge:${operation.id}`, title: operation.title || "未命名知识", reason: info[1], action: info[0], state: info[2], group: "知识处理", updatedAt: operation.updatedAt, operation });
  }
  for (const task of new Map(tasks.map((item) => [item.id, item])).values()) {
    const complete = task.state === "resolved" || task.state === "dismissed";
    if (scope === "done" ? !complete : scope === "pending" && complete) continue;
    const target = new URL(task.targetRoute, "http://semlia.local");
    if (target.pathname === "/governance" && proposalIds.has(target.searchParams.get("proposal") ?? "")) continue;
    const [group, action] = taskKinds[task.kind];
    rows.push({ id: `task:${task.id}`, title: task.title, reason: task.summary, action: complete ? "查看记录" : action, state: taskStates[task.state], group, updatedAt: task.updatedAt, task });
  }
  return rows.sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
}

// Empty authorized pages may still have a cursor. Never mistake one for completion.
export async function readInboxPages<T>(read: (cursor?: string) => Promise<{ items: T[]; nextCursor?: string | null }>, signal?: AbortSignal): Promise<T[]> {
  const items: T[] = [];
  const seen = new Set<string>();
  let cursor: string | undefined;
  for (let page = 0; page < 100; page++) {
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    const result = await read(cursor);
    items.push(...result.items);
    if (!result.nextCursor) return items;
    if (seen.has(result.nextCursor)) throw new Error("分页游标重复，待办未完整加载，请重试。");
    cursor = result.nextCursor;
    seen.add(cursor);
  }
  throw new Error("待办超过本次读取范围，请缩小范围后重试；不能确认是否还有待处理事项。");
}

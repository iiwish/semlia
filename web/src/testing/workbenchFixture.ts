import { WorkbenchApiError, type WorkbenchApi, type WorkbenchAttentionItem, type WorkbenchAttentionCounts } from "../workbench";

export function createWorkbenchFixtureApi(): WorkbenchApi {
  let records = workbenchFixtureItems.map((item) => ({ ...item, nextActions: [...item.nextActions] }));
  return {
    async listItems(_workspaceId, filter, cursor) {
      let items = records.filter((item) => filter.view === "team"
        || filter.view === "mine" && item.assigneePrincipalId === "prn_01arz3ndektsv4rrffq69g5fav"
        || filter.view === "initiated" && item.initiatorPrincipalId === "prn_01arz3ndektsv4rrffq69g5fav");
      const search = filter.search?.trim().toLowerCase();
      if (search) items = items.filter((item) => `${item.title} ${item.summary} ${item.id} ${item.targetId} ${item.reasonCode}`.toLowerCase().includes(search));
      if (filter.kind) items = items.filter((item) => item.kind === filter.kind);
      if (filter.state) items = items.filter((item) => item.state === filter.state);
      if (filter.priority) items = items.filter((item) => item.priority === filter.priority);
      if (filter.risk) items = items.filter((item) => item.risk === filter.risk);
      items = [...items].sort((left, right) => filter.sort === "updated_desc"
        ? right.updatedAt.localeCompare(left.updatedAt)
        : filter.sort === "due_asc"
          ? (left.dueAt ?? "9999").localeCompare(right.dueAt ?? "9999")
          : priorityRank(left.priority) - priorityRank(right.priority) || right.updatedAt.localeCompare(left.updatedAt));
      const start = cursor ? Number(cursor) : 0;
      const limit = filter.limit ?? 50;
      const pageItems = items.slice(start, start + limit);
      return {
        items: pageItems,
        counts: countsFor(items),
        page: { limit, nextCursor: start + limit < items.length ? String(start + limit) : undefined },
      };
    },
    async getItem(_workspaceId, attentionItemId) {
      const item = records.find((candidate) => candidate.id === attentionItemId);
      if (!item) throw new WorkbenchApiError("待办不存在或当前主体不可见。", 404, "NOT_FOUND");
      return { ...item, nextActions: [...item.nextActions] };
    },
    async updateItem(_workspaceId, attentionItemId, body) {
      const index = records.findIndex((candidate) => candidate.id === attentionItemId);
      if (index < 0) throw new WorkbenchApiError("待办不存在或当前主体不可见。", 404, "NOT_FOUND");
      const item = records[index];
      if (body.expectedVersion !== item.version) throw new WorkbenchApiError("待办已被其他操作者更新，请重新读取。", 409, "CONFLICT");
      const updated: WorkbenchAttentionItem = {
        ...item,
        state: body.state ?? item.state,
        assigneePrincipalId: body.setAssignee ? body.assigneePrincipalId ?? null : item.assigneePrincipalId,
        version: item.version + 1,
        updatedAt: new Date().toISOString(),
      };
      records = [...records.slice(0, index), updated, ...records.slice(index + 1)];
      return { ...updated, nextActions: [...updated.nextActions] };
    },
  };
}

function countsFor(items: WorkbenchAttentionItem[]): WorkbenchAttentionCounts {
  return {
    total: items.length,
    open: items.filter((item) => item.state === "open").length,
    inProgress: items.filter((item) => item.state === "in_progress").length,
    critical: items.filter((item) => item.priority === "critical").length,
  };
}

function priorityRank(value: WorkbenchAttentionItem["priority"]): number {
  return ({ critical: 0, high: 1, medium: 2, low: 3 } as const)[value];
}

const workbenchFixtureItems: WorkbenchAttentionItem[] = [
  fixtureItem({ id: "ati_01arz3ndektsv4rrffq69g5fav", title: "旧区域别名无法安全废弃", summary: "3 个消费者仍引用旧区域别名，需要确认迁移。", kind: "compatibility", priority: "critical", risk: "critical", targetType: "asset", targetId: "asset_01arz3ndektsv4rrffq69g5fav", targetRoute: "/delivery/compatibility?consumer=csm_01arz3ndektsv4rrffq69g5fav&binding=cbd_01arz3ndektsv4rrffq69g5fav&query=smq_01arz3ndektsv4rrffq69g5fav", reasonCode: "SEMANTIC_QUERY_REFUSED", openedAt: "2026-08-23T08:00:00Z", updatedAt: "2026-08-24T15:00:00Z", dueAt: "2026-08-24T13:00:00Z", nextActions: ["open_target", "publish", "assign", "dismiss"] }),
  fixtureItem({ id: "ati_01arz3ndektsv4rrffq69g5faw", title: "确认客单价退款订单口径", summary: "候选资产版本等待独立审核。", kind: "review", priority: "high", risk: "medium", targetType: "asset", targetId: "prp_01arz3ndektsv4rrffq69g5fav", targetRoute: "/governance?proposal=PROP-128", reasonCode: "PROPOSAL_REVIEW_REQUIRED", openedAt: "2026-08-24T14:20:00Z", updatedAt: "2026-08-24T14:20:00Z", nextActions: ["open_target", "review", "assign", "dismiss"] }),
  fixtureItem({ id: "ati_01arz3ndektsv4rrffq69g5fax", title: "检查客户增长域运行异常", summary: "数据来源发现运行失败，需要查看服务端运行记录。", kind: "source", priority: "high", risk: "medium", targetType: "source", targetId: "src_01arz3ndektsv4rrffq69g5fav", targetRoute: "/sources?source=src_01arz3ndektsv4rrffq69g5fav", reasonCode: "DISCOVERY_FAILED", openedAt: "2026-08-23T18:44:00Z", updatedAt: "2026-08-24T10:00:00Z", dueAt: "2026-08-24T23:59:59Z", nextActions: ["open_target", "manage_source", "assign", "dismiss"] }),
  fixtureItem({ id: "ati_01arz3ndektsv4rrffq69g5fay", title: "补充净收入财务口径证据", summary: "候选资产版本由当前主体发起，等待财务负责人审核。", kind: "review", priority: "low", risk: "low", targetType: "asset", targetId: "prp_01arz3ndektsv4rrffq69g5faw", targetRoute: "/governance?proposal=PROP-126", reasonCode: "PROPOSAL_REVIEW_REQUIRED", openedAt: "2026-08-24T13:20:00Z", updatedAt: "2026-08-24T13:20:00Z", assigneePrincipalId: "prn_01arz3ndektsv4rrffq69g5faw", initiatorPrincipalId: "prn_01arz3ndektsv4rrffq69g5fav", nextActions: ["open_target", "assign", "dismiss"] }),
];

function fixtureItem(input: Partial<WorkbenchAttentionItem> & Pick<WorkbenchAttentionItem, "id" | "title" | "summary" | "kind" | "priority" | "risk" | "targetType" | "targetId" | "targetRoute" | "reasonCode" | "openedAt" | "updatedAt" | "nextActions">): WorkbenchAttentionItem {
  return {
    state: "open",
    assigneePrincipalId: "prn_01arz3ndektsv4rrffq69g5fav",
    ruleVersion: "workbench.conditions.v1",
    traceId: "tr_fixture_workbench",
    version: 1,
    ...input,
  };
}

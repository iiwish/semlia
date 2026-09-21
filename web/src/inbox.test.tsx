import { describe, expect, it } from "vitest";
import { buildInbox, type InboxOperation } from "./inbox";
import type { WorkbenchAttentionItem } from "./workbench";

const operation = (id: string, progress: InboxOperation["progress"], extra: Partial<InboxOperation> = {}): InboxOperation => ({ id, title: "客户", currentVersion: 1, createdBy: "author", createdAt: "2026-09-20T00:00:00Z", updatedAt: "2026-09-20T00:00:00Z", frozen: true, progress, targetCount: 2, releaseId: null, ...extra });
const task = (id: string, proposal: string, state = "open"): WorkbenchAttentionItem => ({ id, kind: "review", title: "审核客户", summary: "核对业务定义", state, targetRoute: `/governance?proposal=${proposal}`, updatedAt: "2026-09-20T00:00:00Z" } as WorkbenchAttentionItem);

describe("unified inbox", () => {
  it("keeps one action per knowledge set and deduplicates its proposal reminders", () => {
    const rows = buildInbox([operation("op", "needs_correction", { proposalIds: ["p1", "p2"] })], [task("t1", "p1"), task("t2", "p2"), task("t3", "other"), task("t3", "other")], "pending");
    expect(rows.map((row) => row.id)).toEqual(["knowledge:op", "task:t3"]);
    expect(rows[0].action).toBe("补充与纠正");
  });
  it("derives completion from source state and preserves completed history", () => {
    const operations = [operation("published", "released"), operation("same", "no_change"), operation("old", "needs_correction", { superseded: true }), operation("new", "needs_correction"), operation("running", "validating")];
    expect(buildInbox(operations, [], "pending").map((row) => row.id)).toEqual(["knowledge:new"]);
    expect(buildInbox(operations, [], "done").map((row) => row.id)).toEqual(["knowledge:published", "knowledge:same", "knowledge:old"]);
    expect(buildInbox([operation("new", "released")], [], "pending")).toEqual([]);
  });
  it("includes drafts and rejected knowledge as explicit manual work, not system progress", () => {
    expect(buildInbox([operation("draft", "draft"), operation("rejected", "rejected"), operation("review", "in_review"), operation("publish", "ready_to_publish")], [], "pending").map((row) => row.action)).toEqual(["继续整理", "补充与纠正", "独立审核", "确认发布"]);
  });
  it("excludes resolved and dismissed collaboration tasks from pending", () => {
    const tasks = [task("1", "1", "resolved"), task("2", "2", "dismissed"), task("3", "3", "in_progress")];
    expect(buildInbox([], tasks, "pending").map((row) => row.id)).toEqual(["task:3"]);
    expect(buildInbox([], tasks, "done")).toHaveLength(2);
  });
});

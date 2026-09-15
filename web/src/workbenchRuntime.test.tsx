import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, vi } from "vitest";

import { clearSessionCSRFToken } from "./apiClient";
import { getSession } from "./identity";
import { WorkbenchApiError, workbenchApi, type WorkbenchApi, type WorkbenchAttentionItem } from "./workbench";
import { WorkbenchRuntimeProvider, useWorkbenchRuntime } from "./workbenchRuntime";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const fullAccess = { read: true, manage: true };

afterEach(() => {
  vi.unstubAllGlobals();
  clearSessionCSRFToken();
});

describe("workbench client", () => {
  it("uses the confirmed item paths, server filters, cursor, CSRF and idempotency header", async () => {
    const requests: Request[] = [];
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      requests.push(request);
      if (request.url.endsWith("/api/v1/session")) return sessionResponse();
      if (request.method === "PATCH") return json({ ...attentionItem(), state: "in_progress", version: 2 });
      if (request.url.includes("/workbench/items/")) return json(attentionItem());
      return json({ items: [attentionItem()], counts: { total: 1, open: 1, inProgress: 0, critical: 1 }, page: { limit: 25, nextCursor: "cursor-2" } });
    }));

    await getSession();
    const page = await workbenchApi.listItems(workspaceId, { view: "team", search: "退款", kind: "review", state: "open", priority: "critical", risk: "high", sort: "due_asc", limit: 25 }, "cursor-1");
    await workbenchApi.getItem(workspaceId, attentionItem().id);
    await workbenchApi.updateItem(workspaceId, attentionItem().id, { setAssignee: false, state: "in_progress", expectedVersion: 1 }, "workbench-key-1");

    expect(page.page.nextCursor).toBe("cursor-2");
    const listURL = new URL(requests.find((request) => request.method === "GET" && request.url.includes("/workbench/items?"))!.url);
    expect(listURL.pathname).toBe(`/api/v1/workspaces/${workspaceId}/workbench/items`);
    expect(Object.fromEntries(listURL.searchParams)).toMatchObject({ view: "team", search: "退款", kind: "review", state: "open", priority: "critical", risk: "high", sort: "due_asc", limit: "25", cursor: "cursor-1" });
    const mutation = requests.find((request) => request.method === "PATCH")!;
    expect(new URL(mutation.url).pathname).toBe(`/api/v1/workspaces/${workspaceId}/workbench/items/${attentionItem().id}`);
    expect(mutation.headers.get("X-Semlia-CSRF")).toBe("csrf-workbench-tests-1234567890");
    expect(mutation.headers.get("Idempotency-Key")).toBe("workbench-key-1");
    expect(await mutation.json()).toEqual({ setAssignee: false, state: "in_progress", expectedVersion: 1 });
  });
});

describe("workbench runtime", () => {
  it("does not request data without workspace.read and exposes a precise forbidden state", async () => {
    const api = makeApi();
    render(<WorkbenchRuntimeProvider workspaceId={workspaceId} api={api} access={{ read: false, manage: true }}><Probe /></WorkbenchRuntimeProvider>);

    await waitFor(() => expect(screen.getByTestId("list-state")).toHaveTextContent("forbidden"));
    expect(screen.getByTestId("list-error")).toHaveTextContent("workspace.read");
    expect(api.listItems).not.toHaveBeenCalled();
  });

  it("keeps server 403 and empty list responses distinct without fixture fallback", async () => {
    const forbiddenApi = makeApi({ listItems: vi.fn().mockRejectedValue(new WorkbenchApiError("工作台访问被拒绝。", 403, "FORBIDDEN")) });
    const forbiddenRender = render(<WorkbenchRuntimeProvider workspaceId={workspaceId} api={forbiddenApi} access={fullAccess}><Probe /></WorkbenchRuntimeProvider>);
    await waitFor(() => expect(screen.getByTestId("list-state")).toHaveTextContent("forbidden"));
    expect(screen.getByTestId("list-error")).toHaveTextContent("工作台访问被拒绝");
    expect(screen.queryByText(attentionItem().id)).not.toBeInTheDocument();
    forbiddenRender.unmount();

    const emptyApi = makeApi({ listItems: vi.fn().mockResolvedValue(page([])) });
    render(<WorkbenchRuntimeProvider workspaceId={workspaceId} api={emptyApi} access={fullAccess}><Probe /></WorkbenchRuntimeProvider>);
    await waitFor(() => expect(screen.getByTestId("list-state")).toHaveTextContent("empty"));
    expect(screen.getByTestId("counts")).toHaveTextContent("0:0:0:0");
  });

  it("keeps server counts, next actions and cursor pages authoritative", async () => {
    const listItems = vi.fn()
      .mockResolvedValueOnce(page([attentionItem("ati_01arz3ndektsv4rrffq69g5fav")], "cursor-2"))
      .mockResolvedValueOnce(page([attentionItem("ati_01arz3ndektsv4rrffq69g5faw")], undefined, { total: 2, open: 1, inProgress: 1, critical: 1 }));
    const api = makeApi({ listItems });
    const user = userEvent.setup();
    render(<WorkbenchRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><Probe /></WorkbenchRuntimeProvider>);

    expect(await screen.findByText("ati_01arz3ndektsv4rrffq69g5fav")).toBeVisible();
    expect(screen.getByTestId("counts")).toHaveTextContent("1:1:0:1");
    await user.click(screen.getByRole("button", { name: "加载更多待办" }));
    expect(await screen.findByText("ati_01arz3ndektsv4rrffq69g5faw")).toBeVisible();
    expect(screen.getByTestId("counts")).toHaveTextContent("2:1:1:1");
    expect(listItems).toHaveBeenLastCalledWith(workspaceId, expect.objectContaining({ view: "mine", sort: "priority_desc" }), "cursor-2");
  });

  it("retains a server-confirmed mutation when list refresh fails", async () => {
    const listItems = vi.fn().mockResolvedValueOnce(page([attentionItem()])).mockRejectedValueOnce(new Error("列表暂时不可用。"));
    const updateItem = vi.fn().mockResolvedValue({ ...attentionItem(), state: "in_progress", version: 2 });
    const api = makeApi({ listItems, updateItem });
    const user = userEvent.setup();
    render(<WorkbenchRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><Probe /></WorkbenchRuntimeProvider>);

    await screen.findByText(attentionItem().id);
    await user.click(screen.getByRole("button", { name: "打开待办" }));
    await waitFor(() => expect(screen.getByTestId("detail-version")).toHaveTextContent("1"));
    await user.click(screen.getByRole("button", { name: "更新待办" }));
    await waitFor(() => expect(screen.getByTestId("detail-version")).toHaveTextContent("2"));
    expect(screen.getByTestId("command-state")).toHaveTextContent("confirmed");
    expect(await screen.findByTestId("refresh-warning")).toHaveTextContent("待办已由服务端确认，但列表刷新失败");
    expect(screen.getByTestId("detail-version")).toHaveTextContent("2");
  });

  it("keeps 409 conflicts explicit and preserves the loaded version", async () => {
    const api = makeApi({ updateItem: vi.fn().mockRejectedValue(new WorkbenchApiError("待办版本已更新。", 409, "CONFLICT")) });
    const user = userEvent.setup();
    render(<WorkbenchRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><Probe /></WorkbenchRuntimeProvider>);

    await screen.findByText(attentionItem().id);
    await user.click(screen.getByRole("button", { name: "打开待办" }));
    await waitFor(() => expect(screen.getByTestId("detail-version")).toHaveTextContent("1"));
    await user.click(screen.getByRole("button", { name: "更新待办" }));
    await waitFor(() => expect(screen.getByTestId("command-state")).toHaveTextContent("conflict:CONFLICT"));
    expect(screen.getByTestId("detail-version")).toHaveTextContent("1");
  });

  it("keeps detail and mutation authorization failures scoped to their own states", async () => {
    const detailForbidden = makeApi({ getItem: vi.fn().mockRejectedValue(new WorkbenchApiError("待办详情访问被拒绝。", 403, "FORBIDDEN")) });
    const detailRender = render(<WorkbenchRuntimeProvider workspaceId={workspaceId} api={detailForbidden} access={fullAccess}><Probe /></WorkbenchRuntimeProvider>);
    const user = userEvent.setup();
    await screen.findByText(attentionItem().id);
    await user.click(screen.getByRole("button", { name: "打开待办" }));
    await waitFor(() => expect(screen.getByTestId("detail-state")).toHaveTextContent("forbidden:FORBIDDEN"));
    expect(screen.getByTestId("detail-error")).toHaveTextContent("待办详情访问被拒绝");
    expect(screen.getByTestId("list-state")).toHaveTextContent("ready");
    detailRender.unmount();

    const mutationForbidden = makeApi({ updateItem: vi.fn().mockRejectedValue(new WorkbenchApiError("待办更新被拒绝。", 403, "FORBIDDEN")) });
    render(<WorkbenchRuntimeProvider workspaceId={workspaceId} api={mutationForbidden} access={fullAccess}><Probe /></WorkbenchRuntimeProvider>);
    await screen.findByText(attentionItem().id);
    await user.click(screen.getByRole("button", { name: "打开待办" }));
    await waitFor(() => expect(screen.getByTestId("detail-version")).toHaveTextContent("1"));
    await user.click(screen.getByRole("button", { name: "更新待办" }));
    await waitFor(() => expect(screen.getByTestId("command-state")).toHaveTextContent("forbidden:FORBIDDEN"));
    expect(screen.getByTestId("detail-version")).toHaveTextContent("1");
  });
});

function Probe() {
  const runtime = useWorkbenchRuntime();
  return <div>
    <output data-testid="list-state">{runtime.state}</output>
    <output data-testid="list-error">{runtime.error}</output>
    <output data-testid="counts">{runtime.counts ? `${runtime.counts.total}:${runtime.counts.open}:${runtime.counts.inProgress}:${runtime.counts.critical}` : ""}</output>
    <output data-testid="detail-state">{runtime.detailState}:{runtime.detailErrorCode}</output>
    <output data-testid="detail-error">{runtime.detailError}</output>
    <output data-testid="detail-version">{runtime.selectedItem?.version}</output>
    <output data-testid="command-state">{runtime.commandState}:{runtime.commandErrorCode}</output>
    <output data-testid="refresh-warning">{runtime.refreshWarning}</output>
    {runtime.items.map((item) => <span key={item.id}>{item.id}</span>)}
    {runtime.nextCursor && <button type="button" onClick={() => void runtime.loadMore()}>加载更多待办</button>}
    <button type="button" onClick={() => void runtime.openItem(attentionItem().id)}>打开待办</button>
    <button type="button" onClick={() => void runtime.updateItem({ setAssignee: false, state: "in_progress" }).catch(() => undefined)}>更新待办</button>
  </div>;
}

function makeApi(overrides: Partial<WorkbenchApi> = {}): WorkbenchApi {
  return {
    listItems: vi.fn().mockResolvedValue(page([attentionItem()])),
    getItem: vi.fn().mockResolvedValue(attentionItem()),
    updateItem: vi.fn().mockResolvedValue({ ...attentionItem(), state: "in_progress", version: 2 }),
    ...overrides,
  };
}

function page(items: WorkbenchAttentionItem[], nextCursor?: string, counts = { total: items.length, open: items.length, inProgress: 0, critical: items.filter((item) => item.priority === "critical").length }) {
  return { items, counts, page: { limit: 50, nextCursor } };
}

function attentionItem(id = "ati_01arz3ndektsv4rrffq69g5fav"): WorkbenchAttentionItem {
  return {
    id,
    kind: "review",
    state: "open",
    priority: "critical",
    risk: "high",
    targetType: "asset",
    targetId: "ast_01arz3ndektsv4rrffq69g5fav",
    targetRoute: "/governance?proposal=prp_01arz3ndektsv4rrffq69g5fav",
    ruleVersion: "workbench.conditions.v1",
    title: "审核退款口径",
    summary: "需要独立审核。",
    reasonCode: "PROPOSAL_REVIEW_REQUIRED",
    traceId: "tr_workbench",
    openedAt: "2026-09-05T01:00:00Z",
    updatedAt: "2026-09-05T01:00:00Z",
    version: 1,
    nextActions: ["open_target", "review", "assign", "dismiss"],
  };
}

function sessionResponse() {
  return json({ account: { id: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Admin" }, workspaces: [{ id: workspaceId, slug: "alpha", displayName: "Alpha", principalId: "prn_01arz3ndektsv4rrffq69g5fav", roleIds: ["workspace_admin"], capabilities: ["workspace.read", "workspace.manage"], authorizationVersion: 7 }], expiresAt: "2026-09-05T18:00:00Z", traceId: "trace-session" }, 200, { "X-Semlia-CSRF": "csrf-workbench-tests-1234567890" });
}

function json(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

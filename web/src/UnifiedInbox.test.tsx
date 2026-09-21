import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CapabilityProvider } from "./authorization";
import { UnifiedInbox } from "./UnifiedInbox";
import { WorkbenchRuntimeProvider } from "./workbenchRuntime";
import { productionAPI } from "./semanticProduction";
import type { WorkbenchApi } from "./workbench";
import type { InboxOperation, InboxScope } from "./inbox";
import { readInboxPages } from "./inbox";

const operation: InboxOperation = { id: "op1", title: "客户定义", currentVersion: 1, createdBy: "author", createdAt: "2026-09-20T00:00:00Z", updatedAt: "2026-09-20T00:00:00Z", frozen: true, progress: "needs_correction", targetCount: 1, releaseId: null };
const api: WorkbenchApi = { listItems: vi.fn(async () => ({ items: [], counts: { total: 0, open: 0, inProgress: 0, critical: 0 }, page: { limit: 100 } })), getItem: vi.fn(), updateItem: vi.fn() };
const open = vi.fn();
function App({ scope = "pending", workspaceId = "workspace1" }: { scope?: InboxScope; workspaceId?: string }) {
  return <CapabilityProvider session={{ principalId: "author", version: "1", capabilities: ["asset.read", "workspace.read"] }}><WorkbenchRuntimeProvider workspaceId={workspaceId} access={{ read: true, manage: false }} api={api}><UnifiedInbox workspaceId={workspaceId} principalId="author" scope={scope} focusSearchRequestEpoch={0} onOpenOperation={open} onOpenTask={vi.fn()} /></WorkbenchRuntimeProvider></CapabilityProvider>;
}
afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks(); });

describe("knowledge actions in the inbox", () => {
  it("finds knowledge even when the collaboration queue is empty and removes it after release", async () => {
    const list = vi.spyOn(productionAPI, "list").mockResolvedValue({ items: [operation], nextCursor: null });
    const user = userEvent.setup();
    render(<App />);
    await user.click(await screen.findByRole("button", { name: "补充与纠正：客户定义" }));
    expect(open).toHaveBeenCalledWith("op1");
    list.mockResolvedValue({ items: [{ ...operation, progress: "released", releaseId: "release" }], nextCursor: null });
    await user.click(screen.getByRole("button", { name: "刷新待办" }));
    expect(await screen.findByText("当前可见范围内没有待处理事项")).toBeVisible();
    expect(screen.queryByRole("button", { name: "补充与纠正：客户定义" })).not.toBeInTheDocument();
    expect(api.updateItem).not.toHaveBeenCalled();
  });
  it("never presents a dependency failure as no work", async () => {
    vi.spyOn(productionAPI, "list").mockRejectedValue(new Error("offline"));
    render(<App />);
    expect(await screen.findByRole("alert")).toHaveTextContent("列表未完整加载");
    expect(screen.queryByText("当前可见范围内没有待处理事项")).not.toBeInTheDocument();
    expect(screen.getByText("数量未确认")).toBeVisible();
  });
  it("uses the author filter for initiated knowledge", async () => {
    const list = vi.spyOn(productionAPI, "list").mockResolvedValue({ items: [], nextCursor: null });
    render(<App scope="initiated" />);
    await waitFor(() => expect(list).toHaveBeenCalledWith("workspace1", expect.objectContaining({ createdBy: "author" }), expect.any(AbortSignal)));
  });
  it("discards responses from a previous workspace", async () => {
    let resolve!: (value: Awaited<ReturnType<typeof productionAPI.list>>) => void;
    vi.spyOn(productionAPI, "list").mockImplementation((workspace) => workspace === "workspace1" ? new Promise((done) => { resolve = done; }) : Promise.resolve({ items: [], nextCursor: null }));
    const view = render(<App />);
    view.rerender(<App workspaceId="workspace2" />);
    await screen.findByText("当前可见范围内没有待处理事项");
    await act(async () => resolve({ items: [operation], nextCursor: null }));
    expect(screen.queryByText("客户定义")).not.toBeInTheDocument();
  });
});

describe("authorized pagination", () => {
  it("continues through empty authorized pages", async () => {
    const read = vi.fn().mockResolvedValueOnce({ items: [], nextCursor: "p2" }).mockResolvedValueOnce({ items: [operation], nextCursor: null });
    expect(await readInboxPages(read)).toEqual([operation]);
    expect(read).toHaveBeenLastCalledWith("p2");
  });
  it("rejects repeated cursors and aborted requests instead of silently truncating", async () => {
    await expect(readInboxPages(async () => ({ items: [], nextCursor: "same" }))).rejects.toThrow("分页游标重复");
    const controller = new AbortController(); controller.abort();
    const read = vi.fn();
    await expect(readInboxPages(read, controller.signal)).rejects.toThrow("Aborted");
    expect(read).not.toHaveBeenCalled();
  });
});

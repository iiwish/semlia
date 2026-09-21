import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { KnowledgeSourcePicker } from "./KnowledgeSourcePicker";
import { listSources } from "./discovery";
import { sourceContentsAPI } from "./sourceContentApi";

vi.mock("./discovery", () => ({ listSources: vi.fn() }));
vi.mock("./sourceContentApi", () => ({ sourceContentsAPI: { snapshots: vi.fn(), members: vi.fn() } }));
beforeEach(() => { vi.resetAllMocks(); });

it("loads only verified source snapshots and ignores stale member pagination", async () => {
  vi.mocked(listSources).mockResolvedValue({ items: [{ id: "source", name: "订单来源" }], page: {} } as never);
  vi.mocked(sourceContentsAPI.snapshots).mockResolvedValue({ items: [{ id: "old", historyQuality: "verified", createdAt: "2026-09-21" }, { id: "new", historyQuality: "verified", createdAt: "2026-09-22" }, { id: "bad", historyQuality: "unverified", createdAt: "2026-09-22" }] } as never);
  let resolveMore!: (value: unknown) => void;
  vi.mocked(sourceContentsAPI.members).mockImplementation(async (_workspace, _source, snapshot, cursor) => cursor ? new Promise((resolve) => { resolveMore = resolve as never; }) : ({ items: [{ objectId: snapshot, name: snapshot, kind: "dataset" }], nextCursor: snapshot === "old" ? "more" : null }) as never);
  const onMembers = vi.fn();
  render(<KnowledgeSourcePicker workspaceId="workspace" onMembers={onMembers} />);
  await screen.findByRole("option", { name: "订单来源" });
  await userEvent.selectOptions(screen.getByLabelText("知识数据来源"), "source");
  await waitFor(() => expect(screen.getByLabelText("知识来源快照").querySelectorAll("option")).toHaveLength(3));
  await userEvent.selectOptions(screen.getByLabelText("知识来源快照"), "old");
  await userEvent.click(await screen.findByRole("button", { name: "加载更多来源成员" }));
  await userEvent.selectOptions(screen.getByLabelText("知识来源快照"), "new");
  await waitFor(() => expect(onMembers).toHaveBeenLastCalledWith([expect.objectContaining({ objectId: "new", snapshotId: "new" })]));
  await act(async () => resolveMore({ items: [{ objectId: "stale", name: "stale", kind: "dataset" }], nextCursor: null }));
  expect(onMembers).toHaveBeenLastCalledWith([expect.objectContaining({ objectId: "new", snapshotId: "new" })]);
});

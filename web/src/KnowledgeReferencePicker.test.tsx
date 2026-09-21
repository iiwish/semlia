import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { getAsset, listAssets, type CatalogAssetDetail } from "./catalog";
import { KnowledgeReferenceEditor, KnowledgeWorkspaceContext } from "./KnowledgeReferencePicker";

vi.mock("./catalog", () => ({ getAsset: vi.fn(), listAssets: vi.fn() }));
const asset = (id: string, released = true): CatalogAssetDetail => ({ id, title: id === "published" ? "支付订单" : "未发布订单", assetType: "business_object", address: `commerce.${id}`, currentRevision: { id: `rev-${id}`, content: { spec: { members: [{ id: "amount", name: "支付金额" }] } } }, authoritySections: released ? [{ kind: "released_state", availability: "available", releaseId: "release-1", revisionId: `rev-${id}` }] : [] }) as unknown as CatalogAssetDetail;
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(listAssets).mockResolvedValue({ items: [asset("published"), asset("draft", false)], page: { limit: 100 } });
  vi.mocked(getAsset).mockImplementation(async (_workspace, id) => asset(id, id === "published"));
});

it("selects a published member and freezes all identifiers while rejecting drafts", async () => {
  const onChange = vi.fn();
  render(<KnowledgeWorkspaceContext.Provider value="workspace"><KnowledgeReferenceEditor label="输入属性" member onChange={onChange} /></KnowledgeWorkspaceContext.Provider>);
  const trigger = screen.getByRole("button", { name: "选择输入属性" });
  await userEvent.click(trigger);
  expect(screen.getByLabelText("搜索已发布知识")).toHaveFocus();
  await userEvent.click(await screen.findByRole("button", { name: /未发布订单/ }));
  expect(await screen.findByRole("alert")).toHaveTextContent("没有可选的当前已发布版本");
  expect(screen.getByRole("button", { name: "固定此版本" })).toBeDisabled();
  await userEvent.click(screen.getByRole("button", { name: /支付订单/ }));
  await userEvent.selectOptions(await screen.findByLabelText("已发布成员"), "amount");
  await userEvent.click(screen.getByRole("button", { name: "固定此版本" }));
  expect(onChange).toHaveBeenLastCalledWith({ assetId: "published", revisionId: "rev-published", releaseId: "release-1", memberId: "amount" });
  expect(trigger).toHaveFocus();
  expect(listAssets).toHaveBeenCalledWith("workspace", "", "business_object", undefined, expect.any(AbortSignal));
});

it("does not retarget an existing frozen reference or accept a stale detail response", async () => {
  const value = { assetId: "historical", revisionId: "rev-old", releaseId: "release-old", memberId: "amount" };
  const onChange = vi.fn();
  let resolveDetail!: (detail: CatalogAssetDetail) => void;
  vi.mocked(getAsset).mockReturnValueOnce(new Promise((resolve) => { resolveDetail = resolve; }));
  render(<KnowledgeWorkspaceContext.Provider value="workspace"><KnowledgeReferenceEditor label="输入属性" value={value} member onChange={onChange} /></KnowledgeWorkspaceContext.Provider>);
  await userEvent.click(screen.getByRole("button", { name: "选择输入属性" }));
  await userEvent.click(await screen.findByRole("button", { name: /支付订单/ }));
  await userEvent.type(screen.getByLabelText("搜索已发布知识"), "其他");
  await act(async () => resolveDetail(asset("published")));
  await waitFor(() => expect(screen.queryByLabelText("已发布成员")).not.toBeInTheDocument());
  await userEvent.keyboard("{Escape}");
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  expect(onChange).not.toHaveBeenCalled();
  await userEvent.click(screen.getByText("固定版本标识"));
  expect(screen.getByText("rev-old")).toBeVisible();
});

it("refuses a release that belongs to a different revision", async () => {
  vi.mocked(getAsset).mockResolvedValue({ ...asset("published"), authoritySections: [{ ...asset("published").authoritySections[0], revisionId: "old-revision" }] });
  render(<KnowledgeWorkspaceContext.Provider value="workspace"><KnowledgeReferenceEditor label="基础业务对象" onChange={vi.fn()} /></KnowledgeWorkspaceContext.Provider>);
  await userEvent.click(screen.getByRole("button", { name: "选择基础业务对象" }));
  await userEvent.click(await screen.findByRole("button", { name: /支付订单/ }));
  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.getByRole("button", { name: "固定此版本" })).toBeDisabled();
});

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { KnowledgeRevisionWorkbench } from "./KnowledgeRevisionWorkbench";
import { KnowledgeRevisionUncertainError } from "./knowledgeProductionRevision";
import { CapabilityProvider } from "./authorization";
import { CatalogRuntimeProvider } from "./testing/catalogFixture";
import { assets } from "./testing/data";

const asset = { ...assets[0], knowledgeSpec: {}, revisionRecord: { ...assets[0].revisionRecord, revisionId: "rev_opened", definition: "Opened baseline" } };
function Form({ onSubmit, current = asset, fieldPath = "definition" }: { onSubmit: (value: unknown) => Promise<void>; current?: typeof asset; fieldPath?: string }) {
  return <CapabilityProvider session={{ principalId: "author", version: "1", capabilities: ["asset.propose"] }}><CatalogRuntimeProvider fixtureAssets={[current]}><KnowledgeRevisionWorkbench asset={current} request={{ assetId: asset.id, fieldPath, origin: "ask", context: "Synthetic answer pin" }} onCancel={vi.fn()} onNotify={vi.fn()} onStartAIGeneration={vi.fn()} onSubmit={onSubmit} /></CatalogRuntimeProvider></CapabilityProvider>;
}

it("captures the opened baseline and saves a draft without claiming review submission", async () => {
  const onSubmit = vi.fn().mockResolvedValue(undefined);
  const view = render(<Form onSubmit={onSubmit} />);
  const user = userEvent.setup();
  await user.clear(screen.getByLabelText("业务定义候选值"));
  await user.type(screen.getByLabelText("业务定义候选值"), "Desired correction");
  view.rerender(<Form onSubmit={onSubmit} current={{ ...asset, revisionRecord: { ...asset.revisionRecord, revisionId: "rev_advanced", definition: "Advanced baseline" } }} />);
  await user.click(screen.getByRole("button", { name: "运行检查" }));
  await user.click(screen.getByRole("button", { name: "保存修订并继续确认" }));
  expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ baseRevisionId: "rev_opened", changes: [{ field: "definition", before: "Opened baseline", after: "Desired correction" }] }));
  expect(screen.queryByRole("button", { name: "提交审核" })).not.toBeInTheDocument();
});

it("locks uncertain edits and retries the frozen submission instead of creating another command", async () => {
  let fail!: (error: Error) => void;
  const onSubmit = vi.fn().mockReturnValueOnce(new Promise((_resolve, reject) => { fail = reject; })).mockResolvedValue(undefined);
  render(<Form onSubmit={onSubmit} />);
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("业务定义候选值"), " corrected");
  await user.click(screen.getByRole("button", { name: "运行检查" }));
  await user.dblClick(screen.getByRole("button", { name: "保存修订并继续确认" }));
  expect(onSubmit).toHaveBeenCalledOnce();
  await act(async () => fail(new KnowledgeRevisionUncertainError()));
  expect(screen.getByLabelText("业务定义候选值")).toBeDisabled();
  expect(screen.getByLabelText("知识修订原因")).toBeDisabled();
  expect(screen.getByRole("button", { name: "取消修订" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "重试同一保存请求" }));
  expect(onSubmit.mock.calls[1][0]).toEqual(onSubmit.mock.calls[0][0]);
});

it("preserves editable unsent content on a deterministic validation error", async () => {
  const onSubmit = vi.fn().mockRejectedValue(new Error("Baseline changed"));
  render(<Form onSubmit={onSubmit} />);
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("业务定义候选值"), " corrected");
  await user.click(screen.getByRole("button", { name: "运行检查" }));
  await user.click(screen.getByRole("button", { name: "保存修订并继续确认" }));
  expect(await screen.findByText("Baseline changed")).toBeVisible();
  expect(screen.getByLabelText("业务定义候选值")).toHaveValue("Opened baseline corrected");
  expect(screen.getByLabelText("业务定义候选值")).toBeEnabled();
});

it("binds the editor to the visible definition when opened from definition.boundary on a typed knowledge object", async () => {
  const onSubmit = vi.fn().mockResolvedValue(undefined);
  render(<Form onSubmit={onSubmit} fieldPath="definition.boundary" />);
  const user = userEvent.setup();
  expect(screen.getByRole("heading", { name: "业务定义", level: 3 })).toBeVisible();
  expect(screen.getByLabelText("业务定义候选值")).toHaveValue("Opened baseline");
  await user.type(screen.getByLabelText("业务定义候选值"), " plus exclusions");
  await user.type(screen.getByLabelText("知识修订原因"), "Clarify the boundary");
  await user.click(screen.getByRole("button", { name: "运行检查" }));
  await user.click(screen.getByRole("button", { name: "保存修订并继续确认" }));
  expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ changes: [{ field: "definition", before: "Opened baseline", after: "Opened baseline plus exclusions" }] }));
});

it.each(["definition.excludes", "definition.includes", "wiki.disambiguation", "vocabulary"])("opens governed wording when opened from %s on a typed knowledge object", async (fieldPath) => {
  const onSubmit = vi.fn().mockResolvedValue(undefined);
  const worded = { ...asset, vocabulary: { aliases: ["GMV"], includes: [], excludes: [], examples: [], confusableWith: [], sampleQuestions: [] } };
  render(<Form onSubmit={onSubmit} fieldPath={fieldPath} current={worded} />);
  const user = userEvent.setup();
  expect(screen.getByRole("heading", { name: "业务词汇", level: 3 })).toBeVisible();
  expect(screen.getByLabelText("别名（每行一项）")).toHaveValue("GMV");
  await user.type(screen.getByLabelText("别名（每行一项）"), "\n成交额");
  await user.type(screen.getByLabelText("明确排除（每行一项）"), "无效订单");
  await user.type(screen.getByLabelText("知识修订原因"), "Record how people ask");
  await user.click(screen.getByRole("button", { name: "运行检查" }));
  await user.click(screen.getByRole("button", { name: "保存修订并继续确认" }));
  expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ changes: [
    { field: "aliases", before: ["GMV"], after: ["GMV", "成交额"] },
    { field: "excludes", before: undefined, after: ["无效订单"] },
  ] }));
});

it("blocks incomplete vocabulary before saving", async () => {
  const onSubmit = vi.fn().mockResolvedValue(undefined);
  render(<Form onSubmit={onSubmit} fieldPath="vocabulary" />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "添加典型问题" }));
  await user.type(screen.getByLabelText("知识修订原因"), "Add a sample");
  await user.click(screen.getByRole("button", { name: "运行检查" }));
  expect(screen.getByRole("alert")).toHaveTextContent("典型问题不能为空。");
  await user.type(screen.getByLabelText("典型问题 1"), "8月 GMV 是多少？");
  await user.click(screen.getByRole("button", { name: "运行检查" }));
  await user.click(screen.getByRole("button", { name: "保存修订并继续确认" }));
  expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ changes: [{ field: "sampleQuestions", before: undefined, after: [{ question: "8月 GMV 是多少？" }] }] }));
});

it("opens the type-specific spec when the requested target is an executable member", async () => {
  render(<Form onSubmit={vi.fn().mockResolvedValue(undefined)} fieldPath="spec.expression" />);
  expect(screen.getByRole("heading", { name: "类型专属知识", level: 3 })).toBeVisible();
});

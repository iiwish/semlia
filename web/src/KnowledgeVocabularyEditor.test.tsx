import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, expect, it, vi } from "vitest";
import { getAsset } from "./catalog";
import { KnowledgeVocabularyEditor } from "./KnowledgeVocabularyEditor";
import { emptyVocabulary, type KnowledgeSpec, type KnowledgeVocabulary } from "./knowledge";
import { findingDetail } from "./knowledgeMessages";

vi.mock("./catalog", () => ({ getAsset: vi.fn(), listAssets: vi.fn() }));

const stale = { assetId: "ast_model", revisionId: "rev_model_1", releaseId: "rls_1" };
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(getAsset).mockImplementation(async (_workspace, id) => ({ id, title: "订单分析模型", currentRevision: { id: "rev_model_2" }, authoritySections: [{ kind: "released_state", availability: "available", releaseId: "rls_2", revisionId: "rev_model_2" }] }) as unknown as Awaited<ReturnType<typeof getAsset>>);
});

function Harness({ initial, spec: initialSpec = {}, onValue }: { initial: KnowledgeVocabulary; spec?: KnowledgeSpec; onValue: (value: KnowledgeVocabulary, spec: KnowledgeSpec) => void }) {
  const [value, setValue] = useState(initial);
  const [spec, setSpec] = useState(initialSpec);
  return <KnowledgeVocabularyEditor type="business_object" workspaceId="wsp_a" value={value} spec={spec} onChange={(next) => { setValue(next); onValue(next, spec); }} onSpecChange={(next) => { setSpec(next); onValue(value, next); }} />;
}

it("flags references behind the current published revision and refreshes them in place", async () => {
  const onValue = vi.fn();
  render(<Harness initial={{ ...emptyVocabulary(), sampleQuestions: [{ question: "8月 GMV 是多少？", expects: { refs: [stale] } }] }} onValue={onValue} />);
  const banner = await screen.findByText(/1 个引用不是当前已发布版本/);
  expect(banner).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "刷新典型问题 1 的期望召回 1为当前版本" }));
  expect(onValue).toHaveBeenLastCalledWith(expect.objectContaining({ sampleQuestions: [{ question: "8月 GMV 是多少？", expects: { refs: [{ assetId: "ast_model", revisionId: "rev_model_2", releaseId: "rls_2" }] } }] }), {});
  await waitFor(() => expect(screen.queryByText(/引用不是当前已发布版本/)).not.toBeInTheDocument());
});

it("refreshes every stale reference at once and keeps current ones untouched", async () => {
  const onValue = vi.fn();
  const current = { assetId: "ast_model", revisionId: "rev_model_2", releaseId: "rls_2" };
  render(<Harness initial={{ ...emptyVocabulary(), confusableWith: [{ ref: stale, distinction: "不同" }], sampleQuestions: [{ question: "q", expects: { refs: [current] } }] }} onValue={onValue} />);
  await userEvent.click(await screen.findByRole("button", { name: "全部刷新为当前版本" }));
  const [value] = onValue.mock.lastCall!;
  expect(value.confusableWith[0].ref).toEqual(current);
  expect(value.sampleQuestions[0].expects.refs[0]).toEqual(current);
});

it("edits attribute aliases and value aliases of text members through the spec", async () => {
  const onValue = vi.fn();
  const spec: KnowledgeSpec = { members: [{ id: "region", name: "当前区域", valueType: "string" }, { id: "amount", name: "金额", valueType: "number" }] };
  render(<Harness initial={emptyVocabulary()} spec={spec} onValue={onValue} />);
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("当前区域 属性别名"), "区域，大区");
  const group = screen.getByRole("group", { name: "属性别名与取值别名" });
  expect(within(group).getAllByRole("button", { name: "添加取值别名" })).toHaveLength(1);
  await user.click(within(group).getByRole("button", { name: "添加取值别名" }));
  await user.type(screen.getByLabelText("当前区域 规范取值 1"), "华东");
  await user.type(screen.getByLabelText("当前区域 规范取值 1 的说法"), "江浙沪、华东地区");
  const [, latest] = onValue.mock.lastCall!;
  expect(latest.members[0]).toEqual({ id: "region", name: "当前区域", valueType: "string", aliases: ["区域", "大区"], valueAliases: [{ value: "华东", aliases: ["江浙沪", "华东地区"] }] });
  expect(latest.members[1]).toEqual(spec.members![1]);
});

it("shows the server's specific vocabulary findings and generic text otherwise", () => {
  expect(findingDetail({ code: "PRODUCTION_VOCABULARY_CONFLICT", severity: "blocker", message: "别名冲突：「gmv」会在发布后指向两个同类资产。" })).toContain("「gmv」");
  expect(findingDetail({ code: "PRODUCTION_SAMPLE_QUESTION_NOT_RECALLED", severity: "blocker", message: "" })).toContain("典型问题检索不到");
  expect(findingDetail({ code: "RISK_BLOCKER", severity: "blocker", message: "internal detail" })).not.toContain("internal detail");
});

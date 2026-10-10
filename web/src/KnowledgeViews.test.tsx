import { act, render as renderUI, screen, within } from "@testing-library/react";
import type { ReactElement } from "react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { askReleasedSemantics, type AskResponse } from "./ask";
import { getExecution, type ExecutionResult } from "./execution";
import { AskView, errorMessage } from "./KnowledgeViews";
import { CapabilityProvider } from "./authorization";

function render(ui: ReactElement) {
  return renderUI(ui, { wrapper: ({ children }) => <CapabilityProvider session={{ principalId: "author", version: "1", capabilities: ["asset.propose"] }}>{children}</CapabilityProvider> });
}

vi.mock("./ask", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./ask")>();
  return { ...actual, askReleasedSemantics: vi.fn() };
});

vi.mock("./execution", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./execution")>();
  return { ...actual, getExecution: vi.fn() };
});

const askMock = vi.mocked(askReleasedSemantics);
const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";

function resolvedResponse(): AskResponse {
  return {
    agentRun: {
      id: "arun_01arz3ndektsv4rrffq69g5fav",
      model: "gpt-test",
      configRevision: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/mset_01arz3ndektsv4rrffq69g5fav",
      inputHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
      outputDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      status: "succeeded",
      costMicros: 12,
      startedAt: "2026-09-04T08:00:00Z",
    },
    interpretation: {
      schema: "semlia.ask-interpretation/v1",
      outcome: "query",
      query: { schemaVersion: "1.0.0", intent: "describe", measures: [{ search: "净收入" }], context: { mode: "current" } },
    },
    definitions: [{
      assetId: "ast_01arz3ndektsv4rrffq69g5fav",
      revisionId: "rev_01arz3ndektsv4rrffq69g5fav",
      address: "commerce.net_revenue",
      assetType: "metric",
      name: "净收入",
      definition: "收入扣除确认退款后的已发布业务口径。",
      contentDigest: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
    }],
    resolution: {
      id: "smq_01arz3ndektsv4rrffq69g5fav",
      schemaVersion: "1.0.0",
      resolverVersion: "alpha-1",
      requestDigest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
      outcome: "resolved",
      channel: "ask",
      releaseId: "rls_01arz3ndektsv4rrffq69g5fav",
      plan: {
        id: "rsp_01arz3ndektsv4rrffq69g5fav",
        queryId: "smq_01arz3ndektsv4rrffq69g5fav",
        releaseId: "rls_01arz3ndektsv4rrffq69g5fav",
        resolverVersion: "alpha-1",
        intent: "describe",
        assets: [{ assetId: "ast_01arz3ndektsv4rrffq69g5fav", revisionId: "rev_01arz3ndektsv4rrffq69g5fav", address: "commerce.net_revenue", assetType: "metric" }],
        objects: [],
        executionStatus: "not_configured",
        planDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        createdAt: "2026-09-04T08:00:01Z",
      },
      validation: {
        id: "qvr_01arz3ndektsv4rrffq69g5fav",
        queryId: "smq_01arz3ndektsv4rrffq69g5fav",
        validator: "semantic-plan",
        validatorVersion: "alpha-1",
        inputDigest: "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
        status: "passed",
        results: [],
        createdAt: "2026-09-04T08:00:01Z",
        completedAt: "2026-09-04T08:00:01Z",
      },
      createdAt: "2026-09-04T08:00:01Z",
    },
  };
}

describe("real Ask", () => {
  beforeEach(() => { askMock.mockReset(); vi.mocked(getExecution).mockReset(); localStorage.clear(); });

  it("routes an empty published workspace to knowledge confirmation without calling the model", async () => {
    const user = userEvent.setup();
    const openKnowledge = vi.fn();
    render(<AskView workspaceId={workspaceId} noPublishedKnowledge onOpenKnowledge={openKnowledge} onOpenEvidence={() => {}} onStartRevision={() => {}} />);
    expect(screen.getByRole("heading", { name: "尚无已发布知识" })).toBeVisible();
    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "收入是多少？");
    expect(screen.getByRole("button", { name: "发送问题" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "查看待确认知识" }));
    expect(openKnowledge).toHaveBeenCalledOnce();
    expect(askMock).not.toHaveBeenCalled();
  });

  it("renders only released definitions and an honest unexecuted plan", async () => {
    const user = userEvent.setup();
    const openEvidence = vi.fn();
    askMock.mockResolvedValue(resolvedResponse());
    render(<AskView workspaceId={workspaceId} onOpenEvidence={openEvidence} onStartRevision={() => {}} />);

    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "净收入的定义是什么？");
    await user.click(screen.getByRole("button", { name: "发送问题" }));

    expect(await screen.findByText(/收入扣除确认退款后的已发布业务口径/)).toBeVisible();
    const plan = screen.getByRole("region", { name: "已解析语义计划" });
    expect(within(plan).getByText("已完成语义解析，未执行数据查询")).toBeVisible();
    expect(screen.queryByText(/¥|同比|目标低/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /净收入.*commerce.net_revenue/ }));
    expect(openEvidence).toHaveBeenCalledWith({ assetId: "ast_01arz3ndektsv4rrffq69g5fav", revisionId: "rev_01arz3ndektsv4rrffq69g5fav", releaseId: "rls_01arz3ndektsv4rrffq69g5fav" });
  });

  it("shows how the question's wording bound to governed vocabulary", async () => {
    const user = userEvent.setup();
    askMock.mockResolvedValue({ ...resolvedResponse(), vocabularyMatches: [
      { text: "gmv", assetId: "ast_01arz3ndektsv4rrffq69g5fav", name: "有效支付金额", assetType: "metric", via: "alias" },
      { text: "江浙沪", assetId: "ast_01arz3ndektsv4rrffq69g5fa1", name: "客户", assetType: "business_object", memberId: "region", memberName: "当前区域", value: "华东", via: "value_alias" },
      { text: "有效支付金额", assetId: "ast_01arz3ndektsv4rrffq69g5fav", name: "有效支付金额", assetType: "metric", via: "name" },
    ] });
    render(<AskView workspaceId={workspaceId} onOpenEvidence={() => {}} onStartRevision={() => {}} />);
    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "江浙沪 GMV 是多少？");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    const matches = await screen.findByRole("region", { name: "问题中的词汇匹配" });
    expect(matches).toHaveTextContent("gmv→有效支付金额别名");
    expect(matches).toHaveTextContent("江浙沪→客户 · 当前区域 = 华东取值别名");
    expect(within(matches).getAllByRole("listitem")).toHaveLength(2);
  });

  it("omits the vocabulary section when only names matched", async () => {
    const user = userEvent.setup();
    askMock.mockResolvedValue({ ...resolvedResponse(), vocabularyMatches: [{ text: "净收入", assetId: "ast_01arz3ndektsv4rrffq69g5fav", name: "净收入", assetType: "metric", via: "name" }] });
    render(<AskView workspaceId={workspaceId} onOpenEvidence={() => {}} onStartRevision={() => {}} />);
    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "净收入是多少？");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    await screen.findByText("计划已验证");
    expect(screen.queryByRole("region", { name: "问题中的词汇匹配" })).not.toBeInTheDocument();
  });

  it("maps provider failure to an explicit no-fallback boundary", () => {
    const failure = errorMessage({ message: "unavailable", code: "PROVIDER_UNAVAILABLE" });
    expect(failure).toEqual({
      code: "PROVIDER_UNAVAILABLE",
      title: "模型服务暂不可用",
      detail: "本次请求已记录为失败，没有回退到演示答案。请检查默认模型及凭证后重试。",
    });
  });

  it("aborts a hidden question and ignores its late result after another question", async () => {
    let finish!: (value: AskResponse) => void;
    askMock.mockReturnValueOnce(new Promise((resolve) => { finish = resolve; })).mockResolvedValueOnce({ ...resolvedResponse(), agentRun: { ...resolvedResponse().agentRun, id: "arun_new" }, definitions: [] });
    const props = { workspaceId, onOpenEvidence: vi.fn(), onStartRevision: vi.fn() };
    const view = render(<AskView {...props} />);
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "旧问题");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    const signal = (askMock.mock.calls[0] as unknown as [string, unknown, AbortSignal])[2];
    expect(signal).toBeInstanceOf(AbortSignal);
    view.rerender(<AskView {...props} hidden />);
    expect(signal.aborted).toBe(true);
    view.rerender(<AskView {...props} />);
    expect(screen.getByText("请求已取消")).toBeVisible();
    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "新问题");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    await act(async () => finish(resolvedResponse()));
    expect(screen.queryByText(/收入扣除确认退款后的已发布业务口径/)).not.toBeInTheDocument();
    expect(screen.getByText("新问题")).toBeVisible();
  });

  it("carries the answer pin into correction and preserves a completed answer while hidden", async () => {
    askMock.mockResolvedValue(resolvedResponse());
    const props = { workspaceId, onOpenEvidence: vi.fn(), onStartRevision: vi.fn() };
    const view = render(<AskView {...props} />);
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "净收入");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    await screen.findByText(/收入扣除确认退款后的已发布业务口径/);
    view.rerender(<AskView {...props} hidden />);
    view.rerender(<AskView {...props} />);
    await user.click(screen.getByRole("button", { name: "指出问题" }));
    await user.click(screen.getByRole("button", { name: "修订相关知识" }));
    expect(props.onStartRevision).toHaveBeenCalledWith({ assetId: resolvedResponse().definitions[0].assetId, revisionId: resolvedResponse().definitions[0].revisionId, releaseId: resolvedResponse().resolution!.releaseId }, "definition.boundary", expect.stringContaining("净收入"));
    expect(askMock).toHaveBeenCalledTimes(1);
  });

  it("keeps issue reporting available without allowing an unauthorized correction", async () => {
    askMock.mockResolvedValue(resolvedResponse());
    const onStartRevision = vi.fn();
    render(<CapabilityProvider session={{ principalId: "consumer", version: "1", capabilities: ["semantic.resolve"] }}><AskView workspaceId={workspaceId} onOpenEvidence={vi.fn()} onStartRevision={onStartRevision} /></CapabilityProvider>);
    const user = userEvent.setup();
    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "净收入");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    await user.click(await screen.findByRole("button", { name: "指出问题" }));
    expect(screen.getByRole("button", { name: "修订相关知识" })).toBeDisabled();
    expect(screen.getByText("当前身份没有提出知识修订的权限。" )).toBeVisible();
    await user.click(screen.getByRole("radio", { name: /问法被错误理解/ }));
    expect(screen.getByRole("button", { name: "修改本次问题" })).toBeEnabled();
    expect(onStartRevision).not.toHaveBeenCalled();
  });

  it("maps a missing default model to an actionable configuration boundary", () => {
    expect(errorMessage({ message: "request conflicts with current state", code: "CONFLICT" })).toEqual({
      code: "CONFLICT",
      title: "尚未配置可用的默认模型",
      detail: "请先在系统设置中启用模型提供方，并为当前工作区选择默认模型。",
    });
  });

  it("renders clarification without claiming a plan", async () => {
    const user = userEvent.setup();
    const response = resolvedResponse();
    response.interpretation = { schema: "semlia.ask-interpretation/v1", outcome: "clarification", clarification: "请说明要查询的指标。" };
    response.definitions = [];
    delete response.resolution;
    askMock.mockResolvedValue(response);
    render(<AskView workspaceId={workspaceId} onOpenEvidence={() => {}} onStartRevision={() => {}} />);

    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "帮我看看");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    expect(await screen.findByText("请说明要查询的指标。")).toBeVisible();
    expect(screen.queryByRole("region", { name: "已解析语义计划" })).not.toBeInTheDocument();
  });

  it("renders a resolver refusal without guessing or fabricating evidence", async () => {
    const user = userEvent.setup();
    const response = resolvedResponse();
    response.definitions = [];
    response.resolution!.outcome = "refused";
    response.resolution!.refusal = {
      code: "AMBIGUOUS_MATCH",
      candidateIds: ["ast_01arz3ndektsv4rrffq69g5fav"],
      clarification: "存在多个同名指标，请提供语义地址。",
      details: {},
    };
    delete response.resolution!.plan;
    delete response.resolution!.validation.planId;
    askMock.mockResolvedValue(response);
    render(<AskView workspaceId={workspaceId} onOpenEvidence={() => {}} onStartRevision={() => {}} />);

    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "收入是多少？");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    expect(await screen.findByText("语义解析被拒绝")).toBeVisible();
    expect(screen.getByText("匹配到多个候选，需要消歧")).toBeVisible();
    expect(screen.getByText(/AMBIGUOUS_MATCH/)).toBeVisible();
    expect(screen.getByText("这是显式拒绝，不会被替换成猜测、样例值或未发布知识。")).toBeVisible();
    expect(screen.queryByRole("region", { name: "已解析语义计划" })).not.toBeInTheDocument();
  });
  it("shows an earlier finished execution only on the empty home, never beneath a new answer", async () => {
    const user = userEvent.setup();
    localStorage.setItem(`semlia.execution.${workspaceId}`, "run_earlier");
    const finished: ExecutionResult = { availability: "metadata_only", replay: true, run: { id: "run_earlier", workspaceId, queryId: "smq_earlier", planId: "rsp_earlier", planDigest: `sha256:${"a".repeat(64)}`, principalId: "prn_actor", channel: "web", sourceId: "src_source", sourceRevisionId: "srv_revision", adapterVersion: "postgres-aggregate/v1", state: "succeeded", rowCount: 1, byteCount: 64, startedAt: "2026-09-05T10:00:00Z", traceId: "a".repeat(32), policyVersion: 1, timeoutMs: 1000, maxRows: 10, maxBytes: 1024, cancelRequested: false } };
    vi.mocked(getExecution).mockResolvedValue(finished);
    const response = resolvedResponse();
    response.definitions = [];
    response.resolution!.outcome = "refused";
    response.resolution!.refusal = { code: "MISSING_PHYSICAL_BINDING", candidateIds: [], clarification: "没有已发布分析模型同时覆盖指标、属性、口径与统计时间。", details: {} };
    delete response.resolution!.plan;
    delete response.resolution!.validation.planId;
    askMock.mockResolvedValue(response);
    render(<AskView workspaceId={workspaceId} onOpenEvidence={() => {}} onStartRevision={() => {}} />);

    expect(await screen.findByText(/原始结果行未存储/)).toBeVisible();
    await user.type(screen.getByRole("textbox", { name: "向 Semlia 提问" }), "新客户的金额？");
    await user.click(screen.getByRole("button", { name: "发送问题" }));
    expect(await screen.findByText("语义解析被拒绝")).toBeVisible();
    expect(screen.getByText("没有已发布模型覆盖所需成员")).toBeVisible();
    expect(screen.queryByText(/原始结果行未存储/)).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "只读查询执行" })).not.toBeInTheDocument();
  });
});

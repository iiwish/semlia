import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { askReleasedSemantics, type AskResponse } from "./ask";
import { AskView, errorMessage } from "./KnowledgeViews";

vi.mock("./ask", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./ask")>();
  return { ...actual, askReleasedSemantics: vi.fn() };
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
  beforeEach(() => askMock.mockReset());

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
    expect(openEvidence).toHaveBeenCalledWith("ast_01arz3ndektsv4rrffq69g5fav");
  });

  it("maps provider failure to an explicit no-fallback boundary", () => {
    const failure = errorMessage({ message: "unavailable", code: "PROVIDER_UNAVAILABLE" });
    expect(failure).toEqual({
      code: "PROVIDER_UNAVAILABLE",
      title: "模型服务暂不可用",
      detail: "本次请求已记录为失败，没有回退到演示答案。请检查默认模型及凭证后重试。",
    });
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
    expect(screen.getByText("AMBIGUOUS_MATCH")).toBeVisible();
    expect(screen.getByText("这是显式拒绝，不会被替换成猜测、样例值或未发布知识。")).toBeVisible();
    expect(screen.queryByRole("region", { name: "已解析语义计划" })).not.toBeInTheDocument();
  });
});

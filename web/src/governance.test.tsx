import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, vi } from "vitest";

import { CatalogRuntimeProvider, useCatalogRuntime } from "./catalogRuntime";
import { ProductApp } from "./ProductApp";
import { GovernanceApiError } from "./governance";
import { assets } from "./data";
import type { Asset } from "./types";

const governanceMocks = vi.hoisted(() => ({
  listProposals: vi.fn(),
  createProposal: vi.fn(),
  getProposal: vi.fn(),
  submitProposal: vi.fn(),
  listValidationRuns: vi.fn(),
  getPolicyDecision: vi.fn(),
  createReview: vi.fn(),
  listReviewBatches: vi.fn(),
  assembleReviewBatches: vi.fn(),
  getReviewBatch: vi.fn(),
  confirmReviewBatch: vi.fn(),
  listReleases: vi.fn(),
  publishRelease: vi.fn(),
  getRelease: vi.fn(),
  rollbackRelease: vi.fn(),
  listModelProviders: vi.fn(),
  createModelProvider: vi.fn(),
  updateModelProvider: vi.fn(),
  createModelSetting: vi.fn(),
  updateModelSetting: vi.fn(),
  setDefaultModelSetting: vi.fn(),
  generateProposal: vi.fn(),
}));

vi.mock("./governance", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./governance")>();
  return { ...actual, ...governanceMocks };
});

const netRevenue = assets.find((asset) => asset.name === "净收入") as Asset;

function App({ session }: { session?: Parameters<typeof ProductApp>[0]["session"] }) {
  return <CatalogRuntimeProvider fixtureAssets={assets}><ProductApp session={session} /></CatalogRuntimeProvider>;
}

function emptyWorkspace() {
  governanceMocks.listProposals.mockResolvedValue([]);
  governanceMocks.listReleases.mockResolvedValue([]);
  governanceMocks.listReviewBatches.mockResolvedValue([]);
  governanceMocks.listModelProviders.mockResolvedValue([]);
}

beforeEach(() => {
  vi.clearAllMocks();
  emptyWorkspace();
  governanceMocks.getProposal.mockResolvedValue({});
  governanceMocks.getRelease.mockResolvedValue({});
  governanceMocks.listValidationRuns.mockResolvedValue([]);
  governanceMocks.getPolicyDecision.mockResolvedValue(null);
});

describe("governance API convergence", () => {
  it("renders governance failures as error states and never falls back to fixture data", async () => {
    const user = userEvent.setup();
    governanceMocks.listProposals.mockRejectedValue(new GovernanceApiError("governance storage unreachable", "CONFLICT"));
    governanceMocks.listReleases.mockRejectedValue(new GovernanceApiError("governance storage unreachable", "CONFLICT"));
    governanceMocks.listModelProviders.mockRejectedValue(new GovernanceApiError("governance storage unreachable", "CONFLICT"));
    render(<App />);

    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    const alerts = await screen.findAllByRole("alert");
    expect(alerts.some((alert) => alert.textContent.includes("提案加载失败：governance storage unreachable"))).toBe(true);
    expect(screen.queryByText("统一客单价的退款订单处理口径")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /查看候选资产版本/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    await user.click(await screen.findByRole("button", { name: /模型配置/ }));
    const configuration = await screen.findByRole("region", { name: "模型配置" });
    expect(within(configuration).getByRole("alert")).toHaveTextContent("governance storage unreachable");
    expect(within(configuration).queryByText("OpenAI Enterprise")).not.toBeInTheDocument();
    expect(within(configuration).queryByText("gpt-4.1")).not.toBeInTheDocument();
  });

  it("creates and submits real proposals from the revision workbench", async () => {
    const user = userEvent.setup();
    governanceMocks.createProposal.mockResolvedValue({
      id: "prp_test_0001",
      targetObjectType: "semantic_asset",
      targetObjectId: netRevenue.id,
      state: "draft",
      title: "修订净收入的计算表达式",
      summary: "修订计算表达式。",
      reason: "口径需要扣减确认退款。",
      createdBy: "catalog-web",
      createdAt: "2026-09-03T09:00:00Z",
      updatedAt: "2026-09-03T09:00:00Z",
      changeSet: [],
    });
    governanceMocks.listProposals.mockResolvedValue([{
      id: "prp_test_0001",
      targetObjectType: "semantic_asset",
      targetObjectId: netRevenue.id,
      assetId: netRevenue.id,
      state: "in_review",
      title: "修订净收入的计算表达式",
      summary: "修订计算表达式。",
      reason: "口径需要扣减确认退款。",
      createdBy: "catalog-web",
      createdAt: "2026-09-03T09:00:00Z",
      updatedAt: "2026-09-03T09:00:00Z",
    }]);
    governanceMocks.submitProposal.mockResolvedValue({
      id: "prp_test_0001",
      targetObjectType: "semantic_asset",
      targetObjectId: netRevenue.id,
      assetId: netRevenue.id,
      state: "in_review",
      title: "修订净收入的计算表达式",
      summary: "修订计算表达式。",
      reason: "口径需要扣减确认退款。",
      createdBy: "catalog-web",
      createdAt: "2026-09-03T09:00:00Z",
      updatedAt: "2026-09-03T09:00:00Z",
      changeSet: [{ id: "chg_test_0001", fieldPath: "spec.expression", op: "update", beforeValue: netRevenue.revisionRecord.typeSpec.expression, afterValue: "SUM(paid_amount - confirmed_refund_amount)", createdAt: "2026-09-03T09:00:00Z" }],
    });
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    await user.click(screen.getByRole("button", { name: "打开语义资产 净收入" }));
    await user.click(screen.getByRole("button", { name: "修订知识" }));
    await user.click(within(screen.getByRole("dialog", { name: "选择知识修订对象" })).getByRole("button", { name: /计算表达式/ }));
    const workbench = screen.getByRole("region", { name: "净收入 知识修订工作台" });
    const expression = within(workbench).getByRole("textbox", { name: "计算表达式候选值" });
    await user.clear(expression);
    await user.type(expression, "SUM(paid_amount - confirmed_refund_amount)");
    await user.type(within(workbench).getByRole("textbox", { name: "知识修订原因" }), "口径需要扣减确认退款。");
    await user.click(within(workbench).getByRole("button", { name: "运行检查" }));
    await user.click(within(workbench).getByRole("button", { name: "提交审核" }));

    expect(governanceMocks.createProposal).toHaveBeenCalledTimes(1);
    const request = governanceMocks.createProposal.mock.calls[0][1];
    expect(request.targetObjectType).toBe("semantic_asset");
    expect(request.targetObjectId).toBe(netRevenue.id);
    expect(request.createdBy).toBe("catalog-web");
    expect(request.reason).toBe("口径需要扣减确认退款。");
    expect(request.changeSet).toEqual([expect.objectContaining({
      fieldPath: "spec.expression",
      op: "update",
      beforeValue: netRevenue.revisionRecord.typeSpec.expression,
      afterValue: "SUM(paid_amount - confirmed_refund_amount)",
    })]);
    expect(governanceMocks.submitProposal).toHaveBeenCalledWith(expect.any(String), "prp_test_0001");
    expect(await screen.findByRole("region", { name: "净收入 @13 候选资产版本详情" })).toBeVisible();
  });

  it("never echoes credential material when creating a model provider", async () => {
    const user = userEvent.setup();
    governanceMocks.createModelProvider.mockResolvedValue({
      provider: {
        id: "prv_test_0001",
        protocol: "openai_compatible",
        displayName: "企业向量网关",
        baseUrl: "https://models.example.com/v1",
        credentialEnv: "SEMLIA_COMPAT_GATEWAY_KEY",
        credentialRevision: "f1xture0001a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a9b8c7d6",
        enabled: true,
        createdAt: "2026-09-03T09:00:00Z",
        updatedAt: "2026-09-03T09:00:00Z",
      },
      models: [],
    });
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    await user.click(await screen.findByRole("button", { name: /模型配置/ }));
    const configuration = await screen.findByRole("region", { name: "模型配置" });
    await user.click(within(configuration).getByRole("button", { name: "添加供应商" }));
    const dialog = screen.getByRole("dialog", { name: "添加模型供应商" });
    await user.type(within(dialog).getByLabelText("配置名称"), "企业向量网关");
    await user.selectOptions(within(dialog).getByLabelText("供应商"), "openai_compatible");
    await user.type(within(dialog).getByLabelText("Base URL"), "https://models.example.com/v1");
    await user.type(within(dialog).getByLabelText("凭据环境变量"), "SEMLIA_COMPAT_GATEWAY_KEY");
    const secretInput = within(dialog).getByLabelText("API Key");
    expect(secretInput).toHaveAttribute("type", "password");
    await user.type(secretInput, "sk-super-secret-42");
    await user.click(within(dialog).getByRole("button", { name: "添加供应商" }));

    expect(governanceMocks.createModelProvider).toHaveBeenCalledTimes(1);
    const request = governanceMocks.createModelProvider.mock.calls[0][1];
    expect(request.credential).toBe("sk-super-secret-42");
    expect(request.credentialEnv).toBe("SEMLIA_COMPAT_GATEWAY_KEY");
    const response = await governanceMocks.createModelProvider.mock.results[0].value;
    expect(response).not.toHaveProperty("credential");
    expect(await screen.findByText("企业向量网关")).toBeVisible();
    expect(screen.getByText(/SEMLIA_COMPAT_GATEWAY_KEY/)).toBeVisible();
    expect(document.body.textContent).not.toContain("sk-super-secret-42");
  });

  it("surfaces a helpful empty state when no default LLM is configured", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    await user.click(screen.getByRole("button", { name: "打开语义资产 净收入" }));
    await user.click(screen.getByRole("button", { name: "修订知识" }));
    await user.click(within(screen.getByRole("dialog", { name: "选择知识修订对象" })).getByRole("button", { name: /计算表达式/ }));
    const workbench = screen.getByRole("region", { name: "净收入 知识修订工作台" });
    await user.click(within(workbench).getByRole("button", { name: "AI 提案" }));

    const dialog = await screen.findByRole("dialog", { name: "让模型起草 净收入 的知识修订" });
    expect(within(dialog).getByText("尚未配置可用的默认 LLM 模型")).toBeVisible();
    await user.click(within(dialog).getByRole("button", { name: "打开模型配置" }));
    expect(await screen.findByRole("region", { name: "模型配置" })).toBeVisible();
    expect(screen.queryByRole("dialog", { name: "让模型起草 净收入 的知识修订" })).not.toBeInTheDocument();
  });

  it("renders provider failures of AI generation with the stable error code", async () => {
    const user = userEvent.setup();
    governanceMocks.listModelProviders.mockResolvedValue([{
      provider: { id: "prv_test_llm", protocol: "openai", displayName: "OpenAI Enterprise", credentialEnv: "SEMLIA_OPENAI_API_KEY", credentialRevision: "f1xture0001a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a9b8c7d6", enabled: true, createdAt: "2026-09-03T09:00:00Z", updatedAt: "2026-09-03T09:00:00Z" },
      models: [{ id: "mdl_test_llm", providerId: "prv_test_llm", kind: "llm", model: "gpt-4.1", enabled: true, isDefault: true, capability: "推理", tokenLimit: 128000, createdAt: "2026-09-03T09:00:00Z", updatedAt: "2026-09-03T09:00:00Z" }],
    }]);
    governanceMocks.generateProposal.mockRejectedValue(new GovernanceApiError("provider dial failed: connection refused", "PROVIDER_UNAVAILABLE"));
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    await user.click(screen.getByRole("button", { name: "打开语义资产 净收入" }));
    await user.click(screen.getByRole("button", { name: "修订知识" }));
    await user.click(within(screen.getByRole("dialog", { name: "选择知识修订对象" })).getByRole("button", { name: /计算表达式/ }));
    const workbench = screen.getByRole("region", { name: "净收入 知识修订工作台" });
    await user.click(within(workbench).getByRole("button", { name: "AI 提案" }));

    const dialog = await screen.findByRole("dialog", { name: "让模型起草 净收入 的知识修订" });
    await user.type(within(dialog).getByRole("textbox", { name: "AI 生成指令" }), "补充退款口径边界");
    await user.click(within(dialog).getByRole("button", { name: "生成提案草稿" }));

    const failure = await within(dialog).findByRole("alert");
    expect(failure).toHaveTextContent("PROVIDER_UNAVAILABLE");
    expect(failure).toHaveTextContent("provider dial failed: connection refused");
    expect(failure).toHaveTextContent("模型服务暂不可用");
    expect(governanceMocks.generateProposal).toHaveBeenCalledTimes(1);
  });

  it("renders the generated agent-attributed proposal and submits it through the governed path", async () => {
    const user = userEvent.setup();
    governanceMocks.listModelProviders.mockResolvedValue([{
      provider: { id: "prv_test_llm", protocol: "openai", displayName: "OpenAI Enterprise", credentialEnv: "SEMLIA_OPENAI_API_KEY", credentialRevision: "f1xture0001a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a9b8c7d6", enabled: true, createdAt: "2026-09-03T09:00:00Z", updatedAt: "2026-09-03T09:00:00Z" },
      models: [{ id: "mdl_test_llm", providerId: "prv_test_llm", kind: "llm", model: "gpt-4.1", enabled: true, isDefault: true, capability: "推理", tokenLimit: 128000, createdAt: "2026-09-03T09:00:00Z", updatedAt: "2026-09-03T09:00:00Z" }],
    }]);
    governanceMocks.generateProposal.mockResolvedValue({
      agentRun: { id: "agr_test_0001", model: "gpt-4.1", configRevision: "f1xture0001/mdl_test_llm", inputHash: "f1xture0002a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a9b8c7d6", status: "succeeded", outputDigest: "f1xture0003a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a9b8c7d6", costMicros: 320, startedAt: "2026-09-03T09:00:00Z", finishedAt: "2026-09-03T09:00:02Z", durationMs: 1840 },
      proposal: {
        id: "prp_test_0002",
        targetObjectType: "semantic_asset",
        targetObjectId: netRevenue.id,
        assetId: netRevenue.id,
        state: "draft",
        title: "AI 草案：净收入",
        summary: "补充退款口径边界。",
        reason: "补充退款口径边界。",
        agentRunId: "agr_test_0001",
        createdBy: "semlia-agent",
        createdAt: "2026-09-03T09:00:00Z",
        updatedAt: "2026-09-03T09:00:00Z",
        changeSet: [{ id: "chg_test_0002", fieldPath: "definition.boundary", op: "update", beforeValue: netRevenue.revisionRecord.definition, afterValue: "补充退款口径边界。", createdAt: "2026-09-03T09:00:00Z" }],
      },
    });
    governanceMocks.listProposals.mockResolvedValue([{
      id: "prp_test_0002",
      targetObjectType: "semantic_asset",
      targetObjectId: netRevenue.id,
      assetId: netRevenue.id,
      state: "in_review",
      title: "AI 草案：净收入",
      summary: "补充退款口径边界。",
      reason: "补充退款口径边界。",
      agentRunId: "agr_test_0001",
      createdBy: "semlia-agent",
      createdAt: "2026-09-03T09:00:00Z",
      updatedAt: "2026-09-03T09:00:00Z",
    }]);
    governanceMocks.submitProposal.mockResolvedValue({
      id: "prp_test_0002",
      targetObjectType: "semantic_asset",
      targetObjectId: netRevenue.id,
      assetId: netRevenue.id,
      state: "in_review",
      title: "AI 草案：净收入",
      summary: "补充退款口径边界。",
      reason: "补充退款口径边界。",
      agentRunId: "agr_test_0001",
      createdBy: "semlia-agent",
      createdAt: "2026-09-03T09:00:00Z",
      updatedAt: "2026-09-03T09:00:00Z",
      changeSet: [{ id: "chg_test_0002", fieldPath: "definition.boundary", op: "update", beforeValue: netRevenue.revisionRecord.definition, afterValue: "补充退款口径边界。", createdAt: "2026-09-03T09:00:00Z" }],
    });
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识资产" }));
    await user.click(screen.getByRole("button", { name: "打开语义资产 净收入" }));
    await user.click(screen.getByRole("button", { name: "修订知识" }));
    await user.click(within(screen.getByRole("dialog", { name: "选择知识修订对象" })).getByRole("button", { name: /计算表达式/ }));
    const workbench = screen.getByRole("region", { name: "净收入 知识修订工作台" });
    await user.click(within(workbench).getByRole("button", { name: "AI 提案" }));

    const dialog = await screen.findByRole("dialog", { name: "让模型起草 净收入 的知识修订" });
    await user.type(within(dialog).getByRole("textbox", { name: "AI 生成指令" }), "补充退款口径边界");
    await user.click(within(dialog).getByRole("button", { name: "生成提案草稿" }));

    expect(await within(dialog).findByText("AI 草案：净收入")).toBeVisible();
    expect(within(dialog).getByText("agr_test_0001")).toBeVisible();
    expect(within(dialog).getByText("definition.boundary")).toBeVisible();
    await user.click(within(dialog).getByRole("button", { name: "提交 AI 提案" }));
    expect(governanceMocks.submitProposal).toHaveBeenCalledWith(expect.any(String), "prp_test_0002");
    expect(await screen.findByRole("region", { name: "净收入 @13 候选资产版本详情" })).toBeVisible();
  });

  it("confirms review batches with a mandatory reason", async () => {
    const user = userEvent.setup();
    governanceMocks.listReviewBatches.mockResolvedValue([{
      id: "rvb_test_0001",
      status: "open",
      groupingRule: { targetObjectType: "semantic_asset", diffCategory: "definition", matchedRuleId: "rule.g1.low_evidence" },
      policyVersion: "2026.08.4",
      memberCount: 1,
      createdBy: "catalog-web",
      createdAt: "2026-09-03T09:00:00Z",
    }]);
    governanceMocks.getReviewBatch.mockResolvedValue({
      id: "rvb_test_0001",
      status: "open",
      groupingRule: { targetObjectType: "semantic_asset", diffCategory: "definition", matchedRuleId: "rule.g1.low_evidence" },
      policyVersion: "2026.08.4",
      memberCount: 1,
      createdBy: "catalog-web",
      createdAt: "2026-09-03T09:00:00Z",
      maxRiskProposalId: "prp_test_0009",
      members: [{ proposalId: "prp_test_0009", addedReason: { matchedRuleId: "rule.g1.low_evidence", riskLevel: "low", reasonCode: "EVIDENCE_ONLY", ruleVersion: "2026.08.4", inputsDigest: "f1xture0001a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6a9b8c7d6" }, sample: true, splitOut: false, createdAt: "2026-09-03T09:00:00Z" }],
      samples: [],
      exclusions: [],
    });
    governanceMocks.confirmReviewBatch.mockResolvedValue({
      id: "rvb_test_0001",
      status: "confirmed",
      groupingRule: { targetObjectType: "semantic_asset", diffCategory: "definition", matchedRuleId: "rule.g1.low_evidence" },
      policyVersion: "2026.08.4",
      memberCount: 1,
      createdBy: "catalog-web",
      decidedBy: "prn_test_reviewer",
      decidedAt: "2026-09-03T09:05:00Z",
      createdAt: "2026-09-03T09:00:00Z",
      maxRiskProposalId: "prp_test_0009",
      members: [],
      samples: [],
      exclusions: [],
    });
    render(<App />);

    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    await user.click(await screen.findByRole("button", { name: "汇编批次" }));
    expect(governanceMocks.assembleReviewBatches).toHaveBeenCalledTimes(1);
    await user.click(await screen.findByRole("button", { name: /rvb_test_0001 · 1 个成员/ }));
    await user.click(await screen.findByRole("button", { name: "确认批次" }));
    const dialog = screen.getByRole("dialog", { name: "确认 1 个提案" });
    expect(within(dialog).getByRole("button", { name: "批准批次" })).toBeDisabled();
    await user.type(within(dialog).getByRole("textbox", { name: "批次确认意见" }), "样本抽查通过，批量批准。");
    await user.click(within(dialog).getByRole("button", { name: "批准批次" }));
    expect(governanceMocks.confirmReviewBatch).toHaveBeenCalledWith(expect.any(String), "rvb_test_0001", { decision: "approve", reason: "样本抽查通过，批量批准。" });
    expect(await screen.findByRole("status")).toHaveTextContent("评审批次已确认：批准");
  });

  it("reflects real proposal states through the catalog workflow seam", async () => {
    governanceMocks.listProposals.mockResolvedValue([{
      id: "prp_test_0004",
      targetObjectType: "semantic_asset",
      targetObjectId: netRevenue.id,
      assetId: netRevenue.id,
      state: "validating",
      title: "修订净收入的业务定义",
      summary: "补充退款边界。",
      reason: "补充退款边界。",
      createdBy: "catalog-web",
      createdAt: "2026-09-03T09:00:00Z",
      updatedAt: "2026-09-03T09:00:00Z",
    }]);
    function WorkflowProbe() {
      const runtime = useCatalogRuntime();
      const projected = runtime.assets.find((asset) => asset.id === netRevenue.id)?.revisionRecord.workflowState ?? "none";
      const untouched = runtime.assets.find((asset) => asset.name === "客单价")?.revisionRecord.workflowState ?? "none";
      return <div data-testid="workflow-probe">{projected}|{untouched}</div>;
    }
    render(<CatalogRuntimeProvider fixtureAssets={assets}><WorkflowProbe /><ProductApp /></CatalogRuntimeProvider>);
    await screen.findByRole("button", { name: "系统设置" });
    await vi.waitFor(() => {
      expect(screen.getByTestId("workflow-probe")).toHaveTextContent(/^proposed\|in_review$/);
    });
  });

  it("renders the separation-of-duty explanation returned by the review API", async () => {
    const user = userEvent.setup();
    governanceMocks.listProposals.mockResolvedValue([{
      id: "prp_test_0003",
      targetObjectType: "semantic_asset",
      targetObjectId: netRevenue.id,
      assetId: netRevenue.id,
      state: "in_review",
      title: "修订净收入的业务定义",
      summary: "补充退款边界。",
      reason: "补充退款边界。",
      createdBy: "catalog-web",
      createdAt: "2026-09-03T09:00:00Z",
      updatedAt: "2026-09-03T09:00:00Z",
    }]);
    governanceMocks.createReview.mockRejectedValue(new GovernanceApiError("the proposal author cannot review their own proposal", "SEPARATION_OF_DUTY", { conflict: "the proposal author cannot review their own proposal", scope: "workspace", policySource: "docs/SSOT.md §8.4 + docs/specs/access-control/product-design.md FR-007" }));
    render(<App />);

    await user.click(screen.getByRole("button", { name: "变更与发布" }));
    await user.click(await screen.findByRole("button", { name: "查看候选资产版本 净收入 @13" }));
    await user.click(await screen.findByRole("button", { name: "审核候选版本 净收入 @13" }));
    const dialog = screen.getByRole("dialog", { name: "审核 净收入 · @13" });
    await user.type(within(dialog).getByRole("textbox", { name: "审核意见" }), "尝试批准自己的提案。");
    await user.click(within(dialog).getByRole("button", { name: "批准版本" }));

    const conflict = await within(dialog).findByRole("alert");
    expect(conflict).toHaveTextContent("职责分离冲突");
    expect(conflict).toHaveTextContent("SEPARATION_OF_DUTY");
    expect(conflict).toHaveTextContent("the proposal author cannot review their own proposal");
    expect(conflict).toHaveTextContent("FR-007");
  });
});

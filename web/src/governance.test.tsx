import { act, render, renderHook, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, vi } from "vitest";

import * as catalog from "./catalog";
import type { KnowledgeSpec } from "./knowledge";
import * as production from "./semanticProduction";
import { CatalogRuntimeProvider } from "./testing/catalogFixture";
import { useCatalogRuntime } from "./catalogRuntime";
import { ProductApp } from "./ProductApp";
import { GovernanceApiError } from "./governance";
import { GovernanceRuntimeProvider, useGovernanceRuntime } from "./governanceRuntime";
import { assets, authorizationSession } from "./testing/data";
import type { Asset } from "./types";

vi.mock("./sessionRuntime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./sessionRuntime")>();
  return { ...actual, useSessionRuntime: () => ({ activeWorkspaceId: "wsp_01arz3ndektsv4rrffq69g5fav", session: null }) };
});

const governanceMocks = vi.hoisted(() => ({
  listProposals: vi.fn(),
  createProposal: vi.fn(),
  getProposal: vi.fn(),
  submitProposal: vi.fn(),
  listValidationRuns: vi.fn(),
  getPolicyDecision: vi.fn(),
  createReview: vi.fn(),
  listReviews: vi.fn(),
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

function App({ session = authorizationSession }: { session?: Parameters<typeof ProductApp>[0]["session"] }) {
  return <CatalogRuntimeProvider fixtureAssets={assets}><ProductApp session={session} /></CatalogRuntimeProvider>;
}

function emptyWorkspace() {
  governanceMocks.listProposals.mockResolvedValue([]);
  governanceMocks.listReleases.mockResolvedValue({ items: [], page: { limit: 50, total: 0 } });
  governanceMocks.listReviewBatches.mockResolvedValue([]);
  governanceMocks.listModelProviders.mockResolvedValue([]);
}

beforeEach(() => {
  window.history.replaceState({}, "", "/");
  vi.clearAllMocks();
  emptyWorkspace();
  governanceMocks.getProposal.mockResolvedValue({});
  governanceMocks.getRelease.mockResolvedValue({});
  governanceMocks.listValidationRuns.mockResolvedValue([]);
  governanceMocks.getPolicyDecision.mockResolvedValue(null);
  governanceMocks.listReviews.mockResolvedValue([]);
});

describe("governance API convergence", () => {
  it("checkpoints a created draft before attempting submission", async () => {
    governanceMocks.createProposal.mockResolvedValue({ id: "prp_checkpoint", state: "draft" });
    governanceMocks.submitProposal.mockRejectedValue(new Error("submit unavailable"));
    const created = vi.fn();
    const { result } = renderHook(() => useGovernanceRuntime(), { wrapper: ({ children }) => <GovernanceRuntimeProvider workspaceId="wsp_checkpoint">{children}</GovernanceRuntimeProvider> });
    await act(async () => {
      await expect(result.current.createAndSubmitProposal({ targetObjectType: "semantic_asset", targetObjectId: "asset", title: "Candidate", summary: "Evidence", reason: "Reviewed", changeSet: [], createdBy: "test" }, created)).rejects.toThrow("submit unavailable");
    });
    expect(created).toHaveBeenCalledWith(expect.objectContaining({ id: "prp_checkpoint" }));
    expect(created.mock.invocationCallOrder[0]).toBeLessThan(governanceMocks.submitProposal.mock.invocationCallOrder[0]);
  });

  it("does not submit when the draft checkpoint cannot be saved", async () => {
    governanceMocks.createProposal.mockResolvedValue({ id: "prp_checkpoint", state: "draft" });
    const { result } = renderHook(() => useGovernanceRuntime(), { wrapper: ({ children }) => <GovernanceRuntimeProvider workspaceId="wsp_checkpoint">{children}</GovernanceRuntimeProvider> });
    await act(async () => {
      await expect(result.current.createAndSubmitProposal({ targetObjectType: "semantic_asset", targetObjectId: "asset", title: "Candidate", summary: "Evidence", reason: "Reviewed", changeSet: [], createdBy: "test" }, () => { throw new Error("checkpoint unavailable"); })).rejects.toThrow("checkpoint unavailable");
    });
    expect(governanceMocks.submitProposal).not.toHaveBeenCalled();
  });

  it("renders governance failures as error states and never falls back to fixture data", async () => {
    const user = userEvent.setup();
    governanceMocks.listProposals.mockRejectedValue(new GovernanceApiError("governance storage unreachable", "CONFLICT"));
    governanceMocks.listReleases.mockRejectedValue(new GovernanceApiError("governance storage unreachable", "CONFLICT"));
    governanceMocks.listModelProviders.mockRejectedValue(new GovernanceApiError("governance storage unreachable", "CONFLICT"));
    render(<App />);

    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^批量审核/ }));
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

  it("loads release history with the server cursor and authoritative total", async () => {
    const first = { id: "rls_01arz3ndektsv4rrffq69g5fav", sequence: 2, state: "published" as const, manifestDigest: `sha256:${"a".repeat(64)}`, publishedBy: "prn_publisher", publishedAt: "2026-09-05T04:00:00Z", createdAt: "2026-09-05T04:00:00Z" };
    const second = { ...first, id: "rls_01arz3ndektsv4rrffq69g5faw", sequence: 1, manifestDigest: `sha256:${"b".repeat(64)}`, publishedAt: "2026-09-04T04:00:00Z", createdAt: "2026-09-04T04:00:00Z" };
    governanceMocks.listReleases.mockImplementation(async (_workspaceId: string, cursor?: string) => cursor
      ? { items: [second], page: { limit: 1, total: 2 } }
      : { items: [first], page: { limit: 1, total: 2, nextCursor: "release-next" } });
    governanceMocks.getRelease.mockImplementation(async (_workspaceId: string, releaseId: string) => {
      const release = releaseId === first.id ? first : second;
      return { ...release, manifest: { assets: [], objects: [] }, authority: "release_manifests", availability: "available", objectAvailability: "available", consumerImpactAvailability: "available", diffAvailability: "available", consumerImpact: { current: 0, pinned: 0 }, priorPinDiff: [], currentRegistryDiff: [] };
    });
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识库" }));
    await user.click(screen.getByRole("button", { name: "发布记录" }));
    await vi.waitFor(() => expect(document.querySelector(".governance-list-summary")).toHaveTextContent("发布记录总计 2"));
    await user.click(screen.getByRole("button", { name: "加载更多发布记录" }));
    expect(await screen.findByRole("button", { name: /查看发布记录 #1/ })).toBeVisible();
    expect(governanceMocks.listReleases).toHaveBeenLastCalledWith(expect.any(String), "release-next");
  });

  it("shows an explicit release-detail failure instead of an endless loading claim", async () => {
    const release = { id: "rls_01arz3ndektsv4rrffq69g5fav", sequence: 1, state: "published" as const, manifestDigest: `sha256:${"a".repeat(64)}`, publishedBy: "prn_publisher", publishedAt: "2026-09-05T04:00:00Z", createdAt: "2026-09-05T04:00:00Z" };
    governanceMocks.listReleases.mockResolvedValue({ items: [release], page: { limit: 1, total: 1 } });
    governanceMocks.getRelease.mockRejectedValue(new GovernanceApiError("release manifest store unavailable", "DEPENDENCY_UNAVAILABLE"));
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识库" }));
    await user.click(screen.getByRole("button", { name: "发布记录" }));
    const releaseRow = await screen.findByRole("button", { name: /查看发布记录 #1 #1/ });
    expect(releaseRow).toHaveTextContent("清单读取失败");
    expect(releaseRow).toHaveTextContent("release manifest store unavailable");
    expect(releaseRow).not.toHaveTextContent("清单加载中");

    await user.click(releaseRow);
    const failure = await screen.findByRole("alert");
    expect(failure).toHaveTextContent("发布记录加载失败");
    expect(failure).toHaveTextContent("release manifest store unavailable");
    expect(governanceMocks.getRelease).toHaveBeenCalledTimes(2);
  });

  it("saves a pinned calculation revision as one production draft before explicit confirmation", async () => {
    const user = userEvent.setup();
    const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
    const digest = `sha256:${"a".repeat(64)}`;
    const paid = { assetId: "ast_paid", revisionId: "rev_paid", releaseId: "rls_paid" };
    const refund = { assetId: "ast_refund", revisionId: "rev_refund", releaseId: "rls_refund" };
    const beforeSpec: KnowledgeSpec = {
      kind: "derived", unit: "CNY", nullPolicy: "exclude", zeroDenominator: "null", rollup: "recompute_from_inputs",
      expression: { op: "add", left: { op: "ref", ref: paid }, right: { op: "ref", ref: refund } },
    };
    const afterSpec: KnowledgeSpec = { ...beforeSpec, expression: { ...beforeSpec.expression!, op: "subtract" } };
    const asset: Asset = { ...netRevenue, knowledgeSpec: beforeSpec, revisionRecord: { ...netRevenue.revisionRecord, contentHash: digest } };
    const content: production.AssetContent = {
      address: asset.key, assetType: "metric", displayName: asset.name, definition: asset.definition,
      scope: "合成财务验收", ownerPrincipalId: authorizationSession.principalId, spec: beforeSpec,
    };
    const revision = {
      id: asset.revisionRecord.revisionId, assetId: asset.id, sequence: asset.revisionRecord.sequence,
      schemaVersion: "1.0.0", contentDigest: digest, content, createdAt: "2026-09-03T09:00:00Z", createdBy: authorizationSession.principalId, evidence: [],
    };
    const input: production.ProductionInput = {
      snapshots: [{ sourceId: "src_orders", snapshotId: "snp_orders", digest, coverageKeys: ["public"] }],
      candidates: [{ candidateId: "cand_consumed", snapshotId: "snp_orders", digest, targetKeys: ["metric"], primaryTargetKey: "metric" }],
      evidence: [{ evidenceId: "evd_orders", snapshotId: "snp_orders", digest }],
      dependencies: [paid, refund].map((ref) => ({ kind: "semantic_asset", targetId: ref.assetId, revisionId: ref.revisionId, releaseId: ref.releaseId })),
    };
    const expectedDraft: production.ProductionDraft = {
      input: { ...input, candidates: [] },
      targets: [{ intent: "update", kind: "semantic_asset", localKey: "revision", title: asset.name,
        targetId: asset.id, baseRevisionId: revision.id, content: { ...content, spec: afterSpec },
        changes: [{ fieldPath: "spec", op: "update", beforeValue: beforeSpec, afterValue: afterSpec }], evidenceIds: ["evd_orders"] }],
    };
    const origin = {
      summary: { id: "prodop_origin" }, version: 3, setDigest: digest, input,
      targets: [{ localKey: "metric", targetId: asset.id, outcome: "proposal", contentDigest: digest, proposalId: "prp_origin", declaration: { kind: "semantic_asset", evidenceIds: ["evd_orders"] } }],
    } as unknown as production.ProductionOperation;
    const savedOperation = {
      summary: { id: "prodop_revision", createdBy: authorizationSession.principalId, progress: "draft", frozen: false },
      version: 1, setDigest: digest, inputDigest: digest, input: expectedDraft.input,
      targets: [{ localKey: "revision", targetId: asset.id, outcome: "proposal", proposalId: "prp_revision", declaration: expectedDraft.targets[0] }],
      generationRunIds: [], generationApplications: [], activeValidation: { status: "not_requested" }, unresolvedCodes: [],
    } as unknown as production.ProductionOperation;
    const detailFor = (currentRevision: catalog.CatalogRevision, title: string, address: string, releaseId: string): catalog.CatalogAssetDetail => ({
      id: currentRevision.assetId, title, address, assetType: "metric", lifecycleState: "active", summary: "合成验收",
      createdAt: revision.createdAt, updatedAt: revision.createdAt, relationCount: 0, currentRevisionId: currentRevision.id, currentRevision,
      authoritySections: [{ kind: "released_state", authority: "release_manifests", availability: "available", releaseId, revisionId: currentRevision.id,
        values: {}, records: [], recordsPage: { limit: 50, total: 0 } }],
    });
    const references = [paid, refund].map((ref, index) => {
      const title = index === 0 ? "支付金额" : "确认退款";
      const address = index === 0 ? "commerce.paid" : "commerce.refund";
      return detailFor({ ...revision, id: ref.revisionId, assetId: ref.assetId,
        content: { ...content, address, displayName: title, spec: { kind: "aggregate", aggregation: "sum", unit: "CNY", nullPolicy: "exclude",
          inputRef: { assetId: "ast_orders", revisionId: "rev_orders", releaseId: "rls_orders", memberId: index === 0 ? "paid_amount" : "confirmed_refund_amount" } } },
      }, title, address, ref.releaseId);
    });
    const getAsset = vi.spyOn(catalog, "getAsset").mockImplementation(async (_workspace, id) => {
      if (id === asset.id) return detailFor(revision, asset.name, asset.key, "rls_origin");
      const detail = references.find((value) => value.id === id);
      if (!detail) throw new Error(`Unexpected catalog asset ${id}`);
      return detail;
    });
    vi.spyOn(catalog, "listAssets").mockResolvedValue({ items: references, page: { limit: 50, total: 2 } });
    const getRevision = vi.spyOn(catalog, "getAssetRevision").mockResolvedValue(revision);
    const release = vi.spyOn(production.productionAPI, "release").mockResolvedValue({
      id: "rls_origin", attribution: { operationId: origin.summary.id, version: origin.version, setDigest: digest, role: "applied", proposalIds: ["prp_origin"] },
      protection: { rootReleaseId: "rls_origin", rollbackDepth: 0 }, rolledBackReleaseId: null,
      beforeHead: { presence: "absent" }, afterManifest: { assets: [{ assetId: asset.id, revisionId: revision.id }] },
    } as production.ProductionRelease);
    const getOperation = vi.spyOn(production.productionAPI, "get").mockImplementation(async (_workspace, id) => {
      if (id === origin.summary.id) return origin;
      if (id === savedOperation.summary.id) return savedOperation;
      throw new Error(`Unexpected production operation ${id}`);
    });
    const getSnapshot = vi.spyOn(production, "getProductionSnapshot").mockResolvedValue({
      id: "snp_orders", sourceId: "src_orders", sourceRevisionId: "srcv_orders", adapterVersion: "1.0", scopeDigest: digest,
      contentDigest: digest, historyQuality: "verified", coverageStatus: "complete", memberCount: 0, diagnosticCount: 0, createdAt: revision.createdAt,
      coverage: [{ key: "public", status: "complete", enumerationComplete: true, diagnosticCodes: [] }],
    });
    vi.spyOn(production, "listProductionMembers").mockResolvedValue({ snapshotId: "snp_orders", historyQuality: "verified", items: [], nextCursor: null });
    vi.spyOn(production.productionAPI, "list").mockResolvedValue({ items: [], nextCursor: null });
    vi.spyOn(production.productionAPI, "rules").mockResolvedValue([]);
    const forbiddenWrites = (["recordRule", "submit", "validate", "review", "publish", "replace", "generate"] as const).map((method) =>
      vi.spyOn(production.productionAPI, method).mockRejectedValue(new Error(`Unexpected automatic ${method}`)));
    let finishSave!: (result: Awaited<ReturnType<typeof production.productionAPI.create>>) => void;
    const create = vi.spyOn(production.productionAPI, "create").mockReturnValue(new Promise((resolve) => { finishSave = resolve; }));
    const view = render(<CatalogRuntimeProvider fixtureAssets={[asset]}><ProductApp session={authorizationSession} /></CatalogRuntimeProvider>);
    try {
      await user.click(screen.getByRole("button", { name: "知识库" }));
      await user.click(screen.getByRole("button", { name: "打开语义资产 净收入" }));
      await user.click(screen.getByRole("button", { name: "修订知识" }));
      await user.click(within(screen.getByRole("dialog", { name: "选择知识修订对象" })).getByRole("button", { name: /计算表达式/ }));
      const workbench = screen.getByRole("region", { name: "净收入 知识修订工作台" });
      await user.selectOptions(within(workbench).getByRole("combobox", { name: "派生计算算子" }), "subtract");
      for (const [side, title] of [["左项", "支付金额"], ["右项", "确认退款"]]) {
        await user.selectOptions(within(workbench).getByRole("combobox", { name: `派生计算${side}算子` }), "ref");
        await user.click(within(workbench).getByRole("button", { name: `选择派生计算${side}引用` }));
        const picker = screen.getByRole("dialog", { name: `选择派生计算${side}引用` });
        await user.click(await within(picker).findByRole("button", { name: new RegExp(title) }));
        await user.click(within(picker).getByRole("button", { name: "固定此版本" }));
      }
      await user.type(within(workbench).getByRole("textbox", { name: "知识修订原因" }), "口径需要扣减确认退款。");
      await user.click(within(workbench).getByRole("button", { name: "运行检查" }));
      const previousPath = window.location.pathname;
      const saveButton = within(workbench).getByRole("button", { name: "保存修订并继续确认" });
      await user.click(saveButton);
      await vi.waitFor(() => expect(create).toHaveBeenCalledTimes(1));
      expect(create).toHaveBeenCalledWith(workspaceId, expectedDraft, expect.stringMatching(/^web-revision-/), expect.any(AbortSignal));
      expect(getAsset).toHaveBeenLastCalledWith(workspaceId, asset.id, expect.any(AbortSignal));
      expect(getRevision).toHaveBeenCalledWith(workspaceId, asset.id, revision.id, expect.any(AbortSignal));
      expect(release).toHaveBeenCalledWith(workspaceId, "rls_origin", expect.any(AbortSignal));
      expect(getOperation).toHaveBeenCalledWith(workspaceId, origin.summary.id, expect.any(AbortSignal), 3);
      expect(getSnapshot).toHaveBeenCalledWith(workspaceId, "src_orders", "snp_orders", expect.any(AbortSignal));
      expect(saveButton).toBeDisabled();
      expect(workbench).toBeVisible();
      expect(window.location.pathname).toBe(previousPath);
      expect(screen.queryByRole("region", { name: "知识确认" })).not.toBeInTheDocument();
      expect(forbiddenWrites.every((write) => write.mock.calls.length === 0)).toBe(true);
      await act(async () => { finishSave({ operationId: "prodop_revision", version: 1 } as Awaited<ReturnType<typeof production.productionAPI.create>>); });
      expect(await screen.findByRole("region", { name: "知识确认" })).toBeVisible();
      expect(window.location.pathname).toBe("/work/operations/prodop_revision");
      expect(await screen.findByRole("region", { name: "未确认的修订上下文" })).toHaveTextContent("口径需要扣减确认退款。");
      expect(await screen.findByRole("textbox", { name: "业务规则声明" })).toHaveValue("");
      expect(screen.getByRole("checkbox", { name: "确认声明支持当前定义与范围" })).not.toBeChecked();
      expect(screen.getByRole("button", { name: "记录业务确认" })).toBeDisabled();
      expect(create).toHaveBeenCalledTimes(1);
      expect(forbiddenWrites.every((write) => write.mock.calls.length === 0)).toBe(true);
      expect(governanceMocks.createProposal).not.toHaveBeenCalled();
      expect(governanceMocks.submitProposal).not.toHaveBeenCalled();
      expect(governanceMocks.createReview).not.toHaveBeenCalled();
      expect(governanceMocks.publishRelease).not.toHaveBeenCalled();
    } finally {
      view.unmount();
      vi.restoreAllMocks();
    }
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

    await user.click(screen.getByRole("button", { name: "知识库" }));
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

    await user.click(screen.getByRole("button", { name: "知识库" }));
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

    await user.click(screen.getByRole("button", { name: "知识库" }));
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

    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^批量审核/ }));
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
    render(<CatalogRuntimeProvider fixtureAssets={assets}><WorkflowProbe /><ProductApp session={authorizationSession} /></CatalogRuntimeProvider>);
    await screen.findByRole("button", { name: "系统设置" });
    await vi.waitFor(() => {
      expect(screen.getByTestId("workflow-probe")).toHaveTextContent(`${netRevenue.revisionRecord.workflowState}|${assets.find(asset => asset.name === "客单价")?.revisionRecord.workflowState}`);
    });
  });

  it("restores persisted approval facts when proposal data is reloaded", async () => {
    governanceMocks.listProposals.mockResolvedValue([{
      id: "prp_test_reviewed",
      targetObjectType: "semantic_asset",
      targetObjectId: netRevenue.id,
      assetId: netRevenue.id,
      state: "in_review",
      title: "修订净收入的业务定义",
      summary: "补充退款边界。",
      reason: "补充退款边界。",
      createdBy: "prn_test_author",
      createdAt: "2026-09-03T09:00:00Z",
      updatedAt: "2026-09-03T09:00:00Z",
    }]);
    governanceMocks.listReviews.mockResolvedValue([{
      id: "rvw_test_approved",
      proposalId: "prp_test_reviewed",
      reviewerPrincipalId: "prn_test_reviewer",
      channel: "expert",
      decision: "approved",
      note: "独立评审通过。",
      createdAt: "2026-09-03T10:00:00Z",
    }]);
    window.history.replaceState({}, "", "/governance?proposal=prp_test_reviewed");
    render(<App />);


    expect(await screen.findByText("待发布")).toBeInTheDocument();
    expect(governanceMocks.listReviews).toHaveBeenCalledWith(
      expect.any(String), "prp_test_reviewed", expect.any(AbortSignal));
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
    window.history.replaceState({}, "", "/governance?proposal=prp_test_0003");
    render(<App />);


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

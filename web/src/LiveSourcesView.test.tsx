import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, vi } from "vitest";

import { assets } from "./testing/data";
import type { DiscoveryRun, SemanticCandidate, SourceConnection, SourceDiscoveryRun } from "./discovery";
import { LiveSourcesView } from "./LiveSourcesView";
import { ProductionApiError, type ProductionOperation, type ProductionDraft } from "./semanticProduction";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";

const mocks = vi.hoisted(() => ({
  listSources: vi.fn(),
  createSource: vi.fn(),
  updateSource: vi.fn(),
  deleteSource: vi.fn(),
  rotateSourceCredential: vi.fn(),
  testSourceConnection: vi.fn(),
  listSourceRuns: vi.fn(),
  startSourceRun: vi.fn(),
  getDiscoveryRun: vi.fn(),
  listSemanticCandidates: vi.fn(),
  decideSemanticCandidate: vi.fn(),
  createAndSubmitProposal: vi.fn(),
  loadProposal: vi.fn(),
  submitProposal: vi.fn(),
  ensureAsset: vi.fn(),
  allowed: new Set(["source.read", "source.manage", "ingestion.run", "asset.propose"]),
  listArtifacts: vi.fn(),
  stageArtifact: vi.fn(),
  finalizeArtifactSet: vi.fn(),
  registerSQL: vi.fn(),
  getArtifactSet: vi.fn(),
  listSchedules: vi.fn(),
  createSchedule: vi.fn(),
  runScheduleNow: vi.fn(),
  listOccurrences: vi.fn(),
  getSemanticCandidate: vi.fn(),
  productionList: vi.fn(), productionGet: vi.fn(), productionCreate: vi.fn(), productionReplace: vi.fn(), productionSubmit: vi.fn(),
  getProductionSnapshot: vi.fn(), listProductionMembers: vi.fn(),
}));

vi.mock("./semanticProduction", async (importOriginal) => ({
  ...await importOriginal<typeof import("./semanticProduction")>(),
  getProductionSnapshot: mocks.getProductionSnapshot,
  listProductionMembers: mocks.listProductionMembers,
  productionAPI: { list: mocks.productionList, get: mocks.productionGet, create: mocks.productionCreate, replace: mocks.productionReplace, submit: mocks.productionSubmit, rules: vi.fn().mockResolvedValue([]) },
}));
vi.mock("./governance", async (importOriginal) => ({ ...await importOriginal<typeof import("./governance")>(), listModelProviders: vi.fn().mockResolvedValue([]) }));

vi.mock("./ingestion", async (importOriginal) => ({ ...await importOriginal<typeof import("./ingestion")>(), ingestionApi: {
  listArtifacts: mocks.listArtifacts, stageArtifact: mocks.stageArtifact, finalizeArtifactSet: mocks.finalizeArtifactSet,
  registerSQL: mocks.registerSQL, getArtifactSet: mocks.getArtifactSet, listSchedules: mocks.listSchedules,
  createSchedule: mocks.createSchedule, runScheduleNow: mocks.runScheduleNow, listOccurrences: mocks.listOccurrences,
} }));

vi.mock("./discovery", async (importOriginal) => ({
  ...await importOriginal<typeof import("./discovery")>(),
  listSources: mocks.listSources,
  createSource: mocks.createSource,
  updateSource: mocks.updateSource,
  deleteSource: mocks.deleteSource,
  rotateSourceCredential: mocks.rotateSourceCredential,
  testSourceConnection: mocks.testSourceConnection,
  listSourceRuns: mocks.listSourceRuns,
  startSourceRun: mocks.startSourceRun,
  getDiscoveryRun: mocks.getDiscoveryRun,
  listSemanticCandidates: mocks.listSemanticCandidates,
  decideSemanticCandidate: mocks.decideSemanticCandidate,
  getSemanticCandidate: mocks.getSemanticCandidate,
}));

vi.mock("./catalogRuntime", () => ({
  useCatalogRuntime: () => ({ workspaceId, assets, ensureAsset: mocks.ensureAsset }),
}));

vi.mock("./catalog", () => ({
  getAsset: vi.fn(async (_workspaceId: string, assetId: string) => {
    const asset = assets.find((item) => item.id === assetId)!;
    return {
      id: asset.id,
      address: `${asset.identity.namespace}.${asset.key}`,
      assetType: "metric",
      lifecycleState: "active",
      currentRevisionId: asset.revisionRecord.revisionId,
      title: asset.name,
      summary: asset.definition,
      createdAt: "2026-09-04T08:00:00Z",
      updatedAt: "2026-09-04T08:00:00Z",
      relationCount: 0,
      currentRevision: {
        id: asset.revisionRecord.revisionId,
        assetId: asset.id,
        sequence: asset.revisionRecord.sequence,
        schemaVersion: asset.revisionRecord.schemaVersion,
        contentDigest: asset.revisionRecord.contentHash,
        content: { definition: asset.definition },
        createdBy: "fixture",
        createdAt: "2026-09-04T08:00:00Z",
        evidence: [],
      },
    };
  }),
}));

vi.mock("./governanceRuntime", () => ({
  useGovernanceRuntime: () => ({ createAndSubmitProposal: mocks.createAndSubmitProposal, loadProposal: mocks.loadProposal, submitProposal: mocks.submitProposal }),
}));

vi.mock("./authorization", () => ({ useCan: (action: string) => mocks.allowed.has(action), useResourceCan: () => ({ principalId: "test-principal" }) }));

const source: SourceConnection = {
  id: "src_01arz3ndektsv4rrffq69g5fav",
  name: "Alpha Warehouse",
  sourceKind: "postgresql",
  version: 4,
  adapterKind: "postgresql_catalog",
  host: "warehouse.internal",
  port: 5432,
  database: "analytics",
  username: "metadata_ro",
  sslMode: "require",
  artifactPaths: [],
  status: "active",
  credentialVersion: 2,
  createdAt: "2026-09-04T08:00:00Z",
  updatedAt: "2026-09-04T08:00:00Z",
};

const run: SourceDiscoveryRun = {
  id: "run_01arz3ndektsv4rrffq69g5fav",
  sourceConnectionId: source.id,
  operationsPath: "/operations/runtime?run=run_01arz3ndektsv4rrffq69g5fav",
  credentialVersion: 2,
  status: "degraded",
  snapshotId: "snapshot-orders",
  stats: { datasets: 8, fields: 42 },
  createdAt: "2026-09-04T08:10:00Z",
  updatedAt: "2026-09-04T08:12:00Z",
};

let storedProduction: ProductionOperation | null = null;
function storeProduction(draft: ProductionDraft, version = 1) {
  storedProduction = {
    summary: { id: "operation-orders", currentVersion: version, createdBy: "test-principal", createdAt: "2026-09-11", updatedAt: "2026-09-11", frozen: false, progress: "draft", targetCount: draft.targets.length, releaseId: null },
    version, input: draft.input, inputDigest: "input", setDigest: "set", baselineHead: { presence: "absent" },
    targets: draft.targets.map((declaration) => ({ localKey: declaration.localKey, declaration, outcome: "changed", proposalId: null, targetId: null, targetRevisionId: null })),
    activeValidation: { status: "not_requested" }, generationRunIds: [], generationApplications: [], unresolvedCodes: [],
  } as unknown as ProductionOperation;
  return { operationId: storedProduction.summary.id, version, setDigest: "set" };
}

const candidate: SemanticCandidate = {
  id: "scd_01arz3ndektsv4rrffq69g5fav",
  sourceConnectionId: source.id,
  sourceRevisionId: "srv_01arz3ndektsv4rrffq69g5fav",
  discoveryRunId: run.id,
  candidateKey: "entity:public.orders",
  candidateKind: "data_asset",
  title: "public.orders",
  proposalInput: { qualifiedName: "public.orders", fields: [{ name: "order_id" }] },
  evidence: [{ locator: "postgresql://catalog/public.orders" }],
  contentDigest: `sha256:${"a".repeat(64)}`,
  status: "pending",
  createdAt: "2026-09-04T08:12:00Z",
  updatedAt: "2026-09-04T08:12:00Z",
};

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  window.history.replaceState({}, "", "/sources");
  storedProduction = null;
  mocks.productionList.mockImplementation(async () => ({ items: storedProduction ? [storedProduction.summary] : [], nextCursor: null }));
  mocks.productionGet.mockImplementation(async () => storedProduction);
  mocks.productionCreate.mockImplementation(async (_workspace: string, draft: ProductionDraft) => storeProduction(draft));
  mocks.productionReplace.mockImplementation(async (_workspace: string, _operation: string, draft: ProductionDraft) => storeProduction(draft, 2));
  mocks.productionSubmit.mockImplementation(async () => ({ operationId: "operation-orders", version: 1 }));
  mocks.getSemanticCandidate.mockResolvedValue(candidate);
  mocks.getProductionSnapshot.mockResolvedValue({ id: "snapshot-orders", sourceId: source.id, sourceRevisionId: candidate.sourceRevisionId, adapterVersion: "1", scopeDigest: "scope", contentDigest: "digest", historyQuality: "verified", coverageStatus: "complete", coverage: [{ key: "orders", status: "complete", enumerationComplete: true, diagnosticCodes: [] }], memberCount: 0, diagnosticCount: 0, createdAt: "2026-09-11" });
  mocks.listProductionMembers.mockResolvedValue({ snapshotId: "snapshot-orders", items: [], nextCursor: null });
  mocks.allowed = new Set(["source.read", "source.manage", "ingestion.run", "asset.propose"]);
  mocks.listArtifacts.mockResolvedValue({ items: [], total: 0, limit: 50 });
  mocks.listSchedules.mockResolvedValue({ items: [], total: 0, limit: 50 });
  mocks.listOccurrences.mockResolvedValue({ items: [], total: 0, limit: 50 });
  mocks.listSources.mockResolvedValue({ items: [source], total: 1, limit: 50 });
  mocks.listSourceRuns.mockResolvedValue({ items: [run], total: 1, limit: 50 });
  mocks.listSemanticCandidates.mockResolvedValue({ items: [candidate], total: 1, limit: 50 });
  mocks.ensureAsset.mockResolvedValue(undefined);
  mocks.testSourceConnection.mockResolvedValue(undefined);
  mocks.loadProposal.mockImplementation(async (id: string) => ({ id, state: "in_review", targetObjectId: assets[0].id, summary: `来源 ${candidate.sourceConnectionId} · revision ${candidate.sourceRevisionId} · run ${candidate.discoveryRunId} · candidate ${candidate.id}` }));
});

function renderView(focusIndex = 0, onOpenProposal = vi.fn(), initialSourceId?: string) {
  return render(<LiveSourcesView focusIndex={focusIndex} runDetailBackRequestEpoch={0} initialSourceId={initialSourceId} onRunDetailOpenChange={() => {}} onOpenProposal={onOpenProposal} />);
}

async function openProductionCandidate(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("tab", { name: /语义候选/ }));
  await user.click(screen.getByRole("button", { name: `查看语义候选 ${candidate.title}` }));
  await waitFor(() => expect(mocks.getProductionSnapshot).toHaveBeenCalled());
  await waitFor(() => expect(screen.queryByText("正在读取来源")).not.toBeInTheDocument());
}

describe("real source and candidate workspace", () => {
  it("recovers the server operation after the entire candidate workspace remounts", async () => {
    const user = userEvent.setup();
    const view = renderView(2);
    await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    await user.type(screen.getByLabelText("业务定义"), "Reviewed definition");
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    await screen.findByText("保存知识草稿已由服务器接收。");
    view.unmount(); localStorage.clear();
    renderView(2); await openProductionCandidate(user);
    await user.click(await screen.findByRole("button", { name: /建模中 · v1/ }));
    await waitFor(() => expect(screen.getByLabelText("业务定义")).toHaveValue("Reviewed definition"));
    expect(mocks.productionCreate).toHaveBeenCalledTimes(1);
    expect(mocks.createAndSubmitProposal).not.toHaveBeenCalled();
    expect(mocks.decideSemanticCandidate).not.toHaveBeenCalled();
  });

  it("reopens a persisted production draft without resubmitting it", async () => {
    const user = userEvent.setup();
    const view = renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    await screen.findByText("保存知识草稿已由服务器接收。");
    view.unmount(); renderView(2); await openProductionCandidate(user);
    await user.click(await screen.findByRole("button", { name: /建模中 · v1/ }));
    await waitFor(() => expect(screen.getByLabelText("资产地址")).toHaveValue("public.orders"));
    expect(mocks.productionCreate).toHaveBeenCalledTimes(1);
    expect(mocks.productionSubmit).not.toHaveBeenCalled();
    expect(mocks.submitProposal).not.toHaveBeenCalled();
  });

  it("recovers an uncertain creation from the server instead of creating again", async () => {
    const user = userEvent.setup();
    mocks.productionCreate.mockImplementationOnce(async (_workspace: string, draft: ProductionDraft) => { storeProduction(draft); throw new TypeError("connection lost"); });
    const view = renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("结果尚不明确");
    expect(screen.getByRole("button", { name: "保存知识草稿" })).toBeDisabled();
    view.unmount(); renderView(2); await openProductionCandidate(user);
    expect(await screen.findByRole("button", { name: /建模中 · v1/ })).toBeVisible();
    expect(screen.queryByRole("button", { name: "整理为知识" })).not.toBeInTheDocument();
    expect(mocks.productionCreate).toHaveBeenCalledTimes(1);
    expect(mocks.decideSemanticCandidate).not.toHaveBeenCalled();
  });

  it("does not require browser storage to create a server-owned draft", async () => {
    const user = userEvent.setup();
    renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    const storage = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("quota exceeded"); });
    try {
      await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
      expect(await screen.findByText("保存知识草稿已由服务器接收。")).toBeVisible();
      expect(mocks.productionCreate).toHaveBeenCalledTimes(1);
      expect(storage).not.toHaveBeenCalled();
    } finally { storage.mockRestore(); }
  });

  it("allows correction after the server definitively rejects production creation", async () => {
    const user = userEvent.setup();
    mocks.productionCreate.mockRejectedValueOnce(new ProductionApiError("invalid input", "INVALID_ARGUMENT", 400));
    renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    await screen.findByRole("alert");
    expect(screen.getByLabelText("业务定义")).not.toBeDisabled();
    expect(screen.getByRole("button", { name: "保存知识草稿" })).not.toBeDisabled();
    await user.type(screen.getByLabelText("业务定义"), "Corrected");
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    await screen.findByText("保存知识草稿已由服务器接收。");
    expect(mocks.productionCreate).toHaveBeenCalledTimes(2);
    expect(mocks.productionCreate.mock.calls[1][2]).not.toBe(mocks.productionCreate.mock.calls[0][2]);
  });

  it("does not turn a failed saved-detail refresh into a new draft creation", async () => {
    const user = userEvent.setup(); renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    await screen.findByRole("button", { name: "保存纠正版本" });
    mocks.productionGet.mockRejectedValueOnce(new ProductionApiError("detail unavailable", "REQUEST_FAILED", 500));
    await user.click(screen.getByRole("button", { name: "核对服务器状态" }));
    await screen.findByText("detail unavailable");
    expect(screen.getByLabelText("业务定义")).toBeDisabled();
    expect(screen.getByRole("button", { name: "保存知识草稿" })).toBeDisabled();
    expect(mocks.productionCreate).toHaveBeenCalledTimes(1);
  });

  it("retries run candidate errors and uses its own source-scoped cursor", async () => {
    const user = userEvent.setup();
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, sourceRevisionId: candidate.sourceRevisionId, adapterVersion: "v1", findings: [] });
    renderView(2);
    await screen.findByRole("button", { name: `查看运行 ${run.id}` });
    mocks.listSemanticCandidates.mockRejectedValueOnce(new Error("candidate read unavailable"));
    await user.click(screen.getByRole("button", { name: `查看运行 ${run.id}` }));
    const section = await screen.findByRole("region", { name: "关联候选" });
    expect(await within(section).findByRole("alert")).toHaveTextContent("candidate read unavailable");
    expect(within(section).queryByText("暂无关联候选")).not.toBeInTheDocument();
    mocks.listSemanticCandidates.mockResolvedValueOnce({ items: [], total: 1, nextCursor: "source-cursor", limit: 50 }).mockResolvedValueOnce({ items: [candidate], total: 1, limit: 50 });
    await user.click(within(section).getByRole("button", { name: "重试" }));
    await user.click(await within(section).findByRole("button", { name: "加载更多候选" }));
    expect(await within(section).findByRole("button", { name: /public.orders/ })).toBeVisible();
    expect(mocks.listSemanticCandidates).toHaveBeenLastCalledWith(workspaceId, expect.objectContaining({ sourceId: source.id, cursor: "source-cursor" }));
  });

  it("refreshes candidates when opening a newly completed schedule run", async () => {
    const user = userEvent.setup();
    const schedule = { id: "sch_fresh", sourceId: source.id, expression: "0 2 * * *", timezone: "Asia/Shanghai", enabled: true, version: 1, misfirePolicy: "skip", createdAt: source.createdAt, updatedAt: source.updatedAt };
    mocks.listSchedules.mockResolvedValue({ items: [schedule], total: 1, limit: 50 });
    mocks.listOccurrences.mockResolvedValue({ items: [{ id: "occ_fresh", scheduleId: schedule.id, sourceId: source.id, eligibleAt: run.createdAt, triggerKind: "cron", state: "enqueued", discoveryRunId: "run_new" }], total: 1, limit: 50 });
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, id: "run_new", status: "succeeded", sourceRevisionId: "srv_new", adapterVersion: "v1", findings: [] });
    renderView(1);
    await user.click(await screen.findByRole("button", { name: "查看计划 sch_fresh" }));
    mocks.listSemanticCandidates.mockResolvedValue({ items: [{ ...candidate, id: "scd_new", title: "new.results", discoveryRunId: "run_new", sourceRevisionId: "srv_new" }], total: 1, limit: 50 });
    await user.click(await screen.findByRole("button", { name: "打开运行" }));
    expect(await screen.findByRole("button", { name: /new.results/ })).toBeVisible();
    expect(mocks.listSemanticCandidates).toHaveBeenLastCalledWith(workspaceId, expect.objectContaining({ sourceId: source.id }));
  });

  it("reloads a scheduled run's candidates when polling reaches a terminal state", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    try {
      const user = userEvent.setup();
      let completed = false;
      const schedule = { id: "sch_poll", sourceId: source.id, expression: "0 2 * * *", timezone: "Asia/Shanghai", enabled: true, version: 1, misfirePolicy: "skip", createdAt: source.createdAt, updatedAt: source.updatedAt };
      mocks.listSchedules.mockResolvedValue({ items: [schedule], total: 1, limit: 50 });
      mocks.listOccurrences.mockResolvedValue({ items: [{ id: "occ_poll", scheduleId: schedule.id, sourceId: source.id, eligibleAt: run.createdAt, triggerKind: "cron", state: "enqueued", discoveryRunId: "run_poll" }], total: 1, limit: 50 });
      mocks.getDiscoveryRun.mockImplementation(async () => ({ ...run, id: "run_poll", status: completed ? "succeeded" : "running", sourceRevisionId: completed ? "srv_poll" : undefined, adapterVersion: "v1", findings: [] }));
      mocks.listSemanticCandidates.mockResolvedValue({ items: [], total: 0, limit: 50 });
      renderView(1);
      await user.click(await screen.findByRole("button", { name: "查看计划 sch_poll" }));
      await user.click(await screen.findByRole("button", { name: "打开运行" }));
      expect(await screen.findByText("等待发现结果")).toBeVisible();
      completed = true;
      mocks.listSemanticCandidates.mockResolvedValue({ items: [{ ...candidate, id: "scd_poll", discoveryRunId: "run_poll", sourceRevisionId: "srv_poll", title: "fresh.after.completion" }], total: 1, limit: 50 });
      await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
      expect(await screen.findByRole("button", { name: /fresh.after.completion/ })).toBeVisible();
    } finally { vi.useRealTimers(); }
  });

  it("clears source run context on a menu navigation request", async () => {
    const user = userEvent.setup();
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, adapterVersion: "v1", findings: [] });
    const view = render(<LiveSourcesView focusIndex={0} navigationEpoch={0} runDetailBackRequestEpoch={0} onRunDetailOpenChange={() => {}} onOpenProposal={() => {}} />);
    await user.click(await screen.findByRole("button", { name: `查看来源 ${source.name}` }));
    await user.click(screen.getByRole("button", { name: "查看结果" }));
    expect(await screen.findByRole("button", { name: "返回来源" })).toBeVisible();
    view.rerender(<LiveSourcesView focusIndex={2} navigationEpoch={1} runDetailBackRequestEpoch={0} onRunDetailOpenChange={() => {}} onOpenProposal={() => {}} />);
    expect(await screen.findByRole("region", { name: "运行记录" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "返回来源" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: `查看运行 ${run.id}` }));
    expect(await screen.findByRole("button", { name: "返回运行记录" })).toBeVisible();
  });

  it("includes the final disclosure in the schedule dialog focus loop", async () => {
    const user = userEvent.setup();
    const schedule = { id: "sch_focus", sourceId: source.id, expression: "0 2 * * *", timezone: "Asia/Shanghai", enabled: true, version: 1, misfirePolicy: "skip", createdAt: source.createdAt, updatedAt: source.updatedAt };
    mocks.listSchedules.mockResolvedValue({ items: [schedule], total: 1, limit: 50 });
    renderView(1);
    await user.click(await screen.findByRole("button", { name: "查看计划 sch_focus" }));
    await user.tab({ shift: true });
    expect(screen.getByText("计划标识与 Cron")).toHaveFocus();
    await user.click(screen.getByText("计划标识与 Cron"));
    expect(screen.getByText("计划标识与 Cron").closest("details")).toHaveAttribute("open");
    await user.tab();
    expect(screen.getByRole("button", { name: "关闭接入计划详情" })).toHaveFocus();
  });

  it("retains an initial run link and returns to the run list", async () => {
    const user = userEvent.setup();
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, adapterVersion: "v1", findings: [] });
    render(<LiveSourcesView focusIndex={2} initialRunId={run.id} runDetailBackRequestEpoch={0} onRunDetailOpenChange={() => {}} onOpenProposal={() => {}} />);
    expect(await screen.findByRole("region", { name: "运行记录详情" })).toBeVisible();
    expect(await screen.findByRole("heading", { name: source.name })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "返回运行记录" }));
    expect(await screen.findByRole("button", { name: `查看运行 ${run.id}` })).toBeVisible();
  });

  it("preserves source history when returning from a run", async () => {
    const user = userEvent.setup();
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, adapterVersion: "v1", findings: [] });
    renderView();
    await user.click(await screen.findByRole("button", { name: `查看来源 ${source.name}` }));
    await user.click(screen.getByRole("tab", { name: "运行历史" }));
    await user.click(screen.getByRole("button", { name: `查看运行 ${run.id}` }));
    await user.click(await screen.findByRole("button", { name: "返回来源" }));
    expect(screen.getByRole("tab", { name: "运行历史" })).toHaveAttribute("aria-selected", "true");
  });

  it("starts from the source candidate without requiring an existing asset or inventing a definition", async () => {
    const user = userEvent.setup(); renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    expect(screen.getByLabelText("业务定义")).toHaveValue("");
    expect(screen.getByLabelText("适用范围")).toHaveValue("");
    expect(screen.queryByLabelText("目标语义资产")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "匹配已有资产" })).toBeVisible();
    expect(mocks.productionCreate).not.toHaveBeenCalled();
    expect(mocks.ensureAsset).not.toHaveBeenCalled();
  });

  it("restores the source route when leaving production", async () => {
    const user = userEvent.setup(); renderView(2); await openProductionCandidate(user);
    window.history.replaceState({}, "", "/governance?production=server-operation");
    await user.click(screen.getByRole("button", { name: "返回来源与候选" }));
    expect(window.location.pathname).toBe("/sources");
    expect(window.location.search).toBe("");
  });

  it("preserves explicit candidate dismissal without creating production", async () => {
    const user = userEvent.setup(); renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByText("忽略此候选", { exact: true }));
    expect(screen.getByRole("button", { name: "忽略候选" })).toBeDisabled();
    await user.type(screen.getByLabelText("忽略依据"), "Not part of the governed scope");
    mocks.decideSemanticCandidate.mockResolvedValue({});
    mocks.getSemanticCandidate.mockResolvedValue({ ...candidate, status: "dismissed" });
    await user.click(screen.getByRole("button", { name: "忽略候选" }));
    await screen.findByText("忽略候选已由服务器接收。");
    expect(mocks.decideSemanticCandidate).toHaveBeenCalledWith(workspaceId, candidate.id, expect.objectContaining({ action: "dismiss", reason: "Not part of the governed scope", idempotencyKey: expect.any(String) }));
    expect(mocks.productionCreate).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "整理为知识" })).toBeDisabled();
  });

  it("opens an actionable source workspace with its own history", async () => {
    const user = userEvent.setup();
    renderView();
    await user.click(await screen.findByRole("button", { name: `查看来源 ${source.name}` }));
    expect(screen.getByRole("region", { name: "数据来源详情" })).toBeVisible();
    expect(screen.getByRole("button", { name: "测试连接" })).toBeVisible();
    await user.click(screen.getByRole("tab", { name: "运行历史" }));
    expect(screen.getByRole("button", { name: `查看运行 ${run.id}` })).toBeVisible();
    expect(screen.getByRole("button", { name: "返回数据来源" })).toBeVisible();
  });

  it("keeps schedule occurrences in ingestion and resolves their final state", async () => {
    const user = userEvent.setup();
    const schedule = { id: "sch_test", sourceId: source.id, expression: "0 2 * * *", timezone: "Asia/Shanghai", enabled: true, version: 1, misfirePolicy: "skip", createdAt: source.createdAt, updatedAt: source.updatedAt };
    mocks.listSchedules.mockResolvedValue({ items: [schedule], total: 1, limit: 50 });
    mocks.listOccurrences.mockResolvedValue({ items: [{ id: "occ_test", scheduleId: schedule.id, sourceId: source.id, eligibleAt: run.createdAt, triggerKind: "cron", state: "enqueued", runtimeRunId: run.id, operationsPath: run.operationsPath }], total: 1, limit: 50 });
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, adapterVersion: "v1", findings: [] });
    renderView(1);
    await user.click(await screen.findByRole("button", { name: `查看计划 ${schedule.id}` }));
    expect(await screen.findByRole("button", { name: "打开运行" })).toBeVisible();
    expect(screen.queryByRole("link", { name: "打开运行" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "打开运行" }));
    expect(await screen.findByRole("region", { name: "运行记录详情" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "返回计划" }));
    expect(screen.getByRole("dialog", { name: "接入计划详情" })).toBeVisible();
  });
  it("makes discovery submission visible and offers its run detail", async () => {
    const user = userEvent.setup();
    mocks.startSourceRun.mockResolvedValue({ ...run, status: "queued" });
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, adapterVersion: "v1", findings: [] });
    render(<LiveSourcesView focusIndex={0} runDetailBackRequestEpoch={0} onRunDetailOpenChange={() => {}} onOpenProposal={() => {}} />);
    await user.click(await screen.findByRole("button", { name: `启动发现 ${source.name}` }));
    expect(await screen.findByText(/发现任务已创建，排队中/)).toBeVisible();
    expect(mocks.startSourceRun).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: "查看运行" }));
    expect(await screen.findByRole("region", { name: "运行记录详情" })).toBeVisible();
  });

  it("shows connection-test progress and supports menu keyboard navigation", async () => {
    const user = userEvent.setup();
    let complete!: () => void;
    mocks.testSourceConnection.mockImplementationOnce(() => new Promise<void>((resolve) => { complete = resolve; }));
    renderView();
    await user.click(await screen.findByRole("button", { name: `更多操作 ${source.name}` }));
    const menu = screen.getByRole("menu");
    expect(within(menu).getByRole("menuitem", { name: "测试连接" })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(screen.getByText(/正在测试 Alpha Warehouse/)).toBeVisible();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await act(async () => complete());
    expect(await screen.findByText("Alpha Warehouse：连接测试通过。")).toBeVisible();
    await user.click(screen.getByRole("button", { name: `更多操作 ${source.name}` }));
    await user.keyboard("{End}");
    expect(screen.getByRole("menuitem", { name: "删除连接" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.getByRole("button", { name: `更多操作 ${source.name}` })).toHaveFocus();
    expect(mocks.deleteSource).not.toHaveBeenCalled();
  });

  it("keeps unavailable actions disabled and lets a read-only user close the menu", async () => {
    mocks.allowed = new Set(["source.read"]);
    const user = userEvent.setup();
    renderView();
    const trigger = await screen.findByRole("button", { name: `更多操作 ${source.name}` });
    await user.click(trigger);
    for (const action of screen.getAllByRole("menuitem")) expect(action).toBeDisabled();
    expect(screen.getByRole("menu")).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(trigger).toHaveFocus();
    expect(screen.getByRole("button", { name: `启动发现 ${source.name}` })).toBeDisabled();
    expect(mocks.startSourceRun).not.toHaveBeenCalled();
  });

  it("opens source details with the keyboard and restores the filtered list", async () => {
    const user = userEvent.setup();
    renderView();
    const open = await screen.findByRole("button", { name: `查看来源 ${source.name}` });
    await user.type(screen.getByRole("searchbox", { name: "搜索数据来源" }), "Alpha");
    open.focus();
    await user.keyboard("{Enter}");
    const detail = screen.getByRole("region", { name: "数据来源详情" });
    expect(detail).toHaveTextContent("warehouse.internal:5432");
    expect(within(detail).queryByRole("textbox")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "返回数据来源" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(open).toHaveFocus();
    expect(screen.getByRole("searchbox", { name: "搜索数据来源" })).toHaveValue("Alpha");
    await user.click(screen.getByRole("button", { name: `更多操作 ${source.name}` }));
    await user.click(screen.getByRole("menuitem", { name: "编辑连接" }));
    expect(screen.getByRole("dialog", { name: "编辑 PostgreSQL 连接" })).toBeVisible();
    expect(screen.queryByRole("dialog", { name: "数据来源详情" })).not.toBeInTheDocument();
  });

  it("uses a table for schedules and keeps execution records inside its detail", async () => {
    const user = userEvent.setup();
    const schedule = { id: "sch_01arz3ndektsv4rrffq69g5fav", sourceId: source.id, expression: "0 2 * * *", timezone: "Asia/Shanghai", enabled: true, version: 1, misfirePolicy: "skip", createdAt: source.createdAt, updatedAt: source.updatedAt };
    mocks.listSchedules.mockResolvedValue({ items: [schedule], total: 1, limit: 50 });
    renderView(1);
    const open = await screen.findByRole("button", { name: `查看计划 ${schedule.id}` });
    expect(screen.getByRole("table", { name: "接入计划" })).toContainElement(open);
    await user.click(open);
    const detail = screen.getByRole("dialog", { name: "接入计划详情" });
    expect(detail).toHaveTextContent("Asia/Shanghai");
    expect(await within(detail).findByText("暂无执行记录")).toBeVisible();
    await user.keyboard("{Escape}");
    expect(open).toHaveFocus();
    await user.type(screen.getByRole("searchbox", { name: "搜索接入计划" }), "no-match");
    expect(screen.getByText("没有匹配的接入计划")).toBeVisible();
  });

  it("focuses the exact source from a persisted deep link without choosing a fallback", async () => {
    const { rerender } = renderView(0, vi.fn(), source.id);

    const exactSource = await screen.findByText("Alpha Warehouse");
    expect(exactSource.closest("[data-source-id]")).toHaveAttribute("aria-current", "true");
    await waitFor(() => expect(exactSource.closest("[data-source-id]")).toHaveFocus());

    rerender(<LiveSourcesView focusIndex={0} runDetailBackRequestEpoch={0} initialSourceId="src_01arz3ndektsv4rrffq69g5fzz" onRunDetailOpenChange={() => {}} onOpenProposal={() => {}} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("找不到目标来源 src_01arz3ndektsv4rrffq69g5fzz");
    expect(exactSource.closest("[data-source-id]")).not.toHaveAttribute("aria-current");
  });

  it("shows API loss explicitly and never substitutes the seeded source", async () => {
    mocks.listSources.mockRejectedValue(new Error("source storage unreachable"));
    renderView();

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("真实数据加载失败");
    expect(alert).toHaveTextContent("source storage unreachable");
    expect(screen.queryByText("PostgreSQL Analytics")).not.toBeInTheDocument();
  });

  it("creates a real source through a write-only password field", async () => {
    const created = { ...source, id: "src_01arz3ndektsv4rrffq69g5faw", name: "Finance Warehouse", credentialVersion: 1 };
    mocks.createSource.mockResolvedValue(created);
    const user = userEvent.setup();
    renderView();

    await screen.findByText("Alpha Warehouse");
    await user.click(screen.getByRole("button", { name: "新建连接" }));
    const dialog = screen.getByRole("dialog", { name: "新建 PostgreSQL 连接" });
    await user.type(within(dialog).getByLabelText("连接名称"), "Finance Warehouse");
    await user.type(within(dialog).getByLabelText("主机"), "finance.internal");
    await user.type(within(dialog).getByLabelText("数据库"), "finance");
    await user.type(within(dialog).getByLabelText("只读用户名"), "finance_ro");
    const password = within(dialog).getByLabelText("密码");
    expect(password).toHaveAttribute("type", "password");
    await user.type(password, "do-not-echo-this");
    await user.click(within(dialog).getByRole("button", { name: "保存连接" }));

    await waitFor(() => expect(mocks.createSource).toHaveBeenCalledWith(workspaceId, expect.objectContaining({ password: "do-not-echo-this", username: "finance_ro" })));
    expect(await screen.findByRole("heading", { name: "Finance Warehouse" })).toBeVisible();
    expect(screen.getByRole("button", { name: "测试连接" })).toBeVisible();
    expect(document.body).not.toHaveTextContent("do-not-echo-this");
  });

  it("never prefills a rotated credential and surfaces connection failure", async () => {
    mocks.testSourceConnection.mockRejectedValue(new Error("read-only role rejected"));
    const user = userEvent.setup();
    renderView();

    await screen.findByText("Alpha Warehouse");
    await user.click(screen.getByRole("button", { name: "更多操作 Alpha Warehouse" }));
    await user.click(screen.getByRole("menuitem", { name: "测试连接" }));
    expect(await screen.findByText("read-only role rejected")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "更多操作 Alpha Warehouse" }));
    await user.click(screen.getByRole("menuitem", { name: "轮换凭据" }));
    const credentialDialog = screen.getByRole("dialog", { name: "轮换连接凭据" });
    expect(within(credentialDialog).getByLabelText("新密码")).toHaveValue("");
    expect(credentialDialog).not.toHaveTextContent("credentialVersion");
  });

  it("renders degraded runs and loads persisted findings on demand", async () => {
    const detail: DiscoveryRun = {
      ...run,
      adapterVersion: "postgresql_catalog/v1",
      sourceRevisionId: candidate.sourceRevisionId,
      findings: [{ sequence: 1, code: "SQL_ARTIFACT_PARSE_WARNING", severity: "warning", locator: "models/orders.sql", details: { parser: "pg_query" } }],
    };
    mocks.getDiscoveryRun.mockResolvedValue(detail);
    const user = userEvent.setup();
    renderView(2);

    expect(await screen.findByText("有警告")).toBeVisible();
    await user.click(screen.getByRole("button", { name: `查看运行 ${run.id}` }));
    const view = await screen.findByRole("region", { name: "运行记录详情" });
    expect(within(view).getByText("SQL_ARTIFACT_PARSE_WARNING")).toBeVisible();
    expect(within(view).getByText("models/orders.sql")).toBeVisible();
  });

  it("pins candidate and snapshot in production creation without client-side candidate conversion", async () => {
    const user = userEvent.setup(); renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    await screen.findByText("保存知识草稿已由服务器接收。");
    expect(mocks.productionCreate).toHaveBeenCalledWith(workspaceId, expect.objectContaining({
      input: expect.objectContaining({ candidates: [expect.objectContaining({ candidateId: candidate.id, snapshotId: "snapshot-orders", digest: candidate.contentDigest, primaryTargetKey: "primary" })] }),
      targets: [expect.objectContaining({ intent: "create", content: expect.objectContaining({ definition: null, scope: null }) })],
    }), expect.any(String));
    expect(mocks.createAndSubmitProposal).not.toHaveBeenCalled();
    expect(mocks.decideSemanticCandidate).not.toHaveBeenCalled();
  });

  it("retries uncertain production creation with the exact same command key", async () => {
    const user = userEvent.setup(); mocks.productionCreate.mockRejectedValueOnce(new TypeError("network lost"));
    renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    await user.click(await screen.findByRole("button", { name: "重试同一请求" }));
    await screen.findByText("保存知识草稿已由服务器接收。");
    expect(mocks.productionCreate).toHaveBeenCalledTimes(2);
    expect(mocks.productionCreate.mock.calls[0]).toEqual(mocks.productionCreate.mock.calls[1]);
    expect(mocks.decideSemanticCandidate).not.toHaveBeenCalled();
  });

  it("leaves candidate conversion untouched when production storage fails", async () => {
    const user = userEvent.setup(); mocks.productionCreate.mockRejectedValue(new ProductionApiError("unavailable", "INTERNAL_ERROR", 503));
    renderView(2); await openProductionCandidate(user);
    await user.click(screen.getByRole("button", { name: "整理为知识" }));
    await user.click(screen.getByRole("button", { name: "保存知识草稿" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("结果尚不明确");
    expect(mocks.decideSemanticCandidate).not.toHaveBeenCalled();
    expect(mocks.createAndSubmitProposal).not.toHaveBeenCalled();
    expect(screen.getByRole("region", { name: "知识确认" })).toBeVisible();
  });

  it("exposes durable scheduling rather than prototype automation", async () => {
    renderView(1);
    const view = screen.getByRole("region", { name: "接入计划" });
    expect(within(view).queryByText("Prototype")).not.toBeInTheDocument();
    expect(within(view).queryByLabelText("计划来源")).not.toBeInTheDocument();
    expect(await within(view).findByRole("button", { name: "新建计划" })).toBeVisible();
    await waitFor(() => expect(mocks.listSchedules).toHaveBeenCalledWith(workspaceId, source.id));
    const user = userEvent.setup();
    await user.click(within(view).getByRole("button", { name: "新建计划" }));
    expect(within(screen.getByRole("dialog", { name: "新建接入计划" })).getByLabelText("计划来源")).toHaveValue(source.id);
  });

  it("navigates source pages explicitly and keeps the page size contract", async () => {
    mocks.listSources.mockResolvedValueOnce({ items: [source], total: 2, limit: 1, nextCursor: "next-source" })
      .mockResolvedValueOnce({ items: [{ ...source, id: "src_later", name: "Later Warehouse" }], total: 2, limit: 1 });
    const user = userEvent.setup();
    renderView();
    await user.click(await screen.findByRole("button", { name: "下一页来源" }));
    expect(await screen.findByText("Later Warehouse")).toBeVisible();
    expect(mocks.listSources).toHaveBeenLastCalledWith(workspaceId, expect.objectContaining({ cursor: "next-source" }));
    expect(screen.queryByText("Alpha Warehouse")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "上一页来源" }));
    expect(await screen.findByText("Alpha Warehouse")).toBeVisible();
    expect(screen.queryByText("Later Warehouse")).not.toBeInTheDocument();
  });

  it("refetches sources when the page size changes", async () => {
    const user = userEvent.setup();
    renderView();
    await screen.findByText("Alpha Warehouse");
    await user.selectOptions(screen.getByLabelText("每页行数"), "25");
    await waitFor(() => expect(mocks.listSources).toHaveBeenLastCalledWith(workspaceId, expect.objectContaining({ limit: 25 })));
  });

  it("sends the observed source version when editing", async () => {
    mocks.updateSource.mockResolvedValue({ ...source, name: "Updated", version: 5 });
    const user = userEvent.setup();
    renderView();
    await user.click(await screen.findByRole("button", { name: "更多操作 Alpha Warehouse" }));
    await user.click(screen.getByRole("menuitem", { name: "编辑连接" }));
    await user.clear(screen.getByLabelText("连接名称"));
    await user.type(screen.getByLabelText("连接名称"), "Updated");
    await user.click(screen.getByRole("button", { name: "保存连接" }));
    await waitFor(() => expect(mocks.updateSource).toHaveBeenCalledWith(workspaceId, source.id, expect.objectContaining({ expectedVersion: 4 })));
  });

  it("uploads a CSV and retains the confirmed artifact set when source refresh fails", async () => {
    const set = { id: "ars_confirmed", sourceId: "src_file", sourceKind: "file", setDigest: "sha256:confirmed", members: [], createdAt: source.createdAt };
    mocks.stageArtifact.mockResolvedValue({ id: "art_csv", kind: "csv", status: "validated" });
    mocks.finalizeArtifactSet.mockResolvedValue(set);
    const user = userEvent.setup();
    renderView();
    await screen.findByText(source.name);
    await user.click(screen.getByRole("button", { name: "导入文件" }));
    await user.type(screen.getByLabelText("来源名称"), "Orders CSV");
    await user.upload(screen.getByLabelText("上传工件"), new File(["id\n1\n"], "orders.csv", { type: "text/csv" }));
    expect(screen.getByText("orders.csv")).toBeVisible();
    mocks.listSources.mockRejectedValueOnce(new Error("refresh unavailable"));
    await user.click(screen.getByRole("button", { name: "校验并保存来源" }));
    expect(await screen.findByRole("dialog", { name: "持久工件集合" })).toHaveTextContent("ars_confirmed");
    expect(mocks.stageArtifact).toHaveBeenCalledWith(workspaceId, expect.any(File), "csv", expect.any(String), undefined);
    expect(mocks.finalizeArtifactSet).toHaveBeenCalledWith(workspaceId, { sourceName: "Orders CSV", artifactIds: ["art_csv"] }, expect.any(String));
    expect(await screen.findByRole("alert")).toHaveTextContent("refresh unavailable");
  });

  it("registers configured-root SQL paths without browser upload", async () => {
    mocks.registerSQL.mockResolvedValue({ id: "ars_sql", sourceId: "src_sql", sourceKind: "sql_bundle", setDigest: "sha256:sql", members: [], createdAt: source.createdAt });
    const user = userEvent.setup();
    renderView();
    await screen.findByText(source.name);
    await user.click(screen.getByRole("button", { name: "导入文件" }));
    await user.type(screen.getByLabelText("来源名称"), "SQL models");
    await user.selectOptions(screen.getByLabelText("工件类型"), "sql_bundle");
    await user.type(screen.getByLabelText("配置根目录内 SQL 路径"), "models/orders.sql\nmodels/items.sql");
    await user.click(screen.getByRole("button", { name: "校验并保存来源" }));
    expect(await screen.findByRole("dialog", { name: "持久工件集合" })).toHaveTextContent("ars_sql");
    expect(mocks.registerSQL).toHaveBeenCalledWith(workspaceId, { sourceName: "SQL models", paths: ["models/orders.sql", "models/items.sql"] }, expect.any(String));
    expect(mocks.stageArtifact).not.toHaveBeenCalled();
  });

  it("keeps unsafe upload errors in the dialog and never finalizes a rejected artifact", async () => {
    mocks.stageArtifact.mockRejectedValue(new Error("UNSAFE_ARTIFACT"));
    const user = userEvent.setup();
    renderView();
    await screen.findByText(source.name);
    await user.click(screen.getByRole("button", { name: "导入文件" }));
    await user.type(screen.getByLabelText("来源名称"), "Unsafe CSV");
    await user.upload(screen.getByLabelText("上传工件"), new File(["id\n=NOW()\n"], "unsafe.csv", { type: "text/csv" }));
    await user.click(screen.getByRole("button", { name: "校验并保存来源" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("UNSAFE_ARTIFACT");
    expect(mocks.finalizeArtifactSet).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog", { name: "导入文件" })).toBeVisible();
  });

  it("does not offer schedule delegation without ingestion.run", async () => {
    mocks.allowed.delete("ingestion.run");
    renderView(1);
    expect(await screen.findByRole("button", { name: "新建计划" })).toBeDisabled();
    expect(mocks.createSchedule).not.toHaveBeenCalled();
  });

  it("loads persisted file sources and exact artifact metadata after remount", async () => {
    const fileSource = { id: "src_file", name: "Persisted CSV", sourceKind: "file", adapterKind: "file_catalog", version: 2, status: "active", activeArtifactSetId: "ars_saved", createdAt: source.createdAt, updatedAt: source.updatedAt };
    mocks.listSources.mockResolvedValue({ items: [fileSource], total: 1, limit: 50 });
    mocks.getArtifactSet.mockResolvedValue({ id: "ars_saved", sourceId: fileSource.id, sourceKind: "file", setDigest: "sha256:saved", members: [], createdAt: source.createdAt });
    const user = userEvent.setup();
    const view = renderView();
    await screen.findByText("Persisted CSV");
    view.unmount();
    renderView();
    await user.click(await screen.findByRole("button", { name: "更多操作 Persisted CSV" }));
    await user.click(screen.getByRole("menuitem", { name: "查看工件" }));
    expect(await screen.findByRole("dialog", { name: "持久工件集合" })).toHaveTextContent("ars_saved");
    expect(screen.queryByRole("button", { name: /轮换凭据/ })).not.toBeInTheDocument();
  });

  it("polls known active runs without erasing loaded source pages", async () => {
    let tick: (() => void) | undefined;
    const setInterval = window.setInterval.bind(window);
    const interval = vi.spyOn(window, "setInterval").mockImplementation((handler, timeout, ...args) => {
      if (timeout === 2500) tick = handler as () => void;
      const timer = timeout === 2500 ? setInterval(() => undefined, 60_000) : setInterval(handler, timeout, ...args);
      return timer as unknown as ReturnType<typeof globalThis.setInterval>;
    });
    mocks.listSourceRuns.mockResolvedValue({ items: [{ ...run, status: "queued" }], total: 1, limit: 50 });
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, adapterVersion: "v1", findings: [] });
    mocks.listSources.mockResolvedValueOnce({ items: [source], total: 2, limit: 1, nextCursor: "page-two" })
      .mockResolvedValueOnce({ items: [{ ...source, id: "src_later", name: "Later Warehouse" }], total: 2, limit: 1 })
      .mockResolvedValue({ items: [source], total: 2, limit: 1, nextCursor: "page-two" });
    const user = userEvent.setup();
    renderView();
    await user.click(await screen.findByRole("button", { name: "下一页来源" }));
    expect(await screen.findByText("Later Warehouse")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "导入文件" }));
    await user.type(screen.getByLabelText("来源名称"), "Typing while polling");
    await act(async () => { tick?.(); });
    await waitFor(() => expect(mocks.getDiscoveryRun).toHaveBeenCalled());
    expect(screen.getByText("Later Warehouse")).toBeVisible();
    expect(mocks.listSources).toHaveBeenCalledTimes(2);
    expect(screen.getByLabelText("来源名称")).toHaveFocus();
    interval.mockRestore();
  });

  it("reveals persisted candidates when an active run completes", async () => {
    let tick: (() => void) | undefined;
    const setInterval = window.setInterval.bind(window);
    const interval = vi.spyOn(window, "setInterval").mockImplementation((handler, timeout, ...args) => {
      if (timeout === 2500) tick = handler as () => void;
      const timer = timeout === 2500 ? setInterval(() => undefined, 60_000) : setInterval(handler, timeout, ...args);
      return timer as unknown as ReturnType<typeof globalThis.setInterval>;
    });
    mocks.listSourceRuns.mockResolvedValue({ items: [{ ...run, status: "queued" }], total: 1, limit: 50 });
    mocks.getDiscoveryRun.mockResolvedValue({ ...run, status: "succeeded", adapterVersion: "v1", findings: [] });
    mocks.listSemanticCandidates.mockResolvedValueOnce({ items: [], total: 0, limit: 50 }).mockResolvedValue({ items: [candidate], total: 1, limit: 50 });
    const user = userEvent.setup();
    renderView(2);
    await user.click(await screen.findByRole("tab", { name: /语义候选/ }));
    await screen.findByText("还没有语义候选");
    await act(async () => { tick?.(); });
    expect(await screen.findByRole("button", { name: `查看语义候选 ${candidate.title}` })).toBeVisible();
    interval.mockRestore();
  });
});

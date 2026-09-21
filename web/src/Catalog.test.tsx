import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, vi } from "vitest";

import { App } from "./App";
import { CatalogRuntimeProvider, useCatalogRuntime } from "./catalogRuntime";
import { listWorkspaces } from "./catalog";
import { getSession } from "./identity";
import { SessionAccountControl, SessionRuntimeProvider } from "./sessionRuntime";


const { workspace, asset, listAssetsMock, getAssetMock, listAssetRevisionsMock, listAssetAuthorityRecordsMock, createWorkspaceMock, createAssetMock, session } = vi.hoisted(() => ({
  listAssetsMock: vi.fn(),
  getAssetMock: vi.fn(),
  listAssetRevisionsMock: vi.fn(),
  listAssetAuthorityRecordsMock: vi.fn(),
  createWorkspaceMock: vi.fn(),
  createAssetMock: vi.fn(),
  workspace: {
    id: "wsp_01arz3ndektsv4rrffq69g5fav",
    slug: "semantic-core",
    displayName: "Semantic Core",
    createdAt: "2026-09-02T04:00:00Z",
    updatedAt: "2026-09-02T04:00:00Z",
  },
  asset: {
    id: "ast_01arz3ndektsv4rrffq69g5fav",
    address: "commerce.net_revenue",
    assetType: "metric" as const,
    lifecycleState: "active" as const,
    currentRevisionId: "rev_01arz3ndektsv4rrffq69g5fav",
    title: "Net revenue",
    summary: "Revenue after refunds and adjustments.",
    updatedAt: "2026-09-02T04:00:00Z",
  },
  session: {
    account: { id: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Catalog Admin" },
    workspaces: [{
      id: "wsp_01arz3ndektsv4rrffq69g5fav",
      slug: "semantic-core",
      displayName: "Semantic Core",
      principalId: "prn_01arz3ndektsv4rrffq69g5fav",
      roleIds: ["workspace_admin"],
      capabilities: ["workspace.read" as const, "workspace.manage" as const, "asset.read" as const],
      authorizationVersion: 1,
    }],
    expiresAt: "2026-09-04T18:00:00Z",
    traceId: "trace-catalog-session",
  },
}));

vi.mock("./identity", async (importOriginal) => ({
  ...await importOriginal<typeof import("./identity")>(),
  getSession: vi.fn().mockResolvedValue(session),
}));

vi.mock("./catalog", () => ({
  listWorkspaces: vi.fn().mockResolvedValue([workspace]),
  listAssets: listAssetsMock.mockResolvedValue({ items: [asset], page: { limit: 100 } }),
  getAsset: getAssetMock.mockResolvedValue({
    ...asset,
    createdAt: asset.updatedAt,
    relationCount: 2,
    currentRevision: {
      id: asset.currentRevisionId,
      assetId: asset.id,
      sequence: 3,
      schemaVersion: "1.0.0",
      contentDigest: "a".repeat(64),
      content: { title: asset.title, summary: asset.summary },
      createdBy: "catalog-web",
      createdAt: asset.updatedAt,
      evidence: [],
    },
    authoritySections: [
      { kind: "definition", authority: "asset_revisions", availability: "available", revisionId: asset.currentRevisionId, values: { fieldCount: 2 }, records: [], recordsPage: { limit: 100, total: 0 } },
      { kind: "released_state", authority: "release_manifests", availability: "not_released", revisionId: asset.currentRevisionId, values: {}, records: [], recordsPage: { limit: 100, total: 0 } },
      { kind: "relations", authority: "asset_relations", availability: "forbidden", revisionId: asset.currentRevisionId, values: {}, records: [], recordsPage: { limit: 100, total: 0 } },
      { kind: "physical_bindings", authority: "governed_objects", availability: "not_configured", revisionId: asset.currentRevisionId, values: {}, records: [], recordsPage: { limit: 100, total: 0 } },
      { kind: "join_contracts", authority: "governed_objects", availability: "failed", revisionId: asset.currentRevisionId, values: {}, records: [], recordsPage: { limit: 100, total: 0 } },
      { kind: "validation", authority: "validation_runs", availability: "available", revisionId: asset.currentRevisionId, values: { runCount: 1, blockerCount: 0, warningCount: 1 }, records: [{ kind: "validation_run", id: "vrn_01arz3ndektsv4rrffq69g5fav", authority: "validation_runs", status: "passed", label: "Schema validation", version: 1 }], recordsPage: { limit: 100, total: 1 } },
      { kind: "lineage", authority: "lineage_edges", availability: "available", revisionId: asset.currentRevisionId, values: { edgeCount: 1 }, records: [{ kind: "lineage", id: "lge_01arz3ndektsv4rrffq69g5fav", authority: "lineage_edges", status: "active", label: "Orders source", relatedId: "pds_01arz3ndektsv4rrffq69g5fav", version: 1 }], recordsPage: { limit: 100, total: 1 } },
      { kind: "evidence", authority: "asset_revision_evidence", availability: "available", revisionId: asset.currentRevisionId, values: { evidenceCount: 0 }, records: [], recordsPage: { limit: 100, total: 0 } },
      { kind: "trust", authority: "governed_objects", availability: "not_configured", revisionId: asset.currentRevisionId, values: {}, records: [], recordsPage: { limit: 100, total: 0 } },
      { kind: "consumer_impact", authority: "consumer_bindings", availability: "available", revisionId: asset.currentRevisionId, values: { current: 0, pinned: 0 }, records: [], recordsPage: { limit: 100, total: 0 } },
    ],
  }),
  listAssetRevisions: listAssetRevisionsMock.mockResolvedValue({ items: [], page: { limit: 100, total: 0 } }),
  listAssetAuthorityRecords: listAssetAuthorityRecordsMock.mockResolvedValue({ items: [], page: { limit: 100, total: 0 } }),
  createWorkspace: createWorkspaceMock,
  createAsset: createAssetMock,
}));

vi.mock("./LiveSourcesView", () => ({
  LiveSourcesView: ({ initialSourceId }: { initialSourceId?: string }) => <section aria-label="真实来源深链">{initialSourceId ?? "未选择来源"}</section>,
}));

vi.mock("./workbench", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./workbench")>();
  return {
    ...actual,
    workbenchApi: {
      ...actual.workbenchApi,
      listItems: vi.fn().mockResolvedValue({ items: [], counts: { total: 0, open: 0, inProgress: 0, critical: 0 }, page: { limit: 50 } }),
    },
  };
});

vi.mock("./governance", async (importOriginal) => ({
  ...await importOriginal<typeof import("./governance")>(),
  listProposals: vi.fn().mockResolvedValue([]),
  listReviewBatches: vi.fn().mockResolvedValue([]),
  listReleases: vi.fn().mockResolvedValue({ items: [], page: { limit: 50, total: 0 } }),
  listModelProviders: vi.fn().mockResolvedValue([]),
  listValidationRuns: vi.fn().mockResolvedValue([]),
  getPolicyDecision: vi.fn().mockResolvedValue(null),
  getProposal: vi.fn().mockResolvedValue({
    id: "prp_01arz3ndektsv4rrffq69g5fav",
    targetObjectType: "semantic_asset",
    targetObjectId: "ast_01arz3ndektsv4rrffq69g5faz",
    state: "in_review",
    title: "Deep-linked proposal",
    summary: "Loaded directly by proposal id.",
    reason: "Review exact target",
    createdBy: "prn_01arz3ndektsv4rrffq69g5fav",
    submittedAt: "2026-09-05T03:00:00Z",
    createdAt: "2026-09-05T02:00:00Z",
    updatedAt: "2026-09-05T03:00:00Z",
    changeSet: [],
  }),
  getRelease: vi.fn().mockResolvedValue({
    id: "rls_01arz3ndektsv4rrffq69g5fav",
    sequence: 42,
    state: "published",
    manifestDigest: `sha256:${"b".repeat(64)}`,
    publishedBy: "prn_01arz3ndektsv4rrffq69g5fav",
    publishedAt: "2026-09-05T04:00:00Z",
    createdAt: "2026-09-05T04:00:00Z",
    manifest: { assets: [], objects: [] },
    authority: "release_manifests",
    availability: "available",
    objectAvailability: "available",
    consumerImpactAvailability: "available",
    diffAvailability: "available",
    consumerImpact: { current: 0, pinned: 0 },
    priorPinDiff: [],
    currentRegistryDiff: [],
  }),
}));

const defaultGetAssetImplementation = getAssetMock.getMockImplementation();
const defaultListAssetsImplementation = listAssetsMock.getMockImplementation();
const defaultListAssetRevisionsImplementation = listAssetRevisionsMock.getMockImplementation();
const defaultListAssetAuthorityRecordsImplementation = listAssetAuthorityRecordsMock.getMockImplementation();

afterEach(() => {
  vi.mocked(getSession).mockResolvedValue(session);
  vi.mocked(listWorkspaces).mockResolvedValue([workspace]);
  if (defaultGetAssetImplementation) getAssetMock.mockImplementation(defaultGetAssetImplementation);
  if (defaultListAssetsImplementation) listAssetsMock.mockImplementation(defaultListAssetsImplementation);
  if (defaultListAssetRevisionsImplementation) listAssetRevisionsMock.mockImplementation(defaultListAssetRevisionsImplementation);
  if (defaultListAssetAuthorityRecordsImplementation) listAssetAuthorityRecordsMock.mockImplementation(defaultListAssetAuthorityRecordsImplementation);
  createWorkspaceMock.mockReset();
  createAssetMock.mockReset();
  window.history.replaceState({}, "", "/");
  window.sessionStorage.clear();
});

function CatalogCreationProbe() {
  const runtime = useCatalogRuntime();
  return <div>
    <button type="button" onClick={() => void runtime.createWorkspace("new-space", "New space")}>创建测试工作区</button>
    <button type="button" onClick={() => void runtime.createAsset({ address: "commerce.created_metric", assetType: "metric", title: "Created metric", summary: "Created after workspace bootstrap." })}>创建测试资产</button>
    <output data-testid="creation-workspace">{runtime.workspaceId}</output>
    <output data-testid="creation-assets">{runtime.assets.map((item) => item.name).join("|")}</output>
  </div>;
}

function CatalogProjectionProbe() {
  const runtime = useCatalogRuntime();
  return <div>
    <button disabled={runtime.loading || !runtime.workspaceId} onClick={() => void runtime.ensureAsset(asset.id)}>读取生产资产</button>
    <output data-testid="production-projection">{runtime.assets.map(item => `${item.name}|${item.owner}`).join(";")}</output>
  </div>;
}

function CatalogWorkspaceProbe() {
  const runtime = useCatalogRuntime();
  return <>
    <SessionAccountControl />
    <button onClick={() => void runtime.ensureAsset(asset.id)}>读取旧详情</button>
    <button onClick={() => void runtime.ensureRevisions(asset.id)}>读取旧修订</button>
    <button onClick={() => void runtime.loadMoreAssets()}>更多旧资产</button>
    <button onClick={() => void runtime.loadMoreRevisions(asset.id)}>更多旧修订</button>
    <button onClick={() => void runtime.loadMoreAuthorityRecords(asset.id, "validation")}>更多旧权威记录</button>
    <output data-testid="workspace-state">{JSON.stringify({ workspaceId: runtime.workspaceId, assets: runtime.assets.map(item => item.name), ids: runtime.catalogAssetIds, details: runtime.detailStates, revisions: runtime.revisionStates, authority: runtime.authorityPageStates, cursor: runtime.nextCursor })}</output>
  </>;
}

describe("production catalog", () => {
  it("follows account-menu workspace selection and discards old catalog, detail and paging responses", async () => {
    const nextWorkspace = { ...workspace, id: "wsp_01arz3ndektsv4rrffq69g5faw", displayName: "合成演示工作区" };
    vi.mocked(getSession).mockResolvedValue({ ...session, workspaces: [...session.workspaces, { ...session.workspaces[0], ...nextWorkspace }] });
    vi.mocked(listWorkspaces).mockResolvedValue([workspace, nextWorkspace]);
    const detail = await defaultGetAssetImplementation!();
    const oldDetail = { ...detail, authoritySections: detail.authoritySections.map((section: { kind: string }) => section.kind === "validation" ? { ...section, recordsPage: { limit: 1, nextCursor: "authority-next" } } : section) };
    getAssetMock.mockResolvedValue(oldDetail);
    listAssetRevisionsMock.mockResolvedValue({ items: [detail.currentRevision], page: { limit: 1, nextCursor: "revision-next" } });
    const oldPage = { items: [asset], page: { limit: 1, nextCursor: "asset-next" } };
    const deferred = () => {
      let resolve!: (value: unknown) => void;
      const promise = new Promise(resolvePromise => { resolve = resolvePromise; });
      return { promise, resolve };
    };
    const oldAppend = deferred();
    const oldDetailRequest = deferred();
    const oldRevisions = deferred();
    const oldAuthority = deferred();
    const newPage = deferred();
    listAssetsMock.mockImplementation((id: string, _query: string, _type: string, cursor?: string) => id === nextWorkspace.id ? newPage.promise : cursor ? oldAppend.promise : Promise.resolve(oldPage));
    const user = userEvent.setup();
    render(<SessionRuntimeProvider><CatalogRuntimeProvider><CatalogWorkspaceProbe /></CatalogRuntimeProvider></SessionRuntimeProvider>);
    const state = () => JSON.parse(screen.getByTestId("workspace-state").textContent!);
    await waitFor(() => expect(state().assets).toEqual([asset.title]));
    await user.click(screen.getByRole("button", { name: "读取旧详情" }));
    await waitFor(() => expect(state().details[asset.id].state).toBe("ready"));
    await user.click(screen.getByRole("button", { name: "读取旧修订" }));
    await waitFor(() => expect(state().revisions[asset.id].state).toBe("ready"));

    getAssetMock.mockReturnValueOnce(oldDetailRequest.promise);
    listAssetRevisionsMock.mockReturnValueOnce(oldRevisions.promise);
    listAssetAuthorityRecordsMock.mockReturnValueOnce(oldAuthority.promise);
    await user.click(screen.getByRole("button", { name: "读取旧详情" }));
    await user.click(screen.getByRole("button", { name: "更多旧修订" }));
    await user.click(screen.getByRole("button", { name: "更多旧权威记录" }));
    await user.click(screen.getByRole("button", { name: "更多旧资产" }));
    await user.click(screen.getByLabelText("账户 Catalog Admin"));
    await user.click(screen.getByRole("radio", { name: "合成演示工作区" }));

    const emptyState = { workspaceId: nextWorkspace.id, assets: [], ids: [], details: {}, revisions: {}, authority: {} };
    expect(state()).toEqual(emptyState);
    await waitFor(() => expect(listAssetsMock).toHaveBeenCalledWith(nextWorkspace.id, "", "", undefined, expect.any(AbortSignal)));
    await act(async () => {
      oldAppend.resolve(oldPage);
      oldDetailRequest.resolve(oldDetail);
      oldRevisions.resolve({ items: [detail.currentRevision], page: { limit: 1 } });
      oldAuthority.resolve({ items: [], page: { limit: 1 } });
    });
    expect(state()).toEqual(emptyState);
    await act(async () => newPage.resolve({ items: [{ ...asset, id: "ast_01arz3ndektsv4rrffq69g5faw", title: "演示订单" }], page: { limit: 100, total: 1 } }));
    expect(state().assets).toEqual(["演示订单"]);
    expect(state().ids).toEqual(["ast_01arz3ndektsv4rrffq69g5faw"]);
    expect(state().details).toEqual({});
    expect(state().revisions).toEqual({});
    expect(state().authority).toEqual({});
  });

  it("keeps the request fingerprint aligned after creating a workspace and its first asset", async () => {
    const createdWorkspace = { ...workspace, id: "wsp_01arz3ndektsv4rrffq69g5faw", slug: "new-space", displayName: "New space" };
    const implementation = getAssetMock.getMockImplementation();
    if (!implementation) throw new Error("catalog detail mock is unavailable");
    const detail = await implementation();
    createWorkspaceMock.mockResolvedValue(createdWorkspace);
    createAssetMock.mockResolvedValue({
      ...detail,
      id: "ast_01arz3ndektsv4rrffq69g5faz",
      workspaceId: createdWorkspace.id,
      address: "commerce.created_metric",
      title: "Created metric",
      summary: "Created after workspace bootstrap.",
      currentRevision: { ...detail.currentRevision, id: "rev_01arz3ndektsv4rrffq69g5faz", assetId: "ast_01arz3ndektsv4rrffq69g5faz", content: { title: "Created metric", summary: "Created after workspace bootstrap." } },
    });
    const user = userEvent.setup();
    render(<CatalogRuntimeProvider><CatalogCreationProbe /></CatalogRuntimeProvider>);

    await user.click(screen.getByRole("button", { name: "创建测试工作区" }));
    expect(await screen.findByTestId("creation-workspace")).toHaveTextContent(createdWorkspace.id);
    await user.click(screen.getByRole("button", { name: "创建测试资产" }));

    expect(createAssetMock).toHaveBeenCalledWith(createdWorkspace.id, expect.objectContaining({ address: "commerce.created_metric" }));
    expect(await screen.findByTestId("creation-assets")).toHaveTextContent("Created metric");
  });

  it("restores the exact source identifier from a production route", async () => {
    window.history.replaceState({}, "", "/sources?source=src_01arz3ndektsv4rrffq69g5fav");
    render(<App />);

    expect(await screen.findByRole("region", { name: "真实来源深链" })).toHaveTextContent("src_01arz3ndektsv4rrffq69g5fav");
  });

  it("restores exact persisted targets when browser history changes", async () => {
    window.history.replaceState({}, "", `/assets?asset=${asset.id}`);
    render(<App />);
    expect(await screen.findByRole("region", { name: "语义资产详情" })).toBeVisible();

    window.history.replaceState({}, "", "/sources?source=src_01arz3ndektsv4rrffq69g5faw");
    window.dispatchEvent(new PopStateEvent("popstate"));
    expect(await screen.findByRole("region", { name: "真实来源深链" })).toHaveTextContent("src_01arz3ndektsv4rrffq69g5faw");
  });

  it("fetches an exact asset deep link even when the target is not on the first catalog page", async () => {
    listAssetsMock.mockResolvedValue({
      items: [{ ...asset, id: "ast_01arz3ndektsv4rrffq69g5faw", currentRevisionId: "rev_01arz3ndektsv4rrffq69g5faw", address: "commerce.other_metric", title: "Other metric" }],
      page: { limit: 1, nextCursor: "next-page" },
    });
    window.history.replaceState({}, "", `/assets?asset=${asset.id}`);
    render(<App />);

    expect(await screen.findByRole("heading", { name: "Net revenue", level: 1 })).toBeVisible();
    expect(getAssetMock).toHaveBeenCalledWith(workspace.id, asset.id);
    expect(screen.getByRole("region", { name: "语义资产详情" })).toHaveTextContent(asset.currentRevisionId);

    await userEvent.click(screen.getByRole("button", { name: "返回知识目录" }));
    expect(await screen.findByRole("button", { name: "打开语义资产 Other metric" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "打开语义资产 Net revenue" })).not.toBeInTheDocument();
  });

  it("fetches an exact release deep link even when the target is not on the first release page", async () => {
    window.history.replaceState({}, "", "/governance?release=rls_01arz3ndektsv4rrffq69g5fav");
    render(<App />);

    const detail = await screen.findByRole("region", { name: "发布 #42 详情" });
    expect(detail).toHaveTextContent("rls_01arz3ndektsv4rrffq69g5fav");
    expect(detail).toHaveTextContent("release_manifests");
  });

  it("fetches an exact proposal deep link even when the target is not on the first proposal page", async () => {
    window.history.replaceState({}, "", "/governance?proposal=prp_01arz3ndektsv4rrffq69g5fav");
    render(<App />);

    const detail = await screen.findByRole("region", { name: /Deep-linked proposal.*候选资产版本详情/ });
    expect(detail).toHaveTextContent("Loaded directly by proposal id.");
    expect(window.location.search).toBe("?proposal=prp_01arz3ndektsv4rrffq69g5fav");
  });

  it("loads a real registry view and opens asset detail", async () => {
    const user = userEvent.setup();
    render(<App />);

    expect(await screen.findByRole("button", { name: "知识库" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "知识库" }));
    expect((await screen.findAllByText("Net revenue")).length).toBeGreaterThan(0);
    expect(screen.getByText("net_revenue")).toBeVisible();
    expect(screen.getByRole("button", { name: "打开语义资产 Net revenue" })).toHaveTextContent("待确认");

    await user.click(screen.getByRole("button", { name: "打开语义资产 Net revenue" }));
    expect(await screen.findByRole("heading", { name: "Net revenue", level: 1 })).toBeVisible();
    expect(screen.getByText("@3")).toBeVisible();
    expect(screen.getByRole("region", { name: "资产权威分区" })).toHaveTextContent("release_manifests");
    expect(screen.getByRole("region", { name: "资产权威分区" })).toHaveTextContent("未发布");
    expect(screen.getByRole("region", { name: "资产权威分区" })).toHaveTextContent("无权访问");
    expect(screen.getByRole("region", { name: "资产权威分区" })).toHaveTextContent("读取失败");
    const assetDetail = screen.getByRole("region", { name: "语义资产详情" });
    expect(assetDetail.querySelector(".asset-header-release")).toHaveTextContent("未发布");
    expect(assetDetail).toHaveTextContent("当前 revision 状态草稿");
    expect(assetDetail).toHaveTextContent("生产固定 revision尚未发布");
    await user.click(screen.getByRole("tab", { name: "本体关系" }));
    expect(screen.getByText("语义关系无权访问")).toBeVisible();
    expect(screen.getByText("Orders source")).toBeVisible();
    await user.click(screen.getByRole("tab", { name: "实现" }));
    expect(screen.getByText("物理绑定未配置")).toBeVisible();
    expect(screen.getByText("JoinContract读取失败")).toBeVisible();
    await user.click(screen.getByRole("tab", { name: "可信度" }));
    expect(screen.getByText("Schema validation")).toBeVisible();
  });

  it("projects production displayName and ownerPrincipalId without losing legacy compatibility", async () => {
    const implementation = getAssetMock.getMockImplementation();
    if (!implementation) throw new Error("catalog detail mock is unavailable");
    const detail = await implementation();
    getAssetMock.mockResolvedValueOnce({ ...detail, currentRevision: { ...detail.currentRevision, content: {
      displayName: "Synthetic production revenue", name: "Legacy name", definition: "Synthetic revenue",
      ownerPrincipalId: "prn_01arz3ndektsv4rrffq69g5fav", owner: "Legacy owner",
    } } });
    render(<CatalogRuntimeProvider><CatalogProjectionProbe /></CatalogRuntimeProvider>);
    await waitFor(() => expect(screen.getByRole("button", { name: "读取生产资产" })).toBeEnabled());
    await userEvent.setup().click(screen.getByRole("button", { name: "读取生产资产" }));
    await waitFor(() => expect(screen.getByTestId("production-projection")).toHaveTextContent("Synthetic production revenue|prn_01arz3ndektsv4rrffq69g5fav"));
  });

  it("keeps a newer current revision in draft while showing the older released basis", async () => {
    const implementation = getAssetMock.getMockImplementation();
    if (!implementation) throw new Error("catalog detail mock is unavailable");
    const detail = await implementation();
    const releasedRevisionId = "rev_01arz3ndektsv4rrffq69g5faa";
    getAssetMock.mockResolvedValue({
      ...detail,
      authoritySections: detail.authoritySections.map((section: { kind: string }) => section.kind === "released_state" ? {
        kind: "released_state",
        authority: "release_manifests",
        availability: "available",
        revisionId: releasedRevisionId,
        releaseId: "rel_01arz3ndektsv4rrffq69g5fav",
        releaseSequence: 7,
        values: {},
        records: [],
        recordsPage: { limit: 100, total: 0 },
      } : section),
    });
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "知识库" }));
    await user.click(await screen.findByRole("button", { name: "打开语义资产 Net revenue" }));
    const assetDetail = await screen.findByRole("region", { name: "语义资产详情" });
    const authority = screen.getByRole("region", { name: "资产权威分区" });
    expect(assetDetail).toHaveTextContent("当前草稿");
    expect(assetDetail).toHaveTextContent("当前 revision 状态草稿");
    expect(assetDetail).toHaveTextContent(`生产固定 revision${releasedRevisionId}`);
    expect(assetDetail).not.toHaveTextContent("生产健康");
    expect(authority).toHaveTextContent(asset.currentRevisionId);
    expect(authority).toHaveTextContent(releasedRevisionId);
    expect(authority).toHaveTextContent("rel_01arz3ndektsv4rrffq69g5fav");
  });

  it("does not report healthy validation when the available section has no completed run count", async () => {
    const implementation = getAssetMock.getMockImplementation();
    if (!implementation) throw new Error("catalog detail mock is unavailable");
    const detail = await implementation();
    getAssetMock.mockResolvedValue({
      ...detail,
      authoritySections: detail.authoritySections.map((section: { kind: string }) => {
        if (section.kind === "released_state") return {
          kind: "released_state",
          authority: "release_manifests",
          availability: "available",
          revisionId: asset.currentRevisionId,
          releaseId: "rel_01arz3ndektsv4rrffq69g5fav",
          releaseSequence: 7,
          values: {},
          records: [],
          recordsPage: { limit: 100, total: 0 },
        };
        if (section.kind === "validation") return {
          kind: "validation",
          authority: "validation_runs",
          availability: "available",
          revisionId: asset.currentRevisionId,
          values: { blockerCount: 0, warningCount: 0 },
          records: [],
          recordsPage: { limit: 100, total: 0 },
        };
        return section;
      }),
    });
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "知识库" }));
    await user.click(await screen.findByRole("button", { name: "打开语义资产 Net revenue" }));
    const assetDetail = await screen.findByRole("region", { name: "语义资产详情" });
    expect(assetDetail).toHaveTextContent("生产");
    expect(assetDetail).toHaveTextContent("当前 revision 状态已发布");
    expect(assetDetail).toHaveTextContent("验证不完整");
    expect(assetDetail).not.toHaveTextContent("健康");
  });

  it("does not substitute the current asset revision for missing relation or lineage authority bases", async () => {
    const implementation = getAssetMock.getMockImplementation();
    if (!implementation) throw new Error("catalog detail mock is unavailable");
    const detail = await implementation();
    getAssetMock.mockResolvedValue({
      ...detail,
      authoritySections: detail.authoritySections.map((section: { kind: string }) => {
        if (section.kind === "relations") return { kind: "relations", authority: "asset_relations", availability: "available", values: {}, records: [], recordsPage: { limit: 100, total: 0 } };
        if (section.kind === "lineage") return {
          kind: "lineage",
          authority: "lineage_edges",
          availability: "available",
          values: { edgeCount: 1 },
          records: [{ kind: "lineage", id: "lge_01arz3ndektsv4rrffq69g5fav", authority: "lineage_edges", status: "active", label: "Orders source", version: 1 }],
          recordsPage: { limit: 100, total: 1 },
        };
        return section;
      }),
    });
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "知识库" }));
    await user.click(await screen.findByRole("button", { name: "打开语义资产 Net revenue" }));
    await user.click(await screen.findByRole("tab", { name: "本体关系" }));
    const relations = screen.getByRole("region", { name: "权威语义关系" });
    const lineage = screen.getByRole("region", { name: "权威数据血缘" });
    expect(relations).toHaveTextContent("服务端未返回 revision 基准");
    expect(lineage).toHaveTextContent("服务端未返回 revision 基准");
    expect(relations).not.toHaveTextContent(asset.currentRevisionId);
    expect(lineage).not.toHaveTextContent(asset.currentRevisionId);
    expect(lineage).toHaveTextContent("服务端未返回 release 基准");
  });

  it("uses server cursors and totals to load the complete asset and revision lists", async () => {
    const secondAsset = { ...asset, id: "ast_01arz3ndektsv4rrffq69g5faw", address: "commerce.gross_revenue", title: "Gross revenue", currentRevisionId: "rev_01arz3ndektsv4rrffq69g5faw" };
    listAssetsMock.mockImplementation(async (_workspaceId: string, _search: string, _assetType: string, cursor?: string) => cursor
      ? { items: [secondAsset], page: { limit: 1, total: 2 } }
      : { items: [asset], page: { limit: 1, total: 2, nextCursor: "assets-next" } });
    listAssetRevisionsMock.mockImplementation(async (_workspaceId: string, _assetId: string, cursor?: string) => cursor
      ? { items: [{ id: "rev_01arz3ndektsv4rrffq69g5faa", assetId: asset.id, sequence: 2, schemaVersion: "1.0.0", contentDigest: "b".repeat(64), content: {}, createdBy: "catalog-web", createdAt: asset.updatedAt, evidence: [] }], page: { limit: 1, total: 2 } }
      : { items: [{ id: asset.currentRevisionId, assetId: asset.id, sequence: 3, schemaVersion: "1.0.0", contentDigest: "a".repeat(64), content: {}, createdBy: "catalog-web", createdAt: asset.updatedAt, evidence: [] }], page: { limit: 1, total: 2, nextCursor: "revisions-next" } });
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "知识库" }));
    await waitFor(() => expect(document.querySelector(".catalog-summary")).toHaveTextContent("已加载 1 / 2 个语义资产"));
    await user.click(screen.getByRole("button", { name: "加载更多资产" }));
    expect(await screen.findByRole("button", { name: "打开语义资产 Gross revenue" })).toBeVisible();

    await user.click(screen.getByRole("button", { name: "打开语义资产 Net revenue" }));
    await user.click(await screen.findByRole("tab", { name: "定义" }));
    const revisions = await screen.findByRole("region", { name: "不可变修订历史" });
    expect(revisions).toHaveTextContent("1 / 2 项");
    await user.click(within(revisions).getByRole("button", { name: "加载更多修订" }));
    expect(await within(revisions).findByText("@2")).toBeVisible();
    expect(listAssetRevisionsMock).toHaveBeenLastCalledWith(workspace.id, asset.id, "revisions-next");
  });

  it("discards a pending catalog page when the server search fingerprint changes", async () => {
    const staleAsset = { ...asset, id: "ast_01arz3ndektsv4rrffq69g5fax", address: "commerce.stale_metric", title: "Stale metric", currentRevisionId: "rev_01arz3ndektsv4rrffq69g5fax" };
    const searchedAsset = { ...asset, id: "ast_01arz3ndektsv4rrffq69g5fay", address: "commerce.fresh_metric", title: "Fresh metric", currentRevisionId: "rev_01arz3ndektsv4rrffq69g5fay" };
    let resolveStalePage: ((page: { items: typeof staleAsset[]; page: { limit: number; total: number } }) => void) | undefined;
    const stalePage = new Promise<{ items: typeof staleAsset[]; page: { limit: number; total: number } }>((resolve) => { resolveStalePage = resolve; });
    listAssetsMock.mockImplementation(async (_workspaceId: string, search: string, _assetType: string, cursor?: string) => {
      if (cursor) return stalePage;
      if (search === "Fresh") return { items: [searchedAsset], page: { limit: 1, total: 1 } };
      return { items: [asset], page: { limit: 1, total: 2, nextCursor: "stale-next" } };
    });
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "知识库" }));
    await user.click(await screen.findByRole("button", { name: "加载更多资产" }));
    await user.type(screen.getByRole("searchbox", { name: "搜索知识目录" }), "Fresh");
    expect(await screen.findByRole("button", { name: "打开语义资产 Fresh metric" })).toBeVisible();

    resolveStalePage?.({ items: [staleAsset], page: { limit: 1, total: 2 } });
    await waitFor(() => expect(screen.queryByRole("button", { name: "打开语义资产 Stale metric" })).not.toBeInTheDocument());
    expect(document.querySelector(".catalog-summary")).toHaveTextContent("已加载 1 / 1 个语义资产");
  });

  it("renders exact typed authority facts and appends authority records by server cursor", async () => {
    const implementation = getAssetMock.getMockImplementation();
    if (!implementation) throw new Error("catalog detail mock is unavailable");
    const detail = await implementation();
    getAssetMock.mockResolvedValue({
      ...detail,
      authoritySections: detail.authoritySections.map((section: { kind: string }) => {
        if (section.kind === "physical_bindings") return { ...section, availability: "available", records: [
          { kind: "physical_binding", id: "pbd_01arz3ndektsv4rrffq69g5fav", authority: "physical_bindings", status: "active", label: "Revenue amount", version: 4, physicalBinding: { assetId: asset.id, datasetId: "pds_orders", fieldId: "pdf_revenue", transform: "gross_amount - refunds" } },
          { kind: "model_grain", id: "grn_01arz3ndektsv4rrffq69g5fav", authority: "model_grains", status: "active", label: "Order grain", version: 2, modelGrain: { assetId: asset.id, grainExpression: "one row per order", grainFieldRefs: ["pdf_order_id"], documentedBy: "evd_grain" } },
          { kind: "entity_key", id: "key_01arz3ndektsv4rrffq69g5fav", authority: "entity_keys", status: "active", label: "Order key", version: 1, entityKey: { assetId: asset.id, keyFieldRefs: ["pdf_order_id"], uniquenessSemantics: "deduplicated" } },
        ], recordsPage: { limit: 3, total: 4, nextCursor: "physical-next" } };
        if (section.kind === "join_contracts") return { ...section, availability: "available", records: [{ kind: "join_contract", id: "jnc_01arz3ndektsv4rrffq69g5fav", authority: "join_contracts", status: "active", label: "Orders to customers", version: 5, joinContract: { direction: "outgoing", leftDatasetId: "pds_orders", rightDatasetId: "pds_customers", leftFieldRefs: ["pdf_customer_id"], rightFieldRefs: ["pdf_id"], joinType: "left", cardinality: "many_to_one", joinExpression: "orders.customer_id = customers.id" } }], recordsPage: { limit: 1, total: 1 } };
        if (section.kind === "relations") return { ...section, availability: "available", records: [{ kind: "relation", id: "rel_01arz3ndektsv4rrffq69g5fav", authority: "asset_relations", status: "asserted", label: "Depends on orders", version: 3, relation: { direction: "outgoing", predicate: "depends_on", plane: "dependency", assertionState: "asserted", subjectAssetId: asset.id, objectAssetId: "ast_orders" } }], recordsPage: { limit: 1, total: 1 } };
        if (section.kind === "lineage") return { ...section, availability: "available", records: [{ kind: "lineage", id: "lge_01arz3ndektsv4rrffq69g5fav", authority: "lineage_edges", status: "active", label: "Orders source", version: 7, lineage: { direction: "incoming", upstreamDatasetId: "pds_raw_orders", downstreamDatasetId: "pds_orders", edgeKind: "derived_from", sourceRevisionId: "srv_orders_v7", codeArtifactId: "car_orders_model", confidence: 0.98 } }], recordsPage: { limit: 1, total: 1 } };
        if (section.kind === "consumer_impact") return { ...section, availability: "available", records: [{ kind: "consumer_binding", id: "cbd_01arz3ndektsv4rrffq69g5fav", authority: "consumer_bindings", status: "active", label: "Finance dashboard", version: 6, consumerBinding: { consumerId: "csm_finance", effectiveReleaseId: "rls_finance", environment: "production", purpose: "month close", mode: "pinned", status: "active", compatibilityConstraint: { schema: "v2" }, expiresAt: "2026-12-01T00:00:00Z" } }], recordsPage: { limit: 1, total: 1 } };
        return section;
      }),
    });
    listAssetAuthorityRecordsMock.mockResolvedValue({ items: [{ kind: "physical_binding", id: "pbd_01arz3ndektsv4rrffq69g5faw", authority: "physical_bindings", status: "active", label: "Currency", version: 1, physicalBinding: { assetId: asset.id, datasetId: "pds_orders", fieldId: "pdf_currency" } }], page: { limit: 1, total: 4 } });
    const user = userEvent.setup();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "知识库" }));
    await user.click(await screen.findByRole("button", { name: "打开语义资产 Net revenue" }));
    await user.click(await screen.findByRole("tab", { name: "实现" }));
    expect(screen.getByText("gross_amount - refunds")).toBeVisible();
    expect(screen.getByText("one row per order")).toBeVisible();
    expect(screen.getByText("deduplicated")).toBeVisible();
    expect(screen.getByText("orders.customer_id = customers.id")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "加载更多权威记录" }));
    expect(await screen.findByText("pdf_currency")).toBeVisible();
    expect(listAssetAuthorityRecordsMock).toHaveBeenCalledWith(workspace.id, asset.id, "physical_bindings", "physical-next");

    await user.click(screen.getByRole("tab", { name: "本体关系" }));
    expect(screen.getByText("outgoing · depends_on")).toBeVisible();
    expect(screen.getByText("pds_raw_orders")).toBeVisible();
    expect(screen.getByText("car_orders_model")).toBeVisible();
    await user.click(screen.getByRole("tab", { name: "交付与影响" }));
    expect(screen.getByText("csm_finance · pinned")).toBeVisible();
    expect(screen.getByText('{"schema":"v2"}')).toBeVisible();
  });

  it("refreshes from the catalog without clearing its assets", async () => {
    const user = userEvent.setup();
    listAssetsMock.mockClear();
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "知识库" }));
    await waitFor(() => expect(listAssetsMock).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole("button", { name: "刷新知识目录" }));

    await waitFor(() => expect(listAssetsMock).toHaveBeenCalledTimes(2));
    await user.click(screen.getByRole("button", { name: "知识库" }));
    expect(screen.getByRole("button", { name: "打开语义资产 Net revenue" })).toBeVisible();
  });

  it("keeps a production detail failure explicit instead of falling back to fixture content", async () => {
    const user = userEvent.setup();
    getAssetMock.mockRejectedValueOnce(new Error("catalog authority unavailable"));
    render(<App />);

    await user.click(await screen.findByRole("button", { name: "知识库" }));
    await user.click(await screen.findByRole("button", { name: "打开语义资产 Net revenue" }));
    const failure = await screen.findByRole("alert");
    expect(failure).toHaveTextContent("资产详情读取失败");
    expect(failure).toHaveTextContent("catalog authority unavailable");
    expect(screen.queryByText("净收入", { exact: true })).not.toBeInTheDocument();
  });
});

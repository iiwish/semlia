import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, vi } from "vitest";

import { CapabilityProvider } from "./authorization";
import { CatalogRuntimeProvider } from "./testing/catalogFixture";
import { CompatibilityImpactView, ProductApp } from "./ProductApp";
import type { CapabilitySession } from "./types";
import { assetTypeProfiles, evaluateAssetTypeRules } from "./assetTypeProfiles";
import { assets, authorizationRoles, authorizationSession } from "./testing/data";

vi.mock("./SemanticProductionPanel", () => ({
  SemanticProductionWorkspace: ({ operationId, onOperationSelected, onBack, backLabel }: { operationId?: string; onOperationSelected: (id: string) => void; onBack?: () => void; backLabel?: string }) => <section aria-label="知识确认">{operationId ? <button onClick={onBack}>{backLabel}</button> : <button onClick={() => onOperationSelected("prodop_menu_test")}>打开测试草稿</button>}</section>,
}));

vi.mock("./sessionRuntime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./sessionRuntime")>();
  return { ...actual, useSessionRuntime: () => ({ activeWorkspaceId: "wsp_01arz3ndektsv4rrffq69g5fav", session: null }) };
});

vi.mock("./authorizationAdminRuntime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./authorizationAdminRuntime")>();
  const { useMemo } = await import("react");
  const { createAuthorizationFixtureApi } = await import("./testing/authorizationFixture");
  return { ...actual, AuthorizationAdminRuntimeProvider: function TestAuthorizationProvider(props: React.ComponentProps<typeof actual.AuthorizationAdminRuntimeProvider>) {
    const api = useMemo(() => createAuthorizationFixtureApi(props.workspaceId), [props.workspaceId]);
    return <actual.AuthorizationAdminRuntimeProvider {...props} authorizationVersion="1" api={api} />;
  } };
});

vi.mock("./operationsRuntime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./operationsRuntime")>();
  const { useMemo } = await import("react");
  const { createOperationsFixtureApi } = await import("./testing/operationsFixture");
  return { ...actual, OperationsRuntimeProvider: function TestOperationsProvider(props: React.ComponentProps<typeof actual.OperationsRuntimeProvider>) {
    const api = useMemo(() => createOperationsFixtureApi(), []);
    return <actual.OperationsRuntimeProvider {...props} api={api} />;
  } };
});

vi.mock("./governanceRuntime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./governanceRuntime")>();
  const { useMemo } = await import("react");
  const { createGovernanceFixtureApi } = await import("./testing/governanceFixtureApi");
  return { ...actual, GovernanceRuntimeProvider: function TestGovernanceProvider(props: React.ComponentProps<typeof actual.GovernanceRuntimeProvider>) {
    const api = useMemo(() => createGovernanceFixtureApi(), []);
    return <actual.GovernanceRuntimeProvider {...props} api={api} />;
  } };
});

vi.mock("./workbenchRuntime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./workbenchRuntime")>();
  const { useMemo } = await import("react");
  const { createWorkbenchFixtureApi } = await import("./testing/workbenchFixture");
  return { ...actual, WorkbenchRuntimeProvider: function TestWorkbenchProvider(props: React.ComponentProps<typeof actual.WorkbenchRuntimeProvider>) {
    const api = useMemo(() => createWorkbenchFixtureApi(), []);
    return <actual.WorkbenchRuntimeProvider {...props} api={api} />;
  } };
});

function App({ session = authorizationSession }: { session?: CapabilitySession }) {
  return <CatalogRuntimeProvider fixtureAssets={assets}><ProductApp session={session} /></CatalogRuntimeProvider>;
}

async function openReviewTask(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "待办" }));
  await user.click(await screen.findByRole("button", { name: "进入审核：确认客单价退款订单口径" }));
  await user.click(await screen.findByRole("button", { name: "进入审核" }));
}

async function openReleaseHistory(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole("button", { name: "知识库" }));
  await user.click(screen.getByRole("button", { name: "发布记录" }));
}

const authorSession: CapabilitySession = { principalId: "prn_01arz3ndektsv4rrffq69g5fav", version: "1", capabilities: [...new Set(authorizationRoles.filter((role) => ["ROLE-ASSET-OWNER", "ROLE-SOURCE-OPERATOR"].includes(role.id)).flatMap((role) => role.permissions)), "workspace.manage", "member.manage", "role.assign"] };

afterEach(() => {
  window.history.replaceState({}, "", "/");
  vi.unstubAllGlobals();
});

describe("Semlia product workspace", () => {
  it("offers creation rather than clearing nonexistent filters in an empty catalog", async () => {
    window.history.replaceState({}, "", "/assets");
    render(<CatalogRuntimeProvider fixtureAssets={[]}><ProductApp session={authorizationSession} /></CatalogRuntimeProvider>);
    expect(await screen.findByText("尚无知识资产")).toBeVisible();
    expect(screen.queryByRole("button", { name: "清除筛选" })).not.toBeInTheDocument();
    const empty = screen.getByText("尚无知识资产").closest(".catalog-empty")!;
    await userEvent.click(within(empty as HTMLElement).getByRole("button", { name: "新建知识" }));
    expect(screen.getByRole("dialog", { name: "新建知识" })).toBeVisible();
    expect(screen.getByLabelText("语义地址")).toHaveFocus();
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog", { name: "新建知识" })).not.toBeInTheDocument();
    expect(within(empty as HTMLElement).getByRole("button", { name: "新建知识" })).toHaveFocus();
  });
  it("retains the draft entry context through detail, reload and back", async () => {
    const user = userEvent.setup();
    window.history.replaceState({}, "", "/assets?section=drafts");
    const first = render(<App />);
    await user.click(screen.getByRole("button", { name: "打开测试草稿" }));
    expect(screen.getByRole("button", { name: "知识库" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "草稿与整理" })).toHaveAttribute("aria-current", "page");
    expect(window.location.search).toContain("from=drafts");
    first.unmount();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "返回草稿与整理" }));
    expect(await screen.findByRole("button", { name: "打开测试草稿" })).toBeVisible();
    expect(window.location.pathname + window.location.search).toBe("/assets?section=drafts");
  });

  it("preserves the originating inbox scope when returning from a direct knowledge link", async () => {
    const user = userEvent.setup();
    window.history.replaceState({}, "", "/governance?production=prodop_menu_test&scope=initiated");
    render(<App />);
    expect(screen.queryByRole("complementary", { name: "治理上下文" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "返回待办" }));
    expect(screen.getByRole("button", { name: "我发起" })).toHaveAttribute("aria-pressed", "true");
    expect(window.location.search).toBe("?view=overview&scope=initiated");
  });

  it("keeps batch review separate from individual actions and release history", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: "批量审核" }));
    expect(await screen.findByRole("region", { name: "评审批次" })).toBeVisible();
    expect(screen.queryByRole("region", { name: "语义资产版本列表" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /查看候选资产版本|当前版本|历史版本/ })).not.toBeInTheDocument();
  });

  it("keeps the topbar free of simulated identity and workspace switching", async () => {
    const user = userEvent.setup();
    render(<App session={authorSession} />);
    const topbar = document.querySelector(".topbar") as HTMLElement;
    expect(within(topbar).queryByRole("combobox", { name: "工作区" })).not.toBeInTheDocument();
    expect(within(topbar).getByText("模拟数据")).toBeVisible();
    expect(within(topbar).queryByRole("combobox", { name: "本机验收身份" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    expect(screen.queryByRole("button", { name: /本机验收/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "知识库" }));
    expect(screen.getByRole("button", { name: "刷新知识目录" })).toBeVisible();
    expect(within(topbar).queryByRole("button", { name: "刷新知识目录" })).not.toBeInTheDocument();
  });

  it("does not expose acceptance tools in a regular session", async () => {
    const user = userEvent.setup();
    render(<App />);
    expect(screen.queryByRole("combobox", { name: "本机验收身份" })).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "工作区" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    expect(screen.queryByRole("button", { name: /本机验收/ })).not.toBeInTheDocument();
  });

  it("uses a three-layer semantic operations workspace", () => {
    render(<App />);

    const primaryNavigation = screen.getByLabelText("Semlia 主功能");
    expect(primaryNavigation).toBeInTheDocument();
    expect(within(primaryNavigation).getAllByRole("button").map((button) => button.getAttribute("aria-label"))).toEqual(["问数", "知识库", "待办", "数据接入"]);
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    expect(context).toBeInTheDocument();
    expect(within(context).queryByText("语义问答", { exact: true })).not.toBeInTheDocument();
    expect(within(context).queryByText("示例工作区")).not.toBeInTheDocument();
    expect(within(context).queryByText("MX", { exact: true })).not.toBeInTheDocument();
    const contextHeader = context.querySelector(".context-panel-header");
    const contextSearch = within(context).getByRole("searchbox", { name: "搜索会话" });
    expect(contextHeader).toHaveTextContent("最近会话");
    expect(context.firstElementChild).toBe(contextHeader);
    expect(contextHeader?.nextElementSibling).toBe(contextSearch.closest(".context-search"));
    expect(screen.getByRole("main")).toHaveClass("workspace-canvas");
  });

  it("keeps the complete product navigation available during local acceptance", async () => {
    const user = userEvent.setup();
    render(<App session={authorSession} />);

    const primaryNavigation = screen.getByLabelText("Semlia 主功能");
    expect(within(primaryNavigation).getAllByRole("button").map((button) => button.getAttribute("aria-label"))).toEqual(["问数", "知识库", "待办", "数据接入"]);
    expect(within(screen.getByLabelText("Semlia 平台管理")).getByRole("button", { name: "系统设置" })).toBeVisible();

    await user.click(within(primaryNavigation).getByRole("button", { name: "问数" }));
    expect(screen.getByRole("textbox", { name: "向 Semlia 提问" })).toBeVisible();
  });

  it("restores a persisted Workbench item from its URL", async () => {
    window.history.replaceState({}, "", "/?view=overview&attentionItem=ati_01arz3ndektsv4rrffq69g5fax");
    render(<App />);

    const detail = await screen.findByRole("region", { name: "工作台待办详情" });
    expect(within(detail).getByRole("heading", { name: "检查客户增长域运行异常" })).toBeVisible();
    expect(within(detail).getByText("ati_01arz3ndektsv4rrffq69g5fax")).toBeVisible();
    expect(screen.getByRole("button", { name: "待办" })).toHaveAttribute("aria-current", "page");
  });

  it("restores asset and immutable release detail routes without a browser merge", async () => {
    const asset = assets[0];
    window.history.replaceState({}, "", `/assets?asset=${encodeURIComponent(asset.id)}`);
    const assetRender = render(<App />);
    expect(await screen.findByRole("heading", { name: asset.name, level: 1 })).toBeVisible();
    expect(screen.getByRole("region", { name: "语义资产详情" })).toBeVisible();
    assetRender.unmount();

    window.history.replaceState({}, "", "/governance?release=release-2026.08.3");
    render(<App />);
    const release = await screen.findByRole("region", { name: "发布 #6 详情" });
    expect(within(release).getByText("fixture.release_manifest")).toBeVisible();
    expect(within(release).queryByText("Fluxale Production")).not.toBeInTheDocument();
  });

  it("restores a compatibility route without a redundant consumer parameter", async () => {
    window.history.replaceState({}, "", "/delivery/compatibility?binding=cbd_01arz3ndektsv4rrffq69g5fav&query=smq_01arz3ndektsv4rrffq69g5fav");
    render(<App />);

    const detail = await screen.findByRole("region", { name: "兼容性影响详情" });
    expect(detail).toHaveTextContent("由 binding 解析中");
    expect(detail).toHaveTextContent("cbd_01arz3ndektsv4rrffq69g5fav");
    expect(detail).toHaveTextContent("smq_01arz3ndektsv4rrffq69g5fav");
  });

  it("loads the exact persisted consumer, binding, and refused query for a compatibility target", async () => {
    const consumerId = "csm_01arz3ndektsv4rrffq69g5fav";
    const bindingId = "cbd_01arz3ndektsv4rrffq69g5fav";
    const queryId = "smq_01arz3ndektsv4rrffq69g5fav";
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      const body = url.includes(`/consumers/${consumerId}`) ? {
        id: consumerId, stableKey: "finance-dashboard", name: "Finance dashboard", kind: "application", status: "active", ownerPrincipalRef: "prn_finance", metadata: {}, createdAt: "2026-09-05T01:00:00Z", updatedAt: "2026-09-05T01:00:00Z",
      } : url.includes(`/consumer-bindings/${bindingId}`) ? {
        id: bindingId, consumerId, environment: "production", purpose: "Quarter close", mode: "pinned", releaseId: "rls_01arz3ndektsv4rrffq69g5fav", compatibilityConstraint: { requires: "region_v2" }, status: "active", version: 3, createdAt: "2026-09-05T01:00:00Z", updatedAt: "2026-09-05T01:00:00Z",
      } : {
        id: queryId, schemaVersion: "1.0.0", resolverVersion: "resolver-9", requestDigest: `sha256:${"a".repeat(64)}`, outcome: "refused", channel: "api", releaseId: "rls_01arz3ndektsv4rrffq69g5fav", consumerId, bindingId,
        refusal: { code: "STALE_RELEASE_BINDING", candidateIds: [], clarification: "Pin the consumer to a compatible release.", details: {} },
        validation: { id: "qvr_01arz3ndektsv4rrffq69g5fav", queryId, validator: "compatibility", validatorVersion: "1.0.0", inputDigest: `sha256:${"b".repeat(64)}`, status: "failed", results: [{ severity: "blocker", code: "STALE_RELEASE_BINDING", message: "Pinned release is stale.", details: {} }], createdAt: "2026-09-05T01:00:00Z", completedAt: "2026-09-05T01:00:01Z" },
        createdAt: "2026-09-05T01:00:00Z",
      };
      return new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    vi.stubGlobal("fetch", fetchMock);
    const compatibilitySession: CapabilitySession = { principalId: "prn_reader", version: "authz-1", capabilities: ["binding.read", "semantic.resolve"] };
    render(<CapabilityProvider session={compatibilitySession}><CompatibilityImpactView workspaceId="wsp_01arz3ndektsv4rrffq69g5fav" consumerId={consumerId} bindingId={bindingId} queryId={queryId} /></CapabilityProvider>);

    const detail = await screen.findByRole("region", { name: "兼容性影响详情" });
    expect(within(detail).getByText("Finance dashboard")).toBeVisible();
    expect(within(detail).getByText("pinned / active")).toBeVisible();
    expect(within(detail).getAllByText("STALE_RELEASE_BINDING").length).toBeGreaterThan(0);
    expect(within(detail).getByText("Pin the consumer to a compatible release.")).toBeVisible();
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("keeps compatibility partitions independent when one read is forbidden", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/consumers/")) return new Response(JSON.stringify({ code: "CAPABILITY_DENIED", message: "consumer is outside scope" }), { status: 403, headers: { "Content-Type": "application/json" } });
      return new Response(JSON.stringify({ id: "cbd_01arz3ndektsv4rrffq69g5fav", consumerId: "csm_01arz3ndektsv4rrffq69g5fav", environment: "production", purpose: "Quarter close", mode: "current", compatibilityConstraint: {}, status: "active", version: 2, createdAt: "2026-09-05T01:00:00Z", updatedAt: "2026-09-05T01:00:00Z" }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    vi.stubGlobal("fetch", fetchMock);
    const session: CapabilitySession = { principalId: "prn_reader", version: "authz-1", capabilities: ["binding.read"] };
    render(<CapabilityProvider session={session}><CompatibilityImpactView workspaceId="wsp_01arz3ndektsv4rrffq69g5fav" consumerId="csm_01arz3ndektsv4rrffq69g5fav" bindingId="cbd_01arz3ndektsv4rrffq69g5fav" queryId="smq_01arz3ndektsv4rrffq69g5fav" /></CapabilityProvider>);

    expect(await within(screen.getByRole("region", { name: "消费绑定事实" })).findByText("Quarter close")).toBeVisible();
    expect(within(screen.getByRole("region", { name: "消费者事实" })).getByRole("alert")).toHaveTextContent("consumer is outside scope");
    expect(within(screen.getByRole("region", { name: "语义查询事实" })).getByRole("alert")).toHaveTextContent("需要 semantic.resolve 权限");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("resolves a missing compatibility consumer from the persisted binding", async () => {
    const consumerId = "csm_01arz3ndektsv4rrffq69g5fav";
    const bindingId = "cbd_01arz3ndektsv4rrffq69g5fav";
    const queryId = "smq_01arz3ndektsv4rrffq69g5fav";
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes(`/consumer-bindings/${bindingId}`)) return new Response(JSON.stringify({ id: bindingId, consumerId, environment: "production", purpose: "Month close", mode: "current", compatibilityConstraint: {}, status: "active", version: 2, createdAt: "2026-09-05T01:00:00Z", updatedAt: "2026-09-05T01:00:00Z" }), { status: 200, headers: { "Content-Type": "application/json" } });
      if (url.includes(`/consumers/${consumerId}`)) return new Response(JSON.stringify({ id: consumerId, stableKey: "finance-dashboard", name: "Finance dashboard", kind: "application", status: "active", ownerPrincipalRef: "prn_finance", metadata: {}, createdAt: "2026-09-05T01:00:00Z", updatedAt: "2026-09-05T01:00:00Z" }), { status: 200, headers: { "Content-Type": "application/json" } });
      return new Response(JSON.stringify({ id: queryId, schemaVersion: "1.0.0", resolverVersion: "resolver-9", requestDigest: `sha256:${"a".repeat(64)}`, outcome: "resolved", channel: "api", consumerId, bindingId, createdAt: "2026-09-05T01:00:00Z" }), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    vi.stubGlobal("fetch", fetchMock);
    const session: CapabilitySession = { principalId: "prn_reader", version: "authz-1", capabilities: ["binding.read", "semantic.resolve"] };
    render(<CapabilityProvider session={session}><CompatibilityImpactView workspaceId="wsp_01arz3ndektsv4rrffq69g5fav" bindingId={bindingId} queryId={queryId} /></CapabilityProvider>);

    expect(await within(screen.getByRole("region", { name: "消费者事实" })).findByText("Finance dashboard")).toBeVisible();
    expect(screen.getByRole("region", { name: "兼容性影响详情" })).toHaveTextContent(consumerId);
    expect(fetchMock.mock.calls.some(([input]) => String(input instanceof Request ? input.url : input).includes(`/consumers/${consumerId}`))).toBe(true);
  });

  it("searches the session-only Ask conversation directly from context", async () => {
    const user = userEvent.setup();
    render(<App />);

    const context = screen.getByRole("complementary", { name: "治理上下文" });
    const conversationSearch = within(context).getByRole("searchbox", { name: "搜索会话" });
    await user.click(screen.getByRole("button", { name: "编辑会话标题：新会话" }));
    const titleInput = screen.getByRole("textbox", { name: "编辑会话标题" });
    await user.clear(titleInput);
    await user.type(titleInput, "客单价口径");
    await user.keyboard("{Enter}");
    await user.keyboard("{Control>}k{/Control}");
    expect(conversationSearch).toHaveFocus();

    await user.type(conversationSearch, "客单价");
    expect(within(context).getByRole("button", { name: /客单价口径/ })).toBeVisible();
    expect(within(context).queryByRole("button", { name: /8 月收入诊断/ })).not.toBeInTheDocument();

    await user.clear(conversationSearch);
    await user.type(conversationSearch, "不存在的会话");
    expect(within(context).getByText("没有匹配会话")).toBeVisible();
    await user.click(within(context).getByRole("button", { name: "清除会话搜索" }));
    expect(within(context).getByRole("button", { name: /客单价口径/ })).toBeVisible();
  });

  it("collapses, reopens and resizes the secondary menu while keeping a single-level page title", async () => {
    const user = userEvent.setup();
    render(<App />);

    const initialContext = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(initialContext).getByRole("button", { name: "收起二级菜单" }));
    expect(screen.queryByRole("complementary", { name: "治理上下文" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "数据接入" }));
    const reopenedContext = screen.getByRole("complementary", { name: "治理上下文" });
    expect(within(reopenedContext).getByText("数据接入", { exact: true })).toBeVisible();
    expect(within(reopenedContext).queryByText(/页面/)).not.toBeInTheDocument();
    expect(document.querySelector(".workspace-breadcrumb")).toHaveTextContent(/^数据来源$/);

    const resizeHandle = within(reopenedContext).getByRole("separator", { name: "调整二级菜单宽度" });
    resizeHandle.focus();
    await user.keyboard("{End}");
    expect(resizeHandle).toHaveAttribute("aria-valuenow", "440");
    expect(document.querySelector(".app-shell")).toHaveStyle("--context-panel-width: 440px");
  });

  it("opens the normal Ask composer without fabricating an answer", async () => {
    const user = userEvent.setup();
    render(<App />);

    expect(screen.queryByText("原型 · 模拟数据")).not.toBeInTheDocument();
    expect(screen.queryByText(/稳定版 ·/)).not.toBeInTheDocument();
    expect(screen.queryByText("可信语义问答")).not.toBeInTheDocument();
    expect(screen.queryByText("知识索引就绪")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /编辑会话标题：新会话/ }));
    const titleInput = screen.getByRole("textbox", { name: "编辑会话标题" });
    await user.clear(titleInput);
    await user.type(titleInput, "华东收入复盘");
    await user.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: /编辑会话标题：华东收入复盘/ })).toBeVisible();
    expect(screen.queryByRole("region", { name: "数据库到 LLM 问答链路" })).not.toBeInTheDocument();
    expect(screen.queryByText("Prototype 预览")).not.toBeInTheDocument();
    expect(screen.getByText("仅使用已发布知识 · 查询需单独确认")).toBeVisible();
    expect(screen.queryByText("净收入确认口径")).not.toBeInTheDocument();
    expect(screen.queryByText("Semlia 检索已发布知识块和语义资产，委托 Cube 执行查询，并保留答案、口径与来源之间的完整链路。")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /连接数据/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("complementary", { name: "回答执行与证据" })).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "向 Semlia 提问" })).toBeVisible();
  });

  it("enables normal Ask input without fabricated evidence", () => {
    render(<App />);

    expect(screen.queryByRole("button", { name: /KB-2048/ })).not.toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "向 Semlia 提问" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "发送问题" })).toBeDisabled();
  });

  it("does not offer knowledge revision before an answer exists", () => {
    render(<App />);

    expect(screen.queryByRole("button", { name: "指出问题" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "指出回答中的知识问题" })).not.toBeInTheDocument();
  });

  it("uses normal navigation without a prototype runtime notice", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^待处理/ }));
    expect(screen.queryByText("Prototype 数据环境")).not.toBeInTheDocument();
    expect(screen.queryByText("不会写入服务端")).not.toBeInTheDocument();
  });

  it("filters the semantic asset catalog and opens an explainable asset contract", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识库" }));
    expect(screen.getByRole("region", { name: "知识目录" })).toBeVisible();
    expect(screen.queryByRole("heading", { name: "知识目录" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "保存视图" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "语义资产详情" })).not.toBeInTheDocument();
    await user.type(screen.getByRole("searchbox", { name: "搜索知识目录" }), "净收入");

    const catalog = screen.getByRole("region", { name: "知识目录" });
    expect(within(catalog).getByText("净收入")).toBeVisible();
    expect(within(catalog).queryByText("活跃客户")).not.toBeInTheDocument();

    await user.click(within(catalog).getByRole("button", { name: "打开语义资产 净收入" }));
    expect(screen.getByRole("heading", { name: "净收入" })).toBeVisible();
    const detail = screen.getByRole("region", { name: "语义资产详情" });
    const backToCatalog = screen.getByRole("button", { name: "返回知识目录" });
    expect(backToCatalog).toBeVisible();
    expect(backToCatalog.closest(".topbar")).toBeTruthy();
    expect(detail.querySelector(".asset-detail-titlebar")).toBeNull();
    expect(detail.querySelector(".asset-detail-commandbar")).not.toBeInTheDocument();
    expect(within(detail).getByRole("tab", { name: "概览" })).toHaveAttribute("aria-selected", "true");
    expect(within(detail).getByRole("complementary", { name: "生产可用判断" })).toHaveTextContent("当前 revision 可用于生产");
    expect(within(detail).getByRole("complementary", { name: "生产可用判断" })).toHaveTextContent("没有阻断门禁");
    expect(within(detail).getByRole("complementary", { name: "生产可用判断" })).toHaveTextContent("质量阻断0");
    expect(within(detail).getByRole("complementary", { name: "生产可用判断" })).toHaveTextContent("类型门禁0");
    expect(within(detail).getByText("健康", { selector: ".asset-health-state" })).toBeVisible();
    expect(within(detail).getByRole("region", { name: "指标摘要" })).toHaveTextContent("paid_amount");
    expect(detail.querySelector(".asset-overview-paths")).not.toBeInTheDocument();
    expect(within(detail).getByText("@12", { selector: ".asset-header-state strong" })).toBeVisible();
    expect(detail).toHaveTextContent("release-2026.08.3");
    expect(detail).not.toHaveTextContent("/ 100");

    const revisionLauncher = within(detail).getByRole("button", { name: "修订知识" });
    expect(revisionLauncher).toBeVisible();
    await user.click(revisionLauncher);
    const revisionPicker = within(detail).getByRole("dialog", { name: "选择知识修订对象" });
    expect(within(revisionPicker).getByText("来源资料与已发布版本保持只读")).toBeVisible();
    await user.click(within(revisionPicker).getByRole("button", { name: /计算表达式/ }));
    expect(within(detail).getByRole("region", { name: "净收入 知识修订工作台" })).toBeVisible();
    expect(within(detail).getByRole("textbox", { name: "计算表达式候选值" })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "取消修订" }));
    expect(within(detail).getByRole("button", { name: "修订知识" })).toBeVisible();

    expect(within(detail).getAllByRole("tab")).toHaveLength(6);
    await user.click(within(detail).getByRole("tab", { name: "定义" }));
    expect(within(detail).getByRole("region", { name: "指标内容模板" })).toHaveTextContent("聚合与派生计算");
    expect(within(detail).getByRole("region", { name: "指标内容模板" })).toHaveTextContent("输入属性或指标");
    expect(within(detail).getByRole("heading", { name: "权威语义页" })).toBeVisible();
    const wiki = within(detail).getByRole("region", { name: "权威语义页" });
    expect(within(wiki).getByText("AI 检索词")).toBeVisible();
    expect(within(wiki).getByText("commerce.net_revenue", { exact: false })).toBeVisible();
    expect(within(wiki).getByText("跨语义域出现同名含义", { exact: false })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "提出修订" }));
    expect(within(detail).getByRole("region", { name: "净收入 知识修订工作台" })).toBeVisible();
    expect(within(detail).getByText("当前发布值 · @12")).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "取消修订" }));

    await user.click(within(detail).getByRole("tab", { name: "本体关系" }));
    expect(within(detail).getByRole("tab", { name: "本体关系" })).toHaveAttribute("aria-selected", "true");
    expect(within(detail).getByRole("heading", { name: "电商经营本体" })).toBeVisible();
    expect(within(detail).getByRole("button", { name: "语义关系" })).toHaveAttribute("aria-pressed", "true");
    expect(within(detail).getByRole("img", { name: "净收入 的语义关系图" })).toBeVisible();
    expect(within(detail).getByText("一致性通过")).toBeVisible();
    expect(within(detail).getByRole("table", { name: "本体关系类型约束" })).toHaveTextContent("端点类型、方向与基数");
    expect(within(detail).getByText("commerce.orders_model")).toBeVisible();
    expect(within(detail).getAllByText("证据推导", { exact: false }).length).toBeGreaterThan(0);
    await user.click(within(detail).getByRole("button", { name: "概念层级" }));
    expect(within(detail).getByRole("img", { name: "净收入 的本体层级图" })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "依赖与影响" }));
    expect(within(detail).getByRole("img", { name: "净收入 的依赖与影响图" })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "提出关系修订" }));
    expect(within(detail).getByRole("region", { name: "净收入 知识修订工作台" })).toBeVisible();
    expect(within(detail).getByRole("textbox", { name: "本体关系候选值" })).toBeVisible();
    await user.click(within(detail).getByRole("button", { name: "取消修订" }));

    await user.click(within(detail).getByRole("tab", { name: "实现" }));
    expect(within(detail).getByRole("heading", { name: "PhysicalBinding" })).toBeVisible();
    expect(within(detail).getByText("BIND-NET-REV-12")).toBeVisible();
    expect(within(detail).getByRole("heading", { name: "JoinContract" })).toBeVisible();

    await user.click(within(detail).getByRole("tab", { name: "可信度" }));
    expect(within(detail).getByRole("region", { name: "可信度摘要" })).toHaveTextContent("可信度健康");
    expect(within(detail).getByRole("table", { name: "指标验证规则" })).toHaveTextContent("definition");
    expect(within(detail).getByRole("heading", { name: "主张与证据" })).toBeVisible();
    expect(within(detail).getByRole("table", { name: "字段主张与证据" })).toHaveTextContent("spec.expression");
    expect(within(detail).getByText("EVD-2041", { exact: false })).toBeVisible();
    expect(within(detail).getByRole("table", { name: "最近验证运行" })).toHaveTextContent("VALRUN-");

    await user.click(within(detail).getByRole("tab", { name: "交付与影响" }));
    expect(within(detail).getByRole("heading", { name: "交付地址与消费状态" })).toBeVisible();
    expect(within(detail).getByRole("heading", { name: "当前生产指针" })).toBeVisible();
    expect(within(detail).getByRole("heading", { name: "消费者绑定" })).toBeVisible();
    expect(within(detail).getByRole("table", { name: "消费者绑定" })).toHaveTextContent("兼容");

    await user.click(backToCatalog);
    expect(screen.getByRole("region", { name: "知识目录" })).toBeVisible();
    expect(screen.getByRole("searchbox", { name: "搜索知识目录" })).toHaveValue("净收入");
    expect(screen.queryByRole("region", { name: "语义资产详情" })).not.toBeInTheDocument();
  });

  it("specializes the stable detail shell for a non-executable business concept", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识库" }));
    await user.type(screen.getByRole("searchbox", { name: "搜索知识目录" }), "有效支付订单");
    const catalog = screen.getByRole("region", { name: "知识目录" });
    await user.click(within(catalog).getByRole("button", { name: "打开语义资产 有效支付订单" }));

    const detail = screen.getByRole("region", { name: "语义资产详情" });
    expect(within(detail).getAllByRole("tab")).toHaveLength(6);
    expect(within(detail).getByRole("tab", { name: "实现" })).toBeVisible();

    await user.click(within(detail).getByRole("tab", { name: "定义" }));
    const template = within(detail).getByRole("region", { name: "业务口径内容模板" });
    expect(template).toHaveTextContent("业务边界与判定条件");
    expect(template).toHaveTextContent("口径能力");

    await user.click(within(detail).getByRole("tab", { name: "可信度" }));
    const rules = within(detail).getByRole("table", { name: "业务口径验证规则" });
    expect(rules).toHaveTextContent("definition");
    expect(rules).toHaveTextContent("语义定义");

    await user.click(within(detail).getByRole("tab", { name: "交付与影响" }));
    expect(within(detail).getByText("尚无使用方")).toBeVisible();
    expect(within(detail).queryByRole("button", { name: "注册使用方" })).not.toBeInTheDocument();
  });

  it("distinguishes an optional empty implementation from a missing contract", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识库" }));
    await user.type(screen.getByRole("searchbox", { name: "搜索知识目录" }), "支付订单数");
    const catalog = screen.getByRole("region", { name: "知识目录" });
    await user.click(within(catalog).getByRole("button", { name: "打开语义资产 支付订单数" }));
    const detail = screen.getByRole("region", { name: "语义资产详情" });

    await user.click(within(detail).getByRole("tab", { name: "实现" }));
    expect(within(detail).getByText("尚无连接契约")).toBeVisible();
    expect(within(detail).queryByRole("button", { name: "创建 JoinContract" })).not.toBeInTheDocument();

    await user.click(within(detail).getByRole("tab", { name: "交付与影响" }));
    expect(within(detail).getByText("尚无使用方")).toBeVisible();
  });

  it("defines templates for exactly five primary knowledge types", () => {
    expect(Object.keys(assetTypeProfiles)).toEqual(["业务对象", "业务口径", "指标", "数据资产", "分析模型"]);
    Object.values(assetTypeProfiles).forEach((profile) => {
      expect(profile.requiredFields).toHaveLength(5);
      expect(profile.definitionTitle.length).toBeGreaterThan(0);
      expect(Object.keys(profile.emptyStates)).toEqual(["relations", "bindings", "joins", "evidence", "consumers"]);
    });

    const concept = assets.find((asset) => asset.type === "业务口径")!;
    expect(assetTypeProfiles.业务口径.implementationMode).toBe("optional");
    expect(evaluateAssetTypeRules(concept)).toHaveLength(concept.readiness.length);

    const dimension = assets.find((asset) => asset.name === "业务区域")!;
    expect(evaluateAssetTypeRules(dimension)).toEqual(expect.arrayContaining([
      expect.objectContaining({ id: "ownership", state: "failed" }),
      expect.objectContaining({ id: "validation", state: "failed" }),
    ]));
  });

  it("restores the unified inbox on an old production-list deep link", () => {
    window.history.replaceState({}, "", "/governance?productionList=1");
    render(<App />);
    expect(screen.getByRole("region", { name: "统一待办" })).toBeVisible();
    expect(screen.queryByRole("region", { name: "语义资产版本列表" })).not.toBeInTheDocument();
  });

  it("keeps pending-work navigation when opening my tasks and switching back", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^待处理/ }));
    expect(screen.queryByRole("complementary", { name: "治理上下文" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^待处理/ })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: /^我发起/ })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "知识库" }));
    await user.click(screen.getByRole("button", { name: "发布记录" }));
    expect(screen.getByRole("region", { name: "语义资产版本列表" })).toBeVisible();
    expect(window.location.pathname + window.location.search).toBe("/assets?section=history");
    expect(screen.queryByRole("button", { name: /查看候选资产版本/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "评审批次" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^我发起/ }));
    expect(screen.getByRole("region", { name: "统一待办" })).toBeVisible();
    expect(window.location.search).toBe("?view=overview&scope=initiated");
  });

  it("restores inbox search, type and scroll after a task detail, and defaults primary entry to pending", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: "我发起" }));
    expect(screen.getByRole("button", { name: "我发起" })).toHaveFocus();
    await user.type(screen.getByRole("searchbox", { name: "搜索待办" }), "净收入");
    await user.selectOptions(screen.getByRole("combobox", { name: "筛选待办类型" }), "知识处理");
    const task = await screen.findByRole("button", { name: "进入审核：补充净收入财务口径证据" });
    const canvas = screen.getByRole("main");
    canvas.scrollTop = 240;
    await user.click(task);
    expect(window.location.search).toContain("scope=initiated");
    await screen.findByRole("region", { name: "工作台待办详情" });
    await user.click(screen.getByRole("button", { name: "返回待办" }));
    await screen.findByRole("button", { name: "进入审核：补充净收入财务口径证据" });
    expect(screen.getByRole("button", { name: "我发起" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("searchbox", { name: "搜索待办" })).toHaveValue("净收入");
    expect(screen.getByRole("combobox", { name: "筛选待办类型" })).toHaveValue("知识处理");
    expect(canvas.scrollTop).toBe(240);
    await user.click(screen.getByRole("button", { name: "已结束" }));
    expect(window.location.search).toBe("?view=overview&scope=done");
    expect(screen.getByRole("searchbox", { name: "搜索待办" })).toHaveValue("");
await user.click(screen.getByRole("button", { name: "我发起" }));
    expect(screen.getByRole("searchbox", { name: "搜索待办" })).toHaveValue("净收入");
    await user.click(screen.getByRole("button", { name: "批量审核" }));
    expect(window.location.search).toBe("?reviews=1&scope=initiated");
    await user.click(screen.getByRole("button", { name: "返回待办" }));
    expect(screen.getByRole("button", { name: "我发起" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("searchbox", { name: "搜索待办" })).toHaveValue("净收入");
    await user.click(screen.getByRole("button", { name: "知识库" }));
    await user.click(screen.getByRole("button", { name: "待办" }));
    expect(screen.getByRole("button", { name: "待处理" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("searchbox", { name: "搜索待办" })).toHaveValue("");
  });

  it("preserves pending-work navigation on a direct my-tasks link", () => {
    window.history.replaceState({}, "", "/?view=overview");
    render(<App />);
    const context = screen.getByRole("group", { name: "待办视图" });
    expect(within(context).getByRole("button", { name: /^待处理/ })).toHaveAttribute("aria-pressed", "true");
    expect(within(context).getByRole("button", { name: /^我发起/ })).toBeVisible();
    expect(within(context).getByRole("button", { name: /^已结束/ })).toBeVisible();
  });

  it("reviews included changes from the candidate version detail", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "待办" }));

    expect(within(screen.getByRole("group", { name: "待办视图" })).getByRole("button", { name: /^待处理/ })).toBeVisible();
    expect(screen.queryByRole("tab", { name: /变更事项|资产版本/ })).not.toBeInTheDocument();
    await openReviewTask(user);
    expect(screen.getByRole("tab", { name: /版本差异.*2/ })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("button", { name: "收起发布门禁" }));
    expect(screen.getByRole("button", { name: "展开发布门禁" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "展开发布门禁" }));
    await user.click(screen.getByRole("tab", { name: /变更来源.*1/ }));
    expect(screen.getByRole("heading", { name: "包含的变更事项" })).toBeVisible();
    expect(screen.getByText("PROP-128")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "审核候选版本 客单价 @9" }));

    const dialog = screen.getByRole("dialog", { name: "审核 客单价 · @9" });
    expect(within(dialog).getAllByText("数据结构检查", { exact: false }).length).toBeGreaterThan(0);
    expect(within(dialog).getByText("职责分离由服务端强制")).toBeVisible();
    const approve = within(dialog).getByRole("button", { name: "批准版本" });
    expect(approve).toBeDisabled();
    await user.type(within(dialog).getByRole("textbox", { name: "审核意见" }), "退款口径变化已由增长组确认。");
    await user.click(approve);
    expect(await screen.findByRole("status")).toHaveTextContent("评审已记录：批准");
    expect(screen.getByRole("button", { name: "发布 @9" })).toBeVisible();
  });

  it("shows immutable release bindings", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "待办" }));
    await openReleaseHistory(user);
    const versionRegistry = screen.getByRole("region", { name: "语义资产版本列表" });
    expect(versionRegistry.firstElementChild).toHaveClass("asset-version-toolbar");
    expect(within(versionRegistry).queryByRole("heading", { name: "资产版本" })).not.toBeInTheDocument();
    expect(within(versionRegistry).queryByText("3 个候选 · 6 条已发布")).not.toBeInTheDocument();
    const currentRelease = screen.getByRole("button", { name: "查看发布记录 @12 #6" });
    expect(currentRelease).toHaveTextContent("当前版本");
    await user.click(currentRelease);
    const release = screen.getByRole("region", { name: "发布 #6 详情" });
    expect(within(release).getByText("fixture.release_manifest")).toBeVisible();
    expect(within(release).getByText("服务端消费影响")).toBeVisible();
    expect(within(release).getByText("当前合同提供权威计数，不提供消费者明细。")).toBeVisible();
    expect(within(release).queryByText("Fluxale Production")).not.toBeInTheDocument();
  });

  it("keeps every secondary menu inside its primary module", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^批量审核/ }));
    expect(screen.queryByRole("complementary", { name: "治理上下文" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: /变更事项|资产版本/ })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "待办" })).toHaveAttribute("aria-current", "page");
    expect(screen.queryByRole("region", { name: "语义资产版本列表" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "资产版本" })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "消费与反馈" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "数据接入" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    expect(within(context).getByText("数据接入", { exact: true })).toBeVisible();
    expect(within(context).queryByRole("button", { name: "搜索资产、变更与功能" })).not.toBeInTheDocument();
    await user.click(within(context).getByRole("button", { name: /接入计划/ }));
    expect(screen.getByRole("button", { name: "数据接入" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("region", { name: "接入计划" })).toBeVisible();
    expect(within(context).getByRole("button", { name: /运行记录/ })).toBeVisible();
    expect(screen.queryByRole("tab", { name: /运行活动/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "运行记录" })).not.toBeInTheDocument();

    await user.click(within(context).getByRole("button", { name: /运行记录/ }));
    expect(screen.getByRole("region", { name: "运行记录" })).toBeVisible();
    expect(screen.queryByRole("region", { name: "接入计划" })).not.toBeInTheDocument();
  });

  it("opens searchable commands and jumps directly to an asset", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    await user.keyboard("{Control>}k{/Control}");
    const palette = screen.getByRole("dialog", { name: "搜索与命令" });
    await user.type(within(palette).getByRole("textbox", { name: "搜索资产、变更或功能" }), "净收入");
    await user.click(within(palette).getAllByRole("option")[0]);

    expect(screen.getByRole("heading", { name: "净收入" })).toBeVisible();
  });

  it("keeps asset content in the catalog rather than the secondary navigation", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识库" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    expect(within(context).getByRole("button", { name: "知识目录" })).toBeVisible();
    expect(within(context).queryByText("最近打开")).not.toBeInTheDocument();
    await user.click(within(context).getByRole("button", { name: "搜索知识目录" }));
    expect(screen.getByRole("searchbox", { name: "搜索知识目录" })).toHaveFocus();
    expect(context.querySelectorAll(".context-list > button")).toHaveLength(3);
    expect(within(context).queryByRole("button", { name: /打开最近资产/ })).not.toBeInTheDocument();
    expect(within(context).queryByRole("button", { name: /关系与映射/ })).not.toBeInTheDocument();
    expect(within(context).queryByRole("button", { name: /知识块与证据/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: /清单|语义关系|物理映射/ })).not.toBeInTheDocument();
    const catalog = screen.getByRole("region", { name: "知识目录" });
    expect(within(catalog).queryByRole("button", { name: /净收入依赖支付订单数，语义关系/ })).not.toBeInTheDocument();
    await user.click(within(catalog).getByRole("button", { name: "展开净收入的关系与实现" }));
    expect(within(catalog).getByRole("button", { name: /净收入依赖支付订单数，语义关系/ })).toBeVisible();
    expect(within(catalog).getByRole("button", { name: /analytics.orders 映射至 净收入，物理绑定/ })).toBeVisible();
    expect(within(catalog).getByRole("button", { name: /净收入 关联 共享维度 \/ 业务区域，JoinContract/ })).toBeVisible();

    await user.click(within(catalog).getByRole("button", { name: /analytics.orders 映射至 净收入，物理绑定/ }));
    expect(screen.getByRole("heading", { name: "净收入" })).toBeVisible();
    expect(screen.getByRole("tab", { name: "实现" })).toHaveAttribute("aria-selected", "true");
    expect(within(screen.getByRole("region", { name: "语义资产详情" })).getAllByText("BIND-NET-REV-12").some((element) => element.closest("article")?.getAttribute("aria-current") === "true")).toBe(true);

    expect(context.querySelectorAll(".context-list > button")).toHaveLength(3);
    expect(within(context).queryByText("净收入")).not.toBeInTheDocument();
    expect(within(context).getByRole("button", { name: "知识目录" })).toHaveAttribute("aria-current", "page");
    await user.click(screen.getByRole("button", { name: "返回知识目录" }));
    expect(screen.getByRole("region", { name: "知识目录" })).toBeVisible();
  });

  it("keeps the three knowledge workflow entries stable across detail and module navigation", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "知识库" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    const menuNames = () => Array.from(screen.getByRole("complementary", { name: "治理上下文" }).querySelectorAll(".context-list > button strong")).map((element) => element.textContent);
    const expected = ["知识目录", "草稿与整理", "发布记录"];

    expect(menuNames()).toEqual(expected);
    await user.click(within(screen.getByRole("region", { name: "知识目录" })).getByRole("button", { name: `打开语义资产 ${assets[2].name}` }));
    expect(screen.getByRole("heading", { name: assets[2].name })).toBeVisible();
    expect(menuNames()).toEqual(expected);
    expect(within(context).getByRole("button", { name: "知识目录" })).toHaveAttribute("aria-current", "page");

    await user.click(within(context).getByRole("button", { name: "草稿与整理" }));
    expect(menuNames()).toEqual(expected);
    expect(within(context).getByRole("button", { name: "草稿与整理" })).toHaveAttribute("aria-current", "page");
    await user.click(within(context).getByRole("button", { name: "发布记录" }));
    expect(menuNames()).toEqual(expected);
    expect(within(context).getByRole("button", { name: "发布记录" })).toHaveAttribute("aria-current", "page");

    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^待处理/ }));
    await user.click(screen.getByRole("button", { name: "知识库" }));
    expect(menuNames()).toEqual(expected);
  });

  it("opens system settings as a focused member directory", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.endsWith("/auth/methods")) return Response.json({ password: true, oidc: false });
      return Response.json({ items: url.includes("/members") ? [{ id: "mbr_test", accountId: "usr_test", principalId: "prn_test", displayName: "Verified member", status: "active", roleIds: ["workspace_admin"], admittedAt: "2026-09-04T09:00:00Z" }] : [], page: { limit: 50, total: 1 } });
    }));
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const directory = screen.getByRole("region", { name: "成员管理" });
    const list = within(directory).getByRole("region", { name: "工作区成员列表" });
    expect(await within(list).findByText("Verified member")).toBeVisible();
    expect(within(list).getByText("prn_test")).toBeVisible();
    expect(screen.queryByText("EMP-10001")).not.toBeInTheDocument();
    expect(screen.queryByText("24 名成员")).not.toBeInTheDocument();
    await user.type(within(directory).getByRole("searchbox", { name: "搜索成员与邀请" }), "missing");
    expect(within(list).queryByText("Verified member")).not.toBeInTheDocument();
  });

  it("keeps Access Control reachable for a role-manage-only session", async () => {
    const user = userEvent.setup();
    const manageOnlySession: CapabilitySession = {
      principalId: "EMP-ROLE-MANAGER",
      version: "authzv-manage-only",
      capabilities: ["role.manage"],
    };
    render(<App session={manageOnlySession} />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    const accessControlEntry = within(context).getByRole("button", { name: /访问控制/ });
    expect(accessControlEntry).toBeVisible();
    await user.click(accessControlEntry);

    const accessControl = screen.getByRole("region", { name: "访问控制" });
    expect(within(accessControl).getByText("角色管理上下文不可用")).toBeVisible();
    expect(within(accessControl).getByText(/缺少 role.read/)).toBeVisible();
  });

  it("keeps Operations reachable for a runtime-manage-only session without unauthorized reads", async () => {
    const user = userEvent.setup();
    render(<App session={{ principalId: "EMP-RUNTIME-MANAGER", version: "authzv-runtime-manage", capabilities: ["runtime.manage"] }} />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    const operationsEntry = within(context).getByRole("button", { name: /审计与运行/ });
    expect(operationsEntry).toBeVisible();
    await user.click(operationsEntry);

    const operations = screen.getByRole("region", { name: "审计与运行" });
    expect(within(operations).getByText("需要运行读取上下文")).toBeVisible();
    expect(within(operations).getByText(/runtime\.read/)).toBeVisible();
  });

  it("hides Operations when the session has no audit or runtime capability", async () => {
    const user = userEvent.setup();
    render(<App session={{ principalId: "EMP-MEMBER", version: "authzv-member", capabilities: ["workspace.read"] }} />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    expect(within(screen.getByRole("complementary", { name: "治理上下文" })).queryByRole("button", { name: /审计与运行/ })).not.toBeInTheDocument();
  });

  it("inspects roles and grouped permissions from access control", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /访问控制/ }));

    const accessControl = screen.getByRole("region", { name: "访问控制" });
    expect(within(accessControl).getByRole("tab", { name: "角色与权限" })).toHaveAttribute("aria-selected", "true");
    expect(within(accessControl).getByRole("tab", { name: "角色分配" })).toBeVisible();
    expect(within(accessControl).getByRole("tab", { name: "有效权限检查" })).toBeVisible();
    expect(within(accessControl).getByRole("button", { name: "查看角色 Workspace Admin" })).toBeVisible();
    expect(within(accessControl).getByRole("button", { name: "查看角色 Reviewer" })).toBeVisible();

    await user.click(within(accessControl).getByRole("button", { name: "查看角色 Reviewer" }));
    const roleDialog = screen.getByRole("dialog", { name: "Reviewer 角色详情" });
    expect(within(roleDialog).getByText("proposal.review")).toBeVisible();
    expect(within(roleDialog).getByText("审核语义提案并记录评审结论。", { exact: true })).toBeVisible();
    expect(within(roleDialog).queryByText("release.publish")).not.toBeInTheDocument();
    expect(within(roleDialog).queryByRole("button", { name: "编辑角色" })).not.toBeInTheDocument();
    await user.click(within(roleDialog).getByRole("button", { name: "基于此角色创建" }));

    const editor = screen.getByRole("dialog", { name: "基于 Reviewer 创建角色" });
    expect(within(editor).getByText("发布通过治理门禁的候选版本。", { exact: true })).toBeVisible();
    const roleName = within(editor).getByLabelText("角色名称");
    await user.clear(roleName);
    await user.type(roleName, "收入域高级评审");
    await user.click(within(editor).getByRole("checkbox", { name: "配置权限 asset.edit" }));
    await user.click(within(editor).getByRole("button", { name: "预览角色变更" }));
    expect(within(editor).getByRole("region", { name: "角色变更预览" })).toHaveTextContent("新增 1 项权限");
    await user.click(within(editor).getByRole("button", { name: "创建自定义角色" }));

    expect(within(accessControl).getByText("收入域高级评审")).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("收入域高级评审已创建");
    await user.click(within(accessControl).getByRole("button", { name: "查看角色 收入域高级评审" }));
    const customRoleDialog = screen.getByRole("dialog", { name: "收入域高级评审 角色详情" });
    await user.click(within(customRoleDialog).getByRole("button", { name: "编辑角色" }));
    const customRoleEditor = screen.getByRole("dialog", { name: "编辑角色 收入域高级评审" });
    expect(within(customRoleEditor).getByRole("checkbox", { name: "配置权限 asset.edit" })).toBeChecked();
    await user.click(within(customRoleEditor).getByRole("checkbox", { name: "配置权限 asset.edit" }));
    await user.click(within(customRoleEditor).getByRole("button", { name: "预览角色变更" }));
    expect(within(customRoleEditor).getByRole("region", { name: "角色变更预览" })).toHaveTextContent("移除 1 项权限");
    await user.click(within(customRoleEditor).getByRole("button", { name: "保存角色变更" }));
    expect(screen.getByRole("status")).toHaveTextContent("收入域高级评审已更新");
  });

  it("assigns scoped roles with a review step and session version update", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /访问控制/ }));
    const accessControl = screen.getByRole("region", { name: "访问控制" });
    await user.click(within(accessControl).getByRole("tab", { name: "角色分配" }));
    await user.click(within(accessControl).getByRole("button", { name: "分配角色" }));

    const dialog = screen.getByRole("dialog", { name: "分配角色" });
    await user.type(within(dialog).getByRole("textbox", { name: "主体 ID" }), "AGENT-GOVERNANCE");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "角色" }), "ROLE-AUDITOR");
    await user.click(within(dialog).getByRole("button", { name: "预览授权" }));
    expect(within(dialog).getByRole("region", { name: "授权变更预览" })).toHaveTextContent("申请 10 项操作权限");
    await user.click(within(dialog).getByRole("button", { name: "确认分配" }));

    expect(within(accessControl).getAllByText("AGENT-GOVERNANCE").length).toBeGreaterThan(0);
    expect(within(accessControl).getAllByText("Auditor").length).toBeGreaterThan(1);
    expect(within(accessControl).getByText("2", { exact: true })).toBeVisible();
    expect(screen.getByRole("status")).toHaveTextContent("角色分配已创建");
  });

  it("blocks conflicting roles in protected scopes", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /访问控制/ }));
    const accessControl = screen.getByRole("region", { name: "访问控制" });
    await user.click(within(accessControl).getByRole("tab", { name: "角色分配" }));
    await user.click(within(accessControl).getByRole("button", { name: "分配角色" }));
    const dialog = screen.getByRole("dialog", { name: "分配角色" });
    await user.type(within(dialog).getByRole("textbox", { name: "主体 ID" }), "USR-REVIEWER");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "角色" }), "ROLE-PUBLISHER");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "范围类型" }), "asset");
    await user.type(within(dialog).getByRole("textbox", { name: "范围 ID" }), "METRIC-NET-REVENUE");
    await user.click(within(dialog).getByRole("button", { name: "预览授权" }));

    await user.click(within(dialog).getByRole("button", { name: "确认分配" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("评审者与发布者必须相互独立");
  });

  it("explains effective access and keeps business titles separate from roles", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    expect(screen.queryByText("语义产品经理")).not.toBeInTheDocument();

    await user.click(within(context).getByRole("button", { name: /访问控制/ }));
    const accessControl = screen.getByRole("region", { name: "访问控制" });
    await user.click(within(accessControl).getByRole("tab", { name: "有效权限检查" }));
    await user.type(within(accessControl).getByRole("textbox", { name: "检查主体 ID" }), "USR-AUDITOR");
    await user.click(within(accessControl).getByRole("button", { name: "检查有效权限" }));
    let result = within(accessControl).getByRole("region", { name: "有效权限结果" });
    expect(result).toHaveTextContent("拒绝");
    expect(result).toHaveTextContent("NO_MATCHING_GRANT");
    expect(result).toHaveTextContent("authzv-2026.09.01-001");

    await user.clear(within(accessControl).getByRole("textbox", { name: "检查主体 ID" }));
    await user.type(within(accessControl).getByRole("textbox", { name: "检查主体 ID" }), "EMP-10001");
    await user.click(within(accessControl).getByRole("button", { name: "检查有效权限" }));
    result = within(accessControl).getByRole("region", { name: "有效权限结果" });
    expect(result).toHaveTextContent("允许");
    expect(result).toHaveTextContent("Workspace Admin");
    expect(result).toHaveTextContent("Semlia 工作区");
  });

  it("configures LLM and Embedding models as separate system settings", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /模型配置/ }));

    const configuration = screen.getByRole("region", { name: "模型配置" });
    expect(within(configuration).queryByRole("heading", { name: "模型配置" })).not.toBeInTheDocument();
    expect(configuration.querySelector(".model-config-stats")).not.toBeInTheDocument();
    expect(within(configuration).getByRole("tab", { name: "LLM 模型" })).toHaveAttribute("aria-selected", "true");
    expect(within(configuration).getByRole("combobox", { name: "默认 LLM 模型" })).toHaveValue("mdl_fixture_001");
    expect(within(configuration).getByText("gpt-4.1")).toBeVisible();
    expect(within(configuration).getByText("claude-sonnet-4-20250514")).toBeVisible();

    await user.click(within(configuration).getByRole("tab", { name: "Embedding 模型" }));
    expect(within(configuration).getByRole("tab", { name: "Embedding 模型" })).toHaveAttribute("aria-selected", "true");
    expect(within(configuration).getByText("text-embedding-3-large")).toBeVisible();
    expect(within(configuration).getByText("bge-m3")).toBeVisible();
    expect(within(configuration).getByText("3,072 维 · 8,191 tokens")).toBeVisible();
    expect(configuration.querySelector(".embedding-boundary")).not.toBeInTheDocument();
    expect(within(configuration).getByRole("button", { name: "重建向量索引" })).toBeVisible();

    await user.selectOptions(within(configuration).getByRole("combobox", { name: "默认 Embedding 模型" }), "mdl_fixture_005");
    expect(screen.getByRole("status")).toHaveTextContent("Embedding 默认模型已切换为 bge-m3");
    expect(within(configuration).getByRole("button", { name: "重建向量索引" })).toBeDisabled();
    expect(within(configuration).getByRole("region", { name: "持久化向量索引" })).not.toHaveTextContent("42%");
    expect(within(configuration).queryByText("RUN-IDX-260901-1132")).not.toBeInTheDocument();

    await user.click(within(configuration).getByRole("button", { name: "添加供应商" }));
    const dialog = screen.getByRole("dialog", { name: "添加模型供应商" });
    await user.type(within(dialog).getByLabelText("配置名称"), "企业向量网关");
    await user.selectOptions(within(dialog).getByLabelText("供应商"), "OpenAI 兼容");
    await user.type(within(dialog).getByLabelText("Base URL"), "https://models.example.com/v1");
    await user.type(within(dialog).getByLabelText("凭据环境变量"), "SEMLIA_COMPAT_GATEWAY_KEY");
    await user.type(within(dialog).getByLabelText("API Key"), "sk-prototype");
    await user.click(within(dialog).getByRole("button", { name: "添加供应商" }));
    expect(within(configuration).getByText("企业向量网关")).toBeVisible();
    expect(within(configuration).getAllByText(/尚未配置模型/).length).toBeGreaterThan(0);

  });

  it("uses audit and runtime as a global operations center", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /审计与运行/ }));
    const runtime = screen.getByRole("region", { name: "审计与运行" });
    expect(within(runtime).queryByRole("heading", { name: "审计与运行" })).not.toBeInTheDocument();
    expect(within(runtime).getByRole("region", { name: "运行状态概览" })).toHaveTextContent("需要关注");
    expect(within(runtime).queryByText("日志投递")).not.toBeInTheDocument();
    const runTable = within(runtime).getByRole("region", { name: "全局运行记录" });
    expect(within(runTable).getByText("PostgreSQL Analytics")).toBeVisible();
    await user.click(within(runTable).getByRole("button", { name: "查看运行 PostgreSQL Analytics" }));
    expect(screen.getByRole("dialog", { name: "PostgreSQL Analytics" })).toHaveTextContent("生成候选版本");
    await user.click(within(screen.getByRole("dialog", { name: "PostgreSQL Analytics" })).getByRole("button", { name: "关闭" }));

    await user.click(within(runtime).getByRole("tab", { name: "审计日志" }));
    const auditTable = within(runtime).getByRole("region", { name: "审计事件" });
    expect(within(auditTable).getByText("client-codex-mcp")).toBeVisible();
    await user.type(within(runtime).getByRole("textbox", { name: "操作者 ID" }), "client-codex-mcp");
    await user.click(within(runtime).getByRole("button", { name: "应用筛选" }));
    expect(within(auditTable).getByText("semantic.resolve.completed")).toBeVisible();
    expect(within(auditTable).queryByText("runtime.discovery.started")).not.toBeInTheDocument();
    await user.click(within(auditTable).getByRole("button", { name: "查看审计事件 semantic.resolve.completed" }));
    expect(screen.getByRole("dialog", { name: "semantic.resolve.completed" })).toHaveTextContent("tr_d219a4");
    await user.click(within(screen.getByRole("dialog", { name: "semantic.resolve.completed" })).getByRole("button", { name: "关闭" }));

    await user.click(within(runtime).getByRole("tab", { name: "运行设置" }));
    await user.clear(within(runtime).getByRole("spinbutton", { name: "失败重试上限" }));
    await user.type(within(runtime).getByRole("spinbutton", { name: "失败重试上限" }), "4");
    await user.click(within(runtime).getByRole("button", { name: "保存更改" }));
    expect(await screen.findByText(/运行设置版本 8 已由服务端确认/)).toBeVisible();
  });

  it("keeps interfaces, bindings and runtime feedback in their owning modules", async () => {
    const user = userEvent.setup();
    render(<App />);

    expect(screen.queryByRole("button", { name: "语义交付" })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /接口与集成/ }));
    const integrations = screen.getByRole("region", { name: "接口与集成" });
    expect(within(integrations).getAllByText("REST API", { exact: true }).length).toBeGreaterThan(0);
    expect(within(integrations).getByText("MCP Server", { exact: true })).toBeVisible();
    expect(within(integrations).getByText("CLI", { exact: true })).toBeVisible();
    expect(within(integrations).getByText("TypeScript SDK", { exact: true })).toBeVisible();
    expect(within(integrations).queryByText("Fluxale Production")).not.toBeInTheDocument();
    expect(within(integrations).queryByText("演示环境未连接集成服务，凭据与投递操作不可用。")).not.toBeInTheDocument();
    expect(within(integrations).getByRole("button", { name: "创建客户端" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "待办" }));
    await openReleaseHistory(user);
    await user.click(screen.getByRole("button", { name: "查看发布记录 @12 #6" }));
    const release = screen.getByRole("region", { name: /发布 #6 详情/ });
    expect(release).toBeVisible();
    expect(within(release).getByText("服务端消费影响")).toBeVisible();
    expect(within(release).queryByText("Fluxale Production")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^待处理/ }));
    expect(screen.getByRole("region", { name: "统一待办" })).toBeVisible();
    expect(screen.getByRole("heading", { name: "待办" })).toBeVisible();
    expect(within(context).queryByRole("button", { name: /动态/ })).not.toBeInTheDocument();
  });

  it("never simulates credential mutation in the fixture workspace", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /接口与集成/ }));
    const integrations = screen.getByRole("region", { name: "接口与集成" });
    expect(within(integrations).queryByText("已过期", { exact: true })).not.toBeInTheDocument();
    expect(within(integrations).getByRole("button", { name: "创建客户端" })).toBeDisabled();
    expect(within(integrations).queryByRole("button", { name: /撤销客户端/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.queryByText("Prototype 数据环境")).not.toBeInTheDocument();
  });

  it("does not label connected Integration Settings as a prototype", async () => {
    const user = userEvent.setup();
    render(<CatalogRuntimeProvider fixtureAssets={assets}><ProductApp session={authorSession} /></CatalogRuntimeProvider>);
    await user.click(screen.getByRole("button", { name: "系统设置" }));
    await user.click(within(screen.getByRole("complementary", { name: "治理上下文" })).getByRole("button", { name: /接口与集成/ }));
    expect(screen.getByRole("region", { name: "接口与集成" })).toBeVisible();
    expect(screen.queryByText("Prototype 管理页面")).not.toBeInTheDocument();
    expect(screen.queryByText("接口客户端尚未接入 Alpha 后端；操作不会形成真实凭据。")).not.toBeInTheDocument();
    expect(screen.queryByText("Prototype 数据环境")).not.toBeInTheDocument();
  });

  it("gates review decisions behind a mandatory reason with server-enforced separation of duty", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "待办" }));

    await openReviewTask(user);
    await user.click(screen.getByRole("button", { name: "审核候选版本 客单价 @9" }));
    const reviewDialog = screen.getByRole("dialog", { name: "审核 客单价 · @9" });
    expect(within(reviewDialog).getByText("职责分离由服务端强制")).toBeVisible();
    expect(within(reviewDialog).getByRole("button", { name: "批准版本" })).toBeDisabled();
    expect(within(reviewDialog).getByRole("button", { name: "退回版本" })).toBeDisabled();
    await user.type(within(reviewDialog).getByRole("textbox", { name: "审核意见" }), "退款口径变化需要财务确认。");
    expect(within(reviewDialog).getByRole("button", { name: "批准版本" })).toBeEnabled();

    await user.click(within(reviewDialog).getByRole("button", { name: "批准版本" }));
    expect(await screen.findByRole("status")).toHaveTextContent("评审已记录：批准");
    await user.click(screen.getByRole("button", { name: "发布 @9" }));
    const publishDialog = screen.getByRole("dialog", { name: "发布客单价 @9" });
    expect(within(publishDialog).getByText("独立发布者由服务端强制")).toBeVisible();
    expect(within(publishDialog).getByRole("button", { name: "确认发布并生效" })).toBeEnabled();
  });

  it("hides governance actions from principals without the matching capabilities", async () => {
    const user = userEvent.setup();
    const limitedSession: CapabilitySession = {
      principalId: "EMP-LIMITED",
      version: "authzv-2026.09.01-001",
      capabilities: ["workspace.read", "asset.read", "asset.propose", "workspace.manage"],
    };
    render(<App session={limitedSession} />);
    await user.click(screen.getByRole("button", { name: "待办" }));

    expect(screen.queryByRole("button", { name: /汇编批次/ })).not.toBeInTheDocument();
    await openReviewTask(user);
    expect(screen.queryByRole("button", { name: "审核候选版本 客单价 @9" })).not.toBeInTheDocument();
  });

  it("opens tasks from the unified queue and retains stable scope navigation", async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(await screen.findByRole("button", { name: "检查来源：检查客户增长域运行异常" }));
    const detail = await screen.findByRole("region", { name: "工作台待办详情" });
    expect(detail).toHaveTextContent("DISCOVERY_FAILED");
    expect(screen.queryByRole("complementary", { name: "治理上下文" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "返回待办" }));
    expect(screen.getByRole("region", { name: "统一待办" })).toBeVisible();
    await user.selectOptions(screen.getByRole("combobox", { name: "筛选待办类型" }), "问数问题");
    expect(await screen.findByRole("button", { name: "检查使用约束：旧区域别名无法安全废弃" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "检查来源：检查客户增长域运行异常" })).not.toBeInTheDocument();
    await user.selectOptions(screen.getByRole("combobox", { name: "筛选待办类型" }), "");
    await user.click(screen.getByRole("button", { name: /^我发起/ }));
    expect(await screen.findByRole("button", { name: "进入审核：补充净收入财务口径证据" })).toBeVisible();
  });

  it("scopes version comparison to one semantic asset", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "待办" }));
    await openReleaseHistory(user);
    expect(screen.queryByRole("button", { name: "比较版本" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "查看发布记录 @12 #6" }));
    await user.click(screen.getByRole("button", { name: "查看差异基准" }));

    const dialog = screen.getByRole("dialog", { name: "发布差异基准" });
    expect(dialog).toBeVisible();
    expect(within(dialog).getByText("与前一发布固定版本比较")).toBeVisible();
    expect(within(dialog).getByText("与当前注册表比较")).toBeVisible();
    expect(within(dialog).getAllByText("所选发布固定版本").length).toBeGreaterThan(0);
    expect(within(dialog).getByText("fixture.release_manifest")).toBeVisible();
  });

  it("provides distinct, functional pages for data ingestion", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Response.json({ code: "UNAVAILABLE", message: "source API unavailable" }, { status: 503 })));
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "数据接入" }));
    expect(screen.getByRole("region", { name: "数据来源" })).toBeVisible();
    expect(await screen.findByText("source API unavailable")).toBeVisible();
    expect(screen.queryByText("PostgreSQL Analytics")).not.toBeInTheDocument();
    const context = screen.getByRole("complementary", { name: "治理上下文" });
    await user.click(within(context).getByRole("button", { name: /接入计划/ }));
    expect(screen.getByRole("region", { name: "接入计划" })).toBeVisible();
    await user.click(within(context).getByRole("button", { name: /运行记录/ }));
    expect(screen.getByRole("region", { name: "运行记录" })).toBeVisible();
    expect(screen.queryByText("RUN-SESSION")).not.toBeInTheDocument();
  }, 15_000);

  it("never substitutes prototype schedules when the schedule API fails", async () => {
    const requests: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      requests.push(url);
      const path = new URL(url).pathname;
      if (path.endsWith("/schedules")) return Response.json({ code: "UNAVAILABLE", message: "schedule API unavailable" }, { status: 503 });
      return Response.json({ items: path.endsWith("/sources") ? [{ id: "src_test", name: "Persisted source", sourceKind: "postgresql", status: "active", version: 1, host: "db", port: 5432, database: "analytics", username: "reader", sslMode: "require", credentialVersion: 1, createdAt: "2026-09-04T09:00:00Z", updatedAt: "2026-09-04T09:00:00Z" }] : [], page: { limit: 50, total: 1 } });
    }));
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole("button", { name: "数据接入" }));
    expect(await screen.findByRole("button", { name: "查看来源 Persisted source" })).toBeVisible();
    await user.click(within(screen.getByRole("complementary", { name: "治理上下文" })).getByRole("button", { name: /接入计划/ }));
    expect(screen.getByRole("region", { name: "接入计划" })).toBeVisible();
    expect((await screen.findAllByText(/schedule API unavailable/)).length).toBeGreaterThan(0);
    expect(requests.some(url => url.includes("/sources/src_test/schedules"))).toBe(true);
    expect(screen.queryByText("经营数据元数据同步")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "新建构建任务" })).not.toBeInTheDocument();
  }, 15_000);

  it("completes review, publish and compatibility feedback using API facts", async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.click(screen.getByRole("button", { name: "待办" }));

    expect(screen.queryByRole("complementary", { name: "治理上下文" })).not.toBeInTheDocument();
    await openReviewTask(user);
    const backToVersions = screen.getByRole("button", { name: "返回待办" });
    expect(backToVersions.closest(".topbar")).not.toBeNull();
    expect(document.querySelector(".governance-detail-commandbar")).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "客单价 @9 候选资产版本详情" })).toHaveTextContent("版本范围");
    await user.click(screen.getByRole("button", { name: "审核候选版本 客单价 @9" }));
    const lifecycleDialog = screen.getByRole("dialog", { name: "审核 客单价 · @9" });
    await user.type(within(lifecycleDialog).getByRole("textbox", { name: "审核意见" }), "口径变化已确认，可以发布。");
    await user.click(within(lifecycleDialog).getByRole("button", { name: "批准版本" }));
    await user.click(await screen.findByRole("button", { name: "发布 @9" }));
    await user.click(within(screen.getByRole("dialog", { name: "发布客单价 @9" })).getByRole("button", { name: "确认发布并生效" }));
    expect(await screen.findByRole("status")).toHaveTextContent("发布完成：批次 rls_fixture_0007 · 序列 #7");

    await user.click(backToVersions);
    await openReleaseHistory(user);
    await user.click(screen.getByRole("button", { name: "查看发布记录 @12 #6" }));
    const release = screen.getByRole("region", { name: /发布 #6 详情/ });
    expect(release).toBeVisible();
    expect(within(release).getByText("服务端消费影响")).toBeVisible();
    expect(within(release).queryByText("Fluxale Production")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "待办" }));
    await user.click(screen.getByRole("button", { name: /^待处理/ }));
    await user.click(await screen.findByRole("button", { name: "检查使用约束：旧区域别名无法安全废弃" }));
    const attentionDetail = await screen.findByRole("region", { name: "工作台待办详情" });
    expect(within(attentionDetail).getByText("SEMANTIC_QUERY_REFUSED")).toBeVisible();
    expect(screen.getByRole("button", { name: "待办" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "数据接入" })).not.toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "返回待办" }).closest(".topbar")).not.toBeNull();
    await user.click(within(attentionDetail).getByRole("button", { name: "处理兼容性" }));
    const compatibility = await screen.findByRole("region", { name: "兼容性影响详情" });
    expect(compatibility).toHaveTextContent("服务端拒绝与绑定事实");
    expect(compatibility).not.toHaveTextContent("Fixture target");
    expect(compatibility).toHaveTextContent("csm_01arz3ndektsv4rrffq69g5fav");
    expect(compatibility).toHaveTextContent("cbd_01arz3ndektsv4rrffq69g5fav");
    expect(compatibility).toHaveTextContent("smq_01arz3ndektsv4rrffq69g5fav");
    expect(window.location.pathname).toBe("/delivery/compatibility");
    expect(screen.queryByRole("region", { name: "工作台待办详情" })).not.toBeInTheDocument();
  }, 15_000);
});

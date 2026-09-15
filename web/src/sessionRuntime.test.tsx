import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AUTHORIZATION_STALE_EVENT, clearSessionCSRFToken, SESSION_INVALID_EVENT } from "./apiClient";
import { CapabilityProvider, useCan } from "./authorization";
import { MemberAdministrationView } from "./MemberAdministrationView";
import { SessionAccountControl, SessionAuthorizationNotice, SessionEntryState, SessionRuntimeProvider, useSessionRuntime } from "./sessionRuntime";
import { permissionGroups } from "./permissionCatalog";
import type { SessionWorkspace } from "./identity";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const accountId = "usr_01arz3ndektsv4rrffq69g5fav";
const principalId = "prn_01arz3ndektsv4rrffq69g5fav";

afterEach(() => {
  vi.unstubAllGlobals();
  clearSessionCSRFToken();
  window.sessionStorage.clear();
  window.history.replaceState({}, "", "/");
});

describe("controlled-user session runtime", () => {
  it("renders the real sign-in state after an unauthenticated response", async () => {
    const requests: Request[] = [];
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      requests.push(request);
      return json({ code: "UNAUTHORIZED", message: "session required", traceId: "trace-1", details: {} }, 401);
    }));

    render(<SessionRuntimeProvider><SessionEntryState /></SessionRuntimeProvider>);

    expect(await screen.findByRole("heading", { name: "登录工作区" }, { timeout: 5_000 })).toBeVisible();
    expect(screen.getByLabelText("登录账号")).toBeVisible();
    expect(screen.getByRole("button", { name: "登录" })).toBeVisible();
    expect(screen.queryByLabelText("Semlia 主功能")).not.toBeInTheDocument();
    expect(requests[0].credentials).toBe("include");
  });

  it("keeps callback failures explicit instead of falling back to fixtures", async () => {
    window.history.replaceState({}, "", "/?auth_error=access_denied&auth_error_description=%E7%BB%84%E7%BB%87%E7%AD%96%E7%95%A5%E6%8B%92%E7%BB%9D%E4%BA%86%E7%99%BB%E5%BD%95");
    vi.stubGlobal("fetch", vi.fn(async () => json({ code: "UNAUTHORIZED", message: "session required", traceId: "trace-2", details: {} }, 401)));

    render(<SessionRuntimeProvider><SessionEntryState /></SessionRuntimeProvider>);

    expect(await screen.findByRole("heading", { name: "无法进入 Semlia" })).toBeVisible();
    expect(screen.getByText("组织策略拒绝了登录")).toBeVisible();
    expect(screen.getByRole("link", { name: "重新登录" })).toBeVisible();
  });

  it("shows an admitted account with no active workspace and logs out through CSRF", async () => {
    const requests: Request[] = [];
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      requests.push(request);
      if (request.method === "DELETE") return new Response(null, { status: 204 });
      return sessionResponse([]);
    }));
    const user = userEvent.setup();

    render(<SessionRuntimeProvider><SessionEntryState /></SessionRuntimeProvider>);

    expect(await screen.findByRole("heading", { name: "尚未加入工作区" })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "退出登录" }));
    expect(await screen.findByRole("heading", { name: "登录工作区" })).toBeVisible();
    const logoutRequest = requests.find((request) => request.method === "DELETE");
    expect(logoutRequest?.headers.get("X-Semlia-CSRF")).toBe("csrf-verifier-value-for-tests-1234567890");
  });

  it("uses server capabilities and keeps workspace preference per account", async () => {
    const secondWorkspace: SessionWorkspace = { id: "wsp_01arz3ndektsv4rrffq69g5faw", slug: "finance", displayName: "财务语义", principalId: "prn_01arz3ndektsv4rrffq69g5faw", roleIds: ["auditor"], capabilities: ["audit.read"], authorizationVersion: 4 };
    window.sessionStorage.setItem(`semlia.workspace.${accountId}`, secondWorkspace.id);
    vi.stubGlobal("fetch", vi.fn(async () => sessionResponse([defaultWorkspace(), secondWorkspace])));

    render(<SessionRuntimeProvider><CapabilityHarness /></SessionRuntimeProvider>);

    expect(await screen.findByText("财务语义")).toBeVisible();
    expect(screen.getByText("可审计")).toBeVisible();
    expect(screen.getByText("不可管理成员")).toBeVisible();
  });

  it("does not infer capabilities from an administrator role when the server omits them", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => sessionResponse([{ ...defaultWorkspace(), capabilities: undefined }])));
    render(<SessionRuntimeProvider><CapabilityHarness /></SessionRuntimeProvider>);
    expect(await screen.findByText("Alpha Workspace")).toBeVisible();
    expect(screen.getByText("不可审计")).toBeVisible();
    expect(screen.getByText("不可管理成员")).toBeVisible();
  });

  it("returns an expired session to sign-in and refreshes a stale authorization without clearing view state", async () => {
    const fetchMock = vi.fn(async () => sessionResponse([defaultWorkspace()]));
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<SessionRuntimeProvider><StatefulHarness /></SessionRuntimeProvider>);

    const input = await screen.findByRole("textbox", { name: "未提交内容" });
    await user.type(input, "保留我");
    act(() => window.dispatchEvent(new CustomEvent(AUTHORIZATION_STALE_EVENT)));
    expect(await screen.findByRole("alert")).toHaveTextContent("操作未获授权");
    expect(input).toHaveValue("保留我");
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));

    act(() => window.dispatchEvent(new CustomEvent(SESSION_INVALID_EVENT)));
    expect(await screen.findByRole("heading", { name: "会话已过期" })).toBeVisible();
    expect(screen.queryByRole("textbox", { name: "未提交内容" })).not.toBeInTheDocument();
  });
});

describe("real member administration", () => {
  it("lists members and creates an invitation with the in-memory CSRF verifier", async () => {
    const requests: Request[] = [];
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      requests.push(request);
      if (request.url.endsWith("/auth/methods")) return json({ password: true, oidc: true });
      if (request.url.endsWith("/api/v1/session")) return sessionResponse([defaultWorkspace()]);
      if (request.url.endsWith("/members")) return json({ items: [adminMember()] });
      if (request.url.endsWith("/invitations") && request.method === "GET") return json({ items: [] });
      if (request.url.endsWith("/invitations") && request.method === "POST") return json({ id: "ivn_01arz3ndektsv4rrffq69g5fav", issuer: "https://issuer.example", email: "new.user@example.com", roleId: "reviewer", status: "pending", expiresAt: "2026-09-11T10:00:00Z", createdAt: "2026-09-04T10:00:00Z" }, 201);
      return json({ code: "NOT_FOUND", message: "not found", traceId: "trace-x", details: {} }, 404);
    }));
    const user = userEvent.setup();
    render(<SessionRuntimeProvider><MemberHarness /></SessionRuntimeProvider>);

    expect(await screen.findByText("Alpha Admin")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "邀请成员" }));
    const dialog = screen.getByRole("dialog", { name: "邀请工作区成员" });
    await user.type(within(dialog).getByLabelText("组织邮箱"), "new.user@example.com");
    await user.selectOptions(within(dialog).getByLabelText("初始角色"), "reviewer");
    await user.click(within(dialog).getByRole("button", { name: "创建邀请" }));

    expect(await screen.findByText("new.user@example.com")).toBeVisible();
    const invitationRequest = requests.find((request) => request.method === "POST");
    expect(invitationRequest?.headers.get("X-Semlia-CSRF")).toBe("csrf-verifier-value-for-tests-1234567890");
    expect(await invitationRequest?.json()).toEqual({ email: "new.user@example.com", roleId: "reviewer" });
  });

  it("keeps the final administrator unchanged when the server rejects suspension", async () => {
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      if (request.url.endsWith("/api/v1/session")) return sessionResponse([defaultWorkspace()]);
      if (request.url.endsWith("/members") && request.method === "GET") return json({ items: [adminMember()] });
      if (request.url.endsWith("/invitations")) return json({ items: [] });
      if (request.method === "PATCH") return json({ code: "FINAL_ADMIN_REQUIRED", message: "工作区必须保留至少一名有效管理员。", traceId: "trace-final", details: {} }, 409);
      return json({}, 404);
    }));
    const user = userEvent.setup();
    render(<SessionRuntimeProvider><MemberHarness /></SessionRuntimeProvider>);

    await user.click(await screen.findByRole("button", { name: "停用 Alpha Admin" }));
    await user.click(screen.getByRole("button", { name: "确认停用" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("工作区必须保留至少一名有效管理员");
    expect(screen.getByText("有效")).toBeVisible();
  });
});

function CapabilityHarness() {
  const runtime = useSessionRuntime();
  if (!runtime.capabilitySession) return <SessionEntryState />;
  return <CapabilityProvider session={runtime.capabilitySession}><div><span>{runtime.activeWorkspace?.displayName}</span><PermissionProbe /></div></CapabilityProvider>;
}

function PermissionProbe() {
  const audit = useCan("audit.read");
  const memberManage = useCan("member.manage");
  return <><span>{audit ? "可审计" : "不可审计"}</span><span>{memberManage ? "可管理成员" : "不可管理成员"}</span></>;
}

function StatefulHarness() {
  const runtime = useSessionRuntime();
  if (runtime.phase !== "authenticated") return <SessionEntryState />;
  return <><SessionAuthorizationNotice /><input aria-label="未提交内容" /><SessionAccountControl /></>;
}

function MemberHarness() {
  const runtime = useSessionRuntime();
  if (!runtime.capabilitySession) return <SessionEntryState />;
  return <CapabilityProvider session={runtime.capabilitySession}><MemberAdministrationView /></CapabilityProvider>;
}

function defaultWorkspace() {
  return { id: workspaceId, slug: "alpha", displayName: "Alpha Workspace", principalId, roleIds: ["workspace_admin"], capabilities: permissionGroups.flatMap(group => group.actions), authorizationVersion: 3 };
}

function adminMember() {
  return { id: "mbr_01arz3ndektsv4rrffq69g5fav", accountId, displayName: "Alpha Admin", principalId, status: "active", roleIds: ["workspace_admin"], admittedAt: "2026-09-04T09:00:00Z" };
}

function sessionResponse(workspaces: Array<Omit<SessionWorkspace, "capabilities"> & { capabilities?: SessionWorkspace["capabilities"] }>) {
  return json({ account: { id: accountId, displayName: "Alpha Admin" }, workspaces, expiresAt: "2026-09-04T18:00:00Z", traceId: "trace-session" }, 200, { "X-Semlia-CSRF": "csrf-verifier-value-for-tests-1234567890" });
}

function json(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

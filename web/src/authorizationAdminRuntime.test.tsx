import { useState } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import { AuthorizationAdminRuntimeProvider, type AuthorizationAdminApi, useAuthorizationAdminRuntime } from "./authorizationAdminRuntime";
import type { AuthorizationRole } from "./types";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const role: AuthorizationRole = { id: "workspace_admin", name: "Workspace Admin", description: "管理工作区。", category: "system", permissions: ["role.read"], version: 1 };
const fullAccess = { roleRead: true, roleManage: true, roleAssign: true, authorizationInspect: true };

describe("authorization administration runtime", () => {
  it("does not request unauthorized resources", async () => {
    const api = makeApi();

    render(<AuthorizationAdminRuntimeProvider workspaceId={workspaceId} api={api} access={{ roleRead: false, roleManage: false, roleAssign: false, authorizationInspect: true }}><RuntimeProbe /></AuthorizationAdminRuntimeProvider>);

    await waitFor(() => expect(screen.getByTestId("runtime-state")).toHaveTextContent("ready"));
    expect(api.listRoles).not.toHaveBeenCalled();
    expect(api.listBindings).not.toHaveBeenCalled();
  });

  it("rejects a capability-mismatched command before calling the API", async () => {
    const api = makeApi();
    const user = userEvent.setup();

    render(<AuthorizationAdminRuntimeProvider workspaceId={workspaceId} api={api} access={{ roleRead: true, roleManage: false, roleAssign: false, authorizationInspect: false }}><UnauthorizedMutationProbe /></AuthorizationAdminRuntimeProvider>);

    await screen.findByText("Workspace Admin");
    await user.click(screen.getByRole("button", { name: "尝试创建角色" }));
    expect(await screen.findByTestId("mutation-result")).toHaveTextContent("FORBIDDEN");
    expect(api.createRole).not.toHaveBeenCalled();
  });

  it("refetches authoritative data before completing a mutation", async () => {
    const custom = { ...role, id: "custom_reader", name: "目录读取者", category: "custom" as const };
    const listRoles = vi.fn().mockResolvedValueOnce([role]).mockResolvedValueOnce([role, custom]);
    const api = makeApi({ listRoles, createRole: vi.fn().mockResolvedValue({ value: custom, authorizationVersion: 8 }) });
    const user = userEvent.setup();

    render(<AuthorizationAdminRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><RuntimeProbe /></AuthorizationAdminRuntimeProvider>);

    expect(await screen.findByText("Workspace Admin")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "创建测试角色" }));
    expect(await screen.findByText("目录读取者")).toBeVisible();
    expect(screen.getByTestId("authorization-version")).toHaveTextContent("8");
    expect(listRoles).toHaveBeenCalledTimes(2);
  });

  it("reports forbidden loading without exposing fixture roles", async () => {
    const forbidden = Object.assign(new Error("没有角色读取权限"), { status: 403, code: "FORBIDDEN" });
    const api = makeApi({ listRoles: vi.fn().mockRejectedValue(forbidden) });

    render(<AuthorizationAdminRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><RuntimeProbe /></AuthorizationAdminRuntimeProvider>);

    await waitFor(() => expect(screen.getByTestId("runtime-state")).toHaveTextContent("forbidden"));
    expect(screen.getByTestId("runtime-error")).toHaveTextContent("没有角色读取权限");
    expect(screen.queryByText("Workspace Admin")).not.toBeInTheDocument();
  });

  it("does not complete a mutation when the authoritative refetch fails", async () => {
    const listRoles = vi.fn().mockResolvedValueOnce([role]).mockRejectedValueOnce(new Error("refetch failed"));
    const api = makeApi({ listRoles, createRole: vi.fn().mockResolvedValue({ value: { ...role, id: "custom_reader", category: "custom" }, authorizationVersion: 8 }) });
    const user = userEvent.setup();

    render(<AuthorizationAdminRuntimeProvider workspaceId={workspaceId} api={api} access={fullAccess}><MutationFailureProbe /></AuthorizationAdminRuntimeProvider>);

    await screen.findByText("Workspace Admin");
    await user.click(screen.getByRole("button", { name: "创建并等待重载" }));
    expect(await screen.findByTestId("mutation-result")).toHaveTextContent("failed");
    expect(screen.getByTestId("runtime-state")).toHaveTextContent("error");
    expect(screen.queryByText("custom_reader")).not.toBeInTheDocument();
  });
});

function RuntimeProbe() {
  const runtime = useAuthorizationAdminRuntime();
  return <div>
    <output data-testid="runtime-state">{runtime.state}</output>
    <output data-testid="runtime-error">{runtime.error}</output>
    <output data-testid="authorization-version">{runtime.authorizationVersion}</output>
    {runtime.roles.map((item) => <span key={item.id}>{item.name}</span>)}
    <button type="button" onClick={() => void runtime.createRole({ name: "目录读取者", description: "读取目录。", permissions: ["asset.read"] })}>创建测试角色</button>
  </div>;
}

function MutationFailureProbe() {
  const runtime = useAuthorizationAdminRuntime();
  const [result, setResult] = useState("idle");
  const create = async () => {
    try {
      await runtime.createRole({ name: "目录读取者", description: "读取目录。", permissions: ["asset.read"] });
      setResult("completed");
    } catch {
      setResult("failed");
    }
  };
  return <div>
    <output data-testid="runtime-state">{runtime.state}</output>
    <output data-testid="mutation-result">{result}</output>
    {runtime.roles.map((item) => <span key={item.id}>{item.name}</span>)}
    <button type="button" onClick={() => void create()}>创建并等待重载</button>
  </div>;
}

function UnauthorizedMutationProbe() {
  const runtime = useAuthorizationAdminRuntime();
  const [result, setResult] = useState("idle");
  const create = async () => {
    try {
      await runtime.createRole({ name: "目录读取者", description: "读取目录。", permissions: ["asset.read"] });
      setResult("completed");
    } catch (reason) {
      setResult(reason && typeof reason === "object" && "code" in reason ? String(reason.code) : "failed");
    }
  };
  return <div>
    <output data-testid="mutation-result">{result}</output>
    {runtime.roles.map((item) => <span key={item.id}>{item.name}</span>)}
    <button type="button" onClick={() => void create()}>尝试创建角色</button>
  </div>;
}

function makeApi(overrides: Partial<AuthorizationAdminApi> = {}): AuthorizationAdminApi {
  return {
    listRoles: vi.fn().mockResolvedValue([role]),
    listBindings: vi.fn().mockResolvedValue([]),
    createRole: vi.fn(),
    updateRole: vi.fn(),
    createBinding: vi.fn(),
    revokeBinding: vi.fn(),
    inspect: vi.fn(),
    ...overrides,
  };
}

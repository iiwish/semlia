import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";

import { AccessControlView } from "./AccessControlView";
import { CapabilityProvider } from "./authorization";
import {
  AuthorizationAdminRuntimeProvider,
  type AuthorizationAdminApi,
} from "./authorizationAdminRuntime";
import type { AuthorizationBinding, AuthorizationRole, CapabilitySession } from "./types";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const principalId = "prn_01arz3ndektsv4rrffq69g5fav";
const session: CapabilitySession = {
  principalId,
  version: "7",
  capabilities: ["role.read", "role.manage", "role.assign", "authorization.inspect"],
};

const systemRole: AuthorizationRole = {
  id: "workspace_admin",
  name: "Workspace Admin",
  description: "管理工作区授权。",
  category: "system",
  permissions: ["workspace.read", "role.read", "role.manage", "role.assign", "authorization.inspect"],
  version: 1,
};

const customRole: AuthorizationRole = {
  id: "custom_catalog_reader",
  name: "目录读取者",
  description: "读取已发布目录。",
  category: "custom",
  permissions: ["workspace.read", "asset.read"],
  version: 2,
};

function makeApi(overrides: Partial<AuthorizationAdminApi> = {}): AuthorizationAdminApi {
  return {
    listRoles: vi.fn().mockResolvedValue([systemRole]),
    listBindings: vi.fn().mockResolvedValue([]),
    createRole: vi.fn().mockResolvedValue({ value: customRole, authorizationVersion: 8 }),
    updateRole: vi.fn().mockResolvedValue({ value: { ...customRole, version: 3 }, authorizationVersion: 8 }),
    createBinding: vi.fn(),
    revokeBinding: vi.fn(),
    inspect: vi.fn(),
    ...overrides,
  };
}

function renderAccessControl(api: AuthorizationAdminApi, onNotify = vi.fn(), capabilities = session.capabilities) {
  const access = {
    roleRead: capabilities.includes("role.read"),
    roleManage: capabilities.includes("role.manage"),
    roleAssign: capabilities.includes("role.assign"),
    authorizationInspect: capabilities.includes("authorization.inspect"),
  };
  return render(
    <CapabilityProvider session={{ ...session, capabilities: [...capabilities] }}>
      <AuthorizationAdminRuntimeProvider workspaceId={workspaceId} api={api} access={access}>
        <AccessControlView onNotify={onNotify} />
      </AuthorizationAdminRuntimeProvider>
    </CapabilityProvider>,
  );
}

describe("real authorization administration", () => {
  it("shows an honest restricted role-management surface without loading unreadable catalogues", async () => {
    const api = makeApi({
      listRoles: vi.fn().mockRejectedValue(Object.assign(new Error("roles forbidden"), { status: 403, code: "FORBIDDEN" })),
      listBindings: vi.fn().mockRejectedValue(Object.assign(new Error("bindings forbidden"), { status: 403, code: "FORBIDDEN" })),
    });

    renderAccessControl(api, vi.fn(), ["role.manage"]);

    expect(await screen.findByRole("tab", { name: "角色与权限" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByText("角色管理上下文不可用")).toBeVisible();
    expect(screen.getByText(/缺少 role.read/)).toBeVisible();
    expect(screen.queryByRole("tab", { name: "角色分配" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "有效权限检查" })).not.toBeInTheDocument();
    expect(api.listRoles).not.toHaveBeenCalled();
    expect(api.listBindings).not.toHaveBeenCalled();
  });

  it("lets an inspect-only session operate without requesting role or assignment catalogues", async () => {
    const api = makeApi({
      listRoles: vi.fn().mockRejectedValue(Object.assign(new Error("roles forbidden"), { status: 403, code: "FORBIDDEN" })),
      listBindings: vi.fn().mockRejectedValue(Object.assign(new Error("bindings forbidden"), { status: 403, code: "FORBIDDEN" })),
      inspect: vi.fn().mockResolvedValue({
        principalId,
        action: "release.publish",
        allowed: false,
        reasonCode: "NO_MATCHING_GRANT",
        explanation: "没有匹配授权。",
        authorizationVersion: "7",
        scope: { type: "workspace", id: workspaceId, label: workspaceId },
      }),
    });
    const user = userEvent.setup();

    renderAccessControl(api, vi.fn(), ["authorization.inspect"]);

    expect(await screen.findByRole("tab", { name: "有效权限检查" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByRole("tab", { name: "角色与权限" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "角色分配" })).not.toBeInTheDocument();
    expect(api.listRoles).not.toHaveBeenCalled();
    expect(api.listBindings).not.toHaveBeenCalled();
    await user.type(screen.getByLabelText("检查主体 ID"), principalId);
    await user.click(screen.getByRole("button", { name: "检查有效权限" }));
    expect(await screen.findByText("服务端最终决策")).toBeVisible();
  });

  it("lets an assignment-only session load bindings while role reads remain forbidden", async () => {
    const binding: AuthorizationBinding = {
      id: "bnd_assign_only",
      workspaceId,
      principalId,
      roleId: "custom_external_role",
      roleVersion: 4,
      scope: { type: "workspace", id: workspaceId, label: workspaceId },
      assignedBy: principalId,
      assignedAt: "2026-09-04T09:00:00Z",
      status: "active",
      version: 2,
    };
    const api = makeApi({
      listRoles: vi.fn().mockRejectedValue(Object.assign(new Error("roles forbidden"), { status: 403, code: "FORBIDDEN" })),
      listBindings: vi.fn().mockResolvedValue([binding]),
    });

    renderAccessControl(api, vi.fn(), ["role.assign"]);

    expect(await screen.findByRole("tab", { name: "角色分配" })).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByRole("row", { name: new RegExp(principalId) })).toHaveTextContent("custom_external_role");
    expect(screen.queryByRole("tab", { name: "角色与权限" })).not.toBeInTheDocument();
    expect(api.listRoles).not.toHaveBeenCalled();
    expect(api.listBindings).toHaveBeenCalledTimes(1);
  });

  it("creates an assignment from explicit role identity when the role catalogue is unavailable", async () => {
    const binding: AuthorizationBinding = {
      id: "bnd_assign_only_create",
      workspaceId,
      principalId,
      roleId: "rol_01arz3ndektsv4rrffq69g5fav",
      roleVersion: 4,
      scope: { type: "workspace", id: workspaceId, label: workspaceId },
      assignedBy: principalId,
      assignedAt: "2026-09-04T09:00:00Z",
      status: "active",
      version: 1,
    };
    const listBindings = vi.fn().mockResolvedValueOnce([]).mockResolvedValueOnce([binding]);
    const api = makeApi({
      listRoles: vi.fn().mockRejectedValue(Object.assign(new Error("roles forbidden"), { status: 403, code: "FORBIDDEN" })),
      listBindings,
      createBinding: vi.fn().mockResolvedValue({ value: binding, authorizationVersion: 8 }),
    });
    const user = userEvent.setup();

    renderAccessControl(api, vi.fn(), ["role.assign"]);

    await screen.findByText("尚无角色分配");
    await user.click(screen.getByRole("button", { name: "分配角色" }));
    const dialog = screen.getByRole("dialog", { name: "分配角色" });
    await user.type(within(dialog).getByLabelText("主体 ID"), principalId);
    await user.type(within(dialog).getByLabelText("角色 ID"), binding.roleId);
    await user.clear(within(dialog).getByLabelText("角色版本"));
    await user.type(within(dialog).getByLabelText("角色版本"), "4");
    await user.click(within(dialog).getByRole("button", { name: "预览授权" }));
    await user.click(within(dialog).getByRole("button", { name: "确认分配" }));

    expect(api.createBinding).toHaveBeenCalledWith(workspaceId, expect.objectContaining({
      principalId,
      roleId: binding.roleId,
      expectedRoleVersion: 4,
    }));
    expect(await screen.findByRole("row", { name: new RegExp(principalId) })).toBeVisible();
    expect(listBindings).toHaveBeenCalledTimes(2);
    expect(api.listRoles).not.toHaveBeenCalled();
  });

  it("keeps role management controls and assignment data hidden from a role-read-only session", async () => {
    const api = makeApi();
    const user = userEvent.setup();

    renderAccessControl(api, vi.fn(), ["role.read"]);

    await user.click(await screen.findByRole("button", { name: "查看角色 Workspace Admin" }));
    const detail = screen.getByRole("dialog", { name: "Workspace Admin 角色详情" });
    expect(within(detail).getByText("不可见")).toBeVisible();
    expect(within(detail).queryByRole("button", { name: "基于此角色创建" })).not.toBeInTheDocument();
    expect(api.listBindings).not.toHaveBeenCalled();
  });

  it("keeps a successful assignment surface when the independent role read is forbidden", async () => {
    const binding: AuthorizationBinding = {
      id: "bnd_split_state",
      workspaceId,
      principalId,
      roleId: "custom_external_role",
      roleVersion: 4,
      scope: { type: "workspace", id: workspaceId, label: workspaceId },
      assignedBy: principalId,
      assignedAt: "2026-09-04T09:00:00Z",
      status: "active",
      version: 2,
    };
    const api = makeApi({
      listRoles: vi.fn().mockRejectedValue(Object.assign(new Error("没有角色读取权限"), { status: 403, code: "FORBIDDEN" })),
      listBindings: vi.fn().mockResolvedValue([binding]),
    });
    const user = userEvent.setup();

    renderAccessControl(api, vi.fn(), ["role.read", "role.assign"]);

    expect(await screen.findByRole("alert")).toHaveTextContent("没有角色读取权限");
    await user.click(screen.getByRole("tab", { name: "角色分配" }));
    expect(await screen.findByRole("row", { name: new RegExp(principalId) })).toBeVisible();
  });

  it("renders a recoverable backend failure without fixture fallback", async () => {
    const api = makeApi({ listRoles: vi.fn().mockRejectedValue(new Error("authorization storage unreachable")) });
    const user = userEvent.setup();

    renderAccessControl(api);

    expect(await screen.findByRole("alert")).toHaveTextContent("authorization storage unreachable");
    expect(screen.queryByText("Security Admin")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "重试加载访问控制" }));
    expect(api.listRoles).toHaveBeenCalledTimes(2);
  });

  it("creates a custom role only after server confirmation and refetch", async () => {
    const listRoles = vi.fn()
      .mockResolvedValueOnce([systemRole])
      .mockResolvedValueOnce([systemRole, customRole]);
    const api = makeApi({ listRoles });
    const onNotify = vi.fn();
    const user = userEvent.setup();

    renderAccessControl(api, onNotify);

    await user.click(await screen.findByRole("button", { name: "查看角色 Workspace Admin" }));
    const detail = screen.getByRole("dialog", { name: "Workspace Admin 角色详情" });
    expect(within(detail).queryByRole("button", { name: /编辑角色/ })).not.toBeInTheDocument();
    await user.click(within(detail).getByRole("button", { name: "基于此角色创建" }));
    const editor = screen.getByRole("dialog", { name: /基于 Workspace Admin 创建角色/ });
    await user.clear(within(editor).getByLabelText("角色名称"));
    await user.type(within(editor).getByLabelText("角色名称"), "目录读取者");
    await user.clear(within(editor).getByLabelText("角色说明"));
    await user.type(within(editor).getByLabelText("角色说明"), "读取已发布目录。");
    await user.click(within(editor).getByRole("button", { name: "预览角色变更" }));
    await user.click(within(editor).getByRole("button", { name: "创建自定义角色" }));

    expect(api.createRole).toHaveBeenCalledWith(workspaceId, {
      name: "目录读取者",
      description: "读取已发布目录。",
      permissions: systemRole.permissions,
    });
    expect(await screen.findByRole("button", { name: "查看角色 目录读取者" })).toBeVisible();
    expect(listRoles).toHaveBeenCalledTimes(2);
    expect(onNotify).toHaveBeenCalledWith("目录读取者已创建，授权版本已刷新。");
  });

  it("creates and revokes bindings through versioned server commands", async () => {
    const binding: AuthorizationBinding = {
      id: "bnd_01arz3ndektsv4rrffq69g5fav",
      workspaceId,
      principalId,
      roleId: systemRole.id,
      roleVersion: 1,
      scope: { type: "workspace", id: workspaceId, label: workspaceId },
      assignedBy: principalId,
      assignedAt: "2026-09-04T09:00:00Z",
      status: "active",
      version: 1,
    };
    const listBindings = vi.fn().mockResolvedValueOnce([]).mockResolvedValueOnce([binding]).mockResolvedValueOnce([{ ...binding, status: "revoked", version: 2 }]);
    const api = makeApi({
      listBindings,
      createBinding: vi.fn().mockResolvedValue({ value: binding, authorizationVersion: 8 }),
      revokeBinding: vi.fn().mockResolvedValue({ value: { ...binding, status: "revoked", version: 2 }, authorizationVersion: 9 }),
    });
    const user = userEvent.setup();

    renderAccessControl(api);
    await user.click(await screen.findByRole("tab", { name: "角色分配" }));
    await user.click(screen.getByRole("button", { name: "分配角色" }));
    const dialog = screen.getByRole("dialog", { name: "分配角色" });
    await user.type(within(dialog).getByLabelText("主体 ID"), principalId);
    await user.click(within(dialog).getByRole("button", { name: "预览授权" }));
    await user.click(within(dialog).getByRole("button", { name: "确认分配" }));

    expect(api.createBinding).toHaveBeenCalledWith(workspaceId, expect.objectContaining({
      principalId,
      roleId: systemRole.id,
      expectedRoleVersion: 1,
      scope: { type: "workspace", id: workspaceId, label: workspaceId },
    }));
    const row = await screen.findByRole("row", { name: new RegExp(principalId) });
    await user.click(within(row).getByRole("button", { name: "撤销分配" }));
    const revokeDialog = screen.getByRole("dialog", { name: "撤销角色分配" });
    await user.type(within(revokeDialog).getByLabelText("撤销原因"), "职责调整");
    await user.click(within(revokeDialog).getByRole("button", { name: "确认撤销" }));

    expect(api.revokeBinding).toHaveBeenCalledWith(workspaceId, binding.id, { expectedVersion: 1, reason: "职责调整" });
    expect(listBindings).toHaveBeenCalledTimes(3);
  });

  it("keeps a stale custom role unchanged when the server reports a conflict", async () => {
    const conflict = Object.assign(new Error("角色版本已更新，请重新加载。"), { status: 409, code: "VERSION_CONFLICT" });
    const listRoles = vi.fn().mockResolvedValue([customRole]);
    const api = makeApi({ listRoles, updateRole: vi.fn().mockRejectedValue(conflict) });
    const user = userEvent.setup();

    renderAccessControl(api);
    await user.click(await screen.findByRole("button", { name: "查看角色 目录读取者" }));
    await user.click(screen.getByRole("button", { name: "编辑角色" }));
    const editor = screen.getByRole("dialog", { name: "编辑角色 目录读取者" });
    await user.clear(within(editor).getByLabelText("角色名称"));
    await user.type(within(editor).getByLabelText("角色名称"), "新的本地名称");
    await user.click(within(editor).getByRole("button", { name: "预览角色变更" }));
    await user.click(within(editor).getByRole("button", { name: "保存角色变更" }));

    expect(await within(editor).findByRole("alert")).toHaveTextContent("角色版本已更新，请重新加载");
    expect(api.updateRole).toHaveBeenCalledWith(workspaceId, customRole.id, expect.objectContaining({ expectedVersion: 2, name: "新的本地名称" }));
    expect(listRoles).toHaveBeenCalledTimes(1);
  });

  it("uses the server inspector and keeps a forbidden result explicit", async () => {
    const inspect = vi.fn().mockRejectedValue(Object.assign(new Error("没有有效权限检查权限"), { status: 403, code: "FORBIDDEN" }));
    const api = makeApi({ inspect });
    const user = userEvent.setup();

    renderAccessControl(api);
    await user.click(await screen.findByRole("tab", { name: "有效权限检查" }));
    await user.type(screen.getByLabelText("检查主体 ID"), principalId);
    await user.click(screen.getByRole("button", { name: "检查有效权限" }));

    expect(inspect).toHaveBeenCalledWith(workspaceId, {
      principalId,
      action: "release.publish",
      resource: { type: "workspace", id: workspaceId, label: workspaceId },
    });
    expect(await screen.findByRole("alert")).toHaveTextContent("没有有效权限检查权限");
  });
});

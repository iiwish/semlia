import { afterEach, expect, vi } from "vitest";

import { clearSessionCSRFToken } from "./apiClient";
import { authorizationAdminApi, AuthorizationAdminApiError } from "./authorizationAdmin";
import { getSession } from "./identity";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const principalId = "prn_01arz3ndektsv4rrffq69g5fav";

afterEach(() => {
  vi.unstubAllGlobals();
  clearSessionCSRFToken();
});

describe("authorization administration client", () => {
  it("uses the generated shared client, CSRF verifier, and versioned role contract", async () => {
    const requests: Request[] = [];
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      requests.push(request);
      if (request.url.endsWith("/api/v1/session")) return sessionResponse();
      if (request.url.endsWith("/authorization/roles") && request.method === "GET") return json({ items: [roleDTO()] });
      if (request.url.endsWith("/authorization/roles") && request.method === "POST") return json({ role: { ...roleDTO(), id: "custom_reader", category: "custom" }, authorizationVersion: 8 }, 201);
      return json({ code: "NOT_FOUND", message: "not found", traceId: "trace-x", details: {} }, 404);
    }));

    await getSession();
    const roles = await authorizationAdminApi.listRoles(workspaceId);
    const created = await authorizationAdminApi.createRole(workspaceId, {
      name: "目录读取者",
      description: "读取语义目录。",
      permissions: ["workspace.read", "asset.read"],
    });

    expect(roles[0]).toMatchObject({ id: "workspace_admin", category: "system", version: 1, permissions: ["workspace.read", "role.read"] });
    expect(created).toMatchObject({ value: { id: "custom_reader", category: "custom" }, authorizationVersion: 8 });
    const mutation = requests.find((request) => request.method === "POST");
    expect(mutation?.headers.get("X-Semlia-CSRF")).toBe("csrf-authorization-admin-tests-1234567890");
    expect(await mutation?.json()).toEqual({ name: "目录读取者", description: "读取语义目录。", actions: ["workspace.read", "asset.read"] });
  });

  it("keeps a server conflict explicit instead of mutating local state", async () => {
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      if (request.url.endsWith("/api/v1/session")) return sessionResponse();
      return json({ code: "VERSION_CONFLICT", message: "角色版本已更新。", traceId: "trace-conflict", details: {} }, 409);
    }));

    await getSession();
    await expect(authorizationAdminApi.updateRole(workspaceId, "custom_reader", {
      expectedVersion: 2,
      name: "目录读取者",
      description: "读取语义目录。",
      permissions: ["asset.read"],
    })).rejects.toMatchObject({ status: 409, code: "VERSION_CONFLICT", traceId: "trace-conflict" } satisfies Partial<AuthorizationAdminApiError>);
  });

  it("maps the authoritative inspect decision and requested scope", async () => {
    vi.stubGlobal("fetch", vi.fn(async (request: Request) => {
      if (request.url.endsWith("/api/v1/session")) return sessionResponse();
      if (request.url.endsWith("/authorization:inspect")) return json({ allowed: false, action: "release.publish", principalId, reasonCode: "NO_MATCHING_GRANT", authorizationVersion: 9 });
      return json({}, 404);
    }));

    await getSession();
    const result = await authorizationAdminApi.inspect(workspaceId, {
      principalId,
      action: "release.publish",
      resource: { type: "workspace", id: workspaceId, label: "当前工作区" },
    });

    expect(result).toMatchObject({ allowed: false, reasonCode: "NO_MATCHING_GRANT", authorizationVersion: "9", scope: { id: workspaceId, label: "当前工作区" } });
    expect(result.explanation).toContain("服务端没有找到");
  });
});

function roleDTO() {
  return { id: "workspace_admin", name: "Workspace Admin", description: "管理工作区。", category: "system", actions: ["workspace.read", "role.read"], version: 1 };
}

function sessionResponse() {
  return json({
    account: { id: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Admin" },
    workspaces: [{ id: workspaceId, slug: "alpha", displayName: "Alpha", principalId, roleIds: ["workspace_admin"], capabilities: ["workspace.read", "role.read", "role.manage", "role.assign", "authorization.inspect"], authorizationVersion: 7 }],
    expiresAt: "2026-09-04T18:00:00Z",
    traceId: "trace-session",
  }, 200, { "X-Semlia-CSRF": "csrf-authorization-admin-tests-1234567890" });
}

function json(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page, type Route } from "@playwright/test";
import { mkdirSync } from "node:fs";
import { resolve } from "node:path";

const evidenceDirectory = process.env.SEMLIA_BROWSER_EVIDENCE_DIR ?? resolve(process.cwd(), "../docs/evidence/ALPHA-T002/screenshots");
const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";

test.beforeAll(() => mkdirSync(evidenceDirectory, { recursive: true }));

test("unauthenticated users see a keyboard-accessible sign-in state", async ({ page }, testInfo) => {
  await page.route("**/api/v1/session", (route) => route.fulfill({ status: 401, contentType: "application/json", body: JSON.stringify({ code: "UNAUTHENTICATED", message: "session required", traceId: "trace-sign-in", details: {} }) }));
  await page.goto("/");

  const login = page.getByRole("link", { name: "使用组织账户登录" });
  await expect(page.getByRole("heading", { name: "登录工作区" })).toBeVisible();
  await login.focus();
  await expect(login).toBeFocused();
  await expect(page.getByLabel("Semlia 主功能")).toHaveCount(0);
  const accessibility = await new AxeBuilder({ page }).include(".session-entry-content").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: resolve(evidenceDirectory, `${testInfo.project.name}-sign-in.png`), fullPage: true });
});

test("two principals receive their current member capabilities without losing the desktop shell", async ({ page }, testInfo) => {
  let roleId = "workspace_admin";
  await installAuthenticatedRoutes(page, () => roleId);
  await page.goto("/");

  await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
  await expect(page.getByRole("button", { name: "知识资产" })).toBeVisible();
  await expect(page.getByRole("button", { name: "数据接入" })).toBeVisible();
  await expect(page.getByLabel("账户 Alpha Admin")).toBeVisible();
  await page.getByRole("button", { name: "系统设置" }).click();
  await expect(page.getByRole("region", { name: "工作区成员列表" })).toBeVisible();
  await expect(page.getByRole("button", { name: "邀请成员" })).toBeVisible();
  await expect(page.getByRole("button", { name: "停用 Alpha Admin" })).toBeVisible();

  const clipped = await page.locator(".topbar, .member-admin-toolbar, .member-admin-table").evaluateAll((elements) => elements.filter((element) => element.scrollWidth > element.clientWidth + 1).length);
  expect(clipped).toBe(0);
  const accessibility = await new AxeBuilder({ page }).include(".member-administration-view").analyze();
  expect(accessibility.violations).toEqual([]);
  await page.screenshot({ path: resolve(evidenceDirectory, `${testInfo.project.name}-member-admin.png`), fullPage: true });

  roleId = "auditor";
  await page.reload();
  await page.getByRole("button", { name: "系统设置" }).click();
  await expect(page.getByRole("region", { name: "工作区成员列表" })).toBeVisible();
  await expect(page.getByRole("button", { name: "邀请成员" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "停用 Alpha Admin" })).toHaveCount(0);
  await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
});

test("an administrator can invite a member and logout with the session CSRF verifier", async ({ page }) => {
  let invitationHeader = "";
  let logoutHeader = "";
  await installAuthenticatedRoutes(page, () => "workspace_admin", (route) => {
    invitationHeader = route.request().headers()["x-semlia-csrf"] ?? "";
  }, (route) => {
    logoutHeader = route.request().headers()["x-semlia-csrf"] ?? "";
  });
  await page.goto("/");
  await page.getByRole("button", { name: "系统设置" }).click();
  await page.getByRole("button", { name: "邀请成员" }).click();
  const dialog = page.getByRole("dialog", { name: "邀请工作区成员" });
  await dialog.getByLabel("组织邮箱").fill("reviewer@example.com");
  await dialog.getByLabel("初始角色").selectOption("reviewer");
  await dialog.getByRole("button", { name: "创建邀请" }).click();
  await expect(page.getByText("reviewer@example.com")).toBeVisible();
  expect(invitationHeader).toBe("csrf-verifier-value-for-browser-tests-123456");

  await page.getByLabel("账户 Alpha Admin").click();
  await page.getByRole("button", { name: "退出登录" }).click();
  await expect(page.getByRole("heading", { name: "登录工作区" })).toBeVisible();
  expect(logoutHeader).toBe("csrf-verifier-value-for-browser-tests-123456");
});

async function installAuthenticatedRoutes(page: Page, role: () => string, onInvite?: (route: Route) => void, onLogout?: (route: Route) => void) {
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (path === "/api/v1/session" && request.method() === "GET") {
      return route.fulfill({ status: 200, headers: { "Content-Type": "application/json", "X-Semlia-CSRF": "csrf-verifier-value-for-browser-tests-123456" }, body: JSON.stringify(session(role())) });
    }
    if (path === "/api/v1/session" && request.method() === "DELETE") {
      onLogout?.(route);
      return route.fulfill({ status: 204 });
    }
    if (path === "/api/v1/workspaces") return json(route, { items: [workspace()] });
    if (path.endsWith("/catalog/assets")) return json(route, { items: [asset()], page: { limit: 100 } });
    if (path.endsWith("/members")) return json(route, { items: [member()] });
    if (path.endsWith("/invitations") && request.method() === "GET") return json(route, { items: [] });
    if (path.endsWith("/invitations") && request.method() === "POST") {
      onInvite?.(route);
      const body = request.postDataJSON() as { email: string; roleId: string };
      return json(route, { id: "ivn_01arz3ndektsv4rrffq69g5fav", issuer: "https://issuer.example", email: body.email, roleId: body.roleId, status: "pending", expiresAt: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(), createdAt: "2026-09-04T10:00:00Z" }, 201);
    }
    if (path.includes("/governance/")) return json(route, { items: [], page: { limit: 100 } });
    return json(route, { code: "NOT_FOUND", message: "not found", traceId: "trace-not-found", details: {} }, 404);
  });
}

function session(roleId: string) {
  const capabilities = ["workspace.read", "asset.read", "member.read", ...(roleId === "workspace_admin" ? ["member.manage"] : [])];
  return { account: { id: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Alpha Admin" }, workspaces: [{ id: workspaceId, slug: "alpha", displayName: "Alpha Workspace", principalId: "prn_01arz3ndektsv4rrffq69g5fav", roleIds: [roleId], capabilities, authorizationVersion: roleId === "auditor" ? 4 : 3 }], expiresAt: new Date(Date.now() + 60 * 60 * 1000).toISOString(), traceId: "trace-session" };
}

function workspace() {
  return { id: workspaceId, slug: "alpha", displayName: "Alpha Workspace", createdAt: "2026-09-04T08:00:00Z", updatedAt: "2026-09-04T08:00:00Z" };
}

function asset() {
  return { id: "ast_01arz3ndektsv4rrffq69g5fav", address: "commerce.net_revenue", assetType: "metric", lifecycleState: "active", currentRevisionId: "rev_01arz3ndektsv4rrffq69g5fav", title: "Net revenue", summary: "Revenue after refunds and adjustments.", updatedAt: "2026-09-04T08:00:00Z" };
}

function member() {
  return { id: "mbr_01arz3ndektsv4rrffq69g5fav", accountId: "usr_01arz3ndektsv4rrffq69g5fav", displayName: "Alpha Admin", principalId: "prn_01arz3ndektsv4rrffq69g5fav", status: "active", roleIds: ["workspace_admin"], admittedAt: "2026-09-04T08:00:00Z" };
}

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
}

import { test, expect, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { readSecrets } from "../../scripts/demo/provision.mjs";
import { connectActor } from "../../scripts/demo/session.mjs";

const root = resolve(import.meta.dirname, "../../.semlia/resume-demo/retail-v1");
const state = JSON.parse(readFileSync(`${root}/state.json`, "utf8"));

async function restoreSession(page: Page, slug: string) {
  const { client } = await connectActor(root, state, state.singleAdmin ? "admin" : `${slug}_author`, readSecrets(root).accountPassword);
  await page.context().addCookies([{ name: "semlia_session_dev", value: client.sessionCookie().split("=")[1], url: `http://127.0.0.1:${state.apiPort}`, httpOnly: true, sameSite: "Lax" }]);
}

async function login(page: Page, slug: string, usePassword = false) {
  if (!usePassword) {
    await restoreSession(page, slug);
    await page.goto("/");
    await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
    return;
  }
  await page.goto("/");
  await page.getByLabel("登录账号", { exact: true }).fill(state.singleAdmin ? "admin" : `${slug}_author`);
  await page.getByLabel("密码", { exact: true }).fill(readSecrets(root).accountPassword);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
}

async function screenshot(page: Page, name: string) {
  await page.waitForLoadState("networkidle");
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: `${root}/browser/${name}-${test.info().project.name}.png`, fullPage: true });
}

test("normal catalog displays the published synthetic business names", async ({ page }) => {
  await login(page, "showcase", test.info().project.name === "desktop");
  await page.getByRole("navigation", { name: "Semlia 主功能" }).getByRole("button", { name: "知识资产", exact: true }).click();
  await expect(page.getByText("净收入 · 合成零售", { exact: true }).first()).toBeVisible();
  await screenshot(page, "assets");
});

test("normal source pins, correction and rollback persist after refresh", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  await restoreSession(page, "showcase");
  page.on("response", response => { if (response.url().includes("/api/") && response.status() >= 400) errors.push(`${response.status()} ${new URL(response.url()).pathname}`); });
  const workspace = state.workspaces.showcase;
  await page.goto(`/governance?production=${workspace.baseline.attribution.operationId}`);
  await expect(page.getByLabel("对象名称", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: /固定来源依据/ }).click();
  await expect(page.getByText("历史可验证 · 覆盖完整", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: /固定来源依据/ }).click();
  await page.getByRole("tab", { name: "发布与恢复", exact: true }).click();
  await expect(page.getByText(workspace.baseline.id, { exact: true }).first()).toBeVisible();
  await screenshot(page, "first-production");
  await page.goto(`/governance?production=${workspace.correction.attribution.operationId}`);
  await expect(page.getByLabel("业务定义", { exact: true })).toHaveValue(/succeeded/);
  await page.getByRole("tab", { name: "验证与审核", exact: true }).click();
  await expect(page.getByText("验证通过", { exact: true }).first()).toBeVisible();
  await screenshot(page, "correction");
  await page.goto(`/governance?release=${workspace.rollback.id}`);
  await expect(page.getByText(/发布序列 #3 · 回滚发布/).first()).toBeVisible();
  await screenshot(page, "rollback");
  await page.goto(`/governance?production=${workspace.correctionDraft.operationId}`);
  await expect(page.getByLabel("业务定义", { exact: true })).toHaveValue(/succeeded/);
  await page.evaluate(() => localStorage.clear());
  await page.reload();
  await expect(page.getByLabel("业务定义", { exact: true })).toHaveValue(/succeeded/);
  await expect(page.getByRole("button", { name: "冻结并提交验证", exact: true })).toBeEnabled();
  await screenshot(page, "live-correction-draft");
  expect(errors).toEqual([]);
});

test("workshop retains one source and an empty production workspace", async ({ page }) => {
  test.skip(Boolean(state.singleAdmin), "Retired workshop is not a delivered workspace");
  await login(page, "workshop");
  await page.goto("/sources");
  await expect(page.getByRole("button", { name: "启动发现 合成零售 · 订单客户与退款", exact: true })).toBeVisible();
  await screenshot(page, "workshop-source");
  await page.goto("/governance?productionList=1");
  await expect(page.getByText("暂无生产记录", { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByText("暂无生产记录", { exact: true })).toBeVisible();
  await screenshot(page, "workshop-empty");
});

test("single admin can browse source and membership without exposing retired workspaces", async ({ page }) => {
  test.skip(!state.singleAdmin, "Single-admin delivery only");
  await login(page, "showcase");
  const response = await page.request.get("/api/v1/session");
  expect(response.ok()).toBeTruthy();
  const session = await response.json();
  expect(session.workspaces).toHaveLength(1);
  expect(session.workspaces[0].roleIds).toEqual(["workspace_admin"]);
  await page.goto("/sources");
  await expect(page.getByRole("button", { name: "启动发现 合成零售 · 订单客户与退款", exact: true })).toBeVisible();
  await screenshot(page, "admin-source");
});

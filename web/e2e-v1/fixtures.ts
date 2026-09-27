import { readFileSync, writeFileSync, lstatSync, existsSync } from "node:fs";
import { join } from "node:path";
import { expect, type BrowserContext, type Page } from "@playwright/test";

export const root = process.env.SEMLIA_V1_BROWSER_ROOT!;
if (!root || lstatSync(root).isSymbolicLink() || (lstatSync(root).mode & 0o077)) throw new Error("Unprotected V1 browser root");
function privateJSON<T>(path: string): T {
  const stat = lstatSync(path);
  if (stat.isSymbolicLink() || stat.mode & 0o077) throw new Error("Unprotected V1 browser input");
  try { return JSON.parse(readFileSync(path, "utf8")) as T; }
  catch { throw new Error("Invalid protected V1 browser input"); }
}
export const state = privateJSON<{ owner: string; workspace: { id: string }; sourceId: string; principals: Record<string, string>; published: Record<string, { targetId: string; revisionId: string }>; operations: Record<string, { operationId: string }>; discovery: unknown }>(join(root, "state.json"));
const password = privateJSON<{ accountPassword: string }>(join(root, "secrets.json")).accountPassword;
export const base = `/api/v1/workspaces/${state.workspace.id}`;
export const modelId = state.published["demo_202609.model"].targetId;
export const modelRoute = `/assets/${modelId}`;
export const privatePath = (name: string) => join(root, "browser", name);
export const artifactPath = (name: string) => privatePath(`${process.env.SEMLIA_V1_BROWSER_RUN_ID}/${name}`);
export function writeReceipt(name: string, value: unknown) { writeFileSync(privatePath(name), JSON.stringify(value, null, 2) + "\n", { mode: 0o600 }); }
export function readReceipt<T>(name: string): T | undefined { return existsSync(privatePath(name)) ? privateJSON<T>(privatePath(name)) : undefined; }

export async function login(page: Page, role: "author" | "reviewer" | "publisher" | "consumer" | "denied") {
  await page.goto("/");
  await page.getByLabel("登录账号", { exact: true }).fill(`v1_${role}`);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
  const session = await getJSON(page.context(), "/api/v1/session");
  expect(session.localUAT ?? false).toBe(false);
  expect(session.workspaces.some((item: { id: string; principalId: string }) => item.id === state.workspace.id && item.principalId === state.principals[role])).toBe(true);
  return session;
}

export async function getJSON(context: BrowserContext, path: string) {
  const response = await context.request.get(path);
  expect(response.ok(), `Read-only evidence request failed: ${response.status()}`).toBe(true);
  return response.json();
}

export function observe(page: Page) {
  const errors: string[] = [];
  const denied: { status: number; path: string }[] = [];
  page.on("pageerror", error => errors.push(error.message));
  page.on("console", message => { if (message.type() === "error") errors.push(message.text()); });
  page.on("response", response => { if (response.url().includes("/api/") && response.status() >= 400) denied.push({ status: response.status(), path: new URL(response.url()).pathname }); });
  return { errors, denied };
}

export async function screenshot(page: Page, name: string) {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: artifactPath(name + ".png"), animations: "disabled", fullPage: false });
}

export async function inspectSource(page: Page, name: string) {
  await page.goto("/sources");
  await page.getByRole("button", { name: "查看来源 V1 synthetic orders and customers", exact: true }).click();
  await page.getByRole("tab", { name: "运行历史", exact: true }).click();
  await page.getByRole("button", { name: /^查看运行 / }).first().click();
  const summary = page.getByLabel("发现结果摘要", { exact: true });
  await expect(summary.locator("article").filter({ hasText: "数据集" }).locator("strong")).toHaveText("2");
  await expect(summary.locator("article").filter({ hasText: "字段" }).locator("strong")).toHaveText("8");
  await expect(page.getByRole("button", { name: /semlia_demo_202609.orders/ })).toBeVisible();
  await expect(page.getByRole("button", { name: /semlia_demo_202609.customers/ })).toBeVisible();
  await screenshot(page, `source-${name}`);
}

export async function assertKeyboardFocus(page: Page) {
  const field = page.getByLabel("向 Semlia 提问");
  await field.focus(); await page.keyboard.press("Tab"); await page.keyboard.press("Shift+Tab");
  await expect(field).toBeFocused();
  const style = await field.evaluate(element => { const css = getComputedStyle(element); return { outline: css.outlineStyle, width: css.outlineWidth, shadow: css.boxShadow }; });
  expect(style.outline !== "none" && style.width !== "0px" || style.shadow !== "none").toBe(true);
  await page.emulateMedia({ reducedMotion: "reduce" });
  expect(await page.evaluate(() => matchMedia("(prefers-reduced-motion: reduce)").matches)).toBe(true);
}

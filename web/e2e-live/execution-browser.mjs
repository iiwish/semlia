/* global process, console, document, innerWidth, localStorage */
import { createServer } from "vite";
import react from "@vitejs/plugin-react";
import { chromium, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { mkdir } from "node:fs/promises";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const evidence = process.env.SEMLIA_BROWSER_EVIDENCE_DIR ?? resolve(root, "../docs/evidence/FMB-T007/attempts/a002/screenshots");
await mkdir(evidence, { recursive: true });
const server = await createServer({ configFile: false, root, plugins: [react()], server: { host: "127.0.0.1", port: 0, proxy: { "/api": process.env.SEMLIA_EXECUTION_BROWSER_API } } });
await server.listen();
const browser = await chromium.launch({ headless: true });
try {
  for (const viewport of [{ width: 1440, height: 900 }, { width: 1024, height: 768 }]) {
    const context = await browser.newContext({ viewport, reducedMotion: "reduce" });
    const page = await context.newPage();
    const errors = []; page.on("pageerror", error => errors.push(error.message));
    await page.goto(`${server.resolvedUrls.local[0]}e2e-live/execution-harness.html?workspace=${process.env.SEMLIA_EXECUTION_TEST_WORKSPACE}`);
    await page.getByRole("textbox", { name: "向 Semlia 提问" }).fill("统计已发布净收入");
    await page.getByRole("button", { name: "发送问题" }).click();
    await expect(page.getByText("待执行验证", { exact: true })).toBeVisible();
    const execute = page.getByRole("button", { name: "执行只读查询" });
    for (let index = 0; index < 20 && !await execute.evaluate(element => element === document.activeElement); index++) await page.keyboard.press("Tab");
    await expect(execute).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page.getByRole("cell", { name: "123456789.123456790", exact: true })).toBeVisible();
    await expect(page.getByRole("region", { name: "只读查询执行" })).toBeVisible();
    await page.locator(".execution-harness-scroll").evaluate(element => element.scrollTop = element.scrollHeight);
    const composer = await page.locator(".ask-composer").boundingBox();
    const executionButton = await page.getByRole("button", { name: "重新执行" }).boundingBox();
    if (!composer || !executionButton || executionButton.y + executionButton.height > composer.y) throw new Error("composer overlaps execution command");
    if (await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)) throw new Error("horizontal document overflow");
    const accessibility = await new AxeBuilder({ page }).analyze();
    const serious = accessibility.violations.filter(item => item.impact === "serious" || item.impact === "critical");
    if (serious.length) throw new Error(`accessibility violations: ${JSON.stringify(serious.map(item => ({ id: item.id, nodes: item.nodes.map(node => ({ target: node.target, failureSummary: node.failureSummary })) })))}`);
    await page.screenshot({ path: resolve(evidence, `ask-pg-success-${viewport.width}.png`), fullPage: true });
    await page.locator(".execution-harness-scroll").evaluate(element => element.scrollTop = 0);
    await page.screenshot({ path: resolve(evidence, `ask-pg-top-${viewport.width}.png`), fullPage: true });
    const retained = await page.evaluate(() => JSON.stringify(localStorage));
    if (retained.includes("123456789.123456790")) throw new Error("result persisted in local storage");
    await page.reload();
    await expect(page.getByText(/原始结果行未存储/)).toBeVisible();
    await expect(page.getByRole("cell", { name: "123456789.123456790", exact: true })).toHaveCount(0);
    await page.screenshot({ path: resolve(evidence, `ask-pg-reload-${viewport.width}.png`), fullPage: true });
    if (errors.length) throw new Error(`browser errors: ${errors.join("; ")}`);
    await context.close();
  }
  console.log("PASS: real Ask UI -> deterministic local Chat -> persisted release plan -> PostgreSQL -> reload metadata, 1440 and 1024 desktop");
} finally { await browser.close(); await server.close(); }

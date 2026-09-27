import { test, expect } from "@playwright/test";
import { login, observe, screenshot, modelId, readReceipt, writeReceipt } from "./fixtures";

test("@visual exact historical evidence has readable title and bounded correction control", async ({ page }, info) => {
  await login(page, "consumer");
  const observed = observe(page);
  const original = readReceipt("journey-desktop-production-natural-correction-route-query-optional.json")!;
  await page.goto(`/assets/${modelId}?revision=${original.baselineRevision}&release=${original.baselineRelease}`);
  const evidence = page.getByRole("region", { name: "已发布知识依据", exact: true });
  const heading = evidence.getByRole("heading", { level: 1 });
  const button = evidence.getByRole("button", { name: "修订当前知识", exact: true });
  await expect(heading).toHaveText("老客户客单价分析");
  await expect(button).toBeDisabled();
  const layout = await evidence.evaluate(element => {
    const rect = (value: Element) => { const r = value.getBoundingClientRect(); return { x: r.x, y: r.y, width: r.width, height: r.height }; };
    const header = element.querySelector(".asset-header-main")!;
    return { gridTemplateColumns: getComputedStyle(header).gridTemplateColumns, header: rect(header), copy: rect(element.querySelector(".asset-title-copy")!), heading: rect(element.querySelector("h1")!), button: rect(header.querySelector("button")!) };
  });
  writeReceipt(`${process.env.SEMLIA_V1_BROWSER_RUN_ID}/historical-layout-${info.project.name}.json`, { layout, ...observed });
  await screenshot(page, `historical-layout-${info.project.name}`);
  expect(layout.copy.width).toBeGreaterThanOrEqual(320);
  expect(layout.button.width).toBeLessThanOrEqual(240);
  expect(layout.heading.height).toBeLessThanOrEqual(64);
  expect(layout.copy.x + layout.copy.width <= layout.button.x + 1 || layout.copy.y + layout.copy.height <= layout.button.y + 1).toBe(true);
  expect(observed.errors).toEqual([]); expect(observed.denied).toEqual([]);
});

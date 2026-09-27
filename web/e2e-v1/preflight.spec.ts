import { test, expect } from "@playwright/test";
import { login, observe, screenshot, modelRoute, writeReceipt, assertKeyboardFocus, inspectSource } from "./fixtures";

test("@preflight normal password login, source and natural knowledge revision entry", async ({ page }) => {
  await login(page, "author");
  const observed = observe(page);
  await inspectSource(page, test.info().project.name);
  await page.goto(modelRoute);
  await page.getByRole("tab", { name: "定义", exact: true }).click();
  await page.getByRole("button", { name: "提出修订", exact: true }).click();
  await expect(page.getByRole("region", { name: /知识修订工作台$/ })).toBeVisible();
  await expect(page.getByLabel("业务定义候选值", { exact: true })).toBeEditable();
  await expect(page.getByRole("button", { name: "运行检查", exact: true })).toBeEnabled();
  await screenshot(page, `natural-revision-${test.info().project.name}`);
  await page.getByRole("button", { name: "取消修订", exact: true }).click();
  writeReceipt(`preflight-${test.info().project.name}.json`, { normalPasswordLogin: true, naturalEditableRevision: true, ...observed });
  expect(observed.errors).toEqual([]);
  expect(observed.denied).toEqual([]);
});

test("@preflight consumer keyboard focus and reduced motion", async ({ page }) => {
  await login(page, "consumer");
  const observed = observe(page);
  await page.goto("/ask");
  await assertKeyboardFocus(page);
  await screenshot(page, `keyboard-${test.info().project.name}`);
  writeReceipt(`consumer-preflight-${test.info().project.name}.json`, { reducedMotion: true, keyboardFocus: true, ...observed });
  expect(observed.errors).toEqual([]);
  expect(observed.denied).toEqual([]);
});

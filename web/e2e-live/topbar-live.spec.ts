import { expect, test } from "@playwright/test";
import { openAcceptanceSettings, selectAcceptanceWorkspace } from "./acceptance-controls";

test("keeps acceptance controls secondary without losing data or identity access", async ({ page }, testInfo) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/?view=assets");
  const topbar = page.locator(".topbar");
  const identity = topbar.getByRole("combobox", { name: "本机验收身份" });
  await expect(identity).toHaveValue("author");
  await expect(topbar.getByRole("combobox", { name: "工作区" })).toHaveCount(0);
  const refresh = page.getByRole("button", { name: "刷新知识目录" });
  await expect(refresh).toBeEnabled();
  await expect(page.getByRole("button", { name: /打开语义资产/ }).first()).toBeVisible();
  await expect(topbar.getByText("模拟数据", { exact: true })).toBeVisible();
  await refresh.click();
  await expect(refresh).toBeEnabled();
  await page.screenshot({ path: testInfo.outputPath("catalog.png") });
  await identity.focus();
  await expect(identity).toBeFocused();
  const box = await topbar.boundingBox();
  expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);

  await openAcceptanceSettings(page);
  const selector = page.getByRole("combobox", { name: "工作区" });
  const original = await selector.inputValue();
  const alternative = await selector.locator("option").evaluateAll((options, current) => options.map((option) => (option as HTMLOptionElement).value).find((value) => value !== current), original);
  expect(alternative).toBeTruthy();
  await page.screenshot({ path: testInfo.outputPath("acceptance-settings.png") });
  await selectAcceptanceWorkspace(page, alternative!);
  await expect(topbar.getByText("模拟数据", { exact: true })).toHaveCount(0);
  await selectAcceptanceWorkspace(page, original);
  await expect(topbar.getByText("模拟数据", { exact: true })).toBeVisible();
  for (const actor of ["reviewer", "publisher", "author"]) {
    await identity.selectOption(actor);
    await expect(identity).toHaveValue(actor);
    await openAcceptanceSettings(page);
    await expect(selector).toBeVisible();
  }
});

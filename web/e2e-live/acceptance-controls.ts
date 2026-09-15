import { expect, type Page } from "@playwright/test";

export async function openAcceptanceSettings(page: Page) {
  await page.getByRole("button", { name: "系统设置", exact: true }).click();
  await page.getByRole("complementary", { name: "治理上下文" }).getByRole("button", { name: /本机验收/ }).click();
}

export async function selectAcceptanceWorkspace(page: Page, workspaceId: string) {
  await openAcceptanceSettings(page);
  await page.getByRole("combobox", { name: "工作区" }).selectOption(workspaceId);
  await openAcceptanceSettings(page);
  await expect(page.getByRole("combobox", { name: "工作区" })).toHaveValue(workspaceId);
}

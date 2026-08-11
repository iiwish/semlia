import { expect, test } from "@playwright/test";

test("core semantic governance journey stays inspectable", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");

  await expect(page.getByText("Prototype · Mock data")).toBeVisible();
  await expect(page.getByRole("heading", { name: "语义治理概览" })).toBeVisible();
  await expect(page.getByRole("img", { name: "语义覆盖关系图" })).toBeVisible();
  await expect(page.locator(".workspace-canvas")).toHaveCSS("background-image", "none");
  await expect(page.locator(".metric-strip > div").first()).toHaveCSS("border-top-style", "solid");

  await page.getByRole("button", { name: "接入发现" }).click();
  await expect(page.getByRole("heading", { name: "接入与语义发现" })).toBeVisible();
  await expect(page.getByText("仅元数据读取")).toBeVisible();
  await expect(page.locator(".source-list > article").first()).toHaveCSS("border-top-style", "solid");
  await page.getByRole("button", { name: "运行增量发现" }).click();
  await expect(page.getByText("发现完成：6 个提案")).toBeVisible();
  await page.getByRole("button", { name: "查看 6 个提案" }).click();

  await page.getByRole("button", { name: /审核 PROP-128/ }).click();
  const dialog = page.getByRole("dialog", { name: "审核提案 PROP-128" });
  await expect(dialog.getByText("仅更新本次原型会话，不会写入或发布真实数据。")).toBeVisible();
  await dialog.getByRole("button", { name: "模拟批准" }).click();
  await expect(page.getByRole("status")).toContainText("本次原型会话中标记为批准");

  await page.getByRole("button", { name: "发布版本" }).click();
  await expect(page.getByRole("heading", { name: "Release candidate" })).toBeVisible();
  await page.getByRole("button", { name: "模拟发布 candidate" }).click();
  const publishDialog = page.getByRole("dialog", { name: "发布 release candidate" });
  await expect(publishDialog.getByText("不会生成真实 manifest 或改变消费者 binding", { exact: false })).toBeVisible();
  await publishDialog.getByRole("button", { name: "确认模拟发布" }).click();
  await expect(page.getByText("release-2026.08.4-session")).toBeVisible();

  await page.getByRole("button", { name: "消费监控" }).click();
  await expect(page.getByRole("heading", { name: "消费与反馈" })).toBeVisible();
  await expect(page.locator(".channel-grid > button").first()).toHaveCSS("border-top-style", "solid");
  await page.getByRole("button", { name: "创建绑定" }).click();
  const bindingDialog = page.getByRole("dialog", { name: "创建消费绑定" });
  await bindingDialog.getByLabel("消费者名称").fill("Revenue Copilot");
  await bindingDialog.getByRole("button", { name: "创建模拟绑定" }).click();
  await expect(page.getByText("Revenue Copilot", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "生成修复提案" }).click();
  await expect(page.getByRole("heading", { name: "变更提案" })).toBeVisible();

  await page.getByRole("button", { name: "语义资产" }).click();
  await page.getByRole("searchbox", { name: "搜索语义资产" }).fill("净收入");
  await page.getByRole("button", { name: /净收入，指标/ }).click();
  await expect(page.getByRole("heading", { name: "净收入" })).toBeVisible();
  await page.getByRole("tab", { name: "证据" }).click();
  await expect(page.getByText("Cube schema 编译通过")).toBeVisible();

  const hasDocumentOverflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
  expect(hasDocumentOverflow).toBe(false);
});

test("keyboard navigation reaches the core review flow", async ({ page }) => {
  await page.goto("/");

  const assetNavigation = page.getByRole("button", { name: "语义资产" });
  await assetNavigation.focus();
  await expect(assetNavigation).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: "语义资产" })).toBeVisible();

  const proposalNavigation = page.getByRole("button", { name: "变更提案" });
  await proposalNavigation.focus();
  await page.keyboard.press("Enter");
  await page.getByRole("button", { name: /审核 PROP-128/ }).click();

  await expect(page.getByRole("button", { name: "关闭审核" })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog", { name: "审核提案 PROP-128" })).toBeHidden();
});

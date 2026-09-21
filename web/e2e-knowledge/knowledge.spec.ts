import { expect, test, type Page } from "@playwright/test";

async function overflow(page: Page) {
  expect(await page.locator(".catalog-create-dialog, .knowledge-spec-editor, .knowledge-reference-dialog, .knowledge-revision-workbench, .knowledge-source-picker").evaluateAll((elements) => elements.filter((element) => element.scrollWidth > element.clientWidth + 1).map((element) => element.className))).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  for (const modal of await page.getByRole("dialog").all()) {
    const bounds = await modal.boundingBox();
    expect(bounds).not.toBeNull();
    const viewport = page.viewportSize()!;
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.y).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height);
  }
}

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const asset = { id: "object-order", assetType: "business_object", title: "支付订单 · 合成", address: "fixture.paid_order", currentRevision: { id: "revision-order", content: { spec: { members: [{ id: "amount", name: "支付金额" }, { id: "paid_at", name: "支付时间" }] } } }, authoritySections: [{ kind: "released_state", availability: "available", revisionId: "revision-order", releaseId: "release-fixture" }] };
    const body = path.endsWith("/catalog/assets") ? { items: [asset], page: { limit: 100 } }
      : path.endsWith("/catalog/assets/object-order") ? asset
      : path.endsWith("/sources") ? { items: [{ id: "source-fixture", name: "订单来源 · 合成" }], limit: 100, total: 1 }
      : path.endsWith("/snapshots") ? { items: [{ id: "snapshot-fixture", createdAt: "2026-09-21T08:00:00Z", historyQuality: "verified" }], nextCursor: null }
      : path.endsWith("/members") ? { items: [{ objectId: "orders", name: "fixture.orders", kind: "dataset", revisionId: "source-revision-fixture" }, { objectId: "amount", name: "amount", kind: "field", parentObjectId: "orders", revisionId: "source-revision-fixture" }], nextCursor: null }
      : {};
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });
  await page.goto("/e2e-knowledge/harness.html");
  await expect(page.getByText(/合成验收数据 · API/)).toBeVisible();
  await page.locator("main").evaluate((element) => { element.style.transform = "translateZ(0)"; });
});

test("five-type creation, published member selection, typed literals and keyboard focus", async ({ page }, info) => {
  await page.getByRole("button", { name: "新建知识" }).click();
  const dialog = page.getByRole("dialog", { name: "新建知识", exact: true });
  await expect(dialog.getByLabel("语义地址")).toBeFocused();
  await expect(dialog.getByLabel("资产类型").locator("option")).toHaveCount(5);
  for (const type of ["business_object", "business_term", "metric", "data_asset", "analysis_model"]) {
    await dialog.getByLabel("资产类型").selectOption(type);
    if (type === "business_object") {
      await dialog.getByRole("button", { name: "添加成员", exact: true }).click();
      await dialog.getByLabel("成员 1 含义").fill("支付金额");
      await dialog.getByLabel("成员 1 类型").selectOption("number");
    }
    if (type === "business_term") {
      await dialog.getByLabel("口径能力").selectOption("predicate");
      await dialog.getByLabel("判定条件算子", { exact: true }).selectOption("literal");
      await dialog.getByLabel("判定条件固定值类型").selectOption("number");
      await dialog.getByLabel("判定条件固定值", { exact: true }).fill("12.5");
      await expect(dialog.getByLabel("判定条件固定值", { exact: true })).toHaveValue("12.5");
      await dialog.getByLabel("判定条件固定值类型").selectOption("boolean");
      await dialog.getByLabel("判定条件固定值", { exact: true }).selectOption("true");
    }
    if (type === "metric") {
      const trigger = dialog.getByRole("button", { name: "选择输入属性" });
      await trigger.click();
      const picker = page.getByRole("dialog", { name: "选择输入属性" });
      await expect(picker.getByLabel("搜索已发布知识")).toBeFocused();
      await picker.getByRole("button", { name: /支付订单 · 合成/ }).click();
      await picker.getByLabel("已发布成员").selectOption("amount");
      await overflow(page);
      await page.screenshot({ path: info.outputPath("published-member-picker.png"), fullPage: true });
      await picker.getByRole("button", { name: "固定此版本" }).click();
      await expect(trigger).toBeFocused();
      await dialog.getByText("固定版本标识").click();
      await expect(dialog.getByText("revision-order")).toBeVisible();
      await trigger.click();
      await page.keyboard.press("Escape");
      await expect(picker).toHaveCount(0);
      await expect(dialog).toBeVisible();
      await expect(trigger).toBeFocused();
    }
    if (type === "data_asset") {
      await dialog.getByLabel("知识数据来源").selectOption("source-fixture");
      await dialog.getByLabel("知识来源快照").selectOption("snapshot-fixture");
      await dialog.getByLabel("来源数据集版本").selectOption({ label: "fixture.orders" });
      await dialog.getByRole("button", { name: "添加成员", exact: true }).click();
      await dialog.getByLabel("成员 1 来源字段").selectOption({ label: "amount" });
      await expect(dialog.getByText("已加载 2 个来源成员 · 已全部加载")).toBeVisible();
    }
    if (type === "analysis_model") await dialog.getByRole("button", { name: "添加成员映射" }).click();
    await overflow(page);
    await dialog.locator(".catalog-create-fields").evaluate((element) => { element.scrollTop = 0; });
    await page.screenshot({ path: info.outputPath(`${type}.png`), fullPage: true });
  }
  await dialog.getByRole("button", { name: "创建并打开" }).focus();
  await page.keyboard.press("Tab");
  await expect(dialog.getByRole("button", { name: "关闭", exact: true })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("button", { name: "新建知识" })).toBeFocused();
});

test("canonical data revision selects a snapshot and submits typed structure", async ({ page }, info) => {
  await page.getByRole("button", { name: "修订数据资产" }).click();
  await page.getByLabel("知识数据来源").selectOption("source-fixture");
  await page.getByLabel("知识来源快照").selectOption("snapshot-fixture");
  await page.getByLabel("来源数据集版本").selectOption({ label: "fixture.orders" });
  await page.getByRole("button", { name: "添加成员", exact: true }).click();
  await page.getByLabel("成员 1 来源字段").selectOption({ label: "amount" });
  await page.getByRole("button", { name: "运行检查" }).click();
  await overflow(page);
  await page.locator("main").first().evaluate((element) => { element.scrollTop = 0; });
  await page.screenshot({ path: info.outputPath("data-asset-revision.png"), fullPage: true });
  await page.getByRole("button", { name: "提交审核" }).click();
  const submission = JSON.parse(await page.getByLabel("合成提案提交结果").innerText());
  expect(submission.changes[0].after.datasetRef.snapshotId).toBe("snapshot-fixture");
  expect(submission.changes[0].after.datasetRef.revisionId).toBe("source-revision-fixture");
  expect(submission.changes[0].after.members[0].sourceFieldRef.objectId).toBe("amount");
});

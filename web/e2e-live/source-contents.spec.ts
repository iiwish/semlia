import { test, expect } from "@playwright/test";

test("source contents: real snapshot and bounded synthetic files", async ({ page }, info) => {
  test.skip(!process.env.SEMLIA_SOURCE_PREVIEW_PASSWORD, "Requires explicit local acceptance credentials");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");
  await page.getByLabel("登录账号", { exact: true }).fill(process.env.SEMLIA_SOURCE_PREVIEW_USER ?? "admin");
  await page.getByLabel("密码", { exact: true }).fill(process.env.SEMLIA_SOURCE_PREVIEW_PASSWORD!);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
  const session = await page.request.get("/api/v1/session");
  const workspace = (await session.json()).workspaces[0].id;
  const csrf = session.headers()["x-semlia-csrf"];
  const sources = await (await page.request.get(`/api/v1/workspaces/${workspace}/sources?limit=100`)).json();
  const database = sources.items.find((item: { sourceKind: string }) => item.sourceKind === "postgresql");
  expect(database, "Acceptance requires one already-discovered database source").toBeTruthy();
  await page.goto(`/sources?source=${database.id}`);
  await page.getByRole("button", {name:`查看来源 ${database.name}`,exact:true}).click();
  await page.getByRole("tab", { name: "内容", exact: true }).click();
  await expect(page.getByRole("complementary", { name: "表和视图" }).getByRole("button").first()).toBeVisible();
  await expect(page.getByRole("region", { name: "字段结构" })).toContainText(/可空|未加载完成/);
  await page.screenshot({ path: info.outputPath("database-structure.png"), fullPage: true });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();

  for (const fixture of [
    { kind: "markdown", name: "source-preview-acceptance.md", mime: "text/markdown", content: "# 来源验收样例\n\n本文件为合成测试材料，不包含业务数据。\n\n## 收入口径\n\n收入采用含税金额，退款单独列示。\n" },
    { kind: "csv", name: "source-preview-acceptance.csv", mime: "text/csv", content: "order_id,amount,status\n" + Array.from({ length: 120 }, (_, index) => `${index + 1},${(index + 1) * 10},completed\n`).join("") },
  ]) {
    const sourceName = `来源预览验收（合成）-${fixture.kind}-${info.project.name}`;
    let source = sources.items.find((item: { name: string }) => item.name === sourceName);
    if (!source) {
      const upload = await page.request.post(`/api/v1/workspaces/${workspace}/ingestion/artifacts`, { headers: { "X-Semlia-CSRF": csrf, "Idempotency-Key": `preview-${fixture.kind}-${info.project.name}-${Date.now()}`, "X-Artifact-Kind": fixture.kind, "X-File-Name": fixture.name, "Content-Type": fixture.mime }, data: Buffer.from(fixture.content) });
      expect(upload.status(), await upload.text()).toBe(201);
      const artifact = await upload.json();
      const finalize = await page.request.post(`/api/v1/workspaces/${workspace}/ingestion/artifact-sets:finalize`, { headers: { "X-Semlia-CSRF": csrf, "Idempotency-Key": `preview-set-${fixture.kind}-${info.project.name}-${Date.now()}` }, data: { sourceName, artifactIds: [artifact.id] } });
      expect(finalize.status(), await finalize.text()).toBe(201);
      source = { id: (await finalize.json()).sourceId };
    }
    await page.goto(`/sources?source=${source.id}`);
    await page.getByRole("button", {name:`查看来源 ${sourceName}`,exact:true}).click();
    await page.getByRole("tab", { name: "内容", exact: true }).click();
    if (fixture.kind === "markdown") {
      await expect(page.getByRole("heading", { name: "收入口径", exact: true })).toBeVisible();
      await page.getByRole("tab", { name: "原文", exact: true }).click();
      await expect(page.locator(".content-raw")).toContainText("# 来源验收样例");
      await page.getByRole("tab", { name: "内容", exact: true }).last().click();
    } else {
      await expect(page.getByRole("region", { name: "文件数据预览" })).toContainText("order_id");
      await expect(page.getByText(/仅显示部分内容/)).toBeVisible();
      await expect(page.getByRole("region", { name: "文件数据预览" }).locator("tbody tr")).toHaveCount(100);
    }
    await page.screenshot({ path: info.outputPath(`${fixture.kind}-preview.png`), fullPage: true });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBeTruthy();
    await page.getByRole("button", { name: "返回数据来源", exact: true }).click();
    await expect(page.getByRole("table", { name: "数据来源", exact: true })).toBeVisible();
  }
});

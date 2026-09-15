import { expect, test, type APIRequestContext, type Page } from "@playwright/test";

interface WorkspaceResponse { id: string }
interface AssetResponse { id: string }
interface ProposalResponse { id: string; state: string }

async function selectWorkspace(page: Page, workspaceId: string) {
  const selector = page.getByRole("combobox", { name: "工作区" });
  await expect(selector).toBeVisible();
  await selector.selectOption(workspaceId);
}

async function switchActor(page: Page, actor: "提案发起" | "独立评审" | "独立发布") {
  await page.getByRole("button", { name: actor, exact: true }).click();
  await page.waitForLoadState("domcontentloaded");
  await expect(page.getByRole("button", { name: actor, exact: true, pressed: true })).toBeVisible();
}

async function waitForReviewState(request: APIRequestContext, workspaceId: string, proposalId: string) {
  await expect.poll(async () => {
    const response = await request.get(`/api/v1/workspaces/${workspaceId}/governance/proposals/${proposalId}`, {
      headers: { "X-Semlia-Principal": "local-author" },
    });
    if (!response.ok()) return `http-${response.status()}`;
    return ((await response.json()) as ProposalResponse).state;
  }, { timeout: 30_000 }).toBe("in_review");
}

test("real stack preserves the three-person governance journey across reloads", async ({ page, request }, testInfo) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const suffix = `${testInfo.project.name.replace(/[^a-z0-9]+/g, "-")}-${Date.now()}`;
  const assetName = `本机验收指标 ${suffix}`;

  const workspaceResponse = await request.post("/api/v1/workspaces", {
    data: { slug: `uat-${suffix}`, displayName: `本机验收 ${suffix}` },
  });
  expect(workspaceResponse.ok()).toBe(true);
  const workspace = (await workspaceResponse.json()) as WorkspaceResponse;

  const assetResponse = await request.post(`/api/v1/workspaces/${workspace.id}/catalog/assets`, {
    headers: { "X-Semlia-Principal": "local-author" },
    data: {
      address: `uat.${suffix.replaceAll("-", "_")}`,
      assetType: "metric",
      lifecycleState: "active",
      schemaVersion: "1.0.0",
      createdBy: "live-playwright",
      content: {
        name: assetName,
        definition: "统计已完成支付且未退款的订单数量。",
        expression: "COUNT_DISTINCT(order_id) FILTER payment_status = 'paid'",
        grain: "order_id",
        unit: "笔",
        aggregation: "count_distinct",
      },
    },
  });
  expect(assetResponse.ok()).toBe(true);
  const asset = (await assetResponse.json()) as AssetResponse;

  await page.goto("/");
  const primaryNavigation = page.getByRole("navigation", { name: "Semlia 主功能" });
  await expect(primaryNavigation.getByRole("button")).toHaveCount(5);
  for (const label of ["语义问答", "工作台", "知识资产", "变更与发布", "数据接入"]) {
    await expect(primaryNavigation.getByRole("button", { name: label, exact: true })).toBeVisible();
  }
  await expect(page.getByRole("navigation", { name: "Semlia 平台管理" }).getByRole("button", { name: "系统设置" })).toBeVisible();
  await selectWorkspace(page, workspace.id);
  await page.getByRole("button", { name: `打开语义资产 ${assetName}` }).click();
  await expect(page.getByRole("heading", { name: assetName })).toBeVisible();
  await page.getByRole("button", { name: "修订知识" }).click();
  await page.getByRole("dialog", { name: "选择知识修订对象" }).getByRole("button", { name: /计算表达式/ }).click();

  const workbench = page.getByRole("region", { name: `${assetName} 知识修订工作台` });
  await expect(workbench.getByRole("textbox", { name: "计算表达式候选值" })).toHaveValue("COUNT_DISTINCT(order_id) FILTER payment_status = 'paid'");
  await workbench.getByRole("textbox", { name: "计算表达式候选值" }).fill("COUNT_DISTINCT(order_id) FILTER payment_status = 'paid' AND refunded = false");
  await workbench.getByRole("textbox", { name: "知识修订原因" }).fill("本机真实链路验收：明确排除退款订单。");
  await workbench.getByRole("button", { name: "运行检查" }).click();
  const createResponse = page.waitForResponse((response) =>
    response.request().method() === "POST" && response.url().endsWith(`/api/v1/workspaces/${workspace.id}/governance/proposals`));
  await workbench.getByRole("button", { name: "提交审核" }).click();
  const proposalWireResponse = await createResponse;
  expect(proposalWireResponse.ok()).toBe(true);
  const proposal = (await proposalWireResponse.json()) as ProposalResponse;
  expect(proposal.id).toMatch(/^prp_/);
  await waitForReviewState(request, workspace.id, proposal.id);

  await switchActor(page, "独立评审");
  await selectWorkspace(page, workspace.id);
  await page.getByRole("button", { name: "变更与发布", exact: true }).click();
  const candidateRow = page.getByRole("button", { name: new RegExp(`查看候选资产版本 ${assetName}`) });
  await expect(candidateRow).toContainText("待审核");
  await candidateRow.click();
  await page.getByRole("button", { name: new RegExp(`审核候选版本 ${assetName}`) }).click();
  const reviewDialog = page.getByRole("dialog", { name: new RegExp(`审核 ${assetName}`) });
  await reviewDialog.getByRole("textbox", { name: "审核意见" }).fill("独立评审确认排除退款订单。 ");
  const reviewResponse = page.waitForResponse((response) =>
    response.request().method() === "POST" && response.url().endsWith(`/api/v1/workspaces/${workspace.id}/governance/proposals/${proposal.id}/reviews`));
  await reviewDialog.getByRole("button", { name: "批准版本" }).click();
  expect((await reviewResponse).status()).toBe(201);
  await expect(page.getByRole("status")).toContainText("评审已记录：批准");

  await switchActor(page, "独立发布");
  await selectWorkspace(page, workspace.id);
  await page.getByRole("button", { name: "变更与发布", exact: true }).click();
  const approvedCandidate = page.getByRole("button", { name: new RegExp(`查看候选资产版本 ${assetName}`) });
  await expect(approvedCandidate).toContainText("待发布");
  await approvedCandidate.click();
  await page.getByRole("button", { name: /^发布 @/ }).click();
  const publishDialog = page.getByRole("dialog", { name: new RegExp(`发布${assetName}`) });
  const publishResponse = page.waitForResponse((response) =>
    response.request().method() === "POST" && response.url().endsWith(`/api/v1/workspaces/${workspace.id}/governance/releases`));
  await publishDialog.getByRole("button", { name: "确认发布并生效" }).click();
  expect((await publishResponse).status()).toBe(201);
  await expect(publishDialog).toBeHidden();
  await expect(page.getByRole("region", { name: new RegExp(`${assetName} .*候选资产版本详情`) })).toContainText("发布记录");

  await page.reload();
  await expect(page.getByRole("button", { name: "独立发布", exact: true, pressed: true })).toBeVisible();
  await selectWorkspace(page, workspace.id);
  await page.getByRole("button", { name: "变更与发布", exact: true }).click();
  const releaseRow = page.getByRole("button", { name: /查看发布记录/ }).filter({ hasText: assetName }).first();
  await expect(releaseRow).toContainText("当前版本");
  await releaseRow.click();
  const releaseDetail = page.getByRole("region", { name: /发布 #1 详情/ });
  await expect(releaseDetail).toContainText("当前版本");
  const rollbackResponse = page.waitForResponse((response) =>
    response.request().method() === "POST" && response.url().endsWith("/rollback"));
  await releaseDetail.getByRole("button", { name: "回滚此发布" }).click();
  expect((await rollbackResponse).status()).toBe(201);
  await expect(page.locator(".toast")).toContainText("回滚完成：已创建新的不可变发布 #2");

  await page.reload();
  await selectWorkspace(page, workspace.id);
  await page.getByRole("button", { name: "变更与发布", exact: true }).click();
  await expect(page.getByRole("button", { name: /查看发布记录/ })).toHaveCount(2);
  await expect(page.getByText("回滚发布", { exact: true }).first()).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)).toBe(false);
  await page.screenshot({ path: testInfo.outputPath("local-uat-final.png"), fullPage: true });
  expect(asset.id).toMatch(/^ast_/);
});

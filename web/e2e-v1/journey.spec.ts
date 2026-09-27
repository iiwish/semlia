import { test, expect, type Browser, type Page, type Response } from "@playwright/test";
import { login, observe, screenshot, state, modelRoute, modelId, base, root, writeReceipt, readReceipt, getJSON, inspectSource } from "./fixtures";
import { goldenCases } from "../../scripts/acceptance/v1/core.mjs";
import { assertQueryWindow, counters, independentRows, reconcileExecution } from "../../scripts/acceptance/v1/model.mjs";
import { assertNaturalDraft, assertOperationRoute } from "../../scripts/acceptance/v1-browser-core.mjs";

type Actor = "author" | "reviewer" | "publisher" | "consumer";
async function actorPage(browser: Browser, role: Actor, viewport: { width: number; height: number }) {
  const context = await browser.newContext({ baseURL: process.env.SEMLIA_V1_BROWSER_BASE_URL, viewport, colorScheme: "light" });
  const page = await context.newPage(); await login(page, role);
  return { page, context, observed: observe(page) };
}
const responseFor = (page: Page, suffix: string) => page.waitForResponse(response => response.request().method() === "POST" && new URL(response.url()).pathname === base + suffix, { timeout: 80000 });
async function receipt(name: string, response: Response) {
  const body = await response.json();
  writeReceipt(name + ".json", { status: response.status(), idempotencyKey: response.request().headers()["idempotency-key"], request: response.request().postDataJSON(), body });
  expect(response.ok(), `Normal UI action failed: ${response.status()}`).toBe(true);
  return body;
}

test("@journey normal-password model revision, independent review, Ask, pinned evidence and rollback", async ({ browser }, info) => {
  const name = info.project.name, viewport = info.project.use.viewport!;
  const repair = process.env.SEMLIA_V1_BROWSER_REPAIR;
  if (repair) expect({ repair, name }).toEqual({ repair: "route-query-optional", name: "desktop" });
  const originalScenario = `${name}-production-natural-correction`;
  const interrupted = repair ? readReceipt(`journey-${originalScenario}.json`) : undefined;
  if (repair) {
    expect(interrupted?.operationId).toBeTruthy();
    expect(interrupted?.modelAttemptStarted).toBeUndefined();
    expect(interrupted?.publishedRelease).toBeUndefined();
  }
  const scenario = originalScenario + (repair ? `-${repair}` : "");
  const recordName = `journey-${scenario}.json`;
  const previous = readReceipt(recordName);
  expect(previous, "A journey is already recorded; retain its outcome and request an explicit named repair").toBeUndefined();
  const author = await actorPage(browser, "author", viewport), reviewer = await actorPage(browser, "reviewer", viewport);
  const publisher = await actorPage(browser, "publisher", viewport), consumer = await actorPage(browser, "consumer", viewport);
  const actors = { author, reviewer, publisher, consumer };
  const record: Record<string, unknown> = { name, scenario, correction: "Natural revision uses an unconfirmed production update draft, not the legacy proposal path", synthetic: true, before: counters(root), startedAt: new Date().toISOString() };
  if (repair) record.recovery = { reason: repair, interruptedRun: "1790433492790", originalLedger: `journey-${originalScenario}.json`, operationId: interrupted!.operationId };
  const save = () => writeReceipt(recordName, record);
  save();
  let operationId: string | undefined = interrupted?.operationId as string | undefined, publishedId: string | undefined, rollbackStarted = false;
  const baseline = await getJSON(publisher.context, base + "/governance/releases?limit=1");
  const r1 = await getJSON(publisher.context, base + "/production-releases/" + baseline.items[0].id);
  const originalPin = r1.afterManifest.assets.find((item: { assetId: string }) => item.assetId === modelId);
  expect(originalPin).toBeTruthy();
  const originalRevision = await getJSON(author.context, base + `/catalog/assets/${modelId}/revisions/${originalPin.revisionId}`);
  const originalOperation = await getJSON(author.context, base + "/production-operations/" + state.operations["demo_202609.model"].operationId);
  const r2Definition = `${originalRevision.content.definition}\n合成浏览器验收注记（${name}）：仅核对历史修订展示，不改变指标、日期、数据绑定或计算口径。`;
  record.baselineRelease = r1.id; record.baselineRevision = originalPin.revisionId; save();
  if (repair) {
    expect(r1.id).toBe(interrupted!.baselineRelease);
    expect(originalPin.revisionId).toBe(interrupted!.baselineRevision);
  }

  async function rollbackPublished() {
    if (!publishedId || rollbackStarted) return;
    await publisher.page.goto(`/work/operations/${operationId}?scope=initiated&productionRelease=${publishedId}`);
    await publisher.page.getByRole("tab", { name: "发布记录", exact: true }).click();
    await publisher.page.getByLabel("回滚原因", { exact: true }).fill("合成浏览器验收结束：撤销仅用于历史R1/R2展示核对的非语义定义注记，恢复原始正确模型，保持全部资产和受治理对象不变。");
    const rollback = responseFor(publisher.page, `/production-releases/${publishedId}/rollback`);
    rollbackStarted = true; record.rollbackStarted = true; save();
    await publisher.page.getByRole("button", { name: "创建回滚版本", exact: true }).click();
    const result = await receipt(`rollback-${scenario}`, await rollback);
    const restored = await getJSON(publisher.context, base + "/production-releases/" + result.releaseId);
    record.restoredRelease = restored.id;
    expect(restored.afterManifest.assets).toEqual(r1.afterManifest.assets);
    expect(restored.afterManifest.objects).toEqual(r1.afterManifest.objects);
    const current = await getJSON(publisher.context, base + "/governance/releases?limit=1");
    expect(current.items[0].id).toBe(restored.id);
    const revision = await getJSON(publisher.context, base + `/catalog/assets/${modelId}/revisions/${originalPin.revisionId}`);
    expect(revision.content).toEqual(originalRevision.content);
    record.correctBaselineRestored = true; save();
    await screenshot(publisher.page, `restored-${name}`);
  }

  try {
    await inspectSource(author.page, name);
    if (!repair) {
    await author.page.goto(modelRoute);
    await author.page.getByRole("tab", { name: "定义", exact: true }).click();
    await author.page.getByRole("button", { name: "提出修订", exact: true }).click();
    const editor = author.page.getByRole("region", { name: /知识修订工作台$/ });
    await expect(editor).toBeVisible();
    await expect(editor.getByLabel("业务定义候选值", { exact: true })).toHaveValue(originalRevision.content.definition);
    await editor.getByLabel("业务定义候选值", { exact: true }).fill(r2Definition);
    await editor.getByLabel("知识修订原因", { exact: true }).fill("合成验收：由作者确认仅增加非语义展示注记，原指标、时间窗口、结构化规格与数据绑定全部不变，供独立审核核对R1/R2。完成后回滚恢复原内容。");
    await editor.getByRole("button", { name: "运行检查", exact: true }).click();
    const writes: string[] = [];
    author.page.on("request", request => { if (request.method() === "POST" && new URL(request.url()).pathname.startsWith(base)) writes.push(new URL(request.url()).pathname); });
    const created = responseFor(author.page, "/production-operations");
    await editor.getByRole("button", { name: "保存修订并继续确认", exact: true }).click();
    const createdDraft = await receipt(`operation-create-${scenario}`, await created);
    operationId = createdDraft.operationId; record.operationId = operationId; save();
    await expect(author.page).toHaveURL(new RegExp(`/work/operations/${operationId}`));
    assertOperationRoute(author.page.url(), operationId, null);
    expect(writes).toEqual([base + "/production-operations"]);
    } else {
      record.operationId = operationId; save();
      await author.page.goto(`/work/operations/${operationId}`);
      assertOperationRoute(author.page.url(), operationId, null);
    }
    const operationPath = base + "/production-operations/" + operationId;
    const detail = await getJSON(author.context, operationPath);
    assertNaturalDraft(detail, { assetId: modelId, revisionId: originalPin.revisionId, content: originalRevision.content, definition: r2Definition, snapshots: originalOperation.input.snapshots, evidence: originalOperation.input.evidence });
    expect(detail.targets[0].proposalState).toBe("draft");
    expect(detail.targets[0].reviewIds).toEqual([]);
    const confirmations = await getJSON(author.context, operationPath + `/business-rule-confirmations?version=${detail.version}`);
    expect(confirmations.items).toEqual([]);
    record.unconfirmedDraft = true; record.proposalId = detail.targets[0].proposalId; save();
    await author.page.reload();
    await expect(author.page.getByLabel("业务定义", { exact: true })).toHaveValue(r2Definition);
    const rule = author.page.getByRole("region", { name: "业务规则确认", exact: true });
    await rule.getByLabel("业务规则声明", { exact: true }).fill("作者显式确认：本次只增加合成验收展示注记，原指标、时间窗口、结构化规格、来源覆盖与数据绑定不变；独立审核后才可发布，完成后回滚。");
    await expect(rule.getByRole("button", { name: "记录业务确认", exact: true })).toBeDisabled();
    await rule.getByRole("checkbox", { name: "确认声明支持当前定义与范围", exact: true }).check();
    const confirmed = responseFor(author.page, `/production-operations/${operationId}/business-rule-confirmations`);
    await rule.getByRole("button", { name: "记录业务确认", exact: true }).click();
    await receipt(`business-confirmation-${scenario}`, await confirmed);
    const submitted = responseFor(author.page, `/production-operations/${operationId}/submit`);
    await author.page.getByRole("button", { name: "冻结并提交验证", exact: true }).click();
    await receipt(`submit-${scenario}`, await submitted);
    await expect.poll(async () => (await getJSON(author.context, operationPath)).activeValidation.status, { timeout: 60000 }).toBe("succeeded");
    await screenshot(author.page, `submitted-${name}`);

    await reviewer.page.goto(`/work/operations/${operationId}?scope=pending`);
    const checkTab = reviewer.page.getByRole("tab", { name: "检查与确认", exact: true });
    await checkTab.focus(); await reviewer.page.keyboard.press("Enter");
    await expect(checkTab).toBeFocused();
    await expect(reviewer.page.getByText("验证通过", { exact: true })).toBeVisible();
    await reviewer.page.getByLabel("审核或重验说明", { exact: true }).fill("独立审核：仅增加明示合成浏览器验收注记，exact model/current baseline与全部spec不变，批准此非语义修订用于历史R1/R2核对。作者与审核人分离。");
    const reviewed = responseFor(reviewer.page, `/production-operations/${operationId}/reviews`);
    await reviewer.page.getByRole("button", { name: "批准整个集合", exact: true }).click();
    await receipt(`review-${scenario}`, await reviewed);
    await expect.poll(async () => (await getJSON(reviewer.context, operationPath)).summary.progress).toBe("ready_to_publish");
    await screenshot(reviewer.page, `reviewed-${name}`);

    await consumer.page.goto("/ask");
    const item = goldenCases().find((entry: { id: string }) => entry.id === (name === "desktop" ? "total" : "region"))!;
    record.modelAttemptStarted = true; record.modelCase = item.id; record.beforeAsk = counters(root); save();
    await consumer.page.getByLabel("向 Semlia 提问", { exact: true }).fill("使用已发布分析模型 demo_202609.model，" + item.question);
    const askResponse = responseFor(consumer.page, "/ask");
    await consumer.page.getByRole("button", { name: "发送问题", exact: true }).click();
    await expect(consumer.page.getByRole("button", { name: "发送问题", exact: true })).toBeDisabled();
    const asked = await receipt(`ask-${scenario}`, await askResponse);
    expect(asked.interpretation.outcome).toBe("query"); assertQueryWindow(asked.interpretation.query, item.id);
    expect(asked.resolution.plan.releaseId).toBe(r1.id);
    expect(asked.resolution.plan.model.assetId).toBe(modelId); expect(asked.resolution.plan.model.revisionId).toBe(originalPin.revisionId);
    record.agentRunId = asked.agentRun.id; record.planId = asked.resolution.plan.id; save();
    const executeResponse = responseFor(consumer.page, `/resolved-semantic-plans/${asked.resolution.plan.id}:execute`);
    await consumer.page.getByRole("button", { name: "执行只读查询", exact: true }).click();
    const executed = await receipt(`execute-${scenario}`, await executeResponse);
    const independent = independentRows(root, item.sql);
    reconcileExecution({ ...executed, columns: undefined, rows: independent }, item.expected, item.id);
    reconcileExecution(executed, independent, item.id);
    record.modelPassed = true; record.executionId = executed.run.id; record.independentRows = independent; save();
    await expect(consumer.page.getByRole("region", { name: "只读查询执行", exact: true }).getByText("已完成", { exact: true })).toBeVisible();
    await consumer.page.getByText("执行 SQL 与参数", { exact: true }).click();
    await screenshot(consumer.page, `ask-sql-${name}`);

    await publisher.page.goto(`/work/operations/${operationId}?scope=pending`);
    await publisher.page.getByRole("tab", { name: "发布记录", exact: true }).click();
    await publisher.page.getByRole("checkbox", { name: "已核对整个集合及当前验证审核", exact: true }).check();
    const publishedResponse = responseFor(publisher.page, `/production-operations/${operationId}/publish`);
    await publisher.page.getByRole("button", { name: "发布整个集合", exact: true }).click();
    await receipt(`publish-${scenario}`, await publishedResponse);
    const publishedOperation = await getJSON(publisher.context, operationPath);
    publishedId = publishedOperation.summary.releaseId; record.publishedRelease = publishedId; save();
    expect(publishedId).toBeTruthy();
    const published = await getJSON(publisher.context, base + "/production-releases/" + publishedId);
    expect(published.publishedBy).toBe(state.principals.publisher);
    expect(published.afterManifest.assets).toHaveLength(10);
    expect(published.afterManifest.assets.filter((pin: { assetId: string }) => pin.assetId !== modelId)).toEqual(r1.afterManifest.assets.filter((pin: { assetId: string }) => pin.assetId !== modelId));
    expect(published.afterManifest.objects).toEqual(r1.afterManifest.objects);
    const r2Pin = published.afterManifest.assets.find((pin: { assetId: string }) => pin.assetId === modelId);
    expect(r2Pin.revisionId).not.toBe(originalPin.revisionId);
    const r2 = await getJSON(publisher.context, base + `/catalog/assets/${modelId}/revisions/${r2Pin.revisionId}`);
    expect(r2.content).toEqual({ ...originalRevision.content, definition: r2Definition });
    expect((await getJSON(publisher.context, base + "/production-releases/" + r1.id)).afterManifest).toEqual(r1.afterManifest);
    record.publishedRevision = r2Pin.revisionId; save();
    await screenshot(publisher.page, `published-${name}`);

    const modelDefinition = asked.definitions.find((definition: { assetId: string }) => definition.assetId === modelId);
    expect(modelDefinition.revisionId).toBe(originalPin.revisionId);
    await consumer.page.locator(".answer-evidence").getByRole("button").filter({ hasText: modelDefinition.address }).click();
    await expect(consumer.page).toHaveURL(new RegExp(`/assets/${modelId}\\?`));
    expect(new URL(consumer.page.url()).searchParams.get("revision")).toBe(originalPin.revisionId);
    expect(new URL(consumer.page.url()).searchParams.get("release")).toBe(r1.id);
    const evidence = consumer.page.getByRole("region", { name: "已发布知识依据", exact: true });
    await expect(evidence).toContainText(originalRevision.content.definition);
    await expect(evidence).not.toContainText("合成浏览器验收注记");
    await expect(evidence).toContainText("支付时间");
    await expect(evidence.getByRole("button", { name: "修订当前知识", exact: true })).toBeDisabled();
    await screenshot(consumer.page, `historical-r1-${name}`);
    await consumer.page.goBack();
    await expect(consumer.page).toHaveURL(/\/ask$/);
    await consumer.page.getByRole("button", { name: "指出问题", exact: true }).click();
    await consumer.page.getByLabel("修订对象", { exact: true }).selectOption(modelId);
    await expect(consumer.page.getByRole("button", { name: "修订相关知识", exact: true })).toBeDisabled();
    await expect(consumer.page.getByText("当前身份没有提出知识修订的权限。", { exact: true })).toBeVisible();
    await expect(consumer.page.getByRole("region", { name: /知识修订工作台$/ })).toHaveCount(0);
    record.consumerCorrectionDenied = true;
    await consumer.page.goForward();
    await expect(evidence).toContainText(originalRevision.content.definition);
    await consumer.page.reload();
    await expect(evidence).toContainText(originalRevision.content.definition);
    expect(new URL(consumer.page.url()).searchParams.get("revision")).toBe(originalPin.revisionId);

    await author.page.goto(modelRoute);
    await author.page.getByRole("tab", { name: "定义", exact: true }).click();
    await author.page.getByRole("button", { name: "提出修订", exact: true }).click();
    await expect(author.page.getByLabel("业务定义候选值", { exact: true })).toHaveValue(r2Definition);
    await expect(author.page.getByLabel("业务定义候选值", { exact: true })).toBeEditable();
    record.authorCurrentBaselineEditable = true; save();
    await screenshot(author.page, `current-r2-correction-${name}`);
    await author.page.getByRole("button", { name: "取消修订", exact: true }).click();
    await rollbackPublished();
    const assets = await getJSON(author.context, base + "/catalog/assets?limit=100");
    expect(assets.items).toHaveLength(10); record.assets = assets.items.length;
    for (const actor of Object.values(actors)) { expect(actor.observed.errors).toEqual([]); expect(actor.observed.denied).toEqual([]); }
    record.passed = true; save();
  } finally {
    try { await rollbackPublished(); }
    finally {
      record.after = counters(root); record.observed = Object.fromEntries(Object.entries(actors).map(([key, value]) => [key, value.observed])); save();
      await Promise.all(Object.values(actors).map(actor => actor.context.close()));
    }
  }
});

test("@ambiguous one natural question creates no executable result", async ({ page }, info) => {
  const existing = readReceipt("natural-ambiguity.json");
  expect(existing, "Natural ambiguity already attempted; no implicit retry").toBeUndefined();
  await login(page, "consumer"); const observed = observe(page);
  await page.goto("/ask");
  const before = counters(root);
  writeReceipt("natural-ambiguity.json", { started: true, before });
  const answer = responseFor(page, "/ask");
  await page.getByLabel("向 Semlia 提问", { exact: true }).fill("最近那个指标怎么样？");
  await page.getByRole("button", { name: "发送问题", exact: true }).click();
  const result = await receipt("natural-ambiguity-response", await answer);
  const after = counters(root);
  expect(after.executions).toBe(before.executions);
  expect(result.interpretation.outcome === "clarification" || result.resolution?.refusal && !result.resolution?.plan).toBeTruthy();
  await expect(page.getByRole("button", { name: "执行只读查询", exact: true })).toHaveCount(0);
  await screenshot(page, `natural-ambiguity-${info.project.name}`);
  writeReceipt("natural-ambiguity.json", { started: true, before, after, passed: true, outcome: result.interpretation.outcome, refusalCode: result.resolution?.refusal?.code, ...observed });
  expect(observed.errors).toEqual([]); expect(observed.denied).toEqual([]);
});

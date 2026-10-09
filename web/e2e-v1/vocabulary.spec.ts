import { test, expect, type Page } from "@playwright/test";
import { login, observe, screenshot, state, writeReceipt } from "./fixtures";

const amountId = state.published["demo_202609.amount"].targetId;

async function openDefinition(page: Page, assetId: string) {
  await page.goto(`/assets/${assetId}`);
  const tab = page.getByRole("tab", { name: "定义", exact: true });
  await expect(tab).toBeVisible();
  await tab.click();
  await expect(tab).toHaveAttribute("aria-selected", "true");
}

async function assertFocusRing(page: Page, field: ReturnType<Page["getByLabel"]>) {
  await field.focus(); await page.keyboard.press("Tab"); await page.keyboard.press("Shift+Tab");
  await expect(field).toBeFocused();
  const style = await field.evaluate(element => { const css = getComputedStyle(element); return { outline: css.outlineStyle, width: css.outlineWidth, shadow: css.boxShadow }; });
  expect(style.outline !== "none" && style.width !== "0px" || style.shadow !== "none").toBe(true);
}

test("@vocabulary author reads and revises governed wording without saving", async ({ page }, info) => {
  await login(page, "author");
  const observed = observe(page);
  // Readiness counts sealed production validation and model-based implementation; evidence stays honestly missing.
  await page.goto("/assets");
  await expect(page.getByLabel("有效支付金额就绪度 5/6", { exact: true })).toBeVisible();
  await expect(page.getByLabel("下单金额就绪度 4/6", { exact: true })).toBeVisible();
  await screenshot(page, `vocabulary-readiness-${info.project.name}`);
  await openDefinition(page, amountId);
  // Synthetic data is disclosed by the published knowledge itself.
  await expect(page.getByText(/^合成演示指标/).first()).toBeVisible();
  const section = page.getByRole("region", { name: "业务词汇", exact: true });
  await expect(section).toContainText("GMV；成交额；销售额");
  await expect(section).toContainText("2026年8月的GMV是多少？");
  await screenshot(page, `vocabulary-detail-${info.project.name}`);
  await section.getByRole("button", { name: "修订业务词汇", exact: true }).click();
  const workbench = page.getByRole("region", { name: /知识修订工作台$/ });
  await expect(workbench.locator(".knowledge-field-editor").getByRole("heading", { name: "业务词汇", level: 3 })).toBeVisible();
  const aliases = workbench.getByLabel("别名（每行一项）", { exact: true });
  await expect(aliases).toHaveValue("GMV\n成交额\n销售额");
  await assertFocusRing(page, aliases);
  // The sample expectation was pinned before the model was re-pinned, so the editor offers a refresh.
  const banner = workbench.getByText(/个引用不是当前已发布版本/);
  await expect(banner).toBeVisible();
  await screenshot(page, `vocabulary-editor-stale-${info.project.name}`);
  await workbench.getByRole("button", { name: "全部刷新为当前版本", exact: true }).click();
  await expect(banner).toBeHidden();
  await expect(workbench.getByText("1 项待提交")).toBeVisible();
  await screenshot(page, `vocabulary-editor-${info.project.name}`);
  await page.emulateMedia({ reducedMotion: "reduce" });
  expect(await page.evaluate(() => matchMedia("(prefers-reduced-motion: reduce)").matches)).toBe(true);
  writeReceipt(`${process.env.SEMLIA_V1_BROWSER_RUN_ID}/vocabulary-author-${info.project.name}.json`, observed);
  expect(observed.errors).toEqual([]); expect(observed.denied).toEqual([]);
});

test("@vocabulary consumer sees how wording bound and is asked to pick a shared word", async ({ page }, info) => {
  await login(page, "consumer");
  const observed = observe(page);
  await page.goto("/ask");
  const ask = async (question: string) => {
    await page.getByLabel("向 Semlia 提问", { exact: true }).fill(question);
    const response = page.waitForResponse(item => item.url().endsWith("/ask") && item.request().method() === "POST", { timeout: 120_000 });
    await page.getByRole("button", { name: "发送问题", exact: true }).click();
    return (await response).json();
  };
  const answered = await ask("2026年8月江浙沪的成交额是多少？");
  expect(answered.interpretation.outcome).toBe("query");
  const matches = page.getByRole("region", { name: "问题中的词汇匹配", exact: true });
  await expect(matches).toContainText("江浙沪");
  await expect(matches).toContainText("当前区域 = 华东");
  await expect(matches).toContainText("成交额");
  await expect(page.locator(".answer-copy").last()).toContainText("合成演示");
  await matches.scrollIntoViewIfNeeded();
  await screenshot(page, `vocabulary-ask-matches-${info.project.name}`);
  const clarified = await ask("2026年8月销售额是多少？");
  expect(clarified.interpretation.outcome).toBe("clarification");
  expect(clarified.interpretation.clarification).toContain("下单金额");
  await expect(page.getByText(/「销售额」可能指不同的指标/)).toBeVisible();
  await screenshot(page, `vocabulary-ask-clarification-${info.project.name}`);
  writeReceipt(`${process.env.SEMLIA_V1_BROWSER_RUN_ID}/vocabulary-consumer-${info.project.name}.json`, { observed, answered: answered.vocabularyMatches, clarified: clarified.interpretation });
  expect(observed.errors).toEqual([]); expect(observed.denied).toEqual([]);
});

import { test, expect } from "@playwright/test";
import { login, observe, screenshot, readReceipt, writeReceipt, getJSON, modelId, base, root } from "./fixtures";
import { goldenCases } from "../../scripts/acceptance/v1/core.mjs";
import { assertQueryWindow, counters, independentRows, reconcileExecution } from "../../scripts/acceptance/v1/model.mjs";
import { assertResultGeometry, assertConsumerEvidencePath } from "../../scripts/acceptance/v1-browser-core.mjs";

test("@result-visibility one real result remains readable at both desktop sizes", async ({ page, context }) => {
  test.setTimeout(780000);
  const repair = process.env.SEMLIA_V1_BROWSER_REPAIR;
  expect(repair).toBe("consumer-readable-pins");
  const interrupted = readReceipt("result-visibility-check.json")!;
  expect(interrupted.modelAttemptStarted).toBeUndefined();
  const recordName = "result-visibility-check-consumer-readable-pins.json";
  expect(readReceipt(recordName), "A named visual attempt already exists; no automatic retry").toBeUndefined();
  await login(page, "consumer");
  const observed = observe(page), before = counters(root);
  const record: Record<string, unknown> = { scenario: "result-visibility-check", repair, interruptedRun: "1790434417106", synthetic: true, before, startedAt: new Date().toISOString() };
  const save = () => writeReceipt(recordName, record);
  save();
  const receipts: string[] = [];
  const stages: unknown[] = [];
  let recapture: ((phase: string) => Promise<void>) | undefined;
  page.on("request", request => { if (request.method() === "POST" && new URL(request.url()).pathname.startsWith(base)) receipts.push(new URL(request.url()).pathname); });
  try {
    const proof = readReceipt(`${process.env.SEMLIA_V1_BROWSER_RUN_ID}/verifier-head.json`)!;
    expect(proof.correctBaselineContent).toBe(true);
    expect(Date.now() - Date.parse(proof.checkedAt as string)).toBeLessThan(60000);
    const head = proof.currentRelease;
    const currentPath = base + `/catalog/assets/${modelId}`;
    const revisionPath = currentPath + `/revisions/${proof.currentRevision}`;
    assertConsumerEvidencePath(currentPath); assertConsumerEvidencePath(revisionPath);
    const current = await getJSON(context, currentPath), revision = await getJSON(context, revisionPath);
    expect(current.currentRevisionId).toBe(proof.currentRevision); expect(revision.id).toBe(proof.currentRevision); expect(revision.assetId).toBe(modelId);
    const item = goldenCases().find((entry: { id: string }) => entry.id === "total")!;
    await page.goto("/ask");
    await page.getByLabel("向 Semlia 提问", { exact: true }).fill("使用已发布分析模型 demo_202609.model，" + item.question);
    record.modelAttemptStarted = true; record.releaseId = head; save();
    const asking = page.waitForResponse(response => response.request().method() === "POST" && new URL(response.url()).pathname === base + "/ask", { timeout: 80000 });
    await page.getByRole("button", { name: "发送问题", exact: true }).click();
    const answer = await asking, asked = await answer.json();
    writeReceipt("result-visibility-check-ask.json", { status: answer.status(), idempotencyKey: answer.request().headers()["idempotency-key"], request: answer.request().postDataJSON(), body: asked });
    expect(answer.status()).toBe(200); expect(asked.interpretation.outcome).toBe("query"); assertQueryWindow(asked.interpretation.query, "total");
    expect(asked.resolution.plan.releaseId).toBe(head); expect(asked.resolution.plan.model.assetId).toBe(modelId); expect(asked.resolution.plan.model.revisionId).toBe(proof.currentRevision);
    record.agentRunId = asked.agentRun.id; record.planId = asked.resolution.plan.id; save();
    const executing = page.waitForResponse(response => response.request().method() === "POST" && new URL(response.url()).pathname === base + `/resolved-semantic-plans/${asked.resolution.plan.id}:execute`, { timeout: 80000 });
    await page.getByRole("button", { name: "执行只读查询", exact: true }).click();
    const executedResponse = await executing, executed = await executedResponse.json();
    writeReceipt("result-visibility-check-execute.json", { status: executedResponse.status(), idempotencyKey: executedResponse.request().headers()["idempotency-key"], request: executedResponse.request().postDataJSON(), body: executed });
    expect(executedResponse.status()).toBe(200);
    const independent = independentRows(root, item.sql);
    reconcileExecution({ ...executed, columns: undefined, rows: independent }, item.expected, "total"); reconcileExecution(executed, independent, "total");
    record.executionId = executed.run.id; record.numericPassed = true; save();
    const panel = page.getByRole("region", { name: "只读查询执行", exact: true });
    await expect(panel.getByText("已完成", { exact: true })).toBeVisible();
    await panel.getByText("执行 SQL 与参数", { exact: true }).click();
    await expect(panel.locator("details pre").nth(0)).toHaveText(executed.sql);
    expect(JSON.parse(await panel.locator("details pre").nth(1).innerText())).toEqual(executed.parameters ?? []);
    recapture = async (phase: string) => { for (const viewport of [{ width: 1440, height: 900, name: "desktop" }, { width: 1024, height: 768, name: "compact-desktop" }]) {
      await page.setViewportSize({ width: viewport.width, height: viewport.height });
      await page.emulateMedia({ reducedMotion: "reduce" });
      await panel.locator("details summary").focus();
      await expect(panel.locator("details summary")).toBeFocused();
      await panel.locator("details summary").scrollIntoViewIfNeeded();
      await screenshot(page, `result-sql-${phase}-${viewport.name}`);
      await panel.locator("td").scrollIntoViewIfNeeded();
      await panel.evaluate(element => {
        let current: HTMLElement | null = element as HTMLElement;
        while (current) { if (current.scrollHeight > current.clientHeight) current.scrollTop = current.scrollHeight; current = current.parentElement; }
      });
      const sample = await panel.evaluate(element => {
        const rect = (node: Element) => { const r = node.getBoundingClientRect(); return { x: r.x, y: r.y, width: r.width, height: r.height }; };
        return { viewport: { width: innerWidth, height: innerHeight }, composer: rect(document.querySelector(".ask-composer")!), cells: [...element.querySelectorAll("th,td")].map(cell => {
          const box = rect(cell);
          const points = [[box.x + 2, box.y + 2], [box.x + box.width / 2, box.y + box.height / 2], [box.x + box.width - 2, box.y + box.height - 2]];
          return { ...box, text: cell.textContent, unobscured: points.every(([x,y]) => { const hit = document.elementFromPoint(x,y); return hit === cell || cell.contains(hit); }) };
        }), scroll: { panelTop: element.scrollTop, panelHeight: element.scrollHeight, panelClient: element.clientHeight } };
      });
      stages.push({ phase, viewport: viewport.name, sample }); record.stages = stages; save();
      await screenshot(page, `result-cells-${phase}-${viewport.name}`);
      assertResultGeometry(sample);
      await expect(panel.locator("td")).toHaveText("1500");
    } };
    await recapture("initial");
    expect(receipts).toEqual([base + "/ask", base + `/resolved-semantic-plans/${asked.resolution.plan.id}:execute`]);
    expect(observed.errors).toEqual([]); expect(observed.denied).toEqual([]);
    const after = counters(root); expect(after.modelSteps - before.modelSteps).toBe(1); expect(after.executions - before.executions).toBe(1);
    record.passed = true;
  } finally {
    record.after = counters(root); record.requests = receipts; record.observed = observed; save();
    if (recapture) {
      record.contextReady = true; record.runId = process.env.SEMLIA_V1_BROWSER_RUN_ID; save();
      for (let sequence = 1; sequence <= 2; sequence++) {
        await expect.poll(() => {
          const control = readReceipt("result-visibility-context-control.json");
          return control?.runId === record.runId && control.sequence === sequence ? control.action : undefined;
        }, { timeout: 600000, intervals: [1000] }).toMatch(/^(release|recapture)$/);
        const control = readReceipt("result-visibility-context-control.json")!;
        if (control.action === "release") break;
        expect(sequence, "Only one explicitly requested read-only recapture is allowed").toBe(1);
        await recapture("review-recapture");
      }
      record.contextReleased = true; save();
    }
  }
});

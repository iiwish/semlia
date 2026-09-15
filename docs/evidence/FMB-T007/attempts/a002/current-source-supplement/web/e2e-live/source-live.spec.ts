import AxeBuilder from "@axe-core/playwright";
import { expect, test, type APIRequestContext } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { resolve } from "node:path";

const repositoryRoot = resolve(process.cwd(), "..");
const evidenceDirectory = process.env.SEMLIA_BROWSER_EVIDENCE_DIR ?? resolve(repositoryRoot, "docs/evidence/ALPHA-T004/screenshots");

test.beforeAll(() => mkdirSync(evidenceDirectory, { recursive: true }));

test("real PostgreSQL source reaches a governed proposal without fixture fallback", async ({ page, request }, testInfo) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const suffix = `${testInfo.project.name.replace(/[^a-z0-9]+/g, "_")}_${Date.now()}`;
  const schema = `alpha_${suffix}`.slice(0, 55);
  const role = `alpha_ro_${suffix}`.slice(0, 55);
  const password = `Alpha-${Date.now()}-readonly`;
  preparePostgreSQLFixture(schema, role, password);

  const workspaceResponse = await request.post("/api/v1/workspaces", {
    data: { slug: `source-${suffix.replaceAll("_", "-")}`, displayName: `来源验收 ${suffix}` },
  });
  expect(workspaceResponse.ok()).toBe(true);
  const workspace = await workspaceResponse.json() as { id: string };
  const assetResponse = await request.post(`/api/v1/workspaces/${workspace.id}/catalog/assets`, {
    headers: { "X-Semlia-Principal": "local-author" },
    data: {
      address: `alpha.${schema}_orders`,
      assetType: "entity",
      lifecycleState: "active",
      schemaVersion: "1.0.0",
      createdBy: "source-live-playwright",
      content: {
        name: `订单实体 ${suffix}`,
        definition: "组织内已登记的订单业务实体。",
        entityKeys: ["order_id"],
        grain: "order_id",
      },
    },
  });
  expect(assetResponse.ok()).toBe(true);

  await page.goto("/");
  await page.getByRole("combobox", { name: "工作区" }).selectOption(workspace.id);
  await page.getByRole("button", { name: "数据接入", exact: true }).click();
  await expect(page.getByRole("region", { name: "数据来源" })).toBeVisible();
  await expect(page.getByText("PostgreSQL Analytics")).toHaveCount(0);

  await page.getByRole("button", { name: "新建连接" }).click();
  const dialog = page.getByRole("dialog", { name: "新建 PostgreSQL 连接" });
  await dialog.getByLabel("连接名称").fill(`PostgreSQL ${schema}`);
  await dialog.getByLabel("主机").fill("postgres");
  await dialog.getByLabel("数据库").fill("semlia");
  await dialog.getByLabel("只读用户名").fill(role);
  await dialog.getByLabel("SSL 模式").selectOption("disable");
  await dialog.getByLabel("密码").fill(password);
  const sourceCreate = page.waitForResponse((response) => response.request().method() === "POST" && response.url().endsWith(`/api/v1/workspaces/${workspace.id}/sources`));
  await dialog.getByRole("button", { name: "保存连接" }).click();
  const sourceResponse = await sourceCreate;
  expect(sourceResponse.status()).toBe(201);
  const source = await sourceResponse.json() as { id: string };
  await expect(page.getByText(`PostgreSQL ${schema}`)).toBeVisible();
  await expect(page.locator("body")).not.toContainText(password);

  await page.getByRole("button", { name: `测试连接 PostgreSQL ${schema}` }).click();
  await expect(page.getByText("只读元数据连接通过")).toBeVisible();

  const runStart = page.waitForResponse((response) => response.request().method() === "POST" && response.url().endsWith(`/sources/${source.id}/discovery-runs`));
  await page.getByRole("button", { name: `启动发现 PostgreSQL ${schema}` }).click();
  const runResponse = await runStart;
  expect(runResponse.status()).toBe(202);
  const run = await runResponse.json() as { id: string };
  await waitForTerminalRun(request, workspace.id, run.id);

  const sourceContext = page.getByRole("complementary", { name: "治理上下文" });
  await sourceContext.getByRole("button", { name: /接入运行/ }).click();
  await page.getByRole("button", { name: "刷新接入结果" }).click();
  await expect(page.getByRole("button", { name: `查看运行 ${run.id}` })).toContainText(/已完成|有警告/);
  await page.getByRole("tab", { name: /语义候选/ }).click();
  const candidateRow = page.getByRole("button", { name: `查看语义候选 ${schema}.orders` });
  await expect(candidateRow).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)).toBe(false);
  const sourceAccessibility = await new AxeBuilder({ page }).include(".source-live-view").analyze();
  expect(sourceAccessibility.violations.filter((violation) => violation.impact === "serious" || violation.impact === "critical")).toEqual([]);
  await page.screenshot({ path: resolve(evidenceDirectory, `${testInfo.project.name}-source-workspace.png`), fullPage: true });
  await candidateRow.click();

  const candidateDialog = page.getByRole("dialog", { name: `${schema}.orders` });
  await expect(candidateDialog).toContainText(run.id);
  const proposalCreate = page.waitForResponse((response) => response.request().method() === "POST" && response.url().endsWith(`/api/v1/workspaces/${workspace.id}/governance/proposals`));
  const candidateDecision = page.waitForResponse((response) => response.request().method() === "POST" && response.url().includes("/semantic-candidates/") && response.url().endsWith("/decisions"));
  await candidateDialog.getByRole("button", { name: "创建并打开提案" }).click();
  const proposalResponse = await proposalCreate;
  expect(proposalResponse.status()).toBe(201);
  const proposal = await proposalResponse.json() as { id: string };
  expect((await candidateDecision).status()).toBe(201);
  await expect(page.getByRole("region", { name: new RegExp(`订单实体 ${suffix} .*候选资产版本详情`) })).toBeVisible();
  await expect(page.getByText(run.id)).toBeVisible();

  const navigation = page.getByRole("navigation", { name: "Semlia 主功能" });
  await expect(navigation.getByRole("button")).toHaveCount(5);
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)).toBe(false);
  const accessibility = await new AxeBuilder({ page }).include(".workspace").analyze();
  expect(accessibility.violations.filter((violation) => violation.impact === "serious" || violation.impact === "critical")).toEqual([]);
  await page.screenshot({ path: resolve(evidenceDirectory, `${testInfo.project.name}-source-proposal.png`), fullPage: true });
  expect(proposal.id).toMatch(/^prp_/);
});

async function waitForTerminalRun(request: APIRequestContext, workspaceId: string, runId: string) {
  await expect.poll(async () => {
    const response = await request.get(`/api/v1/workspaces/${workspaceId}/discovery-runs/${runId}`, {
      headers: { "X-Semlia-Principal": "local-author" },
    });
    if (!response.ok()) return `http-${response.status()}`;
    return (await response.json() as { status: string }).status;
  }, { timeout: 45_000, intervals: [250, 500, 1000] }).toMatch(/^(succeeded|degraded)$/);
}

test("durable file ingestion and schedule lifecycle survive reload in an empty workspace", async ({ page, request }, testInfo) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  const suffix = `${testInfo.project.name}-${Date.now()}`;
  const response = await request.post("/api/v1/workspaces", { data: { slug: `fmb-file-${suffix}`, displayName: `文件验收 ${suffix}` } });
  expect(response.ok()).toBe(true);
  const workspace = await response.json() as { id: string };
  const shots = process.env.SEMLIA_BROWSER_EVIDENCE_DIR ?? resolve(repositoryRoot, "docs/evidence/FMB-T004/screenshots");
  mkdirSync(shots, { recursive: true });
  await page.goto("/");
  await page.getByRole("combobox", { name: "工作区" }).selectOption(workspace.id);
  await page.getByRole("button", { name: "数据接入", exact: true }).click();
  await page.getByRole("button", { name: "导入工件", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "导入工件", exact: true });
  await dialog.getByLabel("来源名称").fill(`Orders CSV ${suffix}`);
  await dialog.getByLabel("上传工件").setInputFiles({ name: "orders.csv", mimeType: "text/csv", buffer: Buffer.from("order_id,amount\n1,42\n2,19\n") });
  const stageResponse = page.waitForResponse((value) => value.request().method() === "POST" && value.url().endsWith("/ingestion/artifacts"));
  const finalizeResponse = page.waitForResponse((value) => value.request().method() === "POST" && value.url().endsWith("/ingestion/artifact-sets:finalize")).catch(() => null);
  await dialog.getByRole("button", { name: "校验并保存来源" }).click();
  const staged = await stageResponse;
  expect(staged.status(), await staged.text()).toBe(201);
  const finalized = await finalizeResponse;
  if (!finalized) throw new Error("Artifact stage succeeded but source finalization did not respond");
  expect(finalized.status()).toBe(201);
  const set = await finalized.json() as { id: string; sourceId: string };
  await expect(page.getByRole("dialog", { name: "持久工件集合" })).toContainText(set.id);
  await page.keyboard.press("Escape");
  await page.reload();
  await page.getByRole("combobox", { name: "工作区" }).selectOption(workspace.id);
  await expect(page.getByRole("combobox", { name: "工作区" })).toHaveValue(workspace.id);
  await page.getByRole("button", { name: "数据接入", exact: true }).click();
  await page.getByRole("button", { name: `查看工件 Orders CSV ${suffix}` }).click();
  await expect(page.getByRole("dialog", { name: "持久工件集合" })).toContainText(set.id);
  await expect(page.getByRole("dialog", { name: "持久工件集合" })).toContainText("orders.csv");
  await page.screenshot({ path: resolve(shots, `${testInfo.project.name}-durable-artifacts.png`), fullPage: true });
  await page.keyboard.press("Escape");
  const runResponse = page.waitForResponse((value) => value.request().method() === "POST" && value.url().endsWith(`/sources/${set.sourceId}/discovery-runs`));
  await page.getByRole("button", { name: `启动发现 Orders CSV ${suffix}` }).click();
  const started = await runResponse;
  expect(started.status()).toBe(202);
  const run = await started.json() as { id: string };
  await waitForTerminalRun(request, workspace.id, run.id);
  const context = page.getByRole("complementary", { name: "治理上下文" });
  await context.getByRole("button", { name: /接入运行/ }).click();
  await page.getByRole("button", { name: "刷新接入结果" }).click();
  await expect(page.getByRole("button", { name: `查看运行 ${run.id}` })).toContainText(/已完成|有警告/);
  await page.getByRole("tab", { name: /语义候选/ }).click();
  await expect(page.getByRole("button", { name: /查看语义候选/ }).first()).toBeVisible();
  await context.getByRole("button", { name: /接入自动化/ }).click();
  await page.getByLabel("计划来源").selectOption(set.sourceId);
  await page.getByRole("button", { name: "新建计划", exact: true }).click();
  await page.getByLabel("Cron 表达式").fill("0 0 1 1 *");
  const createSchedule = page.waitForResponse((value) => value.request().method() === "POST" && value.url().endsWith(`/sources/${set.sourceId}/schedules`));
  await page.getByRole("button", { name: "保存计划" }).click();
  const scheduleResponse = await createSchedule;
  expect(scheduleResponse.status()).toBe(201);
  const schedule = await scheduleResponse.json() as { id: string };
  await page.reload();
  await page.getByRole("combobox", { name: "工作区" }).selectOption(workspace.id);
  await expect(page.getByRole("combobox", { name: "工作区" })).toHaveValue(workspace.id);
  await page.getByRole("button", { name: "数据接入", exact: true }).click();
  await context.getByRole("button", { name: /接入自动化/ }).click();
  await page.getByLabel("计划来源").selectOption(set.sourceId);
  await page.getByRole("button", { name: `暂停计划 ${schedule.id}` }).click();
  await expect(page.getByRole("button", { name: `恢复计划 ${schedule.id}` })).toBeEnabled();
  await page.getByRole("button", { name: `编辑计划 ${schedule.id}` }).click();
  await page.getByLabel("Cron 表达式").fill("0 0 2 1 *");
  await page.getByRole("button", { name: "保存计划" }).click();
  await expect(page.getByText("0 0 2 1 *", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: `恢复计划 ${schedule.id}` }).click();
  await expect(page.getByRole("button", { name: `暂停计划 ${schedule.id}` })).toBeEnabled();
  const runNow = page.waitForResponse((value) => value.request().method() === "POST" && value.url().endsWith(`/schedules/${schedule.id}:run-now`));
  await page.getByRole("button", { name: `立即运行计划 ${schedule.id}` }).click();
  const occurrenceResponse = await runNow;
  expect(occurrenceResponse.ok()).toBe(true);
  const occurrence = await occurrenceResponse.json() as { discoveryRunId: string; operationsPath: string };
  await waitForTerminalRun(request, workspace.id, occurrence.discoveryRunId);
  await expect(page.getByRole("link", { name: "打开运行" })).toHaveAttribute("href", occurrence.operationsPath);
  expect(await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1)).toBe(false);
  const accessibility = await new AxeBuilder({ page }).include(".source-live-view").analyze();
  expect(accessibility.violations.filter((violation) => violation.impact === "serious" || violation.impact === "critical")).toEqual([]);
  await page.screenshot({ path: resolve(shots, `${testInfo.project.name}-durable-schedules.png`), fullPage: true });
  await page.getByRole("button", { name: `删除计划 ${schedule.id}` }).click();
  await page.getByRole("button", { name: "确认删除计划" }).click();
  await expect(page.getByRole("button", { name: `编辑计划 ${schedule.id}` })).toHaveCount(0);
});

function preparePostgreSQLFixture(schema: string, role: string, password: string) {
  const sql = `
DO $block$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '${role}') THEN
    CREATE ROLE ${role} LOGIN PASSWORD '${password}' NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION;
  END IF;
END
$block$;
DROP SCHEMA IF EXISTS ${schema} CASCADE;
CREATE SCHEMA ${schema};
CREATE TABLE ${schema}.customers (customer_id bigint PRIMARY KEY, region text NOT NULL);
CREATE TABLE ${schema}.orders (order_id bigint PRIMARY KEY, customer_id bigint NOT NULL REFERENCES ${schema}.customers(customer_id), amount numeric(18,2) NOT NULL);
GRANT CONNECT ON DATABASE semlia TO ${role};
GRANT USAGE ON SCHEMA ${schema} TO ${role};
GRANT SELECT ON ALL TABLES IN SCHEMA ${schema} TO ${role};
`;
  execFileSync(resolve(repositoryRoot, "scripts/dev/compose.sh"), ["exec", "-T", "postgres", "psql", "-U", "semlia", "-d", "semlia", "-v", "ON_ERROR_STOP=1", "-c", sql], { cwd: repositoryRoot, stdio: "pipe" });
}

import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, type BrowserContext, type Page } from "@playwright/test";

export const root = process.env.SEMLIA_ACCEPTANCE_ROOT!;
export const owner = process.env.SEMLIA_ACCEPTANCE_OWNER!;
export const bootstrap = JSON.parse(readFileSync(join(root, "bootstrap.json"), "utf8")) as { workspaceId: string; principals: Record<string, string>; modelSettingId: string; modelConfigRevision: string; mode: string; password: string };
const sessions = new Map<string, Awaited<ReturnType<BrowserContext["cookies"]>>>();

export function sourceSQL(name: string) {
  if (!/^[a-z_]+$/.test(name) || readFileSync(join(root, ".owner"), "utf8") !== owner) throw new Error("Refusing unowned fixture path");
  const filename = `${name}.sql`;
  writeFileSync(join(root, "artifacts", filename), `CREATE TABLE public.${name}_customers (customer_id bigint NOT NULL PRIMARY KEY, name text NOT NULL);
CREATE TABLE public.${name}_orders (order_id bigint NOT NULL PRIMARY KEY, customer_id bigint NOT NULL, amount numeric NOT NULL, status text NOT NULL);
`, { flag: "wx", mode: 0o600 });
  return filename;
}

export async function login(context: BrowserContext, role: "admin" | "author" | "reviewer" | "publisher") {
  await context.clearCookies();
  const cookies = sessions.get(role);
  if (cookies) await context.addCookies(cookies);
  else {
    const page = await context.newPage();
    try {
      await page.goto("/");
      await page.getByLabel("登录账号", { exact: true }).fill(role);
      await page.getByLabel("密码", { exact: true }).fill(bootstrap.password);
      await page.getByRole("button", { name: "登录", exact: true }).click();
      await expect(page.getByLabel("Semlia 主功能")).toBeVisible();
      sessions.set(role, await context.cookies());
    } finally { await page.close(); }
  }
  const session = await context.request.get("/api/v1/session");
  expect(session.ok(), await session.text()).toBeTruthy();
  return session.json();
}

export async function addObject(page: Page, kind: string, title: string) {
  await page.getByLabel("新增对象类型").selectOption(kind);
  await page.getByRole("button", { name: "新增对象", exact: true }).click();
  await page.getByLabel("对象名称", { exact: true }).fill(title);
}

export async function addCompositeTargets(page: Page, name: string) {
  for (const [suffix, type] of [["customers", "entity"], ["revenue", "metric"]]) {
    await addObject(page, "semantic_asset", `${name} ${suffix}`);
    await page.getByLabel("资产地址", { exact: true }).fill(`acceptance.${name}_${suffix}`);
    await page.getByLabel("显示名称", { exact: true }).fill(`${name} ${suffix}`);
    await page.getByLabel("语义类型", { exact: true }).selectOption(type);
  }
  for (const suffix of ["orders", "customers", "revenue"]) {
    await addObject(page, "physical_binding", `${suffix} binding`);
    await page.getByLabel("关联语义对象", { exact: true }).selectOption({ label: `${name} ${suffix}` });
    await page.getByLabel("绑定数据集", { exact: true }).selectOption({ label: `public.${name}_${suffix === "customers" ? "customers" : "orders"}` });
    if (suffix === "revenue") {
      await page.getByLabel("绑定字段", { exact: true }).selectOption({ label: "amount" });
      await page.getByLabel("转换表达式", { exact: true }).fill("amount");
    }
  }
  for (const [suffix, field] of [["orders", "order_id"], ["customers", "customer_id"]]) {
    await addObject(page, "entity_key", `${suffix} key`);
    await page.getByLabel("关联语义对象", { exact: true }).selectOption({ label: `${name} ${suffix}` });
    await page.getByRole("checkbox", { name: `public.${name}_${suffix}.${field}`, exact: true }).check();
  }
  await addObject(page, "model_grain", "revenue grain");
  await page.getByLabel("关联语义对象", { exact: true }).selectOption({ label: `${name} revenue` });
  await page.getByLabel("粒度表达式", { exact: true }).fill("one row per order_id");
  await page.getByRole("checkbox", { name: `public.${name}_orders.order_id`, exact: true }).check();
  await addObject(page, "join_contract", "orders to customers");
  await page.getByLabel("左侧数据集", { exact: true }).selectOption({ label: `public.${name}_orders` });
  await page.getByLabel("右侧数据集", { exact: true }).selectOption({ label: `public.${name}_customers` });
  await page.getByLabel("连接表达式", { exact: true }).fill("orders.customer_id = customers.customer_id");
  await page.getByRole("button", { name: "添加字段对", exact: true }).click();
  await page.getByLabel("左字段 1", { exact: true }).selectOption({ label: "customer_id" });
  await page.getByLabel("右字段 1", { exact: true }).selectOption({ label: "customer_id" });
  await page.getByLabel("关系依据", { exact: true }).fill("Each order references one customer by customer_id.");
}

export async function confirmRules(page: Page, name: string) {
  await page.getByRole("tab", { name: "建模与依据", exact: true }).click();
  for (const suffix of ["orders", "customers", "revenue"]) {
    await page.getByRole("complementary", { name: "生产对象" }).getByRole("button", { name: new RegExp(`^${name} ${suffix}`) }).click();
    const rule = page.getByRole("region", { name: "业务规则确认", exact: true });
    await rule.getByLabel("业务规则声明", { exact: true }).fill(`I confirm the current ${suffix} definition and scope against the reviewed commerce fixture. Cancelled orders are excluded from revenue.`);
    await expect(rule.getByRole("button", { name: "记录业务确认", exact: true })).toBeDisabled();
    await rule.getByRole("checkbox", { name: "确认声明支持当前定义与范围", exact: true }).check();
    await rule.getByRole("button", { name: "记录业务确认", exact: true }).click();
    await expect(rule.getByRole("button", { name: "撤销确认", exact: true })).toBeVisible();
  }
}

export async function openAs(page: Page, role: "author" | "reviewer" | "publisher", operationId: string) {
  await login(page.context(), role);
  await page.goto(`/governance?production=${operationId}`);
  await expect(page.getByLabel("对象名称", { exact: true })).toBeVisible();
}

export async function approve(page: Page, operationId: string) {
  await openAs(page, "reviewer", operationId);
  await page.getByRole("tab", { name: "验证与审核", exact: true }).click();
  await expect(page.getByText("验证通过", { exact: true })).toBeVisible();
  await page.getByLabel("审核或重验说明", { exact: true }).fill("Independently checked every object, physical reference, and business declaration in the frozen set.");
  await page.getByRole("button", { name: "批准整个集合", exact: true }).click();
  await expect(page.getByText("集合审核已由服务器接收。", { exact: true })).toBeVisible();
}

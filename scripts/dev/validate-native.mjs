import { spawnSync, execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { randomBytes } from "node:crypto";
import { resolve } from "node:path";
import { createRequire } from "node:module";

process.umask(0o077);
const require = createRequire(resolve("web/package.json"));
const { chromium } = require("@playwright/test");
const directory = resolve(`.semlia/evidence-work/LAD-T001-native-${Date.now()}`);
mkdirSync(directory, { recursive: true, mode: 0o700 });
const password = randomBytes(24).toString("hex");
const databasePassword = randomBytes(32).toString("hex");
const secret = randomBytes(32).toString("hex");
const username = "native-validation@example.com";
const env = { ...process.env, SEMLIA_NATIVE_ENV_FILE: directory + "/runtime.env", SEMLIA_NATIVE_STATE_DIR: resolve(`.semlia/qa-${Date.now().toString(36)}`) };
let container;
let browser;
const run = (program, args, options = {}) => {
  const result = spawnSync(program, args, { env, encoding: "utf8", ...options });
  if (result.status !== 0) throw new Error(`${program} ${args[0]} failed`);
  return result.stdout;
};
try {
  container = execFileSync("docker", ["run", "-d", "--label", "semlia.validation=LAD-T001-native", "-e", "POSTGRES_PASSWORD", "-e", "POSTGRES_USER=semlia", "-e", "POSTGRES_DB=semlia", "-p", "127.0.0.1::5432", "postgres:18-alpine"], { env: { ...process.env, POSTGRES_PASSWORD: databasePassword }, encoding: "utf8" }).trim();
  for (let attempt = 0; attempt < 60; attempt++) {
    const ready = spawnSync("docker", ["exec", container, "pg_isready", "-h", "127.0.0.1", "-U", "semlia", "-d", "semlia"], { stdio: "ignore" });
    if (ready.status === 0) break;
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  const port = JSON.parse(execFileSync("docker", ["inspect", container], { encoding: "utf8" }))[0].NetworkSettings.Ports["5432/tcp"][0].HostPort;
  writeFileSync(env.SEMLIA_NATIVE_ENV_FILE, `SEMLIA_DATABASE_URL=postgresql://semlia:${databasePassword}@127.0.0.1:${port}/semlia?sslmode=disable\nSEMLIA_POSTGRES_PASSWORD=${databasePassword}\nSEMLIA_SECRET_KEY=${secret}\nSEMLIA_NATIVE_WEB_PORT=18191\nSEMLIA_NATIVE_API_PORT=18190\n`, { mode: 0o600 });
  run("go", ["build", "-o", "build/semlia", "./cmd/semlia"]);
  run("node", ["scripts/dev/native.mjs", "migrate", "up"]);
  run("node", ["scripts/dev/native.mjs", "bootstrap-local-admin", "native-validation", "Native validation", username], { input: password + "\n" });
  console.log(run("node", ["scripts/dev/native.mjs", "up"]).trim());
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("http://127.0.0.1:18191/");
  await page.getByLabel("登录账号", { exact: true }).waitFor();
  await page.screenshot({ path: directory + "/login-desktop.png" });
  await page.setViewportSize({ width: 1024, height: 768 });
  await page.screenshot({ path: directory + "/login-compact.png" });
  await page.getByLabel("登录账号", { exact: true }).fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.getByLabel("Semlia 主功能").waitFor({ timeout: 30000 });
  await page.screenshot({ path: directory + "/workspace-compact.png" });
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.screenshot({ path: directory + "/workspace-desktop.png" });
  const cookies = await context.cookies();
  const session = cookies.find((cookie) => cookie.name === "semlia_session_dev");
  if (!session?.httpOnly || session.sameSite !== "Lax") throw new Error("Session cookie attributes invalid");
  await page.getByRole("button", { name: "系统设置", exact: true }).click();
  await page.getByRole("button", { name: "创建本地账号", exact: true }).click();
  const accountDialog = page.getByRole("dialog", { name: "创建本地账号" });
  await accountDialog.getByLabel("登录账号", { exact: true }).fill("native-member@example.com");
  await accountDialog.getByLabel("姓名", { exact: true }).fill("Native member");
  await accountDialog.getByLabel("初始密码", { exact: true }).fill(randomBytes(24).toString("hex"));
  await accountDialog.getByRole("button", { name: "创建账号", exact: true }).click();
  await page.getByText("Native member", { exact: true }).waitFor();
  await page.getByRole("button", { name: "停用 Native member", exact: true }).click();
  await page.getByRole("button", { name: "确认停用", exact: true }).click();
  await page.getByText("已停用", { exact: true }).waitFor();
  await page.screenshot({ path: directory + "/members-desktop.png" });
  await page.getByLabel(`账户 ${username}`, { exact: true }).click();
  await page.getByRole("button", { name: "修改密码", exact: true }).click();
  const passwordDialog = page.getByRole("dialog", { name: "修改密码" });
  const replacement = randomBytes(24).toString("hex");
  await passwordDialog.getByLabel("当前密码", { exact: true }).fill(password);
  await passwordDialog.getByLabel("新密码", { exact: true }).fill(replacement);
  await passwordDialog.getByLabel("确认新密码", { exact: true }).fill(replacement);
  await passwordDialog.getByRole("button", { name: "修改密码", exact: true }).click();
  await page.getByLabel("登录账号", { exact: true }).waitFor();
  await page.getByLabel("登录账号", { exact: true }).fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.getByRole("alert").filter({ hasText: "账号或密码不正确" }).waitFor();
  await page.getByLabel("密码", { exact: true }).fill(replacement);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.getByLabel("Semlia 主功能").waitFor();
  if (errors.length) throw new Error("Browser runtime errors detected");
  writeFileSync(directory + "/result.json", JSON.stringify({ native: true, passwordLogin: true, memberCreation: true, memberSuspension: true, passwordChange: true, oldPasswordRejected: true, desktop: true, compactDesktop: true, cookieAttributes: true, pageErrors: errors.length }, null, 2));
  console.log(`Native login QA passed: ${directory}`);
} catch (error) {
  if (browser) {
    const page = browser.contexts()[0]?.pages()[0];
    if (page) await page.screenshot({ path: directory + "/failure.png" }).catch(() => undefined);
  }
  console.error(error.message);
  process.exitCode = 1;
} finally {
  await browser?.close();
  spawnSync("node", ["scripts/dev/native.mjs", "down"], { env, stdio: "ignore" });
  if (container) execFileSync("docker", ["rm", "-f", container], { stdio: "ignore" });
}

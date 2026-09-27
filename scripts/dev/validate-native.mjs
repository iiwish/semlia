import { spawnSync, execFileSync } from "node:child_process";
import { existsSync, lstatSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { randomBytes } from "node:crypto";
import { resolve } from "node:path";
import { createRequire } from "node:module";
import { toolEnvironment } from "./native-environment.mjs";

process.umask(0o077);
const require = createRequire(resolve("web/package.json"));
const { chromium } = require("@playwright/test");
const parent = resolve(".semlia/evidence-work");
mkdirSync(parent, { recursive: true, mode: 0o700 });
const directory = mkdtempSync(parent + "/LAD-T001-native-");
const owner = "native_" + randomBytes(16).toString("hex");
writeFileSync(directory + "/.owner", owner, { flag: "wx", mode: 0o600 });
const state = directory + "/native";
mkdirSync(state, { mode: 0o700 });
const ownedPaths = [directory, directory + "/.owner", state].map(path => ({ path, identity: lstatSync(path) }));
const password = randomBytes(24).toString("hex");
const databasePassword = randomBytes(32).toString("hex");
const secret = randomBytes(32).toString("hex");
const username = "native-validation@example.com";
const base = toolEnvironment();
const env = { ...base, SEMLIA_NATIVE_ENV_FILE: directory + "/runtime.env", SEMLIA_NATIVE_STATE_DIR: state };
const dockerConfig = directory + "/docker";
mkdirSync(dockerConfig, { mode: 0o700 });
writeFileSync(dockerConfig + "/config.json", JSON.stringify({ auths: { "https://index.docker.io/v1/": {} }, cliPluginsExtraDirs: [base.HOME + "/.docker/cli-plugins"] }), { mode: 0o600 });
let dockerHost;
let container;
let browser;
let nativeStarted = false;
const docker = (args, extra = {}) => {
  try { return execFileSync("docker", ["--config", dockerConfig, "--host", dockerHost, ...args], { env: { ...base, ...extra }, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim(); }
  catch { throw new Error("Isolated Docker command failed."); }
};
const verifyPaths = paths => {
  for (const { path, identity } of paths) {
    const current = lstatSync(path);
    if (current.isSymbolicLink() || current.dev !== identity.dev || current.ino !== identity.ino || current.uid !== identity.uid || current.mode !== identity.mode) throw new Error("Native validation path identity changed; cleanup refused.");
  }
};
const verifyEvidenceRoot = () => {
  verifyPaths(ownedPaths.slice(0, 2));
  const info = lstatSync(directory), marker = lstatSync(directory + "/.owner");
  if (!info.isDirectory() || info.isSymbolicLink() || info.uid !== process.getuid() || (info.mode & 0o777) !== 0o700 || !marker.isFile() || marker.isSymbolicLink() || marker.uid !== process.getuid() || (marker.mode & 0o777) !== 0o600 || readFileSync(directory + "/.owner", "utf8") !== owner) throw new Error("Native validation ownership mismatch; cleanup refused.");
};
const writeEvidence = (name, content) => {
  verifyEvidenceRoot();
  writeFileSync(directory + "/" + name, content, { flag: "wx", mode: 0o600 });
};
const screenshot = (page, name) => {
  verifyEvidenceRoot();
  if (existsSync(directory + "/" + name)) throw new Error("Native validation screenshot already exists.");
  return page.screenshot({ path: directory + "/" + name });
};
const verifyOwnership = () => {
  verifyEvidenceRoot();
  verifyPaths(ownedPaths.slice(2));
  if (container && (!/^[a-f0-9]{64}$/.test(container) || docker(["container", "inspect", "--format", '{{.Id}}|{{ index .Config.Labels "io.semlia.acceptance.owner" }}', container]) !== `${container}|${owner}`)) throw new Error("Native validation container ownership mismatch; cleanup refused.");
};
const run = (program, args, options = {}) => {
  const result = spawnSync(program, args, { env, encoding: "utf8", ...options });
  if (result.status !== 0) throw new Error(`${program} ${args[0]} failed`);
  return result.stdout;
};
try {
  dockerHost = execFileSync("docker", ["context", "inspect", "--format", "{{.Endpoints.docker.Host}}"], { env: base, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
  if (!/^unix:\/\/\/[^\r\n]+$/.test(dockerHost)) throw new Error("Native validation requires a local Unix Docker endpoint.");
  container = docker(["run", "-d", "--label", "semlia.validation=LAD-T001-native", "--label", `io.semlia.acceptance.owner=${owner}`, "-e", "POSTGRES_PASSWORD", "-e", "POSTGRES_USER=semlia", "-e", "POSTGRES_DB=semlia", "-p", "127.0.0.1::5432", "postgres:18-alpine"], { POSTGRES_PASSWORD: databasePassword });
  verifyOwnership();
  let postgresReady = false;
  for (let attempt = 0; attempt < 60; attempt++) {
    try { docker(["exec", container, "pg_isready", "-h", "127.0.0.1", "-U", "semlia", "-d", "semlia"]); postgresReady = true; break; } catch { /* Wait only for this owned container. */ }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  if (!postgresReady) throw new Error("Isolated PostgreSQL did not become ready.");
  const port = docker(["container", "inspect", "--format", '{{ (index (index .NetworkSettings.Ports "5432/tcp") 0).HostPort }}', container]);
  if (!/^[0-9]{1,5}$/.test(port) || Number(port) < 1 || Number(port) > 65535) throw new Error("Invalid isolated PostgreSQL port.");
  writeFileSync(env.SEMLIA_NATIVE_ENV_FILE, `SEMLIA_DATABASE_URL=postgresql://semlia:${databasePassword}@127.0.0.1:${port}/semlia?sslmode=disable\nSEMLIA_POSTGRES_PASSWORD=${databasePassword}\nSEMLIA_SECRET_KEY=${secret}\nSEMLIA_NATIVE_WEB_PORT=18191\nSEMLIA_NATIVE_API_PORT=18190\n`, { mode: 0o600 });
  run("go", ["build", "-o", "build/semlia", "./cmd/semlia"]);
  run("node", ["scripts/dev/native.mjs", "migrate", "up"]);
  run("node", ["scripts/dev/native.mjs", "bootstrap-local-admin", "native-validation", "Native validation", username], { input: password + "\n" });
  nativeStarted = true;
  console.log(run("node", ["scripts/dev/native.mjs", "up"]).trim());
  verifyOwnership();
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("http://127.0.0.1:18191/");
  await page.getByLabel("登录账号", { exact: true }).waitFor();
  await screenshot(page, "login-desktop.png");
  await page.setViewportSize({ width: 1024, height: 768 });
  await screenshot(page, "login-compact.png");
  await page.getByLabel("登录账号", { exact: true }).fill(username);
  await page.getByLabel("密码", { exact: true }).fill(password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.getByLabel("Semlia 主功能").waitFor({ timeout: 30000 });
  await screenshot(page, "workspace-compact.png");
  await page.setViewportSize({ width: 1440, height: 900 });
  await screenshot(page, "workspace-desktop.png");
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
  await screenshot(page, "members-desktop.png");
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
  writeEvidence("result.json", JSON.stringify({ native: true, passwordLogin: true, memberCreation: true, memberSuspension: true, passwordChange: true, oldPasswordRejected: true, desktop: true, compactDesktop: true, cookieAttributes: true, pageErrors: errors.length }, null, 2));
  console.log(`Native login QA passed: ${directory}`);
} catch (error) {
  if (browser) {
    const page = browser.contexts()[0]?.pages()[0];
    if (page) { try { await screenshot(page, "failure.png"); } catch { /* Untrusted paths must not receive artifacts. */ } }
  }
  try { writeEvidence("failure.log", String(error.stack ?? error)); } catch { /* Preserve an untrusted path without writes. */ }
  console.error("Native browser validation failed; protected evidence retained.");
  process.exitCode = 1;
} finally {
  let cleanupFailed = false;
  try { await browser?.close(); } catch { cleanupFailed = true; }
  let owned = false;
  try { verifyOwnership(); owned = true; } catch { cleanupFailed = true; }
  if (owned) {
    if (nativeStarted) {
      const result = spawnSync("node", ["scripts/dev/native.mjs", "down"], { env, stdio: "ignore" });
      if (result.error || result.status !== 0 || existsSync(state + "/owner.json")) cleanupFailed = true;
    }
    if (container) {
      try {
        verifyOwnership();
        docker(["rm", "--force", "--volumes", container]);
        if (docker(["container", "ls", "--all", "--quiet", "--filter", `id=${container}`]) !== "") cleanupFailed = true;
      } catch { cleanupFailed = true; }
    }
  }
  try { writeEvidence("cleanup.json", JSON.stringify({ owned, cleanupFailed })); } catch { cleanupFailed = true; }
  if (cleanupFailed) { console.error("Native validation cleanup failed; protected evidence retained."); process.exitCode = 1; }
}

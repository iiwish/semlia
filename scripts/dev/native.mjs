import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, openSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createServer, request } from "node:http";
import { createServer as createTCPServer } from "node:net";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseEnv } from "node:util";
import { randomBytes } from "node:crypto";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const state = resolve(process.env.SEMLIA_NATIVE_STATE_DIR ?? join(root, ".semlia/native"));
if (!state.startsWith(join(root, ".semlia") + "/")) throw new Error("Native state must stay in this workspace.");
const controlPath = join(state, "control.sock");
const lockPath = join(state, "owner.json");
const command = process.argv[2] ?? "up";
mkdirSync(state, { recursive: true, mode: 0o700 });

function environment() {
  const read = (path) => existsSync(path) ? parseEnv(readFileSync(path, "utf8")) : {};
  const local = { ...read(join(root, ".semlia/dev.env")), ...read(process.env.SEMLIA_NATIVE_ENV_FILE ?? join(root, ".semlia/native.env")) };
  if (!local.SEMLIA_SECRET_KEY || !local.SEMLIA_POSTGRES_PASSWORD) throw new Error("Run scripts/dev/ensure-env.sh and configure the existing PostgreSQL first.");
  const port = Number(local.SEMLIA_NATIVE_WEB_PORT ?? local.SEMLIA_HTTP_PORT ?? 18081);
  const apiPort = Number(local.SEMLIA_NATIVE_API_PORT ?? 18080);
  for (const value of [port, apiPort]) if (!Number.isInteger(value) || value < 1024 || value > 65535) throw new Error("Invalid native port.");
  if (port === apiPort) throw new Error("Web and API ports must differ.");
  const db = new URL(local.SEMLIA_DATABASE_URL ?? "postgresql://127.0.0.1/semlia");
  if (!local.SEMLIA_DATABASE_URL) { db.username = "semlia"; db.password = local.SEMLIA_POSTGRES_PASSWORD; db.port = local.SEMLIA_POSTGRES_PORT ?? "5433"; db.searchParams.set("sslmode", "disable"); }
  const content = join(state, "content");
  const artifacts = join(state, "artifacts");
  mkdirSync(content, { recursive: true, mode: 0o700 });
  mkdirSync(artifacts, { recursive: true, mode: 0o700 });
  return { ...process.env, ...local, SEMLIA_ENV: "development", SEMLIA_AUTH_MODE: "password", SEMLIA_LOCAL_UAT_IDENTITIES: "false", SEMLIA_HTTP_ADDR: `127.0.0.1:${apiPort}`, SEMLIA_DATABASE_URL: db.href, SEMLIA_ALLOWED_ORIGINS: `http://127.0.0.1:${port}`, SEMLIA_GIT_REPOSITORY: content, SEMLIA_SOURCE_ARTIFACT_ROOT: content, SEMLIA_ARTIFACT_ROOT: artifacts, SEMLIA_ARTIFACT_STORE: "local", SEMLIA_WORKER_CONFIGURED: "true", SEMLIA_MIGRATIONS_PATH: join(root, "migrations"), SEMLIA_NATIVE_WEB_PORT: String(port), SEMLIA_NATIVE_API_PORT: String(apiPort), SEMLIA_VITE_API_TARGET: `http://127.0.0.1:${apiPort}`, VITE_CATALOG_FIXTURE: "", VITE_LOCAL_UAT_IDENTITIES: "" };
}

function run(program, args, env, stdio = "inherit") {
  const result = spawnSync(program, args, { cwd: root, env, stdio });
  if (result.error || result.status !== 0) throw new Error(`${program} command failed.`);
}

function control(action) {
  const owner = JSON.parse(readFileSync(lockPath, "utf8"));
  return new Promise((resolve, reject) => {
    const req = request({ socketPath: controlPath, path: `/${action}`, method: "POST", headers: { authorization: owner.token }, timeout: 2000 }, (res) => {
      res.resume();
      res.on("end", () => res.statusCode === 200 ? resolve() : reject(new Error("Native control refused request.")));
    });
    req.on("error", reject);
    req.on("timeout", () => req.destroy(new Error("Native control timed out.")));
    req.end();
  });
}

async function portAvailable(port) {
  await new Promise((resolve, reject) => {
    const server = createTCPServer();
    server.once("error", reject);
    server.listen(Number(port), "127.0.0.1", () => server.close(resolve));
  });
}

async function supervise() {
  const env = environment();
  const token = randomBytes(32).toString("hex");
  writeFileSync(lockPath, JSON.stringify({ pid: process.pid, token }), { flag: "wx", mode: 0o600 });
  const children = [];
  const live = new Set();
  let stopping = false;
  let timer;
  const server = createServer((req, res) => {
    if (req.headers.authorization !== token || req.method !== "POST") { res.writeHead(403).end(); return; }
    if (req.url === "/status") { res.end("running"); return; }
    if (req.url !== "/stop") { res.writeHead(404).end(); return; }
    res.end("stopping");
    shutdown();
  });
  const cleanup = () => {
    clearTimeout(timer);
    server.close();
    rmSync(controlPath, { force: true });
    rmSync(lockPath, { force: true });
  };
  const shutdown = () => {
    if (stopping) return;
    stopping = true;
    for (const child of live) { try { process.kill(-child.pid, "SIGTERM"); } catch { /* Already exited. */ } }
    timer = setTimeout(() => {
      for (const child of live) { try { process.kill(-child.pid, "SIGKILL"); } catch { /* Already exited. */ } }
    }, 12000);
    if (!live.size) cleanup();
  };
  process.on("SIGTERM", shutdown);
  process.on("SIGINT", shutdown);
  try {
    await portAvailable(env.SEMLIA_NATIVE_WEB_PORT);
    await portAvailable(env.SEMLIA_NATIVE_API_PORT);
    await new Promise((resolve, reject) => { server.once("error", reject); server.listen(controlPath, resolve); });
    const launch = (program, args, name) => {
      const log = openSync(join(state, `${name}.log`), "a", 0o600);
      const child = spawn(program, args, { cwd: root, env, detached: true, stdio: ["ignore", log, log] });
      children.push(child);
      live.add(child);
      child.on("error", shutdown);
      child.on("close", () => {
        live.delete(child);
        shutdown();
        if (!live.size) cleanup();
      });
    };
    launch(join(root, "build/semlia"), ["server"], "server");
    launch(join(root, "build/semlia"), ["worker"], "worker");
    launch("pnpm", ["--dir", "web", "exec", "vite", "--host", "127.0.0.1", "--port", env.SEMLIA_NATIVE_WEB_PORT, "--strictPort"], "web");
  } catch (error) { shutdown(); throw error; }
}

try {
  if (command === "supervise") {
    await supervise();
  } else if (command === "status") {
    await control("status");
    const env = environment();
    const response = await fetch(`http://127.0.0.1:${env.SEMLIA_NATIVE_WEB_PORT}/health/ready`, { signal: AbortSignal.timeout(3000) });
    if (!response.ok) throw new Error("Native readiness check failed.");
    console.log("Native supervisor, Web proxy and API readiness passed.");
  } else if (command === "down") {
    if (existsSync(lockPath)) {
      await control("stop");
      for (let attempt = 0; existsSync(lockPath) && attempt < 160; attempt++) await new Promise((resolve) => setTimeout(resolve, 100));
      if (existsSync(lockPath)) throw new Error("Native processes did not finish shutdown.");
    }
    console.log("Native services stopped; PostgreSQL and data preserved.");
  } else if (command === "up") {
    if (existsSync(lockPath)) { await control("status"); throw new Error("Native services already running. Use make dev-down first."); }
    const env = environment();
    await portAvailable(env.SEMLIA_NATIVE_WEB_PORT);
    await portAvailable(env.SEMLIA_NATIVE_API_PORT);
    run("go", ["build", "-o", "build/semlia", "./cmd/semlia"], env);
    run(join(root, "build/semlia"), ["doctor"], env);
    run(join(root, "build/semlia"), ["check-schema"], env);
    const log = openSync(join(state, "supervisor.log"), "a", 0o600);
    const child = spawn(process.execPath, [fileURLToPath(import.meta.url), "supervise"], { cwd: root, detached: true, stdio: ["ignore", log, log] });
    child.unref();
    let ready = false;
    for (let attempt = 0; attempt < 120; attempt++) {
      await new Promise((resolve) => setTimeout(resolve, 500));
      try {
        const response = await fetch(`http://127.0.0.1:${env.SEMLIA_NATIVE_WEB_PORT}/health/ready`, { signal: AbortSignal.timeout(1000) });
        if (response.ok) { await control("status"); ready = true; break; }
      } catch { /* Wait for startup without exposing configuration. */ }
      if (attempt > 5 && !existsSync(lockPath)) break;
    }
    if (!ready) { if (existsSync(lockPath)) await control("stop"); throw new Error("Native startup failed; inspect .semlia/native logs. No migrations were applied."); }
    console.log(`Semlia: http://127.0.0.1:${env.SEMLIA_NATIVE_WEB_PORT}/`);
  } else if (["migrate", "bootstrap-local-admin", "reset-local-password"].includes(command)) {
    run(join(root, "build/semlia"), process.argv.slice(2), environment());
  } else throw new Error("Usage: native.mjs up|down|migrate|bootstrap-local-admin|reset-local-password");
} catch (error) {
  // Configuration values and child environments must not reach diagnostics.
  console.error(error.code === "EADDRINUSE" ? "A required native port is occupied; nothing was stopped." : error.code === "EEXIST" ? "Native ownership record exists; refusing to overwrite it." : error.message);
  process.exitCode = 1;
}

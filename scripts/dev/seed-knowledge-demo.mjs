import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, openSync, closeSync, readFileSync, writeFileSync, renameSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { parseEnv } from "node:util";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const stateDir = join(root, ".semlia/demo-202609");
const nativePath = join(root, ".semlia/native.env");
const readEnv = (path) => existsSync(path) ? parseEnv(readFileSync(path, "utf8")) : {};

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd: root, encoding: "utf8", ...options });
  if (result.error || result.status !== 0) throw new Error(`${command} failed: ${result.stderr ?? "see command output"}`);
  return result.stdout;
}

if (process.argv[2] !== "--apply") {
  console.log("Usage: node scripts/dev/seed-knowledge-demo.mjs --apply");
  console.log("Adds an isolated synthetic workspace and source database; requires the local native worker.");
  process.exit(0);
}

const env = { ...readEnv(join(root, ".semlia/dev.env")), ...readEnv(nativePath) };
if (!env.SEMLIA_POSTGRES_PASSWORD) throw new Error("Missing local development database settings.");
const database = new URL(env.SEMLIA_DATABASE_URL ?? "postgresql://127.0.0.1:5433/semlia?sslmode=disable");
if (!env.SEMLIA_DATABASE_URL) {
  database.username = "semlia";
  database.password = env.SEMLIA_POSTGRES_PASSWORD;
  database.port = env.SEMLIA_POSTGRES_PORT ?? "5433";
}
if (database.hostname !== "127.0.0.1" || database.port !== "5433" || database.pathname !== "/semlia") throw new Error("Refusing a non-local or non-standard database.");
const container = JSON.parse(run("docker", ["inspect", "semlia-local-postgres-1"]))[0];
const labels = container.Config.Labels;
if (labels["com.docker.compose.project"] !== "semlia-local" || resolve(labels["com.docker.compose.project.working_dir"]) !== root) throw new Error("Local PostgreSQL ownership mismatch.");
run("node", ["scripts/dev/native.mjs", "status"], { stdio: "inherit" });
mkdirSync(stateDir, { recursive: true, mode: 0o700 });
const backup = join(stateDir, "before-demo.dump");
if (!existsSync(backup)) {
  const temporary = `${backup}.${process.pid}.tmp`;
  const fd = openSync(temporary, "wx", 0o600);
  try {
    run("docker", ["exec", "semlia-local-postgres-1", "pg_dump", "-U", "semlia", "-d", "semlia", "-Fc"], { stdio: ["ignore", fd, "pipe"] });
    renameSync(temporary, backup);
  } finally { closeSync(fd); rmSync(temporary, { force: true }); }
}
run("go", ["run", "./cmd/demo-seed", "--apply"], { env: { ...process.env, ...env, SEMLIA_ENV: "development", SEMLIA_DATABASE_URL: database.href }, stdio: "inherit" });
const receipt = JSON.parse(readFileSync(join(stateDir, "state.json"), "utf8"));
const demo = JSON.parse(readFileSync(join(stateDir, "demo.json"), "utf8"));
const references = JSON.parse(env.SEMLIA_EXECUTION_SOURCES || "[]");
const entry = { workspaceId: demo.workspaceId, sourceId: demo.sourceId, dsnEnv: demo.dsnEnv };
const existing = references.find((item) => item.workspaceId === entry.workspaceId && item.sourceId === entry.sourceId);
if (existing && JSON.stringify(existing) !== JSON.stringify(entry)) throw new Error("Existing demo execution mapping differs; refusing overwrite.");
if (!existing) references.push(entry);
const dsn = new URL(database.href);
dsn.pathname = "/semlia_demo_202609";
dsn.username = "semlia_demo_202609";
dsn.password = receipt.Password;
if (env[demo.dsnEnv] && env[demo.dsnEnv] !== dsn.href) throw new Error("Existing demo DSN differs; refusing overwrite.");
let native = existsSync(nativePath) ? readFileSync(nativePath, "utf8") : "";
// Append dotenv overrides while preserving every unrelated setting and comment.
if (env.SEMLIA_EXECUTION_SOURCES !== JSON.stringify(references) || env[demo.dsnEnv] !== dsn.href) {
  native += `\n# Isolated synthetic commerce demo, 2026-09.\nSEMLIA_EXECUTION_SOURCES='${JSON.stringify(references)}'\n${demo.dsnEnv}='${dsn.href}'\n`;
  writeFileSync(nativePath, native, { mode: 0o600 });
  console.log("Demo execution settings added. Restart native services to load them.");
}
console.log(`Demo workspace: ${demo.workspaceId}. Existing accounts and workspace data are unchanged.`);

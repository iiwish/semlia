import fs from "node:fs";
import cp from "node:child_process";
import crypto from "node:crypto";
import { parseEnv } from "node:util";
import { resolve } from "node:path";

process.umask(0o077);
const directory = resolve(process.argv[2] ?? "");
if (!directory.startsWith(resolve(".semlia/backups") + "/")) throw new Error("Select a Semlia backup directory.");
const inventory = JSON.parse(fs.readFileSync(directory + "/containers.json"));
const image = inventory.find((item) => item.Name === "/semlia-local-postgres-1")?.Image;
if (!/^sha256:[a-f0-9]{64}$/.test(image ?? "")) throw new Error("Backup lacks a pinned PostgreSQL image.");
const password = crypto.randomBytes(32).toString("hex");
const docker = (args, options = {}) => cp.execFileSync("docker", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"], ...options });
let id;
let proof;
try {
  id = docker(["run", "-d", "--label", "semlia.validation=LAD-T001-restore", "-e", "POSTGRES_PASSWORD", "-e", "POSTGRES_USER=semlia", "-e", "POSTGRES_DB=semlia", "-p", "127.0.0.1::5432", image], { env: { ...process.env, POSTGRES_PASSWORD: password } }).trim();
  for (let attempt = 0; attempt < 60; attempt++) {
    try { docker(["exec", id, "pg_isready", "-h", "127.0.0.1", "-U", "semlia", "-d", "semlia"]); break; }
    catch { await new Promise((resolve) => setTimeout(resolve, 500)); }
  }
  const fd = fs.openSync(directory + "/database.dump", "r");
  const restored = cp.spawnSync("docker", ["exec", "-i", id, "pg_restore", "-U", "semlia", "-d", "semlia", "--no-owner", "--no-acl", "--exit-on-error"], { stdio: [fd, "pipe", "pipe"] });
  fs.closeSync(fd);
  fs.writeFileSync(directory + "/restore.log", restored.stderr ?? "", { mode: 0o600 });
  if (restored.status !== 0) throw new Error("Restore failed; inspect the private restore.log.");
  const query = (sql) => docker(["exec", id, "psql", "-U", "semlia", "-d", "semlia", "-Atc", sql]).trim();
  const before = query("SELECT version,dirty FROM schema_migrations");
  const info = JSON.parse(docker(["inspect", id]))[0];
  const port = info.NetworkSettings.Ports["5432/tcp"][0].HostPort;
  const local = parseEnv(fs.readFileSync(directory + "/dev.env", "utf8"));
  const env = { ...process.env, SEMLIA_ENV: "development", SEMLIA_AUTH_MODE: "password", SEMLIA_DATABASE_URL: `postgresql://semlia:${password}@127.0.0.1:${port}/semlia?sslmode=disable`, SEMLIA_SECRET_KEY: local.SEMLIA_SECRET_KEY, SEMLIA_ALLOWED_ORIGINS: "http://127.0.0.1:18081" };
  const migrated = cp.spawnSync("./build/semlia", ["migrate", "up"], { env, encoding: "utf8" });
  fs.writeFileSync(directory + "/migration.log", migrated.stdout + migrated.stderr, { mode: 0o600 });
  if (migrated.status !== 0) throw new Error("Isolated migration failed; inspect the private migration.log.");
  proof = { restored: true, before, after: query("SELECT version,dirty FROM schema_migrations"), aclPolicy: "Source role ACLs are excluded in the isolated restore; the live cluster roles are unchanged.", liveDatabaseChanged: false };
} catch (error) {
  console.error(error.message.startsWith("Command failed") ? "Isolated verification command failed." : error.message);
  process.exitCode = 1;
} finally {
  if (id) docker(["rm", "-f", id]);
}
if (proof) {
  proof.temporaryContainerRemoved = true;
  fs.writeFileSync(directory + "/restore-proof.json", JSON.stringify(proof, null, 2), { mode: 0o600 });
  console.log(JSON.stringify(proof));
}

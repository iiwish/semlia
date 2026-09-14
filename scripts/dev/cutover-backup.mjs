import fs from "node:fs";
import cp from "node:child_process";
import crypto from "node:crypto";

process.umask(0o077);
const names = ["semlia-local-server-1", "semlia-local-worker-1", "semlia-local-postgres-1"];
const docker = (args) => cp.execFileSync("docker", args, { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
const inventory = JSON.parse(docker(["inspect", ...names]));
for (const item of inventory) {
  if (item.Config.Labels["com.docker.compose.project"] !== "semlia-local") throw new Error("Unexpected container ownership.");
}
const directory = `.semlia/backups/LAD-T001-cutover-${Date.now()}`;
fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
fs.writeFileSync(`${directory}/containers.json`, JSON.stringify(inventory));
fs.copyFileSync(".semlia/dev.env", `${directory}/dev.env`);
fs.chmodSync(`${directory}/dev.env`, 0o600);
const pg = inventory[2];
const query = (sql) => docker(["exec", pg.Id, "psql", "-U", "semlia", "-d", "semlia", "-Atc", sql]).trim();
const fingerprints = () => Object.fromEntries(["workspaces", "semantic_assets", "model_providers", "model_settings"].map(table => [table, query(`SELECT count(*) || ':' || md5(coalesce(string_agg(row_to_json(t)::text, '' ORDER BY id),'')) FROM ${table} t`)]));
docker(["stop", "--time", "30", inventory[0].Id, inventory[1].Id]);
const before = fingerprints();
const save = (name, args) => {
  const fd = fs.openSync(`${directory}/${name}`, "wx", 0o600);
  try { cp.execFileSync("docker", args, { stdio: ["ignore", fd, "pipe"] }); }
  finally { fs.closeSync(fd); }
};
save("database.dump", ["exec", pg.Id, "pg_dump", "-U", "semlia", "-d", "semlia", "-Fc"]);
for (const [name, volume] of [["content.tar", "semlia-local_git-content"], ["artifacts.tar", "semlia-local_artifact-data"]]) {
  save(name, ["run", "--rm", "--network", "none", "--label", "semlia.validation=LAD-T001-backup", "--mount", `type=volume,source=${volume},target=/snapshot,readonly`, "--entrypoint", "tar", pg.Image, "-C", "/snapshot", "-cf", "-", "."]);
}
const manifest = { writersStopped: true, schema: query("SELECT version,dirty FROM schema_migrations"), fingerprints: before, files: {} };
for (const name of ["database.dump", "content.tar", "artifacts.tar", "containers.json", "dev.env"]) {
  manifest.files[name] = { bytes: fs.statSync(`${directory}/${name}`).size, sha256: crypto.createHash("sha256").update(fs.readFileSync(`${directory}/${name}`)).digest("hex") };
}
fs.writeFileSync(`${directory}/manifest.json`, JSON.stringify(manifest, null, 2));
console.log(directory);

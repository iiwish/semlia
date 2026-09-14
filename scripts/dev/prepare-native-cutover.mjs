import fs from "node:fs";
import cp from "node:child_process";
import crypto from "node:crypto";
import { resolve } from "node:path";
import { parseEnv } from "node:util";

process.umask(0o077);
const directory = resolve(process.argv[2] ?? "");
if (!directory.startsWith(resolve(".semlia/backups") + "/")) throw new Error("Select a Semlia backup.");
const manifest = JSON.parse(fs.readFileSync(`${directory}/manifest.json`));
const proof = JSON.parse(fs.readFileSync(`${directory}/restore-proof.json`));
if (!manifest.writersStopped || !proof.restored || proof.after !== "29|f") throw new Error("Verified consistent backup required.");
for (const [name, record] of Object.entries(manifest.files)) {
  if (crypto.createHash("sha256").update(fs.readFileSync(`${directory}/${name}`)).digest("hex") !== record.sha256) throw new Error("Backup digest mismatch.");
}
const inventory = JSON.parse(fs.readFileSync(`${directory}/containers.json`));
const local = parseEnv(fs.readFileSync(`${directory}/dev.env`, "utf8"));
const preserved = {};
for (const item of inventory.filter(item => !item.Name.includes("postgres"))) {
  const env = Object.fromEntries(item.Config.Env.map(value => { const index = value.indexOf("="); return [value.slice(0,index), value.slice(index+1)]; }));
  if (env.SEMLIA_SECRET_KEY !== local.SEMLIA_SECRET_KEY) throw new Error("Application encryption key differs from local configuration.");
  for (const [key,value] of Object.entries(env)) {
    if (!key.startsWith("SEMLIA_EMBEDDING_")) continue;
    if (key in preserved && preserved[key] !== value) throw new Error("Provider environment differs between server and worker.");
    preserved[key] = value;
  }
}
if (fs.existsSync(".semlia/native.env")) throw new Error("Existing native configuration requires review.");
for (const [archive,target] of [["content.tar","content"],["artifacts.tar","artifacts"]]) {
  const path = `.semlia/native/${target}`;
  fs.mkdirSync(path,{recursive:true,mode:0o700});
  if (fs.readdirSync(path).length) throw new Error("Native data directory is not empty.");
  cp.execFileSync("tar",["-xf",`${directory}/${archive}`,"-C",path]);
}
fs.writeFileSync(".semlia/native.env", Object.entries(preserved).map(([key,value]) => `${key}=${JSON.stringify(value)}`).join("\n")+"\n",{flag:"wx",mode:0o600});
console.log("Content, artifacts and provider credentials preserved in private native configuration.");

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";

// The executable fixture never connects to Docker or PostgreSQL.
const dockerFixture = `#!${process.execPath}
import { readFileSync, writeFileSync } from "node:fs";
const args = process.argv.slice(2);
if (args[0] === "inspect") {
  console.log(JSON.stringify({ "com.docker.compose.project": "semlia-local", "com.docker.compose.project.working_dir": process.cwd() }));
} else if (args.includes("pg_dump")) {
  console.log("fixture dump");
} else if (args.includes("psql")) {
  const sql = readFileSync(0, "utf8");
  if (sql.includes("tablename AS name")) console.log(JSON.stringify([
    "semantic_assets", "semantic_candidates", "production_operations", "releases", "join_contracts", "ontology_revisions", "review_batches", "agent_runs", "embedding_index_versions", "semantic_queries",
    "source_connections", "consumer_bindings", "jobs", "runtime_runs", "runtime_run_events", "attention_items", "outbox_events", "webhook_deliveries", "webhook_fanout_receipts"
  ].map(name => ({ name }))));
  else if (sql.includes("AS child")) console.log("[]");
  else if (sql.includes("pg_get_constraintdef")) console.log(JSON.stringify([{ name: "consumer_bindings_workspace_id_release_id_fkey", definition: "FOREIGN KEY (workspace_id, release_id) REFERENCES releases(workspace_id, id)" }]));
  else if (sql.includes("BEGIN;")) {
    writeFileSync("transaction.sql", sql);
    if (process.env.FAIL_TRANSACTION === "1") { console.error("fixture preservation mismatch"); process.exit(1); }
  }
  else if (sql.includes("pg_trigger")) console.log(JSON.stringify([{ state: process.env.TRIGGER_STATE || "O" }]));
  else if (sql.includes("pg_stat_activity")) console.log(process.env.CONNECTED_CLIENTS || "0");
  else if (sql.startsWith("SELECT count(*) FROM consumer_bindings")) console.log(process.env.PINNED_CONSUMERS || "0");
  else if (sql.includes("AS digest")) console.log(JSON.stringify([...sql.matchAll(/SELECT '([^']+)' AS name/g)].map(match => ({ name: match[1], count: 1, digest: "d41d8cd98f00b204e9800998ecf8427e" }))));
  else if (sql.includes("AS count")) console.log("[]");
  else { console.error("Unexpected fixture query"); process.exit(1); }
} else { console.error("Unexpected fixture command"); process.exit(1); }
`;

function fixture(t, { apply = true, owner = false, env = {} } = {}) {
  const root = mkdtempSync(join(tmpdir(), "semlia-cleanup-test-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const directory of ["scripts/dev", "bin", ".semlia/native/content", ".semlia/native/artifacts"]) mkdirSync(join(root, directory), { recursive: true });
  copyFileSync(new URL("./reset-knowledge-prototype.mjs", import.meta.url), join(root, "scripts/dev/reset-knowledge-prototype.mjs"));
  writeFileSync(join(root, "bin/docker"), dockerFixture);
  chmodSync(join(root, "bin/docker"), 0o700);
  if (owner) writeFileSync(join(root, ".semlia/native/owner.json"), "{}");
  const result = spawnSync(process.execPath, ["scripts/dev/reset-knowledge-prototype.mjs", ...(apply ? ["--apply"] : [])], {
    cwd: root, encoding: "utf8", env: { ...process.env, ...env, PATH: `${join(root, "bin")}:${process.env.PATH}` },
  });
  return { root, result };
}

test("cleanup verifies retained mixed-table rows before commit and restores only the immutable trigger", (t) => {
  const { root, result } = fixture(t, { env: { TRIGGER_STATE: "A" } });
  assert.equal(result.status, 0, result.stderr);
  const transaction = readFileSync(join(root, "transaction.sql"), "utf8");
  assert.doesNotMatch(transaction, /TRUNCATE[^;]*CASCADE|DISABLE TRIGGER USER|ENABLE TRIGGER USER/);
  assert.match(transaction, /ENABLE ALWAYS TRIGGER runtime_run_events_immutable/);
  const guard = transaction.lastIndexOf("Preservation mismatch");
  assert.ok(guard > transaction.lastIndexOf("DELETE FROM outbox_events"), "retained-row verification must follow all deletes");
  assert.ok(guard < transaction.lastIndexOf("COMMIT;"), "retained-row verification must roll back, not report after commit");
  for (const name of ["jobs", "runtime_runs", "runtime_run_events", "attention_items", "outbox_events", "webhook_deliveries", "webhook_fanout_receipts"]) {
    assert.match(transaction, new RegExp(`FROM "${name}" t WHERE \\(.+?\\) IS NOT TRUE`), `preserve the non-targeted rows in ${name}`);
  }
  const backupRoot = join(root, ".semlia/backups");
  const backup = join(backupRoot, readdirSync(backupRoot)[0]);
  assert.equal(statSync(backup).mode & 0o777, 0o700);
  for (const name of ["database.dump", "native-content.tgz", "inspection.json", "verification.json"]) assert.equal(statSync(join(backup, name)).mode & 0o777, 0o600, name);
});

test("inspection is read-only and active services, clients, or pins fail before backup", (t) => {
  for (const options of [{ apply: false }, { owner: true }, { env: { CONNECTED_CLIENTS: "1" } }, { env: { PINNED_CONSUMERS: "1" } }]) {
    const { root, result } = fixture(t, options);
    assert.equal(result.status, options.apply === false ? 0 : 1, result.stderr);
    assert.equal(existsSync(join(root, "transaction.sql")), false);
    assert.equal(existsSync(join(root, ".semlia/backups")), false);
  }
});

test("a transaction error cannot produce a success verification record", (t) => {
  const { root, result } = fixture(t, { env: { FAIL_TRANSACTION: "1" } });
  assert.equal(result.status, 1);
  const backupRoot = join(root, ".semlia/backups");
  const backup = join(backupRoot, readdirSync(backupRoot)[0]);
  assert.equal(existsSync(join(backup, "database.dump")), true);
  assert.equal(existsSync(join(backup, "verification.json")), false);
});

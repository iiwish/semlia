import { spawnSync } from "node:child_process";
import { existsSync, mkdirSync, openSync, closeSync, writeFileSync } from "node:fs";
import { dirname, resolve, join } from "node:path";
import { fileURLToPath } from "node:url";

// Deliberately cannot target an arbitrary database or remote host.
const root = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const container = "semlia-local-postgres-1";
const run = (program, args, options = {}) => {
  const result = spawnSync(program, args, { cwd: root, encoding: "utf8", maxBuffer: 64 * 1024 * 1024, ...options });
  if (result.status !== 0) throw new Error(`${program} failed: ${result.stderr ?? "inspect local logs"}`);
  return result.stdout;
};
const sql = (query) => run("docker", ["exec", "-i", container, "psql", "-X", "-U", "semlia", "-d", "semlia", "-At", "-v", "ON_ERROR_STOP=1"], { input: query });
const rows = (query) => JSON.parse(sql(`SELECT coalesce(json_agg(t),'[]') FROM (${query}) t;`));
const identifier = (value) => `"${value.replaceAll('"', '""')}"`;
const literal = (value) => `'${String(value).replaceAll("'", "''")}'`;
const labels = JSON.parse(run("docker", ["inspect", container, "--format", "{{json .Config.Labels}}"]));
if (labels["com.docker.compose.project"] !== "semlia-local" || labels["com.docker.compose.project.working_dir"] !== root) throw new Error("Not the owned local Semlia database.");
const tables = rows("SELECT tablename AS name FROM pg_tables WHERE schemaname='public'").map((row) => row.name);
const links = rows("SELECT conrelid::regclass::text AS child,confrelid::regclass::text AS parent FROM pg_constraint WHERE contype='f' AND connamespace='public'::regnamespace");
const clear = new Set(["semantic_assets", "semantic_candidates", "production_operations", "releases", "join_contracts", "ontology_revisions", "review_batches", "agent_runs", "embedding_index_versions", "semantic_queries"]);
let changed = true;
while (changed) { changed = false; for (const link of links) if (link.child !== "consumer_bindings" && clear.has(link.parent) && !clear.has(link.child)) { clear.add(link.child); changed = true; } }
const allowed = /^(semantic_|production_|release|resolved_semantic_plans$|asset_revisions$|resource_aliases$|physical_bindings$|model_grains$|entity_keys$|join_contracts$|ontology_|usage_events$|proposal|review|validation_|policy_decisions$|agent_|embedding_|knowledge_chunks$|query_|revision_evidence_links$)/;
for (const table of clear) if (!allowed.test(table)) throw new Error(`Protected table in FK closure: ${table}`);
const counts = rows([...clear].sort().map((name) => `SELECT '${name}' AS name,count(*)::int AS count FROM "${name}"`).join(" UNION ALL "));
if (Number(sql("SELECT count(*) FROM consumer_bindings WHERE release_id IS NOT NULL;").trim())) throw new Error("Pinned consumers require explicit unbinding; refusing cleanup.");
const bindingFK = rows("SELECT conname AS name,pg_get_constraintdef(oid) AS definition FROM pg_constraint WHERE conrelid='consumer_bindings'::regclass AND confrelid='releases'::regclass");
if (bindingFK.length !== 1 || bindingFK[0].name !== "consumer_bindings_workspace_id_release_id_fkey") throw new Error("Unexpected binding dependency.");
console.log(JSON.stringify({ target: "local Semlia only", counts }, null, 2));
if (!process.argv.includes("--apply")) { console.log("Inspection only. --apply requires the native services to be stopped."); process.exit(0); }
if (existsSync(join(root, ".semlia/native/owner.json"))) throw new Error("Stop owned native services before cleanup.");
const writers = Number(sql("SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND backend_type='client backend';").trim());
if (writers) throw new Error("Other database clients are connected; refusing cleanup.");
const derived = "kind IN ('query_execution','embedding_rebuild','semantic_resolution','agent','validation')";
const event = "event_type LIKE 'catalog.asset.%' OR event_type LIKE 'proposal.%' OR event_type LIKE 'release.%' OR event_type LIKE 'production.%'";
const scopedDeletes = {
  runtime_run_events: `runtime_run_id IN (SELECT id FROM runtime_runs WHERE ${derived})`,
  runtime_runs: derived,
  attention_items: "target_type IN ('semantic_asset','asset','proposal','production_operation','release','agent_run','semantic_query','validation_run')",
  jobs: "job_type IN ('candidate.autodraft','governance.proposal.validate','embedding.rebuild','production.generate','production.validate')",
  webhook_deliveries: `event_id IN (SELECT id FROM outbox_events WHERE ${event})`,
  webhook_fanout_receipts: `event_id IN (SELECT id FROM outbox_events WHERE ${event})`,
  outbox_events: event,
};
const preserved = tables.filter((name) => !clear.has(name)).sort();
const preservationQuery = preserved.map((name) => `SELECT ${literal(name)} AS name,count(*)::int AS count,md5(coalesce(string_agg(md5(row_to_json(t)::text),'' ORDER BY md5(row_to_json(t)::text)),'')) AS digest FROM ${identifier(name)} t${scopedDeletes[name] ? ` WHERE (${scopedDeletes[name]}) IS NOT TRUE` : ""}`).join(" UNION ALL ");
const fingerprint = () => rows(preservationQuery);
const before = fingerprint();
const trigger = rows("SELECT tgenabled AS state FROM pg_trigger WHERE tgrelid='runtime_run_events'::regclass AND tgname='runtime_run_events_immutable' AND NOT tgisinternal");
const triggerMode = { O: "ENABLE", A: "ENABLE ALWAYS", R: "ENABLE REPLICA", D: "DISABLE" }[trigger[0]?.state];
if (trigger.length !== 1 || !triggerMode) throw new Error("Unexpected runtime event immutability trigger.");
const directory = join(root, ".semlia/backups", `knowledge-${new Date().toISOString().replace(/[:.]/g, "-")}`);
mkdirSync(directory, { recursive: true, mode: 0o700 });
const output = openSync(join(directory, "database.dump"), "wx", 0o600);
try { run("docker", ["exec", container, "pg_dump", "-U", "semlia", "-d", "semlia", "-Fc"], { stdio: ["ignore", output, "pipe"] }); } finally { closeSync(output); }
const archive = openSync(join(directory, "native-content.tgz"), "wx", 0o600);
try { run("tar", ["-czf", "-", "-C", join(root, ".semlia/native"), "content", "artifacts"], { stdio: ["ignore", archive, "pipe"], env: { ...process.env, COPYFILE_DISABLE: "1" } }); } finally { closeSync(archive); }
writeFileSync(join(directory, "inspection.json"), JSON.stringify({ counts, foreignKeys: links, preserved: before }, null, 2), { mode: 0o600 });
// Check the same retained rows before mutation and before commit, while all tables are locked.
const preservationGuard = `DO $$ BEGIN IF EXISTS (
  SELECT 1 FROM (${preservationQuery}) actual FULL JOIN pg_temp.cleanup_expected expected USING (name)
  WHERE ROW(actual.count,actual.digest) IS DISTINCT FROM ROW(expected.count,expected.digest)
) THEN RAISE EXCEPTION 'Preservation mismatch; cleanup rolled back'; END IF; END $$;`;
sql(`BEGIN;
SET LOCAL lock_timeout='5s';
LOCK TABLE ${tables.sort().map(identifier).join(",")} IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN IF EXISTS (SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND backend_type='client backend') THEN RAISE EXCEPTION 'database clients changed'; END IF; END $$;
CREATE TEMP TABLE cleanup_expected (name text PRIMARY KEY,count bigint,digest text) ON COMMIT DROP;
INSERT INTO pg_temp.cleanup_expected VALUES ${before.map((row) => `(${literal(row.name)},${row.count},${literal(row.digest)})`).join(",")};
${preservationGuard}
DO $$ BEGIN IF EXISTS (SELECT 1 FROM consumer_bindings WHERE release_id IS NOT NULL) THEN RAISE EXCEPTION 'pinned consumer changed'; END IF; END $$;
ALTER TABLE consumer_bindings DROP CONSTRAINT consumer_bindings_workspace_id_release_id_fkey;
TRUNCATE ${[...clear].sort().map((name) => `"${name}"`).join(",") } RESTRICT;
ALTER TABLE consumer_bindings ADD CONSTRAINT consumer_bindings_workspace_id_release_id_fkey ${bindingFK[0].definition};
ALTER TABLE runtime_run_events DISABLE TRIGGER runtime_run_events_immutable;
${Object.entries(scopedDeletes).map(([name, predicate]) => `DELETE FROM ${name} WHERE ${predicate};`).join("\n")}
ALTER TABLE runtime_run_events ${triggerMode} TRIGGER runtime_run_events_immutable;
${preservationGuard}
COMMIT;`);
const after = fingerprint();
if (JSON.stringify(before) !== JSON.stringify(after)) throw new Error(`Preservation mismatch. Restore snapshot: ${directory}`);
writeFileSync(join(directory, "verification.json"), JSON.stringify({ status: "cleared", preserved: after, clearedTables: [...clear].sort() }, null, 2), { mode: 0o600 });
console.log(`Cleanup verified. Protected rows across ${preserved.length} tables retain identical counts and row fingerprints. Snapshot: ${directory}`);

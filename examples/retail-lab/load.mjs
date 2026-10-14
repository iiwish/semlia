// Load the Retail Lab dataset into a local PostgreSQL, verify golden answers,
// or build a portable SQLite copy. Usage:
//   node examples/retail-lab/load.mjs [up|verify|down|sqlite|info] [--container NAME] [--port 5433]
// The default target is the local development container semlia-local-postgres-1.
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync, chmodSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '../..');
const args = process.argv.slice(2);
const command = args[0] && !args[0].startsWith('--') ? args[0] : 'up';
const option = (name, fallback) => { const i = args.indexOf('--' + name); return i >= 0 ? args[i + 1] : fallback; };
const container = option('container', 'semlia-local-postgres-1');
const superuser = option('superuser', 'semlia');
const hostPort = option('port', '5433');
const database = 'semlia_retail_lab';
const roles = { owner: 'retail_lab_owner', reader: 'retail_lab_reader', exec: 'retail_lab_exec' };
const views = ['valid_orders', 'order_net_sales', 'region_daily_sales', 'item_gross_profit', 'member_activity', 'store_monthly_target'];
const statePath = join(repo, '.semlia/retail-lab/credentials.json');
const quote = value => "'" + String(value).replaceAll("'", "''") + "'";

function psql(db, sql, { quiet = true } = {}) {
  const result = spawnSync('docker', ['exec', '-i', container, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', superuser, '-d', db],
    { input: sql, encoding: 'utf8', maxBuffer: 64 * 1024 * 1024 });
  if (result.error || result.status !== 0) throw new Error(`psql failed on ${db}: ${(result.stderr || result.error?.message || '').trim()}`);
  if (!quiet) process.stdout.write(result.stdout);
  return result.stdout.trim();
}

function credentials() {
  if (existsSync(statePath)) return JSON.parse(readFileSync(statePath, 'utf8'));
  const value = { reader: randomBytes(12).toString('hex'), exec: randomBytes(12).toString('hex') };
  mkdirSync(dirname(statePath), { recursive: true, mode: 0o700 });
  writeFileSync(statePath, JSON.stringify(value, null, 2) + '\n', { mode: 0o600 });
  chmodSync(statePath, 0o600);
  return value;
}

function up() {
  const secret = credentials();
  if (psql('postgres', `SELECT count(*) FROM pg_database WHERE datname=${quote(database)};`) !== '0') {
    console.log(`${database} already exists; run "down" first to rebuild. Refreshing role passwords only.`);
  } else {
    psql('postgres', `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=${quote(roles.owner)}) THEN CREATE ROLE ${roles.owner} NOLOGIN; END IF; END $$;`);
    psql('postgres', `CREATE DATABASE ${database} OWNER ${roles.owner} ENCODING 'UTF8' TEMPLATE template0;`);
    psql(database, `REVOKE ALL ON DATABASE ${database} FROM PUBLIC; REVOKE ALL ON SCHEMA public FROM PUBLIC;`);
    const files = [join(here, 'db/schema.sql'), join(here, 'db/seed.sql'), ...views.map(v => join(here, `sql/mart/${v}.sql`))];
    const body = files.map(f => readFileSync(f, 'utf8')).join('\n');
    console.log('Loading schema, seed and mart views ...');
    psql(database, `SET ROLE ${roles.owner};\n${body}\nRESET ROLE;\nANALYZE;`);
  }
  // Discovery reader and execution role: login, no CREATE anywhere, read-only by default,
  // and failing statements are kept out of the server log (required by the execution adapter).
  for (const [role, password] of [[roles.reader, secret.reader], [roles.exec, secret.exec]]) {
    psql('postgres', `DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=${quote(role)}) THEN CREATE ROLE ${role} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; END IF; END $$;
ALTER ROLE ${role} PASSWORD ${quote(password)};
ALTER ROLE ${role} SET default_transaction_read_only = on;
ALTER ROLE ${role} SET log_min_error_statement = panic;
GRANT CONNECT ON DATABASE ${database} TO ${role};`);
    psql(database, `GRANT USAGE ON SCHEMA retail, mart, legacy TO ${role}; GRANT SELECT ON ALL TABLES IN SCHEMA retail, mart, legacy TO ${role};`);
  }
  info();
}

function info() {
  const secret = credentials();
  const counts = psql(database, "SELECT string_agg(format('%s.%s=%s', n.nspname, c.relname, (xpath('/row/c/text()', query_to_xml(format('SELECT count(*) AS c FROM %I.%I', n.nspname, c.relname), false, true, '')))[1]), ', ' ORDER BY n.nspname, c.relname) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relkind = 'r' AND n.nspname IN ('retail', 'legacy');");
  console.log(`\nRetail Lab is ready in ${database} (container ${container}).\nTables: ${counts}\n`);
  console.log('Register a PostgreSQL source in Semlia with:');
  console.log(`  host 127.0.0.1  port ${hostPort}  database ${database}  sslmode disable`);
  console.log(`  discovery user ${roles.reader}  password ${secret.reader}`);
  console.log('\nOptional Ask execution (protected server env, after the source exists):');
  console.log(`  SEMLIA_EXECUTION_DSN_RETAIL_LAB=postgresql://${roles.exec}:${secret.exec}@127.0.0.1:${hostPort}/${database}?sslmode=disable`);
  console.log(`  SEMLIA_EXECUTION_SOURCES=[{"workspaceId":"<workspace>","sourceId":"<source>","dsnEnv":"SEMLIA_EXECUTION_DSN_RETAIL_LAB"}]`);
  console.log('  SEMLIA_EXECUTION_ALLOW_PLAINTEXT=true');
  console.log(`\nCredentials are stored in ${statePath} (gitignored).`);
}

function verify(run) {
  const { questions } = JSON.parse(readFileSync(join(here, 'expected/questions.json'), 'utf8'));
  const { answers } = JSON.parse(readFileSync(join(here, 'expected/answers.json'), 'utf8'));
  let failed = 0;
  for (const q of questions) {
    const actual = run(q.sql);
    const ok = JSON.stringify(actual) === JSON.stringify(answers[q.id]);
    if (!ok) failed++;
    console.log(`${ok ? 'PASS' : 'FAIL'}  ${q.id}  ${q.question}${ok ? '' : `\n      expected ${JSON.stringify(answers[q.id])}\n      actual   ${JSON.stringify(actual)}`}`);
  }
  console.log(`\n${questions.length - failed}/${questions.length} golden answers match.`);
  if (failed) process.exitCode = 1;
}

const rowsOf = text => text === '' ? [] : text.split('\n').map(line => line.split('|'));

function down() {
  psql('postgres', `DROP DATABASE IF EXISTS ${database} WITH (FORCE);`);
  for (const role of [roles.reader, roles.exec, roles.owner]) psql('postgres', `DROP ROLE IF EXISTS ${role};`);
  rmSync(statePath, { force: true });
  console.log(`Dropped ${database} and its roles.`);
}

switch (command) {
  case 'up': up(); break;
  case 'info': info(); break;
  case 'verify': verify(sql => rowsOf(psql(database, `SET default_transaction_read_only = on;\n${sql};`))); break;
  case 'down': down(); break;
  case 'sqlite': (await import('./sqlite.mjs')).build(join(here, 'db/retail_lab.sqlite')); break;
  default: console.error(`Unknown command ${command}. Use up | verify | info | down | sqlite.`); process.exitCode = 2;
}

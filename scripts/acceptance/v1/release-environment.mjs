import assert from 'node:assert/strict';
import { randomBytes } from 'node:crypto';
import { existsSync, mkdirSync, lstatSync, readdirSync, readFileSync, writeFileSync, openSync, closeSync } from 'node:fs';
import { join } from 'node:path';
import { spawn } from 'node:child_process';
import { setTimeout } from 'node:timers/promises';
import { availablePort } from '../../demo/runtime.mjs';
import { DemoClient } from '../../demo/client.mjs';
import { databaseQuery } from '../../demo/provision.mjs';
import { repo, root, environment } from './runtime.mjs';
import { readState, readPrivate, writePrivate, rejectLinks, digest, validateRole, validateDatabase } from './core.mjs';
import { verifyProvisioned, preservationSnapshot, run } from './provision.mjs';
import { assertRestoreTarget } from './release-core.mjs';
export { fileInventory } from './release-storage.mjs';

export const releaseRoot = join(root, 'release-drill');
const planPath = join(releaseRoot, 'plan.json');
const quote = value => "'" + value.replaceAll("'", "''") + "'";

export function privateDirectory(path) {
  rejectLinks(path);
  if (!existsSync(path)) mkdirSync(path, { mode: 0o700 });
  const stat = lstatSync(path);
  assert.ok(stat.isDirectory() && !(stat.mode & 0o077), 'Unsafe release drill directory');
}

export function planDrill() {
  const state = readState(root), database = verifyProvisioned(repo, state);
  privateDirectory(releaseRoot);
  if (existsSync(planPath)) return readPlan();
  assert.equal(readdirSync(releaseRoot).length, 0, 'Unknown contents in release plan directory');
  const owner = 'semlia_v1r_' + randomBytes(8).toString('hex');
  const targets = Object.fromEntries(['install', 'upgrade', 'restore'].map(purpose => {
    const name = owner + '_' + purpose;
    return [purpose, { database: name, role: name + '_owner', root: join(releaseRoot, purpose) }];
  }));
  const identity = { owner, sourceOwner: state.owner, database, targets };
  const marker = owner + ':' + digest(JSON.stringify(identity));
  const plan = { format: 1, ...identity, marker, preservation: preservationSnapshot(repo, database), createdAt: new Date().toISOString() };
  const original = readPrivate(join(root, 'secrets.json'));
  const secrets = { databasePasswords: Object.fromEntries(Object.keys(targets).map(key => [key, randomBytes(32).toString('hex')])), freshSecretKey: randomBytes(32).toString('hex'), freshAccountPassword: randomBytes(24).toString('hex'), recovery: original };
  writePrivate(join(releaseRoot, 'secrets.json'), secrets, true);
  writePrivate(planPath, plan, true);
  return plan;
}

export function readPlan() {
  const plan = readPrivate(planPath), state = readState(root), database = verifyProvisioned(repo, state);
  assert.equal(plan.format, 1); assert.match(plan.owner, /^semlia_v1r_[a-f0-9]{16}$/); assert.equal(plan.sourceOwner, state.owner); assert.deepEqual(plan.database, database);
  const identity = { owner: plan.owner, sourceOwner: plan.sourceOwner, database: plan.database, targets: plan.targets };
  assert.equal(plan.marker, plan.owner + ':' + digest(JSON.stringify(identity)));
  for (const purpose of ['install', 'upgrade', 'restore']) {
    const target = plan.targets[purpose];
    assert.equal(target.database, plan.owner + '_' + purpose); assert.equal(target.role, target.database + '_owner'); assert.equal(target.root, join(releaseRoot, purpose));
  }
  return plan;
}

export function inspectTarget(plan, purpose, requireEmpty = false) {
  const target = plan.targets[purpose]; assert.ok(target);
  const query = sql => databaseQuery(plan.database.container, 'postgres', sql);
  const roleRaw = query(`SELECT json_build_object('marker',shobj_description(oid,'pg_authid'),'superuser',rolsuper,'createdb',rolcreatedb,'createrole',rolcreaterole,'replication',rolreplication,'bypassrls',rolbypassrls,'login',rolcanlogin,'memberships',(SELECT count(*) FROM pg_auth_members WHERE member=pg_roles.oid)) FROM pg_roles WHERE rolname=${quote(target.role)};`);
  const databaseRaw = query(`SELECT json_build_object('owner',pg_get_userbyid(datdba),'marker',shobj_description(oid,'pg_database')) FROM pg_database WHERE datname=${quote(target.database)};`);
  assert.ok(roleRaw && databaseRaw, 'Owned target is missing');
  const role = JSON.parse(roleRaw), database = JSON.parse(databaseRaw);
  validateRole(role, plan.marker); validateDatabase(database, target.role, plan.marker);
  if (requireEmpty) {
    const tables = Number(databaseQuery(plan.database.container, target.database, "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND c.relkind IN ('r','p','v','m','f');"));
    assertRestoreTarget({ ...role, database: target.database, role: database.owner, marker: database.marker, empty: tables === 0 }, { database: target.database, role: target.role, marker: plan.marker });
  }
  return target;
}

export function provisionTargets() {
  const plan = readPlan(), secrets = readPrivate(join(releaseRoot, 'secrets.json'));
  const receiptPath = join(releaseRoot, 'provisioned.json');
  if (existsSync(receiptPath)) {
    const receipt = readPrivate(receiptPath); assert.equal(receipt.owner, plan.owner); assert.equal(receipt.marker, plan.marker); assert.deepEqual(receipt.targets, plan.targets);
  } else for (const target of Object.values(plan.targets)) {
    rejectLinks(target.root);
    if (existsSync(target.root)) assert.ok(lstatSync(target.root).isDirectory() && readdirSync(target.root).length === 0, 'Unknown contents in release target directory');
  }
  const query = sql => databaseQuery(plan.database.container, 'postgres', sql);
  for (const [purpose, target] of Object.entries(plan.targets)) {
    const roleExists = query(`SELECT count(*) FROM pg_roles WHERE rolname=${quote(target.role)};`) === '1';
    const databaseExists = query(`SELECT count(*) FROM pg_database WHERE datname=${quote(target.database)};`) === '1';
    if (!roleExists && !databaseExists) {
      const password = secrets.databasePasswords[purpose]; assert.match(password, /^[a-f0-9]{64}$/);
      query(`BEGIN; CREATE ROLE ${target.role} LOGIN PASSWORD ${quote(password)} NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; COMMENT ON ROLE ${target.role} IS ${quote(plan.marker)}; COMMIT;`);
      query(`CREATE DATABASE ${target.database} OWNER ${target.role} TEMPLATE template0;`);
      query(`COMMENT ON DATABASE ${target.database} IS ${quote(plan.marker)}; REVOKE CONNECT,TEMPORARY ON DATABASE ${target.database} FROM PUBLIC;`);
    }
    inspectTarget(plan, purpose, true);
    privateDirectory(target.root);
    for (const name of ['content', 'inputs', 'artifacts']) privateDirectory(join(target.root, name));
  }
  writePrivate(join(releaseRoot, 'provisioned.json'), { owner: plan.owner, targets: plan.targets, marker: plan.marker, checkedAt: new Date().toISOString() });
  return { owner: plan.owner, targets: Object.values(plan.targets).map(({ database, role }) => ({ database, role })) };
}

export function targetEnvironment(plan, purpose, port, migrationPath = join(repo, 'migrations')) {
  const target = inspectTarget(plan, purpose), secrets = readPrivate(join(releaseRoot, 'secrets.json'));
  assert.ok(Number.isInteger(port) && port > 1024 && port < 65536);
  const env = environment(readState(root));
  delete env.SEMLIA_EXECUTION_DSN_V1;
  const recovered = purpose === 'restore' ? readPrivate(join(target.root, 'recovery-keys.json')) : undefined;
  if (recovered) { assert.equal(recovered.owner, plan.owner); assert.equal(recovered.sourceOwner, plan.sourceOwner); }
  return { ...env,
    SEMLIA_DATABASE_URL: `postgresql://${target.role}:${secrets.databasePasswords[purpose]}@127.0.0.1:${plan.database.port}/${target.database}?sslmode=disable`,
    SEMLIA_HTTP_ADDR: `127.0.0.1:${port}`, SEMLIA_ALLOWED_ORIGINS: `http://127.0.0.1:${port}`,
    SEMLIA_SECRET_KEY: recovered ? recovered.secretKey : secrets.freshSecretKey,
    SEMLIA_EXECUTION_SOURCES: '[]', SEMLIA_V1_MODEL_SECRET: recovered ? recovered.modelSecret : '',
    SEMLIA_GIT_REPOSITORY: join(target.root, 'content'), SEMLIA_SOURCE_ARTIFACT_ROOT: join(target.root, 'inputs'), SEMLIA_ARTIFACT_ROOT: join(target.root, 'artifacts'), SEMLIA_MIGRATIONS_PATH: migrationPath,
  };
}

export async function withTargetServer(plan, purpose, action) {
  const target = inspectTarget(plan, purpose), port = await availablePort(), env = targetEnvironment(plan, purpose, port);
  const log = join(target.root, 'server.log'); rejectLinks(log);
  const fd = openSync(log, 'a', 0o600);
  const child = spawn(join(root, 'server'), ['server'], { cwd: repo, env, stdio: ['ignore', fd, fd] }); closeSync(fd);
  let closed = false; child.once('exit', () => { closed = true; }); child.once('error', () => { closed = true; });
  try {
    let ready = false;
    for (let i = 0; i < 100 && !closed; i++) {
      try { ready = (await fetch(`http://127.0.0.1:${port}/health/ready`, { signal: AbortSignal.timeout(1000) })).ok; } catch { /* Bounded readiness wait. */ }
      if (ready) break; await setTimeout(100);
    }
    assert.ok(ready && !closed, 'Drill server did not become ready');
    return await action(new DemoClient(`http://127.0.0.1:${port}`), port);
  } finally {
    if (!closed) child.kill('SIGTERM');
    for (let i = 0; i < 150 && !closed; i++) await setTimeout(100);
    if (!closed) child.kill('SIGKILL');
    for (let i = 0; i < 50 && !closed; i++) await setTimeout(100);
    assert.ok(closed, 'Owned drill process did not stop');
  }
}

export function databaseFingerprint(plan, database, names, schema = 'public') {
  assert.match(schema, /^[a-z][a-z0-9_]*$/);
  const tables = names ?? JSON.parse(databaseQuery(plan.database.container, database, `SELECT coalesce(json_agg(tablename ORDER BY tablename),'[]') FROM pg_tables WHERE schemaname=${quote(schema)};`));
  return tables.map(table => {
    assert.match(table, /^[a-z][a-z0-9_]*$/);
    const row = JSON.parse(databaseQuery(plan.database.container, database, `SELECT json_build_object('count',count(*),'digest',md5(coalesce(string_agg(value,E'\\n' ORDER BY value),''))) FROM (SELECT row_to_json(t)::text AS value FROM ${schema}.${table} t) rows;`));
    return { table, ...row };
  });
}

export async function installUpgrade(resume = false) {
  const plan = readPlan(), secrets = readPrivate(join(releaseRoot, 'secrets.json')), binary = join(root, 'server');
  const receiptPath = join(releaseRoot, 'install-upgrade.json');
  if (resume) {
    const prior = readPrivate(receiptPath);
    assert.equal(prior.owner, plan.owner); assert.equal(prior.state, 'failed'); assert.deepEqual(prior.targets, {});
    assert.ok(existsSync(join(plan.targets.install.root, 'bootstrap.json')));
    writePrivate(join(releaseRoot, 'install-upgrade-failed-inventory-table.json'), prior, true);
  } else assert.ok(!existsSync(receiptPath), 'Install/upgrade receipt already exists; no automatic rerun');
  const receipt = { owner: plan.owner, state: 'started', startedAt: new Date().toISOString(), binaryDigest: digest(readFileSync(binary)), candidate: false, targets: {} };
  if (resume) receipt.repair = 'inventory-user-accounts-table';
  writePrivate(receiptPath, receipt, !resume);
  try {
    for (const purpose of ['install', 'upgrade']) {
      const resumedInstall = resume && purpose === 'install';
      const target = inspectTarget(plan, purpose, !resumedInstall), port = await availablePort(); let migrationPath = join(repo, 'migrations');
      if (purpose === 'upgrade') {
        migrationPath = join(target.root, 'migrations32'); privateDirectory(migrationPath);
        for (const name of readdirSync(join(repo, 'migrations')).sort().filter(name => /^\d{6}_.*\.sql$/.test(name) && Number(name.slice(0, 6)) <= 32)) {
          writeFileSync(join(migrationPath, name), readFileSync(join(repo, 'migrations', name)), { flag: 'wx', mode: 0o600 });
        }
      }
      const env = targetEnvironment(plan, purpose, port, migrationPath), username = 'v1_release_admin';
      if (!resumedInstall) run(repo, binary, ['migrate', 'up'], { env });
      const baselineSchema = Number(databaseQuery(plan.database.container, target.database, 'SELECT version FROM schema_migrations;'));
      assert.equal(baselineSchema, purpose === 'upgrade' ? 32 : 33);
      if (!resumedInstall) {
        const bootstrap = run(repo, binary, ['bootstrap-local-admin', plan.owner.replaceAll('_', '-') + '-' + purpose, 'V1 synthetic release drill', username], { env, input: secrets.freshAccountPassword + '\n' });
        writePrivate(join(target.root, 'bootstrap.json'), { output: bootstrap });
      }
      const before = databaseFingerprint(plan, target.database, ['user_accounts', 'workspaces', 'principals', 'workspace_memberships']);
      assert.ok(before.every(row => row.count > 0));
      if (purpose === 'upgrade') run(repo, binary, ['migrate', 'up'], { env: targetEnvironment(plan, purpose, port) });
      assert.deepEqual(databaseFingerprint(plan, target.database, before.map(row => row.table)), before);
      run(repo, binary, ['check-schema'], { env: targetEnvironment(plan, purpose, port) });
      const logged = await withTargetServer(plan, purpose, async client => {
        const session = await client.login(username, secrets.freshAccountPassword);
        assert.equal(session.workspaces.length, 1);
        const workspace = session.workspaces[0];
        const catalog = await client.request('GET', `/api/v1/workspaces/${workspace.id}/catalog/assets?limit=100`);
        assert.equal(catalog.items.length, 0);
        return { workspaceId: workspace.id, principalId: workspace.principalId, normalPasswordLogin: true, catalogEmpty: true };
      });
      receipt.targets[purpose] = { database: target.database, role: target.role, beforeSchema: baselineSchema, afterSchema: 33, preservedTables: before, ...logged };
      writePrivate(receiptPath, receipt);
    }
    assert.deepEqual(preservationSnapshot(repo, plan.database), plan.preservation);
    receipt.state = 'passed'; receipt.finishedAt = new Date().toISOString(); writePrivate(receiptPath, receipt);
    return { state: receipt.state, baseline: 'populated normal bootstrap schema32 to33', targets: receipt.targets, modelCalls: 0 };
  } catch (error) { receipt.state = 'failed'; receipt.status = Number.isInteger(error.status) ? error.status : undefined; writePrivate(receiptPath, receipt); throw error; }
}

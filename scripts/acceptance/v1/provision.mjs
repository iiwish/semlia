import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { existsSync, readFileSync, lstatSync } from 'node:fs';
import { join } from 'node:path';
import { parseEnv } from 'node:util';
import { inventory, databaseQuery } from '../../demo/provision.mjs';
import { checkpoint, digest, readPrivate, writePrivate, validOwner, validateRole, validateDatabase, goldenCases } from './core.mjs';

const quote = value => "'" + value.replaceAll("'", "''") + "'";
export function run(repo, program, args, options = {}) {
  const result = spawnSync(program, args, { cwd: repo, encoding: 'utf8', timeout: 240000, maxBuffer: 16 * 1024 * 1024, ...options });
  if (result.error || result.status !== 0) throw new Error(`${program} command failed; protected diagnostics withheld`);
  return result.stdout;
}
export function verifyInventory(repo, state) {
  const target = inventory(repo);
  if (state.database && (state.database.container !== target.container || state.database.port !== target.port)) throw new Error('Database inventory changed');
  if (!validOwner(state.owner)) throw new Error('Invalid V1 owner');
  return target;
}
export const readerRoleSettings = { default_transaction_read_only: 'on', log_min_error_statement: 'panic' };
export function assertReaderPrivacy(config) {
  if (!Array.isArray(config)) throw new Error('Reader role configuration is missing');
  const settings = Object.fromEntries(config.map(item => { const at = item.indexOf('='); return [item.slice(0, at), item.slice(at + 1)]; }));
  for (const [key, value] of Object.entries(readerRoleSettings)) if (settings[key] !== value) throw new Error('Reader role privacy configuration drift');
}
function inspectOwnedResources(repo, state) {
  const target = verifyInventory(repo, state);
  if (!state.database) throw new Error('V1 database ownership receipt required');
  const roles = [], databases = [];
  for (const role of [state.owner, state.owner + '_reader']) {
    const raw = databaseQuery(target.container, 'postgres', `SELECT json_build_object('marker',shobj_description(oid,'pg_authid'),'superuser',rolsuper,'createdb',rolcreatedb,'createrole',rolcreaterole,'replication',rolreplication,'bypassrls',rolbypassrls,'login',rolcanlogin,'config',rolconfig,'memberships',(SELECT count(*) FROM pg_auth_members WHERE member=pg_roles.oid)) FROM pg_roles WHERE rolname=${quote(role)};`);
    if (!raw) throw new Error('Owned database role missing');
    roles.push(JSON.parse(raw));
  }
  for (const suffix of ['app', 'source']) {
    const raw = databaseQuery(target.container, 'postgres', `SELECT json_build_object('owner',pg_get_userbyid(datdba),'marker',shobj_description(oid,'pg_database')) FROM pg_database WHERE datname=${quote(state.owner + '_' + suffix)};`);
    if (!raw) throw new Error('Owned database missing');
    databases.push(JSON.parse(raw));
  }
  assertOwnedResources(state, roles, databases);
  return { target, roles };
}
export function verifyProvisioned(repo, state) {
  const { target, roles } = inspectOwnedResources(repo, state);
  assertReaderPrivacy(roles[1].config);
  return target;
}
export function hardenOwnedReader(repo, root, state) {
  const { target, roles } = inspectOwnedResources(repo, state);
  const config = roles[1].config;
  if (JSON.stringify(config) !== JSON.stringify(['default_transaction_read_only=on'])) {
    assertReaderPrivacy(config);
    return state;
  }
  databaseQuery(target.container, 'postgres', `ALTER ROLE ${state.owner}_reader SET log_min_error_statement='panic';`);
  verifyProvisioned(repo, state);
  return checkpoint(root, state, { readerPrivacyUpgrade: { checkedAt: new Date().toISOString(), ownerVerified: true, setting: 'log_min_error_statement', value: 'panic', scope: 'owned_reader_only' } });
}
export function assertOwnedResources(state, roles, databases) {
  if (!validOwner(state.owner) || roles.length !== 2 || databases.length !== 2) throw new Error('Complete ownership proof required');
  const marker = `${state.owner}:${state.digest}`;
  for (const role of roles) validateRole(role, marker);
  for (const database of databases) validateDatabase(database, state.owner, marker);
}
export function selectConfiguredModel(choices, explicit) {
  const prototype = choices.filter(x => x.slug === 'demo-202609-commerce');
  const matches = explicit ? choices.filter(x => x.slug === explicit) : prototype.length === 1 ? prototype : choices;
  const selected = matches.length === 1 ? matches[0] : null;
  if (!selected) throw new Error(`Expected one enabled default LLM; found ${choices.length} candidates`);
  return selected;
}
function existingModel(repo, target) {
  const raw = databaseQuery(target.container, 'semlia', `SELECT COALESCE(json_agg(json_build_object('workspace',w.id,'slug',w.slug,'providerId',p.id,'settingId',s.id,'protocol',p.protocol,'baseUrl',p.base_url,'credentialEnv',p.credential_env,'model',s.model,'tokenLimit',s.token_limit)),'[]'::json) FROM model_settings s JOIN model_providers p ON p.id=s.provider_id AND p.workspace_id=s.workspace_id JOIN workspaces w ON w.id=s.workspace_id WHERE s.kind='llm' AND s.enabled AND s.is_default AND p.enabled;`);
  const selected = selectConfiguredModel(JSON.parse(raw), process.env.SEMLIA_V1_MODEL_WORKSPACE);
  let env = {};
  for (const file of ['.semlia/dev.env', '.semlia/native.env']) {
    const path = join(repo, file);
    if (!existsSync(path)) continue;
    if (lstatSync(path).isSymbolicLink() || (lstatSync(path).mode & 0o077)) throw new Error('Existing environment file is not protected');
    env = { ...env, ...parseEnv(readFileSync(path, 'utf8')) };
  }
  const secret = env[selected.credentialEnv];
  if (!secret?.trim()) throw new Error(`Missing configured credential variable: ${selected.credentialEnv}`);
  return { selected, secret };
}
export function preservationSnapshot(repo, target) {
  const files = {};
  for (const name of ['.semlia/dev.env', '.semlia/native.env', 'compose.override.yaml']) {
    const path = join(repo, name);
    files[name] = existsSync(path) ? digest(readFileSync(path)) : null;
  }
  const database = databaseQuery(target.container, 'semlia', `SELECT json_build_object('workspaces',(SELECT count(*) FROM workspaces),'accounts',(SELECT count(*) FROM user_accounts),'sources',(SELECT count(*) FROM source_connections),'releases',(SELECT count(*) FROM releases),'providers',md5(COALESCE((SELECT string_agg(row_to_json(p)::text,'' ORDER BY id) FROM model_providers p),'')),'models',md5(COALESCE((SELECT string_agg(row_to_json(s)::text,'' ORDER BY id) FROM model_settings s),'')));`);
  return { files, database: JSON.parse(database) };
}
export function provision(repo, root, initial) {
  let state = initial;
  const target = verifyInventory(repo, state), marker = `${state.owner}:${state.digest}`;
  const path = join(root, 'secrets.json');
  if (!existsSync(path)) {
    if (state.database) throw new Error('Missing secrets; refusing credential rotation');
    const model = existingModel(repo, target);
    writePrivate(path, { databasePassword: randomBytes(32).toString('hex'), readerPassword: randomBytes(32).toString('hex'), secretKey: randomBytes(32).toString('hex'), accountPassword: randomBytes(24).toString('hex'), modelSecret: model.secret, model: model.selected }, true);
  }
  const secrets = readPrivate(path);
  if (!state.database) state = checkpoint(root, state, { database: target, preservation: preservationSnapshot(repo, target) });
  const query = sql => databaseQuery(target.container, 'postgres', sql);
  for (const [role, password] of [[state.owner, secrets.databasePassword], [state.owner + '_reader', secrets.readerPassword]]) {
    const select = `SELECT json_build_object('marker',shobj_description(oid,'pg_authid'),'superuser',rolsuper,'createdb',rolcreatedb,'createrole',rolcreaterole,'replication',rolreplication,'bypassrls',rolbypassrls,'login',rolcanlogin,'memberships',(SELECT count(*) FROM pg_auth_members WHERE member=pg_roles.oid)) FROM pg_roles WHERE rolname=${quote(role)};`;
    let existing = query(select);
    if (!existing) {
      query(`BEGIN; CREATE ROLE ${role} LOGIN PASSWORD ${quote(password)} NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; COMMENT ON ROLE ${role} IS ${quote(marker)}; ${role.endsWith('_reader') ? Object.entries(readerRoleSettings).map(([key, value]) => `ALTER ROLE ${role} SET ${key}=${quote(value)};`).join(' ') : ''} COMMIT;`);
      existing = query(select);
    }
    validateRole(JSON.parse(existing), marker);
  }
  for (const suffix of ['app', 'source']) {
    const db = `${state.owner}_${suffix}`;
    const select = `SELECT json_build_object('owner',pg_get_userbyid(datdba),'marker',shobj_description(oid,'pg_database')) FROM pg_database WHERE datname=${quote(db)};`;
    let existing = query(select);
    if (!existing) {
      query(`CREATE DATABASE ${db} OWNER ${state.owner} TEMPLATE template0;`);
      query(`COMMENT ON DATABASE ${db} IS ${quote(marker)}; REVOKE CONNECT,TEMPORARY ON DATABASE ${db} FROM PUBLIC;`);
      existing = query(select);
    }
    validateDatabase(JSON.parse(existing), state.owner, marker);
  }
  verifyProvisioned(repo, state);
  return state;
}

export function seedSource(repo, root, state, fixture) {
  const target = verifyProvisioned(repo, state), db = state.owner + '_source';
  const query = sql => databaseQuery(target.container, db, sql);
  const expectedDigest = digest(fixture.DataSQL);
  const marker = `${state.owner}:${expectedDigest}`;
  const existing = query("SELECT obj_description(oid,'pg_namespace') FROM pg_namespace WHERE nspname='semlia_demo_202609';");
  const exists = query("SELECT count(*) FROM pg_namespace WHERE nspname='semlia_demo_202609';");
  if (exists === '0') {
    query(`BEGIN; SET LOCAL ROLE ${state.owner}; ${fixture.DataSQL} COMMENT ON SCHEMA semlia_demo_202609 IS ${quote(marker)}; REVOKE ALL ON SCHEMA semlia_demo_202609 FROM PUBLIC; GRANT USAGE ON SCHEMA semlia_demo_202609 TO ${state.owner}_reader; GRANT SELECT ON ALL TABLES IN SCHEMA semlia_demo_202609 TO ${state.owner}_reader; COMMIT; GRANT CONNECT ON DATABASE ${db} TO ${state.owner}_reader;`);
  } else if (existing !== marker) throw new Error('Source schema ownership or input digest mismatch');
  const results = goldenCases().map(item => {
    const result = query(`SELECT COALESCE(json_agg(row_to_json(q)),'[]'::json) FROM (${item.sql}) q;`);
    const rows = JSON.parse(result).map(row => Object.values(row).map(value => typeof value === 'string' && /^\d+\.0+$/.test(value) ? value.replace(/\.0+$/, '') : String(value)));
    if (JSON.stringify(rows) !== JSON.stringify(item.expected)) throw new Error(`Independent source reconciliation failed: ${item.id}`);
    return { id: item.id, sql: item.sql, expected: item.expected, actual: rows, passed: true };
  });
  writePrivate(join(root, 'golden.json'), results);
  return checkpoint(root, state, { sourceDataDigest: expectedDigest });
}

import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { existsSync, lstatSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { validateInventory } from './environment.mjs';
import { checkpoint } from './state.mjs';

export function validateRole(role, marker) {
  if (role.marker !== marker) throw new Error('Role ownership mismatch');
  if (!role.login || role.superuser || role.createdb || role.createrole || role.replication || role.bypassrls || role.memberships !== 0) throw new Error('Unsafe demo role privileges');
}
export function validateDatabase(database, role, marker) {
  if (database.owner !== role || database.marker !== marker) throw new Error('Database ownership mismatch');
}
function run(args, input) {
  const result = spawnSync('docker', args, { input, encoding: 'utf8', maxBuffer: 8 * 1024 * 1024, timeout: 60000 });
  if (result.status !== 0 || result.error) throw new Error('Demo database command failed; no diagnostics exposed');
  return result.stdout.trim();
}
export function inventory(repo) {
  return validateInventory(JSON.parse(run(['inspect', 'semlia-local-postgres-1']))[0], repo);
}
const quote = value => "'" + value.replaceAll("'", "''") + "'";
export function databaseQuery(container, database, sql) {
  return run(['exec', '-i', container, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'semlia', '-d', database], `SET log_statement='none'; SET log_min_duration_statement=-1; SET log_min_duration_sample=-1; SET log_min_error_statement='panic'; SET log_parameter_max_length_on_error=0; SET log_duration=off;\n${sql}\n`);
}
export function readSecrets(root) {
  const path = join(root, 'secrets.json');
  const info = lstatSync(path);
  if (info.isSymbolicLink() || (info.mode & 0o077)) throw new Error('Unsafe protected secrets file');
  const value = JSON.parse(readFileSync(path, 'utf8'));
  for (const key of ['databasePassword', 'secretKey']) if (!/^[a-f0-9]{64}$/.test(value[key])) throw new Error('Invalid protected secrets');
  if (typeof value.accountPassword !== 'string' || value.accountPassword.length < 6) throw new Error('Invalid demo account password');
  return value;
}
export function provision(repo, root, initialState) {
  let state = initialState;
  const target = inventory(repo);
  if (state.database && (state.database.container !== target.container || state.database.port !== target.port)) throw new Error('Database inventory changed');
  const marker = `${state.owner}:${state.digest}`;
  const name = state.owner;
  if (!/^semlia_demo_[a-f0-9]{16}$/.test(name)) throw new Error('Invalid demo owner');
  const secretPath = join(root, 'secrets.json');
  if (!existsSync(secretPath)) {
    if (state.database) throw new Error('Protected secrets missing; refusing replacement');
    writeFileSync(secretPath, JSON.stringify({ databasePassword: randomBytes(32).toString('hex'), secretKey: randomBytes(32).toString('hex'), accountPassword: '132435' }), { flag: 'wx', mode: 0o600 });
  }
  const secrets = readSecrets(root);
  if (!state.database) state = checkpoint(root, state, { database: target });
  const query = sql => databaseQuery(target.container, 'postgres', sql);
  const roleSQL = `SELECT json_build_object('marker',shobj_description(oid,'pg_authid'),'superuser',rolsuper,'createdb',rolcreatedb,'createrole',rolcreaterole,'replication',rolreplication,'bypassrls',rolbypassrls,'login',rolcanlogin,'memberships',(SELECT count(*) FROM pg_auth_members WHERE member=pg_roles.oid)) FROM pg_roles WHERE rolname=${quote(name)};`;
  let role = query(roleSQL);
  if (!role) {
    query(`BEGIN; CREATE ROLE ${name} LOGIN PASSWORD ${quote(secrets.databasePassword)} NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS; COMMENT ON ROLE ${name} IS ${quote(marker)}; COMMIT;`);
    role = query(roleSQL);
  }
  validateRole(JSON.parse(role), marker);
  for (const suffix of ['app', 'source']) {
    const db = `${name}_${suffix}`;
    const read = () => query(`SELECT json_build_object('owner',pg_get_userbyid(datdba),'marker',shobj_description(oid,'pg_database')) FROM pg_database WHERE datname=${quote(db)};`);
    let current = read();
    if (!current) {
      query(`CREATE DATABASE ${db} OWNER ${name} TEMPLATE template0;`);
      query(`COMMENT ON DATABASE ${db} IS ${quote(marker)}; REVOKE CONNECT,TEMPORARY ON DATABASE ${db} FROM PUBLIC;`);
      current = read();
    }
    // A crash between CREATE DATABASE and its marker requires explicit recovery,
    // never silently adopting an unmarked existing database.
    validateDatabase(JSON.parse(current), name, marker);
  }
  return state;
}

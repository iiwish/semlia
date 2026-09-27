import { randomBytes, createHash } from 'node:crypto';
import { existsSync, lstatSync, mkdirSync, openSync, closeSync, readFileSync, writeFileSync, renameSync, unlinkSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
export { validateInventory } from '../../demo/environment.mjs';
export { validateRole, validateDatabase } from '../../demo/provision.mjs';

export const digest = value => 'sha256:' + createHash('sha256').update(value).digest('hex');
export const validOwner = value => /^semlia_v1_[a-f0-9]{16}$/.test(value);

export function safeFailureMessage(error) {
  if (error?.code === 'EEXIST') return 'V1 operation already owned; refusing concurrent initialization.';
  return 'V1 acceptance failed; inspect protected receipts without disclosing configuration.';
}
export function fixtureDigest(fixture) {
  const identities = new Map();
  function canonical(value) {
    if (Array.isArray(value)) return value.map(canonical);
    if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map(key => [key, canonical(value[key])]));
    if (typeof value === 'string' && /^(?:[a-z]+_[0-7][0-9a-hjkmnp-tv-z]{25}|[a-f0-9]{8}-[a-f0-9]{4}-7[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12})$/.test(value)) {
      if (!identities.has(value)) identities.set(value, `identity:${identities.size}:${value.includes('_') ? value.split('_')[0] : 'uuid'}`);
      return identities.get(value);
    }
    return value;
  }
  return digest('v1-fixture/2\n' + JSON.stringify(canonical(fixture)));
}
export function verifyFixture(state, saved, generated) {
  const current = fixtureDigest(generated);
  if (fixtureDigest(saved) !== current || (state.fixtureDigest && state.fixtureDigest !== current)) throw new Error('Synthetic knowledge or data fixture drift');
  return current;
}
export function assertSelfReviewProof(proof) {
  if (!proof?.hasReviewCapability || proof.status !== 403 || proof.code !== 'SOD_CONFLICT') throw new Error('Independent review proof required');
}

export function rejectLinks(path) {
  for (let current = resolve(path); ; current = dirname(current)) {
    if (existsSync(current) && lstatSync(current).isSymbolicLink()) throw new Error('Refusing symbolic path');
    if (dirname(current) === current) break;
  }
}
export function readPrivate(path) {
  rejectLinks(path);
  const info = lstatSync(path);
  if (!info.isFile() || (info.mode & 0o077)) throw new Error('Protected file must be private');
  return JSON.parse(readFileSync(path, 'utf8'));
}
export function writePrivate(path, value, exclusive = false) {
  rejectLinks(path);
  const temp = `${path}.${randomBytes(8).toString('hex')}.tmp`;
  if (exclusive) { writeFileSync(path, JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 }); return; }
  try {
    writeFileSync(temp, JSON.stringify(value, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
    renameSync(temp, path);
  } finally { if (existsSync(temp)) unlinkSync(temp); }
}
export function readState(root) {
  rejectLinks(root);
  if (!existsSync(join(root, 'state.json'))) throw new Error('Refusing unowned directory');
  if (!lstatSync(root).isDirectory() || (lstatSync(root).mode & 0o077)) throw new Error('Owner directory must be private');
  const state = readPrivate(join(root, 'state.json'));
  if (state.format !== 1 || !validOwner(state.owner) || !Number.isSafeInteger(state.revision) || state.revision < 0 || typeof state.digest !== 'string') throw new Error('Invalid owner record');
  return state;
}
export function openState(root, inputDigest) {
  rejectLinks(root);
  if (existsSync(root)) {
    const state = readState(root);
    if (state.digest !== inputDigest) throw new Error('Input digest mismatch');
    return state;
  }
  mkdirSync(root, { mode: 0o700 });
  const state = { format: 1, owner: 'semlia_v1_' + randomBytes(8).toString('hex'), digest: inputDigest, revision: 0 };
  writePrivate(join(root, 'state.json'), state, true);
  return state;
}
export function checkpoint(root, previous, update) {
  if (['format', 'owner', 'digest', 'revision'].some(key => Object.hasOwn(update, key))) throw new Error('Cannot replace state identity');
  const lock = join(root, 'checkpoint.lock');
  const fd = openSync(lock, 'wx', 0o600);
  try {
    const current = readState(root);
    if (current.owner !== previous.owner || current.revision !== previous.revision || current.digest !== previous.digest) throw new Error('Refusing stale checkpoint');
    const next = { ...current, ...update, revision: current.revision + 1 };
    writePrivate(join(root, 'state.json'), next);
    return next;
  } finally { closeSync(fd); unlinkSync(lock); }
}

export function runtimeEnvironment(repo, root, state, secrets, parent = process.env) {
  if (!validOwner(state.owner)) throw new Error('Invalid V1 owner');
  for (const port of [state.database.port, state.apiPort, state.webPort]) if (!Number.isInteger(port) || port < 1024 || port > 65535) throw new Error('Invalid V1 port');
  if (state.apiPort === state.webPort) throw new Error('V1 ports must differ');
  for (const name of ['databasePassword', 'readerPassword', 'secretKey']) if (!/^[a-f0-9]{64}$/.test(secrets[name])) throw new Error('Invalid protected secret');
  const env = Object.fromEntries(['PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'].filter(key => parent[key]).map(key => [key, parent[key]]));
  const source = state.workspace && state.sourceId ? [{ workspaceId: state.workspace.id, sourceId: state.sourceId, dsnEnv: 'SEMLIA_EXECUTION_DSN_V1' }] : [];
  return {
    ...env, SEMLIA_ENV: 'development', SEMLIA_AUTH_MODE: 'password', SEMLIA_LOCAL_UAT_IDENTITIES: 'false',
    SEMLIA_HTTP_ADDR: `127.0.0.1:${state.apiPort}`,
    SEMLIA_ALLOWED_ORIGINS: `http://127.0.0.1:${state.apiPort},http://127.0.0.1:${state.webPort}`,
    SEMLIA_DATABASE_URL: `postgresql://${state.owner}:${secrets.databasePassword}@127.0.0.1:${state.database.port}/${state.owner}_app?sslmode=disable`,
    SEMLIA_SECRET_KEY: secrets.secretKey, SEMLIA_GIT_REPOSITORY: join(root, 'content'),
    SEMLIA_SOURCE_ARTIFACT_ROOT: join(root, 'inputs'), SEMLIA_ARTIFACT_ROOT: join(root, 'artifacts'),
    SEMLIA_ARTIFACT_STORE: 'local', SEMLIA_WORKER_CONFIGURED: 'true', SEMLIA_MIGRATIONS_PATH: join(repo, 'migrations'),
    SEMLIA_SEMANTIC_PRODUCTION_ENABLED: 'true', SEMLIA_PRODUCTION_GENERATION_GRANTS: '[]',
    SEMLIA_EXECUTION_ALLOW_PLAINTEXT: 'true', SEMLIA_DISCOVERY_ALLOW_UNSAFE_SOURCE: 'false',
    SEMLIA_EXECUTION_SOURCES: JSON.stringify(source),
    SEMLIA_EXECUTION_DSN_V1: `postgresql://${state.owner}_reader:${secrets.readerPassword}@127.0.0.1:${state.database.port}/${state.owner}_source?sslmode=disable`,
    SEMLIA_V1_MODEL_SECRET: secrets.modelSecret ?? '', SEMLIA_VITE_API_TARGET: `http://127.0.0.1:${state.apiPort}`,
    VITE_CATALOG_FIXTURE: '', VITE_LOCAL_UAT_IDENTITIES: '',
  };
}

export function goldenCases() {
  const from = "'2026-08-01T00:00:00Z'::timestamptz", to = "'2026-09-01T00:00:00Z'::timestamptz";
  const where = `o.is_valid=1 AND o.paid_at>=${from} AND o.paid_at<${to}`;
  return [
    { id: 'total', question: '2026年8月的有效支付金额是多少？', sql: `SELECT SUM(o.amount)::text FROM semlia_demo_202609.orders o WHERE ${where}`, expected: [['1500']] },
    { id: 'region', question: '2026年8月按客户当前区域分别统计有效支付金额。', sql: `SELECT c.region,SUM(o.amount)::text FROM semlia_demo_202609.orders o JOIN semlia_demo_202609.customers c ON c.id=o.customer_id WHERE ${where} GROUP BY c.region ORDER BY c.region COLLATE "C"`, expected: [['华东', '1000'], ['华南', '500']] },
    { id: 'monthly', question: '对比2026年7月和8月的有效支付金额，按月列出。', sql: "SELECT to_char(o.paid_at AT TIME ZONE 'UTC','YYYY-MM'),SUM(o.amount)::text FROM semlia_demo_202609.orders o WHERE o.is_valid=1 AND o.paid_at>='2026-07-01T00:00:00Z'::timestamptz AND o.paid_at<'2026-09-01T00:00:00Z'::timestamptz GROUP BY 1 ORDER BY 1", expected: [['2026-07', '600'], ['2026-08', '1500']] },
    { id: 'old-customer-average', question: '2026年8月华东地区老客户的客单价是多少？', sql: `SELECT (SUM(o.amount)/NULLIF(COUNT(DISTINCT o.id),0))::text FROM semlia_demo_202609.orders o JOIN semlia_demo_202609.customers c ON c.id=o.customer_id WHERE ${where} AND c.region='华东' AND c.first_paid_at<${from}`, expected: [['200']] },
  ];
}

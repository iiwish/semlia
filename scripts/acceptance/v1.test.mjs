import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { spawn, spawnSync } from 'node:child_process';
import { once } from 'node:events';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { checkpoint, openState, runtimeEnvironment, validateInventory, validateRole, validateDatabase, goldenCases, safeFailureMessage, fixtureDigest, verifyFixture, assertSelfReviewProof } from './v1/core.mjs';
import { selectConfiguredModel, assertOwnedResources, assertReaderPrivacy, readerRoleSettings } from './v1/provision.mjs';
import { requestRuntimeControl, fetchRuntime } from './v1/runtime.mjs';

test('runtime transport survives a real synchronous child after the server expires idle sockets', async () => {
  const directory = mkdtempSync(join(realpathSync(tmpdir()), 'v1-socket-'));
  const socket = join(directory, 'control.sock');
  const server = spawn(process.execPath, ['--input-type=module', '-e', `
    import { createServer } from 'node:http';
    const handler = (req, res) => { res.writeHead(200, { 'Content-Type': 'text/html' }); res.end('<div id="root"></div>'); };
    const control = createServer(handler), http = createServer(handler);
    for (const server of [control, http]) { server.keepAliveTimeout = 2000; server.keepAliveTimeoutBuffer = 0; }
    await new Promise(resolve => control.listen(process.argv[1], resolve));
    await new Promise(resolve => http.listen(0, '127.0.0.1', resolve));
    console.log(JSON.stringify({ port: http.address().port }));
    process.on('SIGTERM', () => { control.closeAllConnections(); http.closeAllConnections(); control.close(); http.close(); });
  `, socket], { stdio: ['ignore', 'pipe', 'pipe'] });
  try {
    const [line] = await once(server.stdout, 'data');
    const url = `http://127.0.0.1:${JSON.parse(line).port}/`;
    await requestRuntimeControl(socket, 'status', 'synthetic-token');
    assert.equal(await (await fetchRuntime(url)).text(), '<div id="root"></div>');
    const child = spawnSync(process.execPath, ['-e', 'setTimeout(() => {}, 2500)'], { timeout: 5000 });
    assert.equal(child.status, 0);
    await requestRuntimeControl(socket, 'status', 'synthetic-token');
    assert.equal(await (await fetchRuntime(url)).text(), '<div id="root"></div>');
  } finally {
    server.kill('SIGTERM'); await once(server, 'close');
    rmSync(directory, { recursive: true, force: true });
  }
});

const repo = '/owned/semlia';
const inventory = () => ({ Id: 'a'.repeat(64), Config: { Labels: { 'com.docker.compose.project': 'semlia-local', 'com.docker.compose.service': 'postgres', 'com.docker.compose.project.working_dir': repo } }, State: { Running: true }, NetworkSettings: { Ports: { '5432/tcp': [{ HostIp: '127.0.0.1', HostPort: '5433' }] } } });
test('rejects unknown ownership, public database binding and unsafe roles', () => {
  assert.deepEqual(validateInventory(inventory(), repo), { container: 'a'.repeat(64), port: 5433 });
  for (const mutate of [x => x.Config.Labels['com.docker.compose.project'] = 'other', x => x.Config.Labels['com.docker.compose.project.working_dir'] = '/other', x => x.NetworkSettings.Ports['5432/tcp'][0].HostIp = '0.0.0.0', x => x.State.Running = false]) {
    const value = inventory(); mutate(value); assert.throws(() => validateInventory(value, repo));
  }
  const role = { marker: 'owner:digest', login: true, superuser: false, createdb: false, createrole: false, replication: false, bypassrls: false, memberships: 0 };
  validateRole(role, 'owner:digest');
  assert.throws(() => validateRole({ ...role, superuser: true }, 'owner:digest'));
  assert.throws(() => validateRole(role, 'another-owner'));
  validateDatabase({ owner: 'role', marker: 'owner:digest' }, 'role', 'owner:digest');
  assert.throws(() => validateDatabase({ owner: 'role', marker: null }, 'role', 'owner:digest'));
});

test('runtime environment does not inherit unrelated secrets or external resources', () => {
  const state = { owner: 'semlia_v1_0123456789abcdef', database: { port: 5433 }, apiPort: 18000, webPort: 18001 };
  const secrets = { databasePassword: 'a'.repeat(64), readerPassword: 'b'.repeat(64), secretKey: 'c'.repeat(64), modelSecret: 'private-model' };
  const env = runtimeEnvironment(repo, '/owned/run', state, secrets, { PATH: '/bin', HOME: '/home/test', COMPOSE_FILE: '/private', SEMLIA_DATABASE_URL: 'existing', SEMLIA_EXECUTION_SOURCES: '["existing"]', ANOTHER_API_KEY: 'do-not-copy' });
  assert.equal(env.SEMLIA_AUTH_MODE, 'password');
  assert.equal(env.SEMLIA_LOCAL_UAT_IDENTITIES, 'false');
  assert.equal(env.SEMLIA_DISCOVERY_ALLOW_UNSAFE_SOURCE, 'false');
  assert.equal(env.SEMLIA_V1_MODEL_SECRET, 'private-model');
  assert.equal(env.COMPOSE_FILE, undefined);
  assert.equal(env.ANOTHER_API_KEY, undefined);
  assert.equal(env.SEMLIA_EXECUTION_SOURCES, '[]');
  const configured = runtimeEnvironment(repo, '/owned/run', { ...state, workspace: { id: 'workspace' }, sourceId: 'source' }, secrets);
  const source = JSON.parse(configured.SEMLIA_EXECUTION_SOURCES)[0];
  assert.match(source.dsnEnv, /^SEMLIA_EXECUTION_DSN_[A-Z0-9_]+$/);
  assert.ok(configured[source.dsnEnv]);
  assert.ok(new URL(env.SEMLIA_DATABASE_URL).pathname.endsWith('_app'));
  assert.throws(() => runtimeEnvironment(repo, '/owned/run', { ...state, owner: 'semlia' }, secrets));
});

test('initialization resumes the same owner, refuses foreign roots and stale checkpoints', () => {
  const parent = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-v1-test-'));
  try {
    const root = join(parent, 'run'), state = openState(root, 'sha256:fixture');
    assert.deepEqual(openState(root, 'sha256:fixture'), state);
    const updated = checkpoint(root, state, { phase: 'provisioned' });
    assert.equal(updated.revision, state.revision + 1);
    assert.throws(() => checkpoint(root, state, { phase: 'stale' }));
    assert.throws(() => openState(root, 'sha256:different'));
    assert.throws(() => checkpoint(root, updated, { owner: 'replacement' }));
    const foreign = join(parent, 'foreign'); mkdirSync(foreign);
    assert.throws(() => openState(foreign, 'sha256:fixture'));
    symlinkSync(root, join(parent, 'linked')); assert.throws(() => openState(join(parent, 'linked'), 'sha256:fixture'));
    assert.equal(JSON.parse(readFileSync(join(root, 'state.json'))).owner, state.owner);
  } finally { rmSync(parent, { recursive: true, force: true }); }
});

test('independent SQL golden covers total, regional breakdown, time buckets and old-customer ratio', () => {
  const cases = goldenCases();
  assert.deepEqual(cases.map(x => x.expected), [[['1500']], [['华东', '1000'], ['华南', '500']], [['2026-07', '600'], ['2026-08', '1500']], [['200']]]);
  assert.ok(cases.every(x => x.sql.startsWith('SELECT ') && !/INSERT|UPDATE|DELETE|DROP/i.test(x.sql)));
  assert.equal(new Set(cases.map(x => x.id)).size, 4);
  assert.deepEqual(goldenCases(), cases);
});

test('model reuse refuses ambiguity and pins an explicitly selected workspace', () => {
  const choices = [{ slug: 'one', model: 'model-a' }, { slug: 'two', model: 'model-b' }];
  assert.throws(() => selectConfiguredModel(choices));
  assert.throws(() => selectConfiguredModel(choices, 'unknown'));
  assert.deepEqual(selectConfiguredModel(choices, 'two'), choices[1]);
  assert.deepEqual(selectConfiguredModel([choices[0]]), choices[0]);
});

test('damaged private JSON cannot expose its input through CLI diagnostics', () => {
  const secret = 'fake-test-secret-should-never-print';
  let error;
  try { JSON.parse(`{\"private\":\"${secret}\" broken}`); } catch (caught) { error = caught; }
  assert.ok(error);
  assert.ok(!safeFailureMessage(error).includes(secret));
  assert.equal(safeFailureMessage(new Error(secret)), 'V1 acceptance failed; inspect protected receipts without disclosing configuration.');
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-v1-private-test-'));
  try {
    const path = join(root, 'secrets.json');
    writeFileSync(path, `{"private":"${secret}" broken}`, { mode: 0o600 });
    const source = `import {readPrivate,safeFailureMessage} from ${JSON.stringify(new URL('./v1/core.mjs', import.meta.url).href)}; try {readPrivate(process.argv[1]);} catch(error) {console.error(safeFailureMessage(error));process.exitCode=1;}`;
    const result = spawnSync(process.execPath, ['--input-type=module', '-e', source, path], { encoding: 'utf8' });
    assert.equal(result.status, 1);
    assert.equal(result.stdout, '');
    assert.equal(result.stderr.trim(), safeFailureMessage(error));
    assert.ok(!result.stderr.includes(secret));
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('runtime revalidation rejects role privilege and database ownership drift', () => {
  const state = { owner: 'semlia_v1_0123456789abcdef', digest: 'sha256:fixture' };
  const marker = state.owner + ':' + state.digest;
  const role = { marker, login: true, superuser: false, createdb: false, createrole: false, replication: false, bypassrls: false, memberships: 0 };
  const db = { owner: state.owner, marker };
  assertOwnedResources(state, [role, role], [db, db]);
  assert.throws(() => assertOwnedResources(state, [{ ...role, createdb: true }, role], [db, db]));
  assert.throws(() => assertOwnedResources(state, [role, role], [{ ...db, marker: 'foreign' }, db]));
  assert.throws(() => assertOwnedResources(state, [role, role], [{ ...db, owner: 'foreign' }, db]));
  assert.throws(() => assertOwnedResources(state, [role], [db, db]));
});

test('fixture fingerprint ignores random identities but rejects changed knowledge with identical SQL', () => {
  const fixture = { DataSQL: 'SELECT 1;', Snapshot: { Assets: [{ AssetID: 'ast_01m3ev2rhdfw29emz0hz8j0bzm', Content: { spec: { aggregation: 'sum' } } }] } };
  const regenerated = structuredClone(fixture);
  regenerated.Snapshot.Assets[0].AssetID = 'ast_01m3ev2cn8fyaab9c5d3kzfh8z';
  assert.equal(fixtureDigest(fixture), fixtureDigest(regenerated));
  assert.equal(verifyFixture({ fixtureDigest: fixtureDigest(fixture) }, fixture, regenerated), fixtureDigest(fixture));
  regenerated.Snapshot.Assets[0].Content.spec.aggregation = 'count_distinct';
  assert.throws(() => verifyFixture({}, fixture, regenerated));
  assert.throws(() => verifyFixture({ fixtureDigest: 'sha256:wrong' }, fixture, fixture));
});

test('self-review proof requires review capability and the stable independence rejection', () => {
  const proof = { hasReviewCapability: true, status: 403, code: 'SOD_CONFLICT' };
  assertSelfReviewProof(proof);
  assert.throws(() => assertSelfReviewProof({ ...proof, hasReviewCapability: false }));
  assert.throws(() => assertSelfReviewProof({ ...proof, code: 'FORBIDDEN' }));
  assert.throws(() => assertSelfReviewProof({ ...proof, status: 409 }));
});

test('reader provisioning and runtime recovery require role-scoped private error logging', () => {
  assert.deepEqual(readerRoleSettings, { default_transaction_read_only: 'on', log_min_error_statement: 'panic' });
  assertReaderPrivacy(['default_transaction_read_only=on', 'log_min_error_statement=panic']);
  assert.throws(() => assertReaderPrivacy(['default_transaction_read_only=on']));
  assert.throws(() => assertReaderPrivacy(['default_transaction_read_only=on', 'log_min_error_statement=error']));
});

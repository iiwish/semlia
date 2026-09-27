import assert from 'node:assert/strict';
import { existsSync, readFileSync, writeFileSync, openSync, closeSync, lstatSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { root, repo, down, up, status } from './runtime.mjs';
import { readState, readPrivate, writePrivate, rejectLinks, digest } from './core.mjs';
import { releaseRoot, readPlan, inspectTarget, privateDirectory, databaseFingerprint, withTargetServer } from './release-environment.mjs';
import { fileInventory, copyStore } from './release-storage.mjs';
import { assertBackupInventory } from './release-core.mjs';
import { connect } from './initialize.mjs';
import { counters } from './model.mjs';
import { governanceHeadProof } from './correction.mjs';
import { verifyProvisioned, preservationSnapshot } from './provision.mjs';

const fixtureText = '-- V1 synthetic backup/restore fixture. Not production business data.\nCREATE TABLE synthetic_restore_evidence (id bigint PRIMARY KEY, label text NOT NULL);\n';
export async function prepareArtifacts(markdown = false) {
  const plan = readPlan(), state = readState(root), receipt = join(releaseRoot, 'artifact-fixture.json');
  if (markdown) {
    const prior = readPrivate(receipt); assert.equal(prior.owner, plan.owner); assert.equal(prior.state, 'failed'); assert.equal(prior.status, 422); assert.ok(prior.artifactSet?.id);
    writePrivate(join(releaseRoot, 'artifact-fixture-sql-unsupported.json'), prior, true);
  } else {
    assert.ok(!existsSync(receipt), 'Artifact fixture already has a receipt');
    const path = join(root, 'inputs', 'synthetic-restore.sql'); rejectLinks(path);
    writeFileSync(path, fixtureText, { flag: 'wx', mode: 0o600 });
  }
  const text = markdown ? '# V1 Synthetic Restore Evidence\n\nThis is generated acceptance data, not a production business definition.\n\nRestore verification must preserve these exact bytes.\n' : fixtureText;
  const { client } = await connect(root, state, 'admin'), base = `/api/v1/workspaces/${state.workspace.id}`;
  const ledger = { owner: plan.owner, state: 'started', kind: markdown ? 'markdown' : 'sql', contentDigest: digest(text), before: counters(root) }; writePrivate(receipt, ledger, !markdown);
  try {
    let set;
    if (markdown) {
      const origin = `http://127.0.0.1:${state.apiPort}`, cookie = client.sessionCookie();
      const session = await fetch(origin + '/api/v1/session', { headers: { Cookie: cookie }, redirect: 'error', signal: AbortSignal.timeout(30000) }); assert.ok(session.ok);
      const csrf = session.headers.get('X-Semlia-CSRF'); assert.ok(csrf); await session.body?.cancel();
      const response = await fetch(origin + base + '/ingestion/artifacts', { method: 'POST', headers: { Origin: origin, Cookie: cookie, 'X-Semlia-CSRF': csrf, 'Content-Type': 'text/markdown', 'X-Artifact-Kind': 'markdown', 'X-File-Name': 'synthetic-restore.md', 'Idempotency-Key': plan.owner + '-restore-markdown-upload' }, body: text, redirect: 'error', signal: AbortSignal.timeout(30000) });
      assert.equal(response.status, 201); const artifact = await response.json(); ledger.artifactId = artifact.id; writePrivate(receipt, ledger);
      set = await client.request('POST', base + '/ingestion/artifact-sets:finalize', { sourceName: 'V1 synthetic restore preview evidence', artifactIds: [artifact.id] }, plan.owner + '-restore-markdown-finalize');
    } else set = await client.request('POST', base + '/ingestion/sql-registrations', { sourceName: 'V1 synthetic backup restore artifact', paths: ['synthetic-restore.sql'] }, plan.owner + '-restore-artifact');
    ledger.artifactSet = set; writePrivate(receipt, ledger);
    assert.equal(set.members.length, 1);
    const previewPath = base + `/ingestion/artifact-sets/${set.id}/members/${set.members[0].artifactId}/preview`;
    const preview = await client.request('GET', previewPath);
    assert.equal(preview.contentDigest, digest(text)); assert.equal(digest(preview.text), digest(text)); assert.equal(preview.truncated, false);
    ledger.previewPath = previewPath; ledger.previewDigest = digest(JSON.stringify(preview));
    ledger.after = counters(root); assert.deepEqual(ledger.after, ledger.before);
    ledger.state = 'passed'; writePrivate(receipt, ledger); return { state: ledger.state, setId: set.id, sourceId: set.sourceId, contentBytes: Buffer.byteLength(text), modelCalls: 0 };
  } catch (error) { ledger.state = 'failed'; ledger.status = Number.isInteger(error.status) ? error.status : undefined; writePrivate(receipt, ledger); throw error; }
}

function dockerFile(command, args, path, writing) {
  rejectLinks(path); const fd = openSync(path, writing ? 'wx' : 'r', 0o600);
  try {
    const result = spawnSync('docker', ['exec', ...(writing ? [] : ['-i']), ...args.slice(0, 1), command, ...args.slice(1)], { cwd: repo, env: { PATH: process.env.PATH, HOME: process.env.HOME }, stdio: writing ? ['ignore', fd, 'pipe'] : [fd, 'pipe', 'pipe'], timeout: 240000, maxBuffer: 2 * 1024 * 1024 });
    assert.ok(!result.error && result.status === 0 && result.signal === null, 'Protected PostgreSQL backup/restore command failed');
  } finally { closeSync(fd); }
}

export async function backupRestore() {
  const plan = readPlan(), state = readState(root), receipt = join(releaseRoot, 'backup-restore.json');
  assert.ok(!existsSync(receipt), 'Backup/restore attempt exists; inspect instead of replaying');
  inspectTarget(plan, 'restore', true);
  const fixture = readPrivate(join(releaseRoot, 'artifact-fixture.json')); assert.equal(fixture.owner, plan.owner); assert.equal(fixture.state, 'passed');
  const head = await governanceHeadProof(root); assert.ok(head.correctBaselineContent);
  const runtime = await status(); assert.equal(runtime.mode, 'source');
  const ledger = { owner: plan.owner, state: 'started', stage: 'preflight', startedAt: new Date().toISOString(), head, before: counters(root), runtimeMode: runtime.mode, candidate: false };
  assert.equal(ledger.before.unfinished, 0); writePrivate(receipt, ledger, true);
  const save = () => writePrivate(receipt, ledger), backup = join(releaseRoot, 'backup'); privateDirectory(backup);
  let stopped = false;
  try {
    ledger.stage = 'freeze-owned-runtime'; save(); await down(); stopped = true;
    verifyProvisioned(repo, state); assert.ok(!existsSync(join(root, 'runtime-owner.json')));
    ledger.stage = 'inventory'; save();
    const recovery = readPrivate(join(releaseRoot, 'secrets.json')).recovery;
    assert.deepEqual(recovery, readPrivate(join(root, 'secrets.json')));
    const inventory = { content: fileInventory(join(root, 'content')), inputs: fileInventory(join(root, 'inputs')), artifacts: fileInventory(join(root, 'artifacts')), keys: Object.fromEntries(['secretKey', 'accountPassword', 'readerPassword', 'modelSecret'].map(key => [key, typeof recovery[key] === 'string' && recovery[key].length > 0])) };
    assertBackupInventory(inventory); ledger.inventory = inventory;
    ledger.databaseFingerprint = databaseFingerprint(plan, state.owner + '_app');
    ledger.sourceFingerprint = databaseFingerprint(plan, state.owner + '_source', undefined, 'semlia_demo_202609');
    assert.deepEqual(ledger.sourceFingerprint.map(row => [row.table, row.count]), [['customers', 4], ['orders', 7]]); save();
    ledger.stage = 'dump-and-copy'; save();
    const dump = join(backup, 'application.dump');
    dockerFile('pg_dump', [plan.database.container, '-U', 'semlia', '--format=custom', '--no-owner', '--no-acl', state.owner + '_app'], dump, true);
    ledger.dump = { digest: digest(readFileSync(dump)), bytes: lstatSync(dump).size }; assert.ok(ledger.dump.bytes > 0);
    for (const name of ['content', 'inputs', 'artifacts']) copyStore(join(root, name), join(backup, name), inventory[name]);
    writePrivate(join(backup, 'recovery-keys.json'), { owner: plan.owner, sourceOwner: state.owner, ...recovery }, true);
    ledger.keyFileDigest = digest(readFileSync(join(backup, 'recovery-keys.json')));
    ledger.sessionPolicy = 'DB session rows retained by full snapshot; no browser cookie files copied or reused. Restore uses a fresh normal password login.';
    ledger.scope = 'Application DB plus content/inputs/artifacts/key material; external synthetic source DB is not part of backup and remains static. Query result rows are ephemeral.'; save();
    ledger.stage = 'restart-original-owned-runtime'; save(); await up(); stopped = false;
    ledger.runtimeRestored = true; save();
    const target = inspectTarget(plan, 'restore', true);
    ledger.stage = 'restore-files'; save();
    assert.equal(digest(readFileSync(dump)), ledger.dump.digest);
    for (const name of ['content', 'inputs', 'artifacts']) copyStore(join(backup, name), join(target.root, name), inventory[name]);
    const recovered = readPrivate(join(backup, 'recovery-keys.json')); assert.equal(recovered.owner, plan.owner); assert.equal(recovered.sourceOwner, state.owner);
    assert.equal(digest(readFileSync(join(backup, 'recovery-keys.json'))), ledger.keyFileDigest);
    writePrivate(join(target.root, 'recovery-keys.json'), recovered, true);
    ledger.stage = 'restore-database'; save(); inspectTarget(plan, 'restore', true);
    dockerFile('pg_restore', [plan.database.container, '-U', 'semlia', '--role=' + target.role, '--no-owner', '--no-acl', '--exit-on-error', '--single-transaction', '-d', target.database], dump, false);
    inspectTarget(plan, 'restore');
    const restoredFingerprint = databaseFingerprint(plan, target.database); assert.deepEqual(restoredFingerprint, ledger.databaseFingerprint);
    ledger.fingerprintVerified = true; ledger.stage = 'restored-normal-api'; save();
    ledger.restored = await withTargetServer(plan, 'restore', async client => {
      const session = await client.login('v1_admin', recovered.accountPassword);
      assert.equal(session.workspaces.length, 1); assert.equal(session.workspaces[0].id, state.workspace.id);
      const base = `/api/v1/workspaces/${state.workspace.id}`;
      const assets = await client.request('GET', base + '/catalog/assets?limit=100'); assert.equal(assets.items.length, 10);
      const release = await client.request('GET', base + '/production-releases/' + head.currentRelease);
      const pin = release.afterManifest.assets.find(item => item.assetId === state.published['demo_202609.model'].targetId);
      assert.equal(pin.revisionId, head.currentRevision);
      const model = await client.request('GET', base + `/catalog/assets/${pin.assetId}/revisions/${pin.revisionId}`); assert.ok(model.content.spec);
      const preview = await client.request('GET', fixture.previewPath);
      assert.equal(preview.contentDigest, fixture.contentDigest); assert.equal(digest(preview.text), fixture.contentDigest); assert.equal(digest(JSON.stringify(preview)), fixture.previewDigest);
      const connection = await client.request('POST', base + `/sources/${state.sourceId}/test`, {}); assert.equal(connection.status, 'succeeded');
      return { normalPasswordLogin: true, assets: assets.items.length, releaseId: release.id, modelRevision: pin.revisionId, manifestDigest: release.afterManifest.digest, artifactDigest: preview.contentDigest, artifactPreviewMatched: true, credentialDecryptSourceTest: connection.status, executionSourcesConfigured: false };
    });
    assert.deepEqual(databaseFingerprint(plan, state.owner + '_source', undefined, 'semlia_demo_202609'), ledger.sourceFingerprint);
    assert.deepEqual(preservationSnapshot(repo, plan.database), plan.preservation);
    ledger.after = counters(root); assert.deepEqual(ledger.after, ledger.before);
    const final = await governanceHeadProof(root); assert.equal(final.currentRelease, head.currentRelease); assert.ok(final.correctBaselineContent);
    ledger.state = 'passed'; ledger.stage = 'complete'; ledger.finishedAt = new Date().toISOString(); save();
    return { state: ledger.state, dumpBytes: ledger.dump.bytes, stores: Object.fromEntries(['content', 'inputs', 'artifacts'].map(name => [name, { files: inventory[name].length, bytes: inventory[name].reduce((sum, item) => sum + item.bytes, 0) }])), fingerprintTables: ledger.databaseFingerprint.length, restored: ledger.restored, runtimeRestored: ledger.runtimeRestored, modelCalls: 0 };
  } catch (error) {
    ledger.state = 'failed'; ledger.status = Number.isInteger(error.status) ? error.status : undefined; save(); throw error;
  } finally {
    if (stopped) {
      try { await up(); ledger.runtimeRestored = true; save(); } catch { ledger.runtimeRestored = false; save(); }
    }
  }
}

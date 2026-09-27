import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, writeFileSync, rmSync, symlinkSync, realpathSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { digest } from './v1/core.mjs';
import { verifyMigrationInventory, withStartupLock, assertRequestedRuntime, verifyCandidate } from './v1/release-candidate.mjs';
import { fileInventory, copyStore, containedPath } from './v1/release-storage.mjs';
import { assertRestoreTarget, assertBackupInventory, assertCandidateMetadata, compareChannelProofs, assertCLIOutcome } from './v1/release-core.mjs';

test('restore writes require exact task database, owner, marker, role boundaries and empty target', () => {
  const expected = { database: 'semlia_v1r_0123456789abcdef_restore', role: 'semlia_v1r_0123456789abcdef_restore_owner', marker: 'owned-marker' };
  const actual = { ...expected, empty: true, superuser: false, createdb: false, createrole: false, replication: false, bypassrls: false, login: true, memberships: 0 };
  assertRestoreTarget(actual, expected);
  for (const change of [{ database: 'semlia' }, { role: 'postgres' }, { marker: 'unrelated' }, { empty: false }, { superuser: true }, { memberships: 1 }]) assert.throws(() => assertRestoreTarget({ ...actual, ...change }, expected));
});

test('candidate migration inventory rejects extra files, changed bytes and links', () => {
  const dir = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-release-'));
  try {
    const file = { name: '000033_test.up.sql', digest: digest('SELECT 1;') };
    writeFileSync(join(dir, file.name), 'SELECT 1;', { mode: 0o600 });
    verifyMigrationInventory(dir, [file]);
    writeFileSync(join(dir, '000034_extra.up.sql'), 'SELECT 2;');
    assert.throws(() => verifyMigrationInventory(dir, [file]));
    rmSync(join(dir, '000034_extra.up.sql'));
    writeFileSync(join(dir, file.name), 'SELECT 2;');
    assert.throws(() => verifyMigrationInventory(dir, [file]));
    rmSync(join(dir, file.name)); symlinkSync('/etc/hosts', join(dir, file.name));
    assert.throws(() => verifyMigrationInventory(dir, [file]));
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('startup lock rejects concurrent launch and checks requested ready identity', async () => {
  const dir = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-startup-'));
  let release; const barrier = new Promise(resolve => { release = resolve; });
  try {
    const first = withStartupLock(dir, async () => { await barrier; return 'ready'; });
    await assert.rejects(withStartupLock(dir, async () => 'wrong'));
    release(); assert.equal(await first, 'ready');
    assert.equal(await withStartupLock(dir, async () => 'next'), 'next');
    assertRequestedRuntime({ mode: 'source' }, 'source');
    assert.throws(() => assertRequestedRuntime({ mode: 'source' }, 'candidate', { binaryDigest: 'b', manifestDigest: 'm' }));
    assert.throws(() => assertRequestedRuntime({ mode: 'candidate', activeBinary: { binaryDigest: 'wrong', manifestDigest: 'm' } }, 'candidate', { binaryDigest: 'b', manifestDigest: 'm' }));
  } finally { release(); rmSync(dir, { recursive: true, force: true }); }
});

test('backup paths cannot escape or follow links and restore compares actual content bytes', () => {
  const dir = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-storage-'));
  try {
    writeFileSync(join(dir, 'synthetic.sql'), 'SELECT 1;', { mode: 0o600 });
    const expected = fileInventory(dir), dest = dir + '-copy';
    try {
      for (const path of ['../escape', '/absolute', 'a/../b', 'a\\b']) assert.throws(() => containedPath(dir, path));
      copyStore(dir, dest, expected); assert.deepEqual(fileInventory(dest), expected);
      assert.throws(() => copyStore(dir, dest, expected));
      writeFileSync(join(dir, 'synthetic.sql'), 'SELECT 2;');
      assert.throws(() => copyStore(dir, dest + '-other', expected));
      symlinkSync('/etc/hosts', join(dir, 'link')); assert.throws(() => fileInventory(dir));
    } finally { rmSync(dest, { recursive: true, force: true }); }
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('full backups require nonempty content, input and artifact stores and protected recovery keys', () => {
  const inventory = { content: [{ path: 'catalog/item.json', bytes: 10, digest: 'a' }], inputs: [{ path: 'synthetic.sql', bytes: 10, digest: 'b' }], artifacts: [{ path: 'synthetic-artifact', bytes: 10, digest: 'c' }], keys: { secretKey: true, accountPassword: true, readerPassword: true, modelSecret: true } };
  assertBackupInventory(inventory);
  for (const name of ['content', 'inputs', 'artifacts']) assert.throws(() => assertBackupInventory({ ...inventory, [name]: [] }));
  assert.throws(() => assertBackupInventory({ ...inventory, keys: { ...inventory.keys, secretKey: false } }));
});

test('candidate metadata rejects schema, platform, source and provenance mismatch', () => {
  const expected = { os: 'darwin', arch: 'arm64', schema: 33, sourceDigest: 'sha256:' + 'a'.repeat(64) };
  const manifest = { formatVersion: 1, product: 'Semlia', version: '1.0.0-rc.1', commit: 'b'.repeat(40), target: { os: 'darwin', arch: 'arm64' }, executable: 'semlia', apiVersion: 'v1', migrationVersion: 33, sourceDigest: expected.sourceDigest, artifactKind: 'local_candidate', acceptance: 'unreviewed' };
  assertCandidateMetadata(manifest, expected);
  for (const change of [{ migrationVersion: 32 }, { sourceDigest: 'sha256:' + 'c'.repeat(64) }, { artifactKind: 'unknown' }, { executable: '../server' }, { target: { os: 'linux', arch: 'amd64' } }]) assert.throws(() => assertCandidateMetadata({ ...manifest, ...change }, expected));
});

test('candidate verification rejects missing and symbolic inputs before any build or launch', () => {
  const dir = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-candidate-'));
  try {
    const absent = join(dir, 'missing');
    assert.throws(() => verifyCandidate(dir, [absent, absent, absent, absent]));
    const target = join(dir, 'manifest.json'); writeFileSync(target, '{}');
    const link = join(dir, 'linked.json'); symlinkSync(target, link);
    assert.throws(() => verifyCandidate(dir, [link, absent, absent, absent]));
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('public channel proofs must agree on exact release/model/query and result digests', () => {
  const proof = { releaseId: 'release', model: { assetId: 'model', revisionId: 'revision' }, planDigest: 'plan', resultDigest: 'result' };
  compareChannelProofs([proof, structuredClone(proof), structuredClone(proof)]);
  for (const change of [{ releaseId: 'wrong' }, { model: { assetId: 'model', revisionId: 'other' } }, { planDigest: 'other' }, { resultDigest: 'other' }]) assert.throws(() => compareChannelProofs([proof, { ...proof, ...change }, proof]));
});

test('CLI revocation requires actual HTTP401 not timeout, spawn failure or usage error', () => {
  const denied = { exit: 1, spawned: true, signal: null, httpStatus: 401, body: { code: 'UNAUTHENTICATED' } };
  assertCLIOutcome(denied, true);
  for (const patch of [{ exit: null }, { exit: 2 }, { spawned: false }, { signal: 'SIGTERM' }, { httpStatus: null }, { body: {} }]) assert.throws(() => assertCLIOutcome({ ...denied, ...patch }, true));
  assertCLIOutcome({ exit: 0, spawned: true, signal: null }, false);
  assert.throws(() => assertCLIOutcome({ exit: 0, spawned: false, signal: null }, false));
});

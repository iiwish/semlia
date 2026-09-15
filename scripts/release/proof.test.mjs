import { test } from 'node:test';
import assert from 'node:assert/strict';
import { generateKeyPairSync, sign } from 'node:crypto';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { filesFor, statement, verifyProof } from './proof.mjs';

test('release proof binds files, identity and the trusted key', t => {
  const root = mkdtempSync(join(tmpdir(), 'semlia-proof-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const version = 'v0.1.0-rc.3', commit = 'a'.repeat(40);
  for (const file of filesFor(version)) {
    mkdirSync(dirname(join(root, file)), { recursive: true });
    writeFileSync(join(root, file), 'fixture');
  }
  const { publicKey, privateKey } = generateKeyPairSync('ed25519');
  const bytes = Buffer.from(JSON.stringify(statement(root, version, commit, 'https://github.com/iiwish/semlia/actions/runs/123')));
  const signature = sign(null, bytes, privateKey);
  assert.doesNotThrow(() => verifyProof(root, bytes, signature, publicKey, version, commit));
  assert.throws(() => verifyProof(root, bytes, signature, publicKey, version, 'b'.repeat(40)));
  assert.throws(() => verifyProof(root, bytes, signature, generateKeyPairSync('ed25519').publicKey, version, commit));
  assert.throws(() => verifyProof(root, Buffer.from('{}'), signature, publicKey, version, commit));
  writeFileSync(join(root, filesFor(version)[0]), 'tampered');
  assert.throws(() => verifyProof(root, bytes, signature, publicKey, version, commit));
  assert.throws(() => filesFor('../../escape'));
});

test('local deployment proof is explicitly local and fails on missing files', t => {
  const root = mkdtempSync(join(tmpdir(), 'semlia-proof-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const version = 'v0.1.0-rc.3', commit = 'b'.repeat(40);
  const run = 'urn:semlia:local-release:12345678-1234-1234-1234-123456789abc';
  assert.throws(() => statement(root, version, commit, run));
  for (const file of filesFor(version, true)) writeFileSync(join(root, file), 'fixture');
  const { publicKey, privateKey } = generateKeyPairSync('ed25519');
  const bytes = Buffer.from(JSON.stringify(statement(root, version, commit, run)));
  const signature = sign(null, bytes, privateKey);
  assert.doesNotThrow(() => verifyProof(root, bytes, signature, publicKey, version, commit));
  rmSync(join(root, 'image.tar'));
  assert.throws(() => verifyProof(root, bytes, signature, publicKey, version, commit));
});

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, realpathSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { seedSource, verifySource } from './source.mjs';

const state = { owner: 'semlia_demo_0123456789abcdef', digest: 'sha256:' + 'a'.repeat(64), database: { container: 'owned' } };
test('source rejects nonempty unseeded database without writing', () => {
  let calls = 0;
  assert.throws(() => seedSource('/unused', state, () => ++calls === 1 ? 'f' : '1'), /nonempty/);
  assert.equal(calls, 2);
});
test('failed import sends data and receipt in the same transaction and stops verification', () => {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'demo-source-'));
  try {
    writeFileSync(join(root, 'source-data.sql'), 'BEGIN;\nSELECT 1;\nCOMMIT;\n');
    let calls = 0;
    assert.throws(() => seedSource(root, state, (_container, database, sql) => {
      assert.equal(database, state.owner + '_source');
      if (++calls === 1) return 'f';
      if (calls === 2) return '0';
      assert.match(sql, /BEGIN;[\s\S]*CREATE TABLE public.demo_seed_receipt[\s\S]*COMMIT;/);
      throw new Error('import failed');
    }), /import failed/);
    assert.equal(calls, 3);
  } finally { rmSync(root, { recursive: true }); }
});
test('source verification is read-only and refuses a foreign receipt', () => {
  assert.throws(() => verifySource('/unused', state, (_container, _database, sql) => {
    assert.match(sql, /^BEGIN READ ONLY; SET LOCAL ROLE semlia_demo_/);
    return 'foreign';
  }), /receipt mismatch/);
});

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, readFileSync, writeFileSync, rmSync, realpathSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { preparePackage } from './package.mjs';

test('package is repeatable and refuses existing modified material', () => {
  const parent = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-demo-package-'));
  try {
    const root = join(parent, 'demo');
    const first = preparePackage(root);
    const second = preparePackage(root);
    assert.equal(first.digest, second.digest);
    assert.equal(first.owner, second.owner);
    assert.equal(JSON.parse(readFileSync(join(root, 'data.json'))).orders.length, 3000);
    writeFileSync(join(root, 'source.sql'), '-- corrupted');
    assert.throws(() => preparePackage(root), /modified/);
    assert.equal(readFileSync(join(root, 'source.sql'), 'utf8'), '-- corrupted');
  } finally { rmSync(parent, { recursive: true }); }
});

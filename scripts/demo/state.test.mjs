import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, mkdirSync, symlinkSync, readFileSync, writeFileSync, rmSync, realpathSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { openState, checkpoint } from './state.mjs';

test('ownership and input digest must match on repeat initialization', () => {
  const parent = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-demo-test-'));
  try {
    const root = join(parent, 'demo');
    const first = openState(root, 'sha256:one');
    assert.deepEqual(openState(root, 'sha256:one'), first);
    assert.throws(() => openState(root, 'sha256:two'), /digest/);
    assert.match(first.owner, /^semlia_demo_[a-f0-9]+$/);
    const unknown = join(parent, 'unknown'); mkdirSync(unknown);
    assert.throws(() => openState(unknown, 'sha256:one'), /unowned/);
    const link = join(parent, 'link'); symlinkSync(root, link);
    assert.throws(() => openState(link, 'sha256:one'), /symbolic/);
    writeFileSync(join(root, 'state.json'), JSON.stringify({ ...first, owner: 'postgres' }));
    assert.throws(() => openState(root, 'sha256:one'), /owner/);
  } finally { rmSync(parent, { recursive: true }); }
});

test('checkpoint preserves input identity and refuses stale writers', () => {
  const parent = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-demo-test-'));
  try {
    const root = join(parent, 'demo');
    const state = openState(root, 'sha256:one');
    const next = checkpoint(root, state, { phases: { generated: true } });
    assert.equal(next.revision, 1);
    assert.equal(JSON.parse(readFileSync(join(root, 'state.json'))).phases.generated, true);
    assert.throws(() => checkpoint(root, state, { phases: {} }), /stale/);
    assert.throws(() => checkpoint(root, next, { owner: 'other' }), /identity/);
  } finally { rmSync(parent, { recursive: true }); }
});

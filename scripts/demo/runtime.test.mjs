import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createServer } from 'node:net';
import { availablePort, unlinkOwned } from './runtime.mjs';
import { mkdtempSync, realpathSync, writeFileSync, lstatSync, existsSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

test('cleanup never removes a path it did not acquire', () => {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'demo-runtime-'));
  const path = join(root, 'owner');
  try {
    writeFileSync(path, 'owned');
    const identity = lstatSync(path);
    unlinkOwned(path, undefined);
    assert.ok(existsSync(path));
    unlinkOwned(path, { dev: identity.dev, ino: identity.ino + 1 });
    assert.ok(existsSync(path));
    unlinkOwned(path, identity);
    assert.ok(!existsSync(path));
  } finally { rmSync(root, { recursive: true }); }
});

test('occupied ports are rejected without stopping their owner', async () => {
  const server = createServer();
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  try {
    await assert.rejects(availablePort(server.address().port), { code: 'EADDRINUSE' });
    assert.equal(server.listening, true);
    const port = await availablePort();
    assert.notEqual(port, server.address().port);
  } finally { await new Promise(resolve => server.close(resolve)); }
});

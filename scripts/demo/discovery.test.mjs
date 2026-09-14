import assert from 'node:assert/strict';
import { test } from 'node:test';
import { discoverSource, allPages } from './discovery.mjs';

test('catalog pagination follows nested page cursors', async () => {
  const client = { request: async (_method, path) => path.includes('cursor=next') ? { items: [2], page: { nextCursor: null } } : { items: [1], page: { nextCursor: 'next' } } };
  assert.deepEqual(await allPages(client, '/api/v1/catalog?limit=1'), [1, 2]);
});

test('discovery pins the exact idempotent run and verified snapshot', async () => {
  const calls = [];
  const client = { request: async (method, path, body) => {
    calls.push({ method, path, body });
    if (method === 'POST') return { id: 'run', sourceConnectionId: 'source' };
    if (path.includes('discovery-runs?')) return { items: [{ id: 'run', sourceConnectionId: 'source', status: 'succeeded', snapshotId: 'snapshot' }] };
    if (path.endsWith('/snapshots/snapshot')) return { id: 'snapshot', sourceId: 'source', historyQuality: 'verified', coverageStatus: 'complete' };
    if (path.includes('/members?')) return { items: [{ kind: 'dataset' }], nextCursor: null };
    if (path.includes('/semantic-candidates?')) return { items: [{ discoveryRunId: 'run' }, { discoveryRunId: 'foreign' }], nextCursor: null };
    throw new Error('Unexpected request');
  } };
  const result = await discoverSource(client, { id: 'workspace', sourceId: 'source' }, 'stable-key');
  assert.equal(result.snapshot.id, 'snapshot');
  assert.equal(result.candidates.length, 1);
  assert.deepEqual(calls[0].body, { idempotencyKey: 'stable-key' });
});

test('failed discovery is refused, not restarted with a new key', async () => {
  let writes = 0;
  const client = { request: async method => method === 'POST' ? (writes++, { id: 'run', sourceConnectionId: 'source' }) : { items: [{ id: 'run', sourceConnectionId: 'source', status: 'failed' }] } };
  await assert.rejects(discoverSource(client, { id: 'workspace', sourceId: 'source' }, 'stable-key'), /terminal failed/);
  assert.equal(writes, 1);
});

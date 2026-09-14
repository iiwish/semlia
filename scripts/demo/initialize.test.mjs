import assert from 'node:assert/strict';
import { test } from 'node:test';
import { ensureBinding, registerDiscoveryArtifact } from './initialize.mjs';
test('existing active binding is not duplicated', async () => {
  let writes = 0;
  const client = { request: async method => { if (method !== 'GET') writes++; return { items: [{ principalId: 'principal', roleId: 'source_operator', status: 'active', workspaceId: 'workspace', scope: { type: 'workspace', id: 'internal-workspace-uuid' } }] }; } };
  await ensureBinding(client, 'workspace', 'principal', 'source_operator');
  assert.equal(writes, 0);
});

test('discovery artifact replay pins the original source version and key', async () => {
  const writes = [];
  for (const version of [1, 2]) {
    const client = { request: async (method, path, body, key) => {
      if (method === 'GET') return { id: 'source', version };
      writes.push({ path, body, key });
      return { id: 'artifact', sourceId: 'source' };
    } };
    await registerDiscoveryArtifact(client, '/workspace', 'source');
  }
  assert.deepEqual(writes[0], writes[1]);
  assert.equal(writes[0].body.expectedSourceVersion, 1);
  assert.equal(writes[0].body.sourceId, 'source');
});

test('discovery artifact refuses foreign source identity and unexpected revisions', async () => {
  for (const source of [{ id: 'foreign', version: 1 }, { id: 'source', version: 3 }]) {
    const client = { request: async method => { assert.equal(method, 'GET'); return source; } };
    await assert.rejects(registerDiscoveryArtifact(client, '/workspace', 'source'), /Unexpected/);
  }
});
test('new binding uses current server role version and exact scope', async () => {
  let posted;
  const client = { request: async (method, path, body) => {
    if (method === 'POST') { posted = body; return {}; }
    return { items: path.endsWith('/roles') ? [{ id: 'source_operator', version: 7 }] : [] };
  } };
  await ensureBinding(client, 'workspace', 'principal', 'source_operator');
  assert.deepEqual(posted, { principalId: 'principal', roleId: 'source_operator', expectedRoleVersion: 7, scope: { type: 'workspace', id: 'workspace' } });
});

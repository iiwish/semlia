import assert from 'node:assert/strict';
import { test } from 'node:test';
import { verifyWorkshop } from './verify.mjs';

test('workshop verifier reads server collections and rejects any production history', async () => {
  const calls = [];
  const client = { request: async (method, path) => {
    calls.push(method);
    return { items: path.includes('/production-operations') ? [{ id: 'unexpected' }] : [] };
  } };
  await assert.rejects(verifyWorkshop(client, { id: 'workshop', sourceId: 'source' }), /Workshop/);
  assert.ok(calls.every(x => x === 'GET'));
});

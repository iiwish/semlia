import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildBaseline, publishBaseline } from './production.mjs';

function fixture() {
  const members = [];
  for (const [table, fields] of [['orders', ['order_id', 'customer_id', 'amount_cents']], ['customers', ['customer_id']]]) {
    members.push({ kind: 'dataset', objectId: table, revisionId: table + '-rev', name: 'public.' + table });
    for (const name of fields) members.push({ kind: 'field', objectId: table + '.' + name, revisionId: table + '.' + name + '-rev', name, parentObjectId: table });
  }
  return { principals: { author: 'author' }, sourceId: 'source', discovery: {
    snapshot: { id: 'snapshot', sourceId: 'source', contentDigest: 'digest', historyQuality: 'verified', coverageStatus: 'complete', coverage: [{ key: 'sql', status: 'complete', enumerationComplete: true }] },
    members, candidates: ['orders', 'customers'].map(name => ({ id: name + '-candidate', title: 'public.' + name, contentDigest: name + '-digest' })),
  } };
}

test('baseline declares five synthetic assets and exact physical pins', () => {
  const payload = buildBaseline(fixture());
  const assets = payload.targets.filter(x => x.kind === 'semantic_asset');
  assert.equal(assets.length, 5);
  assert.ok(assets.every(x => x.identityKey === x.content.address));
  assert.equal(new Set(payload.targets.map(x => x.localKey)).size, payload.targets.length);
  assert.ok(assets.every(x => x.content.definition.includes('合成') && x.content.ownerPrincipalId === 'author'));
  const join = payload.targets.find(x => x.kind === 'join_contract');
  assert.equal(join.content.cardinality, 'many_to_one');
  assert.equal(join.content.pairs[0].left.objectId, 'orders.customer_id');
  assert.equal(join.content.leftDataset.snapshotId, 'snapshot');
  assert.equal(payload.input.candidates.length, 2);
});

test('baseline refuses incomplete source coverage and ambiguous physical names', () => {
  const workspace = fixture();
  workspace.discovery.snapshot.coverageStatus = 'partial';
  assert.throws(() => buildBaseline(workspace), /complete/);
  workspace.discovery.snapshot.coverageStatus = 'complete';
  workspace.discovery.members.push(workspace.discovery.members[0]);
  assert.throws(() => buildBaseline(workspace), /physical/);
});

test('published baseline reentry reads its release without repeating writes', async () => {
  const payload = buildBaseline(fixture());
  const calls = [];
  const author = { request: async (method, path) => {
    calls.push([method, path]);
    if (method === 'POST') return { operationId: 'operation' };
    return { summary: { releaseId: 'release' }, version: 1, input: payload.input, targets: payload.targets.map(declaration => ({ declaration })) };
  } };
  const publisher = { request: async () => ({ id: 'release', publishedBy: 'publisher' }) };
  assert.equal((await publishBaseline({ author, publisher }, 'workspace', payload)).id, 'release');
  assert.equal(calls.filter(([method]) => method === 'POST').length, 1);
});

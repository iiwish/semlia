import assert from 'node:assert/strict';
import { test } from 'node:test';
import { buildCorrection } from './correction.mjs';

test('refund correction updates the exact published asset revision', () => {
  const before = { address: 'demo.retail.net_revenue', definition: '合成 v1 未扣退款', scope: '合成范围', ownerPrincipalId: 'author' };
  const workspace = { baseline: { afterManifest: { assets: [{ assetId: 'asset', revisionId: 'revision' }] } } };
  const baseline = { input: { snapshots: [{ snapshotId: 'snapshot' }], candidates: [{}] }, targets: [{ localKey: 'net_revenue', targetId: 'asset', declaration: { content: before } }] };
  const payload = buildCorrection(workspace, baseline);
  assert.equal(payload.targets.length, 1);
  assert.equal(payload.targets[0].baseRevisionId, 'revision');
  assert.equal(payload.targets[0].changes[0].beforeValue, before.definition);
  assert.ok(payload.targets[0].content.definition.includes('succeeded'));
  assert.equal(payload.input.candidates.length, 0);
  assert.deepEqual(before, baseline.targets[0].declaration.content);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { alignHeadPins, dependenciesOf, judgeVocabularyCase, repin, vocabularyCases, vocabularyStage, vocabularyStages } from './v1/vocabulary.mjs';

const id = (kind, n) => `${kind}_01m3ev2edmfssaj5p015${String(n).padStart(6, '0')}`;
function syntheticHead() {
  const keys = ['orders', 'customers', 'paid', 'old', 'amount', 'count', 'average', 'model'];
  const pins = Object.fromEntries(keys.map((key, i) => [key, { assetId: id('ast', i), revisionId: id('rev', i), releaseId: id('rls', 1) }]));
  const member = (key, memberId) => ({ ...pins[key], memberId });
  const specs = {
    orders: { members: [{ id: 'amount', name: '支付金额' }, { id: 'paid_at', name: '支付时间（UTC）' }] },
    customers: { members: [{ id: 'region', name: '当前区域', valueType: 'string' }, { id: 'first_paid_at', name: '首次有效支付时间' }] },
    paid: { capability: 'predicate', subjectRef: pins.orders },
    old: { capability: 'predicate', subjectRef: pins.customers, predicate: { op: 'lt', left: { op: 'ref', ref: member('customers', 'first_paid_at') }, right: { op: 'parameter', parameter: 'period_start' } } },
    amount: { kind: 'aggregate', inputRef: member('orders', 'amount'), filterRefs: [pins.paid] },
    count: { kind: 'aggregate', inputRef: member('orders', 'amount'), filterRefs: [pins.paid] },
    average: { kind: 'derived', expression: { op: 'divide', left: { op: 'ref', ref: pins.amount }, right: { op: 'ref', ref: pins.count } } },
    model: { baseObjectRef: pins.orders, metricRefs: [pins.amount, pins.count, pins.average], compatibleTermRefs: [pins.paid, pins.old], publicAttributeRefs: [member('customers', 'region')] },
  };
  const head = {};
  for (const key of keys) head['demo_202609.' + key] = { ...pins[key], releaseId: id('rls', 9), content: { address: 'demo_202609.' + key, assetType: 'metric', displayName: key, definition: key + ' definition', ownerPrincipalId: id('prn', 1), spec: specs[key] } };
  return { head: alignHeadPins(head), pins };
}

test('head pins reuse the release existing content cites', () => {
  const { head, pins } = syntheticHead();
  assert.equal(head['demo_202609.orders'].releaseId, pins.orders.releaseId);
  assert.equal(head['demo_202609.model'].releaseId, id('rls', 9), 'an uncited revision is pinned to the head release');
});

test('every stage declares exactly the pins its content references', () => {
  const { head } = syntheticHead();
  const wording = vocabularyStage('wording', head);
  assert.deepEqual(wording.map(t => t.localKey), ['amount', 'count', 'customers', 'booked']);
  const dependencies = dependenciesOf(wording);
  assert.ok(dependencies.some(d => d.targetId === head['demo_202609.model'].assetId), 'sample expectation is a dependency');
  assert.ok(dependencies.some(d => d.targetId === head['demo_202609.orders'].assetId && d.releaseId === id('rls', 1)));
  const region = wording[2].content.spec.members.find(m => m.id === 'region');
  assert.deepEqual(region.valueAliases[0], { value: '华东', aliases: ['江浙沪', '华东地区', '华东区'] });
  assert.deepEqual(wording[2].changes.map(c => c.fieldPath), ['spec']);
  assert.deepEqual(wording[0].changes.map(c => [c.fieldPath, c.op]), [['aliases', 'add'], ['includes', 'add'], ['excludes', 'add'], ['sampleQuestions', 'add']]);
});

test('dependents are re-pinned to re-published revisions in order', () => {
  const { head } = syntheticHead();
  assert.deepEqual(vocabularyStages, ['wording', 'derived', 'model']);
  const fresh = { [head['demo_202609.amount'].assetId]: { revisionId: id('rev', 50), releaseId: id('rls', 50) }, [head['demo_202609.customers'].assetId]: { revisionId: id('rev', 51), releaseId: id('rls', 50) } };
  const derived = vocabularyStage('derived', head, fresh);
  assert.equal(derived[0].content.spec.expression.left.ref.revisionId, id('rev', 50));
  assert.equal(derived[0].content.spec.expression.right.ref.revisionId, head['demo_202609.count'].revisionId, 'unchanged assets keep their pin');
  assert.equal(derived[1].content.spec.predicate.left.ref.memberId, 'first_paid_at', 'member pins keep their member');
  assert.equal(derived[1].content.spec.subjectRef.revisionId, id('rev', 51));
  const spec = derived[1].changes.find(c => c.fieldPath === 'spec');
  assert.deepEqual(spec.beforeValue, head['demo_202609.old'].content.spec);
  assert.throws(() => dependenciesOf([{ content: { a: { assetId: 'x', revisionId: 'r1', releaseId: 'l' }, b: { assetId: 'x', revisionId: 'r2', releaseId: 'l' } } }]), /two pins/);
  assert.deepEqual(repin({ list: [{ assetId: 'y', revisionId: 'r', releaseId: 'l' }] }, {}), { list: [{ assetId: 'y', revisionId: 'r', releaseId: 'l' }] });
});

test('cases are judged by reconciliation or by naming the candidates', () => {
  const cases = Object.fromEntries(vocabularyCases().map(c => [c.id, c]));
  assert.equal(Object.keys(cases).length, 8);
  const clarify = { status: 200, body: { interpretation: { outcome: 'clarification', clarification: '「销售额」可能指不同的指标：有效支付金额、下单金额。' } } };
  assert.equal(judgeVocabularyCase(cases['shared-word'], clarify).passed, true);
  assert.equal(judgeVocabularyCase(cases['shared-word'], { status: 200, body: { interpretation: { outcome: 'query' }, resolution: {} } }).passed, false, 'guessing a metric fails');
  assert.equal(judgeVocabularyCase(cases['out-of-scope'], clarify).passed, true);
  assert.equal(judgeVocabularyCase(cases['alias-metric'], clarify).passed, false);
  const executed = { status: 200, body: { run: { state: 'succeeded' }, sql: 'SELECT 1', columns: ['demo_202609.amount'], rows: [['1500.00']] } };
  assert.equal(judgeVocabularyCase(cases['alias-metric'], { status: 200, body: { interpretation: { outcome: 'query' } } }, executed, [['1500']]).passed, true);
  assert.equal(judgeVocabularyCase(cases['alias-metric'], { status: 200, body: { interpretation: { outcome: 'query' } } }, executed, [['1000']]).outcome, 'wrong-answer');
  const wrongColumn = { ...executed, body: { ...executed.body, columns: ['demo_202609.booked'] } };
  assert.equal(judgeVocabularyCase(cases['alias-metric'], { status: 200, body: { interpretation: { outcome: 'query' } } }, wrongColumn, [['1500']]).passed, false, 'the confusable metric is a wrong answer even with equal numbers');
});

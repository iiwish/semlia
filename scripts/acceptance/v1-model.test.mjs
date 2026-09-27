import assert from 'node:assert/strict';
import { test } from 'node:test';
import { reconcileExecution, assertNoExecution, assertPlanPins, assertQueryWindow, acceptanceRound, caseProven } from './v1/model.mjs';
import { buildFaultContent, modelChange } from './v1/correction.mjs';
import { supplementContent, definitionOnlyTarget } from './v1/value-knowledge.mjs';

test('result reconciliation checks readonly success, exact values and complete rows', () => {
  const result = { run: { state: 'succeeded' }, sql: 'SELECT region, SUM(amount) FROM orders GROUP BY region', availability: 'ephemeral', rows: [['South', '500.00'], ['East', 1000]] };
  reconcileExecution(result, [['East', '1000'], ['South', '500']], 'region');
  assert.throws(() => reconcileExecution({ ...result, rows: [['East', 1000]] }, [['East', '1000'], ['South', '500']], 'region'));
  assert.throws(() => reconcileExecution({ ...result, rows: [['South', 500], ['East', 999]] }, [['East', '1000'], ['South', '500']], 'region'));
  assert.throws(() => reconcileExecution({ ...result, run: { state: 'failed' } }, result.rows, 'region'));
  assert.throws(() => reconcileExecution({ ...result, sql: 'DELETE FROM orders' }, result.rows, 'region'));
  reconcileExecution({ ...result, rows: [['2026-07-01T00:00:00Z', '600.0']] }, [['2026-07', '600']], 'monthly');
  reconcileExecution({ ...result, columns: ['demo_202609.amount', 'demo_202609.customers.region'], rows: [['1000', 'East'], ['500', 'South']] }, [['East', '1000'], ['South', '500']], 'region');
});

test('negative acceptance requires a semantic refusal and zero database execution growth', () => {
  assertNoExecution({ status: 403, body: { code: 'NO_MATCHING_GRANT' } }, { executions: 2, agentRuns: 0, modelSteps: 0 }, { executions: 2, agentRuns: 0, modelSteps: 0 }, 'denied');
  assertNoExecution({ status: 200, body: { interpretation: { outcome: 'clarification' } } }, { executions: 0 }, { executions: 0 }, 'ambiguous');
  assert.throws(() => assertNoExecution({ status: 500, body: {} }, { executions: 0 }, { executions: 0 }, 'denied'));
  assert.throws(() => assertNoExecution({ status: 200, body: { interpretation: { outcome: 'query' }, resolution: {} } }, { executions: 0 }, { executions: 0 }, 'ambiguous'));
  assert.throws(() => assertNoExecution({ status: 403, body: {} }, { executions: 0 }, { executions: 1 }, 'denied'));
  assert.throws(() => assertNoExecution({ status: 403, body: { code: 'CSRF_INVALID' } }, { executions: 0 }, { executions: 0 }, 'denied'));
  assert.throws(() => assertNoExecution({ status: 403, body: { code: 'NO_MATCHING_GRANT' } }, { executions: 0, agentRuns: 0, modelSteps: 0 }, { executions: 0, agentRuns: 1, modelSteps: 1 }, 'denied'));
});

test('execution uses the requested published model and exact plan release/digest', () => {
  const plan = { model: { assetId: 'model', revisionId: 'revision' }, releaseId: 'release', id: 'plan', planDigest: 'digest' };
  const state = { published: { 'demo_202609.model': { targetId: 'model', revisionId: 'revision' } } };
  assertPlanPins(plan, state, 'release');
  assert.throws(() => assertPlanPins({ ...plan, releaseId: 'stale' }, state, 'release'));
  assert.throws(() => assertPlanPins({ ...plan, model: null }, state, 'release'));
});

test('fault injection changes amount binding, preserves the unique key and correction restores content', () => {
  const baseline = { definition: 'Synthetic model', scope: 'Synthetic', spec: { memberBindings: [{ semanticRef: { assetId: 'orders', memberId: 'id' }, dataRef: { assetId: 'data', memberId: 'id' } }, { semanticRef: { assetId: 'orders', memberId: 'amount' }, dataRef: { assetId: 'data', memberId: 'amount' } }] } };
  const fault = buildFaultContent(baseline, 'orders');
  assert.equal(baseline.spec.memberBindings[0].dataRef.memberId, 'id');
  assert.equal(fault.spec.memberBindings[0].dataRef.memberId, 'id');
  assert.equal(fault.spec.memberBindings[1].dataRef.memberId, 'is_valid');
  const target = modelChange(fault, baseline, { assetId: 'model', revisionId: 'fault-revision' }, 'restore');
  assert.deepEqual(target.content, baseline);
  assert.deepEqual(target.changes.find(x => x.fieldPath === 'spec').beforeValue, fault.spec);
  assert.deepEqual(target.changes.find(x => x.fieldPath === 'spec').afterValue, baseline.spec);
  assert.throws(() => buildFaultContent(baseline, 'unknown'));
  const glossary = supplementContent(baseline);
  assert.throws(() => modelChange(baseline, glossary, { assetId: 'model', revisionId: 'baseline' }, 'glossary'));
  assert.deepEqual(definitionOnlyTarget(baseline, { assetId: 'model', revisionId: 'baseline' }).changes.map(x => x.fieldPath), ['definition']);
});

test('semantic date bounds reject a missing final second even when sample values match', () => {
  const query = { timeRange: { from: '2026-07-01T00:00:00Z', to: '2026-09-01T00:00:00Z', granularity: 'month' } };
  assertQueryWindow(query, 'monthly');
  assert.throws(() => assertQueryWindow({ timeRange: { ...query.timeRange, to: '2026-08-31T23:59:59Z' } }, 'monthly'));
  assert.throws(() => assertQueryWindow(query, 'total'));
  assertQueryWindow({ timeRange: { from: '2026-08-01T00:00:00+00:00', to: '2026-09-01T00:00:00Z' } }, 'old-customer-average');
  assert.equal(caseProven({ attempts: [{ case: 'monthly', number: 4, passed: false, semanticWindowReview: { passed: false } }], executionReplays: [{ case: 'monthly', askAttempt: 4, passed: true }] }, 'monthly'), false);
});

test('half-open repair round is explicitly bounded to monthly and unknown rounds fail closed', () => {
  const round = acceptanceRound('ask-half-open-time-window');
  assert.deepEqual(round.cases, ['monthly']);
  assert.equal(round.maxAttemptsPerCase, 2);
  assert.match(round.correction, /half-open/);
  assert.throws(() => acceptanceRound('retry-until-pass'));
});

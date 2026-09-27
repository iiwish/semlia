import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { isDeepStrictEqual } from 'node:util';
import { join } from 'node:path';
import { setTimeout } from 'node:timers/promises';
import { readState, readPrivate, writePrivate, goldenCases } from './core.mjs';
import { connect } from './initialize.mjs';
import { readModelLedger, requestAs, counters, independentRows, reconcileExecution, assertQueryWindow, caseProven } from './model.mjs';
import { readKnowledgeHead, approvedKnowledgeBasis } from './value-knowledge.mjs';

export function buildFaultContent(baseline, ordersId) {
  const content = structuredClone(baseline);
  const bindings = content.spec.memberBindings.filter(x => x.semanticRef.assetId === ordersId && x.semanticRef.memberId === 'amount');
  assert.equal(bindings.length, 1);
  assert.equal(bindings[0].dataRef.memberId, 'amount');
  bindings[0].dataRef.memberId = 'is_valid';
  content.definition += ' 合成故障注入：临时将支付金额错误绑定到有效标记数值列，保留原订单唯一键，专用于字段映射故障与治理纠错演练，不代表正确业务口径。';
  return content;
}
export function modelChange(before, after, pin, label) {
  const changes = ['definition', 'scope', 'spec'].filter(key => !isDeepStrictEqual(before[key], after[key])).map(key => ({ fieldPath: key, op: 'update', beforeValue: before[key], afterValue: after[key] }));
  assert.ok(changes.some(x => x.fieldPath === 'spec'), 'A structured correction is required');
  return { intent: 'update', kind: 'semantic_asset', localKey: 'model', targetId: pin.assetId, baseRevisionId: pin.revisionId, title: `V1 synthetic ${label}`, content: after, changes, evidenceIds: [] };
}

export async function governanceHeadProof(root, { request = requestAs } = {}) {
  const view = await readKnowledgeHead(root, request), knowledgeBasis = approvedKnowledgeBasis(root, view);
  return { currentRelease: view.release.id, currentRevision: view.current.id, correctBaselineContent: true, originalBaselineContent: isDeepStrictEqual(view.original, view.current.content), knowledgeBasis, checkedAt: new Date().toISOString() };
}

export async function governanceRehearsal(root) {
  const state = readState(root), modelLedger = readModelLedger(root);
  assert.ok(caseProven(modelLedger, 'monthly'), 'Monthly real-model acceptance must pass before fault injection');
  const source = modelLedger.attempts.find(x => x.case === 'old-customer-average' && x.passed);
  assert.ok(source, 'A real-model old-customer query must pass before deterministic governance replay');
  const asked = readPrivate(join(root, `model-old-customer-average-${source.number}-ask.json`));
  const query = asked.body.interpretation.query;
  assert.ok(query && asked.body.resolution.plan.model);
  assertQueryWindow(query, 'old-customer-average');
  const path = join(root, 'correction-amount-acceptance.json');
  const fresh = !existsSync(path);
  const ledger = fresh ? { owner: state.owner, scenario: 'amount-binding-fault', baselineRelease: modelLedger.baselineRelease, modelQueryAttempt: source.number, stages: {} } : readPrivate(path);
  assert.equal(ledger.owner, state.owner);
  const save = () => writePrivate(path, ledger);
  const base = `/api/v1/workspaces/${state.workspace.id}`;
  const author = (await connect(root, state, 'author')).client;
  const reviewer = (await connect(root, state, 'reviewer')).client;
  const publisher = (await connect(root, state, 'publisher')).client;
  if (fresh) {
    const current = (await publisher.request('GET', base + '/governance/releases?limit=1')).items[0];
    assert.equal(current.id, ledger.baselineRelease, 'Fresh fault scenario requires the verified baseline head');
  }
  const originalOperation = await author.request('GET', base + '/production-operations/' + state.operations['demo_202609.model'].operationId);
  const baseline = originalOperation.targets[0].declaration.content;
  const fault = buildFaultContent(baseline, state.published['demo_202609.orders'].targetId);
  const originalRelease = await publisher.request('GET', base + '/production-releases/' + ledger.baselineRelease);
  const modelId = state.published['demo_202609.model'].targetId;
  const modelPin = release => release.afterManifest.assets.find(x => x.assetId === modelId);
  const head = release => ({ presence: 'present', releaseId: release.id, manifestDigest: release.afterManifest.digest });

  async function publish(name, before, after, previous) {
    const target = modelChange(before, after, modelPin(previous), name);
    const stage = ledger.stages[name] ??= { target, beforeRelease: previous.id };
    assert.deepEqual(stage.target, target); save();
    const dependencies = Object.values(state.published).filter(x => x.kind === 'semantic_asset' && x.targetId !== modelId).map(({ kind, targetId, releaseId, revisionId }) => ({ kind, targetId, releaseId, revisionId }));
    const payload = { input: { ...originalOperation.input, dependencies }, targets: [target] };
    if (!stage.operationId) {
      const created = await requestAs(root, 'author', 'POST', base + '/production-operations', payload, 'v1-amount-rehearsal-create-' + name);
      stage.creationAttempts ??= [];
      stage.creationAttempts.push({ status: created.status, code: /^[A-Z][A-Z0-9_]{2,63}$/.test(created.body.code) ? created.body.code : undefined }); save();
      writePrivate(join(root, `correction-amount-${name}-create.json`), created);
      assert.equal(created.status, 201);
      stage.operationId = created.body.operationId; save();
    }
    const operationPath = base + '/production-operations/' + stage.operationId;
    let operation = await author.request('GET', operationPath);
    const pin = { expectedVersion: operation.version, setDigest: operation.setDigest };
    if (!operation.summary.releaseId) {
      if (operation.summary.progress === 'draft') {
        await author.request('POST', operationPath + '/business-rule-confirmations', { ...pin, targetKey: 'model', action: 'confirm', declaration: name === 'fault' ? '合成故障注入审批：金额临时错误绑定到有效标记，仅用于封闭验收；唯一键不变，演练后必须恢复。' : '合成纠错审批：支付金额恢复绑定到订单金额列，保留订单唯一键和原统计范围。' }, 'v1-amount-rehearsal-confirm-' + name);
        await author.request('POST', operationPath + '/submit', pin, 'v1-amount-rehearsal-submit-' + name);
      }
      for (let i = 0; i < 240; i++) {
        operation = await author.request('GET', operationPath);
        if (!['queued', 'running'].includes(operation.activeValidation.status)) break;
        await setTimeout(500);
      }
      stage.validation = operation.activeValidation; stage.unresolvedCodes = operation.unresolvedCodes; save();
      assert.equal(operation.activeValidation.status, 'succeeded', 'Structured mutation was not accepted; do not bypass its safety gate');
      const validation = { attemptNo: operation.activeValidation.attemptNo, validationDigest: operation.activeValidation.validationDigest };
      await reviewer.request('POST', operationPath + '/reviews', { ...pin, validation, proposalIds: operation.targets.map(x => x.proposalId), decision: 'approve', note: name === 'fault' ? '独立审核：批准隔离合成金额映射故障，不认可为真实业务规则，须完成纠错及最终恢复。' : '独立审核：确认结构化金额绑定恢复为订单金额列，非文案修订。' }, 'v1-amount-rehearsal-review-' + name);
      await publisher.request('POST', operationPath + '/publish', { ...pin, validation, expectedHead: head(previous) }, 'v1-amount-rehearsal-publish-' + name);
      operation = await author.request('GET', operationPath);
    }
    const release = await publisher.request('GET', base + '/production-releases/' + operation.summary.releaseId);
    assert.equal(release.publishedBy, state.principals.publisher);
    stage.release = release; stage.operationPin = pin; stage.modelPin = modelPin(release); save();
    return release;
  }

  async function replay(name, release, expected, sql) {
    const stage = ledger.stages[name] ??= {};
    if (stage.replay?.finished) return;
    if (stage.replay?.started) throw new Error('Prior deterministic replay requires explicit inspection before a fresh execution');
    const before = counters(root);
    stage.replay = { started: true, before }; save();
    try {
    const response = await requestAs(root, 'consumer', 'POST', base + '/semantic-queries:resolve', { query: { ...query, context: { mode: 'explicit', releaseId: release.id } }, idempotencyKey: 'v1-amount-rehearsal-resolve-' + name });
    writePrivate(join(root, `correction-amount-${name}-resolve.json`), response);
    stage.replay.resolveStatus = response.status; save();
    assert.equal(response.status, 200); assert.ok(response.body.plan);
    const plan = response.body.plan;
    assert.equal(plan.releaseId, release.id); assert.equal(plan.model.assetId, modelId);
    assert.equal(plan.model.revisionId, modelPin(release).revisionId);
    const executed = await requestAs(root, 'consumer', 'POST', base + `/resolved-semantic-plans/${plan.id}:execute`, { planDigest: plan.planDigest, idempotencyKey: 'v1-amount-rehearsal-execute-' + name });
    writePrivate(join(root, `correction-amount-${name}-execute.json`), executed);
    stage.replay.executeStatus = executed.status; save();
    assert.equal(executed.status, 200);
    const actual = independentRows(root, sql);
    reconcileExecution({ ...executed.body, columns: undefined, rows: actual }, expected, 'old-customer-average');
    reconcileExecution(executed.body, actual, 'old-customer-average');
    const after = counters(root);
    assert.equal(after.agentRuns, before.agentRuns, 'Deterministic governance replay must not call the model');
    assert.equal(after.modelSteps, before.modelSteps, 'Deterministic governance replay must not add a model step');
    stage.replay = { ...stage.replay, finished: true, passed: true, after, releaseId: release.id, revisionId: plan.model.revisionId, planId: plan.id, queryId: response.body.id, executionId: executed.body.run.id, actual }; save();
    console.log(JSON.stringify({ stage: name, passed: true, releaseId: release.id, revisionId: plan.model.revisionId, modelCalls: 0 }));
    } catch {
      stage.replay = { ...stage.replay, finished: true, passed: false, reason: 'REPLAY_CONTRACT_FAILED', after: counters(root) }; save();
      console.log(JSON.stringify({ stage: name, passed: false, releaseId: release.id, reason: 'REPLAY_CONTRACT_FAILED' }));
    }
  }
  const golden = goldenCases().find(x => x.id === 'old-customer-average');
  const faultSQL = "SELECT (SUM(o.is_valid)::numeric/NULLIF(COUNT(DISTINCT o.id),0))::text FROM semlia_demo_202609.orders o JOIN semlia_demo_202609.customers c ON c.id=o.customer_id WHERE o.is_valid=1 AND o.paid_at>='2026-08-01T00:00:00Z'::timestamptz AND o.paid_at<'2026-09-01T00:00:00Z'::timestamptz AND c.region='华东' AND c.first_paid_at<'2026-08-01T00:00:00Z'::timestamptz";
  await replay('baseline', originalRelease, golden.expected, golden.sql);
  const faultRelease = await publish('fault', baseline, fault, originalRelease);
  await replay('fault', faultRelease, [['1']], faultSQL);
  const correction = await publish('correction', fault, baseline, faultRelease);
  await replay('correction', correction, golden.expected, golden.sql);
  if (!ledger.stages.rollback?.release) {
    const pin = ledger.stages.correction.operationPin;
    const rolled = await publisher.request('POST', base + `/production-releases/${correction.id}/rollback`, { ...pin, expectedHead: head(correction), reason: '合成回滚演练：恢复历史金额映射故障以核对可回滚性，随后必须正式恢复正确映射。' }, 'v1-amount-rehearsal-rollback');
    const release = await publisher.request('GET', base + '/production-releases/' + rolled.releaseId);
    assert.deepEqual(release.afterManifest, faultRelease.afterManifest);
    ledger.stages.rollback = { release, modelPin: modelPin(release) }; save();
  }
  const rollback = ledger.stages.rollback.release;
  await replay('rollback', rollback, [['1']], faultSQL);
  const restored = await publish('restored', fault, baseline, rollback);
  await replay('restored', restored, golden.expected, golden.sql);
  ledger.complete = ['baseline', 'fault', 'correction', 'rollback', 'restored'].every(name => ledger.stages[name].replay?.passed); ledger.finalRelease = restored.id; save();
  return ledger;
}

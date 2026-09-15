import { isDeepStrictEqual } from 'node:util';
import { setTimeout } from 'node:timers/promises';
import { connectActor } from './session.mjs';
import { readSecrets } from './provision.mjs';
import { checkpoint } from './state.mjs';

export function buildCorrection(workspace, baselineOperation) {
  const original = baselineOperation.targets.find(x => x.localKey === 'net_revenue');
  const pin = workspace.baseline.afterManifest.assets.find(x => x.assetId === original?.targetId);
  if (!original || !pin) throw new Error('Missing published net revenue baseline');
  const before = original.declaration.content;
  const definition = '合成零售净收入 = status=paid 的订单 amount_cents 总额减去这些订单关联的 status=succeeded 退款 amount_cents 总额；pending 退款不扣减。先按 order_id 汇总退款再关联订单，避免明细或多条退款放大金额。单位为人民币分，统计范围为固定 90 天合成案例。';
  return { input: { snapshots: baselineOperation.input.snapshots, candidates: [], evidence: [], dependencies: [] }, targets: [{
    intent: 'update', kind: 'semantic_asset', localKey: 'net_revenue', targetId: original.targetId, baseRevisionId: pin.revisionId,
    title: '净收入退款口径纠正 · 合成零售', content: { ...before, definition },
    changes: [{ fieldPath: 'definition', op: 'update', beforeValue: before.definition, afterValue: definition }], evidenceIds: [],
  }] };
}

export async function initializeCorrectionDraft(root, state) {
  const workspace = state.workspaces.showcase;
  if (!workspace.rollback) throw new Error('Complete rollback before preparing the live correction draft');
  const { client: author, session } = await connectActor(root, state, 'showcase_author', readSecrets(root).accountPassword);
  if (session.workspaces.length !== 1 || session.workspaces[0].id !== workspace.id || session.workspaces[0].principalId !== workspace.principals.author) throw new Error('Unexpected draft author');
  const base = `/api/v1/workspaces/${workspace.id}/production-operations`;
  const baseline = await author.request('GET', base + '/' + workspace.baseline.attribution.operationId);
  const payload = buildCorrection(workspace, baseline);
  const created = await author.request('POST', base, payload, 'resume-retail-v1-live-correction-draft');
  const draft = await author.request('GET', base + '/' + created.operationId);
  if (draft.summary.progress !== 'draft' || draft.version !== 1 || draft.activeValidation.status !== 'not_requested' || !isDeepStrictEqual(draft.targets.map(x => x.declaration), payload.targets)) throw new Error('Live correction draft has been changed; refusing to overwrite user work');
  return checkpoint(root, state, { workspaces: { ...state.workspaces, showcase: { ...workspace, correctionDraft: { operationId: created.operationId, version: draft.version, setDigest: draft.setDigest } } } });
}

async function terminalValidation(author, path) {
  const deadline = Date.now() + 120000;
  let operation;
  do {
    operation = await author.request('GET', path);
    if (!['queued', 'running'].includes(operation.activeValidation.status)) return operation;
    await setTimeout(500);
  } while (Date.now() < deadline);
  throw new Error('Correction validation still active; resume the same operation');
}

export async function initializeCorrection(root, initialState) {
  let state = initialState;
  const password = readSecrets(root).accountPassword;
  for (const slug of ['showcase', 'rehearsal']) {
    let workspace = state.workspaces?.[slug];
    if (!workspace?.baseline) throw new Error('Publish baseline first');
    const save = patch => {
      workspace = { ...workspace, ...patch };
      state = checkpoint(root, state, { workspaces: { ...state.workspaces, [slug]: workspace } });
    };
    const actors = {};
    for (const role of ['author', 'reviewer', 'publisher']) {
      const { client, session } = await connectActor(root, state, `${slug}_${role}`, password);
      if (session.workspaces.length !== 1 || session.workspaces[0].id !== workspace.id || session.workspaces[0].principalId !== workspace.principals[role]) throw new Error('Unexpected correction actor');
      actors[role] = client;
    }
    const { author, reviewer, publisher } = actors;
    const base = `/api/v1/workspaces/${workspace.id}`;
    const baselineOperation = await author.request('GET', base + `/production-operations/${workspace.baseline.attribution.operationId}`);
    const payload = buildCorrection(workspace, baselineOperation);
    const created = await author.request('POST', base + '/production-operations', payload, 'resume-retail-v1-correction');
    const path = base + `/production-operations/${created.operationId}`;
    let operation = await author.request('GET', path);
    if (operation.version !== 1 || !isDeepStrictEqual(operation.targets.map(x => x.declaration), payload.targets)) throw new Error('Correction payload changed');
    const pin = { expectedVersion: operation.version, setDigest: operation.setDigest };
    const expectedHead = { presence: 'present', releaseId: workspace.baseline.id, manifestDigest: workspace.baseline.afterManifest.digest };
    if (!operation.summary.releaseId) {
      if (operation.activeValidation.status === 'not_requested') await author.request('POST', path + '/submit', pin, 'resume-retail-v1-correction-submit');
      operation = await terminalValidation(author, path);
      if (operation.activeValidation.attemptNo === 1) {
        if (operation.activeValidation.status !== 'failed' || !operation.unresolvedCodes.includes('PRODUCTION_BUSINESS_RULE_UNCONFIRMED')) throw new Error('Unconfirmed correction did not fail with the expected rule blocker');
        // Failed attempts intentionally expose no publishable seal. A real seal
        // from the baseline must not authorize this unconfirmed operation.
        const validation = workspace.baseline.attribution.validation;
        let deniedStatus;
        try { await publisher.request('POST', path + '/publish', { ...pin, validation, expectedHead }, 'resume-retail-v1-correction-foreign-seal-publish'); }
        catch (error) { if (![409, 422].includes(error.status)) throw error; deniedStatus = error.status; }
        if (!deniedStatus) throw new Error('Unconfirmed correction unexpectedly published');
        save({ correctionProof: { operationId: created.operationId, unconfirmedAttempt: operation.activeValidation, attemptedValidationFromReleaseId: workspace.baseline.id, deniedPublishStatus: deniedStatus } });
        await author.request('POST', path + '/business-rule-confirmations', { ...pin, targetKey: 'net_revenue', action: 'confirm', declaration: `${payload.targets[0].content.definition}\n合成案例作者确认：只扣成功退款，按订单预聚合，排除未支付订单及待处理退款。这是人工业务声明，不是 AI 输出。` }, 'resume-retail-v1-correction-confirm');
        await author.request('POST', path + '/validations', { ...pin, previousAttemptNo: 1, reason: '已确认成功退款扣减及按订单预聚合的合成业务口径，重新执行验证。' }, 'resume-retail-v1-correction-revalidate');
        operation = await terminalValidation(author, path);
      }
      if (operation.activeValidation.status !== 'succeeded' || operation.activeValidation.attemptNo !== 2 || !workspace.correctionProof) throw new Error('Corrected rule validation is not proven');
      const validation = { attemptNo: 2, validationDigest: operation.activeValidation.validationDigest };
      await reviewer.request('POST', path + '/reviews', { ...pin, validation, proposalIds: operation.targets.map(x => x.proposalId), decision: 'approve', note: '独立确认只扣 succeeded 退款；按订单预聚合避免重复计算，已核对合成数据独立对账口径。' }, 'resume-retail-v1-correction-review');
      const released = await publisher.request('POST', path + '/publish', { ...pin, validation, expectedHead }, 'resume-retail-v1-correction-publish');
      operation = await author.request('GET', path);
      if (operation.summary.releaseId !== released.releaseId) throw new Error('Correction release does not match operation');
    }
    const correction = await publisher.request('GET', base + `/production-releases/${operation.summary.releaseId}`);
    if (correction.publishedBy !== workspace.principals.publisher) throw new Error('Unexpected correction publisher');
    save({ correction });
    const rolled = await publisher.request('POST', base + `/production-releases/${correction.id}/rollback`, { ...pin, expectedHead: { presence: 'present', releaseId: correction.id, manifestDigest: correction.afterManifest.digest }, reason: '合成案例版本回滚演示：恢复 v1 完整内容，保留退款纠正、失败验证与独立审核历史；回滚不代表认可 v1 临时口径为最终标准。' }, 'resume-retail-v1-correction-rollback');
    const rollback = await publisher.request('GET', base + `/production-releases/${rolled.releaseId}`);
    if (rollback.id === correction.id || rollback.rolledBackReleaseId !== correction.id || !isDeepStrictEqual(rollback.afterManifest, workspace.baseline.afterManifest)) throw new Error('Rollback did not preserve the exact baseline manifest');
    save({ rollback });
  }
  return state;
}

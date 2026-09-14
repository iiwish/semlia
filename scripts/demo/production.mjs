import { isDeepStrictEqual } from 'node:util';
import { setTimeout } from 'node:timers/promises';
import { connectActor } from './session.mjs';
import { readSecrets } from './provision.mjs';
import { checkpoint } from './state.mjs';

export async function publishBaseline(actors, workspaceId, payload) {
  const { author, reviewer, publisher } = actors;
  const base = `/api/v1/workspaces/${workspaceId}`;
  const created = await author.request('POST', base + '/production-operations', payload, 'resume-retail-v1-baseline');
  const path = base + `/production-operations/${created.operationId}`;
  let operation = await author.request('GET', path);
  if (operation.version !== 1 || !isDeepStrictEqual(operation.input, payload.input) || !isDeepStrictEqual(operation.targets.map(x => x.declaration), payload.targets)) throw new Error('Baseline server payload differs from deterministic input');
  if (operation.summary.releaseId) return publisher.request('GET', base + `/production-releases/${operation.summary.releaseId}`);
  const pin = { expectedVersion: operation.version, setDigest: operation.setDigest };
  for (const target of payload.targets.filter(x => x.kind === 'semantic_asset')) {
    await author.request('POST', path + '/business-rule-confirmations', {
      ...pin, targetKey: target.localKey, action: 'confirm', declaration: `${target.content.definition}\n范围：${target.content.scope}\n由合成案例作者明确声明；非 AI 输出，非真实企业业务数据。`,
    }, `resume-retail-v1-baseline-rule-${target.localKey}`);
  }
  await author.request('POST', path + '/submit', pin, 'resume-retail-v1-baseline-submit');
  const deadline = Date.now() + 120000;
  do {
    operation = await author.request('GET', path);
    if (!['queued', 'running'].includes(operation.activeValidation.status)) break;
    await setTimeout(500);
  } while (Date.now() < deadline);
  if (operation.activeValidation.status !== 'succeeded') throw new Error('Baseline validation not successful: ' + operation.activeValidation.status + ' ' + operation.unresolvedCodes.join(','));
  const validation = { attemptNo: operation.activeValidation.attemptNo, validationDigest: operation.activeValidation.validationDigest };
  const review = { ...pin, validation, proposalIds: operation.targets.map(x => x.proposalId).filter(Boolean), decision: 'approve', note: '独立审核合成零售来源、主键、粒度与临时支付口径；v1 净收入尚未扣退款，保留后续纠正演示。' };
  let denied = false;
  try { await author.request('POST', path + '/reviews', review, 'resume-retail-v1-baseline-self-review'); }
  catch (error) { if (error.status !== 403) throw error; denied = true; }
  if (!denied) throw new Error('Author self-review unexpectedly allowed');
  await reviewer.request('POST', path + '/reviews', review, 'resume-retail-v1-baseline-review');
  const released = await publisher.request('POST', path + '/publish', { ...pin, validation, expectedHead: { presence: 'absent' } }, 'resume-retail-v1-baseline-publish');
  return publisher.request('GET', base + `/production-releases/${released.releaseId}`);
}

export async function initializeBaseline(root, initialState) {
  let state = initialState;
  const password = readSecrets(root).accountPassword;
  for (const slug of ['showcase', 'rehearsal']) {
    const workspace = state.workspaces?.[slug];
    if (!workspace?.discovery) throw new Error('Complete source discovery first');
    const actors = {};
    for (const role of ['author', 'reviewer', 'publisher']) {
      const { client, session } = await connectActor(root, state, `${slug}_${role}`, password);
      if (session.workspaces.length !== 1 || session.workspaces[0].id !== workspace.id || session.workspaces[0].principalId !== workspace.principals[role]) throw new Error('Unexpected production actor');
      actors[role] = client;
    }
    const baseline = await publishBaseline(actors, workspace.id, buildBaseline(workspace));
    if (baseline.publishedBy !== workspace.principals.publisher || baseline.afterManifest.assets.length !== 5) throw new Error('Baseline release invariant failed');
    state = checkpoint(root, state, { workspaces: { ...state.workspaces, [slug]: { ...workspace, baseline } } });
  }
  return state;
}

export function buildBaseline(workspace) {
  const { snapshot, members, candidates } = workspace.discovery;
  if (snapshot.sourceId !== workspace.sourceId || snapshot.historyQuality !== 'verified' || snapshot.coverageStatus !== 'complete') throw new Error('Production requires complete verified source coverage');
  const physical = (kind, name, parentObjectId) => {
    const found = members.filter(x => x.kind === kind && x.name === name && (!parentObjectId || x.parentObjectId === parentObjectId));
    if (found.length !== 1) throw new Error('Missing or ambiguous physical member: ' + name);
    const { objectId, revisionId } = found[0];
    return { snapshotId: snapshot.id, kind, objectId, revisionId };
  };
  const orders = physical('dataset', 'public.orders'), customers = physical('dataset', 'public.customers');
  const orderId = physical('field', 'order_id', orders.objectId);
  const customerId = physical('field', 'customer_id', customers.objectId);
  const orderCustomer = physical('field', 'customer_id', orders.objectId);
  const amount = physical('field', 'amount_cents', orders.objectId);
  const targets = [];
  const add = (kind, localKey, title, content) => targets.push({ intent: 'create', kind, localKey, identityKey: kind === 'semantic_asset' ? content.address : `resume_retail_v1:${localKey}`, title, content, changes: [], evidenceIds: [] });
  const definitions = [
    ['orders', 'entity', '订单', '合成零售订单；每个 order_id 唯一，客户关系由 customer_id 关联。包含已支付、待支付和取消订单。'],
    ['customers', 'entity', '客户', '合成零售客户；每个 customer_id 唯一，姓名和地区均为演示数据，不对应真实个人。'],
    ['paid_amount', 'metric', '支付金额', '合成零售已支付订单的 amount_cents 之和；只计 status=paid，排除 pending 和 cancelled，不扣退款；金额单位为人民币分。'],
    ['net_revenue', 'metric', '净收入', '合成案例 v1 临时口径：净收入按已支付订单金额汇总，尚未扣除退款。本版本专门用于展示退款口径纠正，不代表最终业务标准。'],
    ['paid_order_count', 'metric', '支付订单数', '合成零售 status=paid 的唯一 order_id 数量；排除 pending 和 cancelled，订单明细和退款连接不得重复计数。'],
  ];
  for (const [key, assetType, title, definition] of definitions) {
    const isCustomer = key === 'customers', dataset = isCustomer ? customers : orders, id = isCustomer ? customerId : orderId;
    add('semantic_asset', key, `${title} · 合成零售`, {
      address: `demo.retail.${key}`, assetType, displayName: `${title} · 合成零售`, definition,
      scope: '合成零售，2026-04-01 至 2026-06-29；仅演示语义治理，数值由独立 SQL 对账，不宣称查询引擎执行了退款聚合。', ownerPrincipalId: workspace.principals.author,
    });
    add('physical_binding', `${key}_binding`, `${title}来源绑定`, { asset: { localKey: key }, dataset, ...(['paid_amount', 'net_revenue'].includes(key) ? { field: amount } : { field: id }) });
    add('model_grain', `${key}_grain`, `${title}基础粒度`, { asset: { localKey: key }, expression: isCustomer ? 'customer_id' : 'order_id', fields: [id] });
    if (assetType === 'entity') add('entity_key', `${key}_key`, `${title}唯一键`, { asset: { localKey: key }, fields: [id], uniqueness: 'exact' });
  }
  add('join_contract', 'orders_customers_join', '订单关联客户 · 合成零售', {
    leftDataset: orders, rightDataset: customers, pairs: [{ left: orderCustomer, right: customerId }], joinType: 'left', cardinality: 'many_to_one',
    expression: 'orders.customer_id = customers.customer_id', notes: '合成客户主键唯一；关联不得放大订单金额或支付订单数。',
  });
  const selections = ['orders', 'customers'].map(key => {
    const found = candidates.filter(x => x.title === `public.${key}`);
    if (found.length !== 1) throw new Error('Missing or ambiguous source candidate');
    const candidate = found[0];
    const targetKeys = targets.filter(x => x.localKey === key || x.localKey.startsWith(key + '_')).map(x => x.localKey).sort();
    return { candidateId: candidate.id, snapshotId: snapshot.id, digest: candidate.contentDigest, targetKeys, primaryTargetKey: key };
  });
  selections.sort((a, b) => a.candidateId < b.candidateId ? -1 : 1);
  targets.sort((a, b) => a.localKey < b.localKey ? -1 : 1);
  const coverageKeys = snapshot.coverage.filter(x => x.status === 'complete' && x.enumerationComplete).map(x => x.key).sort();
  if (!coverageKeys.length) throw new Error('No complete coverage keys');
  return { input: { snapshots: [{ sourceId: workspace.sourceId, snapshotId: snapshot.id, digest: snapshot.contentDigest, coverageKeys }], candidates: selections, evidence: [], dependencies: [] }, targets };
}

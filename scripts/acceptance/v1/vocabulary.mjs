import assert from 'node:assert/strict';
import { join } from 'node:path';
import { setTimeout } from 'node:timers/promises';
import { checkpoint, readState, writePrivate } from './core.mjs';
import { connect } from './initialize.mjs';
import { requestAs, independentRows, counters } from './model.mjs';

// Synthetic business wording for the V1 demo knowledge. It is published as governed revisions on top of the
// existing head, so the frozen V1 fixture and its digest stay untouched.
const prefix = 'demo_202609.';
const BOOKED_DISTINCTION = '下单金额含无效订单（is_valid ≠ 1）；有效支付金额只计有效订单。';

function pin(head, key) {
  const entry = head[prefix + key];
  assert.ok(entry, `Published ${key} required`);
  return { assetId: entry.assetId, revisionId: entry.revisionId, releaseId: entry.releaseId };
}
function member(head, key, memberId) { return { ...pin(head, key), memberId }; }

function isPin(value) {
  return value && typeof value === 'object' && !Array.isArray(value) && typeof value.assetId === 'string' && typeof value.revisionId === 'string' && typeof value.releaseId === 'string';
}

// dependenciesOf lists the exact pins the targets reference. Production validation requires one pin per asset.
export function dependenciesOf(targets) {
  const pins = new Map();
  const walk = value => {
    if (Array.isArray(value)) return value.forEach(walk);
    if (!value || typeof value !== 'object') return;
    if (isPin(value)) {
      const dependency = { kind: 'semantic_asset', targetId: value.assetId, releaseId: value.releaseId, revisionId: value.revisionId };
      const seen = pins.get(value.assetId);
      if (seen) assert.deepEqual(seen, dependency, 'One asset referenced at two pins');
      pins.set(value.assetId, dependency);
    }
    Object.values(value).forEach(walk);
  };
  targets.forEach(target => walk(target.content));
  return [...pins.values()].sort((a, b) => a.targetId.localeCompare(b.targetId));
}

// alignHeadPins reuses the release that existing content already cites for each current revision, so new
// references agree with old ones; an uncited revision is pinned to the head release.
export function alignHeadPins(head) {
  const cited = {};
  const walk = value => {
    if (Array.isArray(value)) return value.forEach(walk);
    if (!value || typeof value !== 'object') return;
    if (isPin(value)) cited[value.assetId + '/' + value.revisionId] = value.releaseId;
    Object.values(value).forEach(walk);
  };
  Object.values(head).forEach(entry => walk(entry.content));
  return Object.fromEntries(Object.entries(head).map(([address, entry]) => [address, { ...entry, releaseId: cited[entry.assetId + '/' + entry.revisionId] ?? entry.releaseId }]));
}

export function updateTarget(entry, key, fields, specAfter) {
  const content = { ...entry.content, ...fields }, changes = [];
  for (const [fieldPath, afterValue] of Object.entries(fields)) changes.push({ fieldPath, op: entry.content[fieldPath] === undefined ? 'add' : 'update', ...(entry.content[fieldPath] === undefined ? {} : { beforeValue: entry.content[fieldPath] }), afterValue });
  if (specAfter && JSON.stringify(specAfter) !== JSON.stringify(entry.content.spec)) {
    changes.push({ fieldPath: 'spec', op: 'update', beforeValue: entry.content.spec, afterValue: specAfter });
    content.spec = specAfter;
  }
  return { intent: 'update', kind: 'semantic_asset', localKey: key, targetId: entry.assetId, baseRevisionId: entry.revisionId, title: entry.content.displayName + '：业务词汇', content, changes, evidenceIds: [] };
}

// Vocabulary-only revisions keep earlier pins compatible, so dependents need no re-pinning and all wording ships in
// one operation. Environments measured before compatibility existed recorded extra re-pin stages; they stay valid.
export const vocabularyStages = ['wording'];

export function vocabularyStage(stage, head) {
  const entry = key => head[prefix + key];
  if (stage === 'wording') {
    const customers = entry('customers').content;
    const customerSpec = { ...customers.spec, members: customers.spec.members.map(m => m.id !== 'region' ? m : { ...m, aliases: ['区域', '大区'], valueAliases: [{ value: '华东', aliases: ['江浙沪', '华东地区', '华东区'] }, { value: '华南', aliases: ['两广', '华南地区', '华南区'] }] }) };
    return [
      updateTarget(entry('amount'), 'amount', {
        aliases: ['GMV', '成交额', '销售额'],
        includes: ['有效订单（is_valid = 1）的支付金额'],
        excludes: ['无效订单（is_valid ≠ 1）'],
        sampleQuestions: [{ question: '2026年8月的GMV是多少？', expects: { refs: [pin(head, 'model')] } }, { question: '江浙沪上个月的成交额' }],
      }),
      updateTarget(entry('count'), 'count', { aliases: ['单量', '订单量'], sampleQuestions: [{ question: '8月的单量是多少？' }] }),
      updateTarget(entry('customers'), 'customers', {}, customerSpec),
      { intent: 'create', kind: 'semantic_asset', localKey: 'booked', identityKey: prefix + 'booked', title: '下单金额 - 歧义演示', evidenceIds: [], changes: [], content: {
        address: prefix + 'booked', assetType: 'metric', displayName: '下单金额', ownerPrincipalId: entry('amount').content.ownerPrincipalId,
        definition: '合成演示指标：全部订单支付金额求和，含无效订单；用于演示“销售额”一词的歧义。',
        scope: '合成演示；2026 年 8 月 UTC；非真实业务数据',
        aliases: ['销售额', '下单额'],
        confusableWith: [{ ref: pin(head, 'amount'), distinction: BOOKED_DISTINCTION }],
        sampleQuestions: [{ question: '8月下单金额是多少？' }],
        spec: { kind: 'aggregate', inputRef: member(head, 'orders', 'amount'), aggregation: 'sum', unit: 'CNY', nullPolicy: 'required', timeAttributeRef: member(head, 'orders', 'paid_at') },
      } },
      updateTarget(entry('average'), 'average', { aliases: ['AOV', '平均订单金额'], excludes: ['按客户平均的金额'], sampleQuestions: [{ question: '华东回头客的客单价' }] }),
      updateTarget(entry('old'), 'old', { aliases: ['老客', '回头客'], sampleQuestions: [{ question: '回头客的客单价是多少？' }] }),
    ];
  }
  throw new Error('Unknown vocabulary stage');
}

export function vocabularyCases() {
  const from = "'2026-08-01T00:00:00Z'::timestamptz", to = "'2026-09-01T00:00:00Z'::timestamptz";
  const where = `o.is_valid=1 AND o.paid_at>=${from} AND o.paid_at<${to}`;
  const joined = 'semlia_demo_202609.orders o JOIN semlia_demo_202609.customers c ON c.id=o.customer_id';
  return [
    { id: 'alias-metric', columns: ['demo_202609.amount'], question: '2026年8月的GMV是多少？', sql: `SELECT SUM(o.amount)::text FROM semlia_demo_202609.orders o WHERE ${where}` },
    { id: 'value-alias', columns: ['demo_202609.amount'], question: '2026年8月江浙沪的成交额是多少？', sql: `SELECT SUM(o.amount)::text FROM ${joined} WHERE ${where} AND c.region='华东'` },
    { id: 'term-alias-derived', columns: ['demo_202609.average'], question: '2026年8月华东区回头客的客单价是多少？', sql: `SELECT (SUM(o.amount)/NULLIF(COUNT(DISTINCT o.id),0))::text FROM ${joined} WHERE ${where} AND c.region='华东' AND c.first_paid_at<${from}` },
    { id: 'count-alias', columns: ['demo_202609.count'], question: '2026年8月的单量是多少？', sql: `SELECT COUNT(DISTINCT o.id)::text FROM semlia_demo_202609.orders o WHERE ${where}` },
    { id: 'attribute-alias', columns: ['demo_202609.customers.region', 'demo_202609.amount'], question: '2026年8月按大区统计成交额。', sql: `SELECT c.region,SUM(o.amount)::text FROM ${joined} WHERE ${where} GROUP BY c.region ORDER BY c.region COLLATE "C"` },
    { id: 'settled-shared-word', columns: ['demo_202609.amount'], question: '2026年8月的有效支付金额（也就是销售额）是多少？', sql: `SELECT SUM(o.amount)::text FROM semlia_demo_202609.orders o WHERE ${where}` },
    { id: 'shared-word', question: '2026年8月销售额是多少？', clarify: ['有效支付金额', '下单金额'] },
    { id: 'out-of-scope', question: '2026年8月的退款金额是多少？', clarify: [] },
  ];
}

async function readHead(root) {
  const state = readState(root), base = `/api/v1/workspaces/${state.workspace.id}`;
  const get = async path => { const r = await requestAs(root, 'author', 'GET', base + path); assert.equal(r.status, 200, path); return r.body; };
  const latest = (await get('/governance/releases?limit=1')).items[0];
  const release = await get('/production-releases/' + latest.id);
  const head = {};
  for (const asset of release.afterManifest.assets) {
    const revision = await get(`/catalog/assets/${asset.assetId}/revisions/${asset.revisionId}`);
    head[revision.content.address] = { assetId: asset.assetId, revisionId: asset.revisionId, releaseId: release.id, content: revision.content };
  }
  return { state, base, release, head: alignHeadPins(head) };
}

// publishStage publishes one stage of targets through the governed path; build may supply ad hoc targets.
export async function publishStage(root, stage, print = console.log, build = vocabularyStage) {
  let { state, base, head } = await readHead(root);
  const saved = state.vocabulary?.[stage];
  if (saved?.releaseId) return state;
  const actors = {};
  for (const role of ['author', 'reviewer', 'publisher']) actors[role] = (await connect(root, state, role)).client;
  const { author, reviewer, publisher } = actors;
  const targets = saved?.targets ?? build(stage, head);
  const snapshot = state.discovery.snapshot;
  const payload = { input: { snapshots: [{ sourceId: state.sourceId, snapshotId: snapshot.id, digest: snapshot.contentDigest, coverageKeys: snapshot.coverage.filter(x => x.status === 'complete' && x.enumerationComplete).map(x => x.key).sort() }], candidates: [], evidence: [], dependencies: dependenciesOf(targets) }, targets };
  let operationId = saved?.operationId;
  if (!operationId) {
    operationId = (await author.request('POST', base + '/production-operations', payload, 'v1-vocabulary-create-' + stage)).operationId;
    state = checkpoint(root, state, { vocabulary: { ...state.vocabulary, [stage]: { operationId, targets } } });
  }
  const path = base + '/production-operations/' + operationId;
  let operation = await author.request('GET', path);
  const version = { expectedVersion: operation.version, setDigest: operation.setDigest };
  if (operation.summary.progress === 'draft') {
    for (const target of targets) await author.request('POST', path + '/business-rule-confirmations', { ...version, targetKey: target.localKey, action: 'confirm', declaration: 'Synthetic acceptance author confirms this governed business wording; this is not a real business attestation. ' + target.content.definition }, `v1-vocabulary-confirm-${stage}-${target.localKey}`);
    await author.request('POST', path + '/submit', version, 'v1-vocabulary-submit-' + stage);
  }
  // Re-read after submission; validation runs asynchronously in the worker.
  for (let i = 0; i < 240; i++) {
    operation = await author.request('GET', path);
    if (operation.summary.releaseId || !['not_requested', 'queued', 'running'].includes(operation.activeValidation?.status)) break;
    await setTimeout(500);
  }
  writePrivate(join(root, `operation-vocabulary-${stage}.json`), operation);
  if (!operation.summary.releaseId && operation.activeValidation.status !== 'succeeded') throw new Error(`Vocabulary ${stage} validation failed: ` + (operation.unresolvedCodes ?? []).join(','));
  if (!operation.summary.releaseId) {
    const validation = { attemptNo: operation.activeValidation.attemptNo, validationDigest: operation.activeValidation.validationDigest };
    await reviewer.request('POST', path + '/reviews', { ...version, validation, proposalIds: operation.targets.map(x => x.proposalId), decision: 'approve', note: 'Independent synthetic reviewer checks governed wording and sample questions; not a real business approval.' }, 'v1-vocabulary-review-' + stage);
    await publisher.request('POST', path + '/publish', { ...version, validation, expectedHead: operation.baselineHead }, 'v1-vocabulary-publish-' + stage);
    operation = await author.request('GET', path);
  }
  const releaseId = operation.summary.releaseId;
  assert.ok(releaseId, 'Vocabulary release required');
  const release = await publisher.request('GET', base + '/production-releases/' + releaseId);
  const pins = { ...state.vocabulary?.pins };
  for (const target of operation.targets) {
    const manifest = release.afterManifest.assets.find(x => x.assetId === target.targetId);
    assert.ok(manifest, 'Published vocabulary pin required');
    pins[target.targetId] = { revisionId: manifest.revisionId, releaseId };
  }
  state = checkpoint(root, state, { vocabulary: { ...state.vocabulary, pins, [stage]: { ...state.vocabulary[stage], releaseId } }, latestRelease: releaseId });
  print(JSON.stringify({ stage, release: releaseId, targets: targets.map(t => t.localKey) }));
  return state;
}

// applyVocabulary publishes every stage through the normal author, independent reviewer and publisher path. Each
// stage resumes its own operation when interrupted and is skipped once released.
export async function applyVocabulary(root, print = console.log) {
  for (const stage of vocabularyStages) await publishStage(root, stage, print);
  return readState(root).vocabulary;
}

const cell = value => {
  if (value === null || value === undefined) return null;
  const text = String(value);
  return /^-?\d+(?:\.\d+)?$/.test(text) ? text.replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '') : text;
};

// reconcile checks a successful read-only run whose named columns equal independently computed rows.
export function reconcile(item, result, expected) {
  assert.equal(result.run?.state, 'succeeded');
  assert.match(result.sql, /^\s*SELECT\b/i);
  assert.equal(result.columns?.length, item.columns.length);
  const indexes = item.columns.map(name => result.columns.indexOf(name));
  assert.ok(indexes.every(index => index >= 0), 'unexpected result columns');
  const normalize = rows => rows.map(row => row.map(cell)).sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));
  assert.deepEqual(normalize(result.rows.map(row => indexes.map(index => row[index]))), normalize(expected));
}

export function judgeVocabularyCase(item, asked, executed, rows) {
  const interpretation = asked.body?.interpretation ?? {};
  if (item.clarify) {
    const text = interpretation.clarification ?? '';
    const passed = asked.status === 200 && interpretation.outcome === 'clarification' && !asked.body?.resolution && item.clarify.every(name => text.includes(name));
    return { passed, outcome: interpretation.outcome ?? asked.body?.code ?? 'error', clarification: text.slice(0, 160) };
  }
  if (!executed) return { passed: false, outcome: interpretation.outcome ?? asked.body?.code ?? 'error', clarification: interpretation.clarification?.slice(0, 160) };
  try { reconcile(item, executed.body, rows); return { passed: executed.status === 200, outcome: 'answered', rows: executed.body.rows }; }
  catch { return { passed: false, outcome: 'wrong-answer', rows: executed.body?.rows }; }
}

// measureVocabulary asks every case as the consumer, executes produced plans and reconciles them with independent SQL.
export async function measureVocabulary(root, phase, rounds = 3, print = console.log) {
  const state = readState(root), base = `/api/v1/workspaces/${state.workspace.id}`, stamp = Date.now();
  const ledger = { phase, startedAt: new Date().toISOString(), attempts: [] };
  for (let round = 1; round <= rounds; round++) {
    for (const item of vocabularyCases()) {
      const key = `v1-vocabulary-${phase}-${item.id}-${round}-${stamp}`, before = counters(root);
      const asked = await requestAs(root, 'consumer', 'POST', base + '/ask', { question: item.question, context: { mode: 'current' }, idempotencyKey: key });
      const plan = asked.body?.resolution?.plan;
      let executed, rows;
      if (item.sql && asked.status === 200 && plan) {
        executed = await requestAs(root, 'consumer', 'POST', base + `/resolved-semantic-plans/${plan.id}:execute`, { planDigest: plan.planDigest, idempotencyKey: key + '-execute' });
        rows = independentRows(root, item.sql);
      }
      const after = counters(root);
      const verdict = judgeVocabularyCase(item, asked, executed, rows);
      const attempt = { case: item.id, round, ...verdict, modelSteps: after.modelSteps - before.modelSteps, executions: after.executions - before.executions };
      ledger.attempts.push(attempt);
      print(JSON.stringify(attempt));
    }
  }
  ledger.summary = Object.fromEntries(vocabularyCases().map(item => [item.id, ledger.attempts.filter(a => a.case === item.id && a.passed).length + '/' + rounds]));
  ledger.passed = ledger.attempts.filter(a => a.passed).length;
  ledger.total = ledger.attempts.length;
  writePrivate(join(root, `vocabulary-${phase}-${stamp}.json`), ledger);
  print(JSON.stringify({ phase, passed: ledger.passed, total: ledger.total, summary: ledger.summary }));
  return ledger;
}

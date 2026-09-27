import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { setTimeout } from 'node:timers/promises';
import { DemoClient } from '../../demo/client.mjs';
import { ensureBinding } from '../../demo/initialize.mjs';
import { allPages, discoverSource } from '../../demo/discovery.mjs';
import { databaseQuery } from '../../demo/provision.mjs';
import { verifyProvisioned } from './provision.mjs';
import { repo } from './runtime.mjs';
import { checkpoint, readState, readPrivate, writePrivate, assertSelfReviewProof, digest } from './core.mjs';

export async function connect(root, state, actor) {
  if (!['admin', 'author', 'reviewer', 'publisher', 'consumer', 'denied'].includes(actor)) throw new Error('Unknown acceptance actor');
  const path = join(root, 'sessions', actor + '.json'), origin = `http://127.0.0.1:${state.apiPort}`;
  const client = new DemoClient(origin);
  if (existsSync(path)) {
    const saved = readPrivate(path);
    if (saved.owner !== state.owner || saved.origin !== origin || saved.actor !== actor) throw new Error('Session ownership mismatch');
    try { return { client, session: await client.resume(saved.cookie) }; }
    catch (error) { if (error.status !== 401) throw error; }
  }
  const session = await client.login('v1_' + actor, readPrivate(join(root, 'secrets.json')).accountPassword);
  writePrivate(path, { owner: state.owner, origin, actor, cookie: client.sessionCookie() });
  return { client, session };
}

export async function initializeActors(root) {
  let state = readState(root);
  const { client: admin, session } = await connect(root, state, 'admin');
  assert.equal(session.workspaces.length, 1);
  const workspace = session.workspaces[0];
  assert.equal(workspace.slug, state.owner.replaceAll('_', '-'));
  if (state.workspace) assert.equal(workspace.id, state.workspace.id);
  else state = checkpoint(root, state, { workspace, principals: { admin: workspace.principalId } });
  const base = `/api/v1/workspaces/${workspace.id}`;
  const principals = { ...state.principals }, password = readPrivate(join(root, 'secrets.json')).accountPassword;
  for (const [actor, role] of [['author', 'semantic_steward'], ['reviewer', 'reviewer'], ['publisher', 'publisher'], ['consumer', 'consumer_developer'], ['denied', 'auditor']]) {
    const label = `V1 synthetic ${actor}`;
    const members = await admin.request('GET', base + '/members');
    const found = members.items.filter(item => item.displayName === label);
    assert.ok(found.length < 2, 'Ambiguous synthetic account');
    if (!found.length) await admin.request('POST', base + '/members', { username: 'v1_' + actor, displayName: label, password, roleId: role });
    const { session: actorSession } = await connect(root, state, actor);
    assert.equal(actorSession.workspaces.length, 1);
    assert.equal(actorSession.workspaces[0].id, workspace.id);
    principals[actor] = actorSession.workspaces[0].principalId;
    if (actor === 'author') {
      await ensureBinding(admin, workspace.id, principals[actor], 'source_operator');
      await ensureBinding(admin, workspace.id, principals[actor], 'reviewer');
    }
    if (['reviewer', 'publisher'].includes(actor)) await ensureBinding(admin, workspace.id, principals[actor], 'auditor');
  }
  assert.equal(new Set(Object.values(principals)).size, 6, 'All synthetic actors must be distinct');
  state = checkpoint(root, state, { principals });
  return state;
}

export async function initializeSource(root) {
  let state = readState(root);
  const { client: admin } = await connect(root, state, 'admin');
  const base = `/api/v1/workspaces/${state.workspace.id}`;
  const secret = readPrivate(join(root, 'secrets.json'));
  if (!state.sourceId) {
    const sources = await allPages(admin, base + '/sources?limit=100');
    const owned = sources.filter(x => x.name === 'V1 synthetic orders and customers');
    if (sources.length !== owned.length || owned.length > 1) throw new Error('Unexpected source in isolated workspace');
    const source = owned[0] ?? await admin.request('POST', base + '/sources', { name: 'V1 synthetic orders and customers', host: '127.0.0.1', port: state.database.port, database: state.owner + '_source', username: state.owner + '_reader', password: secret.readerPassword, sslMode: 'disable' });
    if (source.host !== '127.0.0.1' || source.port !== state.database.port || source.database !== state.owner + '_source' || source.username !== state.owner + '_reader') throw new Error('Source configuration ownership mismatch');
    state = checkpoint(root, state, { sourceId: source.id });
  }
  if (!state.discovery) {
    await admin.request('POST', base + `/sources/${state.sourceId}/test`, {});
    const { client: author } = await connect(root, state, 'author');
    const discovery = await discoverSource(author, { id: state.workspace.id, sourceId: state.sourceId }, 'v1-baseline-discovery');
    const datasets = discovery.members.filter(x => x.kind === 'dataset');
    assert.equal(datasets.length, 2, 'Only two synthetic tables may be discovered');
    assert.equal(discovery.members.filter(x => x.kind === 'field').length, 8);
    state = checkpoint(root, state, { discovery });
  }
  return state;
}

export async function initializeModel(root) {
  let state = readState(root);
  const { client } = await connect(root, state, 'admin');
  const secret = readPrivate(join(root, 'secrets.json')), model = secret.model;
  const base = `/api/v1/workspaces/${state.workspace.id}/governance`;
  const providers = (await client.request('GET', base + '/model-providers')).items;
  if (providers.length > 1) throw new Error('Unexpected models in acceptance workspace');
  let detail = providers[0];
  if (!detail) detail = await client.request('POST', base + '/model-providers', { protocol: model.protocol, displayName: 'V1 configured model - synthetic inputs only', ...(model.baseUrl ? { baseUrl: model.baseUrl } : {}), credentialEnv: 'SEMLIA_V1_MODEL_SECRET', credential: secret.modelSecret });
  assert.equal(detail.provider.credentialEnv, 'SEMLIA_V1_MODEL_SECRET');
  assert.equal(detail.provider.protocol, model.protocol);
  assert.equal(detail.provider.baseUrl ?? '', model.baseUrl ?? '');
  assert.equal(detail.provider.enabled, true);
  let setting = detail.models[0];
  if (detail.models.length > 1) throw new Error('Unexpected model settings');
  if (!setting) setting = await client.request('POST', base + '/model-settings', { providerId: detail.provider.id, kind: 'llm', model: model.model, capability: 'Synthetic V1 interpretation and governed suggestions', tokenLimit: model.tokenLimit });
  assert.equal(setting.model, model.model);
  assert.equal(setting.enabled, true);
  assert.equal(setting.kind, 'llm');
  assert.equal(setting.tokenLimit, model.tokenLimit);
  if (!setting.isDefault) setting = await client.request('POST', base + `/model-settings/${setting.id}/set-default`, {});
  return checkpoint(root, state, { model: { providerId: detail.provider.id, settingId: setting.id, protocol: model.protocol, name: model.model, sourceWorkspace: model.workspace, sourceConfigurationFingerprint: digest(JSON.stringify(model)) } });
}

function physical(state, kind, table, field) {
  const datasets = state.discovery.members.filter(x => x.kind === 'dataset' && x.name === 'semlia_demo_202609.' + table);
  assert.equal(datasets.length, 1, 'Unique discovered dataset required');
  const item = kind === 'dataset' ? datasets[0] : state.discovery.members.find(x => x.kind === 'field' && x.parentObjectId === datasets[0].objectId && x.name === field);
  assert.ok(item, 'Discovered field required');
  return { snapshotId: state.discovery.snapshot.id, kind, objectId: item.objectId, revisionId: item.revisionId };
}
function remap(value, state, fixture) {
  if (Array.isArray(value)) return value.map(x => remap(x, state, fixture));
  if (!value || typeof value !== 'object') {
    if (typeof value === 'string' && value === fixture.Snapshot.Execution.joins[0].id) return state.published.join.targetId;
    return value;
  }
  if (value.assetId && value.revisionId && value.releaseId) {
    const asset = fixture.Snapshot.Assets.find(x => x.AssetID === value.assetId);
    const pin = state.published?.[asset?.Address];
    assert.ok(pin, 'Every knowledge reference must already be published');
    return { assetId: pin.targetId, revisionId: pin.revisionId, releaseId: pin.releaseId, ...(value.memberId ? { memberId: value.memberId } : {}) };
  }
  if (value.snapshotId && value.objectId && value.revisionId) {
    for (const [key, table] of [['order_data', 'orders'], ['customer_data', 'customers']]) {
      const asset = fixture.Snapshot.Assets.find(x => x.Address.endsWith('.' + key));
      if (asset.Content.spec.datasetRef.objectId === value.objectId) return physical(state, 'dataset', table);
      const member = asset.Content.spec.members.find(x => x.sourceFieldRef.objectId === value.objectId);
      if (member) return physical(state, 'field', table, member.id);
    }
    throw new Error('Unknown fixture source reference');
  }
  return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, remap(item, state, fixture)]));
}
async function terminal(author, path) {
  for (let i = 0; i < 240; i++) {
    const operation = await author.request('GET', path);
    if (!['queued', 'running'].includes(operation.activeValidation.status)) return operation;
    await setTimeout(500);
  }
  throw new Error('Validation remains active; resume the same operation');
}
export async function publishTarget(root, key, target) {
  let state = readState(root);
  const actors = {};
  for (const role of ['author', 'reviewer', 'publisher']) actors[role] = (await connect(root, state, role)).client;
  const { author, reviewer, publisher } = actors, base = `/api/v1/workspaces/${state.workspace.id}`;
  const snapshot = state.discovery.snapshot;
  const dependencies = Object.values(state.published ?? {}).filter(pin => pin.kind === 'semantic_asset').map(({ kind, targetId, releaseId, revisionId }) => ({ kind, targetId, releaseId, revisionId }));
  const payload = { input: { snapshots: [{ sourceId: state.sourceId, snapshotId: snapshot.id, digest: snapshot.contentDigest, coverageKeys: snapshot.coverage.filter(x => x.status === 'complete' && x.enumerationComplete).map(x => x.key).sort() }], candidates: [], evidence: [], dependencies }, targets: [target] };
  const saved = state.operations?.[key];
  let operationId = saved?.operationId;
  if (!operationId) {
    const created = await author.request('POST', base + '/production-operations', payload, 'v1-create-' + key);
    operationId = created.operationId;
    state = checkpoint(root, state, { operations: { ...state.operations, [key]: { operationId, payload } } });
  } else assert.deepEqual(saved.payload, payload, 'Refusing changed resume payload');
  const path = base + '/production-operations/' + operationId;
  let operation = await author.request('GET', path);
  assert.deepEqual(operation.targets.map(x => x.declaration), [target]);
  const pin = { expectedVersion: operation.version, setDigest: operation.setDigest };
  if (!operation.summary.releaseId) {
    if (operation.summary.progress === 'draft') {
      if (target.kind === 'semantic_asset') await author.request('POST', path + '/business-rule-confirmations', { ...pin, targetKey: target.localKey, action: 'confirm', declaration: 'Synthetic acceptance author confirms this fixed definition and structured rule; this is not a real business attestation. ' + target.content.definition }, 'v1-confirm-' + key);
      await author.request('POST', path + '/submit', pin, 'v1-submit-' + key);
    }
    operation = await terminal(author, path);
    writePrivate(join(root, 'operation-' + key + '.json'), operation);
    if (operation.activeValidation.status !== 'succeeded') throw new Error(`Validation failed for ${key}: ${(operation.unresolvedCodes ?? []).join(',')}`);
    const validation = { attemptNo: operation.activeValidation.attemptNo, validationDigest: operation.activeValidation.validationDigest };
    const review = { ...pin, validation, proposalIds: operation.targets.map(x => x.proposalId), decision: 'approve', note: 'Independent synthetic reviewer verifies fixed source pins and structured rules; not a real business approval.' };
    await reviewer.request('POST', path + '/reviews', review, 'v1-review-' + key);
    const expectedHead = operation.baselineHead;
    await publisher.request('POST', path + '/publish', { ...pin, validation, expectedHead }, 'v1-publish-' + key);
    operation = await author.request('GET', path);
  }
  assert.ok(operation.summary.releaseId, 'Normal production release required');
  const release = await publisher.request('GET', base + `/production-releases/${operation.summary.releaseId}`);
  assert.equal(release.publishedBy, state.principals.publisher);
  const id = operation.targets[0].targetId;
  const manifest = target.kind === 'semantic_asset' ? release.afterManifest.assets.find(x => x.assetId === id) : release.afterManifest.objects.find(x => x.targetId === id || x.objectId === id);
  assert.ok(manifest, 'Published target manifest pin required');
  const published = { kind: target.kind, targetId: id, releaseId: release.id, ...(target.kind === 'semantic_asset' ? { revisionId: manifest.revisionId } : { objectVersion: manifest.objectVersion, contentDigest: manifest.contentDigest }) };
  state = checkpoint(root, state, { published: { ...state.published, [key]: published }, latestRelease: release.id });
  return state;
}

export async function initializeKnowledge(root, fixture) {
  let state = readState(root);
  for (const asset of fixture.Snapshot.Assets) {
    if (state.published?.[asset.Address]) continue;
    if (asset.AssetType === 'analysis_model') {
      for (const [key, table] of [['order_data', 'orders'], ['customer_data', 'customers']]) {
        const localKey = table + '_binding';
        if (state.published?.[localKey]) continue;
        const published = state.published['demo_202609.' + key];
        const { kind, targetId, revisionId, releaseId } = published;
        state = await publishTarget(root, localKey, { intent: 'create', kind: 'physical_binding', localKey, identityKey: 'v1.' + localKey, title: 'Synthetic source binding', content: { asset: { kind, targetId, revisionId, releaseId }, dataset: physical(state, 'dataset', table) }, changes: [], evidenceIds: [] });
      }
      if (!state.published?.join) state = await publishTarget(root, 'join', { intent: 'create', kind: 'join_contract', localKey: 'join', identityKey: 'v1.orders_customers', title: 'Synthetic many-to-one join', content: { leftDataset: physical(state, 'dataset', 'orders'), rightDataset: physical(state, 'dataset', 'customers'), pairs: [{ left: physical(state, 'field', 'orders', 'customer_id'), right: physical(state, 'field', 'customers', 'id') }], joinType: 'left', cardinality: 'many_to_one', expression: 'field_pairs_equal/v1', notes: 'Synthetic fixed source relationship; customer primary key is unique.' }, changes: [], evidenceIds: [] });
    }
    const content = remap(asset.Content, state, fixture);
    state = await publishTarget(root, asset.Address, { intent: 'create', kind: 'semantic_asset', localKey: asset.Address.split('.').at(-1), identityKey: asset.Address, title: asset.Name + ' - V1 synthetic', content: { address: asset.Address, assetType: asset.AssetType, displayName: asset.Name, definition: content.definition, scope: content.scope, ownerPrincipalId: state.principals.author, spec: content.spec }, changes: [], evidenceIds: [] });
  }
  return proveIndependentReview(root);
}

export async function proveIndependentReview(root) {
  const state = readState(root), { client, session } = await connect(root, state, 'author');
  const hasReviewCapability = session.workspaces[0].capabilities.includes('proposal.review');
  assert.equal(hasReviewCapability, true, 'Self-review probe must reach the independence check');
  const operationId = state.operations['demo_202609.orders'].operationId;
  const path = `/api/v1/workspaces/${state.workspace.id}/production-operations/${operationId}`;
  const operation = await client.request('GET', path);
  const origin = `http://127.0.0.1:${state.apiPort}`;
  const headers = { Cookie: client.sessionCookie(), Origin: origin, Accept: 'application/json' };
  const currentSession = await fetch(origin + '/api/v1/session', { headers, redirect: 'error', signal: AbortSignal.timeout(30000) });
  assert.equal(currentSession.status, 200);
  const csrf = currentSession.headers.get('X-Semlia-CSRF');
  await currentSession.body?.cancel();
  assert.ok(csrf);
  const response = await fetch(origin + path + '/reviews', { method: 'POST', headers: { ...headers, 'Content-Type': 'application/json', 'X-Semlia-CSRF': csrf, 'Idempotency-Key': 'v1-independent-self-review-probe' }, body: JSON.stringify({ expectedVersion: operation.version, setDigest: operation.setDigest, validation: { attemptNo: operation.activeValidation.attemptNo, validationDigest: operation.activeValidation.validationDigest }, proposalIds: operation.targets.map(x => x.proposalId), decision: 'approve', note: 'Synthetic self-review denial probe; never an independent approval.' }), redirect: 'error', signal: AbortSignal.timeout(30000) });
  const body = await response.json();
  const proof = { operationId, principalId: state.principals.author, hasReviewCapability, status: response.status, code: body.code };
  assertSelfReviewProof(proof);
  return checkpoint(root, state, { selfReviewDenied: true, selfReviewProof: proof });
}

export async function verify(root) {
  const state = readState(root), { client } = await connect(root, state, 'admin');
  const base = `/api/v1/workspaces/${state.workspace.id}`;
  const assets = await allPages(client, base + '/catalog/assets?limit=100');
  const releases = await allPages(client, base + '/governance/releases?limit=100');
  const operations = await allPages(client, base + '/production-operations?limit=100');
  assert.ok(assets.length >= 10); assert.equal(new Set(assets.map(x => x.assetType)).size, 5);
  assert.ok(releases.length >= 13); assert.ok(operations.length >= 13);
  assert.equal(Object.keys(state.published).length, 13);
  const releaseIds = new Set(releases.map(x => x.id)), operationIds = new Set(operations.map(x => x.operationId ?? x.id));
  for (const operation of Object.values(state.operations)) assert.ok(operationIds.has(operation.operationId), 'An original operation is missing');
  for (const pin of Object.values(state.published)) assert.ok(releaseIds.has(pin.releaseId), 'An original release is missing');
  assert.equal(state.selfReviewDenied, true);
  assertSelfReviewProof(state.selfReviewProof);
  const release = await client.request('GET', base + '/production-releases/' + state.latestRelease);
  assert.equal(release.afterManifest.assets.length, 10);
  assert.equal(release.afterManifest.objects.length, 3);
  for (const pin of Object.values(state.published)) {
    if (pin.kind === 'semantic_asset') assert.ok(release.afterManifest.assets.some(x => x.assetId === pin.targetId && x.revisionId === pin.revisionId));
    else assert.ok(release.afterManifest.objects.some(x => (x.targetId ?? x.objectId) === pin.targetId && x.objectVersion === pin.objectVersion && x.contentDigest === pin.contentDigest));
  }
  const target = verifyProvisioned(repo, state);
  const calls = JSON.parse(databaseQuery(target.container, state.owner + '_app', "SELECT json_build_object('agentRuns',(SELECT count(*) FROM agent_runs),'modelSteps',(SELECT count(*) FROM agent_steps WHERE kind='model'),'executions',(SELECT count(*) FROM query_execution_runs));"));
  const modelLedger = existsSync(join(root, 'model-acceptance.json')) ? readPrivate(join(root, 'model-acceptance.json')) : { attempts: [] };
  const report = { synthetic: true, knowledgeTypes: 5, assets: assets.length, baselineGovernedObjects: 3, releases: releases.length, operations: operations.length, sourceDatasets: 2, sourceFields: 8, selfReviewDenied: true, realPasswordSessions: 6, baselineModelCalls: 0, actualAgentRuns: calls.agentRuns, actualModelSteps: calls.modelSteps, actualExecutions: calls.executions, recordedAskAttempts: modelLedger.attempts.length, baselineRelease: state.latestRelease, latestRelease: releases[0].id };
  if (!existsSync(join(root, 'baseline-report.json'))) writePrivate(join(root, 'baseline-report.json'), report, true);
  writePrivate(join(root, 'runtime-verification.json'), report);
  return report;
}

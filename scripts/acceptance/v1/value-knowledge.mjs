import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { lstatSync, readFileSync, readdirSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { setTimeout } from 'node:timers/promises';
import { digest, fixtureDigest, readState, readPrivate, writePrivate, rejectLinks } from './core.mjs';
import { requestAs, counters } from './model.mjs';
import { verifyProvisioned } from './provision.mjs';

export const valueGlossarySuffix = '\n\n字段值词汇：demo_202609.customers.region（客户当前区域）的规范值为“华东”和“华南”；业务别名“华东地区”对应“华东”，“华南地区”对应“华南”。这些是精确类别值，不表示模糊包含匹配。';
const ledgerName = 'value-knowledge.json', pointerName = 'value-knowledge-approved.json', modelKey = 'demo_202609.model';
const reconciliationName = 'value-knowledge-reconciliation.json', commandName = 'value-knowledge-root-command.json';
const rootEvidenceNames = [commandName, 'value-knowledge-root-before.json', 'value-knowledge-root-proof-failure.json', 'value-knowledge-root-after-failed.json'];
const categories = ['华东', '华南'];
const jsonDigest = value => digest(JSON.stringify(value));
const modelPin = (release, state) => {
  const pins = release.afterManifest.assets.filter(x => x.assetId === state.published[modelKey].targetId);
  assert.equal(pins.length, 1); return pins[0];
};
const head = release => ({ presence: 'present', releaseId: release.id, manifestDigest: release.afterManifest.digest });
// Projection delivery is live operational state, not part of the immutable release.
const releaseFacts = ({ projectionStatus: _projectionStatus, ...facts }) => facts;
const sha256 = value => assert.match(value ?? '', /^sha256:[a-f0-9]{64}$/);
function privatePath(path, directory = false) {
  rejectLinks(path); const stat = lstatSync(path);
  assert.ok(!stat.isSymbolicLink() && (directory ? stat.isDirectory() : stat.isFile()));
  assert.equal(stat.uid, process.getuid()); assert.equal(stat.mode & 0o777, directory ? 0o700 : 0o600);
  return stat;
}
function protectedJSON(path) { privatePath(path); return readPrivate(path); }
function initialKnowledge(root) {
  privatePath(root, true); privatePath(join(root, 'state.json'));
  const state = readState(root), fixture = protectedJSON(join(root, 'fixture.json'));
  assert.equal(fixtureDigest(fixture), state.fixtureDigest); assert.equal(digest(fixture.DataSQL), state.sourceDataDigest);
  const initial = state.operations[modelKey].payload.targets;
  assert.equal(initial.length, 1); assert.equal(initial[0].intent, 'create');
  const original = initial[0].content, template = fixture.Snapshot.Assets.filter(x => x.Address === modelKey);
  assert.equal(template.length, 1);
  for (const field of ['assetType', 'definition', 'scope']) assert.deepEqual(original[field], template[0].Content[field]);
  return { state, original };
}

export function supplementContent(original) {
  assert.equal(typeof original.definition, 'string'); assert.ok(!original.definition.includes(valueGlossarySuffix));
  return { ...structuredClone(original), definition: original.definition + valueGlossarySuffix };
}

export function definitionOnlyTarget(original, pin) {
  return { intent: 'update', kind: 'semantic_asset', localKey: 'model', targetId: pin.assetId, baseRevisionId: pin.revisionId, title: 'Synthetic field category glossary', content: supplementContent(original), changes: [{ fieldPath: 'definition', op: 'update', beforeValue: original.definition, afterValue: original.definition + valueGlossarySuffix }], evidenceIds: [] };
}

async function get(root, request, path, actor = 'admin') {
  const response = await request(root, actor, 'GET', path); assert.equal(response.status, 200); return response.body;
}
function validateOriginalRelease(release, state) {
  assert.equal(release.id, state.latestRelease); sha256(release.afterManifest.digest);
  const published = Object.values(state.published), assets = published.filter(x => x.kind === 'semantic_asset'), objects = published.filter(x => x.kind !== 'semantic_asset');
  assert.equal(release.afterManifest.assets.length, assets.length); assert.equal(release.afterManifest.objects.length, objects.length);
  for (const pin of assets) {
    const actual = release.afterManifest.assets.filter(x => x.assetId === pin.targetId); assert.equal(actual.length, 1); assert.equal(actual[0].revisionId, pin.revisionId);
  }
  for (const pin of objects) {
    assert.ok(['physical_binding', 'join_contract'].includes(pin.kind));
    const actual = release.afterManifest.objects.filter(x => x.kind === pin.kind && x.targetId === pin.targetId); assert.equal(actual.length, 1);
    assert.equal(actual[0].objectVersion, pin.objectVersion); assert.equal(actual[0].contentDigest, pin.contentDigest);
  }
}
export async function readKnowledgeHead(root, request = requestAs) {
  const { state, original } = initialKnowledge(root), base = `/api/v1/workspaces/${state.workspace.id}`;
  const originalRelease = await get(root, request, base + '/production-releases/' + state.latestRelease);
  validateOriginalRelease(originalRelease, state);
  const latest = (await get(root, request, base + '/governance/releases?limit=1')).items[0]; assert.ok(latest?.id);
  const release = await get(root, request, base + '/production-releases/' + latest.id), saved = state.published[modelKey];
  assert.equal(release.id, latest.id);
  const baseline = await get(root, request, base + `/catalog/assets/${saved.targetId}/revisions/${saved.revisionId}`);
  assert.equal(baseline.assetId, saved.targetId); assert.equal(baseline.id, saved.revisionId); assert.deepEqual(baseline.content, original);
  const pin = modelPin(release, state), current = await get(root, request, base + `/catalog/assets/${pin.assetId}/revisions/${pin.revisionId}`);
  assert.equal(current.assetId, pin.assetId); assert.equal(current.id, pin.revisionId);
  return { state, original, originalRevision: baseline, originalRelease, release, current };
}

export function sourceBindingProof(state, original, data, source, snapshot, bindingOperation, release, bindingRelease) {
  const customers = state.published['demo_202609.customers'], savedData = state.published['demo_202609.customer_data'];
  const publicRefs = original.spec.publicAttributeRefs.filter(x => x.assetId === customers.targetId && x.memberId === 'region'); assert.equal(publicRefs.length, 1);
  const publicRef = { assetId: customers.targetId, revisionId: customers.revisionId, releaseId: customers.releaseId, memberId: 'region' };
  assert.deepEqual(publicRefs[0], publicRef);
  const bindings = original.spec.memberBindings.filter(x => x.semanticRef.assetId === customers.targetId && x.semanticRef.memberId === 'region'); assert.equal(bindings.length, 1);
  assert.deepEqual(bindings[0].semanticRef, publicRef);
  const dataRef = { assetId: savedData.targetId, revisionId: savedData.revisionId, releaseId: savedData.releaseId, memberId: 'region' };
  assert.deepEqual(bindings[0].dataRef, dataRef); assert.equal(data.assetId, dataRef.assetId); assert.equal(data.id, dataRef.revisionId);
  assert.ok(original.spec.dataAssetRefs.some(x => x.assetId === savedData.targetId && x.revisionId === savedData.revisionId && x.releaseId === savedData.releaseId));
  const datasets = state.discovery.members.filter(x => x.kind === 'dataset' && x.name === 'semlia_demo_202609.customers'); assert.equal(datasets.length, 1);
  const fields = state.discovery.members.filter(x => x.kind === 'field' && x.parentObjectId === datasets[0].objectId && x.name === 'region'); assert.equal(fields.length, 1);
  const reference = (item, kind) => ({ snapshotId: state.discovery.snapshot.id, kind, objectId: item.objectId, revisionId: item.revisionId });
  const dataset = reference(datasets[0], 'dataset'), field = reference(fields[0], 'field');
  assert.deepEqual(data.content.spec.datasetRef, dataset);
  const members = data.content.spec.members.filter(x => x.id === 'region'); assert.equal(members.length, 1); assert.deepEqual(members[0].sourceFieldRef, field);
  assert.equal(source.id, state.sourceId); assert.equal(source.host, '127.0.0.1'); assert.equal(source.port, state.database.port);
  assert.equal(source.database, state.owner + '_source'); assert.equal(source.username, state.owner + '_reader'); assert.equal(source.sslMode, 'disable');
  assert.equal(snapshot.id, dataset.snapshotId); assert.equal(snapshot.sourceId, source.id); assert.equal(snapshot.contentDigest, state.discovery.snapshot.contentDigest);
  const publishedBinding = state.published.customers_binding, object = release.afterManifest.objects.filter(x => x.targetId === publishedBinding.targetId);
  assert.equal(object.length, 1); assert.equal(object[0].kind, 'physical_binding'); assert.equal(object[0].objectVersion, publishedBinding.objectVersion); assert.equal(object[0].contentDigest, publishedBinding.contentDigest);
  assert.equal(bindingOperation.summary.id, state.operations.customers_binding.operationId); assert.equal(bindingOperation.summary.releaseId, publishedBinding.releaseId); assert.equal(bindingOperation.summary.progress, 'released');
  assert.equal(bindingRelease.id, publishedBinding.releaseId); assert.equal(bindingRelease.attribution.role, 'applied');
  assert.equal(bindingRelease.attribution.operationId, bindingOperation.summary.id); assert.equal(bindingRelease.attribution.version, bindingOperation.version); assert.equal(bindingRelease.attribution.setDigest, bindingOperation.setDigest);
  assert.ok(Number.isInteger(bindingOperation.version) && bindingOperation.version > 0); sha256(bindingOperation.setDigest); sha256(bindingRelease.afterManifest.digest);
  const publishedObjects = bindingRelease.afterManifest.objects.filter(x => x.kind === 'physical_binding' && x.targetId === publishedBinding.targetId);
  assert.equal(publishedObjects.length, 1); assert.equal(publishedObjects[0].objectVersion, publishedBinding.objectVersion);
  const targets = bindingOperation.targets.filter(x => x.targetId === publishedBinding.targetId); assert.equal(targets.length, 1); sha256(targets[0].contentDigest);
  assert.equal(targets[0].outcome, 'proposal'); assert.equal(targets[0].proposalState, 'released');
  assert.deepEqual(bindingOperation.targets.map(x => x.declaration), state.operations.customers_binding.payload.targets);
  for (const pin of [object[0], publishedObjects[0]]) if (pin.contentDigest !== undefined) assert.equal(targets[0].contentDigest, pin.contentDigest);
  assert.deepEqual(targets[0].declaration.content, { asset: { kind: 'semantic_asset', targetId: savedData.targetId, revisionId: savedData.revisionId, releaseId: savedData.releaseId }, dataset });
  return { sourceId: source.id, snapshotId: snapshot.id, snapshotDigest: snapshot.contentDigest, publicRef, dataRef, dataset, field, binding: object[0], bindingContentDigest: targets[0].contentDigest, bindingOperationId: bindingOperation.summary.id, bindingOperationVersion: bindingOperation.version, bindingSetDigest: bindingOperation.setDigest, bindingReleaseId: bindingRelease.id, bindingManifestDigest: bindingRelease.afterManifest.digest, schema: 'semlia_demo_202609', relation: 'customers', column: 'region' };
}

export const categorySQL = `BEGIN READ ONLY;
SELECT json_build_object('role',current_user,'database',current_database(),'readOnly',current_setting('transaction_read_only'),'schemaMarker',obj_description('semlia_demo_202609'::regnamespace,'pg_namespace'),'values',(SELECT json_agg(region ORDER BY region COLLATE "C") FROM (SELECT DISTINCT region FROM semlia_demo_202609.customers LIMIT 3) categories));
COMMIT;`;

export function validateSourceCategories(value, state, binding) {
  assert.equal(value.role, state.owner + '_reader'); assert.equal(value.database, state.owner + '_source'); assert.equal(value.readOnly, 'on');
  assert.equal(value.schemaMarker, state.owner + ':' + state.sourceDataDigest); assert.deepEqual(value.values, categories);
  assert.equal(binding.schema, 'semlia_demo_202609'); assert.equal(binding.relation, 'customers'); assert.equal(binding.column, 'region');
  return { ...value, bindingDigest: jsonDigest(binding), queryDigest: digest(categorySQL) };
}

export function readSourceCategories(root, binding, { provision = verifyProvisioned, spawn = spawnSync, environment = process.env } = {}) {
  const { state } = initialKnowledge(root), target = provision(resolve('.'), state), secrets = protectedJSON(join(root, 'secrets.json'));
  assert.match(secrets.readerPassword ?? '', /^[a-f0-9]{64}$/);
  const env = Object.fromEntries(['PATH', 'HOME', 'TMPDIR', 'DOCKER_HOST', 'DOCKER_CONTEXT', 'DOCKER_CONFIG'].filter(key => environment[key] !== undefined).map(key => [key, environment[key]]));
  env.PGPASSWORD = secrets.readerPassword; env.PGOPTIONS = '-c default_transaction_read_only=on -c statement_timeout=10000';
  const args = ['exec', '-i', '--env', 'PGPASSWORD', '--env', 'PGOPTIONS', target.container, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-h', '127.0.0.1', '-U', state.owner + '_reader', '-d', state.owner + '_source'];
  const result = spawn('docker', args, { env, input: categorySQL, encoding: 'utf8', timeout: 15000, maxBuffer: 1024 * 1024 });
  if (result.error || result.status !== 0) throw new Error('SOURCE_READER_PROOF_FAILED');
  return validateSourceCategories(JSON.parse(result.stdout), state, binding);
}

function validateManifests(before, after, state, beforeContentDigest) {
  const oldPin = modelPin(before, state), newPin = modelPin(after, state);
  assert.notEqual(newPin.revisionId, oldPin.revisionId); sha256(before.afterManifest.digest); sha256(after.afterManifest.digest); assert.notEqual(before.afterManifest.digest, after.afterManifest.digest);
  assert.deepEqual(after.beforeHead, head(before)); assert.deepEqual(after.beforeManifest, before.afterManifest);
  assert.deepEqual(after.afterManifest, { ...before.afterManifest, digest: after.afterManifest.digest, assets: before.afterManifest.assets.map(x => x.assetId === oldPin.assetId ? { ...x, revisionId: newPin.revisionId } : x) });
  sha256(beforeContentDigest);
  assert.deepEqual(after.beforePins, [{ presence: 'present', kind: 'semantic_asset', targetId: oldPin.assetId, revisionId: oldPin.revisionId, contentDigest: beforeContentDigest }]);
}

export function verifySupplementReceipt(receipt, view) {
  assert.equal(receipt.complete, true); assert.equal(receipt.state, 'complete');
  return verifySupplementFacts(receipt, view);
}

export function verifySupplementFacts(receipt, view) {
  const { state, original, originalRevision, originalRelease, release, current } = view;
  assert.equal(receipt.format, 1); assert.equal(receipt.owner, state.owner);
  assert.equal(receipt.fixtureDigest, state.fixtureDigest); assert.deepEqual(receipt.original, original); assert.equal(receipt.suffix, valueGlossarySuffix);
  validateOriginalRelease(originalRelease, state); assert.deepEqual(releaseFacts(receipt.originalRelease), releaseFacts(originalRelease));
  assert.deepEqual(receipt.beforeRelease.afterManifest, originalRelease.afterManifest, 'Supplement requires the complete original published manifest');
  const principals = { author: state.principals.author, reviewer: state.principals.reviewer, publisher: state.principals.publisher };
  assert.ok(Object.values(principals).every(x => typeof x === 'string' && x.length > 0)); assert.equal(new Set(Object.values(principals)).size, 3); assert.deepEqual(receipt.principals, principals);
  assert.deepEqual(releaseFacts(receipt.afterRelease), releaseFacts(release)); assert.deepEqual(receipt.current, current); assert.deepEqual(current.content, supplementContent(original));
  assert.equal(current.id, modelPin(release, state).revisionId); assert.equal(current.assetId, state.published[modelKey].targetId);
  const savedModel = state.published[modelKey];
  assert.equal(originalRevision.assetId, savedModel.targetId); assert.equal(originalRevision.id, savedModel.revisionId); assert.deepEqual(originalRevision.content, original); sha256(originalRevision.contentDigest);
  assert.equal(receipt.originalOperation.summary.id, state.operations[modelKey].operationId); assert.equal(receipt.originalOperation.summary.releaseId, savedModel.releaseId);
  assert.equal(receipt.originalOperation.targets.length, 1); const originalTarget = receipt.originalOperation.targets[0];
  assert.equal(originalTarget.targetId, savedModel.targetId); assert.equal(originalTarget.contentDigest, originalRevision.contentDigest);
  assert.deepEqual(originalTarget.declaration, state.operations[modelKey].payload.targets[0]); assert.deepEqual(originalTarget.declaration.content, original);
  validateManifests(receipt.beforeRelease, release, state, originalRevision.contentDigest);
  assert.deepEqual(receipt.beforeContent, original);
  const binding = sourceBindingProof(state, original, receipt.source.data, receipt.source.connection, receipt.source.snapshot, receipt.source.bindingOperation, receipt.beforeRelease, receipt.source.bindingRelease);
  assert.deepEqual(receipt.source.binding, binding);
  for (const proof of [receipt.source.before, receipt.source.after]) assert.deepEqual(proof, validateSourceCategories(proof, state, binding));
  assert.deepEqual(receipt.source.before, receipt.source.after);
  assert.deepEqual(receipt.steps.map(x => x.name), ['create', 'confirm', 'submit', 'review', 'publish']);
  for (const step of receipt.steps) { assert.equal(step.state, 'finished'); assert.equal(step.key, `v1-values-${state.owner}-${step.name}`); assert.equal(step.requestDigest, jsonDigest(step.body)); assert.equal(step.response.replayed, false); }
  const [create, confirm, submit, review, publish] = receipt.steps, operation = receipt.operation, target = definitionOnlyTarget(original, modelPin(receipt.beforeRelease, state));
  assert.equal(create.status, 201); assert.equal(confirm.status, 201); assert.equal(submit.status, 202); assert.ok([200, 201].includes(review.status)); assert.equal(publish.status, 201);
  assert.deepEqual(receipt.originalOperation.input, state.operations[modelKey].payload.input); assert.equal(receipt.originalOperation.summary.id, state.operations[modelKey].operationId);
  assert.deepEqual(create.body, { input: receipt.originalOperation.input, targets: [target] });
  const operationId = create.response.operationId, pin = { expectedVersion: create.response.version, setDigest: create.response.setDigest };
  sha256(pin.setDigest); assert.ok(Number.isInteger(pin.expectedVersion) && pin.expectedVersion > 0);
  assert.equal(operation.summary.id, operationId); assert.equal(operation.summary.createdBy, principals.author); assert.equal(operation.version, pin.expectedVersion); assert.equal(operation.setDigest, pin.setDigest);
  assert.deepEqual(operation.baselineHead, head(receipt.beforeRelease)); assert.deepEqual(operation.input, create.body.input);
  assert.equal(operation.summary.progress, 'released'); assert.equal(operation.summary.releaseId, release.id); assert.deepEqual(operation.unresolvedCodes, []);
  assert.equal(operation.targets.length, 1); const result = operation.targets[0];
  assert.deepEqual(result.declaration, target); assert.equal(result.targetId, target.targetId); assert.equal(result.outcome, 'proposal'); assert.equal(result.proposalState, 'released'); assert.equal(result.contentDigest, current.contentDigest);
  const proposalId = result.proposalId; assert.ok(proposalId); assert.deepEqual(create.response.proposalIds, [proposalId]);
  const validation = { attemptNo: operation.activeValidation.attemptNo, validationDigest: operation.activeValidation.validationDigest };
  assert.equal(operation.activeValidation.status, 'succeeded'); assert.ok(Number.isInteger(validation.attemptNo) && validation.attemptNo > 0); sha256(validation.validationDigest);
  assert.equal(confirm.actor, 'author'); assert.equal(create.actor, 'author'); assert.equal(submit.actor, 'author'); assert.equal(review.actor, 'reviewer'); assert.equal(publish.actor, 'publisher');
  assert.deepEqual(confirm.body, { ...pin, targetKey: 'model', action: 'confirm', declaration: valueGlossarySuffix.trim() });
  const event = confirm.response;
  assert.equal(event.operationId, operationId); assert.equal(event.productionVersion, pin.expectedVersion); assert.equal(event.setDigest, pin.setDigest); assert.equal(event.targetKey, 'model'); assert.equal(event.action, 'confirm'); assert.equal(event.principalId, principals.author); assert.equal(event.contentDigest, result.contentDigest);
  assert.equal(receipt.confirmations.items.length, 1); const witness = receipt.confirmations.items[0]; assert.equal(witness.valid, true); assert.deepEqual(witness.event, event); assert.equal(witness.declaration, confirm.body.declaration);
  assert.deepEqual(submit.body, pin); assert.deepEqual(review.body, { ...pin, validation, proposalIds: [proposalId], decision: 'approve', note: 'Independent approval of the exact synthetic category glossary and unchanged source bindings.' });
  assert.deepEqual(publish.body, { ...pin, validation, expectedHead: head(receipt.beforeRelease) });
  for (const step of [submit, review]) { assert.equal(step.response.operationId, operationId); assert.equal(step.response.version, pin.expectedVersion); assert.equal(step.response.setDigest, pin.setDigest); assert.deepEqual(step.response.proposalIds, [proposalId]); }
  assert.equal(review.response.reviewIds.length, 1); assert.deepEqual(result.reviewIds, review.response.reviewIds);
  assert.equal(receipt.reviews.items.length, 1); const reviewFact = receipt.reviews.items[0];
  assert.equal(reviewFact.id, review.response.reviewIds[0]); assert.equal(reviewFact.proposalId, proposalId); assert.equal(reviewFact.decision, 'approved'); assert.equal(reviewFact.reviewerPrincipalId, principals.reviewer);
  assert.equal(receipt.proposal.id, proposalId); assert.equal(receipt.proposal.createdBy, principals.author); assert.equal(receipt.proposal.state, 'released'); assert.equal(receipt.proposal.baseRevisionId, target.baseRevisionId); assert.equal(receipt.proposal.assetId, target.targetId);
  assert.equal(release.publishedBy, principals.publisher); assert.equal(release.attribution.role, 'applied'); assert.equal(release.attribution.operationId, operationId); assert.equal(release.attribution.version, pin.expectedVersion); assert.equal(release.attribution.setDigest, pin.setDigest); assert.deepEqual(release.attribution.validation, validation); assert.deepEqual(release.attribution.proposalIds, [proposalId]); assert.deepEqual(release.attribution.reviewIds, review.response.reviewIds);
  assert.ok(release.attribution.contributors.includes(principals.author)); assert.ok(!release.attribution.contributors.includes(principals.reviewer)); assert.deepEqual(release.originProposalIds, [proposalId]);
  assert.equal(publish.response.operationId, operationId); assert.equal(publish.response.releaseId, release.id); assert.equal(publish.response.manifestDigest, release.afterManifest.digest);
  for (const key of ['modelSteps', 'agentRuns', 'executions']) { assert.ok(Number.isInteger(receipt.beforeCounts[key])); assert.equal(receipt.afterCounts[key], receipt.beforeCounts[key]); }
  return { kind: 'approved-values', fixtureDigest: state.fixtureDigest, releaseId: release.id, revisionId: current.id, manifestDigest: release.afterManifest.digest, contentDigest: current.contentDigest, sourceProofDigest: jsonDigest(receipt.source) };
}

function knownAppliedFailure(failed, command, state) {
  assert.equal(failed.owner, state.owner); assert.equal(failed.state, 'failed'); assert.equal(failed.complete, false);
  for (const counters of [failed.beforeCounts, failed.afterCounts]) { assert.equal(counters.schema, 33); assert.equal(counters.unfinished, 0); }
  assert.equal(command.exit, 1); assert.equal(command.signal, null);
  assert.ok(Date.parse(command.startedAt) <= Date.parse(failed.startedAt)); assert.ok(Date.parse(command.finishedAt) >= Date.parse(failed.finishedAt));
  assert.deepEqual(failed.steps.map(x => x.name), ['create', 'confirm', 'submit', 'review', 'publish']);
  for (const [index, step] of failed.steps.entries()) {
    assert.equal(step.state, 'finished'); assert.equal(step.response.replayed, false);
    assert.ok([[201], [201], [202], [200, 201], [201]][index].includes(step.status));
  }
  const published = failed.steps[4].response;
  assert.equal(published.releaseId, failed.afterRelease.id); assert.equal(published.operationId, failed.operation.summary.id);
  assert.equal(published.manifestDigest, failed.afterRelease.afterManifest.digest); assert.equal(failed.operation.summary.progress, 'released');
  assert.equal(failed.operation.summary.releaseId, published.releaseId); assert.equal(failed.current.id, modelPin(failed.afterRelease, state).revisionId);
}

function preservationFacts(root) {
  const items = [], record = (path, directory = false) => {
    const stat = privatePath(path, directory);
    items.push({ path, directory, dev: stat.dev, ino: stat.ino, ...(directory ? {} : { digest: digest(readFileSync(path)), mtimeMs: stat.mtimeMs, ctimeMs: stat.ctimeMs }) });
  };
  record(root, true);
  for (const name of ['state.json', 'fixture.json', ledgerName, ...rootEvidenceNames]) record(join(root, name));
  for (const name of ['golden.json', 'model-acceptance.json', 'correction-amount-acceptance.json', 'final-candidate.json']) if (lstatSync(join(root, name), { throwIfNoEntry: false })) record(join(root, name));
  const directory = join(root, 'final-candidates');
  if (lstatSync(directory, { throwIfNoEntry: false })) { record(directory, true); for (const name of readdirSync(directory).sort()) record(join(directory, name)); }
  return items;
}

function verifyReconciliation(receipt, failed, command, view, root) {
  assert.equal(receipt.format, 1); assert.equal(receipt.kind, 'read-only-reconciliation'); assert.equal(receipt.owner, view.state.owner);
  assert.equal(receipt.state, 'verified'); assert.equal(receipt.complete, true);
  knownAppliedFailure(failed, command, view.state);
  assert.deepEqual(receipt.originalWrapper, { receipt: ledgerName, receiptDigest: digest(readFileSync(join(root, ledgerName))), command: commandName, commandDigest: digest(readFileSync(join(root, commandName))), exit: 1 });
  const preservedPaths = new Set(receipt.preserved.map(item => item.path)), currentPreservation = preservationFacts(root), cohorts = join(root, 'final-candidates');
  assert.equal(preservedPaths.size, receipt.preserved.length);
  assert.deepEqual(receipt.preserved, currentPreservation.filter(item => preservedPaths.has(item.path)));
  for (const item of currentPreservation) if (!preservedPaths.has(item.path)) assert.ok(item.path === cohorts || item.path.startsWith(cohorts + '/'), 'Only later candidate evidence may be added');
  verifySupplementFacts(failed, view);
  const facts = receipt.publication;
  for (const name of ['original', 'suffix', 'principals', 'steps', 'beforeContent', 'originalOperation', 'operation', 'confirmations', 'proposal', 'reviews', 'current', 'beforeCounts', 'afterCounts']) assert.deepEqual(facts[name], failed[name]);
  for (const name of ['originalRelease', 'beforeRelease', 'afterRelease']) assert.deepEqual(releaseFacts(facts[name]), releaseFacts(failed[name]));
  for (const name of ['data', 'connection', 'snapshot', 'bindingOperation', 'binding', 'before', 'after']) assert.deepEqual(facts.source[name], failed.source[name]);
  assert.deepEqual(releaseFacts(facts.source.bindingRelease), releaseFacts(failed.source.bindingRelease));
  assert.deepEqual(receipt.beforeCounts, failed.afterCounts); assert.deepEqual(receipt.afterCounts, receipt.beforeCounts);
  return verifySupplementFacts(facts, view);
}

export async function reconcileValues(root, { request = requestAs, readCategories = readSourceCategories, counts = counters, print = console.log } = {}) {
  const { state } = initialKnowledge(root), path = join(root, reconciliationName), pointerPath = join(root, pointerName);
  assert.ok(!lstatSync(path, { throwIfNoEntry: false }) && !lstatSync(pointerPath, { throwIfNoEntry: false }), 'Reconciliation is one-shot; no repeat or resume');
  const preserved = preservationFacts(root), failed = protectedJSON(join(root, ledgerName)), command = protectedJSON(join(root, commandName));
  knownAppliedFailure(failed, command, state);
  const originalWrapper = { receipt: ledgerName, receiptDigest: digest(readFileSync(join(root, ledgerName))), command: commandName, commandDigest: digest(readFileSync(join(root, commandName))), exit: 1 };
  const receipt = { format: 1, owner: state.owner, kind: 'read-only-reconciliation', state: 'claimed', complete: false, originalWrapper, preserved, startedAt: new Date().toISOString() };
  const guard = () => { assert.deepEqual(preservationFacts(root), preserved); };
  guard(); writePrivate(path, receipt, true); let owned = privatePath(path), ownedDigest = digest(readFileSync(path));
  const save = () => {
    guard(); const current = privatePath(path); assert.equal(current.dev, owned.dev); assert.equal(current.ino, owned.ino); assert.equal(digest(readFileSync(path)), ownedDigest);
    writePrivate(path, receipt); owned = privatePath(path); ownedDigest = digest(readFileSync(path));
  };
  const readonlyRequest = (...args) => { guard(); assert.equal(args[2], 'GET'); assert.ok(!/(\/ask|:execute|\/test|\/generation)(?:[/?]|$)/.test(args[3])); return request(...args); };
  const base = `/api/v1/workspaces/${state.workspace.id}`, read = url => get(root, readonlyRequest, url);
  try {
    receipt.beforeCounts = counts(root);
    assert.deepEqual(receipt.beforeCounts, failed.afterCounts);
    const expectedRelease = await read(base + '/production-releases/' + failed.steps[4].response.releaseId);
    const view = await readKnowledgeHead(root, readonlyRequest);
    assert.deepEqual(releaseFacts(view.release), releaseFacts(expectedRelease));
    verifySupplementFacts(failed, view);
    const operationPath = base + '/production-operations/' + failed.steps[0].response.operationId, proposalId = failed.proposal.id;
    const facts = { format: 1, owner: state.owner, fixtureDigest: state.fixtureDigest, original: view.original, suffix: failed.suffix, principals: failed.principals, steps: failed.steps,
      beforeCounts: failed.beforeCounts, afterCounts: failed.afterCounts, beforeContent: view.originalRevision.content,
      originalRelease: view.originalRelease, beforeRelease: await read(base + '/production-releases/' + failed.beforeRelease.id), afterRelease: view.release, current: view.current,
      originalOperation: await read(base + '/production-operations/' + state.operations[modelKey].operationId), operation: await read(operationPath),
      confirmations: await read(operationPath + '/business-rule-confirmations?version=' + failed.steps[0].response.version),
      proposal: await read(base + '/governance/proposals/' + proposalId), reviews: await read(base + '/governance/proposals/' + proposalId + '/reviews') };
    const data = state.published['demo_202609.customer_data'];
    facts.source = { data: await read(base + `/catalog/assets/${data.targetId}/revisions/${data.revisionId}`), connection: await read(base + '/sources/' + state.sourceId), snapshot: await read(base + `/sources/${state.sourceId}/snapshots/${state.discovery.snapshot.id}`),
      bindingOperation: await read(base + '/production-operations/' + state.operations.customers_binding.operationId), bindingRelease: await read(base + '/production-releases/' + state.published.customers_binding.releaseId) };
    facts.source.binding = sourceBindingProof(state, view.original, facts.source.data, facts.source.connection, facts.source.snapshot, facts.source.bindingOperation, facts.beforeRelease, facts.source.bindingRelease);
    guard(); facts.source.before = await readCategories(root, facts.source.binding); guard(); facts.source.after = await readCategories(root, facts.source.binding);
    receipt.publication = facts; receipt.afterCounts = counts(root);
    const after = await readKnowledgeHead(root, readonlyRequest); receipt.state = 'verified'; receipt.complete = true; receipt.finishedAt = new Date().toISOString();
    const basis = verifyReconciliation(receipt, failed, command, after, root); save(); guard();
    writePrivate(pointerPath, { format: 1, owner: state.owner, receipt: reconciliationName, receiptDigest: digest(readFileSync(path)), failedReceipt: ledgerName, failedReceiptDigest: originalWrapper.receiptDigest, rootCommand: commandName, rootCommandDigest: originalWrapper.commandDigest }, true);
    assert.deepEqual(approvedKnowledgeBasis(root, after), { ...basis, verification: 'read-only-reconciliation', receiptDigest: digest(readFileSync(path)), pointerDigest: digest(readFileSync(pointerPath)), failedReceiptDigest: originalWrapper.receiptDigest, rootCommandDigest: originalWrapper.commandDigest });
    print(JSON.stringify({ reconciliation: 'value-knowledge', complete: true, originalWrapperExit: 1, releaseId: after.release.id, revisionId: after.current.id, governanceMutations: 0, modelCalls: 0, executions: 0 }));
    return receipt;
  } catch { receipt.complete = false; receipt.state = 'failed'; save(); throw new Error('VALUE_KNOWLEDGE_RECONCILIATION_STOPPED'); }
}

export function valueKnowledgeEvidence(root) {
  const paths = [ledgerName, pointerName].map(name => join(root, name)), present = paths.map(path => Boolean(lstatSync(path, { throwIfNoEntry: false }))), reconciled = Boolean(lstatSync(join(root, reconciliationName), { throwIfNoEntry: false }));
  assert.equal(present[0], present[1], 'Incomplete knowledge supplement cannot be adopted');
  if (reconciled) { assert.ok(present[0]); paths.push(join(root, reconciliationName), ...rootEvidenceNames.map(name => join(root, name))); }
  return present[0] ? paths.map(path => { const stat = privatePath(path); return { path, dev: stat.dev, ino: stat.ino, digest: digest(readFileSync(path)) }; }) : [];
}

export function approvedKnowledgeBasis(root, view) {
  const evidence = valueKnowledgeEvidence(root);
  if (!evidence.length) {
    assert.deepEqual(view.current.content, view.original, 'Current model is not the original baseline');
    assert.deepEqual(view.release.afterManifest, view.originalRelease.afterManifest, 'Current manifest is not the original baseline');
    return { kind: 'original', fixtureDigest: view.state.fixtureDigest, contentDigest: view.current.contentDigest };
  }
  const pointer = protectedJSON(join(root, pointerName)), receipt = protectedJSON(join(root, ledgerName));
  if (pointer.receipt === reconciliationName) {
    const reconciled = protectedJSON(join(root, reconciliationName)), command = protectedJSON(join(root, commandName));
    const receiptDigest = digest(readFileSync(join(root, reconciliationName))), rootCommandDigest = digest(readFileSync(join(root, commandName)));
    assert.deepEqual(pointer, { format: 1, owner: view.state.owner, receipt: reconciliationName, receiptDigest, failedReceipt: ledgerName, failedReceiptDigest: evidence[0].digest, rootCommand: commandName, rootCommandDigest });
    const basis = verifyReconciliation(reconciled, receipt, command, view, root);
    assert.deepEqual(valueKnowledgeEvidence(root), evidence);
    return { ...basis, verification: 'read-only-reconciliation', receiptDigest, pointerDigest: evidence[1].digest, failedReceiptDigest: evidence[0].digest, rootCommandDigest };
  }
  assert.equal(evidence.length, 2);
  assert.deepEqual(pointer, { format: 1, owner: view.state.owner, receipt: ledgerName, receiptDigest: evidence[0].digest });
  const basis = verifySupplementReceipt(receipt, view);
  assert.deepEqual(valueKnowledgeEvidence(root), evidence);
  return { ...basis, receiptDigest: evidence[0].digest, pointerDigest: evidence[1].digest };
}

export async function prepareValues(root, { request = requestAs, readCategories = readSourceCategories, counts = counters, sleep = setTimeout, print = console.log } = {}) {
  const { state, original } = initialKnowledge(root), path = join(root, ledgerName), pointerPath = join(root, pointerName);
  assert.ok(!lstatSync(path, { throwIfNoEntry: false }) && !lstatSync(pointerPath, { throwIfNoEntry: false }), 'Knowledge preparation is one-shot; no repeat or resume');
  assert.ok(!lstatSync(join(root, reconciliationName), { throwIfNoEntry: false }), 'Existing reconciliation cannot be bypassed');
  const watched = [], watch = (path, directory = false) => { const stat = privatePath(path, directory); watched.push({ path, directory, dev: stat.dev, ino: stat.ino, digest: directory ? null : digest(readFileSync(path)) }); };
  watch(root, true); for (const name of ['state.json','fixture.json','golden.json','model-acceptance.json','correction-amount-acceptance.json','final-candidate.json']) if (lstatSync(join(root,name), { throwIfNoEntry: false })) watch(join(root,name));
  const cohorts = join(root, 'final-candidates'), cohortNames = lstatSync(cohorts, { throwIfNoEntry: false }) ? readdirSync(cohorts).sort() : null;
  if (cohortNames) { watch(cohorts,true); for (const name of cohortNames) watch(join(cohorts,name)); }
  const guard = () => {
    for (const old of watched) { const current = privatePath(old.path, old.directory); assert.equal(current.dev,old.dev); assert.equal(current.ino,old.ino); if (!old.directory) assert.equal(digest(readFileSync(old.path)),old.digest); }
    assert.deepEqual(lstatSync(cohorts, { throwIfNoEntry: false }) ? readdirSync(cohorts).sort() : null, cohortNames);
  };
  const receipt = { format: 1, owner: state.owner, fixtureDigest: state.fixtureDigest, state: 'claimed', complete: false, startedAt: new Date().toISOString(), original, suffix: valueGlossarySuffix, principals: { author: state.principals.author, reviewer: state.principals.reviewer, publisher: state.principals.publisher }, steps: [] };
  guard(); writePrivate(path, receipt, true); watch(path);
  const save = () => { guard(); writePrivate(path,receipt); const stat=privatePath(path); Object.assign(watched.at(-1),{dev:stat.dev,ino:stat.ino,digest:digest(readFileSync(path))}); };
  const base = `/api/v1/workspaces/${state.workspace.id}`;
  const read = (url, actor) => { guard(); return get(root,request,url,actor); };
  const post = async (name, actor, url, body, statuses) => {
    guard(); const step={name,actor,key:`v1-values-${state.owner}-${name}`,state:'started',body,requestDigest:jsonDigest(body)}; receipt.steps.push(step); save();
    const response=await request(root,actor,'POST',url,body,step.key); step.status=response.status; step.response=response.body;
    step.state=response.status===0?'unknown':'finished'; save(); assert.ok(statuses.includes(response.status)); assert.equal(response.body.replayed,false); return response.body;
  };
  try {
    assert.ok(Object.values(receipt.principals).every(x=>typeof x==='string'&&x.length>0)); assert.equal(new Set(Object.values(receipt.principals)).size,3); receipt.beforeCounts=counts(root);
    const view=await readKnowledgeHead(root,request); assert.deepEqual(view.current.content,original);
    assert.deepEqual(view.release.afterManifest,view.originalRelease.afterManifest,'Supplement requires the complete original published manifest');
    receipt.originalRelease=view.originalRelease; receipt.beforeRelease=view.release; receipt.beforeContent=view.current.content;
    receipt.originalOperation=await read(base+'/production-operations/'+state.operations[modelKey].operationId);
    assert.deepEqual(receipt.originalOperation.input,state.operations[modelKey].payload.input);
    assert.deepEqual(receipt.originalOperation.targets[0].declaration.content,original);
    const d=state.published['demo_202609.customer_data'];
    const source={data:await read(base+`/catalog/assets/${d.targetId}/revisions/${d.revisionId}`),connection:await read(base+'/sources/'+state.sourceId),snapshot:await read(base+`/sources/${state.sourceId}/snapshots/${state.discovery.snapshot.id}`),bindingOperation:await read(base+'/production-operations/'+state.operations.customers_binding.operationId),bindingRelease:await read(base+'/production-releases/'+state.published.customers_binding.releaseId)};
    source.binding=sourceBindingProof(state,original,source.data,source.connection,source.snapshot,source.bindingOperation,view.release,source.bindingRelease);
    source.before=await readCategories(root,source.binding); assert.deepEqual(source.before,validateSourceCategories(source.before,state,source.binding)); receipt.source=source; save();
    const target=definitionOnlyTarget(original,modelPin(view.release,state)), input=receipt.originalOperation.input;
    const created=await post('create','author',base+'/production-operations',{input,targets:[target]},[201]);
    const operationPath=base+'/production-operations/'+created.operationId, pin={expectedVersion:created.version,setDigest:created.setDigest};
    let operation=await read(operationPath,'author'); assert.deepEqual(operation.baselineHead,head(view.release)); assert.deepEqual(operation.targets.map(x=>x.declaration),[target]);
    await post('confirm','author',operationPath+'/business-rule-confirmations',{...pin,targetKey:'model',action:'confirm',declaration:valueGlossarySuffix.trim()},[201]);
    await post('submit','author',operationPath+'/submit',pin,[202]);
    for(let i=0;i<240;i++){operation=await read(operationPath,'author'); if(!['queued','running'].includes(operation.activeValidation.status))break; await sleep(500);}
    assert.equal(operation.activeValidation.status,'succeeded'); assert.deepEqual(operation.unresolvedCodes,[]); assert.equal(operation.targets.length,1);
    const validation={attemptNo:operation.activeValidation.attemptNo,validationDigest:operation.activeValidation.validationDigest}, proposalId=operation.targets[0].proposalId;
    receipt.validation=operation.activeValidation; save();
    await post('review','reviewer',operationPath+'/reviews',{...pin,validation,proposalIds:[proposalId],decision:'approve',note:'Independent approval of the exact synthetic category glossary and unchanged source bindings.'},[200,201]);
    await post('publish','publisher',operationPath+'/publish',{...pin,validation,expectedHead:head(view.release)},[201]);
    receipt.operation=await read(operationPath); receipt.confirmations=await read(operationPath+'/business-rule-confirmations?version='+pin.expectedVersion);
    receipt.proposal=await read(base+'/governance/proposals/'+proposalId); receipt.reviews=await read(base+'/governance/proposals/'+proposalId+'/reviews');
    const after=await readKnowledgeHead(root,request); receipt.afterRelease=after.release; receipt.current=after.current;
    receipt.source.after=await readCategories(root,source.binding); receipt.afterCounts=counts(root); receipt.state='complete'; receipt.complete=true; receipt.finishedAt=new Date().toISOString();
    const basis=verifySupplementReceipt(receipt,after); save(); guard();
    writePrivate(pointerPath,{format:1,owner:state.owner,receipt:ledgerName,receiptDigest:digest(readFileSync(path))},true);
    assert.deepEqual(approvedKnowledgeBasis(root,after),{...basis,receiptDigest:digest(readFileSync(path)),pointerDigest:digest(readFileSync(pointerPath))});
    print(JSON.stringify({preparation:'value-knowledge',complete:true,releaseId:after.release.id,revisionId:after.current.id,modelCalls:0,executions:0})); return receipt;
  } catch { receipt.complete=false; receipt.state=receipt.steps.some(x=>x.state==='started'||x.state==='unknown')?'unknown':'failed'; save(); throw new Error('VALUE_KNOWLEDGE_PREPARATION_STOPPED'); }
}

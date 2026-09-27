import assert from 'node:assert/strict';
import { readFileSync, lstatSync, mkdirSync, readdirSync } from 'node:fs';
import { randomBytes } from 'node:crypto';
import { join } from 'node:path';
import { readState, readPrivate, writePrivate, goldenCases, digest, rejectLinks } from './core.mjs';
import { status } from './runtime.mjs';
import { governanceHeadProof } from './correction.mjs';
import { valueKnowledgeEvidence } from './value-knowledge.mjs';
import { requestAs, counters, independentRows, reconcileExecution, assertNoExecution, assertPlanPins, assertQueryWindow } from './model.mjs';

export function finalCandidateCases() {
  return [...goldenCases().map(item => ({ ...item, actor: 'consumer', question: '使用已发布分析模型 demo_202609.model，' + item.question })),
    { id: 'denied', actor: 'denied', question: '使用已发布分析模型 demo_202609.model，2026年8月的有效支付金额是多少？' },
    { id: 'ambiguous', actor: 'consumer', question: '最近那个指标怎么样？' }];
}

export function finalCandidateSummary(attempts) {
  const cases = finalCandidateCases();
  assert.deepEqual(attempts.map(item => item.case), cases.map(item => item.id));
  const count = positive => ({ passed: attempts.filter((item, index) => Boolean(cases[index].expected) === positive && item.state === 'finished' && item.passed).length, total: positive ? 4 : 2 });
  return { positive: count(true), negative: count(false), complete: attempts.every(item => item.state === 'finished'), passed: attempts.every(item => item.state === 'finished' && item.passed) };
}

function privatePath(path, directory = false) {
  rejectLinks(path);
  const info = lstatSync(path);
  assert.ok(!info.isSymbolicLink() && (directory ? info.isDirectory() : info.isFile()), 'Unsafe cohort path');
  assert.equal(info.uid, process.getuid(), 'Foreign cohort path');
  assert.equal(info.mode & 0o077, 0, 'Cohort path must be private');
  return info;
}

export function candidateCohortIdentity(root, runtime) {
  privatePath(root, true);
  const state = readState(root);
  for (const name of ['state.json', 'runtime-owner.json', 'runtime-launch.json', 'server']) privatePath(join(root, name));
  const owner = readPrivate(join(root, 'runtime-owner.json')), launch = readPrivate(join(root, 'runtime-launch.json'));
  assert.equal(runtime?.ready, true); assert.equal(runtime.mode, 'candidate');
  assert.equal(runtime.api, `http://127.0.0.1:${state.apiPort}/`); assert.equal(runtime.url, runtime.api); assert.equal(runtime.sourceWebAvailable, false);
  assert.equal(owner.owner, state.owner); assert.equal(launch.owner, state.owner);
  assert.equal(owner.mode, 'candidate'); assert.equal(launch.mode, 'candidate');
  assert.match(owner.token ?? '', /^[a-f0-9]{64}$/);
  assert.match(launch.version ?? '', /^[0-9]+\.[0-9]+\.[0-9]+-rc\.[A-Za-z0-9._-]+$/);
  assert.ok(Number.isFinite(Date.parse(launch.verifiedAt)), 'Candidate verification receipt required');
  assert.equal(typeof launch.sourceDirty, 'boolean'); assert.equal(launch.schema, 33); assert.equal(owner.schema, 33); assert.equal(runtime.activeBinary?.schema, 33);
  for (const key of ['sourceDigest', 'binaryDigest', 'manifestDigest', 'archiveDigest']) assert.match(launch[key] ?? '', /^sha256:[a-f0-9]{64}$/);
  for (const key of ['binaryDigest', 'manifestDigest']) { assert.equal(owner[key], launch[key]); assert.equal(runtime.activeBinary[key], launch[key]); }
  assert.equal(digest(readFileSync(join(root, 'server'))), launch.binaryDigest, 'Running executable changed');
  return Object.fromEntries(['version', 'sourceDigest', 'sourceDirty', 'binaryDigest', 'manifestDigest', 'archiveDigest', 'schema'].map(key => [key, launch[key]]));
}

export async function currentModelConfiguration(root, request = requestAs) {
  const state = readState(root);
  const response = await request(root, 'admin', 'GET', `/api/v1/workspaces/${state.workspace.id}/governance/model-providers`);
  assert.equal(response.status, 200);
  const defaults = response.body.items.flatMap(detail => detail.models.filter(model => detail.provider.enabled && model.enabled && model.kind === 'llm' && model.isDefault).map(model => ({ provider: detail.provider, model })));
  assert.equal(defaults.length, 1, 'One enabled default LLM required');
  const { provider, model } = defaults[0];
  assert.equal(provider.id, state.model.providerId); assert.equal(model.id, state.model.settingId); assert.equal(model.providerId, provider.id);
  assert.match(model.generationConfigRevision ?? '', /^sha256:[a-f0-9]{64}$/);
  return { providerId: provider.id, settingId: model.id, configurationDigest: model.generationConfigRevision };
}

function validateModelConfiguration(value, state) {
  assert.equal(value?.providerId, state.model.providerId); assert.equal(value?.settingId, state.model.settingId);
  assert.match(value?.configurationDigest ?? '', /^sha256:[a-f0-9]{64}$/);
  return value;
}

function validateKnowledgeBasis(value, state) {
  assert.ok(['original', 'approved-values'].includes(value?.kind)); assert.equal(value.fixtureDigest, state.fixtureDigest);
  assert.match(value.contentDigest ?? '', /^sha256:[a-f0-9]{64}$/);
  if (value.kind === 'approved-values') for (const key of ['receiptDigest', 'pointerDigest', 'manifestDigest', 'sourceProofDigest']) assert.match(value[key] ?? '', /^sha256:[a-f0-9]{64}$/);
  if (value.verification !== undefined) {
    assert.equal(value.kind, 'approved-values'); assert.equal(value.verification, 'read-only-reconciliation');
    for (const key of ['failedReceiptDigest', 'rootCommandDigest']) assert.match(value[key] ?? '', /^sha256:[a-f0-9]{64}$/);
  }
  return structuredClone(value);
}

function openCohort(root, state, candidate, cases) {
  const watched = [];
  const watch = (path, directory = false) => { const info = privatePath(path, directory); watched.push({ path, directory, dev: info.dev, ino: info.ino, hash: directory ? null : digest(readFileSync(path)) }); };
  watch(root, true); watch(join(root, 'state.json'));
  const knowledgeEvidence = valueKnowledgeEvidence(root);
  for (const item of knowledgeEvidence) watch(item.path);
  const legacy = join(root, 'final-candidate.json');
  if (lstatSync(legacy, { throwIfNoEntry: false })) {
    watch(legacy); const previous = readPrivate(legacy);
    assert.equal(previous.owner, state.owner); assert.match(previous.runtimeBinaryDigest ?? '', /^sha256:[a-f0-9]{64}$/);
    assert.notEqual(previous.runtimeBinaryDigest, candidate.binaryDigest, 'Candidate already has a legacy cohort');
    assert.equal(previous.summary?.complete, true, 'Unfinished legacy cohort must not be bypassed');
    assert.equal(finalCandidateSummary(previous.attempts).complete, true, 'Unfinished legacy attempts must not be bypassed');
  }
  const directory = join(root, 'final-candidates'), marker = join(directory, '.owner.json');
  if (!lstatSync(directory, { throwIfNoEntry: false })) {
    mkdirSync(directory, { mode: 0o700 }); writePrivate(marker, { format: 1, owner: state.owner }, true);
  }
  watch(directory, true); watch(marker);
  assert.deepEqual(readPrivate(marker), { format: 1, owner: state.owner });
  const key = digest(JSON.stringify([candidate.manifestDigest, candidate.binaryDigest])).slice(7), path = join(directory, key + '.json');
  const entries = readdirSync(directory).sort();
  for (const name of entries) {
    if (name === '.owner.json') continue;
    assert.match(name, /^[a-f0-9]{64}\.json$/, 'Unknown cohort entry');
    const priorPath = join(directory, name); watch(priorPath);
    const previous = readPrivate(priorPath);
    assert.equal(previous.format, 1); assert.equal(previous.owner, state.owner); assert.equal(previous.cohortKey + '.json', name);
    for (const field of ['binaryDigest', 'manifestDigest']) {
      assert.match(previous.candidateBefore?.[field] ?? '', /^sha256:[a-f0-9]{64}$/);
      assert.notEqual(previous.candidateBefore[field], candidate[field], 'Candidate cohort already exists; no repeat or resume');
    }
    assert.equal(previous.summary?.complete, true, 'Unfinished cohort must not be bypassed');
    assert.equal(finalCandidateSummary(previous.attempts).complete, true, 'Unfinished attempts must not be bypassed');
  }
  const guard = () => {
    for (const item of watched) {
      const current = privatePath(item.path, item.directory);
      assert.equal(current.dev, item.dev); assert.equal(current.ino, item.ino);
      if (!item.directory) assert.equal(digest(readFileSync(item.path)), item.hash, 'Cohort evidence changed');
    }
    assert.deepEqual(readdirSync(directory).sort(), entries, 'Cohort inventory changed');
    assert.deepEqual(valueKnowledgeEvidence(root), knowledgeEvidence, 'Supplement evidence changed');
  };
  const cohortId = randomBytes(12).toString('hex');
  const ledger = { format: 1, owner: state.owner, cohortKey: key, cohortId, candidateBefore: candidate, startedAt: new Date().toISOString(), maxAttemptsPerCase: 1, fixedCasesDigest: digest(JSON.stringify(cases)), fixtureDigest: state.fixtureDigest,
    attempts: cases.map(item => ({ case: item.id, actor: item.actor, key: `v1-final-${cohortId}-${item.id}`, state: 'pending', passed: false })) };
  guard(); writePrivate(path, ledger, true); watch(path); entries.push(key + '.json'); entries.sort();
  const save = () => { guard(); writePrivate(path, ledger); const updated = watched.at(-1), info = privatePath(path); updated.dev = info.dev; updated.ino = info.ino; updated.hash = digest(readFileSync(path)); };
  return { ledger, save, guard, path };
}

// This cohort is deliberately separate from repair attempts and has no resume or retry path.
export async function finalCandidate(root, { runtimeStatus = status, modelConfiguration = currentModelConfiguration, headProof = governanceHeadProof, counts = counters, request = requestAs, rows = independentRows, print = console.log } = {}) {
  const state = readState(root), candidateBefore = candidateCohortIdentity(root, await runtimeStatus());
  assert.match(state.fixtureDigest ?? '', /^sha256:[a-f0-9]{64}$/);
  const cases = finalCandidateCases(), record = openCohort(root, state, candidateBefore, cases), { ledger, save, guard } = record;
  const proof = await headProof(root);
  assert.equal(proof.correctBaselineContent, true, 'Final candidate requires the restored correct model content');
  assert.ok(state.fixtureDigest, 'Final candidate requires the full knowledge/data fixture fingerprint');
  const cohortId = ledger.cohortId;
  const model = { targetId: state.published['demo_202609.model'].targetId, revisionId: proof.currentRevision };
  const pinnedState = { published: { 'demo_202609.model': model } };
  const base = `/api/v1/workspaces/${state.workspace.id}`;
  Object.assign(ledger, { head: proof.currentRelease, model, knowledgeBasisBefore: validateKnowledgeBasis(proof.knowledgeBasis, state), runtimeBinaryDigest: candidateBefore.binaryDigest, modelConfigurationBefore: validateModelConfiguration(await modelConfiguration(root), state), before: counts(root) });
  save();
  for (let index = 0; index < cases.length; index++) {
    guard(); assert.deepEqual(candidateCohortIdentity(root, await runtimeStatus()), candidateBefore, 'Candidate changed before model attempt');
    const item = cases[index], attempt = ledger.attempts[index];
    attempt.state = 'started'; attempt.before = counts(root); save();
    try {
      const asked = await request(root, item.actor, 'POST', base + '/ask', { question: item.question, context: { mode: 'explicit', releaseId: ledger.head }, idempotencyKey: attempt.key });
      attempt.askStatus = asked.status;
      if (/^[A-Z][A-Z0-9_]{2,63}$/.test(asked.body?.code)) attempt.code = asked.body.code;
      guard(); writePrivate(join(root, `final-${cohortId}-${item.id}-ask.json`), asked, true);
      attempt.afterAsk = counts(root);
      if (item.expected) {
        assert.equal(asked.status, 200);
        assert.equal(asked.body.interpretation?.outcome, 'query');
        const query = asked.body.interpretation.query, plan = asked.body.resolution.plan;
        assertPlanPins(plan, pinnedState, ledger.head);
        attempt.semanticWindow = { from: query.timeRange?.from, to: query.timeRange?.to, granularity: query.timeRange?.granularity, passed: false };
        assertQueryWindow(query, item.id); attempt.semanticWindow.passed = true;
        const executed = await request(root, item.actor, 'POST', base + `/resolved-semantic-plans/${plan.id}:execute`, { planDigest: plan.planDigest, idempotencyKey: attempt.key + '-execute' });
        attempt.executeStatus = executed.status;
        if (/^[A-Z][A-Z0-9_]{2,63}$/.test(executed.body?.code)) attempt.executionCode = executed.body.code;
        guard(); writePrivate(join(root, `final-${cohortId}-${item.id}-execute.json`), executed, true);
        assert.equal(executed.status, 200);
        assert.equal(executed.body.run.planId, plan.id); assert.equal(executed.body.run.planDigest, plan.planDigest);
        const expected = rows(root, item.sql);
        reconcileExecution({ ...executed.body, columns: undefined, rows: expected }, item.expected, item.id);
        reconcileExecution(executed.body, expected, item.id);
        attempt.dataReconciled = true;
      } else assertNoExecution(asked, attempt.before, attempt.afterAsk, item.id);
      attempt.passed = true;
    } catch {
      attempt.passed = false; attempt.reason = 'FINAL_CANDIDATE_CONTRACT_FAILED';
    }
    attempt.after = counts(root); attempt.state = 'finished'; save();
    print(JSON.stringify({ cohort: 'final-candidate', case: item.id, askStatus: attempt.askStatus, code: attempt.code, executeStatus: attempt.executeStatus, executionCode: attempt.executionCode, passed: attempt.passed }));
    if (attempt.askStatus === 0 || attempt.executeStatus === 0) { ledger.stoppedReason = 'UNKNOWN_HTTP_OUTCOME'; break; }
  }
  ledger.after = counts(root); ledger.summary = finalCandidateSummary(ledger.attempts); ledger.finishedAt = new Date().toISOString();
  ledger.providerAttempts = ledger.after.modelSteps - ledger.before.modelSteps;
  try {
    guard(); ledger.candidateAfter = candidateCohortIdentity(root, await runtimeStatus());
    ledger.modelConfigurationAfter = validateModelConfiguration(await modelConfiguration(root), state);
    const afterHead = await headProof(root);
    ledger.knowledgeBasisAfter = validateKnowledgeBasis(afterHead.knowledgeBasis, state);
    ledger.headAfter = { releaseId: afterHead.currentRelease, revisionId: afterHead.currentRevision, correctBaselineContent: afterHead.correctBaselineContent };
    assert.deepEqual(ledger.candidateAfter, ledger.candidateBefore);
    assert.deepEqual(ledger.modelConfigurationAfter, ledger.modelConfigurationBefore);
    assert.deepEqual(ledger.knowledgeBasisAfter, ledger.knowledgeBasisBefore);
    assert.deepEqual(ledger.headAfter, { releaseId: ledger.head, revisionId: ledger.model.revisionId, correctBaselineContent: true });
    assert.equal(readState(root).fixtureDigest, ledger.fixtureDigest); assert.equal(digest(JSON.stringify(finalCandidateCases())), ledger.fixedCasesDigest);
    ledger.identityStable = true;
  } catch { ledger.identityStable = false; ledger.summary.passed = false; ledger.stoppedReason = 'CANDIDATE_IDENTITY_UNVERIFIED'; }
  save();
  print(JSON.stringify({ cohort: 'final-candidate', candidate: candidateBefore.version, ledgerPath: record.path, head: ledger.head, ...ledger.summary, identityStable: ledger.identityStable, providerAttempts: ledger.providerAttempts }));
  return ledger;
}

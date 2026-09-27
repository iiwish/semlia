import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, writeFileSync, readFileSync, readdirSync, rmSync, realpathSync, symlinkSync, mkdirSync, chmodSync, renameSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { openState, checkpoint, writePrivate, digest } from './v1/core.mjs';
import { currentModelConfiguration, finalCandidate, finalCandidateCases, finalCandidateSummary } from './v1/final-candidate.mjs';

function fixture() {
  const parent = mkdtempSync(join(realpathSync(tmpdir()), 'v1-cohort-')), root = join(parent, 'run');
  const state = checkpoint(root, openState(root, 'sha256:' + 'a'.repeat(64)), { apiPort: 18080, webPort: 18081, fixtureDigest: 'sha256:' + 'b'.repeat(64), workspace: { id: 'wsp_synthetic' }, published: { 'demo_202609.model': { targetId: 'ast_synthetic' } }, model: { providerId: 'provider', settingId: 'setting' } });
  const binary = Buffer.from('synthetic verified executable'), binaryDigest = digest(binary);
  writeFileSync(join(root, 'server'), binary, { mode: 0o700 });
  const candidate = { owner: state.owner, mode: 'candidate', version: '1.0.0-rc.synthetic', sourceDigest: 'sha256:'+'c'.repeat(64), sourceDirty: true, binaryDigest, manifestDigest: 'sha256:'+'d'.repeat(64), archiveDigest: 'sha256:'+'e'.repeat(64), schema: 33, verifiedAt: '2026-09-27T00:00:00.000Z' };
  writePrivate(join(root, 'runtime-launch.json'), candidate, true);
  writePrivate(join(root, 'runtime-owner.json'), { ...candidate, token: 'f'.repeat(64), pid: process.pid }, true);
  const runtime = { mode: 'candidate', ready: true, api: 'http://127.0.0.1:18080/', url: 'http://127.0.0.1:18080/', sourceWebAvailable: false, activeBinary: { schema: 33, binaryDigest, manifestDigest: candidate.manifestDigest } };
  let requests = 0;
  const options = { runtimeStatus: async () => runtime, headProof: async () => ({ currentRelease: 'rls_synthetic', currentRevision: 'rev_synthetic', correctBaselineContent: true, knowledgeBasis: { kind: 'original', fixtureDigest: state.fixtureDigest, contentDigest: 'sha256:'+'9'.repeat(64) } }), modelConfiguration: async () => ({ providerId: 'provider', settingId: 'setting', configurationDigest: 'sha256:'+'f'.repeat(64) }), counts: () => ({ modelSteps: 0, executions: 0, agentRuns: 0 }), request: async () => { requests++; return { status: 403, body: { code: 'FORBIDDEN' } }; }, print: () => {} };
  return { parent, root, state, candidate, runtime, options, requests: () => requests, cleanup: () => rmSync(parent, { recursive: true, force: true }) };
}

function installCandidate(f, binary = 'a distinct verified executable', manifest = '1') {
  writeFileSync(join(f.root, 'server'), binary);
  Object.assign(f.candidate, { binaryDigest: digest(Buffer.from(binary)), manifestDigest: 'sha256:' + manifest.repeat(64) });
  writePrivate(join(f.root, 'runtime-launch.json'), f.candidate);
  writePrivate(join(f.root, 'runtime-owner.json'), { ...f.candidate, token: 'f'.repeat(64), pid: process.pid });
  Object.assign(f.runtime.activeBinary, { binaryDigest: f.candidate.binaryDigest, manifestDigest: f.candidate.manifestDigest });
}

function ledgerPaths(f) {
  const directory = join(f.root, 'final-candidates');
  return readdirSync(directory).filter(name => name !== '.owner.json').map(name => join(directory, name));
}

function successfulResponses(f) {
  const counts = { modelSteps: 0, executions: 0, agentRuns: 0 }, request = f.options.request, cases = finalCandidateCases();
  const columns = { total: ['demo_202609.amount'], region: ['demo_202609.customers.region', 'demo_202609.amount'], monthly: ['demo_202609.orders.paid_at.period', 'demo_202609.amount'], 'old-customer-average': ['demo_202609.average'] };
  f.options.counts = () => ({ ...counts });
  f.options.rows = (_root, sql) => cases.find(item => item.sql === sql).expected;
  f.options.request = async (root, actor, method, path, body) => {
    await request(root, actor, method, path, body);
    if (path.endsWith('/ask')) {
      if (actor === 'denied') return { status: 403, body: { code: 'NO_MATCHING_GRANT' } };
      counts.modelSteps++; counts.agentRuns++;
      const item = cases.find(item => item.question === body.question);
      if (!item.expected) return { status: 200, body: { interpretation: { outcome: 'clarification' } } };
      const plan = { id: 'plan_' + item.id, planDigest: 'sha256:' + '7'.repeat(64), releaseId: 'rls_synthetic', model: { assetId: 'ast_synthetic', revisionId: 'rev_synthetic' } };
      return { status: 200, body: { interpretation: { outcome: 'query', query: { timeRange: { from: item.id === 'monthly' ? '2026-07-01T00:00:00Z' : '2026-08-01T00:00:00Z', to: '2026-09-01T00:00:00Z', ...(item.id === 'monthly' ? { granularity: 'month' } : {}) } } }, resolution: { plan } } };
    }
    const item = cases.find(item => path.endsWith(`/plan_${item.id}:execute`));
    assert.ok(item); counts.executions++;
    return { status: 200, body: { run: { state: 'succeeded', planId: 'plan_' + item.id, planDigest: body.planDigest }, availability: 'ephemeral', sql: item.sql, rows: item.expected, columns: columns[item.id] } };
  };
}

test('filesystem cohorts refuse source, missing or changed candidate identity before every model call', async () => {
  for (const change of [f => { f.runtime.mode = 'source'; }, f => { delete f.runtime.activeBinary.manifestDigest; }, f => { writeFileSync(join(f.root, 'server'), 'changed'); }, f => { f.runtime.activeBinary.binaryDigest = 'sha256:'+'0'.repeat(64); }]) {
    const f = fixture();
    try { change(f); await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 0); }
    finally { f.cleanup(); }
  }
});

test('cohort guards retain reconciled knowledge receipts and the original failed root command across every attempt', async () => {
  for (const changedName of ['value-knowledge.json', 'value-knowledge-reconciliation.json', 'value-knowledge-root-command.json']) {
    const f=fixture();
    try {
      for (const name of ['value-knowledge.json','value-knowledge-approved.json','value-knowledge-reconciliation.json','value-knowledge-root-command.json','value-knowledge-root-before.json','value-knowledge-root-proof-failure.json','value-knowledge-root-after-failed.json']) writePrivate(join(f.root,name),{guardFixture:true},true);
      const proof=await f.options.headProof();
      f.options.headProof=async()=>({...proof,knowledgeBasis:{...proof.knowledgeBasis,kind:'approved-values',verification:'read-only-reconciliation',receiptDigest:digest('reconciliation'),pointerDigest:digest('pointer'),manifestDigest:digest('manifest'),sourceProofDigest:digest('source'),failedReceiptDigest:digest('failed'),rootCommandDigest:digest('command')}});
      const request=f.options.request;
      f.options.request=async(...args)=>{const response=await request(...args);writePrivate(join(f.root,changedName),{drift:true});return response;};
      await assert.rejects(()=>finalCandidate(f.root,f.options));assert.equal(f.requests(),1);
    } finally { f.cleanup(); }
  }
});

test('a distinct candidate gets exactly one fresh fixed cohort without altering the legacy ledger', async () => {
  const f = fixture();
  try {
    const legacy = join(f.root, 'final-candidate.json');
    writePrivate(legacy, { owner: f.state.owner, runtimeBinaryDigest: 'sha256:'+'0'.repeat(64), summary: { complete: true, passed: false }, attempts: finalCandidateCases().map(item => ({ case: item.id, state: 'finished', passed: false })) }, true);
    const before = readFileSync(legacy);
    const result = await finalCandidate(f.root, f.options);
    assert.equal(f.requests(), 6); assert.equal(result.maxAttemptsPerCase, 1);
    assert.deepEqual(result.attempts.map(x => x.case), finalCandidateCases().map(x => x.id));
    assert.deepEqual(readFileSync(legacy), before);
    assert.equal(result.candidateBefore.version, f.candidate.version);
    assert.equal(result.candidateAfter.binaryDigest, f.candidate.binaryDigest);
    assert.equal(result.fixedCasesDigest, digest(JSON.stringify(finalCandidateCases())));
    assert.deepEqual(result.modelConfigurationBefore, result.modelConfigurationAfter);
    await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 6);
  } finally { f.cleanup(); }
});

test('legacy same-binary and ambiguous legacy identity block new provider attempts', async () => {
  for (const previous of [f => ({ owner: f.state.owner, runtimeBinaryDigest: f.candidate.binaryDigest }), f => ({ owner: f.state.owner, attempts: [] }), () => ({ owner: 'foreign', runtimeBinaryDigest: 'sha256:'+'0'.repeat(64) })]) {
    const f = fixture();
    try { writePrivate(join(f.root, 'final-candidate.json'), previous(f), true); await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 0); }
    finally { f.cleanup(); }
  }
});

test('a partial durable claim cannot be resumed or bypassed with another candidate', async () => {
  const f = fixture();
  try {
    await assert.rejects(() => finalCandidate(f.root, { ...f.options, headProof: async () => { throw new Error('synthetic interruption'); } }));
    const path = ledgerPaths(f)[0], original = readFileSync(path);
    assert.equal(JSON.parse(original).attempts.every(item => item.state === 'pending'), true);
    await assert.rejects(() => finalCandidate(f.root, f.options));
    installCandidate(f);
    await assert.rejects(() => finalCandidate(f.root, f.options));
    assert.equal(f.requests(), 0); assert.deepEqual(readFileSync(path), original);
  } finally { f.cleanup(); }
});

test('a changed name or manifest cannot repeat an already claimed binary, nor reuse its manifest', async () => {
  for (const change of [f => installCandidate(f, 'synthetic verified executable', '1'), f => installCandidate(f, 'new executable', 'd')]) {
    const f = fixture();
    try {
      await finalCandidate(f.root, f.options);
      const path = ledgerPaths(f)[0], original = readFileSync(path);
      change(f); await assert.rejects(() => finalCandidate(f.root, f.options));
      assert.equal(f.requests(), 6); assert.deepEqual(readFileSync(path), original);
    } finally { f.cleanup(); }
  }
});

test('a truly distinct verified candidate retains prior bytes and still attempts all six cases once', async () => {
  const f = fixture();
  try {
    const first = await finalCandidate(f.root, f.options), path = ledgerPaths(f)[0], original = readFileSync(path);
    installCandidate(f);
    const second = await finalCandidate(f.root, f.options);
    assert.equal(f.requests(), 12); assert.equal(ledgerPaths(f).length, 2);
    assert.notEqual(first.cohortKey, second.cohortKey); assert.notEqual(first.cohortId, second.cohortId);
    assert.deepEqual(second.attempts.map(item => item.case), first.attempts.map(item => item.case));
    assert.equal(second.attempts.every(item => item.state === 'finished'), true);
    assert.deepEqual(readFileSync(path), original);
  } finally { f.cleanup(); }
});

test('ownership, permissions and symbolic cohort paths fail closed before any model request', async () => {
  const mutations = [
    f => chmodSync(f.root, 0o755),
    f => chmodSync(join(f.root, 'runtime-launch.json'), 0o644),
    f => writePrivate(join(f.root, 'runtime-owner.json'), { ...f.candidate, owner: 'foreign', token: 'f'.repeat(64) }),
    f => { const directory = join(f.root, 'final-candidates'); mkdirSync(directory, { mode: 0o700 }); writePrivate(join(directory, '.owner.json'), { format: 1, owner: 'foreign' }, true); },
    f => { const target = join(f.parent, 'foreign'); mkdirSync(target, { mode: 0o700 }); symlinkSync(target, join(f.root, 'final-candidates')); },
    f => symlinkSync(join(f.parent, 'missing'), join(f.root, 'final-candidate.json')),
    f => { const path = join(f.root, 'runtime-launch.json'), saved = join(f.parent, 'launch.json'); renameSync(path, saved); symlinkSync(saved, path); },
  ];
  for (const mutation of mutations) {
    const f = fixture();
    try { mutation(f); await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 0); }
    finally { f.cleanup(); }
  }
});

test('claimed evidence path replacement cannot cause model calls or writes into a foreign directory', async () => {
  const f = fixture();
  try {
    const foreign = join(f.parent, 'foreign'); mkdirSync(foreign, { mode: 0o700 });
    const originalProof = f.options.headProof;
    f.options.headProof = async () => {
      renameSync(join(f.root, 'final-candidates'), join(f.root, 'saved-cohorts'));
      symlinkSync(foreign, join(f.root, 'final-candidates'));
      return originalProof();
    };
    await assert.rejects(() => finalCandidate(f.root, f.options));
    assert.equal(f.requests(), 0); assert.deepEqual(readdirSync(foreign), []);
  } finally { f.cleanup(); }
});

test('new evidence entries appearing during preflight cannot bypass the cohort inventory', async () => {
  const f = fixture();
  try {
    const originalProof = f.options.headProof;
    f.options.headProof = async () => {
      writePrivate(join(f.root, 'final-candidates', '0'.repeat(64) + '.json'), { partial: true }, true);
      return originalProof();
    };
    await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 0);
  } finally { f.cleanup(); }
});

test('a claimed complete summary does not hide unfinished fixed-case attempts', async () => {
  const f = fixture();
  try {
    await assert.rejects(() => finalCandidate(f.root, { ...f.options, headProof: async () => { throw new Error('synthetic interruption'); } }));
    const path = ledgerPaths(f)[0], partial = JSON.parse(readFileSync(path));
    partial.summary = { complete: true, passed: false }; writePrivate(path, partial);
    installCandidate(f);
    await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 0);
  } finally { f.cleanup(); }
});

test('runtime identity drift fences later model calls and leaves the fixed denominator incomplete', async () => {
  const f = fixture();
  try {
    const originalRequest = f.options.request;
    f.options.request = async (...args) => { const response = await originalRequest(...args); f.runtime.activeBinary.manifestDigest = 'sha256:' + '0'.repeat(64); return response; };
    await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 1);
    const partial = JSON.parse(readFileSync(ledgerPaths(f)[0]));
    assert.equal(partial.attempts[0].state, 'finished'); assert.equal(partial.attempts.slice(1).every(item => item.state === 'pending'), true);
    assert.equal(finalCandidateSummary(partial.attempts).complete, false);
  } finally { f.cleanup(); }
});

test('postflight configuration and head changes retain results but cannot produce a green cohort', async () => {
  for (const name of ['modelConfiguration', 'headProof', 'runtimeStatus']) {
    const f = fixture();
    try {
      successfulResponses(f);
      const original = f.options[name]; let calls = 0;
      f.options[name] = async () => {
        const value = structuredClone(await original());
        if (++calls === (name === 'runtimeStatus' ? 8 : 2)) {
          if (name === 'modelConfiguration') value.configurationDigest = 'sha256:' + '0'.repeat(64);
          else if (name === 'headProof') value.currentRelease = 'rls_drifted';
          else value.mode = 'source';
        }
        return value;
      };
      const result = await finalCandidate(f.root, f.options);
      assert.equal(f.requests(), 10); assert.equal(result.attempts.every(item => item.passed), true);
      assert.equal(result.summary.complete, true); assert.equal(result.summary.passed, false);
      assert.equal(result.identityStable, false); assert.equal(result.stoppedReason, 'CANDIDATE_IDENTITY_UNVERIFIED');
      assert.equal(JSON.parse(readFileSync(ledgerPaths(f)[0])).identityStable, false);
    } finally { f.cleanup(); }
  }
});

test('verified identity permits the unchanged complete successful cohort with five model steps and four executions', async () => {
  const f = fixture();
  try {
    successfulResponses(f);
    const result = await finalCandidate(f.root, f.options);
    assert.deepEqual(result.summary, { positive: { passed: 4, total: 4 }, negative: { passed: 2, total: 2 }, complete: true, passed: true });
    assert.equal(result.identityStable, true); assert.equal(result.providerAttempts, 5); assert.equal(result.after.executions, 4);
    assert.equal(f.requests(), 10); assert.equal(result.attempts.every(item => item.state === 'finished'), true);
  } finally { f.cleanup(); }
});

test('the fixed cohort binds its original or approved supplemental knowledge basis before and after',async()=>{
  const f=fixture();
  try{
    successfulResponses(f);let reads=0;const proof=f.options.headProof;
    f.options.headProof=async()=>({...await proof(),knowledgeBasis:{kind:++reads===1?'original':'approved-values',fixtureDigest:f.state.fixtureDigest,contentDigest:'sha256:'+'9'.repeat(64),receiptDigest:'sha256:'+'1'.repeat(64),pointerDigest:'sha256:'+'2'.repeat(64),manifestDigest:'sha256:'+'3'.repeat(64),sourceProofDigest:'sha256:'+'4'.repeat(64)}});
    const result=await finalCandidate(f.root,f.options);
    assert.equal(result.attempts.every(x=>x.passed),true);assert.equal(result.summary.passed,false);assert.equal(result.identityStable,false);
    assert.equal(result.knowledgeBasisBefore.kind,'original');assert.equal(result.knowledgeBasisAfter.kind,'approved-values');
  }finally{f.cleanup();}
});

test('an unknown HTTP outcome keeps all pending cases in the failed denominator and forbids another cohort', async () => {
  const f = fixture();
  try {
    const request = f.options.request;
    f.options.request = async (...args) => { await request(...args); return { status: 0, body: { code: 'HTTP_OUTCOME_UNKNOWN' } }; };
    const result = await finalCandidate(f.root, f.options);
    assert.equal(result.summary.complete, false); assert.equal(result.summary.passed, false);
    assert.equal(result.stoppedReason, 'UNKNOWN_HTTP_OUTCOME'); assert.equal(f.requests(), 1);
    assert.equal(result.attempts.slice(1).every(item => item.state === 'pending'), true);
    installCandidate(f); await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 1);
  } finally { f.cleanup(); }
});

test('unverifiable model configuration blocks attempts after preserving the one-shot claim', async () => {
  const f = fixture();
  try {
    await assert.rejects(() => finalCandidate(f.root, { ...f.options, modelConfiguration: async () => ({ providerId: 'provider', settingId: 'setting' }) }));
    assert.equal(f.requests(), 0); assert.equal(ledgerPaths(f).length, 1);
    await assert.rejects(() => finalCandidate(f.root, f.options)); assert.equal(f.requests(), 0);
  } finally { f.cleanup(); }
});

test('cohort model identity uses the actual server generation revision without persisting configuration values', async () => {
  const f = fixture();
  try {
    let requested = 0;
    const revision = 'sha256:' + '8'.repeat(64);
    const response = { status: 200, body: { items: [{ provider: { id: 'provider', enabled: true }, models: [{ id: 'setting', providerId: 'provider', kind: 'llm', enabled: true, isDefault: true, generationConfigRevision: revision }] }] } };
    const request = async (root, actor, method, path) => {
      requested++; assert.equal(root, f.root); assert.equal(actor, 'admin'); assert.equal(method, 'GET'); assert.equal(path, '/api/v1/workspaces/wsp_synthetic/governance/model-providers');
      return response;
    };
    assert.deepEqual(await currentModelConfiguration(f.root, request), { providerId: 'provider', settingId: 'setting', configurationDigest: revision });
    delete response.body.items[0].models[0].generationConfigRevision;
    await assert.rejects(() => currentModelConfiguration(f.root, request));
    assert.equal(requested, 2); assert.equal(f.requests(), 0);
  } finally { f.cleanup(); }
});

test('final candidate fixes all four positives and two negatives without consulting prior success', () => {
  const cases = finalCandidateCases();
  assert.deepEqual(cases.map(item => item.id), ['total', 'region', 'monthly', 'old-customer-average', 'denied', 'ambiguous']);
  assert.equal(cases.filter(item => item.expected).length, 4);
  assert.doesNotMatch(cases.at(-1).question, /澄清|请先|不要|未指定|模型/);
  assert.equal(new Set(cases.map(item => item.id)).size, 6);
  assert.equal(digest(JSON.stringify(cases)), 'sha256:c0a64895a5010d642b2a4819a9866a8831cf4166c957c08a20043d61188bfb6a');
});

test('final candidate first-pass denominator includes failures and unattempted cases', () => {
  const attempts = finalCandidateCases().map(item => ({ case: item.id, state: 'pending', passed: false }));
  attempts[0] = { ...attempts[0], state: 'finished', passed: true };
  const summary = finalCandidateSummary(attempts);
  assert.deepEqual(summary.positive, { passed: 1, total: 4 });
  assert.deepEqual(summary.negative, { passed: 0, total: 2 });
  assert.equal(summary.complete, false);
  assert.equal(summary.passed, false);
  assert.throws(() => finalCandidateSummary([...attempts, attempts[0]]));
});

import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { databaseQuery } from '../../demo/provision.mjs';
import { readState, readPrivate, writePrivate, goldenCases } from './core.mjs';
import { verifyProvisioned } from './provision.mjs';
import { connect } from './initialize.mjs';
import { repo } from './runtime.mjs';

function cell(value) {
  if (value === null) return null;
  const text = String(value);
  return /^-?\d+(?:\.\d+)?$/.test(text) ? text.replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '').replace(/^(-?)0+(?=\d)/, '$1') : text;
}
export function reconcileExecution(result, expected, kind) {
  assert.equal(result.run?.state, 'succeeded');
  assert.equal(result.availability, 'ephemeral');
  assert.match(result.sql, /^\s*SELECT\b/i);
  let rows = result.rows;
  if (result.columns) {
    const expectedColumns = { total: ['demo_202609.amount'], region: ['demo_202609.customers.region', 'demo_202609.amount'], monthly: ['demo_202609.orders.paid_at.period', 'demo_202609.amount'], 'old-customer-average': ['demo_202609.average'] }[kind];
    assert.ok(expectedColumns);
    assert.equal(result.columns.length, expectedColumns.length);
    const indexes = expectedColumns.map(name => result.columns.indexOf(name));
    assert.ok(indexes.every(index => index >= 0));
    rows = rows.map(row => indexes.map(index => row[index]));
  }
  const normalize = rows => rows.map(row => row.map((value, index) => kind === 'monthly' && index === 0 ? String(value).slice(0, 7) : cell(value))).sort((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));
  assert.deepEqual(normalize(rows), normalize(expected));
  return { passed: true, rows: result.rows.length };
}
export function assertNoExecution(response, before, after, kind) {
  assert.equal(after.executions, before.executions, 'Negative request created execution');
  if (kind === 'denied') {
    assert.equal(response.status, 403);
    assert.equal(response.body?.code, 'NO_MATCHING_GRANT');
    assert.equal(after.agentRuns, before.agentRuns, 'Denied request started a model run');
    assert.equal(after.modelSteps, before.modelSteps, 'Denied request invoked a model step');
  }
  else {
    assert.equal(response.status, 200);
    assert.equal(response.body?.interpretation?.outcome, 'clarification');
    assert.equal(response.body?.resolution, undefined);
  }
}
export function assertPlanPins(plan, state, releaseId) {
  assert.equal(plan.releaseId, releaseId);
  assert.equal(plan.model?.assetId, state.published['demo_202609.model'].targetId);
  assert.equal(plan.model?.revisionId, state.published['demo_202609.model'].revisionId);
  assert.ok(plan.id && plan.planDigest);
}
export function assertQueryWindow(query, kind) {
  const from = kind === 'monthly' ? '2026-07-01T00:00:00Z' : '2026-08-01T00:00:00Z';
  assert.equal(Date.parse(query.timeRange?.from), Date.parse(from));
  assert.equal(Date.parse(query.timeRange?.to), Date.parse('2026-09-01T00:00:00Z'));
  if (kind === 'monthly') assert.equal(query.timeRange.granularity, 'month');
}
function windowReview(query, kind) {
  const result = { from: query.timeRange?.from, to: query.timeRange?.to, granularity: query.timeRange?.granularity, expectedFrom: kind === 'monthly' ? '2026-07-01T00:00:00Z' : '2026-08-01T00:00:00Z', expectedTo: '2026-09-01T00:00:00Z' };
  try { assertQueryWindow(query, kind); result.passed = true; }
  catch { result.passed = false; }
  return result;
}
export function caseProven(ledger, id) {
  return ledger.attempts.some(attempt => {
    if (attempt.case !== id || attempt.semanticWindow?.passed === false || attempt.semanticWindowReview?.passed === false) return false;
    return attempt.passed || ledger.executionReplays?.some(replay => replay.case === id && replay.askAttempt === attempt.number && (replay.passed || replay.comparisonReview?.passed));
  });
}
export function independentRows(root, sql) {
  assert.match(sql, /^SELECT\b/i);
  const state = readState(root), target = verifyProvisioned(repo, state);
  const raw = databaseQuery(target.container, state.owner + '_source', `BEGIN READ ONLY; SELECT COALESCE(json_agg(row_to_json(q)),'[]'::json) FROM (${sql}) q; COMMIT;`);
  return JSON.parse(raw).map(row => Object.values(row));
}
export function counters(root) {
  const state = readState(root), target = verifyProvisioned(repo, state);
  return JSON.parse(databaseQuery(target.container, state.owner + '_app', "SELECT json_build_object('schema',(SELECT version FROM schema_migrations LIMIT 1),'executions',(SELECT count(*) FROM query_execution_runs),'agentRuns',(SELECT count(*) FROM agent_runs),'modelSteps',(SELECT count(*) FROM agent_steps WHERE kind='model'),'unfinished',(SELECT count(*) FROM agent_runs WHERE status='running'));"));
}
export async function requestAs(root, actor, method, path, body, key) {
  const state = readState(root), { client } = await connect(root, state, actor);
  const origin = `http://127.0.0.1:${state.apiPort}`;
  assert.ok(path.startsWith(`/api/v1/workspaces/${state.workspace.id}/`));
  const headers = { Cookie: client.sessionCookie(), Origin: origin, Accept: 'application/json' };
  const session = await fetch(origin + '/api/v1/session', { headers, signal: AbortSignal.timeout(10000), redirect: 'error' });
  assert.equal(session.status, 200);
  const csrf = session.headers.get('X-Semlia-CSRF');
  await session.body?.cancel();
  assert.ok(csrf);
  let response;
  try {
    response = await fetch(origin + path, { method, headers: { ...headers, 'Content-Type': 'application/json', 'X-Semlia-CSRF': csrf, ...(key ? { 'Idempotency-Key': key } : {}) }, body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(75000), redirect: 'error' });
  } catch {
    return { status: 0, body: { code: 'HTTP_OUTCOME_UNKNOWN' } };
  }
  try { return { status: response.status, body: await response.json() }; }
  catch { return { status: response.status, body: { code: 'HTTP_RESPONSE_INVALID' } }; }
}

export function readModelLedger(root) {
  const state = readState(root), path = join(root, 'model-acceptance.json');
  const ledger = existsSync(path) ? readPrivate(path) : { owner: state.owner, baselineRelease: state.latestRelease, attempts: [] };
  assert.equal(ledger.owner, state.owner);
  return ledger;
}
const safeCode = response => /^[A-Z][A-Z0-9_]{2,63}$/.test(response.body?.code) ? response.body.code : undefined;
function modelCases() {
  return [...goldenCases().map(item => ({ ...item, actor: 'consumer', question: '使用已发布分析模型 demo_202609.model，' + item.question })),
    { id: 'denied', actor: 'denied', question: '使用已发布分析模型 demo_202609.model，2026年8月的有效支付金额是多少？' },
    { id: 'ambiguous', actor: 'consumer', question: '帮我算一下那个指标。未指定指标、时间或分析模型，请先向我澄清，不要自行假设。' }];
}
export function acceptanceRound(id) {
  const rounds = {
    initial: { cases: modelCases().map(item => item.id), correction: 'Initial schema32 then schema33 durable Ask and 8192 output budget' },
    'ask-budget-16384-safe-diagnostics': { cases: ['monthly', 'old-customer-average'], correction: 'Ask output budget 16384 and safe first-response schema diagnostics; provider and timeout unchanged' },
    'ask-half-open-time-window': { cases: ['monthly'], correction: 'Explicit half-open [from,to) time-window prompt and time-bucket order compilation; unchanged question/provider/model/60s timeout/16384 cap' },
  };
  assert.ok(Object.hasOwn(rounds, id), 'Unapproved model acceptance round');
  return { id, maxAttemptsPerCase: 2, ...rounds[id] };
}
export async function modelBaseline(root, retry = false, round = 'initial') {
  const roundConfig = acceptanceRound(round);
  const state = readState(root), base = `/api/v1/workspaces/${state.workspace.id}`;
  const path = join(root, 'model-acceptance.json'), ledger = readModelLedger(root);
  const save = () => writePrivate(path, ledger);
  const cases = modelCases().filter(item => roundConfig.cases.includes(item.id));
  ledger.rounds ??= [];
  if (!ledger.rounds.some(item => item.id === round)) {
    ledger.rounds.push({ ...roundConfig, authorizedBy: 'orchestrator', startedAt: new Date().toISOString() }); save();
  }
  if (round === 'ask-budget-16384-safe-diagnostics' && retry) {
    ledger.rounds.find(item => item.id === round).secondAttemptCorrection = 'Time-granularity compare validation contract; unchanged question/model/timeout'; save();
  }
  for (const item of cases) {
    const allPrevious = ledger.attempts.filter(x => x.case === item.id);
    const previous = allPrevious.filter(x => (x.round ?? 'initial') === round);
    if ((previous.some(x => x.passed) && (item.expected || !retry)) || (previous.length && !retry)) continue;
    if (previous.length >= roundConfig.maxAttemptsPerCase || previous.some(x => x.state === 'started' || x.askStatus === 0)) continue;
    const number = allPrevious.length + 1;
    const attempt = { case: item.id, number, round, roundAttempt: previous.length + 1, actor: item.actor, state: 'started', startedAt: new Date().toISOString(), before: counters(root) };
    ledger.attempts.push(attempt); save();
    const key = `v1-model-${item.id}-${number}`;
    const asked = await requestAs(root, item.actor, 'POST', base + '/ask', { question: item.question, context: { mode: 'current' }, idempotencyKey: key });
    attempt.askStatus = asked.status; attempt.code = safeCode(asked);
    attempt.afterAsk = counters(root);
    // Full normal API receipts are synthetic and protected; public evidence only sees selected metadata.
    writePrivate(join(root, `model-${item.id}-${number}-ask.json`), asked);
    if (item.expected && asked.status === 200 && asked.body.interpretation?.outcome === 'query' && asked.body.resolution?.plan) {
      attempt.modelProducedPlan = true;
      attempt.semanticWindow = windowReview(asked.body.interpretation.query, item.id);
      const plan = asked.body.resolution.plan;
      try {
        assertPlanPins(plan, state, ledger.baselineRelease);
        const executed = await requestAs(root, item.actor, 'POST', base + `/resolved-semantic-plans/${plan.id}:execute`, { planDigest: plan.planDigest, idempotencyKey: key + '-execute' });
        writePrivate(join(root, `model-${item.id}-${number}-execute.json`), executed);
        attempt.executeStatus = executed.status; attempt.executionCode = safeCode(executed);
        assert.equal(executed.status, 200);
        assert.equal(executed.body.run.planId, plan.id);
        assert.equal(executed.body.run.planDigest, plan.planDigest);
        attempt.independentRows = independentRows(root, item.sql);
        reconcileExecution({ ...executed.body, columns: undefined, rows: attempt.independentRows }, item.expected, item.id);
        attempt.comparison = reconcileExecution(executed.body, attempt.independentRows, item.id); attempt.dataReconciled = true; attempt.passed = attempt.semanticWindow.passed;
        if (!attempt.passed) attempt.reason = 'SEMANTIC_TIME_WINDOW_MISMATCH';
      }
      catch { attempt.passed = false; attempt.reason = 'EXECUTION_OR_RESULT_MISMATCH'; }
    } else if (!item.expected) {
      try { assertNoExecution(asked, attempt.before, attempt.afterAsk, item.id); attempt.passed = true; }
      catch { attempt.passed = false; attempt.reason = 'NEGATIVE_CONTRACT_MISMATCH'; }
    } else { attempt.passed = false; attempt.reason = 'ASK_DID_NOT_PRODUCE_PLAN'; }
    attempt.after = counters(root); attempt.state = 'finished'; save();
    console.log(JSON.stringify({ case: attempt.case, number, round, askStatus: attempt.askStatus, code: attempt.code, executeStatus: attempt.executeStatus, executionCode: attempt.executionCode, passed: attempt.passed, reason: attempt.reason, schema: attempt.before.schema, agentRuns: attempt.after.agentRuns - attempt.before.agentRuns, executions: attempt.after.executions - attempt.before.executions }));
  }
  return ledger;
}

export async function replayFixedExecutions(root, compiler = false) {
  const state = readState(root), ledger = readModelLedger(root), base = `/api/v1/workspaces/${state.workspace.id}`;
  const path = join(root, 'model-acceptance.json');
  ledger.executionReplays ??= [];
  const save = () => writePrivate(path, ledger);
  for (const attempt of ledger.attempts.filter(x => x.askStatus === 200 && !x.passed && (compiler ? x.case === 'monthly' && x.executeStatus === 422 && x.executionCode === 'EXECUTION_PLAN_UNSUPPORTED' : x.executeStatus === 200))) {
    if (ledger.executionReplays.some(x => x.case === attempt.case && x.askAttempt === attempt.number)) continue;
    const item = goldenCases().find(x => x.id === attempt.case);
    if (!item) continue;
    const original = readPrivate(join(root, `model-${item.id}-${attempt.number}-execute.json`));
    if (compiler) assert.equal(original.body.code, 'EXECUTION_PLAN_UNSUPPORTED');
    else assert.equal(original.body.run.errorCode, 'EXECUTION_ROLE_NOT_READ_ONLY');
    const asked = readPrivate(join(root, `model-${item.id}-${attempt.number}-ask.json`));
    const plan = asked.body.resolution.plan;
    assertPlanPins(plan, state, ledger.baselineRelease);
    const suffix = compiler ? 'compiler-fixed' : 'reader-fixed';
    const replay = { case: item.id, askAttempt: attempt.number, reason: compiler ? 'TIME_BUCKET_ORDER_COMPILER_FIXED' : 'OWNED_READER_PRIVACY_CONFIGURED', semanticWindow: windowReview(asked.body.interpretation.query, item.id), state: 'started', before: counters(root) };
    ledger.executionReplays.push(replay); save();
    const executed = await requestAs(root, 'consumer', 'POST', base + `/resolved-semantic-plans/${plan.id}:execute`, { planDigest: plan.planDigest, idempotencyKey: `v1-model-${item.id}-${attempt.number}-${suffix}` });
    writePrivate(join(root, `model-${item.id}-${attempt.number}-${suffix}-execute.json`), executed);
    replay.status = executed.status; replay.runState = executed.body.run?.state; replay.errorCode = executed.body.run?.errorCode;
    try {
      assert.equal(executed.status, 200);
      assert.equal(executed.body.run.planId, plan.id); assert.equal(executed.body.run.planDigest, plan.planDigest);
      replay.independentRows = independentRows(root, item.sql);
      reconcileExecution({ ...executed.body, columns: undefined, rows: replay.independentRows }, item.expected, item.id);
      reconcileExecution(executed.body, replay.independentRows, item.id);
      replay.after = counters(root); assert.equal(replay.after.agentRuns, replay.before.agentRuns); assert.equal(replay.after.modelSteps, replay.before.modelSteps);
      replay.passed = true;
    } catch { replay.passed = false; }
    replay.state = 'finished'; save();
    console.log(JSON.stringify({ case: item.id, askAttempt: attempt.number, executionReplay: true, passed: replay.passed, semanticWindowPassed: replay.semanticWindow.passed, runState: replay.runState, errorCode: replay.errorCode, modelCalls: 0 }));
  }
  return ledger;
}

export function reverifyExecutionReceipts(root) {
  const ledger = readModelLedger(root);
  for (const replay of ledger.executionReplays ?? []) {
    if (replay.runState !== 'succeeded' || replay.passed || replay.comparisonReview) continue;
    const item = goldenCases().find(x => x.id === replay.case);
    const executed = readPrivate(join(root, `model-${item.id}-${replay.askAttempt}-reader-fixed-execute.json`));
    const independent = independentRows(root, item.sql);
    reconcileExecution({ ...executed.body, columns: undefined, rows: independent }, item.expected, item.id);
    reconcileExecution(executed.body, independent, item.id);
    replay.comparisonReview = { passed: true, reason: 'COLUMN_ORDER_VERIFIER_FIXED', checkedAt: new Date().toISOString(), noNewExecution: true, noNewModelCall: true };
    writePrivate(join(root, 'model-acceptance.json'), ledger);
    console.log(JSON.stringify({ case: item.id, passed: true, reason: replay.comparisonReview.reason, newExecutions: 0, modelCalls: 0 }));
  }
  return ledger;
}

export function auditSavedWindows(root) {
  const ledger = readModelLedger(root);
  for (const attempt of ledger.attempts) {
    if (attempt.askStatus !== 200 || !goldenCases().some(item => item.id === attempt.case)) continue;
    const saved = readPrivate(join(root, `model-${attempt.case}-${attempt.number}-ask.json`));
    if (!saved.body.interpretation?.query) continue;
    attempt.semanticWindowReview = windowReview(saved.body.interpretation.query, attempt.case);
    console.log(JSON.stringify({ case: attempt.case, attempt: attempt.number, semanticWindow: attempt.semanticWindowReview }));
  }
  writePrivate(join(root, 'model-acceptance.json'), ledger);
  return ledger;
}

export async function replayClaimedRequests(root) {
  const state = readState(root), ledger = readModelLedger(root), base = `/api/v1/workspaces/${state.workspace.id}`;
  ledger.claimReplays ??= [];
  for (const item of modelCases()) {
    const attempt = ledger.attempts.filter(x => x.case === item.id && x.before.schema === 33 && x.state === 'finished' && x.askStatus > 0).at(-1);
    if (!attempt) continue;
    const before = counters(root);
    const replay = await requestAs(root, item.actor, 'POST', base + '/ask', { question: item.question, context: { mode: 'current' }, idempotencyKey: `v1-model-${item.id}-${attempt.number}` });
    const after = counters(root);
    const result = { case: item.id, attempt: attempt.number, status: replay.status, code: safeCode(replay), before, after, checkedAt: new Date().toISOString() };
    try {
      assert.equal(after.agentRuns, before.agentRuns); assert.equal(after.modelSteps, before.modelSteps); assert.equal(after.executions, before.executions);
      if (replay.body.code === 'ASK_CONTEXT_CHANGED') assert.equal(replay.status, 409);
      else {
        assert.equal(replay.status, attempt.askStatus);
        if (replay.status === 200) {
          const original = readPrivate(join(root, `model-${item.id}-${attempt.number}-ask.json`));
          assert.equal(replay.body.agentRun.id, original.body.agentRun.id);
          assert.equal(replay.body.resolution?.id, original.body.resolution?.id);
          assert.equal(replay.body.resolution?.plan?.id, original.body.resolution?.plan?.id);
        } else assert.equal(replay.body.code, attempt.code);
      }
      result.passed = true;
    } catch { result.passed = false; }
    ledger.claimReplays.push(result); writePrivate(join(root, 'model-acceptance.json'), ledger);
    console.log(JSON.stringify({ case: item.id, claimReplay: true, status: result.status, code: result.code, passed: result.passed, newAgentRuns: after.agentRuns - before.agentRuns, newExecutions: after.executions - before.executions }));
  }
  return ledger;
}

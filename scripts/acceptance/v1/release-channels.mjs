import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { root, repo } from './runtime.mjs';
import { readState, readPrivate, writePrivate, goldenCases } from './core.mjs';
import { releaseRoot, readPlan } from './release-environment.mjs';
import { connect } from './initialize.mjs';
import { ensureBinding } from '../../demo/initialize.mjs';
import { governanceHeadProof } from './correction.mjs';
import { counters, readModelLedger, assertQueryWindow, independentRows, reconcileExecution } from './model.mjs';
import { compareChannelProofs, assertCLIOutcome } from './release-core.mjs';

async function machineRequest(origin, token, path, payload, mcp = false) {
  const response = await fetch(origin + path, { method: 'POST', headers: { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json', Accept: mcp ? 'application/json, text/event-stream' : 'application/json', ...(mcp ? { 'MCP-Protocol-Version': '2025-03-26' } : {}) }, body: JSON.stringify(payload), redirect: 'error', signal: AbortSignal.timeout(30000) });
  const text = await response.text(); let body;
  if (response.headers.get('content-type')?.includes('text/event-stream')) {
    const messages = text.split('\n').filter(line => line.startsWith('data:')).map(line => JSON.parse(line.slice(5)));
    body = messages.find(item => item.id === payload.id); assert.ok(body, 'Missing MCP response');
  } else { try { body = JSON.parse(text); } catch { body = {}; } }
  return { status: response.status, body };
}
function cli(state, token, command, input, key) {
  const result = spawnSync(join(root, 'server'), ['semantic', command, JSON.stringify(input)], { cwd: repo, encoding: 'utf8', timeout: 30000, maxBuffer: 2 * 1024 * 1024, env: { PATH: process.env.PATH, HOME: process.env.HOME, SEMLIA_API_URL: `http://127.0.0.1:${state.apiPort}`, SEMLIA_WORKSPACE_ID: state.workspace.id, SEMLIA_API_TOKEN: token, SEMLIA_IDEMPOTENCY_KEY: key } });
  let body; try { body = JSON.parse(result.stdout); } catch { body = {}; }
  const marker = /^semantic request refused \(HTTP (\d{3})\)\n$/.exec(result.stderr ?? '');
  return { exit: result.status, spawned: !result.error, signal: result.signal, httpStatus: marker ? Number(marker[1]) : null, body };
}

export async function channels(resume = false) {
  const drill = readPlan(), state = readState(root), path = join(releaseRoot, 'channels.json');
  let prior;
  if (resume) {
    prior = readPrivate(path); assert.equal(prior.owner, drill.owner); assert.equal(prior.state, 'failed'); assert.equal(prior.revokedAfterFailure, true); assert.deepEqual(prior.proofs, []); assert.equal(prior.before.executions, prior.after.executions);
    writePrivate(join(releaseRoot, 'channels-failed-ledger-field.json'), prior, true);
  } else assert.ok(!existsSync(path), 'Channel attempt exists; inspect rather than replay');
  const head = await governanceHeadProof(root); assert.ok(head.correctBaselineContent);
  const { client: admin } = await connect(root, state, 'admin'), base = `/api/v1/workspaces/${state.workspace.id}`, origin = `http://127.0.0.1:${state.apiPort}`;
  const ledger = { owner: drill.owner, state: 'started', before: counters(root), head, stages: {}, proofs: [] };
  const save = () => writePrivate(path, ledger); save();
  let credential;
  try {
    ledger.stage = 'principal-create'; save();
    const principal = prior ? { id: prior.principalId } : await admin.request('POST', base + '/machine-principals', { name: drill.owner + ' synthetic machine' });
    ledger.principalId = principal.id; save();
    await ensureBinding(admin, state.workspace.id, principal.id, 'consumer_developer');
    ledger.stage = 'consumer-create'; save();
    const consumer = prior ? await admin.request('GET', base + '/consumers/' + prior.consumerId) : await admin.request('POST', base + '/consumers', { stableKey: drill.owner.replaceAll('_', '-'), name: 'V1 synthetic release channels', kind: 'agent' });
    assert.equal(consumer.stableKey, drill.owner.replaceAll('_', '-'));
    ledger.consumerId = consumer.id; save();
    const binding = prior ? await admin.request('GET', base + '/consumer-bindings/' + prior.bindingId) : await admin.request('POST', base + '/consumer-bindings', { consumerId: consumer.id, environment: 'test', purpose: 'Synthetic release acceptance only', mode: 'pinned', releaseId: head.currentRelease });
    assert.equal(binding.consumerId, consumer.id); assert.equal(binding.releaseId, head.currentRelease); assert.equal(binding.mode, 'pinned');
    ledger.bindingId = binding.id; save();
    credential = await admin.request('POST', base + '/client-credentials', { principalId: principal.id, consumerId: consumer.id, bindingId: binding.id, name: 'V1 REST MCP CLI synthetic acceptance', allowedActions: ['asset.read', 'semantic.resolve', 'semantic.execute'], scopeType: 'workspace', scopeId: state.workspace.id, expiresAt: new Date(Date.now() + 3600000).toISOString() });
    writePrivate(join(releaseRoot, resume ? 'machine-credential-resume.json' : 'machine-credential.json'), { owner: drill.owner, ...credential }, true);
    ledger.credentialId = credential.credential.id; save();
    ledger.stage = 'query-receipt'; save();
    const asked = readModelLedger(root).attempts.filter(item => item.case === 'total').map(item => readPrivate(join(root, `model-total-${item.number}-ask.json`))).find(response => response.status === 200 && response.body.interpretation?.outcome === 'query' && response.body.resolution?.plan);
    assert.ok(asked); const query = structuredClone(asked.body.interpretation.query);
    assertQueryWindow(query, 'total'); query.context = { mode: 'binding', bindingId: binding.id };
    const expected = independentRows(root, goldenCases().find(item => item.id === 'total').sql), token = credential.token;
    let sequence = 0;
    const mcp = async (method, params) => machineRequest(origin, token, base + '/mcp', { jsonrpc: '2.0', id: ++sequence, method, params }, true);
    ledger.stage = 'mcp-initialize'; save();
    const initialized = await mcp('initialize', { protocolVersion: '2025-03-26', capabilities: {}, clientInfo: { name: 'semlia-v1-synthetic-release', version: '1' } });
    assert.equal(initialized.status, 200); assert.ok(initialized.body.result?.protocolVersion);
    for (const channel of ['api', 'mcp', 'cli']) {
      ledger.stage = channel + '-resolve'; save();
      const key = drill.owner + '-' + channel;
      let resolved;
      if (channel === 'api') { const response = await machineRequest(origin, token, base + '/semantic-queries:resolve', { query, idempotencyKey: key + '-resolve' }); assert.equal(response.status, 200); resolved = response.body; }
      else if (channel === 'mcp') { const response = await mcp('tools/call', { name: 'semantic_resolve', arguments: { query, idempotencyKey: key + '-resolve' } }); assert.equal(response.status, 200); assert.ok(!response.body.result?.isError); resolved = response.body.result.structuredContent; }
      else { const response = cli(state, token, 'resolve', query, key + '-resolve'); assertCLIOutcome(response); resolved = response.body; }
      writePrivate(join(releaseRoot, channel + '-resolve.json'), resolved);
      const plan = resolved.plan; assert.ok(plan); assert.equal(plan.releaseId, head.currentRelease); assert.equal(plan.model.revisionId, head.currentRevision);
      ledger.stage = channel + '-execute'; save();
      const input = { planId: plan.id, planDigest: plan.planDigest, idempotencyKey: key + '-execute' }; let executed;
      if (channel === 'api') { const response = await machineRequest(origin, token, base + `/resolved-semantic-plans/${plan.id}:execute`, input); assert.equal(response.status, 200); executed = response.body; }
      else if (channel === 'mcp') { const response = await mcp('tools/call', { name: 'semantic_execute', arguments: input }); assert.equal(response.status, 200); assert.ok(!response.body.result?.isError); executed = response.body.result.structuredContent; }
      else { const response = cli(state, token, 'execute', input, key + '-execute'); assertCLIOutcome(response); executed = response.body; }
      writePrivate(join(releaseRoot, channel + '-execute.json'), executed);
      reconcileExecution(executed, expected, 'total'); assert.equal(executed.run.channel, channel);
      ledger.proofs.push({ channel, releaseId: plan.releaseId, model: plan.model, planDigest: plan.planDigest, resultDigest: executed.run.resultDigest, planId: plan.id, runId: executed.run.id }); save();
    }
    compareChannelProofs(ledger.proofs);
    ledger.stage = 'revoke'; save();
    await admin.request('POST', base + '/client-credentials/' + credential.credential.id + '/revoke', {});
    ledger.revoked = true; ledger.beforeRevokedChecks = counters(root); save();
    const proof = ledger.proofs[0], deniedInput = { planId: proof.planId, planDigest: proof.planDigest, idempotencyKey: drill.owner + '-revoked' };
    const rest = await machineRequest(origin, token, base + `/resolved-semantic-plans/${proof.planId}:execute`, deniedInput);
    const mcpDenied = await mcp('tools/call', { name: 'semantic_execute', arguments: deniedInput });
    const cliDenied = cli(state, token, 'execute', deniedInput, deniedInput.idempotencyKey);
    assert.equal(rest.status, 401); assert.equal(mcpDenied.status, 401); assertCLIOutcome(cliDenied, true);
    ledger.revocation = { restStatus: rest.status, mcpStatus: mcpDenied.status, cliExit: cliDenied.exit };
    ledger.after = counters(root); assert.deepEqual(ledger.after, ledger.beforeRevokedChecks);
    assert.equal(ledger.after.executions - ledger.before.executions, 3); assert.equal(ledger.after.modelSteps, ledger.before.modelSteps); assert.equal(ledger.after.agentRuns, ledger.before.agentRuns);
    const final = await governanceHeadProof(root); assert.equal(final.currentRelease, head.currentRelease); assert.ok(final.correctBaselineContent);
    ledger.state = 'passed'; save(); return { state: ledger.state, proofs: ledger.proofs, revocation: ledger.revocation, executions: 3, modelCalls: 0 };
  } catch (error) {
    ledger.state = 'failed'; ledger.status = Number.isInteger(error.status) ? error.status : undefined; ledger.after = counters(root); save();
    throw error;
  } finally {
    if (credential && !ledger.revoked) {
      try { await admin.request('POST', base + '/client-credentials/' + credential.credential.id + '/revoke', {}); ledger.revokedAfterFailure = true; save(); } catch { ledger.revocationNeedsAttention = true; save(); }
    }
  }
}

export async function verifyRevokedChannels() {
  const drill = readPlan(), state = readState(root), ledger = readPrivate(join(releaseRoot, 'channels.json'));
  assert.equal(ledger.owner, drill.owner); assert.equal(ledger.state, 'passed'); assert.equal(ledger.revoked, true);
  const credentials = ['machine-credential.json', 'machine-credential-resume.json'].map(name => join(releaseRoot, name)).filter(existsSync).map(readPrivate).filter(item => item.owner === drill.owner && item.credential.id === ledger.credentialId);
  assert.equal(credentials.length, 1); const credential = credentials[0];
  const before = counters(root), proof = ledger.proofs[0], input = { planId: proof.planId, planDigest: proof.planDigest, idempotencyKey: drill.owner + '-revoked-strict' };
  const base = `/api/v1/workspaces/${state.workspace.id}`, origin = `http://127.0.0.1:${state.apiPort}`;
  const rest = await machineRequest(origin, credential.token, base + `/resolved-semantic-plans/${proof.planId}:execute`, input);
  const mcp = await machineRequest(origin, credential.token, base + '/mcp', { jsonrpc: '2.0', id: 1, method: 'tools/call', params: { name: 'semantic_execute', arguments: input } }, true);
  const result = cli(state, credential.token, 'execute', input, input.idempotencyKey);
  assert.equal(rest.status, 401); assert.equal(mcp.status, 401); assertCLIOutcome(result, true);
  const after = counters(root); assert.deepEqual(after, before);
  const receipt = { owner: drill.owner, state: 'passed', before, after, restStatus: rest.status, mcpStatus: mcp.status, cli: { exit: result.exit, spawned: result.spawned, signal: result.signal, httpStatus: result.httpStatus, code: result.body.code } };
  writePrivate(join(releaseRoot, 'channels-revocation-strict.json'), receipt, true); return receipt;
}

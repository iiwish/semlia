import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, readdirSync, readFileSync, realpathSync, rmSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { openState, checkpoint } from './v1/core.mjs';
import { browserPlan, browserEnvironment, assertNaturalDraft, assertOperationRoute, assertResultGeometry, assertConsumerEvidencePath, safeBrowserFailure, selectBrowserRuntime, assertBrowserRuntimeIdentity } from './v1-browser-core.mjs';

function driverFixture(scenario) {
  const parent = mkdtempSync(join(realpathSync('/tmp'), 'v1-browser-driver-'));
  const root = join(parent, 'run');
  checkpoint(root, openState(root, 'synthetic-fixture'), { apiPort: 52688, webPort: 52689 });
  const script = `
    import assert from 'node:assert/strict';
    import { spawn, spawnSync } from 'node:child_process';
    import { once } from 'node:events';
    import { existsSync, readdirSync } from 'node:fs';
    import { join } from 'node:path';
    import { runBrowserAcceptance } from ${JSON.stringify(new URL('./v1-browser.mjs', import.meta.url).href)};
    import { requestRuntimeControl, fetchRuntime } from ${JSON.stringify(new URL('./v1/runtime.mjs', import.meta.url).href)};
    const [root, scenario] = process.argv.slice(1);
    const runtime = { mode: 'candidate', api: 'http://127.0.0.1:52688/', url: 'http://127.0.0.1:52688/', ready: true, sourceWebAvailable: false, activeBinary: { schema: 33, binaryDigest: 'sha256:'+'a'.repeat(64), manifestDigest: 'sha256:'+'b'.repeat(64) } };
    let calls = 0, server, fixtureURL;
    if (scenario === 'transport') {
      server = spawn(process.execPath, ['--input-type=module', '-e',
        "import {createServer} from 'node:http'; import {appendFileSync} from 'node:fs';" +
        "const root=process.argv[1]; const handler=kind=>(req,res)=>{appendFileSync(root+'/transport.jsonl',JSON.stringify({kind,connection:req.headers.connection})+String.fromCharCode(10),{mode:384});res.end('ready');};" +
        "const control=createServer(handler('control')),http=createServer(handler('http'));for(const s of [control,http]){s.keepAliveTimeout=2000;s.keepAliveTimeoutBuffer=0;}" +
        "await new Promise(r=>control.listen(root+'/fixture.sock',r));await new Promise(r=>http.listen(0,'127.0.0.1',r));console.log(JSON.stringify({port:http.address().port}));" +
        "process.on('SIGTERM',()=>{for(const s of [control,http]){s.closeAllConnections();s.close();}});", root], {stdio:['ignore','pipe','pipe']});
      const [data] = await once(server.stdout, 'data'); fixtureURL = 'http://127.0.0.1:'+JSON.parse(data).port;
    }
    try { process.exitCode = await runBrowserAcceptance('candidate-preflight', { root,
      status: async () => {
        if (++calls === 2 && scenario === 'postflight') throw new Error('private-diagnostic-canary', { cause: Object.assign(new Error('private-inner-canary'), { code: 'ECONNRESET' }) });
        if (server) { await requestRuntimeControl(join(root, 'fixture.sock'), 'status', 'synthetic'); assert.equal(await (await fetchRuntime(fixtureURL)).text(), 'ready'); }
        return runtime;
      },
      counters: () => ({ modelSteps: 7, executions: 8, agentRuns: 9 }),
      governanceHeadProof: async () => ({ correctBaselineContent: true, currentRelease: 'rls_synthetic' }),
      spawnBrowser: (program, args, options) => {
        assert.equal(program, 'pnpm'); assert.ok(args.includes('@preflight'));
        const runs = readdirSync(join(root, 'browser')).filter(name => /^\\d{13}$/.test(name));
        assert.equal(runs.length, 1); assert.ok(existsSync(join(root, 'browser', runs[0], 'counters-before.json')));
        if (scenario === 'spawn-error') return spawnSync(join(root, 'missing-program'), [], options);
        const child = scenario === 'signal' ? 'process.kill(process.pid, "SIGTERM")' : scenario === 'transport' ? 'setTimeout(() => {}, 2500)' : scenario === 'timeout' ? 'setTimeout(() => {}, 2000)' : 'process.exit('+(scenario === 'failed-child' || scenario === 'postflight' ? 2 : 0)+')';
        return spawnSync(process.execPath, ['-e', child], { ...options, timeout: scenario === 'timeout' ? 50 : options.timeout });
      },
    }); } finally { if (server) { server.kill('SIGTERM'); await once(server, 'close'); } }
  `;
  const result = spawnSync(process.execPath, ['--input-type=module', '-e', script, root, scenario], { encoding: 'utf8', timeout: 15000 });
  return { parent, root, result };
}

test('actual browser driver retains before and child evidence when postflight fails without disclosing raw errors', () => {
  const fixture = driverFixture('postflight');
  try {
    assert.equal(fixture.result.status, 1);
    const output = join(fixture.root, 'browser'), run = readdirSync(output).find(name => /^\d{13}$/.test(name));
    const read = name => JSON.parse(readFileSync(join(output, run, name), 'utf8'));
    assert.deepEqual(read('counters-before.json'), { modelSteps: 7, executions: 8, agentRuns: 9 });
    assert.equal(read('child-outcome.json').status, 2);
    assert.equal(read('failure.json').stage, 'after-runtime');
    const raw = readFileSync(join(output, run, 'failure-private.json'), 'utf8');
    assert.ok(raw.includes('private-diagnostic-canary')); assert.ok(raw.includes('ECONNRESET'));
    assert.equal(statSync(join(output, run)).mode & 0o777, 0o700);
    assert.equal(statSync(join(output, run, 'failure-private.json')).mode & 0o777, 0o600);
    assert.ok(!fixture.result.stdout.includes('canary')); assert.ok(!fixture.result.stderr.includes('canary'));
    assert.equal(JSON.parse(fixture.result.stderr).stage, 'after-runtime');
  } finally { rmSync(fixture.parent, { recursive: true, force: true }); }
});

test('actual browser driver preserves success and rejects failed, signalled, timed-out and unspawned children', () => {
  for (const scenario of ['success', 'failed-child', 'signal', 'timeout', 'spawn-error']) {
    const fixture = driverFixture(scenario);
    try {
      assert.equal(fixture.result.status, scenario === 'success' ? 0 : 1, scenario);
      const output = join(fixture.root, 'browser'), run = readdirSync(output).find(name => /^\d{13}$/.test(name));
      const child = JSON.parse(readFileSync(join(output, run, 'child-outcome.json'), 'utf8'));
      const receipt = JSON.parse(readFileSync(join(output, 'driver-candidate-preflight-receipt.json'), 'utf8'));
      assert.equal(receipt.status, child.status); assert.equal(receipt.modelSteps, 0); assert.equal(receipt.executions, 0);
      if (scenario === 'signal') assert.equal(child.signal, 'SIGTERM');
      if (scenario === 'timeout') assert.equal(child.errorCode, 'ETIMEDOUT');
      if (scenario === 'spawn-error') assert.equal(child.errorCode, 'ENOENT');
    } finally { rmSync(fixture.parent, { recursive: true, force: true }); }
  }
});

test('actual browser driver uses fresh control and HTTP connections around its real blocking child', () => {
  const fixture = driverFixture('transport');
  try {
    assert.equal(fixture.result.status, 0, fixture.result.stderr);
    const requests = readFileSync(join(fixture.root, 'transport.jsonl'), 'utf8').trim().split('\n').map(line => JSON.parse(line));
    assert.equal(requests.filter(x => x.kind === 'control').length, 2);
    assert.equal(requests.filter(x => x.kind === 'http').length, 2);
    assert.ok(requests.every(x => x.connection === 'close'));
    const receipt = JSON.parse(readFileSync(join(fixture.root, 'browser', 'driver-candidate-preflight-receipt.json'), 'utf8'));
    assert.equal(receipt.child.status, 0); assert.equal(receipt.modelSteps, 0); assert.equal(receipt.executions, 0);
  } finally { rmSync(fixture.parent, { recursive: true, force: true }); }
});

test('browser acceptance allows only supported desktop projects and explicit bounded commands', () => {
  assert.equal(browserPlan('desktop').project, 'desktop');
  assert.equal(browserPlan('compact-desktop').project, 'compact-desktop');
  assert.equal(browserPlan('preflight').grep, '@preflight');
  assert.equal(browserPlan('ambiguous').grep, '@ambiguous');
  assert.equal(browserPlan('desktop-resume-route-query-optional').repair, 'route-query-optional');
  assert.throws(() => browserPlan('mobile'));
  assert.throws(() => browserPlan('retry-until-pass'));
});

test('candidate browser preflight uses verified embedded API port and refuses identity drift', () => {
  const state = { apiPort: 52688, webPort: 52689 };
  const source = { mode: 'source', url: 'http://127.0.0.1:52689/', api: 'http://127.0.0.1:52688/', ready: true, sourceWebAvailable: true, activeBinary: { binaryDigest: 'sha256:' + 'a'.repeat(64), schema: 33 } };
  const candidate = { ...source, mode: 'candidate', url: source.api, sourceWebAvailable: false, activeBinary: { ...source.activeBinary, manifestDigest: 'sha256:' + 'b'.repeat(64) } };
  assert.equal(browserPlan('candidate-preflight').grep, '@preflight');
  assert.equal(browserPlan('candidate-preflight').readOnly, true);
  assert.equal(selectBrowserRuntime(state, source).port, 52689);
  assert.equal(selectBrowserRuntime(state, candidate, true).port, 52688);
  assert.throws(() => selectBrowserRuntime(state, source, true));
  assert.throws(() => selectBrowserRuntime(state, { ...candidate, activeBinary: {} }, true));
  assert.throws(() => selectBrowserRuntime(state, { ...candidate, url: source.url }, true));
  assert.throws(() => selectBrowserRuntime(state, { ...candidate, sourceWebAvailable: true }, true));
  assertBrowserRuntimeIdentity(candidate, structuredClone(candidate));
  for (const changed of [{ ...candidate, mode: 'source' }, { ...candidate, activeBinary: { ...candidate.activeBinary, binaryDigest: 'sha256:' + 'c'.repeat(64) } }, { ...candidate, activeBinary: { ...candidate.activeBinary, manifestDigest: 'sha256:' + 'c'.repeat(64) } }]) assert.throws(() => assertBrowserRuntimeIdentity(candidate, changed));
});

test('browser postflight diagnostics disclose only allowlisted stages and status categories', () => {
  assert.deepEqual(safeBrowserFailure({ status: 429, message: 'private configuration' }, 'head-proof'), { stage: 'head-proof', code: 'RATE_LIMITED' });
  assert.deepEqual(safeBrowserFailure({ message: 'private configuration' }, 'untrusted private input'), { stage: 'unknown', code: 'UNCLASSIFIED' });
  assert.deepEqual(safeBrowserFailure({ status: '__proto__' }, 'receipt'), { stage: 'receipt', code: 'UNCLASSIFIED' });
  assert.deepEqual(safeBrowserFailure({ code: 'RUNTIME_CONTROL_UNAVAILABLE', cause: new Error('private configuration') }, 'after-runtime'), { stage: 'after-runtime', code: 'RUNTIME_CONTROL_UNAVAILABLE' });
  assert.deepEqual(safeBrowserFailure({ code: 'private configuration' }, 'after-runtime'), { stage: 'after-runtime', code: 'UNCLASSIFIED' });
});

test('consumer visual preflight cannot request governance or production administration', () => {
  assertConsumerEvidencePath('/api/v1/workspaces/wsp_example/catalog/assets/ast_model');
  assertConsumerEvidencePath('/api/v1/workspaces/wsp_example/catalog/assets/ast_model/revisions/rev_exact');
  assert.throws(() => assertConsumerEvidencePath('/api/v1/workspaces/wsp_example/governance/releases?limit=1'));
  assert.throws(() => assertConsumerEvidencePath('/api/v1/workspaces/wsp_example/production-releases/rls_head'));
});

test('execution cells must be in the viewport and survive composer and hit-test occlusion checks', () => {
  const sample = { viewport: { width: 1024, height: 768 }, composer: { x: 100, y: 650, width: 900, height: 100 }, cells: [{ x: 120, y: 560, width: 600, height: 25, unobscured: true }, { x: 120, y: 585, width: 600, height: 25, unobscured: true }] };
  assertResultGeometry(sample);
  assert.throws(() => assertResultGeometry({ ...sample, cells: [{ ...sample.cells[0], y: 660 }, sample.cells[1]] }));
  assert.throws(() => assertResultGeometry({ ...sample, cells: [{ ...sample.cells[0], unobscured: false }, sample.cells[1]] }));
  assert.throws(() => assertResultGeometry({ ...sample, cells: [{ ...sample.cells[0], x: -1 }, sample.cells[1]] }));
});

test('operation route permits absent query but rejects another question mark or incorrect scope', () => {
  assertOperationRoute('http://127.0.0.1/work/operations/op', 'op', null);
  assertOperationRoute('http://127.0.0.1/work/operations/op?scope=initiated', 'op', 'initiated');
  assert.throws(() => assertOperationRoute('http://127.0.0.1/work/operations/op?scope=initiated?from=x', 'op', 'initiated'));
  assert.throws(() => assertOperationRoute('http://127.0.0.1/work/operations/wrong', 'op', null));
  assert.throws(() => assertOperationRoute('http://127.0.0.1/work/operations/op?scope=pending', 'op', 'initiated'));
});

test('browser process inherits neither provider secrets nor database configuration', () => {
  const env = browserEnvironment('/owned/private', 52689, { PATH: '/bin', HOME: '/home', SEMLIA_DEEPSEEK_API_KEY: 'secret', SEMLIA_EXECUTION_SOURCES: 'private', DATABASE_URL: 'private', COMPOSE_FILE: 'private' });
  assert.equal(env.SEMLIA_V1_BROWSER_BASE_URL, 'http://127.0.0.1:52689');
  assert.equal(env.SEMLIA_V1_BROWSER_ROOT, '/owned/private');
  assert.equal(env.SEMLIA_DEEPSEEK_API_KEY, undefined);
  assert.equal(env.SEMLIA_EXECUTION_SOURCES, undefined);
  assert.equal(env.DATABASE_URL, undefined);
  assert.equal(env.COMPOSE_FILE, undefined);
  assert.throws(() => browserEnvironment('/owned/private', 0, {}));
});

test('natural correction creates an exact unsubmitted production update without changing spec or source pins', () => {
  const before = { definition: 'Synthetic baseline', spec: { kind: 'analysis_model', version: 1 } };
  const snapshots = [{ sourceId: 'source', snapshotId: 'snapshot', digest: 'digest', coverageKeys: ['tables'] }];
  const expected = { assetId: 'model', revisionId: 'r1', content: before, definition: 'Synthetic note', snapshots, evidence: [] };
  const operation = { summary: { progress: 'draft', frozen: false, releaseId: null }, activeValidation: { status: 'not_requested' }, input: { snapshots, evidence: [], candidates: [], dependencies: [] }, targets: [{ declaration: { intent: 'update', kind: 'semantic_asset', targetId: 'model', baseRevisionId: 'r1', content: { ...before, definition: 'Synthetic note' }, changes: [{ fieldPath: 'definition', op: 'update', beforeValue: before.definition, afterValue: 'Synthetic note' }] } }] };
  assertNaturalDraft(operation, expected);
  assert.throws(() => assertNaturalDraft({ ...operation, activeValidation: { status: 'queued' } }, expected));
  assert.throws(() => assertNaturalDraft(operation, { ...expected, revisionId: 'r2' }));
  const changedSpec = structuredClone(operation); changedSpec.targets[0].declaration.content.spec.version = 2;
  assert.throws(() => assertNaturalDraft(changedSpec, expected));
  assert.throws(() => assertNaturalDraft({ ...operation, input: { ...operation.input, candidates: [{ candidateId: 'consumed' }] } }, expected));
});

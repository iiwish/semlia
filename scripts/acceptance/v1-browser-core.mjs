import assert from 'node:assert/strict';

export function safeBrowserFailure(error, stage) {
  const stages = ['ownership', 'artifacts', 'before-counters', 'verifier-head', 'playwright', 'child-outcome', 'after-runtime', 'after-counters', 'head-proof', 'receipt', 'saved-evidence'];
  const codes = { 401: 'UNAUTHENTICATED', 403: 'FORBIDDEN', 429: 'RATE_LIMITED' };
  return { stage: stages.includes(stage) ? stage : 'unknown', code: Object.hasOwn(codes, error?.status) ? codes[error.status] : error?.code === 'EEXIST' ? 'OPERATION_LOCKED' : error?.code === 'RUNTIME_CONTROL_UNAVAILABLE' ? 'RUNTIME_CONTROL_UNAVAILABLE' : 'UNCLASSIFIED' };
}

export function browserChildOutcome(result) {
  const codes = ['ETIMEDOUT', 'ENOENT', 'EACCES', 'E2BIG', 'EINVAL'];
  return { status: result.status ?? null, signal: result.signal ?? null, errorCode: result.error ? codes.includes(result.error.code) ? result.error.code : 'UNCLASSIFIED' : null };
}

export function privateBrowserError(error, depth = 0) {
  const detail = { name: typeof error?.name === 'string' ? error.name : 'Error', message: typeof error?.message === 'string' ? error.message : String(error) };
  for (const key of ['stack', 'code']) if (typeof error?.[key] === 'string') detail[key] = error[key];
  if (error?.cause !== undefined && depth < 4) detail.cause = privateBrowserError(error.cause, depth + 1);
  return detail;
}

export function browserPlan(command) {
  const plans = {
    preflight: { grep: '@preflight' },
    'candidate-preflight': { grep: '@preflight', readOnly: true, candidate: true },
    visual: { grep: '@visual', readOnly: true },
    'result-visibility-check': { project: 'desktop', grep: '@result-visibility' },
    'result-visibility-check-consumer-readable-pins': { project: 'desktop', grep: '@result-visibility', repair: 'consumer-readable-pins' },
    desktop: { project: 'desktop', grep: '@journey' },
    'compact-desktop': { project: 'compact-desktop', grep: '@journey' },
    ambiguous: { project: 'desktop', grep: '@ambiguous' },
    'desktop-resume-route-query-optional': { project: 'desktop', grep: '@journey', repair: 'route-query-optional' },
  };
  assert.ok(Object.hasOwn(plans, command), 'Unapproved browser acceptance command');
  return plans[command];
}

export function selectBrowserRuntime(state, runtime, requireCandidate = false) {
  assert.equal(runtime.ready, true); assert.ok(['source', 'candidate'].includes(runtime.mode));
  assert.equal(runtime.api, `http://127.0.0.1:${state.apiPort}/`);
  if (requireCandidate) assert.equal(runtime.mode, 'candidate');
  const candidate = runtime.mode === 'candidate', port = candidate ? state.apiPort : state.webPort;
  assert.equal(runtime.url, `http://127.0.0.1:${port}/`); assert.equal(runtime.sourceWebAvailable, !candidate);
  if (candidate) {
    assert.equal(runtime.activeBinary?.schema, 33);
    for (const key of ['binaryDigest', 'manifestDigest']) assert.match(runtime.activeBinary?.[key] ?? '', /^sha256:[a-f0-9]{64}$/);
  }
  return { port, mode: runtime.mode, activeBinary: runtime.activeBinary };
}

export function assertBrowserRuntimeIdentity(before, after) {
  for (const key of ['mode', 'api', 'url', 'sourceWebAvailable']) assert.equal(after[key], before[key], 'Browser runtime identity changed');
  assert.equal(after.ready, true); assert.deepEqual(after.activeBinary, before.activeBinary, 'Browser binary identity changed');
}

export function assertConsumerEvidencePath(path) {
  assert.match(path, /^\/api\/v1\/workspaces\/wsp_[a-z0-9]+\/catalog\/assets\/ast_[a-z0-9]+(?:\/revisions\/rev_[a-z0-9]+)?$/);
}

export function assertOperationRoute(value, operationId, scope) {
  const url = new URL(value);
  assert.equal(url.pathname, `/work/operations/${operationId}`);
  assert.ok(value.split('?').length <= 2);
  assert.equal(url.searchParams.get('scope'), scope);
}

export function assertResultGeometry(sample) {
  assert.ok(sample.cells.length >= 2, 'Result header and value cells are required');
  for (const cell of sample.cells) {
    assert.ok(cell.width > 0 && cell.height > 0 && cell.x >= 0 && cell.y >= 0);
    assert.ok(cell.x + cell.width <= sample.viewport.width + 1 && cell.y + cell.height <= sample.viewport.height + 1, 'Result cell is clipped by viewport');
    assert.equal(cell.unobscured, true, 'Result cell fails hit testing');
    const composer = sample.composer;
    assert.ok(cell.y + cell.height <= composer.y + 1 || cell.y >= composer.y + composer.height || cell.x + cell.width <= composer.x || cell.x >= composer.x + composer.width, 'Result cell overlaps composer');
  }
}

export function browserEnvironment(root, webPort, inherited = process.env, runId = String(Date.now())) {
  assert.ok(Number.isInteger(webPort) && webPort > 1024 && webPort < 65536);
  assert.match(runId, /^\d{13}$/);
  return { PATH: inherited.PATH, HOME: inherited.HOME, SEMLIA_V1_BROWSER_ROOT: root, SEMLIA_V1_BROWSER_BASE_URL: `http://127.0.0.1:${webPort}`, SEMLIA_V1_BROWSER_RUN_ID: runId };
}

export function assertNaturalDraft(operation, expected) {
  assert.equal(operation.summary.progress, 'draft');
  assert.equal(operation.summary.frozen, false);
  assert.equal(operation.activeValidation.status, 'not_requested');
  assert.equal(operation.summary.releaseId, null);
  assert.equal(operation.targets.length, 1);
  const target = operation.targets[0].declaration;
  assert.equal(target.kind, 'semantic_asset'); assert.equal(target.intent, 'update');
  assert.equal(target.targetId, expected.assetId); assert.equal(target.baseRevisionId, expected.revisionId);
  assert.deepEqual(target.content, { ...expected.content, definition: expected.definition });
  assert.equal(target.changes.length, 1);
  const change = target.changes[0];
  assert.equal(change.fieldPath, 'definition'); assert.equal(change.op, 'update');
  assert.equal(change.beforeValue, expected.content.definition); assert.equal(change.afterValue, expected.definition);
  assert.deepEqual(operation.input.candidates, []);
  assert.deepEqual(operation.input.snapshots, expected.snapshots);
  assert.deepEqual(operation.input.evidence, expected.evidence);
}

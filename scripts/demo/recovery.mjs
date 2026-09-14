import assert from 'node:assert/strict';
import { fork } from 'node:child_process';
import { once } from 'node:events';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { preparePackage } from './package.mjs';
import { connectActor } from './session.mjs';
import { readSecrets } from './provision.mjs';
import { buildCorrection } from './correction.mjs';
import { checkpoint } from './state.mjs';

const root = fileURLToPath(new URL('../../.semlia/resume-demo/retail-v1/', import.meta.url));
const key = 'resume-retail-v1-interrupted-recovery-draft';

async function createDraft(state) {
  const workspace = state.workspaces.rehearsal;
  assert.ok(workspace.rollback, 'Recovery rehearsal requires the completed rollback');
  const { client, session } = await connectActor(root, state, 'rehearsal_author', readSecrets(root).accountPassword);
  assert.equal(session.workspaces.length, 1);
  assert.equal(session.workspaces[0].id, workspace.id);
  assert.equal(session.workspaces[0].principalId, workspace.principals.author);
  const base = `/api/v1/workspaces/${workspace.id}/production-operations`;
  const baseline = await client.request('GET', base + '/' + workspace.baseline.attribution.operationId);
  const payload = buildCorrection(workspace, baseline);
  const result = await client.request('POST', base, payload, key);
  const draft = await client.request('GET', base + '/' + result.operationId);
  assert.equal(draft.summary.progress, 'draft');
  assert.equal(draft.activeValidation.status, 'not_requested');
  assert.equal(draft.baselineHead.releaseId, workspace.rollback.id);
  assert.deepEqual(draft.targets.map(x => x.declaration), payload.targets);
  return { operationId: result.operationId, version: draft.version, setDigest: draft.setDigest };
}

async function main() {
  process.umask(0o077);
  const state = preparePackage(root);
  if (process.argv[2] === '--child' && process.send) {
    const receipt = await createDraft(state);
    // The parent terminates this owned process after server commit, before checkpoint.
    process.send(receipt);
    setInterval(() => {}, 1000);
    return;
  }
  assert.equal(process.argv.length, 2);
  if (state.workspaces.rehearsal.recoveryDraft) {
    assert.deepEqual(await createDraft(state), state.workspaces.rehearsal.recoveryDraft);
    console.log('Recovery replay verified; no new draft');
    return;
  }
  const child = fork(fileURLToPath(import.meta.url), ['--child'], { stdio: ['ignore', 'ignore', 'ignore', 'ipc'] });
  const closed = once(child, 'exit');
  let interrupted;
  try {
    interrupted = await Promise.race([
      once(child, 'message').then(([receipt]) => receipt),
      closed.then(() => { throw new Error('Recovery child exited before commit confirmation'); }),
      new Promise((_, reject) => { child.recoveryTimer = setTimeout(() => reject(new Error('Recovery child timeout')), 60000); }),
    ]);
  } finally {
    clearTimeout(child.recoveryTimer);
    child.kill('SIGTERM');
    await closed;
  }
  assert.equal(preparePackage(root).revision, state.revision, 'Interrupted child must not checkpoint');
  const recovered = await createDraft(state);
  assert.deepEqual(recovered, interrupted, 'Same idempotency key must recover the committed operation');
  checkpoint(root, state, { workspaces: { ...state.workspaces, rehearsal: { ...state.workspaces.rehearsal, recoveryDraft: recovered } } });
  console.log('Interrupted after server commit; resumed the same draft without a duplicate');
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(() => { console.error('Recovery verification failed; existing server state preserved'); process.exitCode = 1; });
}

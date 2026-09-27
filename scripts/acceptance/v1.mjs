import assert from 'node:assert/strict';
import { existsSync, mkdirSync, openSync, closeSync, lstatSync } from 'node:fs';
import { join } from 'node:path';
import { checkpoint, digest, fixtureDigest, verifyFixture, openState, readState, readPrivate, writePrivate, rejectLinks, safeFailureMessage } from './v1/core.mjs';
import { provision, seedSource, run, preservationSnapshot, verifyInventory, verifyProvisioned, hardenOwnedReader } from './v1/provision.mjs';
import { repo, root, up, down, status } from './v1/runtime.mjs';
import { initializeActors, initializeSource, initializeModel, initializeKnowledge, verify } from './v1/initialize.mjs';
import { verifyCandidate } from './v1/release-candidate.mjs';
import { unlinkOwned } from '../demo/runtime.mjs';

process.umask(0o077);
const command = process.argv[2];
let fd, lock, lockIdentity;
function acquireOperationLock() {
  lock = join(root, 'operation.lock'); rejectLinks(lock); fd = openSync(lock, 'wx', 0o600); lockIdentity = lstatSync(lock);
}
try {
  const candidateCommand = ['candidate-check', 'up-candidate'].includes(command);
  if (process.argv.length !== (candidateCommand ? 7 : 3) || !['init', 'up', 'down', 'status', 'verify', 'harden-reader', 'candidate-check', 'up-candidate'].includes(command)) throw new Error('Unknown V1 lifecycle command');
  const parent = join(repo, '.semlia/v1-acceptance'); rejectLinks(parent);
  if (!existsSync(parent)) mkdirSync(parent, { recursive: true, mode: 0o700 });
  if (command === 'init') {
    const generated = JSON.parse(run(repo, 'go', ['run', './cmd/v1-acceptance']));
    let state = existsSync(root) ? readState(root) : openState(root, fixtureDigest(generated));
    acquireOperationLock();
    if (!existsSync(join(root, 'fixture.json'))) {
      if (state.database || state.fixtureDigest || state.operations) throw new Error('Owned fixture is missing; refusing template adoption');
      writePrivate(join(root, 'fixture.json'), generated, true);
    }
    const fixture = readPrivate(join(root, 'fixture.json'));
    const fingerprint = verifyFixture(state, fixture, generated);
    if (!state.fixtureDigest) {
      const legacy = digest('v1-acceptance/1\n' + generated.DataSQL);
      assert.ok(state.digest === fingerprint || state.digest === legacy, 'Unknown fixture ownership digest');
      if (state.database) verifyProvisioned(repo, state);
      state = checkpoint(root, state, { fixtureDigest: fingerprint, ...(state.digest === legacy ? { fixtureFingerprintUpgrade: { checkedAt: new Date().toISOString(), savedMatchesCurrent: true, originalOwnershipDigest: state.digest, canonicalFixtureDigest: fingerprint } } : {}) });
    }
    state = provision(repo, root, state);
    state = seedSource(repo, root, state, fixture);
    await up();
    await initializeActors(root);
    await initializeSource(root);
    await initializeModel(root);
    await initializeKnowledge(root, fixture);
    const runtime = await status();
    if (!runtime.executionConfigured) { await down(); await up(); }
    const report = await verify(root);
    state = readState(root);
    assert.deepEqual(preservationSnapshot(repo, verifyInventory(repo, state)), state.preservation, 'Existing environment changed; inspect before claiming preservation');
    checkpoint(root, state, { complete: true });
    console.log(JSON.stringify({ ...await status(), ...report, privateReceipt: root }));
  } else if (command === 'harden-reader') {
    const state = readState(root);
    acquireOperationLock();
    hardenOwnedReader(repo, root, state);
    console.log('Owned V1 reader privacy settings verified; no global database settings changed.');
  } else if (command === 'status') console.log(JSON.stringify(await status()));
  else if (command === 'candidate-check') {
    verifyProvisioned(repo, readState(root));
    console.log(JSON.stringify(verifyCandidate(repo, process.argv.slice(3)).identity));
  } else if (['up', 'down', 'up-candidate'].includes(command)) {
    readState(root); acquireOperationLock();
    if (command === 'down') { await down(); console.log('V1 owned services stopped; all databases preserved.'); }
    else console.log(JSON.stringify(await up(command === 'up-candidate' ? process.argv.slice(3) : undefined)));
  }
  else {
    const state = readState(root);
    assert.deepEqual(preservationSnapshot(repo, verifyInventory(repo, state)), state.preservation);
    console.log(JSON.stringify(await verify(root)));
  }
} catch (error) {
  console.error(safeFailureMessage(error));
  process.exitCode = 1;
} finally { if (fd !== undefined) { closeSync(fd); unlinkOwned(lock, lockIdentity); } }

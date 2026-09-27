import { openSync, closeSync, lstatSync } from 'node:fs';
import { join } from 'node:path';
import { root } from './v1/runtime.mjs';
import { readState, safeFailureMessage, rejectLinks } from './v1/core.mjs';
import { unlinkOwned } from '../demo/runtime.mjs';
import { planDrill, provisionTargets, installUpgrade } from './v1/release-environment.mjs';
import { channels, verifyRevokedChannels } from './v1/release-channels.mjs';
import { prepareArtifacts, backupRestore } from './v1/release-restore.mjs';

process.umask(0o077);
let lockFd, lockIdentity;
const lock = join(root, 'operation.lock');
try {
  if (process.argv.length !== 3 || !['plan', 'provision', 'install-upgrade', 'install-upgrade-resume', 'channels', 'channels-resume', 'revocation-check', 'artifact-fixture', 'artifact-markdown', 'backup-restore'].includes(process.argv[2])) throw new Error('Unknown release acceptance command');
  readState(root); rejectLinks(lock); lockFd = openSync(lock, 'wx', 0o600); lockIdentity = lstatSync(lock);
  const actions = { plan: planDrill, provision: provisionTargets, 'install-upgrade': () => installUpgrade(), 'install-upgrade-resume': () => installUpgrade(true), channels: () => channels(), 'channels-resume': () => channels(true), 'revocation-check': verifyRevokedChannels, 'artifact-fixture': () => prepareArtifacts(), 'artifact-markdown': () => prepareArtifacts(true), 'backup-restore': backupRestore };
  const result = await actions[process.argv[2]]();
  console.log(JSON.stringify(process.argv[2] === 'plan' ? { owner: result.owner, targets: result.targets, databaseContainer: result.database.container, sourceOwner: result.sourceOwner } : result));
} catch (error) { console.error(safeFailureMessage(error)); process.exitCode = 1; }
finally { if (lockFd !== undefined) { closeSync(lockFd); unlinkOwned(lock, lockIdentity); } }

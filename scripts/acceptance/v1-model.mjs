import { openSync, closeSync, lstatSync } from 'node:fs';
import { join } from 'node:path';
import { root } from './v1/runtime.mjs';
import { modelBaseline, replayFixedExecutions, reverifyExecutionReceipts, replayClaimedRequests, auditSavedWindows, caseProven } from './v1/model.mjs';
import { governanceRehearsal, governanceHeadProof } from './v1/correction.mjs';
import { safeFailureMessage, readState, rejectLinks } from './v1/core.mjs';
import { finalCandidate } from './v1/final-candidate.mjs';
import { prepareValues, reconcileValues } from './v1/value-knowledge.mjs';
import { unlinkOwned } from '../demo/runtime.mjs';

process.umask(0o077);
let fd, lockIdentity;
const command = process.argv[2];
const lock = join(root, 'operation.lock');
try {
  if (process.argv.length !== 3 || !['baseline', 'retry', 'round-16384', 'round-16384-retry', 'round-half-open', 'round-half-open-retry', 'execute-replay', 'execute-compiler-replay', 'reverify-receipts', 'audit-windows', 'replay', 'governance', 'final-candidate', 'prepare-values', 'reconcile-values'].includes(command)) throw new Error('Unknown model acceptance command');
  readState(root); rejectLinks(lock);
  fd = openSync(lock, 'wx', 0o600); lockIdentity = lstatSync(lock);
  if (command === 'final-candidate') { if (!(await finalCandidate(root)).summary.passed) process.exitCode = 1; }
  else if (command === 'prepare-values') { if (!(await prepareValues(root)).complete) process.exitCode = 1; }
  else if (command === 'reconcile-values') { if (!(await reconcileValues(root)).complete) process.exitCode = 1; }
  else if (command === 'governance') { if (!(await governanceRehearsal(root)).complete) process.exitCode = 1; }
  else if (command === 'audit-windows') auditSavedWindows(root);
  else if (command === 'replay') { if ((await replayClaimedRequests(root)).claimReplays.some(x => !x.passed)) process.exitCode = 1; }
  else if (command === 'reverify-receipts') reverifyExecutionReceipts(root);
  else if (command === 'execute-replay' || command === 'execute-compiler-replay') { if ((await replayFixedExecutions(root, command === 'execute-compiler-replay')).executionReplays.some(x => !x.passed && !x.comparisonReview?.passed)) process.exitCode = 1; }
  else {
    const ledger = await modelBaseline(root, ['retry', 'round-16384-retry', 'round-half-open-retry'].includes(command), command.startsWith('round-half-open') ? 'ask-half-open-time-window' : command.startsWith('round-16384') ? 'ask-budget-16384-safe-diagnostics' : 'initial');
    if (['total', 'region', 'monthly', 'old-customer-average', 'denied', 'ambiguous'].some(id => !caseProven(ledger, id))) process.exitCode = 1;
  }
} catch (error) { console.error(safeFailureMessage(error)); process.exitCode = 1; }
finally {
  if (fd !== undefined) {
    if (command === 'governance') {
      try {
        const proof = await governanceHeadProof(root); console.log(JSON.stringify({ headProof: proof }));
        if (!proof.correctBaselineContent) process.exitCode = 1;
      } catch { console.error('V1 governance head is unverified; do not claim restoration.'); process.exitCode = 1; }
    }
    closeSync(fd); unlinkOwned(lock, lockIdentity);
  }
}

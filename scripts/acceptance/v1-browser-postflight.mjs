import assert from 'node:assert/strict';
import { openSync, closeSync, unlinkSync } from 'node:fs';
import { join } from 'node:path';
import { root, status } from './v1/runtime.mjs';
import { readPrivate, writePrivate } from './v1/core.mjs';
import { counters } from './v1/model.mjs';
import { governanceHeadProof } from './v1/correction.mjs';
import { safeBrowserFailure } from './v1-browser-core.mjs';

process.umask(0o077);
let lockFd, stage = 'ownership';
const lock = join(root, 'operation.lock');
try {
  assert.equal(process.argv.length, 3);
  const runId = process.argv[2]; assert.match(runId, /^\d{13}$/);
  await status(); lockFd = openSync(lock, 'wx', 0o600);
  stage = 'saved-evidence';
  const browser = join(root, 'browser'), output = join(browser, runId);
  const record = readPrivate(join(browser, 'result-visibility-check-consumer-readable-pins.json'));
  assert.equal(record.runId, runId); assert.equal(record.passed, true); assert.equal(record.contextReleased, true);
  const test = readPrivate(join(output, 'playwright', '.last-run.json'));
  assert.equal(test.status, 'passed'); assert.deepEqual(test.failedTests, []);
  const beforeHead = readPrivate(join(output, 'verifier-head.json'));
  stage = 'after-counters'; const after = counters(root);
  assert.deepEqual(after, record.after); assert.equal(after.modelSteps - record.before.modelSteps, 1); assert.equal(after.executions - record.before.executions, 1); assert.equal(after.unfinished, 0);
  stage = 'head-proof'; const head = await governanceHeadProof(root);
  assert.equal(head.correctBaselineContent, true); assert.equal(head.currentRelease, beforeHead.currentRelease); assert.equal(head.currentRevision, beforeHead.currentRevision);
  stage = 'receipt';
  const receipt = { runId, postflightOnly: true, originalLauncherExit: 1, originalLauncherError: 'UNCLASSIFIED', playwrightStatus: test.status, head, after, modelDelta: 1, executionDelta: 1, checkedAt: new Date().toISOString() };
  writePrivate(join(output, 'postflight-only.json'), receipt);
  console.log(JSON.stringify(receipt));
} catch (error) { console.error(JSON.stringify(safeBrowserFailure(error, stage))); process.exitCode = 1; }
finally { if (lockFd !== undefined) { closeSync(lockFd); unlinkSync(lock); } }

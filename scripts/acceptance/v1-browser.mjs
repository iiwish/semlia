import { openSync, closeSync, lstatSync, mkdirSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
import { root as ownedRoot, repo as repository, status as runtimeStatus } from './v1/runtime.mjs';
import { readState, writePrivate, safeFailureMessage, rejectLinks } from './v1/core.mjs';
import { counters as runtimeCounters } from './v1/model.mjs';
import { governanceHeadProof as runtimeHeadProof } from './v1/correction.mjs';
import { browserPlan, browserEnvironment, safeBrowserFailure, selectBrowserRuntime, assertBrowserRuntimeIdentity, browserChildOutcome, privateBrowserError } from './v1-browser-core.mjs';
import { unlinkOwned } from '../demo/runtime.mjs';

export async function runBrowserAcceptance(command, { root = ownedRoot, repo = repository, status = runtimeStatus, counters = runtimeCounters, governanceHeadProof = runtimeHeadProof, spawnBrowser = spawnSync, print = console.log, printError = console.error } = {}) {
  process.umask(0o077);
  let exitCode = 0;
  let lockFd, lockIdentity, saveRun;
  let stage = 'ownership';
  const lock = join(root, 'operation.lock');
  try {
    const plan = browserPlan(command), state = readState(root);
    rejectLinks(lock); lockFd = openSync(lock, 'wx', 0o600); lockIdentity = lstatSync(lock);
    const runtimeBefore = await status(), runtime = selectBrowserRuntime(state, runtimeBefore, plan.candidate);
    stage = 'artifacts';
    const output = join(root, 'browser'); rejectLinks(output);
    if (!existsSync(output)) mkdirSync(output, { mode: 0o700 });
    const runId = String(Date.now()), runDirectory = join(output, runId);
    mkdirSync(runDirectory, { mode: 0o700 });
    const identities = [root, output, runDirectory].map(path => ({ path, identity: lstatSync(path) }));
    const verifyEvidence = () => {
      for (const { path, identity } of identities) {
        rejectLinks(path);
        const current = lstatSync(path);
        if (!current.isDirectory() || current.uid !== process.getuid() || (current.mode & 0o777) !== 0o700 || current.dev !== identity.dev || current.ino !== identity.ino) throw new Error('Browser evidence ownership changed');
      }
    };
    saveRun = (name, value) => { verifyEvidence(); writePrivate(join(runDirectory, name), value, true); };
    saveRun('runtime-before.json', runtimeBefore);
    stage = 'before-counters'; const before = counters(root);
    saveRun('counters-before.json', before);
    let verifierHead;
    if (plan.grep === '@result-visibility') {
      stage = 'verifier-head';
      verifierHead = await governanceHeadProof(root);
      if (!verifierHead.correctBaselineContent) throw new Error('Visual verifier requires correct current head');
      saveRun('verifier-head.json', verifierHead);
    }
    verifyEvidence();
    const logPath = join(output, `driver-${command}-${runId}.log`), fd = openSync(logPath, 'wx', 0o600);
    let result;
    try {
      stage = 'playwright';
      const args = ['--dir', 'web', 'exec', 'playwright', 'test', '--config', 'playwright.v1.config.ts', '--grep', plan.grep];
      if (plan.project) args.push('--project', plan.project);
      const env = browserEnvironment(root, runtime.port, process.env, runId);
      if (plan.repair) env.SEMLIA_V1_BROWSER_REPAIR = plan.repair;
      try { result = spawnBrowser('pnpm', args, { cwd: repo, env, stdio: ['ignore', fd, fd], timeout: 900000 }); }
      catch (error) { result = { status: null, signal: null, error }; }
    } finally { closeSync(fd); }
    stage = 'child-outcome'; const child = browserChildOutcome(result);
    saveRun('child-outcome.json', child);
    if (result.error) saveRun('child-error-private.json', privateBrowserError(result.error));
    stage = 'after-runtime'; const runtimeAfter = await status();
    saveRun('runtime-after.json', runtimeAfter);
    selectBrowserRuntime(state, runtimeAfter, plan.candidate); assertBrowserRuntimeIdentity(runtimeBefore, runtimeAfter);
    stage = 'after-counters'; const after = counters(root);
    saveRun('counters-after.json', after);
    stage = 'head-proof'; const head = await governanceHeadProof(root);
    saveRun('head-proof.json', head);
    stage = 'receipt';
    const receipt = { command, status: result.status, child, runtimeBefore, runtimeAfter, before, after, modelSteps: after.modelSteps - before.modelSteps, executions: after.executions - before.executions, correctBaselineContent: head.correctBaselineContent, currentRelease: head.currentRelease, logPath, checkedAt: new Date().toISOString() };
    saveRun('receipt.json', receipt);
    verifyEvidence(); writePrivate(join(output, `driver-${command}-receipt.json`), receipt);
    print(JSON.stringify({ command, status: result.status, modelSteps: receipt.modelSteps, executions: receipt.executions, correctBaselineContent: head.correctBaselineContent, currentRelease: head.currentRelease, privateLog: logPath }));
    if (result.error || result.signal || result.status !== 0 || !head.correctBaselineContent || verifierHead && verifierHead.currentRelease !== head.currentRelease || (command === 'preflight' || plan.readOnly) && (receipt.modelSteps || receipt.executions || after.agentRuns !== before.agentRuns)) exitCode = 1;
  } catch (error) {
    const failure = safeBrowserFailure(error, stage);
    let evidenceSaved = false;
    if (saveRun) {
      try {
        saveRun('failure-private.json', privateBrowserError(error));
        saveRun('failure.json', { ...failure, checkedAt: new Date().toISOString() });
        evidenceSaved = true;
      } catch { /* Do not write through a replaced or untrusted evidence path. */ }
    }
    printError(JSON.stringify({ message: safeFailureMessage(error), ...failure, evidenceSaved })); exitCode = 1;
  }
  finally { if (lockFd !== undefined) { closeSync(lockFd); unlinkOwned(lock, lockIdentity); } }
  return exitCode;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  process.exitCode = await runBrowserAcceptance(process.argv.length === 3 ? process.argv[2] : undefined);
}

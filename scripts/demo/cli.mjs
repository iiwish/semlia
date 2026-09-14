import { mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { preparePackage } from './package.mjs';
import { provision } from './provision.mjs';
import { seedSource } from './source.mjs';
import { runtimeUp, runtimeDown, runtimeStatus } from './runtime.mjs';
import { initializeSources } from './initialize.mjs';
import { initializeDiscovery } from './discovery.mjs';
import { initializeBaseline } from './production.mjs';
import { initializeCorrection, initializeCorrectionDraft } from './correction.mjs';
import { verifyDemo } from './verify.mjs';

process.umask(0o077);
const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const parent = resolve(repo, '.semlia/resume-demo');
const root = resolve(parent, 'retail-v1');
const command = process.argv[2];
try {
  if (process.argv.length !== 3 || !['generate', 'provision', 'seed-source', 'up', 'down', 'status', 'initialize-sources', 'discover', 'baseline', 'correction', 'draft', 'verify'].includes(command)) throw new Error('Usage: node scripts/demo/cli.mjs generate|provision|seed-source|up|down|status|initialize-sources|discover|baseline|correction|draft|verify');
  mkdirSync(parent, { recursive: true, mode: 0o700 });
  let state = preparePackage(root);
  if (state.singleAdmin && ['initialize-sources', 'discover', 'baseline', 'correction', 'draft'].includes(command)) throw new Error('Single-admin showcase: legacy multi-account rehearsal commands are disabled');
  if (['provision', 'seed-source', 'up'].includes(command)) state = provision(repo, root, state);
  const source = command === 'seed-source' ? seedSource(root, state) : undefined;
  const runtime = command === 'up' ? await runtimeUp() : command === 'down' ? await runtimeDown() : command === 'status' ? await runtimeStatus() : undefined;
  if (command === 'initialize-sources') { await runtimeStatus(); state = await initializeSources(root, state); }
  if (command === 'discover') { await runtimeStatus(); state = await initializeDiscovery(root, state); }
  if (command === 'baseline') { await runtimeStatus(); state = await initializeBaseline(root, state); }
  if (command === 'correction') { await runtimeStatus(); state = await initializeCorrection(root, state); }
  if (command === 'draft') { await runtimeStatus(); state = await initializeCorrectionDraft(root, state); }
  const verification = command === 'verify' ? await verifyDemo(root, state) : undefined;
  const workspaces = state.workspaces && Object.fromEntries(Object.entries(state.workspaces).map(([slug, workspace]) => [slug, { id: workspace.id, sourceId: workspace.sourceId, artifactSetId: workspace.artifactSetId, discoveryRunId: workspace.discovery?.runId, baselineReleaseId: workspace.baseline?.id, correctionReleaseId: workspace.correction?.id, rollbackReleaseId: workspace.rollback?.id }]));
  console.log(JSON.stringify({ directory: root, digest: state.digest, synthetic: true, source, runtime, workspaces, verification }));
} catch (error) {
  console.error(error.code ? `Demo command failed (${error.code})` : error.message);
  process.exitCode = 1;
}

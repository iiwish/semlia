import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { existsSync, lstatSync, mkdirSync, openSync, closeSync } from 'node:fs';
import { createServer, request } from 'node:http';
import { join, resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { availablePort, unlinkOwned } from '../../demo/runtime.mjs';
import { checkpoint, readState, readPrivate, writePrivate, runtimeEnvironment, rejectLinks, digest } from './core.mjs';
import { verifyProvisioned, run } from './provision.mjs';
import { verifyCandidate, installCandidate, regularFile, verifyMigrationInventory, withStartupLock, assertRequestedRuntime } from './release-candidate.mjs';

export const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
export const root = join(repo, '.semlia/v1-acceptance/current');
const ownerPath = join(root, 'runtime-owner.json');
const socketPath = join(root, 'control.sock');
const binary = join(root, 'server');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
export function environment(state = readState(root), launch) {
  const env = runtimeEnvironment(repo, root, state, readPrivate(join(root, 'secrets.json')));
  if (launch?.mode === 'candidate') env.SEMLIA_MIGRATIONS_PATH = launch.migrationsPath;
  return env;
}

async function control(action) {
  const state = readState(root);
  const owner = readPrivate(ownerPath);
  if (owner.owner !== state.owner || !/^[a-f0-9]{64}$/.test(owner.token)) throw new Error('Runtime ownership mismatch');
  await requestRuntimeControl(socketPath, action, owner.token);
  return owner;
}
export function requestRuntimeControl(socketPath, action, token) {
  return new Promise((resolve, reject) => {
    const req = request({ socketPath, path: `/${action}`, method: 'POST', agent: false, headers: { authorization: token }, timeout: 3000 }, res => {
      res.resume(); res.once('end', () => res.statusCode === 200 ? resolve() : reject(new Error('Runtime control refused')));
    });
    req.once('error', error => reject(Object.assign(new Error('Owned runtime control unavailable', { cause: error }), { code: 'RUNTIME_CONTROL_UNAVAILABLE' })));
    req.once('timeout', () => req.destroy()); req.end();
  });
}
export function fetchRuntime(url) {
  // The browser driver blocks in a synchronous child; do not retain idle sockets.
  return fetch(url, { redirect: 'error', headers: { connection: 'close' }, signal: AbortSignal.timeout(3000) });
}
export async function status() {
  const state = readState(root); verifyProvisioned(repo, state);
  const owner = await control('status');
  const mode = owner.mode ?? 'source';
  if (owner.binaryDigest) {
    const launch = readPrivate(join(root, 'runtime-launch.json'));
    if (launch.owner !== state.owner || launch.mode !== mode || launch.binaryDigest !== owner.binaryDigest || launch.manifestDigest !== owner.manifestDigest || digest(regularFile(binary)) !== owner.binaryDigest) throw new Error('Active runtime identity changed');
  } else if (mode === 'candidate') throw new Error('Candidate identity is missing');
  const api = `http://127.0.0.1:${state.apiPort}/`;
  const apiReady = await fetchRuntime(api + 'health/ready');
  await apiReady.arrayBuffer();
  if (!apiReady.ok) throw new Error('Owned backend readiness failed');
  const url = mode === 'candidate' ? api : `http://127.0.0.1:${state.webPort}/`;
  const response = await fetchRuntime(url + 'health/ready');
  await response.arrayBuffer();
  if (!response.ok) throw new Error('V1 runtime readiness failed');
  if (mode === 'candidate') {
    const embedded = await fetchRuntime(url);
    if (!embedded.ok || !embedded.headers.get('content-type')?.includes('text/html') || !(await embedded.text()).includes('<div id="root"')) throw new Error('Candidate embedded frontend is unavailable');
  }
  return { url, api, mode, sourceWebAvailable: mode === 'source', ready: true, executionConfigured: owner.executionConfigured, activeBinary: owner.binaryDigest ? { binaryDigest: owner.binaryDigest, manifestDigest: owner.manifestDigest, schema: owner.schema } : { verification: 'legacy-source-runtime' } };
}
export async function down() {
  readState(root);
  if (!existsSync(ownerPath)) return;
  await control('stop');
  for (let i = 0; i < 160 && existsSync(ownerPath); i++) await sleep(100);
  if (existsSync(ownerPath)) throw new Error('V1 runtime shutdown incomplete; ownership preserved');
}
export async function up(candidatePaths) {
  let state = readState(root); verifyProvisioned(repo, state);
  const candidate = candidatePaths ? verifyCandidate(repo, candidatePaths) : null;
  const mode = candidate ? 'candidate' : 'source';
  if (existsSync(ownerPath)) {
    const owner = readPrivate(ownerPath);
    if ((owner.mode ?? 'source') !== mode || candidate && (owner.manifestDigest !== candidate.identity.manifestDigest || owner.binaryDigest !== candidate.identity.binaryDigest)) throw new Error('Requested runtime differs from active owned runtime; stop it explicitly after validation');
    return assertRequestedRuntime(await status(), mode, candidate?.identity);
  }
  return withStartupLock(root, async () => {
  if (existsSync(ownerPath)) return assertRequestedRuntime(await status(), mode, candidate?.identity);
  if (existsSync(socketPath)) throw new Error('Unowned socket; refusing takeover');
  if (!state.apiPort) {
    const apiPort = await availablePort(); let webPort;
    do { webPort = await availablePort(); } while (webPort === apiPort);
    state = checkpoint(root, state, { apiPort, webPort });
  }
  await availablePort(state.apiPort); if (mode === 'source') await availablePort(state.webPort);
  for (const name of ['content', 'inputs', 'artifacts', 'sessions']) {
    const path = join(root, name); rejectLinks(path);
    if (!existsSync(path)) mkdirSync(path, { mode: 0o700 });
    if (!lstatSync(path).isDirectory() || (lstatSync(path).mode & 0o077)) throw new Error('Unsafe runtime directory');
  }
  let launch;
  if (candidate) launch = installCandidate(repo, root, candidate);
  else {
    rejectLinks(binary);
    const buildEnv = environment(state);
    run(repo, 'go', ['build', '-o', binary, './cmd/semlia'], { env: buildEnv });
    launch = { owner: state.owner, mode: 'source', binaryDigest: digest(regularFile(binary)), schema: 33 };
    writePrivate(join(root, 'runtime-launch.json'), launch);
  }
  const env = environment(state, launch);
  run(repo, binary, ['migrate', 'up'], { env });
  run(repo, binary, ['check-schema'], { env });
  run(repo, binary, ['bootstrap-local-admin', state.owner.replaceAll('_', '-'), 'V1 合成验收', 'v1_admin'], { env, input: readPrivate(join(root, 'secrets.json')).accountPassword + '\n' });
  const fd = openSync(join(root, 'supervisor.log'), 'a', 0o600);
  const child = spawn(process.execPath, [fileURLToPath(import.meta.url), 'supervise'], { cwd: repo, env: { PATH: process.env.PATH, HOME: process.env.HOME }, detached: true, stdio: ['ignore', fd, fd] });
  closeSync(fd); child.unref();
  let failed = false; child.once('error', () => { failed = true; }); child.once('exit', () => { failed = true; });
  for (let i = 0; i < 120; i++) {
    await sleep(500);
    if (failed) throw new Error('V1 supervisor exited before ready');
    let ready;
    try { ready = await status(); } catch { /* Readiness requires all owned children and HTTP. */ }
    if (ready) return assertRequestedRuntime(ready, mode, launch.mode === 'candidate' ? launch : undefined);
  }
  if (existsSync(ownerPath)) await down();
  throw new Error('V1 startup timeout; inspect private logs without disclosing configuration');
  });
}
async function supervise() {
  process.umask(0o077);
  const state = readState(root); verifyProvisioned(repo, state);
  const launch = readPrivate(join(root, 'runtime-launch.json'));
  if (launch.owner !== state.owner || !['source', 'candidate'].includes(launch.mode) || digest(regularFile(binary)) !== launch.binaryDigest || launch.schema !== 33) throw new Error('Runtime launch identity is invalid');
  if (launch.mode === 'candidate') verifyMigrationInventory(launch.migrationsPath, launch.migrations);
  const env = environment(state, launch), token = randomBytes(32).toString('hex');
  await availablePort(state.apiPort); if (launch.mode === 'source') await availablePort(state.webPort);
  writePrivate(ownerPath, { owner: state.owner, pid: process.pid, token, executionConfigured: !!state.sourceId, mode: launch.mode, binaryDigest: launch.binaryDigest, manifestDigest: launch.manifestDigest, schema: launch.schema }, true);
  const ownerIdentity = lstatSync(ownerPath); let socketIdentity, timer, stopping = false;
  const children = new Set();
  const server = createServer((req, res) => {
    if (req.method !== 'POST' || req.headers.authorization !== token) { res.writeHead(403).end(); return; }
    if (req.url === '/status') { res.writeHead(!stopping && children.size === (launch.mode === 'candidate' ? 2 : 3) ? 200 : 503).end(); return; }
    if (req.url !== '/stop') { res.writeHead(404).end(); return; }
    res.end(); stop();
  });
  function cleanup() { clearTimeout(timer); server.close(); unlinkOwned(socketPath, socketIdentity); unlinkOwned(ownerPath, ownerIdentity); }
  function signal(child, kind) { try { process.kill(-child.pid, kind); } catch { /* Already stopped. */ } }
  function stop() {
    if (stopping) return; stopping = true;
    for (const child of children) signal(child, 'SIGTERM');
    timer = setTimeout(() => { for (const child of children) signal(child, 'SIGKILL'); }, 12000);
    if (!children.size) cleanup();
  }
  process.once('SIGTERM', stop); process.once('SIGINT', stop);
  try {
    await new Promise((resolve, reject) => { server.once('error', reject); server.listen(socketPath, resolve); });
    socketIdentity = lstatSync(socketPath);
    const programs = [['server', binary, ['server']], ['worker', binary, ['worker']]];
    if (launch.mode === 'source') programs.push(['web', 'pnpm', ['--dir', 'web', 'exec', 'vite', '--host', '127.0.0.1', '--port', String(state.webPort), '--strictPort']]);
    for (const [name, program, args] of programs) {
      if (program === binary && digest(regularFile(binary)) !== launch.binaryDigest) throw new Error('Runtime binary changed before execution');
      const fd = openSync(join(root, name + '.log'), 'a', 0o600);
      const child = spawn(program, args, { cwd: repo, env, detached: true, stdio: ['ignore', fd, fd] });
      closeSync(fd); children.add(child);
      child.once('error', stop);
      child.once('close', () => { children.delete(child); if (!stopping) stop(); else if (!children.size) cleanup(); });
    }
  } catch { stop(); throw new Error('V1 supervisor failed'); }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv[2] !== 'supervise' || process.argv.length !== 3) throw new Error('Internal V1 runtime entry only');
  supervise().catch(() => { console.error('V1 supervisor failed'); process.exitCode = 1; });
}

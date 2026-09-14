import { spawn, spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { closeSync, existsSync, lstatSync, mkdirSync, openSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs';
import { createServer, request } from 'node:http';
import { createServer as tcpServer } from 'node:net';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { preparePackage } from './package.mjs';
import { inventory, readSecrets } from './provision.mjs';
import { runtimeEnvironment } from './environment.mjs';
import { checkpoint } from './state.mjs';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const root = join(repo, '.semlia/resume-demo/retail-v1');
const ownerPath = join(root, 'runtime-owner.json');
const socketPath = join(root, 'control.sock');
const binary = join(root, 'server');
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

export function unlinkOwned(path, identity) {
  if (!identity || !existsSync(path)) return;
  const actual = lstatSync(path);
  if (actual.dev === identity.dev && actual.ino === identity.ino) unlinkSync(path);
}

export function availablePort(port = 0) {
  return new Promise((resolve, reject) => {
    const server = tcpServer();
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => { const value = server.address().port; server.close(() => resolve(value)); });
  });
}
function current() {
  const state = preparePackage(root);
  const actual = inventory(repo);
  if (actual.container !== state.database?.container || actual.port !== state.database?.port) throw new Error('Runtime database ownership mismatch');
  return state;
}
function environment(state) { return runtimeEnvironment(repo, root, { ...state.database, owner: state.owner, apiPort: state.apiPort }, readSecrets(root)); }
function run(program, args, env, input) {
  const result = spawnSync(program, args, { cwd: repo, env, input, encoding: 'utf8', timeout: 180000, maxBuffer: 8 * 1024 * 1024 });
  if (result.status !== 0 || result.error) throw new Error(`Demo ${args[0]} command failed; protected diagnostics withheld`);
}
async function control(action) {
  if (lstatSync(ownerPath).isSymbolicLink() || (lstatSync(ownerPath).mode & 0o077)) throw new Error('Unsafe runtime ownership file');
  const owner = JSON.parse(readFileSync(ownerPath, 'utf8'));
  if (owner.owner !== current().owner || !/^[a-f0-9]{64}$/.test(owner.token)) throw new Error('Runtime owner mismatch');
  return new Promise((resolve, reject) => {
    const req = request({ socketPath, path: `/${action}`, method: 'POST', headers: { authorization: owner.token }, timeout: 3000 }, res => {
      res.resume(); res.once('end', () => res.statusCode === 200 ? resolve() : reject(new Error('Runtime control refused')));
    });
    req.once('error', reject); req.once('timeout', () => req.destroy(new Error('Runtime control timeout'))); req.end();
  });
}
export async function runtimeStatus() {
  const state = current();
  await control('status');
  const response = await fetch(`http://127.0.0.1:${state.apiPort}/health/ready`, { redirect: 'error', signal: AbortSignal.timeout(3000) });
  if (!response.ok) throw new Error('Demo readiness failed');
  return { url: `http://127.0.0.1:${state.apiPort}/`, ready: true };
}
export async function runtimeDown() {
  current();
  if (!existsSync(ownerPath)) return { stopped: true };
  await control('stop');
  for (let i = 0; i < 160 && existsSync(ownerPath); i++) await sleep(100);
  if (existsSync(ownerPath)) throw new Error('Demo shutdown incomplete; ownership preserved');
  return { stopped: true };
}
export async function runtimeUp() {
  let state = current();
  if (existsSync(ownerPath)) return runtimeStatus();
  if (existsSync(socketPath)) throw new Error('Unowned runtime socket; refusing takeover');
  if (!state.apiPort) state = checkpoint(root, state, { apiPort: await availablePort() });
  await availablePort(state.apiPort);
  for (const name of ['content', 'inputs', 'artifacts']) {
    const path = join(root, name);
    if (!existsSync(path)) mkdirSync(path, { mode: 0o700 });
    if (!lstatSync(path).isDirectory() || (lstatSync(path).mode & 0o077)) throw new Error('Unsafe runtime directory');
  }
  const env = environment(state);
  run('go', ['build', '-o', binary, './cmd/semlia'], env);
  run(binary, ['migrate', 'up'], env);
  run(binary, ['check-schema'], env);
  for (const [slug, label] of (state.singleAdmin ? [['showcase', '成果展示区 · 合成零售']] : [['showcase', '成果展示区 · 合成零售'], ['workshop', '现场操作区 · 合成零售'], ['rehearsal', '验收副本 · 合成零售']])) {
    run(binary, ['bootstrap-local-admin', `resume-${slug}`, label, state.singleAdmin ? 'admin' : `${slug}_admin`], env, readSecrets(root).accountPassword + '\n');
  }
  const fd = openSync(join(root, 'supervisor.log'), 'a', 0o600);
  const child = spawn(process.execPath, [fileURLToPath(import.meta.url), 'supervise'], { cwd: repo, env: { PATH: process.env.PATH, HOME: process.env.HOME }, detached: true, stdio: ['ignore', fd, fd] });
  closeSync(fd); child.unref();
  let exited = false; child.once('error', () => { exited = true; }); child.once('exit', () => { exited = true; });
  for (let i = 0; i < 120; i++) {
    await sleep(500);
    if (exited) throw new Error('Demo supervisor exited before ready');
    try { return await runtimeStatus(); } catch { /* Readiness is not guaranteed by the socket alone. */ }
  }
  if (existsSync(ownerPath)) await runtimeDown();
  throw new Error('Demo startup timeout');
}
async function supervise() {
  process.umask(0o077);
  const state = current();
  await availablePort(state.apiPort);
  const env = environment(state), token = randomBytes(32).toString('hex');
  writeFileSync(ownerPath, JSON.stringify({ owner: state.owner, pid: process.pid, token }), { flag: 'wx', mode: 0o600 });
  const ownerIdentity = lstatSync(ownerPath);
  let socketIdentity;
  const children = new Set();
  let stopping = false, timer;
  const server = createServer((req, res) => {
    if (req.method !== 'POST' || req.headers.authorization !== token) { res.writeHead(403).end(); return; }
    if (req.url === '/status') { res.writeHead(!stopping && children.size === 2 ? 200 : 503).end(); return; }
    if (req.url !== '/stop') { res.writeHead(404).end(); return; }
    res.end(); stop();
  });
  function cleanup() {
    clearTimeout(timer); server.close();
    unlinkOwned(socketPath, socketIdentity);
    unlinkOwned(ownerPath, ownerIdentity);
  }
  function stop() {
    if (stopping) return;
    stopping = true;
    for (const child of children) child.kill('SIGTERM');
    timer = setTimeout(() => { for (const child of children) child.kill('SIGKILL'); }, 12000);
    if (!children.size) cleanup();
  }
  process.once('SIGTERM', stop); process.once('SIGINT', stop);
  try {
    await new Promise((resolve, reject) => { server.once('error', reject); server.listen(socketPath, resolve); });
    socketIdentity = lstatSync(socketPath);
    for (const mode of ['server', 'worker']) {
      const fd = openSync(join(root, `${mode}.log`), 'a', 0o600);
      const child = spawn(binary, [mode], { cwd: repo, env, stdio: ['ignore', fd, fd] });
      closeSync(fd); children.add(child);
      child.once('error', stop);
      child.once('close', () => { children.delete(child); if (!stopping) stop(); else if (!children.size) cleanup(); });
    }
  } catch (error) { stop(); throw error; }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  if (process.argv[2] !== 'supervise' || process.argv.length !== 3) throw new Error('Internal runtime entry only');
  supervise().catch(() => { console.error('Demo supervisor failed'); process.exitCode = 1; });
}

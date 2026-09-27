import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repository = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const realGo = execFileSync('/usr/bin/which', ['go'], { encoding: 'utf8' }).trim();
const contaminated = ['COMPOSE_FILE', 'COMPOSE_PROJECT_NAME', 'COMPOSE_ENV_FILES', 'DOCKER_HOST', 'DOCKER_CONTEXT', 'DOCKER_CONFIG', 'DOCKER_CERT_PATH', 'DOCKER_TLS_VERIFY', 'SEMLIA_SECRET_KEY', 'SEMLIA_DATABASE_URL', 'SEMLIA_EXECUTION_SOURCES', 'SEMLIA_UAT_LLM_KEY', 'PRIVATE_PROVIDER_KEY'];

function fixture(t, options = {}) {
  const root = mkdtempSync(join(tmpdir(), 'semlia-smoke-test-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const path of ['scripts/ci', 'scripts/dev', '.semlia', 'bin']) mkdirSync(join(root, path), { recursive: true });
  for (const path of ['scripts/ci/check-smoke.sh', 'scripts/dev/compose.sh', 'compose.yaml', 'compose.override.yaml']) copyFileSync(join(repository, path), join(root, path));
  writeFileSync(join(root, '.semlia/dev.env'), 'PRIVATE_PROVIDER_KEY=synthetic-private-canary\n');
  writeFileSync(join(root, 'scripts/dev/ensure-env.sh'), '#!/bin/sh\nexit 0\n', { mode: 0o755 });
  const config = JSON.stringify({ root, options, contaminated, repository, realGo });
  const shim = `#!${process.execPath}
import { appendFileSync, existsSync, readFileSync, writeFileSync, statSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { basename, join } from 'node:path';
const {root, options, contaminated, repository, realGo} = ${config};
const tool = basename(process.argv[1]);
let args = process.argv.slice(2);
const host = args.includes('--host') ? args[args.indexOf('--host')+1] : undefined;
const configDirectory = args.includes('--config') ? args[args.indexOf('--config')+1] : undefined;
const configValue = configDirectory ? JSON.parse(readFileSync(join(configDirectory,'config.json'),'utf8')) : undefined;
const configKeys = configValue ? Object.keys(configValue) : undefined;
const anonymousAuth = configValue ? JSON.stringify(configValue.auths) === '{"https://index.docker.io/v1/":{}}' : undefined;
const log = (extra = {}) => appendFileSync(join(root, 'calls.jsonl'), JSON.stringify({tool,args,host,configDirectory,configKeys,anonymousAuth,privateKeys:contaminated.filter(key => process.env[key] !== undefined),...extra})+'\\n');
const path = join(root,'state.json');
const state = existsSync(path) ? JSON.parse(readFileSync(path,'utf8')) : {image:Boolean(options.imageExists)};
const save = () => writeFileSync(path,JSON.stringify(state));
if (tool === 'make') { log(); process.exit(0); }
if (tool === 'node') {
  log();
  if(args.some(arg=>arg.includes('randomBytes(16)'))) {
    if(options.ownerFailure) process.exit(1);
    if(options.invalidOwner) {console.log('');process.exit(0);}
  }
  if(args.includes('--input-type=module')) {
    if(options.portFailure) process.exit(1);
    if(options.invalidPort) {console.log('0');process.exit(0);}
  }
  const child=spawnSync(${JSON.stringify(process.execPath)},args,{stdio:'inherit'});process.exit(child.status??1);
}
if (tool === 'go') {
  log({smoke:process.env.SEMLIA_RUN_SMOKE,url:process.env.SEMLIA_SMOKE_URL,goenv:process.env.GOENV,gowork:process.env.GOWORK,goflags:process.env.GOFLAGS});
  if(options.realNestedGo) {
    const env={...process.env,SEMLIA_SMOKE_LAUNCHER_TEST:'1'};
    if(options.omitContext) delete env.SEMLIA_SMOKE_CONTEXT;
    const child=spawnSync(realGo,['test','-count=1','-run','^TestSmokeIsolationLauncherPropagation$','./tests/smoke'],{cwd:repository,env,stdio:'pipe'});
    log({nestedGoStatus:child.status,missingContextRejected:child.stdout?.toString().includes('launcher did not pass its isolation context')});
    process.exit(child.status??1);
  }
  process.exit(options.testFailure ? 1 : 0);
}
while (['--host','--config'].includes(args[0])) args=args.slice(2);
if (args[0] === 'context') { log(); console.log(options.remote ? 'ssh://unrelated-host' : 'unix:///tmp/semlia-test.sock'); process.exit(0); }
if (args[0] === 'compose') {
  const envFile=args[args.indexOf('--env-file')+1];
  const env=readFileSync(envFile,'utf8');
  const files=args.flatMap((arg,index)=>arg==='-f'?[args[index+1]]:[]);
  const ownerFile=files.at(-1);
  const owner=files.length===3 ? readFileSync(ownerFile,'utf8').match(/io\\.semlia\\.smoke\\.owner: "([a-z0-9-]+)"/)?.[1] : undefined;
  state.project=args.includes('--project-name')?args[args.indexOf('--project-name')+1]:process.env.COMPOSE_PROJECT_NAME;
  state.owner=owner;
  log({envFile,files,envPrivate:env.includes('synthetic-private-canary'),envMode:statSync(envFile).mode & 0o777,hasFreshSecrets:/SEMLIA_SECRET_KEY=[0-9a-f]{64}/.test(env)&&/SEMLIA_POSTGRES_PASSWORD=[0-9a-f]{64}/.test(env),owner});
  if(args.includes('up')) {state.created=true;state.image=true;save();process.exit(options.upFailure?1:0);}
  if(args.includes('down')) {state.removed=true;state.created=false;save();process.exit(0);}
  if(['ps','restart','stop','start','exec'].some(command=>args.includes(command))) {save();process.exit(0);}
  throw new Error('unexpected compose command');
}
log();
if ((args[0]==='ps') || (['container','network','volume'].includes(args[0]) && args[1]==='ls')) {
  const named=args.some(arg=>arg.startsWith('name='));
  if(named&&args[0]==='volume'&&(options.hiddenVolume||options.ownerDrift&&state.created)) {console.log('volume-hidden');process.exit(0);}
  if(named&&args[0]==='network'&&options.hiddenNetwork) {console.log('network-hidden');process.exit(0);}
  if(options.hiddenVolume) process.exit(0);
  if(options.preexisting&&!state.created) {console.log('foreign-existing');process.exit(0);}
  if(state.created) console.log(args[0]==='ps'||args[0]==='container'?'container-owned':args[0]==='network'?'network-owned':'volume-owned');
  process.exit(0);
}
if (args[0]==='image'&&args[1]==='ls') {if(state.image) console.log('image-owned');process.exit(0);}
if (args[1]==='inspect') {
  if(args[0]==='image'&&!state.image) process.exit(1);
  console.log((options.foreignOwner||options.foreignImage&&args[0]==='image'||args.at(-1)==='volume-hidden'?'foreign':state.owner)+'|'+state.project);process.exit(0);
}
if(args[0]==='image'&&args[1]==='rm') {state.imageRemoved=true;state.image=false;save();process.exit(0);}
throw new Error('unexpected docker command');
`;
  for (const tool of ['docker', 'go', 'make', 'node']) { const path = join(root, 'bin', tool); writeFileSync(path, shim); chmodSync(path, 0o755); }
  let status = 0;
  try {
    execFileSync('/bin/bash', [join(root, 'scripts/ci/check-smoke.sh')], { cwd: root, env: { ...process.env, PATH: `${join(root, 'bin')}:${process.env.PATH}`, GOENV: '/synthetic-private-goenv', GOWORK: '/synthetic-private-gowork', GOFLAGS: '-run=NoSmokeTests', ...Object.fromEntries(contaminated.map(key => [key, 'synthetic-private-canary'])) }, stdio: 'pipe', timeout: options.realNestedGo ? 60000 : 20000 });
  } catch (error) { status = error.status ?? -1; }
  const calls = existsSync(join(root, 'calls.jsonl')) ? readFileSync(join(root, 'calls.jsonl'), 'utf8').trim().split('\n').filter(Boolean).map(JSON.parse) : [];
  const privateDirectories = new Set(calls.filter(call => call.hasFreshSecrets && call.owner).map(call => dirname(call.envFile)));
  t.after(() => { for (const path of privateDirectories) if (path.startsWith('/tmp/semlia-smoke.')) rmSync(path, { recursive: true, force: true }); });
  const state = existsSync(join(root, 'state.json')) ? JSON.parse(readFileSync(join(root, 'state.json'), 'utf8')) : {};
  return { root, calls, state, status };
}

test('smoke ignores private parent/Compose/dev.env inputs and never synchronizes shared embedded assets', t => {
  const { root, calls, state, status } = fixture(t);
  assert.equal(status, 0);
  assert.ok(calls.length);
  for (const call of calls) assert.deepEqual(call.privateKeys, [], `${call.tool} inherited private environment`);
  for (const call of calls.filter(call => call.tool === 'docker' && call.args[0] !== 'context')) {
    assert.equal(call.host, 'unix:///tmp/semlia-test.sock');
    assert.deepEqual(call.configKeys, ['auths', 'cliPluginsExtraDirs']);
    assert.equal(call.anonymousAuth, true);
    assert.match(call.configDirectory, /^\/tmp\/semlia-smoke\./);
  }
  assert.equal(calls.some(call => call.tool === 'make'), false);
  const compose = calls.filter(call => call.args[0] === 'compose');
  assert.equal(compose.length, 2);
  for (const call of compose) {
    assert.deepEqual(call.files.slice(0, 2), [join(root, 'compose.yaml'), join(root, 'compose.override.yaml')]);
    assert.equal(call.envPrivate, false);
    assert.equal(call.envMode, 0o600);
    assert.equal(call.hasFreshSecrets, true);
    assert.ok(call.owner);
    assert.notEqual(call.envFile, join(root, '.semlia/dev.env'));
  }
  assert.equal(readFileSync(join(root, '.semlia/dev.env'), 'utf8'), 'PRIVATE_PROVIDER_KEY=synthetic-private-canary\n');
  assert.equal(state.removed, true);
  assert.equal(state.imageRemoved, true);
  assert.equal(existsSync(compose[0].envFile), false);
  const go = calls.find(call => call.tool === 'go');
  assert.equal(go.smoke, '1');
  assert.match(go.url, /^http:\/\/127\.0\.0\.1:\d+$/);
  assert.equal(go.goenv, 'off');
  assert.equal(go.gowork, 'off');
  assert.equal(go.goflags, '');
});

test('smoke refuses a nonlocal Docker endpoint before any Compose or cleanup action', t => {
  const { calls, status } = fixture(t, { remote: true });
  assert.notEqual(status, 0);
  assert.equal(calls.some(call => call.args[0] === 'compose'), false);
});

test('smoke refuses existing project resources before build and does not clean them', t => {
  const { calls, status } = fixture(t, { preexisting: true });
  assert.notEqual(status, 0);
  assert.equal(calls.some(call => call.args[0] === 'compose' || call.args.includes('rm')), false);
});

test('a configured volume hidden from project-label discovery is rejected before Compose starts', t => {
  const { calls, status } = fixture(t, { hiddenVolume: true });
  assert.notEqual(status, 0);
  assert.equal(calls.some(call => call.args[0] === 'compose' || call.args.includes('rm')), false);
});

test('a configured network or preexisting image is never adopted or removed', t => {
  for (const options of [{ hiddenNetwork: true }, { imageExists: true }]) {
    const { calls, status } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(calls.some(call => call.args[0] === 'compose' || call.args.includes('rm')), false);
  }
});

test('a configured volume that loses ownership after startup blocks all destructive cleanup', t => {
  const { calls, status } = fixture(t, { ownerDrift: true });
  assert.notEqual(status, 0);
  assert.equal(calls.some(call => call.args.includes('down') || call.args.includes('rm')), false);
});

test('failed or invalid owner/port generation stops before any create or destructive cleanup', t => {
  for (const options of [{ ownerFailure: true }, { invalidOwner: true }, { portFailure: true }, { invalidPort: true }]) {
    const { calls, status } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(calls.some(call => call.args[0] === 'compose' || call.args.includes('rm')), false);
  }
});

test('smoke refuses destructive cleanup when any resource ownership differs', t => {
  const { calls, status, state } = fixture(t, { foreignOwner: true });
  assert.notEqual(status, 0);
  assert.equal(calls.some(call => call.args.includes('down') || call.args.includes('rm')), false);
  assert.notEqual(state.removed, true);
});

test('a foreign image label prevents both Compose cleanup and image removal', t => {
  const { calls, status } = fixture(t, { foreignImage: true });
  assert.notEqual(status, 0);
  assert.equal(calls.some(call => call.args.includes('down') || call.args.includes('rm')), false);
});

test('failed build/start and failed smoke tests still clean only verified owned resources', t => {
  for (const options of [{ upFailure: true }, { testFailure: true }]) {
    const { state, status } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(state.removed, true);
    assert.equal(state.imageRemoved, true);
  }
});

test('launcher receipt reaches actual Go nested helpers, including destructive lifecycle calls', t => {
  const { root, calls, state, status } = fixture(t, { realNestedGo: true });
  assert.equal(status, 0);
  assert.equal(calls.find(call => call.nestedGoStatus !== undefined)?.nestedGoStatus, 0);
  const compose = calls.filter(call => call.args[0] === 'compose');
  for (const operation of ['ps', 'restart', 'stop', 'start', 'exec']) assert.ok(compose.some(call => call.args.includes(operation)), operation);
  assert.equal(compose.filter(call => call.args.includes('up')).length, 2);
  assert.equal(compose.filter(call => call.args.includes('down')).length, 2);
  for (const call of calls.filter(call => call.tool === 'docker' && call.args[0] !== 'context')) {
    assert.equal(call.host, 'unix:///tmp/semlia-test.sock');
    assert.deepEqual(call.privateKeys, []);
    assert.equal(call.configDirectory, compose[0].configDirectory);
  }
  for (const call of compose) {
    assert.deepEqual(call.files, compose[0].files);
    assert.equal(call.envFile, compose[0].envFile);
    assert.equal(call.owner, compose[0].owner);
  }
  assert.equal(readFileSync(join(root, '.semlia/dev.env'), 'utf8'), 'PRIVATE_PROVIDER_KEY=synthetic-private-canary\n');
  assert.equal(state.removed, true);
  assert.equal(state.imageRemoved, true);
});

test('actual Go helpers refuse a lost launcher receipt before any nested Docker call', t => {
  const { calls, state, status } = fixture(t, { realNestedGo: true, omitContext: true });
  assert.notEqual(status, 0);
  assert.equal(calls.find(call => call.nestedGoStatus !== undefined)?.missingContextRejected, true);
  assert.equal(calls.filter(call => call.args[0] === 'compose').length, 2);
  assert.equal(state.removed, true);
  assert.equal(state.imageRemoved, true);
});

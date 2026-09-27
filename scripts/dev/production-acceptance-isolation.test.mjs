import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repository = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const privateKeys = ['COMPOSE_FILE', 'COMPOSE_PROJECT_NAME', 'DOCKER_HOST', 'DOCKER_CONFIG', 'SEMLIA_EXECUTION_SOURCES', 'PRIVATE_PROVIDER_KEY'];

function fixture(t, options = {}) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'semlia-production-isolation-')));
  t.after(() => {
    const pidFile = join(root, 'server.pid');
    if (existsSync(pidFile)) {
      const pid = Number(readFileSync(pidFile, 'utf8'));
      try {
        const command = execFileSync('ps', ['-p', String(pid), '-o', 'args='], { encoding: 'utf8' });
        if (command.includes(root + '/.semlia/production-acceptance/')) process.kill(pid, 'SIGTERM');
      } catch { /* Already terminated by the launcher. */ }
    }
    rmSync(root, { recursive: true, force: true });
  });
  for (const dir of ['scripts/dev', '.semlia', 'bin']) mkdirSync(join(root, dir), { recursive: true });
  copyFileSync(join(repository, 'scripts/dev/production-acceptance.sh'), join(root, 'scripts/dev/production-acceptance.sh'));
  copyFileSync(join(repository, 'compose.production-acceptance.yaml'), join(root, 'compose.production-acceptance.yaml'));
  writeFileSync(join(root, '.env'), 'PRIVATE_PROVIDER_KEY=synthetic-file-canary\n');
  const shim = `#!${process.execPath}
import {appendFileSync,existsSync,readFileSync,writeFileSync} from 'node:fs'; import {basename,join} from 'node:path'; import {spawnSync} from 'node:child_process';import {randomBytes} from 'node:crypto';
const root=${JSON.stringify(root)}, options=${JSON.stringify(options)}, privateKeys=${JSON.stringify(privateKeys)};
const tool=basename(process.argv[1]); let args=process.argv.slice(2);
const host=args.includes('--host')?args[args.indexOf('--host')+1]:null, config=args.includes('--config')?args[args.indexOf('--config')+1]:null;
const log=(extra={})=>appendFileSync(join(root,'calls.jsonl'),JSON.stringify({tool,args,host,config,privateKeys:privateKeys.filter(key=>process.env[key]!==undefined),...extra})+'\\n');
const stateFile=join(root,'state.json'); const state=existsSync(stateFile)?JSON.parse(readFileSync(stateFile)):{}; const save=()=>writeFileSync(stateFile,JSON.stringify(state));
if(tool==='openssl') {log();const owner=args.at(-1)==='8';if(options[owner?'ownerFailure':'passwordFailure'])process.exit(1);if(options[owner?'invalidOwner':'invalidPassword'])console.log('invalid');else console.log(randomBytes(Number(args.at(-1))).toString('hex'));process.exit(0);}
if(tool==='node') {log();if(args[0]==='-e'&&args[1].includes('createServer')) {if(options.portFailure)process.exit(1);state.ports=(state.ports??0)+1;save();console.log(options.invalidPort?'0':String(18880+state.ports));process.exit(0);}const child=spawnSync(${JSON.stringify(process.execPath)},args,{stdio:'inherit'});process.exit(child.status??1);}
if(tool==='go') {log();if(args[0]==='build'){const output=args[args.indexOf('-o')+1];writeFileSync(output,${JSON.stringify(`#!${process.execPath}\nconst fs=require('node:fs');fs.writeFileSync(${JSON.stringify(join(root,'server.pid'))},String(process.pid));fs.writeFileSync(process.env.SEMLIA_ACCEPTANCE_ROOT+'/bootstrap.json','{}');setInterval(()=>{},1000);\n`)},{mode:0o700});}process.exit(0);}
if(tool==='curl'){log();process.exit(0);}
if(tool==='pnpm'){log();if(args.includes('playwright')){if(options.markerDrift)writeFileSync(process.env.SEMLIA_ACCEPTANCE_ROOT+'/.owner','foreign');if(options.ownerDrift){state.foreign=true;save();}if(options.secretError)console.error('synthetic-login-secret');process.exit(options.browserFailure||options.secretError?1:0);}process.exit(0);}
while(['--config','--host'].includes(args[0]))args=args.slice(2);log();
if(args[0]==='context'){console.log(options.remote?'ssh://foreign':'unix:///tmp/semlia-browser-test.sock');process.exit(0);}
if(args[0]==='compose'){state.project=args[args.indexOf('--project-name')+1];if(args.includes('up')){state.created=true;save();process.exit(options.upFailure?1:0);}if(args.includes('down')){state.created=false;state.removed=true;save();process.exit(0);}throw Error('unexpected compose operation');}
if(['container','network','volume'].includes(args[0])&&args[1]==='ls'){const named=args.some(arg=>arg.startsWith('name='));if(named&&(options.hiddenVolume&&args[0]==='volume'||options.hiddenNetwork&&args[0]==='network')){console.log('foreign-hidden');process.exit(0);}if(state.created)console.log(args[0]+'-owned');process.exit(0);}
if(args[1]==='inspect'){const owner=state.foreign?'foreign':state.project;const format=args[args.indexOf('--format')+1];console.log(format.includes('com.docker.compose.project')?owner+'|'+state.project:owner);process.exit(0);}
throw Error('unexpected fake Docker operation');
`;
  for (const tool of ['docker', 'go', 'node', 'openssl', 'pnpm', 'curl']) { writeFileSync(join(root, 'bin', tool), shim); chmodSync(join(root, 'bin', tool), 0o700); }
  let status = 0, diagnostics = '';
  try { diagnostics = execFileSync('/bin/bash', [join(root, 'scripts/dev/production-acceptance.sh'), '--suite', 'desktop'], { cwd: root, env: { PATH: `${join(root, 'bin')}:${process.env.PATH}`, HOME: process.env.HOME, ...Object.fromEntries(privateKeys.map(key => [key, 'synthetic-parent-canary'])) }, stdio: 'pipe', timeout: 20000 }).toString(); } catch (error) { status = error.status ?? -1; diagnostics = String(error.stdout ?? '') + String(error.stderr ?? ''); }
  const calls = existsSync(join(root, 'calls.jsonl')) ? readFileSync(join(root, 'calls.jsonl'), 'utf8').trim().split('\n').map(JSON.parse) : [];
  const state = existsSync(join(root, 'state.json')) ? JSON.parse(readFileSync(join(root, 'state.json'))) : {};
  return { root, calls, state, status, diagnostics };
}

test('production launcher pins every Compose input and cleans only its owned project', t => {
  const { calls, state, status } = fixture(t);
  assert.equal(status, 0);
  for (const call of calls) assert.deepEqual(call.privateKeys, []);
  const docker = calls.filter(call => call.tool === 'docker' && call.args[0] !== 'context');
  for (const call of docker) { assert.equal(call.host, 'unix:///tmp/semlia-browser-test.sock'); assert.ok(call.config); }
  for (const call of calls.filter(call => call.args[0] === 'compose')) { assert.ok(call.args.includes('--env-file')); assert.ok(call.args.includes('--project-directory')); }
  assert.equal(state.removed, true);
  assert.ok(calls.some(call => call.tool === 'pnpm' && call.args.includes('playwright')));
});

test('unlabelled configured volume/network blocks adoption without cleanup', t => {
  for (const options of [{ hiddenVolume: true }, { hiddenNetwork: true }]) {
    const { calls, status } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(calls.some(call => call.args[0] === 'compose'), false);
  }
});

test('marker or resource ownership drift refuses teardown', t => {
  for (const options of [{ markerDrift: true }, { ownerDrift: true }]) {
    const { calls, state, status } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(calls.some(call => call.args.includes('down')), false);
    assert.notEqual(state.removed, true);
  }
});

test('failed or malformed owner, secret and port generation never creates resources', t => {
  for (const options of [{ ownerFailure: true }, { passwordFailure: true }, { invalidOwner: true }, { invalidPassword: true }, { portFailure: true }, { invalidPort: true }, { remote: true }]) {
    const { calls, status } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(calls.some(call => call.args[0] === 'compose'), false);
  }
});

test('partial startup and browser failure still clean verified owned resources', t => {
  for (const options of [{ upFailure: true }, { browserFailure: true }]) {
    const { status, state } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(state.removed, true);
  }
});

test('credential-bearing browser diagnostics remain in the protected evidence directory', t => {
  const { status, diagnostics, state } = fixture(t, { secretError: true });
  assert.notEqual(status, 0);
  assert.equal(diagnostics.includes('synthetic-login-secret'), false);
  assert.equal(state.removed, true);
});

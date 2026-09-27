import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repository = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const privateKeys = ['DOCKER_HOST', 'DOCKER_CONFIG', 'COMPOSE_FILE', 'SEMLIA_EXECUTION_SOURCES', 'PRIVATE_PROVIDER_KEY', 'SEMLIA_NATIVE_LAN_IP'];

function fixture(t, options = {}) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'semlia-native-isolation-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const dir of ['scripts/dev', 'web/node_modules/@playwright/test', '.semlia', 'bin']) mkdirSync(join(root, dir), { recursive: true });
  for (const file of ['validate-native.mjs', 'native-environment.mjs']) if (existsSync(join(repository, 'scripts/dev', file))) copyFileSync(join(repository, 'scripts/dev', file), join(root, 'scripts/dev', file));
  writeFileSync(join(root, 'web/package.json'), '{}');
  writeFileSync(join(root, 'web/node_modules/@playwright/test/index.js'), `
const locator=new Proxy({}, {get:(_,key)=>key==='then'?undefined:['waitFor','fill','click','screenshot','setViewportSize','goto'].includes(key)?async()=>{}:()=>locator});
const page=locator;
const context={newPage:async()=>page,cookies:async()=>[{name:'semlia_session_dev',httpOnly:true,sameSite:'Lax'}],pages:()=>[page]};
exports.chromium={launch:async()=>{if(${Boolean(options.secretError)})throw Error('synthetic-login-secret');return {newContext:async()=>context,contexts:()=>[context],close:async()=>{if(${Boolean(options.rootDrift)}){const fs=require('node:fs'),parent=${JSON.stringify(join(root,'.semlia/evidence-work'))};const directory=parent+'/'+fs.readdirSync(parent)[0],foreign=${JSON.stringify(join(root,'foreign-receipt'))};fs.mkdirSync(foreign);fs.writeFileSync(foreign+'/cleanup.json','sentinel');fs.renameSync(directory,directory+'.owned');fs.symlinkSync(foreign,directory);}if(${Boolean(options.closeFailure)})throw Error('synthetic close failure')}}}};
`);
  const shim = `#!${process.execPath}
import {appendFileSync,existsSync,mkdirSync,readFileSync,renameSync,symlinkSync,writeFileSync} from 'node:fs'; import {basename,dirname,join} from 'node:path';
const root=${JSON.stringify(root)}, options=${JSON.stringify(options)}, privateKeys=${JSON.stringify(privateKeys)};
const tool=basename(process.argv[1]); let args=process.argv.slice(2);
const host=args.includes('--host')?args[args.indexOf('--host')+1]:null, config=args.includes('--config')?args[args.indexOf('--config')+1]:null;
const record=(extra={})=>appendFileSync(join(root,'calls.jsonl'),JSON.stringify({tool,args,host,config,privateKeys:privateKeys.filter(key=>process.env[key]!==undefined),...extra})+'\\n');
const stateFile=join(root,'docker-state.json');const state=existsSync(stateFile)?JSON.parse(readFileSync(stateFile)):{};const save=()=>writeFileSync(stateFile,JSON.stringify(state));
if(tool==='go'){record();process.exit(0);}
if(tool==='node') {record({state:process.env.SEMLIA_NATIVE_STATE_DIR,envFile:process.env.SEMLIA_NATIVE_ENV_FILE});if(args.includes('up')&&options.markerDrift)writeFileSync(join(dirname(process.env.SEMLIA_NATIVE_ENV_FILE),'.owner'),'foreign');if(args.includes('up')&&options.stateDrift){const state=process.env.SEMLIA_NATIVE_STATE_DIR;renameSync(state,state+'.owned');mkdirSync(join(root,'.semlia','foreign-native'));symlinkSync(join(root,'.semlia','foreign-native'),state);}if(args.includes('up')&&options.upRootDrift){const directory=dirname(process.env.SEMLIA_NATIVE_ENV_FILE),foreign=join(root,'foreign-receipt');renameSync(directory,directory+'.owned');mkdirSync(foreign);writeFileSync(join(foreign,'cleanup.json'),'sentinel');writeFileSync(join(foreign,'result.json'),'sentinel');writeFileSync(join(foreign,'failure.png'),'sentinel');symlinkSync(foreign,directory);}process.exit(args.includes('down')&&options.downFailure?1:0);}
while(['--config','--host'].includes(args[0]))args=args.slice(2);record();
if(args[0]==='context'){console.log(options.remote?'ssh://foreign':'unix:///tmp/semlia-browser-test.sock');process.exit(0);}
if(args[0]==='run'){state.id='a'.repeat(64);state.owner=args.find(arg=>arg.startsWith('io.semlia.acceptance.owner='))?.split('=')[1];state.created=true;save();console.log(options.invalidID?'not-a-container-id':state.id);process.exit(0);}
if(args[0]==='exec')process.exit(0);
if(args.includes('inspect')){if(args.includes('--format')){const f=args[args.indexOf('--format')+1];if(f.includes('Labels'))console.log(state.id+'|'+(options.foreignOwner?'foreign':state.owner));else console.log(options.invalidPort?'0':'19999');}else console.log(JSON.stringify([{NetworkSettings:{Ports:{'5432/tcp':[{HostPort:'19999'}]}}}]));process.exit(0);}
if(args[0]==='rm'){state.removed=true;state.volumeRemoved=args.includes('--volumes')||args.includes('-v');save();process.exit(0);}
if(args[0]==='container'&&args[1]==='ls'){if(state.created&&!state.removed)console.log(state.id);process.exit(0);}
throw Error('unexpected fake command');
`;
  for (const tool of ['node', 'go', 'docker']) writeFileSync(join(root, 'bin', tool), shim, { mode: 0o700 });
  let status = 0, diagnostics = '';
  try { diagnostics = execFileSync(process.execPath, [join(root, 'scripts/dev/validate-native.mjs')], { cwd: root, env: { PATH: `${join(root, 'bin')}:${process.env.PATH}`, HOME: process.env.HOME, ...Object.fromEntries(privateKeys.map(key => [key, 'synthetic-private-canary'])) }, stdio: 'pipe', timeout: 15000 }).toString(); } catch (error) { status = error.status ?? -1; diagnostics = String(error.stdout ?? '') + String(error.stderr ?? ''); }
  const calls = existsSync(join(root, 'calls.jsonl')) ? readFileSync(join(root, 'calls.jsonl'), 'utf8').trim().split('\n').map(JSON.parse) : [];
  const state = existsSync(join(root, 'docker-state.json')) ? JSON.parse(readFileSync(join(root, 'docker-state.json'))) : {};
  return { status, calls, state, diagnostics, foreignReceipt: existsSync(join(root,'foreign-receipt/cleanup.json')) ? readFileSync(join(root,'foreign-receipt/cleanup.json'),'utf8') : undefined, foreignResult: existsSync(join(root,'foreign-receipt/result.json')) ? readFileSync(join(root,'foreign-receipt/result.json'),'utf8') : undefined, foreignScreenshot: existsSync(join(root,'foreign-receipt/failure.png')) ? readFileSync(join(root,'foreign-receipt/failure.png'),'utf8') : undefined };
}

test('native browser launcher uses private random controls and one clean local Docker context', t => {
  const { status, calls, state } = fixture(t);
  assert.equal(status, 0);
  for (const call of calls) assert.deepEqual(call.privateKeys, []);
  const docker = calls.filter(call => call.tool === 'docker' && call.args[0] !== 'context');
  assert.ok(docker.length);
  for (const call of docker) { assert.equal(call.host, 'unix:///tmp/semlia-browser-test.sock'); assert.ok(call.config); }
  const native = calls.filter(call => call.tool === 'node');
  assert.ok(native.some(call => call.args.includes('up')));
  assert.ok(native.some(call => call.args.includes('down')));
  assert.equal(new Set(native.map(call => call.state)).size, 1);
  assert.equal(new Set(native.map(call => dirname(call.envFile))).size, 1);
  assert.equal(state.volumeRemoved, true);
});

test('foreign native marker or container owner prevents both shutdown and deletion', t => {
  for (const options of [{ markerDrift: true }, { foreignOwner: true }, { stateDrift: true }]) {
    const { status, calls, state } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(calls.some(call => call.tool === 'node' && call.args.includes('down')), false);
    assert.notEqual(state.removed, true);
  }
});

test('native shutdown and browser-close failures are reported while owned cleanup still runs', t => {
  for (const options of [{ downFailure: true }, { closeFailure: true }]) {
    const { status, state } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(state.removed, true);
    assert.equal(state.volumeRemoved, true);
  }
});

test('nonlocal Docker or invalid container/port identity prevents native migration', t => {
  for (const options of [{ remote: true }, { invalidID: true }, { invalidPort: true }]) {
    const { status, calls } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(calls.some(call => call.tool === 'node' && call.args.includes('migrate')), false);
  }
});

test('native root replacement refuses cleanup and never overwrites a foreign receipt', t => {
  for (const options of [{ rootDrift: true }, { upRootDrift: true }]) {
    const { status, calls, state, foreignReceipt, foreignResult, foreignScreenshot } = fixture(t, options);
    assert.notEqual(status, 0);
    assert.equal(calls.some(call => call.tool === 'node' && call.args.includes('down')), false);
    assert.notEqual(state.removed, true);
    assert.equal(foreignReceipt, 'sentinel');
    if (options.upRootDrift) { assert.equal(foreignResult, 'sentinel'); assert.equal(foreignScreenshot, 'sentinel'); }
  }
});

test('native browser diagnostics never print a credential-bearing exception', t => {
  const { status, diagnostics } = fixture(t, { secretError: true });
  assert.notEqual(status, 0);
  assert.equal(diagnostics.includes('synthetic-login-secret'), false);
});

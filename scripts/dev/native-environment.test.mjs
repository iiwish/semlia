import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repository = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

function fixture(t, explicit, missing = false, reserved = '') {
  const root = realpathSync(mkdtempSync(join(tmpdir(), 'semlia-native-env-')));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const dir of ['scripts/dev', '.semlia', 'build']) mkdirSync(join(root, dir), { recursive: true });
  for (const file of ['native.mjs', 'native-environment.mjs']) if (existsSync(join(repository, 'scripts/dev', file))) copyFileSync(join(repository, 'scripts/dev', file), join(root, 'scripts/dev', file));
  writeFileSync(join(root, '.semlia/dev.env'), 'SEMLIA_SECRET_KEY=synthetic-default-key\nSEMLIA_POSTGRES_PASSWORD=synthetic-default-password\nSEMLIA_EXECUTION_SOURCES=synthetic-default-source\nPRIVATE_PROVIDER_KEY=synthetic-default-provider\n');
  writeFileSync(join(root, '.semlia/native.env'), 'SEMLIA_SECRET_KEY=synthetic-native-key\n');
  const isolated = join(root, '.semlia/isolated.env');
  if (!missing) writeFileSync(isolated, 'SEMLIA_SECRET_KEY=synthetic-isolated-key\nSEMLIA_POSTGRES_PASSWORD=synthetic-isolated-password\nSEMLIA_DATABASE_URL=postgresql://synthetic:synthetic@127.0.0.1:19999/synthetic\n' + (reserved ? `${reserved}=/synthetic-unrelated-control\n` : ''), { mode: 0o600 });
  writeFileSync(join(root, 'build/semlia'), `#!${process.execPath}\nrequire('node:fs').writeFileSync(${JSON.stringify(join(root, 'child.json'))},JSON.stringify(Object.fromEntries(['SEMLIA_SECRET_KEY','SEMLIA_POSTGRES_PASSWORD','SEMLIA_EXECUTION_SOURCES','PRIVATE_PROVIDER_KEY','SEMLIA_UAT_LLM_KEY','SEMLIA_NATIVE_LAN_IP','NODE_OPTIONS','SEMLIA_NATIVE_ENV_FILE','SEMLIA_NATIVE_STATE_DIR'].filter(k=>process.env[k]!==undefined).map(k=>[k,process.env[k]]))));\n`, { mode: 0o700 });
  const env = { PATH: process.env.PATH, HOME: process.env.HOME, SEMLIA_NATIVE_STATE_DIR: join(root, '.semlia/test-state'), SEMLIA_UAT_LLM_KEY: 'synthetic-parent-provider', SEMLIA_NATIVE_LAN_IP: 'synthetic-parent-lan', SEMLIA_EXECUTION_SOURCES: 'synthetic-parent-source' };
  if (explicit) env.SEMLIA_NATIVE_ENV_FILE = isolated;
  let status = 0;
  try { execFileSync(process.execPath, [join(root, 'scripts/dev/native.mjs'), 'migrate', 'up'], { cwd: root, env, stdio: 'pipe' }); } catch (error) { status = error.status ?? -1; }
  return { status, controls: { SEMLIA_NATIVE_ENV_FILE: isolated, SEMLIA_NATIVE_STATE_DIR: env.SEMLIA_NATIVE_STATE_DIR }, child: existsSync(join(root, 'child.json')) ? JSON.parse(readFileSync(join(root, 'child.json'), 'utf8')) : undefined };
}

test('explicit native config excludes default files and caller runtime/model/LAN values', t => {
  const { status, child, controls } = fixture(t, true);
  assert.equal(status, 0);
  assert.deepEqual(child, { SEMLIA_SECRET_KEY: 'synthetic-isolated-key', SEMLIA_POSTGRES_PASSWORD: 'synthetic-isolated-password', ...controls });
});

test('missing explicit native config fails before running a command', t => {
  const { status, child } = fixture(t, true, true);
  assert.notEqual(status, 0);
  assert.equal(child, undefined);
});

test('default native config preserves dev/native merge and ordinary caller environment', t => {
  const { status, child } = fixture(t, false);
  assert.equal(status, 0);
  assert.equal(child.SEMLIA_SECRET_KEY, 'synthetic-native-key');
  assert.equal(child.SEMLIA_POSTGRES_PASSWORD, 'synthetic-default-password');
  assert.equal(child.SEMLIA_EXECUTION_SOURCES, 'synthetic-default-source');
  assert.equal(child.PRIVATE_PROVIDER_KEY, 'synthetic-default-provider');
  assert.equal(child.SEMLIA_UAT_LLM_KEY, 'synthetic-parent-provider');
});

test('explicit config cannot redirect its own configuration or supervisor state', t => {
  for (const key of ['SEMLIA_NATIVE_ENV_FILE', 'SEMLIA_NATIVE_STATE_DIR']) {
    const { status, child } = fixture(t, true, false, key);
    assert.notEqual(status, 0);
    assert.equal(child, undefined);
  }
});

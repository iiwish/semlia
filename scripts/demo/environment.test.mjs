import assert from 'node:assert/strict';
import { test } from 'node:test';
import { validateInventory, runtimeEnvironment } from './environment.mjs';

const repo = '/project';
const inventory = { Id: 'a'.repeat(64), State: { Running: true }, Config: { Labels: { 'com.docker.compose.project': 'semlia-local', 'com.docker.compose.service': 'postgres', 'com.docker.compose.project.working_dir': repo } }, NetworkSettings: { Ports: { '5432/tcp': [{ HostIp: '127.0.0.1', HostPort: '5433' }] } } };
test('database inventory rejects foreign projects, stopped services and exposed ports', () => {
  assert.equal(validateInventory(inventory, repo).port, 5433);
  assert.throws(() => validateInventory(inventory, '/other'), /ownership/);
  assert.throws(() => validateInventory({ ...inventory, State: { Running: false } }, repo), /running/);
  assert.throws(() => validateInventory({ ...inventory, NetworkSettings: { Ports: { '5432/tcp': [{ HostIp: '0.0.0.0', HostPort: '5433' }] } } }, repo), /loopback/);
});
test('normal runtime never inherits existing application or model credentials', () => {
  const owner = 'semlia_demo_' + 'a'.repeat(16);
  const env = runtimeEnvironment('/project', '/project/.semlia/resume-demo/retail-v1', { owner, port: 5433, apiPort: 19090 }, { databasePassword: 'a'.repeat(64), secretKey: 'b'.repeat(64) }, { PATH: '/bin', SEMLIA_SECRET_KEY: 'existing', SEMLIA_PRODUCTION_GENERATION_GRANTS: '[secret]', OPENAI_API_KEY: 'secret' });
  assert.equal(env.SEMLIA_PRODUCTION_GENERATION_GRANTS, '[]');
  assert.equal(env.OPENAI_API_KEY, undefined);
  assert.equal(env.SEMLIA_AUTH_MODE, 'password');
  assert.match(env.SEMLIA_DATABASE_URL, /semlia_demo_aaaaaaaaaaaaaaaa_app/);
  assert.equal(env.SEMLIA_ALLOWED_ORIGINS, 'http://127.0.0.1:19090');
});

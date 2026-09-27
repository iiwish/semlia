import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

test('self-host server and worker wait for successful migrations', t => {
  const directory = mkdtempSync(join(tmpdir(), 'semlia-compose-contract-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const environment = join(directory, 'empty.env');
  writeFileSync(environment, '', { mode: 0o600 });
  const config = JSON.parse(execFileSync('docker', ['compose', '--env-file', environment, '-f', join(dirname(fileURLToPath(import.meta.url)), 'compose.yaml'), 'config', '--format', 'json'], {
    env: { PATH: process.env.PATH, HOME: process.env.HOME, SEMLIA_COMPOSE_PROJECT: 'semlia-contract-only', SEMLIA_IMAGE: 'semlia:contract-only', SEMLIA_RUNTIME_ENV_FILE: environment, SEMLIA_MIGRATION_ENV_FILE: environment, SEMLIA_BUILD_VERSION: 'contract-only', SEMLIA_ALLOWED_ORIGINS: 'http://127.0.0.1:18080', SEMLIA_PORT: '18080', SEMLIA_ARTIFACT_HOST_DIR: directory, SEMLIA_CONTENT_HOST_DIR: directory, SEMLIA_DATABASE_NETWORK: 'unused-contract-network' }, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
  }));
  for (const service of ['server', 'worker']) assert.equal(config.services[service].depends_on?.migrate?.condition, 'service_completed_successfully', `${service} must not start on migration failure`);
  assert.equal(config.services.migrate.restart, 'no');
});

test('standard smoke Compose resource names match the guarded ownership inventory', t => {
  const directory = mkdtempSync(join(tmpdir(), 'semlia-compose-contract-'));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const environment = join(directory, 'empty.env');
  writeFileSync(environment, '', { mode: 0o600 });
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
  const config = JSON.parse(execFileSync('docker', ['compose', '--env-file', environment, '--project-name', 'semlia-contract-only', '-f', join(root, 'compose.yaml'), '-f', join(root, 'compose.override.yaml'), 'config', '--format', 'json'], {
    env: { PATH: process.env.PATH, HOME: process.env.HOME, SEMLIA_POSTGRES_PASSWORD: 'synthetic-not-a-runtime-password', SEMLIA_SECRET_KEY: '0'.repeat(64) }, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'],
  }));
  assert.deepEqual(Object.keys(config.volumes).sort(), ['artifact-data', 'git-content', 'postgres-data']);
  assert.deepEqual(Object.keys(config.networks), ['backend']);
  for (const [name, value] of Object.entries({ ...config.volumes, ...config.networks })) {
    assert.equal(value.name, `semlia-contract-only_${name}`);
    assert.notEqual(value.external, true);
  }
});

import { existsSync, readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { parseEnv } from 'node:util';

export function toolEnvironment(environment = process.env) {
  return {
    PATH: environment.PATH,
    HOME: environment.HOME,
    TMPDIR: '/tmp',
    LC_ALL: 'C',
    GOENV: 'off',
    GOWORK: 'off',
    GOFLAGS: '',
    GOTOOLCHAIN: environment.GOTOOLCHAIN === 'local' ? 'local' : 'auto',
  };
}

export function nativeConfiguration(root, environment = process.env) {
  const read = path => existsSync(path) ? parseEnv(readFileSync(path, 'utf8')) : {};
  if (environment.SEMLIA_NATIVE_ENV_FILE === undefined) {
    return { local: { ...read(join(root, '.semlia/dev.env')), ...read(join(root, '.semlia/native.env')) }, base: environment };
  }
  if (!environment.SEMLIA_NATIVE_ENV_FILE || !environment.SEMLIA_NATIVE_STATE_DIR) {
    throw new Error('Explicit native configuration requires a file and state directory.');
  }
  const file = resolve(environment.SEMLIA_NATIVE_ENV_FILE);
  let local;
  try { local = parseEnv(readFileSync(file, 'utf8')); }
  catch { throw new Error('Explicit native configuration file is unavailable or invalid.'); }
  if (Object.hasOwn(local, 'SEMLIA_NATIVE_ENV_FILE') || Object.hasOwn(local, 'SEMLIA_NATIVE_STATE_DIR')) {
    throw new Error('Explicit native configuration cannot redirect its file or state controls.');
  }
  return {
    local,
    base: { ...toolEnvironment(environment), SEMLIA_NATIVE_ENV_FILE: file, SEMLIA_NATIVE_STATE_DIR: resolve(environment.SEMLIA_NATIVE_STATE_DIR) },
  };
}

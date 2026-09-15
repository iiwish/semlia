import { join } from 'node:path';

export function validateInventory(item, repo) {
  const labels = item.Config?.Labels ?? {};
  if (labels['com.docker.compose.project'] !== 'semlia-local' || labels['com.docker.compose.service'] !== 'postgres' || labels['com.docker.compose.project.working_dir'] !== repo) throw new Error('Database ownership mismatch');
  if (!item.State?.Running) throw new Error('Database is not running');
  const ports = item.NetworkSettings?.Ports?.['5432/tcp'];
  if (!Array.isArray(ports) || ports.length !== 1 || ports[0].HostIp !== '127.0.0.1') throw new Error('Database must be loopback-only');
  const port = Number(ports[0].HostPort);
  if (!Number.isInteger(port) || port < 1024 || port > 65535 || !/^[a-f0-9]{64}$/.test(item.Id)) throw new Error('Invalid database inventory');
  return { container: item.Id, port };
}

export function runtimeEnvironment(repo, root, config, secrets, parent = process.env) {
  if (!/^semlia_demo_[a-f0-9]{16}$/.test(config.owner)) throw new Error('Invalid demo owner');
  for (const port of [config.port, config.apiPort]) if (!Number.isInteger(port) || port < 1024 || port > 65535) throw new Error('Invalid demo port');
  if (!/^[a-f0-9]{64}$/.test(secrets.databasePassword) || !/^[a-f0-9]{64}$/.test(secrets.secretKey)) throw new Error('Invalid protected configuration');
  const env = Object.fromEntries(['PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'].filter(key => parent[key]).map(key => [key, parent[key]]));
  return { ...env, SEMLIA_ENV: 'development', SEMLIA_AUTH_MODE: 'password', SEMLIA_HTTP_ADDR: `127.0.0.1:${config.apiPort}`, SEMLIA_ALLOWED_ORIGINS: `http://127.0.0.1:${config.apiPort}`, SEMLIA_DATABASE_URL: `postgresql://${config.owner}:${secrets.databasePassword}@127.0.0.1:${config.port}/${config.owner}_app?sslmode=disable`, SEMLIA_SECRET_KEY: secrets.secretKey, SEMLIA_GIT_REPOSITORY: join(root, 'content'), SEMLIA_SOURCE_ARTIFACT_ROOT: join(root, 'inputs'), SEMLIA_ARTIFACT_ROOT: join(root, 'artifacts'), SEMLIA_ARTIFACT_STORE: 'local', SEMLIA_WORKER_CONFIGURED: 'true', SEMLIA_MIGRATIONS_PATH: join(repo, 'migrations'), SEMLIA_SEMANTIC_PRODUCTION_ENABLED: 'true', SEMLIA_PRODUCTION_GENERATION_GRANTS: '[]', SEMLIA_EXECUTION_SOURCES: '[]' };
}

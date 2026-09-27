import assert from 'node:assert/strict';
import { existsSync, lstatSync, readFileSync, writeFileSync, renameSync, mkdirSync, readdirSync, openSync, closeSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { randomBytes } from 'node:crypto';
import { digest, rejectLinks, readState, writePrivate } from './core.mjs';
import { run, verifyProvisioned } from './provision.mjs';
import { assertCandidateMetadata } from './release-core.mjs';
import { unlinkOwned } from '../../demo/runtime.mjs';

export async function withStartupLock(root, action) {
  const path = join(root, 'startup.lock'); rejectLinks(path);
  const fd = openSync(path, 'wx', 0o600), identity = lstatSync(path);
  try { return await action(); } finally { closeSync(fd); unlinkOwned(path, identity); }
}

export function assertRequestedRuntime(status, mode, identity) {
  assert.equal(status.mode, mode, 'Ready runtime mode differs from request');
  if (identity) {
    assert.equal(status.activeBinary?.binaryDigest, identity.binaryDigest);
    assert.equal(status.activeBinary?.manifestDigest, identity.manifestDigest);
  }
  return status;
}

export function verifyMigrationInventory(directory, expected) {
  rejectLinks(directory); assert.ok(lstatSync(directory).isDirectory());
  assert.deepEqual(readdirSync(directory).sort(), expected.map(file => file.name).sort(), 'Candidate migration file set changed');
  for (const file of expected) {
    assert.match(file.name, /^\d{6}_[A-Za-z0-9_]+\.(?:up|down)\.sql$/);
    assert.equal(digest(regularFile(join(directory, file.name))), file.digest, 'Candidate migration changed');
  }
}

export function regularFile(path) {
  rejectLinks(path);
  assert.ok(lstatSync(path).isFile(), 'Candidate input must be a regular file');
  return readFileSync(path);
}

export function verifyCandidate(repo, paths) {
  assert.equal(paths.length, 4, 'Manifest, archive, SBOM and checksums are required');
  const [manifestPath, archivePath, sbomPath, checksumsPath] = paths.map(value => resolve(value));
  const initial = new Map([manifestPath, archivePath, sbomPath, checksumsPath].map(path => [path, digest(regularFile(path))]));
  const manifest = JSON.parse(regularFile(manifestPath));
  const env = { PATH: process.env.PATH, HOME: process.env.HOME, GOWORK: 'off', GOENV: 'off', GOFLAGS: '', GOTOOLCHAIN: 'local', CGO_ENABLED: '0' };
  const sourceDigest = run(repo, 'go', ['run', './scripts/release/manifest.go', 'fingerprint', '-root', repo], { env }).trim();
  assertCandidateMetadata(manifest, { os: process.platform, arch: process.arch === 'x64' ? 'amd64' : process.arch, schema: 33, sourceDigest });
  const staged = dirname(manifestPath), executablePath = join(staged, manifest.executable);
  const bytes = regularFile(executablePath), binaryDigest = digest(bytes);
  assert.ok(lstatSync(executablePath).mode & 0o111, 'Candidate executable is not executable');
  const migrations = readdirSync(join(staged, 'migrations')).sort().map(name => {
    assert.match(name, /^\d{6}_[A-Za-z0-9_]+\.(?:up|down)\.sql$/);
    const bytes = regularFile(join(staged, 'migrations', name)); return { name, bytes, digest: digest(bytes) };
  });
  assert.equal(migrations.filter(file => file.name.startsWith('000033_')).length, 2);
  run(repo, 'go', ['run', './scripts/release/manifest.go', 'verify', '-manifest', manifestPath, '-sbom', sbomPath, '-checksums', checksumsPath, '-archive', archivePath, '-version', manifest.version, '-commit', manifest.commit], { env });
  for (const [path, expected] of initial) assert.equal(digest(regularFile(path)), expected, 'Candidate input changed during verification');
  assert.equal(digest(regularFile(executablePath)), binaryDigest, 'Candidate binary changed during verification');
  verifyMigrationInventory(join(staged, 'migrations'), migrations);
  return { bytes, migrations, identity: { mode: 'candidate', version: manifest.version, sourceDigest, sourceDirty: manifest.sourceDirty, schema: manifest.migrationVersion, binaryDigest, manifestDigest: initial.get(manifestPath), archiveDigest: initial.get(archivePath), verifiedAt: new Date().toISOString() } };
}

export function installCandidate(repo, root, candidate) {
  const state = readState(root); verifyProvisioned(repo, state);
  assert.ok(!existsSync(join(root, 'runtime-owner.json')), 'Stop owned runtime before installing verified candidate');
  const migrationRoot = join(root, 'candidate-migrations'); rejectLinks(migrationRoot);
  if (!existsSync(migrationRoot)) mkdirSync(migrationRoot, { mode: 0o700 });
  const migrationsPath = join(migrationRoot, candidate.identity.manifestDigest.slice(7)); rejectLinks(migrationsPath);
  if (existsSync(migrationsPath)) verifyMigrationInventory(migrationsPath, candidate.migrations);
  else mkdirSync(migrationsPath, { mode: 0o700 });
  for (const path of [migrationRoot, migrationsPath]) assert.ok(lstatSync(path).isDirectory() && !(lstatSync(path).mode & 0o077));
  for (const file of candidate.migrations) {
    const path = join(migrationsPath, file.name);
    if (existsSync(path)) assert.equal(digest(regularFile(path)), file.digest);
    else writeFileSync(path, file.bytes, { flag: 'wx', mode: 0o600 });
  }
  verifyMigrationInventory(migrationsPath, candidate.migrations);
  const binary = join(root, 'server'); rejectLinks(binary);
  if (existsSync(binary)) assert.ok(lstatSync(binary).isFile());
  const temporary = join(root, `candidate-${randomBytes(8).toString('hex')}.tmp`);
  writeFileSync(temporary, candidate.bytes, { flag: 'wx', mode: 0o700 });
  assert.equal(digest(regularFile(temporary)), candidate.identity.binaryDigest);
  renameSync(temporary, binary);
  const launch = { owner: state.owner, ...candidate.identity, migrationsPath, migrations: candidate.migrations.map(({ name, digest }) => ({ name, digest })) };
  writePrivate(join(root, 'runtime-launch.json'), launch);
  return launch;
}

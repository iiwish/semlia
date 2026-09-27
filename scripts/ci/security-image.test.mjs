import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { chmodSync, copyFileSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const repository = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

for (const mode of ['default', 'environment', 'make-variable']) {
  test(`security build and scanner use the same ${mode} image identity without Docker`, t => {
    const root = mkdtempSync(join(tmpdir(), 'semlia-security-image-'));
    t.after(() => rmSync(root, { recursive: true, force: true }));
    mkdirSync(join(root, 'scripts/ci'), { recursive: true });
    mkdirSync(join(root, 'bin'));
    copyFileSync(join(repository, 'Makefile'), join(root, 'Makefile'));
    copyFileSync(join(repository, 'scripts/ci/security-check.sh'), join(root, 'scripts/ci/security-check.sh'));
    const log = join(root, 'docker.jsonl');
    writeFileSync(join(root, 'bin/docker'), `#!${process.execPath}\nrequire('node:fs').appendFileSync(${JSON.stringify(log)}, JSON.stringify(process.argv.slice(2))+'\\n');\n`);
    chmodSync(join(root, 'bin/docker'), 0o755);
    const image = mode === 'default' ? 'semlia:security' : 'semlia:security-isolated-synthetic-owner';
    const env = { PATH: `${join(root, 'bin')}:${process.env.PATH}`, HOME: process.env.HOME };
    if (mode === 'environment') env.SEMLIA_SECURITY_IMAGE = image;
    const args = ['--no-print-directory', 'security-check'];
    if (mode === 'make-variable') args.push(`SEMLIA_SECURITY_IMAGE=${image}`);
    execFileSync('make', args, { cwd: root, env, stdio: 'pipe' });
    const calls = readFileSync(log, 'utf8').trim().split('\n').map(JSON.parse);
    const build = calls.find(args => args[0] === 'build');
    assert.equal(build[build.indexOf('--tag') + 1], image);
    const scan = calls.find(args => args[0] === 'run' && args.includes('image'));
    assert.equal(scan.at(-1), image);
    assert.ok(scan.includes('HIGH,CRITICAL'));
    assert.equal(scan[scan.indexOf('--exit-code') + 1], '1');
  });
}

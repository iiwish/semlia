import { createHash } from 'node:crypto';
import { existsSync, lstatSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { DDL, generate, renderSQL } from '../../examples/resume-demo/generate.mjs';
import { openState } from './state.mjs';

export const digest = value => 'sha256:' + createHash('sha256').update(value).digest('hex');

export function preparePackage(root) {
  const data = generate();
  const files = {
    'data.json': JSON.stringify(data, null, 2) + '\n',
    'source.sql': DDL,
    'source-data.sql': renderSQL(data),
    'expected.json': JSON.stringify(data.expected, null, 2) + '\n',
    'business-rules.md': readFileSync(new URL('../../examples/resume-demo/README.md', import.meta.url), 'utf8'),
  };
  const hashes = Object.fromEntries(Object.entries(files).map(([name, bytes]) => [name, digest(bytes)]));
  const state = openState(root, digest(JSON.stringify(hashes)));
  // Verify all existing inputs before writing any missing ones after interruption.
  for (const [name, bytes] of Object.entries(files)) {
    const path = join(root, name);
    if (existsSync(path) && (lstatSync(path).isSymbolicLink() || readFileSync(path, 'utf8') !== bytes)) throw new Error('Refusing modified package material: ' + name);
  }
  for (const [name, bytes] of Object.entries(files)) {
    const path = join(root, name);
    if (!existsSync(path)) writeFileSync(path, bytes, { flag: 'wx', mode: 0o600 });
  }
  return state;
}

import assert from 'node:assert/strict';
import { existsSync, mkdirSync, lstatSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, relative, resolve, dirname, isAbsolute, sep } from 'node:path';
import { rejectLinks, digest } from './core.mjs';

export function containedPath(root, path) {
  assert.ok(typeof path === 'string' && path && !isAbsolute(path) && !path.includes('\\'));
  const parts = path.split('/'); assert.ok(parts.every(part => part && part !== '.' && part !== '..'));
  const resolved = resolve(root, path);
  assert.ok(resolved.startsWith(resolve(root) + sep)); rejectLinks(resolved); return resolved;
}
export function fileInventory(directory) {
  const files = [];
  const visit = path => {
    rejectLinks(path); const stat = lstatSync(path);
    if (stat.isDirectory()) for (const name of readdirSync(path).sort()) visit(join(path, name));
    else {
      assert.ok(stat.isFile(), 'Unsupported backup entry');
      const name = relative(directory, path); containedPath(directory, name);
      files.push({ path: name, bytes: stat.size, digest: digest(readFileSync(path)) });
    }
  };
  visit(directory); return files;
}
export function copyStore(source, target, expected) {
  assert.deepEqual(fileInventory(source), expected, 'Backup source differs from inventory');
  rejectLinks(target); assert.ok(!existsSync(target) || lstatSync(target).isDirectory() && readdirSync(target).length === 0, 'Restore directory is not empty');
  if (!existsSync(target)) mkdirSync(target, { mode: 0o700 });
  for (const file of expected) {
    const from = containedPath(source, file.path), to = containedPath(target, file.path);
    const parts = relative(target, dirname(to)).split('/').filter(Boolean); let current = target;
    for (const part of parts) { current = join(current, part); rejectLinks(current); if (!existsSync(current)) mkdirSync(current, { mode: 0o700 }); }
    const bytes = readFileSync(from); assert.equal(bytes.length, file.bytes); assert.equal(digest(bytes), file.digest);
    writeFileSync(to, bytes, { flag: 'wx', mode: 0o600 });
  }
  assert.deepEqual(fileInventory(target), expected, 'Restored file bytes differ');
}

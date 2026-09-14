import { randomBytes } from 'node:crypto';
import { closeSync, existsSync, lstatSync, mkdirSync, openSync, readFileSync, renameSync, unlinkSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';

function rejectLinks(path) {
  for (let current = resolve(path); ; current = dirname(current)) {
    if (existsSync(current) && lstatSync(current).isSymbolicLink()) throw new Error('Refusing symbolic path');
    if (dirname(current) === current) break;
  }
}

function readState(root) {
  rejectLinks(root);
  const file = join(root, 'state.json');
  if (!existsSync(file)) throw new Error('Refusing unowned directory');
  rejectLinks(file);
  const state = JSON.parse(readFileSync(file, 'utf8'));
  if (state.format !== 1 || !/^semlia_demo_[a-f0-9]{16}$/.test(state.owner)) throw new Error('Invalid owner record');
  if (!Number.isSafeInteger(state.revision) || state.revision < 0 || typeof state.digest !== 'string') throw new Error('Invalid state record');
  if ((lstatSync(root).mode & 0o077) || (lstatSync(file).mode & 0o077)) throw new Error('Owner directory and record must be private');
  return state;
}

export function openState(root, digest) {
  rejectLinks(root);
  if (existsSync(root)) {
    const state = readState(root);
    if (state.digest !== digest) throw new Error('Input digest mismatch; refusing overwrite');
    return state;
  }
  mkdirSync(root, { mode: 0o700 });
  const state = { format: 1, owner: 'semlia_demo_' + randomBytes(8).toString('hex'), digest, revision: 0, phases: {} };
  writeFileSync(join(root, 'state.json'), JSON.stringify(state, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
  return state;
}

export function checkpoint(root, previous, update) {
  if (['format', 'owner', 'digest', 'revision'].some(key => Object.hasOwn(update, key))) throw new Error('Cannot replace state identity');
  rejectLinks(root);
  const lock = join(root, 'checkpoint.lock');
  const fd = openSync(lock, 'wx', 0o600);
  let temp;
  try {
    const current = readState(root);
    if (current.owner !== previous.owner || current.digest !== previous.digest || current.revision !== previous.revision) throw new Error('Refusing stale checkpoint');
    const next = { ...current, ...update, revision: current.revision + 1 };
    temp = join(root, `state-${randomBytes(8).toString('hex')}.tmp`);
    writeFileSync(temp, JSON.stringify(next, null, 2) + '\n', { flag: 'wx', mode: 0o600 });
    renameSync(temp, join(root, 'state.json'));
    return next;
  } finally {
    if (temp && existsSync(temp)) unlinkSync(temp);
    closeSync(fd);
    unlinkSync(lock);
  }
}

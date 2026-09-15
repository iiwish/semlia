import { randomUUID } from 'node:crypto';
import { existsSync, lstatSync, mkdirSync, readFileSync, renameSync, unlinkSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { DemoClient } from './client.mjs';

export async function connectActor(root, state, username, password, fetcher = fetch) {
  if (!/^semlia_demo_[0-9a-f]{16}$/.test(state.owner)) throw new Error('Invalid session owner');
  if (state.singleAdmin ? username !== 'admin' : !/^(showcase|workshop|rehearsal)_(admin|author|reviewer|publisher)$/.test(username)) throw new Error('Invalid demo actor');
  const origin = `http://127.0.0.1:${state.apiPort}`;
  const client = new DemoClient(origin, fetcher);
  const directory = join(root, 'sessions'), path = join(directory, username + '.json');
  if (!existsSync(directory)) mkdirSync(directory, { mode: 0o700 });
  const directoryInfo = lstatSync(directory);
  if (!directoryInfo.isDirectory() || (directoryInfo.mode & 0o077)) throw new Error('Unsafe session directory');
  if (existsSync(path)) {
    const info = lstatSync(path);
    if (!info.isFile() || (info.mode & 0o077)) throw new Error('Unsafe saved session');
    let saved;
    try { saved = JSON.parse(readFileSync(path, 'utf8')); } catch { throw new Error('Invalid saved session file'); }
    if (saved.owner !== state.owner || saved.username !== username || saved.origin !== origin) throw new Error('Saved session owner or origin mismatch');
    try { return { client, session: await client.resume(saved.cookie) }; }
    catch (error) { if (error.status !== 401) throw error; }
  }
  const session = await client.login(username, password);
  const temporary = join(directory, `.session-${randomUUID()}.tmp`);
  try {
    writeFileSync(temporary, JSON.stringify({ owner: state.owner, origin, username, cookie: client.sessionCookie() }), { mode: 0o600, flag: 'wx' });
    renameSync(temporary, path);
  } finally { if (existsSync(temporary)) unlinkSync(temporary); }
  return { client, session };
}

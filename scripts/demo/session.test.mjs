import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, realpathSync, rmSync, statSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { connectActor } from './session.mjs';

test('actor reentry revalidates its private normal session without another password login', async () => {
  const root = mkdtempSync(join(realpathSync(tmpdir()), 'semlia-session-'));
  const state = { owner: 'semlia_demo_0123456789abcdef', apiPort: 19000 };
  let logins = 0, reads = 0;
  const fetcher = async url => {
    if (url.endsWith('/login')) { logins++; return new Response(null, { status: 204, headers: { 'Set-Cookie': 'semlia_session_dev=normal-token; HttpOnly' } }); }
    reads++;
    return Response.json({ workspaces: [{ id: 'workspace', principalId: 'principal' }] }, { headers: { 'X-Semlia-CSRF': 'csrf' } });
  };
  try {
    await connectActor(root, state, 'showcase_author', 'synthetic-password', fetcher);
    await connectActor(root, state, 'showcase_author', 'synthetic-password', fetcher);
    assert.equal(logins, 1); assert.equal(reads, 2);
    assert.equal(statSync(join(root, 'sessions/showcase_author.json')).mode & 0o777, 0o600);
    await assert.rejects(connectActor(root, { ...state, owner: 'foreign' }, 'showcase_author', 'synthetic-password', fetcher), /owner/);
    await assert.rejects(connectActor(root, state, '../foreign', 'synthetic-password', fetcher), /actor/);
    const single = { ...state, singleAdmin: { username: 'admin' } };
    await connectActor(root, single, 'admin', '132435', fetcher);
    await connectActor(root, single, 'admin', '132435', fetcher);
    assert.equal(logins, 2);
    await assert.rejects(connectActor(root, single, 'showcase_author', 'synthetic-password', fetcher), /actor/);
  } finally { rmSync(root, { recursive: true, force: true }); }
});

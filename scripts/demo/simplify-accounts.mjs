import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { preparePackage } from './package.mjs';
import { provision, readSecrets } from './provision.mjs';
import { runtimeEnvironment } from './environment.mjs';
import { DemoClient } from './client.mjs';
import { checkpoint } from './state.mjs';

const repo = fileURLToPath(new URL('../../', import.meta.url)).replace(/\/$/, '');
const root = join(repo, '.semlia/resume-demo/retail-v1');
let state = provision(repo, root, preparePackage(root));
const secrets = readSecrets(root);
const env = runtimeEnvironment(repo, root, { ...state.database, owner: state.owner, apiPort: state.apiPort }, secrets);
const command = (args, password) => {
  const result = spawnSync(join(root, 'server'), args, { env, input: password + '\n', encoding: 'utf8', timeout: 30000 });
  if (result.status !== 0 || result.error) throw new Error('Account command failed');
};
const login = async (username) => {
  const client = new DemoClient(`http://127.0.0.1:${state.apiPort}`);
  const session = await client.login(username, '132435');
  return { client, session };
};
try {
  if (!state.singleAdmin) {
    const existing = await login('showcase_admin');
    const base = `/api/v1/workspaces/${state.workspaces.showcase.id}`;
    const members = await existing.client.request('GET', base + '/members');
    if (!members.items.some(x => x.displayName === 'admin')) await existing.client.request('POST', base + '/members', { username: 'admin', displayName: 'admin', password: '132435', roleId: 'workspace_admin' });
    state = checkpoint(root, state, { singleAdmin: { username: 'admin', retired: [] } });
  }
  const admin = await login('admin');
  assert.equal(admin.session.workspaces.length, 1);
  assert.equal(admin.session.workspaces[0].id, state.workspaces.showcase.id);
  for (const slug of ['showcase', 'workshop', 'rehearsal']) {
    if (state.singleAdmin?.retired?.includes(slug)) continue;
    const actor = slug === 'showcase' ? admin : await login(`${slug}_admin`);
    const base = `/api/v1/workspaces/${state.workspaces[slug].id}`;
    const members = await actor.client.request('GET', base + '/members');
    const self = actor.session.workspaces[0].principalId;
    for (const member of members.items.sort((a, b) => Number(a.principalId === self) - Number(b.principalId === self))) {
      if (slug === 'showcase' && member.principalId === self) continue;
      if (member.status === 'active') {
        try { await actor.client.request('PATCH', base + '/members/' + member.id, { status: 'suspended' }); }
        catch (error) {
          // Preserve the last-admin invariant in retired workspaces; revoke its
          // sessions and discard its random password below instead.
          if (slug === 'showcase' || member.principalId !== self || error.status !== 409) throw error;
        }
      }
    }
    for (const role of ['admin', 'author', 'reviewer', 'publisher']) command(['reset-local-password', `${slug}_${role}`], randomBytes(32).toString('hex'));
    state = checkpoint(root, state, { singleAdmin: { username: 'admin', retired: [...(state.singleAdmin?.retired ?? []), slug] } });
    console.log(`${slug}: legacy access retired; history preserved`);
  }
  const members = await admin.client.request('GET', `/api/v1/workspaces/${state.workspaces.showcase.id}/members`);
  assert.equal(members.items.filter(x => x.status === 'active').length, 1);
  console.log('admin login verified; one active demo membership');
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}

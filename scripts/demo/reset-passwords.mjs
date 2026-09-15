import { spawnSync } from 'node:child_process';
import { writeFileSync, renameSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { preparePackage } from './package.mjs';
import { provision, readSecrets } from './provision.mjs';
import { runtimeEnvironment } from './environment.mjs';
import { DemoClient } from './client.mjs';

process.umask(0o077);
const repo = fileURLToPath(new URL('../../', import.meta.url));
const root = join(repo, '.semlia/resume-demo/retail-v1');
try {
  const state = provision(repo.replace(/\/$/, ''), root, preparePackage(root));
  if (state.singleAdmin) throw new Error('Legacy account reset disabled for single-admin showcase');
  const secrets = readSecrets(root);
  const env = runtimeEnvironment(repo, root, { ...state.database, owner: state.owner, apiPort: state.apiPort }, secrets);
  for (const slug of ['showcase', 'workshop', 'rehearsal']) {
    for (const role of ['admin', 'author', 'reviewer', 'publisher']) {
      const username = `${slug}_${role}`;
      const result = spawnSync(join(root, 'server'), ['reset-local-password', username], { env, input: '132435\n', encoding: 'utf8', timeout: 30000 });
      if (result.status !== 0 || result.error) throw new Error('Account reset failed; rerun to finish remaining accounts');
      const client = new DemoClient(`http://127.0.0.1:${state.apiPort}`);
      await client.login(username, '132435');
      console.log(`${username}: reset and login verified`);
    }
  }
  const temp = join(root, 'secrets-password-reset.tmp');
  writeFileSync(temp, JSON.stringify({ ...secrets, accountPassword: '132435' }), { flag: 'wx', mode: 0o600 });
  renameSync(temp, join(root, 'secrets.json'));
} catch {
  console.error('Demo password reset incomplete; protected diagnostics withheld');
  process.exitCode = 1;
}

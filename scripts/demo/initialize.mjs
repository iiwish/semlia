import { existsSync, lstatSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { connectActor } from './session.mjs';
import { readSecrets } from './provision.mjs';
import { checkpoint } from './state.mjs';
import { DISCOVERY_DDL } from '../../examples/resume-demo/generate.mjs';
import { digest } from './package.mjs';

export async function registerDiscoveryArtifact(client, base, sourceId) {
  const source = await client.request('GET', base + `/sources/${sourceId}`);
  if (source.id !== sourceId || ![1, 2].includes(source.version)) throw new Error('Unexpected demo source revision');
  return client.request('POST', base + '/ingestion/sql-registrations', {
    sourceName: '合成零售 · 订单客户与退款', sourceId, expectedSourceVersion: 1, paths: ['retail-discovery.sql'],
  }, `resume-retail-v1-${sourceId}-discovery-artifact`);
}

export async function ensureBinding(client, workspace, principal, roleId) {
  const base = `/api/v1/workspaces/${workspace}/authorization`;
  const bindings = await client.request('GET', base + '/role-bindings');
  if (bindings.items.some(x => x.principalId === principal && x.roleId === roleId && x.status === 'active' && x.scope.type === 'workspace' && x.workspaceId === workspace)) return;
  const roles = await client.request('GET', base + '/roles');
  const role = roles.items.find(x => x.id === roleId);
  if (!role) throw new Error('Required server role missing');
  await client.request('POST', base + '/role-bindings', { principalId: principal, roleId, expectedRoleVersion: role.version, scope: { type: 'workspace', id: workspace } });
}

export async function initializeSources(root, initialState) {
  let state = initialState;
  const password = readSecrets(root).accountPassword;
  const sourceBytes = readFileSync(join(root, 'source.sql'), 'utf8');
  const target = join(root, 'inputs/retail.sql');
  if (existsSync(target)) {
    if (lstatSync(target).isSymbolicLink() || readFileSync(target, 'utf8') !== sourceBytes) throw new Error('Source artifact mismatch');
  } else writeFileSync(target, sourceBytes, { flag: 'wx', mode: 0o600 });
  const discoveryBytes = DISCOVERY_DDL + '-- Paid orders exclude pending/cancelled orders. Only succeeded refunds reduce net revenue.\n-- Aggregate refunds per order before joins; all content is synthetic.\n';
  const discoveryTarget = join(root, 'inputs/retail-discovery.sql');
  const discoveryDigest = digest(discoveryBytes);
  if (state.discoveryArtifactDigest && state.discoveryArtifactDigest !== discoveryDigest) throw new Error('Discovery artifact digest changed');
  if (existsSync(discoveryTarget)) {
    if (lstatSync(discoveryTarget).isSymbolicLink() || readFileSync(discoveryTarget, 'utf8') !== discoveryBytes) throw new Error('Discovery artifact mismatch');
  } else writeFileSync(discoveryTarget, discoveryBytes, { flag: 'wx', mode: 0o600 });
  if (!state.discoveryArtifactDigest) state = checkpoint(root, state, { discoveryArtifactDigest: discoveryDigest });
  const workspaces = { ...state.workspaces };
  for (const slug of ['showcase', 'workshop', 'rehearsal']) {
    const { client: admin, session } = await connectActor(root, state, `${slug}_admin`, password);
    const workspace = session.workspaces.find(x => x.slug === `resume-${slug}`);
    if (!workspace || session.workspaces.length !== 1) throw new Error('Unexpected admin workspace');
    if (workspaces[slug] && workspaces[slug].id !== workspace.id) throw new Error('Workspace identity changed');
    const base = `/api/v1/workspaces/${workspace.id}`;
    const principals = {};
    for (const [role, roleId, label] of [['author', 'semantic_steward', '作者'], ['reviewer', 'reviewer', '审核者'], ['publisher', 'publisher', '发布者']]) {
      const username = `${slug}_${role}`, displayName = `${slug} ${label} · 合成案例`;
      const members = await admin.request('GET', base + '/members');
      const found = members.items.filter(x => x.displayName === displayName);
      if (found.length > 1) throw new Error('Ambiguous demo member');
      if (!found.length) await admin.request('POST', base + '/members', { username, displayName, password, roleId });
      const { session: identity } = await connectActor(root, state, username, password);
      const membership = identity.workspaces.find(x => x.id === workspace.id);
      if (!membership || identity.workspaces.length !== 1) throw new Error('Demo member workspace mismatch');
      principals[role] = membership.principalId;
      await ensureBinding(admin, workspace.id, membership.principalId, role === 'author' ? 'source_operator' : 'auditor');
    }
    if (new Set(Object.values(principals)).size !== 3) throw new Error('Demo actors must be distinct');
    const source = await admin.request('POST', base + '/ingestion/sql-registrations', { sourceName: '合成零售 · 订单客户与退款', paths: ['retail.sql'] }, `resume-retail-v1-${slug}-source`);
    const currentArtifact = await registerDiscoveryArtifact(admin, base, source.sourceId);
    if (currentArtifact.sourceId !== source.sourceId || !currentArtifact.id) throw new Error('Discovery artifact source mismatch');
    workspaces[slug] = { ...workspaces[slug], id: workspace.id, principals, sourceId: source.sourceId, artifactSetId: currentArtifact.id };
    if (!source.sourceId || !source.id) throw new Error('Missing registered source identity');
    state = checkpoint(root, state, { workspaces });
  }
  return state;
}

import { setTimeout } from 'node:timers/promises';
import { connectActor } from './session.mjs';
import { readSecrets } from './provision.mjs';
import { checkpoint } from './state.mjs';

export async function allPages(client, path) {
  const items = [], seen = new Set();
  let cursor;
  do {
    const page = await client.request('GET', path + (cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''));
    items.push(...page.items);
    cursor = page.nextCursor ?? page.page?.nextCursor;
    if (cursor && seen.has(cursor)) throw new Error('Repeated API pagination cursor');
    seen.add(cursor);
    if (items.length > 10000) throw new Error('Demo API collection exceeds expected bounds');
  } while (cursor);
  return items;
}

export async function discoverSource(client, workspace, key) {
  const base = `/api/v1/workspaces/${workspace.id}`;
  const source = `${base}/sources/${workspace.sourceId}`;
  const started = await client.request('POST', source + '/discovery-runs', { idempotencyKey: key });
  if (started.sourceConnectionId !== workspace.sourceId) throw new Error('Discovery source mismatch');
  const deadline = Date.now() + 120000;
  let run;
  do {
    const runs = await allPages(client, source + '/discovery-runs?limit=100');
    run = runs.find(item => item.id === started.id);
    if (!run || run.sourceConnectionId !== workspace.sourceId) throw new Error('Exact discovery run missing');
    if (!['queued', 'running'].includes(run.status)) break;
    await setTimeout(500);
  } while (Date.now() < deadline);
  if (['queued', 'running'].includes(run.status)) throw new Error('Discovery still active; resume with the same key');
  if (run.status !== 'succeeded') throw new Error(`Discovery terminal ${run.status}; inspect normal operations view`);
  if (!run.snapshotId) throw new Error('Discovery has no immutable snapshot');
  const snapshot = await client.request('GET', source + `/snapshots/${run.snapshotId}`);
  if (snapshot.sourceId !== workspace.sourceId || snapshot.id !== run.snapshotId || snapshot.historyQuality !== 'verified' || snapshot.coverageStatus !== 'complete') throw new Error('Discovery snapshot is not complete and verified');
  const members = await allPages(client, source + `/snapshots/${snapshot.id}/members?limit=200`);
  const candidates = (await allPages(client, base + `/semantic-candidates?sourceId=${workspace.sourceId}&limit=100`)).filter(item => item.discoveryRunId === run.id);
  if (!members.length || !candidates.length) throw new Error('Discovery produced no source members or candidates');
  return { runId: run.id, snapshot, members, candidates };
}

export async function initializeDiscovery(root, initialState) {
  let state = initialState;
  const password = readSecrets(root).accountPassword;
  for (const slug of ['showcase', 'rehearsal']) {
    const workspace = state.workspaces?.[slug];
    if (!workspace?.sourceId) throw new Error('Initialize sources first');
    const { client, session } = await connectActor(root, state, `${slug}_author`, password);
    if (session.workspaces.length !== 1 || session.workspaces[0].id !== workspace.id || session.workspaces[0].principalId !== workspace.principals.author) throw new Error('Unexpected discovery author');
    const discovery = await discoverSource(client, workspace, `resume-retail-v1-${slug}-discovery-${workspace.artifactSetId}`);
    if (workspace.discovery && workspace.discovery.runId !== discovery.runId) throw new Error('Discovery identity changed');
    state = checkpoint(root, state, { workspaces: { ...state.workspaces, [slug]: { ...workspace, discovery } } });
  }
  return state;
}

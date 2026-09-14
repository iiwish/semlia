import assert from 'node:assert/strict';
import { allPages } from './discovery.mjs';
import { connectActor } from './session.mjs';
import { readSecrets } from './provision.mjs';
import { verifySource } from './source.mjs';

export async function verifyWorkshop(client, workspace) {
  const base = `/api/v1/workspaces/${workspace.id}`;
  for (const path of ['/production-operations', '/governance/releases', '/catalog/assets', `/sources/${workspace.sourceId}/discovery-runs`]) {
    const items = await allPages(client, base + path + '?limit=100');
    if (items.length) throw new Error('Workshop contains unexpected production or discovery history');
  }
  const sources = await allPages(client, base + '/sources?limit=100');
  assert.deepEqual(sources.map(x => x.id), [workspace.sourceId], 'Workshop source must be unique');
  return { assets: 0, releases: 0, operations: 0, discoveryRuns: 0, sources: 1 };
}

export async function verifyDemo(root, state) {
  const password = readSecrets(root).accountPassword;
  const report = { source: verifySource(root, state) };
  for (const slug of (state.singleAdmin ? ['showcase'] : ['showcase', 'workshop', 'rehearsal'])) {
    const workspace = state.workspaces?.[slug];
    if (!workspace) throw new Error('Missing demo workspace');
    const { client, session } = await connectActor(root, state, state.singleAdmin ? 'admin' : `${slug}_reviewer`, password);
    assert.equal(session.workspaces.length, 1);
    assert.equal(session.workspaces[0].id, workspace.id);
    if (!state.singleAdmin) assert.equal(session.workspaces[0].principalId, workspace.principals.reviewer);
    const read = { request: (method, ...args) => { assert.equal(method, 'GET', 'Verifier must not mutate business data'); return client.request(method, ...args); } };
    if (slug === 'workshop') { report[slug] = await verifyWorkshop(read, workspace); continue; }
    const base = `/api/v1/workspaces/${workspace.id}`;
    if (state.singleAdmin) {
      const membership = session.workspaces[0];
      assert.deepEqual(membership.roleIds, ['workspace_admin']);
      const roles = await read.request('GET', base + '/authorization/roles');
      const role = roles.items.find(x => x.id === 'workspace_admin');
      assert.ok(role);
      assert.deepEqual([...membership.capabilities].sort(), [...role.actions].sort());
      for (const action of ['asset.edit', 'proposal.review', 'release.publish', 'release.rollback', 'member.manage', 'source.manage']) assert.ok(membership.capabilities.includes(action));
      const members = await read.request('GET', base + '/members');
      assert.deepEqual(members.items.filter(x => x.status === 'active').map(x => x.principalId), [membership.principalId]);
      report.account = { username: 'admin', workspaces: 1, activeMembers: 1, completeAdminCapabilities: true };
    }
    assert.equal(new Set(Object.values(workspace.principals)).size, 3);
    const operations = await allPages(read, base + '/production-operations?limit=100');
    const expectedOperations = [workspace.baseline.attribution.operationId, workspace.correction.attribution.operationId];
    if (workspace.recoveryDraft) {
      assert.equal(slug, 'rehearsal');
      expectedOperations.push(workspace.recoveryDraft.operationId);
      const recovered = await read.request('GET', base + `/production-operations/${workspace.recoveryDraft.operationId}`);
      assert.equal(recovered.summary.progress, 'draft');
      assert.equal(recovered.activeValidation.status, 'not_requested');
      assert.equal(recovered.setDigest, workspace.recoveryDraft.setDigest);
      assert.equal(recovered.summary.createdBy, workspace.principals.author);
    }
    if (slug === 'showcase') {
      assert.ok(workspace.correctionDraft, 'Showcase needs a live correction draft');
      expectedOperations.push(workspace.correctionDraft.operationId);
      const draft = await read.request('GET', base + `/production-operations/${workspace.correctionDraft.operationId}`);
      assert.equal(draft.summary.progress, 'draft');
      assert.equal(draft.activeValidation.status, 'not_requested');
      assert.equal(draft.baselineHead.releaseId, workspace.rollback.id);
      assert.equal(draft.summary.createdBy, workspace.principals.author);
    }
    assert.deepEqual(operations.map(x => x.id).sort(), expectedOperations.sort(), 'No duplicate production operations');
    const releases = await allPages(read, base + '/governance/releases?limit=100');
    assert.deepEqual(releases.map(x => x.id), [workspace.rollback.id, workspace.correction.id, workspace.baseline.id], 'Exact three-release history');
    const current = {};
    for (const phase of ['baseline', 'correction', 'rollback']) {
      const release = await read.request('GET', base + `/production-releases/${workspace[phase].id}`);
      assert.deepEqual(release, workspace[phase], 'Persisted release must match committed receipt');
      assert.equal(release.publishedBy, workspace.principals.publisher);
      assert.equal(release.afterManifest.assets.length, 5);
      assert.equal(release.afterManifest.objects.length, 13);
      current[phase] = release;
      if (phase === 'rollback') continue;
      const operation = await read.request('GET', base + `/production-operations/${release.attribution.operationId}`);
      assert.equal(operation.summary.createdBy, workspace.principals.author);
      assert.equal(operation.summary.releaseId, release.id);
      assert.equal(operation.activeValidation.status, 'succeeded');
      assert.equal(operation.activeValidation.validationDigest, release.attribution.validation.validationDigest);
      assert.equal(operation.generationRunIds.length, 0, 'No model generation');
      for (const proposalId of release.attribution.proposalIds) {
        const reviews = await read.request('GET', base + `/governance/proposals/${proposalId}/reviews`);
        assert.ok(reviews.items.some(x => x.decision === 'approved' && x.reviewerPrincipalId === workspace.principals.reviewer && release.attribution.reviewIds.includes(x.id)), 'Every proposal needs attributable independent review');
        assert.ok(reviews.items.every(x => x.reviewerPrincipalId !== workspace.principals.author), 'No author self-review');
      }
      const confirmations = await read.request('GET', base + `/production-operations/${release.attribution.operationId}/business-rule-confirmations?version=1`);
      assert.equal(confirmations.items.length, phase === 'baseline' ? 5 : 1);
      assert.ok(confirmations.items.every(x => x.event.principalId === workspace.principals.author && x.event.evidenceOrigin === 'human_declaration'), 'Human declarations are attributable');
      if (phase === 'correction') {
        const attempts = await allPages(read, base + `/production-operations/${release.attribution.operationId}/validations?version=1&limit=100`);
        assert.equal(attempts.length, 2);
        const failed = attempts.find(x => x.attemptNo === 1);
        assert.equal(failed.status, 'failed');
        assert.ok(failed.checks.some(x => x.results.some(y => y.code === 'PRODUCTION_BUSINESS_RULE_UNCONFIRMED')));
        assert.equal(attempts.find(x => x.attemptNo === 2).status, 'succeeded');
      }
    }
    assert.equal(current.rollback.rolledBackReleaseId, current.correction.id);
    assert.deepEqual(current.rollback.afterManifest, current.baseline.afterManifest);
    const assets = await allPages(read, base + '/catalog/assets?limit=100');
    assert.equal(assets.length, 5);
    for (const pin of current.rollback.afterManifest.assets) {
      assert.equal(assets.find(x => x.id === pin.assetId)?.currentRevisionId, pin.revisionId, 'Live catalog must point to restored revision');
    }
    const snapshot = await read.request('GET', base + `/sources/${workspace.sourceId}/snapshots/${workspace.discovery.snapshot.id}`);
    assert.equal(snapshot.historyQuality, 'verified'); assert.equal(snapshot.coverageStatus, 'complete');
    assert.equal(snapshot.contentDigest, workspace.discovery.snapshot.contentDigest);
    report[slug] = { assets: 5, objects: 13, releases: 3, correctionAttempts: 2, rollbackRestored: true, independentReviews: true };
  }
  return report;
}

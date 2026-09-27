import assert from 'node:assert/strict';

export function assertRestoreTarget(actual, expected) {
  assert.match(expected.database, /^semlia_v1r_[a-f0-9]{16}_(?:install|upgrade|restore)$/);
  assert.equal(expected.role, expected.database + '_owner');
  assert.ok(expected.marker);
  for (const key of ['database', 'role', 'marker']) assert.equal(actual[key], expected[key]);
  for (const key of ['superuser', 'createdb', 'createrole', 'replication', 'bypassrls']) assert.equal(actual[key], false);
  assert.equal(actual.login, true); assert.equal(actual.memberships, 0); assert.equal(actual.empty, true);
}

export function assertBackupInventory(inventory) {
  for (const name of ['content', 'inputs', 'artifacts']) {
    assert.ok(Array.isArray(inventory[name]) && inventory[name].length > 0, 'Backup store is empty');
    assert.ok(inventory[name].reduce((sum, file) => sum + file.bytes, 0) > 0, 'Backup has no content bytes');
    for (const file of inventory[name]) assert.ok(file.path && file.digest && Number.isSafeInteger(file.bytes) && file.bytes >= 0);
  }
  for (const name of ['secretKey', 'accountPassword', 'readerPassword', 'modelSecret']) assert.equal(inventory.keys[name], true, 'Recovery key material is missing');
}

export function assertCandidateMetadata(manifest, expected) {
  assert.equal(manifest.formatVersion, 1); assert.equal(manifest.product, 'Semlia');
  assert.match(manifest.version, /^[0-9A-Za-z][0-9A-Za-z._+-]*$/); assert.match(manifest.commit, /^[a-f0-9]{40}$/);
  assert.deepEqual(manifest.target, { os: expected.os, arch: expected.arch });
  assert.equal(manifest.executable, expected.os === 'windows' ? 'semlia.exe' : 'semlia');
  assert.equal(manifest.apiVersion, 'v1'); assert.equal(manifest.migrationVersion, expected.schema);
  assert.match(manifest.sourceDigest, /^sha256:[a-f0-9]{64}$/); assert.equal(manifest.sourceDigest, expected.sourceDigest);
  assert.equal(manifest.artifactKind, 'local_candidate'); assert.equal(manifest.acceptance, 'unreviewed');
}

export function compareChannelProofs(proofs) {
  assert.equal(proofs.length, 3);
  const project = value => ({ releaseId: value.releaseId, model: value.model, planDigest: value.planDigest, resultDigest: value.resultDigest });
  for (const proof of proofs) {
    assert.ok(proof.releaseId && proof.model?.assetId && proof.model?.revisionId && proof.planDigest && proof.resultDigest);
    assert.deepEqual(project(proof), project(proofs[0]));
  }
}

export function assertCLIOutcome(result, revoked = false) {
  assert.equal(result.spawned, true); assert.equal(result.signal, null);
  assert.equal(result.exit, revoked ? 1 : 0);
  if (revoked) { assert.equal(result.httpStatus, 401); assert.equal(result.body?.code, 'UNAUTHENTICATED'); }
}

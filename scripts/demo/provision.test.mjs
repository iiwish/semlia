import assert from 'node:assert/strict';
import { test } from 'node:test';
import { validateRole, validateDatabase } from './provision.mjs';
test('existing role must carry exact owner marker and no elevated privileges', () => {
  const role = { marker: 'owner', superuser: false, createdb: false, createrole: false, replication: false, bypassrls: false, memberships: 0, login: true };
  assert.doesNotThrow(() => validateRole(role, 'owner'));
  assert.throws(() => validateRole({ ...role, marker: null }, 'owner'), /ownership/);
  for (const key of ['superuser', 'createdb', 'createrole', 'replication', 'bypassrls']) assert.throws(() => validateRole({ ...role, [key]: true }, 'owner'), /privileges/);
  assert.throws(() => validateRole({ ...role, memberships: 1 }, 'owner'), /privileges/);
});
test('database cannot be adopted from another role or owner marker', () => {
  assert.doesNotThrow(() => validateDatabase({ owner: 'role', marker: 'owner' }, 'role', 'owner'));
  assert.throws(() => validateDatabase({ owner: 'postgres', marker: 'owner' }, 'role', 'owner'), /ownership/);
  assert.throws(() => validateDatabase({ owner: 'role', marker: 'other' }, 'role', 'owner'), /ownership/);
});

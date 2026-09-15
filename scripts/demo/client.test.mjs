import assert from 'node:assert/strict';
import { test } from 'node:test';
import { DemoClient } from './client.mjs';

test('client uses normal password session and CSRF and never follows redirects', async () => {
  const calls = [];
  const fetcher = async (url, options) => {
    calls.push({ url, ...options });
    if (url.endsWith('/auth/password/login')) return new Response(null, { status: 204, headers: { 'set-cookie': 'semlia_session_dev=test-session; HttpOnly; Path=/; SameSite=Lax' } });
    if (url.endsWith('/api/v1/session')) return Response.json({ workspaces: [] }, { headers: { 'X-Semlia-CSRF': 'test-csrf' } });
    return Response.json({ id: 'created' }, { status: 201 });
  };
  const client = new DemoClient('http://127.0.0.1:19000', fetcher);
  await client.login('author', 'synthetic-password');
  assert.equal(calls[0].url, 'http://127.0.0.1:19000/api/v1/auth/password/login');
  await client.request('POST', '/api/v1/workspaces/test/production-operations', { name: 'example' }, 'demo-key');
  assert.equal(calls[0].headers.Origin, 'http://127.0.0.1:19000');
  assert.equal(calls[2].headers.Cookie, 'semlia_session_dev=test-session');
  assert.equal(calls[2].headers['X-Semlia-CSRF'], 'test-csrf');
  assert.equal(calls[2].headers['Idempotency-Key'], 'demo-key');
  assert.ok(calls.every(x => x.redirect === 'error'));
  assert.ok(calls.every(x => !Object.hasOwn(x.headers, 'X-Semlia-Principal')));
});

test('client rejects external origins, external paths and unauthenticated writes', async () => {
  assert.throws(() => new DemoClient('https://example.com'), /loopback/);
  const client = new DemoClient('http://127.0.0.1:19000', async () => { throw new Error('must not call'); });
  await assert.rejects(client.request('POST', '/api/v1/workspaces', {}), /session/);
  await assert.rejects(client.request('GET', '//example.com'), /path/);
});

test('HTTP failures omit server diagnostics and credentials', async () => {
  const client = new DemoClient('http://127.0.0.1:19000', async () => Response.json({ message: 'secret-data', code: 'SQL password=leak' }, { status: 500 }));
  await assert.rejects(client.request('GET', '/api/v1/session'), error => error.message === 'Demo API request failed (HTTP 500)');
});

test('saved normal session is revalidated and obtains a fresh CSRF token', async () => {
  const calls = [];
  const client = new DemoClient('http://127.0.0.1:19000', async (url, options) => {
    calls.push({ url, options });
    return Response.json({ workspaces: [] }, { headers: { 'X-Semlia-CSRF': 'fresh-csrf' } });
  });
  await client.resume('semlia_session_dev=normal-token');
  await client.request('POST', '/api/v1/example', {});
  assert.equal(calls[0].options.method, 'GET');
  assert.equal(calls[0].options.headers.Cookie, 'semlia_session_dev=normal-token');
  assert.equal(calls[1].options.headers['X-Semlia-CSRF'], 'fresh-csrf');
  assert.equal(client.sessionCookie(), 'semlia_session_dev=normal-token');
  await assert.rejects(client.resume('semlia_session_dev=x; forged=y'), /Invalid saved session/);
});

export class DemoClient {
  #origin;
  #fetch;
  #cookie = '';
  #csrf = '';

  constructor(origin, fetcher = fetch) {
    const url = new URL(origin);
    if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !url.port || url.username || url.password || url.pathname !== '/' || url.search || url.hash) throw new Error('Demo API requires explicit loopback origin');
    this.#origin = url.origin;
    this.#fetch = fetcher;
  }

  async login(username, password) {
    this.#cookie = ''; this.#csrf = '';
    const response = await this.#send('POST', '/api/v1/auth/password/login', { username, password });
    const cookie = response.headers.getSetCookie().find(value => value.startsWith('semlia_session_dev='));
    if (!cookie) throw new Error('Missing normal login session');
    return this.resume(cookie.split(';')[0]);
  }

  sessionCookie() { return this.#cookie; }

  async resume(cookie) {
    this.#cookie = ''; this.#csrf = '';
    if (!/^semlia_session_dev=[A-Za-z0-9_-]{1,2048}$/.test(cookie)) throw new Error('Invalid saved session');
    this.#cookie = cookie;
    const session = await this.#send('GET', '/api/v1/session');
    this.#csrf = session.headers.get('X-Semlia-CSRF') ?? '';
    if (!this.#csrf) throw new Error('Missing session CSRF verifier');
    return session.json();
  }

  async request(method, path, body, key) {
    if (method !== 'GET' && (!this.#cookie || !this.#csrf)) throw new Error('Normal session required');
    const response = await this.#send(method, path, body, key);
    return response.status === 204 ? null : response.json();
  }

  async #send(method, path, body, key) {
    if (!path.startsWith('/') || path.startsWith('//') || path.includes('\\') || new URL(path, this.#origin).origin !== this.#origin) throw new Error('Invalid API path');
    const headers = { Origin: this.#origin, Accept: 'application/json' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    if (this.#cookie) headers.Cookie = this.#cookie;
    if (this.#csrf && method !== 'GET') headers['X-Semlia-CSRF'] = this.#csrf;
    if (key) headers['Idempotency-Key'] = key;
    let response;
    try { response = await this.#fetch(this.#origin + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), redirect: 'error', signal: AbortSignal.timeout(30000) }); }
    catch { throw new Error('Demo API transport failed; verify server state before retrying writes'); }
    if (!response.ok) {
      await response.body?.cancel();
      const error = new Error(`Demo API request failed (HTTP ${response.status})`);
      error.status = response.status;
      throw error;
    }
    return response;
  }
}

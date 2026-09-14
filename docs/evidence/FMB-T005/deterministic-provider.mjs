// Synthetic transport fixture only; not an external model or semantic-quality proof.
import http from 'node:http';
http.createServer(async (req, res) => {
  if (req.url !== '/v1/embeddings' || req.method !== 'POST' || req.headers.authorization !== 'Bearer deterministic-local-qa-only') {
    res.writeHead(401).end(); return;
  }
  let body = '';
  for await (const part of req) { body += part; if(body.length > 131072) { res.writeHead(413).end(); return; } }
  const parsed = JSON.parse(body);
  const data = parsed.input.map((text, index) => ({index, embedding: [1, 1 + text.length % 7]}));
  setTimeout(() => { res.writeHead(200, {'Content-Type':'application/json'}); res.end(JSON.stringify({model:parsed.model,data})); }, Number(process.env.EMBEDDING_FIXTURE_DELAY_MS || 100));
}).listen(18084, '127.0.0.1', () => process.stdout.write('Synthetic embedding transport listening on 127.0.0.1:18084\n'));

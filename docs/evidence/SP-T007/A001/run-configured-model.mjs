import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { execFileSync, spawn } from 'node:child_process';

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), 'model');
const repo = path.resolve(root, '../../../../..');
if (fs.readFileSync(path.join(root, '.owner'), 'utf8').trim() !== 'SP-T007') throw new Error('Unowned evidence directory');
for (const name of fs.readdirSync(root).filter(name => /^call-\d+\.reserved$/.test(name))) {
  if (!fs.existsSync(path.join(root, name.replace('.reserved', '-result.json')))) throw new Error('A reserved call has no known outcome; do not retry');
}
for (const name of fs.readdirSync(root).filter(name => /^call-\d+-result\.json$/.test(name))) {
  if (JSON.parse(fs.readFileSync(path.join(root, name), 'utf8')).status === 'outcome_unknown') throw new Error('Unresolved prior model outcome; do not retry');
}
const lock = path.join(root, '.runner-lock');
const fd = fs.openSync(lock, 'wx', 0o600);
fs.closeSync(fd);
try {
  const config = JSON.parse(execFileSync('docker', ['exec', 'semlia-local-postgres-1', 'psql', '-U', 'semlia', '-d', 'semlia', '-X', '-At', '-v', 'ON_ERROR_STOP=1', '-c', 'SELECT json_build_object($$url$$,p.base_url,$$credential$$,p.credential_env,$$model$$,s.model,$$context$$,s.token_limit) FROM model_providers p JOIN model_settings s ON s.provider_id=p.id WHERE p.enabled AND s.enabled AND s.kind=$$llm$$ AND s.is_default'], { encoding: 'utf8' }));
  const entry = JSON.parse(execFileSync('docker', ['inspect', 'semlia-local-server-1'], { encoding: 'utf8' }))[0].Config.Env.find(value => value.startsWith(config.credential + '='));
  if (!entry) throw new Error('Configured credential unavailable');
  const secret = entry.slice(config.credential.length + 1);
  const env = { ...process.env, SEMLIA_RUN_PRODUCTION_MODEL: '1', SEMLIA_ACCEPTANCE_MODEL_URL: config.url, SEMLIA_ACCEPTANCE_MODEL_NAME: config.model, SEMLIA_ACCEPTANCE_MODEL_SECRET: secret, SEMLIA_ACCEPTANCE_MODEL_CONTEXT: String(config.context), SEMLIA_MODEL_EVIDENCE_ROOT: root };
  const child = spawn('go', ['test', '-count=1', '-timeout=10m', './tests/integration/governance', '-run', '^TestSemanticProductionConfiguredModel$', '-v'], { cwd: repo, env, stdio: ['ignore', 'pipe', 'pipe'] });
  let output = '';
  child.stdout.on('data', bytes => { output += bytes; });
  child.stderr.on('data', bytes => { output += bytes; });
  const code = await new Promise((resolve, reject) => { child.on('close', resolve); child.on('error', reject); });
  output = output.split(secret).join('[REDACTED]');
  fs.writeFileSync(path.join(root, `run-${Date.now()}.log`), output, { flag: 'wx', mode: 0o600 });
  process.stdout.write(output);
  process.exitCode = code ?? 1;
} catch {
  process.stderr.write('Configured model runner failed; inspect retained safe evidence before retrying.\n');
  process.exitCode = 1;
} finally {
  fs.unlinkSync(lock);
}

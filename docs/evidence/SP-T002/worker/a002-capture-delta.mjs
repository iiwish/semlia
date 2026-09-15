import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';

const name = process.argv[2];
if (!name || !/^a002-[a-z0-9-]+$/.test(name)) throw new Error('Expected a fresh a002 evidence name');
const checkout = JSON.parse(fs.readFileSync('docs/evidence/SP-T002/checkout.json', 'utf8'));
const prior = JSON.parse(fs.readFileSync('docs/evidence/SP-T002/worker/final-delta-03.json', 'utf8'));
const result = spawnSync('ruby', ['-rjson', '-ryaml', '-e', 'puts YAML.load_file(ARGV[0]).to_json', 'docs/specs/semantic-production/packets/SP-T002.yaml'], { encoding: 'utf8' });
if (result.status !== 0) throw new Error(result.stderr);
const packet = JSON.parse(result.stdout);
const matches = (file, pattern) => pattern.endsWith('/') ? file.startsWith(pattern) : new RegExp(`^${pattern.split('*').map(part => part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('[^/]*')}$`).test(file);
const externalOwners = new Map([
  ['docs/evidence/SP-T001/summary.md', 'orchestrator-t001-acceptance'],
  ['docs/specs/semantic-production/packets/SP-T001.yaml', 'orchestrator-t001-acceptance'],
  ['docs/specs/semantic-production/packets/SP-T003.yaml', 'orchestrator-t003-planning'],
]);
const ownerOf = file => packet.allowed_files.some(pattern => matches(file, pattern)) ? 'worker'
  : packet.orchestrator_allowed_files.some(pattern => matches(file, pattern)) ? 'orchestrator'
    : externalOwners.get(file) ?? 'outside-packet';
const hash = bytes => crypto.createHash('sha256').update(bytes).digest('hex');
const baseline = new Map(checkout.manifest.map(item => [item.path, item]));
const priorImplementation = new Map(prior.implementation.map(item => [item.path, item]));
const git = spawnSync('git', ['ls-files', '-co', '--exclude-standard', '-z'], { encoding: 'utf8', maxBuffer: 20 << 20 });
if (git.status !== 0) throw new Error(git.stderr);
const files = [...new Set([...baseline.keys(), ...git.stdout.split('\0').filter(Boolean)])].sort();
const current = new Map();
const contextChanges = [];
for (const file of files) {
  let bytes;
  try { bytes = fs.readFileSync(file); } catch (error) { if (error.code !== 'ENOENT') throw error; }
  const sha256 = bytes ? hash(bytes) : null;
  current.set(file, { bytes, sha256 });
  if (sha256 === (baseline.get(file)?.sha256 ?? null)) continue;
  contextChanges.push({ path: file, owner: ownerOf(file), beforeSha256: baseline.get(file)?.sha256 ?? null, afterSha256: sha256, bytes: bytes?.length ?? null });
}

const buildDelta = mode => {
  const implementation = [];
  let diffText = '';
  for (const file of files) {
    if (ownerOf(file) !== 'worker' || file.startsWith('docs/evidence/')) continue;
    const fromPrior = mode === 'a001' && priorImplementation.has(file);
    const beforeSha256 = fromPrior ? priorImplementation.get(file).afterSha256 : baseline.get(file)?.sha256 ?? null;
    const after = current.get(file);
    if (beforeSha256 === after.sha256) continue;
    const beforePath = beforeSha256 ? path.join(fromPrior ? '.semlia/evidence-work/SP-T002-A001' : checkout.baselineCopy, file) : '/dev/null';
    if (beforeSha256 && hash(fs.readFileSync(beforePath)) !== beforeSha256) throw new Error(`Frozen ${mode} hash mismatch: ${file}`);
    const delta = spawnSync('diff', ['-u', '--label', `a/${file}`, '--label', `b/${file}`, beforePath, after.bytes ? file : '/dev/null'], { encoding: 'utf8', maxBuffer: 100 << 20 });
    if (delta.status !== 1) throw new Error(`Unexpected diff status ${file}: ${delta.status} ${delta.stderr}`);
    implementation.push({ path: file, beforeSha256, afterSha256: after.sha256, bytes: after.bytes?.length ?? null });
    diffText += delta.stdout;
  }
  return { implementation, diffText };
};
const directory = 'docs/evidence/SP-T002/worker';
const outsidePacket = contextChanges.filter(item => item.owner === 'outside-packet');
for (const mode of ['baseline', 'a001']) {
  const delta = buildDelta(mode);
  const output = { capturedAt: new Date().toISOString(), attempt: 'SP-T002-A002', implementationBaseline: mode, checkoutCapturedAt: checkout.capturedAt, head: checkout.head, implementation: delta.implementation, contextChangesVersusCheckout: contextChanges, outsidePacket };
  fs.writeFileSync(path.join(directory, `${name}-${mode}.json`), JSON.stringify(output, null, 2), { flag: 'wx' });
  fs.writeFileSync(path.join(directory, `${name}-${mode}.diff`), delta.diffText, { flag: 'wx' });
  console.log(JSON.stringify({ mode, implementationFiles: delta.implementation.length, paths: delta.implementation.map(item => item.path), outsidePacket }, null, 2));
}
if (outsidePacket.length) process.exitCode = 1;

import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { spawnSync } from 'node:child_process';

const name = process.argv[2];
if (!name || !/^[a-z0-9-]+$/.test(name)) throw new Error('Expected a unique evidence name');
const root = process.cwd();
const checkout = JSON.parse(fs.readFileSync('docs/evidence/SP-T002/checkout.json', 'utf8'));
const packetResult = spawnSync('ruby', ['-rjson', '-ryaml', '-e', 'puts YAML.load_file(ARGV[0]).to_json', 'docs/specs/semantic-production/packets/SP-T002.yaml'], { encoding: 'utf8' });
if (packetResult.status !== 0) throw new Error(packetResult.stderr);
const packet = JSON.parse(packetResult.stdout);
// PM confirmed these metadata-only T001 acceptance updates after this checkout.
const externalOwners = new Map([
  ['docs/evidence/SP-T001/summary.md', 'orchestrator-t001-acceptance'],
  ['docs/specs/semantic-production/packets/SP-T001.yaml', 'orchestrator-t001-acceptance'],
]);
const matches = (file, pattern) => pattern.endsWith('/') ? file.startsWith(pattern) : new RegExp(`^${pattern.split('*').map(part => part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('[^/]*')}$`).test(file);
const git = spawnSync('git', ['ls-files', '-co', '--exclude-standard', '-z'], { encoding: 'utf8', maxBuffer: 20 << 20 });
if (git.status !== 0) throw new Error(git.stderr);
const baseline = new Map(checkout.manifest.map(item => [item.path, item]));
const files = [...new Set([...baseline.keys(), ...git.stdout.split('\0').filter(Boolean)])].sort();
const changes = [];
let implementationDiff = '';
for (const file of files) {
  const before = baseline.get(file);
  let bytes;
  try { bytes = fs.readFileSync(file); } catch (error) { if (error.code !== 'ENOENT') throw error; }
  const sha256 = bytes && crypto.createHash('sha256').update(bytes).digest('hex');
  if (sha256 === before?.sha256) continue;
  const owner = packet.allowed_files.some(pattern => matches(file, pattern)) ? 'worker'
    : packet.orchestrator_allowed_files.some(pattern => matches(file, pattern)) ? 'orchestrator' : externalOwners.get(file) ?? 'outside-packet';
  const record = { path: file, owner, beforeSha256: before?.sha256 ?? null, afterSha256: sha256 ?? null, bytes: bytes?.length ?? null };
  changes.push(record);
  if (owner !== 'worker' || file.startsWith('docs/evidence/')) continue;
  const oldPath = before?.sha256 ? path.join(checkout.baselineCopy, file) : '/dev/null';
  if (before?.sha256) {
    const oldHash = crypto.createHash('sha256').update(fs.readFileSync(oldPath)).digest('hex');
    if (oldHash !== before.sha256) throw new Error(`Baseline copy hash mismatch: ${file}`);
  }
  const diff = spawnSync('diff', ['-u', '--label', `a/${file}`, '--label', `b/${file}`, oldPath, bytes ? file : '/dev/null'], { encoding: 'utf8', maxBuffer: 100 << 20 });
  if (diff.status !== 1) throw new Error(`Unexpected diff status for ${file}: ${diff.status} ${diff.stderr}`);
  implementationDiff += diff.stdout;
}
const implementation = changes.filter(item => item.owner === 'worker' && !item.path.startsWith('docs/evidence/'));
const output = { capturedAt: new Date().toISOString(), checkoutCapturedAt: checkout.capturedAt, head: checkout.head, implementation, changes, outsidePacket: changes.filter(item => item.owner === 'outside-packet') };
const directory = path.join(root, 'docs/evidence/SP-T002/worker');
fs.writeFileSync(path.join(directory, `${name}.json`), JSON.stringify(output, null, 2), { flag: 'wx' });
fs.writeFileSync(path.join(directory, `${name}.diff`), implementationDiff, { flag: 'wx' });
console.log(JSON.stringify({ implementationFiles: implementation.length, outsidePacket: output.outsidePacket, paths: implementation.map(item => item.path) }, null, 2));
if (output.outsidePacket.length) process.exitCode = 1;

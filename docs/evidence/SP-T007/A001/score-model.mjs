import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const read = name => JSON.parse(fs.readFileSync(path.join(here, name), 'utf8'));
const gold = read('../../../../tests/fixtures/semantic-production/commerce.gold.json');
const input = read('model/call-04-input.json').payload.targets;
const raw = read('model/call-04-result.json').output.targets;
const corrected = read('model/call-04-applied-operation.json').targets.map(target => target.declaration);
const published = read('model/call-04-release.json').publishedBusinessContent;
const canonical = value => JSON.stringify(Array.isArray(value) ? value.map(value => JSON.parse(canonical(value))) : value && typeof value === 'object' ? Object.fromEntries(Object.keys(value).sort().map(key => [key, JSON.parse(canonical(value[key]))])) : value);
const byKey = targets => new Map(targets.map(target => [target.localKey, target]));
const inputByKey = byKey(input);
function refs(value, result = []) {
  if (!value || typeof value !== 'object') return result;
  if (value.snapshotId && value.objectId && value.revisionId) result.push(canonical(value));
  for (const child of Object.values(value)) refs(child, result);
  return result.sort();
}
function score(targets) {
  const map = byKey(targets);
  const checks = {
    completeTenTargets: targets.length === 10 && canonical([...map.keys()].sort()) === canonical([...inputByKey.keys()].sort()),
    conservedIdentity: targets.every(target => ['kind', 'intent', 'identityKey'].every(key => target[key] === inputByKey.get(target.localKey)?.[key])),
    conservedPhysicalPins: canonical(refs(targets)) === canonical(refs(input)),
    structuralEntities: ['orders', 'customers'].every(key => typeof map.get(key)?.content.definition === 'string' && map.get(key).content.definition.toLowerCase().includes(key.slice(0, -1))),
    exactEntityKeys: ['orders_key', 'customers_key'].every(key => canonical(map.get(key)?.content) === canonical(inputByKey.get(key)?.content)),
    exactGrainAndJoin: map.get('revenue_grain')?.content.expression === gold.grain && map.get('orders_customers_join')?.content.expression === gold.join,
    paidOnlyRevenueDefinition: map.get('revenue')?.content.definition === gold.revenue.definition && map.get('revenue')?.content.scope === gold.revenue.scope,
    paidOnlyExecutableTransform: map.get('revenue_binding')?.content.transform === gold.transform,
  };
  return { checks, passed: Object.values(checks).filter(Boolean).length, total: Object.keys(checks).length };
}
const report = { method: 'Fixed synthetic gold comparison, not model self-evaluation; corrected score uses server-read declarations, not the submitted request.', raw: score(raw), humanCorrected: score(corrected), publishedBusinessMatchesGold: published.revenue.definition === gold.revenue.definition && published.revenue.scope === gold.revenue.scope && published.transform === gold.transform, rawMissingPolicyExplicitlyNull: raw.find(target => target.localKey === 'revenue').content.definition === null };
fs.writeFileSync(path.join(here, 'model-quality.json'), JSON.stringify(report, null, 2) + '\n');
console.log(JSON.stringify(report, null, 2));
if (report.humanCorrected.passed !== report.humanCorrected.total || !report.publishedBusinessMatchesGold) process.exitCode = 1;

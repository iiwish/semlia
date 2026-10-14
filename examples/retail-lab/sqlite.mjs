// Build a portable SQLite copy of Retail Lab from db/schema.sql and db/seed.sql.
// Schemas are flattened: retail.orders -> orders, legacy.rpt_daily_sales -> legacy_rpt_daily_sales.
// Timestamps stay as '+08' text so lexical comparison matches time order; booleans become 0/1.
import { spawnSync } from 'node:child_process';
import { readFileSync, rmSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const flatten = sql => sql.replaceAll('retail.', '').replaceAll('legacy.', 'legacy_');

export function sqliteSchema() {
  return flatten(readFileSync(join(here, 'db/schema.sql'), 'utf8'))
    .replace(/^CREATE SCHEMA .*$/gm, '')
    .replace(/^COMMENT ON [\s\S]*?;$/gm, '')
    .replace(/\bbigserial\b/g, 'INTEGER')
    .replace(/\btimestamptz\b/g, 'TEXT')
    .replace(/\bnumeric\(\d+,\d+\)/g, 'NUMERIC')
    .replace(/\bboolean\b/g, 'INTEGER')
    .replace(/\bdate\b(?= NOT NULL| PRIMARY|,|\n)/g, 'TEXT');
}

export function sqliteSeed() {
  return flatten(readFileSync(join(here, 'db/seed.sql'), 'utf8'))
    .replace(/^SET LOCAL .*$/gm, '')
    .replace(/^SELECT setval\(.*$/gm, '')
    .replace(/, true(?=[,)])/g, ', 1').replace(/, false(?=[,)])/g, ', 0');
}

export function build(target) {
  rmSync(target, { force: true });
  const result = spawnSync('sqlite3', [target], { input: `PRAGMA foreign_keys = ON;\n${sqliteSchema()}\n${sqliteSeed()}\nANALYZE;\nVACUUM;\n`, encoding: 'utf8', maxBuffer: 64 << 20 });
  if (result.error || result.status !== 0) throw new Error('sqlite3 failed: ' + (result.stderr || result.error?.message));
  const counts = spawnSync('sqlite3', [target, "SELECT 'orders=' || count(*) FROM orders; SELECT 'order_items=' || count(*) FROM order_items; SELECT 'refunds=' || count(*) FROM refunds;"], { encoding: 'utf8' }).stdout.trim().split('\n').join(', ');
  console.log(`Built ${target} (${counts}).`);
}

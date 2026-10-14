// Build a portable SQLite copy of Retail Lab from db/schema.sql and db/seed.sql.
// Schemas are flattened: retail.orders -> orders, legacy.rpt_daily_sales -> legacy_rpt_daily_sales.
// Declared types keep their PostgreSQL names so the catalog reports meaningful
// types; timestamps are stored as text with a full '+08:00' offset, which SQLite's
// date functions understand, and booleans become 0/1.
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
    .replace(/\bbigserial PRIMARY KEY\b/g, 'INTEGER PRIMARY KEY')
    // PostgreSQL primary keys are implicitly NOT NULL; SQLite only enforces that for INTEGER PRIMARY KEY.
    .replace(/\b(text|bigint|integer) PRIMARY KEY\b/g, '$1 NOT NULL PRIMARY KEY');
}

export function sqliteSeed() {
  return flatten(readFileSync(join(here, 'db/seed.sql'), 'utf8'))
    .replace(/^SET LOCAL .*$/gm, '')
    .replace(/^SELECT setval\(.*$/gm, '')
    .replace(/'(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\+08'/g, "'$1+08:00'")
    .replace(/, true(?=[,)])/g, ', 1').replace(/, false(?=[,)])/g, ', 0');
}

export function build(target) {
  rmSync(target, { force: true });
  const result = spawnSync('sqlite3', [target], { input: `PRAGMA foreign_keys = ON;\n${sqliteSchema()}\n${sqliteSeed()}\nANALYZE;\nVACUUM;\n`, encoding: 'utf8', maxBuffer: 64 << 20 });
  if (result.error || result.status !== 0) throw new Error('sqlite3 failed: ' + (result.stderr || result.error?.message));
  const counts = spawnSync('sqlite3', [target, "SELECT 'orders=' || count(*) FROM orders; SELECT 'order_items=' || count(*) FROM order_items; SELECT 'refunds=' || count(*) FROM refunds;"], { encoding: 'utf8' }).stdout.trim().split('\n').join(', ');
  console.log(`Built ${target} (${counts}).`);
}

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { databaseQuery } from './provision.mjs';

export function seedSource(root, state, execute = databaseQuery) {
  const name = state.owner;
  if (!/^semlia_demo_[a-f0-9]{16}$/.test(name) || !/^sha256:[a-f0-9]{64}$/.test(state.digest)) throw new Error('Invalid source ownership');
  const db = `${name}_source`;
  const query = sql => execute(state.database.container, db, `SET ROLE ${name};\n${sql}`);
  const exists = query("SELECT to_regclass('public.demo_seed_receipt') IS NOT NULL;");
  if (exists === 'f') {
    const tables = query("SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m');");
    if (tables !== '0') throw new Error('Refusing nonempty unseeded source database');
    const data = readFileSync(join(root, 'source-data.sql'), 'utf8');
    if (!data.endsWith('COMMIT;\n')) throw new Error('Invalid transactional source material');
    query(data.slice(0, -8) + `CREATE TABLE public.demo_seed_receipt (digest text PRIMARY KEY); INSERT INTO public.demo_seed_receipt VALUES ('${state.digest}'); COMMIT;\n`);
  }
  return verifySource(root, state, execute);
}

export function verifySource(root, state, execute = databaseQuery) {
  const name = state.owner;
  if (!/^semlia_demo_[a-f0-9]{16}$/.test(name) || !/^sha256:[a-f0-9]{64}$/.test(state.digest)) throw new Error('Invalid source ownership');
  const query = sql => execute(state.database.container, `${name}_source`, `BEGIN READ ONLY; SET LOCAL ROLE ${name};\n${sql}\nCOMMIT;`);
  if (query('SELECT digest FROM public.demo_seed_receipt;') !== state.digest) throw new Error('Source receipt mismatch');
  const actual = JSON.parse(query(`SELECT json_build_object(
    'paid_count',(SELECT count(*) FROM public.orders WHERE status='paid'),
    'paid_amount_cents',(SELECT sum(amount_cents) FROM public.orders WHERE status='paid'),
    'successful_refunds_cents',(SELECT sum(amount_cents) FROM public.refunds WHERE status='succeeded'),
    'net_revenue_cents',(SELECT sum(amount_cents) FROM public.orders WHERE status='paid')-(SELECT sum(amount_cents) FROM public.refunds WHERE status='succeeded'));
  `));
  const expected = JSON.parse(readFileSync(join(root, 'expected.json'), 'utf8'));
  assert.deepEqual(actual, expected, 'Source SQL totals must reconcile');
  if (query('SELECT count(*) FROM public.customers;') !== '300' || query('SELECT count(*) FROM public.orders;') !== '3000') throw new Error('Source population mismatch');
  const invalid = query(`SELECT count(*) FROM public.orders o WHERE amount_cents <> (SELECT sum(quantity*unit_price_cents) FROM public.order_items i WHERE i.order_id=o.order_id) OR amount_cents < COALESCE((SELECT sum(amount_cents) FROM public.refunds r WHERE r.order_id=o.order_id),0) OR (status <> 'paid' AND EXISTS(SELECT 1 FROM public.refunds r WHERE r.order_id=o.order_id));`);
  if (invalid !== '0') throw new Error('Source invariants failed');
  return { customers: 300, orders: 3000, totals: actual };
}

import assert from 'node:assert/strict';
import { test } from 'node:test';
import { generate, renderSQL } from './generate.mjs';

test('fixed seed produces identical complete retail data', () => {
  const data = generate();
  assert.deepEqual(generate(), data);
  assert.equal(data.customers.length, 300);
  assert.equal(data.orders.length, 3000);
  assert.ok(data.products.length >= 20);
  assert.ok(data.refunds.length > 0);
  assert.equal(renderSQL(data), renderSQL(generate()));
  assert.notDeepEqual(generate(42), data);
});

test('independent reconciliation verifies relations, money and business scope', () => {
  const d = generate();
  const customers = new Set(d.customers.map(x => x.customer_id));
  const products = new Map(d.products.map(x => [x.product_id, x]));
  const orders = new Map(d.orders.map(x => [x.order_id, x]));
  let gross = 0, paidCount = 0, refunded = 0;
  for (const order of d.orders) {
    assert.ok(customers.has(order.customer_id));
    assert.ok(order.ordered_at >= '2026-04-01' && order.ordered_at < '2026-06-30');
    const items = d.order_items.filter(x => x.order_id === order.order_id);
    assert.ok(items.length > 0);
    let total = 0;
    for (const item of items) {
      assert.ok(products.has(item.product_id));
      assert.ok(Number.isSafeInteger(item.unit_price_cents));
      total += item.quantity * item.unit_price_cents;
    }
    assert.equal(order.amount_cents, total);
    const refunds = d.refunds.filter(x => x.order_id === order.order_id);
    assert.ok(refunds.reduce((n, x) => n + x.amount_cents, 0) <= total);
    if (order.status === 'paid') { gross += total; paidCount++; }
    for (const refund of refunds) {
      assert.equal(orders.get(refund.order_id).status, 'paid');
      assert.ok(refund.refunded_at >= order.ordered_at);
      if (refund.status === 'succeeded') refunded += refund.amount_cents;
    }
  }
  assert.deepEqual(d.expected, { paid_count: paidCount, paid_amount_cents: gross, successful_refunds_cents: refunded, net_revenue_cents: gross - refunded });
});

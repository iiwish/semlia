export const VERSION = 'retail-v1';
export const SEED = 20260914;

export function generate(seed = SEED) {
  if (!Number.isSafeInteger(seed) || seed < 0 || seed > 0xffffffff) throw new Error('Invalid seed');
  let state = seed >>> 0;
  const random = max => { state = (Math.imul(state, 1664525) + 1013904223) >>> 0; return state % max; };
  const date = day => new Date(Date.UTC(2026, 3, 1 + day)).toISOString().slice(0, 10);
  const customers = Array.from({ length: 300 }, (_, i) => ({ customer_id: i + 1, name: `Synthetic Customer ${String(i + 1).padStart(3, '0')}`, region: ['east', 'west', 'north', 'south'][i % 4] }));
  const products = Array.from({ length: 30 }, (_, i) => ({ product_id: i + 1, name: `Synthetic Product ${String(i + 1).padStart(2, '0')}`, unit_price_cents: 500 + random(19500) }));
  const orders = [], order_items = [], refunds = [];
  for (let i = 0; i < 3000; i++) {
    const day = random(90);
    const order = { order_id: i + 1, customer_id: random(300) + 1, ordered_at: date(day), status: i % 10 === 0 ? 'cancelled' : i % 10 === 1 ? 'pending' : 'paid', amount_cents: 0 };
    const count = 1 + random(4);
    for (let j = 0; j < count; j++) {
      const product = products[random(products.length)];
      const item = { item_id: order_items.length + 1, order_id: order.order_id, product_id: product.product_id, quantity: 1 + random(3), unit_price_cents: product.unit_price_cents };
      order.amount_cents += item.quantity * item.unit_price_cents;
      order_items.push(item);
    }
    orders.push(order);
    if (order.status === 'paid' && i % 7 === 0) refunds.push({ refund_id: refunds.length + 1, order_id: order.order_id, refunded_at: date(Math.min(89, day + 1 + random(5))), status: i % 3 === 0 ? 'pending' : 'succeeded', amount_cents: Math.max(1, Math.floor(order.amount_cents / (2 + random(3)))) });
  }
  const paid = orders.filter(x => x.status === 'paid');
  const gross = paid.reduce((n, x) => n + x.amount_cents, 0);
  const refunded = refunds.filter(x => x.status === 'succeeded').reduce((n, x) => n + x.amount_cents, 0);
  return { version: VERSION, seed, synthetic: true, customers, products, orders, order_items, refunds, expected: { paid_count: paid.length, paid_amount_cents: gross, successful_refunds_cents: refunded, net_revenue_cents: gross - refunded } };
}

export const DISCOVERY_DDL = `-- Entirely synthetic retail data. Amounts are integer CNY cents.
CREATE TABLE public.customers (customer_id bigint PRIMARY KEY, name text NOT NULL, region text NOT NULL);
CREATE TABLE public.products (product_id bigint PRIMARY KEY, name text NOT NULL, unit_price_cents bigint NOT NULL CHECK (unit_price_cents > 0));
CREATE TABLE public.orders (order_id bigint PRIMARY KEY, customer_id bigint NOT NULL REFERENCES public.customers(customer_id), ordered_at date NOT NULL, status text NOT NULL CHECK (status IN ('paid', 'pending', 'cancelled')), amount_cents bigint NOT NULL CHECK (amount_cents > 0));
CREATE TABLE public.order_items (item_id bigint PRIMARY KEY, order_id bigint NOT NULL REFERENCES public.orders(order_id), product_id bigint NOT NULL REFERENCES public.products(product_id), quantity integer NOT NULL CHECK (quantity > 0), unit_price_cents bigint NOT NULL CHECK (unit_price_cents > 0));
CREATE TABLE public.refunds (refund_id bigint PRIMARY KEY, order_id bigint NOT NULL REFERENCES public.orders(order_id), refunded_at date NOT NULL, status text NOT NULL CHECK (status IN ('pending', 'succeeded')), amount_cents bigint NOT NULL CHECK (amount_cents > 0));
`;

export const DDL = `${DISCOVERY_DDL}COMMENT ON TABLE public.orders IS 'Synthetic retail orders; paid excludes pending and cancelled. Amounts use integer CNY cents.';
COMMENT ON TABLE public.refunds IS 'Synthetic refunds; only succeeded refunds reduce net revenue; never join raw refunds before aggregating per order.';
`;

export function renderSQL(data) {
  const literal = value => typeof value === 'number' ? String(value) : `'${value.replaceAll("'", "''")}'`;
  return 'BEGIN;\n' + DDL + ['customers', 'products', 'orders', 'order_items', 'refunds'].map(table => {
    const rows = data[table];
    return `INSERT INTO public.${table} (${Object.keys(rows[0]).join(', ')}) VALUES\n` + rows.map(row => '(' + Object.values(row).map(literal).join(', ') + ')').join(',\n') + ';\n';
  }).join('') + 'COMMIT;\n';
}

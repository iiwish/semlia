-- Base tables read by sql/mart views. Generated from db/schema.sql; do not edit.

CREATE TABLE retail.regions (
  region_code text PRIMARY KEY,
  region_name text NOT NULL UNIQUE,
  ops_owner   text NOT NULL
);

CREATE TABLE retail.stores (
  store_id    text PRIMARY KEY,
  store_name  text NOT NULL,
  city        text NOT NULL,
  region_code text NOT NULL REFERENCES retail.regions(region_code),
  store_type  text NOT NULL CHECK (store_type IN ('direct', 'franchise')),
  area_sqm    integer NOT NULL CHECK (area_sqm > 0),
  opened_on   date NOT NULL,
  closed_on   date
);

CREATE TABLE retail.store_region_history (
  store_id    text NOT NULL REFERENCES retail.stores(store_id),
  region_code text NOT NULL REFERENCES retail.regions(region_code),
  valid_from  date NOT NULL,
  valid_to    date,
  PRIMARY KEY (store_id, valid_from)
);

CREATE TABLE retail.channels (
  channel_code text PRIMARY KEY,
  channel_name text NOT NULL,
  is_online    boolean NOT NULL
);

CREATE TABLE retail.product_categories (
  category_id   integer PRIMARY KEY,
  parent_id     integer REFERENCES retail.product_categories(category_id),
  category_name text NOT NULL,
  level         smallint NOT NULL CHECK (level IN (1, 2))
);

CREATE TABLE retail.products (
  sku_id           text PRIMARY KEY,
  sku_name         text NOT NULL,
  category_id      integer NOT NULL REFERENCES retail.product_categories(category_id),
  brand            text NOT NULL,
  is_private_label boolean NOT NULL,
  list_price       numeric(12,2) NOT NULL CHECK (list_price > 0),
  cost_price       numeric(12,2) NOT NULL CHECK (cost_price > 0),
  launched_on      date NOT NULL,
  status           text NOT NULL CHECK (status IN ('active', 'discontinued'))
);

CREATE TABLE retail.members (
  member_id        bigint PRIMARY KEY,
  registered_at    timestamptz NOT NULL,
  register_channel text NOT NULL REFERENCES retail.channels(channel_code),
  gender           text NOT NULL CHECK (gender IN ('F', 'M', 'U')),
  birth_year       smallint NOT NULL,
  city             text,
  home_store_id    text REFERENCES retail.stores(store_id),
  tier             text NOT NULL CHECK (tier IN ('normal', 'silver', 'gold', 'black')),
  is_employee      boolean NOT NULL
);

CREATE TABLE retail.promotions (
  promo_id      integer PRIMARY KEY,
  promo_name    text NOT NULL,
  promo_type    text NOT NULL,
  start_date    date NOT NULL,
  end_date      date NOT NULL,
  discount_rate integer NOT NULL CHECK (discount_rate BETWEEN 1 AND 99)
);

CREATE TABLE retail.orders (
  order_id        bigserial PRIMARY KEY,
  order_no        text NOT NULL UNIQUE,
  member_id       bigint REFERENCES retail.members(member_id),
  store_id        text REFERENCES retail.stores(store_id),
  channel_code    text NOT NULL REFERENCES retail.channels(channel_code),
  order_type      text NOT NULL CHECK (order_type IN ('normal', 'staff', 'test')),
  promo_id        integer REFERENCES retail.promotions(promo_id),
  status          text NOT NULL CHECK (status IN ('pending_payment', 'paid', 'shipped', 'completed', 'cancelled', 'closed')),
  ordered_at      timestamptz NOT NULL,
  paid_at         timestamptz,
  goods_amount    numeric(12,2) NOT NULL,
  discount_amount numeric(12,2) NOT NULL DEFAULT 0,
  coupon_amount   numeric(12,2) NOT NULL DEFAULT 0,
  shipping_fee    numeric(12,2) NOT NULL DEFAULT 0,
  paid_amount     numeric(12,2) NOT NULL
);

CREATE TABLE retail.order_items (
  order_id        bigint NOT NULL REFERENCES retail.orders(order_id),
  line_no         integer NOT NULL,
  sku_id          text NOT NULL REFERENCES retail.products(sku_id),
  quantity        integer NOT NULL CHECK (quantity > 0),
  unit_price      numeric(12,2) NOT NULL,
  discount_amount numeric(12,2) NOT NULL DEFAULT 0,
  line_amount     numeric(12,2) NOT NULL,
  unit_cost       numeric(12,2) NOT NULL,
  PRIMARY KEY (order_id, line_no)
);

CREATE TABLE retail.refunds (
  refund_id       bigserial PRIMARY KEY,
  order_id        bigint NOT NULL REFERENCES retail.orders(order_id),
  line_no         integer,
  applied_at      timestamptz NOT NULL,
  completed_at    timestamptz,
  status          text NOT NULL CHECK (status IN ('pending', 'succeeded', 'rejected')),
  refund_amount   numeric(12,2) NOT NULL CHECK (refund_amount > 0),
  shipping_refund numeric(12,2) NOT NULL DEFAULT 0,
  reason_code     text NOT NULL
);

CREATE TABLE retail.inventory_snapshots (
  snapshot_date date NOT NULL,
  store_id      text NOT NULL REFERENCES retail.stores(store_id),
  sku_id        text NOT NULL REFERENCES retail.products(sku_id),
  on_hand_qty   integer NOT NULL CHECK (on_hand_qty >= 0),
  PRIMARY KEY (snapshot_date, store_id, sku_id)
);

CREATE TABLE retail.sales_targets (
  store_id      text NOT NULL REFERENCES retail.stores(store_id),
  target_month  date NOT NULL,
  target_amount numeric(12,2) NOT NULL,
  PRIMARY KEY (store_id, target_month)
);

CREATE TABLE legacy.rpt_daily_sales (
  biz_date  date NOT NULL,
  store_id  text NOT NULL,
  order_cnt integer NOT NULL,
  sales_amt numeric(14,2) NOT NULL,
  PRIMARY KEY (biz_date, store_id)
);

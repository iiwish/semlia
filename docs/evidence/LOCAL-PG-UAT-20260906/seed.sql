BEGIN;
SET ROLE semlia_demo_owner_20260906;
CREATE SCHEMA demo AUTHORIZATION semlia_demo_owner_20260906;
COMMENT ON SCHEMA demo IS 'Synthetic local Semlia UAT data, not real customers or transactions';
CREATE TABLE demo.regions (region_id integer PRIMARY KEY, region_name text NOT NULL UNIQUE);
INSERT INTO demo.regions VALUES (1,'Demo North'),(2,'Demo South'),(3,'Demo East'),(4,'Demo West');
CREATE TABLE demo.customers (
  customer_id integer PRIMARY KEY,
  customer_name text NOT NULL,
  region_id integer NOT NULL REFERENCES demo.regions,
  segment text NOT NULL
);
INSERT INTO demo.customers
SELECT i, 'Demo Customer ' || lpad(i::text,4,'0'), (i-1)%4+1,
       CASE WHEN i%3=0 THEN 'enterprise' ELSE 'retail' END
FROM generate_series(1,240) i;
CREATE TABLE demo.products (
  product_id integer PRIMARY KEY,
  product_name text NOT NULL,
  category text NOT NULL,
  unit_price numeric(18,2) NOT NULL CHECK (unit_price >= 0)
);
INSERT INTO demo.products
SELECT i, 'Demo Product ' || lpad(i::text,3,'0'), 'Category ' || ((i-1)%5+1), 10+i*3.25
FROM generate_series(1,30) i;
CREATE TABLE demo.orders (
  order_id integer PRIMARY KEY,
  customer_id integer NOT NULL REFERENCES demo.customers,
  region_id integer NOT NULL REFERENCES demo.regions,
  order_date date NOT NULL,
  status text NOT NULL CHECK (status IN ('paid','refunded','cancelled')),
  amount numeric(20,8) NOT NULL DEFAULT 0,
  refund_amount numeric(20,8) NOT NULL DEFAULT 0,
  net_amount numeric(20,8) GENERATED ALWAYS AS
    (CASE WHEN status='cancelled' THEN 0 ELSE amount-refund_amount END) STORED,
  currency text NOT NULL DEFAULT 'CNY'
);
INSERT INTO demo.orders(order_id,customer_id,region_id,order_date,status)
SELECT i,(i-1)%240+1,(i-1)%4+1,DATE '2026-06-01'+((i-1)%90),
       CASE WHEN i%10=0 THEN 'cancelled' WHEN i%10=1 THEN 'refunded' ELSE 'paid' END
FROM generate_series(1,1200) i;
CREATE TABLE demo.order_items (
  order_id integer NOT NULL REFERENCES demo.orders,
  line_number integer NOT NULL,
  product_id integer NOT NULL REFERENCES demo.products,
  quantity integer NOT NULL CHECK (quantity > 0),
  unit_price numeric(18,2) NOT NULL,
  line_amount numeric(20,8) GENERATED ALWAYS AS (quantity*unit_price) STORED,
  PRIMARY KEY(order_id,line_number)
);
INSERT INTO demo.order_items(order_id,line_number,product_id,quantity,unit_price)
SELECT o.order_id,l.line,p.product_id,(o.order_id+l.line)%4+1,p.unit_price
FROM demo.orders o CROSS JOIN generate_series(1,3) l(line)
JOIN demo.products p ON p.product_id=(o.order_id*7+l.line)%30+1;
UPDATE demo.orders o SET amount=s.total,
  refund_amount=CASE WHEN o.status='refunded' THEN s.total ELSE 0 END
FROM (SELECT order_id,sum(line_amount) AS total FROM demo.order_items GROUP BY order_id) s
WHERE s.order_id=o.order_id;
CREATE TABLE demo.precision_samples (sample_id integer PRIMARY KEY, amount numeric(20,9) NOT NULL);
INSERT INTO demo.precision_samples VALUES (1,123456789.123456789),(2,0.000000001);
CREATE VIEW demo.monthly_sales AS
SELECT date_trunc('month',order_date)::date AS sales_month,region_id,
       count(*) AS order_count,sum(net_amount) AS net_revenue
FROM demo.orders GROUP BY 1,2;
COMMENT ON TABLE demo.orders IS 'Synthetic orders. Revenue is net_amount; cancelled and fully refunded orders contribute zero.';
COMMENT ON COLUMN demo.orders.net_amount IS 'Exact CNY revenue after cancellations and refunds; generated from amount and refund_amount.';
COMMENT ON TABLE demo.precision_samples IS 'Precision regression: exact sum must be 123456789.123456790.';
GRANT USAGE ON SCHEMA demo TO semlia_demo_discovery_20260906,semlia_demo_execute_20260906;
GRANT SELECT ON ALL TABLES IN SCHEMA demo TO semlia_demo_discovery_20260906,semlia_demo_execute_20260906;
RESET ROLE;
COMMIT;

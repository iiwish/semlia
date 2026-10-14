-- 订单行毛利：行实收减行成本；不分摊优惠券、不扣退款。
CREATE VIEW mart.item_gross_profit AS
SELECT i.order_id,
       i.line_no,
       v.paid_date,
       v.store_id,
       i.sku_id,
       p.category_id,
       c.parent_id AS top_category_id,
       i.quantity,
       i.line_amount,
       i.quantity * i.unit_cost AS line_cost,
       i.line_amount - i.quantity * i.unit_cost AS gross_profit
FROM retail.order_items i
JOIN mart.valid_orders v ON v.order_id = i.order_id
JOIN retail.products p ON p.sku_id = i.sku_id
JOIN retail.product_categories c ON c.category_id = p.category_id;

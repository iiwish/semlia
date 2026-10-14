-- 订单粒度净销售额：先把成功退款按订单汇总，再与订单关联，避免退款 × 订单行的扇出。
CREATE VIEW mart.order_refunds AS
SELECT r.order_id,
       sum(r.refund_amount - r.shipping_refund) AS refund_goods_amount
FROM retail.refunds r
WHERE r.status = 'succeeded'
GROUP BY r.order_id;

CREATE VIEW mart.order_net_sales AS
SELECT v.order_id,
       v.member_id,
       v.store_id,
       v.channel_code,
       v.order_type,
       v.paid_at,
       v.paid_date,
       v.goods_amount,
       v.goods_paid_amount,
       coalesce(r.refund_goods_amount, 0) AS refund_goods_amount,
       v.goods_paid_amount - coalesce(r.refund_goods_amount, 0) AS net_sales
FROM mart.valid_orders v
LEFT JOIN mart.order_refunds r ON r.order_id = v.order_id;

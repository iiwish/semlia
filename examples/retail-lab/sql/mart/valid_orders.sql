-- 有效支付订单：已支付、非测试单、未在支付后取消。所有销售指标的统一过滤口径。
CREATE VIEW mart.valid_orders AS
SELECT o.order_id,
       o.order_no,
       o.member_id,
       o.store_id,
       o.channel_code,
       o.order_type,
       o.promo_id,
       o.paid_at,
       (o.paid_at AT TIME ZONE 'Asia/Shanghai')::date AS paid_date,
       o.goods_amount,
       o.discount_amount,
       o.coupon_amount,
       o.shipping_fee,
       o.paid_amount,
       o.paid_amount - o.shipping_fee AS goods_paid_amount
FROM retail.orders o
WHERE o.paid_at IS NOT NULL
  AND o.order_type <> 'test'
  AND o.status <> 'cancelled';

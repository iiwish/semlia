-- 会员购买行为：仅统计普通订单（排除员工内购与测试单）。
CREATE VIEW mart.member_activity AS
SELECT v.member_id,
       min(v.paid_at) AS first_paid_at,
       max(v.paid_at) AS last_paid_at,
       count(*) AS paid_orders
FROM mart.valid_orders v
WHERE v.member_id IS NOT NULL
  AND v.order_type = 'normal'
GROUP BY v.member_id;

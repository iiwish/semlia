-- 门店月度目标达成：门店归属净销售额 / 月度目标。
CREATE VIEW mart.store_monthly_target AS
SELECT t.store_id,
       t.target_month,
       t.target_amount,
       coalesce(a.net_sales, 0) AS net_sales,
       round(coalesce(a.net_sales, 0) / t.target_amount, 4) AS achievement_rate
FROM retail.sales_targets t
LEFT JOIN (
  SELECT store_id, date_trunc('month', paid_date)::date AS target_month, sum(net_sales) AS net_sales
  FROM mart.order_net_sales
  WHERE store_id IS NOT NULL
  GROUP BY store_id, date_trunc('month', paid_date)::date
) a ON a.store_id = t.store_id AND a.target_month = t.target_month;

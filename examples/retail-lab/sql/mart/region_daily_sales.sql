-- 大区日销售：按门店当前归属大区汇总；电商订单（store_id 为空）不进入大区统计。
CREATE VIEW mart.region_daily_sales AS
SELECT n.paid_date,
       s.region_code,
       g.region_name,
       count(*) AS paid_orders,
       sum(n.goods_amount) AS gmv,
       sum(n.net_sales) AS net_sales
FROM mart.order_net_sales n
JOIN retail.stores s ON s.store_id = n.store_id
JOIN retail.regions g ON g.region_code = s.region_code
GROUP BY n.paid_date, s.region_code, g.region_name;

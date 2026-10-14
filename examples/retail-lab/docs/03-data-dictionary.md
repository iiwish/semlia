# 数据字典：semlia_retail_lab

数据库包含三个 schema：

- `retail`：交易与主数据，是所有指标的唯一可信来源。
- `mart`：基于 retail 的分析视图，口径与《指标口径手册》一致，用于发现和血缘。
- `legacy`：已废弃的旧报表，仅用于对账。

## retail.orders 订单头

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| order_id | bigint | 订单主键 |
| order_no | text | 订单编号，格式 XH + 下单日期 + 序号 |
| member_id | bigint | 会员编号；非会员订单为空 |
| store_id | text | 履约门店；电商平台订单为空 |
| channel_code | text | 渠道：store、miniapp、delivery、ecommerce |
| order_type | text | normal 普通，staff 员工内购，test 测试单 |
| promo_id | integer | 参与的促销活动，可空 |
| status | text | 见下方状态说明 |
| ordered_at | timestamptz | 下单时间 |
| paid_at | timestamptz | 支付时间；未支付为空 |
| goods_amount | numeric(12,2) | 商品原价合计（GMV 口径） |
| discount_amount | numeric(12,2) | 促销折扣 |
| coupon_amount | numeric(12,2) | 优惠券抵扣 |
| shipping_fee | numeric(12,2) | 运费 |
| paid_amount | numeric(12,2) | 顾客实付，含运费 |

订单状态 `status`：

| 值 | 含义 | 是否已支付 |
| --- | --- | --- |
| pending_payment | 待支付 | 否 |
| paid | 已支付待发货 | 是 |
| shipped | 已发货 | 是 |
| completed | 已完成 | 是 |
| cancelled | 已取消，可能发生在支付前或支付后 | 支付后取消时 paid_at 有值 |
| closed | 超时未支付关闭 | 否 |

## retail.order_items 订单行

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| order_id, line_no | bigint, integer | 联合主键 |
| sku_id | text | 商品 |
| quantity | integer | 数量 |
| unit_price | numeric | 售价（标价） |
| discount_amount | numeric | 分摊到本行的促销折扣 |
| line_amount | numeric | 行实收 = quantity × unit_price − discount_amount |
| unit_cost | numeric | 下单时的单位成本 |

## retail.refunds 退款

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| refund_id | bigint | 主键 |
| order_id | bigint | 原订单 |
| line_no | integer | 部分退款对应的订单行；整单退款为空 |
| applied_at | timestamptz | 申请时间 |
| completed_at | timestamptz | 退款完成时间；未完成为空 |
| status | text | pending 处理中，succeeded 退款成功，rejected 已驳回 |
| refund_amount | numeric | 退给顾客的总金额（含退回的运费） |
| shipping_refund | numeric | 其中退回的运费 |
| reason_code | text | quality、wrong_item、no_longer_needed、late_delivery、other |

一个订单可能有多条退款记录。

## 主数据

- `retail.stores`：门店，`region_code` 为 **当前** 归属大区；`closed_on` 非空表示已闭店。
- `retail.store_region_history`：门店大区归属历史，`valid_to` 为空表示当前有效。
- `retail.regions`：大区。
- `retail.channels`：渠道。
- `retail.product_categories`：两级品类，`parent_id` 为空的是一级品类。
- `retail.products`：商品，`list_price` 标价，`cost_price` 当前成本，`is_private_label` 自有品牌。
- `retail.members`：会员，`tier` 为当前等级，没有等级历史；`is_employee` 标记员工。
- `retail.promotions`：促销活动。

## 计划与快照

- `retail.sales_targets`：门店月度目标，`target_month` 为当月 1 日，`target_amount` 是净销售额目标。
- `retail.inventory_snapshots`：月末库存快照，半可加。

## 已废弃：legacy.rpt_daily_sales

旧版日报，按 UTC 日期统计，包含测试单，按含运费实付金额计算，且不扣退款。它的数字与新口径存在系统性差异，**不得作为任何指标的来源**。

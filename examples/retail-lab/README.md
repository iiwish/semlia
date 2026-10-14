# Retail Lab 测试与演示数据集

一套自洽的合成零售数据，覆盖 Semlia 支持的全部来源类型：PostgreSQL 实时库、Markdown 文档、CSV/XLSX、版本化 SQL 和 dbt 工件，并附带可自动核对的标准问答。业务主体“星禾零售”是虚构的，所有人员、门店和交易都是生成数据。

## 目录

| 路径 | 内容 | 在 Semlia 中的用法 |
| --- | --- | --- |
| `db/schema.sql`、`db/seed.sql` | `retail`（交易与主数据）、`legacy`（已废弃旧日报）、`mart`（分析视图）三个 schema，约 1.2 万订单、2.9 万订单行、812 笔退款，时间范围 2026-01 至 2026-09 | 新建 **PostgreSQL** 来源 |
| `sql/mart/*.sql`、`sql/retail/tables.sql` | 6 个 `CREATE VIEW` 文件体现口径与血缘；基础表定义由生成器从 `db/schema.sql` 提取 | **版本化 SQL** 来源，或作为 PostgreSQL 来源的 SQL 证据路径 |
| `docs/*.md` | 业务概览、指标口径手册、术语表、数据字典、会员、退款、门店区域、对齐会纪要、样例问法、数据质量问题 | **CSV / XLSX / Markdown** 来源（每个来源一个文件） |
| `files/*.csv`、`files/business-glossary.xlsx` | 门店主数据、门店目标、指标清单；术语、字段值映射、指标清单三个 Sheet | 同上 |
| `dbt/manifest.json`、`dbt/catalog.json` | 5 个模型、7 个 source，manifest v12 / catalog v1 | **dbt manifest / catalog** 来源 |
| `expected/questions.json`、`expected/answers.json` | 14 道标准题、参考 SQL 和期望结果 | 检查 Ask 回答是否正确 |
| `generate.mjs` | 确定性生成器（固定种子），重新生成上述数据文件 | |
| `load.mjs`、`sqlite.mjs` | 建库、核对、清理、导出 SQLite | 生成的 `.sqlite` 可作为 **SQLite** 来源上传 |

## 快速开始

需要本地 Docker 中运行的 `semlia-local-postgres-1`（开发环境默认的 PostgreSQL 18，宿主端口 5433）。

```bash
node examples/retail-lab/load.mjs up
```

该命令创建数据库 `semlia_retail_lab` 和三个角色：

- `retail_lab_owner`：数据属主，不能登录；
- `retail_lab_reader`：发现用，只读，满足 Semlia 的只读角色校验；
- `retail_lab_exec`：Ask 执行用，只读，并设置了 `log_min_error_statement=panic`。

随机密码保存在 `.semlia/retail-lab/credentials.json`（已被 gitignore），命令结束时会打印 Semlia 中填写的连接参数。需要再次查看时运行：

```bash
node examples/retail-lab/load.mjs info
```

核对 14 道标准题（参考 SQL 的结果与生成器独立计算的期望值比较）：

```bash
node examples/retail-lab/load.mjs verify
```

重建或清理数据库和角色：

```bash
node examples/retail-lab/load.mjs down
```

其他 PostgreSQL 容器可用 `--container NAME --superuser NAME --port PORT` 指定。

### 在 Semlia 中接入

1. **PostgreSQL 来源**：主机 `127.0.0.1`、端口 `5433`、数据库 `semlia_retail_lab`、用户 `retail_lab_reader`、SSL `disable`。发现结果应为 14 张表和 7 个视图，并带主键、外键。
2. **文档来源**：逐个上传 `docs/` 和 `files/` 下的文件。
3. **版本化 SQL 来源**：先把 `sql/` 复制到服务端 `SEMLIA_SOURCE_ARTIFACT_ROOT` 下（本地 native 环境为 `.semlia/native/content/retail-lab/`），再填写 `sql/mart/` 下 6 个视图和 `sql/retail/tables.sql` 的相对路径。基础表定义必须一起登记，否则视图血缘的上游无法解析，发现结果会是降级。
4. **dbt 来源**：同时上传 `dbt/manifest.json` 和 `dbt/catalog.json`。
5. **Ask 执行**（可选）：建好 PostgreSQL 来源后，按 `load.mjs info` 打印的内容，把 `SEMLIA_EXECUTION_DSN_RETAIL_LAB`、`SEMLIA_EXECUTION_SOURCES` 和 `SEMLIA_EXECUTION_ALLOW_PLAINTEXT=true` 写入受保护的服务端配置。执行适配器只允许查询普通表，参考 SQL 因此全部基于 `retail.*`。

## 刻意设计的测试点

数据和文档中故意放入了真实业务里常见的陷阱，用来检验语义治理和问答是否可靠：

| 测试点 | 数据表现 | 正确做法 |
| --- | --- | --- |
| 测试单 | 约 0.8% 的订单 `order_type=test` | 所有指标排除 |
| 支付后取消 | `status=cancelled` 但 `paid_at` 有值，并有整单退款 | 不属于有效支付订单 |
| 时区 | `paid_at` 为带时区时间戳，外卖订单集中在深夜 | 按北京时间切日 |
| 退款扇出 | 一个订单可有多条退款 | 先按订单汇总，再关联 |
| 运费 | 外卖和电商订单含运费，整单退款退运费 | 净销售额两侧都去运费 |
| GMV 与净销售额 | 两套口径，会议纪要记录了争议 | “销售额”默认指净销售额 |
| 员工内购 | `order_type=staff` | 计入销售，不计入会员指标 |
| 电商无门店 | `store_id` 为空 | 不进入大区和达成率 |
| 区域调整 | 长沙门店 2026-07 由华南划入华中，有历史表 | 默认按当前归属，按要求使用历史 |
| 半可加 | 月末库存快照 | 不能跨日期求和 |
| 已废弃来源 | `legacy.rpt_daily_sales` | 不得引用 |
| 别名 | 流水、江浙沪、单量、回头客等 | 映射到规范术语 |
| 未决口径 | 同店销售额、活跃会员数 | 澄清或说明无法回答 |
| 无法回答 | 净利润、个人手机号、写操作 | 拒绝并说明原因 |

完整问法见 `docs/08-sample-questions.md`。

## SQLite 版本

```bash
node examples/retail-lab/load.mjs sqlite
```

生成 `db/retail_lab.sqlite`（约 5 MB，已被 gitignore）。在 Semlia 中选择“导入文件 → SQLite 数据库文件”上传即可，不需要数据库服务或账号；发现结果为 14 张表及其主键和外键，已发布的分析模型可以直接对这个文件执行只读查询。

与 PostgreSQL 版本的差异：

- schema 被展平：`retail.orders` 变为 `main.orders`，`legacy.rpt_daily_sales` 变为 `main.legacy_rpt_daily_sales`；没有 `mart` 视图。
- 列保留 PostgreSQL 风格的声明类型（`numeric(12,2)`、`timestamptz`、`boolean`），Semlia 据此识别字段类型。
- 时间以带 `+08:00` 偏移的文本保存，布尔值为 0/1，金额是 SQLite 浮点数（结果按 15 位有效数字输出）。

## 重新生成与校验

```bash
node examples/retail-lab/generate.mjs
go test ./examples/retail-lab/
```

生成器是确定性的，重复运行输出完全一致。Go 测试使用 Semlia 自己的 Markdown/CSV/XLSX、PostgreSQL SQL 和 dbt 适配器解析全部文件，防止这些文件与系统的解析规则逐渐脱节。手工编写的部分（`db/schema.sql`、`sql/`、`docs/`、`expected/questions.json`）修改后，请重新运行 `load.mjs verify`。

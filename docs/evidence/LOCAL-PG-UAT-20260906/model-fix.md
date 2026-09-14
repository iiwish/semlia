# 本地模型配置修复

任务：LOCAL-MODEL-FIX-20260906。状态：本地模型调用与向量索引故障修复，真实 API 与浏览器验收通过。

用户明确授权修复本地模型调用和 pgvector 缺失，并提供本机 CSV 凭据文件。执行采用 Direct Execute：本次是根任务负责的本机操作，不委派数据库或密钥操作。

范围：仅 Git 忽略目录 `.semlia/local-pg-uat-20260906/` 内的操作脚本、私有 Compose 配置和备份，以及当前证据目录。保留用户已保存模型、端点和凭据摘要；只在所提供凭据与它们匹配时注入 server/worker。不修改产品源代码，不发布 AI 提案，不操作其他项目数据库。

数据库操作：从当前 PostgreSQL 18.4 Alpine 镜像的精确 image ID 派生本地 pgvector 镜像，保持 PostgreSQL 版本、操作系统基底、端口、卷及其他配置。启用扩展前保存元数据库自定义格式备份，确认 embedding_items 为空，再执行仓库既有 `deploy/local/enable-pgvector.sql`。禁止降级、清空或替换数据卷。

验证：配置摘要匹配；启动后两个进程均有凭据；原数据库行数保留；pgvector 扩展和向量列就绪；真实 Chat 问答及定义草稿生成；真实 Embedding 重建、向量检索；已有 PostgreSQL 聚合查询回归。仅模拟资产文本发送至用户配置的模型端点。

停止条件：凭据/端点不匹配、备份失败、数据卷发生变化、非空向量表需要额外迁移、模型拒绝访问或超出当前修复范围。未知失败不盲目重试，不打印原始服务商响应。

pgvector 构建依照[官方安装说明](https://github.com/pgvector/pgvector#installation)，固定 v0.8.6；原始 PostgreSQL image ID 为 `sha256:9a8afca54e7861fd90fab5fdf4c42477a6b1cb7d293595148e674e0a3181de15`。

## 实施结果

- CSV 为键值行结构，使用标准 CSV 解析器读取。凭据和 `openAiCompatible` 端点与保存配置精确匹配；无供应商、模型、端点或凭据摘要变更。
- server 与 worker 均已注入匹配的 `SEMLIA_EMBEDDING_QWEN`。凭据仅存在 Git 忽略的 0600 私有 Compose override；原始 CSV 未修改，未读取或输出密钥全文。
- 本地镜像 `semlia-postgres18-vector:uat-20260906` 使用原 PostgreSQL 18.4 Alpine 基底，pgvector v0.8.6 源码 commit `8ee86c96f0fd72390f890aa8a336fda6d3ab4c6c`；无 PostgreSQL 降级或系统基底切换。
- 自定义格式备份 `metadata-before-pgvector.dump` 共 1,242,820 字节，`pg_restore -l` 检查通过，保存在私有目录。原数据卷 `semlia-local_postgres-data` 及挂载位置保持一致。
- 镜像切换后的 143 个工作区、56 个语义资产、37 次发布与切换前一致；启用前 embedding_items 为 0 行。既有启用 SQL 事务成功，vector 扩展版本 0.8.6、vector_value 列存在。
- 私有 override 持久保留镜像和模型凭据配置，正常本机启动命令继续加载；server 和数据库健康，worker 正常执行任务，端口仍为 18081 和 5433。订单来源 `pgsql` 不受该修复影响。

## 真实验收

| 验证 | 结果 |
| --- | --- |
| Chat API | `deepseek-v4-flash` 成功将中文问题解释为 `demo.net_revenue` 聚合计划；真实只读执行返回 `440940.00000000` |
| AI 定义生成 | 生成 `prp_01m1t9fmjvents3wf4k1p2k9mk`，状态 draft；内容为“模拟订单净额的总和（基于模拟数据）”，仅变更 definition；未提交验证、评审或发布 |
| Embedding | `text-embedding-v4`、1024 维，2/2 个已发布知识块构建完成；索引 `run_01m1t9fmq2envbzmw8h2py6vrb` 为 active |
| 向量检索 | 中文“模拟订单净收入”使用 `mode=vector`、无 fallback；第一项命中 `demo.net_revenue` |
| 浏览器普通问题 | “模拟订单的净收入合计是多少？”得到补充信息请求，未编造金额 |
| 浏览器明确指标问题 | “请计算已发布指标 demo.net_revenue 的合计值。”生成已验证计划；点击“执行只读查询”显示已完成及 `440940.00000000`，执行 ID `run_01m1t9k17teny864bfekedkppd` |
| PostgreSQL 回归 | 2 个指标各执行 2 次新查询，金额和高精度结果一致；重复请求/历史回读不返回结果行 |
| 凭据保护 | 两容器凭据匹配；override 权限为 0600；实际密钥不在 Git tracked diff 或本目录证据中 |

## 当前边界

这次打通真实模型连接、现有资产的定义草稿生成、索引和受治理问答执行；不等同于从数据表自动提炼全部知识资产。普通中文问题仍可能要求明确指标；Ask 的初始解释调用只传问题与固定协议提示，并不将向量检索结果作为初始模型上下文。向量检索可用不代表完整 RAG 问答质量已验收。

浏览器已停留在成功金额结果页，用户可继续手动测试。AI 提案保留草稿供审核；本次未发布它，也未改动产品源代码。`model-check.md` 与 `model-check-results.json` 是修复前失败证据；本次实际回执为 `model-fix-results.json`。

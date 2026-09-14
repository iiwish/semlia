# 本地模型联调检查

日期：2026-09-06。范围：`OrbStack 订单演示（模拟数据）`，不修改模型配置、不自动评审或发布生成内容、不读取业务数据库。

## 当前结果

模型配置已持久化：供应商 `qwen`，协议 `openai_compatible`；默认 LLM 为 `deepseek-v4-flash`，默认 Embedding 为 `text-embedding-v4`（1024 维）。供应商与两个模型均为 enabled。

| 实际请求 | 结果 |
| --- | --- |
| POST ask：询问模拟订单净额 | HTTP 503，`PROVIDER_UNAVAILABLE` |
| POST generate-proposal：为现有模拟指标草拟中文定义 | HTTP 503，`PROVIDER_UNAVAILABLE`；未生成成功提案 |
| POST embedding-index：启动向量重建 | HTTP 503，`EMBEDDING_NOT_CONFIGURED`；未启动索引 |

## 阻塞原因

- 供应商引用 `SEMLIA_EMBEDDING_QWEN`。运行中的 server 和 worker 容器均不存在该凭据环境变量，本机 `.semlia/dev.env` 也未设置该项。检查仅输出是否存在，不输出任何密钥。
- 模型配置界面/API 保存环境变量名称和凭据修订摘要；不将用户输入的 API Key 注入服务进程。Chat 凭据解析器直接读取进程环境；缺失时拒绝请求。
- Semlia 自身元数据库 `semlia-local-postgres-1` 的 `pg_available_extensions` 与 `pg_extension` 均无 `vector`。Embedding 索引依赖此库的 pgvector，不是订单来源库 `pgsql`。

这些请求在本地配置检查阶段失败，不构成外部模型服务连通性或质量验证。未声称 AI 知识碎片生成可用；本次生成入口是针对现有资产的受治理提案，不是自动从表结构创建全套知识资产。

后续需要在 server 与 worker 注入与已存修订匹配的凭据，并经授权为 Semlia 元数据库安装兼容 PostgreSQL 18 的 pgvector。不得降级或替换已有数据卷；模型配置保存成功不等于实际服务可用。

具体状态码、trace ID 和耗时保存在 `model-check-results.json`。本次不改变产品源代码或部署配置。

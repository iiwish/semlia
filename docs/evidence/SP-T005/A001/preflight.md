# SP-T005 执行预检

- Date: 2026-09-11
- Status: Blocked；执行范围及费用授权机制待确认，尚未实现或运行模型。
- Authorization: 用户明确表示“T004 已验收，继续完成 T005 吧”。T004 Accepted；T006、T007 未授权。
- Mode: Direct Execute，宿主不允许未经明确授权的主动委派；本轮没有子代理。
- Worktree: 大量已有未提交和未跟踪改动，包含 T001 至 T004 与其他产品实现。全部保留；本轮仅修改任务状态、T004 验收摘要和本预检记录。

## 阻断依据

1. `contracts/production.md` 第 7 节要求先持久化 AgentRun/job，提供 queued/running/failed/outcome_unknown 查询，幂等重放不再次调用供应商。migration 23 的 `production_generation_links.output_digest` 为非空摘要，migration 25 对 links 设置禁止 UPDATE/DELETE 的触发器。现有表不能直接作为从排队到完成的运行状态记录；不能用伪输出摘要表示尚未生成成功。可保留既有成功输出关联，追加单独的运行请求/调用状态记录。
2. 同一契约要求真实服务端费用授权，且明确请求中的 `maxCostMicros` 不是授权。现有 ModelSetting 只有 token limit，模型客户端仅返回 token usage；`generation.go` 将 token 数映射成 CostMicros，不能作为供应商报价或本功能的可验证费用上限。未发现适用于本生产入口的服务端预算授权存储/配置。
3. SP-T001 handoff 对 T005 明确规定不能无授权临时补写存储迁移。T005 允许文件清单不含 migration、config、readiness 和相关迁移测试；必须先确认增量范围，不能修改已验收 migration 23/25/26，也不能复用成功字段伪造运行状态。

## 建议的受限补齐

- 追加 migration 27 的 up/down，保存生成请求、冻结配置与授权快照、调用领取状态及终态；保持旧 migration 字节不变。原始成功输出与人工应用继续复用既有表。
- 为生产生成增加默认关闭的服务端授权配置，限定 workspace、model setting、token ceiling 和单次费用 ceiling。无法证明费用上限时拒绝实际模型排队；确定性协议替身仅由测试注入，不能由 HTTP 请求切换。
- 供应商未提供实际费用时，结果的 `costMicros` 为 null，不把 token 数或配置预算当作实际费用。真正的费用上限应来自可信的供应商硬限额或明确配置的保守计费上界，不根据模型名称猜价格。
- 接入既有 job worker，不建立通用 Agent 框架；模型调用与数据库事务分离。调用一经领取，租约重试不重复调用供应商，无法证明完成则恢复为 outcome_unknown。
- 实现前把精确文件清单、预算配置语义、RED/GREEN、故障注入和迁移验证写入 SP-T005 packet；完成后交付 Needs_Review，而非直接 Accepted。

拟扩展范围仅限 migration 27、`db/sqlc.yaml`、`internal/platform/config/config.go` 及测试、`.env.example`、`cmd/semlia/readiness.go` 及测试、`tests/integration/db/database_test.go` 与生产迁移测试，以及必要的各层 `production_generation*.go`、共享 HTTP 路由注册。最终清单需在批准后按实现依赖精确展开，不授权修改整个目录。

## 验证状态

已执行 `git status --short`、文件清单检查及相关契约/源码/迁移读取。当前结论是静态预检，不宣称应用测试、协议生成、数据库迁移、实际模型质量或费用门禁已通过。没有修改应用代码、调用供应商、操作数据库或 Docker、提交、推送、部署。

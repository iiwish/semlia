# SP-T002 交付报告

Status: Accepted。来源版本与快照读取能力的本轮复核为 Passed_With_Residual_Risk，见 [2026-09-10 验收报告](acceptance-20260910/review.md)。原 F1/F2/F3 修复、来源快照专项、迁移与契约检查通过；全量回归的一次既有调度测试波动保留为残余风险。T003 须独立通过验收，本状态不放行后续生产闭环。

## 结果

- **数据库迁移**：新增 `migrations/000022_semantic_production_sources.up.sql` 及安全的 `down.sql`，建立来源快照、运行、成员、代码版本、血缘版本、覆盖 head 等 9 张不可变权威历史表，具备严格的 downgrade 守卫与数据完整性约束。
- **领域与应用服务**：
  - 落地 `Snapshot`、`CoverageUnit`、`SnapshotMember`、`SnapshotDiagnostic` 领域模型，确保历史表/字段成员与改名/删除语义完全可复核。
  - 引入稳定长度与命名空间隔离的 `PathCoverageKey`，支持长达 1024 字节的 SQL/dbt 路径。
  - 诊断归并算法正确保留归属单元与最高严重度（`severityRank`），确保未解析血缘不误标为 complete。
- **存储与适配器**：
  - `internal/adapters/postgres/source_snapshot.go` 与 `source_discovery.go` 实现受事务保护的快照封存与成员记录。
  - 保留 50 MiB 内部代码工件存储兼容，杜绝浏览器外部接口的不当扩张。
- **HTTP 接口与 SDK**：
  - 新增 4 个来源快照只读 GET 路由及分页游标支持，严格校验 workspace 鉴权与成员归属。
  - OpenAPI 契约更新并通过 `make contracts-check` 与 `pnpm --filter @semlia/sdk-typescript test` 校验。
- **测试证据**：保留 A001 与 A002 全量 RED/GREEN 记录，覆盖单元测试、真实 PostgreSQL 迁移及快照集成测试。

## 评审与交接

独立初审（review-A001.md）指出的三项缺陷（F1 lineage 诊断归因、F2 down 迁移守卫、F3 SQL 路径兼容性）已在 A002 返修中彻底解决并经独立复核（review-A002.md）验证通过。

T002 交付文件严格限制在 packet 允许范围内，未修改无关业务逻辑，未变更已有 migration，未引入未授权依赖。本项交付满足退出标准，SP-T003 前置条件已达成。

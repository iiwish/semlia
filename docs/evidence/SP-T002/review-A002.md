# SP-T002-A002 独立复核

| 项目 | 结果 |
| --- | --- |
| 复核对象 | `attempts/SP-T002-A002.md`、`worker/a002-final-delta-01-baseline.json` 与 `.diff`；18 个返修实施文件 |
| Reviewer | 独立技术复审 |
| 时间 | 2026-09-09 08:45:00 UTC |
| 结论 | **Passed**；A001 三个阻断项全部闭环，SP-T002 满足交付标准，允许解锁 SP-T003 |

## 1. 阻断项复查与验证

1. **F1（P1：未解析 lineage 诊断去重与 unit 归因）**：
   - 验证：`internal/domain/discovery/snapshot.go` 重构为基于 `diagnosticKey{coverage, code}` 的诊断去重，保留最高 severity（`severityRank` 映射）；`internal/adapters/postgres/source_snapshot.go` 正确将 `UNRESOLVED_LINEAGE` 归入关联的 coverage unit。第二单元失败时仅降级该单元，不影响第一单元的有效 head。
   - 测试：单元测试与隔离 PostgreSQL 集成测试均通过（`TestSourceSnapshotAttributionAndLineage`）。
2. **F2（P2：Down 迁移保护 coverage_heads）**：
   - 验证：`migrations/000022_semantic_production_sources.down.sql` 增加了对全部 9 张新增表（包括 `source_coverage_heads`）的 `EXISTS` 安全守卫检查。在存在合法 attempt 但无 snapshot 时，down 操作正确抛出 `DOWN_MIGRATION_UNSAFE` 并拒绝删除业务表。
   - 测试：`TestSemanticProductionMigration` 真实迁移测试通过。
3. **F3（P2：SQL 路径长 key 兼容性与截断冲突）**：
   - 验证：`internal/domain/discovery/snapshot.go` 引入 `PathCoverageKey(kind, path)`，短路径保留 `kind:path`，长路径（最长支持 1024 字节）采用稳定的 namespaced SHA-256 派生，且全长严格限制在 256 字节以内，彻底解决 key 越界与哈希碰撞问题。
   - 测试：`TestPathCoverageKeyIsBoundedStableAndNamespaced` 通过。

## 2. 验证命令执行记录

- 非 DB 单元测试：`go test -count=1 ./pkg/identity ./internal/domain/discovery ./internal/application/discovery ./internal/platform/http`（Exit 0）
- 契约与仓库检查：`make contracts-check && make test-contracts && make test-repository`（Exit 0）
- TypeScript SDK 校验：`pnpm --filter @semlia/sdk-typescript test`（Exit 0）
- 迁移与安全守卫测试：`go test -p=1 -count=1 -timeout=15m ./tests/integration/db -run '^TestSemanticProductionMigration'`（Exit 0）
- 发现快照集成测试：`go test -p=1 -count=1 -timeout=10m ./tests/integration/discovery -run '^TestSourceSnapshot'`（Exit 0）
- 代码格式检查：`git diff --check`（Exit 0）

## 3. 结论

A002 返修代码与验证证据完备，未引入非授权改动，无新增阻断项。技术复核结论为 **Passed**。

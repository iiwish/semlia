# SP-T002 验收复核

| 项目 | 结论 |
| --- | --- |
| 日期 | 2026-09-10，Asia/Shanghai |
| 授权 | 用户要求完成 T002、T003 的验收 |
| 范围 | 当前工作区的 T002 来源版本集合、读取、保留与迁移；不修改实现 |
| 技术结论 | Passed_With_Residual_Risk；未发现 T002 范围内的新阻断问题 |
| 任务状态 | 保留 Accepted；不代表 T003 或后续生产闭环通过 |
| 基线 | HEAD `63241d5` 加全部既有未提交/未跟踪文件；关键文件见 source-hashes.txt |

## 契约与实现

依据 `docs/specs/semantic-production/packets/SP-T002.yaml`、data-model 第 2 节与 contracts/production.md 来源读取契约进行复核。

| 验收项 | 代码与实际证据 | 结果 |
| --- | --- | --- |
| 不变 revision 仍属于新快照；名称不随 current 漂移 | `TestSourceSnapshotUnchangedMembersAndHistoricalNames` | Pass |
| partial/failed 不推进全局完整 head；乱序不回退 current | `TestSourceSnapshotPartialFailedAndOutOfOrderHeads`、`TestSourceSnapshotOutOfOrderDoesNotRegressCurrent` | Pass |
| 并发重放与唯一投影 | `TestSourceSnapshotConcurrentReplay`；源行锁、快照唯一约束与 run 链接 | Pass |
| code 原字节及 lineage 精确 revision | `TestSourceSnapshotCodeBytesAndPreciseLineage` | Pass |
| source/workspace 隔离、重新鉴权、签名游标与水位 | `ReadSourceSnapshot` 事务核对 authorization_version；`TestSourceSnapshotReadIsolationCursorsAndWatermark` | Pass |
| 分页字节上限及来源配置 | `TestSourceSnapshotMemberByteBudgetAndSourceConfig` | Pass |
| 删除、范围缩小、单元 head | `TestSourceSnapshotUnitHeadsDeletionAndScopeShrink` | Pass |
| A001 F1：最高诊断级别与准确单元归因 | `TestSourceSnapshotUnresolvedLineageKeepsUnitAndMaximumSeverity` 三种顺序；只降级坏单元 | Fixed |
| A001 F2：无 snapshot 的 coverage attempt 阻止降级 | migration 22 down 检查全部九张权威表；`TestSemanticProductionMigrationCoverageAttemptBlocksDowngrade` | Fixed |
| A001 F3：长路径、上限及入队/Begin/Persist 一致 | `PathCoverageKey`；`TestSourceSnapshotLongPathsBeginAndPersistConsistently` 五个子例 | Fixed |
| 历史制品保留与失败 freshness | `TestSourceSnapshotPinsHistoricalArtifactBytesDuringCleanup`、`TestSourceSnapshotWorkerFailureInvalidatesFreshnessWithoutLosingHistory` | Pass |
| 空库降级、旧历史不可验证、拒绝不完整 seal、有损降级拒绝 | `TestSemanticProductionMigration*` 及 PG17/18 生命周期 | Pass |

## 命令结果

所有数据库命令均清除 `SEMLIA_DATABASE_URL`、`SEMLIA_INGESTION_TEST_DATABASE_URL`、`SEMLIA_EXECUTION_TEST_DB`、`SEMLIA_EXECUTION_CRASH_HELPER` 中适用的外部覆盖；数据库包串行运行。只使用测试自行创建并清理的容器，不操作默认 Compose、已有数据库或真实模型。

1. 首轮 `go test -p=1 -count=1 -timeout=20m ./tests/integration/discovery ./tests/integration/ingestion ./tests/integration/catalog`：exit 0。输出分别为 `ok .../discovery 15.408s`、`ok .../ingestion 12.631s`、`ok .../catalog 9.150s`。首轮无独立日志文件，以上为实际工具输出摘要。
2. `go test -p=1 -count=1 -timeout=15m ./tests/integration/db -run '^TestSemanticProductionMigration|^TestSemanticProductionAuthoringMigration|^TestMigrationLifecycleAndTenantSchema$|^TestPostgres17MigrationLifecycle$'`：exit 0，[migrations.log](migrations.log)。本命令显式包含 T003 Authoring 迁移，避免旧任务正则遗漏它们。
3. 全量相关回归：[regression.log](regression.log)。identity、discovery domain/application/adapters、governance domain/application、catalog application、config、HTTP、cmd、contracts、repository、discovery/catalog/governance integration 全部通过；ingestion 包一次失败，整个命令 exit 1，不计为全绿。
4. `go test -p=1 -count=1 -v -timeout=10m ./tests/integration/discovery ./tests/integration/ingestion -run '^TestSourceSnapshot|^TestScheduledExecutionUsesSystemActorAfterCreatorSuspension$'`：exit 0，[focused.log](focused.log)。12 个来源快照顶层用例及调度单例通过，含全部展示的子例，无跳过。
5. `go tool sqlc diff -f db/sqlc.yaml`：exit 0，[sqlc.log](sqlc.log) 为空输出。使用只读 diff 替代会就地生成文件的 `make db-generate-check`，避免改动贡献者工作区。
6. `make contracts-check`：exit 0，[contracts.log](contracts.log)。实际重生成到临时目录并比对 Go/TS 产物。
7. `pnpm --filter @semlia/sdk-typescript test`：exit 0，[sdk.log](sdk.log)。该脚本仅 `tsc --noEmit`，不宣称 SDK runtime 测试通过。
8. `git diff --check`：exit 0；仅检查 Git 跟踪差异，不把它当成所有未跟踪代码的格式证明。

## 残余风险

全量回归中 `TestScheduledExecutionUsesSystemActorAfterCreatorSuspension` 在 `ingestion_test.go:1009` 返回 `processed=2`，断言期望 1；首轮全包及后续单例通过。该包共享一套数据库，`openStore` 只关闭连接池，不清理其他测试留下的 schedule；`ProcessDue` 是全局扫描，使用实际时钟。跨分钟拾取其他 schedule 是代码支持的解释，但本轮没有通过时间控制最终证明这个原因。

以下三个文件与 `.semlia/evidence-work/SP-T002-baseline/` 对应文件逐字节 `cmp` 均 exit 0：`tests/integration/ingestion/ingestion_test.go`、`internal/application/ingestion/schedules.go`、`internal/adapters/postgres/schedules.go`。未发现这项波动由 T002 引入的证据；保留失败日志，不删除测试或降低断言。测试隔离/时钟稳定性需要单独维护，不宣称整个仓库稳定全绿。

本轮没有重演所有数据库故障注入、穷举所有扫描顺序或进行桌面/真实模型验收。T002 结论限于来源历史基础能力；T003 对这些证据的消费检查由 T003 独立验收。

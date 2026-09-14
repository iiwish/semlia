# SP-T005-A002 修复复核

Status: Needs_Review。范围内原 P1 已修复；本记录为直接执行后的自检，不是独立评审或用户验收。T006 未启动。

## 原发现

无业务规则证据的非空 metric 通过确定性验证。正式回归的 [RED](red.log) 为实际 succeeded 与期望 failed 不符；[GREEN](green-missing-rule.log) 和最终全量治理测试通过。草稿及模型建议可保留非空但未确认的内容，schema validator 对所有有实质变更的 semantic_asset 发出 `PRODUCTION_BUSINESS_RULE_UNCONFIRMED`，不会把非空文本当批准。

## 权威与生命周期

- 真实会话主体必须 human、active，拥有来源/证据读取及目标写入权限。客户端不能指定 actor、digest 或 evidence origin。
- 选定证据路径只接受当前 input 与目标 evidenceIds 同时引用的 declared artifact；observed、inferred 和伪造 metadata 均不能生成可信状态。
- 人类声明路径要求显式声明文本，服务器在同一事务创建 declared evidence 和确认事件。无预填 evidence 的真实 HTTP → worker → review → publish 正向用例通过；声明可经 GET 核对，原始输入不被改写。
- 事件追加、不可变，绑定 workspace、operation/version、target key/content digest、set digest、evidence digest、principal 和授权版本。并发同 key 一次创建、一次重放；提交故障不泄漏事件、证据、贡献者、审计或 outbox。
- 版本变化不继承确认，包括内容相同的新版本。撤销不删除历史，重放旧 confirm 不续期；新增/撤销确认要求重新验证和审核。
- 排队事务、worker 加载、结果提交和审核/发布门禁采用同一见证；在途撤销使已计算成功的结果落库为 failed。主体停用、真实 grant 撤销及撤回已审核确认都阻止发布。
- 确认和撤销主体进入既有 contributor 集合，不能审核或发布自身参与的操作。独立审核发布和原有多对象/回滚路径通过。
- 验证器 production.2 拒绝旧生产验证凭证；readiness 要求 migration 28 且确认表与保护触发器完整。已发布历史不回填确认。新增迁移拒绝删除非空确认历史，原 1-27 字节保持不变。

## 验证

| 命令/证据 | 结果 |
| --- | --- |
| 全量 `go test -count=1 -p=1 -timeout=20m ./tests/integration/governance` | exit 0，76.798s，[日志](governance-complete.log) |
| `go test -race -count=1 -p=1 -timeout=10m ./tests/integration/governance -run '^TestProductionBusinessRule'` | exit 0，9.150s，[日志](business-rule-race-final.log) |
| governance/http/cmd 单元与 readiness 集成 | exit 0，[日志](unit-complete.log) |
| 语义生产迁移 + 最新 schema 生命周期 | exit 0，11.908s，[日志](db-scoped-final.log) |
| contracts-check、db-generate-check、test-contracts、test-repository | exit 0，[日志](checks-complete.log)、[sqlc 28 注册后复验](db-generation-complete.log) |
| TypeScript SDK `tsc --noEmit` | exit 0，[日志](sdk-complete.log) |
| `go vet`：governance/postgres/http/cmd | exit 0，[日志](vet.log) |
| 无人工数据库预填证据的声明路径 | [RED](declaration-red.log) 为接口 400；[GREEN](declaration-green.log) 及最终治理/race 通过 |

## 非本次回归

全量数据库 suite [试跑](db.log) 未通过，以下 5 个旧测试均在 [A002 前像的独立副本](db-baseline-failures.log) 复现同类失败，未通过降低断言或改写旧测试掩盖：

1. `TestMigration18ProjectsAndRollsBackIngestionAuditTargets`：按最新版本减固定步数，却假定到达 17。
2. `TestMigration18PreservesPopulatedVersion17PostgreSQLSourceAndRunAcrossRoundTrip`：同类固定步数假设。
3. `TestMigration21EmptyDownUpAndHistoryGuard`：`Up()` 后仍假定最新为 21（两个子例）。
4. `TestMigration20PreservesPopulated19AndLeaseFencing`：按最新版本减两步，却假定到达 19。
5. `TestPopulatedM2UpgradeAndRollbackPreserveGovernedAuthoringRows`：最新 repository 向版本 6 schema 创建 proposal 失败。

前像副本恢复 A002 前源码和迁移 1-27，不含 28；使用独立一次性 PostgreSQL。全量数据库测试不能宣称绿色。修复上述历史测试不属于本次业务规则修复，不影响已通过的目标迁移和治理用例。

## 边界

来源与业务规则均为披露的合成数据，生成复现使用协议替身。没有实际模型、部署、默认 Compose 栈写入、Git 提交或推送。桌面确认入口归属 T006，本轮只提供服务端/API/SDK 能力。源码差异以 [执行前像差异](source-delta.patch) 和 [摘要清单](source-manifest.tsv) 为准，不以 HEAD 误归属既有用户改动。

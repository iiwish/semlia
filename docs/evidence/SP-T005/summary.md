# SP-T005 交付状态

Status: Needs_Review

当前结论：无业务规则证据的非空口径通过验证这一 P1 已修复，T005 等待用户复核验收。服务端可信确认、人类声明证据、版本/权限复核与 SoD 均有运行证据，见 [A002 修复复核](A002/review.md)。全量治理、规则 race、目标迁移及契约检查通过；全量数据库 suite 的 5 个既有旧测试失败已在修复前像复现。T006 不启动。

T005-A001/A002 已完成实现与自检，等待用户验收。执行采用 Direct Execute；没有子代理或独立评审，不将自检冒充独立审查。

## 交付内容

- 固定来源、输入版本及模型修订的异步生成；服务端费用 grant 默认关闭，客户端上限不能扩大权限。
- 完整结构化 schema、身份及引用检查；调用前和结果提交前复核发起人、Agent、来源、草稿及模型配置。
- 不可变原始建议和运行归因；调用领取后不自动重复供应商调用，提交故障恢复为明确 outcome_unknown。
- 显式人工 PUT 应用及有界差异记录；生成本身不改写、提交或发布草稿。
- API/SDK 和模型配置修订读取、migration 27、readiness 接入。
- 共享验证器阻断未决 definition/scope，并规范化失败 findings，使失败结果也符合存储完整性约束。
- migration 28 的不可变业务规则确认/撤销记录；支持选定 declared evidence 或显式人类声明，验证及发布复核当前有效确认，确认人纳入职责分离。旧验证凭证需要重跑。

## 验证结果

| 命令 | 结果与日志 |
| --- | --- |
| 应用治理、HTTP、配置、readiness 单元与集成测试 | exit 0，[unit-final-2.log](A001/unit-final-2.log) |
| 全量治理集成，`-count=1 -p=1 -timeout=20m` | exit 0，71.441s，[governance-final-2.log](A001/governance-final-2.log) |
| 生成集成，`-race -count=1` | exit 0，15.478s，[generation-race-final-2.log](A001/generation-race-final-2.log) |
| PostgreSQL 18 生产迁移及完整 schema 生命周期 | exit 0，8.683s，[migration-final.log](A001/migration-final.log) |
| `make contracts-check` | exit 0，[日志](A001/contracts-check-final.log) |
| `make db-generate-check` | exit 0，[日志](A001/sqlc-check-final.log) |
| `make test-contracts` | exit 0，[日志](A001/contracts-final.log) |
| TypeScript SDK typecheck | exit 0，[日志](A001/sdk-typecheck-final.log) |
| `make test-repository` | exit 0，[日志](A001/repository-final.log) |

实际 RED 保留：初始实现与人工应用失败日志，以及 [未决内容验证反例](A001/unresolved-validation-red.log)。该反例通过用户批准的共享验证器扩展修复，详见 [修复记录](A001/validation-blocker.md)。中间失败日志不删除，不累计重复运行夸大覆盖。

## 复核与边界

见 [风险复核](A001/review.md)、[源码差异](A001/source.diff)、[文件清单](A001/source-delta.json) 和 [旧迁移保留证明](A001/preserved-migrations.json)。差异以执行前未提交工作区为基线，不以 HEAD 误归属既有改动；迁移 1–26 的 52 个文件保持原字节。

测试使用一次性 PostgreSQL 和本地协议替身，不证明实际模型生成质量、供应商价格准确性或完整业务样例验收。没有真实模型调用、部署、提交或推送；T006/T007 未执行。只有用户明确验收后才将 T005 标记 Accepted。

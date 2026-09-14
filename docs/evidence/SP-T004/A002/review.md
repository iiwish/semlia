# SP-T004 A002 风险复核

Review object: A002 执行前工作区快照至本轮最终源码，差异见 [source.diff](source.diff)，文件与哈希见 [source-delta.json](source-delta.json)。本记录是当前执行者单独进行的反例检查，不代表第二位审查者或用户批准。

## 结论

T004 范围内已复现的阻断均已修复，最终常规验证通过；建议进入 Needs_Review，由用户验收。T005 不开放。

## 关键反例

| 风险 | 实现与运行证据 |
| --- | --- |
| 排队等同验证成功、最终异常不落状态 | Submit 只创建 queued attempt/job；worker 执行完整 checks，最后一次执行失败提交完整失败结果。`TestProductionValidationSubmitQueuesWithoutSyntheticSuccess`、`TestProductionValidationFinalFailureIsDurable` 通过。 |
| 混用历史通过结果与旧审批 | validation seal 绑定 operation/version/attempt、集合、输入、新鲜度、规则和检查结果；新轮次不采纳旧审批。`TestProductionValidationWorkerRunsCompleteAttempt` 与 reviewer fencing 测试通过。 |
| 撤权后发布或重放、同一角色兼任 | 提交时复验当前 actor/reviewer 授权、全组贡献者与职责分离。暂停 reviewer 后发布拒绝，暂停 publisher 后成功回执重放仍拒绝。 |
| 部分提交或竞争生成两个 head | 延迟约束阶段注入故障，对 20 张表完整行快照比较，确保无业务、审批、指针、manifest、回执或事件泄漏。同 key 随后成功；两个独立操作并发发布仅一个成功、一个 409，只有一个 release/receipt。 |
| 五类更新及回滚遗漏业务内容 | 完整五类 create、重复回滚、原身份重引入、五类 update/rollback 和非目标 pins 保留均使用真实 PostgreSQL。比较精确 manifest digest、注册表表达式及写版本。 |
| 保留身份误拦后续更新 | migration 26 的 reservation guard 限定为原操作已终态、当前实际 release pin 和精确更新 baseline，合法更新通过；create 重用继续要求完整可信缺席证明。 |
| 旧单项或直接 SQL 绕过集合 | 原 HTTP 入口拒绝生产成员；直接插入无 production binding 的 review/run 由数据库拒绝，测试检查具体守卫错误而非任意失败。 |
| 发布详情泄露未改动对象 | 完整 before/after manifest 逐项检查读取权限；只读得本次新增目标的主体能读 operation，但不能读取含无权旧 pins 的 release。负对照 overlay 移除该检查会返回 200 并令测试失败。 |
| 投影丢失组合归因 | Git release 文档保留五个 proposal、原 operation、attempt/validation/review bindings 和 rollback protection；首次及重复回滚投影、幂等重放均通过。 |
| 历史可篡改或 schema 假就绪 | migration 26 校验真实完整 manifest、精确 before pins、物理输入父版本、结果封存与归因并禁止历史更新/附加；readiness 校验版本 26 与关键启用触发器。PG17/18 迁移生命周期、旧不可验证历史保留与空层降级/重升通过。 |

## 验证证据

- [完整集成回归](integration-final.log)：governance 47.652s、projection 3.883s、catalog 3.901s；96 个顶层 PASS，其中 28 个 production 顶层测试。一个既有 subprocess-only helper 在主进程 SKIP，其父级真实 execution 测试通过；不将 helper 记作独立通过。
- [直接 SQL 旁路测试](legacy-db-guards.log)：review/run 直接插入反例通过，4.255s，并纳入最终完整集成回归。
- [最终单元与 readiness](unit-final.log)：domain/application/HTTP、distribution、Git/projection、cmd 全部通过；MCP 包无独立测试文件，真实通道调用由集成测试覆盖。
- [最终迁移矩阵](migrations-green.log)：25.185s，含 PG17/18 生命周期、tenant schema、machine credential/channel parity、production 迁移；旧 1–25 共 50 个 migration 文件与基线字节一致，见 [哈希](preserved-migrations.json)。
- [sqlc](db-generate-check.log)、[OpenAPI](contracts-check.log)、[契约](test-contracts.log)、[仓库](test-repository.log)、[SDK](sdk.log)、[diff](diff-check.log) 均 exit 0。SDK 命令为 `tsc --noEmit`，不冒充单独的 SDK runtime 单测。
- [读取权限负对照](manifest-read-red.log) 为预期 exit 1：无权读取旧 pins 的主体收到完整 release 200；[正常源码](manifest-read-green.log) exit 0，5.203s。作用域覆盖全部对象及输入时正常读取，不要求工作区全域 asset.read。overlay 仅用于证明测试可辨别漏洞，不替换交付源码。

## 残余边界

- 未做最大 32 targets / 256 checks 的负载与全部并发交错穷举，未运行全仓 `make check`、浏览器或真实模型调用；它们不属于本执行包。
- 数据库不可用时无法写入最终失败状态，worker 保留真实错误，不虚构成功或已持久化状态。运维恢复遵循现有 job/dead-letter 流程。
- 验证历史游标绑定进程密钥；服务重启后需重新查询。不可复核的既有生产历史保持不可复核，不补造可信 seal。
- Git 资产文件是修订投影；发布成员资格以 release manifest 为准。缺席回滚保留资产身份和历史修订，不发送假新修订事件。
- 不包含部署、Git commit/push、用户验收、T005 模型生产或 T006 前端。

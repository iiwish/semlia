# SP-T001 独立评审 A001

Reviewer：独立只读子任务 `sp_t001_review`。时间：2026-09-09 04:53:43 至 04:58:15 UTC，4 分 32 秒。范围为四个新增契约/静态测试，结合当前工作区实际实现核对；不是整仓 dirty diff 评审。

## Findings

1. **P1：旧 rollback 入口未纳入生产集合防绕过。** `contracts/production.md:140`、`data-model.md:139` 的旧入口清单缺 `/governance/releases/{releaseId}/rollback`。当前 `internal/platform/http/governance.go:214` 调用旧服务，`internal/application/governance/publishing.go:420` 只检查 singular origin 的作者，多提案 origin=NULL 时跳过作者检查，且没有原 reviewer/source/exact-before 新契约检查。必须对生产 release 及其回滚后继拒绝或统一新命令，并有事务/数据库防线。
2. **P2：validation attempt 没有与旧约束和执行器衔接。** `data-model.md:121,129` 要求追加重试/impact attempt；migration `000006:228` 却唯一约束 proposal/validator/version，`orchestration.go:82`、`validatejob.go:82` 不执行 in_review 重验。必须规定 attempt 身份、expand-first 唯一约束、legacy 默认、队列/查询键、完整有效结果选择与不倒退 Proposal 状态的重验。
3. **P2：原始 AI 建议缺少可恢复持久化。** `data-model.md:74` 与既有 `agentrun.go:54,71` 只有摘要；`contracts/production.md:110` 承诺 GET 原始建议与人工差异。成功生成后、人工应用前重启会无法从摘要恢复输出。必须定义有界 immutable structured output 的正文/定位、摘要核验、授权、保留及人工版本/差异/actor 关联，不保存 prompt/隐藏推理。
4. **P2：缺席回滚后同一身份没有再生产路径。** `data-model.md:110,119,156,158` 同时要求 released 后继 update、update 有当前 published pin、回滚后保留 identity/正式行、create 目标不得已存在；首次创建被回滚后不能纠正并重发。必须在既有 create/update 内固定受治理重引入规则，不能恢复错误旧内容或改身份来绕过。

行号对应 A001 评审对象，其 SHA-256 见 `worker/artifact-hashes.json`；修正后的最新行号可能变化。以上是新契约的缺口，不是声称 T001 已修改运行代码并引入漏洞。

## 已核实

旧普通 draft base 例外、published version 与 registry CAS 分离、候选显式 primary、code/lineage append-only revision 已在 A001 中固定，没有重复报告。独立评审者运行 `go test -count=1 ./tests/contracts -run '^TestSemanticProductionContract$'` 通过；结构化测试不具备观察上述数据库/旧入口冲突的能力。

PM 独立运行 contracts/repository 两包，91 test/subtest pass、0 fail/skip，6.327 秒；范围摘要核对没有应用或其他范围外变更。基线两组也没有失败。

## 处理

A001 已返回，以上 findings 进入 SP-T001-A002。保留原证据，原 worker 仅修正契约与静态测试，再由同一独立 reviewer 复核；SP-T002 不开放。

# SP-T003 验收复核

| 项目 | 结论 |
| --- | --- |
| 日期 | 2026-09-10，Asia/Shanghai |
| 授权 | 用户要求完成 T002、T003 的验收 |
| 范围 | 当前工作区 T003 候选生产、更新、草稿替换与恢复；不返修产品代码 |
| 技术结论 | Failed / Needs_Fix |
| 后续门禁 | 不满足 T004 至 T007 的前置验收条件 |
| 基线 | HEAD `63241d5` 加全部既有未提交/未跟踪文件；共享文件已包含 T004；见 T002 本轮 source-hashes.txt |

## 阻断发现

### F1 / P1：未经核实的候选与来源可产生权威生产记录

`internal/application/governance/production.go:226` 的 CreateOperation 只校验声明形状、计算摘要，没有查验真实候选/快照、摘要、coverage 与 evidence 关系。`internal/adapters/postgres/semantic_production.go:30` 的事务也没有补检；migration 23 的 candidate link 不引用真实候选的 FK。

实测：启用 `WithAuthorization`，注册真实 workspace_admin 后，分别提交从未入库的合法类型 candidate ID 和 snapshot ID，均返回 HTTP 201，并实际提交一条 production operation。违反 contracts/production.md 第 4 节与 packet 的输入真实性/来源授权门禁。需要在一致授权快照与写事务内核对真实来源、候选、证据和必要完整单元，失败不落任何业务记录。

### F2 / P1：草稿替换成功但返回悬空提案，且不能幂等恢复

`internal/adapters/postgres/semantic_production.go:636` 的 ReplaceDraftTx 接收 `proposals` 和 `cmd` 却未持久化它们，也未保存编辑者 contributor。只写 operation version、targets、links 就提交；target 的 proposal_id 缺少 FK，错误不会被数据库阻止。

实测：PUT 返回 200/version 2，但响应中的新 Proposal ID 在 proposals 表 count=0；replace_draft command count=0；第二位编辑者 contributor count=0；相同 body/key 重放返回 409，而不是原成功结果。造成悬空治理记录、响应丢失后无法可靠恢复以及贡献者隔离依据不完整。应在同一事务补齐真实提案、合法旧成员转换、幂等结果与不可变编辑者归因，不只是补响应。

### F3 / P1：冻结集合仍允许 PUT 产生新版本

`internal/application/governance/production.go:753` 及 ReplaceDraftTx 只检查版本号，没有检查 frozen_at 和全体 Proposal draft 状态。

实测：测试夹具把当前版本设为合法冻结状态，不依赖 T004 验证器；PUT expectedVersion=1 仍返回 200/version 2。违反“PUT 只替换全体 draft，冻结后经 supersede 纠正”的契约。冻结/状态与版本检查必须在同一 operation 锁内完成，不能仅加 HTTP 预检查。

### F4 / P2：历史恢复与原命令重放不符合契约

`internal/platform/http/semantic_production.go:571` 对非当前 version 直接返回 404；`internal/application/governance/production.go:285` 的 create 重放读取当前版本，而不是 production_commands.operation_version 指向的原提交结果。

实测：创建 v1、PUT 到 v2 后，GET `?version=1` 返回 404；原 create key 重放返回 v2 及 v2 的 setDigest。数据库保有两个版本也不能构成有效恢复。应按指定版本读取不可变输入/targets，并以命令记录恢复原 IDs/version/digest。

### F5 / P2：更新请求丢弃 targetId，最终产生内部错误

`internal/platform/http/semantic_production.go:332` 将 wire target 映射为领域声明时不传递 targetId；`TargetDeclaration` 也没有该字段。应用层用 identityKey 代替更新目标，普通 targetId 更新得到空 ID。

实测：通过原治理链真实发布一个资产后，提供其合法 targetId/baseRevisionId 的缩减输入请求得到 HTTP 500/内部 empty UUID 错误。该探针不宣称请求具备完整来源输入：缺失输入应以 4xx 拒绝，而非丢弃目标后内部失败。完整 update/content/changes/base 与目标作用域需要一起修复，不能只把 500 改成错误提示即宣称 update 支持完成。

## 验收矩阵

| 能力 | 结论 |
| --- | --- |
| 原样例的空资产新建、draft identity、初次 Proposal 及同版本重放 | 既有 PG18 测试通过；不证明输入证据真实 |
| 来源/候选真实性与消费授权 | F1 阻断；workspace 权限控制负例通过 |
| 草稿替换的真实提案、幂等日志和编辑者归因 | F2 阻断 |
| 冻结后禁止 PUT | F3 阻断 |
| 旧版本恢复、原提交重放 | F4 不通过 |
| 完整 create/update 五种 target | 仅有限新建样例有证明；targetId 更新存在 F5，不可验收 |
| 无变化检测、完整 candidate decisions、supersede/reuseIdentity、来源撤权竞争、全边界 byte limits | 未取得充分 runtime 证明，不标通过 |
| migration 23 空库 up/down、存在 operation 时拒绝 down | 既有实际迁移测试通过；不等价于所有模型 FK/immutable 约束齐全 |
| 既有治理兼容回归 | 本轮原 governance 全包通过；与新反例失败同时成立 |

源码检查还显示生产写事务没有写 candidate decision、audit/outbox；migration 23 缺少计划要求的多项 immutable 与关联约束；列表采用 offset 且未完整实现恢复筛选；GET input 的存储包装与响应反序列化结构不一致。它们需纳入 T003 返修矩阵，本轮不借未运行探针声称已完成完整安全证明。

## 证据与重跑

主证据为 [probes-reviewed.log](probes-reviewed.log) 和 [验收探针](acceptance_probe_test.go.txt)。使用 Go overlay 将探针作为 integration/governance 的附加测试编译，不修改产品、既有测试或迁移；`.go.txt` 不会污染默认 `go test ./...`。overlay.json 的绝对路径绑定本次工作区，在其他 checkout 需先调整路径。

```sh
env -u SEMLIA_DATABASE_URL -u SEMLIA_INGESTION_TEST_DATABASE_URL \
  -u SEMLIA_EXECUTION_TEST_DB -u SEMLIA_EXECUTION_CRASH_HELPER \
  go test -overlay=docs/evidence/SP-T003/acceptance-20260910/overlay.json \
  -p=1 -count=1 -v -timeout=10m ./tests/integration/governance \
  -run '^TestAcceptanceT003'
```

命令结果为 exit 1。6 个顶层测试中 5 个反例测试失败，授权控制测试通过；未知输入测试含 candidate/snapshot 两个失败子例。数据库为本轮自行创建、迁移和清理的 PostgreSQL 18 容器。未连接默认开发库、未调用真实模型、未部署。

`probes.log` 是第一轮四个反例；`probes-final.log` 是增加授权控制和更新探索后的中间运行。中间更新探针的“valid update”措辞不准确，缺少完整来源输入且 change op 不符合独立契约；该声称不用于验收。主证据探针明确缩减输入边界，使用 update 操作并只断言不应出现内部错误。原日志全部保留，不覆盖失败或探索记录。

完整原测试、迁移、SDK 编译与代码生成校验见 [T002 同轮报告](../../SP-T002/acceptance-20260910/review.md)。原 T003 集成测试仅有一条主流程，handler 未配置 WithAuthorization，且候选只是随机 ID；PUT 只数版本，没有验证新 Proposal、command、contributor。故其绿色结果无法推翻本轮反例。

## 处置

T003 当前为 Needs_Fix。保留历史 A001 证据，不再用旧 Passed 作为下游放行依据。按原 T003 契约制定受限返修，先将上述探针转成正式失败用例，再补齐事务/输入/恢复实现并独立复审。本轮不修改业务代码，也不扩大至 T004、AI 或前端实现。

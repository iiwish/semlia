# SP-T001 后续执行准备

本文件列出详细契约对应的实施归属核对，不授权执行后续任务。SP-T002 至 SP-T007 保持 Draft；每项 Ready 前，PM 把最终精确文件范围写入该任务及自包含 packet，再派发 worker。不得把本清单当作任意修改目录的许可。

| 任务 | 契约对应的准备要求 |
| --- | --- |
| SP-T002 | 在现有来源范围外补核 `pkg/identity/id.go`、`pkg/identity/id_test.go` 的 ssnp/codrev/linrev；如 SDK 导出相应 typed ID，同步 `sdk/typescript/src/ids.ts`、`ids.test.ts`。code/lineage revision 与快照引用的 artifact 保留必须与 `internal/adapters/postgres/artifacts.go` 的 cleanup fence 衔接，并有独立 retention 测试；不能只保存已经过期的字节定位。 |
| SP-T003 | 同样核对 prodop 的 Go/SDK typed ID；创建/更新、primary candidate、后继/重引入、reservation、命令/业务摘要去重、原始 AI 输出和人工应用记录需要在本阶段的 authoring 迁移中建立精确存储结构，或在实施前明确另一个迁移归属。 |
| SP-T004 | 确认 validation attempt 的唯一约束、队列和完整有效结果选择；核对 `internal/domain/governance/validation.go` 是否需要类型扩展并加入受限范围。旧单项 rollback 防绕过、回滚后继分类、完整 before、registry CAS、来源 witness 和 binding 执行来源冻结须分别有实际回归测试。 |
| SP-T005 | 模型只是已批准生产入口的建议来源。原始输出/人工应用的存储迁移不能到此阶段才无授权临时补写；复用已建立结构，单独核对 job/run 生命周期与未知模型结果的恢复路径。 |
| SP-T006 | 独立桌面验收 harness 必须持有 project、环境、端口、数据库/卷及镜像标签，不借默认 Compose 准备样例；只验收两个约定桌面尺寸。 |
| SP-T007 | 专用检出和 Docker 环境、完整门禁、性能、四通道兼容和实际模型授权分别核实。真实模型缺配置/费用授权时继续确定性检查，但不能验收完整 AI 生产阶段。 |

Typed ID、artifact 保留、schema readiness/feature fence 的必要文件属于详细契约落地核对，不改变生产优先、无新连接器、无外部机器权限扩展的产品范围。实际迁移仍只追加新文件，且执行时复查编号；现有应用和用户改动不得被本次 T001 或后续 worker 静默覆盖。

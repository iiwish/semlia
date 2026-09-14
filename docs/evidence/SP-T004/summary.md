# SP-T004 组合验证与发布

- Attempt: SP-T004-A002
- Status: Accepted；2026-09-11 用户明确验收并授权继续 T005。
- 前置任务: SP-T003 Accepted，见 [验收复核](../SP-T003/acceptance-A003/review.md)。
- 执行方式: Direct Execute；无子代理、Git 提交、部署或真实模型调用。

## 交付结论

真实异步验证与完整结果封存、当前轮次审批、事务内重新授权、五类对象原子发布、完整清单与受保护回滚已实现。支持重复回滚、可信缺席身份重引入、已发布保留身份的后续更新，以及版本绑定的验证历史分页。

发布事务保留提案和对象审计、命令回执、outbox、全部目标与未修改对象的 pins。Git 发布投影保留完整组合归因。发布详情按完整 before/after manifest 和原操作输入授权，不依赖粗粒度工作区读取权限。

数据库使用 additive migration 26，readiness 检查版本和关键保护触发器。原 migration 1–25 的 50 个文件保持原字节，既有未知历史不补造可信证明。

## 验证

- 完整治理、投影、目录集成：96 个顶层 PASS，其中 production 28 个；一个既有 subprocess-only helper 在主进程跳过，其父级真实执行测试通过。
- 五类 create/update、重复回滚、原身份重引入、非目标保留、延迟 SQL 故障原子性、并发 head CAS、审核撤权、旧轮次失效、直接 SQL 旁路及完整发布读取权限均通过。
- 单元、PG17/18 迁移、readiness、sqlc/OpenAPI 一致性、契约与仓库检查、SDK TypeScript 类型检查通过。
- 读取权限负对照移除完整 manifest 授权时真实失败，正常源码通过。

详见 [风险复核与日志索引](A002/review.md)、[源码差异](A002/source.diff)、[文件哈希](A002/source-delta.json)。

## 验收边界

T004 已获用户验收，T005 已获继续执行授权。最大集合负载、全部并发交错、完整仓库检查、浏览器与真实模型未在本包中验证。数据库不可用时不虚构已持久化失败；进程重启后验证游标需重新查询。未操作现有开发数据库或默认 Compose 服务。

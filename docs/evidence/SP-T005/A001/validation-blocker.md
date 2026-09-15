# T005 未决定义验证阻断

状态：修复已实现，用户已明确批准共享验证器及对应测试的范围扩展。

## 真实反例

`TestProductionGenerationUnresolvedApplicationCannotPassValidation` 使用真实 PostgreSQL、真实生产 HTTP/worker 路径和本地模型协议替身：

1. 从固定来源创建未决实体草稿。
2. 模型返回 schema-valid 的 `definition=null, scope=null`，成功原文持久化，不自动应用。
3. 人工使用 `PUT + suggestionRunId + expectedVersion` 应用到版本 2。
4. submit 后运行既有生产验证器。
5. 测试期望 failed，实际得到 succeeded。

命令：`go test -count=1 -p=1 -timeout=10m ./tests/integration/governance -run '^TestProductionGenerationUnresolved'`，exit 1。原始日志：[unresolved-validation-red.log](unresolved-validation-red.log)。没有执行 publish，也没有调用付费模型。

## 原因与范围

`internal/application/governance/production_validation_worker.go` 的 schema 检查调用草稿内容检查器。该检查器按契约接受显式 null，但共享验证器缺少将未决 definition/scope 转成 blocker 的检查。草稿 schema 合法不等于具备发布条件。

共享确定性验证器对 null definition/scope 分别产生 PRODUCTION_DEFINITION_UNRESOLVED、PRODUCTION_SCOPE_UNRESOLVED blocker。草稿 schema 和原始建议保持允许 null；普通人工编辑和生成应用使用同一检查，没有生成接口特例。

获批扩展包含 `internal/application/governance/production_validation_worker.go` 及对应测试。生产验证 findings 的空 Details 规范化为与持久层一致的空对象，避免失败结果触发完整性约束而无法保存。成功流程的合成测试夹具明确提供 scope，不再把未决范围用于成功发布断言。产品范围、数据库设计、T006/T007 和付费模型授权不变。

## 当前进展

生成排队、当前配置/预算授权、调用领取及未知恢复、成功输出事务保存、人工应用原文/差异、结构化 schema、API/SDK 接入已实现。已通过的分项包括调用前/期间撤权、来源推进、草稿替换、并发幂等、排队/输出/应用提交故障和五类关联建议。

原始失败日志保留。最终验证与风险结论以 [交付报告](../summary.md) 列出的新鲜命令为准，不使用较早的绿色日志代替修复后的回归。

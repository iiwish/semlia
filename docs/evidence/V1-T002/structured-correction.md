# V1-T002-A003 结构化知识纠错

## 范围与结论

本次修改仅涉及 `internal/domain/governance/production_baseline.go`、新增的 `production_baseline_test.go` 和本证据。域层单测通过；正常 HTTP 纠错、审核、发布及回滚由 T002 验收执行者继续验证。本次未启动环境、访问数据库、调用模型或读取私有配置。

`ReplayProductionChanges` 对原子 `spec` 变更值只进行规范化 JSON 处理，保留知识引用的 `assetId/revisionId/releaseId/memberId` 和来源引用的 `snapshotId/kind/objectId/revisionId`。其余字段继续使用现有本地引用解析。完整内容检查 `InspectProductionContent`、允许的变更路径和基线前值比较均保持原实现。

## RED / GREEN

新增测试后执行：

```sh
go test -count=1 ./internal/domain/governance -run 'TestReplayProductionChangesPreservesVersionedKnowledgeSpecs|TestReplayProductionSpecCorrectionRejectsMismatchedBaselineAndContent|TestProductionSpecCorrectionKeepsContentValidation|TestReplayProductionChangesResolvesNonSpecLocalReferences'
```

有效 RED 命令退出 1：

- `analysis_model`、`metric`、`business_term` 的合法版本引用触发 `invalid governance argument: missing kind`。
- `data_asset` 的来源引用被解析为裸 ID，触发 `production content mismatch: beforeValue differs from published baseline`。
- 错误前值的测试也被错误的引用解析提前拦截，未到达预期的基线一致性校验。
- 无引用的 `business_object` 和非 `spec` 本地引用控制用例通过。

准备测试期间有两次测试夹具修正：不存在于外部测试包的 ID 帮助函数导致编译失败，以及测试误用 `replace` 操作名。改用本文件 ID 帮助函数与现有 `ChangeUpdate` 后才记录上述有效 RED，未改生产代码来适配测试。

加入最小 `spec` 分支后执行：

```sh
gofmt -w internal/domain/governance/production_baseline.go internal/domain/governance/production_baseline_test.go
go test -count=1 ./internal/domain/governance
git diff --check -- internal/domain/governance/production_baseline.go internal/domain/governance/production_baseline_test.go
```

三项均退出 0；域层全套测试输出 `ok github.com/iiwish/semlia/internal/domain/governance 0.616s`。

## 回归覆盖与边界

- 五类合法草稿知识 `spec` 更新均保留完整 before/after 规范 JSON，并能回放为目标内容。
- 分析模型覆盖嵌套 member binding、修订与成员变更；数据资产覆盖 dataset 和 field 的快照/修订引用。
- 错误 `beforeValue`、不匹配目标内容的 `afterValue`、`spec.metricRefs` 非原子路径均拒绝。
- 非法版本 ID、未知字段、数组型 `spec`、`spec` 中的 `localKey` 仍由完整内容校验拒绝。回放函数不承担替代完整内容校验的职责。
- 非 `spec` 字段的已知本地引用仍可解析；未知本地引用仍拒绝。

残余验证：本证据只证明域层修复，不宣称端到端 HTTP 纠错发布通过。未更改 Ask 幂等、迁移、执行器、权限或验收脚本。

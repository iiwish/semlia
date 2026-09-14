# 已有资产匹配阻断

Status: Resolved

用户于 2026-09-11 明确批准本文件列出的单文件扩围。目录兼容修复通过定向、全量目录与治理、race 回归；两个桌面既有资产匹配通过。最终命令与证据见 [验证结果](test-results.md)。

## 运行结果

回滚事件修复已通过真实 outbox 的 RED/GREEN：定向治理测试 4.591s 通过，全量治理 65.114s 通过。隔离桌面项目 `spacc_edcba10e920296c4` 的四项用例通过，包含十对象发布、回滚、二次回滚、刷新及清除 localStorage；恢复清单与原发布相等，原发布保持不可变。

扩展已有资产匹配验收后，项目 `spacc_bef0805106d6c69a` 的两个尺寸均在读取已发布客户资产时收到 catalog detail HTTP 500，匹配对话框保留错误而不降级。新建、生成纠正、确认、审核、发布与两次回滚在此次运行仍通过。最终结果 2 passed / 2 failed，未达到 T006 完成标准。

- 原始证据：`.semlia/production-acceptance/spacc_bef0805106d6c69a/browser/` 的 error-context.md 和 trace.zip。
- 真实对象：同目录上级 `objects-desktop.json`、`objects-compact_desktop.json`，含纠正版本、后继版本及发布清单。
- 桌面 catalog 请求 trace：`2afa31312c98fd4f84d982b94df626ec`。
- 容器、网络和卷已按 owner 清理，见该目录 `cleanup.log`。

## 定位

已在获批的 `tests/integration/governance/production_projection_test.go` 增加发布后的目录权威记录读取断言。相同定向命令现在 RED：`published catalog authority physical_bindings: resource UUID must be version 7`，exit 1，4.531s。该失败发生在首次发布后，不依赖回滚。

生产对象的粒度和键字段引用使用带前缀的 `pfd_...` 标识；`internal/adapters/postgres/catalog.go` 的 `fieldIDsFromStorage` 经 `fieldIDFromStorage` 仅使用 `identity.FromUUID` 解析存储值。目录详情读取权威记录时无法消费生产写入的字段标识。目录列表不读取这些权威详情，所以可搜索到资产而无法选择。

## 最小扩围

请求增加一个产品文件：`internal/adapters/postgres/catalog.go`。仅修复目录存储标识读取对合法 typed ID 与历史 UUID 的兼容，严格校验资源类型，不放松 UUID/授权校验，不改变迁移和生产写入格式。回归测试继续使用已获批的 `tests/integration/governance/production_projection_test.go`，覆盖发布后目录详情、键/粒度/连接字段以及回滚后匹配。

本次其他验证：前端 26 files / 227 tests 通过；typecheck、lint、make web-embed、make web-embed-check 均 exit 0。lint 保留 3 个既有 warning，构建保留大 chunk 提示。新增目录回归仍为 RED，不能把当前全量后端或 T006 宣称为绿色。未调用实际模型、未部署、未提交或推送，T007 未启动。

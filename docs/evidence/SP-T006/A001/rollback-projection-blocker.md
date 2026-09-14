# 回滚投影阻断

Status: Resolved

用户于 2026-09-11 明确批准本文列出的两文件扩围并继续完成 T006。回滚动作修复及真实 outbox 重放通过；两个桌面发布、回滚和二次回滚通过。T006 的剩余匹配阻断见 [目录匹配阻断](catalog-match-blocker.md)。

## 实际结果

隔离项目 `spacc_927ee38d4d88799f` 的真实浏览器测试在 1440x900、1024x768 均完成 SQL 来源导入、发现、十对象建模、协议替身建议、人工纠正、未确认规则阻断、明确规则确认、重验、独立审核、冻结记录的后继纠正和集合发布。首次发布投影为 `ready`。

第一次回滚创建了新发布，刷新及清除 localStorage 后仍定位到该回滚发布；但投影状态为 `failed`，两个桌面用例均在“已同步”断言失败。未降低断言，未把此次验收标为完成。

- 命令：`./scripts/dev/production-acceptance.sh --suite desktop`，exit 1，2 passed / 2 failed。
- 原始记录：`.semlia/production-acceptance/spacc_927ee38d4d88799f/browser/` 下对应 error-context.md 与 trace.zip。
- 隔离清理：同目录 `cleanup.log` 显示本次 container、network、volume 均已移除；无默认 Compose 操作。
- 前端全量组件验证：`pnpm exec vitest run --maxWorkers=2`，26 files / 225 tests passed。此后发布定位防错等专项测试为 12 passed；最终全量与嵌入包仍待完成。

## 根因

`internal/adapters/postgres/production_release_persistence.go:122` 对普通发布和回滚统一写入 `Action: "published"`，同时回滚记录带有 `RolledBackToReleaseID`。

`internal/application/projection/publisher.go:152` 要求 `action == rolled_back` 与回滚目标存在严格对应。实际回滚事件因此被投影发布器拒绝。该校验正确，不应放松。

既有 `productionAssertCompositeProjection` 直接调用 `LoadReleaseProjection` 和 Git writer，未经过 outbox 事件解码，因此无法覆盖这个反例。

## 请求的最小扩展

仅增加以下两项允许文件：

1. `internal/adapters/postgres/production_release_persistence.go`：按回滚目标存在与否生成 `rolled_back` / `published` 事件，不修改权限、事务、迁移或投影校验。
2. `tests/integration/governance/production_projection_test.go`：从真实生产发布与回滚的 outbox 事件经过真实 ReleasePublisher 和 Git writer，验证初次发布、回滚、重放及二次回滚。

批准后先补失败测试，再修复事件动作，运行定向与全量治理集成，并重新执行两个桌面的完整链路、前端全量检查与嵌入同步。T006 尚未达到 Needs_Review；T007 未启动。

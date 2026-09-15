# SP-T001 验证结果

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 新鲜应用层/HTTP 基线 | 4 packages，161 test/subtest pass，0 fail/skip；6.520 秒 | [unit result](baseline-unit.result.json) |
| 新鲜隔离 PostgreSQL 集成基线 | 4 packages，85 pass，0 fail，1 subprocess-helper skip；149.073 秒 | [integration result](baseline-integration.result.json) |
| A001 契约 TDD | 缺文件 RED exit 1；完成文档后 GREEN exit 0 | [RED](worker/red.json)、[GREEN](worker/green-handoff.json) |
| A001 仓库检查 | 首次因新增 PM packet 绝对路径 exit 2；改为 repo-relative 后 exit 0，未改测试断言 | [失败](worker/test-repository.json)、[重跑](worker/test-repository-rerun.json) |
| A002 契约 TDD | 新四组回归对 A001 RED exit 1；契约修正后 GREEN exit 0 | [RED](worker/A002-red.json)、[GREEN](worker/A002-green-final.json) |
| A002 规定门禁 | make test-contracts、make test-repository、diff check 均通过 | [A002 交付](attempts/SP-T001-A002.md) |
| PM 最终独立新鲜复跑 | contracts/repository 两包，95 test/subtest pass，0 fail/skip；6.342 秒 | [result](pm-final-verification.result.json)、[JSONL](pm-final-verification.jsonl) |
| 独立评审及复核 | 首审 1 个 P1、3 个 P2；A002 四项 Fixed，无新增阻断 | [首审](review-a001.md)、[复核](review-a002.md) |

所有 package 通过与个别辅助入口 skip 分开报告。基线 unit/integration 和 PM 最终复跑都加 `-count=1`，不依赖测试缓存。原始输出和失败保留，不把多轮重复运行相加夸大覆盖数。

本次没有执行新生产 API、迁移、真实模型、浏览器、性能、完整 make check 或部署；这些不属于 T001 已通过范围。应用代码未改变，A002 不需要将同一应用基线反复运行来制造更多证据。

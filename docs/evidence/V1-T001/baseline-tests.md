# V1-T001 Go 基线

日期：2026-09-26。源码基线：`2bf50c3`，包含本轮新增计划文档；本命令启动时没有产品实现修改。

实际命令：`go test -count=1 -p=1 -timeout=20m ./...`。

结果：退出 0。所有被执行的包通过，包含真实 PostgreSQL 的 authorization、catalog、db、discovery、embedding、governance、identity、ingestion、m1、operations、projection、usage 和 worker 集成测试。

代表性耗时：acceptance 93.064 秒、db 68.490 秒、governance 81.277 秒。未向既有 Semlia 数据库注入集成测试连接地址。

边界：测试命令的成功不代表 opt-in 项均已执行；实际模型、10,000 资产 Catalog 性能与完整栈 smoke 仍需显式启用并单列结果。这是修复前基线，不是最终候选的全量门禁证明。

前端真实浏览器基线另见 `browser-baseline.md`。后续行为修复需在最终源码重跑相应回归及完整门禁。

# M0 Requirements Checklist

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 0.4.0 |
| 状态 | Completed |
| Feature | m0-foundation |
| 最后更新 | 2026-08-12 |
| Source spec | `docs/specs/m0-foundation/plan.md` v0.4.0 Confirmed |

## 1. Checklist Scope

Reviewed artifacts:

- `docs/SSOT.md`
- `docs/adr/0001-m0-technical-foundation.md`
- `docs/specs/m0-foundation/plan.md`
- `docs/specs/m0-foundation/tasks.md`

本清单检查 M0 需求是否足以安全拆解和执行，不检查尚未存在的实现代码。

## 2. Requirement Quality Checks

- [x] 每个核心 user story 都有明确 actor、触发条件和可观察结果。[Completeness]
- [x] M0-FR-001 至 M0-FR-008 都有对应 task、验收标准和验证命令。[Traceability]
- [x] M0-NFR-001 至 M0-NFR-007 均可通过时间、平台、依赖、数据或测试环境进行验证。[Testability]
- [x] 已定义启动中、依赖不可用、配置错误、迁移失败、任务失败和并发领取等边界状态。[Coverage]
- [x] M0 不包含完整认证，因此 permission state 被明确排除并由生产 fail-closed 配置覆盖当前风险。[Scope]
- [x] “快速”“可靠”“可复现”等表述具有分钟、成功条件、幂等或 clean-tree 指标。[Clarity]
- [x] 已声明 Cube、语义资产、Agent、MCP、OIDC、Kubernetes 等 non-goals。[Scope]
- [x] 性能、可靠性、安全、隐私、可访问性、可观测性和兼容性要求均有覆盖。[NFR]
- [x] 技术选择与 SSOT 的 P-003、P-005、P-006、P-007、P-008 和 P-009 一致。[Consistency]
- [x] Git、PostgreSQL、控制面、release、contract、job 和 outbox 术语使用一致。[Terminology]
- [x] P001 使用独立 prototype package 和显式 mock boundary，不改变 M0 runtime scope 或提前定义 M1 schema。[Scope]
- [x] 每个 task 都声明依赖、并行边界、冲突、允许文件、测试目标、TDD 和证据。[Executability]
- [x] T001 的允许文件包含其 repository contract test target。[Executability]
- [x] 工作图与 task detail 中的 T003、T005、T006、T007 依赖一致。[Consistency]
- [x] Go 1.26、Node.js 24 LTS 和 pnpm 的版本、安装失败与受控变更路径已经明确。[Decision integrity]
- [x] Node.js 仅用于 Web 和 TypeScript SDK 构建，生产控制面不依赖 Node.js 运行时。[Deployment clarity]
- [x] AI、MCP、API、worker 与 CLI 均具有 Go 实现边界，M0 不引入额外语言运行时。[Architecture consistency]
- [x] 所有高影响决策均已由创始人通过 M0 计划确认。[Readiness]
- [x] T008 对 fresh-clone 发现的 doctor 版本漂移和未受控 Go module namespace 声明了精确 allowed files、RED/GREEN contract 与停止条件。[Executability]
- [x] Private incubation、历史 hosted evidence 和公开发布安全门槛在任务与文档契约中可以区分。[Clarity]

## 3. Coverage Matrix

| Requirement | User outcome | Task coverage | Acceptance coverage |
| --- | --- | --- | --- |
| M0-FR-001 | 新贡献者可启动仓库 | T001 | AC-M0-001 |
| M0-FR-002 | 客户端获得稳定契约 | T002 | AC-M0-005 |
| M0-FR-003 | API 可用且错误可诊断 | T003、T006 | AC-M0-001、AC-M0-002 |
| M0-FR-004 | Web 展示真实系统状态 | T004、T006 | AC-M0-001、AC-M0-002 |
| M0-FR-005 | 数据库可安全迁移 | T005、T006 | AC-M0-003 |
| M0-FR-006 | 后台任务可靠执行 | T005、T006 | AC-M0-004 |
| M0-FR-007 | 默认安全且可观察 | T003、T006、T007 | AC-M0-002、AC-M0-006 |
| M0-FR-008 | 贡献和发布有门禁 | T001、T007、T008 | AC-M0-005、AC-M0-006 |

## 4. Findings Summary

- Critical: 0。
- High: 0。
- Medium: 0。
- Low: 1 个已处理的工作图表达问题。

## 5. Resolution Notes

- 工作图使用 Mermaid 重新表达 T003、T005、T006、T007 的实际依赖，避免 ASCII 连线产生歧义。
- T001 的 allowed files 包含 `tests/repository/repository_contract_test.go`，使 TDD test target 位于任务所有权范围内。
- T001 execution packet 把 Go 1.26 或 Node.js 24 LTS 兼容性失败列为 stop condition；任何基线变更先修改并重新审核 TDR。
- T008 packet 把 Go module identity 与 doctor toolchain 一致性列为 fresh-clone 前置 contract；修复仅允许做路径迁移和锁定版本对齐，不得改变 M0 runtime 行为。

## 6. User Review Gate

- Approval: Inherited from confirmed M0 plan and founder's explicit 2026-08-12 continuation for T008 scope.
- Checklist result: Completed with no unresolved Critical or High finding.
- Execute condition: 一致性 analysis 必须为 Clear，且目标 task 拥有完整 packet。

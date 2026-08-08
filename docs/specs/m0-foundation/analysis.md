# M0 Spec Consistency Analysis

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 0.3.0 |
| 状态 | Completed |
| Scope | m0-foundation |
| 最后更新 | 2026-08-08 |

## 1. Inputs

- Product constitution: `docs/SSOT.md` v0.2.0 Confirmed
- Feature plan: `docs/specs/m0-foundation/plan.md` v0.2.0 Confirmed
- Technical decisions: `docs/adr/0001-m0-technical-foundation.md` v0.2.0 Confirmed
- Requirements checklist: `docs/specs/m0-foundation/checklists/requirements.md` v0.2.0 Completed
- Work graph: `docs/specs/m0-foundation/tasks.md` v0.2.0 Confirmed
- Execution packets: T001 已 Accepted；T002 A001 packet 与 OpenAPI-first 技术基线一致

## 2. Coverage

### Requirements covered by tasks

- M0-FR-001：T001。
- M0-FR-002：T002。
- M0-FR-003：T003、T006。
- M0-FR-004：T004、T006。
- M0-FR-005：T005、T006。
- M0-FR-006：T005、T006。
- M0-FR-007：T003、T006、T007。
- M0-FR-008：T001、T007、T008。
- M0-NFR-001：T001、T008。
- M0-NFR-002：T007、T008。
- M0-NFR-003：T001、T007、T008。
- M0-NFR-004：T006、T008。
- M0-NFR-005：T001、T003、T006、T007。
- M0-NFR-006：T005、T007。
- M0-NFR-007：T001、T007。

Requirements without task coverage: None.

Tasks without requirement or plan mapping: None.

Ready or Running tasks without packet: None. T002 A001 packet 完整且与当前技术决策一致。

Packets missing required fields: None at analysis time.

## 3. Constitution Check

- P-001、P-002、P-004：M0 不实现语义和 AI 行为，不产生冲突。
- P-003：T002、T005、T007 建立版本、迁移和确定性生成基础。
- P-005：T001 和 T002 保持 Git 可读、可 diff 和 clean-tree 验证。
- P-006：T002 和 T003 先建立契约与 API，T004 只消费生成客户端。
- P-007：M0 不实现执行引擎，仓库边界为 M1 Cube adapter 保留独立 integration 位置。
- P-008：T001、T003、T006 和 T007 覆盖密钥、配置、日志和供应链安全。
- P-009：所有任务均要求测试、验证、证据和独立 review。

Violations: None.

Risk accepted by user: No constitutional violation requires acceptance.

## 4. Consistency Check

- Terminology drift: None.
- Conflicting requirements or decisions: None.
- Placeholder or status conflicts: None.
- Parallel or file-conflict contradictions: None after work graph correction.
- Dependency cycles: None.
- Task IDs: T001 至 T008 连续且唯一。
- Requirement IDs: M0-FR、M0-NFR 和 AC-M0 命名稳定且无重复定义。

## 5. Non-Functional Requirements

Validation coverage:

- 性能：bootstrap、CI、fresh-clone 和 HTTP 行为均有时间目标或验收记录。
- 可靠性：迁移、幂等、并发领取、重试、dead-letter 和故障恢复有真实 PostgreSQL 测试。
- 安全：生产 fail-closed、日志脱敏、密钥、依赖和容器扫描有 task coverage。
- 隐私：M0 不导入客户数据，仓库和日志禁止真实凭据与样本。
- 可访问性：T004 覆盖键盘、ARIA、颜色和 reduced motion。
- 可观测性：T003、T005 和 T006 传播 trace ID，T007 验证门禁。
- 兼容性：T002 验证契约生成和 drift，T005 验证迁移，T007 验证构建工件。

Gaps: None blocking M0 execution.

## 6. Findings

### Low: Go patch toolchain and module directive must remain aligned

- Location: `docs/adr/0001-m0-technical-foundation.md`, TDR-002；`go.mod`；`.tool-versions`。
- Impact: Go module language version、toolchain patch 和本地版本声明漂移会破坏可复现构建或触发隐式下载。
- Resolution: T001 repository contract 同时验证 `go` directive、`toolchain` directive 和 `.tool-versions`，任何版本变更先更新 TDR 与 packet。
- Status: Resolved in execution constraints.

### Low: Model provider SDK feature lag requires a governed escape hatch

- Location: `docs/adr/0001-m0-technical-foundation.md`, TDR-002 Risks and Mitigations.
- Impact: 新模型能力若晚于其他语言进入 Go SDK，可能阻塞 provider adapter。
- Resolution: provider adapter 允许受测试的原始 HTTP 扩展，但模型输出仍必须经过统一 schema、权限和审计边界。
- Status: Resolved at architecture boundary; implementation belongs to M2.

### Low: Work graph representation could misstate dependencies

- Location: `docs/specs/m0-foundation/tasks.md`, Work Graph.
- Impact: ASCII 连线可能让执行者误以为 T005 不依赖 T003。
- Resolution: 使用显式 Mermaid 有向边，task detail 继续作为依赖权威来源。
- Status: Resolved.

### Low: T001 test ownership was incomplete

- Location: `docs/specs/m0-foundation/tasks.md`, T001 Allowed files.
- Impact: executor 无法在 allowed files 内执行已要求的 RED 测试。
- Resolution: 将 repository contract test 加入 allowed files。
- Status: Resolved.

### Low: OpenAPI 3.1 support differs across generators

- Location: `docs/adr/0001-m0-technical-foundation.md`, TDR-006。
- Impact: 在 Go generator 尚未稳定支持 OpenAPI 3.1 时提前采用会增加生成差异和维护风险。
- Resolution: M0 固定 OpenAPI 3.0.3，使用锁定的 oapi-codegen、openapi-typescript 和 oasdiff；3.1 升级必须通过独立兼容性评审。
- Status: Resolved in T002 packet.

## 7. Execute Gate

- Result: Clear for T002 execution.
- Reason: T001 已由用户明确接受；产品合同、TDR、计划和工作图保持一致；checklist 已完成；没有未解决的 Critical 或 High finding；T002 无其他未满足依赖。
- Constraint: 只允许 T002 进入 Ready。T003 至 T008 保持 Draft。

# M0 Spec Consistency Analysis

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 0.15.0 |
| 状态 | Completed |
| Scope | m0-foundation |
| 最后更新 | 2026-08-11 |

## 1. Inputs

- Product constitution: `docs/SSOT.md` v0.2.0 Confirmed
- Feature plan: `docs/specs/m0-foundation/plan.md` v0.3.0 Confirmed
- Technical decisions: `docs/adr/0001-m0-technical-foundation.md` v0.2.1 Confirmed
- Requirements checklist: `docs/specs/m0-foundation/checklists/requirements.md` v0.3.0 Completed
- Work graph: `docs/specs/m0-foundation/tasks.md` v0.4.0 Confirmed
- Product prototype design: `docs/specs/product-prototype/product-design.md` v0.1.0 Confirmed
- Execution packets: T001、T002、P001、T003、T004、T005、T006 已 Accepted；T007 本地与托管验证、evidence 和三轮 review 完成，M0-T007-A001 状态为 Needs_Review

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

Tasks without requirement or plan mapping: None. P001 映射 SSOT 产品旅程，用于产品评审，不声明满足 M0 runtime requirement。

Ready or Running tasks without packet: None. T007 packet 覆盖本地可复现门禁、最小权限 workflow、依赖/密钥/容器扫描、SBOM、checksum 和可签名 release artifact。

Packets missing required fields: None at analysis time.

## 3. Constitution Check

- P-001、P-002、P-004：M0 runtime 不实现语义和 AI 行为；P001 仅以本地 mock data 表达已确认的产品旅程，不创建生产领域行为。
- P-003：T002、T005、T007 建立版本、迁移和确定性生成基础。
- P-005：T001 和 T002 保持 Git 可读、可 diff 和 clean-tree 验证。
- P-006：T002 建立生产契约；P001 与生产路径隔离；T003 建立 API，T004 只消费生成客户端。
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

- Result: Clear for T007 founder review; the external CI run gate is satisfied.
- Local completion: CI contract RED/GREEN、完整 `make check`、独立 security/build/SBOM/release/repository validation 和三轮 review 均通过；缓存态完整本地 gate 为 61.33 秒。
- Security result: Trivy 0.73.0 immutable-digest scan 对 Go modules、production/development pnpm dependencies、repository secrets、container OS 和 Go binary 均报告 0 个 High/Critical finding。
- Artifact result: 4.2 MB archive 包含 version、完整 commit、migrations、notices 和 21-component CycloneDX 1.7 SBOM；archive 与外部 SBOM 的 SHA-256 校验通过。
- Hosted gate: Public repository `https://github.com/iiwish/semlia` 在提交 `44b0f49d8db1227e9c6c0d4dc1342fb20fd75dc3` 上完成 CI、Security 与 Linux/Darwin x amd64/arm64 Release Build 绿色运行；最长必需 CI job 约 2m28s。
- Acceptance gate: T007 已进入 `Needs_Review`；只有创始人明确接受后才能进入 `Accepted`，T008 在此之前保持 Draft。

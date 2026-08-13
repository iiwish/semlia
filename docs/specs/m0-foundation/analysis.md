# M0 Spec Consistency Analysis

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 0.19.0 |
| 状态 | Completed |
| Scope | m0-foundation |
| 最后更新 | 2026-08-13 |

## 1. Inputs

- Product constitution: `docs/SSOT.md` v0.4.0 Confirmed
- Feature plan: `docs/specs/m0-foundation/plan.md` v0.4.0 Confirmed
- Technical decisions: `docs/adr/0001-m0-technical-foundation.md` v0.2.1 Confirmed
- Requirements checklist: `docs/specs/m0-foundation/checklists/requirements.md` v0.4.0 Completed
- Work graph: `docs/specs/m0-foundation/tasks.md` v0.10.0 Confirmed
- Product prototype design: `docs/specs/product-prototype/product-design.md` v0.4.1 Confirmed
- Execution packets: T001、T002、P001、T003、T004、T005、T006、T007 已 Accepted；T008 packet `M0-T008-A001` 已完成 exact-ref validation 与三轮 review，task 状态为 `Needs_Review`

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
- M0-NFR-005：T001、T003、T006、T007 implement；T008 validates the isolated checkout and evidence boundary。
- M0-NFR-006：T005、T007 implement；T008 validates real PostgreSQL migration and worker journeys。
- M0-NFR-007：T001、T007 implement；T008 validates pinned-tool and security/release gates。

Requirements without task coverage: None.

Tasks without requirement or plan mapping: None. P001 映射 SSOT 产品旅程，用于产品评审，不声明满足 M0 runtime requirement。

Ready、Running or Needs_Review tasks without packet: None. T008 packet 覆盖 fresh-checkout、文档、工具链一致性、受控 module namespace、M0 场景、性能测量、故障恢复和 release report。

Packets missing required fields: None at analysis time.

## 3. Constitution Check

- P-001、P-002、P-004：M0 runtime 不实现语义和 AI 行为；P001 仅以本地 mock data 表达已确认的产品旅程，不创建生产领域行为。
- P-003：T002、T005、T007 建立版本、迁移和确定性生成基础。
- P-005：T001 和 T002 保持 Git 可读、可 diff 和 clean-tree 验证。
- P-006：T002 建立生产契约；P001 与生产路径隔离；T003 建立 API，T004 只消费生成客户端。
- P-007：M0 不实现执行引擎，仓库边界为 M1 Cube adapter 保留独立 integration 位置。
- P-008：T001、T003、T006 和 T007 覆盖密钥、配置、日志和供应链安全。
- P-009：所有任务均要求测试、验证、证据和独立 review。
- P-010：T008 只验证 M0 控制面与工程路径，不采集客户事实；private-incubation 和遥测边界在公开文档中保持明确。

Violations: None.

Risk accepted by user: No constitutional violation requires acceptance.

## 4. Consistency Check

- Terminology drift: None.
- Conflicting requirements or decisions: None.
- Placeholder or status conflicts: None after T007 acceptance and current Private visibility were synchronized into canonical artifacts.
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

### High: Go module namespace is not controlled by the repository owner

- Location: `go.mod` and internal Go imports.
- Impact: The module declares `github.com/semlia/semlia`, while the controlled remote is `github.com/iiwish/semlia`; publishing under an unverified namespace would make imports, provenance and future releases misleading.
- Resolution: T008 performs a literal migration to `github.com/iiwish/semlia`, adds a repository contract and verifies `go list -m`, `go mod tidy -diff`, all tests and source gates. Historical evidence patches remain immutable.
- Status: Resolved at `source-revision-redacted`; repository contract、`go list -m`、tidy and full gates pass.

### Medium: Doctor Go version drift blocks fresh-clone onboarding

- Location: `scripts/doctor.sh`, `.tool-versions`, `go.mod`, `deploy/local/Dockerfile`.
- Impact: `make doctor` rejects the canonical Go 1.26.5 toolchain because the script still requires 1.26.3.
- Resolution: T008 first adds a failing cross-file version contract, then aligns doctor to 1.26.5 and reruns repository checks.
- Status: Resolved at the validated implementation; doctor reports Go 1.26.5 with zero warnings or errors.

### Medium: Public-project wording conflicts with Private incubation

- Location: root README/security policy, T007 evidence, documentation index and M1 planning inputs.
- Impact: Contributors could infer that a public community, vulnerability-reporting channel or tag provenance is already available when the repository is private and those gates are not enabled.
- Resolution: Historical hosted evidence remains labeled with its validation-time visibility; current product and operations docs describe Private incubation and retain public-release features as gates.
- Status: Resolved in README、SECURITY、quickstart and operations documentation; public features remain explicit release gates.

### Medium: Smoke resource assertions bind to the default Compose project

- Location: `tests/smoke/local_stack_test.go`, preserved-volume and network label assertions.
- Impact: A fresh or parallel checkout using a unique `COMPOSE_PROJECT_NAME` inspects `semlia-local` resources instead of its own project, causing false failures or cross-project reads.
- Resolution: T008 derives the expected label from the active task-owned environment and keeps cleanup scoped to that project; Compose topology and runtime behavior remain unchanged.
- Status: Resolved with environment-first Compose precedence、strict project-name validation and task-owned cleanup assertions.

## 7. Execute Gate

- Result: T008 implementation、exact-ref validation and three-pass review are complete under packet `M0-T008-A001`.
- T007 predecessor gate: CI contract RED/GREEN、完整 `make check`、独立 security/build/SBOM/release/repository validation 和 T007 三轮 review 均通过；缓存态完整本地 gate 为 61.33 秒。
- T007 predecessor security and artifact: Trivy 0.73.0 immutable-digest scan 报告 0 个 High/Critical finding；4.2 MB archive 包含完整 commit、migrations、notices、21-component CycloneDX 1.7 SBOM 和通过的 SHA-256 校验。
- T007 hosted gate: 仓库在托管验证执行时为 Public；`https://github.com/iiwish/semlia` 的提交 `source-revision-redacted` 完成 CI、Security 与 Linux/Darwin x amd64/arm64 Release Build 绿色运行，最长必需 CI job 约 2m28s。当前 visibility 为 Private。
- T008 result: exact commit `source-revision-redacted` passed the canonical journey (test 1,061.55s；package 1,061.991s；exit 0). Cold 8m24.895s、warm 567ms、10m58.187s onboarding、real failure recovery、migration、worker、contract、security、release and exact-run cleanup checks all passed.
- Evidence: `docs/evidence/T008/summary.md`, `docs/evidence/T008/test-results.md`, `docs/evidence/T008/diff.patch`, `docs/specs/m0-foundation/release-report.md`.
- Review: spec-compliance、bug/code-quality and QA-acceptance passed with no blocking finding；release report status is `Ready_For_User_Review` and T008 is `Needs_Review`。
- Acceptance gate: Founder 于 2026-08-12 明确接受 T007 并授权继续。只有 founder 明确接受 post-execution M0 evidence 后，T008 与 M0 才能进入 `Accepted`；M1 planning and implementation remain blocked until then。

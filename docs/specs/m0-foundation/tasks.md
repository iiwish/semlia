# M0 Foundation Work Graph

## 元数据

| 字段 | 值 |
| --- | --- |
| Feature | m0-foundation |
| 版本 | 0.1.0 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-08 |
| Source | `docs/SSOT.md`, `docs/specs/m0-foundation/plan.md` |
| TDR | `docs/adr/0001-m0-technical-foundation.md` |

## 1. 状态定义

- `Draft`：task 已具备审核信息，但工作图尚未获得用户确认。
- `Ready`：依赖满足、checklist 和 analysis 完成、execution packet 已生成。
- `Running`：一个执行 attempt 正在进行。
- `Needs_Review`：实现和证据存在，等待规格、质量和 QA 评审。
- `Accepted`：评审通过且用户明确接受。
- `Blocked`：依赖、环境或需求问题阻止推进。

任何 task 在本文档变为 `Confirmed` 前都保持 `Draft`。任务执行前还必须完成 requirements checklist、一致性 analysis 和独立 execution packet。

## 2. 工作图

```text
E001 Repository Foundation
  M0-US-001
    T001 Repository and toolchain baseline
      |
      +--> T002 Versioned contract foundation
      |      |
      |      +--> T003 Control API vertical slice --------+
      |      +--> T004 Web system-status vertical slice --+--> T006 Local integrated environment
      |                                                   |          |
      +------------------> T005 PostgreSQL job/outbox -----+          +--> T008 Fresh-clone acceptance
                                                                     |
E002 Quality and Distribution                                         |
  M0-US-005                                                           |
    T007 CI, security and supply-chain gates -------------------------+
```

可并行边界：

- T003 与 T004 在 T002 完成后可以并行，文件所有权不重叠。
- T005 可与 T004 并行，但依赖 T003 提供应用配置和数据库连接边界。
- T007 可以在 T001 后开始工作流骨架，但最终门禁依赖 T002 至 T006 的验证命令。

## 3. Epic 与 Story

### Epic E001 Repository Foundation

Goal:

交付可运行、可迁移、可观察的 Semlia 模块化单体基础。

Stories:

- M0-US-001 新贡献者启动项目。
- M0-US-002 客户端调用控制面。
- M0-US-003 维护者安全演进数据库。
- M0-US-004 后台任务可靠运行。

Tasks:

- [ ] T001 [M0-US-001] 建立仓库与工具链基线
- [ ] T002 [M0-US-002] 建立版本化公共契约
- [ ] T003 [M0-US-002] 实现控制面 API 纵向基础
- [ ] T004 [P] [M0-US-002] 实现 Web system-status 纵向基础
- [ ] T005 [M0-US-003, M0-US-004] 实现 PostgreSQL 迁移、job 与 outbox
- [ ] T006 [M0-US-001] 组装本地完整运行环境

### Epic E002 Quality and Distribution

Goal:

让任何变更都能在本地和 CI 获得一致证据，并让全新贡献者完成可复现验收。

Stories:

- M0-US-005 贡献变更获得可信反馈。
- M0-US-001 新贡献者启动项目。

Tasks:

- [ ] T007 [M0-US-005] 建立 CI、安全与供应链门禁
- [ ] T008 [M0-US-001, M0-US-005] 完成 fresh-clone 验收与 M0 交付证据

## 4. Task Details

### T001 建立仓库与工具链基线

Status: Draft
Priority: P0
Depends on: None
Blocks: T002, T003, T004, T005, T006, T007, T008
Story / Requirement: M0-US-001, M0-FR-001, M0-FR-008, M0-NFR-003
Parallel: No
Conflicts with: None

Goal:

建立受版本约束的 Python 与 TypeScript workspace、统一根命令和完整开源仓库治理，使后续任务不再自行发明工具入口。

Allowed files:

- `README.md`
- `LICENSE`
- `NOTICE`
- `.gitignore`
- `.editorconfig`
- `.tool-versions`
- `.python-version`
- `pyproject.toml`
- `uv.lock`
- `package.json`
- `pnpm-workspace.yaml`
- `pnpm-lock.yaml`
- `Makefile`
- `CONTRIBUTING.md`
- `CODE_OF_CONDUCT.md`
- `SECURITY.md`
- `scripts/doctor.sh`

Test targets:

- `tests/repository/test_repository_contract.py`

Deliverables:

- Python 3.14 与 Node.js 24 LTS 兼容性探针。
- uv 和 pnpm workspace 根配置。
- `make bootstrap`、`make doctor`、`make clean`。
- Apache 2.0 LICENSE、NOTICE、贡献和安全文档。

Acceptance criteria:

- `make doctor` 检查必需工具、版本、端口和 Docker 能力，并提供可操作错误。
- `make bootstrap` 从无本地依赖缓存状态安装锁定依赖。
- 重复运行 bootstrap 不修改受版本控制文件。
- 仓库不包含机器专用绝对路径和真实凭据。

Definition of Done:

- RED/GREEN 证据、兼容性输出和 clean-tree 检查均存在。
- 文档命令由未参与实现的 reviewer 执行成功。

Validation commands:

- `make doctor`
- `make bootstrap`
- `uv run pytest tests/repository/test_repository_contract.py`
- `git diff --exit-code`

TDD plan:

- RED: 添加 repository contract test，确认缺失工具文件和命令时失败。
- GREEN: 创建最小根配置和治理文件使测试通过。
- REFACTOR: 消除重复版本声明，保持一个可验证版本来源。

Packet path:

- `docs/specs/m0-foundation/packets/T001.yaml`

Evidence required:

- Changed files。
- 工具版本与兼容性探针结果。
- RED/GREEN validation results。
- Fresh bootstrap 时长。
- Diff summary 和 residual risks。

### T002 建立版本化公共契约

Status: Draft
Priority: P0
Depends on: T001
Blocks: T003, T004, T007
Story / Requirement: M0-US-002, M0-FR-002, NFR-003, NFR-006
Parallel: No
Conflicts with: T003, T004 在本任务完成前不得创建重复 DTO

Goal:

建立 API、事件和跨语言类型共享的最小版本化契约，并提供确定性生成和兼容性检查。

Allowed files:

- `packages/schemas/**`
- `packages/domain/**`
- `scripts/generate-contracts.sh`
- `tests/contracts/**`
- `Makefile`
- `pyproject.toml`
- `package.json`
- `pnpm-lock.yaml`
- `uv.lock`

Test targets:

- `tests/contracts/test_schema_compatibility.py`
- `packages/schemas/tests/**`

Deliverables:

- Version、resource ID、timestamp、pagination、error envelope 和 event envelope schema。
- Python 与 TypeScript 类型生成或一致性验证。
- `make contracts` 和 `make contracts-check`。

Acceptance criteria:

- 无效 ID、时间、错误和事件负载被 schema 拒绝。
- 生成两次得到字节级一致结果。
- 修改 schema 未更新生成工件时检查失败。
- 公共 schema 不包含 M1 尚未确认的完整语义资产字段。

Definition of Done:

- 契约测试、生成确定性和兼容性检查通过。
- Spec compliance review 确认没有越过 M0 范围。

Validation commands:

- `make contracts`
- `make contracts-check`
- `uv run pytest tests/contracts`
- `pnpm --filter @semlia/schemas test`
- `git diff --exit-code`

TDD plan:

- RED: 为合法与非法 envelope 编写契约测试并确认缺少 schema 时失败。
- GREEN: 实现最小 schema、生成器和跨语言验证。
- REFACTOR: 统一格式、命名和版本声明，不增加 M1 领域字段。

Packet path:

- `docs/specs/m0-foundation/packets/T002.yaml`

Evidence required:

- Changed files。
- Schema test matrix。
- 生成确定性证明。
- RED/GREEN results、diff summary、residual risks。

### T003 实现控制面 API 纵向基础

Status: Draft
Priority: P0
Depends on: T001, T002
Blocks: T005, T006, T007, T008
Story / Requirement: M0-US-002, M0-FR-003, M0-FR-007
Parallel: Yes
Conflicts with: T004 不冲突；T005 必须等待本任务定义数据库配置边界

Goal:

实现可启动的 FastAPI 控制面，从配置加载到 health、system info、错误 envelope 和 trace ID 形成完整 HTTP 纵向路径。

Allowed files:

- `apps/api/**`
- `packages/domain/src/**`
- `tests/api/**`
- `pyproject.toml`
- `uv.lock`
- `Makefile`

Test targets:

- `tests/api/test_health.py`
- `tests/api/test_system_info.py`
- `tests/api/test_errors.py`
- `tests/api/test_config.py`

Deliverables:

- FastAPI app factory 和分层配置。
- Liveness、readiness 和 system info endpoint。
- 统一错误映射、request ID/trace ID 和结构化日志。
- 生产配置 fail-closed 检查。

Acceptance criteria:

- liveness 不依赖 PostgreSQL。
- readiness 对依赖不可用返回稳定错误码，不泄露连接信息。
- system info 返回 API/schema version，不返回密钥和内部路径。
- 所有响应可通过 header 或 body 关联 trace ID。

Definition of Done:

- API 单元和集成测试通过。
- 日志脱敏与生产配置失败路径有测试证据。
- OpenAPI 与 committed contract 无漂移。

Validation commands:

- `uv run pytest tests/api`
- `uv run ruff check apps/api packages/domain tests/api`
- `uv run pyright apps/api packages/domain`
- `make contracts-check`

TDD plan:

- RED: 先实现 endpoint、错误和配置行为测试，确认 app 尚不存在时失败。
- GREEN: 建立最小 app factory、路由、错误和 telemetry middleware。
- REFACTOR: 分离 transport、application 和 domain，不改变契约。

Packet path:

- `docs/specs/m0-foundation/packets/T003.yaml`

Evidence required:

- Changed files。
- RED/GREEN test output。
- 示例成功与失败响应。
- 日志脱敏证据、diff summary、residual risks。

### T004 实现 Web system-status 纵向基础

Status: Draft
Priority: P0
Depends on: T001, T002
Blocks: T006, T007, T008
Story / Requirement: M0-US-002, M0-FR-004, NFR-004
Parallel: Yes
Conflicts with: T003 不冲突；只通过生成客户端消费契约

Goal:

实现克制的系统状态页面，证明 Web 构建、生成客户端、错误状态和可访问性基础工作正常，不提前设计 M1 产品界面。

Allowed files:

- `apps/web/**`
- `packages/sdk-typescript/**`
- `package.json`
- `pnpm-workspace.yaml`
- `pnpm-lock.yaml`
- `Makefile`

Test targets:

- `apps/web/src/**/*.test.tsx`
- `apps/web/e2e/system-status.spec.ts`

Deliverables:

- React/Vite 应用基础。
- 由 schema 生成的 API client。
- loading、ready、dependency unavailable 和 configuration error 状态。
- 基础主题、键盘焦点、语义结构和 reduced motion 支持。

Acceptance criteria:

- 页面不使用模拟 API 响应作为生产 fallback。
- 所有状态可由屏幕阅读器识别且不只依赖颜色。
- API 错误展示稳定错误码和 trace ID，不暴露堆栈。
- Web 不包含营销 hero、语义资产假页面或不工作的导航。

Definition of Done:

- 组件、可访问性、构建和 Playwright 路径通过。
- 桌面与移动 viewport 截图无溢出和重叠。

Validation commands:

- `pnpm --filter @semlia/web lint`
- `pnpm --filter @semlia/web typecheck`
- `pnpm --filter @semlia/web test`
- `pnpm --filter @semlia/web build`
- `pnpm --filter @semlia/web test:e2e`

TDD plan:

- RED: 为四种系统状态和键盘/ARIA 行为编写失败测试。
- GREEN: 实现最小状态页和生成客户端调用。
- REFACTOR: 清理 view model 和样式，保持契约与布局测试通过。

Packet path:

- `docs/specs/m0-foundation/packets/T004.yaml`

Evidence required:

- Changed files。
- RED/GREEN results。
- Desktop/mobile screenshots。
- Accessibility results、diff summary、residual risks。

### T005 实现 PostgreSQL 迁移、job 与 outbox

Status: Draft
Priority: P0
Depends on: T001, T003
Blocks: T006, T007, T008
Story / Requirement: M0-US-003, M0-US-004, M0-FR-005, M0-FR-006
Parallel: Yes
Conflicts with: T003 的配置文件需要先稳定；不得修改 T004 文件

Goal:

建立真实 PostgreSQL 上可迁移、可审计、可并发验证的最小持久化和 worker 基础。

Allowed files:

- `apps/api/src/semlia/db/**`
- `apps/worker/**`
- `migrations/**`
- `alembic.ini`
- `tests/integration/db/**`
- `tests/integration/worker/**`
- `pyproject.toml`
- `uv.lock`
- `Makefile`

Test targets:

- `tests/integration/db/test_migrations.py`
- `tests/integration/db/test_audit.py`
- `tests/integration/worker/test_claiming.py`
- `tests/integration/worker/test_retry.py`
- `tests/integration/worker/test_outbox.py`

Deliverables:

- Alembic 基线和迁移检查命令。
- workspace identity、audit event、job 和 outbox 最小表。
- psycopg connection/transaction 边界。
- job claim、lease、retry、dead-letter 和 outbox dispatcher。

Acceptance criteria:

- 空库 upgrade、当前 revision 检查、downgrade 和再次 upgrade 通过。
- 应用启动不执行 DDL。
- 两个 worker 并发时同一 job 只有一次有效领取。
- 相同幂等键不会产生两个有效任务。
- 领域写入回滚时 outbox 不可见。

Definition of Done:

- 真实 PostgreSQL 集成测试稳定通过。
- 并发测试重复执行无偶发失败。
- 查询、索引和保留策略经过 review。

Validation commands:

- `make db-test`
- `uv run pytest tests/integration/db tests/integration/worker`
- `uv run alembic upgrade head`
- `uv run alembic downgrade base`
- `uv run alembic upgrade head`

TDD plan:

- RED: 编写迁移、并发领取、重试和事务 outbox 的失败集成测试。
- GREEN: 创建最小表、repository 和 worker loop 使测试通过。
- REFACTOR: 提取明确事务边界和 clock/backoff 接口，保持真实数据库测试通过。

Packet path:

- `docs/specs/m0-foundation/packets/T005.yaml`

Evidence required:

- Changed files 和 migration identifiers。
- RED/GREEN 与重复并发测试结果。
- `EXPLAIN` 或索引依据。
- Diff summary、rollback notes、residual risks。

### T006 组装本地完整运行环境

Status: Draft
Priority: P0
Depends on: T003, T004, T005
Blocks: T007, T008
Story / Requirement: M0-US-001, M0-FR-003, M0-FR-004, M0-FR-005, M0-NFR-004
Parallel: No
Conflicts with: T003、T004、T005 的运行入口在本任务开始前必须稳定

Goal:

使用一个安全、可重复的 Docker Compose 和根命令启动 Web、API、worker、PostgreSQL 与本地可观测性输出。

Allowed files:

- `compose.yaml`
- `compose.override.yaml`
- `.env.example`
- `deploy/local/**`
- `scripts/dev/**`
- `tests/smoke/**`
- `Makefile`
- `README.md`

Test targets:

- `tests/smoke/test_local_stack.py`
- `tests/smoke/test_dependency_failure.py`

Deliverables:

- `make dev`、`make dev-down`、`make smoke`。
- 健康检查、依赖顺序、数据 volume 和安全开发配置。
- PostgreSQL 停止与恢复的故障路径测试。

Acceptance criteria:

- 单一命令启动全部服务并等待 readiness。
- 重启 worker 不丢失可领取任务。
- 停止 PostgreSQL 时 liveness/readiness 语义正确，恢复后无需重建环境。
- 环境没有固定生产密码和主机专用路径。

Definition of Done:

- Fresh start、restart、dependency failure 和 clean shutdown smoke test 通过。
- README 的本地命令与实际行为一致。

Validation commands:

- `make dev`
- `make smoke`
- `docker compose restart worker`
- `make smoke`
- `make dev-down`

TDD plan:

- RED: 添加本地栈和依赖故障 smoke test，确认 compose 不存在时失败。
- GREEN: 实现最小 compose、健康检查和统一命令。
- REFACTOR: 去除重复环境变量和启动逻辑，保持 smoke test 通过。

Packet path:

- `docs/specs/m0-foundation/packets/T006.yaml`

Evidence required:

- Changed files。
- Fresh start 和 restart 时长。
- Smoke test results。
- 容器状态与资源摘要、diff summary、residual risks。

### T007 建立 CI、安全与供应链门禁

Status: Draft
Priority: P0
Depends on: T001, T002, T003, T004, T005, T006
Blocks: T008
Story / Requirement: M0-US-005, M0-FR-008, M0-NFR-002, S-004
Parallel: No
Conflicts with: T001 的根命令和 T006 的 compose 必须稳定

Goal:

让每个 pull request 获得可在本地复现的质量、安全和供应链反馈，并生成 M0 可验证工件。

Allowed files:

- `.github/**`
- `scripts/ci/**`
- `scripts/release/**`
- `Makefile`
- `pyproject.toml`
- `package.json`
- `README.md`
- `SECURITY.md`

Test targets:

- `tests/repository/test_ci_contract.py`
- `.github/workflows/**`

Deliverables:

- Pull request CI、scheduled security CI 和 release build workflow。
- Format、lint、type、unit、integration、migration、Web、contract drift 和 smoke gates。
- Dependency、secret、container scan。
- SBOM、checksum 和签名准备。

Acceptance criteria:

- `make check` 覆盖所有 pull request 必需验证。
- CI failure 能定位到明确命令和 artifact。
- 生成工件包含版本、commit、checksum 和 SBOM。
- 普通变更的必需 CI 目标在 10 分钟内完成。

Definition of Done:

- 本地检查通过，CI 至少完成一次完整绿色运行。
- 安全扫描没有未接受的 Critical 或 High finding。
- Workflow 权限遵循最小权限。

Validation commands:

- `make check`
- `make security-check`
- `make build`
- `make sbom`
- `uv run pytest tests/repository/test_ci_contract.py`

TDD plan:

- RED: 添加 CI contract test，确认缺少门禁、权限或本地映射时失败。
- GREEN: 实现 workflow 和根命令映射使 contract test 通过。
- REFACTOR: 优化缓存与并行，不减少门禁覆盖。

Packet path:

- `docs/specs/m0-foundation/packets/T007.yaml`

Evidence required:

- Changed files。
- Local check results 和 CI run URL。
- Job durations、scan reports、SBOM/checksum samples。
- Diff summary、residual risks。

### T008 完成 fresh-clone 验收与 M0 交付证据

Status: Draft
Priority: P0
Depends on: T001, T002, T003, T004, T005, T006, T007
Blocks: M1 planning
Story / Requirement: M0-US-001, M0-US-005, AC-M0-001 至 AC-M0-006
Parallel: No
Conflicts with: 所有实现任务必须先进入 Needs_Review 或 Accepted

Goal:

从一个无项目缓存的全新 clone 独立验证 M0 用户旅程、性能目标、故障路径、升级路径和公开文档。

Allowed files:

- `README.md`
- `docs/quickstart.md`
- `docs/operations/local-development.md`
- `docs/operations/troubleshooting.md`
- `docs/specs/m0-foundation/analysis.md`
- `docs/specs/m0-foundation/release-report.md`
- `tests/acceptance/**`

Test targets:

- `tests/acceptance/test_fresh_clone.py`
- `tests/acceptance/test_m0_scenarios.py`

Deliverables:

- Quickstart、故障诊断和本地运行文档。
- AC-M0-001 至 AC-M0-006 的独立证据。
- M0 spec compliance、engineering quality 和 QA review。
- M0 release report 和 M1 readiness 判断。

Acceptance criteria:

- 新贡献者在 15 分钟内完成从 clone 到首个成功请求。
- Bootstrap、CI 和响应时间目标有真实测量。
- 所有故障场景提供稳定错误和恢复步骤。
- 文档中的每条命令在干净环境执行成功。

Definition of Done:

- 所有 M0 requirements 与 acceptance scenarios 有证据映射。
- Review 没有 blocking finding。
- 用户明确接受 M0 后任务才可进入 Accepted。

Validation commands:

- `make clean`
- `make bootstrap`
- `make dev`
- `make check`
- `make smoke`
- `uv run pytest tests/acceptance`
- `make dev-down`

TDD plan:

- RED: 在隔离 clone 中执行 acceptance harness，记录缺失文档或行为失败。
- GREEN: 只修复 M0 验收直接发现的问题并补回归测试。
- REFACTOR: 整理文档和诊断输出，不扩大 M0 功能范围。

Packet path:

- `docs/specs/m0-foundation/packets/T008.yaml`

Evidence required:

- Fresh-clone 环境说明和命令记录。
- Acceptance、performance 和 failure-recovery results。
- Review findings 与 resolution。
- Release report、diff summary、residual risks。

## 5. 用户审核闸门

- Approval: Confirmed by founder on 2026-08-08
- Accepted graph: E001、E002 和 T001 至 T008 的范围、依赖、并行边界、验证命令与 Definition of Done。
- Next gate: 完成 requirements checklist 和一致性 analysis；只为第一个可执行任务 T001 生成 packet。
- Execution rule: 没有已审核 packet、干净 worktree 和明确执行授权时，不开始任何 task。

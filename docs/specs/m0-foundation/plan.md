# M0 Foundation Plan

## 元数据

| 字段 | 值 |
| --- | --- |
| Feature ID | m0-foundation |
| 版本 | 0.4.0 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-12 |
| 产品合同 | `docs/SSOT.md` v0.4.0 Confirmed |
| 技术决策 | `docs/adr/0001-m0-technical-foundation.md` |
| 工作图 | `docs/specs/m0-foundation/tasks.md` |
| 审核 | 2026-08-12 经创始人确认继续完成文档、fresh-clone 验收和核心产品闭环 |

## 1. 目标

建立 Semlia 的可复现工程基础，使贡献者能够启动、验证和修改一个包含 Web、API、worker、PostgreSQL 和版本化契约的最小系统，并让 M1 可以直接实现 Cube 导入到语义资产发布的纵向闭环。

M0 的成功不是创建大量目录，而是证明以下事实：

- 全新环境能够按文档完成安装。
- 一个请求可以经过 Web 或 HTTP 进入 API，并获得稳定契约和 trace ID。
- PostgreSQL 迁移可以从空库升级并通过回滚验证。
- 一个后台任务可以被可靠领取、重试并产生 outbox 事件。
- 本地 `make check` 与 CI 使用相同质量门禁。
- 仓库在 Private incubation 期间具备 Apache-2.0、供应链、安全与贡献基础，并明确公开发布前仍需通过的社区和 GitHub 安全门槛。

## 2. 用户故事

### M0-US-001 新贡献者启动项目

作为第一次接触 Semlia 的贡献者，我可以在 15 分钟内完成依赖安装、启动本地环境、运行测试并定位项目结构。

### M0-US-002 客户端调用控制面

作为 Semlia 客户端开发者，我可以调用版本化 health API，获得稳定响应、错误结构和 trace ID，并使用由同一契约生成的 Go 边界类型与 TypeScript 客户端。

### M0-US-003 维护者安全演进数据库

作为维护者，我可以从空 PostgreSQL 创建 schema，验证迁移、重复执行和回滚行为，而应用不会在启动时偷偷修改数据库。

### M0-US-004 后台任务可靠运行

作为平台开发者，我可以提交一个测试任务，观察 worker 的领取、成功、重试、dead-letter 和 outbox 行为，并通过 trace 关联完整过程。

### M0-US-005 贡献变更获得可信反馈

作为贡献者，我可以在本地运行与 CI 等价的检查，并在 10 分钟内获得格式、类型、测试、迁移、构建和安全反馈。

## 3. 功能需求

### M0-FR-001 仓库与工具链

- 仓库采用 TDR 定义的 monorepo 布局。
- Go、Node.js 和 pnpm 版本被明确固定。
- 根目录提供 `make bootstrap`、`make dev`、`make check` 和 `make clean`。
- 锁文件和生成规则可重复执行且不会制造无意义 diff。

### M0-FR-002 公共契约

- 建立 API version、resource ID、timestamp、pagination、error response 和 event envelope 的最小 schema。
- schema 具有机器可读版本和兼容性测试。
- Go transport 类型与 TypeScript 客户端由同一契约产生并验证一致。
- 不提前定义 M1 的完整语义资产 schema。

### M0-FR-003 控制面 API

- API 提供 `/health/live`、`/health/ready` 和 `/api/v1/system/info`。
- readiness 检查数据库迁移状态，不把临时依赖波动错误报告为进程死亡。
- 每个响应包含或关联 trace ID。
- 配置缺失、输入错误和内部错误使用统一错误 envelope。

### M0-FR-004 Web 基础

- Web 使用生成客户端调用 system info 和 readiness。
- 页面清楚区分启动中、可用、依赖不可用和配置错误。
- 核心状态满足键盘操作和 WCAG 2.2 AA 基础要求。
- Web 不在 M0 实现营销首页、资产管理页面或自定义设计系统。

### M0-FR-005 数据库基础

- golang-migrate 从版本化 SQL 文件在空 PostgreSQL 创建最小控制面 schema。
- 基础表覆盖 workspace identity、audit event、job 和 outbox。
- 所有租户数据表包含 `workspace_id` 或明确记录为何属于全局表。
- 迁移测试覆盖 upgrade、重复检查和 downgrade 到 M0 基线。

### M0-FR-006 Worker 与 outbox

- worker 使用数据库锁安全领取任务。
- 任务记录 attempt、lease、available_at、幂等键和最终状态。
- 失败执行有界退避，达到上限进入 dead-letter 状态。
- 领域状态与 outbox 写入共享事务。
- 测试证明两个 worker 不会同时成功领取同一任务。

### M0-FR-007 可观测性和安全默认值

- API、worker 和数据库操作传播 trace context。
- 日志结构化并过滤凭据和敏感配置。
- 生产模式缺少关键密钥、允许来源或数据库安全配置时拒绝启动。
- `.env.example` 只包含安全示例，不包含可误用的真实凭据。

### M0-FR-008 CI、供应链与开源治理

- GitHub Actions 执行 `make check` 对应的分层检查。
- 仓库包含 Apache 2.0 LICENSE、NOTICE、贡献指南、行为准则和安全策略。
- CI 执行依赖、密钥和容器扫描。
- 构建产生 SBOM、校验和和可签名工件。
- PR 模板要求关联需求、测试证据和残余风险。

## 4. 非功能要求

- M0-NFR-001：推荐开发机上首次 bootstrap 在 10 分钟内完成，缓存后在 2 分钟内完成。
- M0-NFR-002：普通 pull request 的必需 CI 检查在 10 分钟内完成。
- M0-NFR-003：所有测试可在 macOS 和 Linux 执行，Windows 通过容器开发路径支持。
- M0-NFR-004：本地开发不依赖付费 SaaS、云账号或外部 LLM Key。
- M0-NFR-005：仓库不包含密钥、个人路径、默认生产密码或客户数据。
- M0-NFR-006：空库迁移和健康检查在 CI 中使用真实 PostgreSQL，不使用 SQLite 替代。
- M0-NFR-007：基础镜像和直接依赖使用受支持版本并由自动化依赖更新监控。

## 5. 范围

### 包含

- 项目根文件、许可证和社区治理。
- Go module 与 TypeScript workspace。
- 最小 API、Web 和 worker 进程。
- 公共 schema 和生成类型。
- PostgreSQL 迁移、基础表和集成测试。
- 本地 Docker Compose 环境。
- 统一命令、CI、安全扫描、SBOM 和基础可观测性。
- 从 clone 到首个 API 请求的 quickstart。

### 不包含

- Cube 导入和连接器业务实现。
- 语义资产、revision、relation、proposal、review 和 release 完整模型。
- 登录页面、OIDC 集成和完整 RBAC。
- MCP、CLI 业务命令和 SDK 发布。
- S3 证据上传业务流程。
- Kubernetes、Helm、云托管和高可用部署。
- Agent、LLM provider 和模型评测。

### 产品评审原型

`prototypes/product` 提供独立、可点击的产品评审原型，用于在继续后端基础建设前确认 Semlia 的核心信息架构和用户旅程。原型只使用显式标识的本地 mock data，不调用或替代生产 API，不计入 M0 runtime 验收，也不构成已确认的领域 schema。原型中被用户接受的产品决策进入后续 M1 specification，未接受的设计可以直接重做。

## 6. 交付顺序

1. T001 固定工具链、根命令、许可证和仓库治理。
2. T002 固化最小公共 schema 与兼容性检查。
3. P001 交付隔离的产品评审原型并由创始人确认核心旅程。
4. T003 和 T004 分别实现 API 与 Web 基础，可在文件边界不冲突时并行。
5. T005 实现 PostgreSQL 迁移、job 和 outbox 行为。
6. T006 组装可重复启动的本地完整环境。
7. T007 将格式、类型、测试、迁移、构建和安全检查接入 CI。
8. T008 从全新 clone 执行 quickstart、安装、升级和故障诊断验收。

## 7. 验收场景

### AC-M0-001 Fresh clone

在没有项目缓存的受支持开发环境中，贡献者根据 quickstart 执行统一命令，启动全部服务并访问 system info。

### AC-M0-002 Dependency failure

停止 PostgreSQL 后，liveness 保持成功，readiness 返回稳定错误代码和 trace ID，Web 展示依赖不可用状态。

### AC-M0-003 Migration safety

CI 从空 PostgreSQL 执行全部 upgrade，检查当前 revision，执行 M0 范围内 downgrade，再次 upgrade，并确认 schema 一致。

### AC-M0-004 Concurrent workers

两个 worker 同时处理同一队列时，每个 job 只有一个成功 lease，失败任务按策略重试并最终可进入 dead-letter。

### AC-M0-005 Contract drift

开发者修改 API schema 但未更新生成客户端时，`make check` 和 CI 明确失败并给出修复命令。

### AC-M0-006 Security baseline

密钥扫描、依赖扫描、容器扫描和日志脱敏测试通过；生产配置缺少必需密钥时 API 拒绝启动。

## 8. 退出标准

- M0-FR-001 至 M0-FR-008 全部有通过证据。
- M0-NFR-001 至 M0-NFR-007 全部满足或存在用户明确接受的风险。
- `make bootstrap`、`make dev` 和 `make check` 在干净环境通过。
- AC-M0-001 至 AC-M0-006 通过独立 QA。
- TDR、任务 evidence 和 release report 完成评审。
- 创始人明确接受 M0 后才能进入 M1。

## 9. 风险与控制

| 风险 | 控制 |
| --- | --- |
| 为未来需求预建过多抽象 | 每个包和基础表必须映射到 M0 requirement 或 M1 已确认入口 |
| 跨语言 schema 生成链不稳定 | 保持 schema 小且确定，锁定生成器并进行 clean-tree 检查 |
| Docker 环境掩盖原生开发问题 | CI 同时运行原生语言测试和容器集成测试 |
| CI 过慢 | 记录每个 job 用时，使用缓存和并行，保持 10 分钟目标 |
| M0 没有用户可见价值 | 以可运行 system status 纵向路径和 M1 readiness 为退出条件，不继续扩张基础设施 |

## 10. 审核闸门

- Approval: Confirmed by founder on 2026-08-08; T008 continuation and release-readiness remediation reconfirmed on 2026-08-12
- Accepted scope: M0-FR-001 至 M0-FR-008、M0-NFR-001 至 M0-NFR-007、AC-M0-001 至 AC-M0-006。
- Execution rule: 实现任务必须遵循已确认工作图、checklist、analysis 和 execution packet。

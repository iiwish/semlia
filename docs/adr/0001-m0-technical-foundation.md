# TDR-0001: M0 Technical Foundation

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 0.1.0 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-08 |
| 来源 | `docs/SSOT.md` v0.1.0 Confirmed |
| 计划 | `docs/specs/m0-foundation/plan.md` |
| 工作图 | `docs/specs/m0-foundation/tasks.md` |
| 审核 | 2026-08-08 经创始人确认 |

## 1. 决策摘要

M0 建立一个可在单机和 CI 中完整运行的模块化单体。Python 控制面、TypeScript Web、PostgreSQL、Git 和 S3 兼容对象存储共享一个 monorepo，通过版本化领域契约连接。开发工具采用 Python 3.14、uv workspace、Node.js 24 LTS 和 pnpm workspace。

M0 不实现完整语义业务能力。它交付可验证的工程底座，使 M1 可以在不重做仓库、运行时、迁移、契约、测试和发布体系的前提下开发第一条 Semantic Registry 纵向闭环。

## 2. Constitution Check

| SSOT 原则 | 满足方式 |
| --- | --- |
| P-003 发布版本不可变 | 契约和迁移从第一天版本化，CI 检查生成工件差异 |
| P-005 Git-native | 仓库、文档、schema 和开发流程都可由标准 Git 工具操作 |
| P-006 Headless first | API 和领域契约先于产品 UI 业务页面 |
| P-007 开放执行生态 | Cube 适配器位于独立 integration 边界，不进入核心领域包 |
| P-008 默认安全 | 密钥、样本数据、日志和服务权限从本地环境起就遵循最小暴露 |
| P-009 世界一流来自可靠性 | M0 强制测试、迁移、可观测性、安全扫描和可复现安装 |

Constitution violations: None.

## 3. 支持材料

- 已确认产品合同：`docs/SSOT.md`
- M0 交付计划：`docs/specs/m0-foundation/plan.md`
- M0 工作图：`docs/specs/m0-foundation/tasks.md`
- Node.js 发布策略：[Node.js Releases](https://nodejs.org/en/about/previous-releases)
- Python 3.14 稳定版本：[Python 3.14](https://www.python.org/downloads/release/python-3140/)
- uv workspace：[Using workspaces](https://docs.astral.sh/uv/concepts/projects/workspaces/)
- pnpm workspace：[pnpm workspaces](https://pnpm.io/workspaces)

## 4. 技术决策

### TDR-001 模块化单体 monorepo

Decision:

- 单一 Git 仓库承载 API、worker、Web、领域包、契约、SDK 和集成。
- 运行时首先部署为 API、worker、Web 三个进程，代码共享同一领域层。
- 模块通过公开包接口和 schema 通信，禁止跨模块直接读取内部表或私有实现。

Requirement mapping:

- P-006、P-007、NFR-006、M0-FR-001。

Rationale:

- 当前团队规模不需要微服务的独立部署和组织成本。
- monorepo 能对跨语言契约、生成客户端和端到端测试执行原子变更。
- 进程边界保留后续独立扩缩容空间。

Alternatives considered:

- 多仓库：边界清晰，但 M0 会增加版本协调和本地开发成本。
- 微服务：提供故障隔离，但当前没有流量和团队所有权证据支持其复杂度。
- 单一 Python UI/API 仓库：简单，但会牺牲前端生态和类型契约质量。

Risks:

- 模块化单体可能演变为无边界大包。
- Python 与 TypeScript 工具链可能产生重复命令和缓存。

Mitigations:

- 每个模块声明公共 API、依赖方向和所有权。
- 根目录只提供统一入口，具体依赖仍由 uv 和 pnpm 锁定。
- CI 检查循环依赖和生成契约是否干净。

Task impact:

- T001、T002、T003、T004、T007。

### TDR-002 Python 3.14、FastAPI 与 uv workspace

Decision:

- API、worker、CLI 和 MCP 服务使用 Python 3.14。
- HTTP 层采用 FastAPI，边界模型采用 Pydantic。
- Python workspace、依赖锁定和命令执行采用 uv。
- 核心领域逻辑保持框架无关，不在实体和值对象中引入 FastAPI 或数据库依赖。

Requirement mapping:

- P-006、NFR-002、NFR-006、M0-FR-002、M0-FR-003。

Rationale:

- Python 对 AI、数据连接器和 MCP 生态适配成本较低。
- 当前 Fluxale 原型已经使用 FastAPI、Pydantic 和 psycopg，可复用经验而不复制原型结构。
- uv workspace 提供单锁文件和多包工作区，适合 API、CLI 和领域包共同演进。

Alternatives considered:

- Go 控制面加 Python Agent 服务：运行效率更高，但增加跨服务契约和部署复杂度。
- Poetry：成熟，但 uv 对工作区、Python 管理和执行入口更统一。
- Django：内建能力完整，但 Semlia 更需要显式领域边界和 API-first 结构。

Risks:

- Python 3.14 的部分第三方库兼容性可能滞后。
- FastAPI 模型可能渗入核心领域。

Mitigations:

- M0 在锁定依赖前验证 PostgreSQL、OpenTelemetry、测试和构建依赖兼容性。
- API DTO、领域对象和持久化模型分层。
- CI 保留 Python 3.13 兼容测试作为降级信号，正式最低版本由 M0 验证结果决定。

Task impact:

- T001、T002、T003、T004。

### TDR-003 React、TypeScript、Node.js 24 LTS 与 pnpm

Decision:

- Web 使用 React、TypeScript 和 Vite。
- Node.js 固定在 24 LTS 系列，JavaScript workspace 使用 pnpm。
- OpenAPI 和 JSON Schema 生成 TypeScript 客户端与类型，禁止手写重复 API DTO。

Requirement mapping:

- P-006、NFR-004、NFR-006、M0-FR-004。

Rationale:

- React 生态适合复杂治理工作台、图谱和 diff 交互。
- TypeScript 与生成契约降低前后端漂移。
- pnpm workspace 提供高效、严格的依赖组织方式。

Alternatives considered:

- Next.js：适合服务端渲染网站，但 M0 产品是认证后的应用工作台。
- Vue：同样可行，但当前项目经验和组件基础在 React。
- npm workspace：工具更普遍，但依赖边界和安装效率弱于 pnpm。

Risks:

- Web 可能在领域 API 稳定前自行创造状态模型。

Mitigations:

- Web 只依赖生成客户端和 UI view model。
- 契约漂移在 CI 中作为失败处理。

Task impact:

- T001、T004、T007。

### TDR-004 PostgreSQL 控制面与 Alembic 迁移

Decision:

- PostgreSQL 是控制面唯一事务数据库，开发和 CI 固定 PostgreSQL 18，兼容目标为 PostgreSQL 17 及以上。
- 使用 psycopg 3 和显式 repository，迁移使用 Alembic。
- M0 建立 workspace、audit event、job 和 outbox 的最小基础表，不提前创建完整 M1 资产模型。
- 应用启动只检查迁移状态，不自动执行 DDL 或写入演示数据。

Requirement mapping:

- D-003、D-004、NFR-002、NFR-003、M0-FR-005。

Rationale:

- PostgreSQL 同时满足事务、JSONB、关系查询、任务锁和早期搜索需求。
- 显式迁移和真实数据库测试能避免原型式启动建表带来的数据风险。

Alternatives considered:

- SQLAlchemy ORM：提高常规 CRUD 速度，但容易隐藏关键查询和事务边界；M0 保留未来引入 SQLAlchemy Core 的可能性。
- SQLite 作为服务端数据库：安装简单，但不能证明目标并发、锁和迁移行为。
- MongoDB：灵活 schema 不抵消 Semlia 对关系、版本和事务一致性的要求。

Risks:

- 直接使用 psycopg 可能增加 repository 样板代码。
- 过早设计完整领域 schema 会固化错误模型。

Mitigations:

- 只实现 M0 运行所需表，领域数据模型在 M1 单独审核。
- 使用小型 row mapper 和测试工厂，不创建通用 ORM 抽象层。

Task impact:

- T003、T005、T006、T007。

### TDR-005 PostgreSQL job 与 transactional outbox

Decision:

- 后台任务使用 PostgreSQL job table 和 `FOR UPDATE SKIP LOCKED` 领取。
- 领域写入与 outbox event 在同一事务提交。
- worker 处理器必须幂等，失败使用有界重试和 dead-letter 状态。

Requirement mapping:

- D-004、NFR-002、NFR-005、M0-FR-006。

Rationale:

- M0 不需要 Redis、Kafka 或 Temporal 的额外运行和一致性成本。
- PostgreSQL 能保证领域状态与待发布事件的一致提交。

Alternatives considered:

- Redis queue：吞吐高，但引入第二套持久化与一致性问题。
- Kafka：适合大规模事件流，但 M0 没有对应负载。
- Temporal：适合长事务和复杂编排，但当前任务生命周期尚未形成。

Risks:

- 高频事件可能扩大 PostgreSQL 表和 vacuum 压力。

Mitigations:

- 分离 job 和 audit 保留策略，增加队列积压和表膨胀指标。
- 只有基准证明瓶颈后才迁移专用系统。

Task impact:

- T005、T006、T007。

### TDR-006 Contract-first API 与稳定错误模型

Decision:

- OpenAPI、JSON Schema、事件 envelope 和错误代码在 `packages/schemas` 中版本化。
- FastAPI 生成的 OpenAPI 必须与提交的规范工件一致。
- 错误响应统一包含 `code`、`message`、`traceId`、`details` 和可选 `retryable`。
- 所有公共 ID、时间、分页、幂等键和版本字段拥有统一格式。

Requirement mapping:

- P-004、P-006、NFR-003、NFR-006、M0-FR-002。

Rationale:

- REST、MCP、CLI、SDK 和 Web 必须共享同一领域语义。
- 稳定错误码和 trace ID 是可支持开源产品的基础。

Alternatives considered:

- UI 与 API 各自定义类型：初期快，但必然产生漂移。
- GraphQL first：查询灵活，但会增加 schema、缓存和授权复杂度，且不能替代命令契约。

Risks:

- 生成代码可能制造噪声和升级摩擦。

Mitigations:

- 只提交需要发布的生成工件，生成过程必须确定且可重复。
- CI 使用 clean-tree check 发现漂移。

Task impact:

- T002、T004、T007。

### TDR-007 OpenTelemetry 与结构化日志

Decision:

- API 和 worker 从第一个可运行版本开始接入 OpenTelemetry。
- 日志使用结构化 JSON，开发环境允许可读格式。
- 每个请求、任务和 outbox 事件传播 trace ID。
- 日志过滤密钥、连接串、Authorization header 和受控样本。

Requirement mapping:

- P-008、NFR-005、M0-FR-007。

Rationale:

- 连接器和 Agent 链路天然跨步骤，事后补追踪成本高。
- 世界一流自托管产品必须提供可操作而非仅供开发者阅读的故障信息。

Alternatives considered:

- 仅标准日志：简单，但无法统一请求、任务和后续 Agent trace。
- 绑定单一可观测性厂商：上手快，但不符合开放和自托管定位。

Risks:

- 追踪配置增加初期代码和测试量。

Mitigations:

- M0 只建立 provider、context propagation 和最小关键 span。
- 默认使用无外部依赖的 console 或 no-op exporter。

Task impact:

- T003、T005、T007。

### TDR-008 GitHub Actions 与供应链门禁

Decision:

- CI 首先支持 GitHub Actions，同时所有检查可由 `make check` 在本地执行。
- 门禁覆盖格式、lint、类型、单元、PostgreSQL 集成、Web 测试、构建、迁移和 schema drift。
- 依赖、密钥和容器扫描在 M0 启用。
- 发布工件使用锁文件、校验和、SBOM 和签名。

Requirement mapping:

- P-009、S-004、NFR-002、M0-FR-008。

Rationale:

- 开源贡献者需要与 CI 一致的本地验证入口。
- 供应链能力越晚加入，历史发布和构建脚本越难补齐。

Alternatives considered:

- 只提供 GitHub Action 命令：减少脚本，但本地重现体验差。
- 首版不做签名和 SBOM：更快，但与 SSOT 的公开供应链承诺冲突。

Risks:

- M0 CI 时间过长，降低贡献体验。

Mitigations:

- 分层缓存和并行 job，pull request 运行必要门禁，定时任务运行扩展矩阵。
- 目标是普通变更 10 分钟内得到完整必需检查结果。

Task impact:

- T001、T007、T008。

## 5. 跨决策风险

| 风险 | 级别 | 缓解 |
| --- | --- | --- |
| M0 变成长期平台建设，迟迟没有 M1 用户价值 | High | M0 只交付 M1 必需底座，退出后立即进入 Cube 纵向闭环 |
| 工具链过多导致贡献门槛高 | Medium | 根目录提供 `make bootstrap`、`make dev`、`make check` 三个主入口 |
| 契约和代码生成过度设计 | Medium | 只定义 M0 health、error、event envelope 和 identity 基础类型 |
| Python 3.14 依赖兼容不足 | Medium | T001 先完成兼容性探针，失败时以 Python 3.13 作为受记录降级 |
| 自托管安全默认值不可靠 | High | 禁止默认密码，生成开发密钥，生产配置缺失时拒绝启动 |

## 6. 任务后果

- T001 必须先验证工具链兼容性和根命令体验。
- T002 在 API 和数据库之前固化最小公共契约。
- T003 与 T004 可在 T002 后并行，但不得共享未声明的内部类型。
- T005 在 API 和数据库基础上实现任务与 outbox 的最小行为。
- T006 组装本地完整运行环境，不创建生产部署抽象。
- T007 将所有已有验证收敛到本地和 CI 的同一命令。
- T008 以新贡献者视角验证从 clone 到首个请求的完整路径。

## 7. 用户审核闸门

- Approval: Confirmed by founder on 2026-08-08
- Accepted decisions: TDR-001 至 TDR-008。
- Execution rule: 每个 M0 task 仍须完成 checklist、analysis 和独立 execution packet 才能进入 `Ready`。

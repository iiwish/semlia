# TDR-0001: M0 Technical Foundation

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 0.2.1 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-08 |
| 来源 | `docs/SSOT.md` v0.2.0 Confirmed |
| 计划 | `docs/specs/m0-foundation/plan.md` |
| 工作图 | `docs/specs/m0-foundation/tasks.md` |
| 审核 | 2026-08-08 经创始人确认 |

## 1. 决策摘要

M0 建立一个可在单机和 CI 中完整运行的模块化单体。Go 控制面、TypeScript Web、PostgreSQL、Git 和 S3 兼容对象存储共享一个 monorepo，通过版本化领域契约连接。开发工具采用 Go 1.26、Node.js 24 LTS 和 pnpm workspace；Node.js 只用于 Web 与 TypeScript SDK 的开发和构建，生产控制面不依赖 Node.js 运行时。

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
- Go toolchain 管理：[Go Toolchains](https://go.dev/doc/toolchain)
- Go module 布局：[Organizing a Go module](https://go.dev/doc/modules/layout)
- MCP SDK 等级：[MCP SDKs](https://modelcontextprotocol.io/docs/sdk)
- OpenAI Go SDK：[openai-go](https://github.com/openai/openai-go)
- pnpm workspace：[pnpm workspaces](https://pnpm.io/workspaces)

## 4. 技术决策

### TDR-001 模块化单体 monorepo

Decision:

- 单一 Git 仓库承载 API、worker、Web、领域包、契约、SDK 和集成。
- Go 代码构建为单一 `semlia` 工件，通过 server、worker、mcp、migrate 和 doctor 子命令承担不同运行角色；Web 静态产物可嵌入该工件或独立部署。
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
- 单一 Go 服务端渲染 UI：部署简单，但会牺牲复杂治理工作台、图谱和 diff 交互所需的前端生态。

Risks:

- 模块化单体可能演变为无边界大包。
- Go 与 TypeScript 工具链可能产生重复命令和缓存。

Mitigations:

- 每个模块声明公共 API、依赖方向和所有权。
- 根目录只提供统一入口，具体依赖仍由 Go modules 和 pnpm 锁定。
- CI 检查循环依赖和生成契约是否干净。

Task impact:

- T001、T002、T003、T004、T007。

### TDR-002 Go 1.26 控制面与单一运行工件

Decision:

- API、worker、CLI、MCP 和迁移命令使用 Go 1.26。
- HTTP 层采用标准库 `net/http`，公共边界由 OpenAPI 生成严格类型和 handler 接口。
- `go.mod` 固定语言和 toolchain 基线，Go modules 管理依赖与校验和。
- 核心领域逻辑保持 transport、数据库、具体 LLM 供应商和 MCP SDK 无关。
- Web 构建产物默认通过 `go:embed` 进入发布二进制，同时保留独立静态部署能力。

Requirement mapping:

- P-006、NFR-002、NFR-006、M0-FR-002、M0-FR-003。

Rationale:

- Semlia 的核心 AI 能力是模型调用、工具编排、结构化输出、可恢复工作流、策略和审计，而不是模型训练；Go 能以显式状态机可靠承载这些控制面职责。
- OpenAI 与 MCP 均提供官方 Go SDK，MCP Go SDK 属于 Tier 1，关键协议能力不要求额外运行时。
- 单一跨平台工件降低自托管安装、升级、容器构建、供应链扫描和故障诊断成本。
- Go 的并发、context cancellation、静态类型和低常驻资源适合 API、worker、connector 和 Agent run。

Alternatives considered:

- Python 控制面：AI 与数据生态丰富，原型速度快，但解释器、依赖、CLI 分发和多进程部署增加长期自托管成本。
- 全栈 TypeScript：语言统一且 Agent 生态活跃，但服务端运行时、依赖树和单文件分发不如 Go 控制面克制。
- Rust 控制面：性能和内存安全优秀，但 M0 迭代速度与开源贡献门槛不符合当前阶段。

Risks:

- 新模型能力可能先出现在其他语言 SDK。
- 生成的 transport 类型可能渗入核心领域。
- 单一二进制中的多个运行角色可能形成隐式耦合。

Mitigations:

- provider adapter 保留类型化扩展和原始 HTTP escape hatch，不因 SDK 发布节奏阻塞协议能力。
- transport DTO、领域对象和持久化模型分层，生成代码只存在于边界目录。
- server、worker 和 MCP 共享 application/domain 包但通过独立 composition root 装配和测试。
- 只有出现必须依赖本地模型、训练框架或专用科学计算库的已确认能力时，才通过独立插件进程引入额外运行时。

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

### TDR-004 PostgreSQL 控制面与版本化 SQL 迁移

Decision:

- PostgreSQL 是控制面唯一事务数据库，开发和 CI 固定 PostgreSQL 18，兼容目标为 PostgreSQL 17 及以上。
- 使用 pgx v5、sqlc 和显式 repository，迁移使用 golang-migrate 管理的版本化 SQL 文件。
- M0 建立 workspace、audit event、job 和 outbox 的最小基础表，不提前创建完整 M1 资产模型。
- 应用启动只检查迁移状态，不自动执行 DDL 或写入演示数据。

Requirement mapping:

- D-003、D-004、NFR-002、NFR-003、M0-FR-005。

Rationale:

- PostgreSQL 同时满足事务、JSONB、关系查询、任务锁和早期搜索需求。
- 显式迁移和真实数据库测试能避免原型式启动建表带来的数据风险。

Alternatives considered:

- 通用 ORM：提高常规 CRUD 速度，但容易隐藏关键查询和事务边界；Semlia 优先保留可审查 SQL 和生成的类型安全调用。
- SQLite 作为服务端数据库：安装简单，但不能证明目标并发、锁和迁移行为。
- MongoDB：灵活 schema 不抵消 Semlia 对关系、版本和事务一致性的要求。

Risks:

- pgx 与 sqlc 仍可能产生 repository 适配代码。
- 过早设计完整领域 schema 会固化错误模型。

Mitigations:

- 只实现 M0 运行所需表，领域数据模型在 M1 单独审核。
- 使用 sqlc 生成查询类型和小型测试工厂，不创建通用 ORM 抽象层。

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

- OpenAPI、JSON Schema、事件 envelope 和错误代码在 `api/` 中版本化。
- M0 使用 OpenAPI 3.0.3；稳定工具链完整支持 3.1 后，以独立兼容性决策升级。
- OpenAPI 是规范来源，生成的 Go transport 类型与 TypeScript 客户端必须和提交的规范工件一致。
- Go 类型使用 oapi-codegen，TypeScript 类型使用 openapi-typescript，breaking change 检测使用 oasdiff；版本进入锁文件和 Go tool directive。
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
- 生成器使用本地锁定版本和临时目录比较，不从远程 URL 读取规范。
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
| Go 或模型 SDK 新能力不同步 | Medium | provider adapter 保留原始 HTTP escape hatch，并以契约测试覆盖能力矩阵 |
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

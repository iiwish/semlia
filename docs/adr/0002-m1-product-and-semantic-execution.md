# TDR-0002: M1 Product And Semantic Execution Foundation

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 1.2.0 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-14 |
| 来源 | `docs/SSOT.md` v0.7.0 Confirmed |
| 产品合同 | `docs/specs/product-prototype/product-design.md` v0.4.2 Confirmed |
| 前端基线 | `docs/specs/m1-semantic-registry/frontend-baseline-audit.md` |
| 审核 | 2026-08-24 经创始人确认仓库优先接入、物理图谱与可选执行适配器边界 |
| 实现闸门 | M0 已于 2026-09-02 获创始人接受；M1 task 仍须通过 plan、work graph、checklist、analysis 与 Ready packet 闸门 |

## 1. 决策摘要

M1 交付企业语义资产平台的第一条生产纵向闭环：从真实仓库 Catalog、DDL/View SQL 和版本化 SQL/dbt 工件发现物理资产、血缘和语义证据，形成可搜索、可追溯、可版本化的物理图谱、语义候选、权威 Wiki 页面和有限本体关系图。M1 不提前实现 M2 的 AI 提案、JoinContract 发布和治理工作流，也不提前实现 M3 的 MCP 可信解析消费面。

生产 Web 保持 React 19、TypeScript 和 Vite，采用 Tailwind CSS 4、shadcn/ui 源码组件、Radix primitives、Lucide、TanStack Router、TanStack Query、TanStack Table、TanStack Virtual、React Hook Form、Zod 和 react-i18next。组件库负责可访问交互原语，Semlia 自有设计 token、信息架构和高密度工作台布局保持规范来源。

Cube Core 是可选的独立执行与验证适配器，不是 Semlia 控制面框架，也不链接进 Go 二进制。M1 的仓库接入、物理图谱、搜索和语义候选路径在未部署 Cube 时完整成立。客户启用 Cube 后，Semlia 通过窄而类型化的 HTTP adapter 使用公开模型与 API，并将诊断结果作为证据；查询执行能力在后续里程碑通过相同 adapter contract 接入。

## 2. Constitution Check

| SSOT 原则 | 满足方式 |
| --- | --- |
| P-001 语义是资产，不是 Prompt | M1 Web 和 API 围绕稳定资产 ID、revision、来源、owner 和 evidence 建模 |
| P-003 发布版本不可变 | M1 建立 revision 与基础 diff，不以 UI 临时状态代替版本事实 |
| P-005 Git-native | 语义内容使用 Git，PostgreSQL 只保存控制面索引和事务状态 |
| P-006 Headless first | OpenAPI 和生成类型仍是 Web 与控制面的公共边界 |
| P-007 开放执行生态 | 仓库与执行适配器位于独立边界，核心领域对象不引用 Cube 或仓库厂商私有类型 |
| P-008 默认安全 | 浏览器不直连仓库或执行运行时；凭据、security context 和业务事实行不进入前端 |
| P-009 可靠性优先 | 真实仓库接入、可选 adapter 合约测试、桌面视觉回归、WCAG 2.2 AA 和性能预算进入验收 |
| P-010 可归因、可解释、保护隐私 | M1 只从真实服务端读取和搜索操作生成最小信号，状态事实可重建，不保存原始搜索文本或客户事实行 |

Constitution violations: None.

## 3. M1 范围边界

M1 includes:

- 仓库只读元数据配置、连接检查、Catalog/DDL/View SQL 发现、增量扫描和可诊断的导入结果。
- 版本化 SQL/dbt 工件导入，以及 PhysicalDataset、PhysicalField、CodeArtifact、LineageEdge、键、粒度和 JoinObservation。
- 物理与语义资产目录、万表搜索、筛选、详情、owner、source、evidence、revision 和 audit trace。
- 有界的一至三跳上游/下游关系视图，以及图形关系的等价文本摘要。
- Git 内容存储、PostgreSQL 索引和真实仓库与版本化转换工件的集成验收。
- 可选 Cube 模型来源的独立 adapter 和 contract test，不作为 M1 核心验收前置条件。
- 服务端生成的 `catalog.asset.read` 与 `catalog.search.completed` 信号，以及 definition、owner、evidence、source health 和 provenance 状态事实。

M1 excludes:

- AI 自动生成 proposal、结构化 patch、验证编排和 release policy；这些属于 M2。
- 发布、rollback、consumer binding、MCP 和 Agent 查询消费；这些属于 M2/M3。
- BI dashboard、Workbook、NL2SQL、聊天主界面和 Cube 运维界面。
- 大量浅层连接器、通用数据目录、移动端和触控专用体验。
- 语义 resolution、consumer/release/binding 归因、综合质量评分、Attention Item、通用事件 ingest 和跨租户学习。

## 4. 前端技术决策

### TDR-009 Tailwind CSS 4、shadcn/ui 与 Radix primitives

Decision:

- Tailwind CSS 4 提供 token 映射、布局和状态 utilities；CSS variables 是颜色、间距、圆角、层级和 motion token 的规范来源。
- shadcn/ui 作为按需引入的源码组件集合，生成代码归属 `web/src/components/ui/**`，不作为黑盒组件依赖。
- Dialog、Popover、Tooltip、Dropdown Menu、Tabs、Select、Checkbox 和可组合表单控件优先使用 Radix primitives。
- 只引入当前交付需要的组件；禁止用 arbitrary value 堆叠复制设计稿，禁止形成通用 shadcn 卡片网格外观。
- 业务组件位于明确 feature 边界，不能把领域语义写入通用 `ui` 组件。

Rationale:

- 可访问的焦点管理、portal、dismiss、keyboard navigation 和 ARIA 交互不应由项目重复实现。
- 源码所有权允许 Semlia 保持安静、高密度、桌面优先的产品语言，而不受主题框架限制。

### TDR-010 Router、server state 与 API 合约

Decision:

- TanStack Router 管理路由、search params、可分享筛选条件和详情选中状态。
- `openapi-react-query` 与 TanStack Query 管理 API 请求、缓存、失效、重试和异步状态，并复用现有 `openapi-fetch` 与生成 TypeScript 类型。
- 公共 API DTO 只由 OpenAPI 生成；Zod 只验证 UI 输入、URL 参数和非 API 外部数据，禁止复制服务端 DTO。
- React local state、Router state、Query cache 和 form state 是默认状态层。没有已测量的跨域客户端状态需求时，不引入 Redux 或 Zustand。

Rationale:

- 资产目录需要可恢复、可分享的路由状态，也需要一致的 loading、error、empty 和 stale-data 语义。
- 生成契约和单一 server-state cache 能减少原型式 view model 漂移。

### TDR-011 高密度数据工作台 primitives

Decision:

- TanStack Table 处理资产表格的列、排序、筛选和选择状态。
- TanStack Virtual 只用于真实数据量造成可测 DOM 或滚动成本的长列表和表格。
- React Hook Form 管理表单生命周期，Zod 管理 UI 输入校验。
- `cmdk` 用于全局命令与搜索入口，Sonner 用于非阻塞反馈，react-i18next 提供中文和英文产品 chrome。
- Lucide 是产品图标来源；已有标准图标时不绘制自定义 SVG 图标。

### TDR-012 关系图的有界采用

Decision:

- M1 关系图默认采用 `@xyflow/react`，仅渲染服务端限制的一至三跳子图，并始终提供列表或关系摘要作为等价操作面。
- 图谱模块按路由 lazy load，不进入首次工作台 shell bundle。
- 任务开始前使用真实仓库与转换工件样本执行节点数、边数、布局和交互基准。只有证据证明可见图规模超过 React Flow 的目标范围时，才单独评估 Cytoscape.js 或 WebGL 方案。
- 不同时安装 React Flow 与 Cytoscape.js，不在浏览器中加载完整企业关系图。

### TDR-013 前端质量与测试基线

Decision:

- 产品只验收 `1440x900` 普通桌面和 `1024x768` 紧凑桌面；不新增移动端导航、截图或 acceptance test。
- Vitest 和 Testing Library 覆盖组件行为；Playwright 覆盖核心旅程、键盘操作、Chromium/Firefox/WebKit 桌面浏览器和稳定截图。
- `@axe-core/playwright` 扫描关键页面与 dialog，serious 和 critical violation 必须为零；整体目标遵循 WCAG 2.2 AA。
- MSW 只用于测试中的网络边界，不成为生产 fallback 或 mock 数据入口。
- 初始工作台 shell 的 JavaScript gzip 预算为 150 KiB，CSS gzip 预算为 25 KiB；图谱和后续编辑器必须 lazy load。超过预算或相对基线增长 10% 需要记录证据并审核。
- UI 在标准宽带和推荐部署规格下首次可交互时间不高于 2.5 s；测试保留 mock-data disclosure、visible focus 和 reduced-motion 断言。

Deferred:

- Storybook 在共享组件 inventory 足以产生独立维护收益时引入。
- CodeMirror 在 M2 的 YAML 或结构化编辑器进入确认范围后引入。
- `motion` 只在存在可中断、连续性要求高的复杂交互时引入；普通 hover、tab 和 dialog 使用 CSS 与 Radix 状态。
- Next.js、MUI、Ant Design 和第二套通用组件系统不进入 M1。

## 5. 来源与执行适配器技术决策

### TDR-014 仓库优先接入与可选执行 adapter

Decision:

- M1 的规范来源路径由 `CatalogAdapter`、`TransformationAdapter` 和可选 `LineageAdapter` 组成，读取仓库 Catalog、DDL/View SQL、数据库约束、版本化 SQL/dbt 工件和已有血缘。
- Adapter DTO 在 integration 边界终止，application service 将其映射为 `SourceRevision`、`PhysicalDataset`、`PhysicalField`、`CodeArtifact`、`LineageEdge` 和 `JoinObservation`。
- 来源 adapter 必须返回稳定 source identity、revision、capability、diagnostic 和增量 fingerprint；不把仓库厂商私有类型写入领域模型。
- Cube Core 通过独立 `CubeAdapter` 作为可选语义模型来源和执行运行时接入，不链接进 Go 二进制，不成为 M1 核心路径或本地启动前置条件。
- M1 的 Cube capability 仅包含显式进入任务范围的 health、metadata 和 diagnostic；M2/M3 的 compile、validate 和 query 能力通过同一 adapter contract 按需扩展。
- 浏览器只调用 Semlia API。仓库、Cube 和其他运行时的凭据、安全上下文与内部 endpoint 不暴露给 Web。
- 本地与 CI 以真实仓库 fixture 验证核心路径；Cube 使用独立可选 profile、固定版本与 digest 验证，不依赖 Cube 的测试必须可独立通过。

Integration contract:

1. Semlia 对来源配置和 revision 生成稳定 discovery fingerprint。
2. Adapter 返回能力声明、规范化 metadata、版本归因、可诊断错误和不包含事实明细的证据。
3. Import service 幂等写入物理对象、血缘、知识块、证据和语义候选，并保留原始 source identity 映射。
4. Git 保存规范语义内容；PostgreSQL 保存物理图谱、索引、run 状态、diagnostic 和 audit event；大型证据进入对象存储。
5. 集成测试使用 repository-local fixture、固定 schema revision 和可重复断言；可选外部服务禁止使用 `latest`。

Rationale:

- 企业的普遍冷启动输入是仓库元数据、SQL/dbt 工件和杂乱文档，而不是既有 Cube 项目。
- 物理图谱是万表检索、物理绑定和 JoinContract 的必要基础，必须在执行引擎选择之前成立。
- 独立 adapter 保护 Go 模块化单体、Apache-2 开源边界和不同仓库与执行运行时的接入空间。

Alternatives considered:

- 只从 Cube 导入：无法服务尚未采用 Cube、但拥有大量表和 SQL 的企业。
- 把 Cube Core 链接进 Go 二进制：运行时和语言边界不匹配，升级与供应链耦合过强。
- 在 Go 中重写查询编译器：成本高且会让 Semlia 偏离语义控制面定位。
- 浏览器直连仓库或执行运行时：泄露凭据和授权边界，并绕过 Semlia 审计与策略。

Risks and mitigations:

| 风险 | 控制 |
| --- | --- |
| 仓库方言和 metadata 差异 | capability contract、规范化 DTO、方言 fixture 和 conformance tests |
| SQL 无法完整静态解析 | 保留原始证据与 unresolved 状态，确定性解析优先，禁止 LLM 推断静默升级为事实 |
| 厂商或 Cube 对象污染领域模型 | adapter DTO 在 integration 边界终止，application 层显式映射 |
| 大型项目扫描超时 | fingerprint 增量发现、后台 job、有界重试、可恢复 run 和分阶段 metadata |
| 测试只验证 mock | 核心路径使用真实数据库 fixture；可选运行时使用固定版本独立 profile |

## 6. 后端依赖原则

### TDR-015 分里程碑引入窄依赖

基础控制面继续使用 Go `net/http`、pgx v5、sqlc、golang-migrate、PostgreSQL job/outbox 和 OpenTelemetry。依赖只在拥有已确认 requirement 和验证任务时进入 `go.mod`，不为路线图占位安装。

| 能力 | 选择 | 最早里程碑 | 采用条件 |
| --- | --- | --- | --- |
| Catalog integration | 标准库 `database/sql` 或驱动提供的只读 metadata API + 项目内 typed adapter | M1 | Catalog、DDL、约束、增量 fingerprint 与真实数据库 fixture |
| Transformation integration | 结构化 manifest/catalog parser 与按方言隔离的 SQL parser adapter | M1 | 版本化 dbt/SQL fixture、字段血缘和 unresolved 退化路径 |
| Cube integration | 标准库 HTTP client + 项目内窄 typed adapter | M1 optional | health、metadata、diagnostic 与独立 Cube contract test |
| Git content adapter | system Git 与 `go-git/v5` 二选一 | M1 spike | 比较 shallow clone、credentials、worktree、diff、跨平台和二进制体积后确认 |
| S3/MinIO evidence | `github.com/aws/aws-sdk-go-v2` | 首个对象存储 task | evidence 大小与生命周期证明本地文件系统不足 |
| MCP server/client | `github.com/modelcontextprotocol/go-sdk` | M3 | MCP consumption requirement Confirmed |
| OIDC | `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2` | Auth milestone | 多用户认证和 provider contract Confirmed |
| JSON Schema validation | `github.com/santhosh-tekuri/jsonschema/v6` | M2 | 校验 AI structured output 或版本化 schema |
| Release policy | `github.com/google/cel-go` | M2 | policy language 与审计语义 Confirmed |
| JWK/JWT | `github.com/lestrrat-go/jwx/v3` | Security context task | 内部 token、JWK rotation 或执行适配器 security context 进入范围 |
| OTLP metrics/export | `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` 等官方 exporter | Observability task | collector endpoint 与生产 telemetry contract Confirmed |

Deferred systems:

- 不引入 Gin、Fiber、通用 ORM、Redis、Kafka、Temporal、Elasticsearch、独立向量数据库或微服务拆分。
- PostgreSQL full-text/trigram 能满足前期资产搜索；只有查询质量和规模基准证明不足后才评估专用搜索系统。
- pgvector 只在已确认检索场景、评测集和数据治理边界存在后采用。
- 所有直接依赖必须与 Apache License 2.0 项目分发兼容，固定版本，进入 SBOM、漏洞扫描和许可证检查。

### TDR-016 飞轮事实流与 M1 最小信号

Decision:

- `audit_events` 保存写入、权限、审核、发布和 Agent 工具调用的不可抵赖记录；`outbox_events` 只负责领域事件可靠投递；`usage_events` 保存产品使用与反馈信号；OpenTelemetry 保存延迟、错误和队列等运行数据。四类事实不能互相替代。
- M1 只实现 `catalog.asset.read` 和 `catalog.search.completed` 两个 UsageEvent。前者在服务端真实返回 asset revision 后生成；后者使用 `matched | zero_result | failed` outcome，并记录结果数量区间。
- M1 不提供任意 `POST /usage-events`。UsageEvent 由可信 application service 产生，使用 `(workspace_id, event_type, idempotency_key)` 去重，并保留 `data_version`、归因 ID、channel、outcome、reason code、trace 和发生/接收时间。
- 搜索信号默认只保存 workspace-scoped HMAC 指纹、语言、token/count bucket、筛选维度与结果区间，不保存原始搜索文本。产品内租户信号不离开 self-hosted 部署；Semlia 项目遥测是独立且默认关闭的能力。
- Discovery run、import、revision 创建属于领域或审计事实；definition、owner、evidence、source health 和 provenance 属于规范状态。它们不伪装成 UsageEvent，也不在 M1 组合成黑盒质量分。
- M1 使用普通 PostgreSQL `usage_events` 表、索引和可验证删除路径。只有 M3 对外消费量与 retention 基准证明需要时才引入时间分区和日聚合；不引入 Kafka、event sourcing、流计算或训练平台。

Rationale:

- 飞轮必须从真实产品行为开始，不能为尚不存在的 release、consumer 或 semantic resolution 伪造归因。
- 分离审计、投递、产品信号和运维 telemetry 可保持合规语义、retention 和指标口径稳定。
- 少量严格 schema 的服务端事件比 UI clickstream 或通用 ingest 更容易测试、解释和长期演进。

## 7. 任务与验收影响

M1 计划至少包含以下工作流：

1. M1 领域 schema、OpenAPI、错误模型和资产 revision 契约。
2. Catalog、DDL/View SQL 和 Transformation adapter spike，真实数据库 fixture、增量 discovery 与 unresolved 设计。
3. 物理图谱 schema、Git content adapter spike 与 PostgreSQL 物理/语义 asset index。
4. UsageEvent taxonomy、隐私/retention contract、M1 两个服务端 producer 和删除测试。
5. 生产 Web shell、design tokens、Radix/shadcn primitives、router/query 和 i18n 基础。
6. Source setup、discovery run、资产 catalog/detail、empty/error/running state。
7. 有界关系图、文本 fallback、性能和无障碍验证。
8. 真实仓库与版本化 SQL/dbt fixture 的端到端导入、10,000 表检索基准、fresh-workspace 验收、信号归因和交付证据。
9. 可选 Cube adapter 的独立 contract test，证明启用与未启用两种路径均不污染核心领域模型。

`web/**` 是唯一可运行并由 Go 服务嵌入的前端。已确认的产品体验直接在该 package
维护；M1 Catalog 使用生成 API 类型和真实 PostgreSQL 数据。尚未进入对应后端里程碑的
流程必须标明预览或会话边界，且不得在生产请求失败时回退到 fixture。仓库不维护并行原型
package。

## 8. 支持材料

- [shadcn/ui Tailwind v4](https://ui.shadcn.com/docs/tailwind-v4)
- [openapi-react-query](https://openapi-ts.dev/openapi-react-query/)
- [TanStack Router type safety](https://tanstack.com/router/latest/docs/guide/type-safety)
- [TanStack Table virtualization](https://tanstack.com/table/latest/docs/guide/virtualization)
- [React Flow](https://reactflow.dev/)
- [Playwright accessibility testing](https://playwright.dev/docs/accessibility-testing)
- [Cube Core repository](https://github.com/cube-js/cube)
- [Cube production architecture](https://docs.cube.dev/cube-core/architecture)
- [Cube Core deployment](https://docs.cube.dev/admin/deployment/core)
- [Cube data model syntax](https://docs.cube.dev/docs/data-modeling/concepts/syntax)
- [Cube Data APIs](https://docs.cube.dev/reference/core-data-apis)
- [Official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- [AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2)

## 9. 实现闸门

本 TDR 确认技术方向。Founder 于 2026-09-02 明确接受 T008 与 M0，M1 规划闸门已经打开。实现仍须先编写并由用户确认 M1 plan、work graph、checklist 与 analysis，再为满足依赖的单一任务生成 Ready execution packet。

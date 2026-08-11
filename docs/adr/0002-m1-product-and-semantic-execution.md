# TDR-0002: M1 Product And Semantic Execution Foundation

## 元数据

| 字段 | 值 |
| --- | --- |
| 版本 | 1.0.0 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-10 |
| 来源 | `docs/SSOT.md` v0.2.0 Confirmed |
| 产品合同 | `docs/specs/product-prototype/product-design.md` v0.4.1 Confirmed |
| 前端基线 | `docs/specs/m1-semantic-registry/frontend-baseline-audit.md` |
| 审核 | 2026-08-10 经创始人确认前端依赖、Cube Core 边界与后端依赖原则 |
| 实现闸门 | M0 通过 fresh-clone 验收并由创始人接受后，才创建 M1 Ready task |

## 1. 决策摘要

M1 交付 Semantic Registry 的第一条生产纵向闭环：从真实 Cube 项目发现和导入语义模型，形成可搜索、可追溯、可版本化的资产目录、详情和有限关系图。M1 不提前实现 M2 的 AI 提案、验证和发布工作流，也不提前实现 M3 的 MCP 消费面。

生产 Web 保持 React 19、TypeScript 和 Vite，采用 Tailwind CSS 4、shadcn/ui 源码组件、Radix primitives、Lucide、TanStack Router、TanStack Query、TanStack Table、TanStack Virtual、React Hook Form、Zod 和 react-i18next。组件库负责可访问交互原语，Semlia 自有设计 token、信息架构和高密度工作台布局保持规范来源。

Cube Core 是独立部署的首选可执行语义内核，不是 Semlia 控制面框架，也不链接进 Go 二进制。Semlia 通过窄而类型化的 HTTP adapter 使用 Cube 公开模型与 Data API；Cube 负责模型编译、查询规划与执行、执行期访问控制和预聚合，Semlia 负责资产身份、来源证据、revision、审计以及后续里程碑中的提案、审核、release 和 binding。

## 2. Constitution Check

| SSOT 原则 | 满足方式 |
| --- | --- |
| P-001 语义是资产，不是 Prompt | M1 Web 和 API 围绕稳定资产 ID、revision、来源、owner 和 evidence 建模 |
| P-003 发布版本不可变 | M1 建立 revision 与基础 diff，不以 UI 临时状态代替版本事实 |
| P-005 Git-native | 语义内容使用 Git，PostgreSQL 只保存控制面索引和事务状态 |
| P-006 Headless first | OpenAPI 和生成类型仍是 Web 与控制面的公共边界 |
| P-007 开放执行生态 | Cube 位于独立 adapter 边界，核心领域对象不引用 Cube 私有类型 |
| P-008 默认安全 | 浏览器不直连 Cube；凭据、security context 和业务事实行不进入前端 |
| P-009 可靠性优先 | 真实 Cube 合约测试、桌面视觉回归、WCAG 2.2 AA 和性能预算进入验收 |

Constitution violations: None.

## 3. M1 范围边界

M1 includes:

- Cube workspace 配置、连接检查、模型发现、增量扫描和可诊断的导入结果。
- 语义资产目录、搜索、筛选、详情、owner、source、evidence、revision 和 audit trace。
- 有界的一至三跳上游/下游关系视图，以及图形关系的等价文本摘要。
- Git 内容存储、PostgreSQL 索引和真实 Cube 项目的集成验收。

M1 excludes:

- AI 自动生成 proposal、结构化 patch、验证编排和 release policy；这些属于 M2。
- 发布、rollback、consumer binding、MCP 和 Agent 查询消费；这些属于 M2/M3。
- BI dashboard、Workbook、NL2SQL、聊天主界面和 Cube 运维界面。
- dbt 正式适配器、通用数据目录、移动端和触控专用体验。

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
- 任务开始前使用真实 Cube 样本执行节点数、边数、布局和交互基准。只有证据证明可见图规模超过 React Flow 的目标范围时，才单独评估 Cytoscape.js 或 WebGL 方案。
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

## 5. Cube Core 技术决策

### TDR-014 Cube Core 作为外部执行 adapter

Decision:

- Cube Core 作为独立服务运行，可由固定版本与 digest 的容器、本地受控进程或兼容托管服务提供。
- Semlia Go 控制面通过 `CubeAdapter` 应用边界调用 Cube 公开 REST/Meta API，不导入 Cube 私有包，也不复制 Cube compiler。
- M1 adapter 覆盖 capability/health、metadata discovery、模型诊断和导入所需的最小 API；查询执行能力留在同一 adapter 边界，但不是 M1 产品 UI 的必需表面。
- M2 的编译验证和 release gate、M3 的受约束查询消费复用该 adapter，不改变核心资产模型。
- Adapter capability 按里程碑扩展：M1 实现 health/meta/diagnostic，M2 实现 compile/validate，M3 实现携带受约束 security context 的 query；不预建未进入范围的方法。
- Semlia 输出到 Cube 的内容使用版本化、确定性序列化的 Cube YAML 或兼容模型工件。M1 importer 保留 source identity 与 canonical mapping，使后续 validation 和 release 可以复现同一模型。
- 本地与 CI 使用可选 Cube Compose profile 运行真实集成套件；不依赖 Cube 的单元和领域测试保持可独立运行。
- 浏览器首先只调用 Semlia API。Cube 凭据、安全上下文和内部 endpoint 不暴露给 Web。

Integration contract:

1. Semlia 读取经配置的 Cube 项目或版本化模型工件，生成稳定的 discovery input fingerprint。
2. Adapter 调用 Cube health/metadata/diagnostic 能力，返回 Semlia 自有的类型化结果和稳定错误码。
3. Import application service 把 Cube 名称和结构映射为引擎无关资产、关系、source evidence 和 revision。
4. Git 保存规范内容，PostgreSQL 保存索引、run 状态、diagnostic 和 audit event。
5. 集成测试使用固定 Cube image tag 与 digest、repository-local fixture 和可重复断言，不使用 `latest`。

Rationale:

- Cube 已提供成熟的编译、查询、访问控制和预聚合能力，重新实现会扩大风险并偏离 Semlia 的控制平台定位。
- 独立 adapter 保护 Go 模块化单体、Apache-2 开源边界和未来其他语义执行引擎的接入空间。

Alternatives considered:

- 把 Cube Core 链接进 Go 二进制：运行时和语言边界不匹配，升级与供应链耦合过强。
- 在 Go 中重写 Cube compiler：成本高且会产生长期兼容性分叉。
- 浏览器直连 Cube：短期少一跳，但泄露凭据和授权边界，并让 Web 绕过 Semlia 审计。
- 不采用执行内核：无法验证发布语义是否可执行，也会迫使 Semlia 自建查询能力。

Risks and mitigations:

| 风险 | 控制 |
| --- | --- |
| Cube API 或模型语法升级 | 固定版本与 digest，adapter contract tests，升级作为独立依赖 PR |
| Cube 对象直接污染领域模型 | adapter DTO 在 integration 边界终止，application 层显式映射 |
| 大型项目扫描超时 | fingerprint 增量发现、后台 job、有界重试、可恢复 run 和分阶段 metadata |
| 错误信息不可行动 | 稳定 Semlia error code 保留安全脱敏的 Cube diagnostic 与 trace ID |
| 测试只验证 mock | CI integration profile 启动真实固定版本 Cube Core 并导入 fixture |

## 6. 后端依赖原则

### TDR-015 分里程碑引入窄依赖

基础控制面继续使用 Go `net/http`、pgx v5、sqlc、golang-migrate、PostgreSQL job/outbox 和 OpenTelemetry。依赖只在拥有已确认 requirement 和验证任务时进入 `go.mod`，不为路线图占位安装。

| 能力 | 选择 | 最早里程碑 | 采用条件 |
| --- | --- | --- | --- |
| Cube integration | 标准库 HTTP client + 项目内窄 typed adapter | M1 | health、metadata、diagnostic 与真实 Cube contract test |
| Git content adapter | system Git 与 `go-git/v5` 二选一 | M1 spike | 比较 shallow clone、credentials、worktree、diff、跨平台和二进制体积后确认 |
| S3/MinIO evidence | `github.com/aws/aws-sdk-go-v2` | 首个对象存储 task | evidence 大小与生命周期证明本地文件系统不足 |
| MCP server/client | `github.com/modelcontextprotocol/go-sdk` | M3 | MCP consumption requirement Confirmed |
| OIDC | `github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2` | Auth milestone | 多用户认证和 provider contract Confirmed |
| JSON Schema validation | `github.com/santhosh-tekuri/jsonschema/v6` | M2 | 校验 AI structured output 或版本化 schema |
| Release policy | `github.com/google/cel-go` | M2 | policy language 与审计语义 Confirmed |
| JWK/JWT | `github.com/lestrrat-go/jwx/v3` | Security context task | 内部 token、JWK rotation 或 Cube security context 进入范围 |
| OTLP metrics/export | `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` 等官方 exporter | Observability task | collector endpoint 与生产 telemetry contract Confirmed |

Deferred systems:

- 不引入 Gin、Fiber、通用 ORM、Redis、Kafka、Temporal、Elasticsearch、独立向量数据库或微服务拆分。
- PostgreSQL full-text/trigram 能满足前期资产搜索；只有查询质量和规模基准证明不足后才评估专用搜索系统。
- pgvector 只在已确认检索场景、评测集和数据治理边界存在后采用。
- 所有直接依赖必须与 Apache License 2.0 项目分发兼容，固定版本，进入 SBOM、漏洞扫描和许可证检查。

## 7. 任务与验收影响

M1 计划必须至少包含以下工作流，但在 M0 验收完成前保持规划态：

1. M1 领域 schema、OpenAPI、错误模型和资产 revision 契约。
2. Cube adapter spike、固定容器 contract test 和增量 discovery 设计。
3. Git content adapter spike 与 PostgreSQL asset index。
4. 生产 Web shell、design tokens、Radix/shadcn primitives、router/query 和 i18n 基础。
5. Source setup、discovery run、资产 catalog/detail、empty/error/running state。
6. 有界关系图、文本 fallback、性能和无障碍验证。
7. 真实 Cube fixture 的端到端导入、fresh-workspace 验收和交付证据。

M1 task 不得直接复制 `prototypes/product/**` 到 `web/**`。原型只提供产品合同和交互意图；生产实现必须按 feature 边界重建、使用生成 API 类型，并解决前端基线审计中的阻断问题。

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

本 TDR 确认技术方向，不代表 M1 实现已经获准启动。`docs/specs/m0-foundation/tasks.md` 中 T007 仍需真实 GitHub Actions 运行证据，T008 仍需 fresh-clone acceptance；M0 达到退出标准并由创始人明确接受后，才编写并审核 M1 plan、work graph、checklist、analysis 和 Ready execution packets。

# 语义生产重构任务图

| 元数据 | 值 |
| --- | --- |
| Version | 0.1.0 |
| Status | Confirmed |
| Product contract | [product-design.md](product-design.md) 0.1.0 Confirmed |
| Plan | [plan.md](plan.md) 0.1.0 Confirmed |
| Last updated | 2026-09-09 |
| Approval | 2026-09-11 T005 按条件验收授权完成复核；用户授权 T006 及测试启动器、回滚投影和目录字段标识兼容的受限扩围 |

## 1. 工作图

| Epic | Story | 任务 | 依赖 | 用户可验证结果 |
| --- | --- | --- | --- | --- |
| SP-E01 可复核的生产输入 | SP-US-001、SP-US-004 | SP-T001 基线与详细契约 | 计划及任务图确认 | 确切基线、数据模型、接口与迁移边界 |
| SP-E01 可复核的生产输入 | SP-US-001、SP-US-004 | SP-T002 来源版本集合 | SP-T001 Accepted | 扫描和证据的历史成员、名称、覆盖可核对 |
| SP-E02 受治理的语义生产 | SP-US-001、SP-US-002、SP-US-004 | SP-T003 新建生产与恢复 | SP-T002 技术评审通过；本批用户授权 | 候选生成新资产草稿与关联提案，支持可靠恢复 |
| SP-E02 受治理的语义生产 | SP-US-003、SP-US-004 | SP-T004 治理发布与回滚 | SP-T003 Accepted | 相关对象通过同一验证、审核和原子发布 |
| SP-E02 受治理的语义生产 | SP-US-002 | SP-T005 内部 AI 生产 | SP-T004 Accepted | 来源输入生成受约束建议，进入相同治理路径 |
| SP-E03 可用与可维护 | SP-US-001 至 SP-US-004 | SP-T006 桌面生产体验 | SP-T005 Accepted | 连续操作、证据核对、失败及重开恢复 |
| SP-E03 可用与可维护 | SP-US-001 至 SP-US-004 | SP-T007 完整验收 | SP-T006 Accepted | 三组样例、迁移与接口兼容、实际模型和桌面证据 |

全部串行，优先级表示本生产计划内的顺序。用户明确授权的 T002、T003 批次可在 T002 通过独立技术评审后继续 T003，不把尚未交付的任务标为 Accepted；其他任务仍保留逐项用户验收闸门。外部 EAI 任务不属于依赖或验收前提，不把全部任务同时标为 Ready。

| 验收 | 主要责任任务 | 最终复验 |
| --- | --- | --- |
| SP-AC-001 | SP-T002、SP-T003、SP-T006 | SP-T007 |
| SP-AC-002 | SP-T002 | SP-T007 |
| SP-AC-003 | SP-T003、SP-T006 | SP-T007 |
| SP-AC-004 | SP-T005、SP-T006 | SP-T007 |
| SP-AC-005 | SP-T003、SP-T004、SP-T006 | SP-T007 |
| SP-AC-006 | SP-T004、SP-T005 | SP-T007 |
| SP-AC-007 | SP-T004、SP-T006 | SP-T007 |
| SP-AC-008 | SP-T002、SP-T003、SP-T004、SP-T006 | SP-T007 |
| SP-AC-009 | SP-T003、SP-T006 | SP-T007 |
| SP-AC-010 | SP-T002、SP-T003、SP-T004、SP-T005 | SP-T007 |
| SP-AC-011 | SP-T006 | SP-T007 |
| SP-AC-012 | SP-T004、SP-T006 | SP-T007 |

## 2. 公共执行约束

- 全部执行包展开本文件的共享文件组，不依赖聊天上下文。允许范围是上限，不要求修改其中每个文件。
- **G-CONTRACT：** `api/openapi/semlia.v1.yaml`、`api/gen/go/types.gen.go`、`sdk/typescript/src/schema.gen.ts`、`sdk/typescript/src/client.ts`、`sdk/typescript/src/client.test.ts`、`sdk/typescript/src/index.ts`、`api/README.md`、`tests/contracts/contract_test.go`。仅用于该任务新增或修改的生产契约，不扩展外部机器权限。
- **G-SQLC：** `internal/adapters/postgres/sqlc/*.go`，glob 仅允许 sqlc 根据该任务 SQL/schema 自动生成的变化，不手工修改生成代码。
- **G-WEB：** `internal/platform/web/static/**`，仅允许既有构建产生的嵌入前端生成物，不手改 hash 文件。
- 每项允许修改自身 `docs/evidence/SP-T00N/` 中的真实证据、当前任务状态块和自身执行包；不得修改其他任务验收或历史证据。
- 迁移编号根据当前尾号 000021 预留。执行时如编号已被其他工作占用，先修正精确文件清单，不覆盖或重命名已有迁移。
- 共享 OpenAPI、生成物和治理存储构成冲突；任务串行执行。用户授权 PM 委派当前任务的受限执行和独立评审，不授权并行 governed task、模型付费调用、部署、提交或推送 Git。
- SP-T001 验收需包含详细契约批准。SP-T002 至 SP-T006 如需要改变该契约的产品含义或安全边界，暂停并重新审核，不以“实现细节”为由扩大范围。
- TDD 默认先证实失败，再最小实现，再受限重构。审批状态、缺少工具、测试跳过和预算不足不能记为通过。
- 每项必须先做规格符合性检查，再做工程风险检查；独立评审与用户 acceptance 在交付时明确记录，不能把自检伪装成独立评审。

## 3. SP-T001：生产基线与详细契约

Status: Accepted
Priority: P0
Depends on: 本计划与任务图 Confirmed；执行包和工作区预检通过
Blocks: SP-T002 至 SP-T007
Story / Requirement: SP-US-001、SP-US-004；SP-FR-001 至 SP-FR-010、SP-NFR-001 至 SP-NFR-005 的基线与契约；SP-TD-001 至 SP-TD-008
Parallel: No
Conflicts with: 所有改动当前工作区、生产契约、迁移或测试基础设施的任务
Estimate: 实测约 1.3 有效小时（约 75 分钟，含 PM、执行、测试、独立评审与返修；精确时间见交付证据）

目标：
确认当前代码哪些能力可复用、哪些用例未覆盖，固定后续任务需要的数据与接口决策；不把阅读代码当作测试通过。

允许修改范围：
- `docs/specs/semantic-production/data-model.md`
- `docs/specs/semantic-production/contracts/production.openapi.yaml`
- `docs/specs/semantic-production/contracts/production.md`
- `docs/specs/semantic-production/analysis.md` 与 `checklists/requirements.md` 的契约核对结果
- `tests/contracts/semantic_production_contract_test.go`
- 本任务证据与执行包；工期校准只更新 `plan.md` 第 6 节和各任务 Estimate，不自行变更行为范围

Test targets：
既有 ingestion/discovery/governance/projection 集成测试；新静态契约测试 `TestSemanticProductionContract`；真实工作区与数据库隔离说明。

交付内容：
基线与逐项 SP-AC 覆盖矩阵；来源快照模型；create/update 与无已发布基线约束；生产集合、幂等恢复、冻结输入、授权和审计契约；多提案发布归因、依赖闭合与回滚缺席语义；新旧 API/数据库兼容及有损降级拒绝规则；后续隔离验收环境方案。

验收标准：
实际运行既有测试并标注通过、失败、跳过与缺覆盖；静态 schema 校验只证明文档，不证明 API 实现。详细契约满足 SP-TD-001 至 SP-TD-008，无未处理高风险设计问题，获得用户审核后才开放实现。

Definition of Done：
记录 exact checkout、未提交/未跟踪依赖和命令结果；现有测试失败不能由本任务修改应用或放松断言掩盖。若暴露基础故障，先提出受限修复和工期影响，后续实现保持阻断。详细契约、兼容矩阵和验收环境可供执行者独立使用。

验证命令：
```sh
go test -count=1 ./internal/application/discovery ./internal/application/ingestion ./internal/application/governance ./internal/platform/http
go test -count=1 -timeout=20m ./tests/integration/ingestion ./tests/integration/discovery ./tests/integration/governance ./tests/integration/projection
go test -count=1 ./tests/contracts -run '^TestSemanticProductionContract$'
make test-contracts
make test-repository
```

TDD plan：基线部分只运行现有测试，不修改其断言。新静态契约测试先对缺失/错误契约 RED，再完成文档使 GREEN；REFACTOR 只消除校验辅助代码重复。无应用实现。

Packet path: `docs/specs/semantic-production/packets/SP-T001.yaml`
Evidence required: `baseline.md`、覆盖矩阵、契约测试 RED/GREEN、详细决策、环境所有权、真实结果、校准估算及用户审核状态。

交付证据：[SP-T001 报告](../../evidence/SP-T001/summary.md)。A001 与 A002 均完成真实 RED/GREEN；独立复核四项契约缺口均 Fixed，无新增阻断。2026-09-09 用户明确接受详细契约及本项交付，并授权继续 T002、T003。

## 4. SP-T002：来源版本与生产输入集合

Status: Accepted
Priority: P0
Depends on: SP-T001 Accepted，详细数据/接口契约已确认
Blocks: SP-T003 至 SP-T007
Story / Requirement: SP-US-001、SP-US-004；SP-FR-001、SP-FR-002、SP-FR-008；SP-NFR-001、SP-NFR-002、SP-NFR-003、SP-NFR-005；SP-TD-002、SP-TD-008
Parallel: No
Conflicts with: 所有 discovery/ingestion、来源 schema、目录读取和生成物改动
Estimate: 1.5–3 有效小时

目标：
让生产与历史证据使用确定的来源成员、名称和 revision，不依赖最新指针拼装历史。

允许修改范围：
- `migrations/000022_semantic_production_sources.up.sql`、`migrations/000022_semantic_production_sources.down.sql`
- `db/queries/discovery.sql`、`db/queries/catalog.sql`、G-SQLC
- `internal/domain/discovery/model.go`、`internal/domain/discovery/source.go`、`internal/domain/discovery/snapshot.go`、`internal/domain/discovery/snapshot_test.go`
- `internal/application/discovery/service.go`、`internal/application/discovery/control.go`、`internal/application/discovery/snapshot.go`、`internal/application/discovery/snapshot_test.go`
- `internal/adapters/postgres/discovery.go`、`internal/adapters/postgres/source_discovery.go`、`internal/adapters/postgres/source_snapshot.go`、`internal/adapters/postgres/catalog.go`
- `internal/platform/http/discovery.go`、`internal/platform/http/handler.go`、`internal/platform/http/source_snapshot_test.go`、`cmd/semlia/main.go`、G-CONTRACT
- `internal/platform/http/catalog.go`：既有共享 routeKind、路径匹配和 allowedMethods 定义，只注册四个来源快照 GET 路由。
- `tests/integration/discovery/source_snapshot_test.go`、`tests/integration/db/semantic_production_migrations_test.go`
- `pkg/identity/id.go`、`pkg/identity/id_test.go`、`sdk/typescript/src/ids.ts`、`sdk/typescript/src/ids.test.ts`：ssnp/codrev/linrev 类型标识。
- `db/sqlc.yaml`、`cmd/semlia/readiness.go`、`cmd/semlia/readiness_test.go`、`tests/integration/db/database_test.go`：追加 schema 注册、精确 readiness 与完整迁移清单断言，保留旧阶段兼容测试。
- `internal/adapters/postgres/artifacts.go`、`tests/integration/ingestion/source_snapshot_retention_test.go`：快照引用工件的保留与 cleanup fence。
- `internal/application/discovery/service_test.go`、`internal/application/discovery/control_test.go`、`internal/application/discovery/artifacts.go`、`internal/application/discovery/artifacts_test.go`、`internal/domain/discovery/model_test.go`：来源输入、覆盖与不可变字节在既有发现路径中的传递和回归。
- `catalog/adapter.go`、`catalog/adapter_test.go`、`files/adapter.go`、`files/adapter_test.go`、`dbt/adapter.go`、`dbt/adapter_test.go`、`postgresql/adapter.go`、`postgresql/adapter_test.go`、`postgreslive/collector.go`：路径均相对 `internal/adapters/discovery/`；仅传递现有来源的可证明覆盖与代码内容，不增加连接器类型。

Test targets：
SP-AC-001 的输入部分、SP-AC-002、SP-AC-008 的重放部分、SP-AC-010 的来源隔离；既有发现、制品和目录回归。

交付内容：
不可变成员/名称和覆盖投影、固定生产输入引用、当前版本与历史不可验证的明确区别、受授权读取以及增量迁移。现有连接器不扩展新的来源类型。

验收标准：
不变表复用 revision 仍在成员集合中；改名、删除、部分/失败扫描、并发扫描与重复扫描有一致结果。旧缺失信息不被回填成历史真值；历史读取按原 workspace 和来源授权，必要历史缺失阻断新生产。

Definition of Done：
SP-T001 模型与接口符合性通过；真实 PostgreSQL 升级和安全降级断言通过，旧发现和读取契约无回归；不更改已有 migration，不扩大机器路由。

验证命令：
```sh
go test -count=1 ./internal/domain/discovery ./internal/application/discovery ./internal/platform/http
go test -count=1 -timeout=20m ./tests/integration/discovery ./tests/integration/ingestion ./tests/integration/catalog
go test -count=1 -timeout=10m ./tests/integration/db -run '^TestSemanticProductionMigration'
make db-generate-check
make contracts-check
make test-contracts
```

TDD plan：RED 重放不变成员、历史改名、部分覆盖、跨工作区和迁移拒绝用例；GREEN 最小快照及投影；REFACTOR 仅整理受影响的发现持久化与读取辅助逻辑。

Packet path: `docs/specs/semantic-production/packets/SP-T002.yaml`
Evidence required: 输入与成员黄金集、RED/GREEN、迁移前后数据核对、并发/授权结果、差异、残余风险和评审结论。

交付证据：[SP-T002 报告](../../evidence/SP-T002/summary.md)、[2026-09-10 验收复核](../../evidence/SP-T002/acceptance-20260910/review.md)。来源快照专项、旧 F1/F2/F3、迁移与契约复核通过，保留 Accepted。全量回归存在既有调度测试的一次波动，失败及单例通过证据均保留，不宣称所有回归轮次全绿。

## 5. SP-T003：候选生产与服务端恢复

Status: Accepted
Priority: P0
Depends on: SP-T002 实现、验证及独立技术评审通过；2026-09-09 用户明确授权继续本批 T002、T003
Blocks: SP-T004 至 SP-T007
Story / Requirement: SP-US-001、SP-US-002、SP-US-004；SP-FR-003、SP-FR-005、SP-FR-008、SP-FR-009；SP-NFR-001、SP-NFR-002、SP-NFR-003、SP-NFR-005；SP-TD-001、SP-TD-003、SP-TD-004、SP-TD-008
Parallel: No
Conflicts with: 候选决策、资产新建、Proposal schema、生产命令与契约生成物改动
Estimate: 2–4 有效小时

目标：
通过一个服务端生产命令，把有版本证据的候选转成新资产或已有资产变更；返回可恢复的权威结果，不要求浏览器先造目标再关联提案。

允许修改范围：
- `migrations/000023_semantic_production_authoring.up.sql`、`migrations/000023_semantic_production_authoring.down.sql`
- `db/queries/discovery.sql`、`db/queries/registry.sql`、`db/queries/catalog.sql`、`db/queries/governance.sql`、`db/queries/semantic_production.sql`、G-SQLC
- `internal/domain/governance/proposal.go`、`internal/domain/governance/proposal_test.go`、`internal/domain/governance/production.go`、`internal/domain/governance/production_test.go`
- `internal/application/governance/production.go`、`internal/application/governance/production_test.go`、`internal/application/governance/authoring.go`、`internal/application/governance/proposal.go`
- `internal/application/catalog/service.go`、`internal/application/catalog/service_test.go`、`internal/application/discovery/control.go`
- `internal/adapters/postgres/semantic_production.go`、`internal/adapters/postgres/governance.go`、`internal/adapters/postgres/catalog.go`、`internal/adapters/postgres/source_discovery.go`
- `internal/platform/http/semantic_production.go`、`internal/platform/http/semantic_production_test.go`、`internal/platform/http/handler.go`、`cmd/semlia/main.go`、G-CONTRACT
- `tests/integration/governance/production_authoring_test.go`、`tests/integration/db/semantic_production_migrations_test.go`
- `pkg/identity/id.go`、`pkg/identity/id_test.go`、`sdk/typescript/src/ids.ts`、`sdk/typescript/src/ids.test.ts`：prodop 类型标识。
- `db/sqlc.yaml`、`cmd/semlia/readiness.go`、`cmd/semlia/readiness_test.go`、`tests/integration/db/database_test.go`：schema 注册、精确 migration 23 readiness 和迁移清单；保留旧阶段断言。
- `internal/platform/config/config.go`、`internal/platform/config/config_test.go`、`.env.example`：默认关闭的生产写 feature fence；不代替部署时停用旧 writer。
- `internal/platform/http/catalog.go`、`internal/platform/http/governance.go`：既有共享路由注册、production 成员旧入口防绕过及兼容字段投影，不扩展机器权限。

Test targets：
SP-AC-001 的冷启动、SP-AC-003、SP-AC-009 的服务端部分、SP-AC-010 的并发与授权；旧单目标 authoring 和 catalog 创建兼容。

交付内容：
明确 create/update 的结构化生产命令、有限目标集合与局部引用、草稿身份和提案关联、结构化 provenance、服务端幂等与恢复查询、冻结前版本冲突检测。创建不发布；依赖验证和发布由 SP-T004 完成。

验收标准：
零资产/零 release 的工作区可产生实际草稿和提案；同键同请求重放返回原结果，同键异内容冲突。不同候选争用相同语义身份时明确冲突而不是制造重复。任一写入失败没有半完成关联；清空所有浏览器缓存仍能从服务端恢复。恢复每次重新鉴权。

Definition of Done：
数据库约束和服务契约共同区分新建/更新；新建声明不能被旧单项目录或提案操作升级为发布事实；旧 API 已有路径仍有效。失败与提交不明的故障注入通过，前端不在本任务中重写。

验证命令：
```sh
go test -count=1 ./internal/domain/governance ./internal/application/governance ./internal/application/catalog ./internal/platform/http
go test -count=1 -timeout=20m ./tests/integration/governance ./tests/integration/catalog ./tests/integration/discovery
go test -count=1 -timeout=10m ./tests/integration/db -run '^TestSemanticProductionMigration'
make db-generate-check
make contracts-check
make test-contracts
```

TDD plan：RED 冷启动、原子失败、重复请求、语义身份冲突、权限撤销与版本冲突；GREEN 服务端生产与恢复；REFACTOR 提取必要事务内帮助函数，不从一个事务中调用另一个自行提交的仓储方法。

Packet path: `docs/specs/semantic-production/packets/SP-T003.yaml`
Evidence required: 零预置业务对象证明、创建前后计数和真实对象关联、恢复/并发 RED/GREEN、兼容结果、迁移及评审证据。
交付证据：[SP-T003 报告](../../evidence/SP-T003/summary.md)、[A003 验收复核](../../evidence/SP-T003/acceptance-A003/review.md)。原阻断及本轮发现的规范化类型安全、并发列表版本混合均已修复并验证，按用户条件授权 Accepted。T004 可继续；无可信发布/生成证明的请求仍明确拒绝，T005 至 T007 不自动放行。

## 6. SP-T004：验证审核、原子发布与回滚

Status: Accepted
Priority: P0
Depends on: SP-T003 Accepted
Blocks: SP-T005 至 SP-T007
Story / Requirement: SP-US-003、SP-US-004；SP-FR-002、SP-FR-005、SP-FR-006、SP-FR-007、SP-FR-008；SP-NFR-001、SP-NFR-002、SP-NFR-003、SP-NFR-005；SP-TD-002、SP-TD-003、SP-TD-005、SP-TD-008
Parallel: No
Conflicts with: 所有验证、风险策略、审核、发布、回滚、目录权威投影和 release 消费读取改动
Estimate: 3–6 有效小时

目标：
用原有治理机制完成新建及更新的组合语义生产，任一相关对象未满足门禁都不能部分发布。

允许修改范围：
- `migrations/000026_semantic_production_release_integrity.up.sql`、`migrations/000026_semantic_production_release_integrity.down.sql`；migration 23–25 保持原字节。
- 各层 `production_*.go` 辅助文件与对应集成测试、`cmd/semlia/readiness*.go`、`tests/integration/db/database_test.go`；完整文件归属以 A002 执行包为准。
- `db/queries/governance.sql`、`db/queries/semantic_production.sql`、`db/queries/catalog.sql`、`db/queries/registry.sql`、`db/queries/audit.sql`、`db/queries/authorization.sql`、`db/queries/projection.sql`、G-SQLC
- `internal/domain/governance/production.go`、`internal/domain/governance/release.go`、`internal/domain/governance/reviewbatch.go` 及各自同名 `_test.go`
- `internal/application/governance/production.go`、`internal/application/governance/production_test.go`、`internal/application/governance/validation.go`、`internal/application/governance/orchestration.go`、`internal/application/governance/validatejob.go`、`internal/application/governance/validators.go`、`internal/application/governance/validators_test.go`
- `internal/application/governance/decisioninputs.go`、`internal/application/governance/decisioninputs_test.go`、`internal/application/governance/review.go`、`internal/application/governance/review_test.go`、`internal/application/governance/publishing.go`、`internal/application/governance/publishing_access_test.go`、`internal/application/governance/authoring.go`
- `internal/adapters/postgres/semantic_production.go`、`internal/adapters/postgres/governance.go`、`internal/adapters/postgres/release_publishing.go`、`internal/adapters/postgres/reviewbatch.go`、`internal/adapters/postgres/catalog.go`
- `internal/domain/projection/model.go`、`internal/application/projection/publisher.go`、`internal/application/projection/publisher_test.go`、`internal/adapters/postgres/projection.go`、`internal/adapters/gitcontent/adapter.go`、`internal/adapters/gitcontent/adapter_test.go`
- `internal/platform/http/semantic_production.go`、`internal/platform/http/semantic_production_test.go`、`internal/platform/http/governance.go`、G-CONTRACT
- `tests/integration/governance/production_release_test.go`、`tests/integration/governance/release_test.go`、`tests/integration/governance/production_authoring_test.go`、`tests/integration/projection/production_test.go`、`tests/integration/db/semantic_production_migrations_test.go`

Test targets：
SP-AC-005 至 SP-AC-008、SP-AC-010 的审批/发布竞争、SP-AC-012 的无消费者与兼容部分；现有发布/回滚与四通道消费回归。

交付内容：
冻结目标集合、完整输入摘要、声明内新对象引用解析、集合验证与既有审核归因、事务内重新鉴权和基线检查、原子发布全部 pins、多提案来源关联、目录与内容投影、带依赖检查的回滚。源版本变化和人工纠正使对应验证与批准失效，不降低门槛。

验收标准：
两个实体、一个口径及绑定/键/粒度/Join 可形成完整 release；无 consumer/binding 也可发布。任一成员失败或权限变化，release、pins、终态和 outbox 均不部分提交。旧 digest 不变，回滚保留历史，先前不存在的发布目标恢复缺席而非删除。新集合成员不能借旧单项接口绕过检查。

Definition of Done：
发布、审核、失败恢复和兼容性风险获独立检查；新旧提案和 release 的真实 PostgreSQL 与投影测试通过。不得只让原始样例通过而跳过反例或旧 API。

验证命令：
```sh
go test -count=1 ./internal/domain/governance ./internal/application/governance ./internal/platform/http
go test -count=1 -timeout=20m ./tests/integration/governance ./tests/integration/projection ./tests/integration/catalog
go test -count=1 -timeout=10m ./tests/integration/db -run '^TestSemanticProductionMigration|^TestMachineCredentialLifecycleAndChannelParity$'
go test -count=1 ./internal/application/distribution ./internal/adapters/mcp ./cmd/semlia
make db-generate-check
make contracts-check
make test-contracts
```

TDD plan：RED 新建引用、无消费者发布、失效审核、成员越权、并发发布、事务故障、多提案归因及缺席回滚；GREEN 最小集合治理与发布；REFACTOR 复用单项/组合提交帮助函数，保留旧行为断言。

Packet path: `docs/specs/semantic-production/packets/SP-T004.yaml`
Evidence required: 全对象发布与差异、验证及审核摘要匹配、原子失败、回滚前后历史、四通道兼容、RED/GREEN 和独立风险评审。

执行状态：Accepted，用户于 2026-09-11 明确验收并授权继续 T005。真实验证、完整五类创建/更新、原子失败、授权与审批失效、head CAS、完整清单保留、受保护重复回滚、身份重引入和 Git 归因均有运行证据。见 [交付报告](../../evidence/SP-T004/summary.md) 和 [风险复核](../../evidence/SP-T004/A002/review.md)。

## 7. SP-T005：内部 AI 语义生产

Status: Accepted
Priority: P1
Depends on: SP-T004 Accepted
Blocks: SP-T006、SP-T007
Story / Requirement: SP-US-002；SP-FR-002、SP-FR-004、SP-FR-005、SP-FR-006、SP-FR-009；SP-NFR-001、SP-NFR-002、SP-NFR-003；SP-TD-004、SP-TD-006
Parallel: No
Conflicts with: 生产协调、模型生成与 run 归因、授权输入投影和契约改动
Estimate: 1–2 有效小时

目标：
使内部 AI 根据来源与证据产生可治理的语义定义和关系，而非只对已有资产修改描述。

允许修改范围：
- `internal/application/governance/generation.go`、`internal/application/governance/aiproposal.go`、`internal/application/governance/aiproposal_test.go`、`internal/application/governance/production.go`
- `internal/application/governance/production_generation.go`、`internal/application/governance/production_generation_test.go`、`internal/application/governance/production_output.v1.schema.json`
- `internal/adapters/postgres/semantic_production.go`、`internal/adapters/postgres/governance.go`、`db/queries/semantic_production.sql`、G-SQLC
- `internal/platform/http/semantic_production.go`、`internal/platform/http/semantic_production_test.go`、`internal/platform/http/modelconfig.go`、`cmd/semlia/main.go`、G-CONTRACT
- `tests/integration/governance/production_generation_test.go`
- `tests/fixtures/semantic-production/` 下版本化合成来源、业务规则与协议响应；此 glob 仅用于同一有限案例的多文件材料

Test targets：
SP-AC-004、SP-AC-006 的生成纠正、SP-AC-010 的输入注入/授权；旧 GenerateProposal 的模型协议回归。

交付内容：
来源固定输入、结构化输出 schema、局部目标引用、同一生产命令提交、模型 run 与人工修改归因、失败和未知结果查询。不引入通用 Agent 框架、外部测试客户端或新的供应商依赖。

验收标准：
合法协议输出可生成新实体/口径/关系提案，越权引用和缺失业务规则被拒绝或保留未决；模型失败不发布、不创建假成功提案。模型请求与数据库事务分离，提交时重新检查输入和授权。重放生产请求不自动重放付费生成。

Definition of Done：
确定性协议测试通过，原始建议/人工纠正的受影响检查可复核，供应商费用未知时明确未知。实际模型调用由 SP-T007 独立执行，本任务不能用 stub 验收报告宣称真实生成质量通过。

验证命令：
```sh
go test -count=1 ./internal/application/governance ./internal/platform/http
go test -count=1 -timeout=20m ./tests/integration/governance
make db-generate-check
make contracts-check
make test-contracts
```

TDD plan：RED 结构错误、伪造身份、来源提示注入、模型失败、输入过期和纠正重检；GREEN 同契约生成与持久化；REFACTOR 只提取生成输入/结果映射公共部分，不重写模型客户端。

Packet path: `docs/specs/semantic-production/packets/SP-T005.yaml`
Evidence required: 协议替身标识、输入/输出与变更摘要、运行归因、失败与重放断言、RED/GREEN、真实模型未验证声明。

执行预检：T004 已验收。用户于 2026-09-11 批准 [A001 预检](../../evidence/SP-T005/A001/preflight.md) 的受限范围扩展，允许 migration 27、默认关闭的服务端生成授权配置及必要的运行/HTTP/迁移验证接入；精确范围见 [A001 执行包](packets/SP-T005.yaml)。旧迁移保持原字节，本轮只用协议替身，不授权真实模型调用。

交付范围：未决 definition/scope 阻断已修复，全量治理集成、生成 race、迁移及契约检查通过。见 [交付报告](../../evidence/SP-T005/summary.md) 与 [风险复核](../../evidence/SP-T005/A001/review.md)。实际模型验证归属 T007，桌面实现归属 T006。

验收状态：Accepted。用户授权检查验收 T005、无问题后继续 T006；[本轮复核](../../evidence/SP-T005/acceptance-review/re-review.md) 未发现新的阻断问题，全量治理集成、相关单元和契约检查通过。P1 的 [A002 可信业务规则确认](../../evidence/SP-T005/A002/review.md) 修复有效；全量数据库 suite 的 5 项旧测试失败及实际模型未验证限制保留。T006 开放执行预检。

## 8. SP-T006：连续桌面生产体验

Status: Accepted

目录字段标识兼容、回滚事件动作及读取失败的错误隔离与写入锁定通过复核。新鲜两个桌面完整流程 4 passed，前端 230 tests、治理/目录集成及相关检查通过。按用户条件授权完成验收，见 [T006 验收复核](../../evidence/SP-T006/acceptance-review/re-review.md)。T007 开放预检。

执行范围包含用户于 2026-09-11 明确批准的回滚 outbox 动作修复与真实投影回归测试两文件扩围，见 [回滚投影阻断](../../evidence/SP-T006/A001/rollback-projection-blocker.md)。桌面验收保留发布、回滚和重复回滚投影成功断言。

执行预检：T005 Accepted。用户已批准真实后台协议替身启动器的两项 Go 测试支持文件扩展，见 [预检](../../evidence/SP-T006/preflight.md)。当前执行包为 [SP-T006-A001](packets/SP-T006.yaml)，不以外部假端点制造 actual_model 验收证据。
Priority: P1
Depends on: SP-T005 Accepted
Blocks: SP-T007
Story / Requirement: SP-US-001 至 SP-US-004；SP-FR-003、SP-FR-004、SP-FR-005、SP-FR-009、SP-FR-010；SP-NFR-001、SP-NFR-002、SP-NFR-004、SP-NFR-005；SP-TD-004、SP-TD-007、SP-TD-008
Parallel: No
Conflicts with: 来源、知识、资产、治理视图、路由、SDK 和嵌入前端改动
Estimate: 2–4 有效小时

目标：
用户从来源候选完成建模、审核、发布和恢复，不跨页面重复录入，也不靠浏览器缓存维护后台流程。

允许修改范围：
- `web/src/LiveSourcesView.tsx`、`web/src/LiveSourcesView.test.tsx`、`web/src/candidateRecovery.ts`、`web/src/candidateRecovery.test.tsx`
- `web/src/semanticProduction.ts`、`web/src/semanticProductionRuntime.tsx`、`web/src/semanticProductionRuntime.test.tsx`、`web/src/SemanticProductionPanel.tsx`、`web/src/SemanticProductionPanel.test.tsx`
- `web/src/KnowledgeViews.tsx`、`web/src/KnowledgeViews.test.tsx`、`web/src/CatalogControls.tsx`、`web/src/governance.ts`、`web/src/governanceRuntime.tsx`、`web/src/governance.test.tsx`
- `web/src/ProductApp.tsx`、`web/src/ProductApp.test.tsx`、`web/src/styles.css`、`web/src/ingestion-workspace.css`、G-WEB
- `web/e2e-production/production.spec.ts`、`web/e2e-production/fixtures.ts`、`web/playwright.production.config.ts`、`web/package.json`
- `scripts/dev/production-acceptance.sh`、`compose.production-acceptance.yaml`
- `internal/testsupport/productionacceptance/main.go`、`internal/testsupport/productionacceptance/main_test.go`
- `internal/adapters/postgres/production_release_persistence.go`、`internal/adapters/postgres/catalog.go`、`tests/integration/governance/production_projection_test.go`，仅限用户获批修复与对应回归

Test targets：
SP-AC-001、SP-AC-003 至 SP-AC-009 的用户路径、SP-AC-011；组件边界与真实后台浏览器验收。

交付内容：
候选新建/匹配、来源依据、AI 建议及人工纠正、结构化内容与关系审查、冻结版本验证、集合审核和发布/回滚状态、服务端恢复入口。新验收脚本拥有独立 Compose 项目和资源，不能使用默认开发栈进行破坏性设置。

验收标准：
两个桌面尺寸完成连续真实路径，刷新/清除 localStorage 后仍可恢复。权限与错误状态来自实际 API，失败不降级 mock。保留键盘与可见焦点、reduced motion，文字不重叠；工具图标使用已有 lucide，页面不改造成业务任务中心。

Definition of Done：
组件测试、两个尺寸的真实浏览器截图及核心操作通过；隔离栈的准备和清理校验 project/端口/卷所有权；业务资产只经正常生产 API 创建。旧来源、目录与单项治理可继续使用，嵌入包同步。

验证命令：
```sh
pnpm --filter @semlia/web test
pnpm --filter @semlia/web typecheck
pnpm --filter @semlia/web lint
pnpm --filter @semlia/web build
./scripts/dev/production-acceptance.sh --suite desktop
make web-embed
make web-embed-check
```

`production-acceptance.sh` 使用独立 owner、随机端口和专属卷，经保护单测及真实准备/清理验收。脚本拒绝未确认所有权的现有实例，不回退到默认 Compose。

TDD plan：RED 冷启动选择、结果不明、缓存丢失、关联对象审查、失效审核和权限变化；GREEN 接入新生产 API 与真实状态；REFACTOR 删除本任务替代的浏览器写入协调，保留可恢复的旧记录提示。

Packet path: `docs/specs/semantic-production/packets/SP-T006.yaml`
Evidence required: 组件 RED/GREEN、真实网络与对象引用、两尺寸截图、键盘/焦点/刷新恢复、隔离准备清理日志和视觉评审。

## 9. SP-T007：完整回归与生产验收

Status: Accepted

用户于 2026-09-11 授权检查验收。逐任务差异、678 个非文档源码快照文件、完整门禁与安全原始结果复核通过；关键生产、预留防护、启动器及契约新鲜测试通过，未发现阻断问题。见 [验收复核](../../evidence/SP-T007/acceptance-review.md)。
Priority: P1
Depends on: SP-T006 Accepted；最终测试环境所有权明确；实际模型分项运行前另需配置与受限调用授权
Blocks: 本生产阶段用户验收；不自动启动 EAI 任务
Story / Requirement: SP-US-001 至 SP-US-004；SP-FR-001 至 SP-FR-010、SP-NFR-001 至 SP-NFR-005；SP-AC-001 至 SP-AC-012；SP-TD-008
Parallel: No
Conflicts with: 冻结待验收工作区的所有实现改动、默认开发栈与任何部署操作
Estimate: 2–4 有效小时

目标：
独立证明首次生产、变化生产和失败恢复的全部验收要求，区分协议能力、真实 AI 质量和已有兼容性，不只录一段正常操作。

允许修改范围：
- `tests/integration/governance/production_acceptance_test.go`、`tests/integration/governance/production_generation_test.go`
- `tests/integration/db/semantic_production_migrations_test.go`、`tests/integration/projection/production_test.go`
- `tests/fixtures/semantic-production/` 的同一合成案例及评分答案；黄金答案与实际模型输入分离
- `web/e2e-production/production.spec.ts`、`web/e2e-production/fixtures.ts`
- `scripts/dev/production-acceptance.sh` 的验收集合与隔离检查，不修改应用业务行为
- `docs/specs/semantic-production/release-report.md`、本任务证据与执行包
- 用户授权的兼容测试修复：`tests/integration/db/{database_test,execution_test,governed_objects_test,webhook_interfaces_test,machine_interfaces_test}.go` 及 `tests/integration/operations/operations_test.go`，保留历史保护和并发断言。
- 用户授权的纯格式修正：`internal/platform/http/handler.go`、`internal/application/governance/production_history.go`、`internal/domain/governance/{release,proposal,production}.go`，仅 gofmt。
- 用户授权的文档可移植性修正：`docs/evidence/SP-T005/acceptance-review/re-review.md`、`docs/evidence/SP-T006/A001/test-results.md`，仅本机路径替换，不改变历史结论或结果。
- 用户授权的安全补丁：`go.mod`、`go.sum`、`pnpm-workspace.yaml`、`pnpm-lock.yaml`，仅 grpc 1.83.2、js-yaml 4.3.2 及对应校验信息；保持安全扫描标准并完整复验。

Test targets：
全部 SP-AC；明确 opt-in 的 `TestSemanticProductionConfiguredModel`；旧单目标和四通道契约；10,000 资产基准；迁移/降级保护；两个桌面尺寸。

交付内容：
逐项验收矩阵、完整实际模型样例和人工纠正证据、故障/并发/权限反例、原始与修正结果分别评分、数据库与接口兼容报告、可复现版本及当前阶段发布报告。这里的发布报告不是部署授权。

验收标准：
全部固定案例适用断言通过，失败/跳过/受阻单列；模型生成由实际配置的供应商完成，不将 stub、人工填写或模型自评冒充证据。正常和拒绝案例同时通过，已发布内容不依赖任何 consumer 或外部 AI 客户端。

Definition of Done：
规格符合性、独立工程风险评审和桌面验收完整；新鲜测试、差异与残余风险可复核，用户接受后才将阶段标记 Accepted。发现产品问题记录责任模块的受限修复任务或新 attempt，经批准后修复和复验，保留原验收历史；不在本测试任务中夹带业务改动。无实际模型授权时先完成确定性检查并明确受阻项，不伪造阶段完成。

验证命令：
```sh
go test -count=1 -timeout=30m ./tests/integration/ingestion ./tests/integration/discovery ./tests/integration/governance ./tests/integration/projection ./tests/integration/catalog ./tests/integration/db
SEMLIA_RUN_M1_BENCHMARK=1 go test -count=1 -timeout=10m ./tests/performance/catalog -run '^TestCatalog10000Performance$'
SEMLIA_RUN_PRODUCTION_MODEL=1 go test -count=1 -timeout=10m ./tests/integration/governance -run '^TestSemanticProductionConfiguredModel$'
./scripts/dev/production-acceptance.sh --suite full
make check
```

最后两项只在该次验收拥有的专用完整工作区快照与环境中执行，包含当前未提交/未跟踪实现，但不复制客户数据或已有凭据。`make check` 会启动/停止 Compose 并构建固定标签镜像，执行包必须确认其 Docker 环境、精确 project、env、端口、镜像标签与清理范围；独立目录不等于 Docker 隔离，共享标签冲突时须使用独立 Docker 环境，不得重标用户镜像或影响默认开发栈。实际模型测试读取环境中的受限配置，不将密钥写入命令、报告或测试代码。

TDD plan：RED 先补尚缺的验收断言并证实触发对应反例；GREEN 只在责任实现任务的修复经批准并集成后形成；REFACTOR 限定测试辅助逻辑，禁止降低断言。已有全部覆盖时复跑并记录，不为制造 RED 破坏生产代码。

Packet path: `docs/specs/semantic-production/packets/SP-T007.yaml`
Evidence required: 完整 AC 矩阵、实际模型/协议替身区分、费用与数据边界、原始和修正质量、迁移与回滚、全量门禁、性能与截图、独立 review、用户 acceptance。

## 10. 执行状态与恢复

SP-T001 至 SP-T007 全部 Accepted，本阶段剩余任务 0 项。专用环境 full suite、两个桌面尺寸与最终安全补丁后的完整门禁通过，T007 验收复核完成；实际模型使用 4/30 次授权调用。证据与风险见 [阶段验收报告](release-report.md)。外部接入的 3 项任务仍为 Draft/Deferred，恢复前需用户确认优先级并重新审核计划，不自动执行。

任务的有效小时是估算，不是持续后台执行承诺。SP-T001 与 SP-T003 验收时校准剩余工期；具体假设、缓冲和检查点见计划第 6 节。外部接入任务保持 Deferred。

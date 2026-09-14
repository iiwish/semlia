# 外部 AI 接入任务图

| 元数据 | 值 |
| --- | --- |
| Version | 0.1.1 |
| Status | Ready_For_User_Review |
| Scheduling | Deferred；全部 EAI 任务不进入当前执行队列 |
| Product contract | [product-design.md](product-design.md) 0.4.1 Confirmed |
| Plan | [plan.md](plan.md) 0.1.1 Ready_For_User_Review；Deferred |
| Last updated | 2026-09-09 |
| Approval | Pending；全部任务保持 Draft；先完成语义生产验收，再由用户确认恢复接入并重新审核计划 |

## 1. 工作图与边界

当前交付是 [语义生产闭环重构](../semantic-production/product-design.md)。下表为后续接入任务的相对顺序和范围，P0/P1 仅表示恢复该计划后的内部优先级，不覆盖生产优先决策。生产闭环不依赖本表任何任务。

| Epic | Story | 任务 | 依赖 | 结果 |
| --- | --- | --- | --- | --- |
| EAI-E01 公开消费验证 | EAI-S01 普通机器客户端正确消费，US-001/US-004 | EAI-T001 | 无 | 新鲜基线与逐项覆盖结论 |
| EAI-E01 公开消费验证 | EAI-S02 外部 AI 使用可移植接入包，US-001/US-002/US-004 | EAI-T002 | EAI-T001 验收通过 | 协议探针、Skill、合成案例与独立 AI 使用证据 |
| EAI-E02 探索契约 | EAI-S03 未治理数据的只读接入边界，US-002/US-003/US-005 | EAI-T003 | EAI-T002 验收通过 | 探索身份、版本、投影与拒绝的完整契约，独立提交审核 |

三个任务串行。当前只规划 R0 实现和 R1 契约；候选写入、业务映射校验、完整调研及生产查询不在本工作图内。对完整产品 FR/NFR 的分期覆盖见计划第 6 节，不声称三个任务实现全部产品要求。

审批流程：语义生产闭环验收、用户明确恢复接入后，重新核对并确认计划与任务图，再生成下一任务的执行包；包完整、前置验收和只读一致性检查通过后才将该任务改为 Ready。本文列出的 packet 路径是待生成位置，不代表文件已存在。任务执行后保持 Needs_Review，用户验收后才为 Accepted。

## 2. EAI-T001：公开消费基线

Status: Draft
Scheduling: Deferred
Priority: P0
Depends on: 语义生产闭环验收且用户明确恢复接入；执行前重新核对并确认本计划与任务图、执行包及工作区检查
Blocks: EAI-T002
Story / Requirement: EAI-S01；FR-001、FR-003、FR-007、FR-008、FR-009、FR-011、FR-012；NFR-002 至 NFR-006；TD-001、TD-002、TD-004、TD-008
Parallel: No
Conflicts with: EAI-T002、EAI-T003；其他修改机器身份、distribution 或相关测试的任务

目标：
在不修改产品行为的情况下复跑现有公开接口测试，逐项区分计划 B-01 至 B-08 的已验证、失败和覆盖缺口。获得可信基线，不用静态阅读或历史报告充当本轮通过证据。

允许修改范围：
- `docs/evidence/EAI-T001/summary.md`
- `docs/evidence/EAI-T001/test-results.md`
- `docs/evidence/EAI-T001/coverage.md`
- `docs/evidence/EAI-T001/` 下脱敏的真实命令输出和工作区差异清单
- 本任务在 `docs/specs/data-finding-workflow/tasks.md` 的状态与 evidence 引用

Test targets：
- `tests/integration/db/machine_interfaces_test.go`
- `cmd/semlia/mcp_test.go`
- `internal/platform/http/machine_test.go`
- `internal/application/identity/machine_test.go`
- `internal/application/distribution/service_test.go`

交付内容：
记录工作区提交、相关未提交/未跟踪依赖、工具链与隔离数据库；保留真实命令状态；B-01 至 B-08 每项关联现有断言或明确未覆盖，新增断言需求交给 EAI-T002 的参考探针。

验收标准：
已有测试确实运行，公开结果与权限边界无失败；既有覆盖不足不标为通过。测试数据库独立，普通调用只用最小机器凭据；不调用执行工具，不把 fixture 设置归为 Agent 能力。

Definition of Done：
覆盖表完整，命令和失败归因可复核，凭据及敏感内容未进入 evidence，未修改业务代码、接口或迁移。发现产品故障时停止并提交失败证据，EAI-T002 不开始；修复需另行确认文件范围，不夹带进本任务。

验证命令：
```sh
go test -count=1 -timeout=10m ./tests/integration/db -run '^TestMachineCredentialLifecycleAndChannelParity$'
go test -count=1 ./cmd/semlia ./internal/platform/http ./internal/application/identity ./internal/application/distribution
make test-contracts
make test-repository
```

TDD plan：验证型任务，无行为实现。先执行现有断言；失败保留，不修改断言或生产实现制造绿色。不需要行为开发的 TDD 豁免。

Packet path: `docs/specs/data-finding-workflow/packets/EAI-T001.yaml`
Evidence required: 新鲜测试结果、B 用例覆盖、工作区身份与差异清单、隔离环境及清理结果、残余风险、规格符合性和代码/安全评审结论。

## 3. EAI-T002：最薄参考接入包

Status: Draft
Scheduling: Deferred
Priority: P0
Depends on: EAI-T001 Accepted
Blocks: EAI-T003
Story / Requirement: EAI-S02；FR-001 至 FR-013 的 R0 子集，具体为计划第 6 节列出的已发布消费及缺能力拒绝；NFR-002 至 NFR-006；TD-001 至 TD-004、TD-008
Parallel: No
Conflicts with: EAI-T001、EAI-T003；修改 MCP、CLI、SDK、机器身份或合成集成环境的任务

目标：
让工程师以普通消费者身份运行协议验证，并让外部 AI 使用相同契约完成已发布语义的合成案例；不能把固定脚本称为 Agent，也不新增 AI 服务端。

允许修改范围：
- `examples/external-ai/README.md`
- `examples/external-ai/SKILL.md`
- `examples/external-ai/probe/main.go`
- `examples/external-ai/probe/main_test.go`
- `examples/external-ai/cases/`：只允许合成输入与版本化输入 schema
- `tests/testdata/external-ai/`：只允许隔离的黄金答案与评分 schema，不放入 Skill 可读材料
- `tests/integration/db/external_ai_reference_test.go`
- `docs/evidence/EAI-T002/`：真实、脱敏的测试与外部宿主运行证据
- 本任务在 `docs/specs/data-finding-workflow/tasks.md` 的状态与 evidence 引用

不得修改核心应用服务、生产 HTTP/MCP、权限、OpenAPI、数据库、依赖锁文件、产品 UI 或部署文件。若探针暴露服务端缺陷，先记录并另行审批修复范围。

Test targets：
参考包的输入校验、工具白名单、预算、超时/重试与退出状态；通过真实 HTTP/MCP 的 B-01 至 B-08；现有机器生命周期与四通道一致性；至少一个外部 AI 宿主的同契约合成案例。

交付内容：
Skill 使用真实工具名，解释 release-only 与能力缺口；Go 探针验证结构化调用，输入和结果严格按 schema 校验。运行端只读合成输入，不接收 oracle。接入文档区分测试设置、普通调用、评分与清理。

验收标准：
- R0 八类基线均有可执行断言，匹配版本和拒绝语义，不要求各通道 ID/时间/channel 相等。
- 真实 AI 宿主在未知黄金答案的条件下完成已发布案例，输出正确版本引用；需要探索或业务规则的案例明确受阻，不伪造完成。
- Bounded retry 计入总预算；取消停止派发；撤销后的后续调用拒绝。不得为测试取消而启动真实 SQL。
- 不加载 Skill 的协议客户端仍受相同权限门禁；停用参考客户端不影响 Semlia 基础能力。
- 真实宿主不可用时该验收项为未验证，不以协议探针的通过替代，不自动申请安装或调用付费模型。

Definition of Done：
协议与真实 AI 使用分别有证据，敏感数据与 oracle 隔离，所有代码在允许范围内，既有通道无回归，残余缺口能映射到 R1/R2。没有完整业务数据结果时不标记预算/实际需求已完成。

验证命令（新增目录与测试在本任务实现后存在）：
```sh
go test -count=1 ./examples/external-ai/probe
go test -count=1 -timeout=10m ./tests/integration/db -run '^(TestMachineCredentialLifecycleAndChannelParity|TestExternalAIReferenceConsumption)$'
make test-contracts
make test-repository
make contracts-check
```

真实 AI 宿主步骤：按参考包说明配置隔离 endpoint 与最小凭据，运行固定案例，记录真实工具调用及回答，用隔离 oracle 评分。宿主证据必须注明版本，不在本计划中虚构可执行的宿主命令。

TDD plan：
RED 先添加输入与预算/工具白名单的失败测试，以及 B 用例缺口的网络级断言；GREEN 实现最小探针与案例读取；REFACTOR 仅整理本包。安全失败样本与正常成功样本都需通过，不能靠拒绝所有调用获得绿色。

Packet path: `docs/specs/data-finding-workflow/packets/EAI-T002.yaml`
Evidence required: RED/GREEN 命令、公开接口调用与版本、oracle 评分、真实宿主记录、敏感信息检查、真实 diff、两轮评审及残余 R1/R2 缺口。

## 4. EAI-T003：只读探索详细契约

Status: Draft
Scheduling: Deferred
Priority: P1
Depends on: EAI-T002 Accepted
Blocks: R1 探索实现规划；本工作图不包含该实现
Story / Requirement: EAI-S03；FR-002 至 FR-006、FR-008 至 FR-010、FR-013 的探索与后续治理边界；NFR-002 至 NFR-006；TD-005、TD-006、TD-007、TD-008
Parallel: No
Conflicts with: EAI-T001、EAI-T002；其他探索身份、来源快照或证据契约设计

目标：
将已验证缺口收敛为可以实现和审查的最小只读探索契约，明确冷启动身份、快照、授权投影与拒绝，避免先实现再补安全定义。

允许修改范围：
- `docs/specs/data-finding-workflow/contracts/exploration.md`
- `docs/specs/data-finding-workflow/contracts/exploration.openapi.yaml`
- `docs/specs/data-finding-workflow/data-model.md`
- `docs/specs/data-finding-workflow/exploration-plan.md`
- `docs/specs/data-finding-workflow/checklists/exploration.md`
- `tests/contracts/external_ai_contract_test.go`：只校验独立探索契约的 OpenAPI 完整性，不改变生产行为
- `docs/evidence/EAI-T003/`：只读代码分析、契约验证和风险记录
- 本任务在 `docs/specs/data-finding-workflow/tasks.md` 的状态与 evidence 引用

只允许文档、契约与其静态校验测试，不修改应用 `api/openapi/`、权限或迁移，不运行写入真实环境的试验。

Test targets：
凭据签发/校验/撤销链路，来源和物理 revision 的持久化，证据与发布投影；设计反例包括无 release、改名、删除、未变化成员、部分/失败扫描、分页并发与权限撤销。

交付内容：
- 决定凭据用途与 consumer/binding/source scope 关系，证明旧消费凭据不获得新权限；列出兼容迁移、轮换和撤销路径。
- 定义最小只读搜索、revision 详情及证据读取的 REST/MCP 映射、schema、分页、限额、字段白名单、错误与审计。接口名在本任务中确定，不冒充当前工具。
- 明确扫描成员与历史名称/字段的权威模型，说明是否新增持久字段及索引、如何处理缺失历史，不覆盖既有不可变记录。
- 定义性能基准与验收数据，沿用 SSOT 万表目标；记录正确候选与拒绝反例，不能仅用延迟证明质量。
- 将候选提案、业务映射校验、执行和管理端改动列为独立范围；不借“只读探索”自动扩大。

验收标准：
上述决策均有明确结果、替代方案与风险；schema 可验证，字段能映射到权威记录或明确新增数据模型，权限矩阵可生成正反测试，历史版本与无 release 案例有确定行为。缺关键决策时文档保持 Draft，不以占位符进入实施。

Definition of Done：
详细契约和探索实施计划达到 Ready_For_User_Review，需求检查完成，未写入产品或生产状态；交付的是可审查契约，不宣称探索已实现。用户对该契约的确认是下一阶段实现规划的前置条件。

验证命令：
```sh
go test -count=1 ./tests/contracts -run '^TestExternalAIExplorationContract$'
git diff --check
```

新增静态测试沿用 `tests/contracts/contract_test.go` 中锁定的 `kin-openapi/openapi3` loader 与 Validate；不安装新工具。契约链接、schema 引用和权限矩阵另以只读校验脚本检查并保留命令记录。

TDD plan：契约设计任务，不实现行为；先固定正反例与状态表，再形成 schema 和数据映射，并对缺失字段与不一致引用运行校验。后续行为实现必须单独采用 RED/GREEN 验证。

Packet path: `docs/specs/data-finding-workflow/packets/EAI-T003.yaml`
Evidence required: 代码依据、权威映射、权限矩阵、历史版本反例、schema/链接校验、技术风险及用户审核状态。

## 5. 交付与停止条件

任务证据位于各自 `docs/evidence/EAI-T00N/`，本文件只记录有证据的状态。当前没有任何 Ready、Running 或 Accepted 任务，也没有生成执行包。

出现未说明的权限扩大、真实数据访问、生产写入、服务端修复、依赖修改或不在范围内的文件需求时停止，不通过删除测试、降低验收或修改既有用户工作规避问题。工作区含大量未提交实现，执行前必须重新核对依赖，不能假定清洁 HEAD 与当前代码等价。

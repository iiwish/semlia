# Semlia 语义资产设计

## 元数据

| 字段 | 值 |
| --- | --- |
| 文档状态 | Confirmed |
| 最后更新 | 2026-09-21 |
| 适用范围 | Semlia 领域模型、Web 原型、REST/MCP/CLI/SDK 交付契约 |
| 上位依据 | `docs/SSOT.md` v0.7.0 Confirmed |

## 1. 决策摘要

语义资产是 Semlia 中具有稳定身份、明确业务含义、版本化实现、可追溯证据、治理状态和消费契约的知识对象。语义资产不是一段说明文字，也不是表或字段的别名；它是人类与机器可以共同引用、验证、发布和执行的最小治理单元。

知识资产默认进入知识目录，左侧 contextual panel 只显示最近打开的资产：

```text
知识资产
├── 最近打开（资产快捷入口）
└── 知识目录（资产中心清单）
    ├── 语义资产
    ├── 数据资产（经整理的数据知识）
    ├── 语义关系
    ├── PhysicalBinding
    └── JoinContract
```

“本体”是语义资产及其受治理关系在一个领域中的机器可理解投影。“物理数据图谱”是来源对象、代码、血缘、键、粒度和 Join 观察的事实投影。两者不是与语义资产并列的资产类型，也不形成用户必须依次经过的页面。

## 2. 规范定义

一个对象只有同时满足以下条件，才是 Semlia 语义资产：

1. **稳定可寻址**：拥有工作区内唯一且不会随名称变化的 `asset_id`、命名空间和 key。
2. **含义结构化**：包含定义、适用范围、排除规则、业务示例和类型专属语义。
3. **责任明确**：拥有语义域、owner、治理级别和适用策略。
4. **实现可解析**：可执行资产具备明确的表达式、粒度、时间语义、物理绑定和 JoinContract。
5. **证据可追溯**：关键字段能够追溯到具体来源 revision、权威类型和验证结果。
6. **版本不可变**：每次内容变化形成新的 `AssetRevision`，生产消费只引用不可变 `Release` 中的 revision。
7. **消费受契约约束**：消费者通过稳定 ID、release 和 Binding 使用资产，不读取未发布草稿。

可以用以下等式理解：

```text
SemanticAsset
= Stable Identity
+ Semantic Definition
+ Typed Relations
+ Physical Implementation
+ Evidence and Validation
+ Ownership and Policy
+ Revision and Release
+ Consumer Contract
```

### 2.1 不属于语义资产的对象

- 仅被扫描到的表、字段、SQL、文件和代码工件属于物理对象或来源工件。
- `KnowledgeBlock` 是最小可引用知识单元，支持资产字段但不直接成为生产事实。
- `JoinObservation` 是连接候选；只有经治理发布的 `JoinContract` 可以用于生产解析。
- AI 推断、查询历史和运行结果属于候选或证据，不能仅凭置信度成为已发布含义。
- Dashboard、报表和问答结果是消费者或消费结果，不是语义资产本身。

## 3. 资产类型体系

主要知识为业务对象、业务口径、指标、数据资产和分析模型。分类服务于独立维护与复用，内部属性、条件、聚合、关系和绑定保持强类型，不按执行引擎构件创建所有一级知识。

### 3.1 主要知识对象

| 目标标识 | 对象与职责 | 最小契约 |
| --- | --- | --- |
| `business_object` | 业务对象：客户、订单等对象或事件 | 自身定义、身份键、粒度、生命周期、可寻址属性和业务关系 |
| `business_term` | 业务口径：独立术语与可选判定 | 定义、边界、示例；可执行时包含对象、条件、参数、依赖和有效范围 |
| `metric` | 指标：基础聚合或派生计算 | 类型、属性/指标输入、聚合/公式、过滤、时间、单位、空值和可加性 |
| `data_asset` | 数据资产：实际数据的解释与使用契约 | 来源/字段成员、用途、覆盖、粒度、身份、时间、质量与加工实现 |
| `analysis_model` | 分析模型：场景内的知识和数据组合 | 对象、公开成员、适用口径、模型粒度、默认时间、资产、绑定与连接 |

目标标识不等于当前 API 的可提交枚举。生产入口仍接收七种类型和六个公共 AssetContent 字段，完整对应与兼容要求见[知识对象契约 §5](../knowledge-to-data/knowledge-types.md#5-当前接口覆盖与兼容要求)。本设计不授权删除既有 ID、改写历史 release 或执行迁移。

每份对象和指标自身解释含义，不额外维护同名概念。业务口径管理独立术语及判定规则，定义与条件共同发布；分群对应其中的可选判定能力，不要求同名双对象。仅用于解释的口径不能直接参与执行，名单快照、定时刷新和触达不属于首轮必备能力。

属性属于业务对象或事件，按 `asset_id + revision_id + member_id` 寻址并随所属修订发布。维度是属性承担的查询角色；原始字段不自动成为一级知识，共享日历和层级按实际复用需要管理。

订单实付金额是属性，SUM 金额和 COUNT DISTINCT 订单键是基础指标，客单价是派生指标。基础与派生统一治理，内部严格区分输入、聚合和聚合后计算；度量可作为适配器构件，不要求用户另建同名资产。

对象定义身份，资产解释一份数据，模型组织场景组合，三者不合并。数据资产引用来源和已验证加工，不复制明细或第二套结构；模型引用属性映射和连接契约，不复制指标公式。见[数据资产、本体与血缘](../knowledge-to-data/data-assets-ontology-lineage.md)。

### 3.2 治理对象与支撑对象

`policy`、`contract` 和 `test` 具有稳定 ID、revision 和 release 范围，但不属于知识目录的五类主要知识。它们作为治理对象被知识资产引用，并在对应任务视图中展示：

| 类型 | 作用 | 默认入口 |
| --- | --- | --- |
| `policy` | 约束访问、发布、质量或使用行为 | 资产“可信度”与系统策略 |
| `contract` | 固定生产者输入、语义输出和消费者兼容性 | 资产“实现”与“交付与影响” |
| `test` | 对定义、编译、结果、关系和兼容性执行可复现检查 | 资产“可信度” |

治理对象可以被全局搜索和 API 直接寻址，但不占用知识目录的高频类型筛选。`SemanticRelation`、`PhysicalBinding`、`JoinContract`、`KnowledgeBlock`、`EvidenceArtifact`、`OntologyRevision` 与血缘记录不是主要知识类型，通过资产详情或目录子对象进入。原始物理表字段是可调查来源；数据资产是引用来源、包含业务使用契约的主要知识类型，两者身份与生命周期分开。

### 3.3 类型 Profile

五类主要知识由版本化 `AssetTypeProfile` 驱动。Profile 是产品、验证服务和对外交付共用的类型契约，不是前端专属配置：

```text
AssetTypeProfile
├── ContentTemplate       类型专属字段、章节重点和必填契约
├── RelationPolicy        允许关系、方向、基数和本体一致性约束
├── ImplementationPolicy  PhysicalBinding 与 JoinContract 的要求
├── ValidationProfile     发布门禁、质量提醒和修复建议
└── EmptyStatePolicy      不适用、可选缺失和阻断缺失的呈现规则
```

每条类型验证规则包含 `rule_id`、`severity`、`field_path`、期望条件、实际结果、证据引用、validator 版本和 remediation。严重级别固定为：

| 级别 | 含义 | 发布行为 |
| --- | --- | --- |
| `Blocker` | 资产身份、含义或执行安全不成立 | 阻止候选 revision 发布 |
| `Warning` | 可以发布但存在可解释的质量缺口 | 进入 owner 待办并保留风险说明 |
| `Info` | 不影响发布的上下文或建议 | 仅用于理解和审计 |
| `Not Applicable` | 该规则不适用于当前资产能力 | 不计为缺失或通过 |

空状态不能统一为“暂无数据”：

| 空状态 | 使用条件 | 界面行为 |
| --- | --- | --- |
| `not_applicable` | 该类型或当前能力不需要此对象 | 明确说明为何不适用，不显示修复动作 |
| `optional` | 对象可缺失且不阻止发布 | 说明消费或治理收益，可提供非阻断动作 |
| `missing` | 缺失会阻止发布或破坏含义完整性 | 展示原因、责任人和直接修复动作 |

## 4. 统一资产契约

### 4.1 身份

| 字段 | 规则 |
| --- | --- |
| `asset_id` | 不可变、全局唯一、与名称和路径解耦 |
| `namespace` | 表达工作区内的领域边界，例如 `commerce` |
| `key` | 命名空间内稳定、机器可读，例如 `net_revenue` |
| `name` | 当前主要展示名称，可随 revision 变化 |
| `aliases` | 受版本管理的同义名称，不自动等同于主名称 |
| `type` | 使用受控类型枚举，不允许来源系统自定义类型覆盖核心语义 |
| `domain` | 负责该含义的业务语义域 |

稳定身份遵循 [TDR-0003](../../adr/0003-resource-identifiers.md)：PostgreSQL 保存 UUIDv7，公共契约使用同值 TypeID，普通产品界面展示 `namespace.key` 语义地址。随机技术 ID 不进入名称、key 或业务定义；key 变更保留 alias 解析，关系和 release manifest 始终引用不可变 ID。

### 4.2 语义定义

所有资产包含：

- 一句话业务定义。
- 包含规则、排除规则和边界条件。
- 正例、反例和常见歧义。
- 类型专属结构化规格。
- 默认时间语义、单位和格式（适用时）。
- 适合 AI 检索和解释的受控上下文，不把 Prompt 作为事实来源。

### 4.3 关系

本体关系与物理实现分为四个平面，界面和领域模型不能混用：

| 层次 | 回答的问题 | 典型关系 |
| --- | --- | --- |
| 概念层级 | 这个概念的上位、下位、等价和互斥概念是什么？ | `broader_than`、`narrower_than`、`equivalent_to`、`disjoint_with`、`synonym_of` |
| 语义关系 | 这个资产在业务上与什么有关？ | `contains`、`measures`、`describes`、`filters_by`、`replaces` |
| 依赖与影响 | 这个资产由什么定义并影响什么？ | `depends_on`、`derived_from`、`validated_by`、`consumed_by` |
| 物理实现 | 这个含义如何落到数据并安全连接？ | `PhysicalBinding`、`EntityKey`、`ModelGrain`、`JoinContract` |

前三个平面属于 `SemanticRelation`，物理实现是独立治理对象。每条语义关系包含来源 revision、创建主体、证据、断言状态和有效 release 范围；断言状态固定为 `asserted`、`inferred`、`candidate` 或 `deprecated`。推导关系必须可以定位推理规则和证据，候选关系不能进入生产默认解析。

每个 `RelationType` 声明 source/target 类型约束、inverse、cardinality、directed/symmetric 推理行为和当前 `OntologyRevision` 下的校验结果。`joins_to` 只表达发现或导航关系；生产连接必须使用声明键、基数、粒度影响、必要过滤和扇出风险的 `JoinContract`。

### 4.4 物理实现

可执行知识必须通过依赖和所选模型实现解析到已验证的版本化绑定，不要求每个指标或口径直接拥有一份表映射。派生指标引用指标，业务口径引用对象属性；分析模型实现引用成员绑定，数据资产实现引用来源字段及加工工件。

以下是成员绑定的目标阅读投影，不是当前 API 请求体；ID 均为合成示例，物理实现未确认：

```yaml
id: example.binding.customer_first_paid
semanticMemberRef:
  assetId: example.object.customer
  revisionId: example-r1
  memberId: first_valid_paid_date
dataMemberRef:
  assetId: example.asset.customer_master
  revisionId: example-r1
  memberId: first_paid_date
physicalBindingRef: null
```

成员绑定固定两端的资产、修订和成员；底层 PhysicalBinding 固定来源 revision、数据集、字段或表达式及适用条件。模型实现引用对应绑定版本，而不是复制相同映射。来源、成员含义或表达式变化需要形成相应修订并重新验证，不能仅凭稳定资产 ID 自动追随最新实现。

模型、业务对象、属性、口径、指标、数据资产及绑定必须在同一 release 中形成兼容组合。关联、映射与派生关系分别声明类型和约束，不能将“有关联”视为可执行 Join。注册账号、统一客户和历史快照等数据资产不可仅因名称相近互换。尚未实现的去重或清洗不能仅靠文字声明通过发布。

### 4.4.1 本体、血缘与影响

本体明确领域对象、关系的方向/类型、身份、基数和一致性约束，并由本体版本与资产 release 共同固定；不要求导入全部业务事实行或首发支持 OWL 推理。数据血缘记录数据集/字段的输入、转换与输出，语义依赖记录资产之间的计算与定义依赖，证据引用说明依据。三类关系分别呈现，通过版本化映射连通变更影响，不合并为无类型图边。

首轮追踪选定案例的数据集级和关键计算字段血缘，保留 SQL/dbt/ETL 工件与来源 revision。未知解析范围、静态推导、运行观察与人工确认显式区分；血缘不自动授权 Join 或证明结果正确。来源变更沿血缘、映射、语义依赖定位待重验资产，并按权限展示影响。

### 4.5 证据与验证

证据按字段支持资产结论，使用 `DECLARED`、`CONSTRAINED`、`DERIVED`、`OBSERVED`、`INFERRED` 五种权威类型。界面展示权威类型、来源 revision、适用范围、时效和冲突，而不是只展示一个置信度数字。

验证至少覆盖：

- 身份与引用完整性。
- 类型专属 schema。
- 表达式或执行模型编译。
- 粒度、键、基数和扇出风险。
- 数据结果与边界样本。
- 权限、策略和消费兼容性。
- 证据新鲜度与冲突。

### 4.6 责任、版本与消费

- owner 对业务含义负责，maintainer 对实现和验证负责，两者通过带有效时间的 `OwnershipAssignment` 关联资产。
- `SemanticAsset` 是稳定身份，`AssetRevision` 是不可变内容，资产版本标识固定为 `asset_id + revision_id`。
- `asset_id` 与 `revision_id` 在数据库中使用 UUIDv7，在 API、事件、Git、日志、CLI 和 MCP 中使用同值 TypeID；界面默认以 `namespace.key@sequence` 提供可读地址。
- revision sequence 只在单个资产内递增，不能作为跨资产主键；核心对象不得仅使用数据库自增 ID。
- `Release` 是一次发布批次及其不可变 manifest，`ReleaseManifestEntry` 固定具体资产 revision、实现对象和兼容性结论。
- `EnvironmentDeployment` 表达环境当前指向的 release，不修改 `AssetRevision` 或发布历史。
- `released` revision 是唯一允许生产消费者默认使用的内容；候选内容属于 `ChangeSet`、`Proposal` 或审核中的候选快照。
- `deprecated` 必须声明替代资产、迁移窗口和受影响消费者。
- 消费者通过 `ConsumerBinding` 固定资产 revision 或兼容版本范围，所有解析结果返回 asset、revision、release、deployment 和证据引用。

### 4.7 规范对象边界

语义资产采用写模型与读模型分离。规范对象保持独立身份和不可变边界，Web 详情使用投影组合对象，不把整个详情作为单条可变记录持久化。

```text
Workspace
└── Domain
    └── SemanticAsset                  稳定身份
        ├── AssetRevision[]            不可变语义快照
        │   ├── CommonDefinition
        │   ├── TypeSpecificSpec
        │   ├── SemanticRelationRef[]
        │   ├── MemberBindingRef[]
        │   ├── PhysicalBindingRef[]
        │   ├── JoinContractRef[]
        │   └── PolicyRef[]
        ├── ChangeSet / Proposal       未发布修改
        └── OwnershipAssignment[]

EvidenceArtifact + ClaimEvidenceLink
ValidationSpec + ValidationRun
Release + ReleaseManifestEntry
EnvironmentDeployment
Consumer + ConsumerBinding
UsageEvent + QualitySnapshot + AttentionItem
```

| 对象 | 规范职责 | 不承载的内容 |
| --- | --- | --- |
| `SemanticAsset` | `asset_id`、namespace、稳定 key、资产类型、语义域和生命周期 | 可变定义、表达式、当前 release 和健康结论 |
| `AssetRevision` | 名称、别名、定义、边界、类型专属规格及对象引用的不可变快照 | 部署指针、动态质量状态和使用统计 |
| `ChangeSet` | 基于明确 revision 的字段级修改集合 | 已发布事实 |
| `Release` | 发布活动、签名、策略结果和 manifest | 资产版本身份 |
| `EnvironmentDeployment` | 环境到 release 的可审计指针 | revision 内容 |
| `ConsumerBinding` | 消费者、用途、环境、版本约束、已解析 revision 和迁移约束 | 使用事件聚合 |
| `QualitySnapshot` | 在明确策略、特征集和证据窗口下计算的质量结论 | 资产 revision 内容 |

资产详情读取 `SemanticAssetDetailProjection`：

```text
identity
+ selected_revision
+ deployment_context
+ readiness_evaluation
+ relation_and_implementation_summary
+ claim_and_evidence_coverage
+ validation_runs
+ consumer_bindings
```

### 4.8 状态正交性

资产状态由多个正交维度组成，不使用一个枚举同时表达工作流、发布和健康：

| 维度 | 典型状态 | 所属对象 |
| --- | --- | --- |
| `workflow_state` | `draft`、`proposed`、`in_review`、`released` | ChangeSet 或候选 revision |
| `lifecycle_state` | `active`、`deprecated`、`retired` | SemanticAsset |
| `deployment_state` | `unreleased`、`staging`、`production` | EnvironmentDeployment |
| `health_state` | `healthy`、`warning`、`blocked` | QualitySnapshot |
| `compatibility_state` | `compatible`、`conditional`、`breaking`、`not_evaluated` | ReleaseManifestEntry 或 ConsumerBinding |

“需关注”是 `QualitySnapshot` 的展示结论，不是资产生命周期或发布状态。目录可以组合显示发布与健康状态，但筛选条件和 API 字段保持分离。

### 4.9 类型专属规格

`AssetRevision` 使用有判别字段的 `TypeSpecificSpec`，所有对象自身保存定义、名称、边界和别名，专业字段按对象和成员能力区分：

| 结构 | 关键规格 |
| --- | --- |
| 业务对象 | 身份键、粒度、生命周期、属性、业务关系和隐私分类 |
| 属性成员 | 稳定成员 ID、类型、值域、编码、空值、历史策略、时间粒度和实现引用 |
| 业务口径 | 术语边界、消歧、可选的适用对象、布尔条件、参数、依赖与有效范围 |
| 基础指标 | 属性输入、聚合函数、去重身份、过滤、空值、时间、单位和可加性 |
| 派生指标 | 指标依赖、公式、顺序、窗口、同范围约束、零分母和汇总重算 |
| 数据资产 | 来源与字段成员、用途、覆盖、粒度、时间、规则、加工实现和质量 |
| 分析模型 | 场景、对象、公开成员、口径、默认时间、数据资产组合、绑定和获准连接 |

成员固定所属资产修订与成员 ID。删改含义或类型时追踪下游，改名不生成新身份，也不使绑定免于重验。口径定义与判定同版本审核。行级属性、对象粒度派生属性、聚合条件和聚合后计算严格区分，不把所有条件拼成 WHERE。

表达式保留可读形式和规范中间表示。适配器负责编译，SQL 字符串不作为稳定语义身份；物理绑定、粒度、键和连接独立记录，由模型按版本引用。

### 4.10 字段主张、证据与验证

资产 revision 中需要治理的事实表示为字段级 `Claim`。证据通过 `ClaimEvidenceLink` 支持或反驳主张，证据材料本身保持不可变和可复用：

```yaml
claim:
  claim_id: clm_net_revenue_expression_v12
  asset_revision: metric_01J4NETREVENUE8W4Q9D7K2@12
  field_path: spec.expression
  value_digest: sha256:...

evidence_link:
  evidence_id: evd_cube_compile_2041
  polarity: supports
  strength: corroborating
```

`EvidenceArtifact` 至少包含来源 revision、权威类型、观察时间、适用范围、内容摘要、敏感级别和 digest。`ValidationSpec` 定义可重复执行的检查，`ValidationRun` 固定 validator 版本、输入 digest、环境、结果和输出证据。发布门禁读取验证运行和证据窗口，不回写 `AssetRevision`。

### 4.11 规范版本与扩展

- 所有可交换对象包含 `spec_version`，规范序列化使用确定性字段顺序和内容摘要。
- 核心 schema 保持受控，连接器和生态插件通过命名空间 `facets` 扩展，不向核心对象追加来源专属字段。
- 扩展 facet 声明不可变 schema URL、生产者和版本；未知 facet 可以透传，不能改变核心发布语义。
- REST、MCP、CLI、SDK、Git 文件和 release manifest 使用同一规范对象与兼容规则。
- breaking schema change 需要显式迁移和新规范版本，不能由适配器静默解释。

### 4.12 LLM Wiki 与本体投影

LLM Wiki 与本体不是两套资产，也不对应两个独立仓库。它们是同一个 `SemanticAsset + AssetRevision` 的两种受版本约束的读模型：

| 投影 | 主要用户 | 核心内容 | 生产约束 |
| --- | --- | --- | --- |
| `LLMWikiContext` | 业务用户、治理者与 LLM | 权威摘要、检索词、别名、消歧规则、正反例、典型问题和字段证据 | 只能由明确资产 revision 编译，不能由 Prompt 或会话自由改写 |
| `OntologyContext` | 解析器、Agent、开发者与治理者 | 领域路径、概念层级、类型化关系、RelationType 约束、本体一致性和 release 范围 | 所有节点与边必须指向稳定资产和不可变 revision |

资产详情默认是一张权威 Wiki 页面。“定义”展示 LLM 可用上下文和类型专属契约；“本体关系”把概念层级、语义关系、依赖与影响拆成三个明确视角，并用可审计关系列表展示 predicate、方向、断言状态、证据和 release；“实现”展示 PhysicalBinding、粒度、JoinContract 和执行适配器。图用于理解局部路径，结构化列表和 RelationType contract 仍是审核与 API 的事实来源。

本体关系图默认只加载当前资产的一跳邻域，避免把完整知识图谱压缩为不可读画布。用户按需切换关系平面、扩展路径或进入影响分析；关系的方向、类型、断言状态、证据、约束和发布范围不得仅依赖图形位置或颜色表达。关系修订进入 proposal，不在图中直接改变权威事实。

`OntologyRevision` 与单个 `AssetRevision` 独立演进。资产详情必须同时标明当前资产 revision 和本体 revision，提供相邻本体 revision 的关系与约束差异，不能用资产序号推导本体版本；release manifest 负责固定二者的组合。

## 5. 可发布就绪度

Semlia 不使用无法解释的综合“可信分”。资产就绪度由可复核门禁组成：

| 门禁 | 通过条件 |
| --- | --- |
| 身份 | 稳定 ID、key、类型和语义域有效 |
| 责任 | owner 和必要 maintainer 已明确 |
| 定义 | 定义、边界和类型专属字段完整 |
| 关系 | 必要依赖、业务对象和属性关系无未解析冲突 |
| 映射 | 执行依赖可解析到已验证绑定与粒度；涉及连接时具备 JoinContract |
| 证据 | 发布字段拥有满足策略的权威证据 |
| 验证 | 必要验证通过且结果可复现 |
| 兼容 | 受影响消费者已识别，迁移策略满足发布政策 |

每个门禁只有“通过、需关注、不适用”三种状态，并提供原因和下一步。目录排序可以使用状态、业务影响和等待时间，不用黑盒分数替代负责人判断。

## 6. 生命周期

```text
discovered -> draft -> proposed -> validating -> in_review -> released
                                                        |          |
                                                        v          v
                                                     rejected   deprecated -> retired
```

- `discovered` 是观察结果，不代表可信资产。
- 草稿与 AI 输出通过结构化变更事项进入治理。
- 用户只修订知识，不编辑 SourceRevision、EvidenceArtifact 或已发布 AssetRevision。资产标题区常驻知识修订入口，并先展示可修订的结构化字段、关联 Claim 与证据状态；问答反馈、权威语义页字段和字段级 Claim 都定位到同一个知识修订工作台。
- 知识修订以当前发布 revision 为基线，保存结构化候选值、修订原因、证据引用、验证结果和消费影响，并在提交后形成候选资产 revision。
- 没有实质变化的来源扫描不会生成待办。
- 发布后的修正必须创建新 revision 和新 release，不能原地修改。
- 消费失败、回滚和知识缺口形成 Attention Item，再进入新的变更事项。

## 7. 产品信息架构

### 7.1 知识目录

知识目录使用一张资产中心清单查找和比较可独立治理的知识对象：

- 按名称、key、别名和语义域搜索。
- 按对象类型、状态、owner 和语义域筛选；类型筛选是普通过滤条件，不形成独立页面或模式切换。
- 语义资产是默认主行；已持久化语义关系、PhysicalBinding 和 JoinContract 作为所属资产的可展开子对象展示。
- 搜索覆盖资产和子对象；命中子对象时自动展开所属资产。选择治理对象类型时仍使用同一张列表，不形成页面模式。
- 资产主行展示名称、稳定 ID、资产类型、语义域、关系与实现摘要、owner、就绪门禁和发布状态。
- 子对象展示对象 ID、关系或实现范围及其专属状态。资产发布状态、关系发布状态、实现验证状态和契约状态不得混为一个通用状态。
- 不在目录中展示综合可信分。
- 目录与详情采用两级工作面：目录负责查找和比较，选择资产后进入独立详情页。
- 返回目录时保留搜索、筛选、排序和浏览位置，不在紧凑桌面上把长目录与完整详情纵向堆叠。
- 左侧 contextual panel 按最近访问顺序显示资产快捷入口，选择后直接打开对应资产详情。

资产详情使用六个任务视图，首屏先给出生产判断和专业视图入口，再按任务展开权威信息：

| Tab | 主要内容 |
| --- | --- |
| 概览 | 当前 revision 的核心语义摘要、发布上下文、阻断优先的生产判断和专业视图入口；不重复专业明细 |
| 定义 | LLM Wiki 上下文、规范定义、类型专属语义规格、口径边界、示例、公式、时间与单位 |
| 本体关系 | 领域路径、本体 revision、一跳局部图、概念层级、语义关系、依赖与影响、断言状态、RelationType 约束和一致性结论 |
| 实现 | PhysicalBinding、来源 revision、表达式、粒度、时间字段、JoinContract 和执行适配器 |
| 可信度 | 字段主张与证据、权威类型、来源 revision、验证运行、适用策略、冲突和门禁结论 |
| 交付与影响 | 稳定交付地址、部署指针、release、消费者 Binding、版本约束、解析活动、兼容性、废弃与迁移计划 |

五类主要知识共享详情框架，成员通过所属对象定位：

| 对象 | 定义重点 | 实现重点 |
| --- | --- | --- |
| 业务对象 | 自身含义、身份、属性及关系 | 对象键、属性映射、粒度与时间 |
| 业务口径 | 独立术语、边界、判定条件和参数 | 依赖解析、规则求值及适用场景 |
| 指标 | 基础/派生类型、聚合/公式、单位和时间 | 输入映射、聚合阶段、同范围计算和安全连接 |
| 数据资产 | 用途、字段含义、覆盖、粒度与质量 | 来源成员、必要加工、过滤与验证 |
| 分析模型 | 场景、公开成员、粒度和默认时间 | 数据资产组合、成员绑定和获准连接 |

门禁按能力校验，解释可用不等于执行可用：

| 对象/能力 | 阻断条件 | 合理空状态 |
| --- | --- | --- |
| 对象定义 | 身份含义、成员归属或必要定义冲突 | 尚无物理实现时仅可解释 |
| 属性执行 | 类型、空值、历史语义、粒度或映射不完整 | 平面属性没有层级 |
| 口径解释 | 定义、范围、责任或必要消歧缺失 | 描述性术语没有判定规则 |
| 口径判定 | 对象、依赖、参数不完整或规则与定义冲突 | 查询时求值无需名单物化 |
| 基础指标 | 输入、聚合、过滤、单位、时间或绑定无效 | 无需跨资产连接 |
| 派生指标 | 依赖循环、范围/单位冲突、分母或汇总语义缺失 | 观测指标没有目标值 |
| 数据资产执行 | 核心含义或粒度未知、来源无权、必要加工未实现 | 原始来源没有上游加工 |
| 模型执行 | 成员不能解析、连接不安全、范围/版本不兼容 | 单一资产无需 Join |

仅用于解释的口径隐藏空实现页；声称可执行而缺少必要实现时显示阻断，不能隐藏风险。所有类型保留证据、责任、不可变修订和权限约束，属性与计算子类型的校验不能因一级分类简化而省略。

资产详情读取面向前端的 `SemanticAssetDetailProjection`，组合稳定身份、选中 revision、release 上下文、类型专属规格、实现摘要、证据摘要、消费摘要和可用历史 revision。默认打开当前稳定 revision，标题区始终同时显示稳定 key、revision 和 release；历史 revision 为只读，候选 revision 进入变更与发布域处理。概览不重复各专业视图的完整内容，也不展示综合可信分，只给出当前执行契约、是否存在阻断门禁以及责任和生效上下文。

### 7.2 目录中的关系与物理实现

关系与物理实现是可寻址的治理对象，但在默认浏览层级中归属于语义资产：

- **语义关系**：目录结果展示关系 ID、两端资产、关系类型、owner、release 和发布状态；打开后进入所属资产的“本体关系”，并定位该关系。
- **PhysicalBinding**：目录结果展示来源对象、目标资产、来源 revision、owner 和验证状态；打开后进入所属资产的“实现”，并定位该绑定。
- **JoinContract**：目录结果展示两端对象、基数、owner、release 和契约状态；打开后进入所属资产的“实现”，并定位该契约。

临时推断关系、JoinObservation、运行日志和未持久化候选不进入知识目录。关系详情以结构化列表为默认决策界面；关系图只在路径分析或影响分析任务中按需展开。

## 8. 核心用户流程

### 8.1 从发现到资产

```text
SourceRevision
-> Physical Objects and Knowledge Blocks
-> field-level Evidence
-> Semantic Asset candidate
-> Change Item
-> Validation and Review
-> AssetRevision in immutable Release
```

### 8.2 从资产到消费

```text
Consumer request
-> released SemanticAsset
-> typed relations
-> PhysicalBinding and JoinContract
-> validated ResolvedSemanticPlan
-> execution adapter when data is required
-> result with provenance
```

## 9. 原型设计契约

- Surface：桌面端高密度知识治理工作面，目标视口为 `1440x900` 与 `1024x768`。
- Visual thesis：资产详情是一份可执行的知识档案，不是指标 dashboard。
- Signature move：从资产稳定身份和生产可用判断出发，六个局部视图使用同一条选中线切换“概览、定义、本体关系、实现、可信度、交付与影响”。
- Layout：目录与详情采用稳定分栏；紧凑桌面改为上下布局，不产生横向溢出。
- Components：搜索、类型与状态菜单、资产列表、详情 Tabs、门禁清单、关系图、绑定表和证据列表。
- States：已发布、需关注、草稿、筛选为空、无证据、无绑定、映射异常和键盘焦点。
- Mandatory：明确 stable ID、revision、release、owner、门禁原因、物理绑定和消费者。
- Forbidden：综合可信分、把物理表直接标成已发布语义、把本体做成独立资产仓库、把知识块做成必经页面、卡片嵌套卡片。

## 10. 行业互操作原则

- 执行适配器保持属性、聚合、判定、对象身份和 Join 的类型差异，可映射到 Cube 等运行时构件；运行时的度量、维度、分群枚举不决定产品目录分类，也不替代 Semlia 稳定身份。
- 面向消费者的公开表面只暴露经过策展的资产与明确 Join 路径，遵循 [Cube Views](https://docs.cube.dev/docs/data-modeling/views) 所体现的消费门面原则。
- 业务口径中的术语关系采用 broader、narrower、related 等可映射语义，保持与 [W3C SKOS](https://www.w3.org/TR/skos-reference/skos.html) 的互操作可能，但首发不依赖 RDF 或独立图数据库。
- 来源、活动、主体与派生关系保持可交换的 provenance 结构，参考 [W3C PROV-O](https://www.w3.org/TR/prov-o/)；Semlia 领域对象仍使用自身稳定 ID、revision 和 release 规则。

## 11. 验收规则

1. 用户能够用稳定名称和类型找到资产，并在一个详情中回答“是什么、在本体中处于哪里、如何实现、为何可信、如何交付、影响谁”。
2. 本体关系和物理实现使用独立视图但能从同一资产互相追溯，不混淆业务关系与生产 JoinContract。
3. 目录不展示不可解释的综合可信分。
4. 未发布资产、推断证据和 Join 观察不会被表现为可生产消费事实。
5. 从问答引用、变更事项、构建运行和消费反馈进入资产时保留正确的局部上下文。
6. `1440x900` 与 `1024x768` 下列表、Tabs、关系图和映射内容没有遮挡或横向溢出。

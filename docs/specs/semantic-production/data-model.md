# 语义生产数据模型

| 元数据 | 值 |
| --- | --- |
| Version | 0.1.0 |
| Status | Planned Contract，供 SP-T002 至 SP-T007 实施与评审；不是已上线数据库或 API |
| Scope | [生产设计](product-design.md)、[计划](plan.md) SP-TD-001 至 SP-TD-008 |
| Wire contract | [生产接口](contracts/production.md)、[OpenAPI](contracts/production.openapi.yaml) |

## 1. 权威与复用

`production_business_rule_events` 记录 workspace、operation/version/targetKey、set/content/evidence 摘要、真实人类 principal、authorization version、幂等请求摘要及时间。记录不可变、按 sequence 追加；最新 confirm/revoke 事件决定当前确认，当前权限独立复核。仅持久化摘要和证据引用，不复制语义正文。每版本最多 256 个事件，跨版本不继承确认；有历史事件的降级返回 `DOWN_MIGRATION_UNSAFE`。完整约束见 [业务规则确认](contracts/business-rule-confirmation.md)。

系统采用现有 PostgreSQL 模块化单体。来源、工件、发现运行、物理对象 revision、候选、Proposal、Validation、Review、Release、审计和 outbox 保持各自权威。生产操作只协调固定输入、目标、命令幂等及结果定位；不增加审批状态机、语义内容副本或消费者前置条件。

当前代码依据如下，全部以工作区实际内容为准：

| 依据 | 实施约束 |
| --- | --- |
| `internal/adapters/postgres/discovery.go:465`、`:521` | dataset/field 内容未变时复用 revision；历史成员必须独立记录，不能以 revision 创建时间归属推断扫描成员 |
| `db/queries/discovery.sql` | 身份行名称可更新；历史名称必须来自不可变快照 |
| `migrations/000003_semantic_registry_foundation.up.sql:247` | semantic_assets 允许 `current_revision_id IS NULL`，可持有未发布稳定身份 |
| `migrations/000006_m2_governed_authoring.up.sql:65` | `target_object_id NOT NULL`；asset/base 两列本身可空，但 `proposals_target_shape` 强制 semantic_asset 有真实 base revision |
| `migrations/000007_m2_governance_objects.up.sql:165`、`internal/domain/governance/object.go` | 实际支持五类 target：semantic_asset、physical_binding、model_grain、entity_key、join_contract；绑定是独立治理对象 |
| `internal/application/governance/proposal.go`、`authoring.go` | create/submit、100 changes、不可变提交和已有 Proposal 状态转换复用，补 intent 分支而非伪造基线 |
| `internal/adapters/postgres/release_publishing.go:261`、`:597` | 现有单提案发布和回滚为事务入口；生产集合扩展同一存储边界 |
| `internal/domain/governance/release.go`、`canonical.go` | 旧 manifest digest 算法和 1 MiB canonical JSON 限额保持原样 |

以下表名和新增列是精确的规划存储契约，不是本任务创建的迁移。数据库 UUIDv7、workspace 复合外键、TypeID wire 编码、UTC 时间、`ON DELETE RESTRICT` 及既有错误/审计规范沿用。新资源前缀预留 `ssnp`（来源快照）和 `prodop`（生产操作）；不得把存储 UUID 暴露到 API。

## 2. 来源快照

### 2.1 表与键

| 表 | 字段与约束 |
| --- | --- |
| `source_snapshots` | `id, workspace_id, source_connection_id, source_revision_id, adapter_version, scope_digest, content_digest, history_quality, coverage_status, created_at`；`history_quality IN (verified,unverifiable)`；`coverage_status IN (complete,partial,failed)`；快照及内容 immutable |
| `source_snapshot_runs` | `(workspace_id, run_id)` 唯一，FK 到现有 discovery run 与 snapshot；一次新扫描可关联复用的相同快照，不丢本次运行时间/诊断 |
| `source_snapshot_scope` | `(workspace_id,snapshot_id,coverage_key)` PK；固定 selector、适配器配置摘要、`status`、`enumeration_complete` 和诊断 codes；不包含凭据 |
| `source_snapshot_members` | `(workspace_id,snapshot_id,kind,object_id)` PK；`revision_id, historical_name, historical_locator, content_digest, coverage_key, parent_object_id NULL, parent_revision_id NULL`；kind 为 dataset/field/code/lineage；FK 指向本次快照范围与真实版本 |
| `source_snapshot_diagnostics` | `(workspace_id,snapshot_id,ordinal)` PK；`code,severity,coverage_key,locator,message`，无敏感原始错误；运行特有诊断同时留在 run，不覆写快照 |
| `source_code_revisions` | `id (TypeID codrev),workspace_id,code_artifact_id,source_revision_id,adapter_version,path,language,blob_oid,content_digest,created_at`；append-only；`UNIQUE(workspace_id,code_artifact_id,adapter_version,content_digest)`，digest 包含路径/语言/可复核内容摘要 |
| `source_lineage_revisions` | `id (TypeID linrev),workspace_id,lineage_edge_id,source_revision_id,adapter_version,upstream_object_id,upstream_revision_id,downstream_object_id,downstream_revision_id,edge_kind,code_revision_id NULL,confidence,content_digest,created_at`；append-only；`UNIQUE(workspace_id,lineage_edge_id,adapter_version,content_digest)` |
| `source_effective_snapshots` | `(workspace_id,source_connection_id,scope_digest)` 唯一；仅指向 verified + complete 快照；版本 CAS 和来源锁保证原子切换 |
| `source_coverage_heads` | `(workspace_id,source_connection_id,coverage_key,selector_digest)` PK；`latest_attempt_run_id,latest_attempt_status,latest_verified_snapshot_id NULL,version`；每次声明覆盖的尝试都 CAS 更新 attempt，只有该 unit verified+complete 才更新 verified pointer；partial 全局快照中的 complete unit 可独立推进 |

dataset/field 的 revision 必须属于指定 object；field 的 parent 是本快照内 dataset 及其 revision。`code_artifacts` 的 Upsert 会修改 blob/language/digest，`lineage_edges` 的唯一性不含 adapter，二者均不能充当历史 revision。固定使用上述两个 append-only revision 表及新增 codrev/linrev TypeID；旧行只提供兼容 identity/当前投影。代码 blob 必须内容寻址且保留可复核字节，不跟随可变文件路径读取；无法恢复旧字节则该历史 unverifiable。lineage 两端和代码依据均引用本快照精确 revision，不足时保留未解析诊断并阻断依赖它的生产。

`UNIQUE(workspace_id, source_connection_id, source_revision_id, adapter_version, scope_digest, content_digest)` 保证相同投影重放复用。`content_digest` 包含版本化 schema、adapter、完整范围/覆盖、按 `(kind, object_id)` 排序的成员（包括名称、定位、revision、父引用）与规范化诊断；不包含 run ID、凭据、时间戳。范围 selector 规范化后排序，不能把不同范围的相同成员误当同一快照。

### 2.2 历史与覆盖

1. 每个 observed member 均进入快照，包括复用旧 revision 的未变化表/字段。改名保留同一可证明 identity 时，保存本次名称；若外部 stable key 改变，只能记录 possible rename，不能凭相似名称合并身份。
2. `complete` 表示声明范围内枚举及必需成员元数据都完成，不表示整库；范围本身始终可见。只有两个覆盖相同范围且 `enumeration_complete=true` 的可信快照之间，缺席成员才能判定为删除。
3. `partial` 保存真实已观察成员和逐范围缺口，不继承上次成员来假装本次观察。`failed` 保存诊断与确实得到的有限成员，不切换有效指针。缩小范围不等于删除范围外对象。
4. 生产可选择 partial 快照内单独 `complete` 的 coverage unit，但必须显式固定 coverage keys；所有直接/传递依赖都在选定完整范围或已发布 pin 内。选择失败/部分 unit 返回 `INPUT_INCOMPLETE`，不能由客户端 `allowPartial` 绕过。快照仍显示 partial；不将局部可用标为全量成功。
5. 历史迁移只根据可信不可变记录构建 verified 快照。任何缺失成员集合、旧名称、范围或覆盖的运行标为 `unverifiable`，成员列表返回已证明的记录及 `historyQuality`，不回填当前值。新扫描建立新的可信版本，不修饰过去。
6. 来源新快照发布后，受影响的输入判 stale；未变化且依赖覆盖、成员版本/名称/证据摘要均一致的输入可通过确定性影响复核获得新的 validation 结果。原结果从不改写或直接沿用为新输入通过。

生产 freshness 按 source_coverage_heads 逐选定 unit 检查，不只看全局 effective 指针；最新尝试在必要 unit 失败/不完整即不能用旧缓存推导“当前已检查”。同一操作对同 source 的同 coverage unit 只能选择一个快照；重叠范围/同 object 的互相矛盾 revision 拒绝，历史对比不是同一生产输入的两个权威版本。完整成员定义包含父 dataset revision，因此父 revision 改变时 field 可能需要新 revision 以满足现有 parent 约束；不强制复用错误父版本。

读取索引：snapshot `(workspace_id,source_connection_id,created_at DESC,id DESC)`；member `(workspace_id,snapshot_id,kind,object_id)`；diagnostic 按 ordinal。列表最多 200，不加载全库到浏览器。

## 3. 生产操作与命令

| 表 | 字段与约束 |
| --- | --- |
| `production_operations` | `id,workspace_id,created_by,current_version,created_at,updated_at,supersedes_operation_id NULL`；不存 approved/released 等独立审批状态 |
| `production_versions` | `(workspace_id,operation_id,version)` PK；`input_json,input_digest,request_digest,set_digest,frozen_at NULL,created_by,created_at`；每个版本的输入及内容 append-only，冻结标志只能由 submit CAS 一次设置 |
| `production_targets` | `(workspace_id,operation_id,version,local_key)` PK；`kind,intent,target_id,identity_key NULL,base_revision_id NULL,base_object_version NULL,registry_write_version NULL,content_json,content_digest,proposal_id NULL,outcome`；每版本 target identity 唯一；outcome 为 proposal/no_change |
| `production_candidate_links` | `(workspace_id,candidate_id,candidate_digest,operation_id,version,local_key)` 唯一；`decision_id,is_primary`；FK 至实际候选/目标/既有 candidate decision；每 candidate/version 恰好一个 primary；不解析 summary 判断关联 |
| `production_contributors` | `(workspace_id,operation_id,principal_id)` 唯一；记录作者、实质编辑者、生成发起人及 agent 作者的贡献角色；不可删以规避 SoD |
| `production_identity_reservations` | `(workspace_id,kind,identity_key)` 唯一；`target_id,creation_operation_id,owner_operation_id,created_at`；creation_operation_id 不可变，owner_operation_id 是当前受权操作的 CAS claim；target ID 不可重新分配；保留拒绝/回滚后的身份 |
| `production_commands` | `(workspace_id,principal_id,command_kind,idempotency_key)` 唯一；`request_digest,operation_id,operation_version,result_kind,result_id,committed_at`；与业务写入同事务。无靠 TTL 后失忆而重建的窗口 |
| `production_request_claims` | `(workspace_id,business_digest)` 唯一；`operation_id,created_at`；business_digest 固定规范化 input、声明/更新目标、真实 published bases 和 supersedes，排除 caller、幂等键、trace 与分配 ID；防不同键且无候选的同输入更新重复造 Proposal |
| `production_generation_links` | `(workspace_id,operation_id,agent_run_id)` 唯一；`input_version,input_digest,model_config_revision,output_digest,provider_mode`；引用现有 AgentRun/Step；人工应用由独立 immutable 关联记录，不储存隐藏推理 |
| `production_generation_outputs` | `(workspace_id,agent_run_id)` PK/FK 至 AgentRun 与 generation link；`operation_id,input_version,schema_version,canonical_output bytea,output_digest,canonical_bytes,created_at`；正文为符合 StructuredGenerationOutput 的规范 UTF-8 JSON 字节，canonical_bytes=octet_length(canonical_output) 且 1..1048576；append-only，无 prompt/raw_provider/hidden_reasoning 列 |
| `production_generation_applications` | `(workspace_id,operation_id,applied_version)` PK；`agent_run_id,source_version,source_output_digest,applied_content_digest,canonical_delta bytea,delta_digest,actor_principal_id,created_at`；FK 至输出、来源版本及应用后 immutable version；delta 为服务器计算的有界规范 JSON 差异，append-only |

命令用途包括 create、replace_draft、submit、validate、review、publish、generate、rollback；命令摘要含目标资源 ID，不能用同键跨资源重放。幂等主体是当前认证 principal，不是请求 body 声称的作者。request 中 unknown keys、重复 JSON object keys、非法 UTF-8、非有限/越界数字及尾随 JSON 拒绝；体积为 min(1,048,576 bytes, 现有端点/代理更小限制)，模型输入/输出同限，嵌套深度最多 32。

规范化使用版本 `semlia.production/v1`：TypeID 严格 parse 后规范输出；enum 原样；标题 trim；业务定义/表达式不随意 trim/改写；Unicode 文本原样；不将 `1` 和 `1.0` 擅自视为相同。键递归排序与数字字面量保留复用 `CanonicalJSON`。无序集合（targets 按 localKey、snapshots 按 sourceId/snapshotId、coverage keys、evidence IDs、candidate IDs、dependency pins）排序并拒绝重复；changes 为执行顺序，保留数组次序并拒绝重叠/重复 field paths。摘要排除幂等键、trace、服务端时间及派生 ID；包含 supersedes、期望版本、来源、配置引用和所有业务内容。服务器计算权威摘要，客户端摘要只作一致性断言。

最多 32 targets，每 target 0..100 changes，总共最多 256；服务端在 schema 解析后检查 aggregate，OpenAPI 无法单独表达跨数组求和。create 是真实创建声明，不要求合成 `add /`；其 content 是全部初始草稿，changes 必须空。update 的完整 desired content 与 changes 重放结果必须一致；服务器生成并核验 before/after digest，拒绝客户端伪造旧值。规范化结果等于基线时 outcome=no_change、proposal_id=NULL。若全部 no_change，操作仍可查询，但不能 submit/publish，不生成 Proposal 或 release。

同键同内容重放先重新检查当前 workspace membership、所有源/证据/目标权限，再返回原结果定位和 `replayed=true`；权限撤销返回 403，不回显缓存内容。同键不同内容 409。另一个 principal 不能用同键取到作者结果；有范围权限者可在操作列表恢复合法可见对象，不能借此复用他人命令身份。不同键相同 candidate/create identity 同样由唯一约束串行处理：同规范输入且有权读取时返回既有操作链接 `ALREADY_PRODUCED`，内容不同则 `IDENTITY_CONFLICT`；不能自动选择无关现有资产。

create operation 还在同事务写 production_request_claims，使相同规范业务输入的 update（即使没有 candidate，或另一主体用不同 key）只能创建一套 Proposal；返回 ALREADY_PRODUCED 前仍重新授权完整已有操作。不得用 dedup 把另一主体变为原作者。显式 supersede 的 digest 包含前序操作 ID，允许必要纠正/重验证的新尝试，同时保留前次失败事实；重复的同一 successor 仍唯一。

同步 create/replace/publish 的业务结果不使用会提前提交的 running 行；事务 commit 前崩溃没有结果，重试安全；commit 后丢响应可按同键或列表找回。异步 validate/generate 在短事务内写既有 job 与命令结果，然后 worker 在事务外执行；超时不确定的模型调用标为 `outcome_unknown`，不自动付费重试。客户端恢复缓存仅作便利，不承担权威。

## 4. 新建与更新

### 4.1 新建身份

create 的根字段不接收 targetId、baseRevisionId 或 baseObjectVersion。普通首次 create 不带 reuseIdentity，服务端在同一事务中检查当前授权、锁定自然身份、分配五种现有 target TypeID、写 reservation。semantic_asset 同时只插入 `lifecycle_state=draft,current_revision_id=NULL` 的身份行，不写 asset_revision、不写 active、不造 release。其他四类对象不插入不完整正式行，草稿内容保存在 Proposal 关联声明；发布事务才插入完整治理对象行。已发布后被可信回滚移除的身份使用 §4.4 的 create+reuseIdentity，不占用新的业务身份。

`identityKey` 对 semantic_asset 必须等于 namespace.key 的 address；对其余对象是工作区内该 kind 的稳定声明键，并同时检查既有领域唯一性（例如 binding 的 asset/dataset/field、实体 key、grain 的约束）。不能只靠自由文本 identityKey 规避已有对象匹配。相互引用以本版本 localKey 解析，解析后持久化稳定 target ID；只允许本集合 create 声明及固定已发布 dependency pin，不允许任意外部草稿。

同事务创建原有 `Proposal(state=draft,intent=create)`、完整初始内容、candidate decision 和关联行、幂等结果、审计/outbox。candidate 可以关联多个相关产物，但同一 candidate digest + 产物身份不得重复创建。模型建议只是草稿，definition/scope 暂缺时用显式 null 及 unresolved 结果表示，不用假文本表示业务事实；submit 的确定性验证阻止缺依据/未决业务规则发布。

每个 CandidateSelection 显式包含 `primaryTargetKey`，且必须属于 targetKeys。primary 对应一个实际 changed/create Proposal，禁止任取数组第一项。`semantic_candidate_decisions.proposal_id` 写 primary Proposal，`semantic_candidates.proposal_id` 是最新该 immutable decision 的兼容投影；完整关联以 production_candidate_links 为准。全 no_change 不创建 convert decision、不改变候选决定；混合集合若某候选的 primary 为 no_change，返回 `CONTENT_MISMATCH`，由调用者明确选择实际变化的 primary 或不转换该候选。supersede 写新的 immutable decision（关联前一 decision ID）并 CAS 更新候选当前 singular 投影，绝不修改旧 decision；旧链接持续定位原版本。旧 convert 命令遇到 production 已关联候选返回 `PRODUCTION_SET_REQUIRED`，不能覆盖多目标关联。

### 4.2 Proposal 的 expand-first 约束

追加迁移为 proposals 增加 `intent NOT NULL DEFAULT 'update'`、`creation_content jsonb NULL`、`base_object_version integer NULL`、`production_operation_id NULL`、`production_version integer NULL`、`reintroduction_creation_release_id NULL`、`reintroduction_absence_release_id NULL` 和冻结摘要关联。保持 `target_object_id NOT NULL`，绝不先将其放空。增加 workspace 复合 FK 及延迟约束触发器，目标必须是同集合 reservation 或真实对象。两个 reintroduction 字段同时有值或同时为 NULL，只允许生产 create 携带。

替换 `proposals_target_shape` 的精确分支如下；原有 semantic_asset 的 asset/base 两列并非列级 NOT NULL，但 check 等效要求它们非空：

| intent / target | 强制条件 |
| --- | --- |
| create / semantic_asset | asset_id=target_object_id，asset_id 非空；base_revision_id/base_object_version 为 NULL；creation_content 为 object；有效 reservation；asset 当前无发布 pin；历史已发布身份须有可信 reintroduction 关联 |
| create / governed object | asset_id/base_revision_id/base_object_version 为 NULL；creation_content 为 object；有效 reservation；首次目标正式对象不存在，或满足 §4.4 的可信缺席重引入且固定 registry_write_version；不得仅凭已有 row 伪造 baseline |
| update / semantic_asset | asset_id=target_object_id，asset_id/base_revision_id 非空；base revision 属同 workspace/asset；仅生产成员要求其为冻结 baseline release 中精确 pin，非生产 legacy update 保留真实草稿 revision 作为 base 的旧合法路径；creation_content/base_object_version 为 NULL |
| update / governed object | asset_id/base_revision_id/creation_content 为 NULL；生产成员必须有正 base_object_version 且匹配冻结 published object snapshot；非生产历史 update 保留旧路径，无凭据不得猜填 version |

旧调用默认 update，既有调用者仍需原有 base；这不是向旧接口开放 create。新约束先 `NOT VALID` 添加并校验已迁移行，替换旧 check 在单次 DDL 事务完成，不留无约束窗口。新生产语义由 feature fence 控制，旧进程清空前禁用 create/生产集合写入。历史 proposal 保留原状态/内容/归因，不因无法恢复历史 base_object_version 而伪造数值。

### 4.3 编辑与状态

draft 的 PUT 为整个集合 replacement + `expectedVersion` CAS；新建 version 保留旧 input/content/actor；未提交原 Proposal 的 change set 可按现有 draft 规则更新，删除的成员 Proposal 按既有合法拒绝路径终结，reserved identity 不复用给别的业务对象。新版本关联具体 proposal ID；版本历史不是当前审批事实。

submit 冻结版本，禁止成员/内容变更，沿 `draft -> proposed -> validating -> in_review -> released|rejected`，复用原验证排队与结果。内容、来源选择、依赖或业务修正发生在冻结后时，创建显式 `supersedesOperationId` 的新操作和新 Proposal；旧审批不迁移。旧集合如仍非终态，原 proposal 状态机逐成员受权拒绝，并在一个事务中转移 reservation 所有权；当前仍有 published pin 的后继使用 update，历史 released 但当前可信 absent 的身份使用 §4.4 的 create+reuseIdentity。不存在任意将 in_review 改回 draft 的新边。

工具失败恢复或来源无影响复核可对同一冻结输入新建完整 validation attempt；不覆写旧失败。审批入口创建原 Review 行且携带集合版本/摘要关联，无独立 collection_approval 表。集合 progress 由 Proposal 状态、完整验证集合、有效 Review 和 release 关联计算。

### 4.4 缺席身份的重引入

`intent=create` 的可选 `reuseIdentity` 精确包含 targetId、creationOperationId、creationReleaseId、absenceReleaseId 和 present expectedHead（releaseId+manifestDigest）。服务端在同一 workspace/head/reservation 锁事务验证：原 operation 确实创建该 kind+identityKey+targetId；creation release 的完整归因及 pin 可复核；absenceReleaseId 是受保护回滚链中将该 target 从 present 移为 absent 的实际事件；当前 expectedHead 等于工作区 head 且仍无该 target 的 pin。旧历史不可验证、任意未发布 legacy row、只声称 absent 或使用 absent head 均拒绝，不能借此采用他人的无关身份。

reservation 的 creation_operation_id 永不更改；owner_operation_id 在原操作无活动写入 claim 后 CAS 转移到本次生产，唯一业务身份/已有 domain uniqueness 继续检查。操作者须有同身份及全部来源/证据权限，不能用知道原 ID 替代授权。新 set digest 包含完整 reuseIdentity、当前真实 absent head、registry_write_version 和所有新 desired content；create 的两个 base 列仍为 NULL。贡献者包含本次作者/编辑者及被复用原内容的贡献者，旧批准不迁移，必须全新验证与独立审核。

发布时再次检查 current head 与 absence、reservation、registry CAS 和依赖闭合，原子恢复所有新 pins。semantic_asset 保留同一 identity，可创建新真实 revision；若 desired content digest 与已有不可变 revision 完全相同，复用该 revision，遵循现有 `(asset_id,content_digest)` 唯一约束，absence->presence 本身仍是实质变化。governed object 如已有历史 row，则以捕获的 registry_write_version CAS 更新完整新内容、分配递增版本并恢复适用的非 retired 状态，不插入重复主键。不存在正式 row 时才 insert。完整 before pin 仍是 absent，后续回滚可再次移除发布 pin而不删除历史。全流程只有 create/update 两意图及原 Proposal 状态机。

## 5. 固定集合与治理

`set_digest` 包含 schema version、input digest、baseline release（空工作区为显式 absent）、每个稳定 target/intent/base/content/proposal ID、依赖边、选定证据和 policy/validator registry 版本。draft creation 返回当前 set digest；submit CAS 后该集合不可变。publication 以该 digest 为唯一候选内容，禁止使用 mutable registry/current snapshot 代替依赖。

每个验证结果及审核关联 `(operation_id,version,set_digest,proposal_id,proposal_content_digest,input_digest,attempt_no)`。原 Review 行不可变；集合绑定在附加 `production_validation_bindings` / `production_review_bindings` 表中，以原 validation_run/review ID 为唯一 FK。审核绑定额外固定 validation_digest。集合一致性结果使用既有 validation run/result：每个适用成员均有关联，不添加第二审批状态机。required validators 的列表和版本冻结；failed/cancelled/running/missing/not_checked 不等于通过；not_applicable 必须具有确定性适用范围理由。

`production_validation_bindings` 同时固定 `freshness_witness_json,freshness_digest`：每个所选 coverage unit 的 head version、所比较 snapshot ID、required member/证据摘要和影响判断。freshness witness 是该 validation attempt 的输入，不覆写原 production input。发布事务锁定 coverage heads 后逐项对照 witness；head 在验证后再次变化则需要新 impact attempt，不能只接受相同 setDigest 的旧通过。即使 business content 未变且新 witness 确认无影响，新 attempt 也使所有旧审核不能用于发布，要求重新独立审核；受影响内容则拒绝并走 successor。

### 5.1 Validation Attempt 的持久化与执行

`production_validation_attempts` 的 PK 为 `(workspace_id,operation_id,production_version,attempt_no)`，attempt_no 由服务器从 1 递增，单版本最多 256。字段为 `set_digest,input_digest,freshness_witness_json,freshness_digest,required_checks_json,required_checks_digest,status,validation_digest NULL,created_by,created_at,completed_at NULL`。required_checks 是完整排序的 `(proposal_id,validator_id,validator_version)` 集合，最多 256，包含所有成员必需的单对象及集合一致性检查；声明/输入 immutable，status 只表示该执行组 queued/running/succeeded/failed，不是审批。production_versions 增加 `active_validation_attempt_no NULL`，FK 至同版本 attempt；enqueue 时即 CAS 切到新 attempt，旧成功不能自动回退为有效。

expand-first 为既有 validation_runs 增加可空 `production_operation_id,production_version,production_attempt_no`，三者全 NULL 为 legacy 或全非空为 production，复合 FK 至 attempt。将 000006 的 `UNIQUE(proposal_id,validator_id,validator_version)` 用同一 DDL 事务替换为两个索引：legacy 部分唯一键保持原三列且 `WHERE production_operation_id IS NULL`；production 部分唯一键为 `(workspace_id,production_operation_id,production_version,production_attempt_no,proposal_id,validator_id,validator_version)`，`WHERE production_operation_id IS NOT NULL`。旧行保持 NULL，旧查询显式仅匹配 NULL 分支；不能给历史行猜填 attempt。

submit 在事务内分配 attempt=1，并为该集合每个 proposal 排队既有 validation job。production payload 扩充 operationId、productionVersion、productionAttemptNo、setDigest；幂等 job key 包含 workspace/operation/version/attempt/proposal，不能沿用仅 proposal/旧 orchestration attempt 的 key。旧 payload/旧分支原样工作；旧 job 的 transport retries 与 production semantic attempt 是不同字段，重试同 job 只续跑同一 attempt 的相同 run/result，不生成新 attempt或读取别的 attempt 终态结果。`GetProposalValidationRun`、结果/政策查询以及 `EnsureDecisionForProposal` 的 production 分支必须带完整 attempt key。

`/validations` 的 previousAttemptNo 与 active pointer 做 CAS；有相同 attempt 在运行则同键返回原结果，不同键拒绝并发 enqueue。可以从 validating（初次工具失败）或 in_review 重验；production executor 在 in_review 不使用现有“只 EnsureDecision 后返回”的 shortcut，也不倒退 Proposal 为 validating。它按本 attempt 的完整 required checks 重跑全部确定性检查（包括无影响来源复核），不拼接前一次通过。初次 validating 完成后沿原规则进入 in_review；in_review 重验完成保持 in_review，发布资格仅由当前完整结果决定。terminal Proposal 不可重验。

完成器在一次事务检查该 attempt 恰好有 required checks 全集、每个 run/result 的 input/set/freshness digest 一致、全体 terminal succeeded、无 blocker 且 applicability 正确；计算 validation_digest，内容包括 operation/version/attempt、required checks、按固定键排序的全部 run/result 内容摘要、freshness 和 policy inputs。任何缺失、failed、cancelled、工具未完成或多余不匹配 run 都不能 succeeded。job 最终失败将 attempt 置 failed，部分结果只作诊断。只选择 active pointer 指向的完整 succeeded attempt；不允许按 validator 逐条取最新或挑选不同 attempt 的成功。每 check 最多 256 findings，attempt 全部 checks 的规范响应内容合计最多 1 MiB；worker 持久化前执行字节预算检查，预留有界失败诊断空间，超限记录 LIMIT_EXCEEDED 并失败而非截断成功。历史 GET 按 attempt 完整投影既有 run/results，不建立第二结果副本。

ReviewProductionRequest 和 PublishProductionRequest 必须给 `validation={attemptNo,validationDigest}`，事务内与唯一 active completed attempt 比较。production_review_bindings 持有相同 FK/digest；新 attempt enqueue 起旧 Review 即不再有效，全部成员重新审核，包括 source 无影响重验。旧不可变 Review 仍保留；现有 CountProposalReviews 的 production 分支只计当前 attempt/channel，允许独立 reviewer 对新 attempt 重新审核，legacy 分支保留原 stage 唯一性。发布时同时复核该 attempt 的完整集合、当前 freshness、原 policy 和全体绑定 Review，不能仅凭提案 state=in_review 或旧 policy decision 放行。

依赖闭合：内部局部引用必须指向同一冻结集合，并采用其 proposed content；外部引用只能是输入固定 baseline release 的完整 pin。字段必须属于固定 dataset revision；binding、grain、key 和 JoinContract 以一致字段、实体与 grain 校验。禁止悬空引用；允许图中的语义关系循环，但 resolver 必须 visited-set 有界遍历，执行型 dependency cycle 由确定性 validator 拒绝，不能无限拓扑排序或部分发布。

发布者必须是当前有权 human，不能是任意成员作者、任何已采用审核的 reviewer；审核者不能是集合中任意成员作者（包含产生内容的 agent principal、发起 generation 的人以及参与实质 draft 编辑的人），不能通过分拆成员自审。记录 contributors 集合与真实 principal ID，不信任 display name。所有成员均要求既有 policy 指定的有效独立人工审核；不开放 G2/G3 或模型自动批准。发布时重新核实 reviewer 当前 membership/作用域和必要 review capability，撤销的授权不能支撑新发布。

事务顺序固定：workspace/authorization version fence -> operation -> proposals 按 ID 排序 -> targets/reservations 按 kind/ID 排序 -> source effective pointers -> dependency pins。发布事务内重新读取当前主体授权、SoD、集合/内容摘要、来源影响复核结果、全部 base/current published pins、验证/审核、policy 及依赖闭合；authorization/grant 修改沿相同 workspace fence，撤权不得越过已读取授权快照。并发 head 或基线变化返回 409，不自动 rebase。

同事务写全体新 revisions/object rows、完整 manifest、release snapshots、归因、全体 Proposal released、catalog published 指针/投影 intent、审计和 outbox、幂等结果；任意一步失败全部回滚。外部投影由 outbox 幂等交付，读取以 committed release 为准；异步投影未 ready 明示，不宣称数据库半提交。模型/网络、消费者通知不在事务内。

旧单项 publish/submit/review/changes 入口遇到 production 成员必须 `PRODUCTION_SET_REQUIRED`，包括 draft、冻结及已释放成员；不得以省略新字段或直接 catalog status 更新绕过。旧单 proposal validation-runs 聚合读取对 production 成员同样拒绝，改用按 operation/version/attempt 分页并含完整 checks/results 投影的明确历史入口。旧 `/governance/releases/{releaseId}/rollback` 及旧 PublishingService/Store.RestoreRelease 同样拒绝所有 production release 与其任意层回滚后继，依据 §6 的持久化 protection 而不是 singular origin。普通非集合历史提案/非保护 release 继续原路径。数据库发布与回滚防线必须验证保护继承、成员归因完整与 manifest 一致；仅应用路由 guard 不足。

## 6. Release、归因与回滚

| 表/扩展 | 约束 |
| --- | --- |
| `release_proposals` | `(workspace_id,release_id,proposal_id)` PK；全体参与成员，每行固定 operation/version/set/content digest、author principal、validation attempt/digest、采用的 review IDs、role=applied/reverted；FK 及不可变触发器 |
| `releases` 保护扩展 | `production_root_release_id NULL,production_rollback_parent_id NULL,production_rollback_depth NULL`；全 NULL 仅普通 release，root=self/depth=0/parent=NULL 为生产发布，后继 root=parent.root/depth=parent.depth+1/parent=实际 rollback target；workspace FK、序列单调及 immutable 约束 |
| `production_release_manifests` | release ID 唯一；`before_release_id NULL,before_manifest_json,before_manifest_digest,after_manifest_digest,attribution_digest`；NULL before ID 表示已证明的空 manifest，不是未知历史 |
| `production_release_before_pins` | 每受影响 target 恰好一行；presence=present 时精确 asset revision 或 object snapshot version/digest；presence=absent 时这些列必须全部 NULL |
| `production_release_binding_inputs` | `(workspace_id,release_id,binding_id,binding_version)` PK；固定 `snapshot_id,dataset_revision_id` 与所需 field revision 引用；只保存来源关联，不复制第二份执行 payload |

完整 before manifest 在 publish 前 head 上读取并同事务保存，包含未受影响 pins；after manifest 是应用整个集合后的完整闭合视图，不仅是变更对象。旧 `origin_proposal_id` 在恰好一个 applied proposal 时保持该值，多提案时 NULL；额外 `originProposalIds` 返回全体 proposal，不选第一个伪造唯一作者。新受权 release detail 返回精确 provenance，目录只从 release 读取已发布权威，不复制草稿业务内容。

所有生产根 release 及每一层 rollback 都保存 protection；回滚空 manifest 或 originProposalId=NULL 不会解除保护。旧 HTTP handler、旧 PublishingService.Rollback 与 Store.RestoreRelease 在 workspace 锁内检查标记，返回 409 PRODUCTION_SET_REQUIRED，不调用旧 planRestore。新命令继承 root 与原全成员/验证/审核归因，再执行当前 R/SoD/exact-before 检查。生产根保护 FK 允许同一事务 self-reference，parent 序列必须更早且同 workspace，禁止循环或跳过继承。

数据库 BEFORE INSERT 防线检查 releases.rolled_back_to_release_id（既有字段实际指向被回滚目标）：目标带保护时 NEW 必须带正确 root/parent/depth；未适配旧调用默认 NULL，因此不能提交旧旁路。DEFERRABLE constraint trigger 在 commit 核实保护 release 有匹配的新 production_commands(kind=publish/rollback) 结果、完整 production_release_manifests/release_proposals/before pins、所绑定 attempt/Review 与完整 manifest；无任一记录即拒绝。新 repository 按同 workspace/authorization fence 复核真实主体，不把任意客户端/session 自报“已授权”当数据库票据。更新/删除保护或无归因克隆 release 均被 immutable/constraint 防线拒绝。

现有 `000021` 的 object snapshot trigger 已优先复制相同已发布版本的 immutable snapshot，应继续复用；新 binding 的 execution provenance 分支必须从 production_release_binding_inputs 的精确 source_snapshot_members/revisions 构造现有 `release_execution_relations` / `release_execution_binding_pins`，禁止落入读取当前 dataset/field 名称及 current_revision_id 的 legacy fallback。context 引用须在插入 release objects 前同事务准备。无该精确上下文的生产 binding 拒绝发布；原非生产绑定继续旧受支持路径。回滚/携带旧 pin 复制对应原 immutable provenance；同一 dataset 的不同 binding 产生冲突 provenance 时全体拒绝，不选任一结果。

旧 `manifest_digest` 仍严格调用现有 `ManifestDigestPayloadWithObjects`：asset-only 保持旧数组形状，有 object 时保持原 wrapper/排序/storage UUID 摘要行为。任何旧 release bytes/digest 不重算；归因和 before manifest 使用独立 digest，不混入原 payload。空 after manifest 的 digest 仍是该既有算法对空数组的真实结果，不是空字符串。

回滚只允许目标为当前 latest release，请求固定 expected head、set digest、版本与 reason，并独立授权 `release.rollback` 和 SoD（对原全部作者/审核者）后，在一个事务生成新 release。新完整 manifest 等于目标的可信 before manifest，包括未触及对象及新建对象的缺席；不存在的原 pin 必须从 current published 集合移除。保留 semantic identity、asset revisions、原 release、原审计和对象历史；不删除 asset，也不把未发布 draft revision 作为恢复 baseline。

治理对象 current registry 如需反映恢复内容，写新的不可变版本及 current pointer，但新 release 的 pin/内容以 before snapshot 为准；不能通过 inverse patch 猜测历史内容。这里严格区分 published `baseObjectVersion` 与 registry 的写入 CAS token：后继生产以旧 published pin 的精确内容计算变更，同时服务端在 creation/submit 记录 `registryWriteVersion`；发布检查 published pin 未变、当前 registry token 未变且 registry 可治理内容等于该 published baseline（忽略 version/time/audit 元数据），然后从当前 registry counter 分配新版本。回滚造成 counter 大于 published version 本身不是 stale；counter 外部变化或内容偏离才冲突，不能要求两个 version 数值相等。`registryWriteVersion` 进入 set digest 但不是客户端可伪造的基线。被撤销首次创建的正式对象可保留为非发布/retired registry 历史，消费和目录 released 状态仅依赖发布 pins。回滚也可被回滚：恢复所回滚 release 的 before manifest，仍需 latest/head/授权检查。

回滚复核恢复后的依赖闭合及原精确证据可读性；历史 before 不可信、删除关键 pin 造成悬空依赖、当前 head 改变或必要权限缺失则拒绝。服务端不从任意较早 asset pin/当前对象拼装 before，不要求本次来源 current 与历史快照相同（恢复的是已验证历史），但引用历史须真实可复核。仅不带 production protection 的旧 releases 走已有已支持 rollback 语义；带保护的任意后继必须用新权威命令。新生产 rollback 端点对没有可信 before 的旧 release 返回 `PRIOR_STATE_UNKNOWN`。

## 7. 迁移与兼容矩阵

| 二进制/API/数据库组合 | 支持条件 |
| --- | --- |
| 旧二进制 + 旧 schema | 原有行为，不具备本生产契约 |
| 新二进制 + 旧 schema | 启动/写 readiness 拒绝，提示需要迁移；不能静默降级写生产 |
| 新二进制 + expanded schema、feature 未启用 | 原有 API 和数据继续兼容；生产写入关闭 |
| 旧二进制 + expanded schema、无新语义行 | 仅受演练的旧路径临时兼容；部署必须先 drain/fence 旧 writer，不能据此承诺混跑 |
| 新二进制 + feature 已启用 | 新接口及旧普通提案兼容；旧单项命令对 production 成员拒绝 |
| 旧二进制 + 任一新 create/生产集合/新历史快照/多提案/absence rollback 记录 | 不支持且必须由部署 fence 阻止启动/流量/worker；DB grant/运行身份隔离旧 writer，不能假设旧代码会读懂一个新 min_version 字段 |
| 旧 REST/MCP/CLI/SDK client + 新 server | 旧消费 URL、旧 release pins/digest 不变，附加字段可忽略；旧 singular provenance 对多提案为 null，需完整归因的界面必须升级；禁止将 null 显示为某个猜测作者 |

迁移顺序：备份/记录 schema 和二进制基线 -> append-only expand 表/nullable 关联/索引 -> 可信历史迁移与显式 unverifiable 标记 -> 新约束校验 -> 新 binary 启动并保持 feature off -> drain 所有旧 HTTP/job/migration writers（独立 DB role 撤权或全部停止） -> 启用 production feature。不得编辑原迁移文件或让未更新旧二进制自动写 expanded schema。source-only 阶段新增权威历史也必须记入退回能力判断。

down 仅能在一个事务内确认所有新增表、关联字段及条件语义都可无损表达为旧 schema 后执行。最保守可执行条件是新来源快照、生产操作/命令/保留身份、create/reintroduction proposals、set/attempt/review bindings、release protection、多归因、before manifests、generation links/outputs/applications 均无权威记录；历史 defaults 可逆且旧 check/validation run 唯一键全通过。任一不可表达行存在立即报 `DOWN_MIGRATION_UNSAFE`，事务无 DDL/数据变动；不得 DELETE/TRUNCATE 新表后声称成功，不自动改 intent、不伪造 base revision。

保留旧数据的 binary 回退需要停用 feature、冻结写入、导出证据并恢复启用前完整隔离备份，或另行批准有损迁移；不是常规 down。后续迁移测试必须覆盖空 expand/down、含旧数据 expand、可信/不足历史、create、多提案和 absent rollback 的拒绝，核对失败前后行数/digest/schema 相同。已有更早 destructive down（例如 000007）不因此变安全，验收仅在明确拥有的隔离数据库运行。

## 8. 验证边界

`TestSemanticProductionContract` 只校验 OpenAPI 结构、关键 schema 接受/拒绝与机器可读不变量，不证明数据库锁、事务、鉴权、模型质量或已部署接口。SP-T002 至 SP-T007 必须以实际实现补 source diff、冷启动、重复/并发/故障注入、撤权、SoD、所有成员原子发布、完整 rollback、旧 API/client 与迁移证据。

模型和网络始终在事务外；模型配置/授权/input digest 在结果落库前复核，原结构化建议和人工修改分别归因。协议 stub 与实际/付费模型证据分列；本契约不授予真实模型调用、部署或访问客户数据权限。来源文本是数据，不构成系统指令、工具许可或业务规则批准。桌面验收仅 1440x900 与 1024x768；无消费者也能生产和发布。

### 8.1 AI 输出的持久化与读取

成功模型结果通过 StructuredGenerationOutput schema、有界大小/深度/总 changes、目标身份与当前授权/来源版本检查后，规范化为 `{schemaVersion,targets}`。同一事务写 production_generation_outputs 的正文+摘要+字节数、成功 AgentRun/Step 和 generation link；commit 后即使无人应用且进程重启，GET 也只从此行恢复原建议。正文与 delta 存规范 UTF-8 JSON 字节而不是依赖 jsonb 重序列化，避免数字字面量重编码改变已固定的摘要；数据库限制字节数及有效 JSON object/array 形状，应用仍需完整 schema 检查。AgentRun.output_digest、Step.output_hash 与输出行摘要必须一致，读取时重新计算校验，缺行/不符返回 INTERNAL_ERROR，不从摘要猜造正文，也不重发模型补内容。

失败/未决/取消 run 不写输出正文，原始 prompt、raw provider response、无效输出和 hidden reasoning 从不落库；仅保留允许的错误 code、输入/输出 hash（如适用）、时间及可用成本。输出表 UPDATE/DELETE 被 immutable trigger 拒绝，保留期至少与关联 operation/Proposal/Release 的全部历史一致，无自动 TTL；不能先清除成功正文再继续声称可恢复。后续单独授权的数据擦除必须同步显式标记证据不可用并阻断依赖，不能在本契约默默降级。

人工应用使用 PUT 的 suggestionRunId，并在同事务保存应用后 immutable version 与 production_generation_applications。run.input_version 必须等于请求 expectedVersion，source_version/output_digest 指向真实原输出；actor 取当前主体；服务器按 localKey 比较完整模型声明与人工声明，delta 为最多 256 个 `{localKey,change}`，沿用 add/update/remove 的值对，整体 target 增删使用仅供归因的 `$target` fieldPath，业务字段修改使用 dot path。delta 最大 1 MiB，输出和人工内容原文分别由 output 行与版本记录保持，差异摘要可重算。无差异也记录空 delta，不宣称纠正模型；应用操作不能由客户端提交 actor/delta/digest。每个 applied version 最多一个此关联，GET 指定 version 返回该关联，避免将全历史 diff 无界内联。已应用后的后续人工编辑用普通受权 PUT，保留其版本历史与此前应用关联，不将旧 run 偷当新输入的生成结果。读取输出/应用前重新检查当前完整 R scope，禁止以原 key/旧权限泄露正文。

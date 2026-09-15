# 语义生产接口契约

| 元数据 | 值 |
| --- | --- |
| Version | 0.1.0 |
| Status | Planned Contract；本文件与 YAML 是 SP-T001 的实施规格，不代表接口已上线 |
| Schema | [production.openapi.yaml](production.openapi.yaml)（独立 OpenAPI 3.0.3，不替换正式 `api/openapi/semlia.v1.yaml`） |
| Storage and invariants | [data-model.md](../data-model.md) |
| Requirements | SP-FR-001 至 SP-FR-010、SP-NFR-001 至 SP-NFR-005、SP-TD-001 至 SP-TD-008 |

## 1. 通用约定

业务定义与范围的发布前确认遵循 [业务规则确认契约](business-rule-confirmation.md)。`business-rule-confirmations` 是独立的明确人类确认动作，不由人工 PUT、模型输出、证据 metadata 或 release review 隐式生成。

所有路径前缀为 `/api/v1/workspaces/{workspaceId}`，使用既有管理端 SessionCookie、CSRF/origin 防护和服务端 workspace/principal 解析。路径中的 workspace 是检查条件，不是权限来源。机器 bearer allowlist、机器发布权限和消费协议均不扩展；开发身份适配只限既有显式 development 模式。本契约中的公开管理 API 不等于外部 machine API。

新 TypeID 为 ssnp、prodop、codrev、linrev；已有 ID 前缀保持不变。YAML ResourceId 验证通用 shape，服务端按字段强制 wsp/src/ssnp/prodop/agr/rls/ast/rev/phb/mgn/eky/jct/pds/pdr/pfd/pfr/evd/scd 等准确 domain 类型和 UUIDv7。ID、revision、candidate、evidence 及所有 nested 引用必须属于同 workspace；跨 workspace 或不存在的对象统一 404，不先泄露名称。已知 workspace 内 capability 拒绝为 403。

OpenAPI 定义结构形状，以下为同等强制的服务端语义，不得只依赖生成 SDK/schema validator：

- 写入 body 最大 1 MiB，服从已有更严格限制；仅 `application/json`，重复键、未知字段、尾随 JSON、非法 UTF-8、嵌套深度超过 32、非法数值拒绝。规范化 draft input+targets 最大 768 KiB，预留派生元数据空间；模型原始输入/输出各最大 1 MiB。
- 最多 32 targets、每 target 最多 100 changes、合计最多 256 changes。create changes 必须为空；create 声明本身是真实语义变化。重复 localKey、target identity、候选或 evidence/pin 视为错误而非静默去重。
- 所有返回 JSON 最大 4 MiB；列表可少于 limit，按字节预算停止在完整 item 边界并设置 nextCursor，不截断单个对象。完整 manifest 各最多 1 MiB、1000 asset pins 与 1000 object pins，遵循现有 release 边界；超过时明确 LIMIT_EXCEEDED，不丢未变对象。单 item 仍超预算则明确错误，不返回空成功页。
- 字段文本限额以 YAML 字符长度为上限，存储字节更严格时取更小者。schema 没有携带 workspace、createdBy、reviewer、publishedBy、approved、toolName、凭据、provider endpoint 或 release state 的写入口。
- HTTP 无副作用 GET 都重新鉴权；POST/PUT 的命令权限及结果读取权限分别检查。任何缺少必需对象 scope 的读取整项拒绝/从列表排除，不能隐藏一半输入后仍称“完整可审查”。现有低权限 release read 保留原脱敏行为，详见兼容部分。

## 2. 方法与权限

表中路径省略通用前缀。R 表示对实际关联的 asset/source/evidence/binding 分别核实 `asset.read`、`source.read`、`evidence.read`、`binding.read`，不是任意一个即可。W 表示 `asset.propose`；含 governed target 时额外按既有对象规则核实 `binding.manage`。创建新身份在 workspace scope 检查；更新在真实 target scope 检查。父对象与所有依赖读权限必须在同一授权快照内成立。

| 方法与路径 | 权限与前置条件 | 成功与结果 |
| --- | --- | --- |
| GET `/sources/{sourceId}/snapshots` | source.read(source) | 200 SnapshotPage，包括 partial/failed/unverifiable，不静默只留完整版本 |
| GET `/sources/{sourceId}/snapshots/{snapshotId}` | source.read(source)，source/snapshot 关系一致 | 200 SourceSnapshot，固定范围、覆盖与摘要；不内联无界成员 |
| GET `/sources/{sourceId}/snapshots/{snapshotId}/members` | 同上 | 200 SnapshotMemberPage，包含未变化 revision 和历史名称 |
| GET `/sources/{sourceId}/snapshots/{snapshotId}/diagnostics` | 同上 | 200 DiagnosticPage，已脱敏诊断 |
| POST `/production-operations` | W、必要 binding.manage、R | 201 创建；全 no_change 或同键重放 200；返回命令结果及 Location |
| GET `/production-operations` | R，每项按完整关联 scope 筛选 | 200 有界摘要，可用 sourceId/candidateId/createdBy 精确过滤 |
| GET `/production-operations/{operationId}` | R | 200 当前或 `version` 指定历史的输入/目标/提案/验证/审核/生成 run 与发布链接 |
| PUT `/production-operations/{operationId}` | W、必要 binding.manage、R；expectedVersion 当前且全体 draft | 200 全量替换后新版本；不修改任何冻结内容 |
| POST `/production-operations/{operationId}/submit` | W、R、validation.run；exact version/setDigest | 202 冻结、原 Proposal 提交和初次验证队列同时提交；重放 200 |
| GET `/production-operations/{operationId}/validations` | R；必填 version | 200 ValidationAttemptPage，按 attemptNo 降序读取该版本完整 attempt 归属与精确 run IDs |
| POST `/production-operations/{operationId}/validations` | validation.run、R；相同冻结输入；previousAttemptNo CAS | 202 新完整确定性 attempt；同键仅返回原队列；不能更换 source/base 或混用旧结果 |
| POST `/production-operations/{operationId}/reviews` | proposal.review、R、集合级 SoD；validation 指向当前完整成功 attempt | 201 原有 Review 行及 exact attempt/digest 绑定；重放 200；decision 对 selected proposalIds 原子生效 |
| POST `/production-operations/{operationId}/publish` | human release.publish、R、SoD；validation 与全体 Review 绑定同一当前完整成功 attempt | 201 唯一新 release；同键重放 200；不需要 consumer/binding registration |
| POST `/production-operations/{operationId}/generation` | W、R；当前 draft；有效模型配置及显式费用授权 | 202 已保存 AgentRun/job；重放 200，不重发模型 |
| GET `/production-operations/{operationId}/generation/{runId}` | R，run 必须关联该 operation | 200 原始结构化建议、状态与可用归因；不返回 prompt/密钥/隐藏推理 |
| GET `/production-releases/{releaseId}` | R，包括来源/evidence 历史读取 | 200 同一 Release 的生产详情；旧 release 无必要历史则 PRIOR_STATE_UNKNOWN |
| POST `/production-releases/{releaseId}/rollback` | human release.rollback、R、集合级 SoD；目标是 current latest | 201 恢复完整 before manifest 的新 release；重放 200 |

YAML 中共用 responses 是状态全集；具体 operation 必须遵循本表：PUT 只返回 200，review 首次返回 201，create 全 no_change 返回 200，不能随意互换。所有异步成功只表示队列意图已提交，不是 validation/model 已通过。

## 3. 来源读取与分页

快照由现有 ingestion/discovery 完成路径在同一投影事务产生，没有客户端写快照的新旁路，也没有新增 connector。后续正式 discovery run 响应以可空 `snapshotId` 附加关联，旧无可信历史运行可无 ID 或关联 historyQuality=unverifiable；不能为所有旧运行填当前 snapshot ID。snapshot/run 多对一关系保留每次运行。

所有列表默认 limit=50，1..200；nextCursor 为 null 表示结束，不要求客户端猜 total。cursor 为版本化 opaque token，绑定 kind、workspace、source/snapshot、所有 filter、初次查询 upper watermark 及最后 key。每页重新鉴权，权限撤销不会因 cursor 固定而失效；无权 items 不计可见总数。游标篡改、错资源、错 workspace、不同 filters 或过期 schema 返回 400 INVALID_CURSOR，不自动从头重复。

快照与 operation 按 `(created_at DESC,id DESC)` keyset；operation 的排序不使用会变的 updated_at，列表 summary 返回当前可见版本。成员按 `(kind,object_id)`，诊断按 ordinal。固定 watermark 防止翻页时新插入重复/漏读原集合；权限变化可导致可见结果减少，这是安全筛选而不是 snapshot 数据变化。返回 item 预算不足时使用最后实际输出的 key，下一页继续。

validation attempt history 按 attemptNo 降序，version 必填且固定于 cursor，upper watermark 为首轮最大 attemptNo；每 item 是一个完整 attempt 的 result envelope，不将各 validator 的 latest result 拼成 item。当前 queued/running 元数据可变至 terminal，输入声明不可变；分页不暗示运行已成功。

SnapshotMember 的 name/locator/revision/digest/coverageKey 是本次观察，不从 current 表取历史名称。field 必须带 parentObjectId 和 parentRevisionId，二者均匹配同一快照 dataset；dataset 不允许伪造 field parent。code 的 revisionId 为 codrev，lineage 为 linrev。完整性由 CoverageUnit.status 与 enumerationComplete 同时判定；`status=complete` 必须 enumerationComplete=true，无 blocker 缺失；partial/failed 不能假推删除。不可验证历史只返回确证成员，historyQuality 保留 unverifiable。

## 4. 请求、摘要与幂等

CreateProductionRequest 固定 ProductionInput 与所有 ProductionTarget。input.snapshots 的 digest 必须等于服务端快照；coverageKeys 显式选择可用完整 unit。candidate digest 绑定候选真实内容版本而非决定状态；snapshotId 必须是其实际 evidence 版本。candidate.targetKeys 都存在，primaryTargetKey 显式属于其中并对应真实 Proposal，用作旧 singular proposalId 的兼容桥；不允许数组顺序决定 primary。证据 ID+digest 必须存在、可读且属于固定 input，业务规则 attestation 来自有权人，不从 AI 文本/置信度推断。

create 声明以 localKey 协调本集合引用；服务端保留 target TypeID，semantic asset 仅预留 draft 身份，不制造 base revision。首次 create 不带 reuseIdentity；可信首次发布后回滚缺席的同身份可带该明确证明重新 create，规则见下文。update 必须带目标 ID 和当前真实 published baseRevisionId 或 baseObjectVersion；两者互斥且按 kind 选择，不能传 create 的 identityKey。资产 address、assetType、稳定 target identity 不得作为普通 update 改写；这类领域迁移不在首个样例范围，返回 INVALID_ARGUMENT。仅非生产旧接口保留已有真实 draft revision update 行为。

reuseIdentity 是 create 的条件数据，不是第三 intent。它要求 targetId、creationOperationId、creationReleaseId、实际移除 pin 的 absenceReleaseId 及 present expectedHead；服务端证明原 reservation 的 kind/identityKey/身份归属、创建与保护回滚链，当前 head digest 相符且该 pin 仍 absent。仅凭保留的 registry row 或“我想复用这个 ID”不构成证明，未知 legacy 历史 fail closed。其 expectedHead 必须与服务器冻结 baseline 一致，并在 submit/publish 再 CAS；不能填 absent head、root baseRevisionId/baseObjectVersion 或伪造旧 published pin。合法重引入沿原 create Proposal、新验证、新审核与完整原子发布，保留原 ID/history；已有 row 以 registry CAS 更新而非重复 INSERT，语义 revision 相同 digest 可安全复用。原已发布历史不强迫当前 absent 身份使用无合法基线的 update。数据模型 §4.4 固定所有存储条件。

结构化字段映射沿用现有权威：AssetContent 写 asset revision 业务 content（identity address/type 校验既有身份）；BindingContent 写 PhysicalBinding，GrainContent 写 ModelGrain，EntityKeyContent 写 EntityKey，JoinContent 写 JoinContract。写入前将 localKey/PublishedReference 解到 existing domain IDs；snapshot revision/evidence 关联保留在 production input 与验证绑定中，不把物理历史伪装为 current field。`dataset`、Join 两端必须 PhysicalReference.kind=dataset，`field`、fields、pairs 必须 kind=field；字段与 dataset 归属和精确版本由服务端校验。Join pairs 同时提供左右值，保持顺序和相等数量。SemanticReference 的 asset 位置只允许 semantic_asset 类型。

update changes 使用现有 dot-separated fieldPath（不使用 JSON Pointer），仅允许该 kind 内容 schema 的真实业务字段，不允许单独的 `.`, 空 segment、数组索引、`__proto__`、身份/权限/状态/摘要字段或相互重叠路径。标量/数组整体替换，原子 before/after value 根据基线校验，服务器生成 ChangeSetItem digests；desired content 必须等于 ApplyChangeSet 的输出，否则 CONTENT_MISMATCH。无变化的 target 没有 Proposal，其他成员可把它作为既有 pinned dependency，不创建虚假 diff。

规范化与 digest 的算法、排序及数值边界见 data-model §3。`Idempotency-Key` 每个 POST/PUT 必填，ASCII 8..128。唯一键为 workspace+认证 principal+command kind+key；request digest 还包含 URI resource ID、API/schema version、expectedVersion 与完整规范化内容。相同 key 不同 body 409 IDEMPOTENCY_CONFLICT；相同 key 相同 body 在重新授权后返回原 commit 的 IDs/version/digest，`replayed=true`。原结果的 outcome 描述当时命令，不误写成当前治理状态；客户端接着 GET 获取当前状态。

幂等记录与业务 commit 同事务保存且不默认 TTL 删除。进程死于 commit 前则重试可以执行；commit 后失去响应则重放只返回原结果。记录不可按浏览器/session 生命周期清除。不同 key 的 duplicate candidate/create identity 由自然键及候选关联约束拒绝，ALREADY_PRODUCED 只向有权主体附带 operationId；新 principal 不获得原命令重放身份。supersede 同时验证旧操作与 reservation 权限，写新 immutable candidate decision 并 CAS 更新旧 singular 投影，不修改历史 decision。

没有候选的 update 也通过 workspace+business digest 的唯一 claim 防止不同 key 重复创建相同提案；business digest 不含作者/幂等键但含真实输入/base/supersedes。命中时只返回已授权 existing operation 的 ALREADY_PRODUCED，不把请求者归因为原作者。显式后继的新输入或 supersedes 引用形成新 claim；重复提交该后继仍唯一。

## 5. 冻结、验证与审核

ProductionOperation.version 是内容版本，summary.currentVersion 是最新版本；setDigest 包括精确输入、所有 targets/Proposal/content、baseline head、内部边/外部依赖及 validator/policy 版本。服务器同时固定 governed object registryWriteVersion，避免回滚后 published version 与 registry counter 不同导致永久冲突。该 token 是内部写 CAS，不由用户提供，也不替代 published base。activeValidation 返回当前请求的完整 attempt 元数据、requiredChecksDigest、freshnessDigest、runIds 和仅成功时存在的 validationDigest；未提交为 status=not_requested。TargetResult.validationRunIds 只来自同一 active attempt，不能混入历史成功。

PUT 只替换全体 draft，保留旧 immutable operation version。body 的 expectedVersion 与当前不一致 409；即使只是更新 title，也不能绕过 version/set 与贡献者归因。supersedesOperationId 是冻结后的纠正路径；新提案重新验证审核，原集合/结果不覆写。数据模型明确 terminal、身份保留与合法状态转换。

submit 短事务按固定锁序复核授权、源版本、候选、依赖和 base，固定 set、分配 attemptNo=1、让全体成员沿原 `draft -> proposed -> validating` 并与 validation jobs/outbox 同时提交。队列故障返回失败且不留下部分 submitted 集合。验证 worker 在事务外执行确定性检查，结果 commit 时检查授权/version/setDigest/freshness；状态不明、失败、未运行、缺证据不能汇总为通过。生成不可替代 schema/reference/grain/key/join/policy 校验。

重验命令必须 previousAttemptNo=当前 active attempt；服务器 CAS 分配下一个序号（每版本最多 256），同键重放不新建，另一 key 在 active attempt 仍 queued/running 时返回 VERSION_CONFLICT。新 attempt 固定完整 required `(proposal,validator,version)` 集合并全部重跑，包括无影响来源检查。transport/job lease retry 只续跑同一个 attempt，新的人工重验才分配新 attempt。结果查询/去重/job payload/policy 输入必须包含 operation/version/attempt，旧单 proposal+validator 唯一键的扩展与 legacy 部分索引见 data-model §5.1。

production executor 对 in_review 的新 attempt 实际执行检查，不进入当前代码的“仅补 policy 然后返回”分支；Proposal 不倒退到 validating。只选择当前 active attempt 的完整成功集合：不得按 validator 挑不同 attempt 通过结果、不得在当前失败或未完成时回退旧成功。validationDigest 固定全部 required checks、精确 run/result 内容、freshness 与 policy；成功完成时一次提交。full required set 不可由请求者删减，缺失/failed/cancelled/not_checked 不能变成成功。历史通过上述 GET validations 的有界完整 attempt page 读取，checks 投影既有 ValidationRun/Result 的 validator、proposal、状态和脱敏 findings；不假设存在未提供的 run-detail HTTP 接口，也不把全历史并入 activeValidation。每 attempt 最多 256 checks、每 check 最多 256 findings，全部 checks 的规范 JSON 合计最多 1 MiB，超过则记录明确 LIMIT_EXCEEDED 并失败，不能截断后判成功。旧单 proposal validation-runs 聚合入口对 production 成员返回 PRODUCTION_SET_REQUIRED，指向此 operation 历史入口，不静默返回混合或截断的结果；普通 legacy proposal 维持原响应。

ReviewProductionRequest 可以审阅部分 selected members，但每个 Review 都绑定整个 setDigest 和 `validation={attemptNo,validationDigest}`；发布仍要求全体 members 达到原 policy 的全部有效审核。PublishProductionRequest 指定同一 validation，服务器核实 active attempt、完整结果和每个所采用 Review 的绑定一致。review actor 服务端解析，同一请求有任一成员无权/过期/违反 SoD 则全部不写 Review。命令 vocabulary 沿用现有 approve/reject：approve 写 approved Review，Proposal 留在 in_review；reject 写 rejected Review 并将对应 Proposal 转为 rejected。历史 changes_requested Review 仍可读，但本生产命令不新增此命令分支。后续实质更改走 successor，不能将已批准内容偷偷覆盖。

新 attempt enqueue 起所有旧 attempt Review 均不再支持发布；即使 setDigest 未变且来源影响结论为无影响，也要求新的完整成功 attempt 后重新独立审核，不能自动迁移批准。旧 Review immutable 且可审计，production 的 reviewer/channel 去重按当前 attempt 计算，允许同一合法 reviewer 对新 attempt 重审；legacy 原 stage 去重不变。当前队列/失败使 progress 反映 validating/needs_correction，不因 Proposal 仍 in_review 而显示 ready_to_publish。

SoD 按整个集合的真实作者/贡献者检查：任何成员作者、实质编辑者、生成发起人与 agent 作者都不能为该集合提供可用于发布的审核；publisher 不能属于作者集合或所采用 reviewer 集合。发布时 reviewer 当前身份与权限撤销也使该审核不能作为新发布依据。只改 display name、分拆 proposal、旧 endpoint、不同 idempotency key 都不能规避。

## 6. 发布与恢复

PublishProductionRequest 必须固定 expectedHead。首次空工作区传 `{ "presence": "absent" }`；非空必须同时给 releaseId 与 manifestDigest。absent 表示真实无 head，不是“忽略比较”。head 比冻结 baseline 改变即 409 HEAD_CONFLICT，即使认为无关也不自动 rebase；通过新的受审 operation 固定最新 baseline。

发布事务重验当前 authorization/SoD、set digest、input freshness、基线、依赖闭合、全部验证/审核/策略，写全部新 pins、完整 after manifest、完整 before manifest、每 target 的 BeforePin、全提案归因、Proposal released、审计/outbox/幂等结果。任何成员失败 rollback 全事务。消费者不存在不影响提交；projectionStatus 是独立可恢复 outbox 派生状态，pending/failed 不改变 immutable release 或伪造 ready。

input freshness 按选择的 complete coverage unit 和依赖子图判断：最新可靠观测对被引用成员、名称、规则或覆盖有变化，返回 INPUT_STALE；仅未受影响成员变化时必须有本 set 的新确定性 impact validation，固定所比对新 snapshot 后才能视为有效。旧 validation 仍指原输入，不能用 current 临时替换它。来源暂时失败且不能证明所需范围依然可靠时 fail closed，保留已有 release 可读；新通过不是由“扫描失败但缓存仍在”推断。

ProductionRelease 是同一 Release 的受权详情投影，不是第二发布表：originProposalIds 为全部 member Proposal；多成员时旧 originProposalId=null；单 applied member 时保留原 singular ID。attribution 包括 operation/version/setDigest/contributors、完整 validation reference/采用的 reviewIds 与 applied/reverted。更细作者、验证、审核归因通过该版本的 target references 和既有详情读取；集合摘要不可省略任何 Proposal。manifest digest 用原内部算法，不直接对包含 TypeID/contentDigest 的新 wire view 求 hash。

RollbackProductionRequest 的 expectedVersion/setDigest 是目标 release attribution 中的版本/集合；expectedHead 必须 present 且与路径 releaseId/current head 一致。只有 latest 可回滚。原发布的全部作者/审核者 SoD、当前回滚权限、完整 before 可复核性和恢复后依赖闭合在同一事务核实。after manifest 精确等于目标 before manifest，包括首次创建对象的 absence；不存在的 pin 从 published view 消失，但身份/历史不删除。不可用 inverse patch、任意最近旧 revision 或草稿基线冒充 before。

回滚新 release 同时保存本次回滚的 before（回滚前 head），因此可按相同规则再次回滚。governed registry 的写 counter 可以大于恢复的 published objectVersion；后续 update 比较 published base 内容和内部 registryWriteVersion 两个独立条件，从当前 counter 分配新版本，不要求两者数字相等。缺失可信 before 的 legacy release 在生产 rollback 端点拒绝，既有普通 rollback 按原契约不变。

每个生产 release 的 protection 固定 rootReleaseId 与 rollbackDepth=0；每个 rollback 后继保留 root、rollbackParentReleaseId 并将 depth 加一。旧 `/governance/releases/{releaseId}/rollback`、旧 service 以及 repository 方法对所有带 protection 的根/任意后继统一 409 PRODUCTION_SET_REQUIRED，不能因 singular origin 为 null、manifest 为空或已回滚一次而豁免。新 endpoint 是唯一受权回滚命令；所有后继继续携带完整原成员/采用的 validation/Review 归因并复核 R/SoD/exact-before。DB 在 legacy insert 试图回滚 protected parent 时要求匹配继承的保护字段，并在 commit 强制核对新命令、完整 manifest/归因；不能只依赖路由 guard。回滚复核历史验证关联的真实性及恢复后依赖，不要求把历史 source 当当前新输入重新批准。

服务端恢复顺序：按同幂等 key重放确定性命令或 GET operation；本地完全失忆时按 source/candidate/createdBy 列表定位；读取当前 version、member Proposal、validation/review、generationRunIds 和 releaseId；仅对明确未 commit 的操作重发，不根据空 localStorage 猜测创建。`outcome_unknown` 模型先恢复 run，不能自动把失败重试转换成第二次付费调用。

## 7. AI 与人工纠正

首次生产可创建来自真实候选的未决 draft（definition/scope=null），这只是稳定身份和待审声明，不要求用户先造资产/release。generation 读取该操作固定来源、候选、可读 evidence、target skeleton 和模型配置；一个 run 可返回整套最多 32 targets 的建议。运行成本与 token ceiling 是现有授权/配置之内的更小限制，body 中大数不构成费用授权。无可验证预算、配置或 source/evidence 权限时不排队模型。

现有 AgentRun/Step 记录 model/config revision/input hash/output hash/耗时/可用成本；运行在事务外。结果提交前重新检查发起主体及 agent 的当前授权、source/version/input digest、模型配置修订与 output schema/总 changes/大小；失配以明确失败结束 run，不自动将结果应用到新版本。providerMode 由服务端执行环境派生，只能 protocol_stub 或 actual_model，客户端不能把 stub 标成实际模型。

GenerationResult.output 为持久化的 `{schemaVersion:"semlia.production-suggestions/v1",targets:[...]}` 原始结构化建议，不是仅保留 hash。成功结果必须正文和 outputDigest 同时非空；非成功状态两者均为 null，不自动 submit/publish/覆盖 draft。schema-gated 正文、canonical byte count、digest 与 AgentRun/Step 成功状态在同一事务保存于 production_generation_outputs；进程在生成成功后、人工应用前重启仍可通过 GET 恢复原正文。读时核对摘要，不用重新调用模型补缺失内容。原 prompt/raw provider envelope/失败无效输出/hidden reasoning 不持久化，成功正文至少保留至其全部历史依赖的保留期，无自动 TTL。

人工确认/纠正用 PUT + suggestionRunId + expectedVersion：必须引用同 operation 的真实已保存成功 run，且 run.inputVersion=expectedVersion=当前 draft version；服务器保存原 output digest、应用后 immutable version/content digest、真实 actor 与有界结构化 delta/digest。GET operation 指定 version 的 generationApplications 仅返回产生该版本的至多一个关联；其 sourceVersion/sourceOutputDigest 可定位原正文，appliedVersion 定位人工版本，不无界内联所有旧差异。完整存储/FK、最大 256 delta items 和 1 MiB delta、目标增删 `$target` 归因约定见 data-model §8.1。原文与人工版分开且 immutable；空 delta 不声称人工纠错。应用后的后续人工编辑使用普通 PUT 留版本/贡献者历史，不隐式把旧 run 应用于新输入。原 run 不含供应商费用时返回 null，而不是虚构 0 元。

现有 AgentRun status 的 running/succeeded/failed/cancelled 保留。wire queued 来自 queued job；outcome_unknown 是 failed run 的明确 errorCode + 调用结果不确定标记，不新增审批状态。单 operation 最多 256 generation runs，达到限制返回 LIMIT_EXCEEDED，历史不覆盖；显式新 run 需要新 key 及再次费用授权，重放原 key 永不重新调用 provider。

来源文本、代码注释、dbt 描述及用户 instruction 都是不可信数据；不解释为 publish/tool/SQL 执行许可。不把客户行数据、凭据、隐藏推理写进记录；首个样例是披露的合成订单/客户。实际模型验收另行授权配置和费用，stub/人工/实际模型三个证据类别独立。

## 8. 错误与重试

| HTTP | code | 客户端动作 |
| --- | --- | --- |
| 400 | INVALID_ARGUMENT、INVALID_CURSOR | 修正参数/游标；不自动重新创建 |
| 401 | UNAUTHENTICATED | 重新登录后读取现有操作 |
| 403 | FORBIDDEN、SOD_CONFLICT | 使用有权且职责独立的主体；不回显原缓存结果 |
| 404 | NOT_FOUND | 无此可见对象或跨 workspace 引用；无旁路探测 |
| 409 | IDEMPOTENCY_CONFLICT | 保留 key 与原请求，不能以自动换 key 掩盖冲突 |
| 409 | ALREADY_PRODUCED、IDENTITY_CONFLICT | 读取已授权 operation 链接或明确匹配/纠正身份 |
| 409 | VERSION_CONFLICT、BASELINE_CONFLICT、HEAD_CONFLICT、INPUT_STALE | 读取最新状态并显式建立新版本/后继；不能自动继承批准 |
| 409 | PRODUCTION_SET_REQUIRED | 使用该 production operation 的集合命令 |
| 413 | PAYLOAD_TOO_LARGE | 缩减内容，不能拆成绕过集合原子性的单项发布 |
| 422 | LIMIT_EXCEEDED、CONTENT_MISMATCH、NO_SUBSTANTIVE_CHANGE | 修正有界输入或停止无意义 submit/publish |
| 422 | INPUT_INCOMPLETE、HISTORY_UNVERIFIABLE、EVIDENCE_MISSING、DEPENDENCY_INVALID | 建立可信完整新证据或纠正引用；不可伪造历史 |
| 422 | VALIDATION_REQUIRED、REVIEW_REQUIRED、PRIOR_STATE_UNKNOWN | 完成所需验证/审核或停止不可复核回滚 |
| 422 | MODEL_NOT_CONFIGURED、MODEL_BUDGET_REQUIRED、MODEL_OUTPUT_INVALID | 修正授权配置/输出；自动重试禁用 |
| 409 | MODEL_OUTCOME_UNKNOWN | GET 原 run，结果不明不自动再付费 |
| 503 | FEATURE_UNAVAILABLE、INTERNAL_ERROR | 查询已有操作后用原 key 重试确定性命令；不自动重试模型网络 |

ProductionError 的 message/traceId 可用于显示/排查，raw DB/provider/source diagnostics 和 secrets 不得返回。可见性检查前只给通用错误，operationId/currentVersion 仅在已授权时附带。确定性冲突 retryable=false；503 的 retryable 仅表示原 key 的安全命令重放，不表示模型调用已获新授权。

## 9. 兼容与执行闸门

生产契约只追加到后续正式 API/SDK，不在 SP-T001 改生成物。既有 source/discovery/candidate/proposal/release API 仍可读原普通对象；新 production 成员的旧单项 changes/submit/review/publish/convert 命令以及生产 release/任意回滚后继的旧 `/governance/releases/{releaseId}/rollback` 统一 fail closed，不选第一个成员或依赖 null singular origin 绕过。旧 release read 的 asset.read/binding.read 脱敏语义保持不变；有权需要完整生产归因的客户端使用新详情。`originProposalIds` 的正式附加读取若受限，明确 availability=forbidden，不泄露源/evidence。

旧 release digest 与消费 pins 不重写；多提案的 singular origin 为 null，不能指派虚假唯一作者。MCP/CLI/SDK 的 release-only 消费仍只见已发布 pins，新建 draft 身份和被回滚缺席对象不进入 production fact。原 governance object 及目录详情只能从同一权威 release snapshot 解释发布事实，不把 mutable registry 当成已发布内容。

expand-first、旧 NOT NULL/check 分支、旧二进制 fence、旧 schema readiness 和拒绝有损 down 的精确矩阵见 data-model §4、§7。启用前必须停止/隔离全部旧 writer，不能假设未经更新的旧二进制会遵守新 route guard 或 min schema 字段。down 不得清表装成功，新增权威历史存在时保持 schema/data 原样拒绝。

本任务的 GREEN 只代表静态契约可解析且断言通过。真实事务原子性、跨 scope 授权、幂等竞争、模型异常、发布缺席回滚、旧客户端与数据库迁移仍分别由 SP-T002 至 SP-T007 实现和验证。UI 仅 1440x900、1024x768，保持键盘、焦点、reduced motion、mock 披露，不扩移动端或外部接入范围。

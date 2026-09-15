# SP-T003 A003 修复交付

Status: Needs_Review

T003 的受限修复及执行包检查已完成，等待复审与用户验收。此状态不等于 Accepted，不开放 T004 至 T007 的前置验收门禁。基线为 `.semlia/evidence-work/SP-T003-A003-baseline.tar`，不是 HEAD；既有未提交改动保留。

## 修复覆盖

| 范围 | 当前实现与运行时证据 |
| --- | --- |
| F1 输入真实性 | 事务内核对真实快照、完整 coverage/current head、候选摘要及嵌入来源证据、证据可读性、依赖发布 pin；未知输入无业务提交。 |
| F2 原子替换 | 新 Proposal、changes、targets、links、command、contributor、decision、audit/outbox 同事务；注入 command 失败整体回滚；旧草稿走合法 rejected 转换。 |
| F3 冻结 | operation/version 锁下检查 frozen_at 和全体 draft；数据库版本推进及 Proposal 内容/状态防线拒绝旁路写入。 |
| F4 恢复与重放 | GET 指定历史版本，恢复原始声明和 canonical 字节权威；原 create/replace key 重放读取 command.operation_version，并重新检查原版本完整权限。 |
| F5 五类目标 | semantic_asset、physical_binding、model_grain、entity_key、join_contract 均有真实 create 和已发布基线 update 正例；全 no_change 不产生 Proposal；变更以真实发布内容 replay 并比对完整 desired content。 |
| 身份与候选 | 同键八请求只提交一次；不同键并发只有一份 operation/Proposal/asset，另一请求返回含已授权 operationId 的 ALREADY_PRODUCED；candidate decision 不可变、主投影 CAS；supersede 保留决策历史并仅转移指定前驱的 reservation。 |
| 引用与权限 | 结构化 local/published/physical 引用，验证实际成员及字段父版本；错误父数据集返回 DEPENDENCY_INVALID 且零提交。合法 domain/source scoped grants 无须 workspace-wide grant；撤销旧来源权限后旧版本和原 create 重放拒绝，仍可读的新版不受误伤。 |
| 列表与恢复 | created_at/id keyset、固定 watermark、source/candidate/createdBy filters、主体/资源/筛选绑定加密游标、完整对象权限过滤；真实 Proposal/validation/review/release 投影；响应预算不截断对象。 |
| 严格输入 | production-only parser 拒绝重复/未知键、错误 UTF-8、尾随值、过深/过大输入；保留数字字面量和业务字符串；无序集合确定排序、重复项拒绝；不修改旧 CanonicalJSON 算法。 |
| 持久化 | migration 25 增加 canonical 历史、workspace/principal/版本 FK、完整提交校验、不可变历史、reservation 与旧 writer 防线；readiness 校验新约束。旧记录保留为 unverifiable，不伪造可信历史。 |

冷启动使用真实 catalog adapter/PersistDiscoverySnapshot 生成来源历史，初始无 semantic assets/releases；五类 create 只有真实草稿 Proposal，不提前建立四类正式 registry 对象。候选夹具是显式持久化候选并携带真实来源 witness，不声称覆盖自动候选生成算法。

## 验证结果

全部检查使用本地工作区；数据库检查串行运行于测试自建 PostgreSQL 容器，不连接默认开发库。

| 命令范围 | 结果与日志 |
| --- | --- |
| 执行包八个单元/CLI 包，`-p=1 -count=1` | exit 0；[最终日志](A003-unit-final.log)，包括 migration-25 readiness。 |
| 完整 governance/catalog/discovery 集成包，清除四个外部 DB/执行覆盖变量 | exit 0；91.890 / 19.485 / 124.645 秒；[日志](A003-integration.log)。 |
| migration 22/23/24/25、旧历史保留、危险 down 拒绝、PG17 生命周期 | 包结果 PASS，86.851 秒；[日志](A003-migrations.log)。命令在执行包过滤之外额外纳入 AuthoringMigration/PublishingMigration 两组。 |
| 补充不同键竞争及物理父引用负例 | exit 0，7.811 秒；[日志](A003-boundary-green.log)。[首次日志](A003-boundary.log) 保留：物理负例实际正确返回 422，测试误写 409，修正断言后通过；不是产品缺陷 RED。 |
| db-generate-check / contracts-check | exit 0；[SQL 生成](A003-db-generate-check.log)、[API 生成](A003-contracts-check.log)。 |
| test-contracts / test-repository | exit 0；[契约](A003-test-contracts.log)、[仓库](A003-test-repository.log)。最终契约命令命中缓存，本轮较早执行同命令亦通过，22.517 秒。 |
| SDK test | exit 0；[日志](A003-sdk.log)；该脚本为 `tsc --noEmit`，不是 SDK 运行时测试。 |
| git diff --check | exit 0；[日志](A003-diff-check.log)。 |

最终非集成命令、UTC 起止与退出码见 [检查记录](A003-checks.json)。A002 的失败与通过日志均保留，不用绿色结果覆盖早期反例；新增修复测试没有逐项执行移除实现的 negative control，不声称每项都有独立 RED 日志。

额外运行 governor 通用 artifact validator，exit 1：该脚本要求 `.ai-platform/` 下五个文档，而本仓库采用 `docs/specs/semantic-production/`。这是工具布局不适配，不记为通过，也不为通过脚本另建一套治理文档；执行包要求的仓库契约检查通过。

## 复核与边界

本轮自检覆盖输入到事务的权限/来源复验、历史读取与幂等重放、五类真实发布基线、候选转换、reservation 转移、旧写路径防线及迁移保真。F1 至 F5 有对应正式回归测试；本轮未发现额外已证实的 T003 阻断，但该自检不代替独立验收。

- T004 application/store/HTTP 从 submit 开始的业务函数段，经 gofmt 规范化后与 A003 基线逐字节一致；原始差异仅为格式化。migration 23/24 原始字节不变，见 [边界哈希](A003-preserved-boundaries.json)。T004 测试只补齐 author principal 的来源/内容夹具参数，原业务断言保留，其独立审查问题不在本修复中关闭。
- `reuseIdentity` 已按 wire 解析，但在缺少可信创建/缺席发布证明时返回 PRIOR_STATE_UNKNOWN；`suggestionRunId` 在没有真实、成功且匹配版本的生成输出能力时同样拒绝。本轮不创建伪造 release/run/output，不宣称实现 T004 证明链或 T005 生成应用。
- 迁移前 production history、无法还原为契约内容的 legacy published payload 均 fail closed。不得把旧 jsonb 或当前可变 registry 内容冒充不可变历史证据。
- 游标密钥为进程级随机值，重启会使旧游标失效，客户端需重新查询。全范围列表/内容边界有实现和代表性测试，没有穷举全部 4 MiB 极限组合；权限撤销覆盖确定性历史访问，不声称压力测试了所有并发调度次序。
- production feature 保持默认关闭。上线仍需独立复审、用户接受、旧 writer 排空和正式迁移流程。本轮无部署、现有数据库迁移、真实模型调用、Git commit/push 或前端修改。

源码和文档增量、SHA-256 与基线来源见 [增量清单](A003-source-delta.json) 和 [实际 diff](A003-source.diff)。A003 归档漏含 `tests/integration/db`，这两个获准修改的迁移测试文件使用可用的 A002 返修前归档补充对比，清单明确标注，不冒充精确的 A003 前像。

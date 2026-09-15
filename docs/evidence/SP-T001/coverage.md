# SP-T001 生产验收覆盖矩阵

本表区分既有可复用验证与新增生产验收，不能把已有局部用例的通过外推为 SP-AC 通过。实际命令结果见本目录的 `baseline-*.result.json` 与原始 JSONL；详细 schema 的静态测试仅验证文档。

| 验收 | 已有验证依据 | 尚缺的生产断言 | 责任任务 |
| --- | --- | --- | --- |
| SP-AC-001 空语义工作区 | ingestion 的工件持久化/隔离；discovery 的实际解析与扫描测试 | 无资产、提案、release、consumer 的完整样例；不能复用预置资产的来源 E2E 作为证明 | SP-T002、SP-T003、SP-T006、SP-T007 |
| SP-AC-002 历史成员与改名 | `TestIncrementalDiscoveryReplayAndChangedShape`、`TestPossibleRenameAndUnresolvedLineageBecomeFindings` | 不变成员仍在历史集合，名称快照、删除、部分覆盖、旧历史不可验证 | SP-T002、SP-T007 |
| SP-AC-003 新建/更新 | `TestGovernanceProposalJourneyOverHTTP`、`TestNoSubstantiveChangeNeverEntersTheHumanQueue` | 无基础 revision 的新建、稳定身份预留、重复候选去重、服务端原子关联 | SP-T003、SP-T006、SP-T007 |
| SP-AC-004 AI 与人工纠正 | `TestGenerateProposalWithLiveProviderCreatesAttributedProposal`、无效输出和权限拒绝用例 | 来源集合到新目标的受限生成；原始输出与人工修订分开；获授权的实际模型质量证据 | SP-T005、SP-T006、SP-T007 |
| SP-AC-005 最小产物闭合 | 既有五类目标结构校验、引用验证、单目标发布及内容投影 | 两实体、一口径、物理绑定、键、粒度、Join 同集合验证与原子发布，跨目标引用完整 | SP-T003、SP-T004、SP-T007 |
| SP-AC-006 无效输入不发布 | `TestReferenceValidatorResolvesFieldReferencesAgainstPhysicalGraph`、`TestValidationBlockerDegradesToHumanHandlingNeverAutoRejects` | 冻结输入摘要、来源过期、审核后编辑、集合依赖和纠正后的重新审核；正常路径亦须成功 | SP-T004、SP-T005、SP-T007 |
| SP-AC-007 无消费者治理发布 | `TestPublishRequiresApprovingReviewAndAuditsTheRefusal`、`TestPublishRefusesProposalAuthorAndSoleApprovingReviewer`、单目标 release 测试 | 集合全体成员 SoD、事务内授权复查、多提案归因、禁止旧单项入口绕过 | SP-T004、SP-T006、SP-T007 |
| SP-AC-008 变化/重放/回滚 | discovery replay、`TestRollbackRestoresPriorRevisionPointerAsNewRelease`、`TestGovernedLoopPublishesAndRollsBackWithAppendOnlyProjection` | 多对象完整 before manifest、首次新增对象的缺席回滚、来源历史不变、整体无实质变化不发布 | SP-T002、SP-T003、SP-T004、SP-T007 |
| SP-AC-009 跨浏览器恢复 | ingestion 主体绑定幂等、验证入队去重、workbench 的持久化读取与 restart 用例 | 生产操作权威 ID/列表、清除本地缓存、响应丢失、提交与发布独立幂等、重新鉴权 | SP-T003、SP-T006、SP-T007 |
| SP-AC-010 越权与竞争 | 来源凭据绑定、工件授权 fence、proposal/review 权限拒绝、执行回滚竞争测试 | 生产集合成员逐项授权、来源文本注入、模型输出落库前复查、生产/发布竞争故障注入 | SP-T002、SP-T003、SP-T004、SP-T005、SP-T007 |
| SP-AC-011 桌面完整流程 | 已有 UI 测试可复用；本次未运行浏览器 | 1440x900 与 1024x768 新生产路径、真实 API、焦点/键盘、刷新和失败不回退 fixture | SP-T006、SP-T007 |
| SP-AC-012 生产独立与兼容 | 既有契约测试、单目标发布与消费执行用例 | 新集合的 REST/MCP/CLI/SDK 兼容、无消费者冷启动、生成物门禁、10,000 资产既有门槛 | SP-T004、SP-T006、SP-T007 |

## 证据边界

- `LiveProvider` 生成测试显式启动 `openAICompatibleStub`，不调用外部真实模型；名称不是实际模型验收证据。
- `web/e2e-live/source-live.spec.ts` 先创建 catalog asset，并通过默认 Compose 准备 PostgreSQL 数据；本次不运行该脚本，也不将其等同于冷启动生产样例。
- 既有 `TestExecutionCrashWorker` 是子进程辅助入口，顶层运行可按实现约定 skip；是否通过 crash 场景要检查父用例，不能将 skip 计为通过。
- 本表十二项均不是已验收的产品能力。T001 固定设计与基线，真实生产断言由对应实现任务和 SP-T007 补齐。

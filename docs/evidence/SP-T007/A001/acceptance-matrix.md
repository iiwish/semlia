# SP-T007 验收矩阵

状态：Needs_Review。矩阵区分后端协议、实际模型和浏览器证据；集成测试中的独立 principal 是合成测试角色，不代表真实业务人员签署。

| 验收项 | 核验入口 | 当前证据与边界 |
| --- | --- | --- |
| SP-AC-001 | `TestSemanticProductionCommerceColdStart`；桌面候选新建 | 合成 commerce catalog 经真实 adapter 发现；后端确认初始资产、提案、release、consumer 为空，创建十个关联目标。浏览器两尺寸复验通过。 |
| SP-AC-002 | `TestSourceSnapshotUnchangedMembersAndHistoricalNames`、`TestSourceSnapshotUnitHeadsDeletionAndScopeShrink` | 新鲜 discovery 回归通过；commerce 变化案例核对 customers 未变化成员及历史 revision。 |
| SP-AC-003 | `TestProductionAuthoringPublishedBaselineUpdateAndNoChange`、`TestProductionAuthoringAllFivePublishedUpdates`、commerce 冷启动 | 新鲜 governance 回归通过；新建、已发布基线更新和幂等重放均有正常路径断言。 |
| SP-AC-004 | `TestSemanticProductionConfiguredModel`；`model-quality.md` | 4 次实际调用成功，原始输出与人工纠正分别保存。最后一次服务端应用及发布内容读回；原始 6/8，纠正 8/8，不把结构通过当作商业规则正确。 |
| SP-AC-005 | commerce 人工纠正发布；`TestProductionCompositeFiveKindsPublishAndRepeatedRollback` | 两实体、一指标及七个关联对象；服务端发布内容核对身份、引用、键、粒度、Join 和口径。 |
| SP-AC-006 | `TestProductionGenerationInvalidOutputCreatesNoSuccess`、`TestProductionValidationWorkerRunsCompleteAttempt`、`TestProductionPublishReauthorizesReviewerAndFencesNewAttempt` | 新鲜 governance 回归通过；无效输出、完整验证失败和审核失效拒绝，与正常发布同时覆盖。 |
| SP-AC-007 | `TestProductionBusinessRuleConfirmerCannotReviewOrPublish`、commerce 发布 | 服务端职责分离、冻结 validation digest 与审核集核验；不注册 consumer。 |
| SP-AC-008 | commerce 变化来源生产及回滚；composite 重复回滚 | 第二次扫描、正确旧版基线、第二版发布、回滚 manifest 和幂等重放通过。精确对象投影回归另由 composite 与 projection 测试覆盖。 |
| SP-AC-009 | 桌面断网/缓存清除/重开恢复；`TestProductionAuthoringRecoveryCursorWatermarkAndFilters` | 后端新鲜回归及本轮两尺寸浏览器复验通过。T006 接受前已修复读取失败被并发成功覆盖及失败读取后错误解锁两个问题。 |
| SP-AC-010 | generation security/freshness/atomic、authoring scope、发布并发测试 | 新鲜 governance 回归通过；跨工作区、提示权限注入、撤权、旧输入和并发写入反例均保留。实际模型仅使用合成输入。 |
| SP-AC-011 | `playwright.production.config.ts` 两个桌面项目 | 专用环境真实 API 浏览器 4/4 通过，主代理已查看代表截图；只支持 1440×900 与 1024×768，不纳入移动端。 |
| SP-AC-012 | 六包全量回归、数据库历史迁移、10,000 资产基准、`make check` | 专用环境 full suite 退出 0：六包新鲜回归及 10,000 基准通过，分页/搜索 p95 为 15.025/167.496ms。最终安全补丁快照的第七次完整门禁退出 0，依赖与发布镜像 HIGH/CRITICAL 均为 0。 |

全部结论须结合 `test-results.md` 的最终命令结果及快照边界，不以本矩阵代替门禁。T007 工程交付完成，阶段 Accepted 仍需用户明确验收。

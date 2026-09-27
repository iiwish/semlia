# Semlia 文档中心

[English README](../README.md) | [中文 README](../README.zh-CN.md)

本目录是 Semlia 的产品、架构、研发、运维与项目政策入口。产品定义、核心边界和路线图以 [SSOT](SSOT.md) 为唯一事实来源；原型、规格、ADR 和实现证据不得形成与 SSOT 竞争的定义。

## 推荐阅读路径

| 目标 | 从这里开始 | 接下来阅读 |
| --- | --- | --- |
| 了解产品方向 | [SSOT](SSOT.md) | [语义生产闭环重构](specs/semantic-production/product-design.md)、[知识治理底座](architecture/knowledge-governance.html) |
| 运行项目 | [Quickstart](quickstart.md) | [本地开发](operations/local-development.md)、[故障排查](operations/troubleshooting.md) |
| 验收本地 1.0 候选 | [收敛版 1.0 规格](specs/v1-convergence/spec.md) | [任务图](specs/v1-convergence/tasks.md)、[T005 交付报告](evidence/V1-T005/summary.md)、[发布验证](operations/release-verification.md) |
| 升级与恢复 | [备份与恢复](operations/backup-recovery.md) | [自托管示例](../deploy/examples/README.md)、[T004 实证](evidence/V1-T004/consumption-restore.md) |
| 开始后端开发 | [M1 TDR](adr/0002-m1-product-and-semantic-execution.md) | [SSOT 系统架构](SSOT.md#10-系统架构)、相关 `specs/` 与 OpenAPI 契约 |
| 参与项目 | [贡献指南](CONTRIBUTING.md) | [行为准则](CODE_OF_CONDUCT.md)、[安全政策](SECURITY.md) |

## 产品与架构

| 文档 | 作用 | 状态 |
| --- | --- | --- |
| [项目 SSOT](SSOT.md) | 可信知识驱动问数、当前 1.0 范围与长期架构边界 | Confirmed |
| [收敛版 1.0](specs/v1-convergence/spec.md) | 当前主线：接入、五类知识、独立审核发布、真实 Ask/Execute、对账纠错与基础机器消费 | RC.6 技术验证与用户委托的代理浏览器验收通过；Git 交付以 PR CI 为门禁 |
| [语义生产闭环重构](specs/semantic-production/product-design.md) | 来源与证据、候选到新资产、验证审核、发布、增量与恢复的设计依据 | Confirmed |
| [语义生产重构计划](specs/semantic-production/plan.md) | 复用边界、八项技术决策、依赖与工期区间 | Confirmed |
| [语义生产重构任务图](specs/semantic-production/tasks.md) | 该专项的任务范围、TDD 与验收记录，不替代当前 V1 任务图 | Confirmed |
| [知识治理架构](architecture/knowledge-governance.html) | 五层知识链路、可信解析、风险分流与反馈闭环 | Current |
| [语义资产设计](specs/semantic-assets/semantic-asset-design.md) | 核心资产类型、详情信息架构与发布消费契约 | Confirmed |
| [产品导航设计](specs/product-prototype/navigation-design.md) | 已批准原型范围的责任域、二级入口、深链与返回规则 | Confirmed（该原型范围） |
| [产品原型设计契约](specs/product-prototype/product-design.md) | 已批准原型范围的桌面旅程、状态、视觉约束与验收 | Confirmed（该原型范围） |
| [外部 AI 接入与调研验证](specs/data-finding-workflow/product-design.md) | 后续需求：三类接入能力、同契约参考客户端和预算/实际验证场景 | Confirmed；排期 Deferred |
| [外部 AI 接入技术计划](specs/data-finding-workflow/plan.md) | 后续公开消费基线、参考客户端和只读探索契约；恢复时重新核对 | Ready_For_User_Review；排期 Deferred |
| [外部 AI 接入任务图](specs/data-finding-workflow/tasks.md) | EAI-T001 至 EAI-T003 不在当前执行队列 | Ready_For_User_Review；排期 Deferred |
| [访问控制设计](specs/access-control/product-design.md) | 身份、角色、权限、策略与职责分离体验 | Current |
| [M0 TDR](adr/0001-m0-technical-foundation.md) | 工程基础的技术选择、取舍与质量门禁 | Confirmed |
| [M1 TDR](adr/0002-m1-product-and-semantic-execution.md) | 生产 Web、来源发现、物理图谱和可选执行适配器边界 | Confirmed |
| [资源标识 TDR](adr/0003-resource-identifiers.md) | UUIDv7、TypeID、语义地址与自增 ID 的使用边界 | Confirmed |

## 开发与运维

| 文档 | 作用 |
| --- | --- |
| [Quickstart](quickstart.md) | 从私有仓库 Clone 到本地首次请求 |
| [本地开发](operations/local-development.md) | Native server/worker/Vite、已有 PostgreSQL、显式迁移和普通账号；区分容器门禁 |
| [故障排查](operations/troubleshooting.md) | 工具链、端口、原配置保全、API、数据库与归属安全诊断 |
| [备份与恢复](operations/backup-recovery.md) | 一致备份、非空文件库与密钥、新目标恢复及普通解密/API 验证 |
| [发布验证](operations/release-verification.md) | 本地 RC 身份、最终门禁与独立的正式签名发布边界 |
| [API 契约](../api/README.md) | OpenAPI、生成类型与契约演进规则 |
| [数据库](../db/README.md) | sqlc 查询、索引、迁移与保留策略 |

仓库根目录的 `Makefile` 是稳定开发接口。优先使用 `make doctor`、`make bootstrap`、`make test-repository`、`make contracts-check` 和 `make check`，不要在文档中建立第二套命令入口。

## 交付与证据

`specs/` 保存产品规格、计划、任务、检查清单和执行包，各文档通过元数据标明审核状态，待审文件不作为实施授权；`evidence/` 保存相应实现、测试、评审与视觉验收证据。

当前主线是 [`specs/v1-convergence/`](specs/v1-convergence/) 的收敛版 1.0 本地候选；技术计划已获明确实施授权，最终候选门禁、用户验收与正式公开发布相互独立。五类知识、治理、问数与只读执行是实际链路，不以旧里程碑预览替代；长期 SSOT 能力不自动纳入本轮。

| 证据 | 验证边界 |
| --- | --- |
| [T001](evidence/V1-T001/summary.md) | 隔离合成环境、正常身份、来源与五类知识发布基线 |
| [T002](evidence/V1-T002/summary.md) | 真实模型尝试、独立 SQL、负例、结构化故障/纠错/回滚，保留失败分母 |
| [T003](evidence/V1-T003/summary.md) | 两种桌面尺寸正常 UI、自然修订与独立发布、历史依据、恢复正确基线 |
| [T004](evidence/V1-T004/summary.md) | 普通机器通道、有限升级基线、完整备份恢复及隔离门禁 |
| [T005](evidence/V1-T005/summary.md) | RC.6 同字节界面、代理浏览器验收和固定模型 4/4 正例、2/2 负例通过；新鲜/继承门禁明确区分，不是正式签名发布 |

产品仅支持 1024px 及以上桌面 Web；合成验收不是企业 SLA。无已发布分析模型的规划、完整独立映射治理、长期记忆、自动发布和大量新增连接器不在当前范围。

M0 的计划、工作图、需求检查、分析和发布报告位于 [`specs/m0-foundation/`](specs/m0-foundation/)。已批准原型与访问控制的范围记录分别位于 [`specs/product-prototype/`](specs/product-prototype/) 和 [`specs/access-control/`](specs/access-control/)。[`specs/data-finding-workflow/`](specs/data-finding-workflow/) 是排期 Deferred 的外部 AI 接入与调研验证需求；生产验收后需用户另行确认恢复，不自动执行其任务。它不定义 Semlia 业务任务中心或全站前端重构。A1/A2/A3 是接入方的分级评测场景；第一方测试客户端与内部 AI 语义工程分开，停用前者不影响核心生产。待审文件不代表已批准实施或已经实现。

证据是对特定提交和验收范围的记录，不替代 SSOT、ADR 或当前运行手册。

## 社区政策

- [贡献指南](CONTRIBUTING.md)：开发流程、测试、Commit sign-off 与评审要求。
- [行为准则](CODE_OF_CONDUCT.md)：参与规范、问题报告与处置原则。
- [安全政策](SECURITY.md)：支持范围、漏洞报告和安全门禁。
- [Apache License 2.0](../LICENSE) 与 [NOTICE](../NOTICE)：许可证与归属信息。

这些文件位于 `docs/` 顶层，保持 GitHub 社区健康文件的标准命名，同时减少仓库根目录中的非构建入口。

## 文件布局约定

```text
repository root
  README.md              英文项目入口
  README.zh-CN.md        中文项目入口
  LICENSE / NOTICE       分发所需的法律文件
  Makefile               稳定开发命令入口
  compose*.yaml          默认本地 Compose 入口
  go.mod / package.json  工作区与工具链入口
  docs/
    README.md            文档导航
    SSOT.md              产品与架构事实来源
    CONTRIBUTING.md      社区健康文件
    CODE_OF_CONDUCT.md   社区健康文件
    SECURITY.md          社区健康文件
    adr/                 已接受的技术决策
    architecture/        架构说明与可视化
    assets/              README 与文档使用的稳定媒体资源
    specs/               产品规格、计划和执行材料
    evidence/            测试、评审与验收证据
    operations/          开发、部署、备份、恢复与安全手册
```

根目录只保留代码托管平台、工具链、构建、依赖、许可证和首次阅读所需的标准入口。新文档应按职责进入 `docs/`，不要把临时分析、截图或实现记录散落到根目录。

## 文档规则

1. 产品范围、术语、核心流程和技术边界只在 `SSOT.md` 中定义一次。
2. 功能规格、ADR、API 契约和运行手册必须引用对应 SSOT 条款，不复制形成第二套定义。
3. 改变产品定位、资产模型、发布治理、安全边界或兼容性承诺的决策，必须先更新 SSOT 并经过维护者审核。
4. 文档使用当前时态描述最新事实；变更历史只进入 Changelog、ADR、迁移说明或证据记录。
5. 已发布行为与 Confirmed SSOT 不一致时，该问题按阻断性文档或实现缺陷处理。

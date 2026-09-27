# 收敛版 1.0 任务图

状态：Confirmed。用户于 2026-09-26 批准具体技术计划并要求继续完成。对应 `spec.md`、`plan.md`；任务依次执行，不按页面或文案拆微任务。

## 公共执行契约

每项实施前生成 `packets/V1-T00N.yaml`，固定具体修改文件、输入、验证命令、失败复现、证据和停止条件。下列目录为最大边界，实际 packet 不必占用整个目录。未知模块缺陷先复核范围，不借验收修复做无关重构。

证据分别保存在 `docs/evidence/V1-T00N/`，包括命令、结果、变更、独立评审、风险和用户验收状态。私有环境回执、凭据、完整浏览器 trace 与模型原始响应仅存 `.semlia/v1-acceptance/`，公开证据使用脱敏结果。

所有任务默认 TDD：新增行为先运行失败用例，再修复、回归；纯基线与文档工作不伪造 RED。技术通过不自动等同用户接受。2026-09-27 用户明确委托代理浏览器验收，并授权通过后提交、推送和 PR 合并；本轮 Accepted 表示该授权下的代理验收条件已满足，不表示用户本人测试或正式发布批准。

## V1-T001 隔离环境与验收基线

- Status: Accepted（用户委托的代理验收）；T001 专项证据通过，最终 RC.6 来源及知识入口复验通过。
- Priority: P0
- Depends on: 计划及整批执行授权
- Blocks: V1-T002
- Requirements: V1-FR-001 至 V1-FR-005、V1-NFR-001、V1-NFR-003
- Parallel: No；本任务只读后端、前端、发布审查可并行，均不写代码。
- Conflicts: 所有环境初始化、演示种子和验收脚本写入。
- Allowed files: `scripts/acceptance/`、`scripts/demo/`、`examples/` 中复用或新增的合成验收文件、`tests/acceptance/`、`cmd/v1-acceptance/` 正常初始化与验收命令及测试、`docs/specs/v1-convergence/`、`docs/evidence/V1-T001/`。
- Deliverables: 专属环境与归属回执、固定数据及独立答案、可重跑初始化、按用例记录的实际基线。
- Validation: Node 生成器和生命周期单测、正常服务健康检查、来源发现和版本读回、重复初始化、原环境非侵入核对。
- DoD: 原有数据库和配置保持不变；不把预置已发布数据或协议桩结果算作冷启动模型闭环；后续修复有明确失败依据。
- Packet: `packets/V1-T001.yaml`。

## V1-T002 模型、治理与执行闭环

- Status: Accepted（用户委托的代理验收）；RC.6 固定 cohort 4/4 正例、2/2 负例，独立核对精确类别、老客、半开时间与标量 200。最终累计模型 40 次、执行 37 次，原失败及分母完整保留；性能继承 Round 7 原阈值内的 uncached 证据，不冒充 RC.6 重跑。
- Priority: P0
- Depends on: V1-T001 技术评审通过
- Blocks: V1-T003、V1-T004
- Requirements: V1-FR-002 至 V1-FR-005、V1-NFR-001、V1-NFR-003
- Parallel: No；本任务内允许持久幂等修复、真实模型验收驱动和原子 spec 纠错三个文件边界不交叉的子尝试并行，最终统一回归与评审。
- Conflicts: 语义、治理、分发、执行和生成契约改动。
- Allowed files: `internal/domain/semantic/`、`internal/domain/distribution/`、`internal/domain/execution/`、`internal/domain/governance/`、对应 `internal/application/` 与适配器、`internal/platform/http/`、相关 `tests/`、必需的 API/SDK 生成契约、`docs/evidence/V1-T002/`。Ask 持久幂等所必需的新迁移、SQL 查询及生成物、schema 版本与接线、对应版本断言，以及 `scripts/acceptance/v1*` 验收驱动属于本任务内部修复边界；不修改历史迁移或既有数据库。仅修复验收暴露且落在既有产品边界内的缺陷。
- Deliverables: 三个自然语言问题正确对账、两个负向案例拒绝或澄清、一次纠错和回滚、权限与重复执行保护、真实模型调用记录。
- Validation: 受影响 Go 包无缓存回归、真实 PostgreSQL 集成、实际模型测试、契约一致性；失败复现和修复命令逐项记录。
- DoD: 不绕过模型或治理门禁；原始失败和纠正结果分开报告；独立审查无阻断。
- Packet: `packets/V1-T002.yaml`、`packets/V1-T002-A004.yaml`、`packets/V1-T002-A005.yaml`、`packets/V1-T002-A006.yaml`、`packets/V1-T002-A007.yaml`。A007 边界明确包含目录 PostgreSQL 查询与对应生成物、适配器及检索回归，不增加产品能力。

## V1-T003 桌面闭环与恢复

- Status: Accepted（用户委托的代理验收）；A005 未知目录状态、A006 服务端搜索成员修复均独立审查通过。356 项前端测试、完整生产浏览器及 RC.6 实际 CUA 验收通过；八张两尺寸截图独立审查无阻断。
- Priority: P0
- Depends on: V1-T002 技术评审通过
- Blocks: V1-T005
- Requirements: V1-FR-001 至 V1-FR-005、V1-NFR-002
- Parallel: No；任务内前端修复与真实浏览器验收驱动可在 packet 指定的独立文件边界内并行，最终浏览器运行等待修复整合。
- Conflicts: `web/`、共享生成类型与嵌入产物。
- Allowed files: `web/src/` 受影响组件及测试、`web/e2e*`、Playwright 配置与验收启动脚本、`docs/evidence/V1-T003/`。嵌入产物由整合步骤生成。
- Deliverables: 正常登录、来源核对、知识整理、审核发布、问数结果与纠错的真实接口浏览器证据。
- Validation: 前端 typecheck、lint、Vitest；两种桌面 Playwright；键盘、焦点、刷新、返回、空/错/取消、重复提交及溢出检查。
- DoD: 不用 mock 替代真实接口主旅程；保留合成披露与授权错误；不重做视觉体系或引入移动范围。
- Packet: `packets/V1-T003.yaml`、`packets/V1-T003-A004.yaml`。

## V1-T004 消费与自托管验证

- Status: Accepted（用户委托的代理验收）；A001 至 A004 专项及受审修复通过，RC.6 同字节启动和四项嵌入浏览器预检通过；历史升级/恢复专项不冒充 RC.6 live 重跑。
- Priority: P0
- Depends on: V1-T002、V1-T003 技术评审通过
- Blocks: V1-T005
- Requirements: V1-FR-006、V1-FR-007、V1-NFR-001、V1-NFR-004
- Parallel: No；任务内的 smoke/Compose 修复和公开通道/恢复验收可在 packet 指定的独立文件边界内并行。
- Conflicts: 公开客户端、发布脚本、迁移和 Docker 环境操作。
- Allowed files: `cmd/semlia/`、`internal/adapters/mcp/`、`sdk/typescript/`、相关 `tests/`、`scripts/release/`、`scripts/ci/`、`scripts/dev/`、`scripts/acceptance/v1-release*` 与 `v1/release*` 验收工具、专属 V1 生命周期入口及测试、`deploy/` 公共示例、Makefile、必要本地 Compose 文件、`docs/evidence/V1-T004/`。不修改远程部署私有配置或工作流权限以绕过门禁。
- Deliverables: 普通机器身份的 REST/MCP/CLI 对等测试，撤销拒绝；全新安装、升级和隔离恢复；可定位的候选构建。
- Validation: 机器生命周期/通道一致性、Node 发布证明和脚本单测、迁移集成、Docker smoke、备份恢复指纹、安装后健康检查。
- DoD: 只清理自有资源；不修改既有业务库；候选产物与源码可关联。
- Packet: `packets/V1-T004.yaml`、`packets/V1-T004-A003.yaml`、`packets/V1-T004-A004.yaml`；A004 新增边界限嵌入 Web handler 及其测试。

## V1-T005 最终门禁与交付

- Status: Accepted（用户委托的代理验收）；RC.6 七项新鲜门禁与有界继承证据、固定模型 cohort、实际 CUA 均通过。Git 提交、推送与合并已获授权，须等待 PR CI 全部通过；正式签名发布不在本次范围。
- Priority: P0
- Depends on: V1-T001 至 V1-T004 技术评审通过
- Blocks: 用户最终验收
- Requirements: 全部
- Parallel: No
- Conflicts: 最终源码冻结后的任何行为或契约改动。
- Allowed files: `README.md`、`README.zh-CN.md`、`.env.example` 顶部开发入口说明注释、`docs/` 当前状态与使用说明、`internal/platform/web/static/` 生成产物、候选版本元数据和 `docs/evidence/V1-T005/`。新增行为缺陷返回对应任务修复后重跑。
- Deliverables: 最新文档、完整验收矩阵、独立审查、候选文件及本地地址、已知限制和未验证项。
- Validation: `make check-source`、`make check-browser`、`make check-smoke`、`make security-check`、既有性能基准、版本/校验和/SBOM/源码指纹核对、`git diff --check`。
- DoD: 无未解决阻断项；所有声称通过的项目有本候选证据；不声称真实业务适用性或正式发布；最终状态按用户明确委托的代理验收授权标记，Git 结果以 PR 记录为准。
- Packet: `packets/V1-T005.yaml`。

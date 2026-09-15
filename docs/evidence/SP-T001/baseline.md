# SP-T001 实测基线

## 工作区

- 记录时间：2026-09-09 04:20:39 UTC；PM 开始时间：04:16:25 UTC。
- 分支 `main`，HEAD `source-revision-redacted`，叠加当前未提交和未跟踪实现。不能仅 checkout 该提交复现本次代码。
- [checkout.json](checkout.json) 保存 `git status --short`、1,182 个已跟踪/未跟踪路径的内容 SHA-256 或缺失状态及工具版本。公开证据不包含全机容器清单、忽略文件及其秘密内容；文件摘要不是源码备份。
- Go 1.26.6 darwin/arm64，Node 24.15.0，pnpm 11.1.3；Docker 29.4.0，OrbStack Linux daemon。
- 工作区既有应用、迁移、前端和生成物改动全部保留。T001 新增设计文档、静态契约测试与本次证据，不修改业务实现。

## 新鲜结果

| 命令组 | 实际时间 UTC | 墙钟时间 | 结果 |
| --- | --- | --- | --- |
| application discovery/ingestion/governance 与 platform/http | 04:20:58.886 至 04:21:05.406 | 6.520 秒 | 4 packages 通过；161 test/subtest pass，0 fail，0 skip；其中 117 个顶层测试 |
| integration ingestion/discovery/governance/projection | 04:21:30.475 至 04:23:59.548 | 149.073 秒 | 4 packages 通过；85 test/subtest pass，0 fail，1 skip；其中 81 个顶层测试通过 |

两组均明确使用 `-count=1`，不把 Go 缓存结果算作新鲜运行；`-json` 只用于保存结构化结果。命令墙钟合计 **155.593 秒，约 2 分 36 秒**，不包括环境预检、契约测试、设计与评审；不能将 package 内部时间与墙钟时间相加。

唯一 skip 是 `TestExecutionCrashWorker`，原因 `subprocess-only proof`。该入口供父测试启动带专用环境的子进程，直接运行按实现约定跳过；仍明确记为 skip，不伪记为通过。既有 `LiveProvider` 测试使用本地协议替身，不是实际模型调用。

原始证据：[unit 命令结果](baseline-unit.result.json)、[unit JSONL](baseline-unit.jsonl)、[integration 命令结果](baseline-integration.result.json)、[integration JSONL](baseline-integration.jsonl)。stderr 单独保存为同名前缀 `.stderr.log`。

## 数据库隔离

读取四个包的 `TestMain` 后确认：默认均由 testcontainers 启动该包拥有的 `postgres:18-alpine` 临时实例，动态映射端口，迁移只施加在返回连接上，并在结束时删除自身容器。ingestion 存在显式数据库覆盖，governance 存在 crash-helper 复用连接分支，因此实际启动时清除以下环境变量：

```text
SEMLIA_INGESTION_TEST_DATABASE_URL
SEMLIA_EXECUTION_CRASH_HELPER
SEMLIA_EXECUTION_TEST_DB
SEMLIA_DATABASE_URL
```

没有启动/停止默认 Compose、访问部署服务器或客户数据库，也没有运行 `make check`、`make dev` 或全局 Docker 清理。[environment-after.json](environment-after.json) 仅公开本次测试计数，不包含全机容器身份记录。

## 复用判断与缺口

现有输入处理、扫描、结构校验、提案、独立审核、单目标发布、回滚及投影具备可运行基线，当前没有暴露需先修改应用的基础失败。详细 [SP-AC 覆盖矩阵](coverage.md) 列出已有证据和仍需新增的生产断言；这不等于十二项生产验收通过。

实际目标词汇共有五类：`semantic_asset`、`physical_binding`、`model_grain`、`entity_key`、`join_contract`。`physical_binding` 由 `internal/domain/governance/object.go` 和 migration `000007` 扩展，不能仅依据 `proposal.go` 或 migration `000006` 推断最终 schema。物理绑定保留既有独立治理权威。

## 后续验收环境

SP-T002 至 SP-T005 的数据库验证继续使用 task-owned testcontainers。迁移 down/up 只在本次生成的隔离数据库运行，必须含拒绝有损降级用例。

SP-T006 的桌面验收须提供独立 harness，显式持有 Compose project、环境文件、数据库/卷、端口和镜像标签；测试准备、运行和清理均校验所有权。原有 live E2E 通过默认 Compose 写入样例，只更换浏览器 URL 不构成隔离。目标 viewport 为 1440x900、1024x768，不运行移动验收。

SP-T007 的完整门禁在专用检出和专用环境中执行。独立工作目录或 Compose project 不隔离全局镜像标签；若脚本不能覆盖标签，应使用隔离 Docker 环境。实际模型须先明确配置、授权和费用上限；本次不读取密钥、不调用实际模型。10,000 资产性能、全量构建、完整消费兼容性及浏览器验收均未在 T001 运行，不报告为通过。

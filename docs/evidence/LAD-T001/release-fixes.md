# 发布检查整改证据

日期：2026-09-14。范围：发布元数据、默认浏览器验收、发布工作流依赖及相关回归检查。不包含部署、Git 提交、推送或打标签。

## 实现

- `internal/platform/schema.MigrationVersion` 是服务就绪检查与发布清单生成、验证共用的迁移版本。测试将该常量与迁移目录的最新版本比较，当前为 `29`。
- 默认 `pnpm --dir web test:e2e` 与 `make check-browser` 使用正常密码登录入口，验证本机账号生命周期和隔离语义生产流程。测试服务不提供直接发放会话的登录捷径。
- 浏览器验收使用独立数据库、随机合成密码和分离的管理员、作者、审核人、发布人；模型为协议桩，不调用真实付费模型。角色复用通过正常登录取得的会话，不绕过密码限流。
- 发布构建依赖同一提交的可复用 CI 与安全工作流。CI 包含源码、正常浏览器和容器 smoke；产物上传位于依赖通过后的构建任务，证明任务依赖构建任务。
- Operations 的内存 API 位于 `web/src/testing/`，单测通过 API 注入使用；正常 Provider 不提供 Fixture 开关。
- 内嵌前端产物与当前源码一致。
- 发布脚本拒绝与 `go.mod` 声明不一致的实际编译器，避免 `GOTOOLCHAIN=local` 使用旧宿主工具链。每次打包使用独立暂存目录，扫描器仅挂载候选输入，避免复用被删除的暂存目录及宿主挂载缓存。
- CycloneDX 合并后仅移除没有分数和向量的评分项中的 `method: "Null"` 序列化哨兵，恢复原始扫描中缺省可选方法的含义。漏洞 ID、受影响组件、严重度、有效评分及依赖图保持不变，随后仍执行完整 schema 验证；有分数或向量时拒绝此归一化。

## 验证

- `make check-source` 通过：格式、lint、类型检查、Go 测试、前端 27 文件共 230 测试、契约/SQLC/前端嵌入产物一致性及二进制构建。日志：`.semlia/evidence-work/release-fix-source-final.log` 和补充打包修复后的 `.semlia/evidence-work/release-fix-source-pinned.log`。
- `go test ./tests/repository ./scripts/release ./internal/platform/schema -count=1` 通过。
- `make check-browser` 通过：正常账号创建、停用、改密及旧密码拒绝；语义生产 4 项测试覆盖 `1440x900` 和 `1024x768`。日志：`.semlia/evidence-work/release-fix-browser-final.log`；隔离证据：`.semlia/production-acceptance/spacc_7e95590974010d6d`。
- 生产密码验收还独立通过一轮：`.semlia/evidence-work/release-fix-password-production.log`。
- `make check-smoke` 通过并清理本轮独立容器、卷和镜像。日志：`.semlia/evidence-work/release-fix-smoke-final.log`。
- `make security-check` 通过，依赖/密钥扫描及发布镜像 HIGH/CRITICAL 门禁没有发现阻断项。日志：`.semlia/evidence-work/release-fix-security-retry.log` 和补充打包修复后的 `.semlia/evidence-work/release-fix-security-verified.log`。首轮漏洞库下载超过默认 5 分钟后失败；确认归属后停止本轮后续扫描容器，独立以 20 分钟网络等待预下载同一漏洞库，再运行未修改阈值的原安全门禁。预下载记录：`.semlia/evidence-work/release-fix-db-preload.log`。
- 当前本机监督进程、Web 代理与 API 就绪检查通过，既有数据库与运行服务保持原状。
- `make release GO="$(go env GOROOT)/bin/go"` 通过，包含 SBOM 合并/schema 验证、依赖覆盖、归档内容及 SHA-256 检查。日志：`.semlia/evidence-work/release-fix-bundle-isolated.log`。候选清单记录 `go1.26.6`、迁移 `29`、`sourceDirty: true`、`local_candidate` 和 `unreviewed`。
- 工具链拒绝及 SBOM 归一化回归测试通过，资源标签合同仍通过。早期打包失败证据保留，不以重跑覆盖原记录。

## 发布边界

- 本地验证不能替代冻结提交在 GitHub 上实际运行的 CI 结果；本次未执行远端工作流。
- 工作区有大量既有未提交改动，尚未完成发布提交的逐项归集与冻结。本地构建仍标记为 `local_candidate`、`unreviewed`，不能据此宣布正式发布。
- 旧 `web/e2e/` 原型测试保留但不属于正常默认验收；不能将本轮正常流程验证声称为这些旧用例的逐项迁移。
- ProductApp、Catalog 和 Authorization 的剩余测试数据/原型分支清理属于 LAD-T001 未完成项。本轮不将 LAD-T001 标记为 Accepted。
- 前端 bundle 大小警告保留，未通过放宽阈值隐藏。

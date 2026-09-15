# 私有仓库发布与 maco 展示部署

用户授权：修复调度测试隔离，采用私有仓库兼容发布证明，通过 PR 合并及验收后部署 main。rc.1/rc.2 tag 不覆盖；后续候选版本为 rc.3。

发布执行采用用户授权的本机构建，不依赖 GitHub Actions 额度。PR 以对应提交的完整本地源码、浏览器、冒烟和安全验收记录作为合并依据；GitHub 发布工作流仅手动触发。本地签名收据使用 `urn:semlia:local-release:<UUID>` 标识，覆盖目标 Linux 镜像归档、镜像 digest/commit、CycloneDX SBOM 和验证记录，不冒充 GitHub 托管构建。四平台发布包不在本次 maco 单平台部署范围。

本机构建使用正式 release 相同的 `CGO_ENABLED=0` Linux/amd64 Go 编译及内嵌前端，将二进制和 migrations 放入 `build/maco/`，通过 `deploy/maco.Dockerfile` 包装为固定基础镜像的非 root 容器。最终镜像必须另行经过容器冒烟与安全扫描，不能用另一架构镜像的检查结果代替。

镜像通过本机临时回环 registry 和 SSH 反向转发传输，maco 按 digest 拉取并与签名记录核对。应用使用 `pull_policy: never` 保留已验证的本地镜像，不依赖临时 registry 常驻；恢复时重新验证归档并传输相同 digest。传输完成后关闭临时 registry 和反向转发，不修改共享网络服务。

## 验证合同

- 调度集成测试共享隔离 PostgreSQL，但每个 fixture 退出时只停用自身工作区的计划，清除下次执行和租约字段，保留产品全局调度行为与精确计数断言。泄漏回归修复前失败。
- 发布证明采用持久 Ed25519 项目签名密钥，私钥仅位于本机受限凭据目录及本仓库 Actions secret，公钥固定于 `scripts/release/signing-public.pem`。CI 签名覆盖四个平台发布包、SBOM、校验文件、镜像 digest，以及仓库、版本、commit、构建运行地址。签名后立即使用固定公钥验证，失败阻断发布。
- 这是项目维护者密钥签名的发布收据，不是 GitHub/Sigstore keyless attestation，不声称独立第三方来源保证。验证时必须使用可信 main 中的公钥，不能信任下载包附带的新公钥。密钥轮换需独立审查；签名密钥或有权修改发布工作流的账号失陷会破坏信任边界。
- 发布凭据仅用于 tag 构建后的签名步骤；镜像发布仅该 job 获得 packages:write。失败不降级为无签名发布。

验证命令：`node scripts/release/proof.mjs verify <下载根目录> <tag> <完整commit>`。下载四个 bundle、镜像身份及证明 artifact 时保留目录名称，证明的两个文件置于根目录。

## maco 范围

`deploy/maco.json` / `deploy/maco.compose.yaml` 定义 `semlia-test`，仅在共享 pg-main 创建独立数据库及角色，不使用 Redis。server/worker 各限制 8 个数据库连接，运行角色上限 20；迁移使用独立单次任务，迁移角色上限 5。运行凭据和迁移凭据分离，迁移 URL 不携带仅适用于 pgxpool 的连接池参数。

展示入口为 `https://app.semlia.com`，使用用户配置的 Cloudflare Tunnel 转发至 maco 的 80 端口，再由 `deploy/maco.nginx.conf` 代理至 `127.0.0.1:18086`。站点文件安装于 `/opt/1panel/www/conf.d/semlia-test.conf`，独立维护，不在 1Panel 网站列表登记，不启用面板 API。源站仅接受回环连接，应用仅允许 `https://app.semlia.com` 来源，使用 Secure、HttpOnly 会话 Cookie。公网不暴露健康检查和 metrics，响应禁止缓存及搜索索引。

展示服务使用 password 认证，不声明为开放注册的生产 SaaS。Cloudflare Access 未配置，Tunnel 不构成访客身份限制；简单管理员密码存在公网猜测风险，建议配置 Access 保护。初始 admin 通过容器 bootstrap 命令建立；不将本地原始数据或密码打进镜像。HTTP SSH 转发仅用于运维健康检查，登录通过 HTTPS 域名完成。

部署需重新检查 COS 备份、平台基线、配额、挂载权限及精确 resolved Compose；迁移和应用均使用同一签名镜像 digest。初次部署无旧镜像，不回滚共享实例或删除数据。最终运行、签名和部署证据以 PR/发布记录补充。

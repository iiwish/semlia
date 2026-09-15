# 私有仓库发布与 maco 展示部署

用户授权：修复调度测试隔离，采用私有仓库兼容发布证明，通过 PR 合并及验收后部署 main。rc.1/rc.2 tag 不覆盖；后续候选版本为 rc.3。

## 验证合同

- 调度集成测试共享隔离 PostgreSQL，但每个 fixture 退出时只停用自身工作区的计划，清除下次执行和租约字段，保留产品全局调度行为与精确计数断言。泄漏回归修复前失败。
- 发布证明采用持久 Ed25519 项目签名密钥，私钥仅位于本机受限凭据目录及本仓库 Actions secret，公钥固定于 `scripts/release/signing-public.pem`。CI 签名覆盖四个平台发布包、SBOM、校验文件、镜像 digest，以及仓库、版本、commit、构建运行地址。签名后立即使用固定公钥验证，失败阻断发布。
- 这是项目维护者密钥签名的发布收据，不是 GitHub/Sigstore keyless attestation，不声称独立第三方来源保证。验证时必须使用可信 main 中的公钥，不能信任下载包附带的新公钥。密钥轮换需独立审查；签名密钥或有权修改发布工作流的账号失陷会破坏信任边界。
- 发布凭据仅用于 tag 构建后的签名步骤；镜像发布仅该 job 获得 packages:write。失败不降级为无签名发布。

验证命令：`node scripts/release/proof.mjs verify <下载根目录> <tag> <完整commit>`。下载四个 bundle、镜像身份及证明 artifact 时保留目录名称，证明的两个文件置于根目录。

## maco 范围

`deploy/maco.json` / `deploy/maco.compose.yaml` 定义 `semlia-test`，仅在共享 pg-main 创建独立数据库及角色，不使用 Redis。server/worker 各限制 8 个数据库连接，迁移最多 3 个；运行凭据和迁移凭据分离。仅发布 loopback 端口，以 SSH 转发检查，不增加公网域名、DNS、Tunnel 或 Access 配置。

展示服务使用 password 认证和 development cookie 配置，适用于受控 HTTP 转发，不声明为公网生产 SaaS。初始 admin 通过容器 bootstrap 命令建立；不将本地原始数据或密码打进镜像。

部署需重新检查 COS 备份、平台基线、配额、挂载权限及精确 resolved Compose；迁移和应用均使用同一签名镜像 digest。初次部署无旧镜像，不回滚共享实例或删除数据。最终运行、签名和部署证据以 PR/发布记录补充。

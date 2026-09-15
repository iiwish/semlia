# 本机服务切换验收

日期：2026-09-12。范围：将旧应用容器替换为本机 Go server、worker 和 Vite，复用现有 OrbStack PostgreSQL。

## 运行状态

- 地址：`http://127.0.0.1:18081/`；API 仅监听 `127.0.0.1:18080`。
- 管理员登录名：`admin`，绑定现有 `semantic-core`（Semantic Core）工作区。密码按用户指定值通过终端隐藏 stdin 初始化，不记录在源码、日志或证据中。
- 本地账号支持普通用户名及邮箱格式，不区分大小写；开发和生产统一至少 6 字符。Argon2id、登录限流、CSRF、会话撤销保护保留。
- `make dev-down` / `make dev` 停止及重启验证通过；数据库持续运行。浏览器实际登录并刷新，服务重启后会话有效。

## 数据保护

- 最终备份：`.semlia/backups/LAD-T001-cutover-1789226140147`，包含停写后的数据库 dump、内容卷、工件卷、私有配置、容器清单及 SHA-256。
- `restore-proof.json` 证明独立恢复及 schema 21 → 29 迁移成功，临时数据库已清理；随后现有库迁移至 29，非 dirty。
- `preservation-proof.json`：工作区 153、语义资产 56、模型供应商 1、模型设置 2 均保留；模型供应商和设置行的摘要一致。
- 内容/工件分别复制至 `.semlia/native/content` 和 `.semlia/native/artifacts`。原加密密钥保持不变，模型凭据保存在权限 0600 的 `.semlia/native.env`。未发起模型调用。
- 保留原 PostgreSQL 容器 `cd0dac4bcf38`、自定义 PostgreSQL 镜像、全部原数据卷和备份。

## 精确清理

- server：`c730f3eb928e`。
- worker：`b271bdc288e7`。
- 原应用镜像：`semlia:local`，`sha256:0f155c13e2d4e198430f4ad9a049cb21760b7505bc0fc944a52f198b3a51d1e2`。
- 清理前确认两个已停止容器是该镜像的唯一容器引用；清理后本机 readiness 通过。未执行全局 prune。

## 验证边界

用户名规范化测试实际 RED/GREEN；identity/HTTP/隔离 PostgreSQL 身份测试通过，前端相关 8 项测试通过。此记录确认本次旧服务切换，不代表 LAD-T001 剩余原型分支清理已完成或用户已接受整体任务。

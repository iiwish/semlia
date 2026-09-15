# v0.1.0-rc.1 候选版本

日期：2026-09-14。用途：受保护的简历展示环境候选版，不是生产 SaaS 上线批准。

## 固定范围

- 单一正常应用运行方式，单个 admin 展示入口。
- 确定性合成零售来源、5 个语义资产、真实验证和独立审核历史、3 条发布及回滚清单。
- 目录 displayName / ownerPrincipalId 兼容修复及回归测试。
- 初始化、生命周期、单账号核验和桌面验收脚本。

源码与部署前验收的非文档候选摘要一致：`653548cc381081bf7c8f234c9c1d17c22190da3abea11f4f7fc3e896e9162c13`。完整门禁、源码安全扫描和浏览器证据见 `predeploy-acceptance.md`。本次冻结前正常 API 和来源 SQL 核验再次通过。

## 本机数据快照

路径：`.semlia/resume-demo/v0.1.0-rc.1-backup/`，目录权限 0700，不纳入 Git 或远程附件。快照在停止专属演示 server/worker 后生成，不停止 PostgreSQL 或既有开发服务。

| 文件 | SHA-256 |
| --- | --- |
| app.dump | c3f7d4cb7fde3feca9cf5b5e62bec4292f9a69ee122415ee15c9ce36d07952c2 |
| source.dump | 82239ac9ca8597cd844999faf88b7113588685a1e76791045bdcbcdae75e8889 |
| runtime.tar.gz | 8ab94706bbc6f95f727040163327f27ebf8c08fe558cb26146cef436f184239b |

两个 PostgreSQL 自定义格式备份均通过 `pg_restore --list`，不包含数据库角色定义和 ACL。运行材料包括内容、来源工件、状态及受保护配置；含敏感值，禁止公开。此检查不替代目标服务器上的实际恢复演练。

## 发布边界

Git annotated tag `v0.1.0-rc.1` 固定源码提交。部署必须从该 tag 的提交构建，不从继续变化的工作树构建。镜像摘要、目标数据库恢复、HTTPS、入口保护和服务器回滚验收在部署阶段完成；本次不部署应用。固定弱密码管理员入口不能裸露公网。

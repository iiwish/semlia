# Full-Menu Beta 0.2.0 交付报告

状态：本地交付与验证完成，Needs_Review；正式 Beta 发布尚未 Accepted。
验收日期：2026-09-05。用户授权超时收尾，并确认没有服务器，先完成本地验收。

## 本地交付

真实受治理发布、不可变物理快照、正式 JoinContract、高精度持久化计划和
受限只读 PostgreSQL 执行通过同一服务接通 REST、MCP、CLI、TypeScript SDK
和 Ask。当前授权、凭据撤销、来源标识、限额、取消、并发幂等、真实进程失联
及仅元数据恢复均有实际 PostgreSQL 证据，运行记录不保存结果行或原始 SQL。

Ask 使用实际应用服务和 PostgreSQL，Chat 明确为本地确定性提供方。1440x900
和1024x768验证覆盖键盘、缩减动画、无重叠、刷新后不保留事实行。成员页面
保留位移动画并保持文字不透明，真实动画中途的 Axe 检查通过。

## 验证结果

- 最终 `make check-source`：格式、lint、类型、全部测试、契约/SQL/embed漂移和构建通过。
- 前端22个测试文件、183项测试通过；本轮早期真实全量 Go 集成及无缓存验收通过。
- 常规桌面26/26、登录桌面6/6、真实客户端生命周期桌面2/2通过。
- 生产镜像及依赖/秘密扫描通过，HIGH/CRITICAL漏洞为0。
- migration21升级、server/worker健康、冒烟测试通过；空库降级/重升及非空降级保护有实际证据。
- 最终候选包的完整Go/Web依赖、CycloneDX、来源/归档一致性和SHA256验证通过。

## 候选包

版本：`0.2.0-beta.1-a002.1`，darwin-arm64，Go1.26.6，migration21。
路径：`build/release/fmb-t007-a002-overtime/`。
来源指纹：`sha256:dc81f75b93285a9c7d894b165336ed6863d7a394cb8138ffec483f98be15d4ce`。
最终源码指纹与候选记录完全一致。实际二进制报告相同版本，缺少身份服务时
显示登录失败状态，没有本地UAT工作区回退。来源明确记录dirty本地工作树、
`local_candidate`及`unreviewed`，不是签名或正式接受的发布。

## 环境与边界

本地预览：`http://127.0.0.1:18081`，最终开发镜像、migration21且dirty=false，
显式启用UAT。开发环境的持久化buildVersion为`local-2e53e6f`，不代表发布包
版本；实际镜像ID见最终证据。升级前私有备份为0600权限，仅验证归档可读，
不将该检查表述为恢复验收。

托管HTTPS/OIDC、真实Chat/Embedding、外部Webhook/只读数据源、OTel及托管恢复
按用户指示暂缓，不能用本地测试替代。历史单次发布HTTP422未再复现但原因未
完全确定；宿主高负载时的测试超时和早期失败日志全部保留，没有降低断言。
现有三条非阻断lint警告及Web包体积警告仍存在。

最终证据：`docs/evidence/FMB-T007/attempts/a002/overtime/summary.md`。
A001、A002及T004-T006冻结补丁保持原哈希。六个浏览器文件的A002基线捕获缺口
明确保留；超时修复另有两文件精确diff和哈希，不伪造历史归属。
FMB-T001至T007仍需用户正式接受。无commit、push、外部部署或破坏性卷操作。

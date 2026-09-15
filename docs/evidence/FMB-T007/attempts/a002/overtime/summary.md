# FMB-T007 授权超时收尾

状态：本地目标完成，Needs_Review；正式 Beta 未 Accepted。
用户于2026-09-05明确授权超时继续，并批准无服务器条件下先完成本地收尾。

范围：成员状态对比度缺陷、受影响桌面回归、完整源码门禁、生产安全扫描及
与最终源码一致的发布候选包。单一 worker 实现，root 独立审查和验证。
不扩展产品功能，不覆盖已冻结的 A001/A002 证据，不提交或部署外部环境。

原始失败：`../root-auth-desktop-final.log`，登录桌面测试5/6通过，紧凑桌面
`.member-state-active` 对比度4.31:1低于4.5:1。禁止豁免 Axe 断言。

用户确认目前没有服务器，批准先完成全部本地测试和收尾。托管 HTTPS、OIDC、
真实 Chat/Embedding、外部 Webhook、只读数据源、OTel 和恢复验收列为未执行，
不阻塞本次本地收尾，也不能以本地测试冒充正式上线验收。

## 独立复核与门禁

根因是成员页面祖先的透明度动画；徽标声明颜色没有错误。两尺寸均通过暂停
真实180ms动画至60ms的确定性RED复现。最小实现仅增加成员页面专属的位移
动画，保留原颜色、密度、布局、键盘和缩减动画。root复核两文件diff及
1440/1024截图，无新增阻断问题。详情见`auth-contrast-implementation.md`。

| 验收项 | 权威证据 | 结果 |
| --- | --- | --- |
| 完整源码门禁 | `root-check-source-final.log` | exit0；Go/Web全部测试、格式、lint、类型、契约/SQL/embed漂移、构建通过；Web22文件183项 |
| 两种桌面尺寸 | `root-fixture-desktop.log`、`root-auth-desktop.log`、`root-live-desktop.log` | 26/26、6/6、2/2通过；无重试或跳过 |
| 生产安全 | `root-security.log`、`filesystem.txt`、`container.txt` | exit0；依赖/秘密和镜像HIGH/CRITICAL扫描无发现 |
| 精确候选包 | `root-release.log`、`release.json`、`SHA256SUMS` | exit0；双实扫、官方CycloneDX合并/schema及严格Go/Web覆盖、归档一致性通过 |
| 升级与运行 | `root-dev.log`、`root-smoke.log` | exit0；server/worker健康，实际schema_migrations为21且dirty=false；smoke86.323s |
| 发布到执行及恢复 | `../root-check-source-final.log`、`../live-ask-shell-bounded.log`、`../migration21-lifecycle-pass.log`及当前源码集成测试 | 实际PG发布/解析/执行、通道摘要一致、当前授权、只读/限额/取消/幂等/进程失联/隐私及迁移保护；后端未受本次CSS修复影响 |

`root-remaining-gates.log`逐项记录实际起止时间和exit0。`root-check-source-initial.log`
保留证据文档含个人绝对路径的失败；文档使用可移植路径后该门禁通过。
`root-check-source.log`保留高负载期间单个15秒长流程超时（182/183）；最终
重跑全绿，测试时限与断言未放宽。原始历史HTTP422及其他A002失败仍保留。

## 运行与交付

候选版本：`0.2.0-beta.1-a002.1`，Go1.26.6、darwin-arm64、migration21。
最终源码指纹与release.json完全一致：
`sha256:dc81f75b93285a9c7d894b165336ed6863d7a394cb8138ffec483f98be15d4ce`。
归档SHA256：`906075c3443564a121371f22d3968c3eaf8e6e27d66ff645bcc39a9d9fc0ca9b`。
SBOM SHA256：`e108c097d1de5fa128913a0fab88f87f6c21052b51d83a822d5d1e5b6f534bb2`。
root再次执行SHA256SUMS核验，两项均OK。来源明确为dirty/unreviewed local_candidate。
实际staging二进制以干净环境启动于localhost18090，system/info返回该候选版本，
session返回503；实际浏览器截图和DOM确认仅登录失败界面，无本地UAT回退。
该探针已SIGINT退出0，不遗留运行进程。浏览器探针不是完整OIDC验收。

开发镜像：`sha256:138b5044c0bbc408fd39546731b250bf2fdf59d7447ea0c102ef752da7a9c21e`。
扫描生产镜像：`sha256:720d0e79e862a6c0c3d4f934a1b57b23ebdbe6c90470d3a931e5f8c2367e6f0f`。
本地预览保留于http://127.0.0.1:18081，实际readiness为ready，显式UAT环境；
持久化开发buildVersion不作为候选版本证明。没有提交、推送、外部部署或删除用户数据。

超时实现diff为`diff.patch`，SHA256为797007688051be90e630edeb1d08883f97a03055e8b4a4a37edbff9aacf75fae；
两个手工源码文件的前后哈希记录于`source-manifest.json`，生成embed由发布来源指纹覆盖。
A002冻结补丁仍为8ef1a1eaa921c62b9ce75d8761e39cfa4ee68a6a6ceb35e934fd481527df9edb；
T004-T006及A001补丁也与冻结哈希一致。最终来源指纹不依赖历史基线捕获缺口。

# 正常运行模式与本地冻结

日期：2026-09-14。状态：本地完整门禁通过，候选快照冻结；不是发布批准。

## 范围

- 用户批准本次原型清理和累计未提交改动整理、冻结。直接执行，不委派、不推送、不打发布标签、不部署。
- 冻结基线为 `63241d5300cbffc0f62bc842cac132aa13560076`。累计内容包括 M2、Alpha、FMB、SP 及 LAD 的同项目实现、契约、迁移、测试与交付证据；不是把这些既有实现声明成本轮新增。
- `codex/core-product-loop` 指向 `2f8f613`，已用 `git merge-base --is-ancestor codex/core-product-loop main` 确认完整包含于 main，再通过 `git branch -d` 删除本地引用。提交历史保留，未操作远程分支。
- 本机 `.semlia/` 配置、备份及执行日志保持原地；`.omo/run-continuation/`、根目录临时二进制和浏览器原始 trace/last-run 状态不纳入 Git。历史评审证据按历史记录保留，不作为本次通过证明。

## 正常运行路径

- ProductApp 只接真实来源及成员管理页面，删除模拟成员目录、发现计时器及假成功操作。
- Catalog 和 Authorization 不暴露 fixture 模式；正常入口不导入测试数据。测试替身位于 `web/src/testing/`，通过单测专属 Provider/API 注入。
- CapabilityProvider 未显式提供会话时权限为空。会话权限只来自服务器 capabilities，不根据角色名或测试环境推导管理员权限。
- 最近资产使用返回的资产顺序，不硬编码演示资产 ID。原型专用页面、模式说明和死 CSS 已删除。

## 回归覆盖

- `normalSurface.test.tsx` 对原型分支移除有实际 RED/GREEN；`normalRuntime.test.tsx` 保留遗留环境变量不能开启假身份的断言。
- 成员目录断言改为服务响应、搜索及失败行为；账号生命周期由 session/password 单测与真实密码浏览器验收覆盖。
- 来源及计划操作由 LiveSourcesView/ingestion 单测和隔离生产浏览器流程覆盖，不保留原型定时器和内存 CRUD 的成功假象。
- 旧 `product-journey.spec.ts` 依赖原型数据和模式，已退役；不是逐条等价移植。正常目录导航、权限、稳定最近排序、治理与兼容性保留组件回归；真实来源到分离账号审核、发布、回滚由 production 浏览器套件覆盖。
- 系统状态四项用例迁入默认正常浏览器套件，两种桌面尺寸继续验证健康、错误、键盘刷新、可见焦点和 reduced motion。状态错误用例显式拦截 HTTP，不冒充真实服务故障。

## 验证记录

- `GOFLAGS=-p=1 VITEST_MAX_WORKERS=2 make check-source`：退出 0，覆盖 format、lint、typecheck、Go/SDK/前端测试、契约/SQLC/嵌入漂移和构建。前端 28 文件、233 项测试通过；部分 Go 测试命中缓存，不声明全部重新执行。
- `make check-browser`：退出 0。原生密码、成员管理、改密闭环通过；正常生产和状态浏览器套件 12/12 通过，覆盖 1440×900 和 1024×768。隔离生产证据目录 `.semlia/production-acceptance/spacc_418d9e44c463364b`，原生证据目录 `.semlia/evidence-work/LAD-T001-native-1789369528198`。
- `make check-smoke`：退出 0，157 秒完成，隔离 Compose 资源及自有镜像已清理。
- `make security-check`：退出 0，依赖、secret 与应用镜像扫描通过，HIGH/CRITICAL 为 0；未降低阈值或跳过扫描。
- 最后仅对 6 个源码文件清理空白，随后 `make web-embed-check` 退出 0；产物不变。`git diff --cached --check -- . ':!docs'` 通过。历史 docs 的 Markdown 双空格换行、原始 diff/日志空白保持原样，仓库级 whitespace 检查存在这些历史记录提示，未篡改证据来消除它们。
- 正常构建产物未检出 `EMP-10001`、`createAuthorizationFixtureApi`、`fixtureGovernance`、`fixtureDefaultAccess`、`fixtureAssets`、`RUN-SESSION`。已检查本轮桌面和紧凑桌面截图。
- `node scripts/dev/native.mjs status`：本机 supervisor、Web proxy、API readiness 通过。原运行数据库与账号未被验收修改。

原始日志位于本机 `.semlia/evidence-work/freeze-*.log`，不包含在提交中。冻结提交对象本身记录完整文件清单和快照；候选构建清单的 commit/sourceDigest 是后续制品核对依据。

## 发布边界

本地冻结不等于远程同提交 CI、人工验收或正式发布完成。密码最低 6 字符遵从用户明确要求，其他认证保护保留；已有 lint 警告和 bundle 大小提示独立披露。

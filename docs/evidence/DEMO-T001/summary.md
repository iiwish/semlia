# DEMO-T001 执行证据

## 单账号交付核验

用户要求简化为 `admin` 后，交付入口仅提供该账号和一个成果展示工作区。正常成员接口创建管理员并挂起旧成员，正式密码重置命令撤销全部旧会话、废弃旧账号密码。隐藏工作区的最后管理员保留产品要求的成员记录，口令随机化且不保存。历史作者、审核者和发布清单未删除或改写。

正常 API 新鲜核验：admin 可见 1 个工作区、5 个资产、3 条发布；代表性的旧作者及三个旧管理员使用已知旧密码均返回 401。运行时重启不重新创建旧管理员，CLI 阻止多账号初始化及旧密码重置。针对性测试 4/4 通过；下文多账号浏览器及全门禁为该历史验收阶段证据，不代表单账号调整后重跑了完整门禁。

状态：Needs_Review。目标 1 实现与验证完成，等待用户验收。用户于 2026-09-14 批准实施计划及受限目录修复。直接执行，未委派、提交、推送或部署。

## 已实现并验证

- 确定性生成器：300 名合成客户、30 个商品、3000 笔订单、关联明细与退款；固定 90 天区间、整数分金额。
- 独立核对外键、订单金额、退款上限及支付/净收入汇总，固定种子重复生成一致。
- 数据包归属记录、输入摘要校验、修改后拒绝覆盖；检查点拒绝陈旧写入，目录与记录限制权限，拒绝符号链接。
- 正常密码会话 HTTP 客户端：真实路由、Cookie、CSRF、幂等请求头，拒绝外部地址和重定向，错误不输出服务端敏感诊断。不伪造主体请求头。
- 生成器、状态、数据包和 HTTP 客户端均先执行缺失模块 RED，再实现 GREEN；补充正常登录完整路径断言发现路径错误，修复后通过。macOS 临时目录含系统符号链接，测试使用 canonical tmpdir，保留拒绝链接的保护。
- `node --test examples/resume-demo/*.test.mjs scripts/demo/*.test.mjs`：31/31 通过，退出码 0。覆盖生成器、归属、真实会话协议、会话恢复、发现精确绑定、目录分页、现场区空状态核验、生产声明及纠正基线、来源导入失败和生命周期清理归属。
- `node scripts/demo/cli.mjs generate` 连续执行两次成功，摘要均为 `sha256:c1a1db1588cc88eb873776932a019f26afb1cd563925b459a95569d26c83e0ec`。

公开输入材料位于本机 `.semlia/resume-demo/retail-v1/`；独立的 `secrets.json` 为 0600，本机随机配置不纳入仓库或证据。

## 独立数据库进展

- 数据库归属及正常服务配置隔离单测先 RED 后 GREEN；不继承开发应用环境、模型配置或第三方 API key。
- 角色与数据库归属验证先 RED 后 GREEN，拒绝无匹配标记、外部 owner 和高权限角色。Node 测试累计 12/12 通过，Go `TestResumeDemoDataAndSafety` 新鲜运行通过。
- 已创建专用非高权限角色 `semlia_demo_e3b373fd77fee93b` 及 `_app`、`_source` 两个独立数据库。未改现有角色权限或实例设置；所有权标记固定输入摘要，拒绝接管不明数据库。重复 provision 两次通过。
- 合成数据已在专属 `_source` 库事务导入，receipt 与数据同事务提交。连续两次 `seed-source` 均通过数据库计数、金额关系与独立 SQL 汇总核对，不重复导入。
- 实际结果：2400 笔已支付订单，支付金额 113329929 分，成功退款 3158971 分，净收入 110170958 分。`verify` 使用只读 SQL 事务重新核对 receipt、计数、金额与关系。失败注入单测证明数据和 receipt 同事务提交、导入失败立即停止，并拒绝非空无 receipt 的数据库；未模拟 PostgreSQL 主机断电。
- 原本机服务 `node scripts/dev/native.mjs status` 新鲜执行通过。独立 `_app` 数据库已应用正常迁移，正常 server/worker 原生运行于 `http://127.0.0.1:64351/`。

## 正常接口闭环

- 正式管理员 CLI 初始化三个工作区；正常密码会话创建作者、审核者、发布者并分配权限。成员与来源初始化重跑通过。权限返回的 workspace scope 内部 UUID 与公开 workspaceId 不同，初始化器使用返回的 workspaceId 判断已有绑定，避免重复创建。
- SQL 发现工件采用五张表的 DDL 和普通注释。生成数据包与独立 SQL 对账材料摘要保持不变；发现工件有独立摘要，作为正式来源版本 2 注册。成果展示区首次含 `COMMENT ON TABLE` 的发现保留为 degraded 历史，没有忽略不完整覆盖。
- 展示区与验收副本的成功发现均产生 complete/verified 快照、27 个成员及 5 个候选。发现脚本只处理这两个工作区，不处理现场操作区。
- `baseline` 实际执行退出码 0：两个工作区各发布 5 个语义资产及 13 个配套对象；5 个语义资产都有人工业务确认，验证成功，作者自审收到 HTTP 403，独立审核者批准，独立发布者发布。未使用模型或测试主体注入。
- `correction` 实际执行及随后的完整重复执行均退出码 0，返回相同的纠正发布与回滚发布 ID。首次未确认退款规则的验证被 `PRODUCTION_BUSINESS_RULE_UNCONFIRMED` 阻断；失败尝试不提供可发布 seal。使用真实基线 seal 尝试发布纠正集合收到 HTTP 422，证明旧集合 seal 不能授权未确认集合。证据不把空 seal 导致的 HTTP 400 当作业务保护验证。
- 人工声明成功退款扣减和按订单预聚合后，第 2 次验证通过，独立审核和发布成功。回滚通过正式接口生成另一条 release，校验其完整 afterManifest 与基线完全一致，保留纠正历史。
- 调试期间触发正常登录频率限制后未修改或清空预算。初始化器将真实会话存入 0600 文件，复用前调用正常 session 接口重新授权并获取 CSRF；过期会话仅通过正常密码登录更新。
- 一次发现调用与来源初始化发生检查点竞争，陈旧写入被拒绝。后续以同一发现幂等键查询服务器恢复；不因客户端错误重建运行。独立进程中断验证见下文。

| 工作区 | 基线发布 | 纠正发布 | 回滚发布 |
| --- | --- | --- | --- |
| 成果展示区 | `rls_01m2fgp3zxfqe8smcse89h2q50` | `rls_01m2fh05evfs59tzn424sfv1e1` | `rls_01m2fh05tgfs6tz7hf9917afz5` |
| 独立验收副本 | `rls_01m2fgpecwfrnss79cj87aqefg` | `rls_01m2fh09qtfsaa4kjp9mq8h99y` | `rls_01m2fh0a0bfsbrvahezgztqyaz` |

`node scripts/demo/cli.mjs verify` 实际退出码 0。从正常 API 遍历三条发布、每个提案的独立审核、人工业务确认、两次纠正验证、当前目录 revision 和完整回滚清单。现场操作区 `wsp_01m2ffth47fh7bmr8yzsndwzrd` 的服务端结果为 1 个来源、0 次发现、0 个生产操作、0 个资产、0 条发布。展示区另有 v1 未提交纠正草稿，基线固定于回滚发布；脚本核对无重复生产操作。

## 目录修复与桌面验收

- 用户明确批准受限目录修复。实际生产内容使用 `displayName`/`ownerPrincipalId`，目录 SQL 和前端适配支持这些字段并保留 `name`/`title`/`owner` 兼容。发布和权限逻辑未变。
- Go 新增回归先失败于 `detail title = "Legacy name"`，修复并执行 sqlc 生成后，`go test ./tests/integration/catalog -count=1` 退出码 0。前端名称回归先 RED；字段映射及测试观察面修正后，`pnpm --dir web exec vitest run src/Catalog.test.tsx` 为 16/16，通过既有旧字段测试。
- `make web-embed` 成功；演示实例 `down`/`up` 均成功，保持原端口和发布 ID。重启后的 `verify` 通过，原开发实例 `node scripts/dev/native.mjs status` 通过。
- `pnpm --dir web exec playwright test --config playwright.demo.config.ts`：6/6，退出码 0，桌面 1440×900 和紧凑桌面 1024×768。覆盖正常密码登录、中文目录名、精确来源快照、首次生产发布、纠正验证、回滚发布、草稿清除 localStorage 后刷新恢复、现场区来源与空生产列表。没有 API 路由替身或模型调用。
- 已人工查看两尺寸代表截图：目录、首次生产、回滚和纠正草稿。没有观察到横向溢出或文字重叠；主内容使用内部纵向滚动。截图位于 `.semlia/resume-demo/retail-v1/browser/`，不含密码或会话 Cookie。
- 目录摘要采用延迟详情加载，未打开资产时不包含负责人和发布依据；详情字段映射不把这些缺失信息伪造成已发布/已验证结论。此既有摘要限制不等于服务端没有发布记录。

## 环境核对

确认 `semlia-local-postgres-1` 属于本仓库 Compose 项目 `semlia-local`、服务 `postgres`，容器 ID 为 `cd0dac4bcf382439b143743bd3e038d073fe52dc7479298f53d3bb23d8af4c16`，仅本机 5433 端口。`semlia` 管理角色具备创建数据库能力。每次 provision 重新核对归属，仅创建和操作上述专属资源。

## 中断恢复

- `node scripts/demo/recovery.mjs` 实际退出码 0。在独立验收副本中，子进程通过正常作者会话创建纠正草稿；服务器提交且读取确认后，在写本机检查点前终止这个自有子进程。本机 revision 保持不变，再用同一幂等键恢复，operationId、version、setDigest 完全相同。
- 相同命令再次运行退出码 0，恢复同一草稿，没有重复创建。正式 `verify` 核对该草稿仍未提交，两个工作区发布仍各为 3 条，现场操作区仍为空。
- `go test ./tests/acceptance -run TestResumeDemoDataAndSafety -count=1` 新鲜运行退出码 0（2.254s）。
- 生命周期清理仅删除本进程取得且 dev/inode 一致的记录；未取得 socket 时不主动删除外部 socket，单测覆盖归属不匹配的拒绝。

## 最终门禁与复查

- `GOFLAGS=-p=1 VITEST_MAX_WORKERS=2 make check-source` 退出码 0。format、lint、typecheck、Go/前端测试、契约漂移、sqlc 漂移、嵌入前端漂移、发布构建全部通过；前端为 28 个文件、234 个测试。测试阶段 859s，不把无输出误判为停止。保留 3 条既有 lint 警告和 Vite 大分包警告，不作为测试失败处理。
- 门禁运行期间补充的演示脚本有独立新鲜验证：31 个 Node 单测、Go acceptance `-count=1`、真实中断恢复及完整重入链路。`generate / seed-source / initialize-sources / discover / baseline / correction / draft / verify` 依次退出码 0，业务 ID 与发布数量保持不变。
- 生命周期修复后再次 `down / up` 通过；正常 API 与只读 SQL `verify` 通过；两尺寸浏览器验收再次 6/6（35.8s）。原开发服务 readiness 通过。
- 对 47 个变更或未跟踪文件与本机实际账号密码、数据库密码、密钥、会话 Cookie、运行控制 token 做匹配检查，credentialMatches 为空；不在证据中输出敏感值。`git diff --check` 通过。
- 计划符合性复查：合成材料可重复；展示区及独立副本具有真实发布闭环；现场区保持来源初始态；三种角色、人工确认、验证失败及回滚历史可追溯；无部署、付费模型或开发数据写入。
- 实现复查：检查幂等键及版本约束、来源快照绑定、正常会话恢复、数据库与文件所有权、金额及回滚完整清单。未发现本目标范围内阻断交付的问题。

## 残余边界

本机监督器遭强杀后遗留的未知锁或所有权文件需人工核实，不自动接管；未验证主机断电恢复。目录摘要延迟加载负责人和发布依据。复杂退款数值执行、生产服务器部署及最终简历材料不在本目标范围。使用说明见 `docs/operations/resume-demo.md`；工作树保留待验收，未自动提交或冻结。

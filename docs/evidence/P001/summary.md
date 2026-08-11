# P001 Evidence Summary

## 元数据

| 字段 | 值 |
| --- | --- |
| Task | P001 建立可检查的产品原型 |
| Attempt | PRODUCT-P001-A004 |
| 状态 | Accepted |
| 执行日期 | 2026-08-09 |
| Branch | main |
| Base commit | `2e53e6f` |
| Packet | `docs/specs/product-prototype/packets/P001-A004.yaml` |

## 1. Scope Compliance

P001 交付一个隔离、可点击、明确使用 mock data 的 Semlia 产品原型。实现位于 `prototypes/product/**`，不作为生产 Web，不修改 OpenAPI、生成 SDK、Go 服务或 M1 语义资产业务 schema。

实现、治理与证据范围：

- `prototypes/product/**`
- `package.json`、`pnpm-workspace.yaml`、`pnpm-lock.yaml`、`Makefile`
- `docs/specs/product-prototype/**`
- `docs/specs/m0-foundation/tasks.md`
- `docs/evidence/P001/**`

## 2. Complete User Journey

1. **接入**：检查工作区 readiness、三类来源连接和 metadata-only 最小权限边界。
2. **发现**：运行增量 discovery，形成 6 个带来源证据和风险级别的会话提案。
3. **定义**：搜索语义资产，检查定义、公式、stable ID、owner、血缘、证据和消费者。
4. **治理**：比较 structured diff、validation evidence 和 impact，模拟批准或退回。
5. **发布**：批准形成 release candidate；用户确认 manifest 后模拟发布，也可模拟回滚 binding。
6. **消费**：通过 REST API、MCP、CLI 或 TypeScript SDK 合同创建 release-constrained binding。
7. **反馈**：检查 usage、resolution failure 和 drift，并从运行异常生成修复提案返回治理队列。

Overview lifecycle rail 可直接进入七个阶段。一级导航提供 Sources、Overview、Assets、Proposals、Releases 和 Consumers 六个稳定工作面；topbar 创建提案、context rows 和 release binding 均具有可见结果。

## 3. Product And Visual Result

- 桌面采用 56px activity rail、264px contextual navigator 和 task canvas 三层工作区。
- 视觉系统使用中性色、cobalt focus line、低 elevation 和紧凑列表，参考 Zhizhu 的工作台原则但不复制品牌或产品对象。
- Sources 显示连接、权限和可审计 discovery；Consumers 显示接口合同、binding 与治理反馈。
- approval、candidate、publish、rollback、binding 和 feedback proposal 均产生刷新前持续可见的 session state。
- `Prototype · Mock data` 在普通桌面与紧凑桌面持续可见，模拟写操作在 dialog 和 toast 中再次说明。
- 主工作区使用纯净中性背景，不使用横线或 linear-gradient；指标、readiness、来源、通道、binding 和反馈事件使用独立卡片。
- `1440x900` 与 `1024x768` 是当前产品验证视口；移动端不属于产品支持范围。

## 4. Mock Boundary

- 所有数据来自仓库内 fixture，没有 `fetch`、XHR、WebSocket、远程字体、远程图片或持久化存储。
- discovery、proposal、approval、publish、rollback 和 binding 都是 session-only 产品反馈。
- TypeScript 类型只服务于界面表达，不定义正式领域模型或公共契约。
- 页面分数、来源、提案、验证、release 和消费信号均为产品体验样例。

## 5. Browser Journey

实际浏览器完成：

`Sources -> Discovery -> Proposals -> Approve -> Release candidate -> Publish -> Consumers -> Create binding -> Feedback proposal`

检查视口：`1440x900`、`1024x768`。

浏览器结论：

- Playwright 在普通桌面与紧凑桌面完成七阶段主路径，页面没有横向溢出。
- 自动化视觉合同确认 `.workspace-canvas` 的 `background-image` 为 `none`，核心重复对象具有完整卡片边框。

## 6. Review Results

Spec compliance: Pass.

- 六个视图和七阶段 session lifecycle 全部交付。
- 真实权限、服务端和持久化仍在明确的 prototype 边界外。
- P001 A004 packet 的允许范围没有被突破。

Bug and code quality: Pass with no blocking finding.

- lint、strict typecheck、6 个 unit test、production build 和 4 个桌面 Playwright case 全部通过。
- Go repository、vet 和 contract checks 全部通过。
- 静态扫描未发现网络调用、远程资源或持久化逻辑。

Design review: 27/28, Pass for founder review. 唯一保留项是尚未在低端设备进行性能 profiling。

User acceptance: Accepted by founder on 2026-08-10. The founder approved the product direction and technical selection and confirmed that review remains a direct single-reviewer workflow without a Figma review board.

## 7. Diff

- Patch: `docs/evidence/P001/diff.patch`
- Patch SHA-256: `f4d2492e2867a6a45e03acaab281603219845513e6d714df966d036f5de3ab17`
- Patch lines: 9,972；使用 zero-context unified diff。
- Implementation and governance files: 30；9,470 insertions；224 deletions。
- Evidence files 从 patch 中排除，保证 checksum 稳定。

## 8. Screenshots

A003 截图保留在 `docs/evidence/P001/screenshots/` 作为旅程结构记录。A004 的视觉差异由 Playwright computed-style contract 验证；当前环境没有可供自动截图的 in-app browser tab，实时页面由创始人在 `http://127.0.0.1:4175/` 直接检查。

## 9. Residual Risks

- P001 不验证真实认证、组织隔离、数据库权限、长时 scan、并发审核或发布一致性。
- API、MCP、CLI 和 SDK 只表达产品合同，没有调用正式 endpoint 或签发 credential。
- 原型 fixture 不能直接转为数据库迁移、OpenAPI 或 M1 领域 schema。
- 大规模血缘布局和低端设备性能尚未验证。

## 10. Acceptance Gate

P001 于 2026-08-10 获得 founder 明确接受，状态为 `Accepted`。T003、T004 的产品评审顺序阻塞已解除；后续实现继续遵循已确认的 M0 plan、work graph 和 TDR。

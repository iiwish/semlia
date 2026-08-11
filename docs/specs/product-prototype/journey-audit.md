# Semlia Prototype Journey Audit

## 元数据

| 字段 | 值 |
| --- | --- |
| Audit | P001 journey completeness |
| 模式 | Combined UX and accessibility risk audit |
| 日期 | 2026-08-09 |
| 目标 | 从工作区接入到持续治理形成可点击产品闭环 |
| Evidence | `docs/evidence/P001/journey-audit-before/` |

## 1. User Goal

语义负责人应能在同一工作台完成：连接来源、发现语义、理解资产、审核 AI 提案、形成 release candidate、发布或回滚、配置消费接口，并从真实使用反馈继续治理。

## 2. Captured Flow

| Step | Screenshot | Health | Finding |
| --- | --- | --- | --- |
| 1 治理概览 | `01-overview.png` | Partial | 风险和资产关系清楚，但没有接入状态、生命周期位置或首次行动入口；约 837px 宽度下指标文本发生挤压。 |
| 2 资产发现 | `02-assets.png` | Partial | 搜索、定义、证据和血缘完整；来源接入与 AI 发现过程不可见，顶部 action 在中等宽度下空间不足。 |
| 3 提案审核 | `03-proposals.png` | Partial | diff、验证和影响范围清楚；全局“创建提案”不创建对象，批准后没有 release candidate handoff。 |
| 4 发布与消费 | `04-releases.png` | Partial | immutable manifest 和已有 binding 清楚；没有 candidate 发布、环境推广、回滚操作或消费接口配置。 |

## 3. Strengths

- 三层工作区让模块、上下文和当前对象位置清楚。
- 资产 stable ID、定义、公式、证据、血缘和消费者提供可信语义核心。
- 提案把 AI 输出限制在结构化 diff、验证证据和显式审核中。
- mock boundary 持续可见，模拟操作不暗示真实写入。
- 导航、tabs、dialog 和主要动作具备语义标签与键盘路径。

## 4. UX Risks

### Critical journey gaps

- SSOT J-001 和 J-002 不可执行：没有 workspace readiness、DataSource、最小权限、connection test、scan record 或 AI discovery 结果。
- SSOT J-004 只展示历史结果：approval 不形成 candidate，无法检查发布、环境推广或消费者回滚。
- SSOT J-005 不可执行：API、MCP、CLI、SDK 只是文字概念，没有消费入口、binding creation 或解析契约。
- SSOT J-006 缺少反馈闭环：没有 usage、resolution failure、drift、incident 或从信号生成修复提案的路径。

### Interaction gaps

- “创建提案”和 contextual navigator 多数 action 不改变产品状态。
- Overview 没有表达用户当前处于生命周期的哪一步，也不能直接前往接入与消费治理。
- 页面之间依靠全局图标跳转，审批、发布和消费之间缺少明确 handoff。

### Responsive gaps

- 约 837px 宽度时四列 metric strip 发生不自然断行与视觉碰撞。
- 中等宽度下页面标题 action 和 master-detail 工具接近容器边界。

## 5. Accessibility Risks

- 图标导航有 accessible name 和 focus ring，这是确定优势。
- contextual rows 使用 button 和 chevron，却没有稳定的可见结果，可能给键盘和辅助技术用户造成错误操作预期。
- session 状态主要依赖 toast，后续页面缺少持久状态确认。
- 截图不能证明完整 WCAG 合规；仍需通过键盘、焦点恢复、reduced motion、语义 landmarks 和 200% zoom 测试验证。

## 6. Required Journey

| Stage | User outcome | Prototype surface |
| --- | --- | --- |
| 1 接入 | 工作区、策略和最小权限准备完成 | Sources / readiness |
| 2 发现 | 连接来源并生成可审计 scan 与 AI proposals | Sources / discovery run |
| 3 定义 | 查找并理解权威语义资产 | Assets |
| 4 治理 | 比较、验证并审核 proposal | Proposals |
| 5 发布 | 形成 candidate、模拟发布或回滚 binding | Releases |
| 6 消费 | 配置 REST、MCP、CLI、SDK 与 binding | Consumers |
| 7 反馈 | 处理解析失败、drift 和使用信号 | Consumers / governance feedback |

## 7. Acceptance Recommendations

- 一级导航增加 `接入发现` 与 `消费监控`，而不是把它们藏在设置或 release 详情中。
- Overview 增加可导航 lifecycle rail，持续显示每一阶段的健康状态与下一步。
- 扫描、提案创建、审核、candidate、发布、回滚和 binding creation 在 session 内产生持久可见状态。
- Consumers 同时展示接口契约、active binding、解析健康和反馈信号。
- `720px-960px` 采用两列 metric strip、单列工具区和完整 action wrapping；移动端使用六项稳定底部导航。
- 端到端测试覆盖 `Sources -> Discovery -> Proposal -> Approve -> Candidate -> Publish -> Consumer binding -> Feedback proposal`。

## 8. Evidence Limits

本审计基于当前 mock prototype、浏览器截图和可见交互。真实权限、连接失败、长时 scan、数据规模、并发审核、发布一致性、通知和生产消费延迟不在 P001 中验证。

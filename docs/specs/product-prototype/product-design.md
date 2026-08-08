# Semlia Product Prototype Design Contract

## 元数据

| 字段 | 值 |
| --- | --- |
| Feature | product-prototype |
| 版本 | 0.1.0 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-08 |
| Source | `docs/SSOT.md` v0.2.0 Confirmed |
| 用户授权 | 创始人明确要求优先完成可检查的前端原型 |

## 1. Product Brief

Pitch:

Semlia 是语义资产控制平台，让数据团队在同一个可追溯工作面中发现定义、检查证据、理解关系、审核 AI 提案并发布不可变语义版本。

Problem:

架构和契约底座无法直接证明产品是否正确。原型需要把 SSOT 中抽象的资产、证据、提案、验证、release 和 binding 变成一个可检查、可讨论的完整旅程。

Audience:

- Primary: semantic engineer、analytics engineer、data product owner。
- Secondary: reviewer、data platform maintainer、AI/BI consumer owner。
- Decision maker: 需要判断 Semlia 是否值得继续建设的创始人与开源维护者。

Platform:

响应式 desktop-first Web app。桌面服务高密度重复工作；移动端服务状态检查、搜索、详情阅读与轻量审核。

Core workflows:

1. 用户从 Overview 识别待治理资产、风险和最新 release。
2. 用户在 Assets 搜索、筛选资产，并检查定义、owner、证据、关系和消费者。
3. 用户在 Proposals 打开 AI 提案，比较结构化 diff、验证结果和影响范围。
4. 用户模拟批准或退回提案，看到明确的本地 prototype feedback，不产生真实写入。
5. 用户在 Releases 检查 immutable manifest、变化摘要和消费者 binding。

Required states:

- Active、hover、focus、selected、search-empty、filter-empty。
- Proposal pending、approved-in-session、returned-in-session。
- Validation passed、warning、failed 的可辨识状态。
- Desktop split view、tablet condensed view、mobile detail-first view。
- `prefers-reduced-motion` 下移除非必要位移和绘制动画。

Non-goals:

- 不连接真实 API、数据库、Cube、LLM 或认证服务。
- 不持久化模拟审核结果，不暗示发布真实完成。
- 不定义生产领域 schema，不替代 M1 specification。
- 不实现 BI 看板、NL2SQL 或聊天主界面。

## 2. Information Architecture

| View | Primary job | Primary artifact |
| --- | --- | --- |
| Overview | 确认治理态势和下一步工作 | semantic coverage map 与 attention queue |
| Assets | 查找并理解可信语义 | asset catalog + detail inspector + lineage |
| Proposals | 判断变更是否可信可发布 | structured diff + validation evidence |
| Releases | 确认不可变版本和消费范围 | release manifest + bindings |

Global shell 始终显示 workspace、mock boundary、全局搜索入口、主导航和当前 release。用户不需要通过欢迎页或聊天才能进入工作。

## 3. Visual Thesis

Primary pattern: Scientific Atlas / Command Center。

Secondary tension: Editorial precision。

Design thesis: **让每条语义事实像技术图纸一样可定位、可验证、可追溯。** 页面用清晰坐标、连接线、证据编号和版本标记表达可信度；操作区域保持克制，不把治理工作包装成装饰性 dashboard。

Signature move: **Truth Trace**。选择资产或提案时，关系图、来源证据和 release binding 使用同一强调色同步高亮，让用户一眼看到“定义从哪里来、会影响谁、发布到哪里”。

## 4. Design System

Color tokens:

| Token | Value | Use |
| --- | --- | --- |
| canvas | `#F4F6F5` | 全局背景 |
| surface | `#FFFFFF` | 工作面板 |
| surface-subtle | `#EDF1F0` | 选择、分组和次级区域 |
| ink | `#17201D` | 主文本 |
| muted | `#66736E` | 元数据 |
| border | `#D8DEDB` | 分隔与控件边界 |
| accent | `#0A7C6F` | active、可信路径和主操作 |
| accent-soft | `#DDF2EC` | active 背景 |
| info | `#2F67D8` | source 与消费者信息 |
| warning | `#B66A12` | validation warning |
| danger | `#B73A45` | breaking risk 与退回 |
| violet | `#7655B5` | AI proposal 身份的有限 counterpoint |

Typography:

- UI: `Inter`, `SF Pro Text`, `PingFang SC`, `Microsoft YaHei`, sans-serif。
- Technical metadata: `SFMono-Regular`, `JetBrains Mono`, `Consolas`, monospace。
- Display text 只用于页面标题，最大 28px；panel heading 13-18px。
- CJK body 使用 1.55 line-height；letter spacing 固定为 0。

Layout:

- Desktop shell: 232px sidebar + fluid content；Assets 视图使用最小 340px catalog + fluid detail。
- Detail artifact 使用稳定 split pane，不在 card 中嵌套 card。
- 8px spacing base；panel padding 16/20/24；卡片 radius 最大 8px。
- 低于 980px 时隐藏次要 rail；低于 720px 时 sidebar 变为底部 navigation，split view 改为单列 drill-in。

Components:

- Icon navigation、command/search bar、segmented filters、status badges、data table/list、tab strip、lineage diagram、diff rows、validation checklist、release timeline、dialog 和 toast。
- 所有熟悉操作使用 lucide icon；文字按钮只用于明确命令。
- 不使用 nested cards、oversized hero、gradient、decorative orb 或纯聊天 composition。

Motion:

- Hover/focus 120ms；panel/tab change 180ms；drawer/dialog 240ms。
- 使用 opacity 与 transform，不以 width/height 驱动 motion。
- Truth Trace 的 SVG path draw 320ms；reduced motion 直接显示终态。
- 所有布局尺寸在 hover 和动态内容变化时保持稳定。

Accessibility:

- WCAG 2.2 AA 基础对比；可见 `:focus-visible` ring。
- 主导航、filters、tabs、asset rows、dialog 与 actions 可用键盘完成。
- 状态同时使用 icon、label 和颜色；图形关系有文本摘要。
- 触摸目标移动端至少 44px。

## 5. Asset Manifest

| Asset | Type | Dimensions | Location | Alt / accessible fallback |
| --- | --- | --- | --- | --- |
| Semantic coverage map | Code-native SVG | responsive 16:7 | Overview primary artifact | 关系摘要与节点列表 |
| Asset lineage map | Code-native SVG | responsive 16:8 | Asset detail Lineage tab | 上下游关系文本 |
| Release change trace | HTML/CSS timeline | responsive | Releases view | 有序 release change list |

原型不依赖远程图片、字体或媒体。图谱和时间线展示产品真实对象，不使用装饰性插画。

## 6. Prototype Boundary

- 顶部持续显示 `Prototype · Mock data`，所有模拟写操作在 dialog 和 toast 中再次说明。
- 数据来自 repository-local fixture，不允许生产 fallback 或远程请求。
- session state 只用于演示选择、筛选和审核反馈，刷新即重置。
- 原型 package 与生产 `web/**` 分离，未接受的 UI 不进入生产依赖图。

## 7. Validation Contract

- Build/type/lint/unit test 全部通过。
- Playwright 覆盖 Overview -> Assets -> Proposal review -> Releases 主路径。
- 检查 1440x900、1024x768、390x844 viewport。
- 截图覆盖首屏、资产详情、proposal dialog、mobile view。
- 检查文本溢出、重叠、键盘焦点、mock boundary 和 reduced motion。

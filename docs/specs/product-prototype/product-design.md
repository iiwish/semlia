# Semlia Product Prototype Design Contract

## 元数据

| 字段 | 值 |
| --- | --- |
| Feature | product-prototype |
| 版本 | 0.4.1 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-10 |
| Source | `docs/SSOT.md` v0.2.0 Confirmed |
| 用户授权 | 创始人明确要求优先完成可检查的前端原型 |
| 验收 | 2026-08-10 经创始人直接审核接受；不建立 Figma 评审板 |

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

Desktop-only Web app。产品服务高密度、长时间、重复性的语义治理工作，支持普通桌面与紧凑桌面窗口，不提供移动端产品体验。

Core workflows:

1. 用户在 Sources 检查 workspace readiness、最小权限和 DataSource connection，并运行可审计 discovery。
2. AI discovery 形成带 evidence 的 proposals，用户进入治理队列而不是直接写入语义事实。
3. 用户在 Assets 搜索、筛选资产，并检查 definition、owner、evidence、lineage 和 consumers。
4. 用户在 Proposals 比较 structured diff、validation evidence 与 impact，模拟批准或退回。
5. 已批准提案形成 session release candidate，用户在 Releases 检查 manifest 并模拟 publish 或 rollback binding。
6. 用户在 Consumers 选择 REST、MCP、CLI 或 SDK，创建 release-constrained binding 并检查解析健康。
7. 用户从 usage、failure、drift 和 incident 信号生成新的治理 proposal，完成持续治理闭环。

Required states:

- Active、hover、focus、selected、search-empty、filter-empty。
- Proposal pending、approved-in-session、returned-in-session。
- Discovery ready、running、complete；release candidate ready、published-in-session、rollback-in-session。
- Consumer binding created-in-session、resolution healthy、feedback warning 和 incident risk。
- Validation passed、warning、failed 的可辨识状态。
- Desktop split view 与 compact-desktop stacked workbench。
- `prefers-reduced-motion` 下移除非必要位移和绘制动画。

Non-goals:

- 不连接真实 API、数据库、Cube、LLM 或认证服务。
- 不持久化模拟审核结果，不暗示发布真实完成。
- 不定义生产领域 schema，不替代 M1 specification。
- 不实现 BI 看板、NL2SQL 或聊天主界面。

## 2. Information Architecture

| View | Primary job | Primary artifact |
| --- | --- | --- |
| Sources | 建立工作区并发现现有语义 | readiness + connections + discovery run |
| Overview | 确认治理态势和下一步工作 | semantic coverage map 与 attention queue |
| Assets | 查找并理解可信语义 | asset catalog + detail inspector + lineage |
| Proposals | 判断变更是否可信可发布 | structured diff + validation evidence |
| Releases | 形成 candidate、发布和回滚 | candidate + immutable manifest + rollback |
| Consumers | 提供接口并持续治理使用反馈 | channels + bindings + resolution signals |

Global shell 始终显示 workspace、mock boundary、全局搜索入口、主导航和当前 release。用户不需要通过欢迎页或聊天才能进入工作。

## 3. Visual Thesis

Primary pattern: Quiet operational workbench。

Secondary tension: Editorial archive。

Design thesis: **Semlia 是语义团队长期工作的治理台，不是展示指标的 dashboard。** 界面以稳定三层工作区、低噪声边界、紧凑列表和清晰选中态组织复杂信息，让用户始终知道自己位于哪个治理环节、正在检查哪份语义资产以及下一步可以做什么。

Signature move: **Semantic Focus Line**。一级导航、上下文对象和主工作区共享同一条 cobalt 选中线；用户选择资产、提案或 release 后，左侧对象位置与右侧详情同步，血缘和证据继续承担语义可追溯表达。

Reference intent:

- 参考 `/workspace/local/self/zhizhu` 的安静桌面工作台与三层信息架构。
- 继承 56px activity rail、264px contextual navigator、纯净中性任务画布、紧凑字号和明确 focus line。
- 不复制 Zhizhu 的铁砧标志、知识对象、文案、页面构图或专有产品身份。
- 使用 Semlia 的资产目录、定义、血缘、validation、proposal 和 immutable release 替代参考项目对象。
- 在桌面与紧凑桌面窗口中，让治理状态、mock boundary 与消费绑定比参考界面更显式。

## 4. Design System

Color tokens:

| Token | Value | Use |
| --- | --- | --- |
| canvas | `#F7F8FA` | 全局背景 |
| surface | `#FFFFFF` | 工作面板 |
| surface-subtle | `#F2F4F7` | activity rail、context panel 与分组区域 |
| ink | `#172033` | 主文本 |
| muted | `#526077` | 元数据 |
| border | `#DDE2EA` | 分隔与控件边界 |
| accent | `#315FD5` | active、focus line 与主操作 |
| accent-soft | `#EEF3FF` | active 背景 |
| success | `#168A63` | published 与 validation passed |
| warning | `#B7791F` | validation warning |
| danger | `#C2414B` | breaking risk 与退回 |
| violet | `#7655B5` | AI proposal 身份的有限 counterpoint |

Typography:

- UI: `Inter`, `SF Pro Text`, `PingFang SC`, `Microsoft YaHei`, sans-serif。
- Technical metadata: `SFMono-Regular`, `JetBrains Mono`, `Consolas`, monospace。
- Display text 只用于页面标题，最大 28px；panel heading 13-18px。
- CJK body 使用 1.55 line-height；letter spacing 固定为 0。

Layout:

- Desktop shell: 56px activity rail + 264px contextual navigator + fluid task canvas。
- Contextual navigator 根据当前模块承载域、资产、proposal 或 release 列表；主画布只承载当前对象详情和决策。
- Detail artifact 使用稳定 split pane，不在 card 中嵌套 card。
- 指标、来源、通道、binding 和反馈事件使用独立卡片；承载这些重复卡片的 section 保持无框。
- 主工作区不使用横线、方格纸或 linear-gradient 背景，分隔线只保留在真正需要逐行比较的 table 和 diff 中。
- 8px spacing base；panel padding 16/20/24；控件 radius 6px，主要 framed tool 最大 8px。
- 主内容使用 1040px readable measure；图谱和审核详情允许占满可用画布。
- 低于 1180px 时 contextual navigator 收窄且工具区单列；`1024px` 是产品支持的最小 viewport，不为更窄屏幕设计产品导航或工作流。

Components:

- Icon navigation、command/search bar、segmented filters、status badges、data table/list、tab strip、lineage diagram、diff rows、validation checklist、release timeline、dialog 和 toast。
- 所有熟悉操作使用 lucide icon；文字按钮只用于明确命令。
- 不使用 dark sidebar、nested cards、oversized hero、gradient、decorative orb、彩色 dashboard 拼贴或纯聊天 composition。

Motion:

- Hover/focus 120ms；panel/tab change 180ms；drawer/dialog 240ms。
- 使用 opacity 与 transform，不以 width/height 驱动 motion。
- Truth Trace 的 SVG path draw 320ms；reduced motion 直接显示终态。
- 所有布局尺寸在 hover 和动态内容变化时保持稳定。

Accessibility:

- WCAG 2.2 AA 基础对比；可见 `:focus-visible` ring。
- 主导航、filters、tabs、asset rows、dialog 与 actions 可用键盘完成。
- 状态同时使用 icon、label 和颜色；图形关系有文本摘要。
- 桌面控件保持稳定点击区域，核心动作不依赖 hover 才能发现。

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
- Playwright 覆盖 Sources -> discovery -> Assets -> Proposal review -> candidate -> publish -> Consumers -> feedback 主路径。
- 检查 1440x900 与 1024x768 viewport。
- 截图覆盖接入发现、概览、资产详情、proposal dialog、release candidate 和 consumer binding 的桌面状态。
- 检查文本溢出、重叠、键盘焦点、mock boundary 和 reduced motion。

# Semlia Product Prototype Design Contract

## 元数据

| 字段 | 值 |
| --- | --- |
| Feature | product-prototype |
| 版本 | 0.7.0 |
| 状态 | Confirmed |
| 最后更新 | 2026-08-28 |
| Source | `docs/SSOT.md` v0.7.0 Confirmed |
| 用户授权 | 创始人明确要求优先完成可检查的前端原型 |
| 验收 | 2026-08-10 经创始人直接审核接受；2026-08-14 确认企业语义资产平台定位；不建立 Figma 评审板 |

## 1. Product Brief

Pitch:

Semlia 是企业语义资产平台，让数据团队在同一个可追溯工作面中以 LLM Wiki 和本体组织企业含义，并将业务语义作为可执行、可测试、可发布且具有消费者兼容性约束的软件资产治理。

Problem:

架构和契约底座无法直接证明产品是否正确。原型需要把 SSOT 中抽象的资产、证据、变更事项、验证、release 和 binding 变成一个可检查、可讨论的完整旅程。

Audience:

- Primary: semantic engineer、analytics engineer、data product owner。
- Secondary: reviewer、data platform maintainer、AI/BI consumer owner。
- Decision maker: 需要判断 Semlia 是否值得继续建设的创始人与开源维护者。

Platform:

Desktop-only Web app。产品服务高密度、长时间、重复性的语义治理工作，支持普通桌面与紧凑桌面窗口，不提供移动端产品体验。

Core workflows:

1. 用户登录后进入 Ask，直接针对当前 stable release 提问，并检查回答引用的 knowledge blocks、semantic assets、physical bindings、JoinContract、resolved plan 和 release。用户发现回答中的定义、计算或消歧知识不准确时，通过“指出问题”选择问题类型；系统定位关联 Claim，并以当前发布 revision 为基线打开知识修订工作台。每个语义资产详情的标题区常驻“修订知识”主操作，用户无需预先进入特定 Tab，即可选择业务定义、计算表达式、口径边界或消歧规则开始修订。
2. 用户在知识目录中统一搜索语义资产、语义关系和物理实现，通过资产详情检查 definition、owner、evidence、lineage、physical binding 和 consumers；知识块作为证据上下文出现，不形成必经页面。
   - 资产详情同时是权威 LLM Wiki 页面和受治理本体的局部投影。
   - “定义”展示由当前 revision 编译的检索词、消歧规则、典型问题、口径边界和类型专属契约。
   - “本体关系”默认展示一跳本体血缘图和类型化关系列表；图负责理解，结构化关系负责审计。
   - “实现”展示 PhysicalBinding、粒度、JoinContract 和执行适配器，不与业务关系混排；无执行能力的业务概念隐藏该视图。
   - “可信度”集中展示字段证据、验证运行、策略和门禁结论；“交付与影响”集中展示接口地址、部署指针、消费者绑定和兼容性。
   - 已发布资产详情保持只读；用户从权威语义页、具体知识字段或字段级 Claim 发起修订，在同一工作台比较当前发布值与候选值，记录修订原因并检查只读证据。
   - 知识修订工作台只修改结构化知识和候选 Claim，不允许覆盖 SourceRevision、EvidenceArtifact 或已发布 AssetRevision；保存草稿、运行检查和提交审核形成清晰的递进动作。
   - 七种资产使用同一六视图框架，并由 `AssetTypeProfile` 提供类型专属内容模板、必填契约、发布门禁和空状态；页面不使用七套互不兼容的导航。
3. 用户在工作台通过个人待办队列处理版本审核、运行诊断和消费者迁移确认；所有工作项都返回原始资产版本或运行详情完成处理，正常后台运行与非行动事件保留在各自责任域。
4. 用户提交知识修订后直接进入候选资产版本，在版本详情中统一检查人可读的字段差异、变更来源、knowledge evidence、validation evidence 与 consumer impact，完成版本级批准或退回，再发布不可变资产版本。
5. 管理员在数据接入中管理 DataSource 与接入自动化，通过执行记录追溯 SourceRevision 和诊断异常；有意义的产物形成关联变更事项。
6. 管理员在系统设置中管理工作区成员、策略、模型供应商、LLM 与 Embedding 默认模型、REST/MCP/CLI/SDK 接口、审计和运行配置。

Required states:

- Active、hover、focus、selected、search-empty、filter-empty。
- Change item pending、approved-in-session、returned-in-session。
- Discovery ready、running、complete；release candidate ready、published-in-session、rollback-in-session。
- Release binding visible、resolution healthy、feedback warning 和 incident risk。
- Validation passed、warning、failed 的可辨识状态。
- 类型发布门禁区分发布阻断与质量提醒；空状态区分不适用、可选缺失和阻断缺失，并提供类型专属解释与修复动作。
- Desktop split view 与 compact-desktop stacked workbench。
- `prefers-reduced-motion` 下移除非必要位移和绘制动画。

Non-goals:

- 不连接真实 API、数据库、执行运行时、LLM 或认证服务。
- 不持久化模拟审核结果，不暗示发布真实完成。
- 不定义生产领域 schema，不替代 M1 specification。
- 不实现 BI 看板、Workbook、任意 SQL 生成或无语义约束的通用 ChatBI。
- Ask 是 Semlia 已发布知识和可信解析能力的受治理参考界面；它不绕过 release、策略、证据、PhysicalBinding、JoinContract 和执行适配器边界。

## 2. Information Architecture

| 一级责任域 | Primary job | Primary artifact |
| --- | --- | --- |
| 语义问答 | 使用已发布知识获得可追溯回答 | conversation + answer trace + knowledge citations |
| 工作台 | 推进明确分配给当前用户的行动事项 | work item |
| 知识资产 | 查找并管理知识块、物理证据与可信语义 | knowledge catalog + asset inspector + lineage |
| 变更与发布 | 判断变更事项与 release 是否可信可生效 | change context + structured diff + validation evidence + immutable manifest |
| 数据接入 | 建立来源、配置自动化并按需诊断运行 | DataSource + automation policy + diagnostic run |
| 系统设置 | 管理工作区、策略、模型、对外接口和审计 | membership + policy + model configuration + channel configuration + audit |

Global shell 始终显示当前责任域、上下文对象、搜索入口和主导航。Activity rail 的主功能区从上到下依次为语义问答、工作台、知识资产、变更与发布、数据接入；系统设置固定在底部独立分组。上下文搜索服从当前责任域：语义问答搜索最近会话，工作台搜索待办，知识资产搜索知识目录；`Command K` 聚焦当前责任域的搜索入口，系统级命令不占用会话搜索框。

Navigation hierarchy:

| 一级菜单 | 二级菜单 | 模块内行为 |
| --- | --- | --- |
| 语义问答 | 问答会话 | 新建或切换当前问答会话 |
| 工作台 | 待办 | 在个人行动队列中筛选、排序并推进明确动作 |
| 知识资产 | 最近打开的资产 | 默认进入知识目录；统一列示语义资产、关系、PhysicalBinding 和 JoinContract，并在资产详情检查定义、实现、证据与使用 |
| 变更与发布 | 资产版本 | 统一列示候选、当前和历史资产版本；候选版本详情承载变更事项、差异、验证、审批与消费影响，已发布版本详情承载清单、绑定和回滚边界 |
| 数据接入 | 数据来源、接入自动化、执行记录 | 接入数据库或文件来源，管理自动化策略并诊断执行历史 |
| 系统设置 | 成员、模型配置、接口与集成、审计与运行设置 | 管理成员目录、低频平台配置、推理与索引模型和对外接口 |

资产版本是变更与发布域的主对象。资产版本列表同时展示候选、当前和历史状态，以语义资产稳定 ID 与 revision 作为对象标识；发布批次只记录版本由哪次发布操作产生。变更事项是候选版本的组成部分和审计来源，不形成独立页面或并列列表。一个候选版本可以包含一项或多项变更事项，版本详情统一呈现相对上一 revision 的差异、变更来源、验证门禁、审核决策与消费影响。工作台只负责聚合需要用户处理的候选版本，点击待办进入同一个版本详情。版本比较始终限定在同一语义资产的相邻 revision 之间，不提供缺少资产上下文的全局比较；发布后保留不可变清单、应用绑定和资产级回滚边界。

工作台是面向当前用户的行动入口，不是第二套资产、版本、运行或事件数据源。二级栏直接列示待我处理的工作项，不设置只有一个选项的“待办”菜单；搜索入口聚焦主画布的待办搜索，选择工作项直接进入原始候选版本或运行详情并保持当前项高亮。待办以工作项为主对象，统一记录关联对象、需要执行的动作、风险、负责人、时限和处理状态；主列表支持范围、类型、风险、搜索与排序。候选版本生成、发布回滚、运行结果、消费漂移和策略变化分别保留在资产版本、执行记录、资产消费影响和系统审计中；只有要求人工行动的事件会生成待办。全局健康指标、稳定版本、资产覆盖和原始执行日志保留在各自责任域。

二级菜单只承载长期页面或对象集合，不承担后台处理阶段。知识资产使用最近打开对象作为 contextual panel，单次模块访问中保持对象顺序稳定，选择对象只更新高亮与详情；系统后台记录访问时间，在用户重新进入知识资产时结算排序，首次打开的新对象直接置顶。主画布中的知识目录以语义资产为主行，通过可展开子对象展示关系与物理实现。对象类型是普通筛选条件，不形成独立页面。跨模块动作使用主内容区中带明确目的地的命令按钮，并保留当前来源、资产、变更或 release 上下文。答案引用直接进入资产证据，异常运行直接进入执行诊断，要求人工处理时生成工作台待办。对外接口进入系统设置，消费关系进入资产与版本详情，运行反馈保留在执行记录。

模型配置以供应商为管理边界，采用 BYOK 凭据并保持 API Key 只写。LLM 模型与 Embedding 模型在同一长期页面中使用独立 Tab，分别维护可用模型和默认模型，避免将问答推理与知识索引混为同一运行参数。Embedding 配置记录模型 ID、向量维度、最大输入与检索能力；切换默认索引模型只影响后续索引任务，既有向量继续保留原模型 ID 与维度，直到管理员显式触发重建。前端原型只模拟配置和凭据托管状态，不连接真实模型服务或保存密钥。

成员页直接呈现可搜索的成员目录，字段包括员工 ID、姓名、邮箱、部门、职位与在职状态。当前产品范围只处理成员基础身份信息，不在成员列表中混入工作区配置、身份提供方、默认角色或 RBAC 权限。

MVP 中所有已认证的工作区成员拥有相同的产品操作能力，系统设置不提供角色与权限页面。UI 与 Agent 统一通过携带 `actor_id` 和 `workspace_id` 的 MCP/API 调用业务能力；工作区隔离、只写凭据、不可变版本、危险操作确认和审计记录属于不可绕过的系统边界。后续 RBAC 在统一鉴权层中扩展，不改变现有 MCP/API 工具契约。

## 3. Visual Thesis

Primary pattern: Quiet operational workbench。

Secondary tension: Editorial archive。

Design thesis: **Semlia 是以可信问答为高频入口、以语义治理为核心的长期工作台，不是展示指标的 dashboard。** 界面以稳定三层工作区、低噪声边界、紧凑列表和清晰选中态组织复杂信息，让用户始终知道答案来自哪份知识、哪版语义以及下一步可以做什么。

Signature move: **Semantic Focus Line**。一级导航、上下文对象和主工作区共享同一条 cobalt 选中线；用户选择资产、变更事项或 release 后，左侧对象位置与右侧详情同步，血缘和证据继续承担语义可追溯表达。

Reference intent:

- 参考创始人提供的本地 Zhizhu 项目的安静桌面工作台与三层信息架构。
- 继承 56px activity rail、264px contextual navigator、纯净中性任务画布、紧凑字号和明确 focus line。
- 不复制 Zhizhu 的铁砧标志、知识对象、文案、页面构图或专有产品身份。
- 使用 Semlia 的知识目录、定义、血缘、validation、change item 和 immutable release 替代参考项目对象。
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

- Icon navigation、Ask composer、answer trace、knowledge citations、command/search bar、segmented filters、status badges、data table/list、tab strip、lineage diagram、diff rows、validation checklist、release timeline、dialog 和 toast。
- 所有熟悉操作使用 lucide icon；文字按钮只用于明确命令。
- 不使用 dark sidebar、nested cards、oversized hero、gradient、decorative orb、彩色 dashboard 拼贴或缺少证据检查面的纯聊天 composition。

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

- 所有模拟写操作在 dialog 和 toast 中明确说明原型边界。
- 数据来自 repository-local fixture，不允许生产 fallback 或远程请求。
- session state 只用于演示选择、筛选和审核反馈，刷新即重置。
- 原型 package 与生产 `web/**` 分离，未接受的 UI 不进入生产依赖图。

## 7. Validation Contract

- Build/type/lint/unit test 全部通过。
- Playwright 覆盖 Sources -> discovery -> Assets -> Proposal review -> candidate -> publish -> binding inspection -> feedback work item 主路径。
- 检查 1440x900 与 1024x768 viewport。
- 截图覆盖接入发现、概览、资产详情、proposal dialog、release candidate 和版本绑定的桌面状态。
- 检查文本溢出、重叠、键盘焦点、原型边界和 reduced motion。

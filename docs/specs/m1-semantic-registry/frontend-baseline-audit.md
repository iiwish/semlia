# M1 Frontend Baseline Audit

## 元数据

| 字段 | 值 |
| --- | --- |
| Audit | M1-FE-AUDIT |
| 版本 | 1.0.0 |
| 状态 | Completed |
| 日期 | 2026-08-10 |
| 审计对象 | `web/**` production status slice；`prototypes/product/**` accepted product prototype |
| 产品合同 | `docs/specs/product-prototype/product-design.md` v0.4.1 Confirmed |
| 技术决策 | `docs/adr/0002-m1-product-and-semantic-execution.md` |
| Viewports | `1440x900`、`1024x768` desktop |
| 证据 | `docs/evidence/M1-FE-AUDIT/screenshots/**` |

## 1. 结论

当前代码按 M0 文档正确保持了生产 Web 与产品原型的边界：`web/**` 是调用生成 SDK 的 system-status 纵向基础，`prototypes/product/**` 是 mock-only、不可持久化、不可直接进入生产依赖图的产品验证面。生产实现没有宣称已经具备 Semantic Registry，这一点符合 M0 范围。

产品原型已经证明 Semlia 适合安静、高密度、桌面优先的治理工作台，并完整表达 Sources、Assets、Proposals、Releases 和 Consumers 的长期产品旅程。它不是 M1 的生产代码基础：dialog 可访问性、无效控件、异步状态、紧凑桌面完整性、empty state 一致性和 CSS 可维护性需要在 production implementation 中解决。

Design quality rubric: **Pass with concerns，21/28，平均 1.50/2**。核心产品适配、首屏信息架构和层级清晰；交互完整度、响应式细节、状态覆盖、signature 表达和代码可维护性尚未达到世界一流开源项目的发布标准。

## 2. 审计方法与验证结果

- 逐项比对 SSOT、product design contract、M0 production boundary 和当前 dependency manifests。
- 在真实浏览器检查 `1440x900` 与 `1024x768` 的 Overview、Assets、Proposals 和 proposal dialog。
- 检查 document overflow、关键容器 scroll dimensions、dialog 尺寸、focusable background、mock boundary、当前 release 和 console error。
- 运行 production Web 与 prototype 的 lint、typecheck、build、unit test 和 Playwright 核心旅程。

| 验证 | 结果 |
| --- | --- |
| ESLint | Pass：`@semlia/web`、`@semlia/product-prototype` |
| TypeScript | Pass：两个 workspace |
| Build | Pass：Web JS 207.49 kB / gzip 65.61 kB；prototype JS 267.04 kB / gzip 79.66 kB |
| CSS build | Pass：Web gzip 2.02 kB；prototype gzip 12.22 kB |
| Unit tests | Pass：11 tests（Web 5，prototype 6） |
| Playwright | Pass：12 tests（Web 8，prototype 4） |
| Browser console | 受检页面无 error 或 warning |
| Document overflow | 两个目标 viewport 均无 document-level horizontal overflow |

这些测试证明当前行为可重复，不代表下列交互和可访问性问题已被覆盖。

## 3. Findings

### F-001 High：dialog 没有完整的 modal focus contract

`ReviewDialog`、`CreateProposalDialog`、`PublishDialog` 和 `BindingDialog` 只处理初始 focus 与 Escape；背景没有 inert，focus 没有显式限制在 dialog，部分 dialog 也没有恢复触发点。`aria-modal="true"` 不能替代 keyboard focus containment。1024 viewport 的 proposal dialog 尺寸为 680x624，视觉上可容纳，但页面仍有 19 个 dialog 外 focusable elements。

Evidence: `prototypes/product/src/App.tsx:587`、`:629`、`:654`；`prototypes/product/src/JourneyViews.tsx:108`。

M1 gate: 使用 Radix Dialog，验证初始 focus、Tab/Shift+Tab containment、Escape、outside pointer policy、background inert、关闭后 focus restore 和 reduced motion。

### F-002 High：多个可见控件没有行为

品牌按钮、命令按钮和当前视图筛选入口呈现为可交互控件或快捷入口，但没有动作；保存视图、提案队列筛选、比较版本、查看审核规则和查看 manifest 也没有 handler。可见但无效的操作会破坏用户对治理工作台的信任。

Evidence: `prototypes/product/src/App.tsx:117`、`:142`、`:157`、`:405`、`:511`、`:535`、`:544`、`:566`。

M1 gate: 所有 action 必须实现、明确 disabled 并解释原因，或从当前 scope 中移除；不得使用空 handler 或看似可点击的静态元素。

### F-003 High：discovery 缺少 running 和 failure 状态

设计合同要求 Discovery `ready`、`running`、`complete`。当前 `scanCompleted` boolean 在点击后同步从 Ready 跳到 Complete，没有 pending、cancel、retry 或 error surface，不能作为真实 Cube 增量扫描的交互模型。

Evidence: `docs/specs/product-prototype/product-design.md:45`；`prototypes/product/src/App.tsx:687`、`:742`；`prototypes/product/src/JourneyViews.tsx:79`。

M1 gate: 以 server-owned run state 显示 queued/running/succeeded/failed/cancelled，保留进度、可诊断错误、retry 与后台恢复语义。

### F-004 Medium：1024 viewport 隐藏当前 release

产品合同要求 global shell 始终显示 workspace、mock boundary、全局搜索、主导航和当前 release。`1024x768` 中 mock boundary 可见，但 `Stable · 2026.08.3` 被响应式样式隐藏。

Evidence: `docs/specs/product-prototype/product-design.md:73`；`prototypes/product/src/App.tsx:175`；`docs/evidence/M1-FE-AUDIT/screenshots/overview-1024x768.png`。

M1 gate: 在两个支持 viewport 保留当前 workspace、环境/数据边界和 release identity；可缩短文案，但不能完全消失。

### F-005 Medium：empty result 与 stale detail 同时存在

资产搜索结果为零时，catalog 正确显示“没有匹配资产”，详情 pane 仍显示先前选择的“净收入”。用户可能把 stale detail 误认为当前过滤结果。

Evidence: `prototypes/product/src/App.tsx:390`、`:436`、`:440`；`docs/evidence/M1-FE-AUDIT/screenshots/assets-empty-1024x768.png`。

M1 gate: zero-result 时隐藏详情，或明确标注“保留的先前选择，不在当前结果中”，并提供清除筛选动作。

### F-006 Medium：紧凑桌面存在轻微内部裁切

`1024x768` 中提案导航、lifecycle stage 和部分 domain row 的 `scrollWidth` 分别比 `clientWidth` 大 3 px、3 px 和 8 px。没有 document-level overflow，但焦点边界和文字末端可能被裁切。

Evidence: `docs/evidence/M1-FE-AUDIT/screenshots/overview-1024x768.png`、`assets-1024x768.png`。

M1 gate: 每个固定轨道、segment、row 和 button 使用稳定 grid track、min-width 与 overflow policy；自动测试断言关键控件 `scrollWidth <= clientWidth`。

### F-007 Medium：关系图使用装饰性点阵背景

Overview/lineage 图面使用 radial-gradient 点阵。项目视觉合同明确主工作区不使用方格纸、ruled-line 或 decorative gradient 背景。

Evidence: `docs/specs/product-prototype/product-design.md:125`；`prototypes/product/src/styles.css:1415`、`:3938`、`:4075`。

M1 gate: 生产关系图使用纯净中性 canvas，只保留表达层级、选择、方向和状态所需的图形信息。

### F-008 Medium：prototype source 不适合直接生产化

`App.tsx` 约 756 行，`styles.css` 约 4474 行；同一 selector 在后置 override 区域重复定义，原型 state、业务视图、dialog 与 shell 紧密耦合。直接复制会让设计 token、feature ownership、测试和 responsive fixes 失去清晰边界。

M1 gate: 按 shell、sources、assets、shared UI 和 graph feature 拆分；通用交互来自 Radix/shadcn，业务数据来自生成 API + Query，CSS 由 token 和局部组件层组织。

### F-009 Medium：质量门禁缺少 axe、视觉回归和跨浏览器矩阵

现有 unit/E2E 主旅程通过，但 Playwright 没有 `@axe-core/playwright`、`toHaveScreenshot` 稳定基线或 Firefox/WebKit 项目，也没有 bundle/performance budget gate。

M1 gate: 执行 TDR-013 的 accessibility、visual regression、desktop browser matrix 和 bundle budget；截图差异必须由人工确认，不能盲目更新。

## 4. 已验证优势

- 生产 Web 只使用生成 SDK 调用真实 status API，prototype 明确隔离且持续显示 `Prototype · Mock data`。
- 1440 和 1024 的三层信息架构清楚，没有 document-level horizontal overflow 或明显内容重叠。
- Assets 的搜索、类型 filter、选中态、empty copy 与详情 tabs 已证明工作台主模型可理解。
- Proposal dialog 在两个目标 viewport 都完整落在可视区域，关闭按钮有可访问名称，Review dialog 支持 Escape 和 focus restore。
- 状态不只依赖颜色，核心视图包含 icon、label、文字摘要和 visible focus styling。
- 无控制台 error/warning，当前 lint、typecheck、build、unit 和 E2E 基线全部通过。

## 5. Design Quality Rubric

评分：0 缺失，1 基础，2 强。通过要求总平均不低于 1.5，且核心维度无 0。

| 维度 | 分数 | 依据 |
| --- | ---: | --- |
| Product fit | 2 | 高密度语义治理工作台与目标用户匹配 |
| First viewport | 2 | 无营销 hero，进入产品工作面，核心态势清楚 |
| Visual ambition | 1 | 稳定专业，但独有视觉记忆仍弱 |
| Design thesis | 2 | quiet operational workbench 一致 |
| Signature element | 1 | focus line 存在，但未贯穿所有关键交互 |
| Crafted interactions | 1 | 核心 journey 可演示，多个控件和 modal contract 不完整 |
| Hierarchy | 2 | activity rail、context、canvas 层级清楚 |
| Typography | 2 | 紧凑、可扫描，CJK 与技术 metadata 区分明确 |
| Palette | 2 | 中性 canvas 配合功能色，非单色主题 |
| Layout/responsiveness | 1 | 目标 viewport 可用，1024 有隐藏信息与轻微裁切 |
| Components/states | 1 | selected/empty/passed 等良好，running/error/disabled 不完整 |
| Assets/content | 2 | 真实领域对象和 code-native relationship artifact |
| Motion | 1 | reduced-motion 有方向，交互连续性尚未形成完整证据 |
| Contract fidelity | 1 | 大体符合，仍有 release、grid background 和状态偏差 |
| **Total** | **21/28** | **Pass with concerns，1.50/2** |

## 6. M1 前端退出门禁

M1 production frontend 只有同时满足以下条件才可接受：

1. 真实 Cube project 可完成连接、running discovery、成功或可诊断失败、资产浏览和增量刷新。
2. 两个目标 viewport 没有页面或关键控件横向溢出、文本裁切、重叠和不可达操作。
3. 所有 dialog 通过 keyboard focus containment、restore、Escape、background inert 和 screen-reader name 检查。
4. 每个请求 surface 具备 loading、empty、error、retry、stale 和 permission state；不存在无行为控件。
5. `@axe-core/playwright` 的 serious/critical violation 为零，核心旅程通过 Chromium、Firefox 和 WebKit。
6. 关键视图拥有人工审核的 screenshot baselines，mock fixture 与 production data boundary 明确。
7. 初始 shell JS/CSS、graph lazy chunk 和首次可交互时间满足 TDR-013 预算。
8. UI chrome 同时提供中文和英文，协议名、产品名、API、key、URL 和 Cube identifiers 保持原文。

## 7. Evidence Index

| 文件 | 状态 |
| --- | --- |
| `screenshots/overview-1440x900.png` | 1440 Overview baseline |
| `screenshots/overview-1024x768.png` | 1024 Overview compact baseline |
| `screenshots/assets-1440x900.png` | 1440 Assets baseline |
| `screenshots/assets-1024x768.png` | 1024 Assets compact baseline |
| `screenshots/assets-empty-1024x768.png` | zero-result + stale detail finding |
| `screenshots/proposals-1440x900.png` | proposal workbench baseline |
| `screenshots/proposal-dialog-1440x900.png` | desktop dialog baseline |
| `screenshots/proposal-dialog-1024x768.png` | compact dialog baseline |

## 8. 后续顺序

1. T007 的本地与托管证据已由 founder 接受。
2. 完成正在执行的 T008 fresh-clone、module identity、运维文档和 M0 release report，并由 founder 明确接受 M0。
3. 编写 M1 plan、requirements checklist、work graph 和 consistency analysis。
4. 先确认领域/API/Cube/Git/UsageEvent 契约，再建立生产 Web foundation。
5. 依次实现 Sources/discovery、Assets catalog/detail、关系图、最小服务端信号与完整质量门禁。

本审计是 M1 计划的输入，不创建 Ready task，也不修改已接受的 prototype。

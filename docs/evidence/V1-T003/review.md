# V1-T003 独立评审

状态：技术评审通过，最终用户验收待统一处理。

## 已核对

- 精确修订请求按工作区、资产与修订缓存；工作区 generation 与 promise identity 清理抵御切换后晚到响应；不以当前版本或目录第一项替代缺失引用。
- R1 依据使用独立内容视图，展示回答发布版本与引用修订，不借用 R2 的发布状态或主张该引用就是回答 release 中新发布的资产版本。
- 纠错读取 exact asset 的当前基线；loading/error 挡住旧缓存，403/404 有明确失败，不进入其他资产。
- 已完成问答与短暂结果保留用于返回导航；未进入 Ask 不读取执行历史，隐藏中的请求按 signal/controller fencing 停止等待，不自动重执行。HTTP 中止不被描述为服务端执行已经取消。
- 无 `asset.propose` 能力的用户在修订入口获得明确提示，不能进入假编辑；修改本次问题仍可用。
- 同主体授权版本刷新遵循现有会话提示，保留已经读取的内容，后续操作仍由当前权限和服务端再次授权。CatalogRuntimeProvider 以 account/principal 为统一隔离键，主体切换清除目录、历史与问答子树。

独立审查者执行 Catalog、KnowledgeSpecView、KnowledgeViews、AskExecutionPanel、routes 五个 focused 文件，53 项测试通过；最终主体隔离复核及串行 App/Catalog 33 项通过，无未解决的上述源码问题。作者六文件 108 项回归通过，全量单 worker 38 文件 / 315 项通过。并发构建期间的全量测试曾有四个等待超时，该失败保留，不以增加超时或削弱断言处理。

## 视觉预检

主任务实际查看 1440x900 来源运行详情及 1024x768 自然知识修订截图：合成来源、对象与当前基线清晰，无横向溢出或控件文字重叠。预检来自正常登录和真实 API，没有模型调用或治理写入，不替代最终主旅程。

`/sources/{id}` 的现有契约是定位列表行并赋予焦点，不是自动打开详情。验收驱动通过“查看来源”进入详情；该调整属于测试假设修正，不作为产品缺陷。

既有独立 production harness 最终 12/12 通过，包含两尺寸十对象验证、独立审核、发布、不可变清单、回滚/恢复与既有资产匹配，owner `spacc_326550cc6ec97da4` 的容器、卷和网络已清理。主任务实际查看 `matched-desktop.png` 与 `restored-compact_desktop.png`，未见横向溢出或重叠。首轮新增恢复 locator 错把 detail 当含 title 的失败保留，修正仅改为从正常 list 按 exact operationId 取 summary，不删治理断言。该 harness 不替代正常密码与真实模型的 V1 验收。

## 自然修订

自然修订入口调用 legacy proposal API，正式 production 资产的 reservation 门禁返回 422 INVARIANT_VIOLATION。主旅程停在模型调用和发布之前；16 proposals / 16 operations / 17 releases 不变，legacy proposals 为 0，正确基线仍在。需要复用正式修订契约完成修复，不放松 reservation 或原子发布门禁。

只读诊断确认 migration 26 的生效门禁支持正式 production update，旧入口失败属于 T003 接线缺口，T002 后端不重新开放。A003 使用现有 production create/update 契约：精确编辑基线、可验证的历史生产来源、原始证据和依赖 pin、清空已消费候选链接；仅创建 draft，人工业务确认及独立审核仍在原工作台完成。历史来源不可验证或超过有界追溯范围时明确拒绝，不回退旧提案。

A003 独立复核通过，四个 focused 文件 86 项测试通过。两个审查发现有单独 RED/GREEN：混合集合的 `no_change` 目标继续追溯真实 producer；创建结果未知后重试收到 403 仍保留同一 body/key，不把拒绝解释为原请求未提交。编辑基线在开启时固定，未知命令禁止改写，主体/工作区晚响应不能导航；创建后只进入 draft，原因及回答 pin 明确未确认。历史追溯上限 64，依赖相关历史读取权限；浏览器 pending key 在内存中，不声称刷新后持久恢复。

正常密码的两尺寸功能旅程与自然歧义已通过，共 3 次模型调用、2 次执行；全程正常界面确认、独立审核、发布和回滚，具体账目见 browser-acceptance.md。

视觉失败证据保留：主任务实际查看 `1790433741115/historical-r1-compact-desktop.png`，历史知识标题与正文被挤入图标宽度的竖列。新视图缺少普通资产头部的三列 DOM 骨架，无横向溢出断言不能识别这种错误。

最小修复复用既有图标、标题、操作区布局；专属历史视图取消外框和分区框，普通目录不变。Catalog 26 项及 typecheck 通过，最终只读浏览器两尺寸通过（run `1790434400257`）。主任务查看最终 compact 截图，标题、正文和按钮正常，无嵌套卡片或溢出。

补充 `result-visibility-check` 单独调用 1 次真实模型并执行 1 次，同一 context 调整两尺寸；结果 1500、日期与独立 SQL 一致。每个结果单元格三点 hit-test 通过，正常滚动后结果和参数位于 composer 上方，主任务实际查看两尺寸结果图及 SQL 图。该额外 1/1 不并入原主验收 3/2 分母。

## 收口

- T003 共 4 次实际模型调用、3 次执行；全部新增执行成功，歧义无执行。累计模型 17、执行 16，未结束 agent 为 0。
- 正确 head 为 `rls_01m3f2pbg0f1br2ba4hbtgjgrs`，模型内容恢复原始基线；18 operations / 18 production proposals / 21 releases，legacy proposals 为 0，10 assets / 3 governed objects。
- 所有浏览器与 launcher 会话结束，正常 V1 runtime 保留。`v1.mjs verify` 通过，原环境配置指纹一致，原 `semlia` 库仍为 schema 32。私有 164 文件 / 40 目录权限与无链接检查通过。
- 补充视觉 Playwright 本身通过且 context 已释放，原 launcher 在后置收口退出 1，原原因未分类。该失败不改写为成功；独立 `v1-browser-postflight.mjs 1790434658272` 不启动浏览器或模型，核验 child pass、释放状态、计数、head/pin 和内容后退出 0。保留此验收工具的残余诊断限制。
- 产品与已修复风险无未解决阻断；最终全量源码、构建、安全及候选冻结门禁属于 T005。浏览器 pending key 不跨刷新持久保存，历史生产来源追溯有 64 跳上限，均须在交付中说明。

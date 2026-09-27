# V1-T003 桌面闭环验收

状态：Needs_Review；技术评审通过，最终用户验收待统一处理。

桌面 1440x900 与紧凑桌面 1024x768 的正常密码、来源核对、自然知识修订、显式业务确认、独立审核、发布、真实问数和只读执行、历史依据、纠错入口与 UI 回滚均通过。原模型内容保持正确，未放松后端治理门禁。

精确引用与账户缓存隔离、隐藏请求中止、当前编辑基线、正式 production update 接线和未知创建结果保护均有 focused 回归与独立审查。全量单 worker 基线 315 项通过；后续自然修订 focused 86 项、最终 Catalog 26 项及 typecheck/lint 通过，lint 有四条原有 warning。既有 production desktop harness 12/12 通过，所有自有容器、卷和网络已清理。

正常浏览器主验收单独为 3 次模型调用 / 2 次执行；结果可见性补充为 1 次模型调用 / 1 次执行。全局累计模型 17、执行 16，含此前保留的修复前失败，不能当作统一候选首轮成功率。T005 最终候选 cohort 尚未执行。

历史页视觉竖列缺陷有真实截图 RED，最小 DOM/CSS 修复后两尺寸通过。结果表和原 SQL/参数经正常滚动、矩形与 hit-test 核验可访问。补充视觉 Playwright 通过，原 launcher 后置退出 1 保留；独立无重放 postflight 核验通过，原原因仍未分类。

详细证据：`frontend.md`、`natural-correction.md`、`browser-acceptance.md`、`review.md`。最终全量候选门禁与用户验收尚未完成。

# 桌面验收

专用 Linux 完整套件的浏览器部分 4/4 通过，用时 3.8 分钟。视口为 1440×900 与 1024×768，均连接本轮专用真实 API 与 PostgreSQL；合成业务来源不等于预置语义资产。外部消费者和 EAI 不参与此生产链路。

覆盖空工作区、来源发现候选、十目标生产集合、人工业务纠正与声明、完整验证、分离审核和发布、变化版本、回滚、清除 localStorage 后重开恢复，以及候选匹配已发布对象。AI 浏览器步骤用于确定性协议验收；真实供应商质量独立见 `model-quality.md`。

主代理实际查看本轮 `desktop/published-desktop.png`、`desktop/corrected-compact_desktop.png`、`desktop/published-compact_desktop.png` 及 `desktop/keyboard-match-desktop.png`：字体正常，发布对象卡片和两列紧凑布局无文字相互遮挡，匹配弹窗焦点环可见，发布摘要可读。紧凑发布截图保留了测试的滚动位置，不冒充首屏截图。

浏览器断言包含清除本地缓存后从服务器恢复、键盘 Enter 打开匹配弹窗、Escape 返回触发按钮、可见焦点、reduced-motion 媒体设置和页面横向溢出检查。截图是代表状态检查，不声称覆盖每个长文本组合。

完整套件退出码 0。配套六包新鲜集成回归全部通过；10,000 资产基准分页 p95 15.025ms、搜索 p95 167.496ms，均低于 150/250ms 门槛。测试数据库、项目网络和卷已按 owner `spacc_4ecd7421b6804ffb` 清理，见 `desktop/cleanup.log`；整台专用机器的最终清理另行记录。

本次 full suite 的精确快照是 `full-suite-snapshot-manifest.json`。随后只读复核要求加强直接模型测试入口的预留保护；该测试辅助与反例的最终版本由 `snapshot-manifest-final.json` 和后续完整门禁覆盖，不改浏览器或产品业务实现。

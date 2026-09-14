# T006 桌面语义生产交付

Status: Accepted

执行：Direct Execute，SP-T006-A001 及验收复核。用户批准的条件验收已通过，详见 [验收复核](acceptance-review/re-review.md)。T007 获准执行。

## 交付范围

- 从真实来源候选新建语义资产或匹配已有资产；绑定、实体键、粒度与 Join 使用具名固定版本引用。
- AI 原始建议、应用版本和人工纠正分别保留；业务规则显式确认，冻结验证、独立集合审核、发布与回滚连续完成。
- 冻结版本的纠正创建后继记录；旧验证不能用于新内容。服务器列表及 URL 支持刷新、清除 localStorage 后恢复。
- 回滚发布定位、真实 outbox 动作及目录字段标识兼容修复；来源返回、生产列表返回和原有忽略候选入口保留。
- 独立 Compose 验收启动器、两尺寸浏览器测试及嵌入前端资源同步。

## 验证

验收复核隔离项目 `spacc_143493efba543351`：**4 passed，1.8m**。1440x900 和 1024x768 均完成 SQL 工件导入、发现、十对象建模、协议替身生成及人工纠正、无确认阻断、规则确认、重验、独立审核、冻结后继纠正、发布、回滚、二次回滚、清单相等与原发布不可变、已有资产匹配和草稿保存。键盘打开/Escape 返回焦点、reduced-motion 计算样式、无视口横向溢出通过。

前端 **26 files / 230 tests** 通过。并发读取错误隔离及读失败写入锁定的两个反例通过 RED/GREEN。全量治理与目录集成、typecheck、lint、build、web-embed 和 web-embed-check 通过；早期组合发布 race 及相关检查见 [A001 验证结果](A001/test-results.md)，新鲜复核结果见 [验收记录](acceptance-review/re-review.md)。

本次 container、network、volume 已全部按 owner 清理，见 [清理日志](A001/cleanup.log)；清理后分别查询三类 owner 资源均为空。默认开发栈保持运行，未部署、提交或推送。

## 复核证据

[交付评审](A001/review.md)、[源码差异](A001/source-delta.patch)、[文件清单](A001/changed-files.json)、[嵌入资源校验值](A001/embedded-manifest.json)。未发现本任务范围内的剩余阻断问题；评审由当前执行者完成，不冒充独立第三方审查。

[桌面建模](A001/corrected-desktop.png)、[紧凑桌面建模](A001/corrected-compact_desktop.png)、[桌面匹配焦点](A001/keyboard-match-desktop.png)、[紧凑桌面匹配焦点](A001/keyboard-match-compact_desktop.png)、[回滚恢复](A001/restored-compact_desktop.png)。

[桌面真实版本与发布对象](A001/objects-desktop.json)、[紧凑桌面真实版本与发布对象](A001/objects-compact_desktop.json)、[匹配更新及基础 revision](A001/matched-desktop.json)。原始浏览器和服务日志保存在 `.semlia/production-acceptance/spacc_817d46e7508d4484/`。

## 边界

实际模型未调用，本轮只证明显式 `protocol_stub` 路径，不证明模型质量或实际费用。T005 记录的五项旧数据库测试失败、三个既有 lint warnings 和构建大 chunk 提示保留。通用 delivery validator 只识别 `.ai-platform` 布局，对本仓库 `docs/specs` 布局返回缺失路径；已按本仓库契约和真实文件复核，不将该工具结果记为通过。

T006 已按用户条件授权验收。T007 单独记录实际模型、完整数据库与全量门禁结果。

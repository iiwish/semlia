# SP-T007 交付摘要

状态：Accepted。用户于 2026-09-11 授权检查验收，完成 [验收复核](acceptance-review.md)，未发现阻断问题。SP-T001 至 SP-T007 全部 Accepted；不启动外部 EAI 工作，不包含部署、提交或推送。

## 结果

- SP-AC-001 至 SP-AC-012 的正常、权限拒绝、失败恢复、变化生产和回滚证据完整，见 [验收矩阵](A001/acceptance-matrix.md)。
- 专用环境 full suite 退出 0：两个桌面尺寸浏览器 4/4、六包新鲜回归及 10,000 资产基准通过。分页/搜索 p95 为 15.025/167.496ms。
- 最终 grpc 1.83.2、js-yaml 4.3.2 安全补丁快照的完整 `make check` 退出 0，前端 230/230，真实栈 smoke 通过；依赖及发布镜像 HIGH/CRITICAL 均为 0。
- 真实 deepseek-v4-flash 使用 4/30 次授权合成调用。最终案例原始结果 6/8、合成业务纠正后 8/8；结构骨架已提供，不代表开放式建模准确率或真实业务签署。
- 受限兼容测试、纯格式和文档路径修复保留原保护断言。模型调用预留防护发现的 P2 经真实 RED/GREEN 修复，独立复核无新增 P1/P2；安全补丁增量独立复核通过。

## 边界

原失败日志保留；不忽略安全问题、不降低断言或扫描标准。专用 native 构建环境使用串行 Compose；目录按钮曾发生一次偶发等待失败，原命令及后续完整门禁通过，三个既有 lint 警告未越界修正。

full suite 与最终 `make check` 各自绑定精确快照，Go 缓存结果不声称新鲜运行，补丁前的浏览器成绩不声称补丁后重跑。模型结果仍需要业务规则确认、完整验证及独立审核。

## 证据入口

- [命令、退出码及失败归因](A001/test-results.md)
- [阶段验收报告](../../specs/semantic-production/release-report.md)
- [变更清单与前像](A001/changed-files.json)、[任务增量](A001/source-delta.patch)
- [安全补丁与扫描](A001/security-review.md)
- [独立工程复核](A001/review.md)、[桌面截图](A001/desktop-review.md)
- [专用环境与清理](A001/environment.md)

默认开发栈与客户数据不是测试目标；全部广泛既有工作区改动保留，未执行 Git 写操作。

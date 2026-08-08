# Semlia Documentation

本目录是 Semlia 的项目级事实来源。产品、架构、治理和交付决策以 [SSOT.md](./SSOT.md) 为准。

## 文档状态

| 文档 | 作用 | 当前状态 |
| --- | --- | --- |
| [SSOT.md](./SSOT.md) | 产品宪法、范围、领域模型、架构、质量标准和路线图 | Confirmed |
| [M0 TDR](./adr/0001-m0-technical-foundation.md) | M0 技术选择、取舍、风险和任务影响 | Confirmed |
| [M0 Plan](./specs/m0-foundation/plan.md) | M0 范围、需求、交付顺序和退出标准 | Confirmed |
| [M0 Work Graph](./specs/m0-foundation/tasks.md) | Epic、Story、Task、依赖、文件边界和验证命令 | Confirmed |

## 使用规则

1. 产品范围、术语、核心流程和技术边界只在 `SSOT.md` 中定义一次。
2. 功能规格、ADR、API 合约和运行手册必须引用对应的 SSOT 条款，不得复制后形成第二套定义。
3. 任何改变产品定位、资产模型、发布治理、安全边界或兼容性承诺的决策，都必须先更新 SSOT 并经过维护者审核。
4. 已发布行为与 SSOT 不一致时，该问题按 P0 文档或实现缺陷处理。
5. `Confirmed` SSOT 是产品和技术计划的阻断性项目政策；与其冲突的实现必须先修改 SSOT 并重新审核。

## 后续文档布局

```text
docs/
  README.md
  SSOT.md
  adr/                 # 已接受的技术决策
  specs/               # 经确认的功能规格
  contracts/           # OpenAPI、事件和插件契约
  operations/          # 部署、备份、恢复和安全运行手册
  contributing/        # 开发、测试、评审和发布指南
```

只有在对应内容产生时才创建子目录，避免空文档和重复治理。

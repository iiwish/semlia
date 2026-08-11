# Product Prototype Requirements Checklist

## 元数据

| 字段 | 值 |
| --- | --- |
| Feature | product-prototype |
| 状态 | Completed |
| Date | 2026-08-09 |

- [x] 原型目标是产品评审，不声明生产能力。
- [x] 核心 audience、六个 views 和七阶段主旅程明确。
- [x] mock data、session-only mutation 和 production boundary 明确。
- [x] 首屏、desktop、compact desktop、keyboard、reduced motion 和 empty state 可验证。
- [x] 视觉 tokens、布局、组件、motion 和禁止项使用精确约束。
- [x] 资产策略使用产品相关的 code-native 图谱和 timeline，不依赖远程媒体。
- [x] lint、typecheck、test、build、Playwright 和 screenshot commands 明确。
- [x] P001 不修改生产 API、领域 schema、`web/**` 或已生成 SDK。
- [x] 用户接受原型前，不把其行为视为 M1 product contract。
- [x] SSOT J-001 至 J-006 都有可点击入口、状态结果和下一个 handoff。
- [x] Discovery、candidate、publish、rollback、binding 和 feedback 只改变 session state。
- [x] 1440px 普通桌面与 1024px 紧凑桌面均有明确布局规则。

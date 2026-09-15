# 业务规则确认

状态：Confirmed。归属 SP-T005-A002，2026-09-11 用户批准受限扩展。

所有有实质变更的 semantic_asset 的 definition 和 scope 必须由有写入权限的真实人类明确确认。非空文本、普通 source observation、inferred evidence、模型输出、declared 类型或 metadata 自身均不是确认。reference/no-op 目标不创建新的业务定义；历史已发布内容不伪造回填确认。

POST `production-operations/{operationId}/business-rule-confirmations` 接收 expectedVersion、setDigest、targetKey、action（confirm/revoke）。confirm 必须在 evidenceId 和 declaration 中选择一项：已有 evidenceId 必须是当前版本 input 选定且目标 evidenceIds 引用的 declared artifact；declaration 是有权人类明确提供的业务规则声明（1 至 8192 字节），服务器在同一事务创建绑定目标完整内容的人类声明证据。该证据通过确认事件引用，不改写冻结版本的 input 或目标 evidenceIds。confirm 表示人类确认该证据支持目标的业务定义与范围；来源类型、证据摘要、目标内容摘要、版本、workspace、实际会话主体和授权版本由服务器绑定。客户端不得指定确认人、证据来源类型或摘要。

记录为不可变追加事件；同目标最近事件决定当前确认状态。revoke 由同等当前人类写权限执行，保留原确认与撤销记录；再次确认需要新 key。Idempotency-Key 在 workspace 和主体内唯一，重放必须重新检查当前授权和目标版本，不重复写入。确认和撤销均纳入操作贡献者，不能审核或发布同一操作。

每版本最多 256 个事件。GET 同路径必须提供 `version`，返回按 targetKey 排序的最新事件及当前 valid 状态。重放返回原事件并标记 replayed，不改变后来发生的撤销状态；已发布操作拒绝确认、撤销及写入重放。

确认可以发生在草稿或冻结版本，不能修改已发布操作。冻结版本新增或撤销确认使旧验证失效，必须重新验证与审核。版本变化不继承确认，即使内容摘要相同。验证排队、执行、提交结果、批准和发布均读取同一可信确认见证；权限撤销、主体停用、证据失效或新事件使旧凭证不能用于发布。锁顺序沿用 workspace -> operation -> version，和权限变更/发布串行化。

验证器版本升级为 production.2，旧凭证必须重新验证。没有确认的目标产生 PRODUCTION_BUSINESS_RULE_UNCONFIRMED blocker，不拒绝保存未决草稿。API 不新增角色，复用目标读取/写入与 asset.propose 授权，并额外要求 human 主体。

## 预检

- [x] 授权、版本、证据与 SoD 边界明确；不把业务确认等同 release review。
- [x] 旧验证升级和版本失效规则明确；不回填历史信任。
- [x] 迁移仅新增 28；修复范围不包含 T006、部署或真实模型。
- [x] RED、正反例、撤销、竞态和发布复核均有验证计划。

分析结论：现存 P1 是本次修复目标；受限方案无额外未决 High/Critical 设计问题。实现完成前 P1 保持开放。

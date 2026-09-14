# SP-T003 A003 验收复核

Status: Accepted

授权：2026-09-10 用户要求“review 检查一下，没问题的话就完成 T003 的验收，继续 T004”。本记录按该条件授权完成 T003 验收；不是由执行授权自动推断接受。

## 复核结论

T003 authoring 范围无未解决的已证实阻断。原 F1 至 F5 的输入真实性、原子替换、冻结、历史/重放及真实 published update 均有正式 PostgreSQL 回归覆盖。本次重新检查规范化、服务端声明转换、事务权限/来源复验、baseline replay、supersede、存储防线和有界列表，并重新执行全部 20 个 T003 authoring 顶层集成测试。

复核发现并修复两项边界问题：

1. 规范化函数比较未验证的 `sourceId` interface，内部服务收到对象/数组时会 panic。HTTP typed decode 已有拒绝，但内部调用仍需返回领域错误。补充非空字符串检查与类型安全排序；[RED](canonical-red.log) 真实复现 panic。
2. operation 列表读取页后再读取 latest，在并发 PUT 时可能用 v1 版本号返回 v2 的目标数量/冻结状态，且读取内容偏离筛选版本。逐项恢复固定为页查询选择的 version；[RED](list-red.log) 用确定性 interleaving 复现 v1/两个 v2 targets 的混合结果。

两项正式单元回归与相关三个完整包通过：[GREEN](unit-green.log)，domain 0.656 秒、application 0.708 秒、HTTP 2.741 秒，exit 0。未用概率睡眠模拟列表竞争。

## 运行时验证

- 修改前本次验收重跑 20 个 authoring 顶层测试，exit 0，25.592 秒：[详细日志](runtime.log)。
- 两项修复后同一完整 authoring 集成集重跑，exit 0，54.481 秒：[最终日志](runtime-final.log)。涵盖五类 create/update/no_change、真实引用与错误父版本、作用域授权/历史撤权、原命令恢复、候选决策、supersede、原子失败、存储旁路和同键/不同键竞争。
- 完整 governance/catalog/discovery、migration 25 及 PG17 兼容、readiness、生成/契约/仓库/SDK 类型检查沿用同日 A003 已完成证据，见 [交付报告](../repair-20260910/report.md)。本次两项修复未改变 schema、生成物或 T004 业务段，不声称重新执行了所有迁移。
- `git diff --check` exit 0。所有数据库测试使用自建 PG18，清除外部数据库及执行覆盖变量；无现有数据库迁移、部署、Git 提交或模型调用。

## 验收边界

验收限于 T003 的草稿生产、替换和恢复，不认可 T004 A001 的验证/发布完成声明。可信 release proof 下的 reuseIdentity、validator/policy 完整绑定、提交/重验/审批/发布/回滚的事务边界属于下一 T004 复核执行范围；T005 的真实生成输出和应用仍未实现。相关证明缺失时继续 fail closed，不虚构成功。

进程重启使旧列表游标失效；需重新发起查询。极限响应预算与并发交错未做穷举。复核由当前执行者以独立检查步骤完成，没有虚构第二位审查者或外部批准。

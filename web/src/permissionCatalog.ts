import type { PermissionGroup, PermissionAction } from "./types";

export const permissionGroups: PermissionGroup[] = [
  {
    id: "workspace",
    label: "工作区管理",
    description: "成员、角色与授权策略管理",
    actions: ["workspace.read", "workspace.manage", "member.read", "member.manage", "group.manage", "role.read", "role.manage", "role.assign", "authorization.inspect"],
  },
  {
    id: "governance",
    label: "语义治理",
    description: "资产、证据、提案和验证",
    actions: ["asset.read", "asset.propose", "asset.edit", "evidence.read", "proposal.review", "validation.run"],
  },
  {
    id: "release",
    label: "发布管理",
    description: "受保护版本的发布与回滚",
    actions: ["release.publish", "release.rollback"],
  },
  {
    id: "data",
    label: "数据接入",
    description: "来源、摄取和物理绑定",
    actions: ["source.read", "source.manage", "ingestion.run", "binding.read", "binding.manage"],
  },
  {
    id: "runtime",
    label: "消费与运行",
    description: "语义解析、查询、审计和运行时",
    actions: ["semantic.resolve", "semantic.execute", "audit.read", "runtime.read", "runtime.manage"],
  },
];

export const permissionDescriptions: Record<PermissionAction, string> = {
  "workspace.read": "查看工作区基本信息和治理配置摘要。",
  "workspace.manage": "修改工作区级配置和全局治理参数。",
  "member.read": "查看成员、用户组和授权状态。",
  "member.manage": "邀请、停用成员并维护成员资料。",
  "group.manage": "创建用户组并维护组成员关系。",
  "role.read": "查看系统角色、自定义角色和权限定义。",
  "role.manage": "创建、修改或停用自定义角色。",
  "role.assign": "为主体分配或撤销范围化角色。",
  "authorization.inspect": "检查主体对指定资源的最终有效权限。",
  "asset.read": "查看语义资产、定义、关系和版本信息。",
  "asset.propose": "为语义资产创建结构化变更提案。",
  "asset.edit": "编辑语义资产草稿和待发布定义。",
  "evidence.read": "查看资产证据、来源和验证引用。",
  "proposal.review": "审核语义提案并记录评审结论。",
  "validation.run": "运行语义、引用、SQL 和策略验证。",
  "release.publish": "发布通过治理门禁的候选版本。",
  "release.rollback": "将环境恢复到上一稳定发布版本。",
  "source.read": "查看数据来源、连接状态和元数据快照。",
  "source.manage": "创建、修改、轮换或停用数据来源。",
  "ingestion.run": "启动数据接入、扫描和增量同步任务。",
  "binding.read": "查看物理绑定、粒度和 JoinContract。",
  "binding.manage": "创建或修改资产的物理绑定关系。",
  "semantic.resolve": "将业务语义请求解析为已发布资产和计划。",
  "semantic.execute": "执行经过验证且受发布约束的语义计划。",
  "audit.read": "查看不可变审计事件和授权操作记录。",
  "runtime.read": "查看运行记录、阶段、日志和诊断信息。",
  "runtime.manage": "管理运行队列、重试和运行时配置。",
};

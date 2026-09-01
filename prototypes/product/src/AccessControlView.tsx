import { useEffect, useMemo, useState } from "react";
import { createPortal } from "react-dom";
import { Check, CheckCircle2, CircleAlert, Copy, KeyRound, LockKeyhole, Pencil, Plus, Search, ShieldCheck, X } from "lucide-react";

import { evaluateAuthorization, findSeparationOfDutyConflicts } from "./authorization";
import { authorizationBindings, authorizationPrincipals, authorizationRoles, permissionDescriptions, permissionGroups } from "./data";
import type { AuthorizationBinding, AuthorizationDecision, AuthorizationResource, AuthorizationRole, AuthorizationScope, PermissionAction } from "./types";

type AccessControlTab = "roles" | "assignments" | "inspect";

const tabLabels: Record<AccessControlTab, string> = {
  roles: "角色与权限",
  assignments: "角色分配",
  inspect: "有效权限检查",
};

const scopeOptions: Array<{ value: string; scope: AuthorizationScope; resource: AuthorizationResource }> = [
  { value: "workspace:WS-SEMLIA", scope: { type: "workspace", id: "WS-SEMLIA", label: "Semlia 工作区" }, resource: { type: "workspace", id: "WS-SEMLIA" } },
  { value: "domain:commerce", scope: { type: "domain", id: "commerce", label: "收入域", protected: true }, resource: { type: "domain", id: "commerce", protected: true } },
  { value: "asset:METRIC-NET-REVENUE", scope: { type: "asset", id: "METRIC-NET-REVENUE", label: "净收入", protected: true }, resource: { type: "asset", id: "METRIC-NET-REVENUE", domainId: "commerce", environment: "production", protected: true } },
  { value: "source:SRC-PROD-WAREHOUSE", scope: { type: "source", id: "SRC-PROD-WAREHOUSE", label: "生产数仓" }, resource: { type: "source", id: "SRC-PROD-WAREHOUSE" } },
  { value: "environment:production", scope: { type: "environment", id: "production", label: "生产环境", protected: true }, resource: { type: "environment", id: "production", protected: true } },
];

const principalKindLabels = { user: "用户", group: "用户组", service_account: "服务账号", api_client: "API 客户端", agent: "Agent" } as const;
const highRiskPermissions: PermissionAction[] = ["workspace.manage", "member.manage", "role.manage", "role.assign", "release.publish", "release.rollback", "source.manage", "binding.manage", "runtime.manage"];

function AssignmentDialog({ roles, bindings, onClose, onConfirm }: { roles: AuthorizationRole[]; bindings: AuthorizationBinding[]; onClose: () => void; onConfirm: (binding: AuthorizationBinding) => void }) {
  const [step, setStep] = useState<"configure" | "review">("configure");
  const [principalId, setPrincipalId] = useState("USR-REVIEWER");
  const [roleId, setRoleId] = useState("ROLE-PUBLISHER");
  const [scopeValue, setScopeValue] = useState("asset:METRIC-NET-REVENUE");
  const [expiresAt, setExpiresAt] = useState("2026-12-31");
  const principal = authorizationPrincipals.find((item) => item.id === principalId)!;
  const role = roles.find((item) => item.id === roleId)!;
  const scope = scopeOptions.find((item) => item.value === scopeValue)?.scope ?? scopeOptions[0].scope;
  const conflicts = findSeparationOfDutyConflicts({ principalId, roleId, scope, roles, bindings });

  const confirm = () => onConfirm({
    id: `BIND-SESSION-${String(bindings.length + 1).padStart(3, "0")}`,
    principalId,
    roleId,
    scope,
    assignedBy: "林悦",
    assignedAt: "2026-09-01 16:20",
    expiresAt: expiresAt || undefined,
    status: "active",
  });

  return createPortal(<div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <section className="review-dialog assignment-dialog" role="dialog" aria-modal="true" aria-label="分配角色">
      <header><div><span className="panel-kicker">{step === "configure" ? "步骤 1 / 2" : "步骤 2 / 2"}</span><h2>分配角色</h2></div><button className="icon-button" type="button" aria-label="关闭角色分配" onClick={onClose}><X size={18} /></button></header>
      <div className="dialog-body assignment-dialog-body">
        {step === "configure" ? <div className="assignment-form">
          <label><span>授权主体</span><select aria-label="授权主体" value={principalId} onChange={(event) => setPrincipalId(event.target.value)}>{authorizationPrincipals.map((item) => <option key={item.id} value={item.id}>{item.name} · {principalKindLabels[item.kind]}</option>)}</select></label>
          <label><span>角色</span><select aria-label="角色" value={roleId} onChange={(event) => setRoleId(event.target.value)}>{roles.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
          <label><span>资源范围</span><select aria-label="资源范围" value={scopeValue} onChange={(event) => setScopeValue(event.target.value)}>{scopeOptions.map((item) => <option key={item.value} value={item.value}>{item.scope.label} · {item.scope.type}</option>)}</select></label>
          <label><span>到期时间</span><input aria-label="到期时间" type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></label>
          <div className="assignment-boundary"><ShieldCheck size={16} /><span><strong>服务端强制执行</strong><small>界面预览用于降低误配风险，最终授权仍以服务端决策与审计记录为准。</small></span></div>
        </div> : <section className="assignment-preview" aria-label="授权变更预览">
          <div className="assignment-preview-path"><span><strong>{principal.name}</strong><small>{principalKindLabels[principal.kind]} · {principal.detail}</small></span><span>获得</span><span><strong>{role.name}</strong><small>{role.category === "system" ? "系统角色" : "自定义角色"}</small></span><span>作用于</span><span><strong>{scope.label}</strong><small>{scope.type} · {expiresAt || "长期有效"}</small></span></div>
          <div className="assignment-capability-diff"><CheckCircle2 size={17} /><span><strong>新增 {role.permissions.length} 项操作权限</strong><small>{role.permissions.slice(0, 5).join(" · ")}{role.permissions.length > 5 ? " …" : ""}</small></span></div>
          {conflicts.length > 0 ? <div className="assignment-conflict" role="alert"><CircleAlert size={18} /><span><strong>职责分离冲突</strong><small>{conflicts[0].message}</small><small>恢复方式：选择独立发布者、非受保护范围，或先撤销冲突分配。</small></span></div> : <div className="assignment-clear"><ShieldCheck size={17} /><span><strong>未发现阻断冲突</strong><small>该分配可以在当前原型会话中创建。</small></span></div>}
        </section>}
      </div>
      <footer><button className="secondary-button" type="button" onClick={step === "configure" ? onClose : () => setStep("configure")}>{step === "configure" ? "取消" : "返回修改"}</button><div>{step === "configure" ? <button className="primary-button" type="button" onClick={() => setStep("review")}>预览授权</button> : <button className="primary-button" type="button" disabled={conflicts.some((conflict) => conflict.blocking)} onClick={confirm}>确认分配</button>}</div></footer>
    </section>
  </div>, document.body);
}

function RoleDetailDialog({ role, bindingCount, onClose, onDerive, onEdit }: { role: AuthorizationRole; bindingCount: number; onClose: () => void; onDerive: () => void; onEdit: () => void }) {
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return createPortal(
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog access-role-dialog" role="dialog" aria-modal="true" aria-label={`${role.name} 角色详情`}>
        <header>
          <div><span className="panel-kicker">{role.category === "system" ? "系统角色" : "自定义角色"}</span><h2>{role.name}</h2></div>
          <button className="icon-button" type="button" aria-label="关闭角色详情" onClick={onClose}><X size={18} /></button>
        </header>
        <div className="dialog-body access-role-dialog-body">
          <p>{role.description}</p>
          <div className="access-role-summary"><span><KeyRound size={15} /><strong>{role.permissions.length}</strong> 项操作</span><span><LockKeyhole size={15} /><strong>{bindingCount}</strong> 个有效分配</span>{role.baseRoleId && <span><Copy size={15} />派生自 <strong>{authorizationRoles.find((item) => item.id === role.baseRoleId)?.name ?? role.baseRoleId}</strong></span>}</div>
          <div className="permission-matrix" aria-label={`${role.name} 权限矩阵`}>
            {permissionGroups.filter((group) => group.actions.some((action) => role.permissions.includes(action))).map((group) => <section key={group.id}>
              <header><div><strong>{group.label}</strong><small>{group.description}</small></div><span>{group.actions.filter((action) => role.permissions.includes(action)).length} / {group.actions.length}</span></header>
              <div>{group.actions.map((action) => {
                const granted = role.permissions.includes(action);
                return <span className={granted ? "permission-point is-granted" : "permission-point is-not-granted"} data-granted={String(granted)} key={action}>{granted ? <Check size={12} /> : <X size={12} />}<span><code data-granted={String(granted)}>{action}</code><small>{permissionDescriptions[action]}</small></span></span>;
              })}</div>
            </section>)}
          </div>
        </div>
        <footer><span className="dialog-footnote">{role.category === "system" ? "系统角色由 Semlia 版本管理，不可直接修改" : "自定义角色的修改会生成新的授权版本"}</span><div><button className="secondary-button" type="button" onClick={onClose}>关闭</button>{role.category === "system" ? <button className="primary-button" type="button" onClick={onDerive}><Copy size={15} />基于此角色创建</button> : <button className="primary-button" type="button" onClick={onEdit}><Pencil size={15} />编辑角色</button>}</div></footer>
      </section>
    </div>, document.body
  );
}

type RoleEditorState = {
  mode: "create" | "edit";
  role: AuthorizationRole;
  baseline: AuthorizationRole;
};

function RoleEditorDialog({ state, assignmentCount, onClose, onSave }: { state: RoleEditorState; assignmentCount: number; onClose: () => void; onSave: (role: AuthorizationRole) => void }) {
  const [step, setStep] = useState<"configure" | "review">("configure");
  const [name, setName] = useState(state.role.name);
  const [description, setDescription] = useState(state.role.description);
  const [permissions, setPermissions] = useState<PermissionAction[]>(state.role.permissions);
  const permissionSet = useMemo(() => new Set(permissions), [permissions]);
  const addedPermissions = permissions.filter((action) => !state.baseline.permissions.includes(action));
  const removedPermissions = state.baseline.permissions.filter((action) => !permissions.includes(action));
  const selectedHighRiskPermissions = permissions.filter((action) => highRiskPermissions.includes(action));
  const valid = name.trim().length > 0 && description.trim().length > 0 && permissions.length > 0;
  const dialogLabel = state.mode === "create" ? `基于 ${state.baseline.name} 创建角色` : `编辑角色 ${state.role.name}`;

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  const togglePermission = (action: PermissionAction) => {
    setPermissions((current) => current.includes(action) ? current.filter((item) => item !== action) : [...current, action]);
  };

  const save = () => onSave({ ...state.role, name: name.trim(), description: description.trim(), permissions });

  return createPortal(<div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
    <section className="review-dialog role-editor-dialog" role="dialog" aria-modal="true" aria-label={dialogLabel}>
      <header><div><span className="panel-kicker">{step === "configure" ? "步骤 1 / 2 · 配置" : "步骤 2 / 2 · 复核"}</span><h2>{state.mode === "create" ? "创建自定义角色" : "编辑自定义角色"}</h2></div><button className="icon-button" type="button" aria-label="关闭角色编辑" onClick={onClose}><X size={18} /></button></header>
      <div className="dialog-body role-editor-body">
        {step === "configure" ? <>
          <div className="role-editor-form"><label><span>角色名称</span><input aria-label="角色名称" value={name} onChange={(event) => setName(event.target.value)} /></label><label><span>角色说明</span><textarea aria-label="角色说明" rows={2} value={description} onChange={(event) => setDescription(event.target.value)} /></label></div>
          <section className="permission-editor" aria-label="权限点配置">
            <header><div><strong>权限点配置</strong><small>使用稳定操作标识；注释说明该权限允许执行的产品行为。</small></div><span>{permissions.length} / {permissionGroups.reduce((total, group) => total + group.actions.length, 0)} 已选择</span></header>
            <div>{permissionGroups.map((group) => <section key={group.id}><header><div><strong>{group.label}</strong><small>{group.description}</small></div><span>{group.actions.filter((action) => permissionSet.has(action)).length} / {group.actions.length}</span></header><div>{group.actions.map((action) => <label className="permission-option" key={action}><input type="checkbox" aria-label={`配置权限 ${action}`} checked={permissionSet.has(action)} onChange={() => togglePermission(action)} /><span><code>{action}</code><small>{permissionDescriptions[action]}</small></span></label>)}</div></section>)}</div>
          </section>
        </> : <section className="role-change-preview" aria-label="角色变更预览">
          <div className="role-change-summary"><span><strong>{addedPermissions.length}</strong><small>新增权限</small></span><span><strong>{removedPermissions.length}</strong><small>移除权限</small></span><span><strong>{assignmentCount}</strong><small>受影响分配</small></span><span><strong>{selectedHighRiskPermissions.length}</strong><small>高风险权限</small></span></div>
          <section><header><div><strong>权限差异</strong><small>相对{state.mode === "create" ? `系统角色 ${state.baseline.name}` : "当前已保存版本"}</small></div></header><div className="role-permission-diff"><div><strong>新增 {addedPermissions.length} 项权限</strong>{addedPermissions.length ? addedPermissions.map((action) => <span key={action}><code>{action}</code><small>{permissionDescriptions[action]}</small></span>) : <small>没有新增权限</small>}</div><div><strong>移除 {removedPermissions.length} 项权限</strong>{removedPermissions.length ? removedPermissions.map((action) => <span key={action}><code>{action}</code><small>{permissionDescriptions[action]}</small></span>) : <small>没有移除权限</small>}</div></div></section>
          {selectedHighRiskPermissions.length > 0 && <div className="role-risk-review"><CircleAlert size={17} /><span><strong>包含 {selectedHighRiskPermissions.length} 项高风险权限</strong><small>{selectedHighRiskPermissions.join(" · ")}</small><small>生产实现应要求重新认证、授权上限检查和不可变审计。</small></span></div>}
          <div className="assignment-boundary"><ShieldCheck size={16} /><span><strong>会话原型变更</strong><small>保存后生成新的授权版本；刷新页面会恢复系统初始数据。</small></span></div>
        </section>}
      </div>
      <footer><button className="secondary-button" type="button" onClick={step === "configure" ? onClose : () => setStep("configure")}>{step === "configure" ? "取消" : "返回修改"}</button><div>{step === "configure" ? <button className="primary-button" type="button" disabled={!valid} onClick={() => setStep("review")}>预览角色变更</button> : <button className="primary-button" type="button" onClick={save}>{state.mode === "create" ? "创建自定义角色" : "保存角色变更"}</button>}</div></footer>
    </section>
  </div>, document.body);
}

export function AccessControlView({ onNotify }: { onNotify: (message: string) => void }) {
  const [activeTab, setActiveTab] = useState<AccessControlTab>("roles");
  const [roles, setRoles] = useState<AuthorizationRole[]>(authorizationRoles);
  const [selectedRoleId, setSelectedRoleId] = useState<string | null>(null);
  const [roleEditor, setRoleEditor] = useState<RoleEditorState | null>(null);
  const [bindings, setBindings] = useState<AuthorizationBinding[]>(authorizationBindings);
  const [assignmentOpen, setAssignmentOpen] = useState(false);
  const [authorizationRevision, setAuthorizationRevision] = useState(1);
  const [assignmentSearch, setAssignmentSearch] = useState("");
  const [inspectPrincipalId, setInspectPrincipalId] = useState("USR-AUDITOR");
  const [inspectAction, setInspectAction] = useState<PermissionAction>("release.publish");
  const [inspectScopeValue, setInspectScopeValue] = useState("asset:METRIC-NET-REVENUE");
  const [decision, setDecision] = useState<AuthorizationDecision | null>(null);
  const selectedRole = useMemo(() => roles.find((role) => role.id === selectedRoleId), [roles, selectedRoleId]);
  const authorizationVersion = `authzv-2026.09.01-${String(authorizationRevision).padStart(3, "0")}`;
  const normalizedAssignmentSearch = assignmentSearch.trim().toLowerCase();
  const visibleBindings = bindings.filter((binding) => {
    const principal = authorizationPrincipals.find((item) => item.id === binding.principalId);
    const role = roles.find((item) => item.id === binding.roleId);
    return !normalizedAssignmentSearch || `${principal?.name} ${principal?.kind} ${role?.name} ${binding.scope.label}`.toLowerCase().includes(normalizedAssignmentSearch);
  });

  const deriveRole = () => {
    if (!selectedRole) return;
    const derivedRole: AuthorizationRole = {
      ...selectedRole,
      id: `ROLE-CUSTOM-${Date.now()}`,
      name: `${selectedRole.name} 自定义`,
      category: "custom",
      baseRoleId: selectedRole.id,
      incompatibleRoleIds: selectedRole.incompatibleRoleIds ? [...selectedRole.incompatibleRoleIds] : undefined,
      permissions: [...selectedRole.permissions],
    };
    setRoleEditor({ mode: "create", role: derivedRole, baseline: selectedRole });
    setSelectedRoleId(null);
  };

  const editRole = () => {
    if (!selectedRole || selectedRole.category !== "custom") return;
    setRoleEditor({ mode: "edit", role: selectedRole, baseline: selectedRole });
    setSelectedRoleId(null);
  };

  const saveRole = (role: AuthorizationRole) => {
    if (!roleEditor) return;
    setRoles((current) => roleEditor.mode === "create" ? [...current, role] : current.map((item) => item.id === role.id ? role : item));
    setAuthorizationRevision((current) => current + 1);
    setRoleEditor(null);
    onNotify(`${role.name}${roleEditor.mode === "create" ? "已创建" : "已更新"}，授权版本已刷新；变更仅保存在当前原型会话。`);
  };

  const confirmAssignment = (binding: AuthorizationBinding) => {
    setBindings((current) => [...current, binding]);
    setAuthorizationRevision((current) => current + 1);
    setAssignmentOpen(false);
    onNotify("角色分配已创建，授权版本已更新；变更仅保存在当前原型会话。");
  };

  const inspectAccess = () => {
    const scopeOption = scopeOptions.find((item) => item.value === inspectScopeValue) ?? scopeOptions[0];
    setDecision(evaluateAuthorization({ principalId: inspectPrincipalId, action: inspectAction, resource: scopeOption.resource, roles, bindings, authorizationVersion }));
  };

  return (
    <section className="view settings-view access-control-view" aria-label="访问控制">
      <header className="access-control-header">
        <div><span className="content-label">工作区授权策略</span><p>以操作权限、主体、角色和资源范围表达访问控制；最终决策由服务端执行。</p></div>
        <span className="authz-version"><ShieldCheck size={14} />{authorizationVersion}</span>
      </header>
      <div className="access-control-tabs" role="tablist" aria-label="访问控制视图">
        {(Object.keys(tabLabels) as AccessControlTab[]).map((tab) => <button type="button" role="tab" aria-selected={activeTab === tab} key={tab} onClick={() => setActiveTab(tab)}>{tabLabels[tab]}</button>)}
      </div>

      {activeTab === "roles" && <div className="access-role-catalog">
        <div className="access-control-metrics" aria-label="角色概览">
          <span><strong>{roles.filter((role) => role.category === "system").length}</strong><small>系统角色</small></span>
          <span><strong>{roles.filter((role) => role.category === "custom").length}</strong><small>自定义角色</small></span>
          <span><strong>{permissionGroups.reduce((total, group) => total + group.actions.length, 0)}</strong><small>稳定操作标识</small></span>
          <span><strong>{bindings.filter((binding) => binding.status === "active").length}</strong><small>有效分配</small></span>
        </div>
        <section className="access-role-table" aria-label="角色目录">
          <div className="access-role-head" aria-hidden="true"><span>角色</span><span>类型</span><span>权限</span><span>有效分配</span><span>职责约束</span><span /></div>
          <div className="access-role-body">{roles.map((role) => {
            const bindingCount = bindings.filter((binding) => binding.roleId === role.id && binding.status === "active").length;
            return <button type="button" className="access-role-row" aria-label={`查看角色 ${role.name}`} key={role.id} onClick={() => setSelectedRoleId(role.id)}>
              <span className="access-role-name"><span><ShieldCheck size={15} /></span><span><strong>{role.name}</strong><small>{role.description}</small></span></span>
              <span><span className={`role-kind role-kind-${role.category}`}>{role.category === "system" ? "系统" : "自定义"}</span></span>
              <strong>{role.permissions.length}</strong><strong>{bindingCount}</strong>
              <span>{role.incompatibleRoleIds?.length ? "受 SoD 约束" : "无冲突角色"}</span><span aria-hidden="true">›</span>
            </button>;
          })}</div>
        </section>
      </div>}

      {activeTab === "assignments" && <div className="assignment-surface">
        <div className="assignment-toolbar"><label><Search size={14} /><input type="search" aria-label="搜索角色分配" placeholder="搜索主体、角色或范围" value={assignmentSearch} onChange={(event) => setAssignmentSearch(event.target.value)} /></label><button className="primary-button" type="button" onClick={() => setAssignmentOpen(true)}><Plus size={15} />分配角色</button></div>
        <section className="assignment-table" aria-label="角色分配列表">
          <div className="assignment-head" aria-hidden="true"><span>主体</span><span>角色</span><span>资源范围</span><span>到期时间</span><span>来源</span><span>状态</span></div>
          <div className="assignment-body">{visibleBindings.map((binding) => {
            const principal = authorizationPrincipals.find((item) => item.id === binding.principalId);
            const role = roles.find((item) => item.id === binding.roleId);
            return <div className="assignment-row" key={binding.id}><span><strong>{principal?.name ?? binding.principalId}</strong><small>{principal ? principalKindLabels[principal.kind] : "未知主体"}</small></span><strong>{role?.name ?? binding.roleId}</strong><span><strong>{binding.scope.label}</strong><small>{binding.scope.type}</small></span><span>{binding.expiresAt ?? "长期有效"}</span><span>{binding.assignedBy}</span><span className={`assignment-state state-${binding.status}`}>{binding.status === "active" ? "有效" : binding.status === "expired" ? "已过期" : "已撤销"}</span></div>;
          })}</div>
        </section>
      </div>}
      {activeTab === "inspect" && <div className="access-inspector">
        <section className="access-inspector-form" aria-label="有效权限检查条件"><label><span>检查主体</span><select aria-label="检查主体" value={inspectPrincipalId} onChange={(event) => { setInspectPrincipalId(event.target.value); setDecision(null); }}>{authorizationPrincipals.map((principal) => <option key={principal.id} value={principal.id}>{principal.name} · {principalKindLabels[principal.kind]}</option>)}</select></label><label><span>操作</span><select aria-label="检查操作" value={inspectAction} onChange={(event) => { setInspectAction(event.target.value as PermissionAction); setDecision(null); }}>{permissionGroups.flatMap((group) => group.actions).map((action) => <option key={action} value={action}>{action}</option>)}</select></label><label><span>资源</span><select aria-label="检查资源" value={inspectScopeValue} onChange={(event) => { setInspectScopeValue(event.target.value); setDecision(null); }}>{scopeOptions.map((option) => <option key={option.value} value={option.value}>{option.scope.label} · {option.scope.type}</option>)}</select></label><button className="primary-button" type="button" onClick={inspectAccess}><LockKeyhole size={15} />检查有效权限</button></section>
        {decision ? <section className={`access-decision decision-${decision.allowed ? "allow" : "deny"}`} aria-label="有效权限结果"><header>{decision.allowed ? <CheckCircle2 size={20} /> : <CircleAlert size={20} />}<span><small>最终决策</small><strong>{decision.allowed ? "允许" : "拒绝"}</strong></span><code>{decision.reasonCode}</code></header><p>{decision.explanation}</p><dl><div><dt>操作</dt><dd><code>{decision.action}</code></dd></div><div><dt>命中角色</dt><dd>{roles.find((role) => role.id === decision.roleId)?.name ?? "无"}</dd></div><div><dt>授权范围</dt><dd>{decision.scope?.label ?? "无匹配范围"}</dd></div><div><dt>授权版本</dt><dd><code>{decision.authorizationVersion}</code></dd></div><div><dt>绑定记录</dt><dd><code>{decision.bindingId ?? "none"}</code></dd></div><div><dt>服务端执行</dt><dd>每次请求重新校验</dd></div></dl></section> : <div className="access-inspector-empty"><LockKeyhole size={22} /><strong>选择条件并运行检查</strong><span>结果会解释角色、绑定、范围、原因码和授权版本。</span></div>}
      </div>}
      <div className="member-directory-disclosure">原型授权数据 · 所有变更仅保存在当前会话</div>
      {selectedRole && <RoleDetailDialog role={selectedRole} bindingCount={bindings.filter((binding) => binding.roleId === selectedRole.id && binding.status === "active").length} onClose={() => setSelectedRoleId(null)} onDerive={deriveRole} onEdit={editRole} />}
      {roleEditor && <RoleEditorDialog state={roleEditor} assignmentCount={bindings.filter((binding) => binding.roleId === roleEditor.role.id && binding.status === "active").length} onClose={() => setRoleEditor(null)} onSave={saveRole} />}
      {assignmentOpen && <AssignmentDialog roles={roles} bindings={bindings} onClose={() => setAssignmentOpen(false)} onConfirm={confirmAssignment} />}
    </section>
  );
}

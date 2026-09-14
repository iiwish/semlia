import { useEffect, useMemo, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { Check, CheckCircle2, CircleAlert, Copy, KeyRound, LoaderCircle, LockKeyhole, Pencil, Plus, RefreshCw, Search, ShieldCheck, Trash2, X } from "lucide-react";

import { findSeparationOfDutyConflicts } from "./authorization";
import { useAuthorizationAdminRuntime } from "./authorizationAdminRuntime";
import type { CreateAuthorizationBindingInput, CreateAuthorizationRoleInput, UpdateAuthorizationRoleInput } from "./authorizationAdmin";
import { permissionDescriptions, permissionGroups } from "./permissionCatalog";
import type { AuthorizationBinding, AuthorizationDecision, AuthorizationRole, AuthorizationScopeType, PermissionAction } from "./types";

type AccessControlTab = "roles" | "assignments" | "inspect";

const tabLabels: Record<AccessControlTab, string> = {
  roles: "角色与权限",
  assignments: "角色分配",
  inspect: "有效权限检查",
};

const scopeTypes: AuthorizationScopeType[] = ["workspace", "domain", "asset", "source", "environment", "release", "consumer"];
const highRiskPermissions: PermissionAction[] = ["workspace.manage", "member.manage", "role.manage", "role.assign", "release.publish", "release.rollback", "source.manage", "binding.manage", "runtime.manage"];

function AssignmentDialog({ workspaceId, roles, bindings, pending, error, onClose, onConfirm }: { workspaceId: string; roles: AuthorizationRole[]; bindings: AuthorizationBinding[]; pending: boolean; error: string; onClose: () => void; onConfirm: (input: CreateAuthorizationBindingInput) => Promise<void> }) {
  const [step, setStep] = useState<"configure" | "review">("configure");
  const [principalId, setPrincipalId] = useState((""));
  const [roleId, setRoleId] = useState((roles[0]?.id ?? ""));
  const [roleVersion, setRoleVersion] = useState(String(roles[0]?.version ?? 1));
  const [scopeType, setScopeType] = useState<AuthorizationScopeType>("workspace");
  const [scopeId, setScopeId] = useState(workspaceId);
  const [expiresAt, setExpiresAt] = useState((""));
  const role = roles.find((item) => item.id === roleId);
  const scope = ({ type: scopeType, id: scopeId.trim(), label: scopeId.trim() });
  const conflicts = role ? findSeparationOfDutyConflicts({ principalId, roleId, scope, roles, bindings }) : [];
  const expectedRoleVersion = role?.version ?? Number(roleVersion);
  const valid = Boolean(principalId.trim() && roleId.trim() && Number.isInteger(expectedRoleVersion) && expectedRoleVersion > 0 && scope.id.trim());

  const confirm = () => {
    if (!valid) return;
    return onConfirm({
      principalId: principalId.trim(),
      roleId: roleId.trim(),
      expectedRoleVersion,
      scope,
      expiresAt: expiresAt ? `${expiresAt}T23:59:59Z` : undefined,
    });
  };

  return createPortal(<div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !pending) onClose(); }}>
    <section className="review-dialog assignment-dialog" role="dialog" aria-modal="true" aria-label="分配角色">
      <header><div><span className="panel-kicker">{step === "configure" ? "步骤 1 / 2" : "步骤 2 / 2"}</span><h2>分配角色</h2></div><button className="icon-button" type="button" aria-label="关闭角色分配" disabled={pending} onClick={onClose}><X size={18} /></button></header>
      <div className="dialog-body assignment-dialog-body">
        {step === "configure" ? <div className="assignment-form">
          {(<label><span>主体 ID</span><input aria-label="主体 ID" placeholder="prn_..." value={principalId} onChange={(event) => setPrincipalId(event.target.value)} /></label>)}
          {roles.length > 0 ? <label><span>角色</span><select aria-label="角色" value={roleId} onChange={(event) => { const nextRole = roles.find((item) => item.id === event.target.value); setRoleId(event.target.value); setRoleVersion(String(nextRole?.version ?? 1)); }}>{roles.map((item) => <option key={item.id} value={item.id}>{item.name} · v{item.version ?? 1}</option>)}</select></label> : <><label><span>角色 ID</span><input aria-label="角色 ID" placeholder="rol_..." value={roleId} onChange={(event) => setRoleId(event.target.value)} /></label><label><span>角色版本</span><input aria-label="角色版本" type="number" min={1} step={1} value={roleVersion} onChange={(event) => setRoleVersion(event.target.value)} /></label></>}
          {(<><label><span>范围类型</span><select aria-label="范围类型" value={scopeType} onChange={(event) => { const next = event.target.value as AuthorizationScopeType; setScopeType(next); setScopeId(next === "workspace" ? workspaceId : ""); }}>{scopeTypes.map((item) => <option key={item} value={item}>{item}</option>)}</select></label><label><span>范围 ID</span><input aria-label="范围 ID" value={scopeId} readOnly={scopeType === "workspace"} onChange={(event) => setScopeId(event.target.value)} /></label></>)}
          <label><span>到期时间</span><input aria-label="到期时间" type="date" value={expiresAt} onChange={(event) => setExpiresAt(event.target.value)} /></label>
          <div className="assignment-boundary"><ShieldCheck size={16} /><span><strong>服务端强制执行</strong><small>职责分离、授权上限、主体状态和角色版本由服务端在提交时复核。</small></span></div>
        </div> : <section className="assignment-preview" aria-label="授权变更预览">
          <div className="assignment-preview-path"><span><strong>{principalId}</strong><small>主体 TypeID</small></span><span>获得</span><span><strong>{role?.name ?? roleId}</strong><small>{role ? (role.category === "system" ? "系统角色" : "自定义角色") : "角色 TypeID"} · v{expectedRoleVersion}</small></span><span>作用于</span><span><strong>{scope.label}</strong><small>{scope.type} · {expiresAt || "长期有效"}</small></span></div>
          <div className="assignment-capability-diff"><CheckCircle2 size={17} /><span><strong>{("申请")} {role?.permissions.length ?? 0} 项操作权限</strong><small>{role?.permissions.slice(0, 5).join(" · ")}{(role?.permissions.length ?? 0) > 5 ? " …" : ""}</small></span></div>
          {conflicts.length > 0 ? <div className="assignment-conflict" role="alert"><CircleAlert size={18} /><span><strong>职责分离冲突</strong><small>{conflicts[0].message}</small><small>服务端仍会执行最终约束检查。</small></span></div> : <div className="assignment-clear"><ShieldCheck size={17} /><span><strong>等待服务端策略复核</strong><small>确认后只有服务端接受并重新读取成功，分配才会显示为有效。</small></span></div>}
          {error && <div className="access-command-error" role="alert"><CircleAlert size={16} /><span><strong>角色分配未创建</strong>{error}</span></div>}
        </section>}
      </div>
      <footer><button className="secondary-button" type="button" disabled={pending} onClick={step === "configure" ? onClose : () => setStep("configure")}>{step === "configure" ? "取消" : "返回修改"}</button><div>{step === "configure" ? <button className="primary-button" type="button" disabled={!valid} onClick={() => setStep("review")}>预览授权</button> : <button className="primary-button" type="button" disabled={pending || conflicts.some((conflict) => conflict.blocking)} onClick={() => void confirm()}>{pending ? "正在分配" : "确认分配"}</button>}</div></footer>
    </section>
  </div>, document.body);
}

function RoleDetailDialog({ role, roles, bindingCount, canManage, onClose, onDerive, onEdit }: { role: AuthorizationRole; roles: AuthorizationRole[]; bindingCount: number | null; canManage: boolean; onClose: () => void; onDerive: () => void; onEdit: () => void }) {
  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape") onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose]);

  return createPortal(
    <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section className="review-dialog access-role-dialog" role="dialog" aria-modal="true" aria-label={`${role.name} 角色详情`}>
        <header><div><span className="panel-kicker">{role.category === "system" ? "系统角色" : `自定义角色 · v${role.version ?? 1}`}</span><h2>{role.name}</h2></div><button className="icon-button" type="button" aria-label="关闭角色详情" onClick={onClose}><X size={18} /></button></header>
        <div className="dialog-body access-role-dialog-body">
          <p>{role.description}</p>
          <div className="access-role-summary"><span><KeyRound size={15} /><strong>{role.permissions.length}</strong> 项操作</span><span><LockKeyhole size={15} /><strong>{bindingCount ?? "不可见"}</strong>{bindingCount === null ? "角色分配" : "个有效分配"}</span>{role.baseRoleId && <span><Copy size={15} />派生自 <strong>{roles.find((item) => item.id === role.baseRoleId)?.name ?? role.baseRoleId}</strong></span>}</div>
          <div className="permission-matrix" aria-label={`${role.name} 权限矩阵`}>
            {permissionGroups.filter((group) => group.actions.some((action) => role.permissions.includes(action))).map((group) => <section key={group.id}>
              <header><div><strong>{group.label}</strong><small>{group.description}</small></div><span>{group.actions.filter((action) => role.permissions.includes(action)).length} / {group.actions.length}</span></header>
              <div>{group.actions.map((action) => { const granted = role.permissions.includes(action); return <span className={granted ? "permission-point is-granted" : "permission-point is-not-granted"} data-granted={String(granted)} key={action}>{granted ? <Check size={12} /> : <X size={12} />}<span><code data-granted={String(granted)}>{action}</code><small>{permissionDescriptions[action]}</small></span></span>; })}</div>
            </section>)}
          </div>
        </div>
        <footer><span className="dialog-footnote">{role.category === "system" ? "系统角色由 Semlia 版本管理，不可直接修改" : "保存会追加并激活新的不可变角色版本"}</span><div><button className="secondary-button" type="button" onClick={onClose}>关闭</button>{canManage && (role.category === "system" ? <button className="primary-button" type="button" onClick={onDerive}><Copy size={15} />基于此角色创建</button> : <button className="primary-button" type="button" onClick={onEdit}><Pencil size={15} />编辑角色</button>)}</div></footer>
      </section>
    </div>, document.body
  );
}

type RoleEditorState = { mode: "create" | "edit"; role: AuthorizationRole; baseline: AuthorizationRole };

function RoleEditorDialog({ state, assignmentCount, pending, error, onClose, onSave }: { state: RoleEditorState; assignmentCount: number | null; pending: boolean; error: string; onClose: () => void; onSave: (input: CreateAuthorizationRoleInput | UpdateAuthorizationRoleInput) => Promise<void> }) {
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
    const handleKeyDown = (event: KeyboardEvent) => { if (event.key === "Escape" && !pending) onClose(); };
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [onClose, pending]);

  const togglePermission = (action: PermissionAction) => setPermissions((current) => current.includes(action) ? current.filter((item) => item !== action) : [...current, action]);
  const save = () => onSave({ name: name.trim(), description: description.trim(), permissions, ...(state.mode === "edit" ? { expectedVersion: state.role.version ?? 1 } : {}) });

  return createPortal(<div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !pending) onClose(); }}>
    <section className="review-dialog role-editor-dialog" role="dialog" aria-modal="true" aria-label={dialogLabel}>
      <header><div><span className="panel-kicker">{step === "configure" ? "步骤 1 / 2 · 配置" : "步骤 2 / 2 · 复核"}</span><h2>{state.mode === "create" ? "创建自定义角色" : "编辑自定义角色"}</h2></div><button className="icon-button" type="button" aria-label="关闭角色编辑" disabled={pending} onClick={onClose}><X size={18} /></button></header>
      <div className="dialog-body role-editor-body">
        {step === "configure" ? <><div className="role-editor-form"><label><span>角色名称</span><input aria-label="角色名称" value={name} onChange={(event) => setName(event.target.value)} /></label><label><span>角色说明</span><textarea aria-label="角色说明" rows={2} value={description} onChange={(event) => setDescription(event.target.value)} /></label></div><section className="permission-editor" aria-label="权限点配置"><header><div><strong>权限点配置</strong><small>使用稳定操作标识；服务端会校验当前操作者的授权上限。</small></div><span>{permissions.length} / {permissionGroups.reduce((total, group) => total + group.actions.length, 0)} 已选择</span></header><div>{permissionGroups.map((group) => <section key={group.id}><header><div><strong>{group.label}</strong><small>{group.description}</small></div><span>{group.actions.filter((action) => permissionSet.has(action)).length} / {group.actions.length}</span></header><div>{group.actions.map((action) => <label className="permission-option" key={action}><input type="checkbox" aria-label={`配置权限 ${action}`} checked={permissionSet.has(action)} onChange={() => togglePermission(action)} /><span><code>{action}</code><small>{permissionDescriptions[action]}</small></span></label>)}</div></section>)}</div></section></> : <section className="role-change-preview" aria-label="角色变更预览">
          <div className="role-change-summary"><span><strong>{addedPermissions.length}</strong><small>新增权限</small></span><span><strong>{removedPermissions.length}</strong><small>移除权限</small></span><span><strong>{assignmentCount ?? "不可见"}</strong><small>受影响分配</small></span><span><strong>{selectedHighRiskPermissions.length}</strong><small>高风险权限</small></span></div>
          <section><header><div><strong>权限差异</strong><small>相对{state.mode === "create" ? `角色 ${state.baseline.name}` : `当前版本 v${state.role.version ?? 1}`}</small></div></header><div className="role-permission-diff"><div><strong>新增 {addedPermissions.length} 项权限</strong>{addedPermissions.length ? addedPermissions.map((action) => <span key={action}><code>{action}</code><small>{permissionDescriptions[action]}</small></span>) : <small>没有新增权限</small>}</div><div><strong>移除 {removedPermissions.length} 项权限</strong>{removedPermissions.length ? removedPermissions.map((action) => <span key={action}><code>{action}</code><small>{permissionDescriptions[action]}</small></span>) : <small>没有移除权限</small>}</div></div></section>
          {selectedHighRiskPermissions.length > 0 && <div className="role-risk-review"><CircleAlert size={17} /><span><strong>包含 {selectedHighRiskPermissions.length} 项高风险权限</strong><small>{selectedHighRiskPermissions.join(" · ")}</small><small>服务端将执行授权上限与不可变审计检查。</small></span></div>}
          <div className="assignment-boundary"><ShieldCheck size={16} /><span><strong>版本化服务端变更</strong><small>{state.mode === "create" ? "创建首个角色版本。" : `以 v${state.role.version ?? 1} 为预期版本追加新版本。`}确认后重新读取角色目录。</small></span></div>
          {error && <div className="access-command-error" role="alert"><CircleAlert size={16} /><span><strong>角色变更未保存</strong>{error}</span></div>}
        </section>}
      </div>
      <footer><button className="secondary-button" type="button" disabled={pending} onClick={step === "configure" ? onClose : () => setStep("configure")}>{step === "configure" ? "取消" : "返回修改"}</button><div>{step === "configure" ? <button className="primary-button" type="button" disabled={!valid} onClick={() => setStep("review")}>预览角色变更</button> : <button className="primary-button" type="button" disabled={pending} onClick={() => void save()}>{pending ? "正在保存" : state.mode === "create" ? "创建自定义角色" : "保存角色变更"}</button>}</div></footer>
    </section>
  </div>, document.body);
}

function RevokeBindingDialog({ binding, pending, error, onClose, onConfirm }: { binding: AuthorizationBinding; pending: boolean; error: string; onClose: () => void; onConfirm: (reason: string) => Promise<void> }) {
  const [reason, setReason] = useState("");
  return createPortal(<div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget && !pending) onClose(); }}><section className="review-dialog revoke-binding-dialog" role="dialog" aria-modal="true" aria-label="撤销角色分配"><header><div><span className="panel-kicker">不可逆授权变更</span><h2>撤销角色分配</h2></div><button className="icon-button" type="button" aria-label="关闭撤销确认" disabled={pending} onClick={onClose}><X size={18} /></button></header><div className="dialog-body"><p><code>{binding.id}</code> 将立即失效，服务端会提升授权版本并保留审计记录。</p><label className="revoke-reason"><span>撤销原因</span><textarea aria-label="撤销原因" rows={3} value={reason} onChange={(event) => setReason(event.target.value)} /></label>{error && <div className="access-command-error" role="alert"><CircleAlert size={16} /><span><strong>角色分配未撤销</strong>{error}</span></div>}</div><footer><button className="secondary-button" type="button" disabled={pending} onClick={onClose}>取消</button><button className="danger-button" type="button" disabled={pending || !reason.trim()} onClick={() => void onConfirm(reason.trim())}><Trash2 size={15} />{pending ? "正在撤销" : "确认撤销"}</button></footer></section></div>, document.body);
}

function AccessSurfaceBoundary({ state, error, errorCode, loadingTitle, loadingDetail, forbiddenTitle, errorTitle, onRetry, children }: { state: "idle" | "loading" | "ready" | "empty" | "error" | "forbidden"; error: string; errorCode: string; loadingTitle: string; loadingDetail: string; forbiddenTitle: string; errorTitle: string; onRetry: () => Promise<void>; children: ReactNode }) {
  if (state === "loading") return <div className="access-control-placeholder" role="status"><LoaderCircle className="is-spinning" size={22} /><strong>{loadingTitle}</strong><span>{loadingDetail}</span></div>;
  if (state === "error" || state === "forbidden") return <div className="access-control-load-error" role="alert"><CircleAlert size={20} /><span><strong>{state === "forbidden" ? forbiddenTitle : errorTitle}</strong>{error}{errorCode && <code>{errorCode}</code>}</span><button className="secondary-button" type="button" onClick={() => void onRetry()}><RefreshCw size={15} />重试加载访问控制</button></div>;
  return children;
}

export function AccessControlView({ onNotify }: { onNotify: (message: string) => void }) {
  const runtime = useAuthorizationAdminRuntime();
  const canReadRoles = runtime.access.roleRead;
  const canManageRoles = runtime.access.roleManage;
  const canAssignRoles = runtime.access.roleAssign;
  const canInspectAuthorization = runtime.access.authorizationInspect;
  const availableTabs = useMemo<AccessControlTab[]>(() => [canReadRoles || canManageRoles ? "roles" : null, canAssignRoles ? "assignments" : null, canInspectAuthorization ? "inspect" : null].filter((tab): tab is AccessControlTab => tab !== null), [canAssignRoles, canInspectAuthorization, canManageRoles, canReadRoles]);
  const [requestedTab, setActiveTab] = useState<AccessControlTab>(() => availableTabs[0] ?? "roles");
  const activeTab = availableTabs.includes(requestedTab) ? requestedTab : availableTabs[0] ?? "roles";
  const [selectedRoleId, setSelectedRoleId] = useState<string | null>(null);
  const [roleEditor, setRoleEditor] = useState<RoleEditorState | null>(null);
  const [assignmentOpen, setAssignmentOpen] = useState(false);
  const [revokeBinding, setRevokeBinding] = useState<AuthorizationBinding | null>(null);
  const [commandError, setCommandError] = useState("");
  const [assignmentSearch, setAssignmentSearch] = useState("");
  const [inspectPrincipalId, setInspectPrincipalId] = useState((""));
  const [inspectAction, setInspectAction] = useState<PermissionAction>("release.publish");
  const [inspectScopeType, setInspectScopeType] = useState<AuthorizationScopeType>("workspace");
  const [inspectScopeId, setInspectScopeId] = useState(runtime.workspaceId);
  const [inspectDomainId, setInspectDomainId] = useState("");
  const [decision, setDecision] = useState<AuthorizationDecision | null>(null);
  const roles = runtime.roles;
  const bindings = runtime.bindings;
  const canSeeBindingCounts = canAssignRoles && (runtime.bindingsState === "ready" || runtime.bindingsState === "empty");
  const pending = Boolean(runtime.pendingCommand);
  const selectedRole = useMemo(() => roles.find((role) => role.id === selectedRoleId), [roles, selectedRoleId]);
  const normalizedAssignmentSearch = assignmentSearch.trim().toLowerCase();
  const visibleBindings = bindings.filter((binding) => !normalizedAssignmentSearch || `${binding.principalId} ${binding.principalId} ${roles.find((role) => role.id === binding.roleId)?.name ?? binding.roleId} ${binding.scope.label}`.toLowerCase().includes(normalizedAssignmentSearch));
  const versionLabel = (runtime.authorizationVersion || "等待服务端版本");

  const deriveRole = () => {
    if (!selectedRole || !canManageRoles) return;
    setRoleEditor({ mode: "create", role: { ...selectedRole, id: "", name: `${selectedRole.name} 自定义`, category: "custom", baseRoleId: selectedRole.id, version: 1, permissions: [...selectedRole.permissions] }, baseline: selectedRole });
    setSelectedRoleId(null);
    setCommandError("");
  };
  const editRole = () => {
    if (!selectedRole || selectedRole.category !== "custom" || !canManageRoles) return;
    setRoleEditor({ mode: "edit", role: selectedRole, baseline: selectedRole });
    setSelectedRoleId(null);
    setCommandError("");
  };
  const saveRole = async (input: CreateAuthorizationRoleInput | UpdateAuthorizationRoleInput) => {
    if (!roleEditor) return;
    setCommandError("");
    try {
      const saved = roleEditor.mode === "create" ? await runtime.createRole(input) : await runtime.updateRole(roleEditor.role.id, input as UpdateAuthorizationRoleInput);
      setRoleEditor(null);
      onNotify(`${saved.name}${roleEditor.mode === "create" ? "已创建" : "已更新"}，授权版本已刷新。`);
    } catch (reason) {
      setCommandError(errorMessage(reason));
    }
  };
  const confirmAssignment = async (input: CreateAuthorizationBindingInput) => {
    setCommandError("");
    try {
      await runtime.createBinding(input);
      setAssignmentOpen(false);
      onNotify("角色分配已创建，授权版本已更新。");
    } catch (reason) {
      setCommandError(errorMessage(reason));
    }
  };
  const confirmRevocation = async (reason: string) => {
    if (!revokeBinding) return;
    setCommandError("");
    try {
      await runtime.revokeBinding(revokeBinding.id, { expectedVersion: revokeBinding.version ?? 1, reason });
      setRevokeBinding(null);
      onNotify("角色分配已撤销，授权版本已更新。");
    } catch (failure) {
      setCommandError(errorMessage(failure));
    }
  };
  const inspectAccess = async () => {
    const scope = ({ type: inspectScopeType, id: inspectScopeId.trim(), label: inspectScopeId.trim(), ...(inspectScopeType === "asset" && inspectDomainId.trim() ? { domainId: inspectDomainId.trim() } : {}) });
    setDecision(null);
    setCommandError("");
    try {
      setDecision(await runtime.inspect({ principalId: inspectPrincipalId.trim(), action: inspectAction, resource: scope }));
    } catch (reason) {
      setCommandError(errorMessage(reason));
    }
  };

  return <section className="view settings-view access-control-view" aria-label="访问控制">
    <header className="access-control-header"><div><span className="content-label">工作区授权策略</span><p>角色、分配和最终权限检查均来自当前工作区的服务端授权版本。</p></div><span className="authz-version"><ShieldCheck size={14} />{versionLabel}</span></header>
    <div className="access-control-tabs" role="tablist" aria-label="访问控制视图">
      {availableTabs.map((tab) => <button type="button" role="tab" aria-selected={activeTab === tab} key={tab} onClick={() => { setActiveTab(tab); setCommandError(""); }}>{tabLabels[tab]}</button>)}
    </div>

    {activeTab === "roles" && !canReadRoles ? <div className="access-inspector-empty" role="note"><LockKeyhole size={22} /><strong>角色管理上下文不可用</strong><span>当前会话具有 role.manage，但缺少 role.read。为避免猜测角色 ID、版本或权限，界面不会加载或伪造角色；获得角色读取能力后可创建或编辑角色。</span></div> : activeTab === "roles" && <AccessSurfaceBoundary state={runtime.rolesState} error={runtime.rolesError} errorCode={runtime.rolesErrorCode} loadingTitle="正在读取角色" loadingDetail="加载系统角色、自定义角色和不可变版本。" forbiddenTitle="无权读取角色" errorTitle="角色目录加载失败" onRetry={runtime.refreshRoles}><div className="access-role-catalog">
      <div className="access-control-metrics" aria-label="角色概览"><span><strong>{roles.filter((role) => role.category === "system").length}</strong><small>系统角色</small></span><span><strong>{roles.filter((role) => role.category === "custom").length}</strong><small>自定义角色</small></span><span><strong>{permissionGroups.reduce((total, group) => total + group.actions.length, 0)}</strong><small>稳定操作标识</small></span><span><strong>{canSeeBindingCounts ? bindings.filter((binding) => binding.status === "active").length : "不可见"}</strong><small>有效分配</small></span></div>
      <section className="access-role-table" aria-label="角色目录"><div className="access-role-head" aria-hidden="true"><span>角色</span><span>类型</span><span>权限</span><span>有效分配</span><span>版本</span><span /></div><div className="access-role-body">{roles.map((role) => { const bindingCount = canSeeBindingCounts ? bindings.filter((binding) => binding.roleId === role.id && binding.status === "active").length : null; return <button type="button" className="access-role-row" aria-label={`查看角色 ${role.name}`} key={role.id} onClick={() => setSelectedRoleId(role.id)}><span className="access-role-name"><span><ShieldCheck size={15} /></span><span><strong>{role.name}</strong><small>{role.description}</small></span></span><span><span className={`role-kind role-kind-${role.category}`}>{role.category === "system" ? "系统" : "自定义"}</span></span><strong>{role.permissions.length}</strong><strong>{bindingCount ?? "-"}</strong><span>v{role.version ?? 1}</span><span aria-hidden="true">›</span></button>; })}{roles.length === 0 && <div className="access-inline-empty"><ShieldCheck size={20} /><strong>尚无可读取角色</strong><span>当前工作区没有返回系统角色或自定义角色。</span></div>}</div></section>
    </div></AccessSurfaceBoundary>}

    {activeTab === "assignments" && <AccessSurfaceBoundary state={runtime.bindingsState} error={runtime.bindingsError} errorCode={runtime.bindingsErrorCode} loadingTitle="正在读取角色分配" loadingDetail="加载当前工作区的有效、过期和已撤销分配。" forbiddenTitle="无权读取角色分配" errorTitle="角色分配加载失败" onRetry={runtime.refreshBindings}><div className="assignment-surface"><div className="assignment-toolbar"><label><Search size={14} /><input type="search" aria-label="搜索角色分配" placeholder="搜索主体、角色或范围" value={assignmentSearch} onChange={(event) => setAssignmentSearch(event.target.value)} /></label><button className="primary-button" type="button" disabled={!canAssignRoles} onClick={() => { setCommandError(""); setAssignmentOpen(true); }}><Plus size={15} />分配角色</button></div><section className="assignment-table" aria-label="角色分配列表"><div className="assignment-head" aria-hidden="true"><span>主体</span><span>角色</span><span>资源范围</span><span>到期时间</span><span>来源</span><span>状态 / 操作</span></div><div className="assignment-body">{visibleBindings.map((binding) => { const role = roles.find((item) => item.id === binding.roleId); return <div className="assignment-row" role="row" aria-label={`${binding.principalId} ${role?.name ?? binding.roleId} ${binding.scope.label}`} key={binding.id}><span><strong>{binding.principalId}</strong><small>{binding.principalId}</small></span><strong>{role?.name ?? binding.roleId}</strong><span><strong>{binding.scope.label}</strong><small>{binding.scope.type}</small></span><span>{binding.expiresAt ? formatTime(binding.expiresAt) : "长期有效"}</span><span>{binding.assignedBy}</span><span className="assignment-row-action"><span className={`assignment-state state-${binding.status}`}>{binding.status === "active" ? "有效" : binding.status === "expired" ? "已过期" : "已撤销"}</span>{binding.status === "active" && canAssignRoles && <button className="icon-button" type="button" aria-label="撤销分配" title="撤销分配" onClick={() => { setCommandError(""); setRevokeBinding(binding); }}><Trash2 size={13} /></button>}</span></div>; })}{visibleBindings.length === 0 && <div className="access-inline-empty"><LockKeyhole size={20} /><strong>{bindings.length ? "没有匹配分配" : "尚无角色分配"}</strong><span>{bindings.length ? "调整搜索条件。" : "由有权限的管理员创建首个范围化分配。"}</span></div>}</div></section></div></AccessSurfaceBoundary>}

    {activeTab === "inspect" && <div className="access-inspector">
        <section className="access-inspector-form" aria-label="有效权限检查条件">
          {(<label><span>检查主体 ID</span><input aria-label="检查主体 ID" placeholder="prn_..." value={inspectPrincipalId} onChange={(event) => { setInspectPrincipalId(event.target.value); setDecision(null); }} /></label>)}
          <label><span>操作</span><select aria-label="检查操作" value={inspectAction} onChange={(event) => { setInspectAction(event.target.value as PermissionAction); setDecision(null); }}>{permissionGroups.flatMap((group) => group.actions).map((action) => <option key={action} value={action}>{action}</option>)}</select></label>
          {(<><label><span>资源类型</span><select aria-label="资源类型" value={inspectScopeType} onChange={(event) => { const next = event.target.value as AuthorizationScopeType; setInspectScopeType(next); setInspectScopeId(next === "workspace" ? runtime.workspaceId : ""); setInspectDomainId(""); setDecision(null); }}>{scopeTypes.map((item) => <option key={item} value={item}>{item}</option>)}</select></label><label><span>资源 ID</span><input aria-label="资源 ID" readOnly={inspectScopeType === "workspace"} value={inspectScopeId} onChange={(event) => { setInspectScopeId(event.target.value); setDecision(null); }} /></label>{inspectScopeType === "asset" && <label><span>所属域 ID</span><input aria-label="所属域 ID" value={inspectDomainId} onChange={(event) => { setInspectDomainId(event.target.value); setDecision(null); }} /></label>}</>)}
          <button className="primary-button" type="button" disabled={!canInspectAuthorization || pending || !inspectPrincipalId.trim() || !((inspectScopeId.trim()))} onClick={() => void inspectAccess()}><LockKeyhole size={15} />{pending ? "正在检查" : "检查有效权限"}</button>
        </section>
        {commandError && <div className="access-command-error" role="alert"><CircleAlert size={16} /><span><strong>{runtime.errorCode === "FORBIDDEN" ? "无权检查有效权限" : "有效权限检查失败"}</strong>{commandError}</span></div>}
        {decision ? <section className={`access-decision decision-${decision.allowed ? "allow" : "deny"}`} aria-label="有效权限结果"><header>{decision.allowed ? <CheckCircle2 size={20} /> : <CircleAlert size={20} />}<span><small>服务端最终决策</small><strong>{decision.allowed ? "允许" : "拒绝"}</strong></span><code>{decision.reasonCode}</code></header><p>{decision.explanation}</p><dl><div><dt>操作</dt><dd><code>{decision.action}</code></dd></div><div><dt>命中角色</dt><dd>{roles.find((role) => role.id === decision.roleId)?.name ?? decision.roleId ?? "无"}</dd></div><div><dt>授权范围</dt><dd>{decision.scope?.label ?? "无匹配范围"}</dd></div><div><dt>授权版本</dt><dd><code>{decision.authorizationVersion}</code></dd></div><div><dt>绑定记录</dt><dd><code>{decision.bindingId ?? "none"}</code></dd></div><div><dt>执行位置</dt><dd>服务端 Authorizer</dd></div></dl></section> : !commandError && <div className="access-inspector-empty"><LockKeyhole size={22} /><strong>选择条件并运行检查</strong><span>结果由服务端解释角色、绑定、范围、原因码和授权版本。</span></div>}
    </div>}


    {selectedRole && <RoleDetailDialog role={selectedRole} roles={roles} bindingCount={canSeeBindingCounts ? bindings.filter((binding) => binding.roleId === selectedRole.id && binding.status === "active").length : null} canManage={canManageRoles} onClose={() => setSelectedRoleId(null)} onDerive={deriveRole} onEdit={editRole} />}
    {roleEditor && <RoleEditorDialog state={roleEditor} assignmentCount={canSeeBindingCounts ? bindings.filter((binding) => binding.roleId === roleEditor.role.id && binding.status === "active").length : null} pending={runtime.pendingCommand === "create-role" || runtime.pendingCommand === "update-role"} error={commandError} onClose={() => setRoleEditor(null)} onSave={saveRole} />}
    {assignmentOpen && <AssignmentDialog workspaceId={runtime.workspaceId}  roles={roles} bindings={bindings} pending={runtime.pendingCommand === "create-binding"} error={commandError} onClose={() => setAssignmentOpen(false)} onConfirm={confirmAssignment} />}
    {revokeBinding && <RevokeBindingDialog binding={revokeBinding} pending={runtime.pendingCommand === "revoke-binding"} error={commandError} onClose={() => setRevokeBinding(null)} onConfirm={confirmRevocation} />}
  </section>;
}



function errorMessage(reason: unknown): string {
  return reason instanceof Error ? reason.message : "访问控制请求失败。";
}

function formatTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).format(date);
}

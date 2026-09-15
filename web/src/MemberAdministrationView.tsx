import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { AlertTriangle, Check, Clock3, LoaderCircle, MailPlus, PauseCircle, RefreshCw, RotateCcw, Search, Shield, UserMinus, Users, X } from "lucide-react";

import { useCan } from "./authorization";
import {
  createWorkspaceInvitation,
  createPasswordMember,
  getAuthMethods,
  IdentityApiError,
  listWorkspaceInvitations,
  listWorkspaceMembers,
  updateWorkspaceMembership,
  type MembershipStatus,
  type WorkspaceInvitation,
  type WorkspaceMembership,
} from "./identity";
import { useSessionRuntime } from "./sessionRuntime";

type DirectoryTab = "members" | "invitations";

const roleOptions = [
  ["workspace_admin", "Workspace Admin"],
  ["security_admin", "Security Admin"],
  ["semantic_steward", "Semantic Steward"],
  ["asset_owner", "Asset Owner"],
  ["reviewer", "Reviewer"],
  ["publisher", "Publisher"],
  ["source_operator", "Source Operator"],
  ["consumer_developer", "Consumer Developer"],
  ["auditor", "Auditor"],
] as const;

export function MemberAdministrationView() {
  const runtime = useSessionRuntime();
  const canRead = useCan("member.read");
  const canManage = useCan("member.manage");
  const [tab, setTab] = useState<DirectoryTab>("members");
  const [members, setMembers] = useState<WorkspaceMembership[]>([]);
  const [invitations, setInvitations] = useState<WorkspaceInvitation[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [inviteOpen, setInviteOpen] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const [oidc, setOIDC] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void getAuthMethods(controller.signal).then((methods) => setOIDC(methods.oidc)).catch(() => undefined);
    return () => controller.abort();
  }, []);
  const [pendingCommand, setPendingCommand] = useState<{ member: WorkspaceMembership; status: MembershipStatus } | null>(null);

  const load = async (signal?: AbortSignal) => {
    if (!runtime.activeWorkspaceId || !canRead) return;
    setLoading(true);
    try {
      const [nextMembers, nextInvitations] = await Promise.all([
        listWorkspaceMembers(runtime.activeWorkspaceId, signal),
        listWorkspaceInvitations(runtime.activeWorkspaceId, signal),
      ]);
      setMembers(nextMembers);
      setInvitations(nextInvitations);
      setError("");
    } catch (reason) {
      if (signal?.aborted) return;
      setError(identityErrorMessage(reason, "无法读取成员与邀请。"));
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    const timer = window.setTimeout(() => void load(controller.signal), 0);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  // `load` intentionally follows the active workspace and effective read capability only.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canRead, runtime.activeWorkspaceId]);

  const normalizedQuery = query.trim().toLowerCase();
  const visibleMembers = useMemo(() => members.filter((member) => !normalizedQuery || `${member.id} ${member.displayName} ${member.roleIds.join(" ")} ${member.status}`.toLowerCase().includes(normalizedQuery)), [members, normalizedQuery]);
  const visibleInvitations = useMemo(() => invitations.filter((invitation) => !normalizedQuery || `${invitation.id} ${invitation.email ?? ""} ${invitation.subject ?? ""} ${invitation.roleId} ${invitation.status}`.toLowerCase().includes(normalizedQuery)), [invitations, normalizedQuery]);

  if (!canRead) {
    return <section className="view settings-view member-administration-view" aria-label="成员管理"><div className="member-admin-state is-denied"><Shield size={21} /><strong>没有成员读取权限</strong><span>当前工作区角色不能查看成员和邀请。管理员更新授权后，本页会重新校验会话。</span></div></section>;
  }

  const confirmCommand = async () => {
    if (!pendingCommand) return;
    try {
      const updated = await updateWorkspaceMembership(runtime.activeWorkspaceId, pendingCommand.member.id, pendingCommand.status);
      setMembers((current) => current.map((member) => member.id === updated.id ? updated : member));
      setPendingCommand(null);
      setError("");
    } catch (reason) {
      setError(identityErrorMessage(reason, "无法更新成员状态。"));
      setPendingCommand(null);
    }
  };

  return <section className="view settings-view member-administration-view" aria-label="成员管理">
    <div className="member-admin-toolbar">
      <div className="segment-control" aria-label="成员管理视图">
        <button type="button" aria-pressed={tab === "members"} onClick={() => setTab("members")}>成员 <span>{members.length}</span></button>
        {oidc && <button type="button" aria-pressed={tab === "invitations"} onClick={() => setTab("invitations")}>邀请 <span>{invitations.filter((item) => item.status === "pending").length}</span></button>}
      </div>
      <label className="member-admin-search"><Search size={14} /><input type="search" aria-label="搜索成员与邀请" placeholder="搜索姓名、角色或邀请邮箱" value={query} onChange={(event) => setQuery(event.target.value)} />{query && <button type="button" aria-label="清除搜索" onClick={() => setQuery("")}><X size={13} /></button>}</label>
      <button className="icon-button" type="button" aria-label="刷新成员与邀请" title="刷新成员与邀请" onClick={() => void load()} disabled={loading}><RefreshCw className={loading ? "is-spinning" : ""} size={15} /></button>
      {canManage && oidc && <button className="secondary-button" type="button" onClick={() => setInviteOpen(true)}><MailPlus size={15} />邀请成员</button>}
      {canManage && <button className="primary-button" type="button" onClick={() => setAccountOpen(true)}><Users size={15} />创建本地账号</button>}
    </div>
    {error && <div className="member-admin-alert" role="alert"><AlertTriangle size={15} /><span>{error}</span></div>}
    {loading && members.length === 0 && invitations.length === 0 ? <div className="member-admin-state"><LoaderCircle className="is-spinning" size={20} /><strong>正在读取工作区成员</strong></div> : tab === "members" ? <MemberTable members={visibleMembers} canManage={canManage} currentAccountId={runtime.session?.account.id ?? ""} onCommand={(member, status) => status === "active" ? void updateWorkspaceMembership(runtime.activeWorkspaceId, member.id, status).then((updated) => setMembers((current) => current.map((item) => item.id === updated.id ? updated : item))).catch((reason) => setError(identityErrorMessage(reason, "无法恢复成员。"))) : setPendingCommand({ member, status })} /> : <InvitationTable invitations={visibleInvitations} />}
    {inviteOpen && <InvitationDialog workspaceId={runtime.activeWorkspaceId} onClose={() => setInviteOpen(false)} onCreated={(invitation) => { setInvitations((current) => [invitation, ...current]); setTab("invitations"); setInviteOpen(false); }} />}
    {accountOpen && <LocalAccountDialog workspaceId={runtime.activeWorkspaceId} onClose={() => setAccountOpen(false)} onCreated={() => { setAccountOpen(false); setTab("members"); void load(); }} />}
    {pendingCommand && <MembershipCommandDialog command={pendingCommand} onClose={() => setPendingCommand(null)} onConfirm={() => void confirmCommand()} />}
  </section>;
}

function MemberTable({ members, canManage, currentAccountId, onCommand }: { members: WorkspaceMembership[]; canManage: boolean; currentAccountId: string; onCommand: (member: WorkspaceMembership, status: MembershipStatus) => void }) {
  return <section className="member-admin-table" aria-label="工作区成员列表">
    <div className="member-admin-head"><span>成员</span><span>授权角色</span><span>加入时间</span><span>状态</span><span>操作</span></div>
    <div className="member-admin-body">
      {members.map((member) => <div className="member-admin-row" key={member.id}>
        <span className="member-admin-person"><strong>{member.displayName}{member.accountId === currentAccountId && <small>当前账户</small>}</strong><code>{member.principalId}</code></span>
        <span className="member-admin-roles">{member.roleIds.map((role) => <small key={role}>{roleLabel(role)}</small>)}</span>
        <time dateTime={member.admittedAt}>{formatDate(member.admittedAt)}</time>
        <span className={`member-state member-state-${member.status}`}>{membershipStatusLabel(member.status)}</span>
        <span className="member-admin-actions">{canManage && member.status === "active" && <><button className="icon-button" type="button" title={`停用 ${member.displayName}`} aria-label={`停用 ${member.displayName}`} onClick={() => onCommand(member, "suspended")}><PauseCircle size={15} /></button><button className="icon-button is-danger" type="button" title={`移除 ${member.displayName}`} aria-label={`移除 ${member.displayName}`} onClick={() => onCommand(member, "revoked")}><UserMinus size={15} /></button></>}{canManage && member.status === "suspended" && <button className="icon-button" type="button" title={`恢复 ${member.displayName}`} aria-label={`恢复 ${member.displayName}`} onClick={() => onCommand(member, "active")}><RotateCcw size={15} /></button>}</span>
      </div>)}
      {members.length === 0 && <div className="member-admin-state"><Users size={20} /><strong>没有匹配成员</strong><span>调整搜索条件后重试。</span></div>}
    </div>
  </section>;
}

function LocalAccountDialog({ workspaceId, onClose, onCreated }: { workspaceId: string; onClose: () => void; onCreated: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [roleId, setRoleId] = useState("consumer_developer");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { dialog.current?.showModal(); }, []);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true); setError("");
    try { await createPasswordMember(workspaceId, { username: username.trim(), displayName: displayName.trim(), password, roleId }); setPassword(""); onCreated(); }
    catch (reason) { setError(identityErrorMessage(reason, "无法创建本地账号。")); }
    finally { setBusy(false); }
  };
  return <dialog ref={dialog} className="password-change-dialog member-admin-dialog" aria-labelledby="local-account-title" onCancel={(event) => { if (busy) event.preventDefault(); else onClose(); }}><form className="password-login-form" onSubmit={(event) => void submit(event)}>
    <header><h2 id="local-account-title">创建本地账号</h2><button className="icon-button" type="button" aria-label="关闭创建账号" disabled={busy} onClick={onClose}><X size={16} /></button></header>
    <label>登录账号<input type="text" autoComplete="off" required maxLength={320} autoFocus value={username} onChange={(event) => setUsername(event.target.value)} disabled={busy} /></label>
    <label>姓名<input required maxLength={256} value={displayName} onChange={(event) => setDisplayName(event.target.value)} disabled={busy} /></label>
    <label>初始密码<input type="password" autoComplete="new-password" required minLength={6} maxLength={1024} value={password} onChange={(event) => setPassword(event.target.value)} disabled={busy} /></label>
    <label>初始角色<select value={roleId} onChange={(event) => setRoleId(event.target.value)} disabled={busy}>{roleOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
    <p>账号支持字母、数字、点、下划线、短横线或邮箱格式。密码至少 6 个字符。</p>
    {error && <p className="password-error" role="alert">{error}</p>}
    <button type="submit" className="primary-button" disabled={busy}>{busy ? <LoaderCircle className="is-spinning" size={16} /> : <Users size={16} />}创建账号</button>
  </form></dialog>;
}

function InvitationTable({ invitations }: { invitations: WorkspaceInvitation[] }) {
  return <section className="member-admin-table invitation-admin-table" aria-label="工作区邀请列表">
    <div className="member-admin-head"><span>受邀身份</span><span>授权角色</span><span>创建时间</span><span>有效期</span><span>状态</span></div>
    <div className="member-admin-body">
      {invitations.map((invitation) => <div className="member-admin-row" key={invitation.id}>
        <span className="member-admin-person"><strong>{invitation.email || invitation.subject || "指定身份"}</strong><code>{invitation.id}</code></span>
        <span className="member-admin-roles"><small>{roleLabel(invitation.roleId)}</small></span>
        <time dateTime={invitation.createdAt}>{formatDate(invitation.createdAt)}</time>
        <time dateTime={invitation.expiresAt}>{formatDate(invitation.expiresAt)}</time>
        <span className={`invitation-state invitation-state-${invitation.status}`}>{invitationStatusLabel(invitation.status)}</span>
      </div>)}
      {invitations.length === 0 && <div className="member-admin-state"><Clock3 size={20} /><strong>没有匹配邀请</strong><span>新的受控邀请会显示在这里。</span></div>}
    </div>
  </section>;
}

function InvitationDialog({ workspaceId, onClose, onCreated }: { workspaceId: string; onClose: () => void; onCreated: (invitation: WorkspaceInvitation) => void }) {
  const [email, setEmail] = useState("");
  const [roleId, setRoleId] = useState("consumer_developer");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      onCreated(await createWorkspaceInvitation(workspaceId, { email: email.trim(), roleId }));
    } catch (reason) {
      setError(identityErrorMessage(reason, "无法创建邀请。"));
      setSubmitting(false);
    }
  };
  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}><form className="member-admin-dialog" role="dialog" aria-modal="true" aria-labelledby="invite-member-title" onSubmit={(event) => void submit(event)}>
    <header><div><span className="panel-kicker">Controlled admission</span><h2 id="invite-member-title">邀请工作区成员</h2></div><button className="icon-button" type="button" aria-label="关闭邀请" onClick={onClose}><X size={16} /></button></header>
    {error && <div className="member-admin-alert" role="alert"><AlertTriangle size={15} />{error}</div>}
    <label><span>组织邮箱</span><input type="email" required autoFocus value={email} onChange={(event) => setEmail(event.target.value)} placeholder="name@company.example" /></label>
    <label><span>初始角色</span><select aria-label="初始角色" value={roleId} onChange={(event) => setRoleId(event.target.value)}>{roleOptions.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select></label>
    <footer><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="submit" disabled={submitting}>{submitting ? <LoaderCircle className="is-spinning" size={15} /> : <MailPlus size={15} />}{submitting ? "发送中" : "创建邀请"}</button></footer>
  </form></div>;
}

function MembershipCommandDialog({ command, onClose, onConfirm }: { command: { member: WorkspaceMembership; status: MembershipStatus }; onClose: () => void; onConfirm: () => void }) {
  const revoke = command.status === "revoked";
  return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}><section className="member-command-dialog" role="dialog" aria-modal="true" aria-labelledby="member-command-title">
    <header><div className={revoke ? "command-icon is-danger" : "command-icon"}>{revoke ? <UserMinus size={18} /> : <PauseCircle size={18} />}</div><div><span className="panel-kicker">Membership command</span><h2 id="member-command-title">{revoke ? "移除成员" : "停用成员"}</h2></div></header>
    <p>{revoke ? `移除后，${command.member.displayName} 的工作区会话将立即失效，且不能直接恢复。` : `停用后，${command.member.displayName} 的工作区会话将立即失效，可由管理员恢复。`}</p>
    <footer><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className={revoke ? "danger-button" : "primary-button"} type="button" onClick={onConfirm}>{revoke ? <UserMinus size={15} /> : <Check size={15} />}{revoke ? "确认移除" : "确认停用"}</button></footer>
  </section></div>;
}

function roleLabel(roleId: string): string {
  return roleOptions.find(([value]) => value === roleId)?.[1] ?? roleId;
}

function membershipStatusLabel(status: MembershipStatus): string {
  if (status === "active") return "有效";
  if (status === "suspended") return "已停用";
  return "已移除";
}

function invitationStatusLabel(status: WorkspaceInvitation["status"]): string {
  if (status === "pending") return "待接受";
  if (status === "accepted") return "已接受";
  if (status === "expired") return "已过期";
  return "已撤销";
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).format(date);
}

function identityErrorMessage(reason: unknown, fallback: string): string {
  if (reason instanceof IdentityApiError && reason.status === 409) return reason.message || "此操作会移除最后一名有效管理员，已被拒绝。";
  if (reason instanceof IdentityApiError && reason.status === 403) return "当前角色没有执行此操作的权限。会话权限已重新读取。";
  return reason instanceof Error ? reason.message : fallback;
}

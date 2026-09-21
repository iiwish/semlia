/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { AlertTriangle, ChevronDown, Fingerprint, LoaderCircle, LogIn, LogOut, RefreshCw, ShieldCheck, UserRound } from "lucide-react";

import { AUTHORIZATION_STALE_EVENT, clearSessionCSRFToken, SESSION_INVALID_EVENT } from "./apiClient";
import { deleteSession, getSession, IdentityApiError, type SessionResponse, type SessionWorkspace } from "./identity";
import type { CapabilitySession } from "./types";
import { PasswordChangeDialog, PasswordLoginForm } from "./PasswordForms";
import { KeyRound } from "lucide-react";

type SessionPhase = "loading" | "authenticated" | "unauthenticated" | "error";

interface SessionNotice {
  title: string;
  detail: string;
}

interface SessionRuntimeValue {
  phase: SessionPhase;
  session: SessionResponse | null;
  activeWorkspaceId: string;
  activeWorkspace?: SessionWorkspace;
  capabilitySession?: CapabilitySession;
  error: string;
  notice: SessionNotice | null;
  expired: boolean;
  refresh: () => Promise<void>;
  selectWorkspace: (workspaceId: string) => void;
  logout: () => Promise<void>;
  dismissNotice: () => void;
}

const SessionRuntimeContext = createContext<SessionRuntimeValue | null>(null);

export function SessionRuntimeProvider({ children }: { children: ReactNode }) {
  const [phase, setPhase] = useState<SessionPhase>("loading");
  const [session, setSession] = useState<SessionResponse | null>(null);
  const [activeWorkspaceId, setActiveWorkspaceId] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<SessionNotice | null>(null);
  const [expired, setExpired] = useState(false);

  const acceptSession = useCallback((next: SessionResponse) => {
    setSession(next);
    setActiveWorkspaceId((current) => selectInitialWorkspace(next, current));
    setPhase("authenticated");
    setError("");
    setExpired(false);
  }, []);

  const refresh = useCallback(async () => {
    try {
      acceptSession(await getSession());
    } catch (reason) {
      if (reason instanceof IdentityApiError && reason.status === 401) {
        clearSessionCSRFToken();
        setSession(null);
        setActiveWorkspaceId("");
        setPhase("unauthenticated");
        return;
      }
      setError(reason instanceof Error ? reason.message : "无法连接身份服务。");
      setPhase("error");
    }
  }, [acceptSession]);

  useEffect(() => {
    const timer = window.setTimeout(() => void refresh(), 0);
    return () => window.clearTimeout(timer);
  }, [refresh]);

  useEffect(() => {
    const sessionInvalid = () => {
      setSession((current) => {
        setExpired(Boolean(current));
        return null;
      });
      setActiveWorkspaceId("");
      setPhase("unauthenticated");
      clearSessionCSRFToken();
    };
    const authorizationStale = () => {
      setNotice({ title: "操作未获授权", detail: "权限可能已由管理员更新。Semlia 已重新读取当前会话，未丢弃你正在查看的内容。" });
      void getSession().then(acceptSession).catch(() => undefined);
    };
    window.addEventListener(SESSION_INVALID_EVENT, sessionInvalid);
    window.addEventListener(AUTHORIZATION_STALE_EVENT, authorizationStale);
    return () => {
      window.removeEventListener(SESSION_INVALID_EVENT, sessionInvalid);
      window.removeEventListener(AUTHORIZATION_STALE_EVENT, authorizationStale);
    };
  }, [acceptSession]);

  const selectWorkspace = useCallback((workspaceId: string) => {
    setSession((current) => {
      if (!current?.workspaces.some((workspace) => workspace.id === workspaceId)) return current;
      setActiveWorkspaceId(workspaceId);
      storeWorkspaceSelection(current.account.id, workspaceId);
      setNotice(null);
      return current;
    });
  }, []);

  const logout = useCallback(async () => {
    try {
      await deleteSession();
    } finally {
      clearSessionCSRFToken();
      setSession(null);
      setActiveWorkspaceId("");
      setPhase("unauthenticated");
      setExpired(false);
    }
  }, []);

  const activeWorkspace = session?.workspaces.find((workspace) => workspace.id === activeWorkspaceId);
  const capabilitySession = useMemo(() => activeWorkspace ? capabilitySessionFor(activeWorkspace) : undefined, [activeWorkspace]);
  const value = useMemo<SessionRuntimeValue>(() => ({
    phase,
    session,
    activeWorkspaceId,
    activeWorkspace,
    capabilitySession,
    error,
    notice,
    expired,
    refresh,
    selectWorkspace,
    logout,
    dismissNotice: () => setNotice(null),
  }), [activeWorkspace, activeWorkspaceId, capabilitySession, error, expired, logout, notice, phase, refresh, selectWorkspace, session]);

  return <SessionRuntimeContext.Provider value={value}>{children}</SessionRuntimeContext.Provider>;
}

export function useSessionRuntime(): SessionRuntimeValue {
  const value = useContext(SessionRuntimeContext);
  if (!value) throw new Error("useSessionRuntime must be used inside SessionRuntimeProvider");
  return value;
}

export function useOptionalSessionRuntime(): SessionRuntimeValue | null {
  return useContext(SessionRuntimeContext);
}

export function SessionEntryState() {
  const runtime = useSessionRuntime();
  const callbackError = callbackFailure();
  const returnTo = `${window.location.pathname}${window.location.search}${window.location.hash}`;
  const loginHref = `/api/v1/auth/login?returnTo=${encodeURIComponent(returnTo.startsWith("/") ? returnTo : "/")}`;

  if (runtime.phase === "loading") {
    return <SessionEntryShell icon={<LoaderCircle className="is-spinning" size={23} />} kicker="Semlia Identity" title="正在验证会话" detail="读取账户与工作区成员资格。" />;
  }
  if (callbackError || runtime.phase === "error") {
    return <SessionEntryShell icon={<AlertTriangle size={23} />} kicker="登录未完成" title="无法进入 Semlia" detail={callbackError || runtime.error} danger actions={<><button className="secondary-button" type="button" onClick={() => void runtime.refresh()}><RefreshCw size={15} />重试</button><a className="primary-button" href={loginHref}><LogIn size={15} />重新登录</a></>} />;
  }
  if (runtime.phase === "authenticated" && runtime.session && runtime.session.workspaces.length === 0) {
    return <SessionEntryShell icon={<ShieldCheck size={23} />} kicker={runtime.session.account.displayName} title="尚未加入工作区" detail="账户已经验证，但没有有效的工作区成员资格。请联系管理员发送邀请。" actions={<button className="secondary-button" type="button" onClick={() => void runtime.logout()}><LogOut size={15} />退出登录</button>} />;
  }
  return <SessionEntryShell icon={<Fingerprint size={25} />} kicker="Semlia" title={runtime.expired ? "会话已过期" : "登录工作区"} detail={runtime.expired ? "会话已失效，请重新登录。" : "语义治理工作区"} actions={<PasswordLoginForm onAuthenticated={runtime.refresh} loginHref={loginHref} />} />;
}

export function SessionAccountControl() {
  const runtime = useOptionalSessionRuntime();
  const [passwordOpen, setPasswordOpen] = useState(false);
  if (!runtime?.session) return null;
  return <><details className="session-account-control">
    <summary aria-label={`账户 ${runtime.session.account.displayName}`}>
      <UserRound size={15} />
      <span><strong>{runtime.session.account.displayName}</strong><small>已验证</small></span>
      <ChevronDown size={13} />
    </summary>
    <div className="session-account-menu">
      <span><strong>{runtime.session.account.displayName}</strong><small>会话至 {formatSessionTime(runtime.session.expiresAt)}</small></span>
      {runtime.session.workspaces.length > 1 && <>
        <span><small>当前工作区</small><strong title={runtime.activeWorkspace?.displayName}>{runtime.activeWorkspace?.displayName}</strong></span>
        <div role="radiogroup" aria-label="切换工作区" style={{ display: "grid", gap: 7, maxHeight: 220, overflowY: "auto" }}>
          {runtime.session.workspaces.map((workspace) => <label key={workspace.id} style={{ display: "flex", alignItems: "center", gap: 7, fontSize: 11, overflowWrap: "anywhere" }}>
            <input type="radio" name="session-workspace" value={workspace.id} checked={runtime.activeWorkspaceId === workspace.id} onChange={() => runtime.selectWorkspace(workspace.id)} />
            {workspace.displayName}
          </label>)}
        </div>
      </>}
      <button type="button" onClick={() => void runtime.logout()}><LogOut size={15} />退出登录</button>
      {runtime.session.account.localPassword && <button type="button" onClick={() => setPasswordOpen(true)}><KeyRound size={15} />修改密码</button>}
    </div>
  </details>{passwordOpen && <PasswordChangeDialog onClose={() => setPasswordOpen(false)} onChanged={runtime.refresh} />}</>;
}

export function SessionAuthorizationNotice() {
  const runtime = useOptionalSessionRuntime();
  if (!runtime?.notice) return null;
  return <div className="session-authorization-notice" role="alert"><AlertTriangle size={16} /><span><strong>{runtime.notice.title}</strong>{runtime.notice.detail}</span><button type="button" onClick={runtime.dismissNotice}>关闭</button></div>;
}

function SessionEntryShell({ icon, kicker, title, detail, danger = false, actions }: { icon: ReactNode; kicker: string; title: string; detail: string; danger?: boolean; actions?: ReactNode }) {
  return <main className="session-entry-shell"><section className={`session-entry-content${danger ? " is-danger" : ""}`} aria-live="polite"><div className="session-entry-mark">{icon}</div><div><span>{kicker}</span><h1>{title}</h1><p>{detail}</p></div>{actions && <div className="session-entry-actions">{actions}</div>}</section></main>;
}

function capabilitySessionFor(workspace: SessionWorkspace): CapabilitySession {
  const serverCapabilities = workspace.capabilities;
  if (serverCapabilities) {
    return { principalId: workspace.principalId, version: String(workspace.authorizationVersion), capabilities: [...new Set(serverCapabilities)] };
  }
  return { principalId: workspace.principalId, version: String(workspace.authorizationVersion), capabilities: [] };
}

function selectInitialWorkspace(session: SessionResponse, current: string): string {
  const available = session.workspaces;
  if (available.some((workspace) => workspace.id === current)) return current;
  const stored = readWorkspaceSelection(session.account.id);
  if (available.some((workspace) => workspace.id === stored)) return stored;
  const first = available[0]?.id ?? "";
  if (first) storeWorkspaceSelection(session.account.id, first);
  return first;
}

function workspaceStorageKey(accountId: string): string {
  return `semlia.workspace.${accountId}`;
}

function readWorkspaceSelection(accountId: string): string {
  try {
    return window.sessionStorage.getItem(workspaceStorageKey(accountId)) ?? "";
  } catch {
    return "";
  }
}

function storeWorkspaceSelection(accountId: string, workspaceId: string): void {
  try {
    window.sessionStorage.setItem(workspaceStorageKey(accountId), workspaceId);
  } catch {
    // Session storage is only a preference; authorization is still server-side.
  }
}

function callbackFailure(): string {
  const params = new URLSearchParams(window.location.search);
  const code = params.get("auth_error");
  if (!code) return "";
  const description = params.get("auth_error_description");
  return description || `身份提供方返回错误：${code}`;
}

function formatSessionTime(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false }).format(parsed);
}

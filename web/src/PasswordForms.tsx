import { useEffect, useRef, useState, type FormEvent } from "react";
import { Eye, EyeOff, KeyRound, LoaderCircle, LogIn, X } from "lucide-react";
import { changePassword, getAuthMethods, passwordLogin } from "./identity";

export function PasswordLoginForm({ onAuthenticated, loginHref }: { onAuthenticated: () => Promise<void>; loginHref: string }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [visible, setVisible] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [oidc, setOIDC] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    void getAuthMethods(controller.signal).then((methods) => setOIDC(methods.oidc)).catch(() => undefined);
    return () => controller.abort();
  }, []);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await passwordLogin(username.trim(), password);
      setPassword("");
      await onAuthenticated();
    } catch (reason) {
      setPassword("");
      setError(reason instanceof Error ? reason.message : "登录未完成。");
    } finally { setBusy(false); }
  };
  return <form className="password-login-form" onSubmit={(event) => void submit(event)}>
    <label>登录账号<input type="text" autoComplete="username" required maxLength={320} value={username} onChange={(event) => setUsername(event.target.value)} disabled={busy} autoFocus /></label>
    <label htmlFor="login-password">密码</label>
    <div className="password-input"><input id="login-password" type={visible ? "text" : "password"} autoComplete="current-password" required maxLength={1024} value={password} onChange={(event) => setPassword(event.target.value)} disabled={busy} /><button type="button" className="icon-button" aria-label={visible ? "隐藏密码" : "显示密码"} title={visible ? "隐藏密码" : "显示密码"} onClick={() => setVisible(!visible)}>{visible ? <EyeOff size={16} /> : <Eye size={16} />}</button></div>
    {error && <p className="password-error" role="alert">{error}</p>}
    <button type="submit" className="primary-button" disabled={busy}>{busy ? <LoaderCircle size={16} className="is-spinning" /> : <LogIn size={16} />}{busy ? "登录中" : "登录"}</button>
    {oidc && <a className="secondary-button" href={loginHref}><LogIn size={15} />使用组织账户登录</a>}
  </form>;
}

export function PasswordChangeDialog({ onClose, onChanged }: { onClose: () => void; onChanged: () => Promise<void> }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [current, setCurrent] = useState("");
  const [replacement, setReplacement] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => { dialog.current?.showModal(); }, []);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (busy) return;
    if (replacement !== confirmation) { setError("两次输入的新密码不一致。"); return; }
    setBusy(true);
    setError("");
    try {
      await changePassword(current, replacement);
      setCurrent(""); setReplacement(""); setConfirmation("");
      onClose();
      await onChanged();
    } catch (reason) {
      setCurrent("");
      setError(reason instanceof Error ? reason.message : "密码未修改。");
    } finally { setBusy(false); }
  };
  return <dialog ref={dialog} className="password-change-dialog member-admin-dialog" aria-labelledby="password-change-title" onCancel={(event) => { if (busy) event.preventDefault(); else onClose(); }}>
    <form className="password-login-form" onSubmit={(event) => void submit(event)}>
      <header><h2 id="password-change-title">修改密码</h2><button className="icon-button" type="button" aria-label="关闭修改密码" disabled={busy} onClick={onClose}><X size={16} /></button></header>
      <label>当前密码<input type="password" autoComplete="current-password" required maxLength={1024} value={current} onChange={(event) => setCurrent(event.target.value)} disabled={busy} autoFocus /></label>
      <label>新密码<input type="password" autoComplete="new-password" required minLength={6} maxLength={1024} value={replacement} onChange={(event) => setReplacement(event.target.value)} disabled={busy} /></label>
      <label>确认新密码<input type="password" autoComplete="new-password" required minLength={6} maxLength={1024} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} disabled={busy} /></label>
      <p>至少 6 个字符。修改后，所有设备都需要重新登录。</p>
      {error && <p className="password-error" role="alert">{error}</p>}
      <button type="submit" className="primary-button" disabled={busy}>{busy ? <LoaderCircle size={16} className="is-spinning" /> : <KeyRound size={16} />}修改密码</button>
    </form>
  </dialog>;
}

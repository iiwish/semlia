import { useEffect, useRef, useState } from "react";
import { CircleAlert, LoaderCircle, Play, RefreshCw, Square } from "lucide-react";
import { cancelExecution, executePlan, getExecution, type ExecutionPlan, type ExecutionResult } from "./execution";

export function AskExecutionPanel({ workspaceId, plan }: { workspaceId: string; plan?: ExecutionPlan }) {
  return <ScopedExecutionPanel key={`${workspaceId}:${plan?.id ?? "history"}`} workspaceId={workspaceId} plan={plan} />;
}

function ScopedExecutionPanel({ workspaceId, plan }: { workspaceId: string; plan?: ExecutionPlan }) {
  const [result, setResult] = useState<ExecutionResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  const [pendingKey, setPendingKey] = useState("");
  const mounted = useRef(true);
  const storageKey = `semlia.execution.${workspaceId}`;
  useEffect(() => {
    mounted.current = true;
    const controller = new AbortController();
    if (!plan) {
      let stored: string | null = null;
      try { stored = localStorage.getItem(storageKey); } catch { /* Storage can be disabled. */ }
      if (stored) void getExecution(workspaceId, stored, controller.signal).then(value => { if (!controller.signal.aborted) setResult(value); }).catch(() => {});
    }
    return () => { mounted.current = false; controller.abort(); request.current?.abort(); };
  }, [workspaceId, storageKey, plan]);

  const execute = async () => {
    if (!plan || busy) return;
    const controller = new AbortController(); request.current = controller;
    const key = pendingKey || `web-execute-${crypto.randomUUID()}`;
    setPendingKey(key);
    setBusy(true); setResult(null); setError("");
    try {
      const value = await executePlan(workspaceId, plan, key, controller.signal);
      if (!mounted.current) return;
      setResult(value);
      if (value.run.state !== "running") setPendingKey("");
      try { localStorage.setItem(storageKey, value.run.id); } catch { /* Rows are never persisted. */ }
    } catch (failure) {
      if (!mounted.current) return;
      setError(controller.signal.aborted ? "取消请求已发送，执行状态待核对。" : failure instanceof Error ? failure.message : "EXECUTION_REQUEST_FAILED");
    } finally { if (mounted.current) setBusy(false); }
  };
  const inspect = async (cancel = false) => {
    if (!result || busy) return;
    setBusy(true); setError("");
    try {
      const value = cancel ? await cancelExecution(workspaceId, result.run.id) : await getExecution(workspaceId, result.run.id);
      if (mounted.current) setResult(value);
    } catch { if (mounted.current) setError("EXECUTION_STATUS_UNAVAILABLE"); }
    finally { if (mounted.current) setBusy(false); }
  };
  if (!plan && !result) return null;
  const enabled = plan?.executionStatus === "requires_execution_validation";
  const stateLabels = { running: "执行中", succeeded: "已完成", failed: "执行失败", cancelled: "已取消", unknown: "结果未知" };
  return <section className="ask-execution" aria-label="只读查询执行">
    <header><strong>只读查询</strong>{result && <span>{stateLabels[result.run.state]}</span>}
      {enabled && result?.run.state !== "running" && <button type="button" className="secondary-button" disabled={busy} onClick={() => void execute()}>{busy ? <LoaderCircle size={14} /> : pendingKey ? <RefreshCw size={14} /> : <Play size={14} />}{pendingKey ? "重试同一执行请求" : result ? "重新执行" : "执行只读查询"}</button>}
      {result?.run.state === "running" && <><button type="button" className="icon-button" disabled={busy} aria-label="核对执行状态" title="核对执行状态" onClick={() => void inspect()}><RefreshCw size={14}/></button><button type="button" className="icon-button" disabled={busy || result.run.cancelRequested} aria-label="请求取消执行" title="请求取消执行" onClick={() => void inspect(true)}><Square size={14}/></button></>}
      {busy && <button type="button" className="icon-button" aria-label="取消查询" title="取消查询" onClick={() => request.current?.abort()}><Square size={14} /></button>}
    </header>
    {!result && !error && !busy && <p>待校验当前权限、发布版本和专用只读数据源。</p>}
    {busy && <p role="status">正在执行受限只读查询。</p>}
    {error && <p role="alert"><CircleAlert size={14} />{error}</p>}
    {result && <><dl><div><dt>执行记录</dt><dd><code>{result.run.id}</code></dd></div><div><dt>结果</dt><dd>{result.run.rowCount} 行 / {result.run.byteCount} 字节</dd></div><div><dt>生效限制</dt><dd>{result.run.timeoutMs} 毫秒 / {result.run.maxRows} 行 / {result.run.maxBytes} 字节</dd></div><div><dt>策略版本</dt><dd>{result.run.policyVersion}</dd></div></dl>
      {result.run.errorCode && <p role="alert">{result.run.errorCode}</p>}
      {result.availability === "metadata_only" && <p role="status">仅保留执行记录，原始结果行未存储。此响应没有重新执行查询。</p>}
      {result.availability === "ephemeral" && <><div className="execution-results"><table><thead><tr>{result.columns?.map((name, index) => <th key={index} scope="col">{name}</th>)}</tr></thead><tbody>{result.rows?.map((row, index) => <tr key={index}>{row.map((value, column) => <td key={column}>{value === null ? "NULL" : typeof value === "object" ? JSON.stringify(value) : String(value)}</td>)}</tr>)}</tbody></table></div><p>结果仅在本次会话展示；刷新后只保留执行记录。</p></>}
    </>}
  </section>;
}

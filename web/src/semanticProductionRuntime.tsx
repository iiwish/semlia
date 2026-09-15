import { useCallback, useEffect, useRef, useState } from "react";
import { productionAPI, ProductionApiError, productionMessage, type BusinessRuleWitness, type GenerationResult, type OperationSummary, type ProductionAPI, type ProductionOperation, type ProductionRelease } from "./semanticProduction";

type WriteResult = { operationId?: string; releaseId?: string };
type PendingWrite = { key: string; label: string; action: (key: string) => Promise<WriteResult>; scope: string; operationId: string };
export function useSemanticProduction({ workspaceId, identityKey, candidateId, operationId, api = productionAPI }: { workspaceId: string; identityKey: string; candidateId?: string; operationId?: string; api?: ProductionAPI }) {
  const scope = `${workspaceId}:${identityKey}:${candidateId ?? ""}`;
  const [state, setState] = useState<{ scope: string; items: OperationSummary[]; nextCursor: string | null; detail: ProductionOperation | null; rules: BusinessRuleWitness[]; generations: GenerationResult[]; release: ProductionRelease | null }>({ scope, items: [], nextCursor: null, detail: null, rules: [], generations: [], release: null });
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState({ list: "", detail: "", write: "" });
  const [notice, setNotice] = useState("");
  const [uncertain, setUncertain] = useState(false);
  const scopeRef = useRef(scope);
  const listEpoch = useRef(0), detailEpoch = useRef(0);
  const selected = useRef(operationId ?? "");
  const selectedRelease = useRef<{ operationId: string; releaseId: string } | null>(null);
  const active = useRef(true), writing = useRef(false);
  const pending = useRef<PendingWrite | null>(null);
  const controller = useRef<AbortController | null>(null);
  const invalidate = useCallback(() => { listEpoch.current++; detailEpoch.current++; }, []);

  const list = useCallback(async (append = false, cursor?: string) => {
    const epoch = ++listEpoch.current;
    const requestScope = scope;
    try {
      const page = await api.list(workspaceId, { ...(candidateId ? { candidateId } : {}), ...(cursor ? { cursor } : {}) }, controller.current?.signal);
      if (!active.current || scopeRef.current !== requestScope || epoch !== listEpoch.current) return;
      setState((current) => ({ ...current, scope, items: append ? [...new Map([...current.items, ...page.items].map((item) => [item.id, item])).values()] : page.items, nextCursor: page.nextCursor }));
      setErrors((current) => ({ ...current, list: "" }));
    } catch (reason) {
      if (!active.current || scopeRef.current !== requestScope || epoch !== listEpoch.current) return;
      setErrors((current) => ({ ...current, list: productionMessage(reason) }));
      if (reason instanceof ProductionApiError && [401, 403, 404].includes(reason.status)) setState((current) => ({ ...current, items: [], detail: null, rules: [], generations: [], release: null }));
    } finally { if (active.current && scopeRef.current === requestScope && epoch === listEpoch.current) setLoading(false); }
  }, [api, candidateId, scope, workspaceId]);

  const open = useCallback(async (id: string) => {
    selected.current = id;
    const epoch = ++detailEpoch.current;
    const requestScope = scope;
    setLoading(true);
    setState((current) => current.detail?.summary.id === id ? current : { ...current, detail: null, rules: [], generations: [], release: null });
    try {
      const detail = await api.get(workspaceId, id, controller.current?.signal);
      const rules = await api.rules(workspaceId, id, detail.version, controller.current?.signal);
      const generations = await Promise.all(detail.generationRunIds.map((runId) => api.generation(workspaceId, id, runId, controller.current?.signal)));
      const releaseId = selectedRelease.current?.operationId === id ? selectedRelease.current.releaseId : detail.summary.releaseId;
      const release = releaseId ? await api.release(workspaceId, releaseId, controller.current?.signal) : null;
      if (release && release.protection.rootReleaseId !== detail.summary.releaseId) throw new Error("发布记录与生产集合不匹配。");
      if (!active.current || requestScope !== scopeRef.current || epoch !== detailEpoch.current) return;
      setState((current) => ({ ...current, scope, detail, rules, generations, release }));
      setErrors((current) => ({ ...current, detail: "" }));
    } catch (reason) {
      if (!active.current || requestScope !== scopeRef.current || epoch !== detailEpoch.current) return;
      setErrors((current) => ({ ...current, detail: productionMessage(reason) }));
      setState((current) => ({ ...current, detail: null, rules: [], generations: [], release: null }));
    } finally { if (active.current && requestScope === scopeRef.current && epoch === detailEpoch.current) setLoading(false); }
  }, [api, scope, workspaceId]);

  useEffect(() => {
    active.current = true;
    scopeRef.current = scope;
    const currentController = new AbortController();
    controller.current = currentController;
    selected.current = operationId ?? "";
    const locatedRelease = new URLSearchParams(window.location.search).get("productionRelease");
    selectedRelease.current = operationId && locatedRelease ? { operationId, releaseId: locatedRelease } : null;
    pending.current = null; writing.current = false;
    queueMicrotask(() => {
      if (currentController.signal.aborted) return;
      setBusy(false); setUncertain(false); setNotice(""); setErrors({ list: "", detail: "", write: "" }); setLoading(true);
      setState({ scope, items: [], nextCursor: null, detail: null, rules: [], generations: [], release: null });
      void list();
      if (operationId) void open(operationId);
    });
    return () => { active.current = false; currentController.abort(); invalidate(); };
  }, [scope, operationId, list, open, invalidate]);

  const execute = useCallback(async (request: PendingWrite) => {
    if (writing.current || request.scope !== scopeRef.current) return;
    writing.current = true; setBusy(true); setErrors((current) => ({ ...current, write: "" })); setNotice("");
    try {
      const result = await request.action(request.key);
      if (!active.current || request.scope !== scopeRef.current) return;
      pending.current = null; setUncertain(false);
      setNotice(`${request.label}已由服务器接收。`);
      const id = result.operationId ?? request.operationId;
      if (id && result.releaseId) selectedRelease.current = { operationId: id, releaseId: result.releaseId };
      await list();
      if (id) await open(id);
    } catch (reason) {
      if (!active.current || request.scope !== scopeRef.current) return;
      const unknown = !(reason instanceof ProductionApiError) || reason.status >= 500;
      pending.current = unknown ? request : null;
      setUncertain(unknown);
      setErrors((current) => ({ ...current, write: unknown ? `${request.label}结果尚不明确。请核对服务器，或重试同一请求；不会自动重复执行。` : productionMessage(reason) }));
    } finally {
      if (active.current && request.scope === scopeRef.current) { writing.current = false; setBusy(false); }
    }
  }, [list, open]);
  const write = useCallback(async (label: string, action: PendingWrite["action"]) => {
    if (writing.current || pending.current) return;
    await execute({ label, action, scope, operationId: selected.current, key: `web-production:${crypto.randomUUID()}` });
  }, [execute, scope]);
  const refresh = useCallback(async () => { await list(); if (selected.current) await open(selected.current); }, [list, open]);
  const hasActive = state.scope === scope && (state.detail?.activeValidation.status === "queued" || state.detail?.activeValidation.status === "running" || state.generations.some((run) => run.status === "queued" || run.status === "running") || state.release?.projectionStatus === "pending");
  useEffect(() => {
    if (!hasActive) return;
    let polling = false;
    const timer = window.setInterval(() => { if (!polling && !writing.current) { polling = true; void open(selected.current).finally(() => { polling = false; }); } }, 2500);
    return () => window.clearInterval(timer);
  }, [hasActive, open]);
  const visible = state.scope === scope ? state : { items: [], nextCursor: null, detail: null, rules: [], generations: [], release: null };
  return { ...visible, loading, busy, error: errors.write || errors.detail || errors.list, readFailed: Boolean(errors.detail || errors.list), notice, uncertain, open, refresh, write, retry: async () => { if (pending.current) await execute(pending.current); }, loadMore: () => list(true, state.nextCursor ?? undefined), clearSelection: () => { detailEpoch.current++; selected.current = ""; setErrors((current) => ({ ...current, detail: "" })); setState((current) => ({ ...current, detail: null, rules: [], generations: [], release: null })); } };
}

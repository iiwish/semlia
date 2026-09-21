import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { ArrowRight, CheckCircle2, CircleAlert, LoaderCircle, RefreshCw, Search, ShieldCheck } from "lucide-react";
import { useCan } from "./authorization";
import { buildInbox, readInboxPages, type InboxOperation, type InboxScope } from "./inbox";
import { productionAPI, productionMessage } from "./semanticProduction";
import { useWorkbenchRuntime } from "./workbenchRuntime";
import type { WorkbenchAttentionItem } from "./workbench";

export type InboxViewState = { search: string; group: string; scrollTop: number };

export function UnifiedInbox({ workspaceId, principalId, scope, focusSearchRequestEpoch, onOpenOperation, onOpenTask, onOpenReviews, onScopeChange, initialViewState, onRememberView, focusScopeRequestEpoch = 0 }: {
  workspaceId: string; principalId?: string; scope: InboxScope; focusSearchRequestEpoch: number;
  onOpenOperation: (id: string) => void; onOpenTask: (item: WorkbenchAttentionItem) => void;
  onOpenReviews?: () => void;
  onScopeChange?: (scope: InboxScope) => void;
  initialViewState?: InboxViewState;
  onRememberView?: (state: InboxViewState) => void;
  focusScopeRequestEpoch?: number;
}) {
  const { readInboxPage, access } = useWorkbenchRuntime();
  const assetRead = useCan("asset.read");
  const bindingRead = useCan("binding.read");
  const canReadKnowledge = assetRead || bindingRead;
  const [revision, setRevision] = useState(0);
  const [data, setData] = useState<{ key: string; operations: InboxOperation[]; tasks: WorkbenchAttentionItem[]; errors: string[] }>();
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState(initialViewState?.search ?? "");
  const [group, setGroup] = useState(initialViewState?.group ?? "");
  const sectionRef = useRef<HTMLElement>(null);
  const scopeRef = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    if (focusScopeRequestEpoch) scopeRef.current?.querySelector<HTMLButtonElement>('[aria-pressed="true"]')?.focus({ preventScroll: true });
  }, [focusScopeRequestEpoch]);
  const restoredScroll = useRef(false);
  useLayoutEffect(() => {
    const canvas = sectionRef.current?.closest("main");
    return () => onRememberView?.({ search, group, scrollTop: canvas?.scrollTop ?? 0 });
  }, [search, group, onRememberView]);
  useLayoutEffect(() => {
    if (loading || restoredScroll.current) return;
    const canvas = sectionRef.current?.closest("main");
    if (canvas) canvas.scrollTop = initialViewState?.scrollTop ?? 0;
    restoredScroll.current = true;
  }, [loading, initialViewState]);
  const searchRef = useRef<HTMLInputElement>(null);
  const key = `${workspaceId}:${principalId}:${scope}:${canReadKnowledge}:${access.read}`;
  useEffect(() => { if (focusSearchRequestEpoch) searchRef.current?.focus(); }, [focusSearchRequestEpoch]);
  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;
    const load = async () => {
      setLoading(true);
      const errors: string[] = [];
      let operations: InboxOperation[] = [];
      let tasks: WorkbenchAttentionItem[] = [];
      await Promise.all([
        (async () => {
          if (!canReadKnowledge) return;
          try {
            if (scope === "initiated" && !principalId) throw new Error("当前会话缺少身份，无法读取我发起的知识。");
            operations = await readInboxPages((cursor) => productionAPI.list(workspaceId, { cursor, limit: 100, ...(scope === "initiated" ? { createdBy: principalId } : {}) }, signal), signal);
          } catch (error) { errors.push(`知识待办读取失败：${productionMessage(error)}`); }
        })(),
        (async () => {
          if (!access.read) return;
          try {
            const views = scope === "initiated" ? ["initiated" as const] : ["mine" as const, "team" as const, ...(scope === "done" ? ["initiated" as const] : [])];
            const pages = await Promise.all(views.map((view) => readInboxPages(async (cursor) => {
              const page = await readInboxPage(workspaceId, { view, sort: "updated_desc", limit: 100 }, cursor, signal);
              return { items: page.items, nextCursor: page.page.nextCursor };
            }, signal)));
            tasks = pages.flat();
          } catch (error) { errors.push(`协作待办读取失败：${error instanceof Error ? error.message : "请稍后重试"}`); }
        })(),
      ]);
      if (!signal.aborted) { setData({ key, operations, tasks, errors }); setLoading(false); }
    };
    void load();
    return () => controller.abort();
  }, [access.read, canReadKnowledge, key, principalId, readInboxPage, revision, scope, workspaceId]);
  useEffect(() => {
    const refresh = () => { if (document.visibilityState === "visible") setRevision((value) => value + 1); };
    window.addEventListener("focus", refresh);
    const timer = window.setInterval(refresh, 30_000);
    return () => { window.removeEventListener("focus", refresh); window.clearInterval(timer); };
  }, []);
  const current = data?.key === key ? data : undefined;
  const rows = useMemo(() => buildInbox(current?.operations ?? [], current?.tasks ?? [], scope), [current, scope]);
  const visible = rows.filter((row) => (!group || row.group === group) && `${row.title} ${row.reason} ${row.state}`.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()));
  const complete = current && !current.errors.length;
  return <section ref={sectionRef} className="view unified-inbox" aria-label="统一待办">
    <header className="inbox-heading"><h2>待办</h2><div className="production-actions">{onOpenReviews && assetRead && <button className="secondary-button" type="button" onClick={onOpenReviews}><ShieldCheck size={15} />批量审核</button>}<button className="icon-button" title="刷新待办" aria-label="刷新待办" disabled={loading} onClick={() => setRevision((value) => value + 1)}><RefreshCw size={17} /></button></div></header>
    <div ref={scopeRef} className="segment-control inbox-scopes" role="group" aria-label="待办视图">
      {([ ["pending", "待处理"], ["initiated", "我发起"], ["done", "已结束"] ] as const).map(([value, label]) => <button key={value} type="button" aria-pressed={scope === value} onClick={() => onScopeChange?.(value)}>{label}</button>)}
    </div>
    <div className="inbox-toolbar"><label className="inbox-search"><Search size={16} /><input ref={searchRef} type="search" aria-label="搜索待办" placeholder="搜索知识名称或待处理问题" value={search} onChange={(event) => setSearch(event.target.value)} /></label><select aria-label="筛选待办类型" value={group} onChange={(event) => setGroup(event.target.value)}><option value="">全部类型</option>{["知识处理", "接入异常", "问数问题", "运行异常"].map((value) => <option key={value}>{value}</option>)}</select><span>{complete ? `${visible.length} 项` : "数量未确认"}</span></div>
    {!canReadKnowledge && !access.read && <p role="alert">当前账号没有待办读取权限。</p>}
    {current?.errors.map((error) => <div className="production-alert" role="alert" key={error}><CircleAlert size={16} />{error}。列表未完整加载。<button type="button" onClick={() => setRevision((value) => value + 1)}>重试</button></div>)}
    {loading && <p role="status"><LoaderCircle className="spin" size={16} />正在核对待办状态</p>}
    <div className="inbox-list" aria-label="待办列表">{visible.map((row) => <button type="button" className="inbox-row" key={row.id} aria-label={`${row.action}：${row.title}`} onClick={() => row.operation ? onOpenOperation(row.operation.id) : row.task && onOpenTask(row.task)}><span className="inbox-row-main"><strong>{row.title}</strong><small>{row.reason}</small></span><span className="inbox-row-state"><span>{row.state}</span><small>{row.group}</small></span><time dateTime={row.updatedAt}>{new Date(row.updatedAt).toLocaleDateString("zh-CN")}</time><span className="inbox-row-action">{row.action}<ArrowRight size={15} /></span></button>)}</div>
    {!loading && complete && !visible.length && <div className="workbench-empty"><CheckCircle2 size={22} /><strong>{search || group ? "没有匹配的事项" : scope === "pending" ? "当前可见范围内没有待处理事项" : "暂无记录"}</strong>{(search || group) && <button type="button" onClick={() => { setSearch(""); setGroup(""); }}>清除筛选</button>}</div>}
  </section>;
}

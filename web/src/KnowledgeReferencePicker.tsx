import { createContext, useContext, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Search, X } from "lucide-react";
import { getAsset, listAssets, type CatalogAsset, type CatalogAssetDetail } from "./catalog";
import type { KnowledgeReference, KnowledgeSpec, KnowledgeType } from "./knowledge";

// eslint-disable-next-line react-refresh/only-export-components
export const KnowledgeWorkspaceContext = createContext<string | undefined>(undefined);
const referenceKey = (value?: KnowledgeReference) => value ? `${value.assetId}:${value.revisionId}:${value.releaseId}:${value.memberId ?? ""}` : "";

function publishedReference(asset: CatalogAssetDetail): KnowledgeReference | undefined {
  const release = asset.authoritySections.find((section) => section.kind === "released_state" && section.availability === "available");
  if (!release?.releaseId || !release.revisionId || release.revisionId !== asset.currentRevision?.id) return;
  return { assetId: asset.id, revisionId: release.revisionId, releaseId: release.releaseId };
}

export function KnowledgeReferenceEditor({ label, value, onChange, member = false, type = "business_object" }: { label: string; value?: KnowledgeReference; onChange: (value: KnowledgeReference | undefined) => void; member?: boolean; type?: KnowledgeType }) {
  const workspaceId = useContext(KnowledgeWorkspaceContext);
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<{ key: string; name: string }>();
  return <div className="knowledge-reference"><strong>{label}</strong><div className="knowledge-reference-choice"><span>{value?.assetId ? selected?.key === referenceKey(value) ? selected.name : value.assetId : "未选择已发布知识"}{value?.memberId && <small>{value.memberId}</small>}</span><button type="button" className="secondary-button" disabled={!workspaceId} onClick={() => setOpen(true)}><Search size={14} />选择{label}</button>{value && <button type="button" className="icon-button" title={`清除${label}`} aria-label={`清除${label}`} onClick={() => { onChange(undefined); setSelected(undefined); }}><X size={14} /></button>}</div>
    {value?.assetId && <details><summary>固定版本标识</summary><dl className="knowledge-reference-pins">{Object.entries(value).map(([key, id]) => <div key={key}><dt>{key}</dt><dd><code>{id}</code></dd></div>)}</dl></details>}
    {open && workspaceId && <PublishedReferenceDialog workspaceId={workspaceId} label={label} type={type} member={member} onClose={() => setOpen(false)} onSelect={(ref, name) => { onChange(ref); setSelected({ key: referenceKey(ref), name }); setOpen(false); }} />}
  </div>;
}

function PublishedReferenceDialog({ workspaceId, label, type, member, onClose, onSelect }: { workspaceId: string; label: string; type: KnowledgeType; member: boolean; onClose: () => void; onSelect: (ref: KnowledgeReference, name: string) => void }) {
  const dialog = useRef<HTMLDivElement>(null);
  const [search, setSearch] = useState("");
  const [items, setItems] = useState<CatalogAsset[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [detail, setDetail] = useState<CatalogAssetDetail>();
  const [memberId, setMemberId] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  const listRequest = useRef<AbortController | null>(null);
  const close = useRef(onClose);
  useEffect(() => { close.current = onClose; }, [onClose]);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialog.current?.querySelector<HTMLInputElement>("input")?.focus();
    const keyboard = (event: KeyboardEvent) => {
      if (event.key === "Escape") { event.stopImmediatePropagation(); event.preventDefault(); close.current(); }
      if (event.key !== "Tab") return;
      const elements = Array.from(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), [tabindex="0"]') ?? []);
      const first = elements[0], last = elements.at(-1);
      if (event.shiftKey && (document.activeElement === first || !dialog.current?.contains(document.activeElement))) { event.preventDefault(); last?.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
    };
    document.addEventListener("keydown", keyboard, true);
    return () => { document.removeEventListener("keydown", keyboard, true); request.current?.abort(); listRequest.current?.abort(); previous?.focus(); };
  }, []);
  useEffect(() => {
    const controller = new AbortController(); listRequest.current?.abort(); listRequest.current = controller;
    request.current?.abort();
    const timer = window.setTimeout(() => {
      setLoading(true); setDetail(undefined); setMemberId(""); setError(""); setCursor(undefined); setItems([]);
      void listAssets(workspaceId, search, type, undefined, controller.signal).then((page) => { if (!controller.signal.aborted) { setItems(page.items); setCursor(page.page.nextCursor ?? undefined); } }).catch(() => { if (!controller.signal.aborted) setError("已发布知识读取失败，请重新搜索。"); }).finally(() => { if (!controller.signal.aborted) setLoading(false); });
    }, 150);
    return () => { window.clearTimeout(timer); controller.abort(); };
  }, [workspaceId, search, type]);
  const inspect = async (id: string) => {
    request.current?.abort(); const controller = new AbortController(); request.current = controller;
    setDetail(undefined); setMemberId(""); setError("");
    try {
      const asset = await getAsset(workspaceId, id, controller.signal);
      if (controller.signal.aborted) return;
      if (asset.assetType !== type || !publishedReference(asset)) { setError("该知识没有可选的当前已发布版本。"); return; }
      setDetail(asset);
    } catch { if (!controller.signal.aborted) setError("知识版本读取失败，请重试。"); }
  };
  const more = async () => {
    if (!cursor || loading) return;
    const controller = new AbortController(); listRequest.current?.abort(); listRequest.current = controller; setLoading(true);
    try { const page = await listAssets(workspaceId, search, type, cursor, controller.signal); if (!controller.signal.aborted) { setItems((current) => [...current, ...page.items]); setCursor(page.page.nextCursor ?? undefined); } }
    catch { if (!controller.signal.aborted) setError("更多知识读取失败，请重试。"); }
    finally { if (!controller.signal.aborted) setLoading(false); }
  };
  const spec = detail?.currentRevision?.content.spec as KnowledgeSpec | undefined;
  const members = spec?.members ?? [];
  const ref = detail && publishedReference(detail);
  return createPortal(<div className="modal-backdrop knowledge-reference-backdrop"><div ref={dialog} role="dialog" aria-modal="true" aria-label={`选择${label}`} className="knowledge-reference-dialog"><header><h3>选择{label}</h3><button type="button" className="icon-button" aria-label="关闭知识选择" title="关闭知识选择" onClick={onClose}><X size={18} /></button></header>
    <label className="production-field"><span>搜索知识</span><input aria-label="搜索已发布知识" value={search} onChange={(event) => setSearch(event.target.value)} /></label>
    <div className="knowledge-reference-results" aria-label="知识搜索结果">{items.filter((item) => item.assetType === type).map((item) => <button type="button" className="knowledge-reference-result" key={item.id} onClick={() => void inspect(item.id)} aria-pressed={detail?.id === item.id}><strong>{item.title}</strong><small>{item.address}</small></button>)}</div>
    {loading ? <p role="status">正在读取知识</p> : !items.length && <p role="status">没有匹配的知识</p>}{cursor && <button type="button" className="secondary-button" disabled={loading} onClick={() => void more()}>加载更多知识</button>}
    {error && <p role="alert">{error}</p>}
    {detail && <section className="knowledge-reference-selected"><strong>{detail.title} · 已发布</strong><code>{ref?.revisionId}</code>{member && <label className="production-field"><span>已发布成员</span><select aria-label="已发布成员" value={memberId} onChange={(event) => setMemberId(event.target.value)}><option value="">选择成员</option>{members.map((item) => <option key={item.id} value={item.id}>{item.name || item.id} · {item.id}</option>)}</select></label>}{member && !members.length && <p role="status">该版本没有可引用的成员</p>}</section>}
    <footer><button type="button" className="secondary-button" onClick={onClose}>取消</button><button type="button" className="primary-button" disabled={!ref || (member && !members.some((item) => item.id === memberId))} onClick={() => { if (ref && detail) onSelect({ ...ref, ...(member ? { memberId } : {}) }, detail.title); }}>固定此版本</button></footer>
  </div></div>, document.body);
}

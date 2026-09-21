import { useEffect, useRef, useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
import { CircleAlert, Database, LoaderCircle, Plus, RefreshCw, X } from "lucide-react";

import type { SemanticAssetType } from "./catalog";
import { useCatalogRuntime } from "./catalogRuntime";
import { KnowledgeSpecEditor } from "./KnowledgeSpecEditor";
import { KnowledgeSourcePicker, type KnowledgeSourceMember } from "./KnowledgeSourcePicker";
import { initialKnowledgeSpec, type KnowledgeSpec } from "./knowledge";

const assetTypes: Array<{ value: SemanticAssetType; label: string }> = [
  { value: "business_object", label: "业务对象" },
  { value: "business_term", label: "业务口径" },
  { value: "metric", label: "指标" },
  { value: "data_asset", label: "数据资产" },
  { value: "analysis_model", label: "分析模型" },
];

export function CatalogWorkspaceControl() {
  const { workspaces, workspaceId, workspace, setWorkspaceId, loading } = useCatalogRuntime();
  return (
    <div className="catalog-workspace-control">
      <label>
        <span>验收数据</span>
        <select aria-label="工作区" value={workspaceId} onChange={(event) => setWorkspaceId(event.target.value)}>
          {workspaces.map((item) => <option key={item.id} value={item.id}>{item.displayName}</option>)}
        </select>
      </label>
      <span className="catalog-connection-state"><i data-loading={loading || undefined} />{loading ? "同步中" : workspace ? "Registry 已连接" : "未配置"}</span>
    </div>
  );
}

export function CatalogRefreshButton() {
  const { loading, refresh } = useCatalogRuntime();
  return <button className="icon-button" type="button" title="刷新知识目录" aria-label="刷新知识目录" onClick={refresh} disabled={loading}><RefreshCw size={15} className={loading ? "is-spinning" : undefined} /></button>;
}

export function CatalogDataNotice() {
  const { workspace } = useCatalogRuntime();
  const simulated = workspace?.displayName.includes("模拟数据");
  return simulated ? <span className="catalog-data-notice">模拟数据</span> : null;
}

export function CreateCatalogAssetButton({ onCreated, compact = true }: { onCreated?: (assetId: string) => void; compact?: boolean }) {
  const [open, setOpen] = useState(false);
  return <>
    <button className={compact ? "icon-button catalog-create-button" : "primary-button"} type="button" title="新建知识" aria-label="新建知识" onClick={() => setOpen(true)}>
      <Plus size={16} />{!compact && <span>新建知识</span>}
    </button>
    {open && createPortal(<CreateAssetDialog onClose={() => setOpen(false)} onCreated={(assetId) => { setOpen(false); onCreated?.(assetId); }} />, document.body)}
  </>;
}

export function CatalogEntryState() {
  const { workspaces, workspaceId, assets, loading, error, query } = useCatalogRuntime();
  if (loading && workspaces.length === 0) return <RuntimeState icon={<LoaderCircle className="is-spinning" size={22} />} title="正在连接语义注册表" detail="读取工作区与 Catalog 合同。" />;
  if (error && !workspaceId) return <RuntimeState icon={<CircleAlert size={22} />} title="无法连接后端" detail={error} danger />;
  if (!workspaceId) return <WorkspaceBootstrap />;
  if (!loading && assets.length === 0 && !query) return <EmptyCatalog />;
  return null;
}

function WorkspaceBootstrap() {
  const { createWorkspace } = useCatalogRuntime();
  const [slug, setSlug] = useState("default");
  const [displayName, setDisplayName] = useState("默认工作区");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      await createWorkspace(slug.trim(), displayName.trim());
      setError("");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "无法创建工作区。");
    } finally {
      setSubmitting(false);
    }
  };
  return (
    <div className="catalog-entry-shell">
      <form className="catalog-entry-panel" onSubmit={(event) => void submit(event)}>
        <Database size={23} />
        <div><span>Semlia Registry</span><h1>创建第一个工作区</h1><p>工作区建立公开 TypeID 命名空间，并承载真实语义资产。</p></div>
        {error && <div className="catalog-runtime-error" role="alert">{error}</div>}
        <label><span>显示名称</span><input required value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label>
        <label><span>Slug</span><input required pattern="[a-z0-9][a-z0-9-]*" value={slug} onChange={(event) => setSlug(event.target.value)} /></label>
        <button className="primary-button" type="submit" disabled={submitting}>{submitting ? "创建中" : "创建工作区"}</button>
      </form>
    </div>
  );
}

function EmptyCatalog() {
  return (
    <div className="catalog-entry-shell">
      <section className="catalog-entry-panel catalog-empty-panel">
        <Database size={23} />
        <div><span>真实 PostgreSQL 工作区</span><h1>注册第一个语义资产</h1><p>当前工作区为空。创建结果会立即写入 M1 Catalog，而不是浏览器 Mock。</p></div>
        <CreateCatalogAssetButton compact={false} />
      </section>
    </div>
  );
}

function RuntimeState({ icon, title, detail, danger = false }: { icon: React.ReactNode; title: string; detail: string; danger?: boolean }) {
  return <div className="catalog-entry-shell"><section className={`catalog-entry-panel catalog-runtime-state${danger ? " is-danger" : ""}`}>{icon}<div><span>Semlia Registry</span><h1>{title}</h1><p>{detail}</p></div></section></div>;
}

function CreateAssetDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (assetId: string) => void }) {
  const { createAsset, workspaceId } = useCatalogRuntime();
  const [members, setMembers] = useState<KnowledgeSourceMember[]>([]);
  const [address, setAddress] = useState("commerce.");
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const [assetType, setAssetType] = useState<SemanticAssetType>("metric");
  const [scope, setScope] = useState("");
  const [spec, setSpec] = useState<KnowledgeSpec>(() => initialKnowledgeSpec("metric"));
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const dialog = useRef<HTMLFormElement>(null);
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    dialog.current?.querySelector<HTMLInputElement>("input")?.focus();
    return () => { previous?.focus(); };
  }, []);
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setSubmitting(true);
    try {
      const asset = await createAsset({ address: address.trim(), title: title.trim(), summary: summary.trim(), assetType, scope, spec });
      onCreated(asset.id);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "无法创建语义资产。");
    } finally {
      setSubmitting(false);
    }
  };
  return (
    <div className="dialog-backdrop catalog-create-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <form ref={dialog} className="catalog-create-dialog" role="dialog" aria-modal="true" aria-labelledby="catalog-create-title" onSubmit={(event) => void submit(event)} onKeyDown={(event) => {
        if (!dialog.current?.contains(event.target as Node)) return;
        if (event.key === "Escape" && !submitting) { event.stopPropagation(); onClose(); }
        if (event.key !== "Tab") return;
        const controls = Array.from(dialog.current.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), summary')).filter((item) => !item.closest("details:not([open])") || item.tagName === "SUMMARY");
        const first = controls[0], last = controls.at(-1);
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
      }}>
        <header><div><span className="panel-kicker">知识草稿</span><h2 id="catalog-create-title">新建知识</h2></div><button className="icon-button" type="button" aria-label="关闭" onClick={onClose}><X size={17} /></button></header>
        {error && <div className="catalog-runtime-error" role="alert"><CircleAlert size={15} />{error}</div>}
        <div className="catalog-create-fields">
          <label><span>语义地址</span><input required value={address} onChange={(event) => setAddress(event.target.value)} placeholder="commerce.net_revenue" /></label>
          <label><span>资产类型</span><select aria-label="资产类型" value={assetType} onChange={(event) => { const type = event.target.value as SemanticAssetType; setAssetType(type); setSpec(initialKnowledgeSpec(type)); }}>{assetTypes.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
          <label><span>显示名称</span><input required value={title} onChange={(event) => setTitle(event.target.value)} placeholder="净收入" /></label>
          <label><span>规范摘要</span><textarea required value={summary} onChange={(event) => setSummary(event.target.value)} placeholder="声明业务含义、范围与关键排除项。" /></label>
          <label><span>适用范围</span><textarea value={scope} onChange={(event) => setScope(event.target.value)} /></label>
          {assetType === "data_asset" && <KnowledgeSourcePicker workspaceId={workspaceId} onMembers={setMembers} />}
          <KnowledgeSpecEditor type={assetType} value={spec} onChange={setSpec} members={members} workspaceId={workspaceId} />
        </div>
        <footer><button className="secondary-button" type="button" onClick={onClose}>取消</button><button className="primary-button" type="submit" disabled={submitting}>{submitting ? "写入中" : "创建并打开"}</button></footer>
      </form>
    </div>
  );
}

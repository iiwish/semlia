import { FormEvent, useEffect, useMemo, useState } from "react";
import {
  Activity,
  BookOpen,
  Boxes,
  ChevronRight,
  CircleAlert,
  Clock3,
  Database,
  FileCode2,
  GitBranch,
  LoaderCircle,
  Plus,
  RefreshCw,
  Search,
  Settings,
  ShieldCheck,
  Table2,
  X,
} from "lucide-react";

import {
  createAsset,
  createWorkspace,
  getAsset,
  listAssets,
  listWorkspaces,
  type CatalogAsset,
  type CatalogAssetDetail,
  type SemanticAssetType,
  type Workspace,
} from "./catalog";
import { StatusView } from "./StatusView";
import "./styles.css";

const assetTypes: Array<{ value: SemanticAssetType | ""; label: string }> = [
  { value: "", label: "All types" },
  { value: "metric", label: "Metrics" },
  { value: "measure", label: "Measures" },
  { value: "dimension", label: "Dimensions" },
  { value: "entity", label: "Entities" },
  { value: "semantic_model", label: "Semantic models" },
  { value: "segment", label: "Segments" },
  { value: "concept", label: "Concepts" },
];

export function App() {
  if (window.location.pathname === "/status") return <StatusView />;
  return <CatalogWorkspace />;
}

function CatalogWorkspace() {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [workspaceId, setWorkspaceId] = useState("");
  const [assets, setAssets] = useState<CatalogAsset[]>([]);
  const [selected, setSelected] = useState<CatalogAssetDetail | null>(null);
  const [search, setSearch] = useState("");
  const [assetType, setAssetType] = useState<SemanticAssetType | "">("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showWorkspaceDialog, setShowWorkspaceDialog] = useState(false);
  const [showAssetDialog, setShowAssetDialog] = useState(false);
  const [refreshVersion, setRefreshVersion] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    listWorkspaces(controller.signal)
      .then((items) => {
        setWorkspaces(items);
        setWorkspaceId((current) => current || items[0]?.id || "");
        setShowWorkspaceDialog(items.length === 0);
        setError("");
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!workspaceId) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      listAssets(workspaceId, search.trim(), assetType, controller.signal)
        .then((page) => {
          setAssets(page.items);
          setError("");
        })
        .catch((reason: Error) => setError(reason.message))
        .finally(() => setLoading(false));
    }, 160);
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [workspaceId, search, assetType, refreshVersion]);

  const workspace = workspaces.find((item) => item.id === workspaceId);
  const counts = useMemo(() => {
    const result = new Map<string, number>();
    for (const asset of assets) result.set(asset.assetType, (result.get(asset.assetType) ?? 0) + 1);
    return result;
  }, [assets]);

  const selectAsset = async (asset: CatalogAsset) => {
    setSelected({ ...asset, createdAt: asset.updatedAt, relationCount: 0 });
    try {
      setSelected(await getAsset(workspaceId, asset.id));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Unable to load asset detail.");
    }
  };

  return (
    <div className="catalog-shell">
      <aside className="rail" aria-label="Primary navigation">
        <a className="rail-brand" href="/" aria-label="Semlia catalog">S</a>
        <nav>
          <a className="rail-link active" href="/" title="Catalog"><BookOpen size={19} /><span>Catalog</span></a>
          <button className="rail-link" type="button" title="Ontology"><Boxes size={19} /><span>Ontology</span></button>
          <button className="rail-link" type="button" title="Sources"><Database size={19} /><span>Sources</span></button>
          <button className="rail-link" type="button" title="Lineage"><GitBranch size={19} /><span>Lineage</span></button>
        </nav>
        <nav className="rail-bottom">
          <a className="rail-link" href="/status" title="System status"><Activity size={19} /><span>Status</span></a>
          <button className="rail-link" type="button" title="Settings"><Settings size={19} /><span>Settings</span></button>
        </nav>
      </aside>

      <div className="catalog-main">
        <header className="workspace-bar">
          <div className="workspace-identity">
            <span className="workspace-product">Semlia</span>
            <ChevronRight size={14} aria-hidden="true" />
            <select
              aria-label="Workspace"
              value={workspaceId}
              onChange={(event) => { setWorkspaceId(event.target.value); setSelected(null); }}
            >
              {workspaces.map((item) => <option value={item.id} key={item.id}>{item.displayName}</option>)}
            </select>
          </div>
          <div className="workspace-health"><span aria-hidden="true" />Registry connected</div>
        </header>

        <main className="catalog-page">
          <div className="catalog-heading">
            <div>
              <p className="eyebrow">Semantic registry</p>
              <h1>Catalog</h1>
              <p className="page-description">Governed definitions, ownership and evidence across the workspace.</p>
            </div>
            <div className="heading-actions">
              <button className="icon-button" type="button" title="Refresh catalog" aria-label="Refresh catalog" onClick={() => { setLoading(true); setRefreshVersion((value) => value + 1); }}>
                <RefreshCw size={17} />
              </button>
              <button className="primary-button" type="button" onClick={() => setShowAssetDialog(true)} disabled={!workspaceId}>
                <Plus size={17} /> New asset
              </button>
            </div>
          </div>

          <section className="catalog-metrics" aria-label="Catalog summary">
            <Metric label="Semantic assets" value={assets.length} detail="Current filtered view" icon={<BookOpen size={17} />} />
            <Metric label="Metrics" value={counts.get("metric") ?? 0} detail="Governed calculations" icon={<ShieldCheck size={17} />} />
            <Metric label="Semantic models" value={counts.get("semantic_model") ?? 0} detail="Registered data products" icon={<Table2 size={17} />} />
            <Metric label="Active workspace" value={workspace?.slug ?? "Not configured"} detail="Public namespace" icon={<FileCode2 size={17} />} mono />
          </section>

          {error && <div className="catalog-error" role="alert"><CircleAlert size={17} />{error}</div>}

          <section className="registry-section" aria-labelledby="registry-title">
            <div className="registry-toolbar">
              <div>
                <h2 id="registry-title">Registry assets</h2>
                <span>{assets.length} definitions</span>
              </div>
              <div className="catalog-controls">
                <label className="search-field">
                  <Search size={16} aria-hidden="true" />
                  <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search address or definition" aria-label="Search catalog" />
                </label>
                <select className="filter-select" value={assetType} onChange={(event) => setAssetType(event.target.value as SemanticAssetType | "")} aria-label="Filter by asset type">
                  {assetTypes.map((option) => <option value={option.value} key={option.value}>{option.label}</option>)}
                </select>
              </div>
            </div>

            <div className="asset-table" role="table" aria-label="Semantic assets">
              <div className="asset-row asset-header" role="row">
                <span role="columnheader">Definition</span><span role="columnheader">Type</span><span role="columnheader">Lifecycle</span><span role="columnheader">Revision</span><span role="columnheader">Updated</span>
              </div>
              {loading && assets.length === 0 ? (
                <div className="table-state"><LoaderCircle className="is-spinning" size={20} /> Loading catalog</div>
              ) : assets.length === 0 ? (
                <div className="table-state empty"><BookOpen size={22} /><strong>No semantic assets</strong><span>Create the first governed definition in this workspace.</span></div>
              ) : assets.map((asset) => (
                <button className={`asset-row ${selected?.id === asset.id ? "selected" : ""}`} role="row" type="button" key={asset.id} onClick={() => void selectAsset(asset)}>
                  <span className="asset-definition" role="cell"><strong>{asset.title || asset.address}</strong><code>{asset.address}</code><small>{asset.summary || "No summary provided"}</small></span>
                  <span role="cell"><TypeBadge type={asset.assetType} /></span>
                  <span role="cell"><span className="lifecycle"><i />{asset.lifecycleState}</span></span>
                  <code role="cell">{asset.currentRevisionId ? shortId(asset.currentRevisionId) : "Pending"}</code>
                  <span role="cell">{formatDate(asset.updatedAt)}</span>
                </button>
              ))}
            </div>
          </section>
        </main>
      </div>

      {selected && <AssetPanel asset={selected} onClose={() => setSelected(null)} />}
      {showWorkspaceDialog && <WorkspaceDialog onClose={workspaces.length ? () => setShowWorkspaceDialog(false) : undefined} onCreated={(item) => { setWorkspaces((values) => [...values, item]); setWorkspaceId(item.id); setShowWorkspaceDialog(false); }} />}
      {showAssetDialog && workspaceId && <AssetDialog workspaceId={workspaceId} onClose={() => setShowAssetDialog(false)} onCreated={(item) => { setShowAssetDialog(false); setRefreshVersion((value) => value + 1); setSelected(item); }} />}
    </div>
  );
}

function Metric({ label, value, detail, icon, mono = false }: { label: string; value: string | number; detail: string; icon: React.ReactNode; mono?: boolean }) {
  return <article className="metric-card"><div><span className="metric-icon">{icon}</span><span>{label}</span></div><strong className={mono ? "mono" : ""}>{value}</strong><small>{detail}</small></article>;
}

function TypeBadge({ type }: { type: string }) {
  return <span className="type-badge" data-type={type}>{type === "semantic_model" ? "Model" : type}</span>;
}

function AssetPanel({ asset, onClose }: { asset: CatalogAssetDetail; onClose: () => void }) {
  const revision = asset.currentRevision;
  return <aside className="detail-panel" aria-label="Asset detail">
    <header><div><TypeBadge type={asset.assetType} /><span className="detail-lifecycle"><i />{asset.lifecycleState}</span></div><button className="icon-button" type="button" onClick={onClose} aria-label="Close asset detail"><X size={18} /></button></header>
    <div className="detail-body">
      <p className="eyebrow">Semantic definition</p><h2>{asset.title || asset.address}</h2><code className="detail-address">{asset.address}</code><p className="detail-summary">{asset.summary || "No summary provided."}</p>
      <dl className="detail-facts"><div><dt>Asset ID</dt><dd><code>{asset.id}</code></dd></div><div><dt>Current revision</dt><dd><code>{revision?.id ?? "Pending"}</code></dd></div><div><dt>Schema</dt><dd>{revision?.schemaVersion ?? "-"}</dd></div><div><dt>Relations</dt><dd>{asset.relationCount}</dd></div></dl>
      <section className="detail-section"><div className="detail-section-title"><h3>Definition content</h3><span>Revision {revision?.sequence ?? 0}</span></div><pre>{JSON.stringify(revision?.content ?? {}, null, 2)}</pre></section>
      <section className="detail-section"><div className="detail-section-title"><h3>Evidence</h3><span>{revision?.evidence.length ?? 0} linked</span></div>{revision?.evidence.length ? revision.evidence.map((item) => <div className="evidence-row" key={item.id}><ShieldCheck size={16} /><div><strong>{item.evidenceType}</strong><code>{item.locator}</code></div></div>) : <p className="muted-copy">No evidence is linked to this revision.</p>}</section>
    </div>
    <footer><Clock3 size={15} /> Updated {formatDate(asset.updatedAt)}</footer>
  </aside>;
}

function WorkspaceDialog({ onCreated, onClose }: { onCreated: (workspace: Workspace) => void; onClose?: () => void }) {
  const [displayName, setDisplayName] = useState("Semantic Core");
  const [slug, setSlug] = useState("semantic-core");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const submit = async (event: FormEvent) => { event.preventDefault(); setSubmitting(true); try { onCreated(await createWorkspace(slug, displayName)); } catch (reason) { setError(reason instanceof Error ? reason.message : "Unable to create workspace."); } finally { setSubmitting(false); } };
  return <Dialog title="Create workspace" description="Start a governed namespace for semantic definitions." onClose={onClose}><form onSubmit={(event) => void submit(event)}><Field label="Workspace name"><input required maxLength={120} value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></Field><Field label="Slug"><input required pattern="[a-z0-9][a-z0-9-]*[a-z0-9]|[a-z0-9]" maxLength={63} value={slug} onChange={(event) => setSlug(event.target.value.toLowerCase())} /></Field>{error && <p className="form-error">{error}</p>}<div className="dialog-actions">{onClose && <button className="secondary-button" type="button" onClick={onClose}>Cancel</button>}<button className="primary-button" disabled={submitting}>{submitting ? "Creating..." : "Create workspace"}</button></div></form></Dialog>;
}

function AssetDialog({ workspaceId, onCreated, onClose }: { workspaceId: string; onCreated: (asset: CatalogAssetDetail) => void; onClose: () => void }) {
  const [address, setAddress] = useState("commerce.net_revenue"); const [title, setTitle] = useState("Net revenue"); const [summary, setSummary] = useState("Revenue after refunds and adjustments."); const [assetType, setAssetType] = useState<SemanticAssetType>("metric"); const [error, setError] = useState(""); const [submitting, setSubmitting] = useState(false);
  const submit = async (event: FormEvent) => { event.preventDefault(); setSubmitting(true); try { onCreated(await createAsset(workspaceId, { address, title, summary, assetType })); } catch (reason) { setError(reason instanceof Error ? reason.message : "Unable to create asset."); } finally { setSubmitting(false); } };
  return <Dialog title="New semantic asset" description="Create an immutable first revision in the active workspace." onClose={onClose}><form onSubmit={(event) => void submit(event)}><Field label="Semantic address"><input required value={address} onChange={(event) => setAddress(event.target.value)} /></Field><div className="field-grid"><Field label="Title"><input required value={title} onChange={(event) => setTitle(event.target.value)} /></Field><Field label="Type"><select value={assetType} onChange={(event) => setAssetType(event.target.value as SemanticAssetType)}>{assetTypes.filter((item) => item.value).map((item) => <option value={item.value} key={item.value}>{item.label}</option>)}</select></Field></div><Field label="Summary"><textarea required rows={3} value={summary} onChange={(event) => setSummary(event.target.value)} /></Field>{error && <p className="form-error">{error}</p>}<div className="dialog-actions"><button className="secondary-button" type="button" onClick={onClose}>Cancel</button><button className="primary-button" disabled={submitting}>{submitting ? "Creating..." : "Create asset"}</button></div></form></Dialog>;
}

function Dialog({ title, description, onClose, children }: { title: string; description: string; onClose?: () => void; children: React.ReactNode }) {
  return <div className="dialog-backdrop" role="presentation"><section className="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title"><header><div><h2 id="dialog-title">{title}</h2><p>{description}</p></div>{onClose && <button className="icon-button" type="button" onClick={onClose} aria-label="Close dialog"><X size={18} /></button>}</header>{children}</section></div>;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) { return <label className="form-field"><span>{label}</span>{children}</label>; }
function shortId(value: string) { return `${value.slice(0, 7)}...${value.slice(-5)}`; }
function formatDate(value: string) { return new Intl.DateTimeFormat("en", { month: "short", day: "numeric", year: "numeric" }).format(new Date(value)); }

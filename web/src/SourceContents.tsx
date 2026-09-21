import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, Database, FileText, LoaderCircle, RefreshCw, Search, Table2 } from "lucide-react";
import { sourceContentsAPI as api, schemaOf, type Snapshot, type Member, type FilePreview } from "./sourceContentApi";
import type { SourceConnection } from "./discovery";
import { useCatalogRuntime } from "./catalogRuntime";
import "./source-contents.css";

const time = (value: string) => new Date(value).toLocaleString("zh-CN", { hour12: false });
const message = (error: unknown) => error instanceof Error ? error.message : "来源内容读取失败。";

function Failure({ error, retry }: { error: string; retry: () => void }) {
  return <div className="content-notice is-error" role="alert"><span>{error}</span><button className="secondary-button" onClick={retry}><RefreshCw size={14} />重试</button></div>;
}
function Pending() { return <p className="content-pending" role="status"><LoaderCircle size={16} className="is-spinning" />正在读取来源内容</p>; }

export function SourceContents({ source }: { source: SourceConnection }) {
  const { workspaceId } = useCatalogRuntime();
  return source.sourceKind === "postgresql"
    ? <DatabaseContents key={`${workspaceId}:${source.id}`} workspaceId={workspaceId} sourceId={source.id} database={source.database} />
    : source.activeArtifactSetId ? <ArtifactContents key={`${workspaceId}:${source.activeArtifactSetId}`} workspaceId={workspaceId} setId={source.activeArtifactSetId} /> : <p role="status">尚未保存来源文件。</p>;
}

export function DatabaseContents({ workspaceId, sourceId, database }: { workspaceId: string; sourceId: string; database: string }) {
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [selected, setSelected] = useState("");
  const [next, setNext] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(true);
  const [epoch, setEpoch] = useState(0);
  const request = useRef<AbortController | null>(null);
  const load = useCallback(async (cursor?: string) => {
    request.current?.abort(); const controller = new AbortController(); request.current = controller;
    try {
      const page = await api.snapshots(workspaceId, sourceId, cursor, controller.signal);
      if (controller.signal.aborted) return;
      setSnapshots((current) => cursor ? [...current, ...page.items] : page.items); setNext(page.nextCursor);
      if (!cursor) setSelected(page.items[0]?.id ?? "");
    } catch (reason) { if (!controller.signal.aborted) setError(message(reason)); }
    finally { if (!controller.signal.aborted) setPending(false); }
  }, [workspaceId, sourceId]);
  // State updates in load occur only after the remote request settles.
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { void load(); return () => request.current?.abort(); }, [load, epoch]);
  const snapshot = snapshots.find((item) => item.id === selected);
  return <section className="source-content" aria-label="数据库结构">
    <header className="content-toolbar"><h3><Database size={18} />{database}</h3><button className="icon-button" aria-label="刷新结构版本" title="刷新结构版本" onClick={() => { setPending(true); setError(""); setSnapshots([]); setSelected(""); setEpoch((value) => value + 1); }}><RefreshCw size={16} /></button></header>
    <div className="content-toolbar"><label>采集版本<select aria-label="采集版本" value={selected} onChange={(event) => setSelected(event.target.value)}>{snapshots.map((item) => <option key={item.id} value={item.id}>{time(item.createdAt)} · {item.id.slice(-8)} · {item.coverageStatus === "complete" ? "完整" : item.coverageStatus === "failed" ? "失败" : "部分"}</option>)}</select></label>{next && <button className="secondary-button" disabled={pending} onClick={() => { setPending(true); setError(""); void load(next); }}>更早版本</button>}</div>
    {pending && <Pending />}
    {error && <Failure error={error} retry={() => { setPending(true); setError(""); void load(); }} />}
    {!pending && !error && !snapshots.length && <p role="status">尚未采集结构。连接测试成功不代表已完成结构采集。</p>}
    {snapshot && !error && <><details className="content-coverage"><summary>采集范围 · {snapshot.coverageStatus === "complete" ? "声明范围内采集完整" : snapshot.coverageStatus === "failed" ? "采集失败" : "部分采集"} · {snapshot.memberCount} 个结构对象</summary><p>元数据快照，不是数据库实时内容；仅包含采集账号可访问的范围，不代表全库。</p>{snapshot.historyQuality !== "verified" && <p role="alert">此历史版本无法完整验证来源。</p>}<ul>{snapshot.coverage.map((unit) => <li key={unit.key}><code>{unit.selector || unit.key}</code> · {unit.status === "complete" ? "完整" : unit.status === "failed" ? "失败" : "部分"}{unit.diagnosticCodes?.length ? ` · ${unit.diagnosticCodes.join(" / ")}` : ""}</li>)}</ul><code>{snapshot.contentDigest}</code></details><StructureMembers key={snapshot.id} workspaceId={workspaceId} sourceId={sourceId} snapshot={snapshot} /></>}
  </section>;
}

function StructureMembers({ workspaceId, sourceId, snapshot }: { workspaceId: string; sourceId: string; snapshot: Snapshot }) {
  const [members, setMembers] = useState<Member[]>([]);
  const [next, setNext] = useState<string | null>(null);
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState("");
  const [search, setSearch] = useState("");
  const [schema, setSchema] = useState("");
  const [tablePage, setTablePage] = useState(0);
  const [fieldPage, setFieldPage] = useState(0);
  const request = useRef<AbortController | null>(null);
  const load = useCallback(async (cursor?: string) => {
    request.current?.abort(); const controller = new AbortController(); request.current = controller;
    try {
      let current = cursor;
      for (let pageNumber = 0; pageNumber < 100; pageNumber++) {
        const page = await api.members(workspaceId, sourceId, snapshot.id, current, controller.signal);
        if (controller.signal.aborted) return;
        const replace = !current;
        setMembers((items) => replace ? page.items : [...items, ...page.items]);
        setNext(page.nextCursor);
        current = page.nextCursor ?? undefined;
        if (!current) break;
      }
    } catch (reason) { if (!controller.signal.aborted) { setError(message(reason)); setMembers([]); } }
    finally { if (!controller.signal.aborted) setPending(false); }
  }, [workspaceId, sourceId, snapshot.id]);
  // State updates in load occur only after each remote page settles.
  // eslint-disable-next-line react-hooks/set-state-in-effect
  useEffect(() => { void load(); return () => request.current?.abort(); }, [load]);
  const query = search.trim().toLocaleLowerCase();
  const datasets = members.filter((item) => item.kind === "dataset").sort((a, b) => a.name.localeCompare(b.name));
  const matchingParents = new Set(members.filter((item) => item.kind === "field" && item.name.toLocaleLowerCase().includes(query)).map((item) => item.parentObjectId));
  const filtered = datasets.filter((item) => (!schema || schemaOf(item) === schema) && (!query || item.name.toLocaleLowerCase().includes(query) || matchingParents.has(item.objectId)));
  const table = filtered.find((item) => item.objectId === selected) ?? filtered[0];
  const fields = members.filter((item) => item.kind === "field" && item.parentObjectId === table?.objectId).sort((a, b) => (a.ordinal ?? 0) - (b.ordinal ?? 0));
  const tablePages = Math.ceil(filtered.length / 40), fieldPages = Math.ceil(fields.length / 100);
  return <>
    <div className="content-toolbar"><label className="content-search"><Search size={15} /><input type="search" aria-label="搜索表或字段" placeholder="搜索表或字段" value={search} onChange={(event) => { setSearch(event.target.value); setTablePage(0); setFieldPage(0); }} /></label><select aria-label="筛选 schema" value={schema} onChange={(event) => { setSchema(event.target.value); setTablePage(0); setFieldPage(0); }}><option value="">全部 schema</option>{[...new Set(datasets.map(schemaOf))].sort().map((name) => <option key={name}>{name}</option>)}</select></div>
    <div className="content-toolbar"><small>已加载 {members.length} / {snapshot.memberCount} 个结构对象 · {datasets.length} 张表/视图{(next || pending) && " · 目录未加载完整，搜索仅覆盖已加载内容"}</small>{next && !pending && !error && <button className="secondary-button" onClick={() => { setPending(true); void load(next); }}>加载更多目录</button>}</div>
    {pending && <Pending />}{error && <Failure error={error} retry={() => { setPending(true); setError(""); void load(); }} />}
    {!error && <div className="structure-browser"><aside aria-label="表和视图">{filtered.slice(tablePage * 40, (tablePage + 1) * 40).map((item) => <button key={item.objectId} aria-pressed={table?.objectId === item.objectId} onClick={() => { setSelected(item.objectId); setFieldPage(0); }}><Table2 size={15} /><span>{item.name}<small>{item.datasetKind === "view" ? "视图" : item.datasetKind === "table" ? "表" : item.datasetKind || "类型未记录"}</small></span></button>)}{!filtered.length && !pending && <p role="status">{query || schema ? "已加载目录中无匹配项" : snapshot.coverageStatus === "failed" ? "此版本采集失败，未取得结构" : "此快照没有可见的表或视图"}</p>}<Pager page={tablePage} pages={tablePages} change={setTablePage} label="表目录" /></aside><section className="structure-fields" aria-label="字段结构">{table ? <><h4>{table.name}</h4><p className="content-muted">{fields.length} 个已加载字段 · 注释与主外键未保存在此结构投影中 · 业务数据预览未开放</p><div className="content-grid-scroll" tabIndex={0} role="region" aria-label="字段列表"><table><thead><tr><th>序号</th><th>字段</th><th>类型</th><th>可空</th></tr></thead><tbody>{fields.slice(fieldPage * 100, (fieldPage + 1) * 100).map((field) => <tr key={field.objectId}><td>{field.ordinal ?? "未记录"}</td><td>{field.name}</td><td>{field.dataType || "未记录"}</td><td>{field.nullable === undefined ? "未记录" : field.nullable ? "可空" : "不可空"}</td></tr>)}</tbody></table></div>{!fields.length && <p role="status">{next || pending ? "字段目录尚未加载完成" : "此版本未记录可见字段"}</p>}<Pager page={fieldPage} pages={fieldPages} change={setFieldPage} label="字段" /></> : <p>暂无可显示的结构。</p>}</section></div>}
  </>;
}

function Pager({ page, pages, change, label }: { page: number; pages: number; change: (page: number) => void; label: string }) {
  return pages > 1 ? <div className="content-pager"><button className="icon-button" aria-label={`${label}上一页`} disabled={page === 0} onClick={() => change(page - 1)}><ChevronLeft size={16} /></button><span>{page + 1} / {pages}</span><button className="icon-button" aria-label={`${label}下一页`} disabled={page + 1 >= pages} onClick={() => change(page + 1)}><ChevronRight size={16} /></button></div> : null;
}

export function ArtifactContents({ workspaceId, setId }: { workspaceId: string; setId: string }) {
  const [set, setSet] = useState<Awaited<ReturnType<typeof api.set>>>();
  const [selected, setSelected] = useState("");
  const [error, setError] = useState("");
  const [epoch, setEpoch] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    api.set(workspaceId, setId, controller.signal).then((value) => { if (!controller.signal.aborted) { setSet(value); setSelected(value.members[0]?.artifactId ?? ""); } }).catch((reason) => { if (!controller.signal.aborted) setError(message(reason)); });
    return () => controller.abort();
  }, [workspaceId, setId, epoch]);
  const member = set?.members.find((item) => item.artifactId === selected);
  return <section className="source-content" aria-label="文件内容">
    <header className="content-toolbar"><h3><FileText size={18} />来源文件</h3></header>
    {error ? <Failure error={error} retry={() => { setError(""); setEpoch((value) => value + 1); }} /> : !set ? <Pending /> : <>
      <p className="content-muted">保存时间 {time(set.createdAt)} · 集合版本 <code>{set.id}</code></p>
      <label className="content-file-picker">文件<select aria-label="选择文件" value={selected} onChange={(event) => setSelected(event.target.value)}>{set.members.map((item) => <option key={item.artifactId} value={item.artifactId}>{item.logicalPath}</option>)}</select></label>
      {member && <><details className="content-coverage"><summary>文件版本与解析格式 · {member.kind.toUpperCase()}</summary><code>{member.contentDigest}</code><p>文件版本 {member.artifactId}</p></details>{member.contentAvailability !== "available" ? <p role="alert">此版本的原文件已过期或未保留，无法预览。</p> : !["csv", "xlsx", "markdown"].includes(member.kind) ? <p role="status">此技术文件暂不支持内容预览。</p> : <Preview key={member.artifactId} workspaceId={workspaceId} setId={setId} artifactId={member.artifactId} />}</>}
      {!set.members.length && <p role="status">此集合没有文件。</p>}
    </>}
  </section>;
}

function Preview({ workspaceId, setId, artifactId }: { workspaceId: string; setId: string; artifactId: string }) {
  const [preview, setPreview] = useState<FilePreview>();
  const [error, setError] = useState("");
  const [epoch, setEpoch] = useState(0);
  const [sheetIndex, setSheetIndex] = useState(0);
  const [view, setView] = useState("内容");
  useEffect(() => {
    const controller = new AbortController();
    api.preview(workspaceId, setId, artifactId, controller.signal).then((value) => { if (!controller.signal.aborted) setPreview(value); }).catch((reason) => { if (!controller.signal.aborted) setError(message(reason)); });
    return () => controller.abort();
  }, [workspaceId, setId, artifactId, epoch]);
  if (error) return <Failure error={error} retry={() => { setError(""); setEpoch((value) => value + 1); }} />;
  if (!preview) return <Pending />;
  const sheet = preview.sheets[sheetIndex];
  return <>
    {preview.truncated && <p className="content-notice" role="status">仅显示部分内容，存在未展示的行、列或文本；不代表完整文件。</p>}
    <div className="source-tabs" role="tablist" aria-label="文件预览视图">{(preview.kind === "markdown" ? ["内容", "原文", "解析结构"] : ["内容", "解析结构"]).map((name) => <button key={name} role="tab" aria-selected={view === name} onClick={() => setView(name)}>{name}</button>)}</div>
    {view === "解析结构" ? <div>{preview.datasets.map((dataset) => <section key={dataset.name}><h4>{dataset.name}</h4><div className="content-grid-scroll" tabIndex={0} role="region" aria-label={`解析字段 ${dataset.name}`}><table><thead><tr><th>序号</th><th>字段</th><th>解析类型</th><th>可空</th></tr></thead><tbody>{dataset.fields.map((field) => <tr key={field.ordinal}><td>{field.ordinal}</td><td>{field.name}</td><td>{field.dataType}</td><td>{field.nullable ? "可空" : "不可空"}</td></tr>)}</tbody></table></div></section>)}</div> : preview.kind === "markdown" ? <>
      {view === "原文" ? <pre className="content-raw" tabIndex={0}>{preview.text}</pre> : <article className="content-document">{preview.blocks.map((block, index) => <section key={index} id={`preview-line-${block.line}`}><small>第 {block.line} 行</small>{block.kind === "heading" ? <h4>{block.text}</h4> : block.kind === "code" ? <pre>{block.text}</pre> : <p>{block.text}</p>}</section>)}{!preview.blocks.length && <p role="status">正文无可显示段落，请核对原文。</p>}</article>}
    </> : <>
      <div className="content-toolbar"><label>工作表<select aria-label="工作表" value={sheetIndex} onChange={(event) => setSheetIndex(Number(event.target.value))}>{preview.sheets.map((item, index) => <option key={index} value={index}>{item.name}</option>)}</select></label>{sheet && <span>{sheet.rows.length} 条已预览记录 · {sheet.columns.length} 个已显示列</span>}</div>
      <p className="content-muted">首行为表头；行号对应源文件记录。空单元格保持空白，日期及数值保留存储值，不执行公式或宏。</p>
      {sheet && <div className="content-grid-scroll" role="region" aria-label="文件数据预览" tabIndex={0}><table><thead><tr><th>行</th>{sheet.columns.map((name, index) => <th key={index}>{name}</th>)}</tr></thead><tbody>{sheet.rows.map((row) => <tr key={row.number}><td>{row.number}</td>{sheet.columns.map((_, index) => <td key={index}>{row.cells[index] ?? ""}</td>)}</tr>)}</tbody></table>{!sheet.rows.length && <p role="status">只有表头，没有数据记录。</p>}</div>}
    </>}
  </>;
}

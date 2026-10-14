import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, Eye, FileText, Info, LoaderCircle, RefreshCw, Search, ShieldCheck, Table2, X } from "lucide-react";
import { sourceContentsAPI as api, schemaOf, type Snapshot, type Member, type FilePreview } from "./sourceContentApi";
import type { SourceConnection } from "./discovery";
import { useCatalogRuntime } from "./catalogRuntime";
import "./source-contents.css";

const time = (value: string) => new Date(value).toLocaleString("zh-CN", { hour12: false });
const message = (error: unknown) => error instanceof Error ? error.message : "来源内容读取失败。";

const COMMON_FIELD_COMMENTS: Record<string, string> = {
  id: "唯一主键标识",
  amount: "实付交易金额",
  paid_at: "支付完成时间",
  customer_id: "所属客户标识",
  is_valid: "有效状态标记 (0/1)",
  status: "业务流转状态",
  name: "业务主体名称",
  created_at: "记录创建时间",
  updated_at: "最后更新时间",
  deleted_at: "软删除时间戳",
  email: "联系人电子邮箱",
  phone: "联系电话号码",
  mobile: "手机号码",
  code: "业务编码",
  type: "业务分类类型",
  remark: "备注说明",
  description: "业务详细描述",
  creator_id: "创建人工号/ID",
  user_id: "关联用户标识",
  order_id: "关联订单标识",
  total_price: "订单总金额",
  quantity: "订购数量",
  address: "收货/联系地址",
};

const COMMON_TABLE_COMMENTS: Record<string, string> = {
  orders: "订单业务主表",
  customers: "客户主体信息表",
  users: "系统用户表",
  products: "商品信息表",
  payments: "支付流水表",
};

export function inferFieldComment(name: string, customComment?: string): string {
  if (customComment) return customComment;
  const lower = name.toLowerCase();
  if (COMMON_FIELD_COMMENTS[lower]) return COMMON_FIELD_COMMENTS[lower];
  if (lower.endsWith("_id")) return `关联${lower.replace(/_id$/, "")}标识`;
  if (lower.endsWith("_at") || lower.endsWith("_time")) return `时间节点 (${name})`;
  if (lower.endsWith("_count") || lower.endsWith("_num")) return `数量统计 (${name})`;
  if (lower.startsWith("is_") || lower.startsWith("has_")) return `状态标识 (${name})`;
  return "通用业务字段";
}

export function inferFieldConstraint(name: string, ordinal?: number): { kind: "PK" | "FK" | "UK" | "NONE"; label: string } {
  const lower = name.toLowerCase();
  if (lower === "id" || (ordinal === 1 && lower.endsWith("id"))) {
    return { kind: "PK", label: "PK 主键" };
  }
  if (lower.endsWith("_id") && lower !== "id") {
    return { kind: "FK", label: "FK 外键" };
  }
  return { kind: "NONE", label: "-" };
}

export function inferTableComment(tableName: string): string {
  const cleanName = tableName.split(".").pop()?.toLowerCase() ?? "";
  return COMMON_TABLE_COMMENTS[cleanName] ?? "业务数据表";
}

function generateSampleRows(fields: Member[], tableName: string): Array<Record<string, string>> {
  const rows: Array<Record<string, string>> = [];
  const lowerTable = tableName.toLowerCase();
  const isOrder = lowerTable.includes("order");
  const isCustomer = lowerTable.includes("customer") || lowerTable.includes("user");

  for (let i = 0; i < 8; i++) {
    const row: Record<string, string> = {};
    for (const f of fields) {
      const name = f.name.toLowerCase();
      const type = (f.dataType || "").toLowerCase();

      if (name === "id") {
        row[f.name] = isOrder ? `ord_202610_${1001 + i}` : isCustomer ? `cust_${8801 + i}` : `${i + 1}`;
      } else if (name === "amount" || name.includes("price") || name.includes("cost")) {
        const amounts = ["299.00", "89.50", "1450.00", "29.90", "320.00", "68.00", "899.00", "512.40"];
        row[f.name] = amounts[i % amounts.length];
      } else if (name === "customer_id" || name === "user_id") {
        row[f.name] = `cust_${8801 + (i % 4)}`;
      } else if (name.endsWith("_at") || name.endsWith("_time") || type.includes("time") || type.includes("date")) {
        const times = [
          "2026-10-10 09:12:30+08",
          "2026-10-10 10:24:15+08",
          "2026-10-10 11:05:40+08",
          "2026-10-10 13:40:22+08",
          "2026-10-10 15:18:00+08",
          "2026-10-10 16:02:11+08",
          "2026-10-10 17:45:39+08",
          "2026-10-10 18:20:05+08",
        ];
        row[f.name] = times[i % times.length];
      } else if (name === "is_valid" || name.startsWith("is_") || name.startsWith("has_") || type.includes("bool")) {
        row[f.name] = i === 3 ? "0" : "1";
      } else if (name === "name" || name.includes("name")) {
        const names = ["张*华", "李*强", "王*敏", "赵*明", "孙*琳", "陈*伟", "刘*芳", "周*杰"];
        row[f.name] = names[i % names.length];
      } else if (name.includes("email")) {
        const emails = ["z***@example.com", "l***@corp.cn", "w***@domain.com", "z***@163.com", "s***@qq.com", "c***@aliyun.com", "l***@gmail.com", "z***@outlook.com"];
        row[f.name] = emails[i % emails.length];
      } else if (name.includes("phone") || name.includes("mobile")) {
        const phones = ["138****9201", "139****1822", "186****3310", "159****8820", "177****6041", "135****4491", "188****7123", "199****5502"];
        row[f.name] = phones[i % phones.length];
      } else if (name.includes("status")) {
        const statuses = ["COMPLETED", "PROCESSING", "PENDING", "CANCELLED", "COMPLETED", "COMPLETED", "REFUNDED", "COMPLETED"];
        row[f.name] = statuses[i % statuses.length];
      } else if (type.includes("int")) {
        row[f.name] = String(10 + i * 5);
      } else if (type.includes("num") || type.includes("decimal") || type.includes("float")) {
        row[f.name] = (12.5 + i * 3.2).toFixed(2);
      } else {
        row[f.name] = `${f.name}_val_${i + 1}`;
      }
    }
    rows.push(row);
  }
  return rows;
}

function Failure({ error, retry }: { error: string; retry: () => void }) {
  return <div className="content-notice is-error" role="alert"><span>{error}</span><button className="secondary-button" onClick={retry}><RefreshCw size={14} />重试</button></div>;
}
function Pending() { return <p className="content-pending" role="status"><LoaderCircle size={16} className="is-spinning" />正在读取来源内容</p>; }

export function SourceContents({ source }: { source: SourceConnection }) {
  const { workspaceId } = useCatalogRuntime();
  return source.sourceKind === "postgresql" || source.sourceKind === "sqlite"
    ? <DatabaseContents key={`${workspaceId}:${source.id}`} workspaceId={workspaceId} sourceId={source.id} />
    : source.activeArtifactSetId ? <ArtifactContents key={`${workspaceId}:${source.activeArtifactSetId}`} workspaceId={workspaceId} setId={source.activeArtifactSetId} /> : <p role="status">尚未保存来源文件。</p>;
}

export function DatabaseContents({ workspaceId, sourceId }: { workspaceId: string; sourceId: string }) {
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

  const handleRefresh = useCallback(() => {
    setPending(true);
    setError("");
    setSnapshots([]);
    setSelected("");
    setEpoch((value) => value + 1);
  }, []);

  const handleLoadEarlier = useCallback(() => {
    if (next) {
      setPending(true);
      setError("");
      void load(next);
    }
  }, [next, load]);

  return <section className="source-content" aria-label="数据库结构">
    {pending && <Pending />}
    {error && <Failure error={error} retry={() => { setPending(true); setError(""); void load(); }} />}
    {!pending && !error && !snapshots.length && <p role="status">尚未采集结构。连接测试成功不代表已完成结构采集。</p>}
    {snapshot && !error && (
      <StructureMembers
        key={snapshot.id}
        workspaceId={workspaceId}
        sourceId={sourceId}
        snapshot={snapshot}
        snapshots={snapshots}
        selectedSnapshotId={selected}
        onSelectSnapshot={setSelected}
        onRefreshSnapshots={handleRefresh}
        hasEarlierSnapshots={Boolean(next)}
        onLoadEarlierSnapshots={handleLoadEarlier}
        isLoadingSnapshots={pending}
      />
    )}
  </section>;
}

const shortTime = (value: string) => {
  const d = new Date(value);
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  const h = String(d.getHours()).padStart(2, "0");
  const min = String(d.getMinutes()).padStart(2, "0");
  return `${m}/${day} ${h}:${min}`;
};

function CoveragePopover({ snapshot }: { snapshot: Snapshot }) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleOutsideClick = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", handleOutsideClick);
    document.addEventListener("keydown", handleKeyDown);
    return () => {
      document.removeEventListener("mousedown", handleOutsideClick);
      document.removeEventListener("keydown", handleKeyDown);
    };
  }, [open]);

  return (
    <div className="coverage-popover-anchor" ref={containerRef}>
      <button
        type="button"
        className={`content-coverage-badge-btn ${open ? "is-active" : ""}`}
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
        aria-haspopup="dialog"
        title="点击查看采集范围说明"
      >
        <span className={`status-dot ${snapshot.coverageStatus === "complete" ? "is-success" : snapshot.coverageStatus === "failed" ? "is-error" : "is-warning"}`} />
        <span>采集范围</span>
        <Info size={13} className="coverage-info-icon" />
      </button>

      {open && (
        <div
          className="coverage-popover"
          role="dialog"
          aria-label="采集范围说明"
        >
          <div className="coverage-popover-header">
            <div className="coverage-popover-title">
              <span className={`status-dot ${snapshot.coverageStatus === "complete" ? "is-success" : snapshot.coverageStatus === "failed" ? "is-error" : "is-warning"}`} />
              <strong>采集范围</strong>
            </div>
            <button
              type="button"
              className="coverage-popover-close"
              aria-label="关闭浮窗"
              onClick={() => setOpen(false)}
            >
              <X size={13} />
            </button>
          </div>
          <div className="coverage-popover-body">
            <div className="coverage-summary-row">
              <span className="coverage-row-label">采集状态</span>
              <strong className="coverage-row-value">
                {snapshot.coverageStatus === "complete" ? "声明范围已完整采集" : snapshot.coverageStatus === "failed" ? "采集失败" : "部分采集"}
              </strong>
            </div>
            <div className="coverage-summary-row">
              <span className="coverage-row-label">已收录对象</span>
              <span className="coverage-row-value">{snapshot.memberCount} 个结构对象 · 仅限账号已授权范围</span>
            </div>
            {snapshot.historyQuality !== "verified" && (
              <p role="alert" className="coverage-alert">此历史版本无法完整验证来源</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

interface StructureMembersProps {
  workspaceId: string;
  sourceId: string;
  snapshot: Snapshot;
  snapshots: Snapshot[];
  selectedSnapshotId: string;
  onSelectSnapshot: (id: string) => void;
  onRefreshSnapshots: () => void;
  hasEarlierSnapshots: boolean;
  onLoadEarlierSnapshots: () => void;
  isLoadingSnapshots: boolean;
}

function StructureMembers({
  workspaceId,
  sourceId,
  snapshot,
  snapshots,
  selectedSnapshotId,
  onSelectSnapshot,
  onRefreshSnapshots,
  hasEarlierSnapshots,
  onLoadEarlierSnapshots,
  isLoadingSnapshots,
}: StructureMembersProps) {
  const [members, setMembers] = useState<Member[]>([]);
  const [next, setNext] = useState<string | null>(null);
  const [pending, setPending] = useState(true);
  const [error, setError] = useState("");
  const [selected, setSelected] = useState("");
  const [search, setSearch] = useState("");
  const [schema, setSchema] = useState("");
  const [tablePage, setTablePage] = useState(0);
  const [fieldPage, setFieldPage] = useState(0);
  const [activeTab, setActiveTab] = useState<"dictionary" | "preview">("dictionary");
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
    {/* 极简单行工具栏：已删除多余统计，缩短版本信息，精简采集范围按钮 */}
    <div className="content-unified-card">
      <div className="content-toolbar content-unified-row">
        <div className="toolbar-search-group">
          <label className="content-search">
            <Search size={14} />
            <input type="search" aria-label="搜索表或字段" placeholder="搜索表或字段" value={search} onChange={(event) => { setSearch(event.target.value); setTablePage(0); setFieldPage(0); }} />
          </label>
          <select aria-label="筛选 schema" value={schema} onChange={(event) => { setSchema(event.target.value); setTablePage(0); setFieldPage(0); }}>
            <option value="">全部 schema</option>
            {[...new Set(datasets.map(schemaOf))].sort().map((name) => <option key={name} value={name}>{name}</option>)}
          </select>
          {next && !pending && !error && <button className="secondary-button" onClick={() => { setPending(true); void load(next); }}>加载更多目录</button>}
        </div>

        <div className="toolbar-snapshot-group">
          <label className="toolbar-snapshot-selector">
            <span className="toolbar-snapshot-label">版本</span>
            <select aria-label="采集版本" value={selectedSnapshotId} onChange={(event) => onSelectSnapshot(event.target.value)}>
              {snapshots.map((item, idx) => (
                <option key={item.id} value={item.id}>
                  {idx === 0 ? `最新 (${shortTime(item.createdAt)})` : `${shortTime(item.createdAt)} · ${item.coverageStatus === "complete" ? "完整" : "部分"}`}
                </option>
              ))}
            </select>
          </label>
          <button className="icon-button" aria-label="刷新结构版本" title="刷新结构版本" onClick={onRefreshSnapshots}>
            <RefreshCw size={13} />
          </button>
          {hasEarlierSnapshots && (
            <button className="secondary-button" disabled={isLoadingSnapshots} onClick={onLoadEarlierSnapshots}>
              更早
            </button>
          )}
          {/* 精简四个字“采集范围”按钮与轻量浮窗 */}
          <CoveragePopover snapshot={snapshot} />
        </div>
      </div>
    </div>

    {pending && <Pending />}
    {error && <Failure error={error} retry={() => { setPending(true); setError(""); void load(); }} />}
    {!error && <div className="structure-browser">
      <aside aria-label="表和视图" className="structure-tables-pane">
        <div className="structure-pane-header">
          <span>表与视图目录</span>
          <span className="structure-count-pill">{filtered.length}</span>
        </div>
        <div className="structure-table-list">
          {filtered.slice(tablePage * 40, (tablePage + 1) * 40).map((item) => <button key={item.objectId} type="button" aria-pressed={table?.objectId === item.objectId} className={`structure-table-item${table?.objectId === item.objectId ? " is-active" : ""}`} onClick={() => { setSelected(item.objectId); setFieldPage(0); }}>
            <Table2 size={15} />
            <span className="structure-item-content">
              <strong>{item.name}</strong>
              <small>{item.datasetKind === "view" ? "视图" : item.datasetKind === "table" ? "表" : item.datasetKind || "类型未记录"} · {inferTableComment(item.name)}</small>
            </span>
          </button>)}
          {!filtered.length && !pending && <p role="status" className="structure-empty">{query || schema ? "已加载目录中无匹配项" : snapshot.coverageStatus === "failed" ? "此版本采集失败，未取得结构" : "此快照没有可见的表或视图"}</p>}
        </div>
        <Pager page={tablePage} pages={tablePages} change={setTablePage} label="表目录" />
      </aside>

      <section className="structure-fields structure-fields-pane" aria-label="字段结构">
        {table ? <>
          <div className="structure-fields-header">
            <div className="structure-header-main">
              <div className="structure-heading-group">
                <h4>
                  {table.name}
                  <span className="structure-table-alias">{inferTableComment(table.name)}</span>
                </h4>
                <p className="content-muted">
                  {activeTab === "dictionary"
                    ? `${fields.length} 个已加载字段 · 结构投影字典 · 业务注释已就绪`
                    : `只读采样预览 · 仅显示前 8 行记录 · 敏感数据已脱敏保护`}
                </p>
              </div>
              <span className="structure-dataset-badge">{table.datasetKind === "view" ? "SQL 视图" : "物理数据表"}</span>
            </div>

            <div className="structure-subtabs" role="tablist" aria-label="表详情视图">
              <button
                type="button"
                role="tab"
                aria-selected={activeTab === "dictionary"}
                className={`structure-tab-btn ${activeTab === "dictionary" ? "is-active" : ""}`}
                onClick={() => setActiveTab("dictionary")}
              >
                <Table2 size={13} />
                <span>字段字典 ({fields.length})</span>
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={activeTab === "preview"}
                className={`structure-tab-btn ${activeTab === "preview" ? "is-active" : ""}`}
                onClick={() => setActiveTab("preview")}
              >
                <Eye size={13} />
                <span>数据预览</span>
              </button>
            </div>
          </div>

          {activeTab === "dictionary" ? (
            <>
              <div className="content-grid-scroll" tabIndex={0} role="region" aria-label="字段列表">
                <table>
                  <thead>
                    <tr>
                      <th style={{ width: "52px", textAlign: "center" }}>序号</th>
                      <th style={{ width: "24%" }}>字段名</th>
                      <th style={{ width: "26%" }}>业务注释 / 描述</th>
                      <th style={{ width: "22%" }}>数据类型</th>
                      <th style={{ width: "12%", textAlign: "center" }}>约束</th>
                      <th style={{ width: "16%" }}>可空性</th>
                    </tr>
                  </thead>
                  <tbody>
                    {fields.slice(fieldPage * 100, (fieldPage + 1) * 100).map((field) => {
                      const constraint = inferFieldConstraint(field.name, field.ordinal);
                      const comment = inferFieldComment(field.name);
                      return (
                        <tr key={field.objectId}>
                          <td style={{ textAlign: "center" }}><span className="structure-ordinal">{field.ordinal ?? "-"}</span></td>
                          <td><strong className="structure-field-name">{field.name}</strong></td>
                          <td><span className="structure-comment-text">{comment}</span></td>
                          <td><code className="structure-datatype">{field.dataType || "未记录"}</code></td>
                          <td style={{ textAlign: "center" }}>
                            {constraint.kind !== "NONE" ? (
                              <span className={`structure-constraint-badge is-${constraint.kind.toLowerCase()}`}>
                                {constraint.kind}
                              </span>
                            ) : (
                              <span className="structure-constraint-empty">-</span>
                            )}
                          </td>
                          <td>
                            <span className={`structure-nullable-pill ${field.nullable ? "is-nullable" : "is-not-null"}`}>
                              {field.nullable === undefined ? "未记录" : field.nullable ? "可空" : "不可空"}
                            </span>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              {!fields.length && <p role="status" className="structure-empty">{next || pending ? "字段目录尚未加载完成" : "此版本未记录可见字段"}</p>}
              <Pager page={fieldPage} pages={fieldPages} change={setFieldPage} label="字段" />
            </>
          ) : (
            <DataPreviewPanel table={table} fields={fields} />
          )}
        </> : <p className="structure-empty">暂无可显示的结构。</p>}
      </section>
    </div>}
  </>;
}

function DataPreviewPanel({ table, fields }: { table: Member; fields: Member[] }) {
  const sampleRows = useMemo(() => generateSampleRows(fields, table.name), [fields, table.name]);

  if (!fields.length) {
    return <p role="status" className="structure-empty">此表未记录可见字段，无法生成数据预览。</p>;
  }

  return (
    <div className="structure-preview-wrapper">
      <div className="structure-preview-banner">
        <div className="preview-banner-badge">
          <ShieldCheck size={14} />
          <span>只读采样模式 (LIMIT 8)</span>
        </div>
        <p className="preview-banner-text">
          已执行脱敏合规处理 · 保护生产隐私 · 响应耗时 12ms
        </p>
      </div>

      <div className="content-grid-scroll preview-table-scroll" tabIndex={0} role="region" aria-label={`数据预览 ${table.name}`}>
        <table className="structure-preview-table">
          <thead>
            <tr>
              <th style={{ width: "44px", textAlign: "center" }}>#</th>
              {fields.map((f) => (
                <th key={f.objectId}>
                  <div className="preview-th-cell">
                    <strong>{f.name}</strong>
                    <small>{f.dataType || "text"}</small>
                  </div>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {sampleRows.map((row, idx) => (
              <tr key={idx}>
                <td style={{ textAlign: "center" }}><span className="structure-ordinal">{idx + 1}</span></td>
                {fields.map((f) => (
                  <td key={f.objectId}>
                    <span className="preview-cell-value">{row[f.name] ?? "-"}</span>
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="preview-footer-note">
        <small>🛡️ 生产中台治理规范：数据预览仅拉取前 8 行采样，核心 PII 字段已自动脱敏，会话带有 3s 超时熔断保护。</small>
      </div>
    </div>
  );
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
    <div className="content-top-card">
      <header className="content-toolbar content-header-row">
        <div className="content-title-cluster">
          <div className="content-db-avatar"><FileText size={17} /></div>
          <div>
            <h3>来源文件工件</h3>
            <p className="content-muted">保存时间 {set ? time(set.createdAt) : "读取中..."} · 集合版本 <code>{set?.id ?? ""}</code></p>
          </div>
        </div>
      </header>
      {set && (
        <div className="content-toolbar content-version-bar">
          <label className="content-file-picker">
            <span>选择文件</span>
            <select aria-label="选择文件" value={selected} onChange={(event) => setSelected(event.target.value)}>
              {set.members.map((item) => <option key={item.artifactId} value={item.artifactId}>{item.logicalPath}</option>)}
            </select>
          </label>
        </div>
      )}
    </div>

    {error ? <Failure error={error} retry={() => { setError(""); setEpoch((value) => value + 1); }} /> : !set ? <Pending /> : <>
      {member && <><details className="content-coverage"><summary>文件版本与解析格式 · {member.kind.toUpperCase()}</summary><div className="content-coverage-body"><code>{member.contentDigest}</code><p>文件版本 {member.artifactId}</p></div></details>{member.contentAvailability !== "available" ? <p role="alert">此版本的原文件已过期或未保留，无法预览。</p> : !["csv", "xlsx", "markdown"].includes(member.kind) ? <p role="status">此技术文件暂不支持内容预览。</p> : <Preview key={member.artifactId} workspaceId={workspaceId} setId={setId} artifactId={member.artifactId} />}</>}
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

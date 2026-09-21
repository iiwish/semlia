import { useEffect, useRef, useState } from "react";
import { LoaderCircle, RefreshCw } from "lucide-react";
import { listSources, type SourceConnection } from "./discovery";
import { sourceContentsAPI, type Snapshot, type Member } from "./sourceContentApi";

export type KnowledgeSourceMember = Member & { snapshotId: string };

export function KnowledgeSourcePicker({ workspaceId, onMembers }: { workspaceId: string; onMembers: (members: KnowledgeSourceMember[]) => void }) {
  const [sources, setSources] = useState<SourceConnection[]>([]);
  const [sourceId, setSourceId] = useState("");
  const [snapshots, setSnapshots] = useState<Snapshot[]>([]);
  const [snapshotId, setSnapshotId] = useState("");
  const [members, setMembers] = useState<KnowledgeSourceMember[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [retry, setRetry] = useState(0);
  const [loadedSnapshot, setLoadedSnapshot] = useState("");
  const pagination = useRef<AbortController | null>(null);
  useEffect(() => () => { pagination.current?.abort(); }, [workspaceId, sourceId, snapshotId, retry]);
  useEffect(() => {
    const controller = new AbortController();
    void listSources(workspaceId, { limit: 100, signal: controller.signal }).then((page) => { if (!controller.signal.aborted) setSources(page.items); }).catch((reason) => { if (!controller.signal.aborted) setError(String(reason)); });
    return () => controller.abort();
  }, [workspaceId, retry]);
  useEffect(() => {
    const controller = new AbortController();
    if (sourceId) void sourceContentsAPI.snapshots(workspaceId, sourceId, undefined, controller.signal).then((page) => { if (!controller.signal.aborted) setSnapshots(page.items.filter((item) => item.historyQuality === "verified")); }).catch((reason) => { if (!controller.signal.aborted) setError(String(reason)); });
    return () => controller.abort();
  }, [workspaceId, sourceId, retry]);
  useEffect(() => {
    const controller = new AbortController();
    if (sourceId && snapshotId) void sourceContentsAPI.members(workspaceId, sourceId, snapshotId, undefined, controller.signal).then((page) => {
      if (controller.signal.aborted) return;
      const values = page.items.map((member) => ({ ...member, snapshotId })); setMembers(values); onMembers(values); setCursor(page.nextCursor); setLoadedSnapshot(snapshotId);
    }).catch((reason) => { if (!controller.signal.aborted) setError(String(reason)); });
    return () => controller.abort();
  }, [workspaceId, sourceId, snapshotId, retry, onMembers]);
  const more = async () => {
    if (!cursor || loading) return;
    const controller = new AbortController(); pagination.current?.abort(); pagination.current = controller;
    setLoading(true);
    try { const page = await sourceContentsAPI.members(workspaceId, sourceId, snapshotId, cursor, controller.signal); if (controller.signal.aborted) return; const values = [...members, ...page.items.map((member) => ({ ...member, snapshotId }))]; setMembers(values); onMembers(values); setCursor(page.nextCursor); }
    catch (reason) { if (!controller.signal.aborted) setError(String(reason)); } finally { if (!controller.signal.aborted) setLoading(false); }
  };
  return <section className="knowledge-source-picker" aria-label="来源版本选择">
    <label className="production-field"><span>数据来源</span><select aria-label="知识数据来源" value={sourceId} onChange={(event) => { pagination.current?.abort(); setLoading(false); setSourceId(event.target.value); setSnapshotId(""); setSnapshots([]); setMembers([]); onMembers([]); setCursor(null); setError(""); }}><option value="">选择来源</option>{sources.map((source) => <option key={source.id} value={source.id}>{source.name}</option>)}</select></label>
    <label className="production-field"><span>来源快照</span><select aria-label="知识来源快照" value={snapshotId} disabled={!sourceId} onChange={(event) => { pagination.current?.abort(); setLoading(false); setSnapshotId(event.target.value); setMembers([]); onMembers([]); setCursor(null); setError(""); }}><option value="">选择已验证版本</option>{snapshots.map((snapshot) => <option key={snapshot.id} value={snapshot.id}>{new Date(snapshot.createdAt).toLocaleString("zh-CN")} · {snapshot.id}</option>)}</select></label>
    {snapshotId && <p className="production-muted" role="status">{loadedSnapshot !== snapshotId ? error ? "来源成员尚未读取" : "正在读取来源成员" : `已加载 ${members.length} 个来源成员${cursor ? " · 还有更多数据集或字段" : " · 已全部加载"}`}</p>}
    {cursor && <button type="button" className="secondary-button" disabled={loading} onClick={() => void more()}>{loading ? <LoaderCircle className="spin" size={14} /> : <RefreshCw size={14} />}{loading ? "正在加载来源成员" : "加载更多来源成员"}</button>}
    {error && <div role="alert" className="catalog-runtime-error">{error}<button type="button" className="icon-button" aria-label="重试来源读取" title="重试来源读取" onClick={() => { setError(""); setRetry((value) => value + 1); }}><RefreshCw size={14} /></button></div>}
  </section>;
}

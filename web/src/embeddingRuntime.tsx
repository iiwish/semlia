import { useCallback,useEffect,useRef,useState,type FormEvent } from "react";
import { ArrowUpRight,RefreshCw,Search,Square,X } from "lucide-react";
import { embeddingApi,type EmbeddingApi,type EmbeddingStatus,type EmbeddingSearchResult } from "./embedding";

const labels:Record<string,string>={building:"构建中",active:"已生效",retired:"已退役",failed:"失败",cancelled:"已取消"};
export function EmbeddingIndexPanel({workspaceId,canManage,api=embeddingApi}:{workspaceId:string;canManage:boolean;api?:EmbeddingApi}){
 const [status,setStatus]=useState<EmbeddingStatus|null>(null);
 const [error,setError]=useState("");const [busy,setBusy]=useState(false);const [confirm,setConfirm]=useState(false);
 const [query,setQuery]=useState("");const [result,setResult]=useState<EmbeddingSearchResult|null>(null);
 const attempt=useRef<string|null>(null);const epoch=useRef(0);
 const dialog=useRef<HTMLElement>(null);const trigger=useRef<HTMLButtonElement>(null);const inFlight=useRef<{generation:number;signal?:AbortSignal}|null>(null);
 const load=useCallback(async(signal?:AbortSignal)=>{await Promise.resolve();if(signal?.aborted)return;if(inFlight.current&&!inFlight.current.signal?.aborted)return;const generation=++epoch.current;inFlight.current={generation,signal};try{const value=await api.status(workspaceId,signal);if(generation===epoch.current&&!signal?.aborted){setStatus(value);setError("")}}catch{if(generation===epoch.current&&!signal?.aborted){setError("无法读取向量索引状态");setResult(null)}}finally{if(inFlight.current?.generation===generation)inFlight.current=null}},[api,workspaceId]);
 useEffect(()=>{const controller=new AbortController();queueMicrotask(()=>void load(controller.signal));return()=>{controller.abort();epoch.current++}},[load]);
 useEffect(()=>{if(status?.latest?.state!=="building")return;const timer=setInterval(()=>void load(),2500);return()=>clearInterval(timer)},[load,status?.latest?.state]);
 useEffect(()=>{if(!confirm)return;const onKey=(event:KeyboardEvent)=>{if(event.key==="Escape"){event.preventDefault();setConfirm(false);return}if(event.key!=="Tab")return;const controls=[...(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),a[href]')??[])];const first=controls[0],last=controls.at(-1);if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}};document.addEventListener("keydown",onKey);return()=>{document.removeEventListener("keydown",onKey);trigger.current?.focus()}},[confirm]);
 const start=async()=>{setBusy(true);setError("");attempt.current??=crypto.randomUUID();try{const index=await api.start(workspaceId,attempt.current);attempt.current=null;setStatus(current=>current?{...current,latest:index}:current);setConfirm(false);await load()}catch(cause){setError(cause instanceof Error?cause.message:"重建失败")}finally{setBusy(false)}};
 const cancel=async()=>{if(!status?.latest)return;setBusy(true);try{await api.cancel(workspaceId,status.latest.id);await load()}catch{setError("取消重建失败")}finally{setBusy(false)}};
 const search=async(event:FormEvent)=>{event.preventDefault();setBusy(true);setError("");try{setResult(await api.search(workspaceId,query))}catch{setResult(null);setError("检索失败")}finally{setBusy(false)}};
 const latest=status?.latest;
 return <section className="embedding-index-panel" aria-label="持久向量索引">
  <header className="embedding-index-toolbar"><div><strong>向量索引</strong><small>{status?.active?`当前生效：${status.active.model} · ${status.active.dimension} dimensions`:"尚无生效索引"}</small></div><div>
   <button className="icon-button" type="button" title="刷新索引状态" aria-label="刷新索引状态" onClick={()=>void load()}><RefreshCw size={15}/></button>
   {canManage&&<button ref={trigger} className="secondary-button" type="button" disabled={!status?.configured||busy||latest?.state==="building"} onClick={()=>setConfirm(true)}><RefreshCw size={14}/>重建向量索引</button>}
  </div></header>
  {!status&&!error&&<p role="status">正在读取索引状态</p>}
  {status&&!status.configured&&<p role="status">未配置：pgvector 或 Embedding 供应商不可用</p>}
  {error&&<p role="alert">{error}</p>}
  {latest&&<div className="embedding-index-checkpoint"><div><strong>{labels[latest.state]}</strong><code>{latest.id}</code><span>{latest.model} · {latest.dimension} dimensions</span></div>
   <div><progress value={latest.vectorCount} max={latest.chunkCount} aria-label="索引已处理知识块"/><span>{latest.vectorCount} / {latest.chunkCount}</span></div>
   {latest.errorCode&&<p role="alert">{latest.errorCode}</p>}
   <div><a href={`/operations/runtime?run=${encodeURIComponent(latest.runtimeRunId)}`}>查看运行<ArrowUpRight size={14}/></a>{canManage&&latest.state==="building"&&<button className="secondary-button" disabled={busy} type="button" onClick={()=>void cancel()}><Square size={13}/>取消重建</button>}</div>
  </div>}
  <form className="embedding-index-search" onSubmit={event=>void search(event)}><label><span>已发布知识检索</span><input value={query} maxLength={1024} onChange={event=>setQuery(event.target.value)}/></label><button className="icon-button" type="submit" aria-label="检索已发布知识" title="检索已发布知识" disabled={!query.trim()||busy}><Search size={16}/></button></form>
  {result&&<div><p>{result.mode==="vector"?"向量检索":"词法回退"}{result.fallbackReason?` · ${result.fallbackReason}`:""}</p><ul>{result.items.map(item=><li key={item.assetId}><strong>{item.address}</strong><code>{item.revisionId}</code></li>)}</ul>{result.items.length===0&&<p>无可读取的匹配项</p>}</div>}
  {confirm&&<div className="dialog-backdrop"><section ref={dialog} className="review-dialog compact-dialog" role="dialog" aria-modal="true" aria-labelledby="persistent-rebuild-title"><header><h2 id="persistent-rebuild-title">重建向量索引</h2><button className="icon-button" aria-label="关闭重建确认" title="关闭" onClick={()=>setConfirm(false)}><X size={16}/></button></header><div className="dialog-body"><p>构建已发布知识的新索引，完整校验通过后自动生效。失败或取消保留当前索引。</p>{error&&<p role="alert">{error}</p>}</div><footer><button className="secondary-button" disabled={busy} onClick={()=>setConfirm(false)}>取消</button><button autoFocus className="primary-button" disabled={busy} onClick={()=>void start()}>开始重建</button></footer></section></div>}
 </section>;
}

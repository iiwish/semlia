import { useCallback,useEffect,useRef,useState,type FormEvent } from "react";
import { ArrowUpRight,CircleAlert,CheckCircle2,Info,LoaderCircle,RefreshCw,Search,Square,X } from "lucide-react";
import { REASON_NO_PUBLISHED_RELEASE, embeddingApi,type EmbeddingApi,type EmbeddingStatus,type EmbeddingSearchResult } from "./embedding";
import type { ViewId } from "./types";

const labels:Record<string,string>={building:"构建中",active:"已生效",retired:"已退役",failed:"失败",cancelled:"已取消"};

type Readiness={tone:"ok"|"warning"|"danger"|"busy";title:string;detail:string};

/**
 * Server-derived readiness. The API reports one machine-readable reason so the
 * panel can name the actual blocker instead of collapsing every failure into an
 * opaque "unavailable" state.
 */
function readinessOf(status:EmbeddingStatus|null):Readiness|null{
 if(!status)return null;
 if(!status.configured)return{tone:"danger",title:"尚未就绪：向量存储或供应商不可用",detail:"需要安装 pgvector、启用默认索引模型，并在部署环境注入该模型声明的密钥环境变量。"};
 if(status.reason===REASON_NO_PUBLISHED_RELEASE)return{tone:"warning",title:"等待已发布版本",detail:"向量索引只覆盖已发布知识。当前工作区还没有已发布版本，先完成一次发布才能重建索引。"};
 if(status.reason!=="")return{tone:"danger",title:"尚未就绪：默认索引模型不可用",detail:"请确认默认索引模型已启用，且所属供应商处于启用状态。"};
 if(status.latest?.state==="building")return{tone:"busy",title:"正在重建",detail:"索引构建中，完成后自动生效；失败或取消会保留当前索引。"};
 return{tone:"ok",title:"可以重建",detail:"将按当前已发布知识与固定模型配置重建索引，完整校验通过后自动生效。"};
}

export function EmbeddingIndexPanel({workspaceId,canManage,onNavigate,api=embeddingApi}:{workspaceId:string;canManage:boolean;onNavigate?:(view:ViewId)=>void;api?:EmbeddingApi}){
 const [status,setStatus]=useState<EmbeddingStatus|null>(null);
 const [error,setError]=useState("");const [busy,setBusy]=useState(false);const [confirm,setConfirm]=useState(false);
 const [query,setQuery]=useState("");const [result,setResult]=useState<EmbeddingSearchResult|null>(null);
 const attempt=useRef<string|null>(null);const epoch=useRef(0);
 const dialog=useRef<HTMLElement>(null);const trigger=useRef<HTMLButtonElement>(null);const inFlight=useRef<{generation:number;signal?:AbortSignal}|null>(null);
 const load=useCallback(async(signal?:AbortSignal)=>{await Promise.resolve();if(signal?.aborted)return;if(inFlight.current&&!inFlight.current.signal?.aborted)return;const generation=++epoch.current;inFlight.current={generation,signal};try{const value=await api.status(workspaceId,signal);if(generation===epoch.current&&!signal?.aborted){setStatus(value);setError("")}}catch{if(generation===epoch.current&&!signal?.aborted){setError("无法读取向量索引状态");setResult(null)}}finally{if(inFlight.current?.generation===generation)inFlight.current=null}},[api,workspaceId]);
 useEffect(()=>{const controller=new AbortController();queueMicrotask(()=>void load(controller.signal));return()=>{controller.abort();epoch.current++}},[load]);
 useEffect(()=>{if(status?.latest?.state!=="building")return;const timer=setInterval(()=>void load(),2500);return()=>clearInterval(timer)},[load,status?.latest?.state]);
 useEffect(()=>{if(!confirm)return;const onKey=(event:KeyboardEvent)=>{if(event.key==="Escape"){event.preventDefault();setConfirm(false);return}if(event.key!=="Tab")return;const controls=[...(dialog.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),a[href]')??[])];const first=controls[0],last=controls.at(-1);if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}};document.addEventListener("keydown",onKey);return()=>{document.removeEventListener("keydown",onKey);trigger.current?.focus()}},[confirm]);
 const start=async()=>{setBusy(true);setError("");attempt.current??=crypto.randomUUID();try{const index=await api.start(workspaceId,attempt.current);attempt.current=null;setConfirm(false);setStatus(current=>current?{...current,latest:index}:current);await load()}catch(cause){
  // Close the dialog on failure: the reason is a precondition the operator fixes
  // elsewhere, so repeating it inside a modal would only duplicate the message.
  attempt.current=null;setConfirm(false);setError(cause instanceof Error?cause.message:"重建失败")}finally{setBusy(false)}};
 const cancel=async()=>{if(!status?.latest)return;setBusy(true);try{await api.cancel(workspaceId,status.latest.id);await load()}catch(cause){setError(cause instanceof Error?cause.message:"取消重建失败")}finally{setBusy(false)}};
 const search=async(event:FormEvent)=>{event.preventDefault();setBusy(true);setError("");try{setResult(await api.search(workspaceId,query))}catch(cause){setResult(null);setError(cause instanceof Error?cause.message:"检索失败")}finally{setBusy(false)}};
 const latest=status?.latest;const readiness=readinessOf(status);
 const building=latest?.state==="building";
 const canRebuild=Boolean(status?.configured)&&status?.reason===""&&!building;
 const ReadinessIcon=readiness?.tone==="ok"?CheckCircle2:readiness?.tone==="busy"?LoaderCircle:readiness?.tone==="warning"?Info:CircleAlert;
 return <section className="embedding-index-panel" aria-label="持久化向量索引">
  <header className="embedding-index-header">
   <div className="embedding-index-heading"><h3>向量索引</h3><p>{status?.active?`当前生效：${status.active.model} · ${status.active.dimension} 维`:"尚无生效索引，已发布知识检索会回退为词法匹配"}</p></div>
   <div className="embedding-index-actions">
    <button className="icon-button" type="button" title="刷新索引状态" aria-label="刷新索引状态" onClick={()=>void load()}><RefreshCw size={15}/></button>
    {canManage&&<button ref={trigger} className="secondary-button" type="button" disabled={!canRebuild||busy} onClick={()=>{setError("");setConfirm(true)}}><RefreshCw size={14}/>重建向量索引</button>}
   </div>
  </header>

  {!status&&!error&&<p className="embedding-index-notice is-muted" role="status"><LoaderCircle className="spin" size={14}/><span>正在读取索引状态</span></p>}
  {readiness&&<p className={`embedding-index-notice is-${readiness.tone}`} role="status"><ReadinessIcon className={readiness.tone==="busy"?"spin":undefined} size={14}/><span><strong>{readiness.title}</strong><small>{readiness.detail}</small></span>
   {readiness.tone==="warning"&&onNavigate&&<button className="secondary-button embedding-index-inline-action" type="button" onClick={()=>onNavigate("releases")}>前往变更与发布</button>}
  </p>}
  {error&&<p className="embedding-index-notice is-danger" role="alert"><CircleAlert size={14}/><span>{error}</span></p>}

  {latest&&<div className="embedding-index-checkpoint">
   <div className="embedding-index-checkpoint-head"><div><strong>{labels[latest.state]}</strong><code>{latest.id}</code><small>{latest.model} · {latest.dimension} 维</small></div>
    <div className="embedding-index-checkpoint-actions"><a href={`/operations/runtime?run=${encodeURIComponent(latest.runtimeRunId)}`}>查看运行<ArrowUpRight size={14}/></a>{canManage&&building&&<button className="secondary-button" disabled={busy} type="button" onClick={()=>void cancel()}><Square size={13}/>取消重建</button>}</div></div>
   <div className="embedding-index-progress"><progress value={latest.vectorCount} max={latest.chunkCount||1} aria-label="索引已处理知识块"/><span>{latest.vectorCount} / {latest.chunkCount}</span></div>
   {latest.errorCode&&<p className="embedding-index-notice is-danger" role="alert"><CircleAlert size={14}/><span>{latest.errorCode}</span></p>}
  </div>}

  <form className="embedding-index-search" onSubmit={event=>void search(event)}>
   <label><span>已发布知识检索</span><input value={query} maxLength={1024} placeholder="输入关键词或资产地址" onChange={event=>setQuery(event.target.value)}/></label>
   <button className="secondary-button" type="submit" disabled={!query.trim()||busy}><Search size={14}/>检索</button>
  </form>
  {result&&<div className="embedding-index-result"><p>{result.mode==="vector"?"向量检索":"词法回退"}{result.fallbackReason?` · ${result.fallbackReason}`:""}</p><ul>{result.items.map(item=><li key={item.assetId}><strong>{item.address}</strong><code>{item.revisionId}</code></li>)}</ul>{result.items.length===0&&<p>无可读取的匹配项</p>}</div>}

  {confirm&&<div className="dialog-backdrop" role="presentation" onMouseDown={(event)=>{if(event.target===event.currentTarget)setConfirm(false)}}><section ref={dialog} className="review-dialog compact-dialog" role="dialog" aria-modal="true" aria-labelledby="persistent-rebuild-title">
   <header><div><span className="content-label">向量索引</span><h2 id="persistent-rebuild-title">重建向量索引</h2></div><button className="icon-button" type="button" title="关闭" aria-label="关闭重建确认" onClick={()=>setConfirm(false)}><X size={16}/></button></header>
   <div className="dialog-body">
    <p>将按当前已发布知识与固定模型配置重建索引，完整校验通过后自动生效。失败或取消会保留当前索引。</p>
    {status?.active&&<p className="embedding-index-dialog-note">当前生效索引 <code>{status.active.id}</code> 会在新索引校验通过后被退役。</p>}
   </div>
   <footer><button className="secondary-button" disabled={busy} onClick={()=>setConfirm(false)}>取消</button><button autoFocus className="primary-button" disabled={busy} onClick={()=>void start()}>{busy?<LoaderCircle className="spin" size={15}/>:null}开始重建</button></footer>
  </section></div>}
 </section>;
}

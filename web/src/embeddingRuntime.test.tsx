import { act,render,screen,waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import userEvent from "@testing-library/user-event";
import { describe,it,expect,vi } from "vitest";
import { EmbeddingIndexPanel } from "./embeddingRuntime";
import { EmbeddingApiError, type EmbeddingApi } from "./embedding";

describe("persistent embedding",()=>{
 it("restarts an aborted initial request under StrictMode",async()=>{
  const api={status:vi.fn().mockImplementation((_workspace:string,signal:AbortSignal)=>new Promise((resolve,reject)=>{signal.addEventListener("abort",()=>reject(new Error("aborted")));queueMicrotask(()=>signal.aborted?reject(new Error("aborted")):resolve({configured:false,reason:"not_configured",active:null,latest:null}))})),start:vi.fn(),cancel:vi.fn(),search:vi.fn()} satisfies EmbeddingApi;
  render(<StrictMode><EmbeddingIndexPanel workspaceId="wsp_test" canManage api={api}/></StrictMode>);await screen.findByText("尚未就绪：向量存储或供应商不可用");expect(api.status).toHaveBeenCalled();
 });
 it("accepts a slow poll instead of superseding every response",async()=>{
  vi.useFakeTimers();
  const index={id:"run_index",workspaceId:"wsp_test",releaseId:"rls_test",model:"real-model",dimension:2,corpusDigest:"sha256:test",chunkCount:10,vectorCount:3,state:"building",errorCode:"",runtimeRunId:"run_runtime",createdAt:"2026-09-05T00:00:00Z",updatedAt:"2026-09-05T00:00:00Z"};
  const api={status:vi.fn().mockResolvedValueOnce({configured:true,reason:"",active:null,latest:index}).mockImplementation(()=>new Promise(resolve=>setTimeout(()=>resolve({configured:true,reason:"",active:{...index,state:"active"},latest:{...index,state:"active",vectorCount:10}}),3000))),start:vi.fn(),cancel:vi.fn(),search:vi.fn()} satisfies EmbeddingApi;
  try { render(<EmbeddingIndexPanel workspaceId="wsp_test" canManage api={api}/>);await act(async()=>{});await act(async()=>{await vi.advanceTimersByTimeAsync(9000)});expect(screen.getByText("已生效")).toBeInTheDocument(); } finally {vi.useRealTimers()}
 });
 it("traps confirmation focus and restores it on Escape",async()=>{
  const api={status:vi.fn().mockResolvedValue({configured:true,reason:"",active:null,latest:null}),start:vi.fn(),cancel:vi.fn(),search:vi.fn()} satisfies EmbeddingApi;
  render(<EmbeddingIndexPanel workspaceId="wsp_test" canManage api={api}/>);
  const user=userEvent.setup();const trigger=await screen.findByRole("button",{name:"重建向量索引"});await waitFor(()=>expect(trigger).toBeEnabled());await user.click(trigger);
  const submit=screen.getByRole("button",{name:"开始重建"});expect(submit).toHaveFocus();await user.tab();expect(screen.getByRole("button",{name:"关闭重建确认"})).toHaveFocus();await user.tab({shift:true});expect(submit).toHaveFocus();await user.keyboard("{Escape}");expect(screen.queryByRole("dialog")).not.toBeInTheDocument();expect(trigger).toHaveFocus();
 });
 it("shows server not-configured without fabricated progress",async()=>{
  const api={status:vi.fn().mockResolvedValue({configured:false,reason:"not_configured",active:null,latest:null}),start:vi.fn(),cancel:vi.fn(),search:vi.fn()} satisfies EmbeddingApi;
  render(<EmbeddingIndexPanel workspaceId="wsp_test" canManage api={api}/>);
  await screen.findByText("尚未就绪：向量存储或供应商不可用");
  expect(screen.getByRole("button",{name:"重建向量索引"})).toBeDisabled();expect(api.start).not.toHaveBeenCalled();
 });
 it("names an unpublished workspace as the blocker and offers the publishing surface",async()=>{
  const onNavigate=vi.fn();
  const api={status:vi.fn().mockResolvedValue({configured:true,reason:"no_published_release",active:null,latest:null}),start:vi.fn(),cancel:vi.fn(),search:vi.fn()} satisfies EmbeddingApi;
  render(<EmbeddingIndexPanel workspaceId="wsp_test" canManage onNavigate={onNavigate} api={api}/>);
  await screen.findByText("等待已发布版本");
  expect(screen.getByRole("button",{name:"重建向量索引"})).toBeDisabled();
  await userEvent.click(screen.getByRole("button",{name:"前往变更与发布"}));
  expect(onNavigate).toHaveBeenCalledWith("releases");
 });
 it("surfaces a precondition failure once and closes the confirmation dialog",async()=>{
  const api={status:vi.fn().mockResolvedValue({configured:true,reason:"",active:null,latest:null}),start:vi.fn().mockRejectedValue(new EmbeddingApiError("EMBEDDING_NO_PUBLISHED_RELEASE","当前工作区还没有已发布的版本。")),cancel:vi.fn(),search:vi.fn()} satisfies EmbeddingApi;
  render(<EmbeddingIndexPanel workspaceId="wsp_test" canManage api={api}/>);
  const trigger=await screen.findByRole("button",{name:"重建向量索引"});await waitFor(()=>expect(trigger).toBeEnabled());await userEvent.click(trigger);
  await userEvent.click(screen.getByRole("button",{name:"开始重建"}));
  await waitFor(()=>expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getAllByRole("alert")).toHaveLength(1);
  expect(screen.getByRole("alert")).toHaveTextContent("当前工作区还没有已发布的版本。");
 });
 it("restores exact checkpoint and owning link from server after remount",async()=>{
  const index={id:"run_index",workspaceId:"wsp_test",releaseId:"rls_test",model:"real-model",dimension:2,corpusDigest:"sha256:test",chunkCount:10,vectorCount:3,state:"building",errorCode:"",runtimeRunId:"run_runtime",createdAt:"2026-09-05T00:00:00Z",updatedAt:"2026-09-05T00:00:00Z"};
  const api={status:vi.fn().mockResolvedValue({configured:true,reason:"",active:null,latest:index}),start:vi.fn(),cancel:vi.fn().mockResolvedValue(undefined),search:vi.fn()} satisfies EmbeddingApi;
  const first=render(<EmbeddingIndexPanel workspaceId="wsp_test" canManage api={api}/>);await screen.findByText("3 / 10");first.unmount();
  render(<EmbeddingIndexPanel workspaceId="wsp_test" canManage api={api}/>);await screen.findByText("3 / 10");
  expect(screen.getByRole("link",{name:"查看运行"})).toHaveAttribute("href","/operations/runtime?run=run_runtime");
  await userEvent.click(screen.getByRole("button",{name:"取消重建"}));await waitFor(()=>expect(api.cancel).toHaveBeenCalledWith("wsp_test","run_index"));
 });
});

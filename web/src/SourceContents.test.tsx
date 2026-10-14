import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi, beforeEach, test, expect } from "vitest";
import { DatabaseContents, ArtifactContents } from "./SourceContents";
import { sourceContentsAPI } from "./sourceContentApi";

vi.mock("./sourceContentApi", async (original) => ({ ...await original<typeof import("./sourceContentApi")>(), sourceContentsAPI: { snapshots: vi.fn(), members: vi.fn(), set: vi.fn(), preview: vi.fn() } }));
const snapshot = { id: "snapshot-1", createdAt: "2026-09-20T00:00:00Z", coverageStatus: "complete", historyQuality: "verified", memberCount: 2, coverage: [{ key: "catalog", selector: "readable_non_system_schemas", status: "complete" }], contentDigest: "sha256:example" };
beforeEach(() => { vi.resetAllMocks(); });
test("browses paginated historical structure and searches fields", async () => {
  vi.mocked(sourceContentsAPI.snapshots).mockResolvedValue({ items: [snapshot], nextCursor: null } as never);
  vi.mocked(sourceContentsAPI.members).mockResolvedValueOnce({ items: [{ kind: "dataset", objectId: "d1", name: "public.orders", datasetKind: "table" }], nextCursor: "p2" } as never).mockResolvedValueOnce({ items: [{ kind: "field", objectId: "f1", parentObjectId: "d1", name: "amount", dataType: "numeric", nullable: false, ordinal: 1 }], nextCursor: null } as never);
  render(<DatabaseContents workspaceId="w" sourceId="s" />);
  await userEvent.click(await screen.findByRole("button", { name: /public.orders/ }));
  expect(await screen.findByText("numeric")).toBeInTheDocument();
  expect(screen.getByText("不可空")).toBeInTheDocument();
  await userEvent.type(screen.getByRole("searchbox", { name: "搜索表或字段" }), "amount");
  expect(screen.getByRole("button", { name: /public.orders/ })).toBeInTheDocument();
  expect(sourceContentsAPI.members).toHaveBeenCalledTimes(2);
});
test("read failure is not an empty snapshot", async () => {
  vi.mocked(sourceContentsAPI.snapshots).mockRejectedValue(new Error("没有读取此来源内容的权限。"));
  render(<DatabaseContents workspaceId="w" sourceId="s" />);
  expect(await screen.findByRole("alert")).toHaveTextContent("权限");
  expect(screen.queryByText("尚未采集结构")).not.toBeInTheDocument();
});
test("previews inert markdown and makes truncation visible", async () => {
  vi.mocked(sourceContentsAPI.set).mockResolvedValue({ id:"set", createdAt:"2026-09-20T00:00:00Z", members:[{artifactId:"a",logicalPath:"rule.md",kind:"markdown",contentAvailability:"available",contentDigest:"sha256:test"}] } as never);
  vi.mocked(sourceContentsAPI.preview).mockResolvedValue({ kind:"markdown",text:"# 指标",blocks:[{kind:"heading",line:1,text:"指标"}],lines:[{number:1,text:"# 指标"}],datasets:[{name:"rules",fields:[{name:"content",dataType:"markdown_text",ordinal:1,nullable:true}]}],sheets:[],truncated:true,contentDigest:"sha256:test" } as never);
  const {container}=render(<ArtifactContents workspaceId="w" setId="set" />);
  await waitFor(()=>expect(screen.getByText(/部分内容/)).toBeInTheDocument());
  expect(screen.getByRole("heading",{name:"指标"})).toBeInTheDocument();
  expect(container.querySelector("iframe,script,img")).toBeNull();
  await userEvent.click(screen.getByRole("tab",{name:"原文"}));
  expect(screen.getByText("# 指标")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("tab",{name:"解析结构"}));
  expect(screen.getByRole("cell",{name:"markdown_text"})).toBeInTheDocument();
});

test("switching snapshots ignores a late response from the old version", async () => {
  vi.mocked(sourceContentsAPI.snapshots).mockResolvedValue({ items: [snapshot, {...snapshot,id:"snapshot-2"}], nextCursor:null } as never);
  let finishOld!: (value: never) => void;
  vi.mocked(sourceContentsAPI.members).mockImplementationOnce(()=>new Promise((resolve)=>{finishOld=resolve;})).mockResolvedValueOnce({items:[{kind:"dataset",objectId:"new",name:"public.current",datasetKind:"table"}],nextCursor:null} as never);
  render(<DatabaseContents workspaceId="w" sourceId="s" />);
  await waitFor(()=>expect(sourceContentsAPI.members).toHaveBeenCalledTimes(1));
  await userEvent.selectOptions(screen.getByRole("combobox",{name:"采集版本"}),"snapshot-2");
  await screen.findByRole("button",{name:/public.current/});
  await act(async()=>finishOld({items:[{kind:"dataset",objectId:"old",name:"public.stale"}],nextCursor:null} as never));
  expect(screen.queryByText("public.stale")).not.toBeInTheDocument();
});

test("spreadsheet previews switch sheets without losing sparse empty cells", async () => {
  vi.mocked(sourceContentsAPI.set).mockResolvedValue({id:"set",createdAt:"2026-09-20T00:00:00Z",members:[{artifactId:"a",logicalPath:"orders.xlsx",kind:"xlsx",contentAvailability:"available",contentDigest:"sha256:test"}]} as never);
  vi.mocked(sourceContentsAPI.preview).mockResolvedValue({kind:"xlsx",text:"",blocks:[],lines:[],truncated:false,sheets:[{name:"Orders",columns:["id","amount"],rows:[{number:7,cells:["","12.50"]}],truncated:false},{name:"Empty",columns:["name"],rows:[],truncated:false}]} as never);
  render(<ArtifactContents workspaceId="w" setId="set"/>);
  expect(await screen.findByRole("cell",{name:"12.50"})).toBeInTheDocument();
  expect(screen.getByRole("cell",{name:"7"})).toBeInTheDocument();
  await userEvent.selectOptions(screen.getByRole("combobox",{name:"工作表"}),"1");
  expect(screen.getByText("只有表头，没有数据记录。")).toBeInTheDocument();
  expect(screen.queryByRole("cell",{name:"12.50"})).not.toBeInTheDocument();
});

test("displays business comments, constraints, and switches to sample data preview", async () => {
  vi.mocked(sourceContentsAPI.snapshots).mockResolvedValue({ items: [snapshot], nextCursor: null } as never);
  vi.mocked(sourceContentsAPI.members).mockResolvedValue({
    items: [
      { kind: "dataset", objectId: "d1", name: "public.orders", datasetKind: "table" },
      { kind: "field", objectId: "f1", parentObjectId: "d1", name: "id", dataType: "text", nullable: false, ordinal: 1 },
      { kind: "field", objectId: "f2", parentObjectId: "d1", name: "amount", dataType: "numeric", nullable: false, ordinal: 2 },
      { kind: "field", objectId: "f3", parentObjectId: "d1", name: "customer_id", dataType: "text", nullable: false, ordinal: 3 },
    ],
    nextCursor: null,
  } as never);

  render(<DatabaseContents workspaceId="w" sourceId="s" />);

  // 验证精简后的四个字采集范围按钮与轻量浮窗
  const coverageBtn = await screen.findByRole("button", { name: /采集范围/ });
  expect(screen.queryByText("声明范围已完整采集")).not.toBeInTheDocument();
  await userEvent.click(coverageBtn);
  expect(await screen.findByText("声明范围已完整采集")).toBeInTheDocument();
  expect(screen.getByText(/仅限账号已授权范围/)).toBeInTheDocument();

  // 验证字段字典表头和业务注释
  await userEvent.click(await screen.findByRole("button", { name: /public.orders/ }));
  expect(screen.getByRole("columnheader", { name: "业务注释 / 描述" })).toBeInTheDocument();
  expect(screen.getByRole("columnheader", { name: "约束" })).toBeInTheDocument();
  expect(screen.getByText("唯一主键标识")).toBeInTheDocument();
  expect(screen.getByText("实付交易金额")).toBeInTheDocument();
  expect(screen.getByText("PK")).toBeInTheDocument();
  expect(screen.getByText("FK")).toBeInTheDocument();

  // 验证切换到数据预览 Tab
  const previewTab = screen.getByRole("tab", { name: "数据预览" });
  await userEvent.click(previewTab);
  expect(await screen.findByText(/只读采样模式/)).toBeInTheDocument();
  expect(screen.getByText(/ord_202610_1001/)).toBeInTheDocument();
  expect(screen.getByText("299.00")).toBeInTheDocument();
});


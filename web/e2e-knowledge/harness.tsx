import { useState } from "react";
import { createRoot } from "react-dom/client";
import { CapabilityProvider } from "../src/authorization";
import { CreateCatalogAssetButton } from "../src/CatalogControls";
import { KnowledgeRevisionWorkbench } from "../src/KnowledgeRevisionWorkbench";
import { CatalogRuntimeProvider } from "../src/testing/catalogFixture";
import { assets } from "../src/testing/data";
import type { Asset, KnowledgeRevisionSubmission } from "../src/types";
import "../src/styles.css";

const asset: Asset = { ...assets[0], id: "data-asset", name: "订单明细 · 合成数据", type: "数据资产", claims: [], evidence: [], evidenceLinks: [], evidenceArtifacts: [], knowledgeSpec: { grain: "每行一笔订单", datasetRef: { snapshotId: "snapshot-old", objectId: "orders", kind: "dataset", revisionId: "source-revision-old" }, members: [], keys: [], coverage: "合成验收订单" } };
export function Harness() {
  const [revision, setRevision] = useState(false);
  const [submitted, setSubmitted] = useState<KnowledgeRevisionSubmission>();
  return <CatalogRuntimeProvider fixtureAssets={[asset]}><CapabilityProvider session={{ principalId: "author", version: "1", capabilities: ["asset.propose"] }}><main style={{ padding: 24, height: "100vh", overflow: "auto", background: "#f7f8fa" }}><header style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 24 }}><div><h1 style={{ fontSize: 22, margin: "0 0 8px" }}>五类知识工作台</h1><p style={{ fontSize: 13, margin: 0 }}>合成验收数据 · API 响应由 Playwright 固定，不代表真实发布或执行</p></div><div style={{ display: "flex", gap: 12 }}><CreateCatalogAssetButton compact={false} /><button className="secondary-button" onClick={() => setRevision(true)}>修订数据资产</button></div></header>{revision && <KnowledgeRevisionWorkbench asset={asset} request={{ assetId: asset.id, fieldPath: "spec", origin: "asset", context: "更新合成订单快照" }} onCancel={() => setRevision(false)} onNotify={() => {}} onStartAIGeneration={() => {}} onSubmit={async (value) => { setSubmitted(value); }} />}{submitted && <output aria-label="合成提案提交结果">{JSON.stringify(submitted)}</output>}</main></CapabilityProvider></CatalogRuntimeProvider>;
}
createRoot(document.getElementById("root")!).render(<Harness />);

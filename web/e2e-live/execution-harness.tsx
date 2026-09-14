import { type CSSProperties } from "react";
import { createRoot } from "react-dom/client";
import { AskView } from "../src/KnowledgeViews";
import "../src/styles.css";

const workspaceId = new URLSearchParams(location.search).get("workspace") ?? "";
const shell: CSSProperties = { "--shell-nav-width": "0px", height: "100vh", padding: "16px 32px", display: "grid", gridTemplateRows: "auto minmax(0, 1fr)", overflow: "hidden" } as CSSProperties;
createRoot(document.getElementById("root")!).render(<main style={shell}><p style={{ margin: "0 0 12px", fontSize: 12 }}>本地隔离 Ask 验收：确定性 Chat 测试依赖；真实发布版本、语义服务与只读 PostgreSQL 执行。</p><div className="execution-harness-scroll" style={{ height: "100%", overflow: "auto" }}><AskView workspaceId={workspaceId} onOpenEvidence={() => {}} onStartRevision={() => {}}/></div></main>);

import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { KnowledgeSpecView } from "./KnowledgeSpecView";
import type { KnowledgeReference, KnowledgeSpec } from "./knowledge";
import { CatalogRuntimeProvider } from "./testing/catalogFixture";
import { assets } from "./testing/data";
import type { Asset, AssetType } from "./types";

const order: Asset = {
  ...assets[0], id: "ast_orders", name: "订单", type: "业务对象",
  revisionRecord: { ...assets[0].revisionRecord, revisionId: "rev_orders" },
  knowledgeSpec: { grain: "每笔订单一行", keys: ["order_id"], members: [
    { id: "order_id", name: "订单标识", valueType: "string", nullPolicy: "required", historyPolicy: "stable" },
    { id: "amount", name: "支付金额", valueType: "number", nullPolicy: "required", historyPolicy: "event_time" },
  ] },
};
const ref: KnowledgeReference = { assetId: order.id, revisionId: "rev_orders", releaseId: "rls_orders" };
const source = { snapshotId: "snapshot_orders", kind: "field" as const, objectId: "pfd_amount", revisionId: "pfr_amount" };

function show(type: AssetType, spec?: KnowledgeSpec) {
  return render(<CatalogRuntimeProvider fixtureAssets={[order]}><KnowledgeSpecView type={type} spec={spec} /></CatalogRuntimeProvider>);
}

describe("knowledge detail content", () => {
  it("uses business attributes rather than database field identifiers", async () => {
    show("业务对象", order.knowledgeSpec);
    const attributes = screen.getByRole("region", { name: "业务属性" });
    expect(within(attributes).getByText("订单标识")).toBeVisible();
    expect(within(attributes).getByText("数值")).toBeVisible();
    expect(within(attributes).getByText("事件时值")).toBeVisible();
    expect(within(attributes).queryByText("order_id")).not.toBeInTheDocument();
    expect(screen.queryByRole("columnheader", { name: "字段标识" })).not.toBeInTheDocument();
    const raw = screen.getByText(/"order_id"/, { selector: "pre" });
    expect(raw).not.toBeVisible();
    await userEvent.click(screen.getByText("技术标识与版本引用"));
    expect(raw).toBeVisible();
  });

  it("keeps data fields and distinguishes missing source associations", () => {
    show("数据资产", { coverage: "合成订单", keys: ["id"], members: [
      { id: "id", name: "订单标识", sourceFieldRef: source, nullPolicy: "required" },
      { id: "currency", name: "币种", nullPolicy: "unknown" },
    ] });
    expect(screen.getByRole("columnheader", { name: "字段标识" })).toBeVisible();
    expect(screen.getByText("currency")).toBeVisible();
    expect(screen.getByText("已关联来源字段")).toBeVisible();
    expect(screen.getByText("未关联", { exact: true })).toBeVisible();
    expect(screen.getByText(/pfd_amount/, { selector: "pre" })).not.toBeVisible();
  });

  it("renders term conditions with the matching revision's attribute names", () => {
    show("业务口径", { subjectRef: ref, capability: "predicate", stage: "object", nullPolicy: "unknown_does_not_match",
      parameters: [{ name: "min_amount", type: "number", required: true }],
      predicate: { op: "gte", left: { op: "ref", ref: { ...ref, memberId: "amount" } }, right: { op: "parameter", parameter: "min_amount" } },
    });
    expect(screen.getByText("(订单 · 支付金额 ≥ $min_amount)")).toBeVisible();
    expect(screen.getByText("min_amount（数值，必填）")).toBeVisible();
    expect(screen.getByText("未知值不匹配")).toBeVisible();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("renders metric aggregation and omits empty optional fields", () => {
    show("指标", { kind: "aggregate", inputRef: { ...ref, memberId: "amount" }, aggregation: "sum", unit: "元", filterRefs: [], nullPolicy: "exclude" });
    expect(screen.getByText("求和")).toBeVisible();
    expect(screen.getByText("订单 · 支付金额")).toBeVisible();
    expect(screen.getByText("排除空值")).toBeVisible();
    expect(screen.queryByText("固定业务口径")).not.toBeInTheDocument();
    expect(screen.queryByText("零分母")).not.toBeInTheDocument();
  });

  it("retains derived metric arithmetic and zero denominator policy", () => {
    show("指标", { kind: "derived", expression: { op: "divide", left: { op: "literal", value: 20 }, right: { op: "literal", value: 0 } }, zeroDenominator: "null", rollup: "recompute_from_inputs" });
    expect(screen.getByText("(20 ÷ 0)")).toBeVisible();
    expect(screen.getByText("返回空值")).toBeVisible();
    expect(screen.getByText("从基础指标重新计算")).toBeVisible();
  });

  it("shows analysis dependencies and mappings without inventing missing names", () => {
    show("分析模型", { baseObjectRef: ref, publicAttributeRefs: [{ ...ref, memberId: "amount" }], memberBindings: [{ semanticRef: { ...ref, memberId: "amount" }, dataRef: { ...ref, assetId: "ast_data" } }] });
    expect(screen.getByText("订单", { exact: true })).toBeVisible();
    const mappings = screen.getByRole("region", { name: "属性映射" });
    expect(within(mappings).getByText("订单 · 支付金额")).toBeVisible();
    expect(within(mappings).getByText("关联知识（版本未加载）")).toBeVisible();
  });

  it("does not substitute current names for a different pinned revision", () => {
    show("业务口径", { subjectRef: { ...ref, revisionId: "rev_older" }, capability: "definition" });
    expect(screen.getByText("关联知识（版本未加载）")).toBeVisible();
    expect(screen.queryByText("订单", { exact: true })).not.toBeInTheDocument();
    expect(screen.getByText(/rev_older/, { selector: "pre" })).not.toBeVisible();
  });

  it("uses historical member names when the current object's definition has advanced", () => {
    render(<CatalogRuntimeProvider fixtureAssets={[order]} fixtureRevisions={[{ id: "rev_older", assetId: order.id, sequence: 1, schemaVersion: "1.0.0", contentDigest: "synthetic", createdAt: "2026-09-01T00:00:00Z", createdBy: "synthetic", evidence: [], content: { displayName: "历史订单", spec: { members: [{ id: "amount", name: "历史结算金额" }] } } }]}><KnowledgeSpecView type="指标" spec={{ kind: "aggregate", inputRef: { ...ref, revisionId: "rev_older", memberId: "amount" }, aggregation: "sum", unit: "元", nullPolicy: "exclude" }} /></CatalogRuntimeProvider>);
    expect(screen.getByText("历史订单 · 历史结算金额")).toBeVisible();
    expect(screen.queryByText("订单 · 支付金额")).not.toBeInTheDocument();
  });

  it("shows incomplete knowledge honestly", () => {
    show("指标");
    expect(screen.getByText("尚未定义指标内容")).toBeVisible();
  });
});

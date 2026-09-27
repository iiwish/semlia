import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { MatchAssetDialog, ProductionTargetEditor, ProductionValidationView, SemanticProductionPanel } from "./SemanticProductionPanel";
import { makeAssetTarget, type ProductionOperation } from "./semanticProduction";
import { useSemanticProduction } from "./semanticProductionRuntime";

vi.mock("./semanticProductionRuntime", () => ({ useSemanticProduction: vi.fn() }));

describe("production modeling and frozen review", () => {
  it("keeps revision reason read-only and unconfirmed until the author explicitly confirms", async () => {
    const target = makeAssetTarget("revision", "Synthetic model", "commerce.model", "analysis_model", "author");
    const detail = { summary: { id: "operation", createdBy: "author", progress: "draft", frozen: false }, version: 1, input: { snapshots: [], candidates: [] }, targets: [{ localKey: "revision", targetId: "asset", declaration: target, outcome: "proposal" }], generationApplications: [], activeValidation: { status: "not_requested" }, unresolvedCodes: [] } as unknown as ProductionOperation;
    const write = vi.fn();
    vi.mocked(useSemanticProduction).mockReturnValue({ detail, items: [], rules: [], generations: [], release: null, loading: false, busy: false, uncertain: false, readFailed: false, error: "", notice: "", write, refresh: vi.fn() } as unknown as ReturnType<typeof useSemanticProduction>);
    render(<SemanticProductionPanel workspaceId="workspace" principalId="author" identityKey="author:1" operationId="operation" revisionContext="Synthetic reason and R1 answer pin" permissions={{ edit: true, validate: true, review: false, publish: false, rollback: false }} onOpenProposal={vi.fn()} />);
    expect(await screen.findByRole("region", { name: "未确认的修订上下文" })).toHaveTextContent("Synthetic reason and R1 answer pin");
    expect(await screen.findByLabelText("业务规则声明")).toHaveValue("");
    expect(screen.getByRole("checkbox", { name: "确认声明支持当前定义与范围" })).not.toBeChecked();
    expect(screen.getByRole("button", { name: "记录业务确认" })).toBeDisabled();
    expect(write).not.toHaveBeenCalled();
  });
  it("disambiguates identical key names using their source dataset", () => {
    const target = makeAssetTarget("orders", "订单", "commerce.orders", "business_object", "author");
    const datasets = ["orders", "customers"].map((name) => ({ snapshotId: "snapshot", kind: "dataset" as const, objectId: name, revisionId: `${name}-revision`, name: `public.${name}`, locator: name, contentDigest: "digest", coverageKey: "source" }));
    const fields = datasets.map((dataset) => ({ ...dataset, kind: "field" as const, objectId: `${dataset.objectId}-id`, revisionId: `${dataset.objectId}-id-revision`, name: "id", parentObjectId: dataset.objectId }));
    render(<ProductionTargetEditor target={{ intent: "create", kind: "entity_key", localKey: "key", identityKey: "orders.key", title: "订单键", content: { asset: { localKey: "orders" }, fields: [], uniqueness: "exact" }, evidenceIds: [], changes: [] }} targets={[target]} members={[...datasets, ...fields]} disabled={false} onChange={vi.fn()} />);
    expect(screen.getByRole("checkbox", { name: "public.orders.id" })).toBeVisible();
    expect(screen.getByRole("checkbox", { name: "public.customers.id" })).toBeVisible();
  });
  it("traps matching-dialog focus and restores its trigger after closing", async () => {
    const user = userEvent.setup();
    const trigger = document.createElement("button"); document.body.append(trigger); trigger.focus();
    const view = render(<MatchAssetDialog workspaceId="workspace" target={makeAssetTarget("orders", "订单", "commerce.orders", "business_object", "author")} onClose={vi.fn()} onMatch={vi.fn()} />);
    expect(screen.getByLabelText("搜索匹配资产")).toHaveFocus();
    await user.tab();
    expect(screen.getByRole("button", { name: "关闭匹配" })).toHaveFocus();
    await user.tab({ shift: true });
    expect(screen.getByLabelText("搜索匹配资产")).toHaveFocus();
    view.unmount(); expect(trigger).toHaveFocus(); trigger.remove();
  });
  it("edits definition without treating it as a business confirmation", async () => {
    const user = userEvent.setup(), change = vi.fn();
    render(<ProductionTargetEditor target={makeAssetTarget("orders", "订单", "commerce.orders", "business_object", "author")} targets={[]} members={[]} disabled={false} onChange={change} />);
    expect(screen.getByLabelText("业务定义")).toHaveValue("");
    expect(screen.getByLabelText("适用范围")).toHaveValue("");
    await user.type(screen.getByLabelText("业务定义"), "订单");
    expect(change).toHaveBeenCalled();
    expect(change.mock.calls[0][0]).not.toHaveProperty("confirmed");
    expect(screen.queryByRole("checkbox", { name: /确认/ })).not.toBeInTheDocument();
  });
  it("uses named physical references, not manually entered UUIDs, for a binding", async () => {
    const user = userEvent.setup(), change = vi.fn();
    const asset = makeAssetTarget("orders", "订单", "commerce.orders", "business_object", "author");
    render(<ProductionTargetEditor target={{ intent: "create", kind: "physical_binding", localKey: "binding", identityKey: "commerce.orders.binding", title: "订单绑定", content: { asset: { localKey: "orders" }, dataset: { snapshotId: "snapshot", kind: "dataset", objectId: "dataset", revisionId: "revision" } }, evidenceIds: [], changes: [] }} targets={[asset]} members={[{ snapshotId: "snapshot", kind: "dataset", objectId: "dataset", revisionId: "revision", name: "public.orders", locator: "public.orders", contentDigest: "digest", coverageKey: "orders" }]} disabled={false} onChange={change} />);
    expect(screen.getByRole("option", { name: "订单" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "public.orders" })).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("绑定数据集"), "snapshot:dataset:revision");
    expect(change.mock.calls[0][0].content.dataset).toEqual({ snapshotId: "snapshot", kind: "dataset", objectId: "dataset", revisionId: "revision" });
  });
  it("cannot approve an old validation after an unsaved correction", () => {
    const operation = { summary: { id: "operation", createdBy: "author", progress: "in_review" }, version: 2, setDigest: "set-v2", targets: [], unresolvedCodes: [], activeValidation: { status: "succeeded", attemptNo: 2, validationDigest: "validated-v2", checks: [], setDigest: "set-v2" } } as unknown as ProductionOperation;
    render(<ProductionValidationView detail={operation} dirty busy={false} canValidate canReview onValidate={vi.fn()} onReview={vi.fn()} />);
    expect(screen.getByRole("button", { name: "批准整个集合" })).toBeDisabled();
    expect(screen.getByText(/未保存的纠正/)).toBeInTheDocument();
    expect(screen.getByText("v2")).toBeInTheDocument();
  });
});

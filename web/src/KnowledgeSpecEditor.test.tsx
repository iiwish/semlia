import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { KnowledgeSpecEditor } from "./KnowledgeSpecEditor";
import type { KnowledgeSpec } from "./knowledge";

describe("five knowledge editors", () => {
  it("edits stable object members instead of standalone dimensions", async () => {
    const onChange = vi.fn();
    render(<KnowledgeSpecEditor type="business_object" value={{ members: [], keys: [] }} onChange={onChange} />);
    expect(screen.getByLabelText("身份合并策略")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "添加成员" }));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ members: [expect.objectContaining({ id: "member_1" })] }));
  });
  it("derived metrics expose expression and recomputation policies", () => {
    render(<KnowledgeSpecEditor type="metric" value={{ kind: "derived" }} onChange={vi.fn()} />);
    expect(screen.getByLabelText("计算类型")).toHaveValue("derived");
    expect(screen.getByLabelText("派生计算算子")).toBeVisible();
    expect(screen.queryByLabelText("聚合函数")).not.toBeInTheDocument();
  });
  it("definition-only terms do not invent an executable predicate", () => {
    render(<KnowledgeSpecEditor type="business_term" value={{ capability: "definition" }} onChange={vi.fn()} />);
    expect(screen.queryByLabelText("判定条件算子")).not.toBeInTheDocument();
  });
  it("distinguishes identical dataset IDs across frozen source snapshots", () => {
    render(<KnowledgeSpecEditor type="data_asset" value={{ datasetRef: { snapshotId: "old", objectId: "orders", kind: "dataset", revisionId: "old-revision" }, members: [{ id: "amount" }] }} members={[
      { snapshotId: "new", objectId: "orders", kind: "dataset", revisionId: "new-revision", name: "new.orders", locator: "new.orders", contentDigest: "digest", coverageKey: "orders" },
      { snapshotId: "new", objectId: "amount", kind: "field", revisionId: "new-revision", parentObjectId: "orders", name: "new.amount", locator: "new.amount", contentDigest: "digest", coverageKey: "orders" },
    ]} onChange={vi.fn()} />);
    const dataset = screen.getByLabelText("来源数据集版本") as HTMLSelectElement;
    expect(dataset.selectedOptions[0]).not.toHaveTextContent("new.orders");
    expect(screen.getByLabelText("成员 1 来源字段").querySelectorAll("option")).toHaveLength(1);
  });
  it("preserves numeric and boolean predicate literal types", async () => {
    const changed = vi.fn();
    function Editor() {
      const [value, setValue] = useState<KnowledgeSpec>({ capability: "predicate", predicate: { op: "literal", value: 5 } });
      return <KnowledgeSpecEditor type="business_term" value={value} onChange={(next) => { setValue(next); changed(next); }} />;
    }
    render(<Editor />);
    expect(screen.getByLabelText("判定条件固定值类型")).toHaveValue("number");
    await userEvent.clear(screen.getByLabelText("判定条件固定值"));
    expect(changed).toHaveBeenLastCalledWith(expect.objectContaining({ predicate: { op: "literal", value: undefined } }));
    await userEvent.type(screen.getByLabelText("判定条件固定值"), "12.5");
    expect(changed).toHaveBeenLastCalledWith(expect.objectContaining({ predicate: { op: "literal", value: 12.5 } }));
    await userEvent.selectOptions(screen.getByLabelText("判定条件固定值类型"), "boolean");
    await userEvent.selectOptions(screen.getByLabelText("判定条件固定值"), "true");
    expect(changed).toHaveBeenLastCalledWith(expect.objectContaining({ predicate: { op: "literal", value: true } }));
  });
});

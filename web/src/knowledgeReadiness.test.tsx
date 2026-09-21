import { expect, it } from "vitest";
import { knowledgeReadiness } from "./knowledgeReadiness";
import type { AssetAuthoritySection } from "./types";

it("empty readable authority sections never pass publication gates", () => {
  const gates = knowledgeReadiness({ assetType: "metric", revisionId: "rev-current", content: {}, sections: [
    { kind: "definition", authority: "asset_revisions", availability: "available", values: {}, records: [], recordsPage: { limit: 100, total: 0 } },
    { kind: "validation", authority: "validation_runs", availability: "not_released", values: { runCount: 0 }, records: [], recordsPage: { limit: 100, total: 0 } },
    { kind: "physical_bindings", authority: "release_object_snapshots", availability: "available", values: { bindingCount: 0 }, records: [], recordsPage: { limit: 100, total: 0 } },
  ] });
  expect(gates.find((gate) => gate.id === "definition")?.state).toBe("warning");
  expect(gates.find((gate) => gate.id === "validation")?.state).toBe("warning");
  expect(gates.find((gate) => gate.id === "mapping")?.state).toBe("warning");
  expect(gates.find((gate) => gate.id === "compatibility")?.state).toBe("warning");
});

it("validation of an older published revision cannot certify the current draft", () => {
  const gates = knowledgeReadiness({ assetType: "metric", revisionId: "new", content: {}, sections: [
    { kind: "validation", authority: "validation_runs", availability: "available", revisionId: "old", values: { runCount: 2, blockerCount: 0, warningCount: 0 }, records: [], recordsPage: { limit: 100, total: 0 } },
  ] });
  expect(gates.find((gate) => gate.id === "validation")?.state).toBe("warning");
});

it.each(["queued", "running", "failed", "missing", "partial"])("%s validation cannot certify readiness", (status) => {
  const section: AssetAuthoritySection = { kind: "validation", authority: "validation_runs", availability: "available", revisionId: "current", values: { runCount: 2, blockerCount: 0, warningCount: 0 }, records: status === "missing" ? [] : [{ kind: "validation_run", id: "run1", authority: "validation_runs", status: status === "partial" ? "succeeded" : status, label: "结构", version: 1 }], recordsPage: { limit: 100, total: 2 } };
  const gates = knowledgeReadiness({ assetType: "metric", revisionId: "current", content: {}, sections: [section] });
  expect(gates.find((gate) => gate.id === "validation")?.state).toBe("warning");
});

it("only complete successful current revision validation passes", () => {
  const section: AssetAuthoritySection = { kind: "validation", authority: "validation_runs", availability: "available", revisionId: "current", values: { runCount: 1, blockerCount: 0, warningCount: 0 }, records: [{ kind: "validation_run", id: "run1", authority: "validation_runs", status: "succeeded", label: "结构", version: 1 }], recordsPage: { limit: 100, total: 1 } };
  expect(knowledgeReadiness({ assetType: "metric", revisionId: "current", content: {}, sections: [section] }).find((gate) => gate.id === "validation")?.state).toBe("passed");
});

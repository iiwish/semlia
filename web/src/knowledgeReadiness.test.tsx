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

it("judges physical implementation by knowledge type", () => {
  const passedValidation: AssetAuthoritySection = { kind: "validation", authority: "production_validation_seals", availability: "available", revisionId: "current", values: { runCount: 1, blockerCount: 0, warningCount: 0 }, records: [{ kind: "validation_run", id: "run1", authority: "production_validation_seals", status: "succeeded", label: "schema", version: 0 }], recordsPage: { limit: 100, total: 1 } };
  const released: AssetAuthoritySection = { kind: "released_state", authority: "release_assets", availability: "available", revisionId: "current", releaseId: "rls", values: {}, records: [], recordsPage: { limit: 100, total: 0 } };
  const contained: AssetAuthoritySection = { kind: "relations", authority: "semantic_relations", availability: "available", values: { relationCount: 1 }, records: [{ kind: "relation", id: "rel1", authority: "spec_projection", status: "projected", label: "contains", version: 0, relation: { predicate: "contains", direction: "incoming", inferenceRule: "spec_reference:spec.metricRefs" } as NonNullable<AssetAuthoritySection["records"][number]["relation"]> }], recordsPage: { limit: 100, total: 1 } };
  const mapping = (assetType: Parameters<typeof knowledgeReadiness>[0]["assetType"], content: Record<string, unknown>, extra: AssetAuthoritySection[] = []) => knowledgeReadiness({ assetType, revisionId: "current", content, sections: [passedValidation, released, ...extra] }).find((gate) => gate.id === "mapping")?.state;
  expect(mapping("metric", { spec: { kind: "aggregate" } })).toBe("warning");
  expect(mapping("metric", { spec: { kind: "aggregate" } }, [contained])).toBe("passed");
  const filteredByModel = { ...contained, records: [{ ...contained.records[0], relation: { predicate: "filters_by", direction: "incoming", inferenceRule: "spec_reference:spec.compatibleTermRefs" } as NonNullable<AssetAuthoritySection["records"][number]["relation"]> }] };
  const filteredByMetric = { ...contained, records: [{ ...contained.records[0], relation: { predicate: "filters_by", direction: "incoming", inferenceRule: "spec_reference:spec.filterRefs" } as NonNullable<AssetAuthoritySection["records"][number]["relation"]> }] };
  expect(mapping("business_term", { spec: { capability: "predicate" } }, [filteredByModel])).toBe("passed");
  expect(mapping("business_term", { spec: { capability: "predicate" } }, [filteredByMetric])).toBe("warning");
  expect(mapping("analysis_model", { spec: { memberBindings: [{}], dataAssetRefs: [{}] } })).toBe("passed");
  expect(mapping("analysis_model", { spec: { memberBindings: [], dataAssetRefs: [{}] } })).toBe("warning");
  expect(mapping("data_asset", { spec: {} }, [contained])).toBe("warning");
  expect(mapping("data_asset", { spec: {} }, [{ kind: "physical_bindings", authority: "release_object_snapshots", availability: "available", values: { bindingCount: 1 }, records: [], recordsPage: { limit: 100, total: 0 } }])).toBe("passed");
});

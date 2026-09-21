import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { useSemanticProduction } from "./semanticProductionRuntime";
import { buildProductionInput, makeAssetTarget, ProductionApiError, type ProductionAPI, type ProductionOperation } from "./semanticProduction";

const operation = (id = "operation-a", version = 1): ProductionOperation => ({
  summary: { id, currentVersion: version, createdBy: "author", createdAt: "2026-09-11", updatedAt: "2026-09-11", frozen: false, progress: "draft", targetCount: 1, releaseId: null },
  version, input: { snapshots: [], candidates: [], evidence: [], dependencies: [] }, inputDigest: "input", setDigest: "set", baselineHead: { presence: "absent" }, targets: [], activeValidation: { status: "not_requested" }, generationRunIds: [], generationApplications: [], unresolvedCodes: [],
});
function apiFor(overrides: Partial<ProductionAPI> = {}): ProductionAPI {
  return { list: vi.fn().mockResolvedValue({ items: [], nextCursor: null }), get: vi.fn().mockResolvedValue(operation()), rules: vi.fn().mockResolvedValue([]), generation: vi.fn(), create: vi.fn(), replace: vi.fn(), submit: vi.fn(), validate: vi.fn(), review: vi.fn(), publish: vi.fn(), release: vi.fn(), rollback: vi.fn(), recordRule: vi.fn(), generate: vi.fn(), ...overrides };
}
const deferred = <T,>() => { let resolve!: (value: T) => void; const promise = new Promise<T>((done) => { resolve = done; }); return { promise, resolve }; };

describe("semantic production authority", () => {
  it("keeps a detail failure visible when the concurrent list succeeds later", async () => {
    const listed = deferred<{ items: ReturnType<typeof operation>["summary"][]; nextCursor: null }>();
    const api = apiFor({ list: vi.fn().mockReturnValue(listed.promise), get: vi.fn().mockRejectedValue(new ProductionApiError("detail unavailable", "REQUEST_FAILED", 500)) });
    const { result } = renderHook(() => useSemanticProduction({ workspaceId: "workspace", identityKey: "author:1", operationId: "operation-a", api }));
    await waitFor(() => expect(result.current.error).toContain("detail unavailable"));
    await act(async () => { listed.resolve({ items: [operation().summary], nextCursor: null }); });
    expect(result.current.error).toContain("detail unavailable");
    expect(result.current.detail).toBeNull();
  });
  it("rejects a release URL belonging to a different production root", async () => {
    window.history.replaceState({}, "", "/governance?production=operation-a&productionRelease=foreign");
    try {
      const published = operation(); published.summary.releaseId = "original";
      const api = apiFor({ get: vi.fn().mockResolvedValue(published), release: vi.fn().mockResolvedValue({ id: "foreign", protection: { rootReleaseId: "other-root", rollbackDepth: 0 }, projectionStatus: "ready" }) });
      const { result } = renderHook(() => useSemanticProduction({ workspaceId: "workspace", identityKey: "publisher:1", operationId: "operation-a", api }));
      await waitFor(() => expect(result.current.loading).toBe(false));
      await waitFor(() => expect(result.current.error).toContain("发布记录与生产集合不匹配"));
      expect(result.current.release).toBeNull();
    } finally { window.history.replaceState({}, "", "/"); }
  });
  it("retains the returned rollback release across refresh instead of reopening the original release", async () => {
    const published = operation();
    published.summary.releaseId = "original";
    const api = apiFor({ get: vi.fn().mockResolvedValue(published), release: vi.fn().mockImplementation(async (_workspace, id) => ({ id, protection: { rootReleaseId: "original", rollbackDepth: id === "original" ? 0 : 1 }, projectionStatus: "ready" })) });
    const { result } = renderHook(() => useSemanticProduction({ workspaceId: "workspace", identityKey: "publisher:1", operationId: "operation-a", api }));
    await waitFor(() => expect(result.current.release?.id).toBe("original"));
    await act(async () => { await result.current.write("回滚发布", async () => ({ releaseId: "rollback" })); });
    expect(result.current.release?.id).toBe("rollback");
    await act(async () => { await result.current.refresh(); });
    expect(result.current.release?.id).toBe("rollback");
  });

  it("builds a cold-start target without requiring an existing asset or approving a rule", () => {
    const target = makeAssetTarget("orders", "销售订单", "commerce.orders", "business_object", "author");
    expect(target).toMatchObject({ intent: "create", identityKey: "commerce.orders", content: { definition: null, scope: null, ownerPrincipalId: "author" }, evidenceIds: [], changes: [] });
    expect(target).not.toHaveProperty("targetId");
    expect(target).not.toHaveProperty("baseRevisionId");
    const snapshot = { id: "snapshot", sourceId: "source", sourceRevisionId: "revision", adapterVersion: "1", scopeDigest: "scope", contentDigest: "digest", historyQuality: "verified" as const, coverageStatus: "complete" as const, coverage: [{ key: "orders", status: "complete" as const, enumerationComplete: true, diagnosticCodes: [] }], memberCount: 0, diagnosticCount: 0, createdAt: "2026-09-11" };
    expect(buildProductionInput(snapshot, { id: "candidate", contentDigest: "candidate-digest" }, ["orders"], "orders")).toEqual({ snapshots: [{ sourceId: "source", snapshotId: "snapshot", digest: "digest", coverageKeys: ["orders"] }], candidates: [{ candidateId: "candidate", snapshotId: "snapshot", digest: "candidate-digest", targetKeys: ["orders"], primaryTargetKey: "orders" }], evidence: [], dependencies: [] });
    expect(() => buildProductionInput({ ...snapshot, historyQuality: "unverifiable" }, { id: "candidate", contentDigest: "candidate-digest" }, ["orders"], "orders")).toThrow();
  });

  it("recovers server operations after localStorage is cleared and never creates on mount", async () => {
    localStorage.clear();
    const api = apiFor({ list: vi.fn().mockResolvedValue({ items: [operation().summary], nextCursor: null }) });
    const { result } = renderHook(() => useSemanticProduction({ workspaceId: "workspace", identityKey: "author:1", candidateId: "candidate", api }));
    await waitFor(() => expect(result.current.items).toHaveLength(1));
    expect(api.list).toHaveBeenCalledWith("workspace", expect.objectContaining({ candidateId: "candidate" }), expect.any(AbortSignal));
    expect(api.create).not.toHaveBeenCalled();
    await act(async () => { await result.current.open("operation-a"); });
    expect(result.current.detail?.summary.id).toBe("operation-a");
  });

  it("keeps an uncertain write for explicit same-key retry and does not regenerate automatically", async () => {
    const create = vi.fn().mockRejectedValueOnce(new TypeError("network lost")).mockResolvedValueOnce({ operationId: "operation-a" });
    const api = apiFor({ create });
    const { result } = renderHook(() => useSemanticProduction({ workspaceId: "workspace", identityKey: "author:1", api }));
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => { await result.current.write("保存草稿", (key) => api.create("workspace", { input: operation().input, targets: [] }, key)); });
    expect(result.current.uncertain).toBe(true);
    expect(create).toHaveBeenCalledTimes(1);
    expect(api.generate).not.toHaveBeenCalled();
    await act(async () => { await result.current.retry(); });
    expect(create).toHaveBeenCalledTimes(2);
    expect(create.mock.calls[1][2]).toEqual(create.mock.calls[0][2]);
    expect(result.current.detail?.summary.id).toBe("operation-a");
    expect(result.current.uncertain).toBe(false);
  });

  it("discards reverse-order detail results and clears protected data when identity changes", async () => {
    const old = deferred<ProductionOperation>();
    const api = apiFor({ get: vi.fn().mockImplementation((_workspace, id) => id === "old" ? old.promise : Promise.resolve(operation(id))) });
    const { result, rerender } = renderHook(({ identityKey }) => useSemanticProduction({ workspaceId: "workspace", identityKey, api }), { initialProps: { identityKey: "author:1" } });
    await waitFor(() => expect(result.current.loading).toBe(false));
    let first!: Promise<void>;
    act(() => { first = result.current.open("old"); });
    await act(async () => { await result.current.open("new"); });
    await act(async () => { old.resolve(operation("old")); await first; });
    expect(result.current.detail?.summary.id).toBe("new");
    rerender({ identityKey: "reviewer:2" });
    expect(result.current.detail).toBeNull();
  });

  it("does not turn authorization failure into an empty success or a fixture", async () => {
    const api = apiFor({ list: vi.fn().mockRejectedValue(new ProductionApiError("无权读取", "FORBIDDEN", 403)) });
    const { result } = renderHook(() => useSemanticProduction({ workspaceId: "workspace", identityKey: "reader:1", api }));
    await waitFor(() => expect(result.current.error).toContain("无权执行"));
    expect(result.current.items).toEqual([]);
    expect(result.current.detail).toBeNull();
    expect(api.create).not.toHaveBeenCalled();
  });
});

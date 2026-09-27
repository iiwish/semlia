import { beforeEach, expect, it, vi } from "vitest";
import { getAsset, getAssetRevision } from "./catalog";
import { getProductionSnapshot, productionAPI, ProductionApiError, type ProductionOperation, type ProductionRelease } from "./semanticProduction";
import { createKnowledgeRevisionCommand, prepareKnowledgeProductionRevision, KnowledgeRevisionUncertainError } from "./knowledgeProductionRevision";
import type { KnowledgeRevisionSubmission } from "./types";

vi.mock("./catalog", () => ({ getAsset: vi.fn(), getAssetRevision: vi.fn() }));
vi.mock("./semanticProduction", async (original) => {
  const actual = await original<typeof import("./semanticProduction")>();
  return { ...actual, getProductionSnapshot: vi.fn(), productionAPI: { ...actual.productionAPI, get: vi.fn(), release: vi.fn(), create: vi.fn() } };
});
const digest = `sha256:${"a".repeat(64)}`;
const ref = { assetId: "ast_orders", revisionId: "rev_orders", releaseId: "rls_orders" };
const content = { address: "commerce.model", assetType: "analysis_model", displayName: "Model", definition: "Before", scope: "Synthetic", ownerPrincipalId: "prn_author", spec: { baseObjectRef: ref } };
const revision = { id: "rev_model", assetId: "ast_model", sequence: 2, schemaVersion: "1.0.0", contentDigest: digest, content, createdAt: "2026-09-01T00:00:00Z", createdBy: "prn_author", evidence: [] };
const input = { snapshots: [{ sourceId: "src_orders", snapshotId: "snp_orders", digest, coverageKeys: ["public"] }], candidates: [{ candidateId: "consumed", digest, snapshotId: "snp_orders", targetKeys: ["model"], primaryTargetKey: "model" }], evidence: [{ evidenceId: "evd_source", digest, snapshotId: "snp_orders" }], dependencies: [{ kind: "semantic_asset" as const, targetId: ref.assetId, revisionId: ref.revisionId, releaseId: ref.releaseId }] };
const submission = { assetId: "ast_model", baseRevision: "@2", baseRevisionId: "rev_model", title: "Revise model", summary: "Synthetic correction", reason: "Answer R1 requires correction", changes: [{ field: "definition", before: "Before", after: "After" }] } satisfies KnowledgeRevisionSubmission;
const operation = { summary: { id: "prodop_origin" }, version: 3, setDigest: digest, input, targets: [{ localKey: "model", targetId: "ast_model", outcome: "proposal", contentDigest: digest, proposalId: "prp_model", declaration: { kind: "semantic_asset", evidenceIds: ["evd_source"] } }] } as unknown as ProductionOperation;
function release(id: string, previous?: string, operationId = "prodop_origin", rollback = false): ProductionRelease {
  return { id, attribution: { operationId, version: 3, setDigest: digest, role: "applied", proposalIds: ["prp_model"] }, protection: { rootReleaseId: id, rollbackDepth: rollback ? 2 : 0 }, rolledBackReleaseId: rollback ? "rls_reverted" : null, beforeHead: previous ? { presence: "present", releaseId: previous } : { presence: "absent" }, afterManifest: { assets: [{ assetId: "ast_model", revisionId: "rev_model" }] } } as unknown as ProductionRelease;
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(getAsset).mockResolvedValue({ id: "ast_model", address: content.address, assetType: "analysis_model", currentRevisionId: revision.id, currentRevision: revision, authoritySections: [{ kind: "released_state", availability: "available", releaseId: "rls_origin", revisionId: revision.id }] } as unknown as Awaited<ReturnType<typeof getAsset>>);
  vi.mocked(getAssetRevision).mockResolvedValue(structuredClone(revision));
  vi.mocked(productionAPI.release).mockResolvedValue(release("rls_origin"));
  vi.mocked(productionAPI.get).mockResolvedValue(structuredClone(operation));
  vi.mocked(getProductionSnapshot).mockResolvedValue({ id: "snp_orders", sourceId: "src_orders", contentDigest: digest, historyQuality: "verified", coverage: [{ key: "public", status: "complete", enumerationComplete: true }] } as Awaited<ReturnType<typeof getProductionSnapshot>>);
  vi.mocked(productionAPI.create).mockResolvedValue({ operationId: "prodop_correction" } as Awaited<ReturnType<typeof productionAPI.create>>);
});

it("creates an exact update draft with source evidence, no consumed candidates and no business confirmation", async () => {
  const draft = await prepareKnowledgeProductionRevision("wsp_a", submission);
  expect(draft.input).toEqual({ ...input, candidates: [] });
  expect(draft.targets).toEqual([{ intent: "update", kind: "semantic_asset", localKey: "revision", title: "Model", targetId: "ast_model", baseRevisionId: "rev_model", content: { ...content, definition: "After" }, changes: [{ fieldPath: "definition", op: "update", beforeValue: "Before", afterValue: "After" }], evidenceIds: ["evd_source"] }]);
  expect(draft).not.toHaveProperty("supersedesOperationId");
  expect(productionAPI.get).toHaveBeenCalledWith("wsp_a", "prodop_origin", undefined, 3);
});

it("traverses rollback and an unrelated head instead of stealing their source input", async () => {
  const current = await getAsset("wsp_a", "ast_model");
  current.authoritySections[0].releaseId = "rls_restored";
  vi.mocked(productionAPI.release).mockImplementation(async (_workspace, id) => id === "rls_restored" ? release(id, "rls_other", "prodop_wrong", true) : id === "rls_other" ? release(id, "rls_origin", "prodop_other") : release(id));
  vi.mocked(productionAPI.get).mockImplementation(async (_workspace, id) => id === "prodop_other" ? { ...operation, summary: { ...operation.summary, id }, targets: [{ ...operation.targets[0], targetId: "ast_other" }] } : operation);
  expect((await prepareKnowledgeProductionRevision("wsp_a", submission)).input.snapshots).toEqual(input.snapshots);
  expect(productionAPI.get).not.toHaveBeenCalledWith("wsp_a", "prodop_wrong", undefined, 3);
});

it.each(["stale", "unpublished draft", "wrong before", "wrong asset", "wrong revision digest", "wrong operation version", "denied source", "incomplete source", "cycle"])("rejects %s before creating a command", async (failure) => {
  const edit = structuredClone(submission);
  const detail = await getAsset("wsp_a", "ast_model");
  if (failure === "stale") edit.baseRevisionId = "rev_old";
  if (failure === "unpublished draft") detail.authoritySections[0].revisionId = "rev_published";
  if (failure === "wrong before") edit.changes[0].before = "Not the baseline";
  if (failure === "wrong asset") vi.mocked(getAssetRevision).mockResolvedValue({ ...revision, assetId: "ast_other" });
  if (failure === "wrong revision digest") vi.mocked(getAssetRevision).mockResolvedValue({ ...revision, contentDigest: "wrong" });
  if (failure === "wrong operation version") vi.mocked(productionAPI.get).mockResolvedValue({ ...operation, version: 4 });
  if (failure === "denied source") vi.mocked(getProductionSnapshot).mockRejectedValue(new ProductionApiError("Forbidden", "FORBIDDEN", 403));
  if (failure === "incomplete source") vi.mocked(getProductionSnapshot).mockResolvedValue({ ...(await getProductionSnapshot("wsp_a", "src_orders", "snp_orders")), coverage: [] });
  if (failure === "cycle") vi.mocked(productionAPI.release).mockResolvedValue(release("rls_origin", "rls_origin", "ignored", true));
  await expect(prepareKnowledgeProductionRevision("wsp_a", edit)).rejects.toThrow();
  expect(productionAPI.create).not.toHaveBeenCalled();
});

it("does not rebase conflicting dependency pins to latest versions", async () => {
  const edit: KnowledgeRevisionSubmission = { ...submission, changes: [{ field: "spec", before: content.spec, after: { baseObjectRef: { ...ref, revisionId: "rev_other" } } }] };
  await expect(prepareKnowledgeProductionRevision("wsp_a", edit)).rejects.toThrow(/依赖/);
});

it("bounds an otherwise unending release chain to 64 reads", async () => {
  let next = 0;
  vi.mocked(productionAPI.release).mockImplementation(async (_workspace, id) => release(id, `rls_${++next}`, "ignored", true));
  await expect(prepareKnowledgeProductionRevision("wsp_a", submission)).rejects.toThrow(/64/);
  expect(productionAPI.release).toHaveBeenCalledTimes(64);
  expect(productionAPI.create).not.toHaveBeenCalled();
});

it.each(["digest", "attribution"])("rejects a real producer with mismatched %s rather than falling back", async (failure) => {
  vi.mocked(productionAPI.get).mockResolvedValue({ ...operation, targets: [{ ...operation.targets[0], ...(failure === "digest" ? { contentDigest: "wrong" } : { proposalId: "prp_other" }) }] });
  await expect(prepareKnowledgeProductionRevision("wsp_a", submission)).rejects.toThrow(/不匹配/);
  expect(productionAPI.create).not.toHaveBeenCalled();
});

it("skips an exact no-change target in a mixed release and locates its actual producer", async () => {
  (await getAsset("wsp_a", "ast_model")).authoritySections[0].releaseId = "rls_mixed";
  vi.mocked(productionAPI.release).mockImplementation(async (_workspace, id) => id === "rls_mixed" ? release(id, "rls_origin", "prodop_mixed") : release(id));
  vi.mocked(productionAPI.get).mockImplementation(async (_workspace, id) => id === "prodop_mixed" ? { ...operation, summary: { ...operation.summary, id }, input: { ...input, snapshots: [] }, targets: [{ ...operation.targets[0], outcome: "no_change", proposalId: null }] } : operation);
  expect((await prepareKnowledgeProductionRevision("wsp_a", submission)).input.snapshots).toEqual(input.snapshots);
  expect(productionAPI.get).toHaveBeenLastCalledWith("wsp_a", "prodop_origin", undefined, 3);
});

it("shares one in-flight create and retries uncertain delivery with the exact frozen command", async () => {
  let reject!: (error: Error) => void;
  vi.mocked(productionAPI.create).mockReturnValueOnce(new Promise((_resolve, rejectPromise) => { reject = rejectPromise; }));
  const edit = structuredClone(submission);
  const command = createKnowledgeRevisionCommand("wsp_a", edit);
  const first = command.run();
  expect(command.run()).toBe(first);
  await vi.waitFor(() => expect(productionAPI.create).toHaveBeenCalledOnce());
  const [workspace, body, key] = vi.mocked(productionAPI.create).mock.calls[0];
  edit.changes[0].after = "Mutated unsent body";
  reject(new TypeError("Connection lost"));
  await expect(first).rejects.toBeInstanceOf(KnowledgeRevisionUncertainError);
  await command.run();
  expect(productionAPI.create).toHaveBeenLastCalledWith(workspace, body, key, undefined);
  expect((body.targets[0].content as { definition: string }).definition).toBe("After");
  expect(getAsset).toHaveBeenCalledTimes(1);
  expect(command.submission.changes[0].after).toBe("After");
});

it("does not issue a create after the owning identity aborts during preparation", async () => {
  let finish!: (value: Awaited<ReturnType<typeof getAsset>>) => void;
  const detail = await getAsset("wsp_a", "ast_model");
  vi.mocked(getAsset).mockReturnValueOnce(new Promise(resolve => { finish = resolve; }));
  const controller = new AbortController();
  const command = createKnowledgeRevisionCommand("wsp_a", submission, controller.signal);
  const pending = command.run();
  controller.abort(); finish(detail);
  await expect(pending).rejects.toThrow();
  expect(productionAPI.create).not.toHaveBeenCalled();
});

it("keeps an uncertain command frozen after a denied retry and recovers with the same key", async () => {
  vi.mocked(productionAPI.create).mockRejectedValueOnce(new TypeError("response lost"))
    .mockRejectedValueOnce(new ProductionApiError("Permission revoked", "FORBIDDEN", 403));
  const command = createKnowledgeRevisionCommand("wsp_a", submission);
  await expect(command.run()).rejects.toBeInstanceOf(KnowledgeRevisionUncertainError);
  await expect(command.run()).rejects.toBeInstanceOf(KnowledgeRevisionUncertainError);
  await expect(command.run()).resolves.toMatchObject({ operationId: "prodop_correction" });
  expect(productionAPI.create).toHaveBeenCalledTimes(3);
  expect(vi.mocked(productionAPI.create).mock.calls[1]).toEqual(vi.mocked(productionAPI.create).mock.calls[0]);
  expect(vi.mocked(productionAPI.create).mock.calls[2]).toEqual(vi.mocked(productionAPI.create).mock.calls[0]);
  expect(getAsset).toHaveBeenCalledTimes(1);
});

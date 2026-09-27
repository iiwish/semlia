import { useMemo, useState, type ContextType, type ReactNode } from "react";
import { CatalogRuntimeContext, revisionCacheKey } from "../catalogRuntime";
import type { Asset } from "../types";
import type { CatalogRevision, SemanticAssetType } from "../catalog";
import { knowledgeTypes } from "../knowledge";

const workspace = { id: "wsp_01arz3ndektsv4rrffq69g5fav", slug: "product-test", displayName: "产品测试工作区 · 模拟数据", createdAt: "2026-09-02T00:00:00Z", updatedAt: "2026-09-02T00:00:00Z" };
const noop = () => {};
const resolved = async () => {};
const unsupported = async (): Promise<never> => { throw new Error("Use the real Catalog API tests for mutations"); };

export function CatalogRuntimeProvider({ children, fixtureAssets, fixtureRevisions = [] }: { children: ReactNode; fixtureAssets: Asset[]; fixtureRevisions?: CatalogRevision[] }) {
  const [query, setQuery] = useState("");
  const [assetType, setAssetType] = useState<SemanticAssetType | "">("");
  // Synthetic substring search is a fixture boundary, not a production FTS implementation.
  const catalogAssets = useMemo(() => fixtureAssets.filter((asset) => (!assetType || asset.type === knowledgeTypes[assetType]) && [
    asset.name, asset.key, asset.identity.key, asset.domain, asset.definition, ...asset.aliases,
    ...asset.relations.map((relation) => `${relation.id} ${relation.targetName} ${relation.type}`),
    ...asset.bindings.map((binding) => `${binding.id} ${binding.dataset} ${binding.sourceRevision}`),
    ...asset.joinContracts.map((join) => `${join.id} ${join.target} ${join.cardinality}`),
  ].join(" ").toLowerCase().includes(query.trim().toLowerCase())), [assetType, fixtureAssets, query]);
  const value = useMemo<NonNullable<ContextType<typeof CatalogRuntimeContext>>>(() => ({
    workspaces: [workspace], workspaceId: workspace.id, workspace, assets: fixtureAssets,
    catalogAssetIds: catalogAssets.map(item => item.id), loading: false, error: "", query, assetType,
    total: catalogAssets.length, loadingMore: false, appendError: "", detailStates: {},
    revisionStates: Object.fromEntries(fixtureAssets.map(item => [item.id, { state: "ready", error: "", items: [], total: 0, loadingMore: false, appendError: "" }])),
    exactRevisionStates: Object.fromEntries([...fixtureAssets.map(item => [revisionCacheKey(workspace.id, item.id, item.revisionRecord.revisionId), { state: "ready", error: "", revision: { id: item.revisionRecord.revisionId, assetId: item.id, sequence: item.revisionRecord.sequence, schemaVersion: "1.0.0", contentDigest: item.revisionRecord.contentHash, content: { displayName: item.name, spec: item.knowledgeSpec }, createdAt: workspace.createdAt, createdBy: "synthetic", evidence: [] } }]), ...fixtureRevisions.map(revision => [revisionCacheKey(workspace.id, revision.assetId, revision.id), { state: "ready", error: "", revision }])]),
    authorityPageStates: {}, setWorkspaceId: noop,
    setQuery: (nextQuery, nextType) => { setQuery(nextQuery); setAssetType(nextType); },
    ensureAsset: async (id) => fixtureAssets.find(item => item.id === id), ensureRevision: async () => undefined, ensureRevisions: resolved, loadMoreAssets: resolved,
    loadMoreRevisions: resolved, loadMoreAuthorityRecords: resolved,
    createWorkspace: unsupported, createAsset: unsupported, refresh: noop,
  }), [assetType, catalogAssets, fixtureAssets, fixtureRevisions, query]);
  return <CatalogRuntimeContext.Provider value={value}>{children}</CatalogRuntimeContext.Provider>;
}

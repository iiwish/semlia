import { useMemo, useState, type ContextType, type ReactNode } from "react";
import { CatalogRuntimeContext } from "../catalogRuntime";
import type { Asset } from "../types";
import type { SemanticAssetType } from "../catalog";

const workspace = { id: "wsp_01arz3ndektsv4rrffq69g5fav", slug: "product-test", displayName: "产品测试工作区 · 模拟数据", createdAt: "2026-09-02T00:00:00Z", updatedAt: "2026-09-02T00:00:00Z" };
const noop = () => {};
const resolved = async () => {};
const unsupported = async (): Promise<never> => { throw new Error("Use the real Catalog API tests for mutations"); };

export function CatalogRuntimeProvider({ children, fixtureAssets }: { children: ReactNode; fixtureAssets: Asset[] }) {
  const [query, setQuery] = useState("");
  const [assetType, setAssetType] = useState<SemanticAssetType | "">("");
  const value = useMemo<NonNullable<ContextType<typeof CatalogRuntimeContext>>>(() => ({
    workspaces: [workspace], workspaceId: workspace.id, workspace, assets: fixtureAssets,
    catalogAssetIds: fixtureAssets.map(item => item.id), loading: false, error: "", query, assetType,
    total: fixtureAssets.length, loadingMore: false, appendError: "", detailStates: {},
    revisionStates: Object.fromEntries(fixtureAssets.map(item => [item.id, { state: "ready", error: "", items: [], total: 0, loadingMore: false, appendError: "" }])),
    authorityPageStates: {}, setWorkspaceId: noop,
    setQuery: (nextQuery, nextType) => { setQuery(nextQuery); setAssetType(nextType); },
    ensureAsset: resolved, ensureRevisions: resolved, loadMoreAssets: resolved,
    loadMoreRevisions: resolved, loadMoreAuthorityRecords: resolved,
    createWorkspace: unsupported, createAsset: unsupported, refresh: noop,
  }), [assetType, fixtureAssets, query]);
  return <CatalogRuntimeContext.Provider value={value}>{children}</CatalogRuntimeContext.Provider>;
}

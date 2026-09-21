import type { components } from "@semlia/sdk-typescript";

import { apiClient as client } from "./apiClient";

export type Workspace = components["schemas"]["Workspace"];
export type CatalogPage = components["schemas"]["CatalogPage"];
export type CatalogAsset = components["schemas"]["CatalogAssetSummary"];
export type CatalogAssetDetail = components["schemas"]["CatalogAssetDetail"];
export type CatalogAuthorityRecordPage = components["schemas"]["CatalogAuthorityRecordPage"];
export type CatalogAuthoritySectionKind = "relations" | "physical_bindings" | "join_contracts" | "validation" | "lineage" | "consumer_impact";
export type CatalogRelation = components["schemas"]["AssetRelation"];
export type CatalogRelationPage = components["schemas"]["AssetRelationPage"];
export type CatalogRevision = components["schemas"]["AssetRevision"];
export type CatalogRevisionPage = components["schemas"]["AssetRevisionPage"];
export type SemanticAssetType = components["schemas"]["SemanticAssetType"];

export async function listWorkspaces(signal?: AbortSignal): Promise<Workspace[]> {
  const response = await client.GET("/api/v1/workspaces", { signal });
  if (!response.data) throw new Error(errorMessage(response.error));
  return response.data.items;
}

export async function createWorkspace(slug: string, displayName: string): Promise<Workspace> {
  const response = await client.POST("/api/v1/workspaces", {
    body: { slug, displayName },
  });
  if (!response.data) throw new Error(errorMessage(response.error));
  return response.data;
}

export async function listAssets(
  workspaceId: string,
  search: string,
  assetType: SemanticAssetType | "",
  cursor?: string,
  signal?: AbortSignal,
): Promise<CatalogPage> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/catalog/assets", {
    params: {
      path: { workspaceId },
      query: {
        limit: 100,
        ...(cursor ? { cursor } : {}),
        ...(search ? { search } : {}),
        ...(assetType ? { assetType } : {}),
      },
    },
    signal,
  });
  if (!response.data) throw new Error(errorMessage(response.error));
  return response.data;
}

export async function listAssetAuthorityRecords(
  workspaceId: string,
  assetId: string,
  sectionKind: CatalogAuthoritySectionKind,
  cursor?: string,
  signal?: AbortSignal,
): Promise<CatalogAuthorityRecordPage> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/authority/{sectionKind}", {
    params: {
      path: { workspaceId, assetId, sectionKind },
      query: { limit: 100, ...(cursor ? { cursor } : {}) },
    },
    signal,
  });
  if (!response.data) throw new Error(errorMessage(response.error));
  return response.data;
}

export async function getAsset(workspaceId: string, assetId: string, signal?: AbortSignal): Promise<CatalogAssetDetail> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}", {
    params: { path: { workspaceId, assetId } },
    signal,
  });
  if (!response.data) throw new Error(errorMessage(response.error));
  return response.data;
}

export async function listAssetRelations(workspaceId: string, assetId: string, signal?: AbortSignal): Promise<CatalogRelationPage> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/relations", {
    params: {
      path: { workspaceId, assetId },
      query: { direction: "both", depth: 1 },
    },
    signal,
  });
  if (!response.data) throw new Error(errorMessage(response.error));
  return response.data;
}

export async function listAssetRevisions(workspaceId: string, assetId: string, cursor?: string, signal?: AbortSignal): Promise<CatalogRevisionPage> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/catalog/assets/{assetId}/revisions", {
    params: {
      path: { workspaceId, assetId },
      query: { limit: 100, ...(cursor ? { cursor } : {}) },
    },
    signal,
  });
  if (!response.data) throw new Error(errorMessage(response.error));
  return response.data;
}

export async function createAsset(
  workspaceId: string,
  input: { address: string; assetType: SemanticAssetType; title: string; summary: string; scope?: string; ownerPrincipalId?: string; spec?: import("./knowledge").KnowledgeSpec },
): Promise<CatalogAssetDetail> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/catalog/assets", {
    params: { path: { workspaceId } },
    body: {
      address: input.address,
      assetType: input.assetType,
      lifecycleState: "draft",
      schemaVersion: "1.0.0",
      createdBy: "catalog-web",
      content: { address: input.address, assetType: input.assetType, displayName: input.title, definition: input.summary, scope: input.scope || null, ownerPrincipalId: input.ownerPrincipalId, spec: input.spec },
    },
  });
  if (!response.data) throw new Error(errorMessage(response.error));
  return response.data;
}

function errorMessage(value: unknown): string {
  if (value && typeof value === "object" && "message" in value && typeof value.message === "string") {
    return value.message;
  }
  return "The catalog request could not be completed.";
}

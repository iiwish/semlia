import { createSemliaClient, type components } from "@semlia/sdk-typescript";

export type Workspace = components["schemas"]["Workspace"];
export type CatalogPage = components["schemas"]["CatalogPage"];
export type CatalogAsset = components["schemas"]["CatalogAssetSummary"];
export type CatalogAssetDetail = components["schemas"]["CatalogAssetDetail"];
export type SemanticAssetType = components["schemas"]["SemanticAssetType"];

const client = createSemliaClient({ baseUrl: "" });

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
  signal?: AbortSignal,
): Promise<CatalogPage> {
  const response = await client.GET("/api/v1/workspaces/{workspaceId}/catalog/assets", {
    params: {
      path: { workspaceId },
      query: {
        limit: 100,
        ...(search ? { search } : {}),
        ...(assetType ? { assetType } : {}),
      },
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

export async function createAsset(
  workspaceId: string,
  input: { address: string; assetType: SemanticAssetType; title: string; summary: string },
): Promise<CatalogAssetDetail> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/catalog/assets", {
    params: { path: { workspaceId } },
    body: {
      address: input.address,
      assetType: input.assetType,
      lifecycleState: "active",
      schemaVersion: "1.0.0",
      createdBy: "catalog-web",
      content: { title: input.title, summary: input.summary },
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

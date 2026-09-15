import type { components } from "@semlia/sdk-typescript";

import { apiClient as client, sessionCSRFToken } from "./apiClient";

export type AskRequest = components["schemas"]["AskRequest"];
export type AskResponse = components["schemas"]["AskResponse"];
export type ReleasedDefinition = components["schemas"]["ReleasedDefinition"];

export class AskApiError extends Error {
  readonly code: string;

  constructor(message: string, code = "REQUEST_FAILED") {
    super(message);
    this.name = "AskApiError";
    this.code = code;
  }
}

export async function askReleasedSemantics(workspaceId: string, input: AskRequest): Promise<AskResponse> {
  const response = await client.POST("/api/v1/workspaces/{workspaceId}/ask", {
    params: { path: { workspaceId }, header: { "X-Semlia-CSRF": sessionCSRFToken() } },
    body: input,
  });
  if (response.data) return response.data;
  const error = response.error as { code?: string; message?: string } | undefined;
  throw new AskApiError(error?.message ?? "语义问答请求失败。", error?.code);
}

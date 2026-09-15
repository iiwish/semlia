import type { components } from "@semlia/sdk-typescript";
import { apiClient, sessionCSRFToken } from "./apiClient";

export type ExecutionResult = components["schemas"]["QueryExecutionResult"];
export type ExecutionPlan = components["schemas"]["ResolvedSemanticPlan"];

export async function executePlan(workspaceId: string, plan: ExecutionPlan, idempotencyKey: string, signal?: AbortSignal): Promise<ExecutionResult> {
  const response = await apiClient.POST("/api/v1/workspaces/{workspaceId}/resolved-semantic-plans/{planId}:execute", {
    params: { path: { workspaceId, planId: plan.id }, header: { "X-Semlia-CSRF": sessionCSRFToken() } },
    body: { planId: plan.id, planDigest: plan.planDigest, idempotencyKey, channel: "web" }, signal,
  });
  if (response.data) return response.data;
  throw new Error(response.error?.code ?? "EXECUTION_REQUEST_FAILED");
}

export async function getExecution(workspaceId: string, runId: string, signal?: AbortSignal): Promise<ExecutionResult> {
  const response = await apiClient.GET("/api/v1/workspaces/{workspaceId}/query-executions/{runId}", { params: { path: { workspaceId, runId } }, signal });
  if (response.data) return response.data;
  throw new Error(response.error?.code ?? "EXECUTION_REQUEST_FAILED");
}

export async function cancelExecution(workspaceId: string, runId: string): Promise<ExecutionResult> {
  const response = await apiClient.POST("/api/v1/workspaces/{workspaceId}/query-executions/{runId}:cancel", { params: { path: { workspaceId, runId }, header: { "X-Semlia-CSRF": sessionCSRFToken() } } });
  if (response.data) return response.data;
  throw new Error(response.error?.code ?? "EXECUTION_CANCEL_FAILED");
}

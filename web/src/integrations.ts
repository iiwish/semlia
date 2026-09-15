import type { components } from "@semlia/sdk-typescript";
import { apiClient, sessionCSRFToken } from "./apiClient";

export type ClientCredential = components["schemas"]["ClientCredentialSummary"];
export type CredentialInput = components["schemas"]["IssueClientCredential"];
export type IssuedCredential = components["schemas"]["IssuedCredential"];
export type Webhook = components["schemas"]["WebhookSubscription"];
export type WebhookInput = components["schemas"]["SaveWebhookSubscription"];
export type IssuedWebhook = components["schemas"]["IssuedWebhookSubscription"];
export type Delivery = components["schemas"]["WebhookDelivery"];
export type Consumer = components["schemas"]["Consumer"];
export type ConsumerBinding = components["schemas"]["ConsumerBinding"];
export interface IntegrationData { credentials: ClientCredential[]; consumers: Consumer[]; bindings: ConsumerBinding[]; webhooks: Webhook[]; deliveries: Delivery[]; webhookError?: string }
export class IntegrationPartialSuccess extends Error {}
export interface IntegrationApi {
  load(workspace: string, runtimeRead: boolean, signal?: AbortSignal): Promise<IntegrationData>;
  issue(workspace: string, input: CredentialInput): Promise<IssuedCredential>;
  rotate(workspace: string, id: string): Promise<IssuedCredential>;
  revoke(workspace: string, id: string): Promise<void>;
  createPrincipal(workspace: string, name: string): Promise<{ id: string; name: string }>;
  grant(workspace: string, principalId: string): Promise<void>;
  createConsumer(workspace: string, name: string, stableKey: string, environment: string): Promise<void>;
  createBinding(workspace: string, consumerId: string, environment: string): Promise<void>;
  saveWebhook(workspace: string, input: WebhookInput, id?: string): Promise<IssuedWebhook>;
  rotateWebhook(workspace: string, item: Webhook): Promise<IssuedWebhook>;
  replayDelivery(workspace: string, item: Delivery): Promise<Delivery>;
}
function requireData<T>(result: { data?: T; response: Response }): T {
  if (!result.response.ok || result.data === undefined) throw new Error(result.response.status === 403 ? "权限已变更，请刷新工作区后重试。" : `请求未完成（HTTP ${result.response.status}）`);
  return result.data;
}
const path = (workspaceId: string) => ({ path: { workspaceId } });
const protectedPath = (workspaceId: string) => ({ ...path(workspaceId), header: { "X-Semlia-CSRF": sessionCSRFToken() } });
export const integrationApi: IntegrationApi = {
  async load(workspaceId, runtimeRead, signal) {
    const params = path(workspaceId);
    const credentials = requireData(await apiClient.GET("/api/v1/workspaces/{workspaceId}/client-credentials", { params, signal })).items;
    const consumers = requireData(await apiClient.GET("/api/v1/workspaces/{workspaceId}/consumers", { params, signal })).items;
    const bindings = requireData(await apiClient.GET("/api/v1/workspaces/{workspaceId}/consumer-bindings", { params, signal })).items;
    let webhooks: Webhook[] = [], deliveries: Delivery[] = [], webhookError: string | undefined;
    try {
      webhooks = requireData(await apiClient.GET("/api/v1/workspaces/{workspaceId}/webhooks", { params, signal })).items;
      if (runtimeRead) deliveries = requireData(await apiClient.GET("/api/v1/workspaces/{workspaceId}/webhook-deliveries", { params, signal })).items;
    } catch (error) { webhookError = error instanceof Error ? error.message : "Webhook 不可用"; }
    return { credentials, consumers, bindings, webhooks, deliveries, ...(webhookError ? { webhookError } : {}) };
  },
  async issue(workspace, body) { return requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/client-credentials", { params: path(workspace), body })); },
  async rotate(workspaceId, credentialId) { return requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/client-credentials/{credentialId}/rotate", { params: { path: { workspaceId, credentialId } } })); },
  async revoke(workspaceId, credentialId) { const result = await apiClient.POST("/api/v1/workspaces/{workspaceId}/client-credentials/{credentialId}/revoke", { params: { path: { workspaceId, credentialId } } }); if (!result.response.ok) throw new Error("撤销凭据失败"); },
  async createPrincipal(workspace, name) { return requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/machine-principals", { params: path(workspace), body: { name } })); },
  async grant(workspace, principalId) { requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/authorization/role-bindings", { params: protectedPath(workspace), body: { principalId, roleId: "consumer_developer", expectedRoleVersion: 1, scope: { type: "workspace", id: workspace } } })); },
  async createConsumer(workspace, name, stableKey, environment) {
    const consumer = requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/consumers", { params: protectedPath(workspace), body: { name, stableKey, kind: "agent", metadata: {} } }));
    try { requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/consumer-bindings", { params: protectedPath(workspace), body: { consumerId: consumer.id, environment, purpose: "Released semantic consumption", mode: "current", compatibilityConstraint: {} } })); }
    catch { throw new IntegrationPartialSuccess(`消费方已创建（${consumer.id}），但绑定创建失败。请使用“补建绑定”，勿重复登记。`); }
  },
  async createBinding(workspace, consumerId, environment) { requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/consumer-bindings", { params: protectedPath(workspace), body: { consumerId, environment, purpose: "Released semantic consumption", mode: "current", compatibilityConstraint: {} } })); },
  async saveWebhook(workspaceId, body, subscriptionId) { return subscriptionId ? requireData(await apiClient.PATCH("/api/v1/workspaces/{workspaceId}/webhooks/{subscriptionId}", { params: { path: { workspaceId, subscriptionId } }, body })) : requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/webhooks", { params: path(workspaceId), body })); },
  async rotateWebhook(workspaceId, item) { return requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/webhooks/{subscriptionId}/rotate", { params: { path: { workspaceId, subscriptionId: item.id } }, body: { expectedVersion: item.version } })); },
  async replayDelivery(workspaceId, item) { return requireData(await apiClient.POST("/api/v1/workspaces/{workspaceId}/webhook-deliveries/{deliveryId}/replay", { params: { path: { workspaceId, deliveryId: item.id } }, body: { expectedAttempt: item.attempt } })); },
};

import createClient from "openapi-fetch";

import type { paths, components } from "./schema.gen";

export interface SemliaClientOptions {
  baseUrl: string;
  bearerToken?: string;
  fetch?: typeof globalThis.fetch;
  headers?: Record<string, string>;
  credentials?: RequestCredentials;
  getCSRFToken?: () => string | undefined;
  onCSRFToken?: (token: string) => void;
  onUnauthorized?: (response: Response, request: Request) => void;
  onForbidden?: (response: Response, request: Request) => void;
}

export function createSemliaClient(options: SemliaClientOptions) {
  const suppliedHeaders = new Headers(options.headers);
  if (options.bearerToken && ((options.credentials && options.credentials !== "omit") || options.getCSRFToken || suppliedHeaders.has("cookie") || suppliedHeaders.has("authorization"))) {
    throw new Error("Bearer credentials cannot be combined with browser authentication");
  }
  const client = createClient<paths>({
    baseUrl: options.baseUrl,
    credentials: options.bearerToken ? "omit" : options.credentials ?? "include",
    ...(options.fetch === undefined ? {} : { fetch: options.fetch }),
    headers: { ...options.headers, ...(options.bearerToken ? { Authorization: `Bearer ${options.bearerToken}` } : {}) },
  });
  client.use({
    onRequest({ request }) {
      if (!unsafeMethods.has(request.method)) return;
      const csrfToken = options.getCSRFToken?.();
      if (!csrfToken) return;
      const headers = new Headers(request.headers);
      headers.set("X-Semlia-CSRF", csrfToken);
      return new Request(request, { headers });
    },
    onResponse({ request, response }) {
      const csrfToken = response.headers.get("X-Semlia-CSRF");
      if (csrfToken) options.onCSRFToken?.(csrfToken);
      if (response.status === 401) options.onUnauthorized?.(response, request);
      if (response.status === 403) options.onForbidden?.(response, request);
    },
  });
  return client;
}

export type SemliaClient = ReturnType<typeof createSemliaClient>;

const unsafeMethods = new Set(["POST", "PUT", "PATCH", "DELETE"]);

export function createSourceSnapshotClient(options: SemliaClientOptions & { workspaceId: string; sourceId: string }) {
  const client = createSemliaClient(options);
  const sourcePath = { workspaceId: options.workspaceId, sourceId: options.sourceId };
  type Page = { limit?: number; cursor?: string };
  return {
    list: (query: Page = {}) => client.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots", { params: { path: sourcePath, query } }),
    get: (snapshotId: string) => client.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}", { params: { path: { ...sourcePath, snapshotId } } }),
    members: (snapshotId: string, query: Page & { kind?: components["schemas"]["MemberKind"] } = {}) => client.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}/members", { params: { path: { ...sourcePath, snapshotId }, query } }),
    diagnostics: (snapshotId: string, query: Page = {}) => client.GET("/api/v1/workspaces/{workspaceId}/sources/{sourceId}/snapshots/{snapshotId}/diagnostics", { params: { path: { ...sourcePath, snapshotId }, query } }),
  };
}

export function createSemanticClient(options: SemliaClientOptions & { workspaceId: string; bearerToken: string }) {
  const client = createSemliaClient(options);
  const params = { path: { workspaceId: options.workspaceId } };
  return {
    resolve: (query: components["schemas"]["SemanticQuery"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/semantic-queries:resolve", { params, body: { query, channel: "sdk", idempotencyKey } }),
    describe: (query: components["schemas"]["SemanticQuery"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/semantic-describe", { params, body: { query, channel: "sdk", idempotencyKey } }),
    search: (q: string) => client.GET("/api/v1/workspaces/{workspaceId}/semantic-search", { params: { ...params, query: { q } } }),
    plan: (planId: string) => client.GET("/api/v1/workspaces/{workspaceId}/resolved-semantic-plans/{planId}", { params: { path: { workspaceId: options.workspaceId, planId } } }),
    query: (queryId: string) => client.GET("/api/v1/workspaces/{workspaceId}/semantic-queries/{queryId}", { params: { path: { workspaceId: options.workspaceId, queryId } } }),
    execute: (planId: string, planDigest: string, idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/resolved-semantic-plans/{planId}:execute", { params: { path: { workspaceId: options.workspaceId, planId } }, body: { planId, planDigest, idempotencyKey, channel: "sdk" } }),
    execution: (runId: string) => client.GET("/api/v1/workspaces/{workspaceId}/query-executions/{runId}", { params: { path: { workspaceId: options.workspaceId, runId } } }),
    cancelExecution: (runId: string) => client.POST("/api/v1/workspaces/{workspaceId}/query-executions/{runId}:cancel", { params: { path: { workspaceId: options.workspaceId, runId } } }),
  };
}

export function createProductionOperationClient(options: SemliaClientOptions & { workspaceId: string }) {
  const client = createSemliaClient(options);
  const workspacePath = { workspaceId: options.workspaceId };
  type Page = { limit?: number; cursor?: string; sourceId?: string; candidateId?: string; createdBy?: string };
  return {
    list: (query: Page = {}) =>
      client.GET("/api/v1/workspaces/{workspaceId}/production-operations", { params: { path: workspacePath, query } }),
    create: (body: components["schemas"]["CreateProductionRequest"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/production-operations", {
        params: { path: workspacePath, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
    get: (operationId: string, version?: number) =>
      client.GET("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}", {
        params: { path: { ...workspacePath, operationId }, query: version !== undefined ? { version } : {} },
      }),
    replace: (operationId: string, body: components["schemas"]["ReplaceProductionRequest"], idempotencyKey: string) =>
      client.PUT("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}", {
        params: { path: { ...workspacePath, operationId }, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
    submit: (operationId: string, body: components["schemas"]["SubmitProductionRequest"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/submit", {
        params: { path: { ...workspacePath, operationId }, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
    listValidations: (operationId: string, query: { version: number; limit?: number; cursor?: string }) =>
      client.GET("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/validations", {
        params: { path: { ...workspacePath, operationId }, query },
      }),
    validate: (operationId: string, body: components["schemas"]["ValidateProductionRequest"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/validations", {
        params: { path: { ...workspacePath, operationId }, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
    review: (operationId: string, body: components["schemas"]["ReviewProductionRequest"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/reviews", {
        params: { path: { ...workspacePath, operationId }, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
    recordBusinessRule: (operationId: string, body: components["schemas"]["ProductionBusinessRuleRequest"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/business-rule-confirmations", {
        params: { path: { ...workspacePath, operationId }, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
    businessRules: (operationId: string, version: number) =>
      client.GET("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/business-rule-confirmations", {
        params: { path: { ...workspacePath, operationId }, query: { version } },
      }),
    generate: (operationId: string, body: components["schemas"]["GenerateProductionRequest"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/generation", {
        params: { path: { ...workspacePath, operationId }, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
    generation: (operationId: string, runId: string) =>
      client.GET("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/generation/{runId}", {
        params: { path: { ...workspacePath, operationId, runId } },
      }),
    publish: (operationId: string, body: components["schemas"]["PublishProductionRequest"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/production-operations/{operationId}/publish", {
        params: { path: { ...workspacePath, operationId }, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
  };
}

export function createProductionReleaseClient(options: SemliaClientOptions & { workspaceId: string }) {
  const client = createSemliaClient(options);
  const workspacePath = { workspaceId: options.workspaceId };
  return {
    get: (releaseId: string) =>
      client.GET("/api/v1/workspaces/{workspaceId}/production-releases/{releaseId}", {
        params: { path: { ...workspacePath, releaseId } },
      }),
    rollback: (releaseId: string, body: components["schemas"]["RollbackProductionRequest"], idempotencyKey: string) =>
      client.POST("/api/v1/workspaces/{workspaceId}/production-releases/{releaseId}/rollback", {
        params: { path: { ...workspacePath, releaseId }, header: { "Idempotency-Key": idempotencyKey } },
        body,
      }),
  };
}

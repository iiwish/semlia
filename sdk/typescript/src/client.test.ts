import { createSemliaClient, createSourceSnapshotClient, createProductionOperationClient, createProductionReleaseClient } from "./client";
import type { components, operations } from "./schema.gen";

const client = createSemliaClient({ baseUrl: "http://localhost:8080" });
const snapshotClient = createSourceSnapshotClient({ baseUrl: "http://localhost:8080", workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav", sourceId: "src_01arz3ndektsv4rrffq69g5fav" });
void snapshotClient.list({ limit: 200 });
void snapshotClient.get("ssnp_01arz3ndektsv4rrffq69g5fav");
void snapshotClient.members("ssnp_01arz3ndektsv4rrffq69g5fav", { kind: "field", cursor: "opaque" });
void snapshotClient.diagnostics("ssnp_01arz3ndektsv4rrffq69g5fav", { limit: 50 });
const actorClient = createSemliaClient({
  baseUrl: "http://localhost:8080",
  headers: { "X-Semlia-Principal": "local-reviewer" },
});
const sessionClient = createSemliaClient({
  baseUrl: "http://localhost:8080",
  getCSRFToken: () => "csrf-verifier",
  onUnauthorized: () => undefined,
  onForbidden: () => undefined,
});
const livenessRequest = client.GET("/health/live");
const actorLivenessRequest = actorClient.GET("/health/live");
const sessionRequest = sessionClient.GET("/api/v1/session");
const semanticQueryRequest: components["schemas"]["ResolveSemanticQueryRequest"] = {
  query: {
    schemaVersion: "1.0.0",
    intent: "breakdown",
    measures: [{ address: "commerce.net_revenue" }],
    dimensions: [{ search: "country" }],
    context: { mode: "current" },
  },
  channel: "api",
  idempotencyKey: "report-preview-1",
};
const semanticQueryResponse = sessionClient.POST(
  "/api/v1/workspaces/{workspaceId}/semantic-queries:resolve",
  {
    params: {
      path: { workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" },
      header: { "X-Semlia-CSRF": "csrf-verifier" },
    },
    body: semanticQueryRequest,
  },
);
const consumersRequest = sessionClient.GET(
  "/api/v1/workspaces/{workspaceId}/consumers",
  { params: { path: { workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" } } },
);
const bindingRequest = sessionClient.POST(
  "/api/v1/workspaces/{workspaceId}/consumer-bindings",
  {
    params: {
      path: { workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" },
      header: { "X-Semlia-CSRF": "csrf-verifier" },
    },
    body: {
      consumerId: "csm_01arz3ndektsv4rrffq69g5fav",
      environment: "prod",
      purpose: "stable reporting",
      mode: "current",
      compatibilityConstraint: {},
    },
  },
);
const askRequest = sessionClient.POST(
  "/api/v1/workspaces/{workspaceId}/ask",
  {
    params: {
      path: { workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" },
      header: { "X-Semlia-CSRF": "csrf-verifier" },
    },
    body: {
      question: "What is the released definition of net revenue?",
      context: { mode: "current" },
      idempotencyKey: "ask-1",
    },
  },
);
const auditRequest = sessionClient.GET(
  "/api/v1/workspaces/{workspaceId}/operations/audit-events",
  {
    params: {
      path: { workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" },
      query: { eventType: "source.tested", limit: 50 },
    },
  },
);
const runtimeRequest = sessionClient.GET(
  "/api/v1/workspaces/{workspaceId}/operations/runs",
  {
    params: {
      path: { workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" },
      query: { kind: "semantic_resolution", state: "succeeded" },
    },
  },
);
const runtimePolicyRequest = sessionClient.PATCH(
  "/api/v1/workspaces/{workspaceId}/runtime-policy",
  {
    params: {
      path: { workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" },
      header: { "X-Semlia-CSRF": "csrf-verifier" },
    },
    body: {
      expectedVersion: 1,
      retryCeiling: 3,
      statementTimeoutMs: 30_000,
      webhookTimeoutMs: 10_000,
      queryRowLimit: 10_000,
      queryByteLimit: 10 * 1024 * 1024,
      runMetadataRetentionDays: 30,
    },
  },
);

const errorResponse: components["schemas"]["ErrorResponse"] = {
  code: "DEPENDENCY_UNAVAILABLE",
  message: "database unavailable",
  traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
  details: {},
};

const eventEnvelope: components["schemas"]["EventEnvelope"] = {
  specVersion: "semlia.events/v1",
  id: "event_01ARZ3NDEKTSV4RRFFQ69G5FAV",
  type: "system.readiness.changed",
  source: "urn:semlia:control-plane",
  time: "2026-08-08T08:00:00Z",
  traceId: "4bf92f3577b34da6a3ce929d0e0e4736",
  data: { ready: true },
};

type ReadinessResponses = operations["getReadiness"]["responses"];

void livenessRequest;
void actorLivenessRequest;
void sessionRequest;
void semanticQueryResponse;
void consumersRequest;
void bindingRequest;
void askRequest;
void auditRequest;
void runtimeRequest;
void runtimePolicyRequest;
void errorResponse;
void eventEnvelope;
void (null as unknown as ReadinessResponses);

const prodClient = createProductionOperationClient({ baseUrl: "http://localhost:8080", workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" });
void prodClient.list();
void prodClient.get("prodop_01arz3ndektsv4rrffq69g5fav");
void prodClient.listValidations("prodop_01arz3ndektsv4rrffq69g5fav", { version: 1 });
void prodClient.submit("prodop_01arz3ndektsv4rrffq69g5fav", { expectedVersion: 1, setDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000" }, "submit-1");
void prodClient.validate("prodop_01arz3ndektsv4rrffq69g5fav", { expectedVersion: 1, setDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", previousAttemptNo: 1, reason: "recheck" }, "val-1");
void prodClient.review("prodop_01arz3ndektsv4rrffq69g5fav", { expectedVersion: 1, setDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", validation: { attemptNo: 1, validationDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000" }, proposalIds: ["prp_01arz3ndektsv4rrffq69g5fav"], decision: "approve", note: "lgtm" }, "rev-1");
void prodClient.publish("prodop_01arz3ndektsv4rrffq69g5fav", { expectedVersion: 1, setDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", validation: { attemptNo: 1, validationDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000" }, expectedHead: { presence: "absent" } }, "pub-1");

const prodRelClient = createProductionReleaseClient({ baseUrl: "http://localhost:8080", workspaceId: "wsp_01arz3ndektsv4rrffq69g5fav" });
void prodClient.generate("prodop_01arz3ndektsv4rrffq69g5fav", {
  expectedVersion: 1,
  inputDigest: "sha256:" + "0".repeat(64),
  modelSettingId: "mds_01arz3ndektsv4rrffq69g5fav",
  modelConfigRevision: "sha256:" + "1".repeat(64),
  instruction: "Suggest unresolved definitions from pinned evidence",
  maxOutputTokens: 1024,
  maxCostMicros: 2000000,
}, "generate-1");
void prodClient.generation("prodop_01arz3ndektsv4rrffq69g5fav", "agr_01arz3ndektsv4rrffq69g5fav");
void prodClient.recordBusinessRule("prodop_01arz3ndektsv4rrffq69g5fav", {
  expectedVersion: 1, setDigest: "sha256:" + "0".repeat(64), targetKey: "revenue",
  action: "confirm", evidenceId: "evd_01arz3ndektsv4rrffq69g5fav",
}, "confirm-rule-1");
void prodClient.recordBusinessRule("prodop_01arz3ndektsv4rrffq69g5fav", {
  expectedVersion: 1, setDigest: "sha256:" + "0".repeat(64), targetKey: "revenue", action: "revoke",
}, "revoke-rule-1");
void prodClient.businessRules("prodop_01arz3ndektsv4rrffq69g5fav", 1);
void prodClient.recordBusinessRule("prodop_01arz3ndektsv4rrffq69g5fav", {
  expectedVersion: 1, setDigest: "sha256:" + "0".repeat(64), targetKey: "revenue", action: "confirm",
  declaration: "I declare this synthetic revenue definition and scope as the authorized business owner.",
}, "declare-rule-1");
// @ts-expect-error Confirm requires evidence, not just non-empty generated text.
void prodClient.recordBusinessRule("prodop_01arz3ndektsv4rrffq69g5fav", { expectedVersion: 1, setDigest: "sha256:" + "0".repeat(64), targetKey: "revenue", action: "confirm" }, "missing-evidence");
void prodRelClient.get("rls_01arz3ndektsv4rrffq69g5fav");
void prodRelClient.rollback("rls_01arz3ndektsv4rrffq69g5fav", { expectedVersion: 1, setDigest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", expectedHead: { presence: "absent" }, reason: "rollback test" }, "rb-1");

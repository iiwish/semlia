import { createSemliaClient } from "./client";
import type { components, operations } from "./schema.gen";

const client = createSemliaClient({ baseUrl: "http://localhost:8080" });
const livenessRequest = client.GET("/health/live");

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
void errorResponse;
void eventEnvelope;
void (null as unknown as ReadinessResponses);

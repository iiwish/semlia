import { beforeEach, describe, expect, it, vi } from "vitest";

import { clearSessionCSRFToken } from "./apiClient";
import { getSession } from "./identity";
import {
  createIngestionIdempotencyKey,
  createSourceSchedule,
  finalizeArtifactSet,
  listIngestionArtifacts,
  runSourceScheduleNow,
  stageIngestionArtifact,
} from "./ingestion";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const sourceId = "src_01arz3ndektsv4rrffq69g5fav";
const scheduleId = "sch_01arz3ndektsv4rrffq69g5fav";

describe("ingestion API client", () => {
  beforeEach(() => {
    clearSessionCSRFToken();
    vi.unstubAllGlobals();
  });

  it("maps the authoritative artifact page without inventing preview fields", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({
      items: [{
        id: "art_01arz3ndektsv4rrffq69g5fav",
        kind: "csv",
        schemaVersion: "semlia-artifact-v1",
        contentDigest: "a".repeat(64),
        byteSize: 128,
        mediaType: "text/csv",
        originalName: "orders.csv",
        status: "validated",
        validationSummary: { adapterKind: "file_catalog", adapterVersion: "v1", datasetCount: 1, fieldCount: 4, codeArtifactCount: 0, lineageCount: 0, keyCount: 1, joinCount: 0, findingCount: 0 },
        createdAt: "2026-09-05T00:00:00Z",
      }],
      total: 2,
      limit: 1,
      nextCursor: "cursor-2",
    }));
    vi.stubGlobal("fetch", fetchMock);

    const page = await listIngestionArtifacts(workspaceId, { kind: "csv", status: "validated" });

    expect(page.total).toBe(2);
    expect(page.nextCursor).toBe("cursor-2");
    expect(page.items[0].validationSummary?.fieldCount).toBe(4);
    expect(fetchMock.mock.calls[0][0].url).toContain("kind=csv");
  });

  it("sends raw file bytes with exact type headers, CSRF and idempotency", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ account: { id: "usr_test", displayName: "Test" }, workspaces: [], expiresAt: "2026-09-06T00:00:00Z", traceId: "tr_session" }, 200, { "X-Semlia-CSRF": "csrf-ingestion-tests-123456" }))
      .mockResolvedValueOnce(jsonResponse({ id: "art_01arz3ndektsv4rrffq69g5fav", kind: "csv", schemaVersion: "semlia-artifact-v1", byteSize: 8, mediaType: "text/csv", originalName: "orders.csv", status: "validated", createdAt: "2026-09-05T00:00:00Z" }, 201));
    vi.stubGlobal("fetch", fetchMock);
    await getSession();
    const file = new File(["id,name\n"], "orders.csv", { type: "text/csv" });

    await stageIngestionArtifact(workspaceId, file, "csv", "idem-upload-1");

    const request = fetchMock.mock.calls[1][0] as Request;
    expect(request.headers.get("Content-Type")).toBe("text/csv");
    expect(request.headers.get("X-Artifact-Kind")).toBe("csv");
    expect(request.headers.get("X-File-Name")).toBe("orders.csv");
    expect(request.headers.get("Idempotency-Key")).toBe("idem-upload-1");
    expect(request.headers.get("X-Semlia-CSRF")).toBe("csrf-ingestion-tests-123456");
    expect(await request.text()).toBe("id,name\n");
  });

  it("uses server-confirmed finalize and schedule command routes", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ account: { id: "usr_test", displayName: "Test" }, workspaces: [], expiresAt: "2026-09-06T00:00:00Z", traceId: "tr_session" }, 200, { "X-Semlia-CSRF": "csrf-ingestion-tests-123456" }))
      .mockResolvedValueOnce(jsonResponse(artifactSet(), 201))
      .mockResolvedValueOnce(jsonResponse(schedule(), 201))
      .mockResolvedValueOnce(jsonResponse(occurrence(), 202));
    vi.stubGlobal("fetch", fetchMock);
    await getSession();

    await finalizeArtifactSet(workspaceId, { sourceName: "Orders files", artifactIds: ["art_01arz3ndektsv4rrffq69g5fav"] }, "idem-finalize-1");
    await createSourceSchedule(workspaceId, sourceId, { expression: "0 2 * * *", timezone: "Asia/Shanghai", misfirePolicy: "run_once" }, "idem-schedule-1");
    const result = await runSourceScheduleNow(workspaceId, scheduleId, 3, "idem-run-1");

    expect(result.operationsPath).toBe("/operations/runtime?run=run_01arz3ndektsv4rrffq69g5fav");
    expect(fetchMock.mock.calls.slice(1).map(([request]) => (request as Request).url)).toEqual([
      expect.stringContaining("/ingestion/artifact-sets:finalize"),
      expect.stringContaining(`/sources/${sourceId}/schedules`),
      expect.stringContaining(`/schedules/${scheduleId}:run-now`),
    ]);
  });

  it("creates non-empty replay keys", () => {
    expect(createIngestionIdempotencyKey("upload")).toMatch(/^upload-/);
  });
});

function artifactSet() {
  return { id: "ars_01arz3ndektsv4rrffq69g5fav", sourceId, sourceKind: "file", setDigest: "b".repeat(64), members: [], createdAt: "2026-09-05T00:00:00Z" };
}

function schedule() {
  return { id: scheduleId, sourceId, expression: "0 2 * * *", timezone: "Asia/Shanghai", misfirePolicy: "run_once", enabled: true, version: 3, createdAt: "2026-09-05T00:00:00Z", updatedAt: "2026-09-05T00:00:00Z" };
}

function occurrence() {
  return { id: "occ_01arz3ndektsv4rrffq69g5fav", scheduleId, sourceId, triggerKind: "run_now", scheduleVersion: 3, eligibleAt: "2026-09-05T00:00:00Z", wallClockKey: "manual", state: "enqueued", runtimeRunId: "run_01arz3ndektsv4rrffq69g5fav", operationsPath: "/operations/runtime?run=run_01arz3ndektsv4rrffq69g5fav", idempotencyKey: "idem-run-1", createdAt: "2026-09-05T00:00:00Z" };
}

function jsonResponse(body: unknown, status = 200, headers: Record<string, string> = {}) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });
}

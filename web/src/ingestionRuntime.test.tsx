import { act, render, screen, waitFor } from "@testing-library/react";
import { useEffect } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { IngestionApi, IngestionArtifact, SourceSchedule } from "./ingestion";
import { IngestionRuntimeProvider, useIngestionRuntime } from "./ingestionRuntime";

const workspaceId = "wsp_01arz3ndektsv4rrffq69g5fav";
const sourceId = "src_01arz3ndektsv4rrffq69g5fav";

describe("IngestionRuntimeProvider", () => {
  beforeEach(() => vi.restoreAllMocks());

  it("keeps read and manage capabilities partitioned", async () => {
    const api = makeApi();
    render(<IngestionRuntimeProvider workspaceId={workspaceId} access={{ read: false, manage: true, run: false }} api={api}><Probe /></IngestionRuntimeProvider>);

    expect(await screen.findByTestId("artifact-state")).toHaveTextContent("forbidden");
    expect(api.listArtifacts).not.toHaveBeenCalled();
    await act(async () => { await useProbe().current?.registerSQLSource({ sourceName: "SQL bundle", paths: ["models/orders.sql"] }); });
    expect(api.registerSQL).toHaveBeenCalled();
  });

  it("retains a server-confirmed source when the follow-up list refresh fails", async () => {
    const api = makeApi({
      listArtifacts: vi.fn().mockResolvedValueOnce({ items: [], total: 0, limit: 50 }).mockRejectedValueOnce(new Error("artifact refresh unavailable")),
    });
    render(<IngestionRuntimeProvider workspaceId={workspaceId} access={{ read: true, manage: true, run: false }} api={api}><Probe /></IngestionRuntimeProvider>);
    await waitFor(() => expect(screen.getByTestId("artifact-state")).toHaveTextContent("empty:0"));

    await act(async () => {
      await useProbe().current?.createArtifactSource({ sourceName: "Orders CSV", files: [{ file: new File(["id\n1\n"], "orders.csv", { type: "text/csv" }), kind: "csv" }] });
    });

    expect(screen.getByTestId("confirmed-set")).toHaveTextContent("ars_01arz3ndektsv4rrffq69g5fav");
    expect(screen.getByRole("status")).toHaveTextContent("artifact refresh unavailable");
  });

  it("retains server-confirmed schedule mutations and exact run occurrence", async () => {
    const api = makeApi({
      listSchedules: vi.fn().mockResolvedValueOnce({ items: [], total: 0, limit: 50 }).mockRejectedValueOnce(new Error("schedule refresh unavailable")),
    });
    render(<IngestionRuntimeProvider workspaceId={workspaceId} access={{ read: true, manage: true, run: true }} api={api}><Probe loadSource={sourceId} /></IngestionRuntimeProvider>);
    await waitFor(() => expect(api.listSchedules).toHaveBeenCalledTimes(1));

    let created: SourceSchedule | undefined;
    await act(async () => {
      created = await useProbe().current?.createSchedule(sourceId, { expression: "0 2 * * *", timezone: "Asia/Shanghai", misfirePolicy: "run_once" });
    });
    expect(created?.version).toBe(3);
    expect(screen.getByTestId("schedule-count")).toHaveTextContent("1");
    expect(screen.getByRole("status")).toHaveTextContent("schedule refresh unavailable");

    await act(async () => { await useProbe().current?.runScheduleNow(created!); });
    expect(screen.getByTestId("last-occurrence")).toHaveTextContent("/operations/runtime?run=run_01arz3ndektsv4rrffq69g5fav");
  });

  it("does not replace production errors with fixture records", async () => {
    const api = makeApi({ listArtifacts: vi.fn().mockRejectedValue(new Error("artifact storage unreachable")) });
    render(<IngestionRuntimeProvider workspaceId={workspaceId} access={{ read: true, manage: false, run: false }} api={api}><Probe /></IngestionRuntimeProvider>);

    await waitFor(() => expect(screen.getByTestId("artifact-state")).toHaveTextContent("error:artifact storage unreachable"));
    expect(screen.queryByText("Orders CSV")).not.toBeInTheDocument();
  });

  it("reuses staged identities and command keys after a committed response is lost", async () => {
    const api = makeApi();
    const originalFinalize = api.finalizeArtifactSet;
    api.finalizeArtifactSet = vi.fn().mockRejectedValueOnce(new Error("response lost")).mockImplementation(originalFinalize);
    const originalCreate = api.createSchedule;
    api.createSchedule = vi.fn().mockRejectedValueOnce(new Error("response lost")).mockImplementation(originalCreate);
    render(<IngestionRuntimeProvider workspaceId={workspaceId} access={{ read: true, manage: true, run: true }} api={api}><Probe /></IngestionRuntimeProvider>);
    const input = { sourceName: "Orders CSV", files: [{ file: new File(["id\n1\n"], "orders.csv"), kind: "csv" as const }] };
    await act(async () => { await expect(useProbe().current!.createArtifactSource(input)).rejects.toThrow("response lost"); });
    await act(async () => { await useProbe().current!.createArtifactSource(input); });
    expect(api.stageArtifact).toHaveBeenCalledTimes(1);
    const finalizeCalls = vi.mocked(api.finalizeArtifactSet).mock.calls;
    expect(finalizeCalls[1][2]).toBe(finalizeCalls[0][2]);
    const schedule = { expression: "0 2 * * *", timezone: "Asia/Shanghai", misfirePolicy: "skip" as const };
    await act(async () => { await expect(useProbe().current!.createSchedule(sourceId, schedule)).rejects.toThrow("response lost"); });
    await act(async () => { await useProbe().current!.createSchedule(sourceId, schedule); });
    const scheduleCalls = vi.mocked(api.createSchedule).mock.calls;
    expect(scheduleCalls[1][3]).toBe(scheduleCalls[0][3]);
  });
});

const probeRef: { current: ReturnType<typeof useIngestionRuntime> | null } = { current: null };
function useProbe() { return probeRef; }

function Probe({ loadSource }: { loadSource?: string }) {
  const runtime = useIngestionRuntime();
  const loadSchedules = runtime.loadSchedules;
  useEffect(() => {
    probeRef.current = runtime;
    return () => { probeRef.current = null; };
  }, [runtime]);
  useEffect(() => {
    if (loadSource) void loadSchedules(loadSource).catch(() => undefined);
  }, [loadSchedules, loadSource]);
  const schedules = loadSource ? runtime.schedulesBySource[loadSource]?.items ?? [] : [];
  return <>
    <span data-testid="artifact-state">{runtime.artifactStatus.state}:{runtime.artifactStatus.error || runtime.artifacts.length}</span>
    <span data-testid="confirmed-set">{runtime.confirmedArtifactSet?.id ?? "none"}</span>
    <span data-testid="schedule-count">{schedules.length}</span>
    <span data-testid="last-occurrence">{runtime.lastOccurrence?.operationsPath ?? "none"}</span>
    {runtime.refreshWarning && <span role="status">{runtime.refreshWarning}</span>}
  </>;
}

function makeApi(overrides: Partial<IngestionApi> = {}): IngestionApi {
  const artifact: IngestionArtifact = { id: "art_01arz3ndektsv4rrffq69g5fav", kind: "csv", schemaVersion: "semlia-artifact-v1", contentDigest: "a".repeat(64), byteSize: 5, mediaType: "text/csv", originalName: "orders.csv", status: "validated", contentAvailability: "available", createdAt: "2026-09-05T00:00:00Z" };
  const schedule: SourceSchedule = { id: "sch_01arz3ndektsv4rrffq69g5fav", sourceId, expression: "0 2 * * *", timezone: "Asia/Shanghai", misfirePolicy: "run_once", enabled: true, version: 3, createdAt: "2026-09-05T00:00:00Z", updatedAt: "2026-09-05T00:00:00Z" };
  return {
    listArtifacts: vi.fn().mockResolvedValue({ items: [], total: 0, limit: 50 }),
    stageArtifact: vi.fn().mockResolvedValue(artifact),
    getArtifactSet: vi.fn(),
    finalizeArtifactSet: vi.fn().mockResolvedValue({ id: "ars_01arz3ndektsv4rrffq69g5fav", sourceId, sourceKind: "file", setDigest: "b".repeat(64), members: [], createdAt: "2026-09-05T00:00:00Z" }),
    registerSQL: vi.fn().mockResolvedValue({ id: "ars_01arz3ndektsv4rrffq69g5fav", sourceId, sourceKind: "sql_bundle", setDigest: "b".repeat(64), members: [], createdAt: "2026-09-05T00:00:00Z" }),
    listSchedules: vi.fn().mockResolvedValue({ items: [], total: 0, limit: 50 }),
    getSchedule: vi.fn().mockResolvedValue(schedule),
    createSchedule: vi.fn().mockResolvedValue(schedule),
    updateSchedule: vi.fn().mockResolvedValue(schedule),
    deleteSchedule: vi.fn().mockResolvedValue({ ...schedule, deletedAt: "2026-09-05T00:00:00Z" }),
    pauseSchedule: vi.fn().mockResolvedValue({ ...schedule, enabled: false, version: 4 }),
    resumeSchedule: vi.fn().mockResolvedValue({ ...schedule, enabled: true, version: 4 }),
    runScheduleNow: vi.fn().mockResolvedValue({ id: "occ_01arz3ndektsv4rrffq69g5fav", scheduleId: schedule.id, sourceId, triggerKind: "run_now", scheduleVersion: 3, eligibleAt: "2026-09-05T00:00:00Z", wallClockKey: "manual", state: "enqueued", runtimeRunId: "run_01arz3ndektsv4rrffq69g5fav", operationsPath: "/operations/runtime?run=run_01arz3ndektsv4rrffq69g5fav", idempotencyKey: "idem", createdAt: "2026-09-05T00:00:00Z" }),
    listOccurrences: vi.fn().mockResolvedValue({ items: [], total: 0, limit: 50 }),
    ...overrides,
  };
}

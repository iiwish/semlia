import { beforeEach, describe, expect, it } from "vitest";
import { readCandidateRecovery, withCandidateRecoveryLock, writeCandidateRecovery, type CandidateRecovery } from "./candidateRecovery";

const record: CandidateRecovery = { stage: "submitted", proposalId: "prp_one", assetId: "asset_one", definition: "Definition", reason: "Reviewed", digest: "sha256:one" };
beforeEach(() => localStorage.clear());

describe("candidate recovery checkpoints", () => {
  it("isolates workspaces, principals and candidates", () => {
    writeCandidateRecovery("workspace:actor", "candidate", record);
    expect(readCandidateRecovery("workspace:actor", "candidate")).toEqual(record);
    expect(readCandidateRecovery("other:actor", "candidate")).toBeNull();
    expect(readCandidateRecovery("workspace:other", "candidate")).toBeNull();
    expect(readCandidateRecovery("workspace:actor", "other")).toBeNull();
  });

  it("rejects malformed records rather than allowing another creation", () => {
    writeCandidateRecovery("workspace:actor", "candidate", record);
    const key = localStorage.key(0)!;
    localStorage.setItem(key, JSON.stringify({ stage: "submitted" }));
    expect(() => readCandidateRecovery("workspace:actor", "candidate")).toThrow();
    localStorage.setItem(key, "not JSON");
    expect(() => readCandidateRecovery("workspace:actor", "candidate")).toThrow();
  });

  it("serializes attempts so the second operation sees the first checkpoint", async () => {
    let release!: () => void;
    const pending = new Promise<void>((resolve) => { release = resolve; });
    const first = withCandidateRecoveryLock("workspace:actor", "candidate", async () => { await pending; writeCandidateRecovery("workspace:actor", "candidate", record); });
    const second = withCandidateRecoveryLock("workspace:actor", "candidate", async () => readCandidateRecovery("workspace:actor", "candidate"));
    release();
    await first;
    expect(await second).toEqual(record);
  });
});
